package mbim

import (
	"bytes"
	"testing"
)

func TestSessionIDTLV(t *testing.T) {
	tests := []struct {
		name      string
		sessionID SessionID
		wantData  []byte
	}{
		{name: "zero", wantData: []byte{0, 0, 0, 0}},
		{name: "value", sessionID: 0x12345678, wantData: []byte{0x78, 0x56, 0x34, 0x12}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv := NewSessionIDTLV(tt.sessionID)
			if tlv.Type != TLVTypeSessionID || !bytes.Equal(tlv.Data, tt.wantData) {
				t.Fatalf("NewSessionIDTLV() = %+v, want data %x", tlv, tt.wantData)
			}
			var got SessionID
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if got != tt.sessionID {
				t.Fatalf("UnmarshalTLV() = %d, want %d", got, tt.sessionID)
			}
		})
	}
}

func TestSessionIDUnmarshalTLVValidation(t *testing.T) {
	tests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO, Data: make([]byte, 4)}},
		{name: "empty", tlv: TLV{Type: TLVTypeSessionID}},
		{name: "truncated", tlv: TLV{Type: TLVTypeSessionID, Data: make([]byte, 3)}},
		{name: "trailing data", tlv: TLV{Type: TLVTypeSessionID, Data: make([]byte, 5)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SessionID
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
