package simfile

import (
	"bytes"
	"encoding"
	"testing"
)

func TestAddressBinary(t *testing.T) {
	var _ encoding.BinaryMarshaler = Address("")
	var _ encoding.BinaryUnmarshaler = (*Address)(nil)
	var _ encoding.TextMarshaler = Address("")
	var _ encoding.TextUnmarshaler = (*Address)(nil)

	tests := []struct {
		name string
		text string
		want []byte
	}{
		{
			name: "international",
			text: "+12345",
			want: []byte{0x91, 0x21, 0x43, 0xF5},
		},
		{
			name: "national",
			text: "1234",
			want: []byte{0x81, 0x21, 0x43},
		},
		{
			name: "formatting",
			text: "+1 (234)-5",
			want: []byte{0x91, 0x21, 0x43, 0xF5},
		},
		{
			name: "empty",
			text: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Address(tt.text).MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}

			var decoded Address
			if err := decoded.UnmarshalBinary(got); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(got) != 0 && decoded == "" {
				t.Fatal("UnmarshalBinary() decoded empty address")
			}
		})
	}
}

func TestAddressText(t *testing.T) {
	var address Address
	if err := address.UnmarshalText([]byte("+1234")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	got, err := address.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(got) != "+1234" {
		t.Fatalf("MarshalText() = %q, want +1234", got)
	}
}
