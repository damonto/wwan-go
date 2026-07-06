package uim

import (
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestNASGetSysInfoRequest(t *testing.T) {
	tests := []struct {
		name string
		req  NASGetSysInfoRequest
	}{
		{
			name: "request fields",
			req: NASGetSysInfoRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.Request()
			if got.Service != qcom.ServiceNAS {
				t.Fatalf("Service = 0x%02X, want 0x%02X", got.Service, qcom.ServiceNAS)
			}
			if got.ClientID != tt.req.ClientID {
				t.Fatalf("ClientID = %d, want %d", got.ClientID, tt.req.ClientID)
			}
			if got.MessageID != qcom.MessageNASGetSysInfo {
				t.Fatalf("MessageID = 0x%04X, want 0x%04X", got.MessageID, qcom.MessageNASGetSysInfo)
			}
			if got.Timeout != tt.req.Timeout {
				t.Fatalf("Timeout = %v, want %v", got.Timeout, tt.req.Timeout)
			}
			if len(got.TLVs) != 0 {
				t.Fatalf("TLVs len = %d, want 0", len(got.TLVs))
			}
		})
	}
}

func TestUnmarshalNASGetSysInfoResponse(t *testing.T) {
	tests := []struct {
		name          string
		tlvs          tlv.TLVs
		wantKnown     bool
		wantSupported bool
	}{
		{name: "missing vops"},
		{name: "vops supported", tlvs: tlv.TLVs{tlv.Bytes(0x29, []byte{1})}, wantKnown: true, wantSupported: true},
		{name: "vops unsupported", tlvs: tlv.TLVs{tlv.Bytes(0x29, []byte{0})}, wantKnown: true},
		{name: "empty vops", tlvs: tlv.TLVs{tlv.Bytes(0x29, nil)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalNASGetSysInfoResponse(tt.tlvs)
			if err != nil {
				t.Fatalf("UnmarshalNASGetSysInfoResponse() error = %v", err)
			}
			if got.VoPSKnown != tt.wantKnown {
				t.Fatalf("VoPSKnown = %v, want %v", got.VoPSKnown, tt.wantKnown)
			}
			if got.VoPSSupported != tt.wantSupported {
				t.Fatalf("VoPSSupported = %v, want %v", got.VoPSSupported, tt.wantSupported)
			}
		})
	}
}
