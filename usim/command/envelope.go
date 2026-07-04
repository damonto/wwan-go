package command

import (
	"errors"
	"fmt"

	"github.com/damonto/uicc-go/usim/simfile"
)

const (
	EnvelopeTagSMSPPDownload = 0xD1
)

type SMSPPDownload struct {
	ServiceCenterAddress string
	TPDU                 []byte
}

func (c SMSPPDownload) Envelope() ([]byte, error) {
	if len(c.TPDU) == 0 {
		return nil, errors.New("building SMS-PP download envelope: TPDU is empty")
	}

	body := make([]byte, 0, len(c.TPDU)+32)
	body = appendTLV(body, 0x82, []byte{0x83, 0x81})
	address, err := simfile.Address(c.ServiceCenterAddress).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("building SMS-PP download envelope: %w", err)
	}
	if len(address) != 0 {
		body = appendTLV(body, 0x86, address)
	}
	body = appendTLV(body, 0x8B, c.TPDU)

	out := []byte{EnvelopeTagSMSPPDownload}
	out = appendBERLength(out, len(body))
	out = append(out, body...)
	return out, nil
}

func (c SMSPPDownload) MarshalBinary() ([]byte, error) {
	envelope, err := c.Envelope()
	if err != nil {
		return nil, err
	}
	return Envelope{Data: envelope}.MarshalBinary()
}

func appendTLV(out []byte, tag byte, value []byte) []byte {
	out = append(out, tag)
	out = appendBERLength(out, len(value))
	return append(out, value...)
}

func appendBERLength(out []byte, n int) []byte {
	switch {
	case n <= 0x7F:
		return append(out, byte(n))
	case n <= 0xFF:
		return append(out, 0x81, byte(n))
	default:
		return append(out, 0x82, byte(n>>8), byte(n))
	}
}
