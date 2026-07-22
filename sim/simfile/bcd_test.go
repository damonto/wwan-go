package simfile

import (
	"bytes"
	"testing"
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
			got, err := NewBCD(tt.text)
			if err != nil {
				t.Fatalf("NewBCD() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("NewBCD() = % X, want % X", got, tt.want)
			}
			if got := got.String(); got != tt.wantText {
				t.Fatalf("String() = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestNewBCDError(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "non-hexadecimal digit", text: "12G3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewBCD(tt.text); err == nil {
				t.Fatal("NewBCD() error = nil")
			}
		})
	}
}
