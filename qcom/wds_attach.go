package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wdsMaxLTEAttachPDNs = 56

// WDSIPSupportType identifies the IP family negotiated for LTE attach.
type WDSIPSupportType uint8

const (
	WDSIPSupportIPv4 WDSIPSupportType = iota
	WDSIPSupportIPv6
	WDSIPSupportIPv4v6
)

// WDSLTEAttachParameters contains the network-selected initial attach APN.
type WDSLTEAttachParameters struct {
	APN                string
	APNKnown           bool
	IPSupport          WDSIPSupportType
	IPSupportKnown     bool
	OTAAttachPerformed bool
	OTAAttachKnown     bool
}

// WDSGetLTEAttachParametersRequest encodes Get LTE Attach Parameters.
type WDSGetLTEAttachParametersRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetLTEAttachParametersRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetLTEAttachParameters)
}

// WDSGetLTEAttachParametersResponse is the parsed LTE attach response.
type WDSGetLTEAttachParametersResponse struct {
	Parameters WDSLTEAttachParameters
}

// UnmarshalTLVs parses the optional initial-attach fields.
func (r *WDSGetLTEAttachParametersResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetLTEAttachParametersResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		r.Parameters.APN = string(value)
		r.Parameters.APNKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS LTE attach parameters: IP support TLV length %d, want 1", len(value))
		}
		r.Parameters.IPSupport = WDSIPSupportType(value[0])
		r.Parameters.IPSupportKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS LTE attach parameters: OTA attach TLV length %d, want 1", len(value))
		}
		r.Parameters.OTAAttachPerformed = value[0] != 0
		r.Parameters.OTAAttachKnown = true
	}
	return nil
}

// WDSAttachPDNListAction controls disruption after updating the attach list.
type WDSAttachPDNListAction uint32

const (
	WDSAttachPDNListNoAction WDSAttachPDNListAction = iota + 1
	WDSAttachPDNListDetachOrDisconnect
)

// WDSSetLTEAttachPDNListRequest encodes Set LTE Attach PDN List.
type WDSSetLTEAttachPDNListRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profiles      []uint16
	Action        *WDSAttachPDNListAction
}

// Request validates and encodes the ordered profile list.
func (r WDSSetLTEAttachPDNListRequest) Request() (Request, error) {
	list, err := wdsAttachPDNList(r.Profiles).MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, list)}
	if r.Action != nil {
		if *r.Action < WDSAttachPDNListNoAction || *r.Action > WDSAttachPDNListDetachOrDisconnect {
			return Request{}, fmt.Errorf("encoding QMI WDS LTE attach PDN list: action %d is out of range", *r.Action)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.Action)))
	}
	return Request{
		Service: ServiceWDS, ClientID: r.ClientID, TransactionID: r.TransactionID,
		MessageID: MessageWDSSetLTEAttachPDNList, Timeout: r.Timeout, TLVs: tlvs,
	}, nil
}

// WDSGetLTEAttachPDNListRequest encodes Get LTE Attach PDN List.
type WDSGetLTEAttachPDNListRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetLTEAttachPDNListRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetLTEAttachPDNList)
}

// WDSGetMaxLTEAttachPDNNumberRequest encodes Get Max LTE Attach PDN Number.
type WDSGetMaxLTEAttachPDNNumberRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetMaxLTEAttachPDNNumberRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetMaxLTEAttachPDNNumber)
}

// WDSGetMaxLTEAttachPDNNumberResponse contains the modem attach-list limit.
type WDSGetMaxLTEAttachPDNNumberResponse struct {
	Maximum uint8
}

// UnmarshalTLVs parses the mandatory attach-list limit.
func (r *WDSGetMaxLTEAttachPDNNumberResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetMaxLTEAttachPDNNumberResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI WDS maximum LTE attach PDN number: info TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS maximum LTE attach PDN number: info TLV length %d, want 1", len(value))
	}
	r.Maximum = value[0]
	return nil
}

// WDSLTEAttachPDNList contains active and pending ordered profile IDs.
type WDSLTEAttachPDNList struct {
	Current      []uint16
	CurrentKnown bool
	Pending      []uint16
	PendingKnown bool
}

// WDSGetLTEAttachPDNListResponse is the parsed attach-list response.
type WDSGetLTEAttachPDNListResponse struct {
	List WDSLTEAttachPDNList
}

// UnmarshalTLVs parses the current and pending profile lists.
func (r *WDSGetLTEAttachPDNListResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetLTEAttachPDNListResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		var profiles wdsAttachPDNList
		if err := profiles.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS current LTE attach PDN list: %w", err)
		}
		r.List.Current, r.List.CurrentKnown = profiles, true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		var profiles wdsAttachPDNList
		if err := profiles.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS pending LTE attach PDN list: %w", err)
		}
		r.List.Pending, r.List.PendingKnown = profiles, true
	}
	return nil
}

// WDSLTEAttachParameters returns the network-selected initial attach APN.
func (c *Client) WDSLTEAttachParameters(ctx context.Context) (WDSLTEAttachParameters, error) {
	var parameters WDSLTEAttachParameters
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSGetLTEAttachParametersRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var decoded WDSGetLTEAttachParametersResponse
		if err := decoded.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		parameters = decoded.Parameters
		return nil
	})
	if err != nil {
		return WDSLTEAttachParameters{}, fmt.Errorf("reading QMI WDS LTE attach parameters: %w", err)
	}
	return parameters, nil
}

// WDSLTEAttachPDNList returns the active and pending LTE attach profile lists.
func (c *Client) WDSLTEAttachPDNList(ctx context.Context) (WDSLTEAttachPDNList, error) {
	var list WDSLTEAttachPDNList
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSGetLTEAttachPDNListRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var decoded WDSGetLTEAttachPDNListResponse
		if err := decoded.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		list = decoded.List
		return nil
	})
	if err != nil {
		return WDSLTEAttachPDNList{}, fmt.Errorf("reading QMI WDS LTE attach PDN list: %w", err)
	}
	return list, nil
}

// WDSMaxLTEAttachPDNs returns the modem's maximum LTE attach profile count.
func (c *Client) WDSMaxLTEAttachPDNs(ctx context.Context) (uint8, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetMaxLTEAttachPDNNumber, nil)
	if err != nil {
		return 0, fmt.Errorf("querying QMI WDS maximum LTE attach PDN number: %w", err)
	}
	var parsed WDSGetMaxLTEAttachPDNNumberResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return 0, err
	}
	return parsed.Maximum, nil
}

// WDSSetLTEAttachPDNList replaces the ordered LTE attach profile list.
func (c *Client) WDSSetLTEAttachPDNList(ctx context.Context, profiles []uint16, action *WDSAttachPDNListAction) error {
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req, err := (WDSSetLTEAttachPDNListRequest{
			ClientID: clientID, Timeout: DefaultRequestTimeout, Profiles: profiles, Action: action,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("setting QMI WDS LTE attach PDN list: %w", err)
	}
	return nil
}

type wdsAttachPDNList []uint16

func (profiles wdsAttachPDNList) MarshalBinary() ([]byte, error) {
	if len(profiles) > wdsMaxLTEAttachPDNs {
		return nil, fmt.Errorf("LTE attach PDN list length %d exceeds maximum %d", len(profiles), wdsMaxLTEAttachPDNs)
	}
	encoded := make([]byte, 1, 1+len(profiles)*2)
	encoded[0] = byte(len(profiles))
	for _, profile := range profiles {
		encoded = binary.LittleEndian.AppendUint16(encoded, profile)
	}
	return encoded, nil
}

func (profiles *wdsAttachPDNList) UnmarshalBinary(value []byte) error {
	if len(value) == 0 {
		return errors.New("LTE attach PDN count is missing")
	}
	count := int(value[0])
	if count > wdsMaxLTEAttachPDNs {
		return fmt.Errorf("LTE attach PDN count %d exceeds maximum %d", count, wdsMaxLTEAttachPDNs)
	}
	want := 1 + count*2
	if len(value) != want {
		return fmt.Errorf("LTE attach PDN list length %d, want %d", len(value), want)
	}
	decoded := make(wdsAttachPDNList, count)
	for index := range decoded {
		offset := 1 + index*2
		decoded[index] = binary.LittleEndian.Uint16(value[offset : offset+2])
	}
	*profiles = decoded
	return nil
}
