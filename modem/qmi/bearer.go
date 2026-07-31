package qmi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

type qmiSession struct {
	mu        sync.RWMutex
	sessions  []*qcom.PDNSession
	connected []bool
	infoValue BearerInfo
	started   time.Time
	closeOnce sync.Once
	closeErr  error
}

func (b *Backend) Connect(ctx context.Context, cfg ConnectConfig) (sessionBackend, error) {
	interfaceName := cfg.Interface
	if cfg.ProfileID > 255 {
		return nil, fmt.Errorf("connecting QMI data session: profile ID %d exceeds 255", cfg.ProfileID)
	}
	preferences := qmiIPPreferences(cfg.IPFamily)
	sessions := make([]*qcom.PDNSession, 0, len(preferences))
	infos := make([]qcom.PDNInfo, 0, len(preferences))
	connected := make([]bool, 0, len(preferences))
	for _, preference := range preferences {
		pdn, err := b.client.OpenPDN(ctx, qcom.PDNConfig{
			APN:            cfg.APN,
			Authentication: authenticationToQMI(cfg.Authentication),
			Username:       cfg.Username,
			Password:       cfg.Password,
			IPPreference:   preference,
			ProfileIndex:   uint8(cfg.ProfileID),
		})
		if err != nil {
			return nil, errors.Join(
				fmt.Errorf("connecting QMI %s data session: %w", preference, err),
				closeQMISessions(sessions),
			)
		}
		info := pdn.Info()
		sessions = append(sessions, pdn)
		infos = append(infos, info)
		connected = append(connected, info.PacketDataReady)
	}
	return &qmiSession{
		sessions:  sessions,
		connected: connected,
		started:   time.Now(),
		infoValue: BearerInfo{
			Connected: slices.Contains(connected, true),
			ProfileID: cfg.ProfileID,
			APN:       cfg.APN,
			Network:   mergeQMINetworkConfigs(interfaceName, infos),
		},
	}, nil
}

func (s *qmiSession) Info() BearerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := s.infoValue
	result.Network = cloneNetworkConfig(result.Network)
	return result
}

func (s *qmiSession) Stats(ctx context.Context) (BearerStats, error) {
	s.mu.RLock()
	sessions := slices.Clone(s.sessions)
	s.mu.RUnlock()

	result := BearerStats{Duration: time.Since(s.started)}
	var errs []error
	for i, session := range sessions {
		stats, err := session.PacketStatistics(ctx, qcom.WDSStatisticsAll)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading QMI bearer session %d statistics: %w", i, err))
			continue
		}
		result.RXBytes += stats.RxBytes
		result.TXBytes += stats.TxBytes
		result.RXPackets += uint64(stats.RxPackets)
		result.TXPackets += uint64(stats.TxPackets)
	}
	return result, errors.Join(errs...)
}

func (s *qmiSession) Watch(ctx context.Context) (<-chan Result[BearerEvent], error) {
	s.mu.RLock()
	sessions := slices.Clone(s.sessions)
	s.mu.RUnlock()

	watchCtx, cancel := context.WithCancel(ctx)
	streams := make([]<-chan qcom.WDSPacketServiceEvent, 0, len(sessions))
	for i, session := range sessions {
		stream, err := session.WatchStatus(watchCtx)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("watching QMI bearer session %d: %w", i, err)
		}
		streams = append(streams, stream)
	}

	type sessionUpdate struct {
		index int
		value qcom.WDSPacketServiceEvent
	}
	updates := make(chan sessionUpdate, len(streams))
	var forwarders sync.WaitGroup
	forwarders.Add(len(streams))
	for i, stream := range streams {
		go func() {
			defer forwarders.Done()
			for update := range stream {
				select {
				case updates <- sessionUpdate{index: i, value: update}:
				case <-watchCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		forwarders.Wait()
		close(updates)
	}()

	out := make(chan Result[BearerEvent], 1)
	go func() {
		defer close(out)
		defer cancel()
		for update := range updates {
			if update.value.RefreshError != nil {
				sendStreamResult(ctx, out, Result[BearerEvent]{Err: update.value.RefreshError})
				return
			}
			infos := make([]qcom.PDNInfo, len(sessions))
			for i, session := range sessions {
				infos[i] = session.Info()
			}
			s.mu.Lock()
			s.connected[update.index] = update.value.Status.ConnectionStatus == qcom.WDSConnectionStatusConnected
			s.infoValue.Connected = slices.Contains(s.connected, true)
			s.infoValue.Network = mergeQMINetworkConfigs(s.infoValue.Network.Interface, infos)
			s.mu.Unlock()
			if !sendStreamResult(ctx, out, Result[BearerEvent]{Value: BearerEvent{Info: s.Info()}}) {
				return
			}
		}
	}()
	return out, nil
}

func (s *qmiSession) Disconnect(context.Context) error { return s.Close() }

func (s *qmiSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.RLock()
		sessions := slices.Clone(s.sessions)
		s.mu.RUnlock()
		s.closeErr = closeQMISessions(sessions)
		s.mu.Lock()
		s.infoValue.Connected = false
		clear(s.connected)
		s.mu.Unlock()
	})
	return s.closeErr
}

func closeQMISessions(sessions []*qcom.PDNSession) error {
	var errs []error
	for i, session := range sessions {
		if err := session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing QMI bearer session %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func qmiNetworkConfig(interfaceName string, info qcom.PDNInfo) NetworkConfig {
	result := NetworkConfig{Interface: interfaceName, MTU: info.MTU}
	if addr, ok := netip.AddrFromSlice(info.LocalIPv4); ok && addr.Unmap().Is4() {
		addr = addr.Unmap()
		prefix := 32
		if mask := net.IP(info.IPv4SubnetMask).To4(); mask != nil {
			if ones, bits := net.IPMask(mask).Size(); bits == 32 {
				prefix = ones
			}
		}
		result.Addresses = append(result.Addresses, netip.PrefixFrom(addr, prefix))
	}
	if addr, ok := netip.AddrFromSlice(info.LocalIPv6); ok && addr.Is6() {
		result.Addresses = append(result.Addresses, netip.PrefixFrom(addr, int(info.IPv6PrefixLength)))
	}
	for _, gateway := range []net.IP{info.IPv4Gateway, info.IPv6Gateway} {
		if addr, ok := netip.AddrFromSlice(gateway); ok {
			result.Gateways = append(result.Gateways, addr.Unmap())
		}
	}
	for _, dns := range info.DNS {
		if addr, ok := netip.AddrFromSlice(dns); ok {
			result.DNS = append(result.DNS, addr.Unmap())
		}
	}
	return result
}

func mergeQMINetworkConfigs(interfaceName string, infos []qcom.PDNInfo) NetworkConfig {
	result := NetworkConfig{Interface: interfaceName}
	for _, info := range infos {
		config := qmiNetworkConfig(interfaceName, info)
		result.Addresses = appendUnique(result.Addresses, config.Addresses...)
		result.Gateways = appendUnique(result.Gateways, config.Gateways...)
		result.DNS = appendUnique(result.DNS, config.DNS...)
		if config.MTU != 0 && (result.MTU == 0 || config.MTU < result.MTU) {
			result.MTU = config.MTU
		}
	}
	return result
}

func appendUnique[T comparable](values []T, candidates ...T) []T {
	for _, candidate := range candidates {
		if !slices.Contains(values, candidate) {
			values = append(values, candidate)
		}
	}
	return values
}

func cloneNetworkConfig(config NetworkConfig) NetworkConfig {
	config.Addresses = slices.Clone(config.Addresses)
	config.Gateways = slices.Clone(config.Gateways)
	config.DNS = slices.Clone(config.DNS)
	return config
}

func qmiIPPreferences(family IPFamily) []qcom.WDSIPPreference {
	switch family {
	case IPFamilyIPv4:
		return []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4}
	case IPFamilyIPv6:
		return []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv6}
	case IPFamilyIPv4v6:
		return []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4, qcom.WDSIPPreferenceIPv6}
	default:
		return []qcom.WDSIPPreference{qcom.WDSIPPreferenceDefault}
	}
}

func sendStreamResult[T any](ctx context.Context, out chan<- Result[T], result Result[T]) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case out <- result:
		return true
	case <-ctx.Done():
		return false
	}
}
