package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	voiceNumberMax         = 81
	voiceSIPURIOverflowMax = 47
	voiceDTMFDigitsMax     = 32
	voiceUSSDDataMax       = 182
	voiceCallInfoMax       = 7
	voiceSIPURIMax         = 128
)

type voiceIndicationRegistration uint8

const (
	voiceIndicationCalls voiceIndicationRegistration = iota
	voiceIndicationDTMF
	voiceIndicationUSSD
	voiceIndicationSupplementaryNotification
	voiceIndicationSupplementaryResult
	voiceIndicationModification
	voiceIndicationHandover
	voiceIndicationSpeech
	voiceIndicationPrivacy
	voiceIndicationTTY
)

// VoiceCallState is the current modem state of a voice call.
type VoiceCallState uint8

const (
	VoiceCallStateOriginating   VoiceCallState = 0x01
	VoiceCallStateIncoming      VoiceCallState = 0x02
	VoiceCallStateConversation  VoiceCallState = 0x03
	VoiceCallStateCCInProgress  VoiceCallState = 0x04
	VoiceCallStateAlerting      VoiceCallState = 0x05
	VoiceCallStateHold          VoiceCallState = 0x06
	VoiceCallStateWaiting       VoiceCallState = 0x07
	VoiceCallStateDisconnecting VoiceCallState = 0x08
	VoiceCallStateEnd           VoiceCallState = 0x09
	VoiceCallStateSetup         VoiceCallState = 0x0A
	VoiceCallStatePreAlerting   VoiceCallState = 0x0B
)

// VoiceCallType identifies the media and emergency class of a call.
type VoiceCallType uint8

const (
	VoiceCallTypeVoice            VoiceCallType = 0x00
	VoiceCallTypeVoiceForced      VoiceCallType = 0x01
	VoiceCallTypeVoiceIP          VoiceCallType = 0x02
	VoiceCallTypeVideo            VoiceCallType = 0x03
	VoiceCallTypeVideoShare       VoiceCallType = 0x04
	VoiceCallTypeTest             VoiceCallType = 0x05
	VoiceCallTypeOTAPA            VoiceCallType = 0x06
	VoiceCallTypeStandardOTASP    VoiceCallType = 0x07
	VoiceCallTypeNonstandardOTASP VoiceCallType = 0x08
	VoiceCallTypeEmergency        VoiceCallType = 0x09
	VoiceCallTypeSupplementary    VoiceCallType = 0x0A
	VoiceCallTypeEmergencyIP      VoiceCallType = 0x0B
	VoiceCallTypeECall            VoiceCallType = 0x0C
	VoiceCallTypeEmergencyVideo   VoiceCallType = 0x0D
)

// VoiceCallDirection identifies a mobile-originated or mobile-terminated call.
type VoiceCallDirection uint8

const (
	VoiceCallDirectionMO VoiceCallDirection = 0x01
	VoiceCallDirectionMT VoiceCallDirection = 0x02
)

// VoiceCallMode is the access technology carrying a voice call.
type VoiceCallMode uint8

const (
	VoiceCallModeNoService VoiceCallMode = iota
	VoiceCallModeCDMA
	VoiceCallModeGSM
	VoiceCallModeUMTS
	VoiceCallModeLTE
	VoiceCallModeTDSCDMA
	VoiceCallModeUnknown
	VoiceCallModeWLAN
	VoiceCallModeNR5G
)

// VoiceALS identifies the alternate-line-service line of a call.
type VoiceALS uint8

const (
	VoiceALSLine1 VoiceALS = iota
	VoiceALSLine2
)

// VoiceSubscription identifies the subscription bound to a Voice client.
type VoiceSubscription uint8

const (
	VoiceSubscriptionPrimary VoiceSubscription = iota
	VoiceSubscriptionSecondary
	VoiceSubscriptionTertiary
)

// VoicePresentation describes whether the remote party number may be shown.
type VoicePresentation uint8

const (
	VoicePresentationAllowed     VoicePresentation = 0x00
	VoicePresentationRestricted  VoicePresentation = 0x01
	VoicePresentationUnavailable VoicePresentation = 0x02
	VoicePresentationPayphone    VoicePresentation = 0x04
)

// VoiceCLIR selects temporary calling-line identification restriction.
type VoiceCLIR uint8

const (
	VoiceCLIRSuppression VoiceCLIR = 0x01
	VoiceCLIRInvocation  VoiceCLIR = 0x02
)

// VoiceIPPresentation controls number presentation for an IP call.
type VoiceIPPresentation uint32

const (
	VoiceIPPresentationAllowed VoiceIPPresentation = iota
	VoiceIPPresentationRestricted
)

// VoiceCallAttribute is a transmit/receive media-direction mask.
type VoiceCallAttribute uint64

const (
	VoiceCallAttributeTX VoiceCallAttribute = 1 << iota
	VoiceCallAttributeRX
)

// VoiceRTTMode selects real-time text for an IP call.
type VoiceRTTMode uint32

const (
	VoiceRTTDisabled VoiceRTTMode = iota
	VoiceRTTFull
)

// VoiceDialService selects the radio domain used to originate a call.
type VoiceDialService uint32

const (
	VoiceDialServiceAutomatic       VoiceDialService = 0x01
	VoiceDialServiceGSM             VoiceDialService = 0x02
	VoiceDialServiceWCDMA           VoiceDialService = 0x03
	VoiceDialServiceCDMAAutomatic   VoiceDialService = 0x04
	VoiceDialServiceGSMWCDMA        VoiceDialService = 0x05
	VoiceDialServiceLTE             VoiceDialService = 0x06
	VoiceDialServiceTDSCDMA         VoiceDialService = 0x07
	VoiceDialServiceGSMWCDMATDSCDMA VoiceDialService = 0x08
	VoiceDialServiceCSOnly          VoiceDialService = 0x09
	VoiceDialServicePSOnly          VoiceDialService = 0x0A
	VoiceDialServiceNR5G            VoiceDialService = 0x0B
)

// VoiceCall contains the stable, commonly used fields from QMI Voice call
// information and all-call-status messages.
type VoiceCall struct {
	ID                          uint8
	State                       VoiceCallState
	Type                        VoiceCallType
	Direction                   VoiceCallDirection
	Mode                        VoiceCallMode
	Multiparty                  bool
	ALS                         VoiceALS
	Number                      string
	NumberKnown                 bool
	Presentation                VoicePresentation
	PresentationKnown           bool
	CallerName                  string
	CallerNameKnown             bool
	CallerNamePresentation      VoicePresentation
	CallerNamePresentationKnown bool
	CallerNameCodingScheme      uint8
	CallerNameCodingSchemeKnown bool
	Privacy                     VoicePrivacy
	PrivacyKnown                bool
	EndReason                   VoiceCallEndReason
	EndReasonKnown              bool
	EndReasonText               []uint16
	SIPURI                      string
	SIPURIKnown                 bool
	SIPErrorCode                uint16
	SIPErrorCodeKnown           bool
	AudioAttributes             VoiceCallAttribute
	AudioAttributesKnown        bool
	VideoAttributes             VoiceCallAttribute
	VideoAttributesKnown        bool
	RTTMode                     VoiceRTTMode
	RTTModeKnown                bool
	Secure                      bool
	SecureKnown                 bool
	SRVCC                       bool
	SRVCCKnown                  bool
}

// VoiceCalls is a QMI Voice call table.
type VoiceCalls []VoiceCall

// VoiceCallEndReason is the modem-provided reason a call ended.
type VoiceCallEndReason uint16

// VoiceDialOptions controls optional fields of QMI Voice Dial Call.
type VoiceDialOptions struct {
	CallType          *VoiceCallType
	CLIR              *VoiceCLIR
	EmergencyCategory *uint8
	Service           *VoiceDialService
	AudioAttributes   *VoiceCallAttribute
	VideoAttributes   *VoiceCallAttribute
	Presentation      *VoiceIPPresentation
	CallPull          *bool
	CodecProfile      *uint8
	Secure            *bool
	RTTMode           *VoiceRTTMode
	OriginationNumber string
	Secondary         *bool
}

// VoiceAnswerOptions controls optional fields of QMI Voice Answer Call.
type VoiceAnswerOptions struct {
	CallType        *VoiceCallType
	AudioAttributes *VoiceCallAttribute
	VideoAttributes *VoiceCallAttribute
	Presentation    *VoiceIPPresentation
	Reject          bool
	RejectCause     *uint32
	SIPRejectCause  *uint16
	CodecProfile    *uint8
	RTTMode         *VoiceRTTMode
}

// VoiceIndicationConfig updates selected QMI Voice indication registrations.
// Nil fields are omitted so one caller does not overwrite unrelated settings.
type VoiceIndicationConfig struct {
	CallEvents                      *bool
	DTMFEvents                      *bool
	USSDEvents                      *bool
	SupplementaryNotificationEvents *bool
	SupplementaryResultEvents       *bool
	ModificationEvents              *bool
	HandoverEvents                  *bool
	SpeechEvents                    *bool
	PrivacyEvents                   *bool
	TTYEvents                       *bool
}

// VoiceManageOperation identifies an in-call supplementary operation.
type VoiceManageOperation uint8

const (
	VoiceManageReleaseHeldOrWaiting             VoiceManageOperation = 0x01
	VoiceManageReleaseActiveAcceptHeldOrWaiting VoiceManageOperation = 0x02
	VoiceManageHoldActiveAcceptWaitingOrHeld    VoiceManageOperation = 0x03
	VoiceManageHoldAllExceptSpecified           VoiceManageOperation = 0x04
	VoiceManageConference                       VoiceManageOperation = 0x05
	VoiceManageExplicitTransfer                 VoiceManageOperation = 0x06
	VoiceManageCCBSActivation                   VoiceManageOperation = 0x07
	VoiceManageEndAll                           VoiceManageOperation = 0x08
	VoiceManageReleaseSpecified                 VoiceManageOperation = 0x09
	VoiceManageLocalHold                        VoiceManageOperation = 0x0A
	VoiceManageLocalUnhold                      VoiceManageOperation = 0x0B
)

// VoiceManageRequest performs one in-call supplementary operation.
type VoiceManageRequest struct {
	Operation   VoiceManageOperation
	CallID      *uint8
	RejectCause *uint32
}

// VoiceDTMFOnLength selects a CDMA burst-DTMF pulse width.
type VoiceDTMFOnLength uint8

const (
	VoiceDTMFOn95ms VoiceDTMFOnLength = iota
	VoiceDTMFOn150ms
	VoiceDTMFOn200ms
	VoiceDTMFOn250ms
	VoiceDTMFOn300ms
	VoiceDTMFOn350ms
	VoiceDTMFOnSMS
)

// VoiceDTMFOffLength selects a CDMA burst-DTMF interdigit interval.
type VoiceDTMFOffLength uint8

const (
	VoiceDTMFOff60ms VoiceDTMFOffLength = iota
	VoiceDTMFOff100ms
	VoiceDTMFOff150ms
	VoiceDTMFOff200ms
)

// VoiceDTMFLengths configures a CDMA burst-DTMF sequence.
type VoiceDTMFLengths struct {
	On  VoiceDTMFOnLength
	Off VoiceDTMFOffLength
}

// VoiceDTMFEventType identifies a received or transmitted DTMF event.
type VoiceDTMFEventType uint8

const (
	VoiceDTMFEventReverseBurst    VoiceDTMFEventType = 0x00
	VoiceDTMFEventReverseStart    VoiceDTMFEventType = 0x01
	VoiceDTMFEventReverseStop     VoiceDTMFEventType = 0x03
	VoiceDTMFEventForwardBurst    VoiceDTMFEventType = 0x05
	VoiceDTMFEventForwardStart    VoiceDTMFEventType = 0x06
	VoiceDTMFEventForwardStop     VoiceDTMFEventType = 0x07
	VoiceDTMFEventIPIncomingStart VoiceDTMFEventType = 0x08
	VoiceDTMFEventIPIncomingStop  VoiceDTMFEventType = 0x09
)

// VoiceDTMFEvent contains one QMI Voice DTMF indication.
type VoiceDTMFEvent struct {
	CallID         uint8
	Type           VoiceDTMFEventType
	Digits         string
	OnLength       VoiceDTMFOnLength
	OnLengthKnown  bool
	OffLength      VoiceDTMFOffLength
	OffLengthKnown bool
	Volume         uint16
	VolumeKnown    bool
}

// VoiceUSSDEncoding identifies the coding scheme of a QMI USSD payload.
type VoiceUSSDEncoding uint8

const (
	VoiceUSSDEncodingASCII VoiceUSSDEncoding = 0x01
	VoiceUSSDEncoding8Bit  VoiceUSSDEncoding = 0x02
	VoiceUSSDEncodingUCS2  VoiceUSSDEncoding = 0x03
)

// VoiceUSSDData contains USSD bytes encoded according to Encoding.
type VoiceUSSDData struct {
	Encoding VoiceUSSDEncoding
	Data     []byte
}

// VoiceAlphaEncoding identifies the coding scheme of a network alpha identifier.
type VoiceAlphaEncoding uint8

const (
	VoiceAlphaEncodingGSM  VoiceAlphaEncoding = 0x01
	VoiceAlphaEncodingUCS2 VoiceAlphaEncoding = 0x02
)

// VoiceAlphaIdentifier contains network-provided display text.
type VoiceAlphaIdentifier struct {
	Encoding VoiceAlphaEncoding
	Data     []byte
}

// VoiceSupplementaryServiceType identifies a call-control supplementary result.
type VoiceSupplementaryServiceType uint8

const VoiceSupplementaryServiceUSSD VoiceSupplementaryServiceType = 0x07

// VoiceCallControlResultType identifies how SIM call control modified a USSD request.
type VoiceCallControlResultType uint8

const (
	VoiceCallControlResultVoice VoiceCallControlResultType = iota
	VoiceCallControlResultSupplementaryService
	VoiceCallControlResultUSSD
)

// VoiceUSSDResult contains optional network data returned synchronously by
// Originate USSD.
type VoiceUSSDResult struct {
	Data                                 VoiceUSSDData
	DataKnown                            bool
	UTF16                                []uint16
	FailureCause                         uint16
	FailureCauseKnown                    bool
	Alpha                                VoiceAlphaIdentifier
	AlphaKnown                           bool
	CallControlResult                    VoiceCallControlResultType
	CallControlResultKnown               bool
	CallID                               uint8
	CallIDKnown                          bool
	CallControlSupplementaryService      VoiceSupplementaryServiceType
	CallControlSupplementaryServiceKnown bool
	SIPErrorCode                         uint16
	SIPErrorCodeKnown                    bool
}

// VoiceUSSDAction reports whether the network expects another user response.
type VoiceUSSDAction uint8

const (
	VoiceUSSDActionNotRequired VoiceUSSDAction = 0x01
	VoiceUSSDActionRequired    VoiceUSSDAction = 0x02
)

// VoiceUSSDEvent is an unsolicited USSD notification or release.
type VoiceUSSDEvent struct {
	Released          bool
	Action            VoiceUSSDAction
	Data              VoiceUSSDData
	DataKnown         bool
	UTF16             []uint16
	FailureCauseText  []uint16
	SIPErrorCode      uint16
	SIPErrorCodeKnown bool
}

// VoiceDial originates a voice call and returns the modem-assigned call ID.
func (c *Client) VoiceDial(ctx context.Context, number string, options VoiceDialOptions) (uint8, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return 0, errors.New("dialing QMI Voice call: number is empty")
	}
	if len(number) > voiceNumberMax+voiceSIPURIOverflowMax {
		return 0, fmt.Errorf("dialing QMI Voice call: number length %d exceeds %d", len(number), voiceNumberMax+voiceSIPURIOverflowMax)
	}

	first := min(len(number), voiceNumberMax)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, []byte(number[:first]))}
	if options.CallType != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*options.CallType)))
	}
	if options.CLIR != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*options.CLIR)))
	}
	if options.EmergencyCategory != nil {
		tlvs = append(tlvs, tlv.Uint(0x14, *options.EmergencyCategory))
	}
	if options.Service != nil {
		tlvs = append(tlvs, tlv.Uint(0x16, uint32(*options.Service)))
	}
	if len(number) > first {
		tlvs = append(tlvs, tlv.Bytes(0x17, []byte(number[first:])))
	}
	if options.AudioAttributes != nil {
		tlvs = append(tlvs, voiceUint64TLV(0x18, uint64(*options.AudioAttributes)))
	}
	if options.VideoAttributes != nil {
		tlvs = append(tlvs, voiceUint64TLV(0x19, uint64(*options.VideoAttributes)))
	}
	if options.Presentation != nil {
		tlvs = append(tlvs, tlv.Uint(0x1A, uint32(*options.Presentation)))
	}
	if options.CallPull != nil {
		tlvs = append(tlvs, tlv.Uint(0x20, boolByte(*options.CallPull)))
	}
	if options.CodecProfile != nil {
		tlvs = append(tlvs, tlv.Uint(0x21, *options.CodecProfile))
	}
	if options.Secure != nil {
		tlvs = append(tlvs, tlv.Uint(0x22, boolByte(*options.Secure)))
	}
	if options.RTTMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x23, uint32(*options.RTTMode)))
	}
	if options.OriginationNumber != "" {
		originationNumber := strings.TrimSpace(options.OriginationNumber)
		if originationNumber == "" {
			return 0, errors.New("dialing QMI Voice call: origination number is empty")
		}
		if len(originationNumber) > voiceNumberMax {
			return 0, fmt.Errorf("dialing QMI Voice call: origination number length %d exceeds %d", len(originationNumber), voiceNumberMax)
		}
		tlvs = append(tlvs, tlv.Bytes(0x24, []byte(originationNumber)))
	}
	if options.Secondary != nil {
		tlvs = append(tlvs, tlv.Uint(0x25, boolByte(*options.Secondary)))
	}

	var callID uint8
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceDialCall, tlvs)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x10)
		if !ok {
			return errors.New("parsing QMI Voice dial response: call ID TLV missing")
		}
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice dial response: call ID TLV length %d, want 1", len(value))
		}
		callID = value[0]
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("dialing QMI Voice call: %w", err)
	}
	return callID, nil
}

// VoiceEnd ends one active call.
func (c *Client) VoiceEnd(ctx context.Context, callID uint8, endCauses ...*uint32) error {
	tlvs := tlv.TLVs{tlv.Uint(0x01, callID)}
	if len(endCauses) > 0 && endCauses[0] != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *endCauses[0]))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceEndCall, tlvs); err != nil {
		return fmt.Errorf("ending QMI Voice call %d: %w", callID, err)
	}
	return nil
}

// VoiceAnswer answers or rejects one incoming call.
func (c *Client) VoiceAnswer(ctx context.Context, callID uint8, optionValues ...VoiceAnswerOptions) error {
	var options VoiceAnswerOptions
	if len(optionValues) > 0 {
		options = optionValues[0]
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, callID)}
	if options.CallType != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*options.CallType)))
	}
	if options.AudioAttributes != nil {
		tlvs = append(tlvs, voiceUint64TLV(0x11, uint64(*options.AudioAttributes)))
	}
	if options.VideoAttributes != nil {
		tlvs = append(tlvs, voiceUint64TLV(0x12, uint64(*options.VideoAttributes)))
	}
	if options.Presentation != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, uint32(*options.Presentation)))
	}
	if options.Reject {
		tlvs = append(tlvs, tlv.Uint(0x15, uint8(1)))
	}
	if options.RejectCause != nil {
		tlvs = append(tlvs, tlv.Uint(0x16, *options.RejectCause))
	}
	if options.SIPRejectCause != nil {
		tlvs = append(tlvs, tlv.Uint(0x17, *options.SIPRejectCause))
	}
	if options.CodecProfile != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, *options.CodecProfile))
	}
	if options.RTTMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x19, uint32(*options.RTTMode)))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceAnswerCall, tlvs); err != nil {
		return fmt.Errorf("answering QMI Voice call %d: %w", callID, err)
	}
	return nil
}

// VoiceGetAllCalls returns the modem's current call table.
func (c *Client) VoiceGetAllCalls(ctx context.Context) (VoiceCalls, error) {
	var parsed voiceGetAllCallsResponse
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceGetAllCallInfo, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return parsed.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return nil, fmt.Errorf("reading QMI Voice calls: %w", err)
	}
	return parsed.Calls, nil
}

type voiceGetAllCallsResponse struct {
	Calls VoiceCalls
}

func (r *voiceGetAllCallsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var calls VoiceCalls
	if err := calls.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	*r = voiceGetAllCallsResponse{Calls: calls}
	return nil
}

type voiceAllCallStatusIndication struct {
	Calls VoiceCalls
}

func (i *voiceAllCallStatusIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var calls VoiceCalls
	if err := calls.unmarshalTLVs(tlvs, 0x01, 0x10); err != nil {
		return err
	}
	*i = voiceAllCallStatusIndication{Calls: calls}
	return nil
}

// VoiceSetIndicationReport updates Voice indication registrations.
func (c *Client) VoiceSetIndicationReport(ctx context.Context, config VoiceIndicationConfig) error {
	var tlvs tlv.TLVs
	if config.DTMFEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*config.DTMFEvents)))
	}
	if config.PrivacyEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*config.PrivacyEvents)))
	}
	if config.SupplementaryNotificationEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*config.SupplementaryNotificationEvents)))
	}
	if config.CallEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, boolByte(*config.CallEvents)))
	}
	if config.HandoverEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x14, boolByte(*config.HandoverEvents)))
	}
	if config.SpeechEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, boolByte(*config.SpeechEvents)))
	}
	if config.USSDEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x16, boolByte(*config.USSDEvents)))
	}
	if config.SupplementaryResultEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x17, boolByte(*config.SupplementaryResultEvents)))
	}
	if config.ModificationEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, boolByte(*config.ModificationEvents)))
	}
	if config.TTYEvents != nil {
		tlvs = append(tlvs, tlv.Uint(0x20, boolByte(*config.TTYEvents)))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceIndicationRegister, tlvs); err != nil {
		return fmt.Errorf("configuring QMI Voice indications: %w", err)
	}
	return nil
}

// VoiceWatchCalls subscribes to complete call-table updates.
func (c *Client) VoiceWatchCalls(ctx context.Context) (<-chan []VoiceCall, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceVoice)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice calls: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceVoice, clientID, MessageVoiceAllCallStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI Voice calls: %w", err)
	}
	if err := c.acquireVoiceIndication(ctx, voiceIndicationCalls); err != nil {
		cancel()
		return nil, err
	}

	out := make(chan []VoiceCall, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseVoiceIndication(voiceIndicationCalls)
		for indication := range indications {
			var parsed voiceAllCallStatusIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Calls:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

// VoiceManage performs one 3GPP in-call supplementary operation.
func (c *Client) VoiceManage(ctx context.Context, req VoiceManageRequest) error {
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(req.Operation))}
	if req.CallID != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *req.CallID))
	}
	if req.RejectCause != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *req.RejectCause))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceManageCalls, tlvs); err != nil {
		return fmt.Errorf("managing QMI Voice calls: %w", err)
	}
	return nil
}

// VoiceBurstDTMF sends a CDMA burst-DTMF sequence.
func (c *Client) VoiceBurstDTMF(ctx context.Context, callID uint8, digits string, lengths *VoiceDTMFLengths) error {
	if err := validateVoiceDTMFDigits(digits, voiceDTMFDigitsMax); err != nil {
		return fmt.Errorf("sending QMI Voice burst DTMF: %w", err)
	}
	value := []byte{callID, byte(len(digits))}
	value = append(value, digits...)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if lengths != nil {
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte{byte(lengths.On), byte(lengths.Off)}))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceBurstDTMF, tlvs); err != nil {
		return fmt.Errorf("sending QMI Voice burst DTMF: %w", err)
	}
	return nil
}

// VoiceStartDTMF starts one continuous DTMF tone.
func (c *Client) VoiceStartDTMF(ctx context.Context, callID uint8, digit byte) error {
	if !validVoiceDTMFDigit(digit) {
		return fmt.Errorf("starting QMI Voice DTMF: invalid digit %q", digit)
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceStartContinuousDTMF, tlv.TLVs{tlv.Bytes(0x01, []byte{callID, digit})}); err != nil {
		return fmt.Errorf("starting QMI Voice DTMF: %w", err)
	}
	return nil
}

// VoiceStopDTMF stops a continuous DTMF tone.
func (c *Client) VoiceStopDTMF(ctx context.Context, callID uint8) error {
	if err := c.voiceResultRequest(ctx, MessageVoiceStopContinuousDTMF, tlv.TLVs{tlv.Uint(0x01, callID)}); err != nil {
		return fmt.Errorf("stopping QMI Voice DTMF: %w", err)
	}
	return nil
}

// VoiceOriginateUSSD starts a 3GPP USSD session.
func (c *Client) VoiceOriginateUSSD(ctx context.Context, data VoiceUSSDData) (VoiceUSSDResult, error) {
	value, err := data.MarshalBinary()
	if err != nil {
		return VoiceUSSDResult{}, fmt.Errorf("originating QMI Voice USSD: %w", err)
	}
	var result VoiceUSSDResult
	err = c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceOriginateUSSD, tlv.TLVs{tlv.Bytes(0x01, value)})
		if err != nil {
			return err
		}
		if err := result.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return result, fmt.Errorf("originating QMI Voice USSD: %w", err)
	}
	return result, nil
}

// VoiceAnswerUSSD replies to a network-initiated USSD request.
func (c *Client) VoiceAnswerUSSD(ctx context.Context, data VoiceUSSDData) error {
	value, err := data.MarshalBinary()
	if err != nil {
		return fmt.Errorf("answering QMI Voice USSD: %w", err)
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceAnswerUSSD, tlv.TLVs{tlv.Bytes(0x01, value)}); err != nil {
		return fmt.Errorf("answering QMI Voice USSD: %w", err)
	}
	return nil
}

// VoiceCancelUSSD aborts the current USSD operation.
func (c *Client) VoiceCancelUSSD(ctx context.Context) error {
	if err := c.voiceResultRequest(ctx, MessageVoiceCancelUSSD, nil); err != nil {
		return fmt.Errorf("canceling QMI Voice USSD: %w", err)
	}
	return nil
}

// VoiceBindSubscription binds a Voice client to one modem subscription.
func (c *Client) VoiceBindSubscription(ctx context.Context, subscription VoiceSubscription) error {
	if subscription > VoiceSubscriptionTertiary {
		return fmt.Errorf("binding QMI Voice subscription: value %d is out of range", subscription)
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceBindSubscription, tlv.TLVs{tlv.Uint(0x01, uint8(subscription))}); err != nil {
		return fmt.Errorf("binding QMI Voice subscription: %w", err)
	}
	return nil
}

// VoiceWatchDTMF subscribes to received and transmitted DTMF events.
func (c *Client) VoiceWatchDTMF(ctx context.Context) (<-chan VoiceDTMFEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceVoice)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice DTMF: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceVoice, clientID, MessageVoiceDTMF)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI Voice DTMF: %w", err)
	}
	if err := c.acquireVoiceIndication(ctx, voiceIndicationDTMF); err != nil {
		cancel()
		return nil, err
	}
	out := make(chan VoiceDTMFEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseVoiceIndication(voiceIndicationDTMF)
		for indication := range indications {
			var event VoiceDTMFEvent
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

// VoiceWatchUSSD subscribes to USSD notifications and release indications.
func (c *Client) VoiceWatchUSSD(ctx context.Context) (<-chan VoiceUSSDEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceVoice)
	if err != nil {
		return nil, fmt.Errorf("watching QMI Voice USSD: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	updates, err := transport.Indications(watchCtx, ServiceVoice, clientID, MessageVoiceUSSD)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI Voice USSD: %w", err)
	}
	releases, err := transport.Indications(watchCtx, ServiceVoice, clientID, MessageVoiceUSSDRelease)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI Voice USSD releases: %w", err)
	}
	if err := c.acquireVoiceIndication(ctx, voiceIndicationUSSD); err != nil {
		cancel()
		return nil, err
	}

	out := make(chan VoiceUSSDEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseVoiceIndication(voiceIndicationUSSD)
		for updates != nil || releases != nil {
			var event VoiceUSSDEvent
			select {
			case indication, ok := <-updates:
				if !ok {
					updates = nil
					continue
				}
				if err := event.UnmarshalTLVs(indication.TLVs); err != nil {
					return
				}
			case _, ok := <-releases:
				if !ok {
					releases = nil
					continue
				}
				event.Released = true
			case <-watchCtx.Done():
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

func (c *Client) voiceResultRequest(ctx context.Context, id MessageID, tlvs tlv.TLVs) error {
	return c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, id, tlvs)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

func (c *Client) acquireVoiceIndication(ctx context.Context, registration voiceIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.voiceIndicationRefs == nil {
		c.voiceIndicationRefs = make(map[voiceIndicationRegistration]int)
	}
	if c.voiceIndicationRefs[registration] > 0 {
		c.voiceIndicationRefs[registration]++
		return nil
	}
	c.voiceIndicationRefs[registration] = 1
	if err := c.VoiceSetIndicationReport(ctx, voiceIndicationConfig(registration, true)); err != nil {
		delete(c.voiceIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseVoiceIndication(registration voiceIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.voiceIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.voiceIndicationRefs[registration]--
		return
	}
	delete(c.voiceIndicationRefs, registration)
	_ = c.VoiceSetIndicationReport(ctx, voiceIndicationConfig(registration, false))
}

func voiceIndicationConfig(registration voiceIndicationRegistration, enabled bool) VoiceIndicationConfig {
	switch registration {
	case voiceIndicationCalls:
		return VoiceIndicationConfig{CallEvents: &enabled}
	case voiceIndicationDTMF:
		return VoiceIndicationConfig{DTMFEvents: &enabled}
	case voiceIndicationUSSD:
		return VoiceIndicationConfig{USSDEvents: &enabled}
	case voiceIndicationSupplementaryNotification:
		return VoiceIndicationConfig{SupplementaryNotificationEvents: &enabled}
	case voiceIndicationSupplementaryResult:
		return VoiceIndicationConfig{SupplementaryResultEvents: &enabled}
	case voiceIndicationModification:
		return VoiceIndicationConfig{ModificationEvents: &enabled}
	case voiceIndicationHandover:
		return VoiceIndicationConfig{HandoverEvents: &enabled}
	case voiceIndicationSpeech:
		return VoiceIndicationConfig{SpeechEvents: &enabled}
	case voiceIndicationPrivacy:
		return VoiceIndicationConfig{PrivacyEvents: &enabled}
	case voiceIndicationTTY:
		return VoiceIndicationConfig{TTYEvents: &enabled}
	default:
		return VoiceIndicationConfig{}
	}
}

// UnmarshalTLVs parses a Get All Call Info response.
func (c *VoiceCalls) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return c.unmarshalTLVs(tlvs, 0x10, 0x11)
}

func (c *VoiceCalls) unmarshalTLVs(tlvs tlv.TLVs, callsTLV, numbersTLV byte) error {
	value, ok := tlv.Value(tlvs, callsTLV)
	if !ok {
		*c = nil
		return nil
	}
	if len(value) < 1 {
		return errors.New("parsing QMI Voice calls: call count is missing")
	}
	count := int(value[0])
	want := 1 + count*7
	if count > voiceCallInfoMax || len(value) != want {
		return fmt.Errorf("parsing QMI Voice calls: call table length %d, want %d", len(value), want)
	}
	calls := make(VoiceCalls, 0, count)
	byID := make(map[uint8]int, count)
	for i := range count {
		offset := 1 + i*7
		call := VoiceCall{
			ID:         value[offset],
			State:      VoiceCallState(value[offset+1]),
			Type:       VoiceCallType(value[offset+2]),
			Direction:  VoiceCallDirection(value[offset+3]),
			Mode:       VoiceCallMode(value[offset+4]),
			Multiparty: value[offset+5] == 1,
			ALS:        VoiceALS(value[offset+6]),
		}
		byID[call.ID] = len(calls)
		calls = append(calls, call)
	}

	numbers, ok := tlv.Value(tlvs, numbersTLV)
	if ok {
		if len(numbers) < 1 {
			return errors.New("parsing QMI Voice calls: remote number count is missing")
		}
		numberCount := int(numbers[0])
		if numberCount > voiceCallInfoMax {
			return errors.New("parsing QMI Voice calls: remote number count exceeds protocol maximum")
		}
		numbers = numbers[1:]
		for range numberCount {
			if len(numbers) < 3 {
				return errors.New("parsing QMI Voice calls: remote number header is truncated")
			}
			callID := numbers[0]
			presentation := VoicePresentation(numbers[1])
			length := int(numbers[2])
			numbers = numbers[3:]
			if length > voiceNumberMax || len(numbers) < length {
				return errors.New("parsing QMI Voice calls: remote number data is truncated")
			}
			if index, found := byID[callID]; found {
				calls[index].Presentation = presentation
				calls[index].PresentationKnown = true
				calls[index].Number = string(numbers[:length])
				calls[index].NumberKnown = true
			}
			numbers = numbers[length:]
		}
		if len(numbers) != 0 {
			return errors.New("parsing QMI Voice calls: remote numbers contain trailing data")
		}
	}
	detailTLVs := voiceCallDetailTLVs{
		name:      0x12,
		endReason: 0x18,
		audio:     0x1F,
		video:     0x20,
		sipURI:    0x22,
		ipName:    0x27,
		endText:   0x28,
		sipError:  0x2A,
		rtt:       0x2E,
	}
	if callsTLV == 0x01 {
		detailTLVs = voiceCallDetailTLVs{
			name:      0x11,
			endReason: 0x14,
			audio:     0x1B,
			video:     0x1C,
			sipURI:    0x1E,
			ipName:    0x2D,
			endText:   0x2E,
			namePI:    0x2F,
			secure:    0x32,
			sipError:  0x33,
			rtt:       0x38,
		}
	}
	if err := calls.unmarshalDetails(tlvs, byID, detailTLVs); err != nil {
		return err
	}
	*c = calls
	return nil
}

// UnmarshalTLVs parses a Get Call Info response.
func (c *VoiceCall) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok || len(value) != 5 {
		return errors.New("parsing QMI Voice call info: call TLV missing or malformed")
	}
	call := VoiceCall{
		ID:        value[0],
		State:     VoiceCallState(value[1]),
		Type:      VoiceCallType(value[2]),
		Direction: VoiceCallDirection(value[3]),
		Mode:      VoiceCallMode(value[4]),
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 2 {
			return errors.New("parsing QMI Voice call info: remote number header is truncated")
		}
		length := int(value[1])
		if length > voiceNumberMax || len(value) != 2+length {
			return errors.New("parsing QMI Voice call info: remote number is truncated")
		}
		call.Presentation = VoicePresentation(value[0])
		call.PresentationKnown = true
		call.Number = string(value[2:])
		call.NumberKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice call privacy: TLV length %d, want 1", len(value))
		}
		call.Privacy = VoicePrivacy(value[0])
		call.PrivacyKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if err := decodeVoiceSingleCallerName(value, &call); err != nil {
			return fmt.Errorf("parsing QMI Voice call name: %w", err)
		}
	}
	attributeFields := []struct {
		kind  byte
		apply func(VoiceCallAttribute)
	}{
		{kind: 0x1C, apply: func(value VoiceCallAttribute) {
			call.AudioAttributes = value
			call.AudioAttributesKnown = true
		}},
		{kind: 0x1D, apply: func(value VoiceCallAttribute) {
			call.VideoAttributes = value
			call.VideoAttributesKnown = true
		}},
	}
	for _, field := range attributeFields {
		if value, ok := tlv.Value(tlvs, field.kind); ok {
			if len(value) != 8 {
				return fmt.Errorf("parsing QMI Voice call attributes: TLV 0x%02X length %d, want 8", field.kind, len(value))
			}
			field.apply(VoiceCallAttribute(binary.LittleEndian.Uint64(value)))
		}
	}
	if value, ok := tlv.Value(tlvs, 0x1F); ok {
		if len(value) > voiceSIPURIMax {
			return fmt.Errorf("parsing QMI Voice call SIP URI: length %d exceeds %d", len(value), voiceSIPURIMax)
		}
		call.SIPURI = string(value)
		call.SIPURIKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x20); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice call SRVCC state: TLV length %d, want 1", len(value))
		}
		call.SRVCC = value[0] != 0
		call.SRVCCKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x23); ok {
		name, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice IP caller name: %w", err)
		}
		call.CallerName = string(utf16.Decode(name))
		call.CallerNameKnown = true
		call.CallerNameCodingSchemeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x24); ok {
		text, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice call end reason text: %w", err)
		}
		call.EndReasonText = text
	}
	if value, ok := tlv.Value(tlvs, 0x26); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice call SIP error: TLV length %d, want 2", len(value))
		}
		call.SIPErrorCode = binary.LittleEndian.Uint16(value)
		call.SIPErrorCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x28); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI Voice call RTT mode: TLV length %d, want 4", len(value))
		}
		call.RTTMode = VoiceRTTMode(binary.LittleEndian.Uint32(value))
		call.RTTModeKnown = true
	}
	*c = call
	return nil
}

// UnmarshalTLVs parses QMI Voice DTMF indication TLVs.
func (e *VoiceDTMFEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) < 3 {
		return errors.New("parsing QMI Voice DTMF: event TLV missing or truncated")
	}
	length := int(value[2])
	if length > 64 || len(value) != 3+length {
		return fmt.Errorf("parsing QMI Voice DTMF: digit length %d is invalid for TLV length %d", length, len(value))
	}
	event := VoiceDTMFEvent{CallID: value[0], Type: VoiceDTMFEventType(value[1]), Digits: string(value[3:])}
	var on uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x10, &on, &event.OnLengthKnown); err != nil {
		return err
	}
	event.OnLength = VoiceDTMFOnLength(on)
	var off uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x11, &off, &event.OffLengthKnown); err != nil {
		return err
	}
	event.OffLength = VoiceDTMFOffLength(off)
	if err := parseVoiceOptionalUint16(tlvs, 0x12, &event.Volume, &event.VolumeKnown); err != nil {
		return err
	}
	*e = event
	return nil
}

type voiceCallDetailTLVs struct {
	name      byte
	endReason byte
	audio     byte
	video     byte
	sipURI    byte
	ipName    byte
	endText   byte
	namePI    byte
	secure    byte
	sipError  byte
	rtt       byte
}

func (calls VoiceCalls) unmarshalDetails(tlvs tlv.TLVs, byID map[uint8]int, kinds voiceCallDetailTLVs) error {
	if value, ok := tlv.Value(tlvs, kinds.name); ok {
		if err := applyVoiceCallerNames(value, calls, byID); err != nil {
			return fmt.Errorf("parsing QMI Voice caller names: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.endReason); ok {
		if err := applyVoiceCallRecords(value, 3, calls, byID, func(call *VoiceCall, record []byte) {
			call.EndReason = VoiceCallEndReason(binary.LittleEndian.Uint16(record[1:]))
			call.EndReasonKnown = true
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice call end reasons: %w", err)
		}
	}
	attributeFields := []struct {
		kind  byte
		apply func(*VoiceCall, VoiceCallAttribute)
	}{
		{kind: kinds.audio, apply: func(call *VoiceCall, value VoiceCallAttribute) {
			call.AudioAttributes = value
			call.AudioAttributesKnown = true
		}},
		{kind: kinds.video, apply: func(call *VoiceCall, value VoiceCallAttribute) {
			call.VideoAttributes = value
			call.VideoAttributesKnown = true
		}},
	}
	for _, field := range attributeFields {
		if value, ok := tlv.Value(tlvs, field.kind); ok {
			if err := applyVoiceCallRecords(value, 9, calls, byID, func(call *VoiceCall, record []byte) {
				field.apply(call, VoiceCallAttribute(binary.LittleEndian.Uint64(record[1:])))
			}); err != nil {
				return fmt.Errorf("parsing QMI Voice call attributes TLV 0x%02X: %w", field.kind, err)
			}
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.sipURI); ok {
		if err := applyVoiceCallStrings(value, voiceSIPURIMax, calls, byID, func(call *VoiceCall, value string) {
			call.SIPURI = value
			call.SIPURIKnown = true
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice call SIP URIs: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.ipName); ok {
		if err := applyVoiceCallUTF16(value, 128, calls, byID, func(call *VoiceCall, value []uint16) {
			call.CallerName = string(utf16.Decode(value))
			call.CallerNameKnown = true
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice IP caller names: %w", err)
		}
	}
	if kinds.namePI != 0 {
		if value, ok := tlv.Value(tlvs, kinds.namePI); ok {
			if err := applyVoiceCallRecords(value, 2, calls, byID, func(call *VoiceCall, record []byte) {
				call.CallerNamePresentation = VoicePresentation(record[1])
				call.CallerNamePresentationKnown = true
			}); err != nil {
				return fmt.Errorf("parsing QMI Voice caller name presentations: %w", err)
			}
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.endText); ok {
		if err := applyVoiceCallUTF16(value, 128, calls, byID, func(call *VoiceCall, value []uint16) {
			call.EndReasonText = value
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice call end reason text: %w", err)
		}
	}
	if kinds.secure != 0 {
		if value, ok := tlv.Value(tlvs, kinds.secure); ok {
			if err := applyVoiceCallRecords(value, 2, calls, byID, func(call *VoiceCall, record []byte) {
				call.Secure = record[1] == 1
				call.SecureKnown = true
			}); err != nil {
				return fmt.Errorf("parsing QMI Voice secure calls: %w", err)
			}
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.sipError); ok {
		if err := applyVoiceCallRecords(value, 3, calls, byID, func(call *VoiceCall, record []byte) {
			call.SIPErrorCode = binary.LittleEndian.Uint16(record[1:])
			call.SIPErrorCodeKnown = true
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice call SIP errors: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.rtt); ok {
		if err := applyVoiceCallRecords(value, 5, calls, byID, func(call *VoiceCall, record []byte) {
			call.RTTMode = VoiceRTTMode(binary.LittleEndian.Uint32(record[1:]))
			call.RTTModeKnown = true
		}); err != nil {
			return fmt.Errorf("parsing QMI Voice call RTT modes: %w", err)
		}
	}
	return nil
}

func applyVoiceCallRecords(value []byte, recordSize int, calls []VoiceCall, byID map[uint8]int, apply func(*VoiceCall, []byte)) error {
	if len(value) < 1 {
		return errors.New("record count is missing")
	}
	count := int(value[0])
	value = value[1:]
	if count > voiceCallInfoMax || len(value) != count*recordSize {
		return errors.New("records are truncated")
	}
	for range count {
		record := value[:recordSize]
		if index, ok := byID[record[0]]; ok {
			apply(&calls[index], record)
		}
		value = value[recordSize:]
	}
	return nil
}

func applyVoiceCallStrings(value []byte, maxLength int, calls []VoiceCall, byID map[uint8]int, apply func(*VoiceCall, string)) error {
	if len(value) < 1 {
		return errors.New("record count is missing")
	}
	count := int(value[0])
	value = value[1:]
	if count > voiceCallInfoMax {
		return errors.New("record count exceeds protocol maximum")
	}
	for range count {
		if len(value) < 2 {
			return errors.New("record header is truncated")
		}
		callID := value[0]
		length := int(value[1])
		value = value[2:]
		if length > maxLength || len(value) < length {
			return errors.New("record value is truncated")
		}
		if index, ok := byID[callID]; ok {
			apply(&calls[index], string(value[:length]))
		}
		value = value[length:]
	}
	if len(value) != 0 {
		return errors.New("records contain trailing data")
	}
	return nil
}

func applyVoiceCallUTF16(value []byte, maxLength int, calls []VoiceCall, byID map[uint8]int, apply func(*VoiceCall, []uint16)) error {
	if len(value) < 1 {
		return errors.New("record count is missing")
	}
	count := int(value[0])
	value = value[1:]
	if count > voiceCallInfoMax {
		return errors.New("record count exceeds protocol maximum")
	}
	for range count {
		if len(value) < 2 {
			return errors.New("record header is truncated")
		}
		callID := value[0]
		length := int(value[1])
		value = value[2:]
		if length > maxLength || len(value) < length*2 {
			return errors.New("record value is truncated")
		}
		text := make([]uint16, length)
		for i := range length {
			text[i] = binary.LittleEndian.Uint16(value[i*2:])
		}
		if index, ok := byID[callID]; ok {
			apply(&calls[index], text)
		}
		value = value[length*2:]
	}
	if len(value) != 0 {
		return errors.New("records contain trailing data")
	}
	return nil
}

func applyVoiceCallerNames(value []byte, calls []VoiceCall, byID map[uint8]int) error {
	if len(value) < 1 {
		return errors.New("record count is missing")
	}
	count := int(value[0])
	value = value[1:]
	if count > voiceCallInfoMax {
		return errors.New("record count exceeds protocol maximum")
	}
	for range count {
		if len(value) < 4 {
			return errors.New("record header is truncated")
		}
		callID := value[0]
		presentation := VoicePresentation(value[1])
		codingScheme := value[2]
		length := int(value[3])
		value = value[4:]
		if length > 182 || len(value) < length {
			return errors.New("record value is truncated")
		}
		if index, ok := byID[callID]; ok {
			call := &calls[index]
			call.CallerName = string(value[:length])
			call.CallerNameKnown = true
			call.CallerNamePresentation = presentation
			call.CallerNamePresentationKnown = true
			call.CallerNameCodingScheme = codingScheme
			call.CallerNameCodingSchemeKnown = true
		}
		value = value[length:]
	}
	if len(value) != 0 {
		return errors.New("records contain trailing data")
	}
	return nil
}

func decodeVoiceSingleCallerName(value []byte, call *VoiceCall) error {
	if len(value) < 3 {
		return errors.New("record header is truncated")
	}
	length := int(value[2])
	if length > 182 || len(value) != 3+length {
		return errors.New("record value is truncated")
	}
	call.CallerNamePresentation = VoicePresentation(value[0])
	call.CallerNamePresentationKnown = true
	call.CallerNameCodingScheme = value[1]
	call.CallerNameCodingSchemeKnown = true
	call.CallerName = string(value[3:])
	call.CallerNameKnown = true
	return nil
}

func validateVoiceDTMFDigits(digits string, maxLength int) error {
	if digits == "" {
		return errors.New("digits are empty")
	}
	if len(digits) > maxLength {
		return fmt.Errorf("digit length %d exceeds %d", len(digits), maxLength)
	}
	for i := range len(digits) {
		if !validVoiceDTMFDigit(digits[i]) {
			return fmt.Errorf("invalid digit %q", digits[i])
		}
	}
	return nil
}

func validVoiceDTMFDigit(digit byte) bool {
	return digit >= '0' && digit <= '9' || digit == '*' || digit == '#' || digit >= 'A' && digit <= 'D' || digit >= 'a' && digit <= 'd'
}

// MarshalBinary encodes QMI Voice USSD data.
func (data VoiceUSSDData) MarshalBinary() ([]byte, error) {
	if len(data.Data) == 0 {
		return nil, errors.New("USSD data is empty")
	}
	if len(data.Data) > voiceUSSDDataMax {
		return nil, fmt.Errorf("USSD data length %d exceeds %d", len(data.Data), voiceUSSDDataMax)
	}
	if data.Encoding < VoiceUSSDEncodingASCII || data.Encoding > VoiceUSSDEncodingUCS2 {
		return nil, fmt.Errorf("USSD encoding 0x%02X is invalid", data.Encoding)
	}
	value := []byte{byte(data.Encoding), byte(len(data.Data))}
	return append(value, data.Data...), nil
}

// UnmarshalBinary decodes QMI Voice USSD data.
func (data *VoiceUSSDData) UnmarshalBinary(value []byte) error {
	if len(value) < 2 {
		return errors.New("parsing QMI Voice USSD data: header is truncated")
	}
	encoding := VoiceUSSDEncoding(value[0])
	if encoding < VoiceUSSDEncodingASCII || encoding > VoiceUSSDEncodingUCS2 {
		return fmt.Errorf("parsing QMI Voice USSD data: encoding 0x%02X is invalid", value[0])
	}
	length := int(value[1])
	if length > voiceUSSDDataMax || len(value) != 2+length {
		return fmt.Errorf("parsing QMI Voice USSD data: value length %d, want %d", len(value), 2+length)
	}
	*data = VoiceUSSDData{Encoding: encoding, Data: slices.Clone(value[2:])}
	return nil
}

// UnmarshalTLVs decodes a QMI Voice USSD response.
func (r *VoiceUSSDResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result VoiceUSSDResult
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD response: failure cause TLV length %d, want 2", len(value))
		}
		result.FailureCause = binary.LittleEndian.Uint16(value)
		result.FailureCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		alpha, err := decodeVoiceAlphaIdentifier(value)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD response: %w", err)
		}
		result.Alpha = alpha
		result.AlphaKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		var data VoiceUSSDData
		if err := data.UnmarshalBinary(value); err != nil {
			return err
		}
		result.Data = data
		result.DataKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice USSD response: call-control result TLV length %d, want 1", len(value))
		}
		result.CallControlResult = VoiceCallControlResultType(value[0])
		result.CallControlResultKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice USSD response: call ID TLV length %d, want 1", len(value))
		}
		result.CallID = value[0]
		result.CallIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice USSD response: supplementary service TLV length %d, want 1", len(value))
		}
		result.CallControlSupplementaryService = VoiceSupplementaryServiceType(value[0])
		result.CallControlSupplementaryServiceKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		utf16Value, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD response: UTF-16 data: %w", err)
		}
		result.UTF16 = utf16Value
	}
	if value, ok := tlv.Value(tlvs, 0x18); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD response: SIP error TLV length %d, want 2", len(value))
		}
		result.SIPErrorCode = binary.LittleEndian.Uint16(value)
		result.SIPErrorCodeKnown = true
	}
	*r = result
	return nil
}

// UnmarshalTLVs decodes a QMI Voice USSD indication.
func (e *VoiceUSSDEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) != 1 {
		return errors.New("parsing QMI Voice USSD indication: action TLV missing")
	}
	event := VoiceUSSDEvent{Action: VoiceUSSDAction(value[0])}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		var data VoiceUSSDData
		if err := data.UnmarshalBinary(value); err != nil {
			return err
		}
		event.Data = data
		event.DataKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		utf16Value, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD indication: UTF-16 data: %w", err)
		}
		event.UTF16 = utf16Value
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		failureText, err := decodeVoiceUint16Array(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD indication: failure text: %w", err)
		}
		event.FailureCauseText = failureText
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD indication: SIP error TLV length %d, want 2", len(value))
		}
		event.SIPErrorCode = binary.LittleEndian.Uint16(value)
		event.SIPErrorCodeKnown = true
	}
	*e = event
	return nil
}

func decodeVoiceUint16Array(value []byte, length16 bool) ([]uint16, error) {
	prefix := 1
	if length16 {
		prefix = 2
	}
	if len(value) < prefix {
		return nil, errors.New("length is truncated")
	}
	var count int
	if length16 {
		count = int(binary.LittleEndian.Uint16(value[:2]))
	} else {
		count = int(value[0])
	}
	value = value[prefix:]
	if len(value) != count*2 {
		return nil, fmt.Errorf("value length %d, want %d", len(value), count*2)
	}
	out := make([]uint16, count)
	for i := range count {
		out[i] = binary.LittleEndian.Uint16(value[i*2 : i*2+2])
	}
	return out, nil
}

func decodeVoiceAlphaIdentifier(value []byte) (VoiceAlphaIdentifier, error) {
	if len(value) < 2 {
		return VoiceAlphaIdentifier{}, errors.New("parsing QMI Voice alpha identifier: header is truncated")
	}
	encoding := VoiceAlphaEncoding(value[0])
	if encoding != VoiceAlphaEncodingGSM && encoding != VoiceAlphaEncodingUCS2 {
		return VoiceAlphaIdentifier{}, fmt.Errorf("parsing QMI Voice alpha identifier: encoding %d is unsupported", encoding)
	}
	length := int(value[1])
	if len(value) != 2+length {
		return VoiceAlphaIdentifier{}, fmt.Errorf("parsing QMI Voice alpha identifier: value length %d, want %d", len(value), 2+length)
	}
	return VoiceAlphaIdentifier{Encoding: encoding, Data: slices.Clone(value[2:])}, nil
}

func voiceUint64TLV(kind byte, value uint64) tlv.TLV {
	return tlv.Bytes(kind, binary.LittleEndian.AppendUint64(nil, value))
}

func parseVoiceOptionalUint32(tlvs tlv.TLVs, kind byte, dst *uint32, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI Voice TLV 0x%02X: length %d, want 4", kind, len(value))
	}
	*dst = binary.LittleEndian.Uint32(value)
	*known = true
	return nil
}
