package qcom

import (
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestIMSPGetEnablerStateRequest(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "empty request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := (IMSPGetEnablerStateRequest{ClientID: 7, TransactionID: 9}).Request()
			if req.Service != ServiceIMSP || req.ClientID != 7 || req.TransactionID != 9 || req.MessageID != MessageIMSPGetEnablerState {
				t.Fatalf("Request() = %+v, want IMSP Get Enabler State", req)
			}
			if len(req.TLVs) != 0 {
				t.Fatalf("TLVs = %v, want none", req.TLVs)
			}
		})
	}
}

func TestIMSPGetEnablerStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		want      IMSPEnablerState
		wantError bool
	}{
		{name: "registered", tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(IMSPEnablerRegistered))}, want: IMSPEnablerRegistered},
		{name: "missing", wantError: true},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{4, 0, 0})}, wantError: true},
		{name: "trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{4, 0, 0, 0, 0})}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSPGetEnablerStateResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && got.State != tt.want {
				t.Fatalf("State = %d, want %d", got.State, tt.want)
			}
		})
	}
}
