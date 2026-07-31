package mbim

import (
	"context"
	"fmt"
)

type FirmwareID [16]byte

type FirmwareIDRequest struct {
	TransactionID uint32
	Response      *FirmwareID
}

func (r *FirmwareIDRequest) Request() *Request {
	r.Response = new(FirmwareID)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsFirmwareID, CIDMsFirmwareIDGet, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (id *FirmwareID) UnmarshalBinary(data []byte) error {
	if len(data) != len(id) {
		return fmt.Errorf("parsing MBIM firmware ID: payload length %d, want %d", len(data), len(id))
	}
	copy(id[:], data)
	return nil
}

func (c *Client) FirmwareID(ctx context.Context) (FirmwareID, error) {
	request := FirmwareIDRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return FirmwareID{}, fmt.Errorf("reading MBIM firmware ID: %w", err)
	}
	return *request.Response, nil
}
