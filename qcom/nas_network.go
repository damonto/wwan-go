package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	// Network scans routinely exceed the normal request timeout while the modem
	// searches every enabled RAT and band.
	DefaultNASNetworkScanTimeout = 3 * time.Minute

	nasTLVScanNetworkTypes      = 0x10
	nasTLVScanType              = 0x11
	nasTLVScanBandPreference    = 0x12
	nasTLVScanLTEBandPreference = 0x13
	nasTLVScanTDSBandPreference = 0x14
	nasTLVScanScope             = 0x1A
	nasTLVScanLTEBandsExtended  = 0x1B

	nasTLVScanNetworks      = 0x10
	nasTLVScanRadioAccess   = 0x11
	nasTLVScanMNCThreeDigit = 0x12
	nasTLVScanResult        = 0x13
	nasTLVScanNameSources   = 0x16

	nasTLVRegisterAction        = 0x01
	nasTLVRegisterManualNetwork = 0x10
	nasTLVRegisterDuration      = 0x11
	nasTLVRegisterMNCThreeDigit = 0x12

	nasMaxVisibleNetworks = 40
)

// NASNetworkTypeMask selects radio families for a network scan.
type NASNetworkTypeMask uint8

const (
	NASNetworkTypeGSM NASNetworkTypeMask = 1 << iota
	NASNetworkTypeUMTS
	NASNetworkTypeLTE
	NASNetworkTypeTDSCDMA
)

// NASNetworkScanType identifies the information requested by a scan.
type NASNetworkScanType uint32

const (
	NASNetworkScanPLMN NASNetworkScanType = iota
	NASNetworkScanCSG
	NASNetworkScanModePreference
	NASNetworkScanPCI
	NASNetworkScanCellSearch
)

// NASNetworkScanScope chooses a full-band or acquisition-database scan.
type NASNetworkScanScope uint32

const (
	NASNetworkScanFullBand NASNetworkScanScope = iota
	NASNetworkScanAcquisitionDatabase
)

// NASNetworkScanResult is the modem-reported completion status.
type NASNetworkScanResult uint32

const (
	NASNetworkScanSucceeded NASNetworkScanResult = iota
	NASNetworkScanAborted
	NASNetworkScanRejectedDuringRLF
)

// NASNetworkStatus is the packed in-use, roaming, forbidden, and preference status.
type NASNetworkStatus uint8

// NASNetworkInUseStatus identifies whether a visible network is serving or available.
type NASNetworkInUseStatus uint8

const (
	NASNetworkInUseUnknown NASNetworkInUseStatus = iota
	NASNetworkInUseCurrent
	NASNetworkInUseAvailable
)

// NASNetworkRoamingStatus identifies home or roaming status.
type NASNetworkRoamingStatus uint8

const (
	NASNetworkRoamingUnknown NASNetworkRoamingStatus = iota
	NASNetworkRoamingHome
	NASNetworkRoaming
)

// NASNetworkForbiddenStatus identifies whether registration is forbidden.
type NASNetworkForbiddenStatus uint8

const (
	NASNetworkForbiddenUnknown NASNetworkForbiddenStatus = iota
	NASNetworkForbidden
	NASNetworkAllowed
)

// NASNetworkPreferredStatus identifies operator preference.
type NASNetworkPreferredStatus uint8

const (
	NASNetworkPreferredUnknown NASNetworkPreferredStatus = iota
	NASNetworkPreferred
	NASNetworkNotPreferred
)

// InUse returns the in-use bits from the packed status.
func (s NASNetworkStatus) InUse() NASNetworkInUseStatus {
	return NASNetworkInUseStatus(s & 0x03)
}

// Roaming returns the roaming bits from the packed status.
func (s NASNetworkStatus) Roaming() NASNetworkRoamingStatus {
	return NASNetworkRoamingStatus((s >> 2) & 0x03)
}

// Forbidden returns the registration-forbidden bits from the packed status.
func (s NASNetworkStatus) Forbidden() NASNetworkForbiddenStatus {
	return NASNetworkForbiddenStatus((s >> 4) & 0x03)
}

// Preferred returns the operator-preference bits from the packed status.
func (s NASNetworkStatus) Preferred() NASNetworkPreferredStatus {
	return NASNetworkPreferredStatus((s >> 6) & 0x03)
}

// NASVisibleNetwork is one PLMN returned by a network scan.
type NASVisibleNetwork struct {
	PLMN            NASPLMN
	Status          NASNetworkStatus
	RadioInterfaces []NASRadioInterface
	NameSource      NASNetworkNameSource
	NameSourceKnown bool
}

// NASNetworkScan contains visible networks and the modem completion status.
type NASNetworkScan struct {
	Networks    []NASVisibleNetwork
	Result      NASNetworkScanResult
	ResultKnown bool
}

// NASNetworkScanConfig contains optional network-scan filters.
type NASNetworkScanConfig struct {
	NetworkTypes      *NASNetworkTypeMask
	ScanType          *NASNetworkScanType
	BandPreference    *NASBandPreference
	LTEBandPreference *NASLTEBandPreference
	TDSBandPreference *NASTDSCDMABandPreference
	Scope             *NASNetworkScanScope
	LTEBandsExtended  *NASLTEBandPreferenceExtended
	Timeout           time.Duration
}

// NASPerformNetworkScanRequest encodes Perform Network Scan.
type NASPerformNetworkScanRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASNetworkScanConfig
}

// Request converts the request into a QMI NAS request.
func (r NASPerformNetworkScanRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASPerformNetworkScan,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// UnmarshalTLVs parses Perform Network Scan response TLVs.
func (r *NASNetworkScan) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASNetworkScan{}
	if value, ok := tlv.Value(tlvs, nasTLVScanNetworks); ok {
		networks, err := parseNASVisibleNetworks(value)
		if err != nil {
			return err
		}
		r.Networks = networks
	}
	if value, ok := tlv.Value(tlvs, nasTLVScanNameSources); ok {
		if err := applyNASNetworkNameSources(r.Networks, value); err != nil {
			return err
		}
	}
	if value, ok := tlv.Value(tlvs, nasTLVScanRadioAccess); ok {
		if err := applyNASNetworkRadios(&r.Networks, value); err != nil {
			return err
		}
	}
	if value, ok := tlv.Value(tlvs, nasTLVScanMNCThreeDigit); ok {
		if err := applyNASNetworkMNCDigits(&r.Networks, value); err != nil {
			return err
		}
	}
	if value, ok := tlv.Value(tlvs, nasTLVScanResult); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS network scan: result TLV length %d, want 4", len(value))
		}
		r.Result = NASNetworkScanResult(binary.LittleEndian.Uint32(value))
		r.ResultKnown = true
	}
	return nil
}

// NetworkScan performs a synchronous visible-network scan.
func (c *Client) NetworkScan(ctx context.Context, config NASNetworkScanConfig) (NASNetworkScan, error) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = DefaultNASNetworkScanTimeout
	}
	req, err := (NASPerformNetworkScanRequest{Timeout: timeout, Config: config}).Request()
	if err != nil {
		return NASNetworkScan{}, err
	}
	var result NASNetworkScan
	if err := c.nasReadRequest(ctx, req, &result); err != nil {
		return NASNetworkScan{}, fmt.Errorf("scanning QMI NAS networks: %w", err)
	}
	return result, nil
}

// NASRegisterAction chooses automatic or manual registration.
type NASRegisterAction uint8

const (
	NASRegisterAutomatically NASRegisterAction = 1 + iota
	NASRegisterManually
)

// NASManualRegistration identifies a PLMN and RAT for manual registration.
type NASManualRegistration struct {
	PLMN           NASPLMN
	RadioInterface NASRadioInterface
}

// NASNetworkRegistration configures one registration attempt.
type NASNetworkRegistration struct {
	Action         NASRegisterAction
	Manual         *NASManualRegistration
	ChangeDuration *NASChangeDuration
}

// NASInitiateNetworkRegisterRequest encodes Initiate Network Register.
type NASInitiateNetworkRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Registration  NASNetworkRegistration
}

// Request converts the request into a QMI NAS request.
func (r NASInitiateNetworkRegisterRequest) Request() (Request, error) {
	tlvs, err := r.Registration.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASInitiateNetworkRegister,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// RegisterNetwork initiates automatic or manual network registration.
func (c *Client) RegisterNetwork(ctx context.Context, registration NASNetworkRegistration) error {
	req, err := (NASInitiateNetworkRegisterRequest{
		Timeout:      DefaultRequestTimeout,
		Registration: registration,
	}).Request()
	if err != nil {
		return err
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("registering QMI NAS network: %w", err)
	}
	return nil
}

// MarshalTLVs encodes network-scan configuration fields.
func (config NASNetworkScanConfig) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	if config.NetworkTypes != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVScanNetworkTypes, uint8(*config.NetworkTypes)))
	}
	if config.ScanType != nil {
		if *config.ScanType > NASNetworkScanCellSearch {
			return nil, fmt.Errorf("encoding QMI NAS network scan: scan type %d is out of range", *config.ScanType)
		}
		tlvs = append(tlvs, tlv.Uint(nasTLVScanType, uint32(*config.ScanType)))
	}
	if config.BandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVScanBandPreference, uint64(*config.BandPreference)))
	}
	if config.LTEBandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVScanLTEBandPreference, uint64(*config.LTEBandPreference)))
	}
	if config.TDSBandPreference != nil {
		tlvs = append(tlvs, nasUint64TLV(nasTLVScanTDSBandPreference, uint64(*config.TDSBandPreference)))
	}
	if config.Scope != nil {
		if *config.Scope > NASNetworkScanAcquisitionDatabase {
			return nil, fmt.Errorf("encoding QMI NAS network scan: scope %d is out of range", *config.Scope)
		}
		tlvs = append(tlvs, tlv.Uint(nasTLVScanScope, uint32(*config.Scope)))
	}
	if config.LTEBandsExtended != nil {
		value, err := config.LTEBandsExtended.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding QMI NAS extended LTE band preference: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVScanLTEBandsExtended, value))
	}
	return tlvs, nil
}

// MarshalTLVs encodes network-registration fields.
func (registration NASNetworkRegistration) MarshalTLVs() (tlv.TLVs, error) {
	if registration.Action != NASRegisterAutomatically && registration.Action != NASRegisterManually {
		return nil, fmt.Errorf("encoding QMI NAS network registration: action %d is out of range", registration.Action)
	}
	if registration.Action == NASRegisterManually && registration.Manual == nil {
		return nil, errors.New("encoding QMI NAS network registration: manual network is required")
	}
	tlvs := tlv.TLVs{tlv.Uint(nasTLVRegisterAction, uint8(registration.Action))}
	if registration.Manual != nil {
		if registration.Manual.PLMN.MCC > 999 || registration.Manual.PLMN.MNC > 999 {
			return nil, fmt.Errorf("encoding QMI NAS network registration: PLMN %d/%d is out of range", registration.Manual.PLMN.MCC, registration.Manual.PLMN.MNC)
		}
		value := binary.LittleEndian.AppendUint16(nil, registration.Manual.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, registration.Manual.PLMN.MNC)
		value = append(value, byte(registration.Manual.RadioInterface))
		tlvs = append(tlvs, tlv.Bytes(nasTLVRegisterManualNetwork, value))
		if registration.Manual.PLMN.MNCThreeDigitsKnown {
			tlvs = append(tlvs, tlv.Uint(nasTLVRegisterMNCThreeDigit, boolByte(registration.Manual.PLMN.MNCThreeDigits)))
		}
	}
	if registration.ChangeDuration != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterDuration, uint8(*registration.ChangeDuration)))
	}
	return tlvs, nil
}

func parseNASVisibleNetworks(value []byte) ([]NASVisibleNetwork, error) {
	if len(value) < 2 {
		return nil, errors.New("parsing QMI NAS network scan: network count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[0:2]))
	if count > nasMaxVisibleNetworks {
		return nil, fmt.Errorf("parsing QMI NAS network scan: network count %d exceeds maximum %d", count, nasMaxVisibleNetworks)
	}
	networks := make([]NASVisibleNetwork, count)
	offset := 2
	for i := range count {
		if len(value)-offset < 6 {
			return nil, fmt.Errorf("parsing QMI NAS network scan: network %d is truncated", i)
		}
		descriptionLength := int(value[offset+5])
		end := offset + 6 + descriptionLength
		if end > len(value) {
			return nil, fmt.Errorf("parsing QMI NAS network scan: network %d description is truncated", i)
		}
		networks[i] = NASVisibleNetwork{
			PLMN: NASPLMN{
				MCC:         binary.LittleEndian.Uint16(value[offset : offset+2]),
				MNC:         binary.LittleEndian.Uint16(value[offset+2 : offset+4]),
				Description: string(value[offset+6 : end]),
			},
			Status: NASNetworkStatus(value[offset+4]),
		}
		offset = end
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI NAS network scan: network list has %d trailing bytes", len(value)-offset)
	}
	return networks, nil
}

func applyNASNetworkRadios(networks *[]NASVisibleNetwork, value []byte) error {
	if len(value) < 2 {
		return errors.New("parsing QMI NAS network scan: radio-access count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[0:2]))
	if count > nasMaxVisibleNetworks {
		return fmt.Errorf("parsing QMI NAS network scan: radio-access count %d exceeds maximum %d", count, nasMaxVisibleNetworks)
	}
	want := 2 + count*5
	if len(value) != want {
		return fmt.Errorf("parsing QMI NAS network scan: radio-access TLV length %d, want %d", len(value), want)
	}
	for i := range count {
		offset := 2 + i*5
		mcc := binary.LittleEndian.Uint16(value[offset : offset+2])
		mnc := binary.LittleEndian.Uint16(value[offset+2 : offset+4])
		radio := NASRadioInterface(value[offset+4])
		network := findNASVisibleNetwork(*networks, mcc, mnc)
		if network == nil {
			*networks = append(*networks, NASVisibleNetwork{
				PLMN:            NASPLMN{MCC: mcc, MNC: mnc},
				RadioInterfaces: []NASRadioInterface{radio},
			})
			continue
		}
		if !slices.Contains(network.RadioInterfaces, radio) {
			network.RadioInterfaces = append(network.RadioInterfaces, radio)
		}
	}
	return nil
}

func applyNASNetworkMNCDigits(networks *[]NASVisibleNetwork, value []byte) error {
	if len(value) < 2 {
		return errors.New("parsing QMI NAS network scan: MNC digit count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[0:2]))
	if count > nasMaxVisibleNetworks {
		return fmt.Errorf("parsing QMI NAS network scan: MNC digit count %d exceeds maximum %d", count, nasMaxVisibleNetworks)
	}
	want := 2 + count*5
	if len(value) != want {
		return fmt.Errorf("parsing QMI NAS network scan: MNC digit TLV length %d, want %d", len(value), want)
	}
	for i := range count {
		offset := 2 + i*5
		mcc := binary.LittleEndian.Uint16(value[offset : offset+2])
		mnc := binary.LittleEndian.Uint16(value[offset+2 : offset+4])
		known := false
		for j := range *networks {
			if (*networks)[j].PLMN.MCC != mcc || (*networks)[j].PLMN.MNC != mnc {
				continue
			}
			(*networks)[j].PLMN.MNCThreeDigits = value[offset+4] != 0
			(*networks)[j].PLMN.MNCThreeDigitsKnown = true
			known = true
		}
		if !known {
			*networks = append(*networks, NASVisibleNetwork{PLMN: NASPLMN{
				MCC: mcc, MNC: mnc, MNCThreeDigits: value[offset+4] != 0, MNCThreeDigitsKnown: true,
			}})
		}
	}
	return nil
}

func applyNASNetworkNameSources(networks []NASVisibleNetwork, value []byte) error {
	if len(value) < 1 {
		return errors.New("parsing QMI NAS network scan: name-source count is truncated")
	}
	count := int(value[0])
	want := 1 + count*4
	if len(value) != want {
		return fmt.Errorf("parsing QMI NAS network scan: name-source TLV length %d, want %d", len(value), want)
	}
	if count != len(networks) {
		return fmt.Errorf("parsing QMI NAS network scan: name-source count %d does not match network count %d", count, len(networks))
	}
	for i := range count {
		offset := 1 + i*4
		networks[i].NameSource = NASNetworkNameSource(binary.LittleEndian.Uint32(value[offset : offset+4]))
		networks[i].NameSourceKnown = true
	}
	return nil
}

func findNASVisibleNetwork(networks []NASVisibleNetwork, mcc, mnc uint16) *NASVisibleNetwork {
	for i := range networks {
		if networks[i].PLMN.MCC == mcc && networks[i].PLMN.MNC == mnc {
			return &networks[i]
		}
	}
	return nil
}
