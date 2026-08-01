package simfile

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// BCD stores hexadecimal digits with each byte's nibbles swapped.
// Trailing F nibbles are padding; F digits elsewhere are preserved.
type BCD []byte

func (bcd BCD) MarshalBinary() ([]byte, error) {
	return slices.Clone(bcd), nil
}

func (bcd *BCD) UnmarshalBinary(data []byte) error {
	*bcd = slices.Clone(data)
	return nil
}

func (bcd BCD) MarshalText() ([]byte, error) {
	return []byte(bcd.String()), nil
}

func (bcd *BCD) UnmarshalText(text []byte) error {
	value := string(text)
	if len(value)%2 != 0 {
		value += "F"
	}

	data, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("decoding BCD text: %w", err)
	}
	for i, b := range data {
		data[i] = b>>4 | b<<4
	}
	*bcd = BCD(data)
	return nil
}

func (bcd BCD) String() string {
	data := make([]byte, len(bcd))
	for i, b := range bcd {
		data[i] = b>>4 | b<<4
	}
	return strings.TrimRight(strings.ToUpper(hex.EncodeToString(data)), "F")
}
