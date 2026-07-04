package simfile

import (
	"errors"
	"fmt"
	"strings"
)

type Address string

func (address Address) String() string {
	return string(address)
}

func (address Address) MarshalText() ([]byte, error) {
	return []byte(string(address)), nil
}

func (address *Address) UnmarshalText(text []byte) error {
	*address = Address(string(text))
	return nil
}

func (address Address) MarshalBinary() ([]byte, error) {
	value := strings.TrimSpace(string(address))
	if value == "" {
		return nil, nil
	}

	tonNPI := byte(0x81)
	if strings.HasPrefix(value, "+") {
		tonNPI = 0x91
		value = strings.TrimPrefix(value, "+")
	}
	value = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(value)
	if value == "" {
		return nil, errors.New("marshaling address: value is empty")
	}

	body, err := encodeSwappedBCD(value)
	if err != nil {
		return nil, fmt.Errorf("marshaling address: %w", err)
	}
	return append([]byte{tonNPI}, body...), nil
}

func (address *Address) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		*address = ""
		return nil
	}

	digits, err := decodeSwappedBCD(data[1:], false)
	if err != nil {
		return fmt.Errorf("parsing address: %w", err)
	}
	if digits == "" {
		*address = ""
		return nil
	}

	if data[0] == 0x91 {
		digits = "+" + digits
	}
	*address = Address(digits)
	return nil
}
