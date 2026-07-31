package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type AccessType uint32

const (
	AccessTypeUnknown AccessType = 0
	AccessType3GPP    AccessType = 1
	AccessTypeNon3GPP AccessType = 2
)

type SNSSAI struct {
	SliceServiceType             uint8
	SliceDifferentiator          [3]byte
	HasSliceDifferentiator       bool
	MappedSliceServiceType       uint8
	HasMappedSliceServiceType    bool
	MappedSliceDifferentiator    [3]byte
	HasMappedSliceDifferentiator bool
}

// OptionalSNSSAI preserves the empty-TLV meaning of an absent S-NSSAI.
type OptionalSNSSAI struct {
	Value *SNSSAI
}

// NSSAIList is an ordered list of S-NSSAI values.
type NSSAIList []SNSSAI

type PreconfiguredDefaultNSSAI struct {
	AccessType     AccessType
	PreferredNSSAI NSSAIList
}

// PreconfiguredDefaultNSSAIList groups preconfigured NSSAI by access type.
type PreconfiguredDefaultNSSAIList []PreconfiguredDefaultNSSAI

type RejectedNSSAICause uint8

const (
	RejectedNSSAINotAvailableInPLMN             RejectedNSSAICause = 0
	RejectedNSSAINotAvailableInRegistrationArea RejectedNSSAICause = 1
)

type RejectedSNSSAI struct {
	Cause  RejectedNSSAICause
	SNSSAI SNSSAI
}

// RejectedNSSAIList is the rejected S-NSSAI list carried by its MBIM TLV.
type RejectedNSSAIList []RejectedSNSSAI

func (r RejectedSNSSAI) MarshalBinary() ([]byte, error) {
	if err := validateRejectedNSSAICause(r.Cause); err != nil {
		return nil, fmt.Errorf("encoding rejected S-NSSAI: %w", err)
	}
	if r.SNSSAI.SliceServiceType < 1 || r.SNSSAI.SliceServiceType > 3 {
		return nil, fmt.Errorf("encoding rejected S-NSSAI: SST is %d, want 1, 2, or 3", r.SNSSAI.SliceServiceType)
	}
	if r.SNSSAI.HasMappedSliceServiceType || r.SNSSAI.HasMappedSliceDifferentiator {
		return nil, errors.New("encoding rejected S-NSSAI: mapped S-NSSAI is not allowed")
	}
	encoded, err := r.SNSSAI.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding rejected S-NSSAI: %w", err)
	}
	if encoded[0] != 1 && encoded[0] != 4 {
		return nil, fmt.Errorf("encoding rejected S-NSSAI: length is %d, want 1 or 4", encoded[0])
	}

	data := []byte{encoded[0], byte(r.Cause)}
	return append(data, encoded[1:]...), nil
}

func (r *RejectedSNSSAI) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalRejectedSNSSAIPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing rejected S-NSSAI: trailing data")
	}
	*r = value
	return nil
}

func (n PreconfiguredDefaultNSSAI) MarshalBinary() ([]byte, error) {
	if err := validateNSSAIAccessType(n.AccessType); err != nil {
		return nil, fmt.Errorf("encoding preconfigured default NSSAI: %w", err)
	}
	preferred, err := marshalPreconfiguredDefaultNSSAIList(n.PreferredNSSAI)
	if err != nil {
		return nil, fmt.Errorf("encoding preconfigured default NSSAI: %w", err)
	}
	preferredTLV, err := (TLV{Type: TLVTypeDefaultConfiguredNSSAI, Data: preferred}).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding preconfigured default NSSAI: %w", err)
	}

	data := binary.LittleEndian.AppendUint32(nil, uint32(n.AccessType))
	return append(data, preferredTLV...), nil
}

func (n *PreconfiguredDefaultNSSAI) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalPreconfiguredDefaultNSSAIPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing preconfigured default NSSAI: trailing data")
	}
	*n = value
	return nil
}

func (s SNSSAI) MarshalBinary() ([]byte, error) {
	if s.HasMappedSliceDifferentiator && (!s.HasSliceDifferentiator || !s.HasMappedSliceServiceType) {
		return nil, errors.New("encoding S-NSSAI: mapped SD requires SD and mapped SST")
	}
	return s.marshalBinaryUnchecked(), nil
}

func (s SNSSAI) marshalBinaryUnchecked() []byte {
	hasSliceDifferentiator := s.HasSliceDifferentiator || s.HasMappedSliceDifferentiator
	hasMappedSliceServiceType := s.HasMappedSliceServiceType || s.HasMappedSliceDifferentiator
	length := byte(1)
	switch {
	case s.HasMappedSliceDifferentiator:
		length = 8
	case hasSliceDifferentiator && hasMappedSliceServiceType:
		length = 5
	case hasSliceDifferentiator:
		length = 4
	case hasMappedSliceServiceType:
		length = 2
	}
	data := []byte{length, s.SliceServiceType}
	if hasSliceDifferentiator {
		data = append(data, s.SliceDifferentiator[:]...)
	}
	if hasMappedSliceServiceType {
		data = append(data, s.MappedSliceServiceType)
	}
	if s.HasMappedSliceDifferentiator {
		data = append(data, s.MappedSliceDifferentiator[:]...)
	}
	return data
}

func (s *SNSSAI) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return errors.New("parsing S-NSSAI: payload is truncated")
	}
	length := int(data[0])
	if length != 1 && length != 2 && length != 4 && length != 5 && length != 8 {
		return fmt.Errorf("parsing S-NSSAI: length %d is reserved", length)
	}
	if len(data) != length+1 {
		return fmt.Errorf("parsing S-NSSAI: payload length is %d, want %d", len(data), length+1)
	}

	result := SNSSAI{SliceServiceType: data[1]}
	offset := 2
	if length == 4 || length == 5 || length == 8 {
		copy(result.SliceDifferentiator[:], data[offset:offset+3])
		result.HasSliceDifferentiator = true
		offset += 3
	}
	if length == 2 || length == 5 || length == 8 {
		result.MappedSliceServiceType = data[offset]
		result.HasMappedSliceServiceType = true
		offset++
	}
	if length == 8 {
		copy(result.MappedSliceDifferentiator[:], data[offset:offset+3])
		result.HasMappedSliceDifferentiator = true
	}
	*s = result
	return nil
}

func NewSingleNSSAITLV(value *SNSSAI) (TLV, error) {
	if value == nil {
		return TLV{Type: TLVTypeSingleNSSAI}, nil
	}
	data, err := value.MarshalBinary()
	if err != nil {
		return TLV{}, err
	}
	return TLV{Type: TLVTypeSingleNSSAI, Data: data}, nil
}

// UnmarshalTLV decodes a single S-NSSAI TLV, including its absent form.
func (s *OptionalSNSSAI) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeSingleNSSAI {
		return fmt.Errorf("parsing single S-NSSAI TLV: type is %d, want %d", tlv.Type, TLVTypeSingleNSSAI)
	}
	if len(tlv.Data) == 0 {
		*s = OptionalSNSSAI{}
		return nil
	}
	var value SNSSAI
	if err := value.UnmarshalBinary(tlv.Data); err != nil {
		return fmt.Errorf("parsing single S-NSSAI TLV: %w", err)
	}
	*s = OptionalSNSSAI{Value: &value}
	return nil
}

func NewNSSAIListTLV(typ TLVType, values NSSAIList) (TLV, error) {
	if !isNSSAIListTLVType(typ) {
		return TLV{}, fmt.Errorf("encoding NSSAI list TLV: type is %d, want allowed, configured, or default configured NSSAI", typ)
	}
	if len(values) == 0 {
		return TLV{}, errors.New("encoding NSSAI list TLV: S-NSSAI list is empty")
	}
	data, err := marshalSNSSAIList(values)
	if err != nil {
		return TLV{}, fmt.Errorf("encoding NSSAI list TLV: %w", err)
	}
	return TLV{Type: typ, Data: data}, nil
}

// UnmarshalTLV decodes an allowed, configured, or default configured NSSAI TLV.
func (n *NSSAIList) UnmarshalTLV(tlv TLV) error {
	if !isNSSAIListTLVType(tlv.Type) {
		return fmt.Errorf("parsing NSSAI list TLV: type is %d, want allowed, configured, or default configured NSSAI", tlv.Type)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing NSSAI list TLV: S-NSSAI list is empty")
	}
	values, err := unmarshalSNSSAIList(tlv.Data)
	if err != nil {
		return fmt.Errorf("parsing NSSAI list TLV: %w", err)
	}
	*n = values
	return nil
}

func isNSSAIListTLVType(typ TLVType) bool {
	return typ == TLVTypeAllowedNSSAI || typ == TLVTypeConfiguredNSSAI || typ == TLVTypeDefaultConfiguredNSSAI
}

func NewRejectedNSSAITLV(values RejectedNSSAIList) (TLV, error) {
	if len(values) == 0 {
		return TLV{}, errors.New("encoding rejected NSSAI TLV: list is empty")
	}

	var data []byte
	for index, value := range values {
		encoded, err := value.MarshalBinary()
		if err != nil {
			return TLV{}, fmt.Errorf("encoding rejected S-NSSAI %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return TLV{Type: TLVTypeRejectedNSSAI, Data: data}, nil
}

// UnmarshalTLV decodes a rejected NSSAI TLV.
func (n *RejectedNSSAIList) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeRejectedNSSAI {
		return fmt.Errorf("parsing rejected NSSAI TLV: type is %d, want %d", tlv.Type, TLVTypeRejectedNSSAI)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing rejected NSSAI TLV: list is empty")
	}

	var values RejectedNSSAIList
	data := tlv.Data
	for len(data) > 0 {
		value, consumed, err := unmarshalRejectedSNSSAIPrefix(data)
		if err != nil {
			return fmt.Errorf("parsing rejected S-NSSAI %d: %w", len(values), err)
		}
		values = append(values, value)
		data = data[consumed:]
	}
	*n = values
	return nil
}

func unmarshalRejectedSNSSAIPrefix(data []byte) (RejectedSNSSAI, int, error) {
	if len(data) < 3 {
		return RejectedSNSSAI{}, 0, errors.New("parsing rejected S-NSSAI: payload is truncated")
	}
	length := int(data[0])
	if length != 1 && length != 4 {
		return RejectedSNSSAI{}, 0, fmt.Errorf("parsing rejected S-NSSAI: length %d is reserved", length)
	}
	totalLength := length + 2
	if totalLength > len(data) {
		return RejectedSNSSAI{}, 0, errors.New("parsing rejected S-NSSAI: payload is truncated")
	}
	cause := RejectedNSSAICause(data[1])
	if err := validateRejectedNSSAICause(cause); err != nil {
		return RejectedSNSSAI{}, 0, fmt.Errorf("parsing rejected S-NSSAI: %w", err)
	}
	snssaiData := make([]byte, length+1)
	snssaiData[0] = byte(length)
	copy(snssaiData[1:], data[2:totalLength])
	var snssai SNSSAI
	if err := snssai.UnmarshalBinary(snssaiData); err != nil {
		return RejectedSNSSAI{}, 0, fmt.Errorf("parsing rejected S-NSSAI: %w", err)
	}
	if snssai.SliceServiceType < 1 || snssai.SliceServiceType > 3 {
		return RejectedSNSSAI{}, 0, fmt.Errorf("parsing rejected S-NSSAI: SST is %d, want 1, 2, or 3", snssai.SliceServiceType)
	}
	return RejectedSNSSAI{Cause: cause, SNSSAI: snssai}, totalLength, nil
}

func validateRejectedNSSAICause(cause RejectedNSSAICause) error {
	if cause != RejectedNSSAINotAvailableInPLMN && cause != RejectedNSSAINotAvailableInRegistrationArea {
		return fmt.Errorf("cause is %d, want unavailable in PLMN or registration area", cause)
	}
	return nil
}

func NewPreconfiguredDefaultNSSAITLV(values PreconfiguredDefaultNSSAIList) (TLV, error) {
	if len(values) == 0 || len(values) > 2 {
		return TLV{}, fmt.Errorf("encoding preconfigured default NSSAI TLV: access list count is %d, want 1 or 2", len(values))
	}

	seen := make(map[AccessType]bool, len(values))
	var data []byte
	for index, value := range values {
		if seen[value.AccessType] {
			return TLV{}, fmt.Errorf("encoding preconfigured default NSSAI TLV: duplicate access type %d", value.AccessType)
		}
		seen[value.AccessType] = true
		encoded, err := value.MarshalBinary()
		if err != nil {
			return TLV{}, fmt.Errorf("encoding preconfigured default NSSAI access list %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: data}, nil
}

// UnmarshalTLV decodes a preconfigured default NSSAI TLV.
func (n *PreconfiguredDefaultNSSAIList) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypePreconfiguredDefaultConfiguredNSSAI {
		return fmt.Errorf("parsing preconfigured default NSSAI TLV: type is %d, want %d", tlv.Type, TLVTypePreconfiguredDefaultConfiguredNSSAI)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing preconfigured default NSSAI TLV: access list is empty")
	}

	var values PreconfiguredDefaultNSSAIList
	seen := make(map[AccessType]bool, 2)
	data := tlv.Data
	for len(data) > 0 {
		if len(values) == 2 {
			return errors.New("parsing preconfigured default NSSAI TLV: more than two access lists")
		}
		value, consumed, err := unmarshalPreconfiguredDefaultNSSAIPrefix(data)
		if err != nil {
			return fmt.Errorf("parsing preconfigured default NSSAI access list %d: %w", len(values), err)
		}
		if seen[value.AccessType] {
			return fmt.Errorf("parsing preconfigured default NSSAI TLV: duplicate access type %d", value.AccessType)
		}
		seen[value.AccessType] = true
		values = append(values, value)
		data = data[consumed:]
	}
	*n = values
	return nil
}

func unmarshalPreconfiguredDefaultNSSAIPrefix(data []byte) (PreconfiguredDefaultNSSAI, int, error) {
	if len(data) < 4 {
		return PreconfiguredDefaultNSSAI{}, 0, errors.New("parsing preconfigured default NSSAI: access type is truncated")
	}
	accessType := AccessType(binary.LittleEndian.Uint32(data[:4]))
	if err := validateNSSAIAccessType(accessType); err != nil {
		return PreconfiguredDefaultNSSAI{}, 0, fmt.Errorf("parsing preconfigured default NSSAI: %w", err)
	}
	preferredTLV, consumed, err := unmarshalTLVPrefix(data[4:])
	if err != nil {
		return PreconfiguredDefaultNSSAI{}, 0, fmt.Errorf("parsing preconfigured default NSSAI: %w", err)
	}
	if preferredTLV.Type != TLVTypeDefaultConfiguredNSSAI {
		return PreconfiguredDefaultNSSAI{}, 0, fmt.Errorf("parsing preconfigured default NSSAI: preferred NSSAI TLV type is %d, want %d", preferredTLV.Type, TLVTypeDefaultConfiguredNSSAI)
	}
	preferred, err := unmarshalSNSSAIList(preferredTLV.Data)
	if err != nil {
		return PreconfiguredDefaultNSSAI{}, 0, fmt.Errorf("parsing preconfigured default NSSAI: %w", err)
	}
	if _, err := marshalPreconfiguredDefaultNSSAIList(preferred); err != nil {
		return PreconfiguredDefaultNSSAI{}, 0, fmt.Errorf("parsing preconfigured default NSSAI: %w", err)
	}
	return PreconfiguredDefaultNSSAI{
		AccessType:     accessType,
		PreferredNSSAI: preferred,
	}, 4 + consumed, nil
}

func marshalPreconfiguredDefaultNSSAIList(values []SNSSAI) ([]byte, error) {
	for index, value := range values {
		if value.HasMappedSliceServiceType || value.HasMappedSliceDifferentiator {
			return nil, fmt.Errorf("preferred S-NSSAI %d contains a mapped S-NSSAI", index)
		}
	}
	return marshalSNSSAIList(values)
}

func validateNSSAIAccessType(value AccessType) error {
	if value != AccessType3GPP && value != AccessTypeNon3GPP {
		return fmt.Errorf("access type is %d, want 3GPP or non-3GPP", value)
	}
	return nil
}

func marshalSNSSAIList(values []SNSSAI) ([]byte, error) {
	var data []byte
	for index, value := range values {
		encoded, err := value.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding S-NSSAI %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return data, nil
}

func unmarshalSNSSAIList(data []byte) ([]SNSSAI, error) {
	var values []SNSSAI
	for len(data) > 0 {
		length := int(data[0]) + 1
		if length > len(data) {
			return nil, fmt.Errorf("parsing S-NSSAI %d: payload is truncated", len(values))
		}
		var value SNSSAI
		if err := value.UnmarshalBinary(data[:length]); err != nil {
			return nil, fmt.Errorf("parsing S-NSSAI %d: %w", len(values), err)
		}
		values = append(values, value)
		data = data[length:]
	}
	return values, nil
}
