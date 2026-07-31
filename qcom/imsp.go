package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// IMSPEnablerState describes IMS presence initialization and registration.
type IMSPEnablerState uint32

const (
	IMSPEnablerUninitialized IMSPEnablerState = 1 + iota
	IMSPEnablerInitialized
	IMSPEnablerAirplane
	IMSPEnablerRegistered
)

// IMSPGetEnablerStateRequest encodes IMSP Get Enabler State.
type IMSPGetEnablerStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r IMSPGetEnablerStateRequest) Request() Request {
	return Request{
		Service:       ServiceIMSP,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSPGetEnablerState,
		Timeout:       r.Timeout,
	}
}

// IMSPGetEnablerStateResponse contains the current IMS presence state.
type IMSPGetEnablerStateResponse struct {
	State IMSPEnablerState
}

// UnmarshalTLVs parses IMSP Get Enabler State response TLVs.
func (r *IMSPGetEnablerStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSPGetEnablerStateResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI IMSP enabler state: state TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI IMSP enabler state: state TLV length %d, want 4", len(value))
	}
	r.State = IMSPEnablerState(binary.LittleEndian.Uint32(value))
	return nil
}

// IMSPEnablerState returns IMS presence initialization and registration state.
func (c *Client) IMSPEnablerState(ctx context.Context) (IMSPEnablerState, error) {
	var result IMSPGetEnablerStateResponse
	err := c.withServiceClient(ctx, ServiceIMSP, func(clientID uint8) error {
		req := IMSPGetEnablerStateRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return 0, fmt.Errorf("querying QMI IMSP enabler state: %w", err)
	}
	return result.State, nil
}
