package mbim

import (
	"errors"
	"fmt"
)

const (
	minimumLADNDNNLength = 3
	maximumLADNDNNLength = 102
)

type LADN struct {
	DNN      string
	TAILists TAILists
}

// LADNList is the list of local-area data networks carried by a LADN TLV.
type LADNList []LADN

func NewLADNTLV(values LADNList, version uint16) (TLV, error) {
	version = networkParametersVersion(version)
	if version < mbimExVersion30 {
		return TLV{}, errors.New("encoding LADN TLV: MBIMEx 3.0 or later is required")
	}
	if len(values) == 0 {
		return TLV{}, errors.New("encoding LADN TLV: list is empty")
	}

	var data []byte
	for index, value := range values {
		encoded, err := marshalLADN(value, version)
		if err != nil {
			return TLV{}, fmt.Errorf("encoding LADN TLV value %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return TLV{Type: TLVTypeLADN, Data: data}, nil
}

// UnmarshalTLV decodes a LADN TLV for the negotiated MBIMEx version.
func (l *LADNList) UnmarshalTLV(tlv TLV, version uint16) error {
	version = networkParametersVersion(version)
	if version < mbimExVersion30 {
		return errors.New("parsing LADN TLV: MBIMEx 3.0 or later is required")
	}
	if tlv.Type != TLVTypeLADN {
		return fmt.Errorf("parsing LADN TLV: type is %d, want %d", tlv.Type, TLVTypeLADN)
	}
	if len(tlv.Data) == 0 {
		return errors.New("parsing LADN TLV: list is empty")
	}

	var values LADNList
	data := tlv.Data
	for len(data) > 0 {
		value, consumed, err := unmarshalLADNPrefix(data, version)
		if err != nil {
			return fmt.Errorf("parsing LADN TLV value %d: %w", len(values), err)
		}
		values = append(values, value)
		data = data[consumed:]
	}
	*l = values
	return nil
}

func marshalLADN(value LADN, version uint16) ([]byte, error) {
	if len(value.TAILists) == 0 {
		return nil, errors.New("TAI list is empty")
	}

	var data []byte
	if version >= mbimExVersion40 {
		data = marshalTLV(TLVTypeWCharString, utf16Bytes(value.DNN))
	} else {
		length := len(value.DNN)
		if length < minimumLADNDNNLength || length > maximumLADNDNNLength {
			return nil, fmt.Errorf("DNN length is %d, want %d through %d octets", length, minimumLADNDNNLength, maximumLADNDNNLength)
		}
		data = append(data, byte(length))
		data = append(data, value.DNN...)
	}

	for index, list := range value.TAILists {
		encoded, err := list.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("TAI list %d: %w", index, err)
		}
		data = append(data, encoded...)
	}
	return data, nil
}

func unmarshalLADNPrefix(data []byte, version uint16) (LADN, int, error) {
	var result LADN
	var offset int
	if version >= mbimExVersion40 {
		dnnTLV, consumed, err := unmarshalTLVPrefix(data)
		if err != nil {
			return LADN{}, 0, fmt.Errorf("parsing DNN TLV: %w", err)
		}
		if dnnTLV.Type != TLVTypeWCharString {
			return LADN{}, 0, fmt.Errorf("parsing DNN TLV: type is %d, want %d", dnnTLV.Type, TLVTypeWCharString)
		}
		result.DNN, err = utf16RawString(dnnTLV.Data)
		if err != nil {
			return LADN{}, 0, fmt.Errorf("parsing DNN: %w", err)
		}
		offset = consumed
	} else {
		if len(data) < 1 {
			return LADN{}, 0, errors.New("parsing DNN: length is truncated")
		}
		length := int(data[0])
		if length < minimumLADNDNNLength || length > maximumLADNDNNLength {
			return LADN{}, 0, fmt.Errorf("parsing DNN: length is %d, want %d through %d", length, minimumLADNDNNLength, maximumLADNDNNLength)
		}
		if len(data) < length+1 {
			return LADN{}, 0, errors.New("parsing DNN: value is truncated")
		}
		result.DNN = string(data[1 : length+1])
		offset = length + 1
	}

	for offset < len(data) && data[offset] <= byte(TAIListTypeMultiplePLMNs) {
		list, consumed, err := unmarshalTAIListPrefix(data[offset:])
		if err != nil {
			return LADN{}, 0, fmt.Errorf("parsing TAI list %d: %w", len(result.TAILists), err)
		}
		result.TAILists = append(result.TAILists, list)
		offset += consumed
	}
	if len(result.TAILists) == 0 {
		return LADN{}, 0, errors.New("parsing LADN: TAI list is empty")
	}
	return result, offset, nil
}
