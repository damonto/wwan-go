package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type TrafficParameters struct {
	TrafficDescriptor []byte
}

func (p TrafficParameters) MarshalBinary() ([]byte, error) {
	if len(p.TrafficDescriptor) > int(^uint16(0)) {
		return nil, errors.New("encoding MBIM traffic parameters: traffic descriptor exceeds UINT16 length")
	}

	data := binary.BigEndian.AppendUint16(nil, uint16(len(p.TrafficDescriptor)))
	return append(data, p.TrafficDescriptor...), nil
}

func (p *TrafficParameters) UnmarshalBinary(data []byte) error {
	if len(data) < 2 {
		return errors.New("parsing MBIM traffic parameters: length is truncated")
	}
	length := int(binary.BigEndian.Uint16(data[:2]))
	if len(data) != length+2 {
		return fmt.Errorf("parsing MBIM traffic parameters: payload length is %d, want %d", len(data), length+2)
	}
	*p = TrafficParameters{TrafficDescriptor: slices.Clone(data[2:])}
	return nil
}

func NewTrafficParametersTLV(value TrafficParameters) (TLV, error) {
	data, err := value.MarshalBinary()
	if err != nil {
		return TLV{}, err
	}
	return TLV{Type: TLVTypeTrafficParameters, Data: data}, nil
}

// UnmarshalTLV decodes a traffic parameters TLV.
func (p *TrafficParameters) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeTrafficParameters {
		return fmt.Errorf("parsing traffic parameters TLV: type is %d, want %d", tlv.Type, TLVTypeTrafficParameters)
	}
	var value TrafficParameters
	if err := value.UnmarshalBinary(tlv.Data); err != nil {
		return fmt.Errorf("parsing traffic parameters TLV: %w", err)
	}
	*p = value
	return nil
}
