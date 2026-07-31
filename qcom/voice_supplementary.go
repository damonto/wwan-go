package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const voiceFlashPayloadMax = 81

// VoiceFlashType identifies a 3GPP2 flash operation.
type VoiceFlashType uint8

const (
	VoiceFlashSimple VoiceFlashType = iota
	VoiceFlashActivateAnswerHold
	VoiceFlashDeactivateAnswerHold
)

// VoiceFlashRequest contains the call and optional flash payload.
type VoiceFlashRequest struct {
	CallID  uint8
	Payload string
	Type    *VoiceFlashType
}

// VoiceSupplementaryAction identifies a network supplementary-service action.
type VoiceSupplementaryAction uint8

const (
	VoiceSupplementaryActivate   VoiceSupplementaryAction = 0x01
	VoiceSupplementaryDeactivate VoiceSupplementaryAction = 0x02
	VoiceSupplementaryRegister   VoiceSupplementaryAction = 0x03
	VoiceSupplementaryErase      VoiceSupplementaryAction = 0x04
)

// VoiceSupplementaryReason identifies a forwarding, barring, or line service.
type VoiceSupplementaryReason uint8

const (
	VoiceReasonForwardUnconditional VoiceSupplementaryReason = 0x01
	VoiceReasonForwardBusy          VoiceSupplementaryReason = 0x02
	VoiceReasonForwardNoReply       VoiceSupplementaryReason = 0x03
	VoiceReasonForwardUnreachable   VoiceSupplementaryReason = 0x04
	VoiceReasonForwardAll           VoiceSupplementaryReason = 0x05
	VoiceReasonForwardConditional   VoiceSupplementaryReason = 0x06
	VoiceReasonBarAllOutgoing       VoiceSupplementaryReason = 0x07
	VoiceReasonBarOutgoingIntl      VoiceSupplementaryReason = 0x08
	VoiceReasonBarOutgoingIntlHome  VoiceSupplementaryReason = 0x09
	VoiceReasonBarAllIncoming       VoiceSupplementaryReason = 0x0A
	VoiceReasonBarIncomingRoaming   VoiceSupplementaryReason = 0x0B
	VoiceReasonBarAll               VoiceSupplementaryReason = 0x0C
	VoiceReasonBarAllOutgoingGroup  VoiceSupplementaryReason = 0x0D
	VoiceReasonBarAllIncomingGroup  VoiceSupplementaryReason = 0x0E
	VoiceReasonCallWaiting          VoiceSupplementaryReason = 0x0F
	VoiceReasonCLIP                 VoiceSupplementaryReason = 0x10
	VoiceReasonCLIR                 VoiceSupplementaryReason = 0x11
	VoiceReasonCOLP                 VoiceSupplementaryReason = 0x12
	VoiceReasonCOLR                 VoiceSupplementaryReason = 0x13
	VoiceReasonCNAP                 VoiceSupplementaryReason = 0x14
	VoiceReasonBarIncomingNumber    VoiceSupplementaryReason = 0x15
	VoiceReasonBarIncomingAnonymous VoiceSupplementaryReason = 0x16
)

// VoiceServiceClass is the extended supplementary-service class bitmask.
type VoiceServiceClass uint32

const (
	VoiceServiceClassVoice VoiceServiceClass = 0x0001
	VoiceServiceClassData  VoiceServiceClass = 0x0002
	VoiceServiceClassFax   VoiceServiceClass = 0x0004
	VoiceServiceClassSMS   VoiceServiceClass = 0x0008
)

// VoiceActiveStatus reports whether a supplementary service is active.
type VoiceActiveStatus uint8

const (
	VoiceStatusInactive VoiceActiveStatus = iota
	VoiceStatusActive
)

// VoiceProvisionStatus reports how a supplementary service is provisioned.
type VoiceProvisionStatus uint8

const (
	VoiceNotProvisioned VoiceProvisionStatus = iota
	VoiceProvisionedPermanently
	VoiceProvisionPresentationRestricted
	VoiceProvisionPresentationAllowed
)

// VoiceSupplementaryQuery selects a legacy or extended service class.
type VoiceSupplementaryQuery struct {
	ServiceClass         *uint8
	ExtendedServiceClass *VoiceServiceClass
}

// VoiceSetSupplementaryRequest changes one network supplementary service.
type VoiceSetSupplementaryRequest struct {
	Action               VoiceSupplementaryAction
	Reason               VoiceSupplementaryReason
	ServiceClass         *uint8
	ExtendedServiceClass *VoiceServiceClass
	Password             string
	Number               string
	NoReplyTimer         *uint8
}

// VoiceSupplementaryResult contains common network response details.
type VoiceSupplementaryResult struct {
	FailureCause      uint16
	FailureCauseKnown bool
	Active            VoiceActiveStatus
	Provision         VoiceProvisionStatus
	StatusKnown       bool
	RetryDuration     uint16
	RetryKnown        bool
	SIPErrorCode      uint16
	SIPErrorCodeKnown bool
}

// VoiceCallWaitingStatus describes call-waiting provisioning and classes.
type VoiceCallWaitingStatus struct {
	VoiceSupplementaryResult
	ServiceClass         uint8
	ServiceClassKnown    bool
	ExtendedServiceClass VoiceServiceClass
	ExtendedClassKnown   bool
}

// VoiceCallBarringStatus describes call-barring service classes and failures.
type VoiceCallBarringStatus struct {
	VoiceSupplementaryResult
	ServiceClass         uint8
	ServiceClassKnown    bool
	ExtendedServiceClass VoiceServiceClass
	ExtendedClassKnown   bool
}

// VoiceLineStatus describes provisioning for a line-identification service.
type VoiceLineStatus struct {
	VoiceSupplementaryResult
}

// VoiceCOLRPresentation identifies connected-line presentation restriction.
type VoiceCOLRPresentation uint32

const (
	VoiceCOLRNotRestricted VoiceCOLRPresentation = iota
	VoiceCOLRRestricted
)

// VoiceCOLRStatus contains COLR provisioning and presentation policy.
type VoiceCOLRStatus struct {
	VoiceLineStatus
	Presentation      VoiceCOLRPresentation
	PresentationKnown bool
}

// VoiceCallForwardingRule contains one network call-forwarding rule.
type VoiceCallForwardingRule struct {
	Active               bool
	ServiceClass         VoiceServiceClass
	ExtendedServiceClass bool
	Number               string
	Presentation         VoicePresentation
	Screening            uint8
	NumberType           uint8
	NumberPlan           uint8
	NoReplyTimer         uint8
}

// VoiceCallForwardingRules is a network-provided call-forwarding rule list.
type VoiceCallForwardingRules []VoiceCallForwardingRule

// VoiceCallForwardingStatus contains call-forwarding rules and common errors.
type VoiceCallForwardingStatus struct {
	VoiceSupplementaryResult
	Rules VoiceCallForwardingRules
}

// UnmarshalTLVs parses a call-waiting response.
func (s *VoiceCallWaitingStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = VoiceCallWaitingStatus{}
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x11, retry: 0x17, sip: 0x19}); err != nil {
		return err
	}
	s.VoiceSupplementaryResult = result
	if err := decodeVoiceServiceClasses(tlvs, &s.ServiceClass, &s.ServiceClassKnown, &s.ExtendedServiceClass, &s.ExtendedClassKnown); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x1A); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice call waiting: status TLV length %d, want 2", len(value))
		}
		s.Active = VoiceActiveStatus(value[0])
		s.Provision = VoiceProvisionStatus(value[1])
		s.StatusKnown = true
	}
	return nil
}

// UnmarshalTLVs parses a call-barring response.
func (s *VoiceCallBarringStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = VoiceCallBarringStatus{}
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x11, retry: 0x18, sip: 0x1A}); err != nil {
		return err
	}
	s.VoiceSupplementaryResult = result
	return decodeVoiceServiceClasses(tlvs, &s.ServiceClass, &s.ServiceClassKnown, &s.ExtendedServiceClass, &s.ExtendedClassKnown)
}

// UnmarshalTLVs parses a line-identification response.
func (s *VoiceLineStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = VoiceLineStatus{}
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x11, status: 0x10, retry: 0x16, sip: 0x18}); err != nil {
		return err
	}
	s.VoiceSupplementaryResult = result
	return nil
}

// UnmarshalTLVs parses a connected-line restriction response.
func (s *VoiceCOLRStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = VoiceCOLRStatus{}
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x11, status: 0x10, retry: 0x17, sip: 0x19}); err != nil {
		return err
	}
	s.VoiceSupplementaryResult = result
	var presentation uint32
	if err := parseVoiceOptionalUint32(tlvs, 0x16, &presentation, &s.PresentationKnown); err != nil {
		return err
	}
	s.Presentation = VoiceCOLRPresentation(presentation)
	return nil
}

// UnmarshalTLVs parses a call-forwarding response.
func (s *VoiceCallForwardingStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = VoiceCallForwardingStatus{}
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x11, retry: 0x18, sip: 0x1D}); err != nil {
		return err
	}
	s.VoiceSupplementaryResult = result
	return s.Rules.UnmarshalTLVs(tlvs)
}

type voiceBarringPasswordResult VoiceSupplementaryResult

func (r *voiceBarringPasswordResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result VoiceSupplementaryResult
	err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{failure: 0x10, retry: 0x15, sip: 0x17})
	if err != nil {
		return err
	}
	*r = voiceBarringPasswordResult(result)
	return nil
}

// VoiceSendFlash sends a 3GPP2 flash operation.
func (c *Client) VoiceSendFlash(ctx context.Context, req VoiceFlashRequest) error {
	if len(req.Payload) > voiceFlashPayloadMax {
		return fmt.Errorf("sending QMI Voice flash: payload length %d exceeds %d", len(req.Payload), voiceFlashPayloadMax)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, req.CallID)}
	if req.Payload != "" {
		tlvs = append(tlvs, tlv.Bytes(0x10, []byte(req.Payload)))
	}
	if req.Type != nil {
		if *req.Type > VoiceFlashDeactivateAnswerHold {
			return fmt.Errorf("sending QMI Voice flash: type %d is out of range", *req.Type)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*req.Type)))
	}
	if err := c.voiceResultRequest(ctx, MessageVoiceSendFlash, tlvs); err != nil {
		return fmt.Errorf("sending QMI Voice flash: %w", err)
	}
	return nil
}

// VoiceSetSupplementaryService activates, deactivates, registers, or erases a service.
func (c *Client) VoiceSetSupplementaryService(ctx context.Context, req VoiceSetSupplementaryRequest) (VoiceSupplementaryResult, error) {
	tlvs := tlv.TLVs{tlv.Bytes(0x01, []byte{byte(req.Action), byte(req.Reason)})}
	if req.ServiceClass != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *req.ServiceClass))
	}
	if req.Password != "" {
		if !validVoicePassword(req.Password) {
			return VoiceSupplementaryResult{}, errors.New("setting QMI Voice supplementary service: password must contain exactly four ASCII digits")
		}
		tlvs = append(tlvs, tlv.Bytes(0x11, []byte(req.Password)))
	}
	if req.Number != "" {
		number := strings.TrimSpace(req.Number)
		if number == "" {
			return VoiceSupplementaryResult{}, errors.New("setting QMI Voice supplementary service: number is empty")
		}
		if len(number) > voiceNumberMax {
			return VoiceSupplementaryResult{}, fmt.Errorf("setting QMI Voice supplementary service: number length %d exceeds %d", len(number), voiceNumberMax)
		}
		tlvs = append(tlvs, tlv.Bytes(0x12, []byte(number)))
	}
	if req.NoReplyTimer != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, *req.NoReplyTimer))
	}
	if req.ExtendedServiceClass != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, uint32(*req.ExtendedServiceClass)))
	}

	var parsed voiceSetSupplementaryResponse
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceSetSupplementary, tlvs)
		if err != nil {
			return err
		}
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return parsed.Result, fmt.Errorf("setting QMI Voice supplementary service: %w", err)
	}
	return parsed.Result, nil
}

type voiceSetSupplementaryResponse struct {
	Result VoiceSupplementaryResult
}

func (r *voiceSetSupplementaryResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result VoiceSupplementaryResult
	if err := result.unmarshalTLVs(tlvs, voiceSupplementaryTLVs{
		failure: 0x10,
		status:  0x15,
		retry:   0x17,
		sip:     0x18,
	}); err != nil {
		return err
	}
	*r = voiceSetSupplementaryResponse{Result: result}
	return nil
}

// VoiceCallWaiting queries call-waiting state.
func (c *Client) VoiceCallWaiting(ctx context.Context, query VoiceSupplementaryQuery) (VoiceCallWaitingStatus, error) {
	var status VoiceCallWaitingStatus
	err := c.voiceSupplementaryQuery(ctx, MessageVoiceGetCallWaiting, query, &status)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice call waiting: %w", err)
	}
	return status, nil
}

// VoiceCallBarring queries call-barring state for one reason.
func (c *Client) VoiceCallBarring(ctx context.Context, reason VoiceSupplementaryReason, query VoiceSupplementaryQuery) (VoiceCallBarringStatus, error) {
	var status VoiceCallBarringStatus
	tlvs := append(tlv.TLVs{tlv.Uint(0x01, uint8(reason))}, encodeVoiceSupplementaryQuery(query)...)
	err := c.voiceSupplementaryRead(ctx, MessageVoiceGetCallBarring, tlvs, &status)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice call barring: %w", err)
	}
	return status, nil
}

// VoiceCLIP queries calling-line identification presentation state.
func (c *Client) VoiceCLIP(ctx context.Context) (VoiceLineStatus, error) {
	status, err := c.voiceLineStatus(ctx, MessageVoiceGetCLIP)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice CLIP: %w", err)
	}
	return status, nil
}

// VoiceCLIRStatus queries calling-line identification restriction state.
func (c *Client) VoiceCLIRStatus(ctx context.Context) (VoiceLineStatus, error) {
	status, err := c.voiceLineStatus(ctx, MessageVoiceGetCLIR)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice CLIR: %w", err)
	}
	return status, nil
}

// VoiceCOLP queries connected-line identification presentation state.
func (c *Client) VoiceCOLP(ctx context.Context) (VoiceLineStatus, error) {
	status, err := c.voiceLineStatus(ctx, MessageVoiceGetCOLP)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice COLP: %w", err)
	}
	return status, nil
}

// VoiceCOLR queries connected-line identification restriction state.
func (c *Client) VoiceCOLR(ctx context.Context) (VoiceCOLRStatus, error) {
	var status VoiceCOLRStatus
	err := c.voiceSupplementaryRead(ctx, MessageVoiceGetCOLR, nil, &status)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice COLR: %w", err)
	}
	return status, nil
}

// VoiceCNAP queries calling-name presentation state.
func (c *Client) VoiceCNAP(ctx context.Context) (VoiceLineStatus, error) {
	status, err := c.voiceLineStatus(ctx, MessageVoiceGetCNAP)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice CNAP: %w", err)
	}
	return status, nil
}

func (c *Client) voiceLineStatus(ctx context.Context, id MessageID) (VoiceLineStatus, error) {
	var status VoiceLineStatus
	err := c.voiceSupplementaryRead(ctx, id, nil, &status)
	return status, err
}

// VoiceCallForwarding queries call-forwarding rules for one reason.
func (c *Client) VoiceCallForwarding(ctx context.Context, reason VoiceSupplementaryReason, query VoiceSupplementaryQuery) (VoiceCallForwardingStatus, error) {
	var status VoiceCallForwardingStatus
	tlvs := append(tlv.TLVs{tlv.Uint(0x01, uint8(reason))}, encodeVoiceSupplementaryQuery(query)...)
	err := c.voiceSupplementaryRead(ctx, MessageVoiceGetCallForwarding, tlvs, &status)
	if err != nil {
		return status, fmt.Errorf("reading QMI Voice call forwarding: %w", err)
	}
	return status, nil
}

// VoiceSetCallBarringPassword changes the four-digit network barring password.
func (c *Client) VoiceSetCallBarringPassword(ctx context.Context, reason VoiceSupplementaryReason, oldPassword, newPassword string) (VoiceSupplementaryResult, error) {
	if !validVoicePassword(oldPassword) || !validVoicePassword(newPassword) {
		return VoiceSupplementaryResult{}, errors.New("setting QMI Voice call-barring password: passwords must contain exactly four ASCII digits")
	}
	value := []byte{byte(reason)}
	value = append(value, oldPassword...)
	value = append(value, newPassword...)
	value = append(value, newPassword...)
	var decoded voiceBarringPasswordResult
	err := c.voiceSupplementaryRead(ctx, MessageVoiceSetBarringPassword, tlv.TLVs{tlv.Bytes(0x01, value)}, &decoded)
	result := VoiceSupplementaryResult(decoded)
	if err != nil {
		return result, fmt.Errorf("setting QMI Voice call-barring password: %w", err)
	}
	return result, nil
}

func (c *Client) voiceSupplementaryQuery(ctx context.Context, id MessageID, query VoiceSupplementaryQuery, result tlvUnmarshaler) error {
	tlvs := encodeVoiceSupplementaryQuery(query)
	return c.voiceSupplementaryRead(ctx, id, tlvs, result)
}

func (c *Client) voiceSupplementaryRead(ctx context.Context, id MessageID, request tlv.TLVs, result tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, id, request)
		if err != nil {
			return err
		}
		if err := result.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		return resultOK(resp)
	})
}

// UnmarshalTLVs parses legacy or extended call-forwarding rule TLVs.
func (r *VoiceCallForwardingRules) UnmarshalTLVs(tlvs tlv.TLVs) error {
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		rules, err := decodeVoiceForwardingExtended(value, true)
		if err != nil {
			return err
		}
		*r = rules
		return nil
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		rules, err := decodeVoiceForwardingExtended(value, false)
		if err != nil {
			return err
		}
		*r = rules
		return nil
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		rules, err := decodeVoiceForwardingLegacy(value)
		if err != nil {
			return err
		}
		*r = rules
		return nil
	}
	*r = nil
	return nil
}

func decodeVoiceForwardingLegacy(value []byte) ([]VoiceCallForwardingRule, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI Voice call forwarding: count is missing")
	}
	count := int(value[0])
	if count > 13 {
		return nil, fmt.Errorf("parsing QMI Voice call forwarding: count %d exceeds 13", count)
	}
	offset := 1
	rules := make([]VoiceCallForwardingRule, count)
	for i := range count {
		if len(value)-offset < 4 {
			return nil, fmt.Errorf("parsing QMI Voice call forwarding rule %d: header is truncated", i)
		}
		length := int(value[offset+2])
		if length > voiceNumberMax || len(value)-offset < 4+length {
			return nil, fmt.Errorf("parsing QMI Voice call forwarding rule %d: number is truncated", i)
		}
		rules[i] = VoiceCallForwardingRule{
			Active:       value[offset] != 0,
			ServiceClass: VoiceServiceClass(value[offset+1]),
			Number:       string(value[offset+3 : offset+3+length]),
			NoReplyTimer: value[offset+3+length],
		}
		offset += 4 + length
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI Voice call forwarding: %d trailing bytes", len(value)-offset)
	}
	return rules, nil
}

func decodeVoiceForwardingExtended(value []byte, extendedClass bool) ([]VoiceCallForwardingRule, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI Voice extended call forwarding: count is missing")
	}
	count := int(value[0])
	if count > 13 {
		return nil, fmt.Errorf("parsing QMI Voice extended call forwarding: count %d exceeds 13", count)
	}
	offset := 1
	rules := make([]VoiceCallForwardingRule, count)
	for i := range count {
		classLength := 1
		if extendedClass {
			classLength = 4
		}
		metadataLength := 1 + classLength + 1 + 4 + 1
		if len(value)-offset < metadataLength {
			return nil, fmt.Errorf("parsing QMI Voice extended call forwarding rule %d: header is truncated", i)
		}
		rule := VoiceCallForwardingRule{Active: value[offset] != 0, ExtendedServiceClass: extendedClass}
		offset++
		if extendedClass {
			rule.ServiceClass = VoiceServiceClass(binary.LittleEndian.Uint32(value[offset : offset+4]))
			offset += 4
		} else {
			rule.ServiceClass = VoiceServiceClass(value[offset])
			offset++
		}
		rule.NoReplyTimer = value[offset]
		offset++
		rule.Presentation = VoicePresentation(value[offset])
		rule.Screening = value[offset+1]
		rule.NumberType = value[offset+2]
		rule.NumberPlan = value[offset+3]
		offset += 4
		length := int(value[offset])
		offset++
		if length > voiceNumberMax || len(value)-offset < length {
			return nil, fmt.Errorf("parsing QMI Voice extended call forwarding rule %d: number is truncated", i)
		}
		rule.Number = string(value[offset : offset+length])
		offset += length
		rules[i] = rule
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI Voice extended call forwarding: %d trailing bytes", len(value)-offset)
	}
	return rules, nil
}

func encodeVoiceSupplementaryQuery(query VoiceSupplementaryQuery) tlv.TLVs {
	var tlvs tlv.TLVs
	if query.ServiceClass != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *query.ServiceClass))
	}
	if query.ExtendedServiceClass != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, uint32(*query.ExtendedServiceClass)))
	}
	return tlvs
}

type voiceSupplementaryTLVs struct {
	failure byte
	status  byte
	retry   byte
	sip     byte
}

func (r *VoiceSupplementaryResult) unmarshalTLVs(tlvs tlv.TLVs, kinds voiceSupplementaryTLVs) error {
	var result VoiceSupplementaryResult
	if err := parseVoiceOptionalUint16(tlvs, kinds.failure, &result.FailureCause, &result.FailureCauseKnown); err != nil {
		return err
	}
	if kinds.status != 0 {
		if value, ok := tlv.Value(tlvs, kinds.status); ok {
			if len(value) != 2 {
				return fmt.Errorf("parsing QMI Voice supplementary status: TLV length %d, want 2", len(value))
			}
			result.Active = VoiceActiveStatus(value[0])
			result.Provision = VoiceProvisionStatus(value[1])
			result.StatusKnown = true
		}
	}
	if err := parseVoiceOptionalUint16(tlvs, kinds.retry, &result.RetryDuration, &result.RetryKnown); err != nil {
		return err
	}
	if err := parseVoiceOptionalUint16(tlvs, kinds.sip, &result.SIPErrorCode, &result.SIPErrorCodeKnown); err != nil {
		return err
	}
	*r = result
	return nil
}

func decodeVoiceServiceClasses(tlvs tlv.TLVs, legacy *uint8, legacyKnown *bool, extended *VoiceServiceClass, extendedKnown *bool) error {
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI Voice service class: TLV length %d, want 1", len(value))
		}
		*legacy = value[0]
		*legacyKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI Voice extended service class: TLV length %d, want 4", len(value))
		}
		*extended = VoiceServiceClass(binary.LittleEndian.Uint32(value))
		*extendedKnown = true
	}
	return nil
}

func parseVoiceOptionalUint16(tlvs tlv.TLVs, kind byte, dst *uint16, known *bool) error {
	if kind == 0 {
		return nil
	}
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI Voice TLV 0x%02X: length %d, want 2", kind, len(value))
	}
	*dst = binary.LittleEndian.Uint16(value)
	*known = true
	return nil
}

func validVoicePassword(password string) bool {
	if len(password) != 4 {
		return false
	}
	for i := range len(password) {
		if password[i] < '0' || password[i] > '9' {
			return false
		}
	}
	return true
}
