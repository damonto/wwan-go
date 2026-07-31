package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDSConnectionStatus is the current state of a WDS packet-data connection.
type WDSConnectionStatus uint8

const (
	WDSConnectionStatusDisconnected   WDSConnectionStatus = 0x01
	WDSConnectionStatusConnected      WDSConnectionStatus = 0x02
	WDSConnectionStatusSuspended      WDSConnectionStatus = 0x03
	WDSConnectionStatusAuthenticating WDSConnectionStatus = 0x04
)

// WDSIPFamilyPreference is the IP family reported for a packet-data call.
type WDSIPFamilyPreference uint8

const (
	WDSIPFamilyPreferenceNonIP       WDSIPFamilyPreference = 0x02
	WDSIPFamilyPreferenceIPv4        WDSIPFamilyPreference = 0x04
	WDSIPFamilyPreferenceIPv6        WDSIPFamilyPreference = 0x06
	WDSIPFamilyPreferenceUnspecified WDSIPFamilyPreference = 0x08
)

// WDSTechnologyName identifies the logical WDS technology carrying a call.
type WDSTechnologyName int16

const (
	WDSTechnologyNameCDMA              WDSTechnologyName = -32767
	WDSTechnologyNameUMTS              WDSTechnologyName = -32764
	WDSTechnologyNameWLANLocalBreakout WDSTechnologyName = -32736
	WDSTechnologyNameIWLANS2B          WDSTechnologyName = -32735
	WDSTechnologyNameEPC               WDSTechnologyName = -30592
	WDSTechnologyNameEMBMS             WDSTechnologyName = -30590
	WDSTechnologyNameNonIP             WDSTechnologyName = -30588
	WDSTechnologyNameModemLinkLocal    WDSTechnologyName = -30584
)

// WDSPacketServiceStatus contains connection state and optional addressing
// information returned by Get Packet Service Status or its indication.
type WDSPacketServiceStatus struct {
	ConnectionStatus          WDSConnectionStatus
	ReconfigurationRequired   bool
	IPFamily                  WDSIPFamilyPreference
	IPFamilyKnown             bool
	Technology                WDSTechnologyName
	TechnologyKnown           bool
	BearerID                  uint8
	BearerIDKnown             bool
	XLATCapable               bool
	XLATCapableKnown          bool
	ChangedIPConfiguration    WDSRuntimeSettingsMask
	CallEndReason             WDSCallEndReason
	CallEndReasonKnown        bool
	VerboseCallEndReason      WDSVerboseCallEndReason
	VerboseCallEndReasonKnown bool
	Runtime                   WDSRuntimeSettings
}

// WDSPacketServiceEvent is one connection update. RefreshError reports a
// follow-up status/runtime-settings query error without hiding the indication.
type WDSPacketServiceEvent struct {
	Status       WDSPacketServiceStatus
	RefreshError error
}

type WDSGetPacketServiceStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	IPConfigMask  WDSRuntimeSettingsMask
}

func (r WDSGetPacketServiceStatusRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.IPConfigMask != 0 {
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(r.IPConfigMask)))
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetPacketServiceStatus,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

type WDSIndicationRegisterConfig struct {
	SuppressPacketService *bool
	ExtendedIPConfig      *bool
	ProfileChanges        *bool
}

type WDSIndicationRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDSIndicationRegisterConfig
}

func (r WDSIndicationRegisterRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config.SuppressPacketService != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*r.Config.SuppressPacketService)))
	}
	if r.Config.ExtendedIPConfig != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*r.Config.ExtendedIPConfig)))
	}
	if r.Config.ProfileChanges != nil {
		tlvs = append(tlvs, tlv.Uint(0x19, boolByte(*r.Config.ProfileChanges)))
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSIndicationRegister,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

type wdsGetPacketServiceStatusResponse WDSPacketServiceStatus

func (r *wdsGetPacketServiceStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = wdsGetPacketServiceStatusResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS packet service status: connection status TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS packet service status: connection status TLV length %d, want 1", len(value))
	}
	status := WDSPacketServiceStatus{ConnectionStatus: WDSConnectionStatus(value[0])}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS packet service status: IP family TLV length %d, want 1", len(value))
		}
		status.IPFamily = WDSIPFamilyPreference(value[0])
		status.IPFamilyKnown = true
		if status.IPFamily == WDSIPFamilyPreferenceIPv4 || status.IPFamily == WDSIPFamilyPreferenceIPv6 {
			status.Runtime.IPFamily = WDSIPFamily(status.IPFamily)
		}
	}
	if err := status.Runtime.unmarshalPacketTLVs(tlvs, wdsPacketRuntimeResponseTLVs); err != nil {
		return err
	}
	*r = wdsGetPacketServiceStatusResponse(status)
	return nil
}

type wdsPacketServiceIndication WDSPacketServiceStatus

func (i *wdsPacketServiceIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = wdsPacketServiceIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS packet service indication: status TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI WDS packet service indication: status TLV length %d, want 2", len(value))
	}
	status := WDSPacketServiceStatus{
		ConnectionStatus:        WDSConnectionStatus(value[0]),
		ReconfigurationRequired: value[1] == 1,
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS packet service indication: call end reason TLV length %d, want 2", len(value))
		}
		status.CallEndReason = WDSCallEndReason(binary.LittleEndian.Uint16(value[:2]))
		status.CallEndReasonKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service indication: verbose call end reason TLV length %d, want 4", len(value))
		}
		status.VerboseCallEndReason = WDSVerboseCallEndReason{
			Type:   WDSVerboseCallEndReasonType(binary.LittleEndian.Uint16(value[:2])),
			Reason: int16(binary.LittleEndian.Uint16(value[2:4])),
		}
		status.VerboseCallEndReasonKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS packet service indication: IP family TLV length %d, want 1", len(value))
		}
		status.IPFamily = WDSIPFamilyPreference(value[0])
		status.IPFamilyKnown = true
		if status.IPFamily == WDSIPFamilyPreferenceIPv4 || status.IPFamily == WDSIPFamilyPreferenceIPv6 {
			status.Runtime.IPFamily = WDSIPFamily(status.IPFamily)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS packet service indication: technology name TLV length %d, want 2", len(value))
		}
		status.Technology = WDSTechnologyName(int16(binary.LittleEndian.Uint16(value[:2])))
		status.TechnologyKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS packet service indication: bearer ID TLV length %d, want 1", len(value))
		}
		status.BearerID = value[0]
		status.BearerIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS packet service indication: XLAT capability TLV length %d, want 1", len(value))
		}
		status.XLATCapable = value[0] == 1
		status.XLATCapableKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service indication: changed IP configuration TLV length %d, want 4", len(value))
		}
		status.ChangedIPConfiguration = WDSRuntimeSettingsMask(binary.LittleEndian.Uint32(value[:4]))
	}
	if err := status.Runtime.unmarshalPacketTLVs(tlvs, wdsPacketRuntimeIndicationTLVs); err != nil {
		return err
	}
	*i = wdsPacketServiceIndication(status)
	return nil
}

type wdsPacketRuntimeTLVs struct {
	ipv4       byte
	ipv6       byte
	gateway4   byte
	netmask4   byte
	gateway6   byte
	dns4First  byte
	dns4Second byte
	dns6First  byte
	dns6Second byte
	mtu        byte
}

var (
	wdsPacketRuntimeResponseTLVs   = wdsPacketRuntimeTLVs{ipv4: 0x11, ipv6: 0x12, gateway4: 0x13, netmask4: 0x14, gateway6: 0x15, dns4First: 0x16, dns4Second: 0x17, dns6First: 0x18, dns6Second: 0x19, mtu: 0x1A}
	wdsPacketRuntimeIndicationTLVs = wdsPacketRuntimeTLVs{ipv4: 0x17, ipv6: 0x18, gateway4: 0x19, netmask4: 0x1A, gateway6: 0x1B, dns4First: 0x1C, dns4Second: 0x1D, dns6First: 0x1E, dns6Second: 0x1F, mtu: 0x20}
)

func (runtime *WDSRuntimeSettings) unmarshalPacketTLVs(tlvs tlv.TLVs, kinds wdsPacketRuntimeTLVs) error {
	if value, ok := tlv.Value(tlvs, kinds.ipv4); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service status: IPv4 address TLV length %d, want 4", len(value))
		}
		runtime.LocalIPv4 = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, kinds.ipv6); ok {
		if len(value) != 17 {
			return fmt.Errorf("parsing QMI WDS packet service status: IPv6 address TLV length %d, want 17", len(value))
		}
		runtime.LocalIPv6 = slices.Clone(value[:16])
		runtime.IPv6PrefixLength = value[16]
	}
	if value, ok := tlv.Value(tlvs, kinds.gateway4); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service status: IPv4 gateway TLV length %d, want 4", len(value))
		}
		runtime.IPv4Gateway = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, kinds.netmask4); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service status: IPv4 netmask TLV length %d, want 4", len(value))
		}
		runtime.IPv4SubnetMask = qmiIPv4(value)
	}
	if value, ok := tlv.Value(tlvs, kinds.gateway6); ok {
		if len(value) != 17 {
			return fmt.Errorf("parsing QMI WDS packet service status: IPv6 gateway TLV length %d, want 17", len(value))
		}
		runtime.IPv6Gateway = slices.Clone(value[:16])
	}
	for _, kind := range []byte{kinds.dns4First, kinds.dns4Second} {
		if value, ok := tlv.Value(tlvs, kind); ok {
			if len(value) != 4 {
				return fmt.Errorf("parsing QMI WDS packet service status: IPv4 DNS address TLV length %d, want 4", len(value))
			}
			runtime.DNS = append(runtime.DNS, qmiIPv4(value))
		}
	}
	for _, kind := range []byte{kinds.dns6First, kinds.dns6Second} {
		if value, ok := tlv.Value(tlvs, kind); ok {
			if len(value) != net.IPv6len {
				return fmt.Errorf("parsing QMI WDS packet service status: IPv6 DNS address TLV length %d, want %d", len(value), net.IPv6len)
			}
			runtime.DNS = append(runtime.DNS, slices.Clone(value[:net.IPv6len]))
		}
	}
	if value, ok := tlv.Value(tlvs, kinds.mtu); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet service status: MTU TLV length %d, want 4", len(value))
		}
		runtime.MTU = binary.LittleEndian.Uint32(value[:4])
	}
	runtime.DNS = uniqueWDSIPs(runtime.DNS)
	return nil
}

type wdsExtendedIPConfigIndication struct {
	Changed WDSRuntimeSettingsMask
}

func (i *wdsExtendedIPConfigIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = wdsExtendedIPConfigIndication{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI WDS extended IP configuration: changed mask TLV length %d, want 4", len(value))
	}
	i.Changed = WDSRuntimeSettingsMask(binary.LittleEndian.Uint32(value[:4]))
	return nil
}

// WDSPacketServiceStatus reads the status associated with the Client's WDS
// control point. Stateful PDN users should prefer PDNSession.Status.
func (c *Client) WDSPacketServiceStatus(ctx context.Context, mask WDSRuntimeSettingsMask) (WDSPacketServiceStatus, error) {
	var status WDSPacketServiceStatus
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		var err error
		status, err = c.wdsPacketServiceStatus(ctx, clientID, mask, DefaultRequestTimeout)
		return err
	})
	if err != nil {
		return WDSPacketServiceStatus{}, fmt.Errorf("reading QMI WDS packet service status: %w", err)
	}
	return status, nil
}

func (c *Client) wdsPacketServiceStatus(ctx context.Context, clientID uint8, mask WDSRuntimeSettingsMask, timeout time.Duration) (WDSPacketServiceStatus, error) {
	req := WDSGetPacketServiceStatusRequest{ClientID: clientID, Timeout: timeout, IPConfigMask: mask}.Request()
	resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return WDSPacketServiceStatus{}, err
	}
	if err := resultOK(resp); err != nil {
		return WDSPacketServiceStatus{}, err
	}
	var parsed wdsGetPacketServiceStatusResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSPacketServiceStatus{}, err
	}
	return WDSPacketServiceStatus(parsed), nil
}

// Status reads the current state and refreshes negotiated runtime settings for
// a connected PDN.
func (s *PDNSession) Status(ctx context.Context) (WDSPacketServiceStatus, error) {
	if s == nil {
		return WDSPacketServiceStatus{}, errors.New("reading QMI WDS packet service status: session is nil")
	}
	s.mu.RLock()
	clientID := s.wdsClientID
	ready := s.wdsClientReady
	timeout := s.timeout
	requested := s.requestedSettings
	s.mu.RUnlock()
	if !ready {
		return WDSPacketServiceStatus{}, errors.New("reading QMI WDS packet service status: session is closed")
	}

	status, err := s.client.wdsPacketServiceStatus(ctx, clientID, 0, timeout)
	if err != nil {
		return WDSPacketServiceStatus{}, fmt.Errorf("reading QMI WDS packet service status: %w", err)
	}
	if status.ConnectionStatus == WDSConnectionStatusConnected {
		runtime, err := s.runtimeSettings(ctx, requested)
		if err != nil {
			return status, fmt.Errorf("refreshing QMI WDS runtime settings: %w", err)
		}
		status.Runtime = runtime
	}
	s.applyPacketServiceStatus(status)
	return cloneWDSPacketServiceStatus(status), nil
}

// WatchStatus emits an initial snapshot followed by packet-service and
// extended-IP-configuration updates for this PDN.
func (s *PDNSession) WatchStatus(ctx context.Context) (<-chan WDSPacketServiceEvent, error) {
	if s == nil {
		return nil, errors.New("watching QMI WDS packet service status: session is nil")
	}
	transport, err := s.client.indicationTransport()
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	clientID := s.wdsClientID
	ready := s.wdsClientReady
	s.mu.RUnlock()
	if !ready {
		return nil, errors.New("watching QMI WDS packet service status: session is closed")
	}

	watchCtx, cancel := context.WithCancel(ctx)
	go cancelPDNWatch(watchCtx, s.done, cancel)
	statuses, err := transport.Indications(watchCtx, ServiceWDS, clientID, MessageWDSGetPacketServiceStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI WDS packet service status: %w", err)
	}
	extended, err := transport.Indications(watchCtx, ServiceWDS, clientID, MessageWDSExtendedIPConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI WDS extended IP configuration: %w", err)
	}
	if err := s.acquireStatusIndications(ctx); err != nil {
		cancel()
		return nil, err
	}
	initial, err := s.Status(ctx)
	if err != nil {
		s.releaseStatusIndications()
		cancel()
		return nil, err
	}

	out := make(chan WDSPacketServiceEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer s.releaseStatusIndications()
		if !sendWDSPacketServiceEvent(watchCtx, out, WDSPacketServiceEvent{Status: initial}) {
			return
		}

		for statuses != nil || extended != nil {
			var event WDSPacketServiceEvent
			select {
			case ind, ok := <-statuses:
				if !ok {
					statuses = nil
					continue
				}
				var parsed wdsPacketServiceIndication
				if err := parsed.UnmarshalTLVs(ind.TLVs); err != nil {
					return
				}
				status := WDSPacketServiceStatus(parsed)
				if status.ConnectionStatus == WDSConnectionStatusConnected &&
					(!s.packetDataReady() || status.ReconfigurationRequired || status.ChangedIPConfiguration != 0) {
					refreshed, refreshErr := s.Status(watchCtx)
					if refreshErr == nil {
						refreshed.ReconfigurationRequired = status.ReconfigurationRequired
						refreshed.ChangedIPConfiguration = status.ChangedIPConfiguration
						status = refreshed
					}
					event.RefreshError = refreshErr
				} else {
					s.applyPacketServiceStatus(status)
					status.Runtime = s.runtimeSnapshot()
				}
				event.Status = cloneWDSPacketServiceStatus(status)
			case ind, ok := <-extended:
				if !ok {
					extended = nil
					continue
				}
				var parsed wdsExtendedIPConfigIndication
				if err := parsed.UnmarshalTLVs(ind.TLVs); err != nil {
					return
				}
				status, refreshErr := s.Status(watchCtx)
				status.ReconfigurationRequired = true
				status.ChangedIPConfiguration = parsed.Changed
				event = WDSPacketServiceEvent{Status: status, RefreshError: refreshErr}
			case <-watchCtx.Done():
				return
			}
			if !sendWDSPacketServiceEvent(watchCtx, out, event) {
				return
			}
		}
	}()
	return out, nil
}

func (s *PDNSession) acquireStatusIndications(ctx context.Context) error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.statusWatchers > 0 {
		s.statusWatchers++
		return nil
	}
	if err := s.setStatusIndications(ctx, true); err != nil {
		return err
	}
	s.statusWatchers = 1
	return nil
}

func (s *PDNSession) releaseStatusIndications() {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.statusWatchers == 0 {
		return
	}
	s.statusWatchers--
	if s.statusWatchers == 0 {
		_ = s.setStatusIndications(ctx, false)
	}
}

func (s *PDNSession) setStatusIndications(ctx context.Context, enabled bool) error {
	s.mu.RLock()
	clientID := s.wdsClientID
	ready := s.wdsClientReady
	timeout := s.timeout
	s.mu.RUnlock()
	if !ready {
		return errors.New("configuring QMI WDS packet service indications: session is closed")
	}
	suppress := !enabled
	req := WDSIndicationRegisterRequest{
		ClientID: clientID,
		Timeout:  timeout,
		Config: WDSIndicationRegisterConfig{
			SuppressPacketService: &suppress,
			ExtendedIPConfig:      &enabled,
		},
	}.Request()
	resp, err := s.client.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return fmt.Errorf("configuring QMI WDS packet service indications: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("configuring QMI WDS packet service indications: %w", err)
	}
	return nil
}

func (s *PDNSession) applyPacketServiceStatus(status WDSPacketServiceStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wdsClientReady {
		return
	}
	s.connectionStatus = status.ConnectionStatus
	if status.ConnectionStatus != WDSConnectionStatusConnected {
		s.runtime = WDSRuntimeSettings{}
		s.info = PDNInfo{}
		return
	}
	if hasWDSRuntimeSettings(status.Runtime) {
		s.runtime = cloneWDSRuntimeSettings(status.Runtime)
	}
	s.info = pdnInfo(s.runtime, true)
}

func (s *PDNSession) packetDataReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info.PacketDataReady
}

func (s *PDNSession) runtimeSnapshot() WDSRuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneWDSRuntimeSettings(s.runtime)
}

func cancelPDNWatch(ctx context.Context, done <-chan struct{}, cancel context.CancelFunc) {
	select {
	case <-done:
		cancel()
	case <-ctx.Done():
	}
}

func hasWDSRuntimeSettings(settings WDSRuntimeSettings) bool {
	return len(settings.LocalIPv4) > 0 || len(settings.LocalIPv6) > 0 ||
		len(settings.IPv4Gateway) > 0 || len(settings.IPv6Gateway) > 0 ||
		len(settings.DNS) > 0 || settings.MTU != 0 || len(settings.PCSCFIPs) > 0 ||
		settings.IPFamily != 0 || settings.IMCN
}

func cloneWDSRuntimeSettings(settings WDSRuntimeSettings) WDSRuntimeSettings {
	settings.LocalIPv4 = slices.Clone(settings.LocalIPv4)
	settings.LocalIPv6 = slices.Clone(settings.LocalIPv6)
	settings.IPv4Gateway = slices.Clone(settings.IPv4Gateway)
	settings.IPv4SubnetMask = slices.Clone(settings.IPv4SubnetMask)
	settings.IPv6Gateway = slices.Clone(settings.IPv6Gateway)
	settings.DNS = cloneIPs(settings.DNS)
	settings.PCSCFIPs = cloneIPs(settings.PCSCFIPs)
	return settings
}

func cloneWDSPacketServiceStatus(status WDSPacketServiceStatus) WDSPacketServiceStatus {
	status.Runtime = cloneWDSRuntimeSettings(status.Runtime)
	return status
}

func sendWDSPacketServiceEvent(ctx context.Context, out chan<- WDSPacketServiceEvent, event WDSPacketServiceEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
