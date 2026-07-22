package simfile

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// BCD stores hexadecimal digits with each byte's nibbles swapped.
// Trailing F nibbles are padding; F digits elsewhere are preserved.
type BCD []byte

// NewBCD encodes a hexadecimal string as swapped-nibble BCD.
func NewBCD(value string) (BCD, error) {
	if len(value)%2 != 0 {
		value += "F"
	}

	data, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("encoding BCD: %w", err)
	}
	for i, b := range data {
		data[i] = b>>4 | b<<4
	}
	return BCD(data), nil
}

func (bcd BCD) String() string {
	data := make([]byte, len(bcd))
	for i, b := range bcd {
		data[i] = b>>4 | b<<4
	}
	return strings.TrimRight(strings.ToUpper(hex.EncodeToString(data)), "F")
}
