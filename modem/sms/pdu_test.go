package sms

import (
	"bytes"
	"encoding"
	"strings"
	"testing"
	"time"
)

var _ encoding.BinaryUnmarshaler = (*smsTimestamp)(nil)

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
		{name: "UTF16 emoji single", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("🙏", 35)}, wantParts: 1},
		{name: "UTF16 emoji multipart", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("🙏", 36)}, wantParts: 2},
		{name: "UTF16 surrogate at multipart boundary", config: MessageConfig{Number: "+15551234", Text: strings.Repeat("界", 66) + "🙏" + strings.Repeat("界", 3)}, wantParts: 2},
		{name: "UTF16 emoji modifier", config: MessageConfig{Number: "+15551234", Text: "麻煩你了🙏🏻"}, wantParts: 1},
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
				var part Part
				if err := part.UnmarshalBinary(pdu); err != nil {
					t.Fatalf("UnmarshalBinary() error = %v", err)
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
			septets, err := GSM7("hello").MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			pdu := buildSubmitPDU(MessageConfig{Number: "+15551234"}, smsAlphabetGSM7, septets, tt.header)
			var part Part
			if err := part.UnmarshalBinary(pdu); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
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

func TestDecodeSMSPDUUndefinedExtension(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
	}{
		{name: "single part"},
		{name: "concatenated part with septet padding", header: []byte{5, 0, 3, 0x42, 2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			septets := []byte("<#> Code 123456. \x1b\x00 AbCdEf12345")
			packed, headerSeptets := PackSeptets(septets, tt.header)
			first := byte(0)
			if len(tt.header) != 0 {
				first = 0x40
			}
			pdu := []byte{0, first, 4, 0x91, 0x21, 0x43, 0, 0}
			pdu = append(pdu, 0x62, 0x90, 0x51, 0x41, 0, 0, 0)
			pdu = append(pdu, byte(headerSeptets+len(septets)))
			pdu = append(pdu, packed...)

			var part Part
			if err := part.UnmarshalBinary(pdu); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			const want = "<#> Code 123456. @ AbCdEf12345"
			if part.Message.Text != want {
				t.Errorf("text = %q, want %q", part.Message.Text, want)
			}
			if !bytes.Equal(part.Message.PDU, pdu) {
				t.Error("UnmarshalBinary() did not retain the original PDU")
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
			septets, err := GSM7("hello").MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			header := []byte{5, 0, 3, 0x42, 3, 2}
			packed, headerSeptets := PackSeptets(septets, header)
			pdu := append([]byte{0, 0x40}, test.address...)
			pdu = append(pdu, 0, 0)
			pdu = append(pdu, make([]byte, 7)...)
			pdu = append(pdu, byte(headerSeptets+len(septets)))
			pdu = append(pdu, packed...)

			var part Part
			if err := part.UnmarshalBinary(pdu); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
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

func TestSMSTimestampUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name       string
		value      []byte
		want       time.Time
		wantOffset int
		wantErr    bool
	}{
		{
			name:  "August regression",
			value: []byte{0x62, 0x80, 0x11, 0x21, 0x43, 0x65, 0x00},
			want:  time.Date(2026, time.August, 11, 12, 34, 56, 0, time.UTC),
		},
		{
			name:  "date and time fields ending in eight or nine",
			value: []byte{0x92, 0x90, 0x92, 0x91, 0x85, 0x95, 0x00},
			want:  time.Date(2029, time.September, 29, 19, 58, 59, 0, time.UTC),
		},
		{
			name:       "positive timezone containing nine",
			value:      []byte{0x62, 0x80, 0x11, 0x21, 0x43, 0x65, 0x93},
			want:       time.Date(2026, time.August, 11, 12, 34, 56, 0, time.FixedZone("want", 9*60*60+45*60)),
			wantOffset: 9*60*60 + 45*60,
		},
		{
			name:       "negative timezone",
			value:      []byte{0x62, 0x80, 0x11, 0x21, 0x43, 0x65, 0x2a},
			want:       time.Date(2026, time.August, 11, 12, 34, 56, 0, time.FixedZone("want", -(5*60*60+30*60))),
			wantOffset: -(5*60*60 + 30*60),
		},
		{name: "truncated", value: make([]byte, 6), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var timestamp smsTimestamp
			err := timestamp.UnmarshalBinary(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got := time.Time(timestamp)
			if !got.Equal(tt.want) {
				t.Errorf("UnmarshalBinary() = %v, want %v", got, tt.want)
			}
			if _, gotOffset := got.Zone(); gotOffset != tt.wantOffset {
				t.Errorf("UnmarshalBinary() offset = %d, want %d", gotOffset, tt.wantOffset)
			}
		})
	}
}

func TestSMSAssemblerOutOfOrderAndDuplicate(t *testing.T) {
	pdus, err := EncodePDUs(MessageConfig{Number: "+15551234", Text: strings.Repeat("x", 307)})
	if err != nil {
		t.Fatalf("EncodePDUs() error = %v", err)
	}
	parts := make([]Part, len(pdus))
	for i, pdu := range pdus {
		if err := parts[i].UnmarshalBinary(pdu); err != nil {
			t.Fatalf("Part[%d].UnmarshalBinary() error = %v", i, err)
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
			var part Part
			if err := part.UnmarshalBinary(tt.pdu); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestDecodeSMSStatusReport(t *testing.T) {
	pdu := []byte{0, 2, 0x2a}
	pdu = appendAddress(pdu, "+15551234")
	pdu = append(pdu, make([]byte, 15)...)
	var part Part
	if err := part.UnmarshalBinary(pdu); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if !part.Message.DeliveryReport || part.Message.MessageReference != 0x2a {
		t.Errorf("status report = %+v, want delivery report reference 42", part.Message)
	}
}
