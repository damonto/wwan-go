package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSStatusRequestDecoding(t *testing.T) {
	ready := binary.LittleEndian.AppendUint32(nil, uint32(WMSReady3GPPAnd3GPP2))
	simReady := binary.LittleEndian.AppendUint64(nil, uint64(WMSSIMReady3GPP|WMSSIMReady3GPP2))
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		resp    Response
	}{
		{
			name:    "transport layer",
			message: MessageWMSGetTransportLayer,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSTransportLayer(context.Background())
				if err == nil && (!got.RegisteredKnown || !got.Registered || !got.InfoKnown || got.Type != WMSTransportIMS || got.Capability != WMSTransportCapabilityGW) {
					t.Fatalf("transport layer = %+v", got)
				}
				return err
			},
			resp: successResponse(MessageWMSGetTransportLayer, tlv.Bytes(0x10, []byte{1}), tlv.Bytes(0x11, []byte{0, 1})),
		},
		{
			name:    "transport network",
			message: MessageWMSGetTransportNetwork,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSTransportNetwork(context.Background())
				if err == nil && (!got.Known || got.Status != WMSTransportNetworkFullService) {
					t.Fatalf("transport network = %+v", got)
				}
				return err
			},
			resp: successResponse(MessageWMSGetTransportNetwork, tlv.Bytes(0x10, []byte{4})),
		},
		{
			name:    "service ready",
			message: MessageWMSGetServiceReadyState,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSServiceReady(context.Background())
				if err == nil && (!got.EventsRegisteredKnown || !got.EventsRegistered || !got.StatusKnown || got.Status != WMSReady3GPPAnd3GPP2 || !got.SIMEventsKnown || got.SIMEventsRegistered || !got.SIMStatusKnown || got.SIMStatus != WMSSIMReady3GPP|WMSSIMReady3GPP2) {
					t.Fatalf("service ready = %+v", got)
				}
				return err
			},
			resp: successResponse(
				MessageWMSGetServiceReadyState,
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x11, ready),
				tlv.Bytes(0x12, []byte{0}),
				tlv.Bytes(0x13, simReady),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != tt.message || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v, want message 0x%04X without TLVs", req, tt.message)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestWMSStatusIndicationDecoding(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "transport layer",
			fn: func() error {
				var got WMSTransportLayerState
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x10, []byte{0, 1})})
				if err == nil && (!got.Registered || !got.InfoKnown || got.Capability != WMSTransportCapabilityGW) {
					t.Fatalf("transport layer = %+v", got)
				}
				return err
			},
		},
		{
			name: "transport network",
			fn: func() error {
				var got WMSTransportNetworkState
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{3})})
				if err == nil && (!got.Known || got.Status != WMSTransportNetworkLimitedService) {
					t.Fatalf("transport network = %+v", got)
				}
				return err
			},
		},
		{
			name: "service ready",
			fn: func() error {
				var got WMSServiceReadyState
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, uint32(WMSReady3GPP))),
					tlv.Bytes(0x10, binary.LittleEndian.AppendUint64(nil, uint64(WMSSIMReady3GPP))),
				})
				if err == nil && (!got.StatusKnown || got.Status != WMSReady3GPP || !got.SIMStatusKnown || got.SIMStatus != WMSSIMReady3GPP) {
					t.Fatalf("service ready = %+v", got)
				}
				return err
			},
		},
		{
			name: "SMSC address",
			fn: func() error {
				var got WMSSMSCAddress
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{'1', '4', '5', 8, '+', '8', '6', '1', '3', '8', '0', '0'})})
				if err == nil && (got.Type != "145" || got.Digits != "+8613800") {
					t.Fatalf("SMSC address = %+v", got)
				}
				return err
			},
		},
		{
			name: "memory full",
			fn: func() error {
				var got WMSMemoryFullState
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 1})})
				if err == nil && (got.Storage != WMSStorageNV || got.MessageMode != WMSMessageModeGW) {
					t.Fatalf("memory full = %+v", got)
				}
				return err
			},
		},
		{
			name: "call status",
			fn: func() error {
				var got WMSCallStatus
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{byte(WMSCallConnecting)})})
				if err == nil && got != WMSCallConnecting {
					t.Fatalf("call status = %d, want %d", got, WMSCallConnecting)
				}
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Fatalf("decode() error = %v", err)
			}
		})
	}
}

func TestWMSStatusDecodersRejectMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "transport registration missing", fn: func() error {
			var state WMSTransportLayerState
			return state.UnmarshalTLVs(nil)
		}},
		{name: "transport info truncated", fn: func() error {
			var state WMSTransportLayerState
			return state.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1}), tlv.Bytes(0x10, []byte{0})})
		}},
		{name: "network status missing", fn: func() error {
			var state WMSTransportNetworkState
			return state.UnmarshalTLVs(nil)
		}},
		{name: "ready status truncated", fn: func() error {
			var state WMSServiceReadyState
			return state.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1})})
		}},
		{name: "SIM status truncated", fn: func() error {
			var state WMSServiceReadyState
			return state.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, make([]byte, 4)), tlv.Bytes(0x10, make([]byte, 7))})
		}},
		{name: "SMSC address missing", fn: func() error {
			var address WMSSMSCAddress
			return address.UnmarshalTLVs(nil)
		}},
		{name: "memory info truncated", fn: func() error {
			var state WMSMemoryFullState
			return state.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1})})
		}},
		{name: "call status missing", fn: func() error {
			var status WMSCallStatus
			return status.UnmarshalTLVs(nil)
		}},
		{name: "call status truncated", fn: func() error {
			var status WMSCallStatus
			return status.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})})
		}},
		{name: "call status out of range", fn: func() error {
			var status WMSCallStatus
			return status.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{5})})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("decode() error = nil, want error")
			}
		})
	}
}

func TestWMSWatchSMSCAddress(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forwards changed address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &wmsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageWMSIndicationRegister)},
				{resp: successResponse(MessageWMSIndicationRegister)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			out, err := client.WMSWatchSMSCAddress(ctx)
			if err != nil {
				t.Fatalf("WMSWatchSMSCAddress() error = %v", err)
			}
			transport.emit(Indication{Service: ServiceWMS, ClientID: 7, MessageID: MessageWMSSMSCAddress, TLVs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{'1', '4', '5', 8, '+', '8', '6', '1', '3', '8', '0', '0'}),
			}})
			select {
			case got := <-out:
				if got.Type != "145" || got.Digits != "+8613800" {
					t.Fatalf("SMSC address = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for WMS SMSC address event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestWMSWatchServiceReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &wmsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
		{resp: successResponse(MessageWMSIndicationRegister)},
		{resp: successResponse(MessageWMSIndicationRegister)},
	}}}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
	out, err := client.WMSWatchServiceReady(ctx)
	if err != nil {
		t.Fatalf("WMSWatchServiceReady() error = %v", err)
	}
	transport.emit(Indication{Service: ServiceWMS, ClientID: 7, MessageID: MessageWMSServiceReady, TLVs: tlv.TLVs{
		tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, uint32(WMSReady3GPPAnd3GPP2))),
	}})
	select {
	case got := <-out:
		if !got.StatusKnown || got.Status != WMSReady3GPPAnd3GPP2 {
			t.Fatalf("service ready = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WMS service-ready event")
	}
	cancel()
	transport.waitCalls(t, 2)
}

func TestWMSWatchCallStatus(t *testing.T) {
	tests := []struct {
		name string
		want WMSCallStatus
	}{
		{name: "forwards connecting state", want: WMSCallConnecting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &wmsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageWMSIndicationRegister)},
				{resp: successResponse(MessageWMSIndicationRegister)},
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			out, err := client.WMSWatchCallStatus(ctx)
			if err != nil {
				t.Fatalf("WMSWatchCallStatus() error = %v", err)
			}
			transport.emit(Indication{Service: ServiceWMS, ClientID: 7, MessageID: MessageWMSCallStatus, TLVs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(tt.want)}),
			}})
			select {
			case got := <-out:
				if got != tt.want {
					t.Fatalf("call status = %d, want %d", got, tt.want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for WMS call-status event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}
