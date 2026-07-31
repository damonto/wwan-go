package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	pdsNMEAMaxLength = 200
	pdsURLMaxLength  = 255
)

// PDSOperatingMode identifies how the modem computes a position. Unknown is
// valid in event reports but cannot be configured as a tracking-session mode.
type PDSOperatingMode int8

const (
	PDSOperatingModeUnknown    PDSOperatingMode = -1
	PDSOperatingModeStandalone PDSOperatingMode = 0
	PDSOperatingModeMSBased    PDSOperatingMode = 1
	PDSOperatingModeMSAssisted PDSOperatingMode = 2
)

// PDSPositionSessionStatus identifies the result or progress of a positioning
// session.
type PDSPositionSessionStatus uint8

const (
	PDSPositionSessionSuccess PDSPositionSessionStatus = iota
	PDSPositionSessionInProgress
	PDSPositionSessionGeneralFailure
	PDSPositionSessionTimeout
	PDSPositionSessionUserEnded
	PDSPositionSessionBadParameter
	PDSPositionSessionPhoneOffline
	PDSPositionSessionEngineLocked
	PDSPositionSessionEmergencyCall
)

// PDSTrackingSessionState identifies whether a tracking session is active.
type PDSTrackingSessionState uint8

const (
	PDSTrackingSessionUnknown PDSTrackingSessionState = iota
	PDSTrackingSessionInactive
	PDSTrackingSessionActive
)

// PDSNetworkMode selects the network family used for A-GPS setup.
type PDSNetworkMode uint8

const (
	PDSNetworkModeUMTS PDSNetworkMode = iota
	PDSNetworkModeCDMA
)

// PDSEventReportConfig updates selected PDS event-report settings. Nil fields
// are omitted so callers do not overwrite settings they do not own.
type PDSEventReportConfig struct {
	NMEAPosition                    *bool
	ExtendedNMEAPosition            *bool
	ParsedPosition                  *bool
	ExternalXTRADataRequest         *bool
	ExternalTimeInjectionRequest    *bool
	ExternalWiFiPositionRequest     *bool
	SatelliteInformation            *bool
	VXNetworkInitiatedRequest       *bool
	SUPLNetworkInitiatedPrompt      *bool
	UMTSCPNetworkInitiatedPrompt    *bool
	CommunicationEvent              *bool
	AccelerometerStreamingReady     *bool
	GyroStreamingReady              *bool
	TimeSyncRequest                 *bool
	PositionReliability             *bool
	SensorDataUsage                 *bool
	TimeSourceInformation           *bool
	HeadingUncertainty              *bool
	NMEADebugStrings                *bool
	ExtendedExternalXTRADataRequest *bool
}

// PDSExtendedNMEA contains an NMEA sentence and the positioning mode that
// produced it.
type PDSExtendedNMEA struct {
	OperatingMode PDSOperatingMode
	Sentence      string
}

// PDSEvent contains the supported fields from one PDS Event Report
// indication. Known fields distinguish an omitted TLV from a zero value.
type PDSEvent struct {
	NMEA               string
	NMEAKnown          bool
	ExtendedNMEA       PDSExtendedNMEA
	ExtendedNMEAKnown  bool
	SessionStatus      PDSPositionSessionStatus
	SessionStatusKnown bool
}

// PDSGPSServiceState contains the GPS engine and tracking-session states.
type PDSGPSServiceState struct {
	Enabled         bool
	TrackingSession PDSTrackingSessionState
}

// PDSDefaultTrackingSession contains the modem's default positioning
// parameters. The wire interval is seconds; older Qualcomm API documentation
// incorrectly described it as milliseconds.
type PDSDefaultTrackingSession struct {
	OperatingMode          PDSOperatingMode
	PositionTimeoutSeconds uint8
	IntervalSeconds        uint32
	AccuracyMeters         uint32
}

// PDSAGPSServer identifies an IPv4 A-GPS server endpoint.
type PDSAGPSServer struct {
	Address netip.Addr
	Port    uint32
}

// PDSAGPSConfig contains the A-GPS server forms returned by the modem. The
// modem may return an address, a URL, or both.
type PDSAGPSConfig struct {
	Server *PDSAGPSServer
	URL    *string
}

// PDSAGPSConfigUpdate selects A-GPS fields to update. Nil fields are omitted.
type PDSAGPSConfigUpdate struct {
	Server      *PDSAGPSServer
	URL         *string
	NetworkMode *PDSNetworkMode
}

// PDSResetRequest encodes QMI PDS Reset.
type PDSResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r PDSResetRequest) Request() Request {
	return pdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePDSReset)
}

// PDSSetEventReportRequest encodes QMI PDS Set Event Report.
type PDSSetEventReportRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDSEventReportConfig
}

// Request converts the event settings into PDS TLVs.
func (r PDSSetEventReportRequest) Request() Request {
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSSetEventReport,
		Timeout:       r.Timeout,
		TLVs:          pdsEventReportTLVs(r.Config),
	}
}

// PDSGetGPSServiceStateRequest encodes QMI PDS Get GPS Service State.
type PDSGetGPSServiceStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a PDS request.
func (r PDSGetGPSServiceStateRequest) Request() Request {
	return pdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePDSGetGPSServiceState)
}

// PDSSetGPSServiceStateRequest encodes QMI PDS Set GPS Service State.
type PDSSetGPSServiceStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the desired GPS engine state into a PDS request.
func (r PDSSetGPSServiceStateRequest) Request() Request {
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSSetGPSServiceState,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, boolByte(r.Enabled))},
	}
}

// PDSGetDefaultTrackingSessionRequest encodes QMI PDS Get Default Tracking
// Session.
type PDSGetDefaultTrackingSessionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a PDS request.
func (r PDSGetDefaultTrackingSessionRequest) Request() Request {
	return pdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePDSGetDefaultTrackingSession)
}

// PDSSetDefaultTrackingSessionRequest encodes QMI PDS Set Default Tracking
// Session.
type PDSSetDefaultTrackingSessionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Session       PDSDefaultTrackingSession
}

// Request converts the default tracking parameters into a PDS request.
func (r PDSSetDefaultTrackingSessionRequest) Request() (Request, error) {
	if err := validatePDSTrackingOperatingMode(r.Session.OperatingMode); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDS default tracking session: %w", err)
	}
	value := []byte{byte(r.Session.OperatingMode), r.Session.PositionTimeoutSeconds}
	value = binary.LittleEndian.AppendUint32(value, r.Session.IntervalSeconds)
	value = binary.LittleEndian.AppendUint32(value, r.Session.AccuracyMeters)
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSSetDefaultTrackingSession,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// PDSGetAGPSConfigRequest encodes QMI PDS Get AGPS Config.
type PDSGetAGPSConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	NetworkMode   PDSNetworkMode
}

// Request converts the network selection into a PDS request.
func (r PDSGetAGPSConfigRequest) Request() (Request, error) {
	if err := validatePDSNetworkMode(r.NetworkMode); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDS A-GPS configuration query: %w", err)
	}
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSGetAGPSConfig,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x12, uint8(r.NetworkMode))},
	}, nil
}

// PDSSetAGPSConfigRequest encodes QMI PDS Set AGPS Config.
type PDSSetAGPSConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDSAGPSConfigUpdate
}

// Request converts selected A-GPS fields into a PDS request.
func (r PDSSetAGPSConfigRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	if r.Config.Server != nil {
		value, err := r.Config.Server.MarshalBinary()
		if err != nil {
			return Request{}, fmt.Errorf("encoding QMI PDS A-GPS configuration: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if r.Config.URL != nil {
		if len(*r.Config.URL) > pdsURLMaxLength {
			return Request{}, fmt.Errorf("encoding QMI PDS A-GPS configuration: URL length %d exceeds %d", len(*r.Config.URL), pdsURLMaxLength)
		}
		value := append([]byte{byte(len(*r.Config.URL))}, *r.Config.URL...)
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}
	if r.Config.NetworkMode != nil {
		if err := validatePDSNetworkMode(*r.Config.NetworkMode); err != nil {
			return Request{}, fmt.Errorf("encoding QMI PDS A-GPS configuration: %w", err)
		}
		tlvs = append(tlvs, tlv.Uint(0x14, uint8(*r.Config.NetworkMode)))
	}
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSSetAGPSConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDSGetAutoTrackingStateRequest encodes QMI PDS Get Auto Tracking State.
type PDSGetAutoTrackingStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a PDS request.
func (r PDSGetAutoTrackingStateRequest) Request() Request {
	return pdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessagePDSGetAutoTrackingState)
}

// PDSSetAutoTrackingStateRequest encodes QMI PDS Set Auto Tracking State.
type PDSSetAutoTrackingStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the desired automatic tracking state into a PDS request.
func (r PDSSetAutoTrackingStateRequest) Request() Request {
	return Request{
		Service:       ServicePDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDSSetAutoTrackingState,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, boolByte(r.Enabled))},
	}
}

// UnmarshalTLVs parses a QMI PDS Event Report indication.
func (e *PDSEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*e = PDSEvent{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) > pdsNMEAMaxLength {
			return fmt.Errorf("parsing QMI PDS event: NMEA length %d exceeds %d", len(value), pdsNMEAMaxLength)
		}
		e.NMEA = string(value)
		e.NMEAKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 3 {
			return errors.New("parsing QMI PDS event: extended NMEA TLV is truncated")
		}
		length := int(binary.LittleEndian.Uint16(value[1:3]))
		if length > pdsNMEAMaxLength {
			return fmt.Errorf("parsing QMI PDS event: extended NMEA length %d exceeds %d", length, pdsNMEAMaxLength)
		}
		if len(value) != 3+length {
			return fmt.Errorf("parsing QMI PDS event: extended NMEA TLV length %d, want %d", len(value), 3+length)
		}
		e.ExtendedNMEA = PDSExtendedNMEA{
			OperatingMode: PDSOperatingMode(int8(value[0])),
			Sentence:      string(value[3:]),
		}
		e.ExtendedNMEAKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI PDS event: session status TLV length %d, want 1", len(value))
		}
		e.SessionStatus = PDSPositionSessionStatus(value[0])
		e.SessionStatusKnown = true
	}
	return nil
}

// PDSGetGPSServiceStateResponse is the parsed GPS service state response.
type PDSGetGPSServiceStateResponse struct {
	State PDSGPSServiceState
}

// UnmarshalTLVs parses QMI PDS Get GPS Service State response TLVs.
func (r *PDSGetGPSServiceStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PDSGetGPSServiceStateResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDS GPS service state: state TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI PDS GPS service state: state TLV length %d, want 2", len(value))
	}
	r.State = PDSGPSServiceState{
		Enabled:         value[0] != 0,
		TrackingSession: PDSTrackingSessionState(value[1]),
	}
	return nil
}

// PDSGetDefaultTrackingSessionResponse is the parsed default tracking
// session response.
type PDSGetDefaultTrackingSessionResponse struct {
	Session PDSDefaultTrackingSession
}

// UnmarshalTLVs parses QMI PDS Get Default Tracking Session response TLVs.
func (r *PDSGetDefaultTrackingSessionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PDSGetDefaultTrackingSessionResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDS default tracking session: info TLV missing")
	}
	if len(value) != 10 {
		return fmt.Errorf("parsing QMI PDS default tracking session: info TLV length %d, want 10", len(value))
	}
	r.Session = PDSDefaultTrackingSession{
		OperatingMode:          PDSOperatingMode(int8(value[0])),
		PositionTimeoutSeconds: value[1],
		IntervalSeconds:        binary.LittleEndian.Uint32(value[2:6]),
		AccuracyMeters:         binary.LittleEndian.Uint32(value[6:10]),
	}
	return nil
}

// PDSGetAGPSConfigResponse is the parsed A-GPS configuration response.
type PDSGetAGPSConfigResponse struct {
	Config PDSAGPSConfig
}

// UnmarshalTLVs parses QMI PDS Get AGPS Config response TLVs.
func (r *PDSGetAGPSConfigResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PDSGetAGPSConfigResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		var server PDSAGPSServer
		if err := server.UnmarshalBinary(value); err != nil {
			return err
		}
		r.Config.Server = &server
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		url, err := decodePDSURL(value)
		if err != nil {
			return err
		}
		r.Config.URL = &url
	}
	return nil
}

// PDSGetAutoTrackingStateResponse is the parsed automatic tracking state.
type PDSGetAutoTrackingStateResponse struct {
	Enabled bool
}

// UnmarshalTLVs parses QMI PDS Get Auto Tracking State response TLVs.
func (r *PDSGetAutoTrackingStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PDSGetAutoTrackingStateResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDS automatic tracking state: state TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI PDS automatic tracking state: state TLV length %d, want 1", len(value))
	}
	r.Enabled = value[0] != 0
	return nil
}

// PDSSetEventReport configures PDS event indications.
func (c *Client) PDSSetEventReport(ctx context.Context, config PDSEventReportConfig) error {
	req := PDSSetEventReportRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI PDS event reports: %w", err)
	}
	return nil
}

// PDSReset resets this control point's PDS service state.
func (c *Client) PDSReset(ctx context.Context) error {
	req := PDSResetRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("resetting QMI PDS control point: %w", err)
	}
	return nil
}

// PDSGPSServiceState returns the GPS engine and tracking-session states.
func (c *Client) PDSGPSServiceState(ctx context.Context) (PDSGPSServiceState, error) {
	req := PDSGetGPSServiceStateRequest{Timeout: DefaultRequestTimeout}.Request()
	var response PDSGetGPSServiceStateResponse
	if err := c.pdsRequest(ctx, req, &response); err != nil {
		return PDSGPSServiceState{}, fmt.Errorf("querying QMI PDS GPS service state: %w", err)
	}
	return response.State, nil
}

// PDSSetGPSServiceState enables or disables the GPS engine.
func (c *Client) PDSSetGPSServiceState(ctx context.Context, enabled bool) error {
	req := PDSSetGPSServiceStateRequest{Timeout: DefaultRequestTimeout, Enabled: enabled}.Request()
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI PDS GPS service state: %w", err)
	}
	return nil
}

// PDSDefaultTrackingSession returns the modem's default positioning
// parameters.
func (c *Client) PDSDefaultTrackingSession(ctx context.Context) (PDSDefaultTrackingSession, error) {
	req := PDSGetDefaultTrackingSessionRequest{Timeout: DefaultRequestTimeout}.Request()
	var response PDSGetDefaultTrackingSessionResponse
	if err := c.pdsRequest(ctx, req, &response); err != nil {
		return PDSDefaultTrackingSession{}, fmt.Errorf("querying QMI PDS default tracking session: %w", err)
	}
	return response.Session, nil
}

// PDSSetDefaultTrackingSession updates the modem's default positioning
// parameters.
func (c *Client) PDSSetDefaultTrackingSession(ctx context.Context, session PDSDefaultTrackingSession) error {
	req, err := (PDSSetDefaultTrackingSessionRequest{
		Timeout: DefaultRequestTimeout,
		Session: session,
	}).Request()
	if err != nil {
		return err
	}
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI PDS default tracking session: %w", err)
	}
	return nil
}

// PDSAGPSConfig returns the A-GPS server configuration for a network mode.
func (c *Client) PDSAGPSConfig(ctx context.Context, networkMode PDSNetworkMode) (PDSAGPSConfig, error) {
	req, err := (PDSGetAGPSConfigRequest{
		Timeout:     DefaultRequestTimeout,
		NetworkMode: networkMode,
	}).Request()
	if err != nil {
		return PDSAGPSConfig{}, err
	}
	var response PDSGetAGPSConfigResponse
	if err := c.pdsRequest(ctx, req, &response); err != nil {
		return PDSAGPSConfig{}, fmt.Errorf("querying QMI PDS A-GPS configuration: %w", err)
	}
	return response.Config, nil
}

// PDSSetAGPSConfig updates selected A-GPS server fields.
func (c *Client) PDSSetAGPSConfig(ctx context.Context, config PDSAGPSConfigUpdate) error {
	req, err := (PDSSetAGPSConfigRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI PDS A-GPS configuration: %w", err)
	}
	return nil
}

// PDSAutoTrackingState returns whether automatic tracking is enabled.
func (c *Client) PDSAutoTrackingState(ctx context.Context) (bool, error) {
	req := PDSGetAutoTrackingStateRequest{Timeout: DefaultRequestTimeout}.Request()
	var response PDSGetAutoTrackingStateResponse
	if err := c.pdsRequest(ctx, req, &response); err != nil {
		return false, fmt.Errorf("querying QMI PDS automatic tracking state: %w", err)
	}
	return response.Enabled, nil
}

// PDSSetAutoTrackingState enables or disables automatic tracking.
func (c *Client) PDSSetAutoTrackingState(ctx context.Context, enabled bool) error {
	req := PDSSetAutoTrackingStateRequest{Timeout: DefaultRequestTimeout, Enabled: enabled}.Request()
	if err := c.pdsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI PDS automatic tracking state: %w", err)
	}
	return nil
}

// PDSWatchEvents subscribes to NMEA, extended NMEA, and positioning-session
// status events. Canceling ctx releases the shared modem registration.
func (c *Client) PDSWatchEvents(ctx context.Context) (<-chan PDSEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI PDS events: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServicePDS, clientID, MessagePDSEventReport)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDS events: %w", err)
	}
	if err := c.acquirePDSEvents(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDS events: %w", err)
	}

	out := make(chan PDSEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releasePDSEvents()
		for indication := range indications {
			var event PDSEvent
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

// PDSWatchGPSReady subscribes to the legacy empty GPS Ready indication.
func (c *Client) PDSWatchGPSReady(ctx context.Context) (<-chan struct{}, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI PDS GPS readiness: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServicePDS, clientID, MessagePDSGPSReady)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDS GPS readiness: %w", err)
	}
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		defer cancel()
		for range indications {
			select {
			case out <- struct{}{}:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) pdsRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServicePDS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServicePDS, clientID, req.MessageID, req.TLVs, req.Timeout)
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

func (c *Client) acquirePDSEvents(ctx context.Context) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.pdsEventRefs > 0 {
		c.pdsEventRefs++
		return nil
	}
	if err := c.setPDSEvents(ctx, true); err != nil {
		return err
	}
	c.pdsEventRefs = 1
	return nil
}

func (c *Client) releasePDSEvents() {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.pdsEventRefs == 0 {
		return
	}
	if c.pdsEventRefs > 1 {
		c.pdsEventRefs--
		return
	}
	c.pdsEventRefs = 0
	_ = c.setPDSEvents(ctx, false)
}

func (c *Client) setPDSEvents(ctx context.Context, enabled bool) error {
	return c.PDSSetEventReport(ctx, PDSEventReportConfig{
		NMEAPosition:         &enabled,
		ExtendedNMEAPosition: &enabled,
	})
}

func pdsEventReportTLVs(config PDSEventReportConfig) tlv.TLVs {
	settings := []struct {
		kind    byte
		enabled *bool
	}{
		{kind: 0x10, enabled: config.NMEAPosition},
		{kind: 0x11, enabled: config.ExtendedNMEAPosition},
		{kind: 0x12, enabled: config.ParsedPosition},
		{kind: 0x13, enabled: config.ExternalXTRADataRequest},
		{kind: 0x14, enabled: config.ExternalTimeInjectionRequest},
		{kind: 0x15, enabled: config.ExternalWiFiPositionRequest},
		{kind: 0x16, enabled: config.SatelliteInformation},
		{kind: 0x17, enabled: config.VXNetworkInitiatedRequest},
		{kind: 0x18, enabled: config.SUPLNetworkInitiatedPrompt},
		{kind: 0x19, enabled: config.UMTSCPNetworkInitiatedPrompt},
		{kind: 0x1A, enabled: config.CommunicationEvent},
		{kind: 0x1B, enabled: config.AccelerometerStreamingReady},
		{kind: 0x1C, enabled: config.GyroStreamingReady},
		{kind: 0x1D, enabled: config.TimeSyncRequest},
		{kind: 0x1E, enabled: config.PositionReliability},
		{kind: 0x1F, enabled: config.SensorDataUsage},
		{kind: 0x20, enabled: config.TimeSourceInformation},
		{kind: 0x21, enabled: config.HeadingUncertainty},
		{kind: 0x22, enabled: config.NMEADebugStrings},
		{kind: 0x23, enabled: config.ExtendedExternalXTRADataRequest},
	}
	tlvs := make(tlv.TLVs, 0, len(settings))
	for _, setting := range settings {
		if setting.enabled != nil {
			tlvs = append(tlvs, tlv.Uint(setting.kind, boolByte(*setting.enabled)))
		}
	}
	return tlvs
}

func pdsEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServicePDS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func validatePDSTrackingOperatingMode(mode PDSOperatingMode) error {
	switch mode {
	case PDSOperatingModeStandalone, PDSOperatingModeMSBased, PDSOperatingModeMSAssisted:
		return nil
	default:
		return fmt.Errorf("operating mode %d is out of range", mode)
	}
}

func validatePDSNetworkMode(mode PDSNetworkMode) error {
	if mode > PDSNetworkModeCDMA {
		return fmt.Errorf("network mode %d is out of range", mode)
	}
	return nil
}

// MarshalBinary encodes a QMI PDS A-GPS server.
func (server PDSAGPSServer) MarshalBinary() ([]byte, error) {
	address := server.Address.Unmap()
	if !address.Is4() {
		return nil, fmt.Errorf("A-GPS server address %q is not IPv4", server.Address)
	}
	addressBytes := address.As4()
	value := append([]byte(nil), addressBytes[:]...)
	value = binary.LittleEndian.AppendUint32(value, server.Port)
	return value, nil
}

// UnmarshalBinary decodes a QMI PDS A-GPS server.
func (server *PDSAGPSServer) UnmarshalBinary(value []byte) error {
	if len(value) != 8 {
		return fmt.Errorf("parsing QMI PDS A-GPS configuration: server TLV length %d, want 8", len(value))
	}
	*server = PDSAGPSServer{
		Address: netip.AddrFrom4([4]byte(value[:4])),
		Port:    binary.LittleEndian.Uint32(value[4:8]),
	}
	return nil
}

func decodePDSURL(value []byte) (string, error) {
	if len(value) < 1 {
		return "", errors.New("parsing QMI PDS A-GPS configuration: URL length is missing")
	}
	length := int(value[0])
	if len(value) != 1+length {
		return "", fmt.Errorf("parsing QMI PDS A-GPS configuration: URL TLV length %d, want %d", len(value), 1+length)
	}
	return string(value[1:]), nil
}
