package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const voiceForwardHistoryMax = 512

// VoiceSupplementaryNotificationType identifies a 3GPP call notification.
type VoiceSupplementaryNotificationType uint8

const (
	VoiceSupplementaryOutgoingForwarded          VoiceSupplementaryNotificationType = 0x01
	VoiceSupplementaryOutgoingWaiting            VoiceSupplementaryNotificationType = 0x02
	VoiceSupplementaryOutgoingCUG                VoiceSupplementaryNotificationType = 0x03
	VoiceSupplementaryOutgoingBarred             VoiceSupplementaryNotificationType = 0x04
	VoiceSupplementaryOutgoingDeflected          VoiceSupplementaryNotificationType = 0x05
	VoiceSupplementaryIncomingCUG                VoiceSupplementaryNotificationType = 0x06
	VoiceSupplementaryIncomingBarred             VoiceSupplementaryNotificationType = 0x07
	VoiceSupplementaryIncomingForwarded          VoiceSupplementaryNotificationType = 0x08
	VoiceSupplementaryIncomingDeflected          VoiceSupplementaryNotificationType = 0x09
	VoiceSupplementaryIncomingIsForwarded        VoiceSupplementaryNotificationType = 0x0A
	VoiceSupplementaryUnconditionalForwardActive VoiceSupplementaryNotificationType = 0x0B
	VoiceSupplementaryConditionalForwardActive   VoiceSupplementaryNotificationType = 0x0C
	VoiceSupplementaryCLIRSuppressionRejected    VoiceSupplementaryNotificationType = 0x0D
	VoiceSupplementaryCallHeld                   VoiceSupplementaryNotificationType = 0x0E
	VoiceSupplementaryCallRetrieved              VoiceSupplementaryNotificationType = 0x0F
	VoiceSupplementaryCallMultiparty             VoiceSupplementaryNotificationType = 0x10
	VoiceSupplementaryIncomingECT                VoiceSupplementaryNotificationType = 0x11
	VoiceSupplementaryOutgoingProgressQueued     VoiceSupplementaryNotificationType = 0x12
)

// VoiceECTCallState describes the remote call involved in an explicit transfer.
type VoiceECTCallState uint8

const (
	VoiceECTCallNone VoiceECTCallState = iota
	VoiceECTCallAlerting
	VoiceECTCallActive
)

// VoiceECTNumber identifies the remote party in an explicit call transfer.
type VoiceECTNumber struct {
	State        VoiceECTCallState
	Presentation VoicePresentation
	Number       string
}

// VoiceSupplementaryCode identifies the forwarding reason carried by a notification.
type VoiceSupplementaryCode uint32

const (
	VoiceSupplementaryCodeForwardUnconditional VoiceSupplementaryCode = 0x01
	VoiceSupplementaryCodeForwardBusy          VoiceSupplementaryCode = 0x02
	VoiceSupplementaryCodeForwardNoReply       VoiceSupplementaryCode = 0x03
	VoiceSupplementaryCodeForwardUnreachable   VoiceSupplementaryCode = 0x04
	VoiceSupplementaryCodeForwardAll           VoiceSupplementaryCode = 0x05
	VoiceSupplementaryCodeForwardConditional   VoiceSupplementaryCode = 0x06
)

// VoiceSupplementaryNotification reports a 3GPP supplementary-service event.
type VoiceSupplementaryNotification struct {
	CallID              uint8
	Type                VoiceSupplementaryNotificationType
	CUGIndex            uint16
	CUGIndexKnown       bool
	ECTNumber           VoiceECTNumber
	ECTNumberKnown      bool
	Code                VoiceSupplementaryCode
	CodeKnown           bool
	ForwardHistory      []uint16
	ForwardHistoryKnown bool
	MediaDirection      VoiceCallAttribute
	MediaDirectionKnown bool
}

// VoiceSupplementaryService identifies a modem-originated supplementary
// request or a network response.
type VoiceSupplementaryService uint8

const (
	VoiceSupplementaryServiceActivate VoiceSupplementaryService = 0x01 + iota
	VoiceSupplementaryServiceDeactivate
	VoiceSupplementaryServiceRegister
	VoiceSupplementaryServiceErase
	VoiceSupplementaryServiceInterrogate
	VoiceSupplementaryServiceRegisterPassword
	VoiceSupplementaryServiceUSSDRequest
)

// VoiceSupplementaryDataSource identifies whether supplementary data came
// from the device or the network.
type VoiceSupplementaryDataSource uint8

const (
	VoiceSupplementaryDataSourceDevice VoiceSupplementaryDataSource = iota
	VoiceSupplementaryDataSourceNetwork
)

// VoiceCallForwardingInfo contains one call-forwarding record returned by the
// network in a supplementary-service indication.
type VoiceCallForwardingInfo struct {
	Status       VoiceActiveStatus
	ServiceClass uint8
	Number       string
	NoReplyTimer uint8
}

// VoiceSupplementaryStatus contains active and provisioned state for a line
// identification service.
type VoiceSupplementaryStatus struct {
	Active    VoiceActiveStatus
	Provision VoiceProvisionStatus
	Known     bool
}

// VoiceSupplementaryResultEvent contains one asynchronous supplementary
// service request or network response.
type VoiceSupplementaryResultEvent struct {
	Service               VoiceSupplementaryService
	ModifiedByCallControl bool
	ServiceClass          uint8
	ServiceClassKnown     bool
	Reason                VoiceSupplementaryReason
	ReasonKnown           bool
	Number                string
	NumberKnown           bool
	NoReplyTimer          uint8
	NoReplyTimerKnown     bool
	USSD                  VoiceUSSDData
	USSDKnown             bool
	CallID                uint8
	CallIDKnown           bool
	Alpha                 VoiceAlphaIdentifier
	AlphaKnown            bool
	Password              string
	PasswordKnown         bool
	NewPassword           string
	NewPasswordAgain      string
	NewPasswordKnown      bool
	DataSource            VoiceSupplementaryDataSource
	DataSourceKnown       bool
	FailureCause          uint16
	FailureCauseKnown     bool
	CallForwarding        []VoiceCallForwardingInfo
	CallForwardingKnown   bool
	CLIR                  VoiceSupplementaryStatus
	CLIP                  VoiceSupplementaryStatus
	COLP                  VoiceSupplementaryStatus
	COLR                  VoiceSupplementaryStatus
	CNAP                  VoiceSupplementaryStatus
	USSUTF16              []uint16
	USSUTF16Known         bool
	ExtendedServiceClass  VoiceServiceClass
	ExtendedClassKnown    bool
	BarredNumbers         []string
	BarredNumbersKnown    bool
}

// VoiceNetworkMode identifies the access technology used for a voice codec.
type VoiceNetworkMode uint32

const (
	VoiceNetworkModeNone VoiceNetworkMode = iota
	VoiceNetworkModeGSM
	VoiceNetworkModeWCDMA
	VoiceNetworkModeCDMA
	VoiceNetworkModeLTE
	VoiceNetworkModeTDSCDMA
	VoiceNetworkModeWLAN
	VoiceNetworkModeNR5G
)

// VoiceSpeechCodec identifies the codec selected for a call.
type VoiceSpeechCodec uint32

const (
	VoiceSpeechCodecNone VoiceSpeechCodec = iota
	VoiceSpeechCodecQCELP13K
	VoiceSpeechCodecEVRC
	VoiceSpeechCodecEVRCB
	VoiceSpeechCodecEVRCWB
	VoiceSpeechCodecEVRCNW
	VoiceSpeechCodecAMRNB
	VoiceSpeechCodecAMRWB
	VoiceSpeechCodecGSMEFR
	VoiceSpeechCodecGSMFR
	VoiceSpeechCodecGSMHR
	VoiceSpeechCodecG711U
	VoiceSpeechCodecG723
	VoiceSpeechCodecG711A
	VoiceSpeechCodecG722
	VoiceSpeechCodecG711AB
	VoiceSpeechCodecG729
	VoiceSpeechCodecEVSNB
	VoiceSpeechCodecEVSWB
	VoiceSpeechCodecEVSSWB
	VoiceSpeechCodecEVSFB
)

// VoiceSpeechCodecInfo contains optional codec negotiation details.
type VoiceSpeechCodecInfo struct {
	NetworkMode       VoiceNetworkMode
	NetworkModeKnown  bool
	Codec             VoiceSpeechCodec
	CodecKnown        bool
	SamplingRate      uint32
	SamplingRateKnown bool
	CallID            uint8
	CallIDKnown       bool
}

// VoiceHandoverState identifies progress of a voice handover.
type VoiceHandoverState uint32

const (
	VoiceHandoverStart VoiceHandoverState = 0x01 + iota
	VoiceHandoverFail
	VoiceHandoverComplete
	VoiceHandoverCancel
)

// VoiceHandoverType identifies source and target radio technologies.
type VoiceHandoverType uint32

const (
	VoiceHandoverGSMToGSM VoiceHandoverType = 0x01 + iota
	VoiceHandoverGSMToWCDMA
	VoiceHandoverWCDMAToWCDMA
	VoiceHandoverWCDMAToGSM
	VoiceHandoverSRVCCLTEToGSM
	VoiceHandoverSRVCCLTEToWCDMA
	VoiceHandoverDRVCCWLANToCDMA
	VoiceHandoverDRVCCWLANToGSMWCDMA
)

// VoiceHandover contains one handover state update.
type VoiceHandover struct {
	State     VoiceHandoverState
	Type      VoiceHandoverType
	TypeKnown bool
}

// VoicePrivacyInfo contains the privacy state for one call.
type VoicePrivacyInfo struct {
	CallID  uint8
	Privacy VoicePrivacy
}

// VoiceWatchSupplementaryNotifications subscribes to 3GPP call notifications.
func (c *Client) VoiceWatchSupplementaryNotifications(ctx context.Context) (<-chan VoiceSupplementaryNotification, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoiceSupplementaryNotify, voiceIndicationSupplementaryNotification)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice supplementary notifications: %w", err)
	}
	return unmarshalTLVStream[VoiceSupplementaryNotification](ctx, raw), nil
}

func (c *Client) watchVoiceTLVs(ctx context.Context, id MessageID, registration voiceIndicationRegistration) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceVoice)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceVoice, clientID, id)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := c.acquireVoiceIndication(ctx, registration); err != nil {
		cancel()
		return nil, err
	}

	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseVoiceIndication(registration)
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

// UnmarshalTLVs parses a supplementary-service notification.
func (e *VoiceSupplementaryNotification) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 2 {
		return errors.New("parsing QMI Voice supplementary notification: notification info TLV missing or malformed")
	}
	event := VoiceSupplementaryNotification{CallID: value[0], Type: VoiceSupplementaryNotificationType(value[1])}
	if err := parseVoiceOptionalUint16(tlvs, 0x10, &event.CUGIndex, &event.CUGIndexKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 3 {
			return errors.New("parsing QMI Voice supplementary ECT number: header is truncated")
		}
		length := int(value[2])
		if length > voiceNumberMax || len(value) != 3+length {
			return errors.New("parsing QMI Voice supplementary ECT number: number is truncated")
		}
		event.ECTNumber = VoiceECTNumber{
			State:        VoiceECTCallState(value[0]),
			Presentation: VoicePresentation(value[1]),
			Number:       string(value[3:]),
		}
		event.ECTNumberKnown = true
	}
	var code uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x12, &code, &event.CodeKnown); err != nil {
		return err
	}
	event.Code = VoiceSupplementaryCode(code)
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		history, err := decodeVoiceUint16Array(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice supplementary forward history: %w", err)
		}
		if len(history) > voiceForwardHistoryMax {
			return fmt.Errorf("parsing QMI Voice supplementary forward history: length %d exceeds %d", len(history), voiceForwardHistoryMax)
		}
		event.ForwardHistory = history
		event.ForwardHistoryKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI Voice supplementary media direction: TLV length %d, want 8", len(value))
		}
		event.MediaDirection = VoiceCallAttribute(binary.LittleEndian.Uint64(value))
		event.MediaDirectionKnown = true
	}
	*e = event
	return nil
}

// VoiceWatchSupplementaryResults subscribes to supplementary-service results.
func (c *Client) VoiceWatchSupplementaryResults(ctx context.Context) (<-chan VoiceSupplementaryResultEvent, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoiceSupplementaryResult, voiceIndicationSupplementaryResult)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice supplementary results: %w", err)
	}
	return unmarshalTLVStream[VoiceSupplementaryResultEvent](ctx, raw), nil
}

// VoiceWatchSpeechCodec subscribes to codec negotiation changes.
func (c *Client) VoiceWatchSpeechCodec(ctx context.Context) (<-chan VoiceSpeechCodecInfo, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoiceSpeechCodecInfo, voiceIndicationSpeech)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice speech codec: %w", err)
	}
	return unmarshalTLVStream[VoiceSpeechCodecInfo](ctx, raw), nil
}

// VoiceWatchHandover subscribes to voice handover state changes.
func (c *Client) VoiceWatchHandover(ctx context.Context) (<-chan VoiceHandover, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoiceHandover, voiceIndicationHandover)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice handovers: %w", err)
	}
	return unmarshalTLVStream[VoiceHandover](ctx, raw), nil
}

// VoiceWatchPrivacy subscribes to per-call privacy changes.
func (c *Client) VoiceWatchPrivacy(ctx context.Context) (<-chan VoicePrivacyInfo, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoicePrivacy, voiceIndicationPrivacy)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice privacy: %w", err)
	}
	return unmarshalTLVStream[VoicePrivacyInfo](ctx, raw), nil
}

// VoiceWatchTTY subscribes to TTY mode changes.
func (c *Client) VoiceWatchTTY(ctx context.Context) (<-chan VoiceTTYMode, error) {
	raw, err := c.watchVoiceTLVs(ctx, MessageVoiceTTY, voiceIndicationTTY)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice TTY mode: %w", err)
	}
	return unmarshalTLVStream[VoiceTTYMode](ctx, raw), nil
}

// UnmarshalTLVs parses a supplementary-service result indication.
func (e *VoiceSupplementaryResultEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 2 {
		return errors.New("parsing QMI Voice supplementary result: service info TLV missing or malformed")
	}
	event := VoiceSupplementaryResultEvent{
		Service:               VoiceSupplementaryService(value[0]),
		ModifiedByCallControl: value[1] != 0,
	}
	if err := parseVoiceOptionalUint8(tlvs, 0x10, &event.ServiceClass, &event.ServiceClassKnown); err != nil {
		return err
	}
	var reason uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x11, &reason, &event.ReasonKnown); err != nil {
		return err
	}
	event.Reason = VoiceSupplementaryReason(reason)
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) > voiceNumberMax {
			return fmt.Errorf("parsing QMI Voice supplementary result number: length %d exceeds %d", len(value), voiceNumberMax)
		}
		event.Number = string(value)
		event.NumberKnown = true
	}
	if err := parseVoiceOptionalUint8(tlvs, 0x13, &event.NoReplyTimer, &event.NoReplyTimerKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if err := event.USSD.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI Voice supplementary result USSD: %w", err)
		}
		event.USSDKnown = true
	}
	if err := parseVoiceOptionalUint8(tlvs, 0x15, &event.CallID, &event.CallIDKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		alpha, err := decodeVoiceAlphaIdentifier(value)
		if err != nil {
			return err
		}
		event.Alpha = alpha
		event.AlphaKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI Voice supplementary result password: TLV length %d, want 4", len(value))
		}
		event.Password = string(value)
		event.PasswordKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x18); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI Voice supplementary result new password: TLV length %d, want 8", len(value))
		}
		event.NewPassword = string(value[:4])
		event.NewPasswordAgain = string(value[4:])
		event.NewPasswordKnown = true
	}
	var source uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x19, &source, &event.DataSourceKnown); err != nil {
		return err
	}
	event.DataSource = VoiceSupplementaryDataSource(source)
	if err := parseVoiceOptionalUint16(tlvs, 0x1A, &event.FailureCause, &event.FailureCauseKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x1B); ok {
		forwarding, err := decodeVoiceCallForwardingInfo(value)
		if err != nil {
			return err
		}
		event.CallForwarding = forwarding
		event.CallForwardingKnown = true
	}
	statusFields := []struct {
		kind   byte
		status *VoiceSupplementaryStatus
	}{
		{kind: 0x1C, status: &event.CLIR},
		{kind: 0x1D, status: &event.CLIP},
		{kind: 0x1E, status: &event.COLP},
		{kind: 0x1F, status: &event.COLR},
		{kind: 0x20, status: &event.CNAP},
	}
	for _, field := range statusFields {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice supplementary result status: TLV 0x%02X length %d, want 2", field.kind, len(value))
		}
		*field.status = VoiceSupplementaryStatus{
			Active:    VoiceActiveStatus(value[0]),
			Provision: VoiceProvisionStatus(value[1]),
			Known:     true,
		}
	}
	if value, ok := tlv.Value(tlvs, 0x21); ok {
		decoded, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice supplementary result UTF-16 USSD: %w", err)
		}
		if len(decoded) > voiceUSSDDataMax {
			return fmt.Errorf("parsing QMI Voice supplementary result UTF-16 USSD: length %d exceeds %d", len(decoded), voiceUSSDDataMax)
		}
		event.USSUTF16 = decoded
		event.USSUTF16Known = true
	}
	var extended uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x22, &extended, &event.ExtendedClassKnown); err != nil {
		return err
	}
	event.ExtendedServiceClass = VoiceServiceClass(extended)
	if value, ok := tlv.Value(tlvs, 0x23); ok {
		numbers, err := decodeVoiceBarredNumbers(value)
		if err != nil {
			return err
		}
		event.BarredNumbers = numbers
		event.BarredNumbersKnown = true
	}
	*e = event
	return nil
}

func decodeVoiceCallForwardingInfo(value []byte) ([]VoiceCallForwardingInfo, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI Voice supplementary call forwarding: count is missing")
	}
	count := int(value[0])
	if count > 13 {
		return nil, fmt.Errorf("parsing QMI Voice supplementary call forwarding: count %d exceeds 13", count)
	}
	offset := 1
	records := make([]VoiceCallForwardingInfo, count)
	for i := range count {
		if len(value)-offset < 4 {
			return nil, fmt.Errorf("parsing QMI Voice supplementary call forwarding record %d: header is truncated", i)
		}
		length := int(value[offset+2])
		if length > voiceNumberMax || len(value)-offset < 4+length {
			return nil, fmt.Errorf("parsing QMI Voice supplementary call forwarding record %d: number is truncated", i)
		}
		records[i] = VoiceCallForwardingInfo{
			Status:       VoiceActiveStatus(value[offset]),
			ServiceClass: value[offset+1],
			Number:       string(value[offset+3 : offset+3+length]),
			NoReplyTimer: value[offset+3+length],
		}
		offset += 4 + length
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI Voice supplementary call forwarding: %d trailing bytes", len(value)-offset)
	}
	return records, nil
}

func decodeVoiceBarredNumbers(value []byte) ([]string, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI Voice supplementary barred numbers: count is missing")
	}
	count := int(value[0])
	if count > 50 {
		return nil, fmt.Errorf("parsing QMI Voice supplementary barred numbers: count %d exceeds 50", count)
	}
	offset := 1
	numbers := make([]string, count)
	for i := range count {
		if offset >= len(value) {
			return nil, fmt.Errorf("parsing QMI Voice supplementary barred number %d: length is missing", i)
		}
		length := int(value[offset])
		offset++
		if length > voiceNumberMax || len(value)-offset < length {
			return nil, fmt.Errorf("parsing QMI Voice supplementary barred number %d: number is truncated", i)
		}
		numbers[i] = string(value[offset : offset+length])
		offset += length
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI Voice supplementary barred numbers: %d trailing bytes", len(value)-offset)
	}
	return numbers, nil
}

// UnmarshalTLVs parses a speech-codec indication.
func (i *VoiceSpeechCodecInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var info VoiceSpeechCodecInfo
	var networkMode uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x10, &networkMode, &info.NetworkModeKnown); err != nil {
		return err
	}
	info.NetworkMode = VoiceNetworkMode(networkMode)
	var codec uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x11, &codec, &info.CodecKnown); err != nil {
		return err
	}
	info.Codec = VoiceSpeechCodec(codec)
	if err := parseVoiceOptionalUint32(tlvs, 0x12, &info.SamplingRate, &info.SamplingRateKnown); err != nil {
		return err
	}
	if err := parseVoiceOptionalUint8(tlvs, 0x13, &info.CallID, &info.CallIDKnown); err != nil {
		return err
	}
	*i = info
	return nil
}

// UnmarshalTLVs parses a handover indication.
func (h *VoiceHandover) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 4 {
		return errors.New("parsing QMI Voice handover: state TLV missing or malformed")
	}
	handover := VoiceHandover{State: VoiceHandoverState(binary.LittleEndian.Uint32(value))}
	var typ uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x10, &typ, &handover.TypeKnown); err != nil {
		return err
	}
	handover.Type = VoiceHandoverType(typ)
	*h = handover
	return nil
}

// UnmarshalTLVs parses a call-privacy indication.
func (i *VoicePrivacyInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 2 {
		return errors.New("parsing QMI Voice privacy: info TLV missing or malformed")
	}
	*i = VoicePrivacyInfo{CallID: value[0], Privacy: VoicePrivacy(value[1])}
	return nil
}

// UnmarshalTLVs parses a TTY-mode indication.
func (m *VoiceTTYMode) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 1 {
		return errors.New("parsing QMI Voice TTY mode: mode TLV missing or malformed")
	}
	*m = VoiceTTYMode(value[0])
	return nil
}
