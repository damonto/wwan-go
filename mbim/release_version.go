package mbim

import (
	"encoding/binary"
	"fmt"
)

// ThreeGPPReleaseVersion identifies the 3GPP release used by 5G packet service.
type ThreeGPPReleaseVersion uint32

const (
	ThreeGPPReleaseVersionPre15   ThreeGPPReleaseVersion = 0
	ThreeGPPReleaseVersion15      ThreeGPPReleaseVersion = 15
	ThreeGPPReleaseVersion16      ThreeGPPReleaseVersion = 16
	ThreeGPPReleaseVersionUnknown ThreeGPPReleaseVersion = ^ThreeGPPReleaseVersion(0)
)

// New3GPPReleaseVersionTLV encodes a 3GPP release version TLV.
func New3GPPReleaseVersionTLV(version ThreeGPPReleaseVersion) (TLV, error) {
	if err := validate3GPPReleaseVersion(version); err != nil {
		return TLV{}, fmt.Errorf("encoding 3GPP release version TLV: %w", err)
	}
	return TLV{
		Type: TLVType3GPPReleaseVersion,
		Data: binary.LittleEndian.AppendUint32(nil, uint32(version)),
	}, nil
}

// UnmarshalTLV decodes a 3GPP release version TLV.
func (v *ThreeGPPReleaseVersion) UnmarshalTLV(tlv TLV) error {
	if tlv.Type != TLVType3GPPReleaseVersion {
		return fmt.Errorf("parsing 3GPP release version TLV: type is %d, want %d", tlv.Type, TLVType3GPPReleaseVersion)
	}
	if len(tlv.Data) != 4 {
		return fmt.Errorf("parsing 3GPP release version TLV: data length is %d, want 4", len(tlv.Data))
	}

	version := ThreeGPPReleaseVersion(binary.LittleEndian.Uint32(tlv.Data))
	if err := validate3GPPReleaseVersion(version); err != nil {
		return fmt.Errorf("parsing 3GPP release version TLV: %w", err)
	}
	*v = version
	return nil
}

func validate3GPPReleaseVersion(version ThreeGPPReleaseVersion) error {
	switch version {
	case ThreeGPPReleaseVersionPre15,
		ThreeGPPReleaseVersion15,
		ThreeGPPReleaseVersion16,
		ThreeGPPReleaseVersionUnknown:
		return nil
	default:
		return fmt.Errorf("value %d is reserved", version)
	}
}
