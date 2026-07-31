package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVUIMPINInfo        = 0x01
	dmsTLVPINRetries        = 0x10
	dmsTLVPIN1Status        = 0x11
	dmsTLVPIN2Status        = 0x12
	dmsTLVActivationState   = 0x01
	dmsTLVFacility          = 0x01
	dmsTLVFacilityStatus    = 0x01
	dmsTLVOperationBlocking = 0x10
	dmsTLVActivationCode    = 0x01

	dmsPINValueMax       = 16
	dmsPUKValueMax       = 16
	dmsActivationCodeMax = 81
	dmsSPCLength         = 6
	dmsMDNMax            = 15
	dmsMINMax            = 15
	dmsHAKeyMax          = 16
	dmsAAAKeyMax         = 16
	dmsPRLDataMax        = 1536
	dmsPRLTotalMax       = 16384
	dmsFacilityCKMax     = 8
)

// DMSPINID identifies one of the two legacy DMS UIM PINs.
type DMSPINID uint8

const (
	DMSPINIDPIN1 DMSPINID = 0x01
	DMSPINIDPIN2 DMSPINID = 0x02
)

// DMSPINOperationResult contains retry counters returned by a PIN operation.
// Counters may be present even when the modem returns an operation error.
type DMSPINOperationResult struct {
	VerifyRetries  uint8
	UnblockRetries uint8
	RetriesKnown   bool
}

// DMSPINProtectionRequest enables or disables one legacy DMS UIM PIN.
type DMSPINProtectionRequest struct {
	ID      DMSPINID
	Enabled bool
	PIN     string
}

// DMSPINVerifyRequest verifies one legacy DMS UIM PIN.
type DMSPINVerifyRequest struct {
	ID  DMSPINID
	PIN string
}

// DMSPINUnblockRequest unblocks a legacy DMS UIM PIN with its PUK.
type DMSPINUnblockRequest struct {
	ID     DMSPINID
	PUK    string
	NewPIN string
}

// DMSPINChangeRequest replaces a legacy DMS UIM PIN.
type DMSPINChangeRequest struct {
	ID     DMSPINID
	OldPIN string
	NewPIN string
}

// DMSUIMSetPINProtectionRequest encodes DMS UIM Set PIN Protection.
type DMSUIMSetPINProtectionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSPINProtectionRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMSetPINProtectionRequest) Request() (Request, error) {
	value, err := r.Info.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMSetPINProtection,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVUIMPINInfo, value)},
	}, nil
}

// DMSUIMVerifyPINRequest encodes DMS UIM Verify PIN.
type DMSUIMVerifyPINRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSPINVerifyRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMVerifyPINRequest) Request() (Request, error) {
	value, err := r.Info.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMVerifyPIN,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVUIMPINInfo, value)},
	}, nil
}

// DMSUIMUnblockPINRequest encodes DMS UIM Unblock PIN.
type DMSUIMUnblockPINRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSPINUnblockRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMUnblockPINRequest) Request() (Request, error) {
	value, err := r.Info.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMUnblockPIN,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVUIMPINInfo, value)},
	}, nil
}

// DMSUIMChangePINRequest encodes DMS UIM Change PIN.
type DMSUIMChangePINRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSPINChangeRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMChangePINRequest) Request() (Request, error) {
	value, err := r.Info.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMChangePIN,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVUIMPINInfo, value)},
	}, nil
}

// DMSUIMGetPINStatusRequest encodes DMS UIM Get PIN Status.
type DMSUIMGetPINStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMGetPINStatusRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSUIMGetPINStatus)
}

// DMSPINStatusResponse contains the optional status of PIN1 and PIN2.
type DMSPINStatusResponse struct {
	PIN1      DMSPINState
	PIN1Known bool
	PIN2      DMSPINState
	PIN2Known bool
}

// UnmarshalTLVs parses a QMI DMS UIM Get PIN Status response.
func (r *DMSPINStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSPINStatusResponse{}
	if value, ok := tlv.Value(tlvs, dmsTLVPIN1Status); ok {
		state, err := decodeDMSPINState(value)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS PIN1 status: %w", err)
		}
		r.PIN1 = state
		r.PIN1Known = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVPIN2Status); ok {
		state, err := decodeDMSPINState(value)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS PIN2 status: %w", err)
		}
		r.PIN2 = state
		r.PIN2Known = true
	}
	return nil
}

// DMSSetPINProtection enables or disables a legacy DMS UIM PIN.
func (c *Client) DMSSetPINProtection(ctx context.Context, req DMSPINProtectionRequest) (DMSPINOperationResult, error) {
	request, err := (DMSUIMSetPINProtectionRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSPINOperationResult{}, fmt.Errorf("setting QMI DMS PIN protection: %w", err)
	}
	result, err := c.dmsPINOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("setting QMI DMS PIN protection: %w", err)
	}
	return result, nil
}

// DMSVerifyPIN verifies a legacy DMS UIM PIN.
func (c *Client) DMSVerifyPIN(ctx context.Context, req DMSPINVerifyRequest) (DMSPINOperationResult, error) {
	request, err := (DMSUIMVerifyPINRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSPINOperationResult{}, fmt.Errorf("verifying QMI DMS PIN: %w", err)
	}
	result, err := c.dmsPINOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("verifying QMI DMS PIN: %w", err)
	}
	return result, nil
}

// DMSUnblockPIN unblocks a legacy DMS UIM PIN with its PUK.
func (c *Client) DMSUnblockPIN(ctx context.Context, req DMSPINUnblockRequest) (DMSPINOperationResult, error) {
	request, err := (DMSUIMUnblockPINRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSPINOperationResult{}, fmt.Errorf("unblocking QMI DMS PIN: %w", err)
	}
	result, err := c.dmsPINOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("unblocking QMI DMS PIN: %w", err)
	}
	return result, nil
}

// DMSChangePIN replaces a legacy DMS UIM PIN.
func (c *Client) DMSChangePIN(ctx context.Context, req DMSPINChangeRequest) (DMSPINOperationResult, error) {
	request, err := (DMSUIMChangePINRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSPINOperationResult{}, fmt.Errorf("changing QMI DMS PIN: %w", err)
	}
	result, err := c.dmsPINOperation(ctx, request)
	if err != nil {
		return result, fmt.Errorf("changing QMI DMS PIN: %w", err)
	}
	return result, nil
}

// DMSPINStatuses returns the current status of legacy DMS UIM PINs.
func (c *Client) DMSPINStatuses(ctx context.Context) (DMSPINStatusResponse, error) {
	var result DMSPINStatusResponse
	if err := c.dmsRead(ctx, MessageDMSUIMGetPINStatus, &result); err != nil {
		return DMSPINStatusResponse{}, fmt.Errorf("querying QMI DMS PIN status: %w", err)
	}
	return result, nil
}

// DMSUIMGetPINStatus is an alias for DMSPINStatuses.
func (c *Client) DMSUIMGetPINStatus(ctx context.Context) (DMSPINStatusResponse, error) {
	return c.DMSPINStatuses(ctx)
}

func (c *Client) dmsPINOperation(ctx context.Context, req Request) (DMSPINOperationResult, error) {
	var result DMSPINOperationResult
	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		var decoded DMSPINOperationResult
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

// UnmarshalTLVs parses the optional QMI DMS PIN retry counts.
func (r *DMSPINOperationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSPINOperationResult{}
	value, ok := tlv.Value(tlvs, dmsTLVPINRetries)
	if !ok {
		return nil
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI DMS PIN retries: TLV length %d, want 2", len(value))
	}
	r.VerifyRetries = value[0]
	r.UnblockRetries = value[1]
	r.RetriesKnown = true
	return nil
}

// MarshalBinary encodes the QMI DMS PIN protection aggregate.
func (req DMSPINProtectionRequest) MarshalBinary() ([]byte, error) {
	if err := validateDMSPINID(req.ID); err != nil {
		return nil, err
	}
	if err := validateDMSASCII(req.PIN, dmsPINValueMax, false); err != nil {
		return nil, fmt.Errorf("validating PIN: %w", err)
	}
	value := []byte{byte(req.ID), boolByte(req.Enabled), byte(len(req.PIN))}
	return append(value, req.PIN...), nil
}

// MarshalBinary encodes the QMI DMS PIN verification aggregate.
func (req DMSPINVerifyRequest) MarshalBinary() ([]byte, error) {
	if err := validateDMSPINID(req.ID); err != nil {
		return nil, err
	}
	if err := validateDMSASCII(req.PIN, dmsPINValueMax, false); err != nil {
		return nil, fmt.Errorf("validating PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.PIN))}
	return append(value, req.PIN...), nil
}

// MarshalBinary encodes the QMI DMS PIN unblock aggregate.
func (req DMSPINUnblockRequest) MarshalBinary() ([]byte, error) {
	if err := validateDMSPINID(req.ID); err != nil {
		return nil, err
	}
	if err := validateDMSASCII(req.PUK, dmsPUKValueMax, false); err != nil {
		return nil, fmt.Errorf("validating PUK: %w", err)
	}
	if err := validateDMSASCII(req.NewPIN, dmsPINValueMax, false); err != nil {
		return nil, fmt.Errorf("validating new PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.PUK))}
	value = append(value, req.PUK...)
	value = append(value, byte(len(req.NewPIN)))
	return append(value, req.NewPIN...), nil
}

// MarshalBinary encodes the QMI DMS PIN change aggregate.
func (req DMSPINChangeRequest) MarshalBinary() ([]byte, error) {
	if err := validateDMSPINID(req.ID); err != nil {
		return nil, err
	}
	if err := validateDMSASCII(req.OldPIN, dmsPINValueMax, false); err != nil {
		return nil, fmt.Errorf("validating old PIN: %w", err)
	}
	if err := validateDMSASCII(req.NewPIN, dmsPINValueMax, false); err != nil {
		return nil, fmt.Errorf("validating new PIN: %w", err)
	}
	value := []byte{byte(req.ID), byte(len(req.OldPIN))}
	value = append(value, req.OldPIN...)
	value = append(value, byte(len(req.NewPIN)))
	return append(value, req.NewPIN...), nil
}

func validateDMSPINID(id DMSPINID) error {
	if id != DMSPINIDPIN1 && id != DMSPINIDPIN2 {
		return fmt.Errorf("invalid DMS PIN ID %d", id)
	}
	return nil
}

func validateDMSASCII(value string, max int, allowEmpty bool) error {
	if !allowEmpty && value == "" {
		return errors.New("value is empty")
	}
	if len(value) > max {
		return fmt.Errorf("value length %d exceeds %d", len(value), max)
	}
	for i := range len(value) {
		if value[i] > 0x7F {
			return errors.New("value contains a non-ASCII character")
		}
	}
	return nil
}

// DMSUIMGetICCIDRequest encodes DMS UIM Get ICCID.
type DMSUIMGetICCIDRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMGetICCIDRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSUIMGetICCID)
}

// DMSICCID returns the ICCID reported by the legacy DMS UIM interface.
func (c *Client) DMSICCID(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSUIMGetICCID)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS UIM ICCID: %w", err)
	}
	return value, nil
}

// ICCID is an alias for DMSICCID.
func (c *Client) ICCID(ctx context.Context) (string, error) {
	return c.DMSICCID(ctx)
}

// DMSUIMGetIMSIRequest encodes DMS UIM Get IMSI.
type DMSUIMGetIMSIRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMGetIMSIRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSUIMGetIMSI)
}

// DMSIMSI returns the IMSI reported by the legacy DMS UIM interface.
func (c *Client) DMSIMSI(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSUIMGetIMSI)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS UIM IMSI: %w", err)
	}
	return value, nil
}

// IMSI is an alias for DMSIMSI.
func (c *Client) IMSI(ctx context.Context) (string, error) {
	return c.DMSIMSI(ctx)
}

// DMSUIMFacility identifies a legacy DMS UIM personalization facility.
type DMSUIMFacility uint8

const (
	DMSUIMFacilityNetwork         DMSUIMFacility = 0x00
	DMSUIMFacilityNetworkSubset   DMSUIMFacility = 0x01
	DMSUIMFacilityServiceProvider DMSUIMFacility = 0x02
	DMSUIMFacilityCorporate       DMSUIMFacility = 0x03
	DMSUIMFacilityUIM             DMSUIMFacility = 0x04
)

// DMSFacilityState identifies the state of a personalization control key.
type DMSFacilityState uint8

const (
	DMSFacilityDeactivated DMSFacilityState = 0x00
	DMSFacilityActivated   DMSFacilityState = 0x01
	DMSFacilityBlocked     DMSFacilityState = 0x02
)

// DMSCKStatus contains the legacy DMS UIM control-key status.
type DMSCKStatus struct {
	FacilityState          DMSFacilityState
	VerifyRetries          uint8
	UnblockRetries         uint8
	OperationBlocking      bool
	OperationBlockingKnown bool
}

// DMSUIMGetCKStatusRequest encodes DMS UIM Get CK Status.
type DMSUIMGetCKStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Facility      DMSUIMFacility
}

// Request converts the request into a QMI DMS request.
func (r DMSUIMGetCKStatusRequest) Request() (Request, error) {
	if r.Facility > DMSUIMFacilityUIM {
		return Request{}, fmt.Errorf("invalid DMS UIM facility %d", r.Facility)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSUIMGetCKStatus,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(dmsTLVFacility, uint8(r.Facility))},
	}, nil
}

// DMSGetCKStatus queries a legacy DMS UIM control-key status.
func (c *Client) DMSGetCKStatus(ctx context.Context, facility DMSUIMFacility) (DMSCKStatus, error) {
	request, err := (DMSUIMGetCKStatusRequest{
		Timeout:  DefaultRequestTimeout,
		Facility: facility,
	}).Request()
	if err != nil {
		return DMSCKStatus{}, fmt.Errorf("querying QMI DMS CK status: %w", err)
	}
	var result DMSCKStatus
	err = c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, request.MessageID, request.TLVs, request.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return DMSCKStatus{}, fmt.Errorf("querying QMI DMS CK status: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses a DMS UIM Get CK Status response.
func (s *DMSCKStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = DMSCKStatus{}
	value, ok := tlv.Value(tlvs, dmsTLVFacilityStatus)
	if !ok {
		return errors.New("parsing QMI DMS CK status: facility status TLV missing")
	}
	if len(value) != 3 {
		return fmt.Errorf("parsing QMI DMS CK status: facility status TLV length %d, want 3", len(value))
	}
	s.FacilityState = DMSFacilityState(value[0])
	s.VerifyRetries = value[1]
	s.UnblockRetries = value[2]
	if value, ok := tlv.Value(tlvs, dmsTLVOperationBlocking); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DMS CK status: operation blocking TLV length %d, want 1", len(value))
		}
		s.OperationBlocking = value[0] != 0
		s.OperationBlockingKnown = true
	}
	return nil
}

// DMSGetActivationStateRequest encodes DMS Get Activation State.
type DMSGetActivationStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetActivationStateRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetActivationState)
}

// ActivationState returns the CDMA activation state reported by DMS.
func (c *Client) ActivationState(ctx context.Context) (DMSActivationState, error) {
	var result DMSActivationState
	if err := c.dmsRead(ctx, MessageDMSGetActivationState, &result); err != nil {
		return 0, fmt.Errorf("querying QMI DMS activation state: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses the QMI DMS activation state.
func (state *DMSActivationState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*state = DMSActivationNotActivated
	value, ok := tlv.Value(tlvs, dmsTLVActivationState)
	if !ok {
		return errors.New("parsing QMI DMS activation state: state TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI DMS activation state: state TLV length %d, want 2", len(value))
	}
	*state = DMSActivationState(binary.LittleEndian.Uint16(value))
	return nil
}

// GetActivationState is an alias for ActivationState.
func (c *Client) GetActivationState(ctx context.Context) (DMSActivationState, error) {
	return c.ActivationState(ctx)
}

// DMSActivateAutomaticRequest encodes DMS Activate Automatic.
type DMSActivateAutomaticRequest struct {
	ClientID       uint8
	TransactionID  uint16
	Timeout        time.Duration
	ActivationCode string
}

// Request converts the request into a QMI DMS request.
func (r DMSActivateAutomaticRequest) Request() (Request, error) {
	if err := validateDMSASCII(r.ActivationCode, dmsActivationCodeMax, true); err != nil {
		return Request{}, fmt.Errorf("validating activation code: %w", err)
	}
	value := append([]byte{byte(len(r.ActivationCode))}, r.ActivationCode...)
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSActivateAutomatic,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVActivationCode, value)},
	}, nil
}

// ActivateAutomatic requests automatic service activation.
func (c *Client) ActivateAutomatic(ctx context.Context, activationCode string) error {
	request, err := (DMSActivateAutomaticRequest{
		Timeout:        DefaultRequestTimeout,
		ActivationCode: activationCode,
	}).Request()
	if err != nil {
		return fmt.Errorf("activating QMI DMS service automatically: %w", err)
	}
	if err := c.dmsReadRequest(ctx, request, nil); err != nil {
		return fmt.Errorf("activating QMI DMS service automatically: %w", err)
	}
	return nil
}

// DMSPreferredRoamingList is one PRL segment used by manual activation.
type DMSPreferredRoamingList struct {
	TotalLength uint16
	Segment     uint8
	Data        []byte
}

// DMSManualActivationRequest contains CDMA manual activation data.
type DMSManualActivationRequest struct {
	SPC                  string
	SID                  uint16
	MDN                  string
	MIN                  string
	MNHAKey              *string
	MNAAAKey             *string
	PreferredRoamingList *DMSPreferredRoamingList
}

// DMSActivateManualRequest encodes DMS Activate Manual.
type DMSActivateManualRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSManualActivationRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSActivateManualRequest) Request() (Request, error) {
	info, err := r.Info.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, info)}
	if r.Info.MNHAKey != nil {
		if err := validateDMSASCII(*r.Info.MNHAKey, dmsHAKeyMax, true); err != nil {
			return Request{}, fmt.Errorf("validating MN-HA key: %w", err)
		}
		value := append([]byte{byte(len(*r.Info.MNHAKey))}, (*r.Info.MNHAKey)...)
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}
	if r.Info.MNAAAKey != nil {
		if err := validateDMSASCII(*r.Info.MNAAAKey, dmsAAAKeyMax, true); err != nil {
			return Request{}, fmt.Errorf("validating MN-AAA key: %w", err)
		}
		value := append([]byte{byte(len(*r.Info.MNAAAKey))}, (*r.Info.MNAAAKey)...)
		tlvs = append(tlvs, tlv.Bytes(0x12, value))
	}
	if r.Info.PreferredRoamingList != nil {
		value, err := r.Info.PreferredRoamingList.MarshalBinary()
		if err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(0x13, value))
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSActivateManual,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// ActivateManual requests manual CDMA service activation.
func (c *Client) ActivateManual(ctx context.Context, req DMSManualActivationRequest) error {
	request, err := (DMSActivateManualRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return fmt.Errorf("activating QMI DMS service manually: %w", err)
	}
	if err := c.dmsReadRequest(ctx, request, nil); err != nil {
		return fmt.Errorf("activating QMI DMS service manually: %w", err)
	}
	return nil
}

// MarshalBinary encodes the QMI DMS manual activation aggregate.
func (req DMSManualActivationRequest) MarshalBinary() ([]byte, error) {
	if err := validateDMSFixedASCII(req.SPC, dmsSPCLength); err != nil {
		return nil, fmt.Errorf("validating SPC: %w", err)
	}
	if err := validateDMSASCII(req.MDN, dmsMDNMax, true); err != nil {
		return nil, fmt.Errorf("validating MDN: %w", err)
	}
	if err := validateDMSASCII(req.MIN, dmsMINMax, true); err != nil {
		return nil, fmt.Errorf("validating MIN: %w", err)
	}
	value := make([]byte, 0, dmsSPCLength+2+1+len(req.MDN)+1+len(req.MIN))
	value = append(value, req.SPC...)
	value = binary.LittleEndian.AppendUint16(value, req.SID)
	value = append(value, byte(len(req.MDN)))
	value = append(value, req.MDN...)
	value = append(value, byte(len(req.MIN)))
	return append(value, req.MIN...), nil
}

// MarshalBinary encodes one QMI DMS preferred roaming list segment.
func (req DMSPreferredRoamingList) MarshalBinary() ([]byte, error) {
	if len(req.Data) > dmsPRLDataMax {
		return nil, fmt.Errorf("PRL segment length %d exceeds %d", len(req.Data), dmsPRLDataMax)
	}
	totalLength := req.TotalLength
	if totalLength == 0 {
		totalLength = uint16(len(req.Data))
	}
	if totalLength < uint16(len(req.Data)) || int(totalLength) > dmsPRLTotalMax {
		return nil, fmt.Errorf("PRL total length %d is invalid", totalLength)
	}
	value := make([]byte, 0, 5+len(req.Data))
	value = binary.LittleEndian.AppendUint16(value, totalLength)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(req.Data)))
	value = append(value, req.Segment)
	return append(value, req.Data...), nil
}

// DMSRestoreFactoryDefaultsRequest encodes DMS Restore Factory Defaults.
type DMSRestoreFactoryDefaultsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	SPC           string
}

// Request converts the request into a QMI DMS request.
func (r DMSRestoreFactoryDefaultsRequest) Request() (Request, error) {
	if err := validateDMSFixedASCII(r.SPC, dmsSPCLength); err != nil {
		return Request{}, fmt.Errorf("validating SPC: %w", err)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSRestoreFactoryDefaults,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, []byte(r.SPC))},
	}, nil
}

// RestoreFactoryDefaults restores the modem's DMS factory settings.
func (c *Client) RestoreFactoryDefaults(ctx context.Context, spc string) error {
	request, err := (DMSRestoreFactoryDefaultsRequest{
		Timeout: DefaultRequestTimeout,
		SPC:     spc,
	}).Request()
	if err != nil {
		return fmt.Errorf("restoring QMI DMS factory defaults: %w", err)
	}
	if err := c.dmsReadRequest(ctx, request, nil); err != nil {
		return fmt.Errorf("restoring QMI DMS factory defaults: %w", err)
	}
	return nil
}

func validateDMSFixedASCII(value string, length int) error {
	if len(value) != length {
		return fmt.Errorf("length %d, want %d", len(value), length)
	}
	for i := range len(value) {
		if value[i] > 0x7F {
			return errors.New("contains a non-ASCII character")
		}
	}
	return nil
}
