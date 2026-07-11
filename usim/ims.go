package usim

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

const (
	// DefaultIMSPDNAPN is the APN normally used for IMS registration over LTE.
	DefaultIMSPDNAPN = "ims"
	// DefaultIMSPDNType asks the modem for a dual-stack IMS PDN.
	DefaultIMSPDNType = "ipv4v6"
)

// IMSPDNConfig describes the IMS packet data network requested from the modem.
type IMSPDNConfig struct {
	APN               string
	PDNType           string
	ProfileIndex      uint8
	RequestTimeout    time.Duration
	MuxDataPort       *qcom.WDSMuxDataPort
	LegacyMuxDataPort qcom.WDSSIOPort
}

// IMSPDNInfo contains the IMS PDN addresses and LTE voice support flags.
type IMSPDNInfo struct {
	SessionID       uint32
	LocalIPv4       net.IP
	LocalIPv6       net.IP
	PCSCFIPs        []net.IP
	VoPSKnown       bool
	VoPSSupported   bool
	PacketDataReady bool
}

// IMSPDNSession keeps the modem IMS PDN open until Close is called.
type IMSPDNSession struct {
	info  func() IMSPDNInfo
	close func() error
}

// Info returns the current IMS PDN information known after session setup.
func (s *IMSPDNSession) Info() IMSPDNInfo {
	if s == nil || s.info == nil {
		return IMSPDNInfo{}
	}
	return cloneIMSPDNInfo(s.info())
}

// Close releases the modem IMS PDN session.
func (s *IMSPDNSession) Close() error {
	if s == nil || s.close == nil {
		return nil
	}
	return s.close()
}

func (cfg IMSPDNConfig) normalized() (IMSPDNConfig, error) {
	cfg.APN = strings.TrimSpace(cfg.APN)
	if cfg.APN == "" {
		cfg.APN = DefaultIMSPDNAPN
	}
	cfg.PDNType = strings.ToLower(strings.TrimSpace(cfg.PDNType))
	if cfg.PDNType == "" {
		cfg.PDNType = DefaultIMSPDNType
	}
	switch cfg.PDNType {
	case "ipv4", "ipv6", "ipv4v6":
	default:
		return IMSPDNConfig{}, fmt.Errorf("normalizing IMS PDN config: unsupported pdn type %q", cfg.PDNType)
	}
	return cfg, nil
}

func cloneIMSPDNInfo(info IMSPDNInfo) IMSPDNInfo {
	info.LocalIPv4 = append(net.IP(nil), info.LocalIPv4...)
	info.LocalIPv6 = append(net.IP(nil), info.LocalIPv6...)
	info.PCSCFIPs = cloneIPs(info.PCSCFIPs)
	return info
}

func cloneIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		out = append(out, append(net.IP(nil), ip...))
	}
	return out
}
