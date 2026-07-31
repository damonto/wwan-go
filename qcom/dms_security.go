package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const dmsUserLockCodeLength = 4

// DMSGetUserLockStateRequest encodes the legacy Get User Lock State message.
type DMSGetUserLockStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSGetUserLockStateRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetUserLockState)
}

// DMSGetUserLockStateResponse contains the legacy device lock state.
type DMSGetUserLockStateResponse struct {
	Enabled bool
}

// UnmarshalTLVs parses the mandatory user-lock flag.
func (r *DMSGetUserLockStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetUserLockStateResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI DMS user lock state: state TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS user lock state: state TLV length %d, want 1", len(value))
	}
	r.Enabled = value[0] != 0
	return nil
}

// DMSSetUserLockStateRequest encodes the legacy Set User Lock State message.
type DMSSetUserLockStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
	Code          string
}

// Request validates and converts the user-lock update into a QMI request.
func (r DMSSetUserLockStateRequest) Request() (Request, error) {
	if err := validateDMSNumericCode(r.Code, dmsUserLockCodeLength); err != nil {
		return Request{}, fmt.Errorf("validating user lock code: %w", err)
	}
	value := append([]byte{boolByte(r.Enabled)}, r.Code...)
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetUserLockState,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// DMSSetUserLockCodeRequest encodes the legacy Set User Lock Code message.
type DMSSetUserLockCodeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	CurrentCode   string
	NewCode       string
}

// Request validates and converts the lock-code replacement into a QMI request.
func (r DMSSetUserLockCodeRequest) Request() (Request, error) {
	if err := validateDMSNumericCode(r.CurrentCode, dmsUserLockCodeLength); err != nil {
		return Request{}, fmt.Errorf("validating current user lock code: %w", err)
	}
	if err := validateDMSNumericCode(r.NewCode, dmsUserLockCodeLength); err != nil {
		return Request{}, fmt.Errorf("validating new user lock code: %w", err)
	}
	value := append([]byte(r.CurrentCode), r.NewCode...)
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetUserLockCode,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// DMSValidateSPCRequest encodes Validate Service Programming Code.
type DMSValidateSPCRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	SPC           string
}

// Request validates and converts the SPC into a QMI request.
func (r DMSValidateSPCRequest) Request() (Request, error) {
	if err := validateDMSNumericCode(r.SPC, dmsSPCLength); err != nil {
		return Request{}, fmt.Errorf("validating SPC: %w", err)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSValidateSPC,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, []byte(r.SPC))},
	}, nil
}

// DMSSetSPCRequest encodes Set Service Programming Code.
type DMSSetSPCRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	CurrentSPC    string
	NewSPC        string
}

// Request validates and converts the SPC replacement into a QMI request.
func (r DMSSetSPCRequest) Request() (Request, error) {
	if err := validateDMSNumericCode(r.CurrentSPC, dmsSPCLength); err != nil {
		return Request{}, fmt.Errorf("validating current SPC: %w", err)
	}
	if err := validateDMSNumericCode(r.NewSPC, dmsSPCLength); err != nil {
		return Request{}, fmt.Errorf("validating new SPC: %w", err)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetSPC,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte(r.CurrentSPC)),
			tlv.Bytes(0x02, []byte(r.NewSPC)),
		},
	}, nil
}

// DMSUserLockState returns the legacy device user-lock state.
func (c *Client) DMSUserLockState(ctx context.Context) (bool, error) {
	var result DMSGetUserLockStateResponse
	if err := c.dmsRead(ctx, MessageDMSGetUserLockState, &result); err != nil {
		return false, fmt.Errorf("querying QMI DMS user lock state: %w", err)
	}
	return result.Enabled, nil
}

// DMSSetUserLockState enables or disables the legacy device user lock.
func (c *Client) DMSSetUserLockState(ctx context.Context, enabled bool, code string) error {
	req, err := (DMSSetUserLockStateRequest{
		Timeout: DefaultRequestTimeout,
		Enabled: enabled,
		Code:    code,
	}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DMS user lock state: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI DMS user lock state: %w", err)
	}
	return nil
}

// DMSSetUserLockCode replaces the legacy device user-lock code.
func (c *Client) DMSSetUserLockCode(ctx context.Context, currentCode, newCode string) error {
	req, err := (DMSSetUserLockCodeRequest{
		Timeout:     DefaultRequestTimeout,
		CurrentCode: currentCode,
		NewCode:     newCode,
	}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DMS user lock code: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI DMS user lock code: %w", err)
	}
	return nil
}

// DMSValidateSPC authenticates a six-digit service programming code.
func (c *Client) DMSValidateSPC(ctx context.Context, spc string) error {
	req, err := (DMSValidateSPCRequest{Timeout: DefaultRequestTimeout, SPC: spc}).Request()
	if err != nil {
		return fmt.Errorf("validating QMI DMS SPC: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("validating QMI DMS SPC: %w", err)
	}
	return nil
}

// DMSSetSPC replaces the six-digit service programming code.
func (c *Client) DMSSetSPC(ctx context.Context, currentSPC, newSPC string) error {
	req, err := (DMSSetSPCRequest{
		Timeout:    DefaultRequestTimeout,
		CurrentSPC: currentSPC,
		NewSPC:     newSPC,
	}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DMS SPC: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI DMS SPC: %w", err)
	}
	return nil
}

func validateDMSNumericCode(value string, length int) error {
	if err := validateDMSFixedASCII(value, length); err != nil {
		return err
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return errors.New("contains a non-digit character")
		}
	}
	return nil
}
