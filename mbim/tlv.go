package mbim

import (
	"encoding/binary"
	"errors"
	"slices"
)

type TLV struct {
	Type TLVType
	Data []byte
}

type TLVs []TLV

func (t *TLVs) UnmarshalBinary(data []byte) error {
	*t = nil
	for len(data) > 0 {
		if len(data) < 8 {
			return errors.New("parsing MBIM TLV: header is truncated")
		}
		paddingLength := int(data[3])
		if paddingLength > 3 {
			return errors.New("parsing MBIM TLV: padding length exceeds 3")
		}
		dataLength := int(binary.LittleEndian.Uint32(data[4:8]))
		if dataLength > len(data)-8-paddingLength {
			return errors.New("parsing MBIM TLV: data is truncated")
		}

		*t = append(*t, TLV{
			Type: TLVType(binary.LittleEndian.Uint16(data[:2])),
			Data: slices.Clone(data[8 : 8+dataLength]),
		})
		data = data[8+dataLength+paddingLength:]
	}
	return nil
}

func mbimTLV(typ TLVType, value []byte) []byte {
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
