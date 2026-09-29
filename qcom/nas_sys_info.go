package qcom

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVSysInfoCDMAService      = 0x10
	nasTLVSysInfoHDRService       = 0x11
	nasTLVSysInfoGSMService       = 0x12
	nasTLVSysInfoWCDMAService     = 0x13
	nasTLVSysInfoLTEService       = 0x14
	nasTLVSysInfoCDMA             = 0x15
	nasTLVSysInfoHDR              = 0x16
	nasTLVSysInfoGSM              = 0x17
	nasTLVSysInfoWCDMA            = 0x18
	nasTLVSysInfoLTE              = 0x19
	nasTLVSysInfoLTEVoice         = 0x21
	nasTLVSysInfoTDSService       = 0x24
	nasTLVSysInfoTDS              = 0x25
	nasTLVSysInfoLTEVoPS          = 0x29
	nasTLVSysInfoLTEVoiceDomain   = 0x2A
	nasTLVSysInfoHDRVoiceDomain   = 0x36
	nasTLVSysInfoHDRSMSDomain     = 0x37
	nasTLVSysInfoLTESMSDomain     = 0x38
	nasTLVSysInfoGSMVoiceDomain   = 0x3A
	nasTLVSysInfoGSMSMSDomain     = 0x3B
	nasTLVSysInfoWCDMAVoiceDomain = 0x3C
	nasTLVSysInfoWCDMASMSDomain   = 0x3D
	nasTLVSysInfoCDMAVoiceDomain  = 0x3F
	nasTLVSysInfoCDMASMSDomain    = 0x40
	nasTLVSysInfoTDSVoiceDomain   = 0x41
	nasTLVSysInfoTDSSMSDomain     = 0x42
	nasTLVSysInfoNR5GService      = 0x4A
	nasTLVSysInfoNR5G             = 0x4B
	nasTLVSysInfoCPSMS            = 0x4D
	nasTLVSysInfoENDC             = 0x4E
	nasTLVSysInfoDCNRRestricted   = 0x4F
	nasTLVSysInfoNR5GTAC          = 0x50
	nasTLVSysInfoTARestricted     = 0x51
	nasTLVSysInfoN1SMS            = 0x52
	nasTLVSysInfoNR5GPCI          = 0x54
	nasTLVSysInfoNR5GVoiceDomain  = 0x56
	nasTLVSysInfoNR5GSMSDomain    = 0x57
	nasTLVSysInfoNR5GVoice        = 0x58
	nasTLVSysInfoNR5GVoPS         = 0x59
)

// NASServiceStatus describes whether a radio technology currently has service.
type NASServiceStatus uint8

const (
	NASServiceStatusNone NASServiceStatus = iota
	NASServiceStatusLimited
	NASServiceStatusAvailable
	NASServiceStatusLimitedRegional
	NASServiceStatusPowerSave
)

// NASServiceDomain identifies the circuit-switched and packet-switched domains.
type NASServiceDomain uint8

const (
	NASServiceDomainNone NASServiceDomain = iota
	NASServiceDomainCircuitSwitched
	NASServiceDomainPacketSwitched
	NASServiceDomainCircuitAndPacketSwitched
	NASServiceDomainCamped
)

// NASRoamingStatus is the roaming state reported for one radio technology.
type NASRoamingStatus uint8

const (
	NASRoamingStatusOff NASRoamingStatus = iota
	NASRoamingStatusOn
	NASRoamingStatusBlinking
	NASRoamingStatusOutOfNeighborhood
	NASRoamingStatusOutOfBuilding
	NASRoamingStatusPreferred
	NASRoamingStatusAvailable
	NASRoamingStatusAlliancePartner
	NASRoamingStatusPremiumPartner
	NASRoamingStatusFullService
	NASRoamingStatusPartialService
	NASRoamingStatusBannerOn
	NASRoamingStatusBannerOff
)

// NASVoiceDomain identifies the network selected for voice service.
type NASVoiceDomain uint32

const (
	NASVoiceDomainNone NASVoiceDomain = iota
	NASVoiceDomainIMS
	NASVoiceDomainOneX
	NASVoiceDomain3GPP
)

// NASSMSDomain identifies the network selected for SMS service.
type NASSMSDomain uint32

const (
	NASSMSDomainNone NASSMSDomain = iota
	NASSMSDomainIMS
	NASSMSDomainOneX
	NASSMSDomain3GPP
)

// NASCPSMSServiceStatus describes control-plane SMS availability.
type NASCPSMSServiceStatus uint32

const (
	NASCPSMSUnavailable NASCPSMSServiceStatus = iota
	NASCPSMSTemporaryFailure
	NASCPSMSAvailable
)

// NASRadioSystemInfo contains service and serving-cell state for one radio technology.
type NASRadioSystemInfo struct {
	ServiceStatus      NASServiceStatus
	ServiceStatusKnown bool

	TrueServiceStatus      NASServiceStatus
	TrueServiceStatusKnown bool
	PreferredDataPath      bool
	PreferredDataPathKnown bool

	SystemInfoKnown        bool
	ServiceDomain          NASServiceDomain
	ServiceDomainKnown     bool
	ServiceCapability      NASServiceDomain
	ServiceCapabilityKnown bool
	RoamingStatus          NASRoamingStatus
	RoamingStatusKnown     bool
	Forbidden              bool
	ForbiddenKnown         bool

	PLMN      NASPLMN
	PLMNKnown bool

	LocationAreaCode      uint16
	LocationAreaCodeKnown bool
	CellID                uint32
	CellIDKnown           bool
	TrackingAreaCode      uint32
	TrackingAreaCodeKnown bool
	PhysicalCellID        uint16
	PhysicalCellIDKnown   bool

	RegistrationRejectDomain NASServiceDomain
	RegistrationRejectCause  uint8
	RegistrationRejectKnown  bool

	VoiceDomain      NASVoiceDomain
	VoiceDomainKnown bool
	SMSDomain        NASSMSDomain
	SMSDomainKnown   bool

	VoiceSupported      bool
	VoiceSupportedKnown bool
	IMSVoiceAvailable   bool
	IMSVoiceKnown       bool
}

// NASSysInfo contains current service information for every supported radio technology.
type NASSysInfo struct {
	CDMA    NASRadioSystemInfo
	HDR     NASRadioSystemInfo
	GSM     NASRadioSystemInfo
	WCDMA   NASRadioSystemInfo
	LTE     NASRadioSystemInfo
	TDSCDMA NASRadioSystemInfo
	NR5G    NASRadioSystemInfo

	CPSMSServiceStatus          NASCPSMSServiceStatus
	CPSMSServiceStatusKnown     bool
	ENDCAvailable               bool
	ENDCAvailableKnown          bool
	DCNRRestricted              bool
	DCNRRestrictedKnown         bool
	TrackingAreaRestricted      bool
	TrackingAreaRestrictedKnown bool
	N1SMSRegistered             bool
	N1SMSRegisteredKnown        bool

	// VoPS mirrors the standard NAS system-info availability fields.
	VoPSKnown       bool
	VoPSSupported   bool
	NRVoPSKnown     bool
	NRVoPSSupported bool
}

// UnmarshalTLVs parses QMI NAS Get System Info response TLVs.
func (s *NASSysInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, MessageTypeResponse)
}

// UnmarshalIndicationTLVs parses QMI NAS System Info indication TLVs.
func (s *NASSysInfo) UnmarshalIndicationTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, MessageTypeIndication)
}

func (s *NASSysInfo) unmarshalTLVs(tlvs tlv.TLVs, messageType MessageType) error {
	*s = NASSysInfo{}
	// Each row lists the response and indication IDs from Qualcomm
	// nas_get_sys_info_resp_msg_data_v01 and nas_sys_info_ind_msg_data_v01.
	serviceTLVs := []struct {
		responseType   byte
		indicationType byte
		threeGPP       bool
		dst            *NASRadioSystemInfo
	}{
		{nasTLVSysInfoCDMAService, 0x10, false, &s.CDMA},
		{nasTLVSysInfoHDRService, 0x11, false, &s.HDR},
		{nasTLVSysInfoGSMService, 0x12, true, &s.GSM},
		{nasTLVSysInfoWCDMAService, 0x13, true, &s.WCDMA},
		{nasTLVSysInfoLTEService, 0x14, true, &s.LTE},
		{nasTLVSysInfoTDSService, 0x25, true, &s.TDSCDMA},
		{nasTLVSysInfoNR5GService, 0x4C, true, &s.NR5G},
	}
	for _, item := range serviceTLVs {
		tlvType := nasTLVType(messageType, item.responseType, item.indicationType)
		value, ok := tlv.Value(tlvs, tlvType)
		if !ok {
			continue
		}
		if err := parseNASServiceStatus(value, item.threeGPP, item.dst); err != nil {
			return fmt.Errorf("parsing QMI NAS service status TLV 0x%02X: %w", tlvType, err)
		}
	}

	systemTLVs := []struct {
		responseType   byte
		indicationType byte
		length         int
		threeGPP       bool
		trackingArea   bool
		dst            *NASRadioSystemInfo
	}{
		{nasTLVSysInfoCDMA, 0x15, 42, false, false, &s.CDMA},
		{nasTLVSysInfoHDR, 0x16, 31, false, false, &s.HDR},
		{nasTLVSysInfoGSM, 0x17, 30, true, false, &s.GSM},
		{nasTLVSysInfoWCDMA, 0x18, 33, true, false, &s.WCDMA},
		{nasTLVSysInfoLTE, 0x19, 29, true, true, &s.LTE},
		{nasTLVSysInfoTDS, 0x26, 50, true, false, &s.TDSCDMA},
		{nasTLVSysInfoNR5G, 0x4D, 29, true, true, &s.NR5G},
	}
	for _, item := range systemTLVs {
		tlvType := nasTLVType(messageType, item.responseType, item.indicationType)
		value, ok := tlv.Value(tlvs, tlvType)
		if !ok {
			continue
		}
		if err := parseNASSystemInfo(value, item.length, item.threeGPP, item.trackingArea, item.dst); err != nil {
			return fmt.Errorf("parsing QMI NAS system information TLV 0x%02X: %w", tlvType, err)
		}
	}

	voiceDomains := []struct {
		responseType   byte
		indicationType byte
		dst            *NASRadioSystemInfo
	}{
		{nasTLVSysInfoCDMAVoiceDomain, 0x40, &s.CDMA},
		{nasTLVSysInfoHDRVoiceDomain, 0x37, &s.HDR},
		{nasTLVSysInfoGSMVoiceDomain, 0x3B, &s.GSM},
		{nasTLVSysInfoWCDMAVoiceDomain, 0x3D, &s.WCDMA},
		{nasTLVSysInfoLTEVoiceDomain, 0x2B, &s.LTE},
		{nasTLVSysInfoTDSVoiceDomain, 0x42, &s.TDSCDMA},
		{nasTLVSysInfoNR5GVoiceDomain, 0x58, &s.NR5G},
	}
	for _, item := range voiceDomains {
		tlvType := nasTLVType(messageType, item.responseType, item.indicationType)
		value, ok := tlv.Value(tlvs, tlvType)
		if !ok {
			continue
		}
		domain, err := parseNASUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS voice-domain TLV 0x%02X: %w", tlvType, err)
		}
		item.dst.VoiceDomain = NASVoiceDomain(domain)
		item.dst.VoiceDomainKnown = true
	}

	smsDomains := []struct {
		responseType   byte
		indicationType byte
		dst            *NASRadioSystemInfo
	}{
		{nasTLVSysInfoCDMASMSDomain, 0x41, &s.CDMA},
		{nasTLVSysInfoHDRSMSDomain, 0x38, &s.HDR},
		{nasTLVSysInfoGSMSMSDomain, 0x3C, &s.GSM},
		{nasTLVSysInfoWCDMASMSDomain, 0x3E, &s.WCDMA},
		{nasTLVSysInfoLTESMSDomain, 0x39, &s.LTE},
		{nasTLVSysInfoTDSSMSDomain, 0x43, &s.TDSCDMA},
		{nasTLVSysInfoNR5GSMSDomain, 0x59, &s.NR5G},
	}
	for _, item := range smsDomains {
		tlvType := nasTLVType(messageType, item.responseType, item.indicationType)
		value, ok := tlv.Value(tlvs, tlvType)
		if !ok {
			continue
		}
		domain, err := parseNASUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS SMS-domain TLV 0x%02X: %w", tlvType, err)
		}
		item.dst.SMSDomain = NASSMSDomain(domain)
		item.dst.SMSDomainKnown = true
	}

	if err := s.parseCapabilities(tlvs, messageType); err != nil {
		return err
	}
	return nil
}

// UnmarshalTLVs parses a NAS Get System Info response.
func (r *NASGetSysInfoResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASGetSysInfoResponse{}
	return r.SysInfo.UnmarshalTLVs(tlvs)
}

// SystemInfo returns service and serving-cell information for every reported RAT.
func (c *Client) SystemInfo(ctx context.Context) (NASSysInfo, error) {
	var result NASSysInfo
	if err := c.nasRead(ctx, MessageNASGetSysInfo, &result); err != nil {
		return NASSysInfo{}, fmt.Errorf("querying QMI NAS system information: %w", err)
	}
	return result, nil
}

func (s *NASSysInfo) parseCapabilities(tlvs tlv.TLVs, messageType MessageType) error {
	boolTLVs := []struct {
		responseType   byte
		indicationType byte
		value          *bool
		known          *bool
	}{
		{nasTLVSysInfoLTEVoice, 0x21, &s.LTE.VoiceSupported, &s.LTE.VoiceSupportedKnown},
		{nasTLVSysInfoLTEVoPS, 0x2A, &s.LTE.IMSVoiceAvailable, &s.LTE.IMSVoiceKnown},
		{nasTLVSysInfoENDC, 0x50, &s.ENDCAvailable, &s.ENDCAvailableKnown},
		{nasTLVSysInfoDCNRRestricted, 0x51, &s.DCNRRestricted, &s.DCNRRestrictedKnown},
		{nasTLVSysInfoTARestricted, 0x53, &s.TrackingAreaRestricted, &s.TrackingAreaRestrictedKnown},
		{nasTLVSysInfoN1SMS, 0x54, &s.N1SMSRegistered, &s.N1SMSRegisteredKnown},
		{nasTLVSysInfoNR5GVoice, 0x5A, &s.NR5G.VoiceSupported, &s.NR5G.VoiceSupportedKnown},
		{nasTLVSysInfoNR5GVoPS, 0x5B, &s.NR5G.IMSVoiceAvailable, &s.NR5G.IMSVoiceKnown},
	}
	for _, item := range boolTLVs {
		tlvType := nasTLVType(messageType, item.responseType, item.indicationType)
		value, ok := tlv.Value(tlvs, tlvType)
		if !ok {
			continue
		}
		parsed, err := parseNASBool(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS TLV 0x%02X: %w", tlvType, err)
		}
		*item.value = parsed
		*item.known = true
	}
	s.VoPSKnown = s.LTE.IMSVoiceKnown
	s.VoPSSupported = s.LTE.IMSVoiceAvailable
	s.NRVoPSKnown = s.NR5G.IMSVoiceKnown
	s.NRVoPSSupported = s.NR5G.IMSVoiceAvailable

	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVSysInfoCPSMS, 0x4F)); ok {
		status, err := parseNASUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS control-plane SMS service status: %w", err)
		}
		s.CPSMSServiceStatus = NASCPSMSServiceStatus(status)
		s.CPSMSServiceStatusKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVSysInfoNR5GTAC, 0x52)); ok {
		if len(value) != 3 {
			return fmt.Errorf("parsing QMI NAS NR5G tracking area code: TLV length %d, want 3", len(value))
		}
		s.NR5G.TrackingAreaCode = uint32(value[0])<<16 | uint32(value[1])<<8 | uint32(value[2])
		s.NR5G.TrackingAreaCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVSysInfoNR5GPCI, 0x56)); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS NR5G physical cell ID: TLV length %d, want 2", len(value))
		}
		s.NR5G.PhysicalCellID = binary.LittleEndian.Uint16(value)
		s.NR5G.PhysicalCellIDKnown = true
	}
	return nil
}

func parseNASServiceStatus(value []byte, threeGPP bool, dst *NASRadioSystemInfo) error {
	want := 2
	if threeGPP {
		want = 3
	}
	if len(value) != want {
		return fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	dst.ServiceStatus = NASServiceStatus(value[0])
	dst.ServiceStatusKnown = true
	if threeGPP {
		dst.TrueServiceStatus = NASServiceStatus(value[1])
		dst.TrueServiceStatusKnown = true
		dst.PreferredDataPath = value[2] != 0
		dst.PreferredDataPathKnown = true
		return nil
	}
	dst.PreferredDataPath = value[1] != 0
	dst.PreferredDataPathKnown = true
	return nil
}

func parseNASSystemInfo(value []byte, want int, threeGPP, trackingArea bool, dst *NASRadioSystemInfo) error {
	if len(value) != want {
		return fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	dst.SystemInfoKnown = true
	dst.ServiceDomainKnown = value[0] != 0
	dst.ServiceDomain = NASServiceDomain(value[1])
	dst.ServiceCapabilityKnown = value[2] != 0
	dst.ServiceCapability = NASServiceDomain(value[3])
	dst.RoamingStatusKnown = value[4] != 0
	dst.RoamingStatus = NASRoamingStatus(value[5])
	dst.ForbiddenKnown = value[6] != 0
	dst.Forbidden = value[7] != 0
	if !threeGPP {
		return nil
	}

	dst.LocationAreaCodeKnown = value[8] != 0
	dst.LocationAreaCode = binary.LittleEndian.Uint16(value[9:11])
	dst.CellIDKnown = value[11] != 0
	dst.CellID = binary.LittleEndian.Uint32(value[12:16])
	dst.RegistrationRejectKnown = value[16] != 0
	dst.RegistrationRejectDomain = NASServiceDomain(value[17])
	dst.RegistrationRejectCause = value[18]
	if value[19] != 0 {
		plmn, err := parseNASSystemPLMN(value[20:26])
		if err != nil {
			return fmt.Errorf("PLMN: %w", err)
		}
		dst.PLMN = plmn
		dst.PLMNKnown = true
	}
	if trackingArea {
		dst.TrackingAreaCodeKnown = value[26] != 0
		dst.TrackingAreaCode = uint32(binary.LittleEndian.Uint16(value[27:29]))
	}
	return nil
}

func parseNASSystemPLMN(value []byte) (NASPLMN, error) {
	// System Info reserves six bytes and pads a two-digit MNC with 0xFF before
	// delegating the canonical decimal representation to NASPLMN.UnmarshalText.
	if len(value) != 6 {
		return NASPLMN{}, fmt.Errorf("PLMN length %d, want 6", len(value))
	}
	text := value
	if value[5] == 0xFF {
		text = value[:5]
	}
	var plmn NASPLMN
	if err := plmn.UnmarshalText(text); err != nil {
		return NASPLMN{}, err
	}
	return plmn, nil
}

func parseNASBool(value []byte) (bool, error) {
	if len(value) != 1 {
		return false, fmt.Errorf("boolean TLV length %d, want 1", len(value))
	}
	return value[0] != 0, nil
}

func parseNASUint32(value []byte) (uint32, error) {
	if len(value) != 4 {
		return 0, fmt.Errorf("uint32 TLV length %d, want 4", len(value))
	}
	return binary.LittleEndian.Uint32(value), nil
}
