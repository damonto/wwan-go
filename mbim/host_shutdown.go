package mbim

import (
	"context"
	"fmt"
)

type HostShutdownRequest struct {
	TransactionID uint32
	Response      *HostShutdownResponse
}

func (r *HostShutdownRequest) Request() *Request {
	r.Response = new(HostShutdownResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsHostShutdown, CIDMsHostShutdownNotify, CommandTypeSet, nil),
		Response:      r.Response,
	}
}

type HostShutdownResponse struct{}

func (*HostShutdownResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM host shutdown response: payload length %d, want 0", len(data))
	}
	return nil
}

// NotifyHostShutdown tells the modem that the host is shutting down.
func (c *Client) NotifyHostShutdown(ctx context.Context) error {
	request := HostShutdownRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("notifying MBIM host shutdown: %w", err)
	}
	return nil
}
