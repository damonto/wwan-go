package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"
)

const mbimConnectSetResponseTimeout = 198 * time.Second

type ConnectRequest struct {
	TransactionID     uint32
	MBIMExVersion     uint16
	Timeout           time.Duration
	SessionID         uint32
	ActivationCommand ActivationCommand
	AccessString      string
	UserName          string
	Password          string
	Compression       Compression
	AuthProtocol      AuthProtocol
	IPType            ContextIPType
	ContextType       ContextType
	MediaPreference   AccessMediaType
	ActivationOption  ActivationOption
	Response          *ConnectInfo
}

func (r *ConnectRequest) Request() *Request {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = mbimConnectSetResponseTimeout
	}

	r.Response = new(ConnectInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       timeout,
		Command: command(
			ServiceBasicConnect,
			CIDConnect,
			CommandTypeSet,
			r.connectSetData(),
		),
		Response: r.Response,
	}
}

type ConnectQueryRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SessionID     uint32
	Response      *ConnectInfo
}

func (r *ConnectQueryRequest) Request() *Request {
	data := make([]byte, 36)
	if r.MBIMExVersion >= mbimExVersion30 {
		data = make([]byte, 4)
	}
	binary.LittleEndian.PutUint32(data[:4], r.SessionID)

	r.Response = new(ConnectInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDConnect,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ConnectInfo struct {
	SessionID        uint32
	ActivationState  ActivationState
	VoiceCallState   VoiceCallState
	IPType           ContextIPType
	ContextType      ContextType
	NwError          uint32
	AccessMedia      AccessMediaType
	AccessString     string
	TLVs             TLVs
	PCO              []ProtocolConfigurationOptions
	PCSCFIPs         []net.IP
	DNSIPs           []net.IP
	IPv4LinkMTU      uint16
	IPv4LinkMTUKnown bool
}

func (r *ConnectInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 36 {
		return errors.New("parsing MBIM connect info: payload is truncated")
	}

	*r = ConnectInfo{
		SessionID:       binary.LittleEndian.Uint32(data[:4]),
		ActivationState: ActivationState(binary.LittleEndian.Uint32(data[4:8])),
		VoiceCallState:  VoiceCallState(binary.LittleEndian.Uint32(data[8:12])),
		IPType:          ContextIPType(binary.LittleEndian.Uint32(data[12:16])),
		NwError:         binary.LittleEndian.Uint32(data[32:36]),
	}
	copy(r.ContextType[:], data[16:32])
	if len(data) == 36 {
		return nil
	}
	if len(data) < 40 {
		return errors.New("parsing MBIM connect info: EX payload is truncated")
	}

	r.AccessMedia = AccessMediaType(binary.LittleEndian.Uint32(data[36:40]))
	var tlvs TLVs
	if err := tlvs.UnmarshalBinary(data[40:]); err != nil {
		return fmt.Errorf("parsing MBIM connect info TLVs: %w", err)
	}
	r.TLVs = tlvs
	for _, tlv := range tlvs {
		switch tlv.Type {
		case TLVTypeWCharString:
			if r.AccessString == "" {
				accessString, err := utf16RawString(tlv.Data)
				if err != nil {
					return fmt.Errorf("parsing MBIM connect access string: %w", err)
				}
				r.AccessString = accessString
			}
		case TLVTypePCO:
			var pco ProtocolConfigurationOptions
			if err := pco.UnmarshalBinary(tlv.Data); err != nil {
				return fmt.Errorf("parsing MBIM connect PCO: %w", err)
			}
			r.PCO = append(r.PCO, pco)
			r.PCSCFIPs = uniqueIPs(append(r.PCSCFIPs, pco.PCSCFIPs...))
			r.DNSIPs = uniqueIPs(append(r.DNSIPs, pco.DNSIPs...))
			if pco.IPv4LinkMTUKnown && !r.IPv4LinkMTUKnown {
				r.IPv4LinkMTU = pco.IPv4LinkMTU
				r.IPv4LinkMTUKnown = true
			}
		}
	}
	return nil
}

type IPConfigurationRequest struct {
	TransactionID uint32
	SessionID     uint32
	Response      *IPConfigurationInfo
}

func (r *IPConfigurationRequest) Request() *Request {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], r.SessionID)

	r.Response = new(IPConfigurationInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDIPConfiguration,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

func (r *IPConfigurationInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 60 {
		return errors.New("parsing MBIM IP configuration: payload is truncated")
	}

	ipv4Addresses, err := parseIPv4Elements(data, binary.LittleEndian.Uint32(data[16:20]), binary.LittleEndian.Uint32(data[12:16]))
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv4 addresses: %w", err)
	}
	ipv6Addresses, err := parseIPv6Elements(data, binary.LittleEndian.Uint32(data[24:28]), binary.LittleEndian.Uint32(data[20:24]))
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv6 addresses: %w", err)
	}

	*r = IPConfigurationInfo{
		SessionID:                  binary.LittleEndian.Uint32(data[:4]),
		IPv4ConfigurationAvailable: IPConfigurationAvailable(binary.LittleEndian.Uint32(data[4:8])),
		IPv6ConfigurationAvailable: IPConfigurationAvailable(binary.LittleEndian.Uint32(data[8:12])),
		IPv4Addresses:              ipv4Addresses,
		IPv6Addresses:              ipv6Addresses,
		IPv4MTU:                    binary.LittleEndian.Uint32(data[52:56]),
		IPv6MTU:                    binary.LittleEndian.Uint32(data[56:60]),
	}
	return nil
}

func (r *Reader) IPConfiguration(ctx context.Context, sessionID uint32) (IPConfigurationInfo, error) {
	request := IPConfigurationRequest{
		TransactionID: r.nextTransactionID(),
		SessionID:     sessionID,
	}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return IPConfigurationInfo{}, fmt.Errorf("reading MBIM IP configuration: %w", err)
	}
	resp := *request.Response
	resp.IPv4Addresses = cloneIPAddresses(resp.IPv4Addresses)
	resp.IPv6Addresses = cloneIPAddresses(resp.IPv6Addresses)
	return resp, nil
}

func writeConnectStringRefs(data []byte, baseOffset int, values ...[]byte) {
	offset := baseOffset
	for i, value := range values {
		fieldOffset := 8 + i*8
		if len(value) != 0 {
			binary.LittleEndian.PutUint32(data[fieldOffset:fieldOffset+4], uint32(offset))
			binary.LittleEndian.PutUint32(data[fieldOffset+4:fieldOffset+8], uint32(len(value)))
		}
		offset = align4(offset + len(value))
	}
}

func (r *ConnectRequest) connectSetData() []byte {
	switch {
	case r.MBIMExVersion >= mbimExVersion40:
		return r.connectSetDataEx4()
	case r.MBIMExVersion >= mbimExVersion30:
		return r.connectSetDataEx3()
	default:
		return r.connectSetDataV1()
	}
}

func (r *ConnectRequest) connectSetDataV1() []byte {
	accessString := utf16Bytes(r.AccessString)
	userName := utf16Bytes(r.UserName)
	password := utf16Bytes(r.Password)

	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[0:4], r.SessionID)
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.ActivationCommand))
	writeConnectStringRefs(data, 60, accessString, userName, password)
	binary.LittleEndian.PutUint32(data[32:36], uint32(r.Compression))
	binary.LittleEndian.PutUint32(data[36:40], uint32(r.AuthProtocol))
	binary.LittleEndian.PutUint32(data[40:44], uint32(r.IPType))
	copy(data[44:60], r.ContextType[:])
	return appendConnectStrings(data, accessString, userName, password)
}

func (r *ConnectRequest) connectSetDataEx3() []byte {
	data := r.connectSetDataEx(40)
	return appendConnectStringTLVs(data, r.AccessString, r.UserName, r.Password)
}

func (r *ConnectRequest) connectSetDataEx4() []byte {
	data := r.connectSetDataEx(44)
	binary.LittleEndian.PutUint32(data[40:44], uint32(r.activationOption()))
	return appendConnectStringTLVs(data, r.AccessString, r.UserName, r.Password)
}

func (r *ConnectRequest) activationOption() ActivationOption {
	if r.ActivationCommand == ActivationCommandDeactivate {
		return ActivationOptionDefault
	}
	return r.ActivationOption
}

func (r *ConnectRequest) connectSetDataEx(size int) []byte {
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[0:4], r.SessionID)
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.ActivationCommand))
	binary.LittleEndian.PutUint32(data[8:12], uint32(r.Compression))
	binary.LittleEndian.PutUint32(data[12:16], uint32(r.AuthProtocol))
	binary.LittleEndian.PutUint32(data[16:20], uint32(r.IPType))
	copy(data[20:36], r.ContextType[:])
	binary.LittleEndian.PutUint32(data[36:40], uint32(r.MediaPreference))
	return data
}

func appendConnectStringTLVs(data []byte, values ...string) []byte {
	for _, value := range values {
		data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(value))...)
	}
	return data
}

func appendConnectStrings(data []byte, values ...[]byte) []byte {
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		data = append(data, value...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return data
}

func parseIPv4Elements(data []byte, offset, count uint32) ([]IPAddress, error) {
	if count == 0 {
		return nil, nil
	}
	if offset > uint32(len(data)) || count > (uint32(len(data))-offset)/8 {
		return nil, errors.New("address table is truncated")
	}
	addresses := make([]IPAddress, 0, count)
	for i := range count {
		entry := data[offset+i*8 : offset+i*8+8]
		addresses = append(addresses, IPAddress{
			PrefixLength: binary.LittleEndian.Uint32(entry[:4]),
			IP:           net.IPv4(entry[4], entry[5], entry[6], entry[7]),
		})
	}
	return addresses, nil
}

func parseIPv6Elements(data []byte, offset, count uint32) ([]IPAddress, error) {
	if count == 0 {
		return nil, nil
	}
	if offset > uint32(len(data)) || count > (uint32(len(data))-offset)/20 {
		return nil, errors.New("address table is truncated")
	}
	addresses := make([]IPAddress, 0, count)
	for i := range count {
		entry := data[offset+i*20 : offset+i*20+20]
		addresses = append(addresses, IPAddress{
			PrefixLength: binary.LittleEndian.Uint32(entry[:4]),
			IP:           slices.Clone(net.IP(entry[4:20])),
		})
	}
	return addresses, nil
}

func cloneIPAddresses(addresses []IPAddress) []IPAddress {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]IPAddress, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, IPAddress{
			IP:           slices.Clone(address.IP),
			PrefixLength: address.PrefixLength,
		})
	}
	return out
}
