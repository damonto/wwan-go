package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDASetLoopbackStateRequest encodes the deprecated Set Loopback State
// operation. New code should prefer WDASetLoopbackConfigRequest.
type WDASetLoopbackStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the legacy loopback state into QMI WDA TLVs.
func (r WDASetLoopbackStateRequest) Request() Request {
	return wdaRequest(
		r.ClientID,
		r.TransactionID,
		r.Timeout,
		MessageWDASetLoopbackState,
		tlv.TLVs{tlv.Uint(0x01, boolByte(r.Enabled))},
	)
}

// WDAGetLoopbackStateRequest encodes Get Loopback State.
type WDAGetLoopbackStateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request returns the empty Get Loopback State request.
func (r WDAGetLoopbackStateRequest) Request() Request {
	return wdaRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDAGetLoopbackState, nil)
}

// WDALoopbackState contains the modem's packet-loopback configuration.
type WDALoopbackState struct {
	Enabled                bool
	EnabledKnown           bool
	ReplicationFactor      uint32
	ReplicationFactorKnown bool
}

// WDAGetLoopbackStateResponse contains the optional state returned by Get
// Loopback State.
type WDAGetLoopbackStateResponse struct {
	State WDALoopbackState
}

// UnmarshalTLVs parses a Get Loopback State response.
func (r *WDAGetLoopbackStateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var state WDALoopbackState
	if err := state.unmarshalTLVs(tlvs, 0x10, 0x11, false); err != nil {
		return err
	}
	*r = WDAGetLoopbackStateResponse{State: state}
	return nil
}

// WDASetLoopbackConfig contains the current loopback state and optional
// replication factor.
type WDASetLoopbackConfig struct {
	Enabled           bool
	ReplicationFactor *uint32
}

// WDASetLoopbackConfigRequest encodes Set Loopback Configuration.
type WDASetLoopbackConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDASetLoopbackConfig
}

// Request converts the loopback configuration into QMI WDA TLVs.
func (r WDASetLoopbackConfigRequest) Request() Request {
	tlvs := tlv.TLVs{tlv.Uint(0x01, boolByte(r.Config.Enabled))}
	if r.Config.ReplicationFactor != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Config.ReplicationFactor))
	}
	return wdaRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDASetLoopbackConfig, tlvs)
}

// WDALoopbackConfigIndication is the asynchronous result of Set Loopback
// Configuration.
type WDALoopbackConfigIndication struct {
	State WDALoopbackState
}

// UnmarshalTLVs parses a Loopback Configuration indication.
func (i *WDALoopbackConfigIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var state WDALoopbackState
	if err := state.unmarshalTLVs(tlvs, 0x01, 0x10, true); err != nil {
		return err
	}
	*i = WDALoopbackConfigIndication{State: state}
	return nil
}

// WDASetPowersaveConfigMask selects powersave notifications controlled by the
// modem.
type WDASetPowersaveConfigMask uint32

const (
	WDAPowersaveConfigUnsupported    WDASetPowersaveConfigMask = 0
	WDAPowersaveConfigDownlinkMarker WDASetPowersaveConfigMask = 1 << 0
	WDAPowersaveConfigFlowControl    WDASetPowersaveConfigMask = 1 << 1
	WDAPowersaveConfigAll            WDASetPowersaveConfigMask = 0x7fffffff
)

// WDASetPowersaveConfig contains the powersave settings for one endpoint.
type WDASetPowersaveConfig struct {
	Endpoint      DataEndpoint
	RequestedMask *WDASetPowersaveConfigMask
}

// WDASetPowersaveConfigRequest encodes Set Powersave Configuration.
type WDASetPowersaveConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDASetPowersaveConfig
}

// Request converts the endpoint powersave configuration into QMI WDA TLVs.
func (r WDASetPowersaveConfigRequest) Request() (Request, error) {
	endpoint, err := r.Config.Endpoint.MarshalBinary()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI WDA powersave endpoint: %w", err)
	}

	tlvs := tlv.TLVs{tlv.Bytes(0x01, endpoint)}
	if r.Config.RequestedMask != nil {
		if err := validateWDAPowersaveConfigMask(*r.Config.RequestedMask); err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.Config.RequestedMask)))
	}
	return wdaRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDASetPowersaveConfig, tlvs), nil
}

// WDASetPowersaveConfigResponse contains the powersave mask supported by the
// modem.
type WDASetPowersaveConfigResponse struct {
	Mask      WDASetPowersaveConfigMask
	MaskKnown bool
}

// UnmarshalTLVs parses a Set Powersave Configuration response.
func (r *WDASetPowersaveConfigResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDASetPowersaveConfigResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI WDA powersave configuration: mask TLV length %d, want 4", len(value))
	}

	mask := WDASetPowersaveConfigMask(binary.LittleEndian.Uint32(value))
	if err := validateWDAPowersaveConfigMask(mask); err != nil {
		return err
	}
	r.Mask = mask
	r.MaskKnown = true
	return nil
}

// WDASetPowersaveModeRequest encodes Set Powersave Mode.
type WDASetPowersaveModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the powersave mode into QMI WDA TLVs.
func (r WDASetPowersaveModeRequest) Request() Request {
	return wdaRequest(
		r.ClientID,
		r.TransactionID,
		r.Timeout,
		MessageWDASetPowersaveMode,
		tlv.TLVs{tlv.Uint(0x01, boolByte(r.Enabled))},
	)
}

// WDADefaultFlowRebindVersion identifies the default-flow rebind capability.
type WDADefaultFlowRebindVersion uint32

const (
	WDADefaultFlowRebindUnsupported WDADefaultFlowRebindVersion = iota
	WDADefaultFlowRebindVersion1
)

// WDASetCapabilityConfig contains the endpoint capabilities negotiated with
// the modem. EthernetPDUCapability is supported by newer WDA revisions.
type WDASetCapabilityConfig struct {
	Endpoint              DataEndpoint
	DefaultFlowRebind     *WDADefaultFlowRebindVersion
	EthernetPDUCapability *bool
}

// WDASetCapabilityRequest encodes Set Capability.
type WDASetCapabilityRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDASetCapabilityConfig
}

// Request converts endpoint capabilities into QMI WDA TLVs.
func (r WDASetCapabilityRequest) Request() (Request, error) {
	endpoint, err := r.Config.Endpoint.MarshalBinary()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI WDA capability endpoint: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, endpoint)}
	if r.Config.DefaultFlowRebind != nil {
		if *r.Config.DefaultFlowRebind > WDADefaultFlowRebindVersion1 {
			return Request{}, fmt.Errorf("QMI WDA default flow rebind version %d is unsupported", *r.Config.DefaultFlowRebind)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(*r.Config.DefaultFlowRebind)))
	}
	if r.Config.EthernetPDUCapability != nil {
		tlvs = append(tlvs, tlv.Uint(0x15, boolByte(*r.Config.EthernetPDUCapability)))
	}
	return wdaRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDASetCapability, tlvs), nil
}

// WDASetCapabilityResponse contains capabilities honored by the modem.
type WDASetCapabilityResponse struct {
	DefaultFlowRebind          WDADefaultFlowRebindVersion
	DefaultFlowRebindKnown     bool
	EthernetPDUCapability      bool
	EthernetPDUCapabilityKnown bool
}

// WDASetLoopbackState applies the deprecated loopback switch. New code should
// use WDASetLoopbackConfiguration so it can also set the replication factor.
func (c *Client) WDASetLoopbackState(ctx context.Context, enabled bool) error {
	req := WDASetLoopbackStateRequest{Timeout: DefaultRequestTimeout, Enabled: enabled}.Request()
	if err := c.wdaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI WDA loopback state: %w", err)
	}
	return nil
}

// WDAGetLoopbackState returns the modem's current loopback state.
func (c *Client) WDAGetLoopbackState(ctx context.Context) (WDALoopbackState, error) {
	req := WDAGetLoopbackStateRequest{Timeout: DefaultRequestTimeout}.Request()
	var parsed WDAGetLoopbackStateResponse
	if err := c.wdaRequest(ctx, req, &parsed); err != nil {
		return WDALoopbackState{}, fmt.Errorf("getting QMI WDA loopback state: %w", err)
	}
	return parsed.State, nil
}

// WDASetPowersaveConfiguration sets and returns the powersave mask supported
// for an endpoint.
func (c *Client) WDASetPowersaveConfiguration(ctx context.Context, config WDASetPowersaveConfig) (WDASetPowersaveConfigResponse, error) {
	req, err := (WDASetPowersaveConfigRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return WDASetPowersaveConfigResponse{}, fmt.Errorf("setting QMI WDA powersave configuration: %w", err)
	}
	var parsed WDASetPowersaveConfigResponse
	if err := c.wdaRequest(ctx, req, &parsed); err != nil {
		return WDASetPowersaveConfigResponse{}, fmt.Errorf("setting QMI WDA powersave configuration: %w", err)
	}
	return parsed, nil
}

// WDASetPowersaveMode enables or disables powersave mode for the terminal.
func (c *Client) WDASetPowersaveMode(ctx context.Context, enabled bool) error {
	req := WDASetPowersaveModeRequest{Timeout: DefaultRequestTimeout, Enabled: enabled}.Request()
	if err := c.wdaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI WDA powersave mode: %w", err)
	}
	return nil
}

// UnmarshalTLVs parses the optional capability fields.
func (r *WDASetCapabilityResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDASetCapabilityResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDA capability: default flow rebind TLV length %d, want 4", len(value))
		}
		r.DefaultFlowRebind = WDADefaultFlowRebindVersion(binary.LittleEndian.Uint32(value))
		r.DefaultFlowRebindKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		capability, err := decodeWDABool(value)
		if err != nil {
			return err
		}
		r.EthernetPDUCapability = capability
		r.EthernetPDUCapabilityKnown = true
	}
	return nil
}

// WDASetLoopbackConfiguration applies the current loopback configuration.
func (c *Client) WDASetLoopbackConfiguration(ctx context.Context, config WDASetLoopbackConfig) error {
	req := WDASetLoopbackConfigRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.wdaRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI WDA loopback configuration: %w", err)
	}
	return nil
}

// WDAWatchLoopbackConfiguration watches asynchronous loopback results.
func (c *Client) WDAWatchLoopbackConfiguration(ctx context.Context) (<-chan WDALoopbackConfigIndication, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceWDA)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WDA loopback configuration: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceWDA, clientID, MessageWDALoopbackConfigResult)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI WDA loopback configuration: %w", err)
	}
	out := make(chan WDALoopbackConfigIndication, 8)
	go func() {
		defer close(out)
		defer cancel()
		for indication := range indications {
			var event WDALoopbackConfigIndication
			if err := event.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// WDASetCapabilities negotiates endpoint capabilities with the modem.
func (c *Client) WDASetCapabilities(ctx context.Context, config WDASetCapabilityConfig) (WDASetCapabilityResponse, error) {
	req, err := (WDASetCapabilityRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return WDASetCapabilityResponse{}, fmt.Errorf("setting QMI WDA capabilities: %w", err)
	}
	var parsed WDASetCapabilityResponse
	if err := c.wdaRequest(ctx, req, &parsed); err != nil {
		return WDASetCapabilityResponse{}, fmt.Errorf("setting QMI WDA capabilities: %w", err)
	}
	return parsed, nil
}

func (s *WDALoopbackState) unmarshalTLVs(tlvs tlv.TLVs, stateKind, replicationKind byte, mandatory bool) error {
	var state WDALoopbackState
	value, ok := tlv.Value(tlvs, stateKind)
	if ok {
		enabled, err := decodeWDABool(value)
		if err != nil {
			return err
		}
		state.Enabled, state.EnabledKnown = enabled, true
	} else if mandatory {
		return errors.New("parsing QMI WDA loopback configuration: state TLV missing")
	}
	value, ok = tlv.Value(tlvs, replicationKind)
	if ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDA loopback state: replication factor TLV length %d, want 4", len(value))
		}
		state.ReplicationFactor = binary.LittleEndian.Uint32(value)
		state.ReplicationFactorKnown = true
	}
	*s = state
	return nil
}

func validateWDAPowersaveConfigMask(mask WDASetPowersaveConfigMask) error {
	if mask&^WDAPowersaveConfigAll != 0 {
		return fmt.Errorf("QMI WDA powersave configuration mask 0x%08X is invalid", uint32(mask))
	}
	return nil
}

func wdaRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID, tlvs tlv.TLVs) Request {
	return Request{
		Service:       ServiceWDA,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
		TLVs:          tlvs,
	}
}

func (c *Client) wdaRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceWDA, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceWDA, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if dst == nil {
			return nil
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func decodeWDABool(value []byte) (bool, error) {
	if len(value) != 1 {
		return false, fmt.Errorf("parsing QMI WDA boolean: TLV length %d, want 1", len(value))
	}
	if value[0] > 1 {
		return false, fmt.Errorf("parsing QMI WDA boolean: value %d, want 0 or 1", value[0])
	}
	return value[0] == 1, nil
}
