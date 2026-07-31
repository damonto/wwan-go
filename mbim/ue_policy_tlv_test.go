package mbim

import (
	"bytes"
	"reflect"
	"testing"
)

func TestURSPRulesTDOnlyTLV(t *testing.T) {
	tests := []struct {
		name     string
		rules    URSPRules
		wantData []byte
	}{
		{name: "no rules"},
		{
			name: "one rule",
			rules: []URSPTrafficDescriptor{
				{Precedence: 1, Data: []byte{0xaa}},
			},
			wantData: []byte{1, 0, 1, 0xaa},
		},
		{
			name: "multiple rules",
			rules: []URSPTrafficDescriptor{
				{Precedence: 0, Data: []byte{0x01}},
				{Precedence: 0xff, Data: []byte{0x10, 0x20, 0x30}},
			},
			wantData: []byte{0, 0, 1, 0x01, 0xff, 0, 3, 0x10, 0x20, 0x30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewURSPRulesTDOnlyTLV(tt.rules)
			if err != nil {
				t.Fatalf("NewURSPRulesTDOnlyTLV() error = %v", err)
			}
			if tlv.Type != TLVTypeURSPRulesTDOnly || !bytes.Equal(tlv.Data, tt.wantData) {
				t.Fatalf("NewURSPRulesTDOnlyTLV() = %+v, want data %x", tlv, tt.wantData)
			}

			var got URSPRules
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.rules) {
				t.Fatalf("UnmarshalTLV() = %+v, want %+v", got, tt.rules)
			}
		})
	}
}

func TestURSPRulesTDOnlyTLVValidation(t *testing.T) {
	marshalTests := []struct {
		name  string
		rules URSPRules
	}{
		{
			name: "duplicate precedence",
			rules: []URSPTrafficDescriptor{
				{Precedence: 1, Data: []byte{0x01}},
				{Precedence: 1, Data: []byte{0x02}},
			},
		},
		{name: "empty traffic descriptor", rules: []URSPTrafficDescriptor{{}}},
		{
			name: "traffic descriptor exceeds UINT16",
			rules: []URSPTrafficDescriptor{{
				Data: make([]byte, 1<<16),
			}},
		},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewURSPRulesTDOnlyTLV(tt.rules); err == nil {
				t.Fatal("NewURSPRulesTDOnlyTLV() error = nil, want non-nil")
			}
		})
	}

	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO}},
		{name: "truncated header", tlv: TLV{Type: TLVTypeURSPRulesTDOnly, Data: []byte{1, 0}}},
		{name: "empty traffic descriptor", tlv: TLV{Type: TLVTypeURSPRulesTDOnly, Data: []byte{1, 0, 0}}},
		{name: "truncated traffic descriptor", tlv: TLV{Type: TLVTypeURSPRulesTDOnly, Data: []byte{1, 0, 2, 0xaa}}},
		{
			name: "duplicate precedence",
			tlv: TLV{Type: TLVTypeURSPRulesTDOnly, Data: []byte{
				1, 0, 1, 0xaa,
				1, 0, 1, 0xbb,
			}},
		},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got URSPRules
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
