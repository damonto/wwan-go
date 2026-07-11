package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

// DeviceCapsRequest queries the mandatory MBIM Basic Connect device capabilities.
type DeviceCapsRequest struct {
	TransactionID uint32
	Response      *DeviceCapsInfo
}

func (r *DeviceCapsRequest) Request() *Request {
	r.Response = new(DeviceCapsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDDeviceCaps,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

// DeviceCapsInfo contains the IP-session capacity needed before MBIM_CID_CONNECT.
type DeviceCapsInfo struct {
	MaxSessions uint32
}

func (r *DeviceCapsInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 32 {
		return errors.New("parsing MBIM device capabilities: payload is truncated")
	}
	*r = DeviceCapsInfo{MaxSessions: binary.LittleEndian.Uint32(data[28:32])}
	return nil
}

// DeviceCaps reads the modem's IP-session capacity.
func (r *Reader) DeviceCaps(ctx context.Context) (DeviceCapsInfo, error) {
	request := DeviceCapsRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return DeviceCapsInfo{}, fmt.Errorf("reading MBIM device capabilities: %w", err)
	}
	return *request.Response, nil
}
