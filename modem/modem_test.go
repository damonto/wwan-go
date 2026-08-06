package modem

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type lifecycleBackend struct {
	unsupportedBackend
	mu         sync.Mutex
	closeCount int
}

type statusWatchBackend struct {
	unsupportedBackend
	stream chan Result[Status]
}

func (b *statusWatchBackend) WatchStatus(context.Context) (<-chan Result[Status], error) {
	return b.stream, nil
}

func (b *lifecycleBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeCount++
	return nil
}

type lifecycleSession struct {
	mu                       sync.Mutex
	value                    BearerInfo
	disconnectErr            error
	disconnectKeepsConnected bool
	closeCount               int
}

type connectBackend struct {
	unsupportedBackend
	sim          SIMState
	power        PowerState
	network      NetworkStatus
	steps        []string
	session      *lifecycleSession
	setCaps      Technology
	statusStream chan Result[Status]
}

func (b *connectBackend) SIMInfo(context.Context) (SIMInfo, error) {
	b.steps = append(b.steps, "read-sim")
	return SIMInfo{State: b.sim}, nil
}

func (b *connectBackend) SendPIN(context.Context, string) error {
	b.steps = append(b.steps, "send-pin")
	b.sim = SIMStateReady
	return nil
}

func (b *connectBackend) PowerState(context.Context) (PowerState, error) {
	b.steps = append(b.steps, "read-power")
	return b.power, nil
}

func (b *connectBackend) SetPowerState(_ context.Context, state PowerState) error {
	b.steps = append(b.steps, "set-power")
	b.power = state
	return nil
}

func (b *connectBackend) NetworkStatus(context.Context) (NetworkStatus, error) {
	b.steps = append(b.steps, "read-network")
	return b.network, nil
}

func (b *connectBackend) Register(_ context.Context, cfg RegisterConfig) error {
	b.steps = append(b.steps, "register")
	b.network.Registration = RegistrationHome
	b.network.OperatorID = cfg.OperatorID
	return nil
}

func (b *connectBackend) SetPacketServiceState(_ context.Context, state PacketServiceState) error {
	b.steps = append(b.steps, "attach")
	b.network.PacketService = state
	return nil
}

func (b *connectBackend) Connect(context.Context, ConnectConfig) (sessionBackend, error) {
	b.steps = append(b.steps, "connect")
	return b.session, nil
}

func (b *connectBackend) SetCapabilities(_ context.Context, technologies Technology) error {
	b.setCaps = technologies
	return nil
}

func (b *connectBackend) WatchStatus(ctx context.Context) (<-chan Result[Status], error) {
	if b.statusStream == nil {
		return b.unsupportedBackend.WatchStatus(ctx)
	}
	return b.statusStream, nil
}

func (s *lifecycleSession) Info() BearerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.value
	value.Network = cloneNetworkConfig(value.Network)
	return value
}

func (s *lifecycleSession) Stats(context.Context) (BearerStats, error) {
	return BearerStats{}, nil
}

func (s *lifecycleSession) Watch(ctx context.Context) (<-chan Result[BearerEvent], error) {
	out := make(chan Result[BearerEvent])
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}

func (s *lifecycleSession) Disconnect(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCount++
	if s.disconnectErr == nil || !s.disconnectKeepsConnected {
		s.value.Connected = false
	}
	return s.disconnectErr
}

func (s *lifecycleSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCount++
	s.value.Connected = false
	return nil
}

func TestModemPowerState(t *testing.T) {
	tests := []struct {
		name    string
		power   PowerState
		closed  bool
		want    PowerState
		wantErr error
	}{
		{name: "reads backend state", power: PowerStateLow, want: PowerStateLow},
		{name: "rejects closed modem", power: PowerStateOn, closed: true, want: PowerStateUnknown, wantErr: ErrClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &connectBackend{power: tt.power}
			modem := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, backend)
			if tt.closed {
				if err := modem.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
			got, err := modem.PowerState(t.Context())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("PowerState() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("PowerState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModemClose(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "repeated close releases owned resources once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &lifecycleBackend{}
			session := &lifecycleSession{value: BearerInfo{Connected: true}}
			m := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, backend)
			bearer := &Bearer{id: 1, modem: m, session: session, done: make(chan struct{})}
			m.bearers[1] = bearer

			if err := m.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := m.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
			if backend.closeCount != 1 || session.closeCount != 1 {
				t.Errorf("close counts = backend %d session %d, want 1 and 1", backend.closeCount, session.closeCount)
			}
			if _, err := m.Info(context.Background()); !errors.Is(err, ErrClosed) {
				t.Errorf("Info() error = %v, want ErrClosed", err)
			}
		})
	}
}

func TestBearerCloseStopsWatch(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "close cancels active watch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &lifecycleBackend{}
			m := newModem(Port{Type: PortMBIM, Path: "/dev/test"}, AccessDirect, backend)
			bearer := &Bearer{id: 1, modem: m, session: &lifecycleSession{}, done: make(chan struct{})}
			m.bearers[1] = bearer
			stream, err := bearer.Watch(context.Background())
			if err != nil {
				t.Fatalf("Watch() error = %v", err)
			}
			if err := bearer.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if _, ok := <-stream; ok {
				t.Fatal("watch stream remained open after Close()")
			}
		})
	}
}

func TestBearerDisconnectFinalizesTerminalSessionError(t *testing.T) {
	errDisconnect := errors.New("disconnect rejected")
	tests := []struct {
		name                     string
		disconnectKeepsConnected bool
		wantOwned                bool
		wantClosed               bool
	}{
		{name: "retires disconnected session", wantClosed: true},
		{name: "keeps connected session for retry", disconnectKeepsConnected: true, wantOwned: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &lifecycleSession{
				value:                    BearerInfo{Connected: true},
				disconnectErr:            errDisconnect,
				disconnectKeepsConnected: tt.disconnectKeepsConnected,
			}
			modem := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, &lifecycleBackend{})
			bearer := &Bearer{id: 1, modem: modem, session: session, done: make(chan struct{})}
			modem.bearers[1] = bearer

			err := bearer.Disconnect(t.Context())
			if !errors.Is(err, errDisconnect) {
				t.Fatalf("Disconnect() error = %v, want %v", err, errDisconnect)
			}
			if _, owned := modem.Bearer(1); owned != tt.wantOwned {
				t.Fatalf("modem owns bearer = %t, want %t", owned, tt.wantOwned)
			}
			if bearer.closed != tt.wantClosed {
				t.Fatalf("bearer closed = %t, want %t", bearer.closed, tt.wantClosed)
			}
			select {
			case <-bearer.done:
				if !tt.wantClosed {
					t.Fatal("retryable bearer lifetime was closed")
				}
			default:
				if tt.wantClosed {
					t.Fatal("terminal bearer lifetime remains open")
				}
			}
		})
	}
}

func TestBearerInfoReturnsNetworkCopy(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "network slices are independent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &lifecycleSession{value: BearerInfo{Network: NetworkConfig{
				Addresses: []netip.Prefix{netip.MustParsePrefix("192.0.2.2/24")},
				Gateways:  []netip.Addr{netip.MustParseAddr("192.0.2.1")},
				DNS:       []netip.Addr{netip.MustParseAddr("1.1.1.1")},
			}}}
			bearer := &Bearer{id: 7, session: session}
			first := bearer.Info()
			first.Network.Addresses[0] = netip.MustParsePrefix("198.51.100.2/24")
			first.Network.Gateways[0] = netip.MustParseAddr("198.51.100.1")
			first.Network.DNS[0] = netip.MustParseAddr("9.9.9.9")
			second := bearer.Info()
			if second.ID != 7 || second.Network.Addresses[0].String() != "192.0.2.2/24" ||
				second.Network.Gateways[0].String() != "192.0.2.1" || second.Network.DNS[0].String() != "1.1.1.1" {
				t.Errorf("second Info() = %+v", second)
			}
		})
	}
}

func TestWatchStatusIncludesOwnedBearers(t *testing.T) {
	tests := []struct {
		name        string
		bearerCount int
	}{
		{name: "no bearers"},
		{name: "active bearers", bearerCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &statusWatchBackend{stream: make(chan Result[Status], 1)}
			m := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, backend)
			for id := range tt.bearerCount {
				bearerID := uint64(id + 1)
				m.bearers[bearerID] = &Bearer{id: bearerID}
			}

			stream, err := m.WatchStatus(t.Context())
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			backend.stream <- Result[Status]{Value: Status{Power: PowerStateOn}}

			result := <-stream
			if result.Err != nil {
				t.Fatalf("status result error = %v", result.Err)
			}
			if result.Value.OwnBearers != tt.bearerCount {
				t.Fatalf("OwnBearers = %d, want %d", result.Value.OwnBearers, tt.bearerCount)
			}
		})
	}
}

func TestWatchStatusTracksBearerChanges(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "connect and disconnect emit status updates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &connectBackend{
				sim:          SIMStateReady,
				power:        PowerStateOn,
				network:      NetworkStatus{Registration: RegistrationHome, PacketService: PacketServiceAttached},
				session:      &lifecycleSession{value: BearerInfo{Connected: true}},
				statusStream: make(chan Result[Status], 1),
			}
			m := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, backend)
			stream, err := m.WatchStatus(t.Context())
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}

			receiveCount := func(want int) {
				t.Helper()
				select {
				case result := <-stream:
					if result.Err != nil {
						t.Fatalf("status result error = %v", result.Err)
					}
					if result.Value.OwnBearers != want {
						t.Fatalf("OwnBearers = %d, want %d", result.Value.OwnBearers, want)
					}
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for OwnBearers = %d", want)
				}
			}

			backend.statusStream <- Result[Status]{Value: Status{Power: PowerStateOn}}
			receiveCount(0)

			bearer, err := m.Connect(t.Context(), ConnectConfig{Interface: "wwan0"})
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			receiveCount(1)

			if err := bearer.Disconnect(t.Context()); err != nil {
				t.Fatalf("Disconnect() error = %v", err)
			}
			receiveCount(0)
		})
	}
}

func TestSensitiveConfigString(t *testing.T) {
	tests := []struct {
		name  string
		value fmt.Stringer
	}{
		{name: "profile", value: Profile{Username: "alice", Password: "secret"}},
		{name: "profile config", value: ProfileConfig{Username: "alice", Password: "secret"}},
		{name: "profile update", value: ProfileUpdate{Username: ptr("alice"), Password: ptr("secret")}},
		{name: "initial EPS", value: InitialEPSConfig{Username: "alice", Password: "secret"}},
		{name: "connect", value: ConnectConfig{PIN: "1234", Username: "alice", Password: "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.value.String()
			for _, secret := range []string{"alice", "secret", "1234"} {
				if strings.Contains(got, secret) {
					t.Errorf("String() = %q, contains credential %q", got, secret)
				}
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "profile and inline APN conflict",
			call: func() error { return validateConnectConfig(ConnectConfig{ProfileID: 1, APN: "internet"}) },
		},
		{
			name: "invalid IP family",
			call: func() error { return validateConnectConfig(ConnectConfig{IPFamily: 8}) },
		},
		{
			name: "text and binary SMS conflict",
			call: func() error { return validateMessageConfig(MessageConfig{Number: "1", Text: "x", Data: []byte{1}}) },
		},
		{
			name: "invalid facility",
			call: func() error { return validateFacilityKey(0, "1234") },
		},
		{
			name: "long facility key",
			call: func() error { return validateFacilityKey(FacilityNetwork, "123456789") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("validation error = nil, want non-nil")
			}
		})
	}
}

func TestConnectOrchestration(t *testing.T) {
	tests := []struct {
		name      string
		backend   connectBackend
		cfg       ConnectConfig
		wantSteps []string
	}{
		{
			name: "waits for every requested transition",
			backend: connectBackend{
				sim:     SIMStateLocked,
				power:   PowerStateOff,
				network: NetworkStatus{Registration: RegistrationIdle, PacketService: PacketServiceDetached},
			},
			cfg: ConnectConfig{PIN: "1234", OperatorID: "00101", Interface: "wwan0"},
			wantSteps: []string{
				"read-sim", "send-pin", "read-sim",
				"read-power", "set-power", "read-power",
				"read-network", "register", "read-network",
				"attach", "read-network", "connect",
			},
		},
		{
			name: "registered on a different operator registers again",
			backend: connectBackend{
				sim:     SIMStateReady,
				power:   PowerStateOn,
				network: NetworkStatus{Registration: RegistrationHome, PacketService: PacketServiceAttached, OperatorID: "00101"},
			},
			cfg:       ConnectConfig{OperatorID: "00102", Interface: "wwan0"},
			wantSteps: []string{"read-power", "read-network", "register", "read-network", "connect"},
		},
		{
			name: "matching registered operator needs no global changes",
			backend: connectBackend{
				sim:     SIMStateReady,
				power:   PowerStateOn,
				network: NetworkStatus{Registration: RegistrationRoaming, PacketService: PacketServiceAttached, OperatorID: "00101"},
			},
			cfg:       ConnectConfig{OperatorID: "00101", Interface: "wwan0"},
			wantSteps: []string{"read-power", "read-network", "connect"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := tt.backend
			backend.session = &lifecycleSession{value: BearerInfo{Connected: true}}
			m := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, &backend)
			bearer, err := m.Connect(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if !slices.Equal(backend.steps, tt.wantSteps) {
				t.Errorf("Connect() steps = %v, want %v", backend.steps, tt.wantSteps)
			}
			if err := bearer.Close(); err != nil {
				t.Errorf("Bearer.Close() error = %v", err)
			}
			if err := m.Close(); err != nil {
				t.Errorf("Modem.Close() error = %v", err)
			}
		})
	}
}

func TestSetCapabilitiesUsesCapabilityPrimitive(t *testing.T) {
	tests := []struct {
		name string
		want Technology
	}{
		{name: "LTE and NR5G", want: TechnologyLTE | TechnologyNR5GSA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &connectBackend{}
			m := newModem(Port{Type: PortQMI, Path: "/dev/test"}, AccessDirect, backend)
			if err := m.SetCapabilities(context.Background(), tt.want); err != nil {
				t.Fatalf("SetCapabilities() error = %v", err)
			}
			if backend.setCaps != tt.want {
				t.Errorf("backend capability = %#x, want %#x", backend.setCaps, tt.want)
			}
		})
	}
}

func TestPollUntil(t *testing.T) {
	tests := []struct {
		name      string
		ctx       func() context.Context
		readyCall int
		wantCalls int
		wantErr   error
	}{
		{name: "polls until ready", ctx: context.Background, readyCall: 2, wantCalls: 2},
		{name: "canceled before query", ctx: canceledContext, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			_, err := pollUntil(tt.ctx(), time.Nanosecond, func(context.Context) (int, error) {
				calls++
				return calls, nil
			}, func(value int) bool {
				return value == tt.readyCall
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("pollUntil() error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("pollUntil() calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func ptr[T any](value T) *T { return &value }
