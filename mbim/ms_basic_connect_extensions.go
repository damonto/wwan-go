package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type SystemCapabilitiesRequest struct {
	TransactionID uint32
	Response      *SystemCapabilitiesInfo
}

func (r *SystemCapabilitiesRequest) Request() *Request {
	r.Response = new(SystemCapabilitiesInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsSystemCapabilities,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type SystemCapabilitiesInfo struct {
	Executors   uint32
	Slots       uint32
	Concurrency uint32
	ModemID     uint64
}

func (i *SystemCapabilitiesInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 20 {
		return fmt.Errorf("parsing MBIM system capabilities: payload length is %d, want 20", len(data))
	}
	*i = SystemCapabilitiesInfo{
		Executors:   binary.LittleEndian.Uint32(data[0:4]),
		Slots:       binary.LittleEndian.Uint32(data[4:8]),
		Concurrency: binary.LittleEndian.Uint32(data[8:12]),
		ModemID:     binary.LittleEndian.Uint64(data[12:20]),
	}
	return nil
}

type SlotInfoStatusRequest struct {
	TransactionID uint32
	SlotIndex     uint32
	Response      *SlotInfoStatus
}

func (r *SlotInfoStatusRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, r.SlotIndex)
	r.Response = new(SlotInfoStatus)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsSlotInfoStatus,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type SlotInfoStatus struct {
	SlotIndex uint32
	State     UICCSlotState
}

func (i *SlotInfoStatus) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("parsing MBIM slot info status: payload length is %d, want 8", len(data))
	}
	state := UICCSlotState(binary.LittleEndian.Uint32(data[4:8]))
	if state > UICCSlotStateActiveESIMNoProfiles {
		return fmt.Errorf("parsing MBIM slot info status: state %d is reserved", state)
	}
	*i = SlotInfoStatus{
		SlotIndex: binary.LittleEndian.Uint32(data[0:4]),
		State:     state,
	}
	return nil
}

type PCORequest struct {
	TransactionID uint32
	Value         PCOValue
	Response      *PCOValue
}

func (r *PCORequest) Request() *Request {
	data, err := r.Value.MarshalBinary()
	r.Response = new(PCOValue)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsPCO,
			CommandTypeQuery,
			data,
			err,
		),
		Response: r.Response,
	}
}

type PCOValue struct {
	SessionID SessionID
	Type      PCOType
	Data      []byte
}

func (v PCOValue) MarshalBinary() ([]byte, error) {
	if uint64(len(v.Data)) > uint64(^uint32(0)) {
		return nil, errors.New("encoding MBIM PCO value: data exceeds UINT32 length")
	}
	if err := validatePCOType(v.Type); err != nil {
		return nil, fmt.Errorf("encoding MBIM PCO value: %w", err)
	}
	return v.marshalBinaryUnchecked(), nil
}

func (v PCOValue) marshalBinaryUnchecked() []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(v.SessionID))
	data = binary.LittleEndian.AppendUint32(data, uint32(len(v.Data)))
	data = binary.LittleEndian.AppendUint32(data, uint32(v.Type))
	data = append(data, v.Data...)
	return padTo4Bytes(data)
}

func (v *PCOValue) UnmarshalBinary(data []byte) error {
	if len(data) < 12 {
		return errors.New("parsing MBIM PCO value: payload is truncated")
	}
	dataSize := binary.LittleEndian.Uint32(data[4:8])
	if uint64(dataSize) > uint64(len(data)-12) {
		return fmt.Errorf("parsing MBIM PCO value: data size %d exceeds remaining payload %d", dataSize, len(data)-12)
	}
	dataEnd := 12 + int(dataSize)
	expectedLength := align4(dataEnd)
	if len(data) < expectedLength {
		return errors.New("parsing MBIM PCO value: data buffer padding is truncated")
	}
	if len(data) > expectedLength {
		return errors.New("parsing MBIM PCO value: data buffer has trailing data")
	}
	typ := PCOType(binary.LittleEndian.Uint32(data[8:12]))
	if err := validatePCOType(typ); err != nil {
		return fmt.Errorf("parsing MBIM PCO value: %w", err)
	}
	*v = PCOValue{
		SessionID: SessionID(binary.LittleEndian.Uint32(data[0:4])),
		Type:      typ,
		Data:      slices.Clone(data[12:dataEnd]),
	}
	return nil
}

func validatePCOType(typ PCOType) error {
	if typ > PCOTypePartial {
		return fmt.Errorf("type is %d, want 0 or 1", typ)
	}
	return nil
}

type DeviceResetRequest struct {
	TransactionID uint32
	Response      *DeviceResetResponse
}

func (r *DeviceResetRequest) Request() *Request {
	r.Response = new(DeviceResetResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsDeviceReset,
			CommandTypeSet,
			nil,
		),
		Response: r.Response,
	}
}

type DeviceResetResponse struct{}

func (*DeviceResetResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return errors.New("parsing MBIM device reset: unexpected payload")
	}
	return nil
}

type LocationInfoStatusRequest struct {
	TransactionID uint32
	Response      *LocationInfoStatus
}

func (r *LocationInfoStatusRequest) Request() *Request {
	r.Response = new(LocationInfoStatus)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsLocationInfoStatus,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type LocationInfoStatus struct {
	LocationAreaCode uint32
	TrackingAreaCode uint32
	CellID           uint32
}

func (i *LocationInfoStatus) UnmarshalBinary(data []byte) error {
	if len(data) != 12 {
		return fmt.Errorf("parsing MBIM location info status: payload length is %d, want 12", len(data))
	}
	*i = LocationInfoStatus{
		LocationAreaCode: binary.LittleEndian.Uint32(data[0:4]),
		TrackingAreaCode: binary.LittleEndian.Uint32(data[4:8]),
		CellID:           binary.LittleEndian.Uint32(data[8:12]),
	}
	return nil
}

func (c *Client) SystemCapabilities(ctx context.Context) (SystemCapabilitiesInfo, error) {
	request := SystemCapabilitiesRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SystemCapabilitiesInfo{}, fmt.Errorf("reading MBIM system capabilities: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SlotInfoStatus(ctx context.Context, slotIndex uint32) (SlotInfoStatus, error) {
	request := SlotInfoStatusRequest{
		TransactionID: c.nextTransactionID(),
		SlotIndex:     slotIndex,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SlotInfoStatus{}, fmt.Errorf("reading MBIM slot info status: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) PCO(ctx context.Context, value PCOValue) (PCOValue, error) {
	if _, err := value.MarshalBinary(); err != nil {
		return PCOValue{}, err
	}
	request := PCORequest{
		TransactionID: c.nextTransactionID(),
		Value:         value,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return PCOValue{}, fmt.Errorf("reading MBIM PCO value: %w", err)
	}
	response := *request.Response
	response.Data = slices.Clone(response.Data)
	return response, nil
}

func (c *Client) ResetDevice(ctx context.Context) error {
	request := DeviceResetRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("resetting MBIM device: %w", err)
	}
	return nil
}

func (c *Client) LocationInfoStatus(ctx context.Context) (LocationInfoStatus, error) {
	request := LocationInfoStatusRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return LocationInfoStatus{}, fmt.Errorf("reading MBIM location info status: %w", err)
	}
	return *request.Response, nil
}
