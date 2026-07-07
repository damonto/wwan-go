package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
)

const (
	pcoOptionPCSCFIPv6 uint16 = 0x0001
	pcoOptionDNSIPv6   uint16 = 0x0003
	pcoOptionPCSCFIPv4 uint16 = 0x000c
	pcoOptionDNSIPv4   uint16 = 0x000d
	pcoOptionIPv4MTU   uint16 = 0x0010
)

type ProtocolConfigurationOptions struct {
	Extension             bool
	ConfigurationProtocol byte
	Options               []PCOOption
	PCSCFIPs              []net.IP
	DNSIPs                []net.IP
	IPv4LinkMTU           uint16
	IPv4LinkMTUKnown      bool
}

type PCOOption struct {
	ID   uint16
	Data []byte
}

func (p *ProtocolConfigurationOptions) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return errors.New("parsing PCO: payload is empty")
	}

	*p = ProtocolConfigurationOptions{
		Extension:             data[0]&0x80 != 0,
		ConfigurationProtocol: data[0] & 0x07,
	}
	data = data[1:]
	for len(data) > 0 {
		if len(data) < 3 {
			return errors.New("parsing PCO: option header is truncated")
		}
		optionID := binary.BigEndian.Uint16(data[:2])
		data = data[2:]

		lengthSize := 1
		length := int(data[0])
		if pcoOptionUsesUint16Length(optionID) {
			if len(data) < 2 {
				return errors.New("parsing PCO: option length is truncated")
			}
			lengthSize = 2
			length = int(binary.BigEndian.Uint16(data[:2]))
		}
		data = data[lengthSize:]
		if length > len(data) {
			return errors.New("parsing PCO: option data is truncated")
		}
		p.Options = append(p.Options, PCOOption{
			ID:   optionID,
			Data: slices.Clone(data[:length]),
		})
		data = data[length:]
	}
	return p.parseOptions()
}

func PCSCFIPsFromPCO(data []byte) ([]net.IP, error) {
	var pco ProtocolConfigurationOptions
	if err := pco.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return pco.PCSCFIPs, nil
}

func (p *ProtocolConfigurationOptions) parseOptions() error {
	pcscfIPs, err := pcscfIPsFromOptions(p.Options)
	if err != nil {
		return err
	}
	dnsIPs, err := dnsIPsFromOptions(p.Options)
	if err != nil {
		return err
	}
	mtu, mtuKnown, err := ipv4LinkMTUFromOptions(p.Options)
	if err != nil {
		return err
	}
	p.PCSCFIPs = pcscfIPs
	p.DNSIPs = dnsIPs
	p.IPv4LinkMTU = mtu
	p.IPv4LinkMTUKnown = mtuKnown
	return nil
}

func pcscfIPsFromOptions(options []PCOOption) ([]net.IP, error) {
	ips, err := ipsFromOptions(options, pcoOptionPCSCFIPv4, pcoOptionPCSCFIPv6)
	if err != nil {
		return nil, fmt.Errorf("parsing PCO P-CSCF IPs: %w", err)
	}
	return ips, nil
}

func dnsIPsFromOptions(options []PCOOption) ([]net.IP, error) {
	ips, err := ipsFromOptions(options, pcoOptionDNSIPv4, pcoOptionDNSIPv6)
	if err != nil {
		return nil, fmt.Errorf("parsing PCO DNS server IPs: %w", err)
	}
	return ips, nil
}

func ipsFromOptions(options []PCOOption, ipv4ID, ipv6ID uint16) ([]net.IP, error) {
	var ips []net.IP
	for _, option := range options {
		switch option.ID {
		case ipv4ID:
			if len(option.Data)%4 != 0 {
				return nil, fmt.Errorf("IPv4 option length %d is not a multiple of 4", len(option.Data))
			}
			for chunk := range slices.Chunk(option.Data, 4) {
				ips = append(ips, net.IPv4(chunk[0], chunk[1], chunk[2], chunk[3]))
			}
		case ipv6ID:
			if len(option.Data)%16 != 0 {
				return nil, fmt.Errorf("IPv6 option length %d is not a multiple of 16", len(option.Data))
			}
			for chunk := range slices.Chunk(option.Data, 16) {
				ips = append(ips, slices.Clone(net.IP(chunk)))
			}
		}
	}
	return uniqueIPs(ips), nil
}

func ipv4LinkMTUFromOptions(options []PCOOption) (uint16, bool, error) {
	for _, option := range options {
		if option.ID != pcoOptionIPv4MTU {
			continue
		}
		if len(option.Data) != 2 {
			return 0, false, fmt.Errorf("parsing PCO: IPv4 link MTU option length %d, want 2", len(option.Data))
		}
		return binary.BigEndian.Uint16(option.Data), true, nil
	}
	return 0, false, nil
}

func pcoOptionUsesUint16Length(optionID uint16) bool {
	switch optionID {
	case 0x0023, 0x0024, 0x0030, 0x0031, 0x0032, 0x0041, 0x0051, 0x0056:
		return true
	default:
		return false
	}
}

func uniqueIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	unique := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if len(ip) == 0 || slices.ContainsFunc(unique, ip.Equal) {
			continue
		}
		unique = append(unique, slices.Clone(ip))
	}
	return unique
}
