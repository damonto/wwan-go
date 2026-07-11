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

// WDSLegacyBindMuxDataPortRequest encodes legacy QMI WDS Bind Data Port.
type WDSLegacyBindMuxDataPortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	DataPort      qcom.WDSSIOPort
}

// Request binds the WDS client to a legacy SIO data port.
func (r WDSLegacyBindMuxDataPortRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSLegacyBindMuxDataPort,
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
	DataPort      qcom.WDSMuxDataPort
}

// Request binds the WDS client to a logical data channel.
func (r WDSBindMuxDataPortRequest) Request() qcom.Request {
	tlvs := make(tlv.TLVs, 0, 3)
	if r.DataPort.Endpoint != nil {
		endpoint := binary.LittleEndian.AppendUint32(nil, uint32(r.DataPort.Endpoint.Type))
		endpoint = binary.LittleEndian.AppendUint32(endpoint, r.DataPort.Endpoint.InterfaceID)
		tlvs = append(tlvs, tlv.Bytes(0x10, endpoint))
	}
	tlvs = append(tlvs, tlv.Uint(0x11, r.DataPort.MuxID))
	if r.DataPort.Reversed {
		tlvs = append(tlvs, tlv.Uint(0x12, uint8(1)))
	}
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSBindMuxDataPort,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// WDSStartNetworkInterfaceRequest encodes QMI WDS Start Network Interface.
type WDSStartNetworkInterfaceRequest struct {
	ClientID             uint8
	TransactionID        uint16
	Timeout              time.Duration
	APN                  string
	IPFamily             qcom.WDSIPFamily
	TechnologyPreference qcom.WDSTechnologyPreference
	ProfileIndex3GPP     uint8
}

// Request converts the high-level request fields into a QMI WDS request.
func (r WDSStartNetworkInterfaceRequest) Request() qcom.Request {
	tlvs := tlv.TLVs{
		tlv.Bytes(0x14, []byte(r.APN)),
		tlv.Uint(0x19, uint8(r.IPFamily)),
		tlv.Uint(0x30, uint8(r.TechnologyPreference)),
	}
	if r.ProfileIndex3GPP != 0 {
		tlvs = append(tlvs, tlv.Uint(0x31, r.ProfileIndex3GPP))
	}
	return qcom.Request{
		Service:       qcom.ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageWDSStartNetworkInterface,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
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

// UnmarshalTLVs reads the packet data handle returned by the modem.
func (r *WDSStartNetworkInterfaceResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
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

// WDSGetRuntimeSettingsResponse is the parsed WDS runtime settings response.
type WDSGetRuntimeSettingsResponse struct {
	Settings qcom.WDSRuntimeSettings
}

// UnmarshalTLVs parses IMS PDN addressing and P-CSCF data.
func (r *WDSGetRuntimeSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetRuntimeSettingsResponse{}
	if value, ok := tlv.Value(tlvs, 0x1E); ok {
		if len(value) < 4 {
			return errors.New("parsing WDS runtime settings: IPv4 address TLV is truncated")
		}
		r.Settings.LocalIPv4 = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, 0x25); ok {
		if len(value) < 17 {
			return errors.New("parsing WDS runtime settings: IPv6 address TLV is truncated")
		}
		r.Settings.LocalIPv6 = slices.Clone(value[:16])
	}
	if value, ok := tlv.Value(tlvs, 0x23); ok {
		ips, err := parseWDSIPv4List(value)
		if err != nil {
			return err
		}
		r.Settings.PCSCFIPs = append(r.Settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x2E); ok {
		ips, err := parseWDSIPv6List(value)
		if err != nil {
			return err
		}
		r.Settings.PCSCFIPs = append(r.Settings.PCSCFIPs, ips...)
	}
	if value, ok := tlv.Value(tlvs, 0x2B); ok && len(value) > 0 {
		r.Settings.IPFamily = qcom.WDSIPFamily(value[0])
	}
	if value, ok := tlv.Value(tlvs, 0x2C); ok && len(value) > 0 {
		r.Settings.IMCN = value[0] == 1
	}
	r.Settings.PCSCFIPs = uniqueWDSIPs(r.Settings.PCSCFIPs)
	return nil
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
