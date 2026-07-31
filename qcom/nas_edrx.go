package qcom

import (
	"encoding/binary"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// NASCIoTLTEMode identifies the LTE wideband, M1, or NB1 camping mode.
type NASCIoTLTEMode uint32

const (
	NASCIoTLTEModeNoService NASCIoTLTEMode = iota
	NASCIoTLTEModeWideband
	NASCIoTLTEModeM1
	NASCIoTLTEModeNB1
)

// NASEDRXParameters contains negotiated extended idle-mode DRX parameters.
type NASEDRXParameters struct {
	Enabled               bool
	EnabledKnown          bool
	CycleLength           uint8
	CycleLengthKnown      bool
	PagingTimeWindow      uint8
	PagingTimeWindowKnown bool
	RadioInterface        NASRadioInterface
	RadioInterfaceKnown   bool
	CIoTLTEMode           NASCIoTLTEMode
	CIoTLTEModeKnown      bool
}

// UnmarshalTLVs parses QMI NAS eDRX Change Info indication TLVs.
func (p *NASEDRXParameters) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*p = NASEDRXParameters{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS eDRX enabled state: TLV length %d, want 1", len(value))
		}
		p.Enabled = value[0] != 0
		p.EnabledKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS eDRX cycle length: TLV length %d, want 1", len(value))
		}
		p.CycleLength = value[0]
		p.CycleLengthKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS eDRX paging time window: TLV length %d, want 1", len(value))
		}
		p.PagingTimeWindow = value[0]
		p.PagingTimeWindowKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS eDRX radio interface: TLV length %d, want 1", len(value))
		}
		p.RadioInterface = NASRadioInterface(value[0])
		p.RadioInterfaceKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS eDRX LTE mode: TLV length %d, want 4", len(value))
		}
		p.CIoTLTEMode = NASCIoTLTEMode(binary.LittleEndian.Uint32(value))
		p.CIoTLTEModeKnown = true
	}
	return nil
}
