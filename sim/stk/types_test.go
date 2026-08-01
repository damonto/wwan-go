package stk

import (
	"bytes"
	"encoding"
	"fmt"
	"testing"
)

func TestTextBinaryAndText(t *testing.T) {
	var _ encoding.BinaryMarshaler = TextString{}
	var _ encoding.BinaryUnmarshaler = (*TextString)(nil)
	var _ encoding.BinaryMarshaler = AlphaIdentifier{}
	var _ encoding.BinaryUnmarshaler = (*AlphaIdentifier)(nil)
	var _ encoding.BinaryMarshaler = gsm7Text("")
	var _ encoding.BinaryUnmarshaler = (*gsm7Text)(nil)
	var _ encoding.TextMarshaler = gsm7Text("")
	var _ encoding.TextUnmarshaler = (*gsm7Text)(nil)
	var _ fmt.Stringer = gsm7Text("")
	var _ encoding.BinaryMarshaler = gsmDefaultText("")
	var _ encoding.BinaryUnmarshaler = (*gsmDefaultText)(nil)
	var _ encoding.TextMarshaler = gsmDefaultText("")
	var _ encoding.TextUnmarshaler = (*gsmDefaultText)(nil)
	var _ fmt.Stringer = gsmDefaultText("")
	var _ encoding.BinaryMarshaler = ucs2Text("")
	var _ encoding.BinaryUnmarshaler = (*ucs2Text)(nil)
	var _ encoding.TextMarshaler = ucs2Text("")
	var _ encoding.TextUnmarshaler = (*ucs2Text)(nil)
	var _ fmt.Stringer = ucs2Text("")

	tests := []struct {
		name string
		data []byte
		want TextString
	}{
		{
			name: "null",
			want: TextString{},
		},
		{
			name: "eight bit",
			data: []byte{0x04, 'H', 'i'},
			want: TextString{DCS: DCSGSM8Unpacked, Value: "Hi"},
		},
		{
			name: "eight bit GSM alphabet",
			data: []byte{0x04, 0x00, 0x01, 0x1b, 0x65},
			want: TextString{DCS: DCSGSM8Unpacked, Value: "@£€"},
		},
		{
			name: "ucs2",
			data: []byte{0x08, 0x4F, 0x60, 0x59, 0x7D},
			want: TextString{DCS: DCSUCS2, Value: "你好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got TextString
			if err := got.UnmarshalBinary(tt.data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.DCS != tt.want.DCS || got.Value != tt.want.Value {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}

			encoded, err := got.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.data) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.data)
			}

			if got.String() != tt.want.Value {
				t.Fatalf("String() = %q, want %q", got.String(), tt.want.Value)
			}
		})
	}
}

func TestAlphaIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{name: "ASCII", data: []byte("main"), want: "main"},
		{name: "GSM default alphabet", data: []byte{0x00, 0x01, 0x02, 0x03}, want: "@£$¥"},
		{name: "GSM extension alphabet", data: []byte{0x1b, 0x14, 0x1b, 0x65}, want: "^€"},
		{name: "UCS2 80 Chinese", data: []byte{0x80, 0x4f, 0x60, 0x59, 0x7d}, want: "你好"},
		{name: "UCS2 80 character ending FF", data: []byte{0x80, 0x4f, 0xff}, want: string(rune(0x4fff))},
		{name: "UCS2 80 padding", data: []byte{0x80, 0x4f, 0xff, 0xff, 0xff}, want: string(rune(0x4fff))},
		{name: "UCS2 81 mixed", data: []byte{0x81, 0x03, 0x02, 'A', 0x80, 0x81}, want: "AĀā"},
		{name: "UCS2 82 mixed", data: []byte{0x82, 0x03, 0x4f, 0x00, 'A', 0xe0, 0xe1}, want: "A你佡"},
		{name: "trailing padding", data: []byte{'m', 'a', 'i', 'n', 0xff, 0xff}, want: "main"},
		{name: "empty", data: nil, want: ""},
		{name: "truncated 80", data: []byte{0x80, 0x4f}, wantErr: true},
		{name: "truncated 81", data: []byte{0x81, 0x02, 0x02, 0x80}, wantErr: true},
		{name: "truncated 82", data: []byte{0x82, 0x01, 0x4f}, wantErr: true},
		{name: "UCS2 82 exceeds BMP", data: []byte{0x82, 0x01, 0xff, 0xff, 0xff}, wantErr: true},
		{name: "UCS2 82 surrogate", data: []byte{0x82, 0x01, 0xd8, 0x00, 0x80}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got AlphaIdentifier
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.Value != tt.want {
				t.Fatalf("UnmarshalBinary() = %+v, want value %q", got, tt.want)
			}
		})
	}
}

func TestTextStringMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		text    TextString
		want    []byte
		wantErr bool
	}{
		{name: "packed GSM DCS zero", text: TextString{DCS: DCSGSM7Packed, Value: "Hi"}, want: []byte{0x00, 0xc8, 0x34}},
		{name: "unpacked GSM", text: TextString{DCS: DCSGSM8Unpacked, Value: "@£€"}, want: []byte{0x04, 0x00, 0x01, 0x1b, 0x65}},
		{name: "UCS2", text: TextString{DCS: DCSUCS2, Value: "你好"}, want: []byte{0x08, 0x4f, 0x60, 0x59, 0x7d}},
		{name: "GSM rejects Chinese", text: TextString{DCS: DCSGSM8Unpacked, Value: "你好"}, wantErr: true},
		{name: "UCS2 rejects supplementary character", text: TextString{DCS: DCSUCS2, Value: "😀"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.text.MarshalBinary()
			if tt.wantErr {
				if err == nil {
					t.Fatal("MarshalBinary() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestTextValuesAreMarshalSource(t *testing.T) {
	var text TextString
	if err := text.UnmarshalBinary([]byte{0x04, 'o', 'l', 'd'}); err != nil {
		t.Fatalf("TextString.UnmarshalBinary() error = %v", err)
	}
	text.Value = "new"
	encoded, err := text.MarshalBinary()
	if err != nil {
		t.Fatalf("TextString.MarshalBinary() error = %v", err)
	}
	if !bytes.Equal(encoded, []byte{0x04, 'n', 'e', 'w'}) {
		t.Fatalf("TextString.MarshalBinary() = % X, want new value", encoded)
	}

	var alpha AlphaIdentifier
	if err := alpha.UnmarshalBinary([]byte("old")); err != nil {
		t.Fatalf("AlphaIdentifier.UnmarshalBinary() error = %v", err)
	}
	alpha.Value = "新"
	encoded, err = alpha.MarshalBinary()
	if err != nil {
		t.Fatalf("AlphaIdentifier.MarshalBinary() error = %v", err)
	}
	if !bytes.Equal(encoded, []byte{0x80, 0x65, 0xb0}) {
		t.Fatalf("AlphaIdentifier.MarshalBinary() = % X, want UCS2 新", encoded)
	}
}

func TestTextDecodingRejectsInvalidCharacters(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "unknown GSM extension", run: func() error { return new(gsmDefaultText).UnmarshalBinary([]byte{0x1b, 0x00}) }},
		{name: "invalid GSM UTF-8", run: func() error { return new(gsmDefaultText).UnmarshalText([]byte{0xff}) }},
		{name: "unrepresentable GSM text", run: func() error { return new(gsmDefaultText).UnmarshalText([]byte("😀")) }},
		{name: "lone high surrogate", run: func() error { return new(ucs2Text).UnmarshalBinary([]byte{0xd8, 0x00}) }},
		{name: "lone low surrogate", run: func() error { return new(ucs2Text).UnmarshalBinary([]byte{0xdc, 0x00}) }},
		{name: "UTF16 surrogate pair", run: func() error { return new(ucs2Text).UnmarshalBinary([]byte{0xd8, 0x3d, 0xde, 0x00}) }},
		{name: "marshal invalid UCS2 UTF-8", run: func() error { _, err := ucs2Text(string([]byte{0xff})).MarshalText(); return err }},
		{name: "invalid UCS2 UTF-8", run: func() error { return new(ucs2Text).UnmarshalText([]byte{0xff}) }},
		{name: "unrepresentable UCS2 text", run: func() error { return new(ucs2Text).UnmarshalText([]byte("😀")) }},
		{name: "alpha supplementary character", run: func() error { _, err := (AlphaIdentifier{Value: "😀"}).MarshalBinary(); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("decode error = nil, want error")
			}
		})
	}
}

func TestGSM7BinaryAndText(t *testing.T) {
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
			encoded, err := gsm7Text(tt.want).MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}

			var got gsm7Text
			if err := got.UnmarshalBinary(encoded); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("UnmarshalBinary() = %q, want %q", got, tt.want)
			}

			text, err := got.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error = %v", err)
			}
			if string(text) != tt.want {
				t.Fatalf("MarshalText() = %q, want %q", text, tt.want)
			}
			if err := got.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
		})
	}
}
