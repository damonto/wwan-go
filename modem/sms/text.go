package sms

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// GSM7 is text represented with unpacked GSM 7-bit default-alphabet septets.
type GSM7 string

func (text GSM7) String() string {
	return string(text)
}

// MarshalBinary encodes text as unpacked GSM 7-bit default-alphabet septets.
func (text GSM7) MarshalBinary() ([]byte, error) {
	result := make([]byte, 0, len(text))
	for _, r := range text {
		encoded, ok := encodeGSM7Rune(r)
		if !ok {
			return nil, fmt.Errorf("encoding GSM7: character %q is not representable", r)
		}
		result = append(result, encoded...)
	}
	return result, nil
}

// UnmarshalBinary decodes unpacked GSM 7-bit default-alphabet septets.
func (text *GSM7) UnmarshalBinary(septets []byte) error {
	var result strings.Builder
	for i := 0; i < len(septets); i++ {
		value := septets[i]
		if value > 0x7f {
			return fmt.Errorf("decoding GSM7: value %#x is not a septet", value)
		}
		if value == 0x1b {
			i++
			if i >= len(septets) {
				return errors.New("decoding GSM7: trailing escape septet")
			}
			r, ok := gsm7ExtensionDecode[septets[i]]
			if !ok {
				return fmt.Errorf("decoding GSM7: unknown extension %#x", septets[i])
			}
			result.WriteRune(r)
			continue
		}
		result.WriteRune(gsm7DefaultDecode[value&0x7f])
	}
	*text = GSM7(result.String())
	return nil
}

// MarshalText returns the UTF-8 textual form after validating GSM7 support.
func (text GSM7) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// UnmarshalText decodes UTF-8 text and validates that GSM7 can represent it.
func (text *GSM7) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding GSM7 text: value is not valid UTF-8")
	}
	decoded := GSM7(string(data))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}

// UCS2 is text represented with big-endian UCS-2 code units.
type UCS2 string

func (text UCS2) String() string {
	return string(text)
}

// MarshalBinary encodes text as big-endian UCS-2 code units.
func (text UCS2) MarshalBinary() ([]byte, error) {
	return marshalUCS2(string(text), binary.BigEndian)
}

// UnmarshalBinary decodes big-endian UCS-2 code units.
func (text *UCS2) UnmarshalBinary(data []byte) error {
	value, err := unmarshalUCS2(data, binary.BigEndian)
	if err != nil {
		return err
	}
	*text = UCS2(value)
	return nil
}

// MarshalText returns the UTF-8 textual form after validating UCS-2 support.
func (text UCS2) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// UnmarshalText decodes UTF-8 text and validates that UCS-2 can represent it.
func (text *UCS2) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding UCS2 text: value is not valid UTF-8")
	}
	decoded := UCS2(string(data))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}

// UCS2LE is text represented with little-endian UCS-2 code units.
type UCS2LE string

func (text UCS2LE) String() string {
	return string(text)
}

// MarshalBinary encodes text as little-endian UCS-2 code units.
func (text UCS2LE) MarshalBinary() ([]byte, error) {
	return marshalUCS2(string(text), binary.LittleEndian)
}

// UnmarshalBinary decodes little-endian UCS-2 code units.
func (text *UCS2LE) UnmarshalBinary(data []byte) error {
	value, err := unmarshalUCS2(data, binary.LittleEndian)
	if err != nil {
		return err
	}
	*text = UCS2LE(value)
	return nil
}

// MarshalText returns the UTF-8 textual form after validating UCS-2 support.
func (text UCS2LE) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// UnmarshalText decodes UTF-8 text and validates that UCS-2 can represent it.
func (text *UCS2LE) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding UCS2LE text: value is not valid UTF-8")
	}
	decoded := UCS2LE(string(data))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}

// UTF16 is text represented with big-endian UTF-16 code units.
type UTF16 string

func (text UTF16) String() string {
	return string(text)
}

// MarshalBinary encodes text as big-endian UTF-16 code units.
func (text UTF16) MarshalBinary() ([]byte, error) {
	return marshalUTF16(string(text), binary.BigEndian)
}

// UnmarshalBinary decodes big-endian UTF-16 code units.
func (text *UTF16) UnmarshalBinary(data []byte) error {
	value, err := unmarshalUTF16(data, binary.BigEndian)
	if err != nil {
		return err
	}
	*text = UTF16(value)
	return nil
}

// MarshalText returns the UTF-8 textual form after validating UTF-16 support.
func (text UTF16) MarshalText() ([]byte, error) {
	if !utf8.ValidString(string(text)) {
		return nil, errors.New("encoding UTF16 text: value is not valid UTF-8")
	}
	return []byte(text), nil
}

// UnmarshalText decodes UTF-8 text and validates that UTF-16 can represent it.
func (text *UTF16) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding UTF16 text: value is not valid UTF-8")
	}
	*text = UTF16(string(data))
	return nil
}

// UTF16LE is text represented with little-endian UTF-16 code units.
type UTF16LE string

func (text UTF16LE) String() string {
	return string(text)
}

// MarshalBinary encodes text as little-endian UTF-16 code units.
func (text UTF16LE) MarshalBinary() ([]byte, error) {
	return marshalUTF16(string(text), binary.LittleEndian)
}

// UnmarshalBinary decodes little-endian UTF-16 code units.
func (text *UTF16LE) UnmarshalBinary(data []byte) error {
	value, err := unmarshalUTF16(data, binary.LittleEndian)
	if err != nil {
		return err
	}
	*text = UTF16LE(value)
	return nil
}

// MarshalText returns the UTF-8 textual form after validating UTF-16 support.
func (text UTF16LE) MarshalText() ([]byte, error) {
	if !utf8.ValidString(string(text)) {
		return nil, errors.New("encoding UTF16LE text: value is not valid UTF-8")
	}
	return []byte(text), nil
}

// UnmarshalText decodes UTF-8 text and validates that UTF-16 can represent it.
func (text *UTF16LE) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding UTF16LE text: value is not valid UTF-8")
	}
	*text = UTF16LE(string(data))
	return nil
}

func marshalUCS2(value string, order binary.ByteOrder) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, errors.New("encoding UCS2: value is not valid UTF-8")
	}
	result := make([]byte, 0, len(value)*2)
	for _, r := range value {
		if r > 0xffff || r >= 0xd800 && r <= 0xdfff {
			return nil, fmt.Errorf("encoding UCS2: character %q is outside UCS2", r)
		}
		result = append(result, 0, 0)
		order.PutUint16(result[len(result)-2:], uint16(r))
	}
	return result, nil
}

func unmarshalUCS2(data []byte, order binary.ByteOrder) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("decoding UCS2: payload has odd byte length")
	}
	runes := make([]rune, len(data)/2)
	for i := range runes {
		unit := order.Uint16(data[i*2:])
		if unit >= 0xd800 && unit <= 0xdfff {
			return "", errors.New("decoding UCS2: payload contains a surrogate code point")
		}
		runes[i] = rune(unit)
	}
	return string(runes), nil
}

func marshalUTF16(value string, order binary.ByteOrder) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, errors.New("encoding UTF16: value is not valid UTF-8")
	}
	units := utf16.Encode([]rune(value))
	result := make([]byte, len(units)*2)
	for i, unit := range units {
		order.PutUint16(result[i*2:], unit)
	}
	return result, nil
}

func unmarshalUTF16(data []byte, order binary.ByteOrder) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("decoding UTF16: payload has odd byte length")
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[i*2:])
	}
	for i := 0; i < len(units); i++ {
		unit := units[i]
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", errors.New("decoding UTF16: payload contains an unpaired high surrogate")
			}
			i++
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", errors.New("decoding UTF16: payload contains an unpaired low surrogate")
		}
	}
	return string(utf16.Decode(units)), nil
}
