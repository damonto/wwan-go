package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestQOSRequestEncoding(t *testing.T) {
	muxID := uint8(3)
	legacyPort := WDSSIOPortA2MuxRMNET1
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		want        map[byte][]byte
	}{
		{
			name: "flow status",
			request: func() (Request, error) {
				return (QOSGetFlowStatusRequest{ClientID: 7, FlowID: 0x01020304}).Request(), nil
			},
			wantMessage: MessageQOSGetStatus,
			want:        map[byte][]byte{0x01: {4, 3, 2, 1}},
		},
		{
			name: "modern data port",
			request: func() (Request, error) {
				return (QOSBindDataPortRequest{ClientID: 7, Config: QOSDataPortConfig{
					Endpoint: &DataEndpoint{Type: DataEndpointBAMDMUX, InterfaceID: 1},
					MuxID:    &muxID,
				}}).Request()
			},
			wantMessage: MessageQOSBindDataPort,
			want: map[byte][]byte{
				0x10: {5, 0, 0, 0, 1, 0, 0, 0},
				0x11: {3},
			},
		},
		{
			name: "legacy data port",
			request: func() (Request, error) {
				return (QOSBindDataPortRequest{ClientID: 7, Config: QOSDataPortConfig{LegacyPort: &legacyPort}}).Request()
			},
			wantMessage: MessageQOSBindDataPort,
			want:        map[byte][]byte{0x12: {0x05, 0x0E}},
		},
		{
			name: "bind subscription",
			request: func() (Request, error) {
				return (QOSBindSubscriptionRequest{ClientID: 7, Subscription: QOSSubscriptionTertiary}).Request()
			},
			wantMessage: MessageQOSBindSubscription,
			want: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(QOSSubscriptionTertiary)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if req.Service != ServiceQOS || req.ClientID != 7 || req.MessageID != tt.wantMessage {
				t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
			}
			for kind, want := range tt.want {
				assertTLV(t, req.TLVs, kind, want)
			}
		})
	}
}

func TestQOSRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request func() error
		want    string
	}{
		{
			name: "no data port",
			request: func() error {
				_, err := (QOSBindDataPortRequest{}).Request()
				return err
			},
			want: "no port selected",
		},
		{
			name: "conflicting data ports",
			request: func() error {
				muxID := uint8(1)
				legacyPort := WDSSIOPortA2MuxRMNET0
				_, err := (QOSBindDataPortRequest{Config: QOSDataPortConfig{MuxID: &muxID, LegacyPort: &legacyPort}}).Request()
				return err
			},
			want: "mutually exclusive",
		},
		{
			name: "invalid subscription",
			request: func() error {
				_, err := (QOSBindSubscriptionRequest{Subscription: 4}).Request()
				return err
			},
			want: "subscription 4 is out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("request error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestQOSResponseDecoding(t *testing.T) {
	statusInfo := binary.LittleEndian.AppendUint32(nil, 7)
	statusInfo = append(statusInfo, byte(QOSFlowStatusSuspended), byte(QOSFlowEventSuspended))

	tests := []struct {
		name    string
		decode  func(tlv.TLVs) error
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "flow status",
			decode: func(items tlv.TLVs) error {
				var response QOSGetFlowStatusResponse
				if err := response.UnmarshalTLVs(items); err != nil {
					return err
				}
				if response.Status != QOSFlowStatusActivated {
					t.Fatalf("status = %d", response.Status)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(QOSFlowStatusActivated))},
		},
		{
			name: "network status",
			decode: func(items tlv.TLVs) error {
				var response QOSGetNetworkStatusResponse
				if err := response.UnmarshalTLVs(items); err != nil {
					return err
				}
				if !response.Supported {
					t.Fatal("Supported = false")
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(1))},
		},
		{
			name: "status indication",
			decode: func(items tlv.TLVs) error {
				var indication QOSFlowStatusIndication
				if err := indication.UnmarshalTLVs(items); err != nil {
					return err
				}
				update := indication.Update
				if update.ID != 7 || update.Status != QOSFlowStatusSuspended || update.Event != QOSFlowEventSuspended ||
					!update.ReasonKnown || update.Reason != 8 {
					t.Fatalf("update = %+v", update)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Bytes(0x01, statusInfo), tlv.Uint(0x10, uint8(8))},
		},
		{
			name: "missing flow status",
			decode: func(items tlv.TLVs) error {
				var response QOSGetFlowStatusResponse
				return response.UnmarshalTLVs(items)
			},
			wantErr: true,
		},
		{
			name: "flow status trailing byte",
			decode: func(items tlv.TLVs) error {
				var response QOSGetFlowStatusResponse
				return response.UnmarshalTLVs(items)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, []byte{byte(QOSFlowStatusActivated), 0})},
			wantErr: true,
		},
		{
			name: "status aggregate trailing byte",
			decode: func(items tlv.TLVs) error {
				var indication QOSFlowStatusIndication
				return indication.UnmarshalTLVs(items)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, append(slices.Clone(statusInfo), 0))},
			wantErr: true,
		},
		{
			name: "reason trailing byte",
			decode: func(items tlv.TLVs) error {
				var indication QOSFlowStatusIndication
				return indication.UnmarshalTLVs(items)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, statusInfo), tlv.Bytes(0x10, []byte{8, 0})},
			wantErr: true,
		},
		{
			name: "network support trailing byte",
			decode: func(items tlv.TLVs) error {
				var response QOSGetNetworkStatusResponse
				return response.UnmarshalTLVs(items)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})},
			wantErr: true,
		},
		{
			name: "bound subscription trailing byte",
			decode: func(items tlv.TLVs) error {
				var response QOSGetBindSubscriptionResponse
				return response.UnmarshalTLVs(items)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 5))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.decode(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decode error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientQOSNetworkSupportedReusesClient(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "network status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceQOS, 5)},
				{resp: successResponse(MessageQOSGetNetworkStatus, tlv.Uint(0x01, uint8(1)))},
				{resp: successResponse(MessageReleaseClientID)},
			}}
			client := &Client{transport: transport, slot: 1}

			supported, err := client.QOSNetworkSupported(context.Background())
			if err != nil || !supported {
				t.Fatalf("QOSNetworkSupported() = %v, %v", supported, err)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestQOSWatchNetworkStatusSubscribesDirectly(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &qosIndicationTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{resp: allocatedClientResponse(ServiceQOS, 5)},
					{resp: successResponse(MessageReleaseClientID)},
				}},
				indications: map[MessageID][]Indication{
					MessageQOSNetworkStatus: {{TLVs: tlv.TLVs{tlv.Uint(0x01, uint8(1))}}},
				},
			}
			client := &Client{transport: transport, slot: 1}

			updates, err := client.QOSWatchNetworkStatus(context.Background())
			if err != nil {
				t.Fatalf("QOSWatchNetworkStatus() error = %v", err)
			}
			supported, ok := <-updates
			if !ok || !supported {
				t.Fatalf("supported = %v, ok %v", supported, ok)
			}
			if _, ok := <-updates; ok {
				t.Fatal("updates channel remains open")
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

type qosIndicationTransport struct {
	fakeTransport
	indications map[MessageID][]Indication
}

func (t *qosIndicationTransport) Indications(_ context.Context, _ ServiceType, _ uint8, id MessageID) (<-chan Indication, error) {
	items := t.indications[id]
	out := make(chan Indication, len(items))
	for _, indication := range items {
		out <- indication
	}
	close(out)
	return out, nil
}
