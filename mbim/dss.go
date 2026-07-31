package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type DSSLinkState uint32

const (
	DSSLinkStateDeactivate DSSLinkState = 0
	DSSLinkStateActivate   DSSLinkState = 1
)

type DSSConnectRequest struct {
	TransactionID   uint32
	DeviceServiceID [16]byte
	SessionID       SessionID
	LinkState       DSSLinkState
	Response        *emptyResponse
}

func (r *DSSConnectRequest) Request() *Request {
	data := make([]byte, 0, 24)
	data = append(data, r.DeviceServiceID[:]...)
	data = binary.LittleEndian.AppendUint32(data, uint32(r.SessionID))
	data = binary.LittleEndian.AppendUint32(data, uint32(r.LinkState))
	r.Response = new(emptyResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceDSS, CIDDSSConnect, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (c *Client) SetDSSLinkState(ctx context.Context, serviceID [16]byte, sessionID SessionID, state DSSLinkState) error {
	if sessionID > 0xff {
		return errors.New("setting MBIM DSS link state: session ID uses reserved high bits")
	}
	if state > DSSLinkStateActivate {
		return fmt.Errorf("setting MBIM DSS link state: state %d is outside 0..%d", state, DSSLinkStateActivate)
	}
	request := DSSConnectRequest{
		TransactionID:   c.nextTransactionID(),
		DeviceServiceID: serviceID,
		SessionID:       sessionID,
		LinkState:       state,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("setting MBIM DSS link state: %w", err)
	}
	return nil
}
