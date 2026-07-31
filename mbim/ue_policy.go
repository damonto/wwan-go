package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type URSPTrafficDescriptor struct {
	Precedence uint8
	Data       []byte
}

// URSPRules is the traffic-descriptor list carried by a URSP rules TLV.
type URSPRules []URSPTrafficDescriptor

type UEPolicyRequest struct {
	TransactionID uint32
	Query         TLVs
	Response      *UEPolicyInfo
}

func (r *UEPolicyRequest) Request() *Request {
	data, err := r.Query.MarshalBinary()
	r.Response = new(UEPolicyInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsUEPolicy,
			CommandTypeQuery,
			data,
			err,
		),
		Response: r.Response,
	}
}

type UEPolicyInfo struct {
	TLVs         TLVs
	URSPRules    URSPRules
	HasURSPRules bool
}

func (d URSPTrafficDescriptor) MarshalBinary() ([]byte, error) {
	if len(d.Data) == 0 {
		return nil, errors.New("encoding URSP traffic descriptor: traffic descriptor is empty")
	}
	if len(d.Data) > int(^uint16(0)) {
		return nil, errors.New("encoding URSP traffic descriptor: traffic descriptor exceeds UINT16 length")
	}

	data := []byte{d.Precedence}
	data = binary.BigEndian.AppendUint16(data, uint16(len(d.Data)))
	return append(data, d.Data...), nil
}

func (d *URSPTrafficDescriptor) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalURSPTrafficDescriptorPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing URSP traffic descriptor: trailing data")
	}
	*d = value
	return nil
}

func (i UEPolicyInfo) MarshalBinary() ([]byte, error) {
	if err := validateURSPRulesTLVs(i.TLVs); err != nil {
		return nil, fmt.Errorf("encoding MBIM UE policy: %w", err)
	}
	data, err := i.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding MBIM UE policy TLVs: %w", err)
	}
	return data, nil
}

func (i *UEPolicyInfo) UnmarshalBinary(data []byte) error {
	var tlvs TLVs
	if err := tlvs.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("parsing MBIM UE policy TLVs: %w", err)
	}
	if err := validateURSPRulesTLVs(tlvs); err != nil {
		return fmt.Errorf("parsing MBIM UE policy: %w", err)
	}

	result := UEPolicyInfo{TLVs: tlvs}
	for _, tlv := range tlvs {
		if tlv.Type != TLVTypeURSPRulesTDOnly {
			continue
		}
		var rules URSPRules
		if err := rules.UnmarshalTLV(tlv); err != nil {
			return fmt.Errorf("parsing MBIM UE policy URSP rules: %w", err)
		}
		result.URSPRules = rules
		result.HasURSPRules = true
	}
	*i = result
	return nil
}

func validateURSPRulesTLVs(tlvs TLVs) error {
	count := 0
	for _, tlv := range tlvs {
		if tlv.Type != TLVTypeURSPRulesTDOnly {
			continue
		}
		count++
		if count > 1 {
			return errors.New("more than one URSP rules TLV")
		}
		var rules URSPRules
		if err := rules.UnmarshalTLV(tlv); err != nil {
			return fmt.Errorf("invalid URSP rules TLV: %w", err)
		}
	}
	if count == 0 {
		return errors.New("URSP rules TLV is missing")
	}
	return nil
}

func NewURSPRulesTDOnlyTLV(rules URSPRules) (TLV, error) {
	data, err := marshalURSPTrafficDescriptors(rules)
	if err != nil {
		return TLV{}, fmt.Errorf("encoding URSP rules traffic descriptors TLV: %w", err)
	}
	return TLV{Type: TLVTypeURSPRulesTDOnly, Data: data}, nil
}

// UnmarshalTLV decodes a traffic-descriptor-only URSP rules TLV.
func (r *URSPRules) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeURSPRulesTDOnly {
		return fmt.Errorf("parsing URSP rules traffic descriptors TLV: type is %d, want %d", tlv.Type, TLVTypeURSPRulesTDOnly)
	}
	rules, err := unmarshalURSPTrafficDescriptors(tlv.Data)
	if err != nil {
		return fmt.Errorf("parsing URSP rules traffic descriptors TLV: %w", err)
	}
	*r = rules
	return nil
}

func marshalURSPTrafficDescriptors(rules URSPRules) ([]byte, error) {
	var data []byte
	var precedences [256]bool
	for i, rule := range rules {
		if precedences[rule.Precedence] {
			return nil, fmt.Errorf("URSP rule %d duplicates precedence %d", i, rule.Precedence)
		}
		precedences[rule.Precedence] = true
		encoded, err := rule.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding URSP rule %d: %w", i, err)
		}
		data = append(data, encoded...)
	}
	return data, nil
}

func unmarshalURSPTrafficDescriptors(data []byte) (URSPRules, error) {
	var rules URSPRules
	var precedences [256]bool
	for len(data) > 0 {
		rule, consumed, err := unmarshalURSPTrafficDescriptorPrefix(data)
		if err != nil {
			return nil, fmt.Errorf("parsing URSP rule %d: %w", len(rules), err)
		}
		if precedences[rule.Precedence] {
			return nil, fmt.Errorf("duplicate precedence %d", rule.Precedence)
		}
		precedences[rule.Precedence] = true
		rules = append(rules, rule)
		data = data[consumed:]
	}
	return rules, nil
}

func unmarshalURSPTrafficDescriptorPrefix(data []byte) (URSPTrafficDescriptor, int, error) {
	if len(data) < 3 {
		return URSPTrafficDescriptor{}, 0, errors.New("parsing URSP traffic descriptor: header is truncated")
	}
	length := int(binary.BigEndian.Uint16(data[1:3]))
	if length == 0 {
		return URSPTrafficDescriptor{}, 0, errors.New("parsing URSP traffic descriptor: traffic descriptor is empty")
	}
	totalLength := 3 + length
	if totalLength > len(data) {
		return URSPTrafficDescriptor{}, 0, errors.New("parsing URSP traffic descriptor: traffic descriptor is truncated")
	}
	return URSPTrafficDescriptor{
		Precedence: data[0],
		Data:       append([]byte(nil), data[3:totalLength]...),
	}, totalLength, nil
}

func (c *Client) UEPolicy(ctx context.Context, query TLVs) (UEPolicyInfo, error) {
	if c.mbimExVersion < mbimExVersion40 {
		return UEPolicyInfo{}, errors.New("reading MBIM UE policy: CID requires MBIMEx 4.0")
	}
	if _, err := query.MarshalBinary(); err != nil {
		return UEPolicyInfo{}, fmt.Errorf("encoding MBIM UE policy query: %w", err)
	}
	request := UEPolicyRequest{
		TransactionID: c.nextTransactionID(),
		Query:         query,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return UEPolicyInfo{}, fmt.Errorf("reading MBIM UE policy: %w", err)
	}
	return *request.Response, nil
}
