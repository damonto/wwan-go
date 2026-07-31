package sms

import (
	"bytes"
	"strings"
	"testing"
)

func TestSMSPDUBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		config    MessageConfig
		wantParts int
	}{
		{name: "GSM7 single", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("a", 160)}, wantParts: 1},
		{name: "GSM7 multipart", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("a", 161)}, wantParts: 2},
		{name: "GSM7 extension single", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("^", 80)}, wantParts: 1},
		{name: "GSM7 extension multipart", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("^", 81)}, wantParts: 2},
		{name: "UCS2 single", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("界", 70)}, wantParts: 1},
		{name: "UCS2 multipart", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("界", 71)}, wantParts: 2},
		{name: "binary single", config: MessageConfig{Number: "+15551234", Data: bytes.Repeat([]byte{0xaa}, 140)}, wantParts: 1},
		{name: "binary multipart", config: MessageConfig{Number: "+15551234", Data: bytes.Repeat([]byte{0xaa}, 141)}, wantParts: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdus, err := EncodePDUs(tt.config)
			if err != nil {
				t.Fatalf("EncodePDUs() error = %v", err)
			}
			if len(pdus) != tt.wantParts {
				t.Fatalf("len(PDUs) = %d, want %d", len(pdus), tt.wantParts)
			}
			assembler := Assembler{}
			var got Message
			for _, pdu := range pdus {
				part, err := DecodePDU(pdu)
				if err != nil {
					t.Fatalf("DecodePDU() error = %v", err)
				}
				if message, complete := assembler.Add(part); complete {
					got = message
				}
			}
			if got.Text != tt.config.Text {
				t.Errorf("decoded text = %q, want %q", got.Text, tt.config.Text)
			}
			if !bytes.Equal(got.Data, tt.config.Data) {
				t.Errorf("decoded data length = %d, want %d", len(got.Data), len(tt.config.Data))
			}
			if got.Number != tt.config.Number {
				t.Errorf("decoded number = %q, want %q", got.Number, tt.config.Number)
			}
		})
	}
}

func TestDecodeSMSPDUUDH(t *testing.T) {
	tests := []struct {
		name      string
		header    []byte
		wantRef   uint16
		wantTotal uint8
		wantIndex uint8
	}{
		{name: "8 bit reference", header: []byte{5, 0, 3, 0x42, 3, 2}, wantRef: 0x42, wantTotal: 3, wantIndex: 2},
		{name: "16 bit reference", header: []byte{6, 8, 4, 0x12, 0x34, 3, 2}, wantRef: 0x1234, wantTotal: 3, wantIndex: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			septets, _ := EncodeGSM7("hello")
			pdu := buildSubmitPDU(MessageConfig{Number: "+15551234"}, smsAlphabetGSM7, septets, tt.header)
			part, err := DecodePDU(pdu)
			if err != nil {
				t.Fatalf("DecodePDU() error = %v", err)
			}
			if part.Reference != tt.wantRef || part.Total != tt.wantTotal || part.Index != tt.wantIndex {
				t.Errorf("multipart = (%#x, %d, %d), want (%#x, %d, %d)", part.Reference, part.Total, part.Index, tt.wantRef, tt.wantTotal, tt.wantIndex)
			}
			if part.Message.Text != "hello" {
				t.Errorf("text = %q, want hello", part.Message.Text)
			}
		})
	}
}

func TestDecodeSMSPDUAlphanumericOrigin(t *testing.T) {
	tests := []struct {
		name       string
		address    []byte
		wantNumber string
	}{
		{name: "four characters in seven semi-octets", address: []byte{7, 0xd0, 0xd6, 0x27, 0x36, 0x09}, wantNumber: "VOXI"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			septets, _ := EncodeGSM7("hello")
			header := []byte{5, 0, 3, 0x42, 3, 2}
			packed, headerSeptets := PackSeptets(septets, header)
			pdu := append([]byte{0, 0x40}, test.address...)
			pdu = append(pdu, 0, 0)
			pdu = append(pdu, make([]byte, 7)...)
			pdu = append(pdu, byte(headerSeptets+len(septets)))
			pdu = append(pdu, packed...)

			part, err := DecodePDU(pdu)
			if err != nil {
				t.Fatalf("DecodePDU() error = %v", err)
			}
			if part.Message.Number != test.wantNumber {
				t.Errorf("number = %q, want %q", part.Message.Number, test.wantNumber)
			}
			if part.Message.Text != "hello" {
				t.Errorf("text = %q, want hello", part.Message.Text)
			}
			if part.Reference != 0x42 || part.Total != 3 || part.Index != 2 {
				t.Errorf("multipart = (%#x, %d, %d), want (%#x, 3, 2)", part.Reference, part.Total, part.Index, 0x42)
			}
		})
	}
}

func TestSMSAssemblerOutOfOrderAndDuplicate(t *testing.T) {
	pdus, err := EncodePDUs(MessageConfig{Number: "+15551234", Text: strings.Repeat("x", 307)})
	if err != nil {
		t.Fatal(err)
	}
	parts := make([]Part, len(pdus))
	for i, pdu := range pdus {
		parts[i], err = DecodePDU(pdu)
		if err != nil {
			t.Fatal(err)
		}
	}

	assembler := Assembler{}
	order := []int{2, 0, 0, 1}
	var got Message
	for _, index := range order {
		if message, complete := assembler.Add(parts[index]); complete {
			got = message
		}
	}
	if got.Text != strings.Repeat("x", 307) {
		t.Errorf("assembled text length = %d, want 307", len(got.Text))
	}
	if len(got.PDUs) != 3 {
		t.Errorf("len(PDUs) = %d, want 3", len(got.PDUs))
	}
}

func TestDecodeSMSPDUFailures(t *testing.T) {
	tests := []struct {
		name string
		pdu  []byte
	}{
		{name: "empty", pdu: nil},
		{name: "truncated SMSC", pdu: []byte{2, 0x91}},
		{name: "missing TPDU", pdu: []byte{0}},
		{name: "reserved MTI", pdu: []byte{0, 3}},
		{name: "truncated submit", pdu: []byte{0, 1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodePDU(tt.pdu); err == nil {
				t.Fatal("DecodePDU() error = nil, want non-nil")
			}
		})
	}
}

func TestDecodeSMSStatusReport(t *testing.T) {
	pdu := []byte{0, 2, 0x2a}
	pdu = appendAddress(pdu, "+15551234")
	pdu = append(pdu, make([]byte, 15)...)
	part, err := DecodePDU(pdu)
	if err != nil {
		t.Fatalf("DecodePDU() error = %v", err)
	}
	if !part.Message.DeliveryReport || part.Message.MessageReference != 0x2a {
		t.Errorf("status report = %+v, want delivery report reference 42", part.Message)
	}
}
