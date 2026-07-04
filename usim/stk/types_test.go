package stk

import (
	"bytes"
	"encoding"
	"testing"
)

func TestTextBinaryAndText(t *testing.T) {
	var _ encoding.BinaryMarshaler = Text{}
	var _ encoding.BinaryUnmarshaler = (*Text)(nil)
	var _ encoding.TextMarshaler = Text{}
	var _ encoding.TextUnmarshaler = (*Text)(nil)

	tests := []struct {
		name string
		data []byte
		want Text
	}{
		{
			name: "null",
			want: Text{},
		},
		{
			name: "eight bit",
			data: []byte{0x04, 'H', 'i'},
			want: Text{DCS: 0x04, Raw: []byte("Hi"), String: "Hi"},
		},
		{
			name: "ucs2",
			data: []byte{0x08, 0x4F, 0x60, 0x59, 0x7D},
			want: Text{DCS: 0x08, Raw: []byte{0x4F, 0x60, 0x59, 0x7D}, String: "你好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Text
			if err := got.UnmarshalBinary(tt.data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.DCS != tt.want.DCS || got.String != tt.want.String || !bytes.Equal(got.Raw, tt.want.Raw) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}

			encoded, err := got.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.data) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.data)
			}

			text, err := got.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(text) != tt.want.String {
				t.Fatalf("MarshalText() = %q, want %q", text, tt.want.String)
			}
		})
	}
}

func TestTextUnmarshalAlphaIdentifier(t *testing.T) {
	var got Text
	if err := got.UnmarshalText([]byte("main")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if got.DCS != 0x04 || got.String != "main" || !bytes.Equal(got.Raw, []byte("main")) {
		t.Fatalf("UnmarshalText() = %+v, want main alpha identifier", got)
	}
}

func TestGSM7Text(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "ascii", want: "Hello"},
		{name: "default alphabet", want: "@£$èéùìòÇ"},
		{name: "extension alphabet", want: "^{}\\[~]|€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := gsm7Text(tt.want).MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}

			var got gsm7Text
			if err := got.UnmarshalText(encoded); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("UnmarshalText() = %q, want %q", got, tt.want)
			}
		})
	}
}
