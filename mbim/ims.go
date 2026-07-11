package mbim

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	DefaultIMSPDNAPN       = "ims"
	DefaultIMSPDNSessionID = 1
)

type IMSPDNConfig struct {
	APN            string
	IPType         ContextIPType
	SessionID      uint32
	RequestTimeout time.Duration
}

type IMSPDNInfo struct {
	SessionID        uint32
	LocalIPv4        net.IP
	LocalIPv6        net.IP
	PCSCFIPs         []net.IP
	DNSIPs           []net.IP
	IPv4LinkMTU      uint16
	IPv4LinkMTUKnown bool
	IPType           ContextIPType
	AccessString     string
	NwError          uint32
	VoPSKnown        bool
	VoPSSupported    bool
	PacketDataReady  bool
}

type IMSPDNSession struct {
	reader    *Reader
	sessionID uint32
	timeout   time.Duration
	info      IMSPDNInfo

	closeOnce sync.Once
	closeErr  error
}

func (r *Reader) OpenIMSPDN(ctx context.Context, cfg IMSPDNConfig) (*IMSPDNSession, error) {
	if r == nil {
		return nil, errors.New("opening MBIM IMS PDN: reader is nil")
	}
	cfg = normalizeIMSPDNConfig(cfg)
	deviceCaps, err := r.DeviceCaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening MBIM IMS PDN: %w", err)
	}
	if deviceCaps.MaxSessions == 0 {
		return nil, errors.New("opening MBIM IMS PDN: device reports zero IP sessions")
	}
	if cfg.SessionID >= deviceCaps.MaxSessions {
		return nil, fmt.Errorf("opening MBIM IMS PDN: session ID %d is out of range for %d supported sessions", cfg.SessionID, deviceCaps.MaxSessions)
	}
	if err := r.ensurePacketServiceAttached(ctx); err != nil {
		return nil, fmt.Errorf("opening MBIM IMS PDN: %w", err)
	}

	session := &IMSPDNSession{
		reader:    r,
		sessionID: cfg.SessionID,
		timeout:   cfg.RequestTimeout,
	}
	connect, err := session.activate(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if connect.ActivationState != ActivationStateActivated {
		_ = session.Close()
		return nil, fmt.Errorf("opening MBIM IMS PDN: activation state %d, want %d", connect.ActivationState, ActivationStateActivated)
	}

	ipConfig, err := r.IPConfiguration(ctx, cfg.SessionID)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("opening MBIM IMS PDN: %w", err)
	}

	session.info = IMSPDNInfo{
		SessionID:        cfg.SessionID,
		LocalIPv4:        firstIP(ipConfig.IPv4Addresses),
		LocalIPv6:        firstIP(ipConfig.IPv6Addresses),
		PCSCFIPs:         cloneIPs(connect.PCSCFIPs),
		DNSIPs:           cloneIPs(connect.DNSIPs),
		IPv4LinkMTU:      connect.IPv4LinkMTU,
		IPv4LinkMTUKnown: connect.IPv4LinkMTUKnown,
		IPType:           connect.IPType,
		AccessString:     connect.AccessString,
		NwError:          connect.NwError,
		PacketDataReady:  true,
	}
	return session, nil
}

func (s *IMSPDNSession) Info() IMSPDNInfo {
	if s == nil {
		return IMSPDNInfo{}
	}
	info := s.info
	info.LocalIPv4 = slices.Clone(info.LocalIPv4)
	info.LocalIPv6 = slices.Clone(info.LocalIPv6)
	info.PCSCFIPs = cloneIPs(info.PCSCFIPs)
	info.DNSIPs = cloneIPs(info.DNSIPs)
	return info
}

func (s *IMSPDNSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.reader == nil {
			return
		}
		timeout := s.timeout
		if timeout <= 0 {
			timeout = mbimConnectSetResponseTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		request := ConnectRequest{
			TransactionID:     s.reader.nextTransactionID(),
			MBIMExVersion:     s.reader.mbimExVersion,
			Timeout:           timeout,
			SessionID:         s.sessionID,
			ActivationCommand: ActivationCommandDeactivate,
			IPType:            s.info.IPType,
			ContextType:       ContextTypeIMS,
			MediaPreference:   AccessMediaType3GPP,
		}
		err := s.reader.transmit(ctx, request.Request())
		if errors.Is(err, StatusContextNotActivated) {
			err = nil
		}
		if err != nil {
			err = fmt.Errorf("closing MBIM IMS PDN: %w", err)
		}
		s.closeErr = err
	})
	return s.closeErr
}

func (s *IMSPDNSession) activate(ctx context.Context, cfg IMSPDNConfig) (ConnectInfo, error) {
	request := ConnectRequest{
		TransactionID:     s.reader.nextTransactionID(),
		MBIMExVersion:     s.reader.mbimExVersion,
		Timeout:           cfg.RequestTimeout,
		SessionID:         cfg.SessionID,
		ActivationCommand: ActivationCommandActivate,
		AccessString:      cfg.APN,
		Compression:       CompressionNone,
		AuthProtocol:      AuthProtocolNone,
		IPType:            cfg.IPType,
		ContextType:       ContextTypeIMS,
		MediaPreference:   AccessMediaType3GPP,
	}
	if err := s.reader.transmit(ctx, request.Request()); err != nil {
		return ConnectInfo{}, fmt.Errorf("activating MBIM IMS PDN: %w", err)
	}
	return *request.Response, nil
}

func normalizeIMSPDNConfig(cfg IMSPDNConfig) IMSPDNConfig {
	cfg.APN = strings.TrimSpace(cfg.APN)
	if cfg.APN == "" {
		cfg.APN = DefaultIMSPDNAPN
	}
	if cfg.IPType == 0 {
		cfg.IPType = ContextIPTypeIPv4v6
	}
	if cfg.SessionID == 0 {
		cfg.SessionID = DefaultIMSPDNSessionID
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = mbimConnectSetResponseTimeout
	}
	return cfg
}

func (r *Reader) ensurePacketServiceAttached(ctx context.Context) error {
	info, err := r.PacketService(ctx)
	if err != nil {
		return err
	}
	switch info.PacketServiceState {
	case PacketServiceStateAttached:
		return nil
	case PacketServiceStateAttaching:
		return r.waitPacketServiceAttached(ctx)
	case PacketServiceStateDetached, PacketServiceStateUnknown:
		info, err := r.SetPacketService(ctx, PacketServiceActionAttach)
		if err != nil {
			return err
		}
		if info.PacketServiceState != PacketServiceStateAttached {
			return fmt.Errorf("attaching MBIM packet service: state %d, want %d", info.PacketServiceState, PacketServiceStateAttached)
		}
		return nil
	default:
		return fmt.Errorf("attaching MBIM packet service: state %d is not attachable", info.PacketServiceState)
	}
}

func (r *Reader) waitPacketServiceAttached(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, mbimCIDLongResponseTimeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for MBIM packet service attach: %w", ctx.Err())
		case <-ticker.C:
			info, err := r.PacketService(ctx)
			if err != nil {
				return err
			}
			switch info.PacketServiceState {
			case PacketServiceStateAttached:
				return nil
			case PacketServiceStateDetached, PacketServiceStateUnknown:
				return fmt.Errorf("waiting for MBIM packet service attach: state %d", info.PacketServiceState)
			}
		}
	}
}

func firstIP(addresses []IPAddress) net.IP {
	for _, address := range addresses {
		if len(address.IP) == 0 || address.IP.IsUnspecified() {
			continue
		}
		return slices.Clone(address.IP)
	}
	return nil
}

func cloneIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	cloned := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		cloned = append(cloned, slices.Clone(ip))
	}
	return cloned
}
