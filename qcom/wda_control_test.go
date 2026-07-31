package qcom

import (
	"context"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDAControlRequestEncoding(t *testing.T) {
	replication := uint32(4)
	mask := WDAPowersaveConfigDownlinkMarker | WDAPowersaveConfigFlowControl
	rebind := WDADefaultFlowRebindVersion1
	ethernet := true
	endpoint := DataEndpoint{Type: DataEndpointPCIe, InterfaceID: 2}

	tests := []struct {
		name        string
		req         Request
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name:        "set legacy loopback",
			req:         WDASetLoopbackStateRequest{Enabled: true}.Request(),
			wantMessage: MessageWDASetLoopbackState,
			wantTLVs:    map[byte][]byte{0x01: {1}},
		},
		{
			name:        "get loopback",
			req:         WDAGetLoopbackStateRequest{}.Request(),
			wantMessage: MessageWDAGetLoopbackState,
		},
		{
			name: "set loopback configuration",
			req: WDASetLoopbackConfigRequest{Config: WDASetLoopbackConfig{
				Enabled: true, ReplicationFactor: &replication,
			}}.Request(),
			wantMessage: MessageWDASetLoopbackConfig,
			wantTLVs: map[byte][]byte{
				0x01: {1},
				0x10: {4, 0, 0, 0},
			},
		},
		{
			name:        "set powersave configuration",
			req:         mustWDAControlRequest(t, WDASetPowersaveConfigRequest{Config: WDASetPowersaveConfig{Endpoint: endpoint, RequestedMask: &mask}}),
			wantMessage: MessageWDASetPowersaveConfig,
			wantTLVs: map[byte][]byte{
				0x01: {3, 0, 0, 0, 2, 0, 0, 0},
				0x10: {3, 0, 0, 0},
			},
		},
		{
			name:        "set powersave mode",
			req:         WDASetPowersaveModeRequest{Enabled: true}.Request(),
			wantMessage: MessageWDASetPowersaveMode,
			wantTLVs:    map[byte][]byte{0x01: {1}},
		},
		{
			name: "set capabilities",
			req: mustWDAControlRequest(t, WDASetCapabilityRequest{Config: WDASetCapabilityConfig{
				Endpoint: endpoint, DefaultFlowRebind: &rebind, EthernetPDUCapability: &ethernet,
			}}),
			wantMessage: MessageWDASetCapability,
			wantTLVs: map[byte][]byte{
				0x01: {3, 0, 0, 0, 2, 0, 0, 0},
				0x10: {1, 0, 0, 0},
				0x15: {1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Service != ServiceWDA || tt.req.MessageID != tt.wantMessage {
				t.Fatalf("request = service 0x%X message 0x%04X, want service 0x%X message 0x%04X", tt.req.Service, tt.req.MessageID, ServiceWDA, tt.wantMessage)
			}
			if len(tt.req.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(tt.req.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				assertTLV(t, tt.req.TLVs, kind, want)
			}
		})
	}
}

func TestWDAControlRequestValidation(t *testing.T) {
	invalidMask := WDASetPowersaveConfigMask(0x80000000)
	invalidVersion := WDADefaultFlowRebindVersion(2)
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "powersave mask",
			fn: func() error {
				_, err := (WDASetPowersaveConfigRequest{Config: WDASetPowersaveConfig{RequestedMask: &invalidMask}}).Request()
				return err
			},
		},
		{
			name: "flow rebind version",
			fn: func() error {
				_, err := (WDASetCapabilityRequest{Config: WDASetCapabilityConfig{DefaultFlowRebind: &invalidVersion}}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("Request() error = nil, want non-nil")
			}
		})
	}
}

func TestWDALoopbackDecoding(t *testing.T) {
	tests := []struct {
		name    string
		fn      func() (WDALoopbackState, error)
		want    WDALoopbackState
		wantErr bool
	}{
		{
			name: "get response",
			fn: func() (WDALoopbackState, error) {
				var response WDAGetLoopbackStateResponse
				err := response.UnmarshalTLVs(tlv.TLVs{
					tlv.Uint(0x10, uint8(1)),
					tlv.Uint(0x11, uint32(4)),
				})
				return response.State, err
			},
			want: WDALoopbackState{Enabled: true, EnabledKnown: true, ReplicationFactor: 4, ReplicationFactorKnown: true},
		},
		{
			name: "indication",
			fn: func() (WDALoopbackState, error) {
				var indication WDALoopbackConfigIndication
				err := indication.UnmarshalTLVs(tlv.TLVs{
					tlv.Uint(0x01, uint8(0)),
					tlv.Uint(0x10, uint32(2)),
				})
				return indication.State, err
			},
			want: WDALoopbackState{EnabledKnown: true, ReplicationFactor: 2, ReplicationFactorKnown: true},
		},
		{
			name: "missing indication state",
			fn: func() (WDALoopbackState, error) {
				var indication WDALoopbackConfigIndication
				return indication.State, indication.UnmarshalTLVs(nil)
			},
			wantErr: true,
		},
		{
			name: "invalid boolean",
			fn: func() (WDALoopbackState, error) {
				var response WDAGetLoopbackStateResponse
				return response.State, response.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint8(2))})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if tt.wantErr {
				if err == nil {
					t.Fatal("decode error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("decode error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("state = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWDAControlResponseDecoding(t *testing.T) {
	tests := []struct {
		name    string
		fn      func() error
		wantErr bool
	}{
		{
			name: "powersave mask",
			fn: func() error {
				var got WDASetPowersaveConfigResponse
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint32(3))})
				if err == nil && (!got.MaskKnown || got.Mask != 3) {
					t.Fatalf("response = %+v", got)
				}
				return err
			},
		},
		{
			name: "capabilities",
			fn: func() error {
				var got WDASetCapabilityResponse
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Uint(0x10, uint32(WDADefaultFlowRebindVersion1)),
					tlv.Uint(0x15, uint8(1)),
				})
				if err == nil && (!got.DefaultFlowRebindKnown || got.DefaultFlowRebind != WDADefaultFlowRebindVersion1 || !got.EthernetPDUCapabilityKnown || !got.EthernetPDUCapability) {
					t.Fatalf("response = %+v", got)
				}
				return err
			},
		},
		{
			name: "truncated powersave mask",
			fn: func() error {
				var got WDASetPowersaveConfigResponse
				return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, []byte{1})})
			},
			wantErr: true,
		},
		{
			name: "invalid Ethernet capability",
			fn: func() error {
				var got WDASetCapabilityResponse
				return got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x15, uint8(2))})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
		})
	}
}

func TestWDAWatchLoopbackConfiguration(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forwards valid indication"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &wdaControlIndicationTransport{
				serviceBoundFakeTransport: serviceBoundFakeTransport{
					fakeTransport: fakeTransport{t: t},
					service:       ServiceWDA,
				},
				indications: make(chan Indication, 1),
			}
			client := &Client{transport: transport, slot: 1}
			ctx, cancel := context.WithCancel(context.Background())
			updates, err := client.WDAWatchLoopbackConfiguration(ctx)
			if err != nil {
				cancel()
				t.Fatalf("WDAWatchLoopbackConfiguration() error = %v", err)
			}
			transport.indications <- Indication{TLVs: tlv.TLVs{
				tlv.Uint(0x01, uint8(1)),
				tlv.Uint(0x10, uint32(5)),
			}}
			close(transport.indications)

			select {
			case got := <-updates:
				if !got.State.Enabled || got.State.ReplicationFactor != 5 {
					t.Fatalf("indication = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for loopback indication")
			}
			cancel()
		})
	}
}

type wdaControlRequest interface {
	Request() (Request, error)
}

func mustWDAControlRequest(t *testing.T, request wdaControlRequest) Request {
	t.Helper()
	got, err := request.Request()
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	return got
}

type wdaControlIndicationTransport struct {
	serviceBoundFakeTransport
	indications chan Indication
}

func (t *wdaControlIndicationTransport) Indications(_ context.Context, service ServiceType, clientID uint8, message MessageID) (<-chan Indication, error) {
	if service != ServiceWDA || clientID != 0 || message != MessageWDALoopbackConfigResult {
		t.t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", service, clientID, message)
	}
	return t.indications, nil
}
