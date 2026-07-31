package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type SARControlMode uint32

const (
	SARControlModeDevice SARControlMode = iota
	SARControlModeOS
)

type SARBackoffState uint32

const (
	SARBackoffStateDisabled SARBackoffState = iota
	SARBackoffStateEnabled
)

type SARWiFiHardwareState uint32

const (
	SARWiFiHardwareStateIntegrated SARWiFiHardwareState = iota
	SARWiFiHardwareStateNotIntegrated
)

type TransmissionNotificationStatus uint32

const (
	TransmissionNotificationStatusDisabled TransmissionNotificationStatus = iota
	TransmissionNotificationStatusEnabled
)

type TransmissionState uint32

const (
	TransmissionStateInactive TransmissionState = iota
	TransmissionStateActive
)

type SARConfigState struct {
	AntennaIndex uint32
	BackoffIndex uint32
}

func (s SARConfigState) MarshalBinary() ([]byte, error) {
	return s.appendBinary(nil), nil
}

func (s SARConfigState) appendBinary(data []byte) []byte {
	data = binary.LittleEndian.AppendUint32(data, s.AntennaIndex)
	return binary.LittleEndian.AppendUint32(data, s.BackoffIndex)
}

func (s *SARConfigState) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("parsing MBIM SAR configuration state: payload length %d, want 8", len(data))
	}
	*s = SARConfigState{
		AntennaIndex: binary.LittleEndian.Uint32(data[:4]),
		BackoffIndex: binary.LittleEndian.Uint32(data[4:8]),
	}
	return nil
}

type SARConfig struct {
	Mode         SARControlMode
	BackoffState SARBackoffState
	States       []SARConfigState
}

func (c SARConfig) MarshalBinary() ([]byte, error) {
	if err := validateSARControlMode(c.Mode); err != nil {
		return nil, fmt.Errorf("encoding MBIM SAR configuration: %w", err)
	}
	if err := validateSARBackoffState(c.BackoffState); err != nil {
		return nil, fmt.Errorf("encoding MBIM SAR configuration: %w", err)
	}
	return c.marshalBinary(), nil
}

func (c SARConfig) marshalBinary() []byte {
	elements := make([][]byte, len(c.States))
	for i, state := range c.States {
		elements[i] = state.appendBinary(nil)
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(c.Mode))
	header = binary.LittleEndian.AppendUint32(header, uint32(c.BackoffState))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

type SARConfigInfo struct {
	Mode            SARControlMode
	BackoffState    SARBackoffState
	WiFiIntegration SARWiFiHardwareState
	States          []SARConfigState
}

func (i *SARConfigInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("parsing MBIM SAR configuration: payload is truncated")
	}

	mode := SARControlMode(binary.LittleEndian.Uint32(data[:4]))
	if err := validateSARControlMode(mode); err != nil {
		return fmt.Errorf("parsing MBIM SAR configuration: %w", err)
	}
	backoffState := SARBackoffState(binary.LittleEndian.Uint32(data[4:8]))
	if err := validateSARBackoffState(backoffState); err != nil {
		return fmt.Errorf("parsing MBIM SAR configuration: %w", err)
	}
	wifiIntegration := SARWiFiHardwareState(binary.LittleEndian.Uint32(data[8:12]))
	if err := validateSARWiFiHardwareState(wifiIntegration); err != nil {
		return fmt.Errorf("parsing MBIM SAR configuration: %w", err)
	}

	count := binary.LittleEndian.Uint32(data[12:16])
	refs, err := offsetSizeRefs(data, 16, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM SAR configuration states: %w", err)
	}
	states := make([]SARConfigState, count)
	for index, ref := range refs {
		if err := states[index].UnmarshalBinary(ref.bytes(data)); err != nil {
			return fmt.Errorf("parsing MBIM SAR configuration state %d: %w", index, err)
		}
	}

	*i = SARConfigInfo{
		Mode:            mode,
		BackoffState:    backoffState,
		WiFiIntegration: wifiIntegration,
		States:          states,
	}
	return nil
}

type SARConfigRequest struct {
	TransactionID uint32
	Response      *SARConfigInfo
}

func (r *SARConfigRequest) Request() *Request {
	r.Response = new(SARConfigInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsSAR, CIDMsSARConfig, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type SARConfigSetRequest struct {
	TransactionID uint32
	Config        SARConfig
	Response      *SARConfigInfo
}

func (r *SARConfigSetRequest) Request() *Request {
	data, err := r.Config.MarshalBinary()
	r.Response = new(SARConfigInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       commandWithError(ServiceMsSAR, CIDMsSARConfig, CommandTypeSet, data, err),
		Response:      r.Response,
	}
}

type TransmissionStatusInfo struct {
	ChannelNotification TransmissionNotificationStatus
	State               TransmissionState
	HysteresisTimer     uint32
}

func (i *TransmissionStatusInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 12 {
		return fmt.Errorf("parsing MBIM SAR transmission status: payload length %d, want 12", len(data))
	}
	notification := TransmissionNotificationStatus(binary.LittleEndian.Uint32(data[:4]))
	if err := validateTransmissionNotificationStatus(notification); err != nil {
		return fmt.Errorf("parsing MBIM SAR transmission status: %w", err)
	}
	state := TransmissionState(binary.LittleEndian.Uint32(data[4:8]))
	if err := validateTransmissionState(state); err != nil {
		return fmt.Errorf("parsing MBIM SAR transmission status: %w", err)
	}
	*i = TransmissionStatusInfo{
		ChannelNotification: notification,
		State:               state,
		HysteresisTimer:     binary.LittleEndian.Uint32(data[8:12]),
	}
	return nil
}

type TransmissionStatusRequest struct {
	TransactionID uint32
	Response      *TransmissionStatusInfo
}

func (r *TransmissionStatusRequest) Request() *Request {
	r.Response = new(TransmissionStatusInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsSAR, CIDMsSARTransmissionStatus, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type TransmissionStatusSetRequest struct {
	TransactionID       uint32
	ChannelNotification TransmissionNotificationStatus
	HysteresisTimer     uint32
	Response            *TransmissionStatusInfo
}

func (r *TransmissionStatusSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.ChannelNotification))
	data = binary.LittleEndian.AppendUint32(data, r.HysteresisTimer)
	r.Response = new(TransmissionStatusInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceMsSAR, CIDMsSARTransmissionStatus, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (c *Client) SARConfig(ctx context.Context) (SARConfigInfo, error) {
	request := SARConfigRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SARConfigInfo{}, fmt.Errorf("reading MBIM SAR configuration: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetSARConfig(ctx context.Context, config SARConfig) (SARConfigInfo, error) {
	if _, err := config.MarshalBinary(); err != nil {
		return SARConfigInfo{}, err
	}
	request := SARConfigSetRequest{
		TransactionID: c.nextTransactionID(),
		Config:        config,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SARConfigInfo{}, fmt.Errorf("setting MBIM SAR configuration: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) TransmissionStatus(ctx context.Context) (TransmissionStatusInfo, error) {
	request := TransmissionStatusRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return TransmissionStatusInfo{}, fmt.Errorf("reading MBIM SAR transmission status: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetTransmissionStatus(ctx context.Context, notification TransmissionNotificationStatus, hysteresisTimer uint32) (TransmissionStatusInfo, error) {
	if err := validateTransmissionNotificationStatus(notification); err != nil {
		return TransmissionStatusInfo{}, fmt.Errorf("setting MBIM SAR transmission status: %w", err)
	}
	request := TransmissionStatusSetRequest{
		TransactionID:       c.nextTransactionID(),
		ChannelNotification: notification,
		HysteresisTimer:     hysteresisTimer,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return TransmissionStatusInfo{}, fmt.Errorf("setting MBIM SAR transmission status: %w", err)
	}
	return *request.Response, nil
}

// ReadTransmissionStatus waits for the next SAR transmission status notification.
func (c *Client) ReadTransmissionStatus(ctx context.Context) (TransmissionStatusInfo, error) {
	indication, err := c.NextIndication(ctx, ServiceMsSAR, CIDMsSARTransmissionStatus)
	if err != nil {
		return TransmissionStatusInfo{}, fmt.Errorf("reading MBIM SAR transmission notification: %w", err)
	}
	var info TransmissionStatusInfo
	if err := info.UnmarshalBinary(indication.InformationBuffer); err != nil {
		return TransmissionStatusInfo{}, fmt.Errorf("reading MBIM SAR transmission notification: %w", err)
	}
	return info, nil
}

// WatchTransmissionStatus streams SAR transmission status notifications until ctx is done.
func (c *Client) WatchTransmissionStatus(ctx context.Context) (<-chan TransmissionStatusInfo, error) {
	results, err := c.WatchTransmissionStatusResults(ctx)
	if err != nil {
		return nil, err
	}
	return watchValues(ctx, results), nil
}

// WatchTransmissionStatusResults streams SAR transmission status
// notifications and reports receiver or payload errors through the terminal result.
func (c *Client) WatchTransmissionStatusResults(ctx context.Context) (<-chan WatchResult[TransmissionStatusInfo], error) {
	indications, err := c.WatchIndicationResults(ctx, ServiceMsSAR, CIDMsSARTransmissionStatus)
	if err != nil {
		return nil, fmt.Errorf("watching MBIM SAR transmission notifications: %w", err)
	}
	return watchDecoded(ctx, indications, "watching MBIM SAR transmission notifications", func(data []byte) (TransmissionStatusInfo, error) {
		var info TransmissionStatusInfo
		if err := info.UnmarshalBinary(data); err != nil {
			return TransmissionStatusInfo{}, err
		}
		return info, nil
	}), nil
}

func validateSARControlMode(mode SARControlMode) error {
	if mode > SARControlModeOS {
		return fmt.Errorf("control mode %d is reserved", mode)
	}
	return nil
}

func validateSARBackoffState(state SARBackoffState) error {
	if state > SARBackoffStateEnabled {
		return fmt.Errorf("backoff state %d is reserved", state)
	}
	return nil
}

func validateSARWiFiHardwareState(state SARWiFiHardwareState) error {
	if state > SARWiFiHardwareStateNotIntegrated {
		return fmt.Errorf("Wi-Fi hardware state %d is reserved", state)
	}
	return nil
}

func validateTransmissionNotificationStatus(status TransmissionNotificationStatus) error {
	if status > TransmissionNotificationStatusEnabled {
		return fmt.Errorf("channel notification status %d is reserved", status)
	}
	return nil
}

func validateTransmissionState(state TransmissionState) error {
	if state > TransmissionStateActive {
		return fmt.Errorf("transmission state %d is reserved", state)
	}
	return nil
}
