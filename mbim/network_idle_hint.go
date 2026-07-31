package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

type NetworkIdleHint uint32

const (
	NetworkIdleHintDisabled NetworkIdleHint = 0
	NetworkIdleHintEnabled  NetworkIdleHint = 1
)

type NetworkIdleHintRequest struct {
	TransactionID uint32
	Response      *NetworkIdleHint
}

func (r *NetworkIdleHintRequest) Request() *Request {
	r.Response = new(NetworkIdleHint)
	return basicUint32Request(r.TransactionID, CIDNetworkIdleHint, CommandTypeQuery, nil, r.Response)
}

type NetworkIdleHintSetRequest struct {
	TransactionID uint32
	Hint          NetworkIdleHint
	Response      *NetworkIdleHint
}

func (r *NetworkIdleHintSetRequest) Request() *Request {
	data, err := r.Hint.MarshalBinary()
	r.Response = new(NetworkIdleHint)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       commandWithError(ServiceBasicConnect, CIDNetworkIdleHint, CommandTypeSet, data, err),
		Response:      r.Response,
	}
}

func (h NetworkIdleHint) MarshalBinary() ([]byte, error) {
	if h > NetworkIdleHintEnabled {
		return nil, fmt.Errorf("encoding MBIM network idle hint: state %d is outside 0..%d", h, NetworkIdleHintEnabled)
	}
	return binary.LittleEndian.AppendUint32(nil, uint32(h)), nil
}

func (r *NetworkIdleHint) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("parsing MBIM network idle hint: payload length is %d, want 4", len(data))
	}
	hint := NetworkIdleHint(binary.LittleEndian.Uint32(data[0:4]))
	if hint > NetworkIdleHintEnabled {
		return fmt.Errorf("parsing MBIM network idle hint: state %d is outside 0..%d", hint, NetworkIdleHintEnabled)
	}
	*r = hint
	return nil
}

func (c *Client) NetworkIdleHint(ctx context.Context) (NetworkIdleHint, error) {
	request := NetworkIdleHintRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return 0, fmt.Errorf("reading MBIM network idle hint: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetNetworkIdleHint(ctx context.Context, hint NetworkIdleHint) (NetworkIdleHint, error) {
	if hint > NetworkIdleHintEnabled {
		return 0, fmt.Errorf("setting MBIM network idle hint: state %d is outside 0..%d", hint, NetworkIdleHintEnabled)
	}
	request := NetworkIdleHintSetRequest{TransactionID: c.nextTransactionID(), Hint: hint}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return 0, fmt.Errorf("setting MBIM network idle hint: %w", err)
	}
	return *request.Response, nil
}
