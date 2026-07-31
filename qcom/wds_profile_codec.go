package qcom

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// MarshalTLVs encodes a complete WDS profile configuration.
func (cfg WDSProfileConfig) MarshalTLVs() (tlv.TLVs, error) {
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("wwan-go-%d", cfg.PDPType)
	}
	if err := validateWDSProfileConfig(cfg); err != nil {
		return nil, err
	}

	profileTLVs, err := wdsProfileUpdateFromConfig(cfg).MarshalTLVs()
	if err != nil {
		return nil, err
	}
	return append(tlv.TLVs{tlv.Uint(wdsTLVProfileID, uint8(cfg.Type))}, profileTLVs...), nil
}

func wdsProfileUpdateFromConfig(cfg WDSProfileConfig) WDSProfileUpdate {
	update := WDSProfileUpdate{
		Name:                          &cfg.Name,
		APN:                           &cfg.APN,
		PDPType:                       &cfg.PDPType,
		HeaderCompression:             cfg.HeaderCompression,
		DataCompression:               cfg.DataCompression,
		UMTSRequestedQoS:              cfg.UMTSRequestedQoS,
		UMTSMinimumQoS:                cfg.UMTSMinimumQoS,
		GPRSRequestedQoS:              cfg.GPRSRequestedQoS,
		GPRSMinimumQoS:                cfg.GPRSMinimumQoS,
		PCSCFUsingPCO:                 cfg.PCSCFUsingPCO,
		PDPAccessControl:              cfg.PDPAccessControl,
		PCSCFUsingDHCP:                cfg.PCSCFUsingDHCP,
		IMCN:                          cfg.IMCN,
		PDPContextNumber:              cfg.PDPContextNumber,
		PDPContextSecondary:           cfg.PDPContextSecondary,
		PDPContextPrimaryID:           cfg.PDPContextPrimaryID,
		UMTSRequestedQoSWithSignaling: cfg.UMTSRequestedQoSWithSignaling,
		UMTSMinimumQoSWithSignaling:   cfg.UMTSMinimumQoSWithSignaling,
		AddressAllocationPreference:   cfg.AddressAllocationPreference,
		LTEQoS:                        cfg.LTEQoS,
		APNDisabled:                   cfg.APNDisabled,
		RoamingDisallowed:             cfg.RoamingDisallowed,
		VLAN:                          cfg.VLAN,
		APNType:                       cfg.APNType,
	}
	if cfg.Username != "" {
		update.Username = &cfg.Username
	}
	if cfg.Password != "" {
		update.Password = &cfg.Password
	}
	if cfg.Authentication != 0 {
		update.Authentication = &cfg.Authentication
	}
	if cfg.PrimaryIPv4DNS.IsValid() {
		update.PrimaryIPv4DNS = &cfg.PrimaryIPv4DNS
	}
	if cfg.SecondaryIPv4DNS.IsValid() {
		update.SecondaryIPv4DNS = &cfg.SecondaryIPv4DNS
	}
	if cfg.IPv4AddressPreference.IsValid() {
		update.IPv4AddressPreference = &cfg.IPv4AddressPreference
	}
	if cfg.IPv6AddressPreference.IsValid() {
		update.IPv6AddressPreference = &cfg.IPv6AddressPreference
	}
	if cfg.PrimaryIPv6DNS.IsValid() {
		update.PrimaryIPv6DNS = &cfg.PrimaryIPv6DNS
	}
	if cfg.SecondaryIPv6DNS.IsValid() {
		update.SecondaryIPv6DNS = &cfg.SecondaryIPv6DNS
	}

	return update
}

// MarshalTLVs encodes the selected WDS profile fields.
func (update WDSProfileUpdate) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	if update.Name != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileName, []byte(*update.Name)))
	}
	if update.PDPType != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfilePDPType, uint8(*update.PDPType)))
	}
	if update.HeaderCompression != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfileHeaderCompression, uint8(*update.HeaderCompression)))
	}
	if update.DataCompression != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfileDataCompression, uint8(*update.DataCompression)))
	}
	if update.APN != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileAPN, []byte(*update.APN)))
	}
	if update.PrimaryIPv4DNS != nil {
		value, err := encodeWDSProfileIPv4(*update.PrimaryIPv4DNS)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile primary IPv4 DNS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfilePrimaryIPv4DNS, value))
	}
	if update.SecondaryIPv4DNS != nil {
		value, err := encodeWDSProfileIPv4(*update.SecondaryIPv4DNS)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile secondary IPv4 DNS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileSecondaryIPv4DNS, value))
	}
	if update.UMTSRequestedQoS != nil {
		value, err := update.UMTSRequestedQoS.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile UMTS requested QoS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileUMTSRequestedQoS, value))
	}
	if update.UMTSMinimumQoS != nil {
		value, err := update.UMTSMinimumQoS.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile UMTS minimum QoS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileUMTSMinimumQoS, value))
	}
	if update.GPRSRequestedQoS != nil {
		value, err := update.GPRSRequestedQoS.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile GPRS requested QoS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileGPRSRequestedQoS, value))
	}
	if update.GPRSMinimumQoS != nil {
		value, err := update.GPRSMinimumQoS.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile GPRS minimum QoS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileGPRSMinimumQoS, value))
	}
	if update.Username != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileUsername, []byte(*update.Username)))
	}
	if update.Password != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfilePassword, []byte(*update.Password)))
	}
	if update.Authentication != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfileAuth, uint8(*update.Authentication)))
	}
	if update.IPv4AddressPreference != nil {
		value, err := encodeWDSProfileIPv4(*update.IPv4AddressPreference)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile IPv4 address preference: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileIPv4Preference, value))
	}
	if update.PCSCFUsingPCO != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVPCSCFUsingPCO, []byte{boolByte(*update.PCSCFUsingPCO)}))
	}
	if update.PDPAccessControl != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfilePDPAccessControl, uint8(*update.PDPAccessControl)))
	}
	if update.PCSCFUsingDHCP != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVPCSCFUsingDHCP, []byte{boolByte(*update.PCSCFUsingDHCP)}))
	}
	if update.IMCN != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVIMCNFlag, []byte{boolByte(*update.IMCN)}))
	}
	if update.PDPContextNumber != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfilePDPContextNumber, *update.PDPContextNumber))
	}
	if update.PDPContextSecondary != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfilePDPContextSecondary, []byte{boolByte(*update.PDPContextSecondary)}))
	}
	if update.PDPContextPrimaryID != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfilePDPContextPrimaryID, *update.PDPContextPrimaryID))
	}
	if update.IPv6AddressPreference != nil {
		value, err := encodeWDSProfileIPv6(*update.IPv6AddressPreference)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile IPv6 address preference: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileIPv6Preference, value))
	}
	if update.UMTSRequestedQoSWithSignaling != nil {
		value, err := update.UMTSRequestedQoSWithSignaling.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile UMTS requested QoS with signaling: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileUMTSRequestedQoSSig, value))
	}
	if update.UMTSMinimumQoSWithSignaling != nil {
		value, err := update.UMTSMinimumQoSWithSignaling.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile UMTS minimum QoS with signaling: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileUMTSMinimumQoSSig, value))
	}
	if update.PrimaryIPv6DNS != nil {
		value, err := encodeWDSProfileIPv6(*update.PrimaryIPv6DNS)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile primary IPv6 DNS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfilePrimaryIPv6DNS, value))
	}
	if update.SecondaryIPv6DNS != nil {
		value, err := encodeWDSProfileIPv6(*update.SecondaryIPv6DNS)
		if err != nil {
			return nil, fmt.Errorf("encoding WDS profile secondary IPv6 DNS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileSecondaryIPv6DNS, value))
	}
	if update.AddressAllocationPreference != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVProfileAddressAllocation, uint8(*update.AddressAllocationPreference)))
	}
	if update.LTEQoS != nil {
		value, err := update.LTEQoS.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile LTE QoS: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileLTEQoS, value))
	}
	if update.APNDisabled != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileAPNDisabled, []byte{boolByte(*update.APNDisabled)}))
	}
	if update.RoamingDisallowed != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileRoamingDisallowed, []byte{boolByte(*update.RoamingDisallowed)}))
	}
	if update.VLAN != nil {
		value, err := update.VLAN.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding profile VLAN range: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileVLAN, value))
	}
	if update.APNType != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileAPNType, binary.LittleEndian.AppendUint64(nil, uint64(*update.APNType))))
	}
	if update.CLATEnabled != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileCLATEnabled, []byte{boolByte(*update.CLATEnabled)}))
	}
	if update.IPv6PrefixDelegation != nil {
		tlvs = append(tlvs, tlv.Bytes(wdsTLVProfileIPv6PrefixDelegation, []byte{boolByte(*update.IPv6PrefixDelegation)}))
	}
	return tlvs, nil
}

func encodeWDSProfileIPv4(address netip.Addr) ([]byte, error) {
	address = address.Unmap()
	if !address.Is4() {
		return nil, fmt.Errorf("address %q is not IPv4", address)
	}
	bytes := address.As4()
	return []byte{bytes[3], bytes[2], bytes[1], bytes[0]}, nil
}

func encodeWDSProfileIPv6(address netip.Addr) ([]byte, error) {
	address = address.Unmap()
	if !address.Is6() {
		return nil, fmt.Errorf("address %q is not IPv6", address)
	}
	bytes := address.As16()
	return append([]byte(nil), bytes[:]...), nil
}

// MarshalBinary encodes UMTS QoS parameters.
func (qos WDSUMTSGrantedQoS) MarshalBinary() ([]byte, error) {
	value := []byte{qos.TrafficClass}
	value = binary.LittleEndian.AppendUint32(value, qos.MaximumUplinkBitrate)
	value = binary.LittleEndian.AppendUint32(value, qos.MaximumDownlinkBitrate)
	value = binary.LittleEndian.AppendUint32(value, qos.GuaranteedUplinkBitrate)
	value = binary.LittleEndian.AppendUint32(value, qos.GuaranteedDownlinkBitrate)
	value = append(value, qos.DeliveryOrder)
	value = binary.LittleEndian.AppendUint32(value, qos.MaximumSDUSize)
	value = append(value, qos.SDUErrorRatio, qos.ResidualBitErrorRatio, qos.ErroneousSDUDelivery)
	value = binary.LittleEndian.AppendUint32(value, qos.TransferDelay)
	return binary.LittleEndian.AppendUint32(value, qos.TrafficHandlingPriority), nil
}

// MarshalBinary encodes GPRS QoS parameters.
func (qos WDSGPRSGrantedQoS) MarshalBinary() ([]byte, error) {
	value := binary.LittleEndian.AppendUint32(nil, qos.PrecedenceClass)
	value = binary.LittleEndian.AppendUint32(value, qos.DelayClass)
	value = binary.LittleEndian.AppendUint32(value, qos.ReliabilityClass)
	value = binary.LittleEndian.AppendUint32(value, qos.PeakThroughputClass)
	return binary.LittleEndian.AppendUint32(value, qos.MeanThroughputClass), nil
}

// MarshalBinary encodes UMTS QoS parameters with signaling indication.
func (qos WDSUMTSQoSWithSignaling) MarshalBinary() ([]byte, error) {
	value, err := qos.QoS.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(value, byte(qos.SignalingIndication)), nil
}

// MarshalBinary encodes LTE QoS parameters.
func (qos WDSLTEQoS) MarshalBinary() ([]byte, error) {
	value := []byte{byte(qos.ClassIdentifier)}
	value = binary.LittleEndian.AppendUint32(value, qos.GuaranteedDownlinkBitrate)
	value = binary.LittleEndian.AppendUint32(value, qos.MaximumDownlinkBitrate)
	value = binary.LittleEndian.AppendUint32(value, qos.GuaranteedUplinkBitrate)
	return binary.LittleEndian.AppendUint32(value, qos.MaximumUplinkBitrate), nil
}

// MarshalBinary encodes an inclusive VLAN range.
func (vlan WDSVLANRange) MarshalBinary() ([]byte, error) {
	value := binary.LittleEndian.AppendUint16(nil, vlan.Start)
	return binary.LittleEndian.AppendUint16(value, vlan.End), nil
}

func unmarshalWDSProfileSettings(tlvs tlv.TLVs, settings *WDSProfileSettings) error {
	if value, ok := tlv.Value(tlvs, wdsTLVProfileName); ok {
		settings.Name, settings.NameKnown = string(value), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePDPType); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfilePDPType)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP type: %w", err)
		}
		settings.PDPType, settings.PDPKnown = WDSPDPType(decoded), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileHeaderCompression); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfileHeaderCompression)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP header compression: %w", err)
		}
		settings.HeaderCompression, settings.HeaderCompressionKnown = WDSPDPHeaderCompression(decoded), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileDataCompression); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfileDataCompression)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP data compression: %w", err)
		}
		settings.DataCompression, settings.DataCompressionKnown = WDSPDPDataCompression(decoded), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileAPN); ok {
		settings.APN, settings.APNKnown = string(value), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePrimaryIPv4DNS); ok {
		address, err := decodeWDSProfileIPv4(value, wdsTLVProfilePrimaryIPv4DNS)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile primary IPv4 DNS: %w", err)
		}
		settings.PrimaryIPv4DNS, settings.PrimaryIPv4DNSKnown = address, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileSecondaryIPv4DNS); ok {
		address, err := decodeWDSProfileIPv4(value, wdsTLVProfileSecondaryIPv4DNS)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile secondary IPv4 DNS: %w", err)
		}
		settings.SecondaryIPv4DNS, settings.SecondaryIPv4DNSKnown = address, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileUMTSRequestedQoS); ok {
		var qos WDSUMTSGrantedQoS
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile UMTS requested QoS: %w", err)
		}
		settings.UMTSRequestedQoS, settings.UMTSRequestedQoSKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileUMTSMinimumQoS); ok {
		var qos WDSUMTSGrantedQoS
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile UMTS minimum QoS: %w", err)
		}
		settings.UMTSMinimumQoS, settings.UMTSMinimumQoSKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileGPRSRequestedQoS); ok {
		var qos WDSGPRSGrantedQoS
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile GPRS requested QoS: %w", err)
		}
		settings.GPRSRequestedQoS, settings.GPRSRequestedQoSKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileGPRSMinimumQoS); ok {
		var qos WDSGPRSGrantedQoS
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile GPRS minimum QoS: %w", err)
		}
		settings.GPRSMinimumQoS, settings.GPRSMinimumQoSKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileUsername); ok {
		settings.Username, settings.UsernameKnown = string(value), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePassword); ok {
		settings.Password, settings.PasswordKnown = string(value), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileAuth); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfileAuth)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile authentication: %w", err)
		}
		settings.Authentication, settings.AuthenticationKnown = WDSAuthenticationMask(decoded), true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileIPv4Preference); ok {
		address, err := decodeWDSProfileIPv4(value, wdsTLVProfileIPv4Preference)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile IPv4 address preference: %w", err)
		}
		settings.IPv4AddressPreference, settings.IPv4AddressPreferenceKnown = address, true
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVPCSCFUsingPCO, &settings.PCSCFUsingPCO, &settings.PCSCFUsingPCOKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile P-CSCF using PCO: %w", err)
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePDPAccessControl); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfilePDPAccessControl)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP access control: %w", err)
		}
		settings.PDPAccessControl, settings.PDPAccessControlKnown = WDSPDPAccessControl(decoded), true
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVPCSCFUsingDHCP, &settings.PCSCFUsingDHCP, &settings.PCSCFUsingDHCPKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile P-CSCF using DHCP: %w", err)
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVIMCNFlag, &settings.IMCN, &settings.IMCNKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile IMCN: %w", err)
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePDPContextNumber); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfilePDPContextNumber)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP context number: %w", err)
		}
		settings.PDPContextNumber, settings.PDPContextNumberKnown = decoded, true
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVProfilePDPContextSecondary, &settings.PDPContextSecondary, &settings.PDPContextSecondaryKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile PDP context secondary: %w", err)
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePDPContextPrimaryID); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfilePDPContextPrimaryID)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile PDP context primary ID: %w", err)
		}
		settings.PDPContextPrimaryID, settings.PDPContextPrimaryIDKnown = decoded, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileIPv6Preference); ok {
		address, err := decodeWDSProfileIPv6(value, wdsTLVProfileIPv6Preference)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile IPv6 address preference: %w", err)
		}
		settings.IPv6AddressPreference, settings.IPv6AddressPreferenceKnown = address, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileUMTSRequestedQoSSig); ok {
		var qos WDSUMTSQoSWithSignaling
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile UMTS requested QoS with signaling: %w", err)
		}
		settings.UMTSRequestedQoSWithSignaling, settings.UMTSRequestedQoSWithSignalingKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileUMTSMinimumQoSSig); ok {
		var qos WDSUMTSQoSWithSignaling
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile UMTS minimum QoS with signaling: %w", err)
		}
		settings.UMTSMinimumQoSWithSignaling, settings.UMTSMinimumQoSWithSignalingKnown = qos, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfilePrimaryIPv6DNS); ok {
		address, err := decodeWDSProfileIPv6(value, wdsTLVProfilePrimaryIPv6DNS)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile primary IPv6 DNS: %w", err)
		}
		settings.PrimaryIPv6DNS, settings.PrimaryIPv6DNSKnown = address, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileSecondaryIPv6DNS); ok {
		address, err := decodeWDSProfileIPv6(value, wdsTLVProfileSecondaryIPv6DNS)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile secondary IPv6 DNS: %w", err)
		}
		settings.SecondaryIPv6DNS, settings.SecondaryIPv6DNSKnown = address, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileAddressAllocation); ok {
		decoded, err := decodeWDSProfileByte(value, wdsTLVProfileAddressAllocation)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS profile address allocation preference: %w", err)
		}
		settings.AddressAllocationPreference = WDSAddressAllocationPreference(decoded)
		settings.AddressAllocationPreferenceKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileLTEQoS); ok {
		var qos WDSLTEQoS
		if err := qos.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile LTE QoS: %w", err)
		}
		settings.LTEQoS, settings.LTEQoSKnown = qos, true
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVProfileAPNDisabled, &settings.APNDisabled, &settings.APNDisabledKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile APN disabled: %w", err)
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVProfileRoamingDisallowed, &settings.RoamingDisallowed, &settings.RoamingDisallowedKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile roaming disallowed: %w", err)
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileVLAN); ok {
		var vlan WDSVLANRange
		if err := vlan.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WDS profile VLAN range: %w", err)
		}
		settings.VLAN, settings.VLANKnown = vlan, true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVProfileAPNType); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI WDS profile settings: APN type TLV length %d, want 8", len(value))
		}
		settings.APNType = WDSAPNTypeMask(binary.LittleEndian.Uint64(value))
		settings.APNTypeKnown = true
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVProfileCLATEnabled, &settings.CLATEnabled, &settings.CLATEnabledKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile CLAT enabled: %w", err)
	}
	if err := decodeWDSProfileBool(tlvs, wdsTLVProfileIPv6PrefixDelegation, &settings.IPv6PrefixDelegation, &settings.IPv6PrefixDelegationKnown); err != nil {
		return fmt.Errorf("parsing QMI WDS profile IPv6 prefix delegation: %w", err)
	}
	return nil
}

func decodeWDSProfileByte(value []byte, kind byte) (uint8, error) {
	if len(value) != 1 {
		return 0, fmt.Errorf("parsing QMI WDS profile settings: TLV 0x%02x length %d, want 1", kind, len(value))
	}
	return value[0], nil
}

func decodeWDSProfileBool(tlvs tlv.TLVs, kind byte, target, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	decoded, err := decodeWDSProfileByte(value, kind)
	if err != nil {
		return err
	}
	*target, *known = decoded != 0, true
	return nil
}

func decodeWDSProfileIPv4(value []byte, kind byte) (netip.Addr, error) {
	if len(value) != 4 {
		return netip.Addr{}, fmt.Errorf("parsing QMI WDS profile settings: TLV 0x%02x length %d, want 4", kind, len(value))
	}
	return netip.AddrFrom4([4]byte{value[3], value[2], value[1], value[0]}), nil
}

func decodeWDSProfileIPv6(value []byte, kind byte) (netip.Addr, error) {
	if len(value) != 16 {
		return netip.Addr{}, fmt.Errorf("parsing QMI WDS profile settings: TLV 0x%02x length %d, want 16", kind, len(value))
	}
	return netip.AddrFrom16([16]byte(value)), nil
}

// UnmarshalBinary decodes UMTS QoS parameters.
func (qos *WDSUMTSGrantedQoS) UnmarshalBinary(value []byte) error {
	if len(value) != 33 {
		return fmt.Errorf("UMTS QoS length %d, want 33", len(value))
	}
	*qos = WDSUMTSGrantedQoS{
		TrafficClass:              value[0],
		MaximumUplinkBitrate:      binary.LittleEndian.Uint32(value[1:5]),
		MaximumDownlinkBitrate:    binary.LittleEndian.Uint32(value[5:9]),
		GuaranteedUplinkBitrate:   binary.LittleEndian.Uint32(value[9:13]),
		GuaranteedDownlinkBitrate: binary.LittleEndian.Uint32(value[13:17]),
		DeliveryOrder:             value[17],
		MaximumSDUSize:            binary.LittleEndian.Uint32(value[18:22]),
		SDUErrorRatio:             value[22],
		ResidualBitErrorRatio:     value[23],
		ErroneousSDUDelivery:      value[24],
		TransferDelay:             binary.LittleEndian.Uint32(value[25:29]),
		TrafficHandlingPriority:   binary.LittleEndian.Uint32(value[29:33]),
	}
	return nil
}

// UnmarshalBinary decodes GPRS QoS parameters.
func (qos *WDSGPRSGrantedQoS) UnmarshalBinary(value []byte) error {
	if len(value) != 20 {
		return fmt.Errorf("GPRS QoS length %d, want 20", len(value))
	}
	*qos = WDSGPRSGrantedQoS{
		PrecedenceClass:     binary.LittleEndian.Uint32(value[0:4]),
		DelayClass:          binary.LittleEndian.Uint32(value[4:8]),
		ReliabilityClass:    binary.LittleEndian.Uint32(value[8:12]),
		PeakThroughputClass: binary.LittleEndian.Uint32(value[12:16]),
		MeanThroughputClass: binary.LittleEndian.Uint32(value[16:20]),
	}
	return nil
}

// UnmarshalBinary decodes UMTS QoS parameters with signaling indication.
func (qos *WDSUMTSQoSWithSignaling) UnmarshalBinary(value []byte) error {
	if len(value) != 34 {
		return fmt.Errorf("UMTS QoS with signaling length %d, want 34", len(value))
	}
	var decoded WDSUMTSGrantedQoS
	if err := decoded.UnmarshalBinary(value[:33]); err != nil {
		return err
	}
	*qos = WDSUMTSQoSWithSignaling{QoS: decoded, SignalingIndication: int8(value[33])}
	return nil
}

// UnmarshalBinary decodes LTE QoS parameters.
func (qos *WDSLTEQoS) UnmarshalBinary(value []byte) error {
	if len(value) != 17 {
		return fmt.Errorf("LTE QoS length %d, want 17", len(value))
	}
	*qos = WDSLTEQoS{
		ClassIdentifier:           WDSQoSClassIdentifier(value[0]),
		GuaranteedDownlinkBitrate: binary.LittleEndian.Uint32(value[1:5]),
		MaximumDownlinkBitrate:    binary.LittleEndian.Uint32(value[5:9]),
		GuaranteedUplinkBitrate:   binary.LittleEndian.Uint32(value[9:13]),
		MaximumUplinkBitrate:      binary.LittleEndian.Uint32(value[13:17]),
	}
	return nil
}

// UnmarshalBinary decodes an inclusive VLAN range.
func (vlan *WDSVLANRange) UnmarshalBinary(value []byte) error {
	if len(value) != 4 {
		return fmt.Errorf("VLAN range length %d, want 4", len(value))
	}
	*vlan = WDSVLANRange{
		Start: binary.LittleEndian.Uint16(value[0:2]),
		End:   binary.LittleEndian.Uint16(value[2:4]),
	}
	return nil
}
