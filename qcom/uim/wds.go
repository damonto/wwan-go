package uim

import (
	"encoding/binary"
	"errors"
	"net"
	"slices"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

// WDSStartNetworkInterfaceRequest encodes QMI WDS Start Network Interface.
type WDSStartNetworkInterfaceRequest struct {
	ClientID             uint8
	TransactionID        uint16
	Timeout              time.Duration
	APN                  string
	IPFamily             qcom.WDSIPFamily
	TechnologyPreference qcom.WDSTechnologyPreference
}

// Request converts the high-level request fields into a QMI WDS request.
func (r WDSStartNetworkInterfaceRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSStartNetworkInterface,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x14, []byte(r.APN)),
			tlv.Uint(0x19, uint8(r.IPFamily)),
			tlv.Uint(0x30, uint8(r.TechnologyPreference)),
		},
	}
}

// WDSStartNetworkInterfaceResponse is the parsed WDS start network response.
type WDSStartNetworkInterfaceResponse struct {
	PacketDataHandle        uint32
	CallEndReason           qcom.WDSCallEndReason
	HasCallEndReason        bool
	VerboseCallEndReason    qcom.WDSVerboseCallEndReason
	HasVerboseCallEndReason bool
}

// UnmarshalResponse reads the packet data handle returned by the modem.
func (r *WDSStartNetworkInterfaceResponse) UnmarshalResponse(tlvs tlv.TLVs) error {
	*r = WDSStartNetworkInterfaceResponse{}

	if value, ok := tlv.Value(tlvs, 0x01); ok {
		if len(value) < 4 {
			return errors.New("parsing WDS start network response: packet data handle TLV is truncated")
		}
		r.PacketDataHandle = binary.LittleEndian.Uint32(value[:4])
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) < 2 {
			return errors.New("parsing WDS start network response: call end reason TLV is truncated")
		}
		r.CallEndReason = qcom.WDSCallEndReason(binary.LittleEndian.Uint16(value[:2]))
		r.HasCallEndReason = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) < 4 {
			return errors.New("parsing WDS start network response: verbose call end reason TLV is truncated")
		}
		r.VerboseCallEndReason = qcom.WDSVerboseCallEndReason{
			Type:   qcom.WDSVerboseCallEndReasonType(binary.LittleEndian.Uint16(value[:2])),
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
func (r WDSStopNetworkInterfaceRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSStopNetworkInterface,
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
	RequestedSettings qcom.WDSRuntimeSettingsMask
}

// Request converts the runtime-settings selector into a QMI WDS request.
func (r WDSGetRuntimeSettingsRequest) Request() qcom.Request {
	requested := r.RequestedSettings
	if requested == 0 {
		requested = qcom.WDSRuntimeRequestedIMSSettings
	}
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSGetRuntimeSettings,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(0x10, uint32(requested)),
		},
	}
}

// UnmarshalWDSRuntimeSettings parses IMS PDN addressing and P-CSCF data.
func UnmarshalWDSRuntimeSettings(tlvs tlv.TLVs) (qcom.WDSRuntimeSettings, error) {
	var settings qcom.WDSRuntimeSettings
	if value, ok := tlv.Value(tlvs, 0x1E); ok {
		if len(value) < 4 {
			return qcom.WDSRuntimeSettings{}, errors.New("parsing WDS runtime settings: IPv4 address TLV is truncated")
		}
		settings.LocalIPv4 = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, 0x25); ok {
		if len(value) < 17 {
			return qcom.WDSRuntimeSettings{}, errors.New("parsing WDS runtime settings: IPv6 address TLV is truncated")
		}
		settings.LocalIPv6 = slices.Clone(value[:16])
	}
	if value, ok := tlv.Value(tlvs, 0x23); ok {
		ips, err := parseWDSIPv4List(value)
		if err != nil {
			return qcom.WDSRuntimeSettings{}, err
		}
		settings.PCSCFIPs = append(settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x2E); ok {
		ips, err := parseWDSIPv6List(value)
		if err != nil {
			return qcom.WDSRuntimeSettings{}, err
		}
		settings.PCSCFIPs = append(settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x2B); ok && len(value) > 0 {
		settings.IPFamily = qcom.WDSIPFamily(value[0])
	}
	if value, ok := tlv.Value(tlvs, 0x2C); ok && len(value) > 0 {
		settings.IMCN = value[0] == 1
	}
	settings.PCSCFIPs = uniqueWDSIPs(settings.PCSCFIPs)
	return settings, nil
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
