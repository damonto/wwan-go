package mbim

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/contract"
)

const closeTimeout = 5 * time.Second

type session struct {
	mu        sync.RWMutex
	backend   *Backend
	id        uint32
	infoValue BearerInfo
	started   time.Time
	closed    bool
}

func (b *Backend) Connect(ctx context.Context, cfg ConnectConfig) (sessionBackend, error) {
	return b.ConnectPort(ctx, cfg, Port{Type: PortNetwork, Name: cfg.Interface})
}

// ConnectPort starts an MBIM data session on the selected network port.
func (b *Backend) ConnectPort(ctx context.Context, cfg ConnectConfig, _ Port) (sessionBackend, error) {
	interfaceName := cfg.Interface
	if cfg.ProfileID != 0 {
		profile, err := b.profile(ctx, cfg.ProfileID)
		if err != nil {
			return nil, err
		}
		cfg.APN = profile.APN
		cfg.Authentication = profile.Authentication
		cfg.Username = profile.Username
		cfg.Password = profile.Password
		if cfg.IPFamily == IPFamilyUnknown {
			cfg.IPFamily = profile.IPFamily
		}
	}
	sessionID, err := b.reserveSession(ctx, cfg.SessionID)
	if err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			b.releaseSession(sessionID)
		}
	}()

	connect, err := b.client.SetConnect(ctx, mbimproto.ConnectConfig{
		SessionID:         mbimproto.SessionID(sessionID),
		ActivationCommand: mbimproto.ActivationCommandActivate,
		AccessString:      cfg.APN,
		UserName:          cfg.Username,
		Password:          cfg.Password,
		AuthProtocol:      authProtocol(cfg.Authentication),
		IPType:            contextIPType(cfg.IPFamily),
		ContextType:       mbimproto.ContextTypeInternet,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting MBIM data session %d: %w", sessionID, err)
	}
	if connect.ActivationState != mbimproto.ActivationStateActivated {
		return nil, fmt.Errorf("connecting MBIM data session %d: activation state is %d", sessionID, connect.ActivationState)
	}
	ip, err := b.client.IPConfiguration(ctx, mbimproto.SessionID(sessionID))
	if err != nil {
		_, disconnectErr := b.client.SetConnect(ctx, mbimproto.ConnectConfig{
			SessionID:         mbimproto.SessionID(sessionID),
			ActivationCommand: mbimproto.ActivationCommandDeactivate,
			ContextType:       mbimproto.ContextTypeInternet,
		})
		return nil, errors.Join(fmt.Errorf("reading MBIM session IP configuration: %w", err), disconnectErr)
	}
	release = false
	return &session{
		backend: b,
		id:      sessionID,
		started: time.Now(),
		infoValue: BearerInfo{
			Connected: true,
			ProfileID: cfg.ProfileID,
			APN:       cfg.APN,
			Network:   networkConfig(interfaceName, ip),
		},
	}, nil
}

func (b *Backend) profile(ctx context.Context, id int32) (Profile, error) {
	profiles, err := b.Profiles(ctx)
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("connecting MBIM data session: profile %d not found", id)
}

func (b *Backend) reserveSession(ctx context.Context, requested *uint32) (uint32, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return 0, fmt.Errorf("allocating MBIM session: %w", err)
	}
	maximum := caps.MaxSessions
	if maximum == 0 {
		maximum = 1
	}
	if err := b.prepareSessions(ctx, maximum); err != nil {
		return 0, err
	}
	return b.reserveSessionID(maximum, requested)
}

type sessionControl interface {
	QueryConnect(context.Context, mbimproto.SessionID) (mbimproto.ConnectInfo, error)
	SetConnect(context.Context, mbimproto.ConnectConfig) (mbimproto.ConnectInfo, error)
}

func (b *Backend) prepareSessions(ctx context.Context, maximum uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sessionsPrepared {
		return nil
	}
	if err := cleanupStaleSessions(ctx, b.client, maximum); err != nil {
		return fmt.Errorf("preparing MBIM data sessions: %w", err)
	}
	b.sessionsPrepared = true
	return nil
}

func cleanupStaleSessions(ctx context.Context, client sessionControl, maximum uint32) error {
	for sessionID := range maximum {
		info, err := client.QueryConnect(ctx, mbimproto.SessionID(sessionID))
		if errors.Is(err, mbimproto.StatusContextNotActivated) {
			continue
		}
		if err != nil {
			return fmt.Errorf("querying MBIM session %d before first connection: %w", sessionID, err)
		}
		if info.ActivationState == mbimproto.ActivationStateUnknown ||
			info.ActivationState == mbimproto.ActivationStateDeactivated {
			continue
		}
		_, err = client.SetConnect(ctx, mbimproto.ConnectConfig{
			SessionID:         mbimproto.SessionID(sessionID),
			ActivationCommand: mbimproto.ActivationCommandDeactivate,
			ContextType:       mbimproto.ContextTypeInternet,
		})
		if errors.Is(err, mbimproto.StatusContextNotActivated) {
			continue
		}
		if err != nil {
			return fmt.Errorf("disconnecting stale MBIM session %d: %w", sessionID, err)
		}
	}
	return nil
}

func (b *Backend) reserveSessionID(maximum uint32, requested *uint32) (uint32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.slots == nil {
		b.slots = make(map[uint32]struct{})
	}
	if requested != nil {
		if *requested >= maximum {
			return 0, fmt.Errorf("allocating MBIM session: ID %d exceeds maximum %d", *requested, maximum-1)
		}
		if _, exists := b.slots[*requested]; exists {
			return 0, fmt.Errorf("allocating MBIM session: ID %d is already in use", *requested)
		}
		b.slots[*requested] = struct{}{}
		return *requested, nil
	}
	for id := range maximum {
		if _, exists := b.slots[id]; exists {
			continue
		}
		b.slots[id] = struct{}{}
		return id, nil
	}
	return 0, fmt.Errorf("allocating MBIM session: all %d session IDs are in use", maximum)
}

func (b *Backend) releaseSession(id uint32) {
	b.mu.Lock()
	delete(b.slots, id)
	b.mu.Unlock()
}

func (s *session) Info() BearerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := s.infoValue
	result.Network = cloneNetworkConfig(result.Network)
	return result
}

func (s *session) Stats(ctx context.Context) (BearerStats, error) {
	stats, err := s.backend.client.PacketStatistics(ctx)
	if err != nil {
		return BearerStats{}, fmt.Errorf("reading MBIM bearer statistics: %w", err)
	}
	return BearerStats{
		RXBytes:   stats.InOctets,
		TXBytes:   stats.OutOctets,
		RXPackets: stats.InPackets,
		TXPackets: stats.OutPackets,
		Duration:  time.Since(s.started),
	}, nil
}

func (s *session) Watch(ctx context.Context) (<-chan Result[BearerEvent], error) {
	return contract.PollStream(ctx, 2*time.Second, func(ctx context.Context) (BearerEvent, error) {
		connect, err := s.backend.client.QueryConnect(ctx, mbimproto.SessionID(s.id))
		if err != nil {
			return BearerEvent{}, fmt.Errorf("reading MBIM bearer: %w", err)
		}
		s.mu.Lock()
		s.infoValue.Connected = connect.ActivationState == mbimproto.ActivationStateActivated
		connected := s.infoValue.Connected
		interfaceName := s.infoValue.Network.Interface
		s.mu.Unlock()
		if connected {
			ip, err := s.backend.client.IPConfiguration(ctx, mbimproto.SessionID(s.id))
			if err != nil {
				return BearerEvent{}, fmt.Errorf("reading MBIM bearer IP configuration: %w", err)
			}
			s.mu.Lock()
			s.infoValue.Network = networkConfig(interfaceName, ip)
			s.mu.Unlock()
		}
		return BearerEvent{Info: s.Info()}, nil
	}), nil
}

func (s *session) Disconnect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	_, err := s.backend.client.SetConnect(ctx, mbimproto.ConnectConfig{
		SessionID:         mbimproto.SessionID(s.id),
		ActivationCommand: mbimproto.ActivationCommandDeactivate,
		ContextType:       mbimproto.ContextTypeInternet,
	})
	if err != nil {
		return fmt.Errorf("disconnecting MBIM session %d: %w", s.id, err)
	}
	s.closed = true
	s.infoValue.Connected = false
	s.backend.releaseSession(s.id)
	return nil
}

func (s *session) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return s.Disconnect(ctx)
}

func networkConfig(interfaceName string, info mbimproto.IPConfigurationInfo) NetworkConfig {
	result := NetworkConfig{Interface: interfaceName, MTU: max(info.IPv4MTU, info.IPv6MTU)}
	addresses := append(slices.Clone(info.IPv4Addresses), info.IPv6Addresses...)
	for _, address := range addresses {
		if addr, ok := netip.AddrFromSlice(address.IP); ok {
			result.Addresses = append(result.Addresses, netip.PrefixFrom(addr.Unmap(), int(address.PrefixLength)))
		}
	}
	for _, gateway := range []net.IP{info.IPv4Gateway, info.IPv6Gateway} {
		if addr, ok := netip.AddrFromSlice(gateway); ok {
			result.Gateways = append(result.Gateways, addr.Unmap())
		}
	}
	dnsServers := append(slices.Clone(info.IPv4DNSServers), info.IPv6DNSServers...)
	for _, dns := range dnsServers {
		if addr, ok := netip.AddrFromSlice(dns); ok {
			result.DNS = append(result.DNS, addr.Unmap())
		}
	}
	return result
}

func cloneNetworkConfig(config NetworkConfig) NetworkConfig {
	config.Addresses = slices.Clone(config.Addresses)
	config.Gateways = slices.Clone(config.Gateways)
	config.DNS = slices.Clone(config.DNS)
	return config
}

func contextIPType(family IPFamily) mbimproto.ContextIPType {
	switch family {
	case IPFamilyIPv4:
		return mbimproto.ContextIPTypeIPv4
	case IPFamilyIPv6:
		return mbimproto.ContextIPTypeIPv6
	case IPFamilyIPv4v6:
		return mbimproto.ContextIPTypeIPv4v6
	default:
		return mbimproto.ContextIPTypeDefault
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
