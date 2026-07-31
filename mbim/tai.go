package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type TAIListType uint8

const (
	TAIListTypeNonConsecutive TAIListType = 0
	TAIListTypeConsecutive    TAIListType = 1
	TAIListTypeMultiplePLMNs  TAIListType = 2
)

type TAIList struct {
	Type TAIListType
	PLMN PLMN
	TACs []uint32
	TAIs []TrackingAreaIdentity
}

// TAILists is the list of tracking-area identity lists carried by a TAI TLV.
type TAILists []TAIList

func (l TAIList) MarshalBinary() ([]byte, error) {
	if err := validateTAIList(l); err != nil {
		return nil, fmt.Errorf("encoding TAI list: %w", err)
	}

	data := []byte{byte(l.Type)}
	switch l.Type {
	case TAIListTypeNonConsecutive, TAIListTypeConsecutive:
		data = binary.LittleEndian.AppendUint16(data, l.PLMN.MCC)
		data = binary.LittleEndian.AppendUint16(data, l.PLMN.MNC)
		data = append(data, byte(len(l.TACs)))
		for _, tac := range l.TACs {
			data = binary.LittleEndian.AppendUint32(data, tac)
		}
	case TAIListTypeMultiplePLMNs:
		data = append(data, byte(len(l.TAIs)))
		for _, tai := range l.TAIs {
			data = binary.LittleEndian.AppendUint16(data, tai.PLMN.MCC)
			data = binary.LittleEndian.AppendUint16(data, tai.PLMN.MNC)
			data = binary.LittleEndian.AppendUint32(data, tai.TAC)
		}
	}
	return data, nil
}

func (l *TAIList) UnmarshalBinary(data []byte) error {
	value, consumed, err := unmarshalTAIListPrefix(data)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return errors.New("parsing TAI list: trailing data")
	}
	*l = value
	return nil
}

func NewTAITLV(lists TAILists) (TLV, error) {
	if len(lists) == 0 {
		return TLV{}, errors.New("encoding TAI TLV: list is empty")
	}

	var data []byte
	for index, list := range lists {
		encoded, err := list.MarshalBinary()
		if err != nil {
			return TLV{}, fmt.Errorf("encoding TAI TLV list %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return TLV{Type: TLVTypeTAI, Data: data}, nil
}

// UnmarshalTLV decodes a TAI TLV.
func (l *TAILists) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeTAI {
		return fmt.Errorf("parsing TAI TLV: type is %d, want %d", tlv.Type, TLVTypeTAI)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing TAI TLV: list is empty")
	}

	var lists TAILists
	data := tlv.Data
	for len(data) > 0 {
		list, consumed, err := unmarshalTAIListPrefix(data)
		if err != nil {
			return fmt.Errorf("parsing TAI TLV list %d: %w", len(lists), err)
		}
		lists = append(lists, list)
		data = data[consumed:]
	}
	*l = lists
	return nil
}

func unmarshalTAIListPrefix(data []byte) (TAIList, int, error) {
	if len(data) < 1 {
		return TAIList{}, 0, errors.New("parsing TAI list: type is truncated")
	}

	listType := TAIListType(data[0])
	var list TAIList
	var totalLength int
	switch listType {
	case TAIListTypeNonConsecutive, TAIListTypeConsecutive:
		if len(data) < 6 {
			return TAIList{}, 0, errors.New("parsing single-PLMN TAI list: header is truncated")
		}
		count := int(data[5])
		totalLength = 6 + count*4
		if totalLength > len(data) {
			return TAIList{}, 0, errors.New("parsing single-PLMN TAI list: TAC values are truncated")
		}
		list = TAIList{
			Type: listType,
			PLMN: PLMN{
				MCC: binary.LittleEndian.Uint16(data[1:3]),
				MNC: binary.LittleEndian.Uint16(data[3:5]),
			},
			TACs: make([]uint32, count),
		}
		for index := range count {
			offset := 6 + index*4
			list.TACs[index] = binary.LittleEndian.Uint32(data[offset : offset+4])
		}
	case TAIListTypeMultiplePLMNs:
		if len(data) < 2 {
			return TAIList{}, 0, errors.New("parsing multi-PLMN TAI list: header is truncated")
		}
		count := int(data[1])
		totalLength = 2 + count*8
		if totalLength > len(data) {
			return TAIList{}, 0, errors.New("parsing multi-PLMN TAI list: TAI values are truncated")
		}
		list = TAIList{Type: listType, TAIs: make([]TrackingAreaIdentity, count)}
		for index := range count {
			offset := 2 + index*8
			list.TAIs[index] = TrackingAreaIdentity{
				PLMN: PLMN{
					MCC: binary.LittleEndian.Uint16(data[offset : offset+2]),
					MNC: binary.LittleEndian.Uint16(data[offset+2 : offset+4]),
				},
				TAC: binary.LittleEndian.Uint32(data[offset+4 : offset+8]),
			}
		}
	default:
		return TAIList{}, 0, fmt.Errorf("parsing TAI list: type %d is reserved", listType)
	}

	if err := validateTAIList(list); err != nil {
		return TAIList{}, 0, fmt.Errorf("parsing TAI list: %w", err)
	}
	return list, totalLength, nil
}

func validateTAIList(list TAIList) error {
	switch list.Type {
	case TAIListTypeNonConsecutive, TAIListTypeConsecutive:
		if len(list.TACs) < 1 || len(list.TACs) > 16 {
			return fmt.Errorf("TAC count is %d, want 1 through 16", len(list.TACs))
		}
		if len(list.TAIs) != 0 {
			return errors.New("single-PLMN list contains multi-PLMN TAI values")
		}
		if err := validatePLMN(list.PLMN); err != nil {
			return err
		}
		for index, tac := range list.TACs {
			if err := validateTAC(tac); err != nil {
				return fmt.Errorf("TAC %d: %w", index, err)
			}
			if list.Type == TAIListTypeConsecutive && index > 0 && tac != list.TACs[index-1]+1 {
				return fmt.Errorf("TAC %d is %#x, want %#x for a consecutive list", index, tac, list.TACs[index-1]+1)
			}
		}
	case TAIListTypeMultiplePLMNs:
		if len(list.TAIs) < 1 || len(list.TAIs) > 16 {
			return fmt.Errorf("TAI count is %d, want 1 through 16", len(list.TAIs))
		}
		if list.PLMN != (PLMN{}) || len(list.TACs) != 0 {
			return errors.New("multi-PLMN list contains single-PLMN fields")
		}
		for index, tai := range list.TAIs {
			if err := validatePLMN(tai.PLMN); err != nil {
				return fmt.Errorf("TAI %d: %w", index, err)
			}
			if err := validateTAC(tai.TAC); err != nil {
				return fmt.Errorf("TAI %d: %w", index, err)
			}
		}
	default:
		return fmt.Errorf("type %d is reserved", list.Type)
	}
	return nil
}

func validatePLMN(plmn PLMN) error {
	if plmn.MCC&0xF000 != 0 {
		return errors.New("PLMN MCC has nonzero unused bits")
	}
	if !validBCDNibbles(plmn.MCC, 3) {
		return errors.New("PLMN MCC contains a non-decimal BCD digit")
	}

	if plmn.MNC&0x8000 != 0 {
		if plmn.MNC&0x7F00 != 0 {
			return errors.New("two-digit PLMN MNC has nonzero unused bits")
		}
		if !validBCDNibbles(plmn.MNC, 2) {
			return errors.New("PLMN MNC contains a non-decimal BCD digit")
		}
		return nil
	}
	if plmn.MNC&0xF000 != 0 {
		return errors.New("three-digit PLMN MNC has nonzero unused bits")
	}
	if !validBCDNibbles(plmn.MNC, 3) {
		return errors.New("PLMN MNC contains a non-decimal BCD digit")
	}
	return nil
}

func validBCDNibbles(value uint16, count int) bool {
	for range count {
		if value&0xF > 9 {
			return false
		}
		value >>= 4
	}
	return true
}

func validateTAC(tac uint32) error {
	if tac > 0xFFFFFF {
		return fmt.Errorf("TAC %#x exceeds 24 bits", tac)
	}
	return nil
}
