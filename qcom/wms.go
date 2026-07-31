package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wmsMessageLengthMax = 255

type wmsIndicationRegistration uint8

const (
	wmsIndicationNone wmsIndicationRegistration = iota
	wmsIndicationMT
	wmsIndicationTransportLayer
	wmsIndicationTransportNetwork
	wmsIndicationServiceReady
	wmsIndicationSMSCAddress
	wmsIndicationMemoryFull
	wmsIndicationMessageWaiting
	wmsIndicationCallStatus
	wmsIndicationBroadcastConfig
	wmsIndicationTransportMWI
)

// WMSStorage identifies a modem SMS store.
type WMSStorage uint8

const (
	WMSStorageUIM WMSStorage = iota
	WMSStorageNV
)

// WMSMessageFormat identifies the over-the-air representation of an SMS.
type WMSMessageFormat uint8

const (
	WMSMessageFormatCDMA           WMSMessageFormat = 0x00
	WMSMessageFormatGWPointToPoint WMSMessageFormat = 0x06
	WMSMessageFormatGWBroadcast    WMSMessageFormat = 0x07
	WMSMessageFormatMWI            WMSMessageFormat = 0x08
)

// WMSMessageMode selects the CDMA or 3GPP messaging domain.
type WMSMessageMode uint8

const (
	WMSMessageModeCDMA WMSMessageMode = iota
	WMSMessageModeGW
)

// WMSTag describes the read and delivery state of a stored SMS.
type WMSTag uint8

const (
	WMSTagMTRead WMSTag = iota
	WMSTagMTNotRead
	WMSTagMOSent
	WMSTagMONotSent
)

// WMSMessageProtocol identifies the network protocol used for an SMS ACK.
type WMSMessageProtocol uint8

const (
	WMSMessageProtocolCDMA WMSMessageProtocol = iota
	WMSMessageProtocolWCDMA
)

// WMSAckIndicator reports whether a transfer-route SMS requires an ACK.
type WMSAckIndicator uint8

const (
	WMSAckRequired WMSAckIndicator = iota
	WMSAckNotRequired
)

// WMSCDMAServiceOption selects the CDMA service option used for SMS delivery.
type WMSCDMAServiceOption uint8

const (
	WMSCDMAServiceOptionAuto WMSCDMAServiceOption = 0x00
	WMSCDMAServiceOption6    WMSCDMAServiceOption = 0x06
	WMSCDMAServiceOption14   WMSCDMAServiceOption = 0x0E
)

// WMSCDMAForceOnDC controls forced CDMA SMS delivery on a dedicated channel.
type WMSCDMAForceOnDC struct {
	Force         bool
	ServiceOption WMSCDMAServiceOption
}

// WMSGSMCauseInfo contains the RP and TP causes returned by a 3GPP network.
type WMSGSMCauseInfo struct {
	RPCause uint16
	TPCause uint8
}

// WMSDeliveryFailureType classifies a temporary or permanent send failure.
type WMSDeliveryFailureType uint8

const (
	WMSDeliveryFailureTemporary WMSDeliveryFailureType = iota
	WMSDeliveryFailurePermanent
)

// WMSDeliveryFailureCause identifies the condition that blocked SMS delivery.
type WMSDeliveryFailureCause uint8

const (
	WMSDeliveryBlockedByCallControl WMSDeliveryFailureCause = iota
	WMSDeliveryBlockedByVoiceOrDataCall
)

// WMSRejectCause contains a lower-layer reject type and cause value.
type WMSRejectCause struct {
	Type  uint32
	Value uint8
}

// WMSAck3GPP2Failure contains the CDMA failure information sent in a negative ACK.
type WMSAck3GPP2Failure struct {
	ErrorClass uint8
	CauseCode  uint8
}

// WMSAck3GPPFailure contains the GSM/UMTS failure information sent in a negative ACK.
type WMSAck3GPPFailure struct {
	RPCause uint8
	TPCause uint8
}

// WMSAckFailureCause explains why the modem could not deliver a network ACK.
type WMSAckFailureCause uint8

const (
	WMSAckFailureNoNetworkResponse WMSAckFailureCause = iota
	WMSAckFailureNetworkReleasedLink
	WMSAckFailureNotSent
)

// WMSAckError preserves the QMI error and the optional modem ACK failure cause.
type WMSAckError struct {
	Err               error
	FailureCause      WMSAckFailureCause
	FailureCauseKnown bool
}

func (e *WMSAckError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.FailureCauseKnown {
		return fmt.Sprintf("%v: ACK failure cause %d", e.Err, e.FailureCause)
	}
	return e.Err.Error()
}

func (e *WMSAckError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WMSMessageReference identifies one SMS stored by the modem.
type WMSMessageReference struct {
	Storage WMSStorage
	Index   uint32
}

// WMSRawMessage contains a raw CDMA or 3GPP SMS PDU.
type WMSRawMessage struct {
	Tag    WMSTag
	Format WMSMessageFormat
	Data   []byte
}

// WMSSendOptions controls optional WMS Raw Send fields.
type WMSSendOptions struct {
	ForceOnDC      *WMSCDMAForceOnDC
	SMSOnIMS       *bool
	FollowOnDC     bool
	LinkTimer      *uint8
	RetryMessageID *uint32
	CommandTPDU    *bool
}

// WMSSendResult contains the modem-assigned message ID and optional rejection
// details returned by WMS Raw Send.
type WMSSendResult struct {
	MessageID            uint16
	MessageIDKnown       bool
	CauseCode            uint16
	CauseCodeKnown       bool
	ErrorClass           uint8
	ErrorClassKnown      bool
	GSMCause             WMSGSMCauseInfo
	GSMCauseKnown        bool
	DeliveryFailure      WMSDeliveryFailureType
	DeliveryFailureKnown bool
	DeliveryCause        WMSDeliveryFailureCause
	DeliveryCauseKnown   bool
	CallControlAlphaID   []byte
	CallControlKnown     bool
	RejectCause          WMSRejectCause
	RejectCauseKnown     bool
	IMSRejectCause       uint16
	IMSRejectCauseKnown  bool
}

// WMSWriteRequest stores one raw SMS on the UIM or in modem NV storage.
type WMSWriteRequest struct {
	Storage WMSStorage
	Format  WMSMessageFormat
	Data    []byte
	Tag     *WMSTag
}

// WMSReadRequest selects one stored raw SMS.
type WMSReadRequest struct {
	Reference   WMSMessageReference
	MessageMode *WMSMessageMode
	SMSOnIMS    *bool
}

// WMSDeleteRequest deletes one or more stored messages. A nil Index deletes
// every message matching the remaining selectors.
type WMSDeleteRequest struct {
	Storage     WMSStorage
	Index       *uint32
	Tag         *WMSTag
	MessageMode *WMSMessageMode
}

// WMSListRequest selects messages in one modem store.
type WMSListRequest struct {
	Storage     WMSStorage
	Tag         *WMSTag
	MessageMode *WMSMessageMode
}

// WMSListedMessage is one entry returned by WMS List Messages.
type WMSListedMessage struct {
	Reference WMSMessageReference
	Tag       WMSTag
}

// WMSEventReportConfig selects WMS event-report indications. Nil fields are
// omitted so callers can update only the settings they own.
type WMSEventReportConfig struct {
	MTMessages       *bool
	CallControlInfo  *bool
	MessageWaiting   *bool
	LowerLayerErrors *bool
}

// WMSAckRequest acknowledges a transfer-route SMS.
type WMSAckRequest struct {
	TransactionID uint32
	Protocol      WMSMessageProtocol
	Success       bool
	Failure3GPP2  *WMSAck3GPP2Failure
	Failure3GPP   *WMSAck3GPPFailure
	SMSOnIMS      *bool
}

// WMSETWSNotificationType identifies an ETWS primary or secondary notification.
type WMSETWSNotificationType uint8

const (
	WMSETWSNotificationPrimary WMSETWSNotificationType = iota
	WMSETWSNotificationSecondaryGSM
	WMSETWSNotificationSecondaryUMTS
)

// WMSPLMN identifies the network associated with an ETWS notification.
type WMSPLMN struct {
	MCC uint16
	MNC uint16
}

// WMSIncomingMessage is delivered for WMS Event Report MT-message events.
// Stored messages are read before delivery; ReadError preserves the original
// storage reference when the follow-up read fails.
type WMSIncomingMessage struct {
	Stored           bool
	Reference        WMSMessageReference
	Tag              WMSTag
	Format           WMSMessageFormat
	Data             []byte
	AckIndicator     WMSAckIndicator
	TransactionID    uint32
	SMSOnIMS         bool
	SMSOnIMSKnown    bool
	MessageMode      WMSMessageMode
	MessageModeKnown bool
	ETWSNotification WMSETWSNotificationType
	ETWSData         []byte
	ETWSKnown        bool
	ETWSPLMN         WMSPLMN
	ETWSPLMNKnown    bool
	SMSCAddress      string
	SMSCAddressKnown bool
	ReadError        error
}

// WMSSMSCAddress is the SMS service-center address configured in EF-SMSP.
type WMSSMSCAddress struct {
	Type   string
	Digits string
}

// WMSSetSMSCAddress updates the SMS service-center address. A nil index lets
// the modem select the default EF-SMSP record.
func (c *Client) WMSSetSMSCAddress(ctx context.Context, address WMSSMSCAddress, index *uint8) error {
	if len(address.Digits) > 21 {
		return fmt.Errorf("setting QMI WMS SMSC address: digits length %d exceeds 21", len(address.Digits))
	}
	if strings.IndexByte(address.Digits, 0) >= 0 {
		return errors.New("setting QMI WMS SMSC address: digits contain NUL")
	}
	if len(address.Type) > 3 {
		return fmt.Errorf("setting QMI WMS SMSC address: type length %d exceeds 3", len(address.Type))
	}
	if strings.IndexByte(address.Type, 0) >= 0 {
		return errors.New("setting QMI WMS SMSC address: type contains NUL")
	}

	tlvs := tlv.TLVs{tlv.Bytes(0x01, []byte(address.Digits))}
	if address.Type != "" {
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte(address.Type)))
	}
	if index != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *index))
	}
	if err := c.wmsResultRequest(ctx, MessageWMSSetSMSCAddress, tlvs); err != nil {
		return fmt.Errorf("setting QMI WMS SMSC address: %w", err)
	}
	return nil
}

// WMSSendRaw sends an already encoded CDMA or 3GPP SMS PDU.
func (c *Client) WMSSendRaw(ctx context.Context, format WMSMessageFormat, data []byte, options WMSSendOptions) (WMSSendResult, error) {
	value, err := encodeWMSRawMessage(format, data)
	if err != nil {
		return WMSSendResult{}, err
	}

	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if options.ForceOnDC != nil {
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte{
			boolByte(options.ForceOnDC.Force),
			byte(options.ForceOnDC.ServiceOption),
		}))
	}
	if options.FollowOnDC {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(1)))
	}
	if options.LinkTimer != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, *options.LinkTimer))
	}
	if options.SMSOnIMS != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, boolByte(*options.SMSOnIMS)))
	}
	if options.RetryMessageID != nil {
		tlvs = append(tlvs,
			tlv.Uint(0x14, uint8(1)),
			tlv.Uint(0x15, *options.RetryMessageID),
		)
	}
	if options.CommandTPDU != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, boolByte(*options.CommandTPDU)))
	}

	var parsed wmsRawSendResponse
	err = c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSRawSend, tlvs)
		if err != nil {
			return err
		}
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if !parsed.Result.MessageIDKnown && options.RetryMessageID == nil {
			return errors.New("parsing QMI WMS raw send response: message ID TLV missing")
		}
		return nil
	})
	if err != nil {
		return parsed.Result, fmt.Errorf("sending QMI WMS raw message: %w", err)
	}
	return parsed.Result, nil
}

// WMSWriteRaw writes an already encoded SMS to modem storage.
func (c *Client) WMSWriteRaw(ctx context.Context, req WMSWriteRequest) (WMSMessageReference, error) {
	value, err := encodeWMSStoredRawMessage(req.Storage, req.Format, req.Data)
	if err != nil {
		return WMSMessageReference{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if req.Tag != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.Tag)))
	}

	reference := WMSMessageReference{Storage: req.Storage}
	err = c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSRawWrite, tlvs)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x01)
		if !ok {
			return errors.New("parsing QMI WMS raw write response: storage index TLV missing")
		}
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WMS raw write response: storage index TLV length %d, want 4", len(value))
		}
		reference.Index = binary.LittleEndian.Uint32(value)
		return nil
	})
	if err != nil {
		return WMSMessageReference{}, fmt.Errorf("writing QMI WMS raw message: %w", err)
	}
	return reference, nil
}

// WMSReadRaw reads one SMS from modem storage.
func (c *Client) WMSReadRaw(ctx context.Context, req WMSReadRequest) (WMSRawMessage, error) {
	value := []byte{byte(req.Reference.Storage)}
	value = binary.LittleEndian.AppendUint32(value, req.Reference.Index)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if req.MessageMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.MessageMode)))
	}
	if req.SMSOnIMS != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*req.SMSOnIMS)))
	}

	var message WMSRawMessage
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSRawRead, tlvs)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x01)
		if !ok {
			return errors.New("parsing QMI WMS raw read response: raw message TLV missing")
		}
		return message.UnmarshalBinary(value)
	})
	if err != nil {
		return WMSRawMessage{}, fmt.Errorf("reading QMI WMS raw message: %w", err)
	}
	return message, nil
}

// WMSDelete deletes stored messages matching req.
func (c *Client) WMSDelete(ctx context.Context, req WMSDeleteRequest) error {
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(req.Storage))}
	if req.Index != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *req.Index))
	}
	if req.Tag != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*req.Tag)))
	}
	if req.MessageMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, uint8(*req.MessageMode)))
	}

	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSDelete, tlvs)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("deleting QMI WMS messages: %w", err)
	}
	return nil
}

// WMSListMessages lists stored SMS indices and tags.
func (c *Client) WMSListMessages(ctx context.Context, req WMSListRequest) ([]WMSListedMessage, error) {
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(req.Storage))}
	if req.Tag != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.Tag)))
	}
	if req.MessageMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*req.MessageMode)))
	}

	var messages []WMSListedMessage
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSListMessages, tlvs)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x01)
		if !ok {
			return errors.New("parsing QMI WMS message list: message list TLV missing")
		}
		messages, err = decodeWMSMessageList(req.Storage, value)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listing QMI WMS messages: %w", err)
	}
	return messages, nil
}

// WMSSetEventReport configures WMS event-report indications.
func (c *Client) WMSSetEventReport(ctx context.Context, config WMSEventReportConfig) error {
	var tlvs tlv.TLVs
	if config.MTMessages != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*config.MTMessages)))
	}
	if config.CallControlInfo != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*config.CallControlInfo)))
	}
	if config.MessageWaiting != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*config.MessageWaiting)))
	}
	if config.LowerLayerErrors != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, boolByte(*config.LowerLayerErrors)))
	}

	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSSetEventReport, tlvs)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("configuring QMI WMS event reports: %w", err)
	}
	return nil
}

// WMSAcknowledge sends the network ACK required by a transfer-route message.
func (c *Client) WMSAcknowledge(ctx context.Context, req WMSAckRequest) error {
	// The IDL aggregate order is transaction ID, protocol, success.
	value := binary.LittleEndian.AppendUint32(nil, req.TransactionID)
	value = append(value, byte(req.Protocol), boolByte(req.Success))
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if req.Failure3GPP2 != nil {
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte{req.Failure3GPP2.ErrorClass, req.Failure3GPP2.CauseCode}))
	}
	if req.Failure3GPP != nil {
		tlvs = append(tlvs, tlv.Bytes(0x11, []byte{req.Failure3GPP.RPCause, req.Failure3GPP.TPCause}))
	}
	if req.SMSOnIMS != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*req.SMSOnIMS)))
	}

	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSSendAck, tlvs)
		if err != nil {
			return err
		}
		resultErr := resultOK(resp)
		value, ok := tlv.Value(resp.TLVs, 0x10)
		if ok && len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS ACK response: failure cause TLV length %d, want 1", len(value))
		}
		if resultErr == nil {
			return nil
		}
		ackErr := &WMSAckError{Err: resultErr}
		if ok {
			ackErr.FailureCause = WMSAckFailureCause(value[0])
			ackErr.FailureCauseKnown = true
		}
		return ackErr
	})
	if err != nil {
		return fmt.Errorf("acknowledging QMI WMS message: %w", err)
	}
	return nil
}

// WMSWatchIncoming subscribes to incoming SMS events. Stored messages are read
// before being sent to the returned channel.
func (c *Client) WMSWatchIncoming(ctx context.Context) (<-chan WMSIncomingMessage, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceWMS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS messages: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceWMS, clientID, MessageWMSEventReport)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI WMS messages: %w", err)
	}
	if err := c.acquireWMSIndication(ctx, wmsIndicationMT); err != nil {
		cancel()
		return nil, err
	}

	out := make(chan WMSIncomingMessage, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseWMSIndication(wmsIndicationMT)
		for indication := range indications {
			var message WMSIncomingMessage
			if err := message.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			if message.Stored {
				raw, readErr := c.WMSReadRaw(watchCtx, WMSReadRequest{Reference: message.Reference})
				message.ReadError = readErr
				if readErr == nil {
					message.Tag = raw.Tag
					message.Format = raw.Format
					message.Data = raw.Data
				}
			}
			select {
			case out <- message:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) acquireWMSIndication(ctx context.Context, registration wmsIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.wmsIndicationRefs == nil {
		c.wmsIndicationRefs = make(map[wmsIndicationRegistration]int)
	}
	if c.wmsIndicationRefs[registration] > 0 {
		c.wmsIndicationRefs[registration]++
		return nil
	}
	c.wmsIndicationRefs[registration] = 1
	if err := c.setWMSIndication(ctx, registration, true); err != nil {
		delete(c.wmsIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseWMSIndication(registration wmsIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.wmsIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.wmsIndicationRefs[registration]--
		return
	}
	delete(c.wmsIndicationRefs, registration)
	_ = c.setWMSIndication(ctx, registration, false)
}

func (c *Client) setWMSIndication(ctx context.Context, registration wmsIndicationRegistration, enabled bool) error {
	switch registration {
	case wmsIndicationMT:
		return c.WMSSetEventReport(ctx, WMSEventReportConfig{MTMessages: &enabled})
	case wmsIndicationMessageWaiting:
		return c.WMSSetEventReport(ctx, WMSEventReportConfig{MessageWaiting: &enabled})
	case wmsIndicationTransportLayer:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{TransportLayer: &enabled})
	case wmsIndicationTransportNetwork:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{TransportNetwork: &enabled})
	case wmsIndicationCallStatus:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{CallStatus: &enabled})
	case wmsIndicationServiceReady:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{ServiceReady: &enabled})
	case wmsIndicationBroadcastConfig:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{BroadcastConfig: &enabled})
	case wmsIndicationTransportMWI:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{TransportMWI: &enabled})
	case wmsIndicationSMSCAddress:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{SMSCAddress: &enabled})
	case wmsIndicationMemoryFull:
		return c.WMSSetIndicationRegistration(ctx, WMSIndicationConfig{MemoryFull: &enabled})
	default:
		return fmt.Errorf("setting QMI WMS indication: registration %d is out of range", registration)
	}
}

func encodeWMSRawMessage(format WMSMessageFormat, data []byte) ([]byte, error) {
	if len(data) > wmsMessageLengthMax {
		return nil, fmt.Errorf("encoding QMI WMS raw message: data length %d exceeds %d", len(data), wmsMessageLengthMax)
	}
	value := []byte{byte(format)}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(data)))
	return append(value, data...), nil
}

func encodeWMSStoredRawMessage(storage WMSStorage, format WMSMessageFormat, data []byte) ([]byte, error) {
	raw, err := encodeWMSRawMessage(format, data)
	if err != nil {
		return nil, err
	}
	return append([]byte{byte(storage)}, raw...), nil
}

// MarshalBinary encodes the raw-message aggregate returned by QMI WMS.
func (m WMSRawMessage) MarshalBinary() ([]byte, error) {
	if len(m.Data) > wmsMessageLengthMax {
		return nil, fmt.Errorf("encoding QMI WMS raw message: data length %d exceeds %d", len(m.Data), wmsMessageLengthMax)
	}
	value := []byte{byte(m.Tag), byte(m.Format)}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(m.Data)))
	return append(value, m.Data...), nil
}

// UnmarshalBinary decodes the raw-message aggregate returned by QMI WMS.
func (m *WMSRawMessage) UnmarshalBinary(value []byte) error {
	*m = WMSRawMessage{}
	if len(value) < 4 {
		return errors.New("parsing QMI WMS raw message: value is truncated")
	}
	length := int(binary.LittleEndian.Uint16(value[2:4]))
	if len(value) != 4+length {
		return fmt.Errorf("parsing QMI WMS raw message: value length %d, want %d", len(value), 4+length)
	}
	*m = WMSRawMessage{
		Tag:    WMSTag(value[0]),
		Format: WMSMessageFormat(value[1]),
		Data:   slices.Clone(value[4 : 4+length]),
	}
	return nil
}

func (r *WMSSendResult) unmarshalRawSendTLVs(tlvs tlv.TLVs) error {
	var result WMSSendResult
	if value, ok := tlv.Value(tlvs, 0x01); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS raw send response: message ID TLV length %d, want 2", len(value))
		}
		result.MessageID = binary.LittleEndian.Uint16(value)
		result.MessageIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS raw send response: CDMA cause TLV length %d, want 2", len(value))
		}
		result.CauseCode = binary.LittleEndian.Uint16(value)
		result.CauseCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS raw send response: CDMA error class TLV length %d, want 1", len(value))
		}
		result.ErrorClass = value[0]
		result.ErrorClassKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 3 {
			return fmt.Errorf("parsing QMI WMS raw send response: GSM cause TLV length %d, want 3", len(value))
		}
		result.GSMCause = WMSGSMCauseInfo{RPCause: binary.LittleEndian.Uint16(value[:2]), TPCause: value[2]}
		result.GSMCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS raw send response: delivery failure TLV length %d, want 1", len(value))
		}
		result.DeliveryFailure = WMSDeliveryFailureType(value[0])
		result.DeliveryFailureKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if err := decodeWMSDeliveryCause(&result, value); err != nil {
			return fmt.Errorf("parsing QMI WMS raw send response: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if err := decodeWMSCallControl(&result, value); err != nil {
			return fmt.Errorf("parsing QMI WMS raw send response: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if err := decodeWMSRejectCause(&result, value); err != nil {
			return fmt.Errorf("parsing QMI WMS raw send response: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS raw send response: IMS reject cause TLV length %d, want 2", len(value))
		}
		result.IMSRejectCause = binary.LittleEndian.Uint16(value)
		result.IMSRejectCauseKnown = true
	}
	*r = result
	return nil
}

type wmsRawSendResponse struct {
	Result WMSSendResult
}

func (r *wmsRawSendResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result WMSSendResult
	if err := result.unmarshalRawSendTLVs(tlvs); err != nil {
		return err
	}
	*r = wmsRawSendResponse{Result: result}
	return nil
}

func decodeWMSDeliveryCause(result *WMSSendResult, value []byte) error {
	if len(value) != 1 {
		return fmt.Errorf("delivery failure cause TLV length %d, want 1", len(value))
	}
	cause := WMSDeliveryFailureCause(value[0])
	if cause > WMSDeliveryBlockedByVoiceOrDataCall {
		return fmt.Errorf("delivery failure cause %d is out of range", cause)
	}
	result.DeliveryCause = cause
	result.DeliveryCauseKnown = true
	return nil
}

func decodeWMSCallControl(result *WMSSendResult, value []byte) error {
	if len(value) == 0 {
		return errors.New("call-control alpha ID length is missing")
	}
	length := int(value[0])
	if len(value) != 1+length {
		return fmt.Errorf("call-control alpha ID value length %d, want %d", len(value), 1+length)
	}
	result.CallControlAlphaID = slices.Clone(value[1:])
	result.CallControlKnown = true
	return nil
}

func decodeWMSRejectCause(result *WMSSendResult, value []byte) error {
	if len(value) != 5 {
		return fmt.Errorf("reject cause TLV length %d, want 5", len(value))
	}
	result.RejectCause = WMSRejectCause{
		Type:  binary.LittleEndian.Uint32(value[:4]),
		Value: value[4],
	}
	result.RejectCauseKnown = true
	return nil
}

func decodeWMSMessageList(storage WMSStorage, value []byte) ([]WMSListedMessage, error) {
	if len(value) < 4 {
		return nil, errors.New("parsing QMI WMS message list: count is truncated")
	}
	count := int(binary.LittleEndian.Uint32(value[:4]))
	value = value[4:]
	if len(value) != count*5 {
		return nil, fmt.Errorf("parsing QMI WMS message list: value length %d, want %d", len(value), count*5)
	}
	messages := make([]WMSListedMessage, 0, count)
	for i := range count {
		offset := i * 5
		messages = append(messages, WMSListedMessage{
			Reference: WMSMessageReference{
				Storage: storage,
				Index:   binary.LittleEndian.Uint32(value[offset : offset+4]),
			},
			Tag: WMSTag(value[offset+4]),
		})
	}
	return messages, nil
}

// UnmarshalTLVs parses a WMS event-report MT or ETWS message.
func (m *WMSIncomingMessage) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*m = WMSIncomingMessage{}
	message := WMSIncomingMessage{}
	found := false
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI WMS MT message: storage reference TLV length %d, want 5", len(value))
		}
		found = true
		message.Stored = true
		message.Reference = WMSMessageReference{
			Storage: WMSStorage(value[0]),
			Index:   binary.LittleEndian.Uint32(value[1:5]),
		}
	} else if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 8 {
			return errors.New("parsing QMI WMS transfer message: header is truncated")
		}
		length := int(binary.LittleEndian.Uint16(value[6:8]))
		if len(value) != 8+length {
			return fmt.Errorf("parsing QMI WMS transfer message: value length %d, want %d", len(value), 8+length)
		}
		found = true
		message.AckIndicator = WMSAckIndicator(value[0])
		message.TransactionID = binary.LittleEndian.Uint32(value[1:5])
		message.Format = WMSMessageFormat(value[5])
		message.Data = slices.Clone(value[8 : 8+length])
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS event report: message mode TLV length %d, want 1", len(value))
		}
		message.MessageMode = WMSMessageMode(value[0])
		message.MessageModeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) < 3 {
			return errors.New("parsing QMI WMS ETWS message: header is truncated")
		}
		length := int(binary.LittleEndian.Uint16(value[1:3]))
		if len(value) != 3+length {
			return fmt.Errorf("parsing QMI WMS ETWS message: value length %d, want %d", len(value), 3+length)
		}
		found = true
		message.ETWSNotification = WMSETWSNotificationType(value[0])
		message.ETWSData = slices.Clone(value[3:])
		message.ETWSKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WMS ETWS PLMN: TLV length %d, want 4", len(value))
		}
		message.ETWSPLMN = WMSPLMN{
			MCC: binary.LittleEndian.Uint16(value[:2]),
			MNC: binary.LittleEndian.Uint16(value[2:]),
		}
		message.ETWSPLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		message.SMSCAddress = string(value)
		message.SMSCAddressKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS event report: SMS on IMS TLV length %d, want 1", len(value))
		}
		message.SMSOnIMS = value[0] == 1
		message.SMSOnIMSKnown = true
	}
	if !found {
		return errors.New("parsing QMI WMS event report: MT or ETWS message TLV missing")
	}
	*m = message
	return nil
}

func decodeWMSSMSCAddress(value []byte) (WMSSMSCAddress, error) {
	if len(value) < 4 {
		return WMSSMSCAddress{}, errors.New("parsing QMI WMS SMSC address: value is truncated")
	}
	length := int(value[3])
	if len(value) != 4+length {
		return WMSSMSCAddress{}, fmt.Errorf("parsing QMI WMS SMSC address: value length %d, want %d", len(value), 4+length)
	}
	return WMSSMSCAddress{
		Type:   string(value[:3]),
		Digits: string(value[4 : 4+length]),
	}, nil
}
