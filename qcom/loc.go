package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const locClientStringMaxLength = 4

// LOCEventRegistration is a mask selecting location-engine indications.
type LOCEventRegistration uint64

const (
	LOCEventPositionReport               LOCEventRegistration = 1 << 0
	LOCEventGNSSSatelliteInfo            LOCEventRegistration = 1 << 1
	LOCEventNMEA                         LOCEventRegistration = 1 << 2
	LOCEventNINotifyVerifyRequest        LOCEventRegistration = 1 << 3
	LOCEventInjectTimeRequest            LOCEventRegistration = 1 << 4
	LOCEventInjectPredictedOrbitsRequest LOCEventRegistration = 1 << 5
	LOCEventInjectPositionRequest        LOCEventRegistration = 1 << 6
	LOCEventEngineState                  LOCEventRegistration = 1 << 7
	LOCEventFixSessionState              LOCEventRegistration = 1 << 8
	LOCEventWiFiRequest                  LOCEventRegistration = 1 << 9
	LOCEventSensorStreamingReady         LOCEventRegistration = 1 << 10
	LOCEventTimeSyncRequest              LOCEventRegistration = 1 << 11
	LOCEventSPIStreamingReport           LOCEventRegistration = 1 << 12
	LOCEventServerConnectionRequest      LOCEventRegistration = 1 << 13
	LOCEventNIGeofenceNotification       LOCEventRegistration = 1 << 14
	LOCEventGeofenceGeneralAlert         LOCEventRegistration = 1 << 15
	LOCEventGeofenceBreachNotification   LOCEventRegistration = 1 << 16
	LOCEventPedometerControl             LOCEventRegistration = 1 << 17
	LOCEventMotionDataControl            LOCEventRegistration = 1 << 18
)

// LOCClientType identifies a client registering with the location engine.
type LOCClientType uint32

const (
	LOCClientApplicationFramework LOCClientType = iota + 1
	LOCClientNonFramework
	LOCClientPrivileged
)

// LOCFixRecurrence selects single or periodic position fixes.
type LOCFixRecurrence uint32

const (
	LOCFixPeriodic LOCFixRecurrence = iota + 1
	LOCFixSingle
)

// LOCIntermediateReportState controls intermediate position reports.
type LOCIntermediateReportState uint32

const (
	LOCIntermediateReportUnknown LOCIntermediateReportState = iota
	LOCIntermediateReportEnabled
	LOCIntermediateReportDisabled
)

// LOCIndicationStatus is the asynchronous completion status returned by LOC.
type LOCIndicationStatus uint32

const (
	LOCIndicationSuccess LOCIndicationStatus = iota
	LOCIndicationGeneralFailure
	LOCIndicationUnsupported
	LOCIndicationInvalidParameter
	LOCIndicationEngineBusy
	LOCIndicationPhoneOffline
	LOCIndicationTimeout
)

// Error implements error for non-success LOC indication statuses.
func (s LOCIndicationStatus) Error() string {
	text := map[LOCIndicationStatus]string{
		LOCIndicationGeneralFailure:   "general failure",
		LOCIndicationUnsupported:      "unsupported",
		LOCIndicationInvalidParameter: "invalid parameter",
		LOCIndicationEngineBusy:       "engine busy",
		LOCIndicationPhoneOffline:     "phone offline",
		LOCIndicationTimeout:          "timeout",
	}
	if description, ok := text[s]; ok {
		return "QMI LOC indication: " + description
	}
	return fmt.Sprintf("QMI LOC indication status %d", s)
}

// LOCOperationMode selects how the location engine computes positions.
type LOCOperationMode uint32

const (
	LOCOperationModeDefault LOCOperationMode = iota + 1
	LOCOperationModeMSBased
	LOCOperationModeMSAssisted
	LOCOperationModeStandalone
	LOCOperationModeCellID
	LOCOperationModeWWAN
)

// LOCNMEAType is a mask selecting NMEA sentence families.
type LOCNMEAType uint32

const (
	LOCNMEAGGA         LOCNMEAType = 1 << 0
	LOCNMEARMC         LOCNMEAType = 1 << 1
	LOCNMEAGSV         LOCNMEAType = 1 << 2
	LOCNMEAGSA         LOCNMEAType = 1 << 3
	LOCNMEAVTG         LOCNMEAType = 1 << 4
	LOCNMEAPQXFI       LOCNMEAType = 1 << 5
	LOCNMEAPSTIS       LOCNMEAType = 1 << 6
	LOCNMEAGLGSV       LOCNMEAType = 1 << 7
	LOCNMEAGNGSA       LOCNMEAType = 1 << 8
	LOCNMEAGNGNS       LOCNMEAType = 1 << 9
	LOCNMEAGARMC       LOCNMEAType = 1 << 10
	LOCNMEAGAGSV       LOCNMEAType = 1 << 11
	LOCNMEAGAGSA       LOCNMEAType = 1 << 12
	LOCNMEAGAVTG       LOCNMEAType = 1 << 13
	LOCNMEAGAGGA       LOCNMEAType = 1 << 14
	LOCNMEAPQGSA       LOCNMEAType = 1 << 15
	LOCNMEAPQGSV       LOCNMEAType = 1 << 16
	LOCNMEADebug       LOCNMEAType = 1 << 17
	LOCNMEAGPDTM       LOCNMEAType = 1 << 18
	LOCNMEAGNGGA       LOCNMEAType = 1 << 19
	LOCNMEAGNRMC       LOCNMEAType = 1 << 20
	LOCNMEAGNVTG       LOCNMEAType = 1 << 21
	LOCNMEAGAGNS       LOCNMEAType = 1 << 22
	LOCNMEAGBGGA       LOCNMEAType = 1 << 23
	LOCNMEAGBGSA       LOCNMEAType = 1 << 24
	LOCNMEAGBGSV       LOCNMEAType = 1 << 25
	LOCNMEAGBRMC       LOCNMEAType = 1 << 26
	LOCNMEAGBVTG       LOCNMEAType = 1 << 27
	LOCNMEAGQGSV       LOCNMEAType = 1 << 28
	LOCNMEAGIGSV       LOCNMEAType = 1 << 29
	LOCNMEAGNDTM       LOCNMEAType = 1 << 30
	LOCNMEAGSATagBlock LOCNMEAType = 1 << 31

	LOCNMEAAll LOCNMEAType = 0x7FFDFFFF
)

// LOCRegisterEventsConfig configures a LOC control point. Mask is combined
// with active watcher registrations before it is sent to the modem.
type LOCRegisterEventsConfig struct {
	Mask                           LOCEventRegistration
	ClientString                   *string
	ClientType                     *LOCClientType
	PositioningRequestNotification *bool
}

// LOCStartConfig describes one positioning session.
type LOCStartConfig struct {
	SessionID                         uint8
	FixRecurrence                     *LOCFixRecurrence
	IntermediateReports               *LOCIntermediateReportState
	MinimumReportIntervalMilliseconds *uint32
}

// LOCRegisterEventsRequest encodes QMI LOC Register Events.
type LOCRegisterEventsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCRegisterEventsConfig
}

// Request converts the event registration into LOC TLVs.
func (r LOCRegisterEventsRequest) Request() (Request, error) {
	tlvs := tlv.TLVs{tlv.Bytes(0x01, binary.LittleEndian.AppendUint64(nil, uint64(r.Config.Mask)))}
	if r.Config.ClientString != nil {
		if len(*r.Config.ClientString) > locClientStringMaxLength {
			return Request{}, fmt.Errorf("encoding QMI LOC event registration: client string length %d exceeds %d", len(*r.Config.ClientString), locClientStringMaxLength)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte(*r.Config.ClientString)))
	}
	if r.Config.ClientType != nil {
		if *r.Config.ClientType < LOCClientApplicationFramework || *r.Config.ClientType > LOCClientPrivileged {
			return Request{}, fmt.Errorf("encoding QMI LOC event registration: client type %d is out of range", *r.Config.ClientType)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint32(*r.Config.ClientType)))
	}
	if r.Config.PositioningRequestNotification != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*r.Config.PositioningRequestNotification)))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCRegisterEvents,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCStartRequest encodes QMI LOC Start.
type LOCStartRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        LOCStartConfig
}

// Request converts the positioning-session settings into LOC TLVs.
func (r LOCStartRequest) Request() (Request, error) {
	tlvs := tlv.TLVs{tlv.Uint(0x01, r.Config.SessionID)}
	if r.Config.FixRecurrence != nil {
		if *r.Config.FixRecurrence < LOCFixPeriodic || *r.Config.FixRecurrence > LOCFixSingle {
			return Request{}, fmt.Errorf("encoding QMI LOC start: fix recurrence %d is out of range", *r.Config.FixRecurrence)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.Config.FixRecurrence)))
	}
	if r.Config.IntermediateReports != nil {
		if *r.Config.IntermediateReports > LOCIntermediateReportDisabled {
			return Request{}, fmt.Errorf("encoding QMI LOC start: intermediate report state %d is out of range", *r.Config.IntermediateReports)
		}
		tlvs = append(tlvs, tlv.Uint(0x12, uint32(*r.Config.IntermediateReports)))
	}
	if r.Config.MinimumReportIntervalMilliseconds != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, *r.Config.MinimumReportIntervalMilliseconds))
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCStart,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// LOCStopRequest encodes QMI LOC Stop.
type LOCStopRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	SessionID     uint8
}

// Request converts the session ID into a LOC request.
func (r LOCStopRequest) Request() Request {
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCStop,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, r.SessionID)},
	}
}

// LOCSetNMEATypesRequest encodes QMI LOC Set NMEA Types.
type LOCSetNMEATypesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Types         LOCNMEAType
}

// Request converts the NMEA mask into a LOC request.
func (r LOCSetNMEATypesRequest) Request() Request {
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCSetNMEATypes,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Types))},
	}
}

// LOCGetNMEATypesRequest encodes QMI LOC Get NMEA Types.
type LOCGetNMEATypesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a LOC request.
func (r LOCGetNMEATypesRequest) Request() Request {
	return locEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageLOCGetNMEATypes)
}

// LOCSetOperationModeRequest encodes QMI LOC Set Operation Mode.
type LOCSetOperationModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mode          LOCOperationMode
}

// Request converts the operation mode into a LOC request.
func (r LOCSetOperationModeRequest) Request() (Request, error) {
	if err := validateLOCOperationMode(r.Mode); err != nil {
		return Request{}, fmt.Errorf("encoding QMI LOC operation mode: %w", err)
	}
	return Request{
		Service:       ServiceLOC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageLOCSetOperationMode,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Mode))},
	}, nil
}

// LOCGetOperationModeRequest encodes QMI LOC Get Operation Mode.
type LOCGetOperationModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a LOC request.
func (r LOCGetOperationModeRequest) Request() Request {
	return locEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageLOCGetOperationMode)
}

// LOCIndicationResult is the common asynchronous LOC completion status.
type LOCIndicationResult struct {
	Status LOCIndicationStatus
}

// Err returns the modem-reported asynchronous operation error.
func (r LOCIndicationResult) Err() error {
	if r.Status == LOCIndicationSuccess {
		return nil
	}
	return r.Status
}

// LOCOperationIndication contains a status-only LOC completion.
type LOCOperationIndication struct {
	Result LOCIndicationResult
}

// UnmarshalTLVs parses a status-only LOC completion indication.
func (i *LOCOperationIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCOperationIndication{}
	return i.Result.UnmarshalTLVs(tlvs)
}

// LOCNMEAIndication contains one raw NMEA sentence.
type LOCNMEAIndication struct {
	Sentence string
}

// UnmarshalTLVs parses a QMI LOC NMEA indication.
func (i *LOCNMEAIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCNMEAIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI LOC NMEA: sentence TLV missing")
	}
	i.Sentence = string(value)
	return nil
}

// LOCNMEATypesIndication is the asynchronous Get NMEA Types result.
type LOCNMEATypesIndication struct {
	Result     LOCIndicationResult
	Types      LOCNMEAType
	TypesKnown bool
}

// UnmarshalTLVs parses a QMI LOC Get NMEA Types indication.
func (i *LOCNMEATypesIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCNMEATypesIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI LOC NMEA types: types TLV length %d, want 4", len(value))
		}
		i.Types = LOCNMEAType(binary.LittleEndian.Uint32(value))
		i.TypesKnown = true
	}
	return nil
}

// LOCOperationModeIndication is the asynchronous Get Operation Mode result.
type LOCOperationModeIndication struct {
	Result    LOCIndicationResult
	Mode      LOCOperationMode
	ModeKnown bool
}

// UnmarshalTLVs parses a QMI LOC Get Operation Mode indication.
func (i *LOCOperationModeIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = LOCOperationModeIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI LOC operation mode: mode TLV length %d, want 4", len(value))
		}
		i.Mode = LOCOperationMode(binary.LittleEndian.Uint32(value))
		i.ModeKnown = true
	}
	return nil
}

// LOCRegisterEvents updates the caller-owned event mask while preserving bits
// required by active watchers.
func (c *Client) LOCRegisterEvents(ctx context.Context, config LOCRegisterEventsConfig) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()

	baseMask := config.Mask
	config.Mask |= combinedLOCEventMask(c.locEventRefs)
	if err := c.locRegisterEvents(ctx, config); err != nil {
		return fmt.Errorf("registering QMI LOC events: %w", err)
	}
	c.locEventMask = baseMask
	return nil
}

// LOCStart starts a location tracking session.
func (c *Client) LOCStart(ctx context.Context, config LOCStartConfig) error {
	req, err := (LOCStartRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.locRequest(ctx, req); err != nil {
		return fmt.Errorf("starting QMI LOC session %d: %w", config.SessionID, err)
	}
	return nil
}

// LOCStop stops a location tracking session.
func (c *Client) LOCStop(ctx context.Context, sessionID uint8) error {
	req := LOCStopRequest{Timeout: DefaultRequestTimeout, SessionID: sessionID}.Request()
	if err := c.locRequest(ctx, req); err != nil {
		return fmt.Errorf("stopping QMI LOC session %d: %w", sessionID, err)
	}
	return nil
}

// LOCSetNMEATypes selects the NMEA sentence families emitted by the engine.
func (c *Client) LOCSetNMEATypes(ctx context.Context, types LOCNMEAType) error {
	req := LOCSetNMEATypesRequest{Timeout: DefaultRequestTimeout, Types: types}.Request()
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("setting QMI LOC NMEA types: %w", err)
	}
	return nil
}

// LOCNMEATypes returns the NMEA sentence families enabled in the engine.
func (c *Client) LOCNMEATypes(ctx context.Context) (LOCNMEAType, error) {
	req := LOCGetNMEATypesRequest{Timeout: DefaultRequestTimeout}.Request()
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("querying QMI LOC NMEA types: %w", err)
	}
	var indication LOCNMEATypesIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return 0, err
	}
	if err := indication.Result.Err(); err != nil {
		return 0, fmt.Errorf("querying QMI LOC NMEA types: %w", err)
	}
	if !indication.TypesKnown {
		return 0, errors.New("querying QMI LOC NMEA types: types TLV missing")
	}
	return indication.Types, nil
}

// LOCSetOperationMode selects the engine's positioning mode.
func (c *Client) LOCSetOperationMode(ctx context.Context, mode LOCOperationMode) error {
	req, err := (LOCSetOperationModeRequest{Timeout: DefaultRequestTimeout, Mode: mode}).Request()
	if err != nil {
		return err
	}
	if err := c.locOperation(ctx, req); err != nil {
		return fmt.Errorf("setting QMI LOC operation mode: %w", err)
	}
	return nil
}

// LOCOperationMode returns the engine's positioning mode.
func (c *Client) LOCOperationMode(ctx context.Context) (LOCOperationMode, error) {
	req := LOCGetOperationModeRequest{Timeout: DefaultRequestTimeout}.Request()
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("querying QMI LOC operation mode: %w", err)
	}
	var indication LOCOperationModeIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return 0, err
	}
	if err := indication.Result.Err(); err != nil {
		return 0, fmt.Errorf("querying QMI LOC operation mode: %w", err)
	}
	if !indication.ModeKnown {
		return 0, errors.New("querying QMI LOC operation mode: mode TLV missing")
	}
	return indication.Mode, nil
}

// LOCWatchNMEA subscribes to raw NMEA sentences. Starting and stopping the
// positioning session remains under the caller's control.
func (c *Client) LOCWatchNMEA(ctx context.Context) (<-chan string, error) {
	raw, err := c.watchLOCTLVs(ctx, MessageLOCNMEA, LOCEventNMEA)
	if err != nil {
		return nil, fmt.Errorf("watching QMI LOC NMEA: %w", err)
	}
	out := make(chan string, 16)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var nmea LOCNMEAIndication
			if err := nmea.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- nmea.Sentence:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) locRequest(ctx context.Context, req Request) error {
	return c.withServiceClient(ctx, ServiceLOC, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceLOC, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

func (c *Client) locOperation(ctx context.Context, req Request) error {
	indicationTLVs, err := c.locSingleIndication(ctx, req)
	if err != nil {
		return err
	}
	var indication LOCOperationIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return err
	}
	return indication.Result.Err()
}

func (c *Client) locSingleIndication(ctx context.Context, req Request) (tlv.TLVs, error) {
	c.locMu.Lock()
	defer c.locMu.Unlock()

	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceLOC)
	if err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	indications, err := transport.Indications(waitCtx, ServiceLOC, clientID, req.MessageID)
	if err != nil {
		return nil, fmt.Errorf("subscribing QMI LOC result: %w", err)
	}
	resp, err := c.requestServiceWithTimeout(ctx, ServiceLOC, clientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return nil, err
	}
	if err := resultOK(resp); err != nil {
		return nil, err
	}
	select {
	case indication, ok := <-indications:
		if !ok {
			return nil, errors.New("QMI LOC indication stream closed")
		}
		return indication.TLVs, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) locRegisterEvents(ctx context.Context, config LOCRegisterEventsConfig) error {
	req, err := (LOCRegisterEventsRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	return c.locRequest(ctx, req)
}

func (c *Client) acquireLOCEvents(ctx context.Context, mask LOCEventRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()

	if c.locEventRefs == nil {
		c.locEventRefs = make(map[LOCEventRegistration]int)
	}
	oldMask := c.locEventMask | combinedLOCEventMask(c.locEventRefs)
	c.locEventRefs[mask]++
	newMask := c.locEventMask | combinedLOCEventMask(c.locEventRefs)
	if oldMask == newMask {
		return nil
	}
	if err := c.locRegisterEvents(ctx, LOCRegisterEventsConfig{Mask: newMask}); err != nil {
		c.locEventRefs[mask]--
		if c.locEventRefs[mask] == 0 {
			delete(c.locEventRefs, mask)
		}
		return err
	}
	return nil
}

func (c *Client) releaseLOCEvents(mask LOCEventRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.locEventRefs[mask] == 0 {
		return
	}
	oldMask := c.locEventMask | combinedLOCEventMask(c.locEventRefs)
	c.locEventRefs[mask]--
	if c.locEventRefs[mask] == 0 {
		delete(c.locEventRefs, mask)
	}
	newMask := c.locEventMask | combinedLOCEventMask(c.locEventRefs)
	if oldMask != newMask {
		_ = c.locRegisterEvents(ctx, LOCRegisterEventsConfig{Mask: newMask})
	}
}

func combinedLOCEventMask(refs map[LOCEventRegistration]int) LOCEventRegistration {
	var mask LOCEventRegistration
	for registration, count := range refs {
		if count > 0 {
			mask |= registration
		}
	}
	return mask
}

func locEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServiceLOC,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func validateLOCOperationMode(mode LOCOperationMode) error {
	if mode < LOCOperationModeDefault || mode > LOCOperationModeWWAN {
		return fmt.Errorf("operation mode %d is out of range", mode)
	}
	return nil
}

// UnmarshalTLVs parses the common QMI LOC indication result.
func (r *LOCIndicationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = LOCIndicationResult{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI LOC indication: status TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI LOC indication: status TLV length %d, want 4", len(value))
	}
	r.Status = LOCIndicationStatus(binary.LittleEndian.Uint32(value))
	return nil
}
