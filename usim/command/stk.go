package command

import (
	"errors"
	"fmt"

	"github.com/damonto/uicc-go/apdu"
)

type TerminalProfile struct {
	Data []byte
}

func (c TerminalProfile) MarshalBinary() ([]byte, error) {
	return (stkAPDU{ins: 0x10, data: c.Data}).APDU()
}

type Status struct{}

func (c Status) MarshalBinary() ([]byte, error) {
	le := uint16(0)
	return (stkAPDU{ins: 0xF2, le: &le}).APDU()
}

type Fetch struct {
	Length byte
}

func (c Fetch) MarshalBinary() ([]byte, error) {
	le := uint16(c.Length)
	return (stkAPDU{ins: 0x12, le: &le}).APDU()
}

type TerminalResponse struct {
	Data []byte
}

func (c TerminalResponse) MarshalBinary() ([]byte, error) {
	return (stkAPDU{ins: 0x14, data: c.Data}).APDU()
}

type Envelope struct {
	Data []byte
}

func (c Envelope) MarshalBinary() ([]byte, error) {
	if len(c.Data) == 0 {
		return nil, errors.New("building STK envelope APDU: envelope is empty")
	}
	le := uint16(0)
	return (stkAPDU{ins: 0xC2, data: c.Data, le: &le}).APDU()
}

type stkAPDU struct {
	ins  byte
	data []byte
	le   *uint16
}

func (c stkAPDU) MarshalBinary() ([]byte, error) {
	return c.APDU()
}

func (c stkAPDU) APDU() ([]byte, error) {
	if len(c.data) <= 0xff && (c.le == nil || *c.le <= 0xff) {
		var shortLe *byte
		if c.le != nil {
			value := byte(*c.le)
			shortLe = &value
		}
		return (apdu.Request{CLA: 0x80, INS: c.ins, P1: 0x00, P2: 0x00, Data: c.data, Le: shortLe}).MarshalBinary()
	}
	if len(c.data) > 0xffff {
		return nil, fmt.Errorf("APDU data length %d exceeds extended APDU limit", len(c.data))
	}

	out := []byte{0x80, c.ins, 0x00, 0x00, 0x00}
	if len(c.data) > 0 {
		out = append(out, byte(len(c.data)>>8), byte(len(c.data)))
		out = append(out, c.data...)
	}
	if c.le != nil {
		out = append(out, byte(*c.le>>8), byte(*c.le))
	}
	return out, nil
}
