package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// OMASessionType identifies an OMA device-management session.
type OMASessionType uint8

const (
	OMASessionClientDeviceConfigure OMASessionType = iota
	OMASessionClientPRLUpdate
	OMASessionClientHandsFreeActivation
	OMASessionDeviceHandsFreeActivation
	OMASessionNetworkPRLUpdate
	OMASessionNetworkDeviceConfigure
	OMASessionDevicePRLUpdate
)

// OMASessionState describes the progress of an OMA session.
type OMASessionState uint8

const (
	OMASessionCompleteInformationUpdated OMASessionState = iota
	OMASessionCompleteInformationUnavailable
	OMASessionFailed
	OMASessionRetrying
	OMASessionConnecting
	OMASessionConnected
	OMASessionAuthenticated
	OMASessionMDNDownloaded
	OMASessionMSIDDownloaded
	OMASessionPRLDownloaded
	OMASessionMIPProfileDownloaded
)

// OMASessionFailureReason explains why an OMA session failed.
type OMASessionFailureReason uint8

const (
	OMASessionFailureUnknown OMASessionFailureReason = iota
	OMASessionFailureNetworkUnavailable
	OMASessionFailureServerUnavailable
	OMASessionFailureAuthentication
	OMASessionFailureMaximumRetries
	OMASessionFailureCancelled
)

// OMAHFAFeatureDoneState reports the result of hands-free activation.
type OMAHFAFeatureDoneState uint8

const (
	OMAHFAFeatureNotFinished OMAHFAFeatureDoneState = iota
	OMAHFAFeatureSucceeded
	OMAHFAFeatureFailed
)

// OMAEventReportConfig updates selected OMA event-report settings. Nil fields
// are omitted so callers do not overwrite settings they do not own.
type OMAEventReportConfig struct {
	NetworkInitiatedAlerts *bool
	SessionState           *bool
}

// OMANetworkInitiatedAlert identifies a pending network-initiated session.
type OMANetworkInitiatedAlert struct {
	SessionType OMASessionType
	SessionID   uint16
}

// OMAEvent is one OMA Event Report indication.
type OMAEvent struct {
	NetworkInitiatedAlert      OMANetworkInitiatedAlert
	NetworkInitiatedAlertKnown bool
	SessionState               OMASessionState
	SessionStateKnown          bool
	FailureReason              OMASessionFailureReason
	FailureReasonKnown         bool
}

// OMARetryInfo contains the retry counters returned for a retrying session.
// Qualcomm's published protocol does not specify a unit for the timer fields.
type OMARetryInfo struct {
	Count          uint8
	PauseTimer     uint16
	PauseRemaining uint16
}

// OMASessionInfo describes the current session, or the previous session when
// no session is active.
type OMASessionInfo struct {
	State                      OMASessionState
	Type                       OMASessionType
	FailureReason              OMASessionFailureReason
	FailureReasonKnown         bool
	Retry                      OMARetryInfo
	RetryKnown                 bool
	NetworkInitiatedAlert      OMANetworkInitiatedAlert
	NetworkInitiatedAlertKnown bool
}

// OMAFeatureSettings contains the modem's OMA feature configuration.
type OMAFeatureSettings struct {
	DeviceProvisioning      bool
	DeviceProvisioningKnown bool
	PRLUpdate               bool
	PRLUpdateKnown          bool
	HFA                     bool
	HFAKnown                bool
	HFADoneState            OMAHFAFeatureDoneState
	HFADoneStateKnown       bool
}

// OMAFeatureSettingsUpdate updates selected OMA features. Nil fields are
// omitted.
type OMAFeatureSettingsUpdate struct {
	DeviceProvisioning *bool
	PRLUpdate          *bool
	HFA                *bool
}

// OMAResetRequest encodes QMI OMA Reset.
type OMAResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r OMAResetRequest) Request() Request {
	return omaEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageOMAReset)
}

// OMASetEventReportRequest encodes QMI OMA Set Event Report.
type OMASetEventReportRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        OMAEventReportConfig
}

// Request converts the event settings into QMI TLVs.
func (r OMASetEventReportRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config.NetworkInitiatedAlerts != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*r.Config.NetworkInitiatedAlerts)))
	}
	if r.Config.SessionState != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*r.Config.SessionState)))
	}
	return Request{
		Service:       ServiceOMA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageOMASetEventReport,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// OMAStartSessionRequest encodes QMI OMA Start Session.
type OMAStartSessionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	SessionType   OMASessionType
}

// Request validates and converts the session start into a QMI request.
func (r OMAStartSessionRequest) Request() (Request, error) {
	if err := validateOMASessionType(r.SessionType); err != nil {
		return Request{}, fmt.Errorf("encoding QMI OMA session start: %w", err)
	}
	return Request{
		Service:       ServiceOMA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageOMAStartSession,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x10, uint8(r.SessionType))},
	}, nil
}

// OMACancelSessionRequest encodes QMI OMA Cancel Session.
type OMACancelSessionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the cancellation into a QMI request.
func (r OMACancelSessionRequest) Request() Request {
	return omaEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageOMACancelSession)
}

// OMAGetSessionInfoRequest encodes QMI OMA Get Session Info.
type OMAGetSessionInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r OMAGetSessionInfoRequest) Request() Request {
	return omaEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageOMAGetSessionInfo)
}

// OMASendSelectionRequest encodes QMI OMA Send Selection.
type OMASendSelectionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Accept        bool
	SessionID     uint16
}

// Request converts the network-initiated alert decision into a QMI request.
func (r OMASendSelectionRequest) Request() Request {
	value := []byte{boolByte(r.Accept)}
	value = binary.LittleEndian.AppendUint16(value, r.SessionID)
	return Request{
		Service:       ServiceOMA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageOMASendSelection,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x10, value)},
	}
}

// OMAGetFeatureSettingsRequest encodes QMI OMA Get Feature Setting.
type OMAGetFeatureSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r OMAGetFeatureSettingsRequest) Request() Request {
	return omaEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageOMAGetFeatureSetting)
}

// OMASetFeatureSettingsRequest encodes QMI OMA Set Feature Setting.
type OMASetFeatureSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Update        OMAFeatureSettingsUpdate
}

// Request converts selected feature settings into QMI TLVs.
func (r OMASetFeatureSettingsRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Update.DeviceProvisioning != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*r.Update.DeviceProvisioning)))
	}
	if r.Update.PRLUpdate != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*r.Update.PRLUpdate)))
	}
	if r.Update.HFA != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*r.Update.HFA)))
	}
	return Request{
		Service:       ServiceOMA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageOMASetFeatureSetting,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// UnmarshalTLVs parses a QMI OMA Event Report indication.
func (e *OMAEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*e = OMAEvent{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if err := e.NetworkInitiatedAlert.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI OMA event network-initiated alert: %w", err)
		}
		e.NetworkInitiatedAlertKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI OMA event: session state TLV length %d, want 1", len(value))
		}
		e.SessionState = OMASessionState(value[0])
		e.SessionStateKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI OMA event: failure reason TLV length %d, want 1", len(value))
		}
		e.FailureReason = OMASessionFailureReason(value[0])
		e.FailureReasonKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI OMA Get Session Info output.
func (i *OMASessionInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = OMASessionInfo{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI OMA session information: session info TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI OMA session information: session info TLV length %d, want 2", len(value))
	}
	i.State = OMASessionState(value[0])
	i.Type = OMASessionType(value[1])

	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI OMA session information: failure reason TLV length %d, want 1", len(value))
		}
		i.FailureReason = OMASessionFailureReason(value[0])
		i.FailureReasonKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI OMA session information: retry info TLV length %d, want 5", len(value))
		}
		i.Retry = OMARetryInfo{
			Count:          value[0],
			PauseTimer:     binary.LittleEndian.Uint16(value[1:3]),
			PauseRemaining: binary.LittleEndian.Uint16(value[3:5]),
		}
		i.RetryKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if err := i.NetworkInitiatedAlert.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI OMA session information network-initiated alert: %w", err)
		}
		i.NetworkInitiatedAlertKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI OMA Get Feature Setting output.
func (s *OMAFeatureSettings) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = OMAFeatureSettings{}
	settings := []struct {
		kind  byte
		value *bool
		known *bool
	}{
		{kind: 0x10, value: &s.DeviceProvisioning, known: &s.DeviceProvisioningKnown},
		{kind: 0x11, value: &s.PRLUpdate, known: &s.PRLUpdateKnown},
		{kind: 0x12, value: &s.HFA, known: &s.HFAKnown},
	}
	for _, setting := range settings {
		value, ok := tlv.Value(tlvs, setting.kind)
		if !ok {
			continue
		}
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI OMA feature settings: TLV 0x%02X length %d, want 1", setting.kind, len(value))
		}
		*setting.value = value[0] != 0
		*setting.known = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI OMA feature settings: HFA done state TLV length %d, want 1", len(value))
		}
		s.HFADoneState = OMAHFAFeatureDoneState(value[0])
		s.HFADoneStateKnown = true
	}
	return nil
}

// OMAReset resets the OMA service state.
func (c *Client) OMAReset(ctx context.Context) error {
	if err := c.omaRequest(ctx, (OMAResetRequest{Timeout: DefaultRequestTimeout}).Request(), nil); err != nil {
		return fmt.Errorf("resetting QMI OMA service: %w", err)
	}
	return nil
}

// OMASetEventReport updates selected OMA event-report settings.
func (c *Client) OMASetEventReport(ctx context.Context, config OMAEventReportConfig) error {
	req := (OMASetEventReportRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err := c.omaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI OMA event report: %w", err)
	}
	return nil
}

// OMAStartSession starts an OMA device-management session.
func (c *Client) OMAStartSession(ctx context.Context, sessionType OMASessionType) error {
	req, err := (OMAStartSessionRequest{Timeout: DefaultRequestTimeout, SessionType: sessionType}).Request()
	if err != nil {
		return err
	}
	if err := c.omaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("starting QMI OMA session: %w", err)
	}
	return nil
}

// OMACancelSession cancels the active OMA session.
func (c *Client) OMACancelSession(ctx context.Context) error {
	if err := c.omaRequest(ctx, (OMACancelSessionRequest{Timeout: DefaultRequestTimeout}).Request(), nil); err != nil {
		return fmt.Errorf("cancelling QMI OMA session: %w", err)
	}
	return nil
}

// OMASessionInfo reads the current or previous OMA session information.
func (c *Client) OMASessionInfo(ctx context.Context) (OMASessionInfo, error) {
	var info OMASessionInfo
	req := (OMAGetSessionInfoRequest{Timeout: DefaultRequestTimeout}).Request()
	if err := c.omaRequest(ctx, req, &info); err != nil {
		return OMASessionInfo{}, fmt.Errorf("reading QMI OMA session information: %w", err)
	}
	return info, nil
}

// OMASendSelection accepts or rejects a network-initiated OMA alert.
func (c *Client) OMASendSelection(ctx context.Context, sessionID uint16, accept bool) error {
	req := (OMASendSelectionRequest{
		Timeout:   DefaultRequestTimeout,
		Accept:    accept,
		SessionID: sessionID,
	}).Request()
	if err := c.omaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("sending QMI OMA selection: %w", err)
	}
	return nil
}

// OMAFeatureSettings reads the modem's OMA feature configuration.
func (c *Client) OMAFeatureSettings(ctx context.Context) (OMAFeatureSettings, error) {
	var settings OMAFeatureSettings
	req := (OMAGetFeatureSettingsRequest{Timeout: DefaultRequestTimeout}).Request()
	if err := c.omaRequest(ctx, req, &settings); err != nil {
		return OMAFeatureSettings{}, fmt.Errorf("reading QMI OMA feature settings: %w", err)
	}
	return settings, nil
}

// OMASetFeatureSettings updates selected OMA features.
func (c *Client) OMASetFeatureSettings(ctx context.Context, update OMAFeatureSettingsUpdate) error {
	req := (OMASetFeatureSettingsRequest{Timeout: DefaultRequestTimeout, Update: update}).Request()
	if err := c.omaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI OMA feature settings: %w", err)
	}
	return nil
}

// OMAWatchEvents subscribes to network-initiated alerts and session-state
// changes. Canceling ctx releases the shared modem registration.
func (c *Client) OMAWatchEvents(ctx context.Context) (<-chan OMAEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceOMA)
	if err != nil {
		return nil, fmt.Errorf("watching QMI OMA events: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceOMA, clientID, MessageOMAEventReport)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI OMA events: %w", err)
	}
	if err := c.acquireOMAEvents(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI OMA events: %w", err)
	}

	out := make(chan OMAEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseOMAEvents()
		for indication := range indications {
			var event OMAEvent
			if err := event.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- event:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) omaRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceOMA, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceOMA, clientID, req.MessageID, req.TLVs, req.Timeout)
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

func (c *Client) acquireOMAEvents(ctx context.Context) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.omaEventRefs > 0 {
		c.omaEventRefs++
		return nil
	}
	if err := c.setOMAEvents(ctx, true); err != nil {
		return err
	}
	c.omaEventRefs = 1
	return nil
}

func (c *Client) releaseOMAEvents() {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.omaEventRefs == 0 {
		return
	}
	if c.omaEventRefs > 1 {
		c.omaEventRefs--
		return
	}
	c.omaEventRefs = 0
	_ = c.setOMAEvents(ctx, false)
}

func (c *Client) setOMAEvents(ctx context.Context, enabled bool) error {
	return c.OMASetEventReport(ctx, OMAEventReportConfig{
		NetworkInitiatedAlerts: &enabled,
		SessionState:           &enabled,
	})
}

func omaEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServiceOMA,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func validateOMASessionType(sessionType OMASessionType) error {
	if sessionType > OMASessionDevicePRLUpdate {
		return fmt.Errorf("session type %d is out of range", sessionType)
	}
	return nil
}

func (alert OMANetworkInitiatedAlert) MarshalBinary() ([]byte, error) {
	value := []byte{byte(alert.SessionType)}
	return binary.LittleEndian.AppendUint16(value, alert.SessionID), nil
}

func (alert *OMANetworkInitiatedAlert) UnmarshalBinary(value []byte) error {
	if len(value) != 3 {
		return fmt.Errorf("network-initiated alert length %d, want 3", len(value))
	}
	*alert = OMANetworkInitiatedAlert{
		SessionType: OMASessionType(value[0]),
		SessionID:   binary.LittleEndian.Uint16(value[1:3]),
	}
	return nil
}
