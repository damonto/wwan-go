package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type RouteSelectionDescriptorSource uint32

const (
	RouteSelectionDescriptorSourceDefault       RouteSelectionDescriptorSource = 0
	RouteSelectionDescriptorSourceUser          RouteSelectionDescriptorSource = 1
	RouteSelectionDescriptorSourceAdmin         RouteSelectionDescriptorSource = 2
	RouteSelectionDescriptorSourceOperator      RouteSelectionDescriptorSource = 3
	RouteSelectionDescriptorSourceDevice        RouteSelectionDescriptorSource = 4
	RouteSelectionDescriptorSourceModemOperator RouteSelectionDescriptorSource = 5
	RouteSelectionDescriptorSourceModemLocal    RouteSelectionDescriptorSource = 6
)

type RouteSelectionDescriptorPurpose uint32

const (
	RouteSelectionDescriptorPurposeDefault  RouteSelectionDescriptorPurpose = 0
	RouteSelectionDescriptorPurposePurchase RouteSelectionDescriptorPurpose = 1 << 0
)

type RouteSelectionDescriptor struct {
	Source     RouteSelectionDescriptorSource
	Purpose    RouteSelectionDescriptorPurpose
	Precedence uint8
	Contents   []byte
}

// RouteSelectionDescriptors is the descriptor list carried by its MBIM TLV.
type RouteSelectionDescriptors []RouteSelectionDescriptor

const minimumRouteSelectionDescriptorLength = 4

func (d RouteSelectionDescriptor) MarshalBinary() ([]byte, error) {
	if d.Source > RouteSelectionDescriptorSourceModemLocal {
		return nil, fmt.Errorf("encoding MBIM route selection descriptor: source %d is reserved", d.Source)
	}
	if d.Purpose&^RouteSelectionDescriptorPurposePurchase != 0 {
		return nil, fmt.Errorf("encoding MBIM route selection descriptor: purpose %#x contains reserved bits", d.Purpose)
	}
	if len(d.Contents) == 0 {
		return nil, errors.New("encoding MBIM route selection descriptor: contents are empty")
	}
	if len(d.Contents) > int(^uint16(0))-3 {
		return nil, errors.New("encoding MBIM route selection descriptor: contents exceed descriptor UINT16 length")
	}

	descriptorLength := 3 + len(d.Contents)
	data := binary.LittleEndian.AppendUint32(nil, uint32(d.Source))
	data = binary.LittleEndian.AppendUint32(data, uint32(d.Purpose))
	data = binary.BigEndian.AppendUint16(data, uint16(descriptorLength))
	data = append(data, d.Precedence)
	data = binary.BigEndian.AppendUint16(data, uint16(len(d.Contents)))
	return append(data, d.Contents...), nil
}

func (d *RouteSelectionDescriptor) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalRouteSelectionDescriptorPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing MBIM route selection descriptor: trailing data")
	}
	*d = value
	return nil
}

func NewRouteSelectionDescriptorsTLV(values RouteSelectionDescriptors) (TLV, error) {
	if len(values) == 0 {
		return TLV{}, errors.New("encoding route selection descriptors TLV: descriptor list is empty")
	}

	var data []byte
	for index, value := range values {
		encoded, err := value.MarshalBinary()
		if err != nil {
			return TLV{}, fmt.Errorf("encoding route selection descriptor %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return TLV{Type: TLVTypeRouteSelectionDescriptors, Data: data}, nil
}

// UnmarshalTLV decodes a route selection descriptors TLV.
func (d *RouteSelectionDescriptors) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeRouteSelectionDescriptors {
		return fmt.Errorf("parsing route selection descriptors TLV: type is %d, want %d", tlv.Type, TLVTypeRouteSelectionDescriptors)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing route selection descriptors TLV: descriptor list is empty")
	}

	var values RouteSelectionDescriptors
	data := tlv.Data
	for len(data) > 0 {
		value, consumed, err := unmarshalRouteSelectionDescriptorPrefix(data)
		if err != nil {
			return fmt.Errorf("parsing route selection descriptor %d: %w", len(values), err)
		}
		values = append(values, value)
		data = data[consumed:]
	}
	*d = values
	return nil
}

func unmarshalRouteSelectionDescriptorPrefix(data []byte) (RouteSelectionDescriptor, int, error) {
	const mbimHeaderLength = 8
	if len(data) < mbimHeaderLength+2 {
		return RouteSelectionDescriptor{}, 0, errors.New("parsing MBIM route selection descriptor: header is truncated")
	}
	source := RouteSelectionDescriptorSource(binary.LittleEndian.Uint32(data[:4]))
	if source > RouteSelectionDescriptorSourceModemLocal {
		return RouteSelectionDescriptor{}, 0, fmt.Errorf("parsing MBIM route selection descriptor: source %d is reserved", source)
	}
	purpose := RouteSelectionDescriptorPurpose(binary.LittleEndian.Uint32(data[4:8]))
	if purpose&^RouteSelectionDescriptorPurposePurchase != 0 {
		return RouteSelectionDescriptor{}, 0, fmt.Errorf("parsing MBIM route selection descriptor: purpose %#x contains reserved bits", purpose)
	}

	descriptorLength := int(binary.BigEndian.Uint16(data[mbimHeaderLength : mbimHeaderLength+2]))
	if descriptorLength < minimumRouteSelectionDescriptorLength {
		return RouteSelectionDescriptor{}, 0, fmt.Errorf("parsing MBIM route selection descriptor: length is %d, want at least 4", descriptorLength)
	}
	totalLength := mbimHeaderLength + 2 + descriptorLength
	if totalLength > len(data) {
		return RouteSelectionDescriptor{}, 0, errors.New("parsing MBIM route selection descriptor: data is truncated")
	}

	contentsLength := int(binary.BigEndian.Uint16(data[mbimHeaderLength+3 : mbimHeaderLength+5]))
	wantContentsLength := descriptorLength - 3
	if contentsLength != wantContentsLength {
		return RouteSelectionDescriptor{}, 0, fmt.Errorf("parsing MBIM route selection descriptor: contents length is %d, want %d", contentsLength, wantContentsLength)
	}

	return RouteSelectionDescriptor{
		Source:     source,
		Purpose:    purpose,
		Precedence: data[10],
		Contents:   slices.Clone(data[13:totalLength]),
	}, totalLength, nil
}
