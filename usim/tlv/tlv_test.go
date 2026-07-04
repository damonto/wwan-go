package tlv

import (
	"bytes"
	"encoding"
	"errors"
	"io"
	"testing"
)

func TestTypesImplementStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = Item{}
	var _ encoding.BinaryUnmarshaler = (*Item)(nil)
	var _ io.WriterTo = Item{}
	var _ encoding.BinaryMarshaler = Items{}
	var _ encoding.BinaryUnmarshaler = (*Items)(nil)
	var _ io.WriterTo = Items{}
	var _ io.ReaderFrom = (*Items)(nil)
}

func TestItemMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		item    Item
		want    []byte
		wantErr error
	}{
		{
			name: "short form length",
			item: Item{Tag: 0x80, Value: []byte("abc")},
			want: []byte{0x80, 0x03, 'a', 'b', 'c'},
		},
		{
			name: "long form length 0x81",
			item: Item{Tag: 0x80, Value: make([]byte, 0x82)},
			want: append([]byte{0x80, 0x81, 0x82}, make([]byte, 0x82)...),
		},
		{
			name: "long form length 0x82",
			item: Item{Tag: 0x80, Value: make([]byte, 0x100)},
			want: append([]byte{0x80, 0x82, 0x01, 0x00}, make([]byte, 0x100)...),
		},
		{
			name:    "reject oversized value",
			item:    Item{Tag: 0x80, Value: make([]byte, 0x10000)},
			wantErr: errValueTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.item.MarshalBinary()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want %v", err, tt.wantErr)
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

func TestComprehensionItem(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		tag      byte
		required bool
		want     []byte
	}{
		{
			name:     "required",
			item:     NewComprehension(0x01, []byte{0x01, 0x21, 0x00}),
			tag:      0x01,
			required: true,
			want:     []byte{0x81, 0x03, 0x01, 0x21, 0x00},
		},
		{
			name: "optional",
			item: New(0x0D, []byte{0x04, 'h', 'i'}),
			tag:  0x0D,
			want: []byte{0x0D, 0x03, 0x04, 'h', 'i'},
		},
		{
			name:     "preserve high bit",
			item:     New(0xFE, []byte{0xAA}),
			tag:      0x7E,
			required: true,
			want:     []byte{0xFE, 0x01, 0xAA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.ComprehensionTag(); got != tt.tag {
				t.Fatalf("ComprehensionTag() = 0x%02X, want 0x%02X", got, tt.tag)
			}
			if got := tt.item.ComprehensionRequired(); got != tt.required {
				t.Fatalf("ComprehensionRequired() = %t, want %t", got, tt.required)
			}
			got, err := tt.item.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestItemUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    Item
		wantErr error
	}{
		{
			name: "short form length",
			data: []byte{0x80, 0x03, 'a', 'b', 'c'},
			want: Item{Tag: 0x80, Value: []byte("abc")},
		},
		{
			name: "long form length 0x81",
			data: append([]byte{0x80, 0x81, 0x82}, make([]byte, 0x82)...),
			want: Item{Tag: 0x80, Value: make([]byte, 0x82)},
		},
		{
			name: "long form length 0x82",
			data: append([]byte{0x80, 0x82, 0x01, 0x00}, make([]byte, 0x100)...),
			want: Item{Tag: 0x80, Value: make([]byte, 0x100)},
		},
		{
			name:    "indefinite length unsupported",
			data:    []byte{0x80, 0x80, 0x00},
			wantErr: ErrMalformed,
		},
		{
			name:    "reject trailing bytes",
			data:    []byte{0x80, 0x01, 0xAA, 0x81, 0x00},
			wantErr: ErrMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Item
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UnmarshalBinary() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.Tag != tt.want.Tag {
				t.Fatalf("UnmarshalBinary().Tag = 0x%02X, want 0x%02X", got.Tag, tt.want.Tag)
			}
			if !bytes.Equal(got.Value, tt.want.Value) {
				t.Fatalf("UnmarshalBinary().Value = % X, want % X", got.Value, tt.want.Value)
			}
		})
	}
}

func TestBERWrapUnwrap(t *testing.T) {
	tests := []struct {
		name string
		tag  byte
		body []byte
		want []byte
	}{
		{
			name: "short length",
			tag:  0xD0,
			body: []byte{0x81, 0x00},
			want: []byte{0xD0, 0x02, 0x81, 0x00},
		},
		{
			name: "extended length",
			tag:  0xD0,
			body: bytes.Repeat([]byte{0x11}, 0x100),
			want: append([]byte{0xD0, 0x82, 0x01, 0x00}, bytes.Repeat([]byte{0x11}, 0x100)...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := WrapBER(tt.tag, tt.body)
			if err != nil {
				t.Fatalf("WrapBER() error = %v", err)
			}
			if !bytes.Equal(raw, tt.want) {
				t.Fatalf("WrapBER() = % X, want % X", raw, tt.want)
			}

			tag, body, err := UnwrapBER(raw)
			if err != nil {
				t.Fatalf("UnwrapBER() error = %v", err)
			}
			if tag != tt.tag || !bytes.Equal(body, tt.body) {
				t.Fatalf("UnwrapBER() = tag 0x%02X body % X, want tag 0x%02X body % X", tag, body, tt.tag, tt.body)
			}
		})
	}
}

func TestItemsRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		items Items
	}{
		{
			name: "multiple items",
			items: Items{
				{Tag: 0x80, Value: []byte("abc")},
				{Tag: 0x81, Value: []byte{0x01, 0x02}},
			},
		},
		{
			name:  "empty sequence",
			items: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.items.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}

			var got Items
			if _, err := got.ReadFrom(bytes.NewReader(data)); err != nil {
				t.Fatalf("ReadFrom() error = %v", err)
			}

			if len(got) != len(tt.items) {
				t.Fatalf("len(UnmarshalBinary()) = %d, want %d", len(got), len(tt.items))
			}
			for i := range got {
				if got[i].Tag != tt.items[i].Tag {
					t.Fatalf("item %d tag = 0x%02X, want 0x%02X", i, got[i].Tag, tt.items[i].Tag)
				}
				if !bytes.Equal(got[i].Value, tt.items[i].Value) {
					t.Fatalf("item %d value = % X, want % X", i, got[i].Value, tt.items[i].Value)
				}
			}
		})
	}
}
