package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const dsdAvailableSystemMax = 15

type dsdIndicationRegistration uint8

const (
	dsdIndicationSystemStatus dsdIndicationRegistration = iota
	dsdIndicationCurrentDDS
)

// DSDSubscription identifies a modem subscription.
type DSDSubscription uint32

const (
	DSDSubscriptionPrimary DSDSubscription = 1 + iota
	DSDSubscriptionSecondary
	DSDSubscriptionTertiary
)

// DSDSwitchType controls whether a default-data-subscription change persists.
type DSDSwitchType uint32

const (
	DSDSwitchPermanent DSDSwitchType = iota
	DSDSwitchTemporary
)

// DSDSwitchResult reports whether the modem accepted a DDS switch.
type DSDSwitchResult uint32

const (
	DSDSwitchAllowed DSDSwitchResult = iota
	DSDSwitchNotAllowed
	DSDSwitchFailed
)

// DSDCurrentDDS contains the current default data subscription and switch type.
type DSDCurrentDDS struct {
	Subscription      DSDSubscription
	SubscriptionKnown bool
	SwitchType        DSDSwitchType
	SwitchTypeKnown   bool
}

// DSDIndicationConfig selects general DSD indications. Nil fields are omitted.
type DSDIndicationConfig struct {
	CurrentDDS *bool
}

// DSDNetwork identifies the network family of an available data system.
type DSDNetwork uint32

const (
	DSDNetwork3GPP  DSDNetwork = 0
	DSDNetwork3GPP2 DSDNetwork = 1
	DSDNetworkWLAN  DSDNetwork = 2
)

// DSDRAT identifies the radio access technology of a data system.
type DSDRAT uint32

const (
	DSDRATNullBearer DSDRAT = 0x00
	DSDRATWCDMA      DSDRAT = 0x01
	DSDRATGERAN      DSDRAT = 0x02
	DSDRATLTE        DSDRAT = 0x03
	DSDRATTDSCDMA    DSDRAT = 0x04
	DSDRAT3GPPWLAN   DSDRAT = 0x05
	DSDRAT5G         DSDRAT = 0x06
	DSDRAT1X         DSDRAT = 0x65
	DSDRATHRPD       DSDRAT = 0x66
	DSDRATEHRPD      DSDRAT = 0x67
	DSDRAT3GPP2WLAN  DSDRAT = 0x68
	DSDRATWLAN       DSDRAT = 0xC9
)

// DSDServiceOptionMask describes the bearer capabilities active on a RAT.
type DSDServiceOptionMask uint64

const (
	DSDServiceOptionWCDMA          DSDServiceOptionMask = 1 << 0
	DSDServiceOptionHSDPA          DSDServiceOptionMask = 1 << 1
	DSDServiceOptionHSUPA          DSDServiceOptionMask = 1 << 2
	DSDServiceOptionHSDPAPlus      DSDServiceOptionMask = 1 << 3
	DSDServiceOptionDCHSDPAPlus    DSDServiceOptionMask = 1 << 4
	DSDServiceOption64QAM          DSDServiceOptionMask = 1 << 5
	DSDServiceOptionHSPA           DSDServiceOptionMask = 1 << 6
	DSDServiceOptionGPRS           DSDServiceOptionMask = 1 << 7
	DSDServiceOptionEDGE           DSDServiceOptionMask = 1 << 8
	DSDServiceOptionGSM            DSDServiceOptionMask = 1 << 9
	DSDServiceOptionS2B            DSDServiceOptionMask = 1 << 10
	DSDServiceOptionLimitedService DSDServiceOptionMask = 1 << 11
	DSDServiceOptionLTEFDD         DSDServiceOptionMask = 1 << 12
	DSDServiceOptionLTETDD         DSDServiceOptionMask = 1 << 13
	DSDServiceOptionTDSCDMA        DSDServiceOptionMask = 1 << 14
	DSDServiceOptionDCHSUPA        DSDServiceOptionMask = 1 << 15
	DSDServiceOptionLTECADownlink  DSDServiceOptionMask = 1 << 16
	DSDServiceOptionLTECAUplink    DSDServiceOptionMask = 1 << 17
	DSDServiceOptionS2BLimited     DSDServiceOptionMask = 1 << 18
	DSDServiceOption4Point5G       DSDServiceOptionMask = 1 << 19
	DSDServiceOption4Point5GPlus   DSDServiceOptionMask = 1 << 20
	DSDServiceOption1XIS95         DSDServiceOptionMask = 1 << 24
	DSDServiceOption1XIS2000       DSDServiceOptionMask = 1 << 25
	DSDServiceOption1XIS2000RelA   DSDServiceOptionMask = 1 << 26
	DSDServiceOptionHRPDRev0DPA    DSDServiceOptionMask = 1 << 27
	DSDServiceOptionHRPDRevADPA    DSDServiceOptionMask = 1 << 28
	DSDServiceOptionHRPDRevBDPA    DSDServiceOptionMask = 1 << 29
	DSDServiceOption5GTDD          DSDServiceOptionMask = 1 << 40
	DSDServiceOption5GSub6         DSDServiceOptionMask = 1 << 41
	DSDServiceOption5GMMWave       DSDServiceOptionMask = 1 << 42
	DSDServiceOption5GNSA          DSDServiceOptionMask = 1 << 43
	DSDServiceOption5GSA           DSDServiceOptionMask = 1 << 44
)

// DSDNullBearerReason is a mask explaining why no preferred bearer exists.
type DSDNullBearerReason uint64

const (
	DSDNullBearerReasonCSFB              DSDNullBearerReason = 1 << 0
	DSDNullBearerReasonOutOfService      DSDNullBearerReason = 1 << 1
	DSDNullBearerReasonLimitedService    DSDNullBearerReason = 1 << 2
	DSDNullBearerReasonVoiceSameSub      DSDNullBearerReason = 1 << 3
	DSDNullBearerReasonVoiceOtherSub     DSDNullBearerReason = 1 << 4
	DSDNullBearerReasonSRVCC             DSDNullBearerReason = 1 << 5
	DSDNullBearerReasonCircuitSwitchOnly DSDNullBearerReason = 1 << 6
	DSDNullBearerReasonAttachPending     DSDNullBearerReason = 1 << 7
)

// DSDSystem describes one available or preferred data system.
type DSDSystem struct {
	Network        DSDNetwork
	RAT            DSDRAT
	ServiceOptions DSDServiceOptionMask
}

// DSDPreferredSystems contains the current and modem-recommended systems.
type DSDPreferredSystems struct {
	Current     DSDSystem
	Recommended DSDSystem
}

// DSDSystemStatus contains the stable global fields from QMI DSD system
// status. APN policy fields are intentionally left to higher-level callers.
type DSDSystemStatus struct {
	Available             []DSDSystem
	AvailableKnown        bool
	Preferred             DSDPreferredSystems
	PreferredKnown        bool
	NullBearerReason      DSDNullBearerReason
	NullBearerReasonKnown bool
}

// DSDSystemStatusReportConfig updates selected system-status indication
// settings. Nil fields are omitted so callers do not overwrite settings they
// do not own.
type DSDSystemStatusReportConfig struct {
	LimitServiceOptionChanges *bool
	ReportChanges             *bool
	PreferredTechnologyOnly   *bool
	ReportNullBearerReason    *bool
}

// DSDGetSystemStatusRequest encodes QMI DSD Get System Status.
type DSDGetSystemStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DSDGetSystemStatusRequest) Request() Request {
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDGetSystemStatus,
		Timeout:       r.Timeout,
	}
}

// DSDSystemStatusChangeRequest encodes QMI DSD System Status Change.
type DSDSystemStatusChangeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        DSDSystemStatusReportConfig
}

// DSDBindSubscriptionRequest encodes Bind Subscription.
type DSDBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  DSDSubscription
}

// Request validates and converts the subscription into a QMI request.
func (r DSDBindSubscriptionRequest) Request() (Request, error) {
	if !validDSDSubscription(r.Subscription) {
		return Request{}, fmt.Errorf("DSD subscription %d is out of range", r.Subscription)
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDBindSubscription,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Subscription))},
	}, nil
}

// DSDGetBindSubscriptionRequest encodes Get Bind Subscription.
type DSDGetBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DSDGetBindSubscriptionRequest) Request() Request {
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDGetBindSubscription,
		Timeout:       r.Timeout,
	}
}

// DSDIndicationRegisterRequest encodes Indication Register.
type DSDIndicationRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        DSDIndicationConfig
}

// Request converts indication settings into QMI TLVs.
func (r DSDIndicationRegisterRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config.CurrentDDS != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, boolByte(*r.Config.CurrentDDS)))
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDIndicationRegister,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// DSDSwitchDDSRequest encodes Switch DDS.
type DSDSwitchDDSRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  DSDSubscription
	SwitchType    *DSDSwitchType
}

// Request validates and converts a DDS switch into QMI TLVs.
func (r DSDSwitchDDSRequest) Request() (Request, error) {
	if !validDSDSubscription(r.Subscription) {
		return Request{}, fmt.Errorf("DSD subscription %d is out of range", r.Subscription)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Subscription))}
	if r.SwitchType != nil {
		if *r.SwitchType > DSDSwitchTemporary {
			return Request{}, fmt.Errorf("DSD switch type %d is out of range", *r.SwitchType)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.SwitchType)))
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDSwitchDDS,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DSDGetCurrentDDSRequest encodes Get Current DDS.
type DSDGetCurrentDDSRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DSDGetCurrentDDSRequest) Request() Request {
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDGetCurrentDDS,
		Timeout:       r.Timeout,
	}
}

// DSDBindSubscriptionResponse contains a bound subscription when reported.
type DSDBindSubscriptionResponse struct {
	Subscription      DSDSubscription
	SubscriptionKnown bool
}

// UnmarshalTLVs parses a Get Bind Subscription response.
func (r *DSDBindSubscriptionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DSDBindSubscriptionResponse{}
	value, known, err := dsdUint32TLV(tlvs, 0x10)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD bound subscription: %w", err)
	}
	r.Subscription = DSDSubscription(value)
	r.SubscriptionKnown = known
	return nil
}

// DSDGetCurrentDDSResponse contains parsed Get Current DDS fields.
type DSDGetCurrentDDSResponse struct {
	Current DSDCurrentDDS
}

// UnmarshalTLVs parses a Get Current DDS response.
func (r *DSDGetCurrentDDSResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DSDGetCurrentDDSResponse{}
	value, known, err := dsdUint32TLV(tlvs, 0x10)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD current subscription: %w", err)
	}
	r.Current.Subscription = DSDSubscription(value)
	r.Current.SubscriptionKnown = known
	value, known, err = dsdUint32TLV(tlvs, 0x11)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD current switch type: %w", err)
	}
	r.Current.SwitchType = DSDSwitchType(value)
	r.Current.SwitchTypeKnown = known
	return nil
}

// DSDCurrentDDSIndication contains a current-DDS update.
type DSDCurrentDDSIndication struct {
	Current DSDCurrentDDS
}

// UnmarshalTLVs parses a Current DDS indication.
func (i *DSDCurrentDDSIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = DSDCurrentDDSIndication{}
	value, known, err := dsdUint32TLV(tlvs, 0x01)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD current DDS subscription: %w", err)
	}
	if !known {
		return errors.New("parsing QMI DSD current DDS: subscription TLV missing")
	}
	i.Current.Subscription = DSDSubscription(value)
	i.Current.SubscriptionKnown = true
	value, known, err = dsdUint32TLV(tlvs, 0x10)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD current DDS switch type: %w", err)
	}
	i.Current.SwitchType = DSDSwitchType(value)
	i.Current.SwitchTypeKnown = known
	return nil
}

// DSDSwitchDDSIndication contains the result of a Switch DDS request.
type DSDSwitchDDSIndication struct {
	Result DSDSwitchResult
}

// UnmarshalTLVs parses a Switch DDS indication.
func (i *DSDSwitchDDSIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = DSDSwitchDDSIndication{}
	value, known, err := dsdUint32TLV(tlvs, 0x01)
	if err != nil {
		return fmt.Errorf("parsing QMI DSD switch result: %w", err)
	}
	if !known {
		return errors.New("parsing QMI DSD switch result: result TLV missing")
	}
	i.Result = DSDSwitchResult(value)
	return nil
}

// Request converts the indication settings into QMI TLVs.
func (r DSDSystemStatusChangeRequest) Request() Request {
	var tlvs tlv.TLVs
	settings := []struct {
		kind  byte
		value *bool
	}{
		{kind: 0x10, value: r.Config.LimitServiceOptionChanges},
		{kind: 0x11, value: r.Config.ReportChanges},
		{kind: 0x12, value: r.Config.PreferredTechnologyOnly},
		{kind: 0x13, value: r.Config.ReportNullBearerReason},
	}
	for _, setting := range settings {
		if setting.value != nil {
			tlvs = append(tlvs, tlv.Uint(setting.kind, boolByte(*setting.value)))
		}
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDSystemStatusChange,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// DSDGetSystemStatusResponse is the parsed system-status response.
type DSDGetSystemStatusResponse struct {
	Status DSDSystemStatus
}

// UnmarshalTLVs parses the common global DSD system-status fields.
func (r *DSDGetSystemStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return r.Status.UnmarshalTLVs(tlvs)
}

// DSDSystemStatus reads the modem's current global data-system status.
func (c *Client) DSDSystemStatus(ctx context.Context) (DSDSystemStatus, error) {
	var status DSDSystemStatus
	err := c.withServiceClient(ctx, ServiceDSD, func(clientID uint8) error {
		req := DSDGetSystemStatusRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed DSDGetSystemStatusResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		status = parsed.Status
		return nil
	})
	if err != nil {
		return DSDSystemStatus{}, fmt.Errorf("reading QMI DSD system status: %w", err)
	}
	return status, nil
}

// DSDSetSystemStatusReport updates system-status indication settings.
func (c *Client) DSDSetSystemStatusReport(ctx context.Context, config DSDSystemStatusReportConfig) error {
	req := DSDSystemStatusChangeRequest{Timeout: DefaultRequestTimeout, Config: config}
	if err := c.dsdResultRequest(ctx, req.Request()); err != nil {
		return fmt.Errorf("configuring QMI DSD system status indications: %w", err)
	}
	return nil
}

// DSDBindSubscription associates the DSD client with a modem subscription.
func (c *Client) DSDBindSubscription(ctx context.Context, subscription DSDSubscription) error {
	req, err := (DSDBindSubscriptionRequest{
		Timeout:      DefaultRequestTimeout,
		Subscription: subscription,
	}).Request()
	if err != nil {
		return fmt.Errorf("binding QMI DSD subscription: %w", err)
	}
	if err := c.dsdResultRequest(ctx, req); err != nil {
		return fmt.Errorf("binding QMI DSD subscription: %w", err)
	}
	return nil
}

// DSDBoundSubscription returns the subscription associated with the DSD client.
func (c *Client) DSDBoundSubscription(ctx context.Context) (DSDSubscription, error) {
	var response DSDBindSubscriptionResponse
	req := DSDGetBindSubscriptionRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.dsdReadRequest(ctx, req, &response); err != nil {
		return 0, fmt.Errorf("querying QMI DSD bound subscription: %w", err)
	}
	if !response.SubscriptionKnown {
		return 0, errors.New("querying QMI DSD bound subscription: subscription TLV missing")
	}
	return response.Subscription, nil
}

// DSDSetIndicationReport updates general DSD indication settings.
func (c *Client) DSDSetIndicationReport(ctx context.Context, config DSDIndicationConfig) error {
	req := DSDIndicationRegisterRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.dsdResultRequest(ctx, req); err != nil {
		return fmt.Errorf("configuring QMI DSD indications: %w", err)
	}
	return nil
}

// DSDRequestSwitchDDS requests a DDS change without waiting for its result indication.
func (c *Client) DSDRequestSwitchDDS(ctx context.Context, subscription DSDSubscription, switchType DSDSwitchType) error {
	req, err := (DSDSwitchDDSRequest{
		Timeout:      DefaultRequestTimeout,
		Subscription: subscription,
		SwitchType:   &switchType,
	}).Request()
	if err != nil {
		return fmt.Errorf("requesting QMI DSD DDS switch: %w", err)
	}
	if err := c.dsdResultRequest(ctx, req); err != nil {
		return fmt.Errorf("requesting QMI DSD DDS switch: %w", err)
	}
	return nil
}

// DSDSwitchDDS requests a DDS change and waits for the modem's result indication.
func (c *Client) DSDSwitchDDS(ctx context.Context, subscription DSDSubscription, switchType DSDSwitchType) (DSDSwitchResult, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return 0, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceDSD)
	if err != nil {
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
	defer cancel()
	indications, err := transport.Indications(waitCtx, ServiceDSD, clientID, MessageDSDSwitchDDS)
	if err != nil {
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
	}
	req, err := (DSDSwitchDDSRequest{
		ClientID:     clientID,
		Timeout:      DefaultRequestTimeout,
		Subscription: subscription,
		SwitchType:   &switchType,
	}).Request()
	if err != nil {
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
	}
	resp, err := c.requestServiceWithTimeout(waitCtx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
	}
	select {
	case indication, ok := <-indications:
		if !ok {
			return 0, fmt.Errorf("switching QMI DSD DDS: %w", waitCtx.Err())
		}
		var result DSDSwitchDDSIndication
		if err := result.UnmarshalTLVs(indication.TLVs); err != nil {
			return 0, fmt.Errorf("switching QMI DSD DDS: %w", err)
		}
		return result.Result, nil
	case <-waitCtx.Done():
		return 0, fmt.Errorf("switching QMI DSD DDS: %w", waitCtx.Err())
	}
}

// DSDCurrentDDS returns the current default data subscription.
func (c *Client) DSDCurrentDDS(ctx context.Context) (DSDCurrentDDS, error) {
	var response DSDGetCurrentDDSResponse
	req := DSDGetCurrentDDSRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.dsdReadRequest(ctx, req, &response); err != nil {
		return DSDCurrentDDS{}, fmt.Errorf("querying QMI DSD current DDS: %w", err)
	}
	return response.Current, nil
}

// DSDWatchSystemStatus subscribes to global system-status changes.
func (c *Client) DSDWatchSystemStatus(ctx context.Context) (<-chan DSDSystemStatus, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceDSD)
	if err != nil {
		return nil, fmt.Errorf("watching QMI DSD system status: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceDSD, clientID, MessageDSDSystemStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI DSD system status: %w", err)
	}
	if err := c.acquireDSDIndication(ctx, dsdIndicationSystemStatus); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI DSD system status: %w", err)
	}

	out := make(chan DSDSystemStatus, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseDSDIndication(dsdIndicationSystemStatus)
		for indication := range indications {
			var status DSDSystemStatus
			if err := status.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- status:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

// DSDWatchCurrentDDS subscribes to current default-data-subscription changes.
func (c *Client) DSDWatchCurrentDDS(ctx context.Context) (<-chan DSDCurrentDDS, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceDSD)
	if err != nil {
		return nil, fmt.Errorf("watching QMI DSD current DDS: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceDSD, clientID, MessageDSDCurrentDDS)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI DSD current DDS: %w", err)
	}
	if err := c.acquireDSDIndication(ctx, dsdIndicationCurrentDDS); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI DSD current DDS: %w", err)
	}

	out := make(chan DSDCurrentDDS, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseDSDIndication(dsdIndicationCurrentDDS)
		for indication := range indications {
			var update DSDCurrentDDSIndication
			if err := update.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- update.Current:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) dsdResultRequest(ctx context.Context, req Request) error {
	return c.withServiceClient(ctx, ServiceDSD, func(clientID uint8) error {
		req.ClientID = clientID
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

func (c *Client) dsdReadRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceDSD, func(clientID uint8) error {
		req.ClientID = clientID
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func (c *Client) acquireDSDIndication(ctx context.Context, registration dsdIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.dsdIndicationRefs == nil {
		c.dsdIndicationRefs = make(map[dsdIndicationRegistration]int)
	}
	if c.dsdIndicationRefs[registration] > 0 {
		c.dsdIndicationRefs[registration]++
		return nil
	}
	c.dsdIndicationRefs[registration] = 1
	if err := c.setDSDIndicationRegistration(ctx, registration, true); err != nil {
		delete(c.dsdIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseDSDIndication(registration dsdIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.dsdIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.dsdIndicationRefs[registration]--
		return
	}
	delete(c.dsdIndicationRefs, registration)
	_ = c.setDSDIndicationRegistration(ctx, registration, false)
}

func (c *Client) setDSDIndicationRegistration(ctx context.Context, registration dsdIndicationRegistration, enabled bool) error {
	switch registration {
	case dsdIndicationSystemStatus:
		return c.DSDSetSystemStatusReport(ctx, DSDSystemStatusReportConfig{
			ReportChanges:          &enabled,
			ReportNullBearerReason: &enabled,
		})
	case dsdIndicationCurrentDDS:
		return c.DSDSetIndicationReport(ctx, DSDIndicationConfig{CurrentDDS: &enabled})
	default:
		return fmt.Errorf("configuring QMI DSD indications: registration %d is unknown", registration)
	}
}

func validDSDSubscription(subscription DSDSubscription) bool {
	return subscription >= DSDSubscriptionPrimary && subscription <= DSDSubscriptionTertiary
}

func dsdUint32TLV(tlvs tlv.TLVs, typ uint8) (uint32, bool, error) {
	value, ok := tlv.Value(tlvs, typ)
	if !ok {
		return 0, false, nil
	}
	if len(value) != 4 {
		return 0, false, fmt.Errorf("TLV 0x%02x length %d, want 4", typ, len(value))
	}
	return binary.LittleEndian.Uint32(value), true, nil
}

// UnmarshalTLVs parses the common QMI DSD system-status fields.
func (status *DSDSystemStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*status = DSDSystemStatus{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		systems, err := decodeDSDSystemList(value)
		if err != nil {
			return err
		}
		status.Available = systems
		status.AvailableKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 32 {
			return fmt.Errorf("parsing QMI DSD system status: preferred systems TLV length %d, want 32", len(value))
		}
		status.Preferred = DSDPreferredSystems{
			Current:     decodeDSDSystem(value[:16]),
			Recommended: decodeDSDSystem(value[16:32]),
		}
		status.PreferredKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DSD system status: null bearer reason TLV length %d, want 8", len(value))
		}
		status.NullBearerReason = DSDNullBearerReason(binary.LittleEndian.Uint64(value))
		status.NullBearerReasonKnown = true
	}
	return nil
}

func decodeDSDSystemList(value []byte) ([]DSDSystem, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DSD system status: available-system count is missing")
	}
	count := int(value[0])
	if count > dsdAvailableSystemMax {
		return nil, fmt.Errorf("parsing QMI DSD system status: available-system count %d exceeds %d", count, dsdAvailableSystemMax)
	}
	value = value[1:]
	wantLen := count * 16
	if len(value) != wantLen {
		return nil, fmt.Errorf("parsing QMI DSD system status: available-system list length %d, want %d", len(value), wantLen)
	}
	systems := make([]DSDSystem, 0, count)
	for i := range count {
		systems = append(systems, decodeDSDSystem(value[i*16:(i+1)*16]))
	}
	return systems, nil
}

func decodeDSDSystem(value []byte) DSDSystem {
	return DSDSystem{
		Network:        DSDNetwork(binary.LittleEndian.Uint32(value[:4])),
		RAT:            DSDRAT(binary.LittleEndian.Uint32(value[4:8])),
		ServiceOptions: DSDServiceOptionMask(binary.LittleEndian.Uint64(value[8:16])),
	}
}
