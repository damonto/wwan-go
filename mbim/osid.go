package mbim

import (
	"fmt"
	"slices"
)

// OSID is the 16-octet UUID representation carried by an MBIM OSID TLV.
type OSID [16]byte

func NewOSIDTLV(osid OSID) TLV {
	return TLV{
		Type: TLVTypeOSID,
		Data: slices.Clone(osid[:]),
	}
}

// UnmarshalTLV decodes an OSID TLV.
func (o *OSID) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVTypeOSID {
		return fmt.Errorf("parsing OSID TLV: type is %d, want %d", tlv.Type, TLVTypeOSID)
	}
	if len(tlv.Data) != len(OSID{}) {
		return fmt.Errorf("parsing OSID TLV: data length is %d, want %d", len(tlv.Data), len(OSID{}))
	}

	var value OSID
	copy(value[:], tlv.Data)
	*o = value
	return nil
}
