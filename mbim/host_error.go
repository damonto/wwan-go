package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

// HostErrorRequest is an MBIM_HOST_ERROR_MSG sent for a protocol-layer error.
// The transaction ID must identify the message that caused the error when it
// is known.
type HostErrorRequest struct {
	TransactionID uint32
	Status        ProtocolError
}

func (r *HostErrorRequest) Request() *Request {
	return &Request{
		MessageType:   MessageTypeHostError,
		TransactionID: r.TransactionID,
		Command:       r,
	}
}

func (r *HostErrorRequest) MarshalBinary() ([]byte, error) {
	if !validHostProtocolError(r.Status) {
		return nil, fmt.Errorf("encoding MBIM host error: protocol status %d is outside 1..%d", r.Status, ProtocolErrorMaxTransfer)
	}
	return binary.LittleEndian.AppendUint32(nil, uint32(r.Status)), nil
}

func validHostProtocolError(status ProtocolError) bool {
	return status >= ProtocolErrorTimeoutFragment && status <= ProtocolErrorMaxTransfer
}

// SendHostError sends an MBIM_HOST_ERROR_MSG without waiting for a response.
func (c *Client) SendHostError(ctx context.Context, transactionID uint32, status ProtocolError) error {
	request := HostErrorRequest{TransactionID: transactionID, Status: status}
	if err := c.sendOneWay(ctx, request.Request()); err != nil {
		return fmt.Errorf("sending MBIM host error for transaction %d: %w", transactionID, err)
	}
	return nil
}

// CancelTransaction asks the device to cancel an outstanding MBIM transaction.
func (c *Client) CancelTransaction(ctx context.Context, transactionID uint32) error {
	return c.SendHostError(ctx, transactionID, ProtocolErrorCancel)
}
