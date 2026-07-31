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
	imsaTLVIMSRegistered           = 0x10
	imsaTLVFailureCode             = 0x11
	imsaTLVRegStatus               = 0x12
	imsaTLVRegistrationError       = 0x13
	imsaTLVRegistrationRAT         = 0x14
	imsaTLVSMSService              = 0x10
	imsaTLVVoIPService             = 0x11
	imsaTLVVTService               = 0x12
	imsaTLVSMSRAT                  = 0x13
	imsaTLVVoIPRAT                 = 0x14
	imsaTLVVTRAT                   = 0x15
	imsaTLVUTService               = 0x16
	imsaTLVUTRAT                   = 0x17
	imsaTLVVSService               = 0x18
	imsaTLVVSRAT                   = 0x19
	imsaTLVRegistrationIndication  = 0x10
	imsaTLVServiceIndication       = 0x11
	imsaTLVBinding                 = 0x10
	imsaRegistrationErrorMaxLength = 255
)

type imsaIndicationRegistration uint8

const (
	imsaIndicationRegistrationStatus imsaIndicationRegistration = iota
	imsaIndicationServiceStatus
)

// IMSASubscription identifies the modem subscription used by an IMSA client.
type IMSASubscription uint32

const (
	IMSASubscriptionPrimary IMSASubscription = iota
	IMSASubscriptionSecondary
	IMSASubscriptionTertiary
)

// IMSAIndicationConfig selects IMSA status indications. Nil fields are omitted.
type IMSAIndicationConfig struct {
	RegistrationStatus *bool
	ServiceStatus      *bool
}

// IMSAGetRegistrationStatusRequest encodes QMI IMSA Get Registration Status.
type IMSAGetRegistrationStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI IMSA request.
func (r IMSAGetRegistrationStatusRequest) Request() Request {
	return Request{
		Service:       ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSAGetRegistrationStatus,
		Timeout:       r.Timeout,
	}
}

// IMSAGetServiceStatusRequest encodes QMI IMSA Get Service Status.
type IMSAGetServiceStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI IMSA request.
func (r IMSAGetServiceStatusRequest) Request() Request {
	return Request{
		Service:       ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSAGetServiceStatus,
		Timeout:       r.Timeout,
	}
}

// IMSARegisterIndicationsRequest encodes QMI IMSA Register Indications.
type IMSARegisterIndicationsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        IMSAIndicationConfig
}

// Request converts the indication settings into a QMI request.
func (r IMSARegisterIndicationsRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config.RegistrationStatus != nil {
		tlvs = append(tlvs, tlv.Uint(imsaTLVRegistrationIndication, boolByte(*r.Config.RegistrationStatus)))
	}
	if r.Config.ServiceStatus != nil {
		tlvs = append(tlvs, tlv.Uint(imsaTLVServiceIndication, boolByte(*r.Config.ServiceStatus)))
	}
	return Request{
		Service:       ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSARegisterIndications,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// IMSABindRequest encodes QMI IMSA Bind.
type IMSABindRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  IMSASubscription
}

// Request validates and converts the subscription into a QMI request.
func (r IMSABindRequest) Request() (Request, error) {
	if !validIMSASubscription(r.Subscription) {
		return Request{}, fmt.Errorf("IMSA subscription %d is out of range", r.Subscription)
	}
	return Request{
		Service:       ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSABind,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(imsaTLVBinding, uint32(r.Subscription))},
	}, nil
}

// IMSAGetBindRequest encodes QMI IMSA Get Bind.
type IMSAGetBindRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r IMSAGetBindRequest) Request() Request {
	return Request{
		Service:       ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSAGetBind,
		Timeout:       r.Timeout,
	}
}

// IMSAGetBindResponse contains the subscription bound to an IMSA client.
type IMSAGetBindResponse struct {
	Subscription      IMSASubscription
	SubscriptionKnown bool
}

// UnmarshalTLVs parses QMI IMSA Get Bind response TLVs.
func (r *IMSAGetBindResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSAGetBindResponse{}
	value, ok := tlv.Value(tlvs, imsaTLVBinding)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI IMSA binding: TLV length %d, want 4", len(value))
	}
	r.Subscription = IMSASubscription(binary.LittleEndian.Uint32(value))
	r.SubscriptionKnown = true
	return nil
}

// IMSAStatus reads IMS registration and VoIP service state from QMI IMSA.
func (c *Client) IMSAStatus(ctx context.Context) (IMSAStatus, error) {
	if c == nil {
		return IMSAStatus{}, errors.New("querying QMI IMSA status: client is nil")
	}
	var status IMSAStatus
	err := c.withServiceClient(ctx, ServiceIMSA, func(clientID uint8) error {
		var err error
		status, err = c.imsaStatus(ctx, clientID)
		return err
	})
	if err != nil {
		return IMSAStatus{}, fmt.Errorf("querying QMI IMSA status: %w", err)
	}
	return status, nil
}

func (c *Client) imsaStatus(ctx context.Context, clientID uint8) (IMSAStatus, error) {
	regReq := IMSAGetRegistrationStatusRequest{
		ClientID: clientID,
		Timeout:  DefaultRequestTimeout,
	}.Request()
	regResp, err := c.requestServiceWithTimeout(ctx, regReq.Service, regReq.ClientID, regReq.MessageID, regReq.TLVs, regReq.Timeout)
	if err != nil {
		return IMSAStatus{}, err
	}
	if err := resultOK(regResp); err != nil {
		return IMSAStatus{}, err
	}
	var regStatus IMSARegistrationStatusResponse
	if err := regStatus.UnmarshalTLVs(regResp.TLVs); err != nil {
		return IMSAStatus{}, err
	}

	serviceReq := IMSAGetServiceStatusRequest{
		ClientID: clientID,
		Timeout:  DefaultRequestTimeout,
	}.Request()
	serviceResp, err := c.requestServiceWithTimeout(ctx, serviceReq.Service, serviceReq.ClientID, serviceReq.MessageID, serviceReq.TLVs, serviceReq.Timeout)
	if err != nil {
		return IMSAStatus{}, err
	}
	if err := resultOK(serviceResp); err != nil {
		return IMSAStatus{}, err
	}
	var serviceStatus IMSAServiceStatusResponse
	if err := serviceStatus.UnmarshalTLVs(serviceResp.TLVs); err != nil {
		return IMSAStatus{}, err
	}

	mergeIMSAServiceStatus(&regStatus.Status, serviceStatus.Status)
	return regStatus.Status, nil
}

// IMSARegistrationStatusResponse is the parsed QMI IMSA registration status.
type IMSARegistrationStatusResponse struct {
	Status IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA registration fields.
func (r *IMSARegistrationStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSARegistrationStatusResponse{}
	if value, ok := tlv.Value(tlvs, imsaTLVRegStatus); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI IMSA registration status: status TLV length %d, want 4", len(value))
		}
		r.Status.RegistrationKnown = true
		r.Status.Registration = IMSRegistrationStatus(binary.LittleEndian.Uint32(value))
	} else if value, ok := tlv.Value(tlvs, imsaTLVIMSRegistered); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI IMSA registration status: registered TLV length %d, want 1", len(value))
		}
		r.Status.RegistrationKnown = true
		if value[0] != 0 {
			r.Status.Registration = IMSRegistrationStatusRegistered
		} else {
			r.Status.Registration = IMSRegistrationStatusNotRegistered
		}
	}
	if value, ok := tlv.Value(tlvs, imsaTLVFailureCode); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI IMSA registration status: failure code TLV length %d, want 2", len(value))
		}
		r.Status.FailureCodeKnown = true
		r.Status.FailureCode = binary.LittleEndian.Uint16(value)
	}
	if value, ok := tlv.Value(tlvs, imsaTLVRegistrationError); ok {
		if len(value) > imsaRegistrationErrorMaxLength {
			return fmt.Errorf("parsing QMI IMSA registration status: error message length %d exceeds maximum %d", len(value), imsaRegistrationErrorMaxLength)
		}
		r.Status.RegistrationErrorMessage = string(value)
		r.Status.RegistrationErrorMessageKnown = true
	}
	if err := decodeIMSARAT(tlvs, imsaTLVRegistrationRAT, &r.Status.RegistrationRAT, &r.Status.RegistrationRATKnown); err != nil {
		return err
	}
	return nil
}

// IMSARegistrationStatusIndication is a parsed registration-status change.
type IMSARegistrationStatusIndication struct {
	Status IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA Registration Status Changed indication TLVs.
func (i *IMSARegistrationStatusIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = IMSARegistrationStatusIndication{}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI IMSA registration indication: status TLV length %d, want 4", len(value))
		}
		i.Status.Registration = IMSRegistrationStatus(binary.LittleEndian.Uint32(value))
		i.Status.RegistrationKnown = true
	} else if value, ok := tlv.Value(tlvs, 0x01); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI IMSA registration indication: registered TLV length %d, want 1", len(value))
		}
		i.Status.RegistrationKnown = true
		if value[0] != 0 {
			i.Status.Registration = IMSRegistrationStatusRegistered
		} else {
			i.Status.Registration = IMSRegistrationStatusNotRegistered
		}
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI IMSA registration indication: failure code TLV length %d, want 2", len(value))
		}
		i.Status.FailureCode = binary.LittleEndian.Uint16(value)
		i.Status.FailureCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) > imsaRegistrationErrorMaxLength {
			return fmt.Errorf("parsing QMI IMSA registration indication: error message length %d exceeds maximum %d", len(value), imsaRegistrationErrorMaxLength)
		}
		i.Status.RegistrationErrorMessage = string(value)
		i.Status.RegistrationErrorMessageKnown = true
	}
	if err := decodeIMSARAT(tlvs, 0x13, &i.Status.RegistrationRAT, &i.Status.RegistrationRATKnown); err != nil {
		return err
	}
	return nil
}

// IMSAServiceStatusResponse is the parsed QMI IMSA service status.
type IMSAServiceStatusResponse struct {
	Status IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA VoIP service fields.
func (r *IMSAServiceStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var status IMSAStatus
	if err := status.unmarshalServiceTLVs(tlvs); err != nil {
		return err
	}
	*r = IMSAServiceStatusResponse{Status: status}
	return nil
}

// IMSAServiceStatusIndication is a parsed service-status change.
type IMSAServiceStatusIndication struct {
	Status IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA Services Status Changed indication TLVs.
func (i *IMSAServiceStatusIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var status IMSAStatus
	if err := status.unmarshalServiceTLVs(tlvs); err != nil {
		return err
	}
	*i = IMSAServiceStatusIndication{Status: status}
	return nil
}

// IMSARegisterIndications updates IMSA status-indication subscriptions.
func (c *Client) IMSARegisterIndications(ctx context.Context, config IMSAIndicationConfig) error {
	req := IMSARegisterIndicationsRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.imsaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI IMSA indications: %w", err)
	}
	return nil
}

// IMSABindSubscription binds the IMSA client to one modem subscription.
func (c *Client) IMSABindSubscription(ctx context.Context, subscription IMSASubscription) error {
	req, err := (IMSABindRequest{Timeout: DefaultRequestTimeout, Subscription: subscription}).Request()
	if err != nil {
		return fmt.Errorf("binding QMI IMSA subscription: %w", err)
	}
	if err := c.imsaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("binding QMI IMSA subscription: %w", err)
	}
	return nil
}

// IMSABoundSubscription returns the subscription bound to the IMSA client.
func (c *Client) IMSABoundSubscription(ctx context.Context) (IMSAGetBindResponse, error) {
	var result IMSAGetBindResponse
	req := IMSAGetBindRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.imsaRequest(ctx, req, &result); err != nil {
		return IMSAGetBindResponse{}, fmt.Errorf("querying QMI IMSA bound subscription: %w", err)
	}
	return result, nil
}

// WatchIMSARegistrationStatus subscribes to IMS registration changes.
func (c *Client) WatchIMSARegistrationStatus(ctx context.Context) (<-chan IMSAStatus, error) {
	raw, err := c.watchIMSATLVs(ctx, MessageIMSARegistrationChanged, imsaIndicationRegistrationStatus)
	if err != nil {
		return nil, err
	}
	indications := unmarshalTLVStream[IMSARegistrationStatusIndication](ctx, raw)
	out := make(chan IMSAStatus, 8)
	go func() {
		defer close(out)
		for indication := range indications {
			select {
			case out <- indication.Status:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// WatchIMSAServiceStatus subscribes to IMS per-service status changes.
func (c *Client) WatchIMSAServiceStatus(ctx context.Context) (<-chan IMSAStatus, error) {
	raw, err := c.watchIMSATLVs(ctx, MessageIMSAServiceStatusChanged, imsaIndicationServiceStatus)
	if err != nil {
		return nil, err
	}
	indications := unmarshalTLVStream[IMSAServiceStatusIndication](ctx, raw)
	out := make(chan IMSAStatus, 8)
	go func() {
		defer close(out)
		for indication := range indications {
			select {
			case out <- indication.Status:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) watchIMSATLVs(
	ctx context.Context,
	messageID MessageID,
	registration imsaIndicationRegistration,
) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMSA status: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServiceIMSA)
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMSA status: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceIMSA, clientID, messageID)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI IMSA status: subscribe: %w", err)
	}
	if err := c.acquireIMSAIndication(ctx, registration); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI IMSA status: %w", err)
	}

	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseIMSAIndication(registration)
		for indication := range indications {
			select {
			case out <- indication.TLVs:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) acquireIMSAIndication(ctx context.Context, registration imsaIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.imsaIndicationRefs == nil {
		c.imsaIndicationRefs = make(map[imsaIndicationRegistration]int)
	}
	if c.imsaIndicationRefs[registration] > 0 {
		c.imsaIndicationRefs[registration]++
		return nil
	}
	c.imsaIndicationRefs[registration] = 1
	if err := c.setIMSAIndicationRegistration(ctx, registration, true); err != nil {
		delete(c.imsaIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseIMSAIndication(registration imsaIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.imsaIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.imsaIndicationRefs[registration]--
		return
	}
	delete(c.imsaIndicationRefs, registration)
	_ = c.setIMSAIndicationRegistration(ctx, registration, false)
}

func (c *Client) setIMSAIndicationRegistration(ctx context.Context, registration imsaIndicationRegistration, enabled bool) error {
	switch registration {
	case imsaIndicationRegistrationStatus:
		return c.IMSARegisterIndications(ctx, IMSAIndicationConfig{RegistrationStatus: &enabled})
	case imsaIndicationServiceStatus:
		return c.IMSARegisterIndications(ctx, IMSAIndicationConfig{ServiceStatus: &enabled})
	default:
		return fmt.Errorf("configuring QMI IMSA indications: registration %d is unknown", registration)
	}
}

func (c *Client) imsaRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceIMSA, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceIMSA, clientID, req.MessageID, req.TLVs, req.Timeout)
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

func (status *IMSAStatus) unmarshalServiceTLVs(tlvs tlv.TLVs) error {
	serviceFields := []struct {
		typ   uint8
		value *IMSServiceStatus
		known *bool
	}{
		{imsaTLVSMSService, &status.SMSService, &status.SMSServiceKnown},
		{imsaTLVVoIPService, &status.VoIPService, &status.VoIPServiceKnown},
		{imsaTLVVTService, &status.VTService, &status.VTServiceKnown},
		{imsaTLVUTService, &status.UTService, &status.UTServiceKnown},
		{imsaTLVVSService, &status.VSService, &status.VSServiceKnown},
	}
	for _, field := range serviceFields {
		if err := decodeIMSAService(tlvs, field.typ, field.value, field.known); err != nil {
			return err
		}
	}
	ratFields := []struct {
		typ   uint8
		value *IMSServiceRAT
		known *bool
	}{
		{imsaTLVSMSRAT, &status.SMSRAT, &status.SMSRATKnown},
		{imsaTLVVoIPRAT, &status.VoIPRAT, &status.VoIPRATKnown},
		{imsaTLVVTRAT, &status.VTRAT, &status.VTRATKnown},
		{imsaTLVUTRAT, &status.UTRAT, &status.UTRATKnown},
		{imsaTLVVSRAT, &status.VSRAT, &status.VSRATKnown},
	}
	for _, field := range ratFields {
		if err := decodeIMSARAT(tlvs, field.typ, field.value, field.known); err != nil {
			return err
		}
	}
	return nil
}

func decodeIMSAService(tlvs tlv.TLVs, typ uint8, value *IMSServiceStatus, known *bool) error {
	wire, ok := tlv.Value(tlvs, typ)
	if !ok {
		return nil
	}
	if len(wire) != 4 {
		return fmt.Errorf("parsing QMI IMSA TLV 0x%02X: length %d, want 4", typ, len(wire))
	}
	*value = IMSServiceStatus(binary.LittleEndian.Uint32(wire))
	*known = true
	return nil
}

func decodeIMSARAT(tlvs tlv.TLVs, typ uint8, value *IMSServiceRAT, known *bool) error {
	wire, ok := tlv.Value(tlvs, typ)
	if !ok {
		return nil
	}
	if len(wire) != 4 {
		return fmt.Errorf("parsing QMI IMSA RAT TLV 0x%02X: length %d, want 4", typ, len(wire))
	}
	*value = IMSServiceRAT(binary.LittleEndian.Uint32(wire))
	*known = true
	return nil
}

func mergeIMSAServiceStatus(dst *IMSAStatus, src IMSAStatus) {
	dst.SMSService, dst.SMSServiceKnown = src.SMSService, src.SMSServiceKnown
	dst.SMSRAT, dst.SMSRATKnown = src.SMSRAT, src.SMSRATKnown
	dst.VoIPService, dst.VoIPServiceKnown = src.VoIPService, src.VoIPServiceKnown
	dst.VoIPRAT, dst.VoIPRATKnown = src.VoIPRAT, src.VoIPRATKnown
	dst.VTService, dst.VTServiceKnown = src.VTService, src.VTServiceKnown
	dst.VTRAT, dst.VTRATKnown = src.VTRAT, src.VTRATKnown
	dst.UTService, dst.UTServiceKnown = src.UTService, src.UTServiceKnown
	dst.UTRAT, dst.UTRATKnown = src.UTRAT, src.UTRATKnown
	dst.VSService, dst.VSServiceKnown = src.VSService, src.VSServiceKnown
	dst.VSRAT, dst.VSRATKnown = src.VSRAT, src.VSRATKnown
}

func validIMSASubscription(subscription IMSASubscription) bool {
	return subscription <= IMSASubscriptionTertiary
}
