package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

const maxTLVType = TLVType(0xFFF0)

type TLVType uint16

const (
	TLVTypeUEPolicies                          TLVType = 1
	TLVTypeSingleNSSAI                         TLVType = 2
	TLVTypeAllowedNSSAI                        TLVType = 3
	TLVTypeConfiguredNSSAI                     TLVType = 4
	TLVTypeDefaultConfiguredNSSAI              TLVType = 5
	TLVTypePreconfiguredDefaultConfiguredNSSAI TLVType = 6
	TLVTypeRejectedNSSAI                       TLVType = 7
	TLVTypeLADN                                TLVType = 8
	TLVTypeTAI                                 TLVType = 9
	TLVTypeWCharString                         TLVType = 10
	TLVTypeUint16Table                         TLVType = 11
	TLVTypeEAPPacket                           TLVType = 12
	TLVTypePCO                                 TLVType = 13
	TLVTypeRouteSelectionDescriptors           TLVType = 14
	TLVTypeTrafficParameters                   TLVType = 15
	TLVTypeWakeCommand                         TLVType = 16
	TLVTypeWakePacket                          TLVType = 17
	TLVTypeOSID                                TLVType = 18
	TLVType3GPPReleaseVersion                  TLVType = 19
	TLVTypeURSPRulesTDOnly                     TLVType = 20
	TLVTypeSessionID                           TLVType = 21
)

type TLV struct {
	Type TLVType
	Data []byte
}

type TLVs []TLV

func (t TLV) MarshalBinary() ([]byte, error) {
	if err := validateTLVType(t.Type); err != nil {
		return nil, fmt.Errorf("encoding MBIM TLV: %w", err)
	}
	if uint64(len(t.Data)) > uint64(^uint32(0)) {
		return nil, errors.New("encoding MBIM TLV: data exceeds UINT32 length")
	}
	return marshalTLV(t.Type, t.Data), nil
}

func (t *TLV) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalTLVPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing MBIM TLV: trailing data")
	}
	*t = value
	return nil
}

func (t TLVs) MarshalBinary() ([]byte, error) {
	var data []byte
	for i, value := range t {
		encoded, err := value.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding MBIM TLV %d: %w", i, err)
		}
		data = append(data, encoded...)
	}
	return data, nil
}

func (t *TLVs) UnmarshalBinary(data []byte) error {
	var values TLVs
	for len(data) > 0 {
		value, consumed, err := unmarshalTLVPrefix(data)
		if err != nil {
			return fmt.Errorf("parsing MBIM TLV %d: %w", len(values), err)
		}
		values = append(values, value)
		data = data[consumed:]
	}
	*t = values
	return nil
}

func mbimTLV(typ TLVType, value []byte) []byte {
	return marshalTLV(typ, value)
}

func marshalTLV(typ TLVType, value []byte) []byte {
	paddingLength := (4 - len(value)%4) % 4
	data := binary.LittleEndian.AppendUint16(nil, uint16(typ))
	data = append(data, 0)
	data = append(data, byte(paddingLength))
	data = binary.LittleEndian.AppendUint32(data, uint32(len(value)))
	data = append(data, value...)
	for range paddingLength {
		data = append(data, 0)
	}
	return data
}

func marshalTLVsUnchecked(values TLVs) []byte {
	var data []byte
	for _, value := range values {
		data = append(data, marshalTLV(value.Type, value.Data)...)
	}
	return data
}

func unmarshalTLVPrefix(data []byte) (TLV, int, error) {
	if len(data) < 8 {
		return TLV{}, 0, errors.New("parsing MBIM TLV: header is truncated")
	}
	typ := TLVType(binary.LittleEndian.Uint16(data[:2]))
	if err := validateTLVType(typ); err != nil {
		return TLV{}, 0, fmt.Errorf("parsing MBIM TLV: %w", err)
	}
	if data[2] != 0 {
		return TLV{}, 0, errors.New("parsing MBIM TLV: reserved byte is nonzero")
	}

	paddingLength := uint32(data[3])
	if paddingLength > 3 {
		return TLV{}, 0, errors.New("parsing MBIM TLV: padding length exceeds 3")
	}
	dataLength := binary.LittleEndian.Uint32(data[4:8])
	wantPaddingLength := (4 - dataLength%4) % 4
	if paddingLength != wantPaddingLength {
		return TLV{}, 0, fmt.Errorf("parsing MBIM TLV: padding length is %d, want %d", paddingLength, wantPaddingLength)
	}
	totalLength := uint64(8) + uint64(dataLength) + uint64(paddingLength)
	if totalLength > uint64(len(data)) {
		return TLV{}, 0, errors.New("parsing MBIM TLV: data is truncated")
	}

	dataEnd := 8 + int(dataLength)
	totalEnd := dataEnd + int(paddingLength)
	for _, value := range data[dataEnd:totalEnd] {
		if value != 0 {
			return TLV{}, 0, errors.New("parsing MBIM TLV: padding byte is nonzero")
		}
	}
	return TLV{
		Type: typ,
		Data: slices.Clone(data[8:dataEnd]),
	}, totalEnd, nil
}

func validateTLVType(typ TLVType) error {
	if typ == 0 || typ > maxTLVType {
		return fmt.Errorf("type %#x is reserved", typ)
	}
	return nil
}
