package mbim

import (
	"bytes"
	"testing"
)

func TestOSIDTLV(t *testing.T) {
	tests := []struct {
		name string
		osid OSID
	}{
		{name: "zero"},
		{
			name: "UUID",
			osid: OSID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv := NewOSIDTLV(tt.osid)
			if tlv.Type != TLVTypeOSID || !bytes.Equal(tlv.Data, tt.osid[:]) {
				t.Fatalf("NewOSIDTLV() = %+v, want type %d data %x", tlv, TLVTypeOSID, tt.osid)
			}

			var got OSID
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if got != tt.osid {
				t.Fatalf("UnmarshalTLV() = %x, want %x", got, tt.osid)
			}
		})
	}
}

func TestOSIDUnmarshalTLVRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO, Data: make([]byte, 16)}},
		{name: "truncated", tlv: TLV{Type: TLVTypeOSID, Data: make([]byte, 15)}},
		{name: "trailing data", tlv: TLV{Type: TLVTypeOSID, Data: make([]byte, 17)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OSID
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
