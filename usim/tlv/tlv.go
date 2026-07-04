package tlv

import (
	"bytes"
	"errors"
	"io"
	"slices"
)

var (
	ErrMalformed     = errors.New("malformed TLV")
	errValueTooLarge = errors.New("TLV value exceeds 65535 bytes")
)

type Item struct {
	Tag   byte
	Value []byte
}

type Items []Item

func New(tag byte, value []byte) Item {
	return Item{Tag: tag, Value: slices.Clone(value)}
}

func NewComprehension(tag byte, value []byte) Item {
	item := New(tag&0x7f, value)
	item.Tag |= 0x80
	return item
}

func (item Item) ComprehensionTag() byte {
	return item.Tag & 0x7f
}

func (item Item) ComprehensionRequired() bool {
	return item.Tag&0x80 != 0
}

func (item Item) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if _, err := item.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (item Item) WriteTo(w io.Writer) (int64, error) {
	length, err := marshalLength(len(item.Value))
	if err != nil {
		return 0, err
	}

	encoded := make([]byte, 0, 1+len(length)+len(item.Value))
	encoded = append(encoded, item.Tag)
	encoded = append(encoded, length...)
	encoded = append(encoded, item.Value...)
	n, err := w.Write(encoded)
	if err == nil && n != len(encoded) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func (item *Item) UnmarshalBinary(data []byte) error {
	parsed, consumed, err := consume(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return ErrMalformed
	}

	*item = parsed
	return nil
}

func (items Items) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if _, err := items.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (items *Items) UnmarshalBinary(data []byte) error {
	parsed := make(Items, 0, len(data)/2)
	for len(data) > 0 {
		item, consumed, err := consume(data)
		if err != nil {
			return err
		}
		parsed = append(parsed, item)
		data = data[consumed:]
	}

	*items = parsed
	return nil
}

func (items Items) WriteTo(w io.Writer) (int64, error) {
	var written int64
	for _, item := range items {
		n, err := item.WriteTo(w)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func (items *Items) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return int64(len(data)), err
	}
	return int64(len(data)), items.UnmarshalBinary(data)
}

func (items Items) Find(tag byte) (Item, bool) {
	tag &= 0x7f
	for _, item := range items {
		if item.ComprehensionTag() == tag {
			return CloneItem(item), true
		}
	}
	return Item{}, false
}

func (items Items) All(tag byte) Items {
	tag &= 0x7f
	out := make(Items, 0)
	for _, item := range items {
		if item.ComprehensionTag() == tag {
			out = append(out, CloneItem(item))
		}
	}
	return out
}

func WrapBER(tag byte, value []byte) ([]byte, error) {
	length, err := marshalLength(len(value))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(length)+len(value))
	out = append(out, tag)
	out = append(out, length...)
	out = append(out, value...)
	return out, nil
}

func UnwrapBER(data []byte) (byte, []byte, error) {
	if len(data) < 2 {
		return 0, nil, ErrMalformed
	}
	length, size, err := decodeLength(data[1:])
	if err != nil {
		return 0, nil, err
	}
	offset := 1 + size
	if len(data[offset:]) < length {
		return 0, nil, ErrMalformed
	}
	if len(data) != offset+length {
		return 0, nil, ErrMalformed
	}
	return data[0], slices.Clone(data[offset : offset+length]), nil
}

func Consume(data []byte) (Item, int, error) {
	return consume(data)
}

func CloneItem(item Item) Item {
	return Item{Tag: item.Tag, Value: slices.Clone(item.Value)}
}

func CloneItems(items Items) Items {
	if items == nil {
		return nil
	}
	out := make(Items, len(items))
	for i, item := range items {
		out[i] = CloneItem(item)
	}
	return out
}

func marshalLength(length int) ([]byte, error) {
	switch {
	case length < 0x80:
		return []byte{byte(length)}, nil
	case length <= 0xFF:
		return []byte{0x81, byte(length)}, nil
	case length <= 0xFFFF:
		return []byte{0x82, byte(length >> 8), byte(length)}, nil
	default:
		return nil, errValueTooLarge
	}
}

func consume(data []byte) (Item, int, error) {
	if len(data) < 2 {
		return Item{}, 0, ErrMalformed
	}

	length, size, err := decodeLength(data[1:])
	if err != nil {
		return Item{}, 0, err
	}

	offset := 1 + size
	if len(data[offset:]) < length {
		return Item{}, 0, ErrMalformed
	}

	return Item{
		Tag:   data[0],
		Value: slices.Clone(data[offset : offset+length]),
	}, offset + length, nil
}

func decodeLength(data []byte) (int, int, error) {
	if len(data) == 0 {
		return 0, 0, ErrMalformed
	}

	length := int(data[0])
	if length&0x80 == 0 {
		return length, 1, nil
	}

	count := length & 0x7F
	if count == 0 || count > 2 || len(data) < 1+count {
		return 0, 0, ErrMalformed
	}

	length = 0
	for _, b := range data[1 : 1+count] {
		length = (length << 8) | int(b)
	}
	return length, 1 + count, nil
}
