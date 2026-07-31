package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

type EmergencyMode uint32

const (
	EmergencyModeOff EmergencyMode = 0
	EmergencyModeOn  EmergencyMode = 1
)

type EmergencyModeRequest struct {
	TransactionID uint32
	Response      *EmergencyMode
}

func (r *EmergencyModeRequest) Request() *Request {
	r.Response = new(EmergencyMode)
	return basicUint32Request(r.TransactionID, CIDEmergencyMode, CommandTypeQuery, nil, r.Response)
}

func (r *EmergencyMode) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("parsing MBIM emergency mode: payload length is %d, want 4", len(data))
	}
	mode := EmergencyMode(binary.LittleEndian.Uint32(data[0:4]))
	if mode > EmergencyModeOn {
		return fmt.Errorf("parsing MBIM emergency mode: state %d is outside 0..%d", mode, EmergencyModeOn)
	}
	*r = mode
	return nil
}

func (c *Client) EmergencyMode(ctx context.Context) (EmergencyMode, error) {
	request := EmergencyModeRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return 0, fmt.Errorf("reading MBIM emergency mode: %w", err)
	}
	return *request.Response, nil
}
