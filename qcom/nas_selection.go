package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVSelectionEmergencyMode           = 0x10
	nasTLVSelectionModePreference          = 0x11
	nasTLVSelectionBandPreference          = 0x12
	nasTLVSelectionPRLPreference           = 0x13
	nasTLVSelectionRoamingPreference       = 0x14
	nasTLVSelectionLTEBandPreference       = 0x15
	nasTLVSelectionNetwork                 = 0x16
	nasTLVSelectionChangeDuration          = 0x17
	nasTLVSelectionServiceDomain           = 0x18
	nasTLVSelectionGWAcquisitionOrder      = 0x19
	nasTLVSelectionMNCThreeDigits          = 0x1A
	nasTLVSelectionSetAcquisitionOrder     = 0x1E
	nasTLVSelectionSetRegistrationRestrict = 0x1F
	nasTLVSelectionSetUsagePreference      = 0x21
	nasTLVSelectionSetRadioInterface       = 0x22
	nasTLVSelectionSetVoiceDomain          = 0x23
	nasTLVSelectionSetLTEBandsExtended     = 0x24
	nasTLVSelectionSetTDSCDMABands         = 0x1D
	nasTLVSelectionSetNR5GBands            = 0x2B
	nasTLVSelectionSetNR5GDisableMode      = 0x2E
	nasTLVSelectionSetNR5GSABands          = 0x2F
	nasTLVSelectionSetNR5GNSABands         = 0x30

	nasTLVSelectionGetTDSCDMABands         = 0x1A
	nasTLVSelectionGetManualPLMN           = 0x1B
	nasTLVSelectionGetAcquisitionOrder     = 0x1C
	nasTLVSelectionGetRegistrationRestrict = 0x1D
	nasTLVSelectionGetUsagePreference      = 0x1F
	nasTLVSelectionGetVoiceDomain          = 0x20
	nasTLVSelectionGetLTEDisableCause      = 0x21
	nasTLVSelectionGetDisabledRATs         = 0x22
	nasTLVSelectionGetLTEBandsExtended     = 0x23
	nasTLVSelectionGetNR5GBands            = 0x28
	nasTLVSelectionGetNR5GDisableMode      = 0x2B
	nasTLVSelectionGetNR5GSABands          = 0x2C
	nasTLVSelectionGetNR5GNSABands         = 0x2D

	nasMaxAcquisitionOrder = 10
)

// NASModePreference is a bitmask of radio technologies the modem may acquire.
type NASModePreference uint16

const (
	NASModePreferenceCDMA1X NASModePreference = 1 << iota
	NASModePreferenceHDR
	NASModePreferenceGSM
	NASModePreferenceUMTS
	NASModePreferenceLTE
	NASModePreferenceTDSCDMA
	NASModePreferenceNR5G
)

// NASBandPreference is the legacy NAS band-class bitmask.
type NASBandPreference uint64

// NASLTEBandPreference is the deprecated 64-bit LTE band mask.
type NASLTEBandPreference uint64

// NASTDSCDMABandPreference is the TD-SCDMA band mask.
type NASTDSCDMABandPreference uint64

// NASPRLPreference controls CDMA band-class 0 acquisition.
type NASPRLPreference uint16

const (
	NASPRLPreferenceASide NASPRLPreference = 0x0001
	NASPRLPreferenceBSide NASPRLPreference = 0x0002
	NASPRLPreferenceAny   NASPRLPreference = 0x3FFF
)

// NASRoamingPreference controls whether roaming systems may be acquired.
type NASRoamingPreference uint16

const (
	NASRoamingPreferenceOff         NASRoamingPreference = 0x01
	NASRoamingPreferenceNotOff      NASRoamingPreference = 0x02
	NASRoamingPreferenceNotFlashing NASRoamingPreference = 0x03
	NASRoamingPreferenceAny         NASRoamingPreference = 0xFF
)

// NASNetworkSelectionMode chooses automatic or manual PLMN selection.
type NASNetworkSelectionMode uint8

const (
	NASNetworkSelectionAutomatic NASNetworkSelectionMode = iota
	NASNetworkSelectionManual
)

// NASChangeDuration controls whether a preference survives a power cycle.
type NASChangeDuration uint8

const (
	NASChangeUntilPowerCycle NASChangeDuration = iota
	NASChangePermanent
)

// NASServiceDomainPreference selects circuit-switched and packet-switched service.
type NASServiceDomainPreference uint32

const (
	NASServiceDomainCSOnly NASServiceDomainPreference = iota
	NASServiceDomainPSOnly
	NASServiceDomainCSPS
)

// NASGWAcquisitionOrder is the relative GSM/WCDMA acquisition order.
type NASGWAcquisitionOrder uint32

const (
	NASGWAcquisitionAutomatic NASGWAcquisitionOrder = iota
	NASGWAcquisitionGSMThenWCDMA
	NASGWAcquisitionWCDMAThenGSM
)

// NASRegistrationRestriction controls normal, camp-only, or limited registration.
type NASRegistrationRestriction uint32

const (
	NASRegistrationUnrestricted NASRegistrationRestriction = iota
	NASRegistrationCampedOnly
	NASRegistrationLimited
)

// NASUsagePreference identifies a voice- or data-centric subscription.
type NASUsagePreference uint32

const (
	NASUsageUnknown NASUsagePreference = iota
	NASUsageVoiceCentric
	NASUsageDataCentric
)

// NASVoiceDomainPreference selects circuit-switched or packet-switched voice.
type NASVoiceDomainPreference uint32

const (
	NASVoiceDomainCSOnly NASVoiceDomainPreference = iota
	NASVoiceDomainPSOnly
	NASVoiceDomainCSPreferred
	NASVoiceDomainPSPreferred
)

// NASLTEDisableCause identifies why LTE is unavailable.
type NASLTEDisableCause uint32

const (
	NASLTEDisableNone NASLTEDisableCause = iota
	NASLTEDisablePermanentDomainSelection
	NASLTEDisableTemporaryDomainSelection
	NASLTEDisableVoiceDomainSelection
	NASLTEDisableAggressionManagement
	NASLTEDisableUser
	NASLTEDisableNoChange
)

// NASNR5GDisableMode selects which NR5G deployment mode is disabled.
type NASNR5GDisableMode uint32

const (
	NASNR5GDisableNone NASNR5GDisableMode = iota
	NASNR5GDisableSA
	NASNR5GDisableNSA
)

// NASLTEBandPreferenceExtended is Qualcomm's 256-bit LTE band mask.
type NASLTEBandPreferenceExtended struct {
	Bits1To64    uint64
	Bits65To128  uint64
	Bits129To192 uint64
	Bits193To256 uint64
}

// NASNR5GBandPreference is Qualcomm's 512-bit NR5G band mask.
type NASNR5GBandPreference struct {
	Bits1To64    uint64
	Bits65To128  uint64
	Bits129To192 uint64
	Bits193To256 uint64
	Bits257To320 uint64
	Bits321To384 uint64
	Bits385To448 uint64
	Bits449To512 uint64
}

// NASNetworkSelection configures automatic or manual PLMN selection.
type NASNetworkSelection struct {
	Mode           NASNetworkSelectionMode
	PLMN           NASPLMN
	RadioInterface *NASRadioInterface
}

// NASSystemSelectionConfig contains optional changes to modem selection policy.
// Nil pointer and nil slice fields are omitted from the request.
type NASSystemSelectionConfig struct {
	EmergencyMode      *bool
	ModePreference     *NASModePreference
	BandPreference     *NASBandPreference
	PRLPreference      *NASPRLPreference
	RoamingPreference  *NASRoamingPreference
	LTEBandPreference  *NASLTEBandPreference
	NetworkSelection   *NASNetworkSelection
	ChangeDuration     *NASChangeDuration
	ServiceDomain      *NASServiceDomainPreference
	GWAcquisitionOrder *NASGWAcquisitionOrder

	TDSCDMABandPreference *NASTDSCDMABandPreference
	AcquisitionOrder      []NASRadioInterface
	RegistrationRestrict  *NASRegistrationRestriction
	UsagePreference       *NASUsagePreference
	VoiceDomain           *NASVoiceDomainPreference

	LTEBandsExtended *NASLTEBandPreferenceExtended
	NR5GBands        *NASNR5GBandPreference
	NR5GDisableMode  *NASNR5GDisableMode
	NR5GSABands      *NASNR5GBandPreference
	NR5GNSABands     *NASNR5GBandPreference
}

// NASSystemSelectionPreference is the current modem selection policy.
type NASSystemSelectionPreference struct {
	EmergencyMode           bool
	EmergencyModeKnown      bool
	ModePreference          NASModePreference
	ModePreferenceKnown     bool
	BandPreference          NASBandPreference
	BandPreferenceKnown     bool
	PRLPreference           NASPRLPreference
	PRLPreferenceKnown      bool
	RoamingPreference       NASRoamingPreference
	RoamingPreferenceKnown  bool
	LTEBandPreference       NASLTEBandPreference
	LTEBandPreferenceKnown  bool
	NetworkSelection        NASNetworkSelectionMode
	NetworkSelectionKnown   bool
	ServiceDomain           NASServiceDomainPreference
	ServiceDomainKnown      bool
	GWAcquisitionOrder      NASGWAcquisitionOrder
	GWAcquisitionOrderKnown bool

	TDSCDMABandPreference        NASTDSCDMABandPreference
	TDSCDMABandPreferenceKnown   bool
	ManualPLMN                   NASPLMN
	ManualPLMNKnown              bool
	AcquisitionOrder             []NASRadioInterface
	AcquisitionOrderKnown        bool
	RegistrationRestriction      NASRegistrationRestriction
	RegistrationRestrictionKnown bool
	UsagePreference              NASUsagePreference
	UsagePreferenceKnown         bool
	VoiceDomain                  NASVoiceDomainPreference
	VoiceDomainKnown             bool
	LTEDisableCause              NASLTEDisableCause
	LTEDisableCauseKnown         bool
	DisabledRATs                 NASModePreference
	DisabledRATsKnown            bool

	LTEBandsExtended      NASLTEBandPreferenceExtended
	LTEBandsExtendedKnown bool
	NR5GBands             NASNR5GBandPreference
	NR5GBandsKnown        bool
	NR5GDisableMode       NASNR5GDisableMode
	NR5GDisableModeKnown  bool
	NR5GSABands           NASNR5GBandPreference
	NR5GSABandsKnown      bool
	NR5GNSABands          NASNR5GBandPreference
	NR5GNSABandsKnown     bool
}

// NASSetSystemSelectionPreferenceRequest encodes Set System Selection Preference.
type NASSetSystemSelectionPreferenceRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASSystemSelectionConfig
}

// Request converts the request into a QMI NAS request.
func (r NASSetSystemSelectionPreferenceRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASSetSystemSelectionPreference,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// NASGetSystemSelectionPreferenceRequest encodes Get System Selection Preference.
type NASGetSystemSelectionPreferenceRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI NAS request.
func (r NASGetSystemSelectionPreferenceRequest) Request() Request {
	return nasEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageNASGetSystemSelectionPreference)
}

// UnmarshalTLVs parses Get System Selection Preference response or indication TLVs.
func (r *NASSystemSelectionPreference) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASSystemSelectionPreference{}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionEmergencyMode); ok {
		if len(value) != 1 {
			return nasSelectionLengthError(nasTLVSelectionEmergencyMode, len(value), 1)
		}
		r.EmergencyMode = value[0] != 0
		r.EmergencyModeKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionModePreference); ok {
		if len(value) != 2 {
			return nasSelectionLengthError(nasTLVSelectionModePreference, len(value), 2)
		}
		r.ModePreference = NASModePreference(binary.LittleEndian.Uint16(value))
		r.ModePreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionBandPreference); ok {
		if len(value) != 8 {
			return nasSelectionLengthError(nasTLVSelectionBandPreference, len(value), 8)
		}
		r.BandPreference = NASBandPreference(binary.LittleEndian.Uint64(value))
		r.BandPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionPRLPreference); ok {
		if len(value) != 2 {
			return nasSelectionLengthError(nasTLVSelectionPRLPreference, len(value), 2)
		}
		r.PRLPreference = NASPRLPreference(binary.LittleEndian.Uint16(value))
		r.PRLPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionRoamingPreference); ok {
		if len(value) != 2 {
			return nasSelectionLengthError(nasTLVSelectionRoamingPreference, len(value), 2)
		}
		r.RoamingPreference = NASRoamingPreference(binary.LittleEndian.Uint16(value))
		r.RoamingPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionLTEBandPreference); ok {
		if len(value) != 8 {
			return nasSelectionLengthError(nasTLVSelectionLTEBandPreference, len(value), 8)
		}
		r.LTEBandPreference = NASLTEBandPreference(binary.LittleEndian.Uint64(value))
		r.LTEBandPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionNetwork); ok {
		if len(value) != 1 {
			return nasSelectionLengthError(nasTLVSelectionNetwork, len(value), 1)
		}
		r.NetworkSelection = NASNetworkSelectionMode(value[0])
		r.NetworkSelectionKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionServiceDomain); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionServiceDomain, len(value), 4)
		}
		r.ServiceDomain = NASServiceDomainPreference(binary.LittleEndian.Uint32(value))
		r.ServiceDomainKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGWAcquisitionOrder); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGWAcquisitionOrder, len(value), 4)
		}
		r.GWAcquisitionOrder = NASGWAcquisitionOrder(binary.LittleEndian.Uint32(value))
		r.GWAcquisitionOrderKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetTDSCDMABands); ok {
		if len(value) != 8 {
			return nasSelectionLengthError(nasTLVSelectionGetTDSCDMABands, len(value), 8)
		}
		r.TDSCDMABandPreference = NASTDSCDMABandPreference(binary.LittleEndian.Uint64(value))
		r.TDSCDMABandPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetManualPLMN); ok {
		if len(value) != 5 {
			return nasSelectionLengthError(nasTLVSelectionGetManualPLMN, len(value), 5)
		}
		r.ManualPLMN = NASPLMN{
			MCC:                 binary.LittleEndian.Uint16(value[0:2]),
			MNC:                 binary.LittleEndian.Uint16(value[2:4]),
			MNCThreeDigits:      value[4] != 0,
			MNCThreeDigitsKnown: true,
		}
		r.ManualPLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetAcquisitionOrder); ok {
		order, err := parseNASAcquisitionOrder(value)
		if err != nil {
			return err
		}
		r.AcquisitionOrder = order
		r.AcquisitionOrderKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetRegistrationRestrict); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGetRegistrationRestrict, len(value), 4)
		}
		r.RegistrationRestriction = NASRegistrationRestriction(binary.LittleEndian.Uint32(value))
		r.RegistrationRestrictionKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetUsagePreference); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGetUsagePreference, len(value), 4)
		}
		r.UsagePreference = NASUsagePreference(binary.LittleEndian.Uint32(value))
		r.UsagePreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetVoiceDomain); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGetVoiceDomain, len(value), 4)
		}
		r.VoiceDomain = NASVoiceDomainPreference(binary.LittleEndian.Uint32(value))
		r.VoiceDomainKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetLTEDisableCause); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGetLTEDisableCause, len(value), 4)
		}
		r.LTEDisableCause = NASLTEDisableCause(binary.LittleEndian.Uint32(value))
		r.LTEDisableCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetDisabledRATs); ok {
		if len(value) != 2 {
			return nasSelectionLengthError(nasTLVSelectionGetDisabledRATs, len(value), 2)
		}
		r.DisabledRATs = NASModePreference(binary.LittleEndian.Uint16(value))
		r.DisabledRATsKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetLTEBandsExtended); ok {
		var bands NASLTEBandPreferenceExtended
		if err := bands.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS extended LTE band preference: %w", err)
		}
		r.LTEBandsExtended = bands
		r.LTEBandsExtendedKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetNR5GBands); ok {
		var bands NASNR5GBandPreference
		if err := bands.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS NR5G bands: %w", err)
		}
		r.NR5GBands = bands
		r.NR5GBandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetNR5GDisableMode); ok {
		if len(value) != 4 {
			return nasSelectionLengthError(nasTLVSelectionGetNR5GDisableMode, len(value), 4)
		}
		r.NR5GDisableMode = NASNR5GDisableMode(binary.LittleEndian.Uint32(value))
		r.NR5GDisableModeKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetNR5GSABands); ok {
		var bands NASNR5GBandPreference
		if err := bands.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS NR5G SA bands: %w", err)
		}
		r.NR5GSABands = bands
		r.NR5GSABandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSelectionGetNR5GNSABands); ok {
		var bands NASNR5GBandPreference
		if err := bands.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS NR5G NSA bands: %w", err)
		}
		r.NR5GNSABands = bands
		r.NR5GNSABandsKnown = true
	}
	return nil
}

// SystemSelectionPreference returns the current modem selection policy.
func (c *Client) SystemSelectionPreference(ctx context.Context) (NASSystemSelectionPreference, error) {
	var result NASSystemSelectionPreference
	if err := c.nasRead(ctx, MessageNASGetSystemSelectionPreference, &result); err != nil {
		return NASSystemSelectionPreference{}, fmt.Errorf("querying QMI NAS system selection preference: %w", err)
	}
	return result, nil
}

// SetSystemSelectionPreference changes selected modem policy fields.
func (c *Client) SetSystemSelectionPreference(ctx context.Context, config NASSystemSelectionConfig) error {
	req, err := (NASSetSystemSelectionPreferenceRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI NAS system selection preference: %w", err)
	}
	return nil
}

// MarshalTLVs encodes system-selection fields.
func (config NASSystemSelectionConfig) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	if config.EmergencyMode != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionEmergencyMode, boolByte(*config.EmergencyMode)))
	}
	if config.ModePreference != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionModePreference, uint16(*config.ModePreference)))
	}
	if config.BandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVSelectionBandPreference, uint64(*config.BandPreference)))
	}
	if config.PRLPreference != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionPRLPreference, uint16(*config.PRLPreference)))
	}
	if config.RoamingPreference != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionRoamingPreference, uint16(*config.RoamingPreference)))
	}
	if config.LTEBandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVSelectionLTEBandPreference, uint64(*config.LTEBandPreference)))
	}
	if config.NetworkSelection != nil {
		selectionTLVs, err := config.NetworkSelection.MarshalTLVs()
		if err != nil {
			return nil, err
		}
		tlvs = append(tlvs, selectionTLVs...)
	}
	if config.ChangeDuration != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionChangeDuration, uint8(*config.ChangeDuration)))
	}
	if config.ServiceDomain != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionServiceDomain, uint32(*config.ServiceDomain)))
	}
	if config.GWAcquisitionOrder != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionGWAcquisitionOrder, uint32(*config.GWAcquisitionOrder)))
	}
	if config.TDSCDMABandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVSelectionSetTDSCDMABands, uint64(*config.TDSCDMABandPreference)))
	}
	if config.AcquisitionOrder != nil {
		if len(config.AcquisitionOrder) > nasMaxAcquisitionOrder {
			return nil, fmt.Errorf("encoding QMI NAS acquisition order: count %d exceeds maximum %d", len(config.AcquisitionOrder), nasMaxAcquisitionOrder)
		}
		value := make([]byte, 1, 1+len(config.AcquisitionOrder))
		value[0] = byte(len(config.AcquisitionOrder))
		for _, radio := range config.AcquisitionOrder {
			value = append(value, byte(radio))
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSelectionSetAcquisitionOrder, value))
	}
	if config.RegistrationRestrict != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionSetRegistrationRestrict, uint32(*config.RegistrationRestrict)))
	}
	if config.UsagePreference != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionSetUsagePreference, uint32(*config.UsagePreference)))
	}
	if config.VoiceDomain != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionSetVoiceDomain, uint32(*config.VoiceDomain)))
	}
	if config.LTEBandsExtended != nil {
		value, err := config.LTEBandsExtended.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding QMI NAS extended LTE band preference: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSelectionSetLTEBandsExtended, value))
	}
	if config.NR5GBands != nil {
		value, err := config.NR5GBands.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding QMI NAS NR5G bands: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSelectionSetNR5GBands, value))
	}
	if config.NR5GDisableMode != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionSetNR5GDisableMode, uint32(*config.NR5GDisableMode)))
	}
	if config.NR5GSABands != nil {
		value, err := config.NR5GSABands.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding QMI NAS NR5G SA bands: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSelectionSetNR5GSABands, value))
	}
	if config.NR5GNSABands != nil {
		value, err := config.NR5GNSABands.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding QMI NAS NR5G NSA bands: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSelectionSetNR5GNSABands, value))
	}
	return tlvs, nil
}

// MarshalTLVs encodes manual or automatic network-selection fields.
func (selection NASNetworkSelection) MarshalTLVs() (tlv.TLVs, error) {
	if selection.Mode > NASNetworkSelectionManual {
		return nil, fmt.Errorf("encoding QMI NAS network selection: mode %d is out of range", selection.Mode)
	}
	if selection.PLMN.MCC > 999 || selection.PLMN.MNC > 999 {
		return nil, fmt.Errorf("encoding QMI NAS network selection: PLMN %d/%d is out of range", selection.PLMN.MCC, selection.PLMN.MNC)
	}
	value := []byte{byte(selection.Mode)}
	value = binary.LittleEndian.AppendUint16(value, selection.PLMN.MCC)
	value = binary.LittleEndian.AppendUint16(value, selection.PLMN.MNC)
	tlvs := tlv.TLVs{tlv.Bytes(nasTLVSelectionNetwork, value)}
	if selection.PLMN.MNCThreeDigitsKnown {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionMNCThreeDigits, boolByte(selection.PLMN.MNCThreeDigits)))
	}
	if selection.RadioInterface != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSelectionSetRadioInterface, uint8(*selection.RadioInterface)))
	}
	return tlvs, nil
}

func parseNASAcquisitionOrder(value []byte) ([]NASRadioInterface, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS system selection: acquisition order count is truncated")
	}
	count := int(value[0])
	if count > nasMaxAcquisitionOrder {
		return nil, fmt.Errorf("parsing QMI NAS system selection: acquisition order count %d exceeds maximum %d", count, nasMaxAcquisitionOrder)
	}
	if len(value) != 1+count {
		return nil, fmt.Errorf("parsing QMI NAS system selection: acquisition order TLV length %d, want %d", len(value), 1+count)
	}
	order := make([]NASRadioInterface, count)
	for i, radio := range value[1:] {
		order[i] = NASRadioInterface(radio)
	}
	return order, nil
}

func nasSelectionLengthError(kind byte, got, want int) error {
	return fmt.Errorf("parsing QMI NAS system selection: TLV 0x%02x length %d, want %d", kind, got, want)
}

// MarshalBinary encodes the extended LTE band preference bitmap.
func (bands NASLTEBandPreferenceExtended) MarshalBinary() ([]byte, error) {
	value := binary.LittleEndian.AppendUint64(nil, bands.Bits1To64)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits65To128)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits129To192)
	return binary.LittleEndian.AppendUint64(value, bands.Bits193To256), nil
}

// UnmarshalBinary decodes the extended LTE band preference bitmap.
func (bands *NASLTEBandPreferenceExtended) UnmarshalBinary(value []byte) error {
	if len(value) != 32 {
		return fmt.Errorf("band preference length %d, want 32", len(value))
	}
	*bands = NASLTEBandPreferenceExtended{
		Bits1To64:    binary.LittleEndian.Uint64(value[0:8]),
		Bits65To128:  binary.LittleEndian.Uint64(value[8:16]),
		Bits129To192: binary.LittleEndian.Uint64(value[16:24]),
		Bits193To256: binary.LittleEndian.Uint64(value[24:32]),
	}
	return nil
}

// MarshalBinary encodes the NR5G band preference bitmap.
func (bands NASNR5GBandPreference) MarshalBinary() ([]byte, error) {
	value := binary.LittleEndian.AppendUint64(nil, bands.Bits1To64)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits65To128)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits129To192)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits193To256)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits257To320)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits321To384)
	value = binary.LittleEndian.AppendUint64(value, bands.Bits385To448)
	return binary.LittleEndian.AppendUint64(value, bands.Bits449To512), nil
}

// UnmarshalBinary decodes the NR5G band preference bitmap.
func (bands *NASNR5GBandPreference) UnmarshalBinary(value []byte) error {
	if len(value) != 64 {
		return fmt.Errorf("band preference length %d, want 64", len(value))
	}
	*bands = NASNR5GBandPreference{
		Bits1To64:    binary.LittleEndian.Uint64(value[0:8]),
		Bits65To128:  binary.LittleEndian.Uint64(value[8:16]),
		Bits129To192: binary.LittleEndian.Uint64(value[16:24]),
		Bits193To256: binary.LittleEndian.Uint64(value[24:32]),
		Bits257To320: binary.LittleEndian.Uint64(value[32:40]),
		Bits321To384: binary.LittleEndian.Uint64(value[40:48]),
		Bits385To448: binary.LittleEndian.Uint64(value[48:56]),
		Bits449To512: binary.LittleEndian.Uint64(value[56:64]),
	}
	return nil
}

func nasUint64TLV(typ byte, value uint64) tlv.TLV {
	return tlv.Bytes(typ, binary.LittleEndian.AppendUint64(nil, value))
}
