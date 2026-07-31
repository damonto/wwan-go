package qcom

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	wdsAPNMaxLength         = 150
	wdsUsernameMaxLength    = 127
	wdsPasswordMaxLength    = 127
	wdsProfileNameMaxLength = 50
)

// WDSLegacyBindMuxDataPortRequest encodes legacy QMI WDS Bind Data Port.
type WDSLegacyBindMuxDataPortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	DataPort      WDSSIOPort
}

// Request binds the WDS client to a legacy SIO data port.
func (r WDSLegacyBindMuxDataPortRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSLegacyBindMuxDataPort,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(0x01, uint16(r.DataPort)),
		},
	}
}

// WDSBindMuxDataPortRequest encodes QMI WDS Bind Mux Data Port.
type WDSBindMuxDataPortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	DataPort      WDSMuxDataPort
}

// Request binds the WDS client to a logical data channel.
func (r WDSBindMuxDataPortRequest) Request() Request {
	tlvs := make(tlv.TLVs, 0, 4)
	if r.DataPort.Endpoint != nil {
		endpoint, _ := r.DataPort.Endpoint.MarshalBinary() // Fixed-width endpoint encoding cannot fail.
		tlvs = append(tlvs, tlv.Bytes(0x10, endpoint))
	}
	tlvs = append(tlvs, tlv.Uint(0x11, r.DataPort.MuxID))
	if r.DataPort.Reversed {
		tlvs = append(tlvs, tlv.Uint(0x12, uint8(1)))
	}
	if r.DataPort.ClientType != WDSClientTypeReserved {
		tlvs = append(tlvs, tlv.Uint(0x13, uint32(r.DataPort.ClientType)))
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSBindMuxDataPort,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// WDSStartNetworkInterfaceRequest encodes QMI WDS Start Network Interface.
type WDSStartNetworkInterfaceRequest struct {
	ClientID                       uint8
	TransactionID                  uint16
	Timeout                        time.Duration
	PrimaryDNSAddressPreference    *uint32
	SecondaryDNSAddressPreference  *uint32
	PrimaryNBNSAddressPreference   *uint32
	SecondaryNBNSAddressPreference *uint32
	APN                            string
	IPv4AddressPreference          *uint32
	Authentication                 WDSAuthenticationMask
	Username                       string
	Password                       string
	IPPreference                   WDSIPPreference
	TechnologyPreference           WDSTechnologyPreference
	ProfileIndex3GPP               uint8
	ProfileIndex3GPP2              uint8
	EnableAutoconnect              *bool
	ExtendedTechnologyPreference   *WDSExtendedTechnologyPreference
	CallType                       *WDSCallType
}

// Request converts the high-level request fields into a QMI WDS request.
func (r WDSStartNetworkInterfaceRequest) Request() Request {
	var tlvs tlv.TLVs
	for _, field := range []struct {
		kind  uint8
		value *uint32
	}{
		{kind: 0x10, value: r.PrimaryDNSAddressPreference},
		{kind: 0x11, value: r.SecondaryDNSAddressPreference},
		{kind: 0x12, value: r.PrimaryNBNSAddressPreference},
		{kind: 0x13, value: r.SecondaryNBNSAddressPreference},
	} {
		if field.value != nil {
			tlvs = append(tlvs, tlv.Uint(field.kind, *field.value))
		}
	}
	if r.APN != "" {
		tlvs = append(tlvs, tlv.Bytes(0x14, []byte(r.APN)))
	}
	if r.IPv4AddressPreference != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, *r.IPv4AddressPreference))
	}
	if r.Authentication != 0 {
		tlvs = append(tlvs, tlv.Uint(0x16, uint8(r.Authentication)))
	}
	if r.Username != "" {
		tlvs = append(tlvs, tlv.Bytes(0x17, []byte(r.Username)))
	}
	if r.Password != "" {
		tlvs = append(tlvs, tlv.Bytes(0x18, []byte(r.Password)))
	}
	if r.IPPreference != WDSIPPreferenceDefault {
		tlvs = append(tlvs, tlv.Uint(0x19, uint8(r.IPPreference)))
	}
	if r.TechnologyPreference != 0 {
		tlvs = append(tlvs, tlv.Uint(0x30, uint8(r.TechnologyPreference)))
	}
	if r.ProfileIndex3GPP != 0 {
		tlvs = append(tlvs, tlv.Uint(0x31, r.ProfileIndex3GPP))
	}
	if r.ProfileIndex3GPP2 != 0 {
		tlvs = append(tlvs, tlv.Uint(0x32, r.ProfileIndex3GPP2))
	}
	if r.EnableAutoconnect != nil {
		tlvs = append(tlvs, tlv.Uint(0x33, boolByte(*r.EnableAutoconnect)))
	}
	if r.ExtendedTechnologyPreference != nil {
		tlvs = append(tlvs, tlv.Uint(0x34, uint16(*r.ExtendedTechnologyPreference)))
	}
	if r.CallType != nil {
		tlvs = append(tlvs, tlv.Uint(0x35, uint8(*r.CallType)))
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSStartNetworkInterface,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

func validateWDSAuthentication(authentication WDSAuthenticationMask) error {
	const supported = WDSAuthenticationPAP | WDSAuthenticationCHAP
	if authentication&^supported != 0 {
		return fmt.Errorf("unsupported WDS authentication mask 0x%02X", authentication)
	}
	return nil
}

func validateWDSString(value string, maxLength int) error {
	if strings.IndexByte(value, 0) >= 0 {
		return errors.New("value contains a NUL byte")
	}
	if len(value) > maxLength {
		return fmt.Errorf("value length %d exceeds maximum %d", len(value), maxLength)
	}
	return nil
}

// WDSStartNetworkInterfaceResponse is the parsed WDS start network response.
type WDSStartNetworkInterfaceResponse struct {
	PacketDataHandle        uint32
	CallEndReason           WDSCallEndReason
	HasCallEndReason        bool
	VerboseCallEndReason    WDSVerboseCallEndReason
	HasVerboseCallEndReason bool
}

// UnmarshalTLVs reads the packet data handle returned by the modem.
func (r *WDSStartNetworkInterfaceResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSStartNetworkInterfaceResponse{}

	if value, ok := tlv.Value(tlvs, 0x01); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS start network response: packet data handle TLV length %d, want 4", len(value))
		}
		r.PacketDataHandle = binary.LittleEndian.Uint32(value[:4])
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing WDS start network response: call end reason TLV length %d, want 2", len(value))
		}
		r.CallEndReason = WDSCallEndReason(binary.LittleEndian.Uint16(value[:2]))
		r.HasCallEndReason = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS start network response: verbose call end reason TLV length %d, want 4", len(value))
		}
		r.VerboseCallEndReason = WDSVerboseCallEndReason{
			Type:   WDSVerboseCallEndReasonType(binary.LittleEndian.Uint16(value[:2])),
			Reason: int16(binary.LittleEndian.Uint16(value[2:4])),
		}
		r.HasVerboseCallEndReason = true
	}
	return nil
}

// WDSStopNetworkInterfaceRequest encodes QMI WDS Stop Network Interface.
type WDSStopNetworkInterfaceRequest struct {
	ClientID         uint8
	TransactionID    uint16
	Timeout          time.Duration
	PacketDataHandle uint32
}

// Request converts the stop request into a QMI WDS request.
func (r WDSStopNetworkInterfaceRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSStopNetworkInterface,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(0x01, r.PacketDataHandle),
		},
	}
}

// WDSGetRuntimeSettingsRequest encodes QMI WDS Get Runtime Settings.
type WDSGetRuntimeSettingsRequest struct {
	ClientID          uint8
	TransactionID     uint16
	Timeout           time.Duration
	RequestedSettings WDSRuntimeSettingsMask
}

// Request converts the runtime-settings selector into a QMI WDS request.
func (r WDSGetRuntimeSettingsRequest) Request() Request {
	requested := r.RequestedSettings
	if requested == 0 {
		requested = WDSRuntimeRequestedIMSSettings
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetRuntimeSettings,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(0x10, uint32(requested)),
		},
	}
}

// WDSGetRuntimeSettingsResponse is the parsed WDS runtime settings response.
type WDSGetRuntimeSettingsResponse struct {
	Settings WDSRuntimeSettings
}

// UnmarshalTLVs parses IMS PDN addressing and P-CSCF data.
func (r *WDSGetRuntimeSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetRuntimeSettingsResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		r.Settings.ProfileName = string(value)
		r.Settings.ProfileNameKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing WDS runtime settings: PDP type TLV length %d, want 1", len(value))
		}
		r.Settings.PDPType = WDSPDPType(value[0])
		r.Settings.PDPTypeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		r.Settings.APN = string(value)
		r.Settings.APNKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1E); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS runtime settings: IPv4 address TLV length %d, want 4", len(value))
		}
		r.Settings.LocalIPv4 = qmiIPv4(value)
	}
	for _, kind := range []byte{0x15, 0x16} {
		if value, ok := tlv.Value(tlvs, kind); ok {
			if len(value) != 4 {
				return fmt.Errorf("parsing WDS runtime settings: IPv4 DNS TLV length %d, want 4", len(value))
			}
			r.Settings.DNS = append(r.Settings.DNS, qmiIPv4(value))
		}
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		qos, err := parseWDSUMTSGrantedQoS(value)
		if err != nil {
			return err
		}
		r.Settings.UMTSGrantedQoS = qos
		r.Settings.UMTSGrantedQoSKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x19); ok {
		qos, err := parseWDSGPRSGrantedQoS(value)
		if err != nil {
			return err
		}
		r.Settings.GPRSGrantedQoS = qos
		r.Settings.GPRSGrantedQoSKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1B); ok {
		r.Settings.Username = string(value)
		r.Settings.UsernameKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1D); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing WDS runtime settings: authentication TLV length %d, want 1", len(value))
		}
		r.Settings.Authentication = WDSAuthenticationMask(value[0])
		r.Settings.AuthenticationKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x1F); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing WDS runtime settings: profile ID TLV length %d, want 2", len(value))
		}
		r.Settings.ProfileID = WDSProfileID{Type: WDSProfileType(value[0]), Index: value[1]}
		r.Settings.ProfileIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x20); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS runtime settings: IPv4 gateway TLV length %d, want 4", len(value))
		}
		r.Settings.IPv4Gateway = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, 0x21); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS runtime settings: IPv4 subnet mask TLV length %d, want 4", len(value))
		}
		r.Settings.IPv4SubnetMask = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, 0x22); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing WDS runtime settings: P-CSCF using PCO TLV length %d, want 1", len(value))
		}
		r.Settings.PCSCFUsingPCO = value[0] != 0
		r.Settings.PCSCFUsingPCOKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x25); ok {
		if len(value) != 17 {
			return fmt.Errorf("parsing WDS runtime settings: IPv6 address TLV length %d, want 17", len(value))
		}
		r.Settings.LocalIPv6 = slices.Clone(value[:16])
		r.Settings.IPv6PrefixLength = value[16]
	}
	if value, ok := tlv.Value(tlvs, 0x26); ok {
		if len(value) != 17 {
			return fmt.Errorf("parsing WDS runtime settings: IPv6 gateway TLV length %d, want 17", len(value))
		}
		r.Settings.IPv6Gateway = slices.Clone(value[:16])
	}
	for _, kind := range []byte{0x27, 0x28} {
		if value, ok := tlv.Value(tlvs, kind); ok {
			if len(value) != 16 {
				return fmt.Errorf("parsing WDS runtime settings: IPv6 DNS TLV length %d, want 16", len(value))
			}
			r.Settings.DNS = append(r.Settings.DNS, slices.Clone(value[:16]))
		}
	}
	if value, ok := tlv.Value(tlvs, 0x29); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing WDS runtime settings: MTU TLV length %d, want 4", len(value))
		}
		r.Settings.MTU = binary.LittleEndian.Uint32(value[:4])
	}
	if value, ok := tlv.Value(tlvs, 0x23); ok {
		ips, err := parseWDSIPv4List(value)
		if err != nil {
			return err
		}
		r.Settings.PCSCFIPs = append(r.Settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x24); ok {
		domains, err := parseWDSDomainList(value)
		if err != nil {
			return fmt.Errorf("parsing WDS runtime P-CSCF domain list: %w", err)
		}
		r.Settings.PCSCFDomains = domains
	}
	if value, ok := tlv.Value(tlvs, 0x2E); ok {
		ips, err := parseWDSIPv6List(value)
		if err != nil {
			return err
		}
		r.Settings.PCSCFIPs = append(r.Settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x2A); ok {
		domains, err := parseWDSDomainList(value)
		if err != nil {
			return fmt.Errorf("parsing WDS runtime domain list: %w", err)
		}
		r.Settings.DomainNames = domains
	}
	if value, ok := tlv.Value(tlvs, 0x2B); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing WDS runtime settings: IP family TLV length %d, want 1", len(value))
		}
		family := WDSIPFamily(value[0])
		if family != WDSIPFamilyIPv4 && family != WDSIPFamilyIPv6 {
			return errors.New("parsing WDS runtime settings: IP family is invalid")
		}
		r.Settings.IPFamily = family
	}
	if value, ok := tlv.Value(tlvs, 0x2C); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing WDS runtime settings: IMCN TLV length %d, want 1", len(value))
		}
		r.Settings.IMCN = value[0] == 1
	}
	if value, ok := tlv.Value(tlvs, 0x2D); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing WDS runtime settings: extended technology TLV length %d, want 2", len(value))
		}
		r.Settings.ExtendedTechnology = WDSExtendedTechnologyPreference(binary.LittleEndian.Uint16(value))
		r.Settings.ExtendedTechnologyKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x2F); ok {
		pco, err := parseWDSOperatorReservedPCO(value)
		if err != nil {
			return err
		}
		r.Settings.OperatorReservedPCO = pco
		r.Settings.OperatorReservedPCOKnown = true
	}
	r.Settings.PCSCFIPs = uniqueWDSIPs(r.Settings.PCSCFIPs)
	return nil
}

func parseWDSUMTSGrantedQoS(value []byte) (WDSUMTSGrantedQoS, error) {
	var qos WDSUMTSGrantedQoS
	if err := qos.UnmarshalBinary(value); err != nil {
		return WDSUMTSGrantedQoS{}, fmt.Errorf("parsing WDS runtime UMTS granted QoS: %w", err)
	}
	return qos, nil
}

func parseWDSGPRSGrantedQoS(value []byte) (WDSGPRSGrantedQoS, error) {
	var qos WDSGPRSGrantedQoS
	if err := qos.UnmarshalBinary(value); err != nil {
		return WDSGPRSGrantedQoS{}, fmt.Errorf("parsing WDS runtime GPRS granted QoS: %w", err)
	}
	return qos, nil
}

func parseWDSDomainList(value []byte) ([]string, error) {
	if len(value) == 0 {
		return nil, errors.New("list TLV is truncated")
	}
	count := int(value[0])
	rest := value[1:]
	domains := make([]string, 0, count)
	for range count {
		if len(rest) < 2 {
			return nil, errors.New("entry length is truncated")
		}
		length := int(binary.LittleEndian.Uint16(rest[:2]))
		rest = rest[2:]
		if len(rest) < length {
			return nil, errors.New("entry value is truncated")
		}
		domains = append(domains, string(rest[:length]))
		rest = rest[length:]
	}
	if len(rest) != 0 {
		return nil, errors.New("list has trailing data")
	}
	return domains, nil
}

func parseWDSOperatorReservedPCO(value []byte) (WDSOperatorReservedPCO, error) {
	if len(value) < 8 {
		return WDSOperatorReservedPCO{}, errors.New("parsing WDS runtime settings: operator reserved PCO is truncated")
	}
	infoLength := int(value[5])
	want := 8 + infoLength
	if len(value) != want {
		return WDSOperatorReservedPCO{}, fmt.Errorf("parsing WDS runtime settings: operator reserved PCO TLV length %d, want %d", len(value), want)
	}
	return WDSOperatorReservedPCO{
		MCC:                 binary.LittleEndian.Uint16(value[:2]),
		MNC:                 binary.LittleEndian.Uint16(value[2:4]),
		MNCIncludesPCSDigit: value[4] != 0,
		AppSpecificInfo:     slices.Clone(value[6 : 6+infoLength]),
		ContainerID:         binary.LittleEndian.Uint16(value[6+infoLength:]),
	}, nil
}

func parseWDSIPv4List(value []byte) ([]net.IP, error) {
	if len(value) == 0 {
		return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv4 list TLV is truncated")
	}
	count := int(value[0])
	offset := 1
	ips := make([]net.IP, 0, count)
	for range count {
		if len(value) < offset+4 {
			return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv4 list value is truncated")
		}
		ips = append(ips, qmiIPv4(value[offset:offset+4]))
		offset += 4
	}
	if offset != len(value) {
		return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv4 list has trailing data")
	}
	return ips, nil
}

func qmiIPv4(value []byte) net.IP {
	return net.IPv4(value[3], value[2], value[1], value[0])
}

func parseWDSIPv6List(value []byte) ([]net.IP, error) {
	if len(value) == 0 {
		return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv6 list TLV is truncated")
	}
	count := int(value[0])
	offset := 1
	ips := make([]net.IP, 0, count)
	for range count {
		if len(value) < offset+16 {
			return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv6 list value is truncated")
		}
		ips = append(ips, slices.Clone(value[offset:offset+16]))
		offset += 16
	}
	if offset != len(value) {
		return nil, errors.New("parsing WDS runtime settings: P-CSCF IPv6 list has trailing data")
	}
	return ips, nil
}

func uniqueWDSIPs(ips []net.IP) []net.IP {
	unique := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if len(ip) == 0 || slices.ContainsFunc(unique, ip.Equal) {
			continue
		}
		unique = append(unique, slices.Clone(ip))
	}
	return unique
}
