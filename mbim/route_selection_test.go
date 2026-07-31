package mbim

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestRouteSelectionDescriptorsTLV(t *testing.T) {
	tests := []struct {
		name     string
		values   RouteSelectionDescriptors
		wantData []byte
	}{
		{
			name: "one descriptor",
			values: []RouteSelectionDescriptor{
				{
					Source:     RouteSelectionDescriptorSourceUser,
					Purpose:    RouteSelectionDescriptorPurposePurchase,
					Precedence: 2,
					Contents:   []byte{0x11, 0x22},
				},
			},
			wantData: []byte{
				1, 0, 0, 0,
				1, 0, 0, 0,
				0, 5,
				2,
				0, 2,
				0x11, 0x22,
			},
		},
		{
			name: "multiple descriptors",
			values: []RouteSelectionDescriptor{
				{
					Source:     RouteSelectionDescriptorSourceModemLocal,
					Precedence: 0xff,
					Contents:   []byte{0x01},
				},
				{
					Source:     RouteSelectionDescriptorSourceDevice,
					Purpose:    RouteSelectionDescriptorPurposePurchase,
					Precedence: 1,
					Contents:   []byte{0xaa, 0xbb, 0xcc},
				},
			},
			wantData: []byte{
				6, 0, 0, 0,
				0, 0, 0, 0,
				0, 4,
				0xff,
				0, 1,
				0x01,
				4, 0, 0, 0,
				1, 0, 0, 0,
				0, 6,
				1,
				0, 3,
				0xaa, 0xbb, 0xcc,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewRouteSelectionDescriptorsTLV(tt.values)
			if err != nil {
				t.Fatalf("NewRouteSelectionDescriptorsTLV() error = %v", err)
			}
			if tlv.Type != TLVTypeRouteSelectionDescriptors || !bytes.Equal(tlv.Data, tt.wantData) {
				t.Fatalf("NewRouteSelectionDescriptorsTLV() = %+v, want data %x", tlv, tt.wantData)
			}

			var got RouteSelectionDescriptors
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.values) {
				t.Fatalf("UnmarshalTLV() = %+v, want %+v", got, tt.values)
			}
		})
	}
}

func TestRouteSelectionDescriptorsTLVValidation(t *testing.T) {
	marshalTests := []struct {
		name   string
		values RouteSelectionDescriptors
	}{
		{name: "empty list"},
		{name: "empty contents", values: []RouteSelectionDescriptor{{}}},
		{
			name: "reserved source",
			values: []RouteSelectionDescriptor{{
				Source:   RouteSelectionDescriptorSourceModemLocal + 1,
				Contents: []byte{1},
			}},
		},
		{
			name: "reserved purpose",
			values: []RouteSelectionDescriptor{{
				Purpose:  RouteSelectionDescriptorPurpose(1 << 1),
				Contents: []byte{1},
			}},
		},
		{
			name: "descriptor length exceeds UINT16",
			values: []RouteSelectionDescriptor{{
				Contents: make([]byte, int(^uint16(0))-2),
			}},
		},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewRouteSelectionDescriptorsTLV(tt.values); err == nil {
				t.Fatal("NewRouteSelectionDescriptorsTLV() error = nil, want non-nil")
			}
		})
	}

	valid := []byte{
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 4,
		0,
		0, 1,
		0x01,
	}
	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO, Data: valid}},
		{name: "empty list", tlv: TLV{Type: TLVTypeRouteSelectionDescriptors}},
		{name: "truncated header", tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: make([]byte, 9)}},
		{
			name: "reserved source",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: func() []byte {
				data := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(data[:4], uint32(RouteSelectionDescriptorSourceModemLocal+1))
				return data
			}()},
		},
		{
			name: "reserved purpose",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: func() []byte {
				data := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(data[4:8], 1<<1)
				return data
			}()},
		},
		{
			name: "descriptor length too small",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: []byte{
				0, 0, 0, 0,
				0, 0, 0, 0,
				0, 3,
				0,
				0, 0,
			}},
		},
		{
			name: "truncated descriptor",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: []byte{
				0, 0, 0, 0,
				0, 0, 0, 0,
				0, 5,
				0,
				0, 2,
				0x01,
			}},
		},
		{
			name: "contents length exceeds descriptor",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: []byte{
				0, 0, 0, 0,
				0, 0, 0, 0,
				0, 4,
				0,
				0, 2,
				0x01,
			}},
		},
		{
			name: "contents length leaves descriptor data",
			tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: []byte{
				0, 0, 0, 0,
				0, 0, 0, 0,
				0, 5,
				0,
				0, 1,
				0x01, 0x02,
			}},
		},
		{name: "trailing truncated descriptor", tlv: TLV{Type: TLVTypeRouteSelectionDescriptors, Data: append(valid, 0)}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got RouteSelectionDescriptors
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
