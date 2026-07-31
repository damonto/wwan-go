package qcom

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSGetPacketServiceStatusRequest(t *testing.T) {
	tests := []struct {
		name string
		mask WDSRuntimeSettingsMask
		want []byte
	}{
		{name: "status only"},
		{
			name: "network configuration",
			mask: WDSRuntimeRequestedNetworkSettings,
			want: binary.LittleEndian.AppendUint32(nil, uint32(WDSRuntimeRequestedNetworkSettings)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := WDSGetPacketServiceStatusRequest{ClientID: 7, TransactionID: 3, IPConfigMask: tt.mask}.Request()
			if req.Service != ServiceWDS || req.ClientID != 7 || req.MessageID != MessageWDSGetPacketServiceStatus {
				t.Fatalf("Request() = %+v", req)
			}
			value, ok := tlv.Value(req.TLVs, 0x10)
			if len(tt.want) == 0 {
				if ok {
					t.Fatalf("IP configuration TLV = % X, want absent", value)
				}
				return
			}
			if !ok || string(value) != string(tt.want) {
				t.Fatalf("IP configuration TLV = % X, want % X", value, tt.want)
			}
		})
	}
}

func TestWDSIndicationRegisterRequest(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name   string
		config WDSIndicationRegisterConfig
		want   map[uint8][]byte
	}{
		{name: "empty", want: map[uint8][]byte{}},
		{
			name: "all supported fields",
			config: WDSIndicationRegisterConfig{
				SuppressPacketService: &disabled,
				ExtendedIPConfig:      &enabled,
				ProfileChanges:        &enabled,
			},
			want: map[uint8][]byte{
				0x11: {0},
				0x12: {1},
				0x19: {1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (WDSIndicationRegisterRequest{
				ClientID: 7, TransactionID: 3, Timeout: 2 * time.Second, Config: tt.config,
			}).Request()
			if got.Service != ServiceWDS || got.ClientID != 7 || got.TransactionID != 3 ||
				got.MessageID != MessageWDSIndicationRegister || got.Timeout != 2*time.Second {
				t.Fatalf("Request() = %+v", got)
			}
			if len(got.TLVs) != len(tt.want) {
				t.Fatalf("TLV count = %d, want %d", len(got.TLVs), len(tt.want))
			}
			for typ, want := range tt.want {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestDecodeWDSPacketServiceStatus(t *testing.T) {
	tests := []struct {
		name       string
		indication bool
		tlvs       tlv.TLVs
		check      func(*testing.T, WDSPacketServiceStatus)
	}{
		{
			name: "query response",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(WDSConnectionStatusConnected)}),
				tlv.Bytes(0x10, []byte{byte(WDSIPFamilyPreferenceIPv4)}),
				tlv.Bytes(0x11, []byte{1, 0, 168, 192}),
				tlv.Bytes(0x16, []byte{8, 8, 8, 8}),
				tlv.Uint(0x1A, uint32(1500)),
			},
			check: func(t *testing.T, status WDSPacketServiceStatus) {
				if status.ConnectionStatus != WDSConnectionStatusConnected || status.Runtime.IPFamily != WDSIPFamilyIPv4 {
					t.Fatalf("status = %+v", status)
				}
				if !status.Runtime.LocalIPv4.Equal(net.IPv4(192, 168, 0, 1)) || len(status.Runtime.DNS) != 1 || status.Runtime.MTU != 1500 {
					t.Fatalf("runtime = %+v", status.Runtime)
				}
			},
		},
		{
			name:       "status indication",
			indication: true,
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(WDSConnectionStatusDisconnected), 1}),
				tlv.Uint(0x10, uint16(36)),
				tlv.Bytes(0x11, []byte{3, 0, 33, 0}),
				tlv.Uint(0x13, uint16(0x8880)),
				tlv.Uint(0x16, uint32(WDSRuntimeMaskDNSAddress)),
			},
			check: func(t *testing.T, status WDSPacketServiceStatus) {
				if status.ConnectionStatus != WDSConnectionStatusDisconnected || !status.ReconfigurationRequired {
					t.Fatalf("status = %+v", status)
				}
				if !status.CallEndReasonKnown || !status.VerboseCallEndReasonKnown || status.Technology != WDSTechnologyNameEPC {
					t.Fatalf("optional status fields = %+v", status)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status WDSPacketServiceStatus
			var err error
			if tt.indication {
				var decoded wdsPacketServiceIndication
				err = decoded.UnmarshalTLVs(tt.tlvs)
				status = WDSPacketServiceStatus(decoded)
			} else {
				var decoded wdsGetPacketServiceStatusResponse
				err = decoded.UnmarshalTLVs(tt.tlvs)
				status = WDSPacketServiceStatus(decoded)
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			tt.check(t, status)
		})
	}
}

func TestDecodeWDSPacketServiceStatusRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name       string
		indication bool
		tlvs       tlv.TLVs
	}{
		{name: "query status truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, nil)}},
		{name: "query status trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})}},
		{name: "query family trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x10, []byte{4, 0})}},
		{name: "query IPv4 trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x11, make([]byte, 5))}},
		{name: "query IPv6 trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x12, make([]byte, 18))}},
		{name: "query IPv4 DNS trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x16, make([]byte, 5))}},
		{name: "query IPv6 DNS trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x18, make([]byte, 17))}},
		{name: "query MTU trailing", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x1A, make([]byte, 5))}},
		{name: "indication status trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 3))}},
		{name: "indication reason trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x10, make([]byte, 3))}},
		{name: "indication verbose reason trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x11, make([]byte, 5))}},
		{name: "indication family trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x12, make([]byte, 2))}},
		{name: "indication technology trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x13, make([]byte, 3))}},
		{name: "indication bearer ID trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x14, make([]byte, 2))}},
		{name: "indication XLAT trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x15, make([]byte, 2))}},
		{name: "indication changed mask trailing", indication: true, tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 2)), tlv.Bytes(0x16, make([]byte, 5))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.indication {
				err = new(wdsPacketServiceIndication).UnmarshalTLVs(tt.tlvs)
			} else {
				err = new(wdsGetPacketServiceStatusResponse).UnmarshalTLVs(tt.tlvs)
			}
			if err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestPDNSessionWatchStatus(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "initial state and disconnect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &wdsStatusIndicationTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{
						check: func(req Request) {
							assertTLV(t, req.TLVs, 0x11, []byte{0})
							assertTLV(t, req.TLVs, 0x12, []byte{1})
						},
						resp: successResponse(MessageWDSIndicationRegister),
					},
					{
						resp: successResponse(MessageWDSGetPacketServiceStatus,
							tlv.Bytes(0x01, []byte{byte(WDSConnectionStatusConnected)}),
						),
					},
					{
						resp: successResponse(MessageWDSGetRuntimeSettings,
							tlv.Bytes(0x1E, []byte{2, 0, 0, 10}),
							tlv.Uint(0x29, uint32(1500)),
							tlv.Bytes(0x2B, []byte{byte(WDSIPFamilyIPv4)}),
						),
					},
					{
						check: func(req Request) {
							assertTLV(t, req.TLVs, 0x11, []byte{1})
							assertTLV(t, req.TLVs, 0x12, []byte{0})
						},
						resp: successResponse(MessageWDSIndicationRegister),
					},
				}},
			}
			client := &Client{transport: transport, slot: 1}
			session := &PDNSession{
				client:            client,
				timeout:           time.Second,
				wdsClientID:       7,
				wdsClientReady:    true,
				requestedSettings: WDSRuntimeRequestedNetworkSettings,
				done:              make(chan struct{}),
			}

			events, err := session.WatchStatus(ctx)
			if err != nil {
				t.Fatalf("WatchStatus() error = %v", err)
			}
			select {
			case event := <-events:
				if event.RefreshError != nil || event.Status.ConnectionStatus != WDSConnectionStatusConnected || !event.Status.Runtime.LocalIPv4.Equal(net.IPv4(10, 0, 0, 2)) {
					t.Fatalf("initial event = %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for initial status")
			}

			transport.emit(MessageWDSGetPacketServiceStatus, Indication{
				Service:   ServiceWDS,
				ClientID:  7,
				MessageID: MessageWDSGetPacketServiceStatus,
				TLVs:      tlv.TLVs{tlv.Bytes(0x01, []byte{byte(WDSConnectionStatusDisconnected), 0})},
			})
			select {
			case event := <-events:
				if event.Status.ConnectionStatus != WDSConnectionStatusDisconnected {
					t.Fatalf("disconnect event = %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for disconnect status")
			}
			if info := session.Info(); info.PacketDataReady || len(info.LocalIPv4) != 0 {
				t.Fatalf("Info() after disconnect = %+v", info)
			}

			cancel()
			transport.waitCalls(t, 4)
		})
	}
}

type wdsStatusIndicationTransport struct {
	fakeTransport

	mu       sync.Mutex
	channels map[MessageID]chan Indication
}

func (t *wdsStatusIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, id MessageID) (<-chan Indication, error) {
	ch := make(chan Indication, 4)
	t.mu.Lock()
	if t.channels == nil {
		t.channels = make(map[MessageID]chan Indication)
	}
	t.channels[id] = ch
	t.mu.Unlock()
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (t *wdsStatusIndicationTransport) emit(id MessageID, ind Indication) {
	t.mu.Lock()
	ch := t.channels[id]
	t.mu.Unlock()
	ch <- ind
}

func (t *wdsStatusIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}
