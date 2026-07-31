package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

type RadioSwitchState uint32

const (
	RadioSwitchStateOff RadioSwitchState = iota
	RadioSwitchStateOn
)

type RadioStateInfo struct {
	HwRadioState RadioSwitchState
	SwRadioState RadioSwitchState
}

type RadioStateRequest struct {
	TransactionID uint32
	Response      *RadioStateInfo
}

func (r *RadioStateRequest) Request() *Request {
	r.Response = new(RadioStateInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDRadioState,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type RadioStateSetRequest struct {
	TransactionID uint32
	State         RadioSwitchState
	Response      *RadioStateInfo
}

func (r *RadioStateSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.State))
	r.Response = new(RadioStateInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDRadioState,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

func (r *RadioStateInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("parsing MBIM radio state: payload length is %d, want 8", len(data))
	}
	hardwareState := RadioSwitchState(binary.LittleEndian.Uint32(data[:4]))
	if hardwareState > RadioSwitchStateOn {
		return fmt.Errorf("parsing MBIM radio state: hardware state %d is reserved", hardwareState)
	}
	softwareState := RadioSwitchState(binary.LittleEndian.Uint32(data[4:8]))
	if softwareState > RadioSwitchStateOn {
		return fmt.Errorf("parsing MBIM radio state: software state %d is reserved", softwareState)
	}
	r.HwRadioState = hardwareState
	r.SwRadioState = softwareState
	return nil
}

func (c *Client) RadioState(ctx context.Context) (RadioStateInfo, error) {
	request := RadioStateRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return RadioStateInfo{}, fmt.Errorf("reading MBIM radio state: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetRadioState(ctx context.Context, state RadioSwitchState) (RadioStateInfo, error) {
	if state > RadioSwitchStateOn {
		return RadioStateInfo{}, fmt.Errorf("setting MBIM radio state: state %d is reserved", state)
	}
	request := RadioStateSetRequest{
		TransactionID: c.nextTransactionID(),
		State:         state,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return RadioStateInfo{}, fmt.Errorf("setting MBIM radio state: %w", err)
	}
	return *request.Response, nil
}
