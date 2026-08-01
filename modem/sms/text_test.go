package sms

import (
	"bytes"
	"encoding"
	"fmt"
	"testing"
)

var (
	_ encoding.BinaryUnmarshaler = (*Part)(nil)
	_ encoding.BinaryMarshaler   = GSM7("")
	_ encoding.BinaryUnmarshaler = (*GSM7)(nil)
	_ encoding.TextMarshaler     = GSM7("")
	_ encoding.TextUnmarshaler   = (*GSM7)(nil)
	_ encoding.BinaryMarshaler   = UCS2("")
	_ encoding.BinaryUnmarshaler = (*UCS2)(nil)
	_ encoding.TextMarshaler     = UCS2("")
	_ encoding.TextUnmarshaler   = (*UCS2)(nil)
	_ encoding.BinaryMarshaler   = UCS2LE("")
	_ encoding.BinaryUnmarshaler = (*UCS2LE)(nil)
	_ encoding.TextMarshaler     = UCS2LE("")
	_ encoding.TextUnmarshaler   = (*UCS2LE)(nil)
	_ fmt.Stringer               = GSM7("")
	_ fmt.Stringer               = UCS2("")
	_ fmt.Stringer               = UCS2LE("")
)

func TestGSM7BinaryAndTextRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		text GSM7
		want []byte
	}{
		{name: "ASCII", text: "hello", want: []byte{'h', 'e', 'l', 'l', 'o'}},
		{name: "default alphabet", text: "@£", want: []byte{0x00, 0x01}},
		{name: "extension alphabet", text: "^€", want: []byte{0x1B, 0x14, 0x1B, 0x65}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.text.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.want)
			}

			var decoded GSM7
			if err := decoded.UnmarshalBinary(encoded); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if decoded.String() != tt.text.String() {
				t.Fatalf("String() = %q, want %q", decoded, tt.text)
			}

			text, err := tt.text.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(text) != tt.text.String() {
				t.Fatalf("MarshalText() = %q, want %q", text, tt.text)
			}
			if err := decoded.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
		})
	}
}

func TestGSM7TextErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "binary unrepresentable character", run: func() error { _, err := GSM7("😀").MarshalBinary(); return err }},
		{name: "text unrepresentable character", run: func() error { return new(GSM7).UnmarshalText([]byte("😀")) }},
		{name: "invalid UTF-8 text", run: func() error { return new(GSM7).UnmarshalText([]byte{0xff}) }},
		{name: "binary value exceeds septet", run: func() error { return new(GSM7).UnmarshalBinary([]byte{0x80}) }},
		{name: "trailing binary escape", run: func() error { return new(GSM7).UnmarshalBinary([]byte{0x1B}) }},
		{name: "unknown binary extension", run: func() error { return new(GSM7).UnmarshalBinary([]byte{0x1B, 0x00}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("codec error = nil, want non-nil")
			}
		})
	}
}

func TestUCS2BinaryAndTextRoundTrip(t *testing.T) {
	tests := []struct {
		name            string
		marshalBinary   func() ([]byte, error)
		unmarshalBinary func([]byte) (string, error)
		marshalText     func() ([]byte, error)
		unmarshalText   func([]byte) (string, error)
		want            []byte
		wantText        string
	}{
		{
			name:          "big endian",
			marshalBinary: func() ([]byte, error) { return UCS2("A界").MarshalBinary() },
			unmarshalBinary: func(data []byte) (string, error) {
				var text UCS2
				err := text.UnmarshalBinary(data)
				return text.String(), err
			},
			marshalText: func() ([]byte, error) { return UCS2("A界").MarshalText() },
			unmarshalText: func(data []byte) (string, error) {
				var text UCS2
				err := text.UnmarshalText(data)
				return text.String(), err
			},
			want:     []byte{0x00, 0x41, 0x75, 0x4C},
			wantText: "A界",
		},
		{
			name:          "little endian",
			marshalBinary: func() ([]byte, error) { return UCS2LE("A界").MarshalBinary() },
			unmarshalBinary: func(data []byte) (string, error) {
				var text UCS2LE
				err := text.UnmarshalBinary(data)
				return text.String(), err
			},
			marshalText: func() ([]byte, error) { return UCS2LE("A界").MarshalText() },
			unmarshalText: func(data []byte) (string, error) {
				var text UCS2LE
				err := text.UnmarshalText(data)
				return text.String(), err
			},
			want:     []byte{0x41, 0x00, 0x4C, 0x75},
			wantText: "A界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.marshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.want)
			}
			decoded, err := tt.unmarshalBinary(encoded)
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if decoded != tt.wantText {
				t.Fatalf("String() = %q, want %q", decoded, tt.wantText)
			}

			text, err := tt.marshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(text) != tt.wantText {
				t.Fatalf("MarshalText() = %q, want %q", text, tt.wantText)
			}
			decoded, err = tt.unmarshalText(text)
			if err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
			if decoded != tt.wantText {
				t.Fatalf("UnmarshalText() = %q, want %q", decoded, tt.wantText)
			}
		})
	}
}

func TestUCS2TextErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "binary supplementary character", run: func() error { _, err := UCS2("😀").MarshalBinary(); return err }},
		{name: "text supplementary character", run: func() error { return new(UCS2).UnmarshalText([]byte("😀")) }},
		{name: "marshal invalid UTF-8 text", run: func() error { _, err := UCS2(string([]byte{0xff})).MarshalText(); return err }},
		{name: "invalid UTF-8 text", run: func() error { return new(UCS2).UnmarshalText([]byte{0xff}) }},
		{name: "odd big-endian payload", run: func() error { return new(UCS2).UnmarshalBinary([]byte{0x00}) }},
		{name: "odd little-endian payload", run: func() error { return new(UCS2LE).UnmarshalBinary([]byte{0x00}) }},
		{name: "surrogate", run: func() error { return new(UCS2).UnmarshalBinary([]byte{0xD8, 0x00}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("codec error = nil, want non-nil")
			}
		})
	}
}

func TestPartUnmarshalBinaryCopiesInput(t *testing.T) {
	tests := []struct {
		name   string
		config MessageConfig
	}{
		{name: "GSM7", config: MessageConfig{Number: "+15551234", Text: "hello"}},
		{name: "UCS2", config: MessageConfig{Number: "+15551234", Text: "世界"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdus, err := EncodePDUs(tt.config)
			if err != nil {
				t.Fatalf("EncodePDUs() error = %v", err)
			}
			want := bytes.Clone(pdus[0])
			var part Part
			if err := part.UnmarshalBinary(pdus[0]); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			pdus[0][0] ^= 0xFF
			if !bytes.Equal(part.Message.PDU, want) {
				t.Fatalf("Message.PDU = % X, want % X", part.Message.PDU, want)
			}
		})
	}
}
