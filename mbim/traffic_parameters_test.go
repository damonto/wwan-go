package mbim

import (
	"bytes"
	"testing"
)

func TestTrafficParametersTLV(t *testing.T) {
	tests := []struct {
		name       string
		descriptor []byte
		wantData   []byte
	}{
		{name: "empty", wantData: []byte{0, 0}},
		{name: "descriptor", descriptor: []byte{0x11, 0x22, 0x33}, wantData: []byte{0, 3, 0x11, 0x22, 0x33}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := TrafficParameters{TrafficDescriptor: tt.descriptor}
			tlv, err := NewTrafficParametersTLV(value)
			if err != nil {
				t.Fatalf("NewTrafficParametersTLV() error = %v", err)
			}
			if tlv.Type != TLVTypeTrafficParameters || !bytes.Equal(tlv.Data, tt.wantData) {
				t.Fatalf("NewTrafficParametersTLV() = %+v, want data %x", tlv, tt.wantData)
			}

			var got TrafficParameters
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if !bytes.Equal(got.TrafficDescriptor, tt.descriptor) {
				t.Fatalf("TrafficDescriptor = %x, want %x", got.TrafficDescriptor, tt.descriptor)
			}
		})
	}
}

func TestTrafficParametersTLVValidation(t *testing.T) {
	marshalTests := []struct {
		name  string
		value TrafficParameters
	}{
		{name: "descriptor exceeds UINT16", value: TrafficParameters{TrafficDescriptor: make([]byte, 1<<16)}},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewTrafficParametersTLV(tt.value); err == nil {
				t.Fatal("NewTrafficParametersTLV() error = nil, want non-nil")
			}
		})
	}

	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO, Data: []byte{0, 0}}},
		{name: "truncated length", tlv: TLV{Type: TLVTypeTrafficParameters, Data: []byte{0}}},
		{name: "truncated descriptor", tlv: TLV{Type: TLVTypeTrafficParameters, Data: []byte{0, 2, 0xaa}}},
		{name: "trailing data", tlv: TLV{Type: TLVTypeTrafficParameters, Data: []byte{0, 1, 0xaa, 0xbb}}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got TrafficParameters
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
