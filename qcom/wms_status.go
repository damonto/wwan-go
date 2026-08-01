package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WMSIndicationConfig selects WMS status indications. Nil fields are omitted,
// allowing callers to change only the registrations they own.
type WMSIndicationConfig struct {
	TransportLayer   *bool
	TransportNetwork *bool
	CallStatus       *bool
	ServiceReady     *bool
	BroadcastConfig  *bool
	TransportMWI     *bool
	SMSCAddress      *bool
	MemoryFull       *bool
}

// WMSTransportType identifies the SMS transport implementation.
type WMSTransportType uint8

const (
	WMSTransportIMS WMSTransportType = iota
)

// WMSTransportCapability identifies the SMS family supported by a transport.
type WMSTransportCapability uint8

const (
	WMSTransportCapabilityCDMA WMSTransportCapability = iota
	WMSTransportCapabilityGW
)

// WMSTransportLayerState contains optional IMS transport registration and
// capability information.
type WMSTransportLayerState struct {
	Registered      bool
	RegisteredKnown bool
	Type            WMSTransportType
	Capability      WMSTransportCapability
	InfoKnown       bool
}

// WMSReadyStatus identifies which SMS families are ready.
type WMSReadyStatus uint32

const (
	WMSReadyNone WMSReadyStatus = iota
	WMSReady3GPP
	WMSReady3GPP2
	WMSReady3GPPAnd3GPP2
	WMSReady3GPPLimited
	WMSReady3GPP2Limited
	WMSReady3GPPLimitedAnd3GPP2Limited
	WMSReady3GPPLimitedAnd3GPP2
	WMSReady3GPPAnd3GPP2Limited
)

// WMSSIMReadyStatus is a bitmask of SIM-backed messaging families that are
// ready.
type WMSSIMReadyStatus uint64

const (
	WMSSIMReady3GPP  WMSSIMReadyStatus = 1 << 0
	WMSSIMReady3GPP2 WMSSIMReadyStatus = 1 << 1
)

// WMSServiceReadyState contains service-ready registration and status fields.
type WMSServiceReadyState struct {
	EventsRegistered      bool
	EventsRegisteredKnown bool
	Status                WMSReadyStatus
	StatusKnown           bool
	SIMEventsRegistered   bool
	SIMEventsKnown        bool
	SIMStatus             WMSSIMReadyStatus
	SIMStatusKnown        bool
}

// WMSMemoryFullState identifies the store and message family that is full.
type WMSMemoryFullState struct {
	Storage     WMSStorage
	MessageMode WMSMessageMode
}

// WMSCallStatus identifies the current SMS call state.
type WMSCallStatus uint8

const (
	WMSCallIncoming WMSCallStatus = iota
	WMSCallConnected
	WMSCallAborted
	WMSCallDisconnected
	WMSCallConnecting
)

// WMSTransportNetworkStatus describes SMS transport network registration.
type WMSTransportNetworkStatus uint8

const (
	WMSTransportNetworkNoService WMSTransportNetworkStatus = iota
	WMSTransportNetworkInProgress
	WMSTransportNetworkFailed
	WMSTransportNetworkLimitedService
	WMSTransportNetworkFullService
)

// WMSTransportNetworkState contains an optional network registration status.
type WMSTransportNetworkState struct {
	Status WMSTransportNetworkStatus
	Known  bool
}

// UnmarshalTLVs parses a transport-network indication.
func (s *WMSTransportNetworkState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, true)
}

// UnmarshalTLVs parses a transport-layer indication.
func (s *WMSTransportLayerState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, true)
}

// UnmarshalTLVs parses a service-ready indication.
func (s *WMSServiceReadyState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, true)
}

// UnmarshalTLVs parses a memory-full indication.
func (s *WMSMemoryFullState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs)
}

// UnmarshalTLVs parses a call-status indication.
func (s *WMSCallStatus) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs)
}

// UnmarshalTLVs parses an SMSC-address indication.
func (a *WMSSMSCAddress) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return a.unmarshalTLVs(tlvs)
}

// WMSSetIndicationRegistration changes general WMS indication subscriptions.
func (c *Client) WMSSetIndicationRegistration(ctx context.Context, config WMSIndicationConfig) error {
	var tlvs tlv.TLVs
	if config.TransportLayer != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*config.TransportLayer)))
	}
	if config.TransportNetwork != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*config.TransportNetwork)))
	}
	if config.CallStatus != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*config.CallStatus)))
	}
	if config.ServiceReady != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, boolByte(*config.ServiceReady)))
	}
	if config.BroadcastConfig != nil {
		tlvs = append(tlvs, tlv.Uint(0x14, boolByte(*config.BroadcastConfig)))
	}
	if config.TransportMWI != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, boolByte(*config.TransportMWI)))
	}
	if config.SMSCAddress != nil {
		tlvs = append(tlvs, tlv.Uint(0x17, boolByte(*config.SMSCAddress)))
	}
	if config.MemoryFull != nil {
		tlvs = append(tlvs, tlv.Uint(0x18, boolByte(*config.MemoryFull)))
	}
	if err := c.wmsResultRequest(ctx, MessageWMSIndicationRegister, tlvs); err != nil {
		return fmt.Errorf("setting QMI WMS indication registration: %w", err)
	}
	return nil
}

// WMSTransportLayer reads IMS transport registration and capability.
func (c *Client) WMSTransportLayer(ctx context.Context) (WMSTransportLayerState, error) {
	var parsed wmsTransportLayerResponse
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetTransportLayer, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return parsed.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSTransportLayerState{}, fmt.Errorf("reading QMI WMS transport layer: %w", err)
	}
	return parsed.State, nil
}

// WMSServiceReady reads SMS service and SIM readiness.
func (c *Client) WMSServiceReady(ctx context.Context) (WMSServiceReadyState, error) {
	var parsed wmsServiceReadyResponse
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetServiceReadyState, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return parsed.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSServiceReadyState{}, fmt.Errorf("reading QMI WMS service ready state: %w", err)
	}
	return parsed.State, nil
}

// WMSTransportNetwork reads SMS transport network registration.
func (c *Client) WMSTransportNetwork(ctx context.Context) (WMSTransportNetworkState, error) {
	var parsed wmsTransportNetworkResponse
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetTransportNetwork, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return parsed.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSTransportNetworkState{}, fmt.Errorf("reading QMI WMS transport network: %w", err)
	}
	return parsed.State, nil
}

type wmsTransportLayerResponse struct {
	State WMSTransportLayerState
}

func (r *wmsTransportLayerResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var state WMSTransportLayerState
	if err := state.unmarshalTLVs(tlvs, false); err != nil {
		return err
	}
	*r = wmsTransportLayerResponse{State: state}
	return nil
}

type wmsServiceReadyResponse struct {
	State WMSServiceReadyState
}

func (r *wmsServiceReadyResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var state WMSServiceReadyState
	if err := state.unmarshalTLVs(tlvs, false); err != nil {
		return err
	}
	*r = wmsServiceReadyResponse{State: state}
	return nil
}

type wmsTransportNetworkResponse struct {
	State WMSTransportNetworkState
}

func (r *wmsTransportNetworkResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var state WMSTransportNetworkState
	if err := state.unmarshalTLVs(tlvs, false); err != nil {
		return err
	}
	*r = wmsTransportNetworkResponse{State: state}
	return nil
}

// WMSWatchTransportNetwork subscribes to SMS transport registration changes.
func (c *Client) WMSWatchTransportNetwork(ctx context.Context) (<-chan WMSTransportNetworkState, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSTransportNetwork, wmsIndicationTransportNetwork)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS transport network: %w", err)
	}
	return unmarshalTLVStream[WMSTransportNetworkState](ctx, raw), nil
}

// WMSWatchTransportLayer subscribes to IMS transport changes.
func (c *Client) WMSWatchTransportLayer(ctx context.Context) (<-chan WMSTransportLayerState, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSTransportLayer, wmsIndicationTransportLayer)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS transport layer: %w", err)
	}
	return unmarshalTLVStream[WMSTransportLayerState](ctx, raw), nil
}

// WMSWatchServiceReady subscribes to SMS service readiness changes.
func (c *Client) WMSWatchServiceReady(ctx context.Context) (<-chan WMSServiceReadyState, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSServiceReady, wmsIndicationServiceReady)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS service ready state: %w", err)
	}
	return unmarshalTLVStream[WMSServiceReadyState](ctx, raw), nil
}

// WMSWatchMemoryFull subscribes to SMS store-full indications.
func (c *Client) WMSWatchMemoryFull(ctx context.Context) (<-chan WMSMemoryFullState, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSMemoryFull, wmsIndicationMemoryFull)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS memory full state: %w", err)
	}
	return unmarshalTLVStream[WMSMemoryFullState](ctx, raw), nil
}

// WMSWatchCallStatus subscribes to SMS call status changes.
func (c *Client) WMSWatchCallStatus(ctx context.Context) (<-chan WMSCallStatus, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSCallStatus, wmsIndicationCallStatus)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS call status: %w", err)
	}
	return unmarshalTLVStream[WMSCallStatus](ctx, raw), nil
}

// WMSWatchSMSCAddress subscribes to SMS service-center address changes.
func (c *Client) WMSWatchSMSCAddress(ctx context.Context) (<-chan WMSSMSCAddress, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSSMSCAddress, wmsIndicationSMSCAddress)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS SMSC address: %w", err)
	}
	return unmarshalTLVStream[WMSSMSCAddress](ctx, raw), nil
}

func (c *Client) watchWMSTLVs(ctx context.Context, id MessageID, registration wmsIndicationRegistration) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceWMS)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceWMS, clientID, id)
	if err != nil {
		cancel()
		return nil, err
	}
	if registration != wmsIndicationNone {
		if err := c.acquireWMSIndication(ctx, registration); err != nil {
			cancel()
			return nil, err
		}
	}
	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		if registration != wmsIndicationNone {
			defer c.releaseWMSIndication(registration)
		}
		for indication := range indications {
			select {
			case out <- indication.TLVs:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *WMSTransportNetworkState) unmarshalTLVs(tlvs tlv.TLVs, indication bool) error {
	kind := byte(0x10)
	if indication {
		kind = 0x01
	}
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		if indication {
			return errors.New("parsing QMI WMS transport network indication: status TLV missing")
		}
		*s = WMSTransportNetworkState{}
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WMS transport network: status TLV length %d, want 1", len(value))
	}
	*s = WMSTransportNetworkState{Status: WMSTransportNetworkStatus(value[0]), Known: true}
	return nil
}

func (s *WMSTransportLayerState) unmarshalTLVs(tlvs tlv.TLVs, indication bool) error {
	registeredKind, infoKind := byte(0x10), byte(0x11)
	if indication {
		registeredKind, infoKind = 0x01, 0x10
	}
	var state WMSTransportLayerState
	if value, ok := tlv.Value(tlvs, registeredKind); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS transport layer: registration TLV length %d, want 1", len(value))
		}
		registered, err := decodeWMSBool(value[0])
		if err != nil {
			return fmt.Errorf("parsing QMI WMS transport layer registration: %w", err)
		}
		state.Registered = registered
		state.RegisteredKnown = true
	} else if indication {
		return errors.New("parsing QMI WMS transport layer indication: registration TLV missing")
	}
	if value, ok := tlv.Value(tlvs, infoKind); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS transport layer: info TLV length %d, want 2", len(value))
		}
		state.Type = WMSTransportType(value[0])
		state.Capability = WMSTransportCapability(value[1])
		state.InfoKnown = true
	}
	*s = state
	return nil
}

func (s *WMSServiceReadyState) unmarshalTLVs(tlvs tlv.TLVs, indication bool) error {
	var state WMSServiceReadyState
	statusKind, simStatusKind := byte(0x11), byte(0x13)
	if indication {
		statusKind, simStatusKind = 0x01, 0x10
	} else {
		if err := decodeWMSOptionalReadyBool(tlvs, 0x10, &state.EventsRegistered, &state.EventsRegisteredKnown); err != nil {
			return fmt.Errorf("parsing QMI WMS service ready event registration: %w", err)
		}
		if err := decodeWMSOptionalReadyBool(tlvs, 0x12, &state.SIMEventsRegistered, &state.SIMEventsKnown); err != nil {
			return fmt.Errorf("parsing QMI WMS service ready SIM event registration: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, statusKind); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WMS service ready state: status TLV length %d, want 4", len(value))
		}
		state.Status = WMSReadyStatus(binary.LittleEndian.Uint32(value))
		state.StatusKnown = true
	} else if indication {
		return errors.New("parsing QMI WMS service ready indication: status TLV missing")
	}
	if value, ok := tlv.Value(tlvs, simStatusKind); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI WMS service ready state: SIM status TLV length %d, want 8", len(value))
		}
		state.SIMStatus = WMSSIMReadyStatus(binary.LittleEndian.Uint64(value))
		state.SIMStatusKnown = true
	}
	*s = state
	return nil
}

func decodeWMSOptionalReadyBool(tlvs tlv.TLVs, kind byte, dst, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("TLV 0x%02x length %d, want 1", kind, len(value))
	}
	parsed, err := decodeWMSBool(value[0])
	if err != nil {
		return fmt.Errorf("TLV 0x%02x: %w", kind, err)
	}
	*dst = parsed
	*known = true
	return nil
}

func (s *WMSMemoryFullState) unmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS memory full indication: info TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI WMS memory full indication: info TLV length %d, want 2", len(value))
	}
	state := WMSMemoryFullState{Storage: WMSStorage(value[0]), MessageMode: WMSMessageMode(value[1])}
	if state.Storage != WMSStorageUIM && state.Storage != WMSStorageNV {
		return fmt.Errorf("parsing QMI WMS memory full indication: storage %d is out of range", state.Storage)
	}
	if err := validateWMSMessageMode(state.MessageMode); err != nil {
		return fmt.Errorf("parsing QMI WMS memory full indication: %w", err)
	}
	*s = state
	return nil
}

func (s *WMSCallStatus) unmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS call status indication: status TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WMS call status indication: status TLV length %d, want 1", len(value))
	}
	status := WMSCallStatus(value[0])
	if status > WMSCallConnecting {
		return fmt.Errorf("parsing QMI WMS call status indication: status %d is out of range", status)
	}
	*s = status
	return nil
}

func (a *WMSSMSCAddress) unmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS SMSC address indication: address TLV missing")
	}
	if err := a.UnmarshalBinary(value); err != nil {
		return fmt.Errorf("parsing QMI WMS SMSC address indication: %w", err)
	}
	return nil
}
