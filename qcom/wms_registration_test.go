package qcom

import (
	"context"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSIndicationRegistration(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WMSIndicationState
		wantErr bool
	}{
		{
			name: "partial state",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x11, []byte{0}),
				tlv.Bytes(0x18, []byte{1}),
			},
			want: WMSIndicationState{
				TransportLayer: true, TransportLayerKnown: true,
				TransportNetworkKnown: true,
				MemoryFull:            true, MemoryFullKnown: true,
			},
		},
		{name: "empty state"},
		{name: "invalid boolean", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{2})}, wantErr: true},
		{name: "invalid length", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWMS || req.ClientID != 7 || req.MessageID != MessageWMSGetIndicationRegister {
						t.Fatalf("request = %+v", req)
					}
					assertNoRequestTLVs(t, req)
				},
				resp: successResponse(MessageWMSGetIndicationRegister, tt.tlvs...),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			got, err := client.WMSIndicationRegistration(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("WMSIndicationRegistration() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("WMSIndicationRegistration() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
