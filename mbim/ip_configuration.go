package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
)

const (
	ipConfigurationInfoFixedSize = 60
	ipConfigurationAvailableMask = IPConfigurationAvailableAddress | IPConfigurationAvailableGateway |
		IPConfigurationAvailableDNSServer | IPConfigurationAvailableMTU
	ipConfigurationBasicMask = IPConfigurationAvailableAddress | IPConfigurationAvailableGateway |
		IPConfigurationAvailableMTU
)

type IPConfigurationAvailable uint32

const (
	IPConfigurationAvailableAddress IPConfigurationAvailable = 1 << iota
	IPConfigurationAvailableGateway
	IPConfigurationAvailableDNSServer
	IPConfigurationAvailableMTU
)

type IPAddress struct {
	IP           net.IP
	PrefixLength uint32
}

type IPConfigurationInfo struct {
	SessionID                  SessionID
	IPv4ConfigurationAvailable IPConfigurationAvailable
	IPv6ConfigurationAvailable IPConfigurationAvailable
	IPv4Addresses              []IPAddress
	IPv6Addresses              []IPAddress
	IPv4Gateway                net.IP
	IPv6Gateway                net.IP
	IPv4DNSServers             []net.IP
	IPv6DNSServers             []net.IP
	IPv4MTU                    uint32
	IPv6MTU                    uint32
}

type IPConfigurationRequest struct {
	TransactionID uint32
	SessionID     SessionID
	Response      *IPConfigurationInfo
}

type ipConfigurationFields struct {
	name          string
	availability  IPConfigurationAvailable
	addressCount  uint32
	addressOffset uint32
	gatewayOffset uint32
	dnsCount      uint32
	dnsOffset     uint32
	mtu           uint32
}

func (r *IPConfigurationRequest) Request() *Request {
	data := make([]byte, ipConfigurationInfoFixedSize)
	binary.LittleEndian.PutUint32(data[:4], uint32(r.SessionID))

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
	if len(data) < ipConfigurationInfoFixedSize {
		return errors.New("parsing MBIM IP configuration: payload is truncated")
	}

	ipv4Available := IPConfigurationAvailable(binary.LittleEndian.Uint32(data[4:8]))
	ipv6Available := IPConfigurationAvailable(binary.LittleEndian.Uint32(data[8:12]))
	availability := []struct {
		name  string
		value IPConfigurationAvailable
	}{
		{name: "IPv4", value: ipv4Available},
		{name: "IPv6", value: ipv6Available},
	}
	for _, field := range availability {
		if field.value&^ipConfigurationAvailableMask != 0 {
			return fmt.Errorf("parsing MBIM IP configuration: %s availability %#x contains reserved bits", field.name, field.value)
		}
		basic := field.value & ipConfigurationBasicMask
		if basic != 0 && basic != ipConfigurationBasicMask {
			return fmt.Errorf("parsing MBIM IP configuration: %s address, gateway, and MTU availability bits differ", field.name)
		}
	}

	ipv4AddressCount := binary.LittleEndian.Uint32(data[12:16])
	ipv4AddressOffset := binary.LittleEndian.Uint32(data[16:20])
	ipv6AddressCount := binary.LittleEndian.Uint32(data[20:24])
	ipv6AddressOffset := binary.LittleEndian.Uint32(data[24:28])
	ipv4GatewayOffset := binary.LittleEndian.Uint32(data[28:32])
	ipv6GatewayOffset := binary.LittleEndian.Uint32(data[32:36])
	ipv4DNSCount := binary.LittleEndian.Uint32(data[36:40])
	ipv4DNSOffset := binary.LittleEndian.Uint32(data[40:44])
	ipv6DNSCount := binary.LittleEndian.Uint32(data[44:48])
	ipv6DNSOffset := binary.LittleEndian.Uint32(data[48:52])
	ipv4MTU := binary.LittleEndian.Uint32(data[52:56])
	ipv6MTU := binary.LittleEndian.Uint32(data[56:60])

	fields := []ipConfigurationFields{
		{
			name:          "IPv4",
			availability:  ipv4Available,
			addressCount:  ipv4AddressCount,
			addressOffset: ipv4AddressOffset,
			gatewayOffset: ipv4GatewayOffset,
			dnsCount:      ipv4DNSCount,
			dnsOffset:     ipv4DNSOffset,
			mtu:           ipv4MTU,
		},
		{
			name:          "IPv6",
			availability:  ipv6Available,
			addressCount:  ipv6AddressCount,
			addressOffset: ipv6AddressOffset,
			gatewayOffset: ipv6GatewayOffset,
			dnsCount:      ipv6DNSCount,
			dnsOffset:     ipv6DNSOffset,
			mtu:           ipv6MTU,
		},
	}
	for _, field := range fields {
		if err := field.validate(); err != nil {
			return fmt.Errorf("parsing MBIM IP configuration: %w", err)
		}
	}

	ipv4Addresses, err := parseIPv4Elements(data, ipv4AddressOffset, ipv4AddressCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv4 addresses: %w", err)
	}
	ipv6Addresses, err := parseIPv6Elements(data, ipv6AddressOffset, ipv6AddressCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv6 addresses: %w", err)
	}
	ipv4Gateway, err := parseIPv4Address(data, ipv4GatewayOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv4 gateway: %w", err)
	}
	ipv6Gateway, err := parseIPv6Address(data, ipv6GatewayOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv6 gateway: %w", err)
	}
	ipv4DNS, err := parseIPv4Addresses(data, ipv4DNSOffset, ipv4DNSCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv4 DNS servers: %w", err)
	}
	ipv6DNS, err := parseIPv6Addresses(data, ipv6DNSOffset, ipv6DNSCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP configuration IPv6 DNS servers: %w", err)
	}

	ipv4GatewaySize := uint32(0)
	if ipv4GatewayOffset != 0 {
		ipv4GatewaySize = net.IPv4len
	}
	ipv6GatewaySize := uint32(0)
	if ipv6GatewayOffset != 0 {
		ipv6GatewaySize = net.IPv6len
	}
	refs := []valueRef{
		{offset: ipv4AddressOffset, size: ipv4AddressCount * 8},
		{offset: ipv6AddressOffset, size: ipv6AddressCount * 20},
		{offset: ipv4GatewayOffset, size: ipv4GatewaySize},
		{offset: ipv6GatewayOffset, size: ipv6GatewaySize},
		{offset: ipv4DNSOffset, size: ipv4DNSCount * net.IPv4len},
		{offset: ipv6DNSOffset, size: ipv6DNSCount * net.IPv6len},
	}
	if err := validateDataBufferRefs(data, ipConfigurationInfoFixedSize, refs); err != nil {
		return fmt.Errorf("parsing MBIM IP configuration data buffer: %w", err)
	}

	*r = IPConfigurationInfo{
		SessionID:                  SessionID(binary.LittleEndian.Uint32(data[:4])),
		IPv4ConfigurationAvailable: ipv4Available,
		IPv6ConfigurationAvailable: ipv6Available,
		IPv4Addresses:              ipv4Addresses,
		IPv6Addresses:              ipv6Addresses,
		IPv4Gateway:                ipv4Gateway,
		IPv6Gateway:                ipv6Gateway,
		IPv4DNSServers:             ipv4DNS,
		IPv6DNSServers:             ipv6DNS,
		IPv4MTU:                    ipv4MTU,
		IPv6MTU:                    ipv6MTU,
	}
	return nil
}

func (fields ipConfigurationFields) validate() error {
	if fields.availability&IPConfigurationAvailableAddress == 0 &&
		(fields.addressCount != 0 || fields.addressOffset != 0) {
		return fmt.Errorf("%s address fields are nonzero while address information is unavailable", fields.name)
	}
	if fields.availability&IPConfigurationAvailableGateway == 0 && fields.gatewayOffset != 0 {
		return fmt.Errorf("%s gateway offset is nonzero while gateway information is unavailable", fields.name)
	}
	if fields.availability&IPConfigurationAvailableDNSServer == 0 &&
		(fields.dnsCount != 0 || fields.dnsOffset != 0) {
		return fmt.Errorf("%s DNS fields are nonzero while DNS information is unavailable", fields.name)
	}
	if fields.availability&IPConfigurationAvailableMTU == 0 && fields.mtu != 0 {
		return fmt.Errorf("%s MTU is nonzero while MTU information is unavailable", fields.name)
	}
	return nil
}

func (c *Client) IPConfiguration(ctx context.Context, sessionID SessionID) (IPConfigurationInfo, error) {
	request := IPConfigurationRequest{
		TransactionID: c.nextTransactionID(),
		SessionID:     sessionID,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return IPConfigurationInfo{}, fmt.Errorf("reading MBIM IP configuration: %w", err)
	}
	resp := *request.Response
	resp.IPv4Addresses = cloneIPAddresses(resp.IPv4Addresses)
	resp.IPv6Addresses = cloneIPAddresses(resp.IPv6Addresses)
	resp.IPv4Gateway = slices.Clone(resp.IPv4Gateway)
	resp.IPv6Gateway = slices.Clone(resp.IPv6Gateway)
	resp.IPv4DNSServers = cloneIPs(resp.IPv4DNSServers)
	resp.IPv6DNSServers = cloneIPs(resp.IPv6DNSServers)
	return resp, nil
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
		if addresses[len(addresses)-1].PrefixLength > 32 {
			return nil, errors.New("IPv4 prefix length is invalid")
		}
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
		if addresses[len(addresses)-1].PrefixLength > 128 {
			return nil, errors.New("IPv6 prefix length is invalid")
		}
	}
	return addresses, nil
}

func parseIPv4Addresses(data []byte, offset, count uint32) ([]net.IP, error) {
	if count == 0 {
		return nil, nil
	}
	if offset > uint32(len(data)) || count > (uint32(len(data))-offset)/4 {
		return nil, errors.New("IPv4 address table is truncated")
	}
	addresses := make([]net.IP, 0, count)
	for i := range count {
		start := offset + i*4
		addresses = append(addresses, net.IPv4(data[start], data[start+1], data[start+2], data[start+3]))
	}
	return addresses, nil
}

func parseIPv6Addresses(data []byte, offset, count uint32) ([]net.IP, error) {
	if count == 0 {
		return nil, nil
	}
	if offset > uint32(len(data)) || count > (uint32(len(data))-offset)/16 {
		return nil, errors.New("IPv6 address table is truncated")
	}
	addresses := make([]net.IP, 0, count)
	for i := range count {
		start := offset + i*16
		addresses = append(addresses, slices.Clone(net.IP(data[start:start+16])))
	}
	return addresses, nil
}

func parseIPv4Address(data []byte, offset uint32) (net.IP, error) {
	if offset == 0 {
		return nil, nil
	}
	addresses, err := parseIPv4Addresses(data, offset, 1)
	if err != nil {
		return nil, err
	}
	return addresses[0], nil
}

func parseIPv6Address(data []byte, offset uint32) (net.IP, error) {
	if offset == 0 {
		return nil, nil
	}
	addresses, err := parseIPv6Addresses(data, offset, 1)
	if err != nil {
		return nil, err
	}
	return addresses[0], nil
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
