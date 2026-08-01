package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVRoamingIndicator    = 0x10
	nasTLVDataCapabilities    = 0x11
	nasTLVCurrentPLMN         = 0x12
	nasTLVTimeZone            = 0x1A
	nasTLVDaylightSaving      = 0x1B
	nasTLVLocationAreaCode    = 0x1C
	nasTLVCellID              = 0x1D
	nasTLVTrackingAreaCode    = 0x24
	nasTLVMNCIncludesPCSDigit = 0x27
	nasTLVNetworkNameSource   = 0x29

	nasTLVSignalCDMA     = 0x10
	nasTLVSignalHDR      = 0x11
	nasTLVSignalGSM      = 0x12
	nasTLVSignalWCDMA    = 0x13
	nasTLVSignalLTE      = 0x14
	nasTLVSignalTDSRSCP  = 0x15
	nasTLVSignalTDS      = 0x16
	nasTLVSignalNR5G     = 0x17
	nasTLVSignalNR5GRSRQ = 0x18
	nasTLVSignalUMTSRSCP = 0x19

	nasTLVRFBandsExtended = 0x11
	nasTLVRFBandwidths    = 0x12

	nasTLVHomeMNCIncludesPCSDigit = 0x12
	nasTLVHomeNetworkNameSource   = 0x13

	nasTLVNetworkTime3GPP2 = 0x10
	nasTLVNetworkTime3GPP  = 0x11

	nasTLVNetworkTimeUniversal      = 0x01
	nasTLVNetworkTimeZone           = 0x10
	nasTLVNetworkTimeDaylightSaving = 0x11
	nasTLVNetworkTimeRadioInterface = 0x12

	nasMaxDataCapabilities = 10
	nasMaxRFBands          = 16
)

// NASSubscription identifies the subscription used by the NAS control point.
type NASSubscription uint8

const (
	NASSubscriptionPrimary NASSubscription = iota
	NASSubscriptionSecondary
	NASSubscriptionTertiary
)

// NASBindSubscriptionRequest encodes NAS Bind Subscription.
type NASBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  NASSubscription
}

// Request validates and converts the subscription into a QMI request.
func (r NASBindSubscriptionRequest) Request() (Request, error) {
	if r.Subscription > NASSubscriptionTertiary {
		return Request{}, fmt.Errorf("NAS subscription %d is out of range", r.Subscription)
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASBindSubscription,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(nasTLVServingSystem, uint8(r.Subscription))},
	}, nil
}

// NASRoamingIndicator identifies home or roaming service.
type NASRoamingIndicator uint8

const (
	NASRoamingIndicatorRoaming NASRoamingIndicator = iota
	NASRoamingIndicatorHome
)

// NASDataCapability identifies a packet-data technology available on the serving system.
type NASDataCapability uint8

const (
	NASDataCapabilityGPRS NASDataCapability = 1 + iota
	NASDataCapabilityEDGE
	NASDataCapabilityHSDPA
	NASDataCapabilityHSUPA
	NASDataCapabilityWCDMA
	NASDataCapabilityCDMA
	NASDataCapabilityEVDORev0
	NASDataCapabilityEVDORevA
	NASDataCapabilityGSM
	NASDataCapabilityEVDORevB
	NASDataCapabilityLTE
	NASDataCapabilityHSDPAPlus
	NASDataCapabilityDCHSDPAPlus
)

// NASNetworkNameSource identifies where an operator name came from.
type NASNetworkNameSource uint32

const (
	NASNetworkNameSourceUnknown NASNetworkNameSource = iota
	NASNetworkNameSourceOPLPNN
	NASNetworkNameSourceCPHSONS
	NASNetworkNameSourceNITZ
	NASNetworkNameSourceSE13
	NASNetworkNameSourceMCCMNC
	NASNetworkNameSourceSPN
)

// NASPLMN identifies a public land mobile network.
type NASPLMN struct {
	MCC         uint16
	MNC         uint16
	Description string

	MNCThreeDigits      bool
	MNCThreeDigitsKnown bool
}

// String returns MCC and MNC as their conventional decimal PLMN string.
func (p NASPLMN) String() string {
	if p.MNCThreeDigitsKnown && p.MNCThreeDigits {
		return fmt.Sprintf("%03d%03d", p.MCC, p.MNC)
	}
	return fmt.Sprintf("%03d%02d", p.MCC, p.MNC)
}

// MarshalText encodes the PLMN as five or six decimal digits.
func (p NASPLMN) MarshalText() ([]byte, error) {
	if p.MCC > 999 || p.MNC > 999 {
		return nil, fmt.Errorf("encoding PLMN: MCC %d or MNC %d is out of range", p.MCC, p.MNC)
	}
	if p.MNCThreeDigitsKnown && !p.MNCThreeDigits && p.MNC > 99 {
		return nil, fmt.Errorf("encoding PLMN: two-digit MNC %d is out of range", p.MNC)
	}
	return []byte(p.String()), nil
}

// UnmarshalText decodes a five- or six-digit decimal PLMN.
func (p *NASPLMN) UnmarshalText(text []byte) error {
	if len(text) != 5 && len(text) != 6 {
		return fmt.Errorf("decoding PLMN: value %q must contain 5 or 6 digits", text)
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return fmt.Errorf("decoding PLMN: value %q contains a non-decimal character", text)
		}
	}
	decimal := func(digits []byte) uint16 {
		var value uint16
		for _, digit := range digits {
			value = value*10 + uint16(digit-'0')
		}
		return value
	}
	*p = NASPLMN{
		MCC:                 decimal(text[:3]),
		MNC:                 decimal(text[3:]),
		MNCThreeDigits:      len(text) == 6,
		MNCThreeDigitsKnown: true,
	}
	return nil
}

// NASCommonSignalInfo contains RSSI and raw ECIO half-dB units.
type NASCommonSignalInfo struct {
	RSSI int8
	ECIO int16
}

// NASHDRSignalInfo contains HDR signal measurements.
type NASHDRSignalInfo struct {
	NASCommonSignalInfo
	SINRLevel uint8
	IO        int32
}

// NASLTESignalInfo contains LTE signal measurements.
type NASLTESignalInfo struct {
	RSSI int8
	RSRQ int8
	RSRP int16
	// SNR is reported in tenths of a decibel.
	SNR int16
}

// NASTDSCDMASignalInfo contains floating-point TD-SCDMA measurements in dB/dBm.
type NASTDSCDMASignalInfo struct {
	RSSI float32
	RSCP float32
	ECIO float32
	SINR float32
}

// NASNR5GSignalInfo contains NR5G signal measurements.
type NASNR5GSignalInfo struct {
	RSRP int16
	// SNR is reported in tenths of a decibel.
	SNR int16
}

// NASSignalInfo contains the measurements reported for each available RAT.
type NASSignalInfo struct {
	CDMA       NASCommonSignalInfo
	CDMAKnown  bool
	HDR        NASHDRSignalInfo
	HDRKnown   bool
	GSM        int8
	GSMKnown   bool
	WCDMA      NASCommonSignalInfo
	WCDMAKnown bool
	LTE        NASLTESignalInfo
	LTEKnown   bool

	TDSCDMARSCP      int8
	TDSCDMARSCPKnown bool
	TDSCDMA          NASTDSCDMASignalInfo
	TDSCDMAKnown     bool

	NR5G          NASNR5GSignalInfo
	NR5GKnown     bool
	NR5GRSRQ      int16
	NR5GRSRQKnown bool
	UMTSRSCP      int16
	UMTSRSCPKnown bool
}

// UnmarshalTLVs parses QMI NAS Get Signal Info response or Signal Info indication TLVs.
func (r *NASSignalInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASSignalInfo{}
	if value, ok := tlv.Value(tlvs, nasTLVSignalCDMA); ok {
		if err := r.CDMA.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS CDMA signal: %w", err)
		}
		r.CDMAKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalHDR); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI NAS HDR signal: TLV length %d, want 8", len(value))
		}
		r.HDR = NASHDRSignalInfo{
			NASCommonSignalInfo: NASCommonSignalInfo{
				RSSI: int8(value[0]),
				ECIO: int16(binary.LittleEndian.Uint16(value[1:3])),
			},
			SINRLevel: value[3],
			IO:        int32(binary.LittleEndian.Uint32(value[4:8])),
		}
		r.HDRKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalGSM); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS GSM signal: TLV length %d, want 1", len(value))
		}
		r.GSM = int8(value[0])
		r.GSMKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalWCDMA); ok {
		if err := r.WCDMA.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS WCDMA signal: %w", err)
		}
		r.WCDMAKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalLTE); ok {
		if len(value) != 6 {
			return fmt.Errorf("parsing QMI NAS LTE signal: TLV length %d, want 6", len(value))
		}
		r.LTE = NASLTESignalInfo{
			RSSI: int8(value[0]),
			RSRQ: int8(value[1]),
			RSRP: int16(binary.LittleEndian.Uint16(value[2:4])),
			SNR:  int16(binary.LittleEndian.Uint16(value[4:6])),
		}
		r.LTEKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalTDSRSCP); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS TD-SCDMA RSCP: TLV length %d, want 1", len(value))
		}
		r.TDSCDMARSCP = int8(value[0])
		r.TDSCDMARSCPKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalTDS); ok {
		if len(value) != 16 {
			return fmt.Errorf("parsing QMI NAS TD-SCDMA signal: TLV length %d, want 16", len(value))
		}
		r.TDSCDMA = NASTDSCDMASignalInfo{
			RSSI: math.Float32frombits(binary.LittleEndian.Uint32(value[0:4])),
			RSCP: math.Float32frombits(binary.LittleEndian.Uint32(value[4:8])),
			ECIO: math.Float32frombits(binary.LittleEndian.Uint32(value[8:12])),
			SINR: math.Float32frombits(binary.LittleEndian.Uint32(value[12:16])),
		}
		r.TDSCDMAKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalNR5G); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS NR5G signal: TLV length %d, want 4", len(value))
		}
		r.NR5G = NASNR5GSignalInfo{
			RSRP: int16(binary.LittleEndian.Uint16(value[0:2])),
			SNR:  int16(binary.LittleEndian.Uint16(value[2:4])),
		}
		r.NR5GKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalNR5GRSRQ); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS NR5G RSRQ: TLV length %d, want 2", len(value))
		}
		r.NR5GRSRQ = int16(binary.LittleEndian.Uint16(value))
		r.NR5GRSRQKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVSignalUMTSRSCP); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS UMTS RSCP: TLV length %d, want 2", len(value))
		}
		r.UMTSRSCP = int16(binary.LittleEndian.Uint16(value))
		r.UMTSRSCPKnown = true
	}
	return nil
}

// NASActiveBand is the Qualcomm active-band enumeration value.
type NASActiveBand uint16

// NASRFBand contains one currently active radio, band, and channel.
type NASRFBand struct {
	RadioInterface NASRadioInterface
	Band           NASActiveBand
	Channel        uint32
}

// NASBandwidth is the Qualcomm bandwidth enumeration value.
type NASBandwidth uint32

// NASRFBandwidth contains the bandwidth reported for one radio interface.
type NASRFBandwidth struct {
	RadioInterface NASRadioInterface
	Bandwidth      NASBandwidth
}

// NASRFDedicatedBand contains the dedicated band for one radio interface.
type NASRFDedicatedBand struct {
	RadioInterface NASRadioInterface
	Band           NASActiveBand
}

// NASRFBandInfo contains active bands and optional bandwidths.
type NASRFBandInfo struct {
	Bands    []NASRFBand
	Extended bool

	DedicatedBands      []NASRFDedicatedBand
	DedicatedBandsKnown bool
	Bandwidths          []NASRFBandwidth
	BandwidthsKnown     bool
	CIoTLTEMode         NASCIoTLTEMode
	CIoTLTEModeKnown    bool
}

// UnmarshalTLVs parses QMI NAS Get RF Band Info response TLVs.
func (r *NASRFBandInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASRFBandInfo{}
	value, ok := tlv.Value(tlvs, nasTLVServingSystem)
	if !ok {
		return errors.New("parsing QMI NAS RF band info: band list TLV missing")
	}
	bands, err := parseNASRFBands(value, false)
	if err != nil {
		return err
	}
	r.Bands = bands
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		dedicated, err := parseNASRFDedicatedBands(value)
		if err != nil {
			return err
		}
		r.DedicatedBands = dedicated
		r.DedicatedBandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVRFBandsExtended); ok {
		bands, err := parseNASRFBands(value, true)
		if err != nil {
			return err
		}
		r.Bands = bands
		r.Extended = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVRFBandwidths); ok {
		bandwidths, err := parseNASRFBandwidths(value)
		if err != nil {
			return err
		}
		r.Bandwidths = bandwidths
		r.BandwidthsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS RF band LTE mode: TLV length %d, want 4", len(value))
		}
		r.CIoTLTEMode = NASCIoTLTEMode(binary.LittleEndian.Uint32(value))
		r.CIoTLTEModeKnown = true
	}
	return nil
}

// UnmarshalIndicationTLVs parses QMI NAS RF Band Info indication TLVs. Unlike
// Get RF Band Info, each indication TLV contains one aggregate without a count.
func (r *NASRFBandInfo) UnmarshalIndicationTLVs(tlvs tlv.TLVs) error {
	*r = NASRFBandInfo{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI NAS RF band indication: band TLV missing")
	}
	band, err := parseNASRFBand(value, false)
	if err != nil {
		return err
	}
	r.Bands = []NASRFBand{band}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		var dedicated NASRFDedicatedBand
		if err := dedicated.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS dedicated RF band indication: %w", err)
		}
		r.DedicatedBands = []NASRFDedicatedBand{dedicated}
		r.DedicatedBandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		band, err := parseNASRFBand(value, true)
		if err != nil {
			return err
		}
		r.Bands = []NASRFBand{band}
		r.Extended = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		var bandwidth NASRFBandwidth
		if err := bandwidth.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI NAS RF bandwidth indication: %w", err)
		}
		r.Bandwidths = []NASRFBandwidth{bandwidth}
		r.BandwidthsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS RF band indication LTE mode: TLV length %d, want 4", len(value))
		}
		r.CIoTLTEMode = NASCIoTLTEMode(binary.LittleEndian.Uint32(value))
		r.CIoTLTEModeKnown = true
	}
	return nil
}

// NASHomeNetwork contains the provisioned home PLMN and name metadata.
type NASHomeNetwork struct {
	PLMN NASPLMN

	Is3GPP          bool
	Is3GPPKnown     bool
	NameSource      NASNetworkNameSource
	NameSourceKnown bool
}

// UnmarshalTLVs parses QMI NAS Get Home Network response TLVs.
func (r *NASHomeNetwork) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASHomeNetwork{}
	value, ok := tlv.Value(tlvs, nasTLVServingSystem)
	if !ok {
		return errors.New("parsing QMI NAS home network: home-network TLV missing")
	}
	plmn, err := parseNASPLMN(value)
	if err != nil {
		return fmt.Errorf("parsing QMI NAS home network: %w", err)
	}
	r.PLMN = plmn
	if value, ok := tlv.Value(tlvs, nasTLVHomeMNCIncludesPCSDigit); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS home network: MNC digit TLV length %d, want 2", len(value))
		}
		r.Is3GPP = value[0] != 0
		r.Is3GPPKnown = true
		r.PLMN.MNCThreeDigits = value[1] != 0
		r.PLMN.MNCThreeDigitsKnown = r.Is3GPP
	}
	if value, ok := tlv.Value(tlvs, nasTLVHomeNetworkNameSource); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS home network: name-source TLV length %d, want 4", len(value))
		}
		r.NameSource = NASNetworkNameSource(binary.LittleEndian.Uint32(value))
		r.NameSourceKnown = true
	}
	return nil
}

// NASNetworkTime is one network-provided civil timestamp.
type NASNetworkTime struct {
	Year      uint16
	Month     uint8
	Day       uint8
	Hour      uint8
	Minute    uint8
	Second    uint8
	DayOfWeek uint8

	TimeZoneQuarterHours int8
	DaylightSavingHours  uint8
	RadioInterface       NASRadioInterface
}

// NASNetworkTimes contains the latest 3GPP and 3GPP2 timestamps.
type NASNetworkTimes struct {
	ThreeGPP2      NASNetworkTime
	ThreeGPP2Known bool
	ThreeGPP       NASNetworkTime
	ThreeGPPKnown  bool
}

// NASNetworkTimeUpdate is the timestamp carried by a Network Time indication.
type NASNetworkTimeUpdate struct {
	Time NASNetworkTime

	TimeZoneKnown       bool
	DaylightSavingKnown bool
	RadioInterfaceKnown bool
}

// UnmarshalTLVs parses a QMI NAS Network Time indication.
func (r *NASNetworkTimeUpdate) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASNetworkTimeUpdate{}
	value, ok := tlv.Value(tlvs, nasTLVNetworkTimeUniversal)
	if !ok {
		return errors.New("parsing QMI NAS network time indication: universal time TLV is missing")
	}
	if len(value) != 8 {
		return fmt.Errorf("parsing QMI NAS network time indication: universal time TLV length %d, want 8", len(value))
	}
	r.Time = NASNetworkTime{
		Year:      binary.LittleEndian.Uint16(value[0:2]),
		Month:     value[2],
		Day:       value[3],
		Hour:      value[4],
		Minute:    value[5],
		Second:    value[6],
		DayOfWeek: value[7],
	}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkTimeZone); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS network time indication: time zone TLV length %d, want 1", len(value))
		}
		r.Time.TimeZoneQuarterHours = int8(value[0])
		r.TimeZoneKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkTimeDaylightSaving); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS network time indication: daylight-saving TLV length %d, want 1", len(value))
		}
		r.Time.DaylightSavingHours = value[0]
		r.DaylightSavingKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkTimeRadioInterface); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS network time indication: radio-interface TLV length %d, want 1", len(value))
		}
		r.Time.RadioInterface = NASRadioInterface(value[0])
		r.RadioInterfaceKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get Network Time response TLVs.
func (r *NASNetworkTimes) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASNetworkTimes{}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkTime3GPP2); ok {
		networkTime, err := parseNASNetworkTime(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS 3GPP2 network time: %w", err)
		}
		r.ThreeGPP2 = networkTime
		r.ThreeGPP2Known = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkTime3GPP); ok {
		networkTime, err := parseNASNetworkTime(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS 3GPP network time: %w", err)
		}
		r.ThreeGPP = networkTime
		r.ThreeGPPKnown = true
	}
	return nil
}

// SignalInfo returns current signal measurements for every reported RAT.
func (c *Client) SignalInfo(ctx context.Context) (NASSignalInfo, error) {
	var result NASSignalInfo
	if err := c.nasRead(ctx, MessageNASGetSignalInfo, &result); err != nil {
		return NASSignalInfo{}, fmt.Errorf("querying QMI NAS signal information: %w", err)
	}
	return result, nil
}

// RFBandInfo returns active radio bands, channels, and optional bandwidths.
func (c *Client) RFBandInfo(ctx context.Context) (NASRFBandInfo, error) {
	var result NASRFBandInfo
	if err := c.nasRead(ctx, MessageNASGetRFBandInfo, &result); err != nil {
		return NASRFBandInfo{}, fmt.Errorf("querying QMI NAS RF band information: %w", err)
	}
	return result, nil
}

// HomeNetwork returns the provisioned home network.
func (c *Client) HomeNetwork(ctx context.Context) (NASHomeNetwork, error) {
	var result NASHomeNetwork
	if err := c.nasRead(ctx, MessageNASGetHomeNetwork, &result); err != nil {
		return NASHomeNetwork{}, fmt.Errorf("querying QMI NAS home network: %w", err)
	}
	return result, nil
}

// NetworkTime returns the latest network-provided 3GPP and 3GPP2 times.
func (c *Client) NetworkTime(ctx context.Context) (NASNetworkTimes, error) {
	var result NASNetworkTimes
	if err := c.nasRead(ctx, MessageNASGetNetworkTime, &result); err != nil {
		return NASNetworkTimes{}, fmt.Errorf("querying QMI NAS network time: %w", err)
	}
	return result, nil
}

// NASBindSubscription associates the NAS client with a modem subscription.
func (c *Client) NASBindSubscription(ctx context.Context, subscription NASSubscription) error {
	req, err := (NASBindSubscriptionRequest{
		Timeout:      DefaultRequestTimeout,
		Subscription: subscription,
	}).Request()
	if err != nil {
		return fmt.Errorf("binding QMI NAS subscription: %w", err)
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("binding QMI NAS subscription: %w", err)
	}
	return nil
}

func (c *Client) nasRead(ctx context.Context, message MessageID, dst tlvUnmarshaler) error {
	return c.nasReadRequest(ctx, nasEmptyRequest(0, 0, DefaultRequestTimeout, message), dst)
}

func (c *Client) nasReadRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceNAS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceNAS, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if dst == nil {
			return nil
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func nasEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServiceNAS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func (signal NASCommonSignalInfo) MarshalBinary() ([]byte, error) {
	value := []byte{byte(signal.RSSI)}
	return binary.LittleEndian.AppendUint16(value, uint16(signal.ECIO)), nil
}

func (signal *NASCommonSignalInfo) UnmarshalBinary(value []byte) error {
	if len(value) != 3 {
		return fmt.Errorf("common signal length %d, want 3", len(value))
	}
	*signal = NASCommonSignalInfo{
		RSSI: int8(value[0]),
		ECIO: int16(binary.LittleEndian.Uint16(value[1:3])),
	}
	return nil
}

func parseNASRFBands(value []byte, extended bool) ([]NASRFBand, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS RF band info: count is truncated")
	}
	count := int(value[0])
	if count > nasMaxRFBands {
		return nil, fmt.Errorf("parsing QMI NAS RF band info: count %d exceeds maximum %d", count, nasMaxRFBands)
	}
	entryLength := 5
	if extended {
		entryLength = 7
	}
	want := 1 + count*entryLength
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS RF band info: TLV length %d, want %d", len(value), want)
	}
	bands := make([]NASRFBand, count)
	for i := range count {
		offset := 1 + i*entryLength
		band, err := parseNASRFBand(value[offset:offset+entryLength], extended)
		if err != nil {
			return nil, err
		}
		bands[i] = band
	}
	return bands, nil
}

func parseNASRFBand(value []byte, extended bool) (NASRFBand, error) {
	// The legacy and extended TLVs encode the same semantic value with
	// different field widths, so NASRFBand has no single binary representation.
	want := 5
	if extended {
		want = 7
	}
	if len(value) != want {
		return NASRFBand{}, fmt.Errorf("parsing QMI NAS RF band aggregate: length %d, want %d", len(value), want)
	}
	band := NASRFBand{
		RadioInterface: NASRadioInterface(value[0]),
		Band:           NASActiveBand(binary.LittleEndian.Uint16(value[1:3])),
	}
	if extended {
		band.Channel = binary.LittleEndian.Uint32(value[3:7])
		return band, nil
	}
	band.Channel = uint32(binary.LittleEndian.Uint16(value[3:5]))
	return band, nil
}

func parseNASRFDedicatedBands(value []byte) ([]NASRFDedicatedBand, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS dedicated RF bands: count is truncated")
	}
	count := int(value[0])
	if count > nasMaxRFBands {
		return nil, fmt.Errorf("parsing QMI NAS dedicated RF bands: count %d exceeds maximum %d", count, nasMaxRFBands)
	}
	const entryLength = 3
	want := 1 + count*entryLength
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS dedicated RF bands: TLV length %d, want %d", len(value), want)
	}
	bands := make([]NASRFDedicatedBand, count)
	for i := range count {
		offset := 1 + i*entryLength
		if err := bands[i].UnmarshalBinary(value[offset : offset+entryLength]); err != nil {
			return nil, fmt.Errorf("parsing QMI NAS dedicated RF band %d: %w", i, err)
		}
	}
	return bands, nil
}

func (band NASRFDedicatedBand) MarshalBinary() ([]byte, error) {
	value := []byte{byte(band.RadioInterface)}
	return binary.LittleEndian.AppendUint16(value, uint16(band.Band)), nil
}

func (band *NASRFDedicatedBand) UnmarshalBinary(value []byte) error {
	if len(value) != 3 {
		return fmt.Errorf("dedicated RF band length %d, want 3", len(value))
	}
	*band = NASRFDedicatedBand{
		RadioInterface: NASRadioInterface(value[0]),
		Band:           NASActiveBand(binary.LittleEndian.Uint16(value[1:3])),
	}
	return nil
}

func parseNASRFBandwidths(value []byte) ([]NASRFBandwidth, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS RF bandwidths: count is truncated")
	}
	count := int(value[0])
	if count > nasMaxRFBands {
		return nil, fmt.Errorf("parsing QMI NAS RF bandwidths: count %d exceeds maximum %d", count, nasMaxRFBands)
	}
	want := 1 + count*5
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS RF bandwidths: TLV length %d, want %d", len(value), want)
	}
	bandwidths := make([]NASRFBandwidth, count)
	for i := range count {
		offset := 1 + i*5
		if err := bandwidths[i].UnmarshalBinary(value[offset : offset+5]); err != nil {
			return nil, fmt.Errorf("parsing QMI NAS RF bandwidth %d: %w", i, err)
		}
	}
	return bandwidths, nil
}

func (bandwidth NASRFBandwidth) MarshalBinary() ([]byte, error) {
	value := []byte{byte(bandwidth.RadioInterface)}
	return binary.LittleEndian.AppendUint32(value, uint32(bandwidth.Bandwidth)), nil
}

func (bandwidth *NASRFBandwidth) UnmarshalBinary(value []byte) error {
	if len(value) != 5 {
		return fmt.Errorf("RF bandwidth length %d, want 5", len(value))
	}
	*bandwidth = NASRFBandwidth{
		RadioInterface: NASRadioInterface(value[0]),
		Bandwidth:      NASBandwidth(binary.LittleEndian.Uint32(value[1:5])),
	}
	return nil
}

func parseNASPLMN(value []byte) (NASPLMN, error) {
	// This QMI aggregate also carries a description and omits the MNC digit
	// width, unlike the canonical decimal text form implemented by NASPLMN.
	if len(value) < 5 {
		return NASPLMN{}, fmt.Errorf("PLMN aggregate length %d, want at least 5", len(value))
	}
	descriptionLength := int(value[4])
	want := 5 + descriptionLength
	if len(value) != want {
		return NASPLMN{}, fmt.Errorf("PLMN aggregate length %d, want %d", len(value), want)
	}
	return NASPLMN{
		MCC:         binary.LittleEndian.Uint16(value[0:2]),
		MNC:         binary.LittleEndian.Uint16(value[2:4]),
		Description: string(value[5:]),
	}, nil
}

func parseNASNetworkTime(value []byte) (NASNetworkTime, error) {
	// Get Network Time uses this 11-byte aggregate; the indication splits the
	// same semantic timestamp across a shorter core value and optional TLVs.
	if len(value) != 11 {
		return NASNetworkTime{}, fmt.Errorf("TLV length %d, want 11", len(value))
	}
	return NASNetworkTime{
		Year:                 binary.LittleEndian.Uint16(value[0:2]),
		Month:                value[2],
		Day:                  value[3],
		Hour:                 value[4],
		Minute:               value[5],
		Second:               value[6],
		DayOfWeek:            value[7],
		TimeZoneQuarterHours: int8(value[8]),
		DaylightSavingHours:  value[9],
		RadioInterface:       NASRadioInterface(value[10]),
	}, nil
}
