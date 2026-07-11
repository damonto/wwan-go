package uim

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

const DefaultIMSPDNAPN = "ims"

// IMSPDNConfig describes the modem-side IMS PDN request.
type IMSPDNConfig struct {
	APN               string
	IPFamily          qcom.WDSIPFamily
	ProfileIndex      uint8
	RequestTimeout    time.Duration
	MuxDataPort       *qcom.WDSMuxDataPort
	LegacyMuxDataPort qcom.WDSSIOPort
}

// IMSPDNInfo contains IMS PDN addressing and LTE voice service state.
type IMSPDNInfo struct {
	LocalIPv4       net.IP
	LocalIPv6       net.IP
	PCSCFIPs        []net.IP
	IPFamily        qcom.WDSIPFamily
	IMCN            bool
	VoPSKnown       bool
	VoPSSupported   bool
	PacketDataReady bool
}

// IMSPDNSession represents a WDS packet data handle and its QMI clients.
type IMSPDNSession struct {
	reader *Reader
	info   IMSPDNInfo

	timeout          time.Duration
	closeOnce        sync.Once
	closeErr         error
	wdsClientID      uint8
	nasClientID      uint8
	packetDataHandle uint32
}

// OpenIMSPDN starts an IMS PDN using QMI WDS and reads the matching NAS state.
func (r *Reader) OpenIMSPDN(ctx context.Context, cfg IMSPDNConfig) (*IMSPDNSession, error) {
	if r == nil {
		return nil, errors.New("opening IMS PDN: reader is nil")
	}
	if cfg.MuxDataPort != nil && cfg.LegacyMuxDataPort != 0 {
		return nil, errors.New("opening IMS PDN: mux data port and legacy mux data port are mutually exclusive")
	}
	cfg.APN = strings.TrimSpace(cfg.APN)
	if cfg.APN == "" {
		cfg.APN = DefaultIMSPDNAPN
	}
	if cfg.IPFamily == 0 {
		cfg.IPFamily = qcom.WDSIPFamilyIPv4v6
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	session := &IMSPDNSession{
		reader:  r,
		timeout: timeout,
	}

	wdsClientID, err := r.allocateServiceClientID(ctx, qcom.ServiceWDS)
	if err != nil {
		return nil, err
	}
	session.wdsClientID = wdsClientID
	if cfg.MuxDataPort != nil {
		if err := session.bindMuxDataPort(ctx, *cfg.MuxDataPort); err != nil {
			_ = session.Close()
			return nil, err
		}
	} else if cfg.LegacyMuxDataPort != 0 {
		if err := session.bindLegacyMuxDataPort(ctx, cfg.LegacyMuxDataPort); err != nil {
			_ = session.Close()
			return nil, err
		}
	}
	nasClientID, err := r.allocateServiceClientID(ctx, qcom.ServiceNAS)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	session.nasClientID = nasClientID

	if err := session.start(ctx, cfg); err != nil {
		_ = session.Close()
		return nil, err
	}
	info, err := session.readInfo(ctx)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	session.info = info
	return session, nil
}

// Info returns a copy of the IMS PDN runtime state.
func (s *IMSPDNSession) Info() IMSPDNInfo {
	info := s.info
	info.LocalIPv4 = append(net.IP(nil), info.LocalIPv4...)
	info.LocalIPv6 = append(net.IP(nil), info.LocalIPv6...)
	info.PCSCFIPs = cloneIPs(info.PCSCFIPs)
	return info
}

// Close stops the IMS packet data session and releases WDS/NAS QMI clients.
func (s *IMSPDNSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
		defer cancel()

		var err error
		if s.packetDataHandle != 0 && s.wdsClientID != 0 {
			err = errors.Join(err, s.stop(ctx))
		}
		if s.wdsClientID != 0 {
			err = errors.Join(err, s.reader.releaseServiceClientID(ctx, qcom.ServiceWDS, s.wdsClientID))
			s.wdsClientID = 0
		}
		if s.nasClientID != 0 {
			err = errors.Join(err, s.reader.releaseServiceClientID(ctx, qcom.ServiceNAS, s.nasClientID))
			s.nasClientID = 0
		}
		s.closeErr = err
	})
	return s.closeErr
}

func (s *IMSPDNSession) bindMuxDataPort(ctx context.Context, dataPort qcom.WDSMuxDataPort) error {
	req := WDSBindMuxDataPortRequest{
		ClientID: s.wdsClientID,
		Timeout:  s.timeout,
		DataPort: dataPort,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return &qcom.WDSBindMuxDataPortError{Err: err}
	}
	if err := resultOK(resp); err != nil {
		return &qcom.WDSBindMuxDataPortError{Err: err}
	}
	return nil
}

func (s *IMSPDNSession) bindLegacyMuxDataPort(ctx context.Context, dataPort qcom.WDSSIOPort) error {
	req := WDSLegacyBindMuxDataPortRequest{
		ClientID: s.wdsClientID,
		Timeout:  s.timeout,
		DataPort: dataPort,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return fmt.Errorf("binding WDS legacy mux data port: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("binding WDS legacy mux data port: %w", err)
	}
	return nil
}

func (s *IMSPDNSession) start(ctx context.Context, cfg IMSPDNConfig) error {
	req := WDSStartNetworkInterfaceRequest{
		ClientID:             s.wdsClientID,
		Timeout:              s.timeout,
		APN:                  cfg.APN,
		IPFamily:             cfg.IPFamily,
		TechnologyPreference: qcom.WDSTechnologyPreference3GPP,
		ProfileIndex3GPP:     cfg.ProfileIndex,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return err
	}
	var parsed WDSStartNetworkInterfaceResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return err
	}
	if err := resultOK(resp); err != nil {
		return &qcom.WDSStartNetworkError{
			Err:                     err,
			CallEndReason:           parsed.CallEndReason,
			HasCallEndReason:        parsed.HasCallEndReason,
			VerboseCallEndReason:    parsed.VerboseCallEndReason,
			HasVerboseCallEndReason: parsed.HasVerboseCallEndReason,
		}
	}
	s.packetDataHandle = parsed.PacketDataHandle
	return nil
}

func (s *IMSPDNSession) stop(ctx context.Context) error {
	req := WDSStopNetworkInterfaceRequest{
		ClientID:         s.wdsClientID,
		Timeout:          s.timeout,
		PacketDataHandle: s.packetDataHandle,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return err
	}
	if err := resultOK(resp); err != nil {
		return err
	}
	s.packetDataHandle = 0
	return nil
}

func (s *IMSPDNSession) readInfo(ctx context.Context) (IMSPDNInfo, error) {
	runtime, err := s.runtimeSettings(ctx)
	if err != nil {
		return IMSPDNInfo{}, err
	}
	sys, err := s.nasSysInfo(ctx)
	if err != nil {
		return IMSPDNInfo{}, err
	}
	return IMSPDNInfo{
		LocalIPv4:       append(net.IP(nil), runtime.LocalIPv4...),
		LocalIPv6:       append(net.IP(nil), runtime.LocalIPv6...),
		PCSCFIPs:        cloneIPs(runtime.PCSCFIPs),
		IPFamily:        runtime.IPFamily,
		IMCN:            runtime.IMCN,
		VoPSKnown:       sys.VoPSKnown,
		VoPSSupported:   sys.VoPSSupported,
		PacketDataReady: s.packetDataHandle != 0,
	}, nil
}

func (s *IMSPDNSession) runtimeSettings(ctx context.Context) (qcom.WDSRuntimeSettings, error) {
	req := WDSGetRuntimeSettingsRequest{
		ClientID:          s.wdsClientID,
		Timeout:           s.timeout,
		RequestedSettings: qcom.WDSRuntimeRequestedIMSSettings,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return qcom.WDSRuntimeSettings{}, err
	}
	if err := resultOK(resp); err != nil {
		return qcom.WDSRuntimeSettings{}, err
	}
	var parsed WDSGetRuntimeSettingsResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return qcom.WDSRuntimeSettings{}, err
	}
	return parsed.Settings, nil
}

func (s *IMSPDNSession) nasSysInfo(ctx context.Context) (qcom.NASSysInfo, error) {
	req := NASGetSysInfoRequest{
		ClientID: s.nasClientID,
		Timeout:  s.timeout,
	}.Request()
	resp, err := s.reader.requestServiceWithTimeout(
		ctx,
		req.Service,
		req.ClientID,
		req.MessageID,
		req.TLVs,
		req.Timeout,
	)
	if err != nil {
		return qcom.NASSysInfo{}, err
	}
	if err := resultOK(resp); err != nil {
		return qcom.NASSysInfo{}, err
	}
	var parsed NASGetSysInfoResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return qcom.NASSysInfo{}, err
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
