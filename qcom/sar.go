package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// SARRFState selects one modem RF exposure profile.
type SARRFState uint32

const (
	SARRFState0 SARRFState = iota
	SARRFState1
	SARRFState2
	SARRFState3
	SARRFState4
	SARRFState5
	SARRFState6
	SARRFState7
	SARRFState8
	SARRFState9
	SARRFState10
	SARRFState11
	SARRFState12
	SARRFState13
	SARRFState14
	SARRFState15
	SARRFState16
	SARRFState17
	SARRFState18
	SARRFState19
	SARRFState20
)

// SARSetRFStateRequest encodes SAR RF Set State.
type SARSetRFStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	State         SARRFState
}

// Request validates and converts the RF state into a QMI request.
func (r SARSetRFStateRequest) Request() (Request, error) {
	if r.State > SARRFState20 {
		return Request{}, fmt.Errorf("SAR RF state %d is out of range", r.State)
	}
	return Request{
		Service:       ServiceSAR,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageSARRFSetState,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.State))},
	}, nil
}

// SARGetRFStateRequest encodes SAR RF Get State.
type SARGetRFStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r SARGetRFStateRequest) Request() Request {
	return Request{
		Service:       ServiceSAR,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageSARRFGetState,
		Timeout:       r.Timeout,
	}
}

// SARGetRFStateResponse contains the active RF exposure profile.
type SARGetRFStateResponse struct {
	State SARRFState
}

// UnmarshalTLVs parses SAR RF Get State response TLVs.
func (r *SARGetRFStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = SARGetRFStateResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI SAR RF state: state TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI SAR RF state: state TLV length %d, want 4", len(value))
	}
	r.State = SARRFState(binary.LittleEndian.Uint32(value))
	return nil
}

// SARRFState returns the modem's active RF exposure profile.
func (c *Client) SARRFState(ctx context.Context) (SARRFState, error) {
	var result SARGetRFStateResponse
	err := c.sarRequest(ctx, (SARGetRFStateRequest{Timeout: DefaultRequestTimeout}).Request(), &result)
	if err != nil {
		return 0, fmt.Errorf("querying QMI SAR RF state: %w", err)
	}
	return result.State, nil
}

// SetSARRFState selects a modem RF exposure profile.
func (c *Client) SetSARRFState(ctx context.Context, state SARRFState) error {
	req, err := (SARSetRFStateRequest{Timeout: DefaultRequestTimeout, State: state}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI SAR RF state: %w", err)
	}
	if err := c.sarRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI SAR RF state: %w", err)
	}
	return nil
}

func (c *Client) sarRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceSAR, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceSAR, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if dst == nil {
			return nil
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}
