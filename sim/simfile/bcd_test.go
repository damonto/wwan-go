package simfile

import (
	"bytes"
	"encoding"
	"fmt"
	"testing"
)

var (
	_ encoding.BinaryMarshaler   = BCD(nil)
	_ encoding.BinaryUnmarshaler = (*BCD)(nil)
	_ encoding.TextMarshaler     = BCD(nil)
	_ encoding.TextUnmarshaler   = (*BCD)(nil)
	_ fmt.Stringer               = BCD(nil)
)

func TestBCD(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		want     BCD
		wantText string
	}{
		{
			name:     "decimal digits",
			text:     "12345",
			want:     BCD{0x21, 0x43, 0xF5},
			wantText: "12345",
		},
		{
			name:     "hexadecimal digits",
			text:     "898600E615198A555608",
			want:     BCD{0x98, 0x68, 0x00, 0x6E, 0x51, 0x91, 0xA8, 0x55, 0x65, 0x80},
			wantText: "898600E615198A555608",
		},
		{
			name:     "internal F digit",
			text:     "89860110F9900160570",
			want:     BCD{0x98, 0x68, 0x10, 0x01, 0x9F, 0x09, 0x10, 0x06, 0x75, 0xF0},
			wantText: "89860110F9900160570",
		},
		{
			name:     "lowercase input",
			text:     "ab12",
			want:     BCD{0xBA, 0x21},
			wantText: "AB12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BCD
			if err := got.UnmarshalText([]byte(tt.text)); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("UnmarshalText() = % X, want % X", got, tt.want)
			}
			if got := got.String(); got != tt.wantText {
				t.Fatalf("String() = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestBCDUnmarshalTextError(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "non-hexadecimal digit", text: "12G3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BCD
			if err := got.UnmarshalText([]byte(tt.text)); err == nil {
				t.Fatal("UnmarshalText() error = nil")
			}
		})
	}
}

func TestBCDBinaryRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: []byte{}},
		{name: "digits", data: []byte{0x21, 0x43, 0xF5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := bytes.Clone(tt.data)
			var bcd BCD
			if err := bcd.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(data) != 0 {
				data[0] ^= 0xFF
			}
			got, err := bcd.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.data) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.data)
			}
		})
	}
}
