package qcom

import (
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestSARSetRFStateRequest(t *testing.T) {
	tests := []struct {
		name      string
		state     SARRFState
		wantError bool
	}{
		{name: "lowest", state: SARRFState0},
		{name: "highest", state: SARRFState20},
		{name: "out of range", state: SARRFState20 + 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (SARSetRFStateRequest{ClientID: 7, TransactionID: 9, State: tt.state}).Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if req.Service != ServiceSAR || req.ClientID != 7 || req.TransactionID != 9 || req.MessageID != MessageSARRFSetState {
				t.Fatalf("Request() = %+v, want SAR RF Set State", req)
			}
			assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, uint32(tt.state)))
		})
	}
}

func TestSARGetRFStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		want      SARRFState
		wantError bool
	}{
		{name: "state", tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(SARRFState7))}, want: SARRFState7},
		{name: "missing", wantError: true},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{7, 0, 0})}, wantError: true},
		{name: "trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{7, 0, 0, 0, 0})}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SARGetRFStateResponse
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
