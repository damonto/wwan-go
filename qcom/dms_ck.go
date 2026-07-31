package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// DMSCKProtectionRequest updates one legacy UIM personalization facility.
type DMSCKProtectionRequest struct {
	Facility DMSUIMFacility
	State    DMSFacilityState
	Key      string
}

// DMSCKUnblockRequest unblocks one legacy UIM personalization facility.
type DMSCKUnblockRequest struct {
	Facility DMSUIMFacility
	Key      string
}

// DMSCKOperationResult contains a retry counter returned by a CK operation.
// The counter may be present even when the modem rejects the operation.
type DMSCKOperationResult struct {
	Retries      uint8
	RetriesKnown bool
}

// DMSUIMSetCKProtectionRequest encodes UIM Set CK Protection.
type DMSUIMSetCKProtectionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSCKProtectionRequest
}

// Request validates and converts the facility update into a QMI request.
func (r DMSUIMSetCKProtectionRequest) Request() (Request, error) {
	if err := validateDMSUIMFacility(r.Info.Facility); err != nil {
		return Request{}, err
	}
	if r.Info.State != DMSFacilityDeactivated && r.Info.State != DMSFacilityActivated {
		return Request{}, fmt.Errorf("DMS CK facility state %d is out of range", r.Info.State)
	}
	if err := validateDMSCK(r.Info.Key); err != nil {
		return Request{}, fmt.Errorf("validating facility CK: %w", err)
	}
	value := []byte{byte(r.Info.Facility), byte(r.Info.State), byte(len(r.Info.Key))}
	value = append(value, r.Info.Key...)
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMSetCKProtection,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// DMSUIMUnblockCKRequest encodes UIM Unblock CK.
type DMSUIMUnblockCKRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSCKUnblockRequest
}

// Request validates and converts the unblock operation into a QMI request.
func (r DMSUIMUnblockCKRequest) Request() (Request, error) {
	if err := validateDMSUIMFacility(r.Info.Facility); err != nil {
		return Request{}, err
	}
	if err := validateDMSCK(r.Info.Key); err != nil {
		return Request{}, fmt.Errorf("validating facility unblock CK: %w", err)
	}
	value := []byte{byte(r.Info.Facility), byte(len(r.Info.Key))}
	value = append(value, r.Info.Key...)
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMUnblockCK,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// DMSUIMGetStateRequest encodes the legacy UIM Get State message.
type DMSUIMGetStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSUIMGetStateRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSUIMGetState)
}

// DMSUIMGetStateResponse contains the legacy UIM initialization state.
type DMSUIMGetStateResponse struct {
	State DMSUIMState
}

// UnmarshalTLVs parses the mandatory UIM state.
func (r *DMSUIMGetStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSUIMGetStateResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI DMS UIM state: state TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS UIM state: state TLV length %d, want 1", len(value))
	}
	r.State = DMSUIMState(value[0])
	return nil
}

// DMSSetCKProtection activates or deactivates a legacy UIM facility.
func (c *Client) DMSSetCKProtection(ctx context.Context, req DMSCKProtectionRequest) (DMSCKOperationResult, error) {
	request, err := (DMSUIMSetCKProtectionRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSCKOperationResult{}, fmt.Errorf("setting QMI DMS CK protection: %w", err)
	}
	result, err := c.dmsCKOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("setting QMI DMS CK protection: %w", err)
	}
	return result, nil
}

// DMSUnblockCK unblocks a legacy UIM personalization facility.
func (c *Client) DMSUnblockCK(ctx context.Context, req DMSCKUnblockRequest) (DMSCKOperationResult, error) {
	request, err := (DMSUIMUnblockCKRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSCKOperationResult{}, fmt.Errorf("unblocking QMI DMS CK: %w", err)
	}
	result, err := c.dmsCKOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("unblocking QMI DMS CK: %w", err)
	}
	return result, nil
}

// DMSUIMState returns the legacy DMS view of UIM initialization.
func (c *Client) DMSUIMState(ctx context.Context) (DMSUIMState, error) {
	var result DMSUIMGetStateResponse
	if err := c.dmsRead(ctx, MessageDMSUIMGetState, &result); err != nil {
		return 0, fmt.Errorf("querying QMI DMS UIM state: %w", err)
	}
	return result.State, nil
}

func (c *Client) dmsCKOperation(ctx context.Context, req Request) (DMSCKOperationResult, error) {
	var result DMSCKOperationResult
	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		var decoded DMSCKOperationResult
		decodeErr := decoded.UnmarshalTLVs(resp.TLVs)
		if err := resultOK(resp); err != nil {
			result = decoded
			return err
		}
		if decodeErr != nil {
			return decodeErr
		}
		result = decoded
		return nil
	})
	return result, err
}

// UnmarshalTLVs parses the optional QMI DMS facility retry count.
func (r *DMSCKOperationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSCKOperationResult{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS CK retries: TLV length %d, want 1", len(value))
	}
	r.Retries = value[0]
	r.RetriesKnown = true
	return nil
}

func validateDMSUIMFacility(facility DMSUIMFacility) error {
	if facility > DMSUIMFacilityUIM {
		return fmt.Errorf("DMS UIM facility %d is out of range", facility)
	}
	return nil
}

func validateDMSCK(key string) error {
	if err := validateDMSASCII(key, dmsFacilityCKMax, false); err != nil {
		return err
	}
	for i := range len(key) {
		if key[i] == 0 {
			return errors.New("value contains a NUL byte")
		}
	}
	return nil
}
