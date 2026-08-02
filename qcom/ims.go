package qcom

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const DefaultIMSPDNAPN = "ims"

// PDNConfig describes a general QMI WDS packet-data call.
type PDNConfig struct {
	APN               string
	Authentication    WDSAuthenticationMask
	Username          string
	Password          string
	IPPreference      WDSIPPreference
	ProfileIndex      uint8
	Subscription      *WDSSubscription
	RequestTimeout    time.Duration
	MuxDataPort       *WDSMuxDataPort
	LegacyMuxDataPort WDSSIOPort
	LegacyMuxFallback *WDSMuxDataPort
	CallType          *WDSCallType
}

// PDNInfo contains the negotiated network configuration of a packet-data call.
type PDNInfo struct {
	LocalIPv4        net.IP
	LocalIPv6        net.IP
	IPv4Gateway      net.IP
	IPv4SubnetMask   net.IP
	IPv6Gateway      net.IP
	IPv6PrefixLength uint8
	DNS              []net.IP
	MTU              uint32
	IPFamily         WDSIPFamily
	PacketDataReady  bool
}

// IMSPDNConfig describes the modem-side IMS PDN request.
type IMSPDNConfig struct {
	APN               string
	IPPreference      WDSIPPreference
	ProfileIndex      uint8
	RequestTimeout    time.Duration
	MuxDataPort       *WDSMuxDataPort
	LegacyMuxDataPort WDSSIOPort
	LegacyMuxFallback *WDSMuxDataPort
}

// IMSPDNInfo contains IMS-specific state in addition to the underlying PDN.
type IMSPDNInfo struct {
	PDNInfo
	PCSCFIPs      []net.IP
	IMCN          bool
	VoPSKnown     bool
	VoPSSupported bool
}

type pdnOpenConfig struct {
	PDNConfig
	technology        WDSTechnologyPreference
	requestedSettings WDSRuntimeSettingsMask
	profileIPFamily   bool
}

// PDNSession owns a WDS packet-data handle and, on QMUX, its QMI client ID.
type PDNSession struct {
	mu      sync.RWMutex
	client  *Client
	info    PDNInfo
	runtime WDSRuntimeSettings

	timeout           time.Duration
	closeOnce         sync.Once
	closeErr          error
	wdsClientID       uint8
	wdsClientReady    bool
	releaseWDSClient  bool
	packetDataHandle  uint32
	requestedSettings WDSRuntimeSettingsMask
	connectionStatus  WDSConnectionStatus

	watchMu        sync.Mutex
	statusWatchers int
	done           chan struct{}
}

// IMSPDNSession owns an IMS PDN and the NAS client used for VoPS state.
type IMSPDNSession struct {
	pdn  *PDNSession
	info IMSPDNInfo

	closeOnce        sync.Once
	closeErr         error
	nasClientID      uint8
	releaseNASClient bool
}

// OpenPDN starts a general QMI WDS packet-data call. It does not allocate a NAS
// client or apply IMS defaults.
func (c *Client) OpenPDN(ctx context.Context, cfg PDNConfig) (*PDNSession, error) {
	session, err := c.openPDN(ctx, pdnOpenConfig{
		PDNConfig:         cfg,
		requestedSettings: WDSRuntimeRequestedNetworkSettings,
	})
	if err != nil {
		return nil, fmt.Errorf("opening PDN: %w", err)
	}
	return session, nil
}

// OpenIMSPDN starts an IMS PDN and reads the matching NAS voice state. When
// IPPreference leaves the family to the modem, a 3GPP IPv4-only or IPv6-only
// response starts one new WDS call with that family. Info reports the family
// negotiated by the successful call.
func (c *Client) OpenIMSPDN(ctx context.Context, cfg IMSPDNConfig) (*IMSPDNSession, error) {
	if c == nil {
		return nil, errors.New("opening IMS PDN: client is nil")
	}
	cfg.APN = strings.TrimSpace(cfg.APN)
	if cfg.APN == "" {
		cfg.APN = DefaultIMSPDNAPN
	}
	callType := WDSCallTypeEmbedded
	pdnCfg := pdnOpenConfig{
		PDNConfig: PDNConfig{
			APN:               cfg.APN,
			IPPreference:      cfg.IPPreference,
			ProfileIndex:      cfg.ProfileIndex,
			RequestTimeout:    cfg.RequestTimeout,
			MuxDataPort:       cfg.MuxDataPort,
			LegacyMuxDataPort: cfg.LegacyMuxDataPort,
			LegacyMuxFallback: cfg.LegacyMuxFallback,
			CallType:          &callType,
		},
		technology: WDSTechnologyPreference3GPP,
		requestedSettings: WDSRuntimeRequestedIMSSettings |
			WDSRuntimeRequestedNetworkSettings,
		profileIPFamily: cfg.LegacyMuxDataPort != 0,
	}
	pdn, err := c.openPDN(ctx, pdnCfg)
	if err != nil && !pdnCfg.profileIPFamily &&
		(cfg.IPPreference == WDSIPPreferenceDefault || cfg.IPPreference == WDSIPPreferenceUnspecified) {
		if restricted, ok := wdsRestrictedIPPreference(err); ok {
			initialErr := err
			pdnCfg.IPPreference = restricted
			pdn, err = c.openPDN(ctx, pdnCfg)
			if err != nil {
				return nil, errors.Join(
					fmt.Errorf("opening IMS PDN: %w", initialErr),
					fmt.Errorf("opening IMS PDN with network IP family %s: %w", restricted, err),
				)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("opening IMS PDN: %w", err)
	}

	session := &IMSPDNSession{pdn: pdn}
	nasClientID, releaseNASClient, err := c.sessionServiceClientID(ctx, ServiceNAS)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("opening IMS PDN: open NAS client: %w", err), pdn.Close())
	}
	session.nasClientID = nasClientID
	session.releaseNASClient = releaseNASClient

	sys, err := session.nasSysInfo(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("opening IMS PDN: read NAS system info: %w", err), session.Close())
	}
	pdn.mu.RLock()
	pcscfIPs := cloneIPs(pdn.runtime.PCSCFIPs)
	imcn := pdn.runtime.IMCN
	pdn.mu.RUnlock()
	session.info = IMSPDNInfo{
		PDNInfo:       pdn.Info(),
		PCSCFIPs:      pcscfIPs,
		IMCN:          imcn,
		VoPSKnown:     sys.VoPSKnown,
		VoPSSupported: sys.VoPSSupported,
	}
	return session, nil
}

func wdsRestrictedIPPreference(err error) (WDSIPPreference, bool) {
	switch {
	case errors.Is(err, ErrWDSIPv4Only):
		return WDSIPPreferenceIPv4, true
	case errors.Is(err, ErrWDSIPv6Only):
		return WDSIPPreferenceIPv6, true
	default:
		return 0, false
	}
}

func (c *Client) openPDN(ctx context.Context, cfg pdnOpenConfig) (*PDNSession, error) {
	if c == nil {
		return nil, errors.New("client is nil")
	}
	if cfg.MuxDataPort != nil && cfg.LegacyMuxDataPort != 0 {
		return nil, errors.New("mux data port and legacy mux data port are mutually exclusive")
	}
	if cfg.LegacyMuxFallback != nil && cfg.LegacyMuxDataPort == 0 {
		return nil, errors.New("legacy mux fallback requires a legacy mux data port")
	}
	if err := validateWDSIPPreference(cfg.IPPreference); err != nil {
		return nil, err
	}
	if cfg.Subscription != nil {
		if err := validateWDSSubscription(*cfg.Subscription); err != nil {
			return nil, err
		}
	}
	cfg.APN = strings.TrimSpace(cfg.APN)
	if err := validateWDSString(cfg.APN, wdsAPNMaxLength); err != nil {
		return nil, fmt.Errorf("validating WDS APN: %w", err)
	}
	if err := validateWDSString(cfg.Username, wdsUsernameMaxLength); err != nil {
		return nil, fmt.Errorf("validating WDS username: %w", err)
	}
	if err := validateWDSString(cfg.Password, wdsPasswordMaxLength); err != nil {
		return nil, fmt.Errorf("validating WDS password: %w", err)
	}
	if err := validateWDSAuthentication(cfg.Authentication); err != nil {
		return nil, err
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	session := &PDNSession{client: c, timeout: timeout, done: make(chan struct{})}

	wdsClientID, releaseWDSClient, err := c.sessionServiceClientID(ctx, ServiceWDS)
	if err != nil {
		return nil, fmt.Errorf("open WDS client: %w", err)
	}
	session.wdsClientID = wdsClientID
	session.wdsClientReady = true
	session.releaseWDSClient = releaseWDSClient
	session.requestedSettings = cfg.requestedSettings
	if cfg.Subscription != nil {
		if err := session.bindSubscription(ctx, *cfg.Subscription); err != nil {
			return nil, errors.Join(err, session.Close())
		}
	}
	// Some IMS profiles on legacy BAM-DMUX firmware own the IP-family choice.
	// General PDNs still need the caller's explicit family on the same endpoint.
	if !cfg.profileIPFamily && (cfg.IPPreference == WDSIPPreferenceIPv4 || cfg.IPPreference == WDSIPPreferenceIPv6) {
		if err := session.setClientIPFamily(ctx, WDSIPFamily(cfg.IPPreference)); err != nil {
			return nil, errors.Join(err, session.Close())
		}
	}
	if cfg.MuxDataPort != nil {
		if err := session.bindMuxDataPort(ctx, *cfg.MuxDataPort); err != nil {
			return nil, errors.Join(err, session.Close())
		}
	} else if cfg.LegacyMuxDataPort != 0 {
		err := session.bindLegacyMuxDataPort(ctx, cfg.LegacyMuxDataPort)
		if errors.Is(err, QMIErrorDeviceUnsupported) && cfg.LegacyMuxFallback != nil {
			err = session.bindMuxDataPort(ctx, *cfg.LegacyMuxFallback)
		}
		if err != nil {
			return nil, errors.Join(err, session.Close())
		}
	}
	startCfg := cfg
	if cfg.profileIPFamily {
		startCfg.IPPreference = WDSIPPreferenceDefault
	}
	if err := session.start(ctx, startCfg); err != nil {
		return nil, errors.Join(err, session.Close())
	}
	runtime, err := session.runtimeSettings(ctx, cfg.requestedSettings)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("read runtime settings: %w", err), session.Close())
	}
	session.runtime = runtime
	session.info = pdnInfo(runtime, session.packetDataHandle != 0)
	session.connectionStatus = WDSConnectionStatusConnected
	return session, nil
}

func validateWDSIPPreference(preference WDSIPPreference) error {
	switch preference {
	case WDSIPPreferenceDefault,
		WDSIPPreferenceIPv4,
		WDSIPPreferenceIPv6,
		WDSIPPreferenceUnspecified:
		return nil
	default:
		return fmt.Errorf("unsupported WDS IP preference %d", preference)
	}
}

func pdnInfo(runtime WDSRuntimeSettings, ready bool) PDNInfo {
	return PDNInfo{
		LocalIPv4:        append(net.IP(nil), runtime.LocalIPv4...),
		LocalIPv6:        append(net.IP(nil), runtime.LocalIPv6...),
		IPv4Gateway:      append(net.IP(nil), runtime.IPv4Gateway...),
		IPv4SubnetMask:   append(net.IP(nil), runtime.IPv4SubnetMask...),
		IPv6Gateway:      append(net.IP(nil), runtime.IPv6Gateway...),
		IPv6PrefixLength: runtime.IPv6PrefixLength,
		DNS:              cloneIPs(runtime.DNS),
		MTU:              runtime.MTU,
		IPFamily:         runtime.IPFamily,
		PacketDataReady:  ready,
	}
}

// Info returns a defensive copy of the negotiated PDN state.
func (s *PDNSession) Info() PDNInfo {
	if s == nil {
		return PDNInfo{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePDNInfo(s.info)
}

// Info returns a defensive copy of the IMS PDN state.
func (s *IMSPDNSession) Info() IMSPDNInfo {
	if s == nil {
		return IMSPDNInfo{}
	}
	info := s.info
	info.PDNInfo = clonePDNInfo(info.PDNInfo)
	info.PCSCFIPs = cloneIPs(info.PCSCFIPs)
	return info
}

func clonePDNInfo(info PDNInfo) PDNInfo {
	info.LocalIPv4 = append(net.IP(nil), info.LocalIPv4...)
	info.LocalIPv6 = append(net.IP(nil), info.LocalIPv6...)
	info.IPv4Gateway = append(net.IP(nil), info.IPv4Gateway...)
	info.IPv4SubnetMask = append(net.IP(nil), info.IPv4SubnetMask...)
	info.IPv6Gateway = append(net.IP(nil), info.IPv6Gateway...)
	info.DNS = cloneIPs(info.DNS)
	return info
}

// Close stops the packet-data call and releases its WDS client.
func (s *PDNSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
		defer cancel()
		if s.done != nil {
			close(s.done)
		}

		s.mu.RLock()
		packetDataHandle := s.packetDataHandle
		wdsClientReady := s.wdsClientReady
		releaseWDSClient := s.releaseWDSClient
		wdsClientID := s.wdsClientID
		s.mu.RUnlock()

		var err error
		if packetDataHandle != 0 && wdsClientReady {
			err = errors.Join(err, s.stop(ctx))
		}
		if releaseWDSClient {
			err = errors.Join(err, s.client.releaseServiceClientID(ctx, ServiceWDS, wdsClientID))
		}
		s.mu.Lock()
		s.wdsClientID = 0
		s.wdsClientReady = false
		s.releaseWDSClient = false
		s.packetDataHandle = 0
		s.mu.Unlock()
		s.closeErr = err
	})
	return s.closeErr
}

// Close stops the IMS PDN and releases its NAS client.
func (s *IMSPDNSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
		defer cancel()

		var err error
		if s.pdn != nil {
			err = errors.Join(err, s.pdn.Close())
		}
		if s.releaseNASClient && s.pdn != nil {
			err = errors.Join(err, s.pdn.client.releaseServiceClientID(ctx, ServiceNAS, s.nasClientID))
		}
		s.nasClientID = 0
		s.releaseNASClient = false
		s.closeErr = err
	})
	return s.closeErr
}

func (s *PDNSession) setClientIPFamily(ctx context.Context, family WDSIPFamily) error {
	resp, err := s.client.requestServiceWithTimeout(ctx, ServiceWDS, s.wdsClientID, MessageWDSSetClientIPFamily, tlv.TLVs{tlv.Uint(0x01, uint8(family))}, s.timeout)
	if err != nil {
		return fmt.Errorf("set WDS client IP family: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("set WDS client IP family: %w", err)
	}
	return nil
}

func (s *PDNSession) bindSubscription(ctx context.Context, subscription WDSSubscription) error {
	req, err := (WDSBindSubscriptionRequest{
		ClientID:     s.wdsClientID,
		Timeout:      s.timeout,
		Subscription: subscription,
	}).Request()
	if err != nil {
		return err
	}
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("bind WDS subscription: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("bind WDS subscription: %w", err)
	}
	return nil
}

func (s *PDNSession) bindMuxDataPort(ctx context.Context, dataPort WDSMuxDataPort) error {
	req := WDSBindMuxDataPortRequest{ClientID: s.wdsClientID, Timeout: s.timeout, DataPort: dataPort}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return &WDSBindMuxDataPortError{Err: err}
	}
	if err := resultOK(resp); err != nil {
		return &WDSBindMuxDataPortError{Err: err}
	}
	return nil
}

func (s *PDNSession) bindLegacyMuxDataPort(ctx context.Context, dataPort WDSSIOPort) error {
	req := WDSLegacyBindMuxDataPortRequest{ClientID: s.wdsClientID, Timeout: s.timeout, DataPort: dataPort}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("bind WDS legacy mux data port: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("bind WDS legacy mux data port: %w", err)
	}
	return nil
}

func (s *PDNSession) start(ctx context.Context, cfg pdnOpenConfig) error {
	req := WDSStartNetworkInterfaceRequest{
		ClientID:             s.wdsClientID,
		Timeout:              s.timeout,
		APN:                  cfg.APN,
		Authentication:       cfg.Authentication,
		Username:             cfg.Username,
		Password:             cfg.Password,
		IPPreference:         cfg.IPPreference,
		TechnologyPreference: cfg.technology,
		ProfileIndex3GPP:     cfg.ProfileIndex,
		CallType:             cfg.CallType,
	}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("start WDS network: %w", err)
	}
	var parsed WDSStartNetworkInterfaceResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return err
	}
	if err := resultOK(resp); err != nil {
		return &WDSStartNetworkError{
			Err:                     err,
			CallEndReason:           parsed.CallEndReason,
			HasCallEndReason:        parsed.HasCallEndReason,
			VerboseCallEndReason:    parsed.VerboseCallEndReason,
			HasVerboseCallEndReason: parsed.HasVerboseCallEndReason,
		}
	}
	if parsed.PacketDataHandle == 0 {
		return errors.New("start WDS network: packet data handle is missing")
	}
	s.packetDataHandle = parsed.PacketDataHandle
	return nil
}

func (s *PDNSession) stop(ctx context.Context) error {
	s.mu.RLock()
	clientID := s.wdsClientID
	timeout := s.timeout
	packetDataHandle := s.packetDataHandle
	s.mu.RUnlock()
	req := WDSStopNetworkInterfaceRequest{ClientID: clientID, Timeout: timeout, PacketDataHandle: packetDataHandle}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("stop WDS network: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("stop WDS network: %w", err)
	}
	s.mu.Lock()
	s.packetDataHandle = 0
	s.mu.Unlock()
	return nil
}

func (s *PDNSession) runtimeSettings(ctx context.Context, requested WDSRuntimeSettingsMask) (WDSRuntimeSettings, error) {
	s.mu.RLock()
	clientID := s.wdsClientID
	timeout := s.timeout
	s.mu.RUnlock()
	req := WDSGetRuntimeSettingsRequest{ClientID: clientID, Timeout: timeout, RequestedSettings: requested}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return WDSRuntimeSettings{}, err
	}
	if err := resultOK(resp); err != nil {
		return WDSRuntimeSettings{}, err
	}
	var parsed WDSGetRuntimeSettingsResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSRuntimeSettings{}, err
	}
	return parsed.Settings, nil
}

func (s *IMSPDNSession) nasSysInfo(ctx context.Context) (NASSysInfo, error) {
	req := NASGetSysInfoRequest{ClientID: s.nasClientID, Timeout: s.pdn.timeout}.Request()
	resp, err := s.pdn.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return NASSysInfo{}, err
	}
	if err := resultOK(resp); err != nil {
		return NASSysInfo{}, err
	}
	var parsed NASGetSysInfoResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return NASSysInfo{}, err
	}
	return parsed.SysInfo, nil
}

func cloneIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	cloned := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		cloned = append(cloned, append(net.IP(nil), ip...))
	}
	return cloned
}
