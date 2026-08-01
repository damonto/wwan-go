package stk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

var (
	errTruncatedAlphaIdentifier = errors.New("alpha identifier is truncated")
	errInvalidUCS2Length        = errors.New("UCS2 payload has an odd length")
)

// DataCodingScheme identifies the alphabet and packing used by a Text String.
type DataCodingScheme byte

const (
	DCSGSM7Packed   DataCodingScheme = 0x00
	DCSGSM8Unpacked DataCodingScheme = 0x04
	DCSUCS2         DataCodingScheme = 0x08
)

type AlphaIdentifier struct {
	Value string
}

func (a AlphaIdentifier) String() string { return a.Value }

func (a *AlphaIdentifier) UnmarshalBinary(data []byte) error {
	value, err := decodeAlphaIdentifier(data)
	if err != nil {
		return err
	}
	*a = AlphaIdentifier{Value: value}
	return nil
}

func (a AlphaIdentifier) MarshalBinary() ([]byte, error) {
	if a.Value == "" {
		return nil, nil
	}
	raw, err := ucs2Text(a.Value).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding alpha identifier: %w", err)
	}
	return append([]byte{0x80}, raw...), nil
}

type TextString struct {
	DCS   DataCodingScheme
	Value string
}

func (text TextString) String() string { return text.Value }

func (text *TextString) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		*text = TextString{}
		return nil
	}
	dcs := DataCodingScheme(data[0])
	value, err := decodeTextString(dcs, data[1:])
	if err != nil {
		return err
	}
	*text = TextString{DCS: dcs, Value: value}
	return nil
}

func (text TextString) MarshalBinary() ([]byte, error) {
	dcs := text.DCS
	var raw []byte
	if text.Value != "" {
		switch dcs {
		case DCSGSM7Packed:
			var err error
			raw, err = gsm7Text(text.Value).MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("encoding packed GSM text: %w", err)
			}
		case DCSGSM8Unpacked:
			var err error
			raw, err = gsmDefaultText(text.Value).MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("encoding GSM text: %w", err)
			}
		case DCSUCS2:
			var err error
			raw, err = ucs2Text(text.Value).MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("encoding UCS2 text: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported data coding scheme 0x%02X", dcs)
		}
	}
	if len(raw) == 0 && text.Value == "" && dcs == 0 {
		return nil, nil
	}
	return append([]byte{byte(dcs)}, raw...), nil
}

func decodeTextString(dcs DataCodingScheme, data []byte) (string, error) {
	switch dcs {
	case DCSGSM7Packed:
		var text gsm7Text
		if err := text.UnmarshalBinary(data); err != nil {
			return "", fmt.Errorf("decoding packed GSM text: %w", err)
		}
		return string(text), nil
	case DCSGSM8Unpacked:
		var text gsmDefaultText
		if err := text.UnmarshalBinary(data); err != nil {
			return "", err
		}
		return text.String(), nil
	case DCSUCS2:
		var text ucs2Text
		if err := text.UnmarshalBinary(data); err != nil {
			return "", err
		}
		return text.String(), nil
	default:
		return "", fmt.Errorf("unsupported data coding scheme 0x%02X", dcs)
	}
}

func decodeAlphaIdentifier(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	switch data[0] {
	case 0x80:
		return decodeAlphaUCS2(data[1:])
	case 0x81, 0x82:
		return decodeCompressedUCS2(data)
	default:
		var text gsmDefaultText
		if err := text.UnmarshalBinary(trimPadding(data)); err != nil {
			return "", err
		}
		return text.String(), nil
	}
}

func decodeAlphaUCS2(data []byte) (string, error) {
	if len(data)%2 != 0 {
		if data[len(data)-1] != 0xff {
			return "", errInvalidUCS2Length
		}
		data = data[:len(data)-1]
	}
	for len(data) >= 2 && data[len(data)-2] == 0xff && data[len(data)-1] == 0xff {
		data = data[:len(data)-2]
	}
	var text ucs2Text
	if err := text.UnmarshalBinary(data); err != nil {
		return "", err
	}
	return text.String(), nil
}

func decodeCompressedUCS2(data []byte) (string, error) {
	headerLen := 3
	if data[0] == 0x82 {
		headerLen = 4
	}
	if len(data) < headerLen {
		return "", errTruncatedAlphaIdentifier
	}
	length := int(data[1])
	if length > len(data)-headerLen {
		return "", errTruncatedAlphaIdentifier
	}
	base := rune(data[2]) << 7
	if data[0] == 0x82 {
		base = rune(binary.BigEndian.Uint16(data[2:4]))
	}
	payload := data[headerLen : headerLen+length]
	out := make([]rune, 0, length)
	for i := 0; i < len(payload); i++ {
		if payload[i]&0x80 != 0 {
			codePoint := base + rune(payload[i]&0x7f)
			if codePoint > 0xffff || codePoint >= 0xd800 && codePoint <= 0xdfff {
				return "", fmt.Errorf("compressed UCS2 code point U+%04X is outside UCS2", codePoint)
			}
			out = append(out, codePoint)
			continue
		}
		if payload[i] == 0x1b {
			if i+1 >= len(payload) || payload[i+1]&0x80 != 0 {
				return "", errTruncatedAlphaIdentifier
			}
			i++
			r, ok := decodeGSMExtension(payload[i])
			if !ok {
				return "", fmt.Errorf("unknown GSM extension code 0x%02X", payload[i])
			}
			out = append(out, r)
			continue
		}
		r, ok := decodeGSMChar(payload[i])
		if !ok {
			return "", fmt.Errorf("unknown GSM character code 0x%02X", payload[i])
		}
		out = append(out, r)
	}
	return string(out), nil
}

type gsmDefaultText string

func (text gsmDefaultText) String() string {
	return string(text)
}

func (text gsmDefaultText) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		septets, ok := gsm7SeptetsForRune(r)
		if !ok {
			return nil, fmt.Errorf("character %q is not in the GSM default alphabet", r)
		}
		out = append(out, septets...)
	}
	return out, nil
}

func (text *gsmDefaultText) UnmarshalBinary(data []byte) error {
	out := make([]rune, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == 0x1b {
			if i+1 >= len(data) {
				return errors.New("GSM extension escape is truncated")
			}
			i++
			r, ok := decodeGSMExtension(data[i])
			if !ok {
				return fmt.Errorf("unknown GSM extension code 0x%02X", data[i])
			}
			out = append(out, r)
			continue
		}
		r, ok := decodeGSMChar(data[i])
		if !ok {
			return fmt.Errorf("unknown GSM character code 0x%02X", data[i])
		}
		out = append(out, r)
	}
	*text = gsmDefaultText(string(out))
	return nil
}

func (text gsmDefaultText) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func (text *gsmDefaultText) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding GSM text: value is not valid UTF-8")
	}
	decoded := gsmDefaultText(string(data))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}

type ucs2Text string

func (text ucs2Text) String() string {
	return string(text)
}

func (text ucs2Text) MarshalBinary() ([]byte, error) {
	if !utf8.ValidString(string(text)) {
		return nil, errors.New("encoding UCS2 text: value is not valid UTF-8")
	}
	out := make([]byte, 0, len(text)*2)
	for _, r := range text {
		if r > 0xffff || r >= 0xd800 && r <= 0xdfff {
			return nil, fmt.Errorf("character %q is outside UCS2", r)
		}
		out = binary.BigEndian.AppendUint16(out, uint16(r))
	}
	return out, nil
}

func (text *ucs2Text) UnmarshalBinary(data []byte) error {
	if len(data)%2 != 0 {
		return errInvalidUCS2Length
	}
	units := make([]uint16, 0, len(data)/2)
	for len(data) > 0 {
		units = append(units, binary.BigEndian.Uint16(data[:2]))
		data = data[2:]
	}
	runes := make([]rune, len(units))
	for i, unit := range units {
		if unit >= 0xd800 && unit <= 0xdfff {
			return errors.New("UCS2 payload contains a surrogate code point")
		}
		runes[i] = rune(unit)
	}
	*text = ucs2Text(string(runes))
	return nil
}

func (text ucs2Text) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func (text *ucs2Text) UnmarshalText(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("decoding UCS2 text: value is not valid UTF-8")
	}
	decoded := ucs2Text(string(data))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}

func gsm7SeptetsForRune(r rune) ([]byte, bool) {
	septets := gsm7Septets(r)
	if len(septets) == 1 {
		return septets, gsm7Char(septets[0]) == r
	}
	return septets, len(septets) == 2 && septets[0] == 0x1b && gsm7ExtensionChar(septets[1]) == r
}

func decodeGSMChar(value byte) (rune, bool) {
	r := gsm7Char(value)
	return r, r != '?' || value == '?'
}

func decodeGSMExtension(value byte) (rune, bool) {
	r := gsm7ExtensionChar(value)
	return r, r != '?'
}

func trimPadding(data []byte) []byte {
	for len(data) > 0 && data[len(data)-1] == 0xff {
		data = data[:len(data)-1]
	}
	return data
}
