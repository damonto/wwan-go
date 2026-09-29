package qmi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

type statusTransport struct {
	mu                 sync.Mutex
	mode               qcom.DMSOperatingMode
	hardwareRestricted bool
	registered         bool
	networkError       qcom.QMIError
}

func (t *statusTransport) setStatus(mode qcom.DMSOperatingMode, registered bool) {
	t.mu.Lock()
	t.mode = mode
	t.registered = registered
	t.mu.Unlock()
}

func (t *statusTransport) setHardwareRestricted(restricted bool) {
	t.mu.Lock()
	t.hardwareRestricted = restricted
	t.mu.Unlock()
}

func (t *statusTransport) setNetworkError(err qcom.QMIError) {
	t.mu.Lock()
	t.networkError = err
	t.mu.Unlock()
}

type statusTransportSnapshot struct {
	mode               qcom.DMSOperatingMode
	hardwareRestricted bool
	registered         bool
	networkError       qcom.QMIError
}

func (t *statusTransport) snapshot() statusTransportSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return statusTransportSnapshot{
		mode:               t.mode,
		hardwareRestricted: t.hardwareRestricted,
		registered:         t.registered,
		networkError:       t.networkError,
	}
}

func (*statusTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (t *statusTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	state := t.snapshot()
	switch {
	case req.Service == qcom.ServiceDMS && req.MessageID == qcom.MessageDMSGetOperatingMode:
		values := []tlv.TLV{tlv.Bytes(0x01, []byte{byte(state.mode)})}
		if state.hardwareRestricted {
			values = append(values, tlv.Bytes(0x11, []byte{1}))
		}
		return successfulStatusResponse(req, values...), nil
	case req.Service == qcom.ServiceDMS && req.MessageID == qcom.MessageDMSSetEventReport:
		return successfulStatusResponse(req), nil
	case req.Service == qcom.ServiceNAS && req.MessageID == qcom.MessageNASIndicationRegister:
		return successfulStatusResponse(req), nil
	case req.Service == qcom.ServiceUIM && req.MessageID == qcom.MessageGetCardStatus:
		return successfulStatusResponse(req, tlv.Bytes(0x10, make([]byte, 9))), nil
	case req.Service == qcom.ServiceNAS && req.MessageID == qcom.MessageNASGetServingSystem:
		if state.networkError != qcom.QMIErrorNone {
			return failedStatusResponse(req, state.networkError), nil
		}
		return successfulStatusResponse(req, servingSystemTLV(state.registered)), nil
	case req.Service == qcom.ServiceNAS && req.MessageID == qcom.MessageNASGetSignalInfo:
		return successfulStatusResponse(req), nil
	default:
		return qcom.Response{}, fmt.Errorf("test status transport: unexpected service 0x%X message 0x%04X", req.Service, req.MessageID)
	}
}

func (*statusTransport) Close() error { return nil }

type statusIndicationTransport struct {
	*statusTransport
	dmsEvents chan qcom.Indication
}

type networkIndicationTransport struct {
	*statusTransport
}

func newStatusIndicationTransport() *statusIndicationTransport {
	return &statusIndicationTransport{
		statusTransport: &statusTransport{},
		dmsEvents:       make(chan qcom.Indication, 1),
	}
}

func (t *statusIndicationTransport) Indications(
	ctx context.Context,
	service qcom.ServiceType,
	_ uint8,
	message qcom.MessageID,
) (<-chan qcom.Indication, error) {
	if service != qcom.ServiceDMS || message != qcom.MessageDMSSetEventReport {
		return nil, errors.New("test status transport: indication unavailable")
	}
	out := make(chan qcom.Indication, 1)
	go func() {
		defer close(out)
		for {
			select {
			case event, ok := <-t.dmsEvents:
				if !ok {
					return
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (t *networkIndicationTransport) Indications(
	ctx context.Context,
	service qcom.ServiceType,
	_ uint8,
	message qcom.MessageID,
) (<-chan qcom.Indication, error) {
	if service != qcom.ServiceNAS || message != qcom.MessageNASGetServingSystem {
		return nil, errors.New("test status transport: indication unavailable")
	}
	out := make(chan qcom.Indication)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}

func (t *statusIndicationTransport) emitOperatingMode(ctx context.Context, mode qcom.DMSOperatingMode) error {
	event := qcom.Indication{
		Service:   qcom.ServiceDMS,
		MessageID: qcom.MessageDMSSetEventReport,
		TLVs:      tlv.TLVs{tlv.Bytes(0x14, []byte{byte(mode)})},
	}
	select {
	case t.dmsEvents <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *statusIndicationTransport) emitWirelessDisabled(ctx context.Context, disabled bool) error {
	value := byte(0)
	if disabled {
		value = 1
	}
	event := qcom.Indication{
		Service:   qcom.ServiceDMS,
		MessageID: qcom.MessageDMSSetEventReport,
		TLVs:      tlv.TLVs{tlv.Bytes(0x16, []byte{value})},
	}
	select {
	case t.dmsEvents <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func successfulStatusResponse(req qcom.Request, values ...tlv.TLV) qcom.Response {
	tlvs := tlv.TLVs{tlv.Bytes(0x02, []byte{0, 0, 0, 0})}
	tlvs = append(tlvs, values...)
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlvs,
	}
}

func failedStatusResponse(req qcom.Request, err qcom.QMIError) qcom.Response {
	result := binary.LittleEndian.AppendUint16(nil, uint16(qcom.QMIResultFailure))
	result = binary.LittleEndian.AppendUint16(result, uint16(err))
	return qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlv.TLVs{tlv.Bytes(0x02, result)},
	}
}

func servingSystemTLV(registered bool) tlv.TLV {
	if registered {
		return tlv.Bytes(0x01, []byte{
			byte(qcom.NASRegistrationRegistered),
			byte(qcom.NASAttachAttached),
			byte(qcom.NASAttachAttached),
			byte(qcom.NASSelectedNetwork3GPP),
			1,
			byte(qcom.NASRadioInterfaceLTE),
		})
	}
	return tlv.Bytes(0x01, []byte{
		byte(qcom.NASRegistrationNotRegistered),
		byte(qcom.NASAttachDetached),
		byte(qcom.NASAttachDetached),
		byte(qcom.NASSelectedNetwork3GPP),
		0,
	})
}

func newStatusTestBackend(t *testing.T, transport qcom.Transport) *Backend {
	t.Helper()
	client, err := qcom.NewClient(transport)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Client.Close() error = %v", err)
		}
	})
	return New(client, "/dev/test")
}

func receiveStatus(ctx context.Context, t *testing.T, stream <-chan Result[Status]) Status {
	t.Helper()
	select {
	case result, ok := <-stream:
		if !ok {
			t.Fatal("status stream closed before result")
		}
		if result.Err != nil {
			t.Fatalf("status result error = %v", result.Err)
		}
		return result.Value
	case <-ctx.Done():
		t.Fatalf("waiting for status result: %v", ctx.Err())
		return Status{}
	}
}

func TestBackendStatusNormalizesPower(t *testing.T) {
	tests := []struct {
		name               string
		mode               qcom.DMSOperatingMode
		hardwareRestricted bool
		networkError       qcom.QMIError
		want               Status
	}{
		{
			name: "online preserves network status",
			mode: qcom.DMSOperatingModeOnline,
			want: Status{
				Power:         PowerStateOn,
				SIM:           SIMStateAbsent,
				Registration:  RegistrationHome,
				PacketService: PacketServiceAttached,
				Technology:    TechnologyLTE,
			},
		},
		{
			name:         "low power skips radio queries",
			mode:         qcom.DMSOperatingModeLowPower,
			networkError: qcom.QMIErrorInternal,
			want: Status{
				Power:         PowerStateLow,
				SIM:           SIMStateAbsent,
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
		{
			name:               "hardware restriction overrides online mode",
			mode:               qcom.DMSOperatingModeOnline,
			hardwareRestricted: true,
			networkError:       qcom.QMIErrorInternal,
			want: Status{
				Power:         PowerStateLow,
				SIM:           SIMStateAbsent,
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
		{
			name:         "unknown mode clears radio status",
			mode:         qcom.DMSOperatingModeResetting,
			networkError: qcom.QMIErrorInternal,
			want: Status{
				SIM:           SIMStateAbsent,
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
		{
			name:         "online tolerates temporarily unavailable radio",
			mode:         qcom.DMSOperatingModeOnline,
			networkError: qcom.QMIErrorNoRadio,
			want: Status{
				Power: PowerStateOn,
				SIM:   SIMStateAbsent,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &statusTransport{}
			transport.setStatus(tt.mode, true)
			transport.setHardwareRestricted(tt.hardwareRestricted)
			transport.setNetworkError(tt.networkError)
			backend := newStatusTestBackend(t, transport)

			got, err := backend.Status(t.Context())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Status() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBackendPowerStateUsesOperatingModeInfo(t *testing.T) {
	tests := []struct {
		name               string
		mode               qcom.DMSOperatingMode
		hardwareRestricted bool
		want               PowerState
	}{
		{name: "online", mode: qcom.DMSOperatingModeOnline, want: PowerStateOn},
		{name: "hardware restricted", mode: qcom.DMSOperatingModeOnline, hardwareRestricted: true, want: PowerStateLow},
		{name: "offline remains off", mode: qcom.DMSOperatingModeOffline, hardwareRestricted: true, want: PowerStateOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &statusTransport{}
			transport.setStatus(tt.mode, false)
			transport.setHardwareRestricted(tt.hardwareRestricted)
			backend := newStatusTestBackend(t, transport)

			got, err := backend.PowerState(t.Context())
			if err != nil {
				t.Fatalf("PowerState() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("PowerState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatchStatusFallsBackToFullPolling(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "network changes without indications"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &statusTransport{}
			transport.setStatus(qcom.DMSOperatingModeOnline, true)
			backend := newStatusTestBackend(t, transport)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			stream, err := backend.WatchStatus(ctx)
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			first := receiveStatus(ctx, t, stream)
			if first.Registration != RegistrationHome || first.Technology != TechnologyLTE {
				t.Fatalf("initial status = %+v, want registered LTE", first)
			}

			transport.setStatus(qcom.DMSOperatingModeOnline, false)
			second := receiveStatus(ctx, t, stream)
			if second.Registration != RegistrationIdle || second.PacketService != PacketServiceDetached || second.Technology != 0 {
				t.Fatalf("polled status = %+v, want idle and detached", second)
			}
		})
	}
}

func TestWatchStatusPollsPowerWithoutDMSIndications(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "hardware restriction is observed through polling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &networkIndicationTransport{statusTransport: &statusTransport{}}
			transport.setStatus(qcom.DMSOperatingModeOnline, true)
			backend := newStatusTestBackend(t, transport)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			stream, err := backend.WatchStatus(ctx)
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			first := receiveStatus(ctx, t, stream)
			if first.Power != PowerStateOn {
				t.Fatalf("initial status = %+v, want online", first)
			}

			transport.setHardwareRestricted(true)
			second := receiveStatus(ctx, t, stream)
			if second.Power != PowerStateLow || second.Registration != RegistrationIdle ||
				second.PacketService != PacketServiceDetached {
				t.Fatalf("polled status = %+v, want hardware-restricted low power", second)
			}
		})
	}
}

func TestWatchStatusRefreshesAfterPowerOn(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "low power to online refreshes network fields"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newStatusIndicationTransport()
			transport.setStatus(qcom.DMSOperatingModeLowPower, true)
			backend := newStatusTestBackend(t, transport)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			stream, err := backend.WatchStatus(ctx)
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			first := receiveStatus(ctx, t, stream)
			if first.Power != PowerStateLow || first.Registration != RegistrationIdle {
				t.Fatalf("initial status = %+v, want normalized low power", first)
			}

			transport.setStatus(qcom.DMSOperatingModeOnline, true)
			if err := transport.emitOperatingMode(ctx, qcom.DMSOperatingModeOnline); err != nil {
				t.Fatalf("emitOperatingMode() error = %v", err)
			}
			second := receiveStatus(ctx, t, stream)
			if second.Power != PowerStateOn || second.Registration != RegistrationHome ||
				second.PacketService != PacketServiceAttached || second.Technology != TechnologyLTE {
				t.Fatalf("refreshed status = %+v, want online registered LTE", second)
			}
		})
	}
}

func TestWatchStatusQueriesPowerWhenWirelessIsEnabled(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "wireless-enabled event refreshes operating mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newStatusIndicationTransport()
			transport.setStatus(qcom.DMSOperatingModeOnline, true)
			transport.setHardwareRestricted(true)
			backend := newStatusTestBackend(t, transport)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			stream, err := backend.WatchStatus(ctx)
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			first := receiveStatus(ctx, t, stream)
			if first.Power != PowerStateLow {
				t.Fatalf("initial status = %+v, want hardware-restricted low power", first)
			}

			transport.setHardwareRestricted(false)
			if err := transport.emitWirelessDisabled(ctx, false); err != nil {
				t.Fatalf("emitWirelessDisabled() error = %v", err)
			}
			second := receiveStatus(ctx, t, stream)
			if second.Power != PowerStateOn || second.Registration != RegistrationHome ||
				second.PacketService != PacketServiceAttached || second.Technology != TechnologyLTE {
				t.Fatalf("refreshed status = %+v, want online registered LTE", second)
			}
		})
	}
}

type systemInfoIndicationTransport struct {
	*statusTransport
	events chan qcom.Indication
}

func (t *systemInfoIndicationTransport) Indications(ctx context.Context, service qcom.ServiceType, _ uint8, message qcom.MessageID) (<-chan qcom.Indication, error) {
	if service != qcom.ServiceNAS || message != qcom.MessageNASSysInfo {
		return nil, errors.New("test: indication unavailable")
	}
	return t.events, nil
}

func TestWatchStatusRefreshesOnSystemInfo(t *testing.T) {
	t.Run("registration changes without serving system indication", func(t *testing.T) {
		transport := &systemInfoIndicationTransport{statusTransport: &statusTransport{}, events: make(chan qcom.Indication, 1)}
		transport.setStatus(qcom.DMSOperatingModeOnline, true)
		backend := newStatusTestBackend(t, transport)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		stream, err := backend.WatchStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		initial := receiveStatus(ctx, t, stream)
		transport.setStatus(qcom.DMSOperatingModeOnline, false)
		transport.events <- qcom.Indication{Service: qcom.ServiceNAS, MessageID: qcom.MessageNASSysInfo}
		next := receiveStatus(ctx, t, stream)
		if next.Registration == initial.Registration {
			t.Fatalf("registration did not change: %+v", next)
		}
	})
}
