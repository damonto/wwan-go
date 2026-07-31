package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"
)

const (
	slotPollInterval = 500 * time.Millisecond
	slotReadyTimeout = 5 * time.Second
)

type UICCSlotState uint32

const (
	UICCSlotStateUnknown              UICCSlotState = 0
	UICCSlotStateOffEmpty             UICCSlotState = 1
	UICCSlotStateOff                  UICCSlotState = 2
	UICCSlotStateEmpty                UICCSlotState = 3
	UICCSlotStateNotReady             UICCSlotState = 4
	UICCSlotStateActive               UICCSlotState = 5
	UICCSlotStateError                UICCSlotState = 6
	UICCSlotStateActiveESIM           UICCSlotState = 7
	UICCSlotStateActiveESIMNoProfiles UICCSlotState = 8
)

func (c *Client) ensureSlotActivated(ctx context.Context) error {
	slot, err := c.currentActivatedSlot(ctx)
	if err != nil {
		if errors.Is(err, StatusNoDeviceSupport) {
			return nil
		}
		return fmt.Errorf("activating MBIM slot %d: %w", c.slot+1, err)
	}
	if slot == c.slot {
		return nil
	}
	if err := c.activateSlot(ctx, c.slot); err != nil {
		return fmt.Errorf("activating MBIM slot %d: %w", c.slot+1, err)
	}
	if err := c.waitForSlotReady(ctx); err != nil {
		return fmt.Errorf("activating MBIM slot %d: %w", c.slot+1, err)
	}
	return nil
}

func (c *Client) currentActivatedSlot(ctx context.Context) (uint32, error) {
	mappings, err := c.DeviceSlotMappings(ctx)
	if err != nil {
		return 0, err
	}
	if len(mappings) == 0 {
		return 0, errors.New("reading MBIM slot mappings: mapping is empty")
	}
	return mappings[0].Slot, nil
}

func (c *Client) activateSlot(ctx context.Context, slot uint32) error {
	_, err := c.SetDeviceSlotMappings(ctx, []SlotMapping{{Slot: slot}})
	return err
}

// DeviceSlotMappings returns the SIM slot assigned to each modem executor.
func (c *Client) DeviceSlotMappings(ctx context.Context) ([]SlotMapping, error) {
	request := DeviceSlotMappingsRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM device slot mappings: %w", err)
	}
	return slices.Clone(request.Response.SlotMappings), nil
}

// SetDeviceSlotMappings assigns one SIM slot to each modem executor.
func (c *Client) SetDeviceSlotMappings(ctx context.Context, mappings []SlotMapping) ([]SlotMapping, error) {
	request := DeviceSlotMappingsSetRequest{
		TransactionID: c.nextTransactionID(),
		SlotMappings:  slices.Clone(mappings),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM device slot mappings: %w", err)
	}
	return slices.Clone(request.Response.SlotMappings), nil
}

func (c *Client) waitForSlotReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, slotReadyTimeout)
	defer cancel()

	var lastReadyState SubscriberReadyState
	var sawReadyState bool
	for {
		request := SubscriberReadyStatusRequest{
			TransactionID: c.nextTransactionID(),
			MBIMExVersion: c.mbimExVersion,
			SlotID:        c.subscriberReadySlotID(),
		}
		err := c.transmit(ctx, request.Request())
		if err == nil {
			sawReadyState = true
			lastReadyState = request.Response.ReadyState
			if lastReadyState == SubscriberReadyStateInitialized || lastReadyState == SubscriberReadyStateNoESIMProfile {
				return nil
			}
		}
		if ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("waiting for MBIM SIM readiness: %w", err)
			}
			if sawReadyState {
				return fmt.Errorf("waiting for MBIM SIM readiness: last ready state %#x", lastReadyState)
			}
			return errors.New("waiting for MBIM SIM readiness: timeout")
		}

		timer := time.NewTimer(slotPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

type DeviceSlotMappingsRequest struct {
	TransactionID uint32
	Response      *DeviceSlotMappingsResponse
}

type SlotMapping struct {
	Slot uint32
}

func (r *DeviceSlotMappingsRequest) Request() *Request {
	r.Response = new(DeviceSlotMappingsResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDDeviceSlotMappings,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type DeviceSlotMappingsSetRequest struct {
	TransactionID uint32
	SlotMappings  []SlotMapping
	Response      *DeviceSlotMappingsResponse
}

func (r *DeviceSlotMappingsSetRequest) Request() *Request {
	mapCount := uint32(len(r.SlotMappings))
	data := binary.LittleEndian.AppendUint32(nil, mapCount)
	if mapCount > 0 {
		dataOffset := 4 + mapCount*8
		for i := range mapCount {
			data = binary.LittleEndian.AppendUint32(data, dataOffset+i*4)
			data = binary.LittleEndian.AppendUint32(data, 4)
		}
		for _, mapping := range r.SlotMappings {
			data = binary.LittleEndian.AppendUint32(data, mapping.Slot)
		}
	}

	r.Response = new(DeviceSlotMappingsResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDDeviceSlotMappings,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type DeviceSlotMappingsResponse struct {
	SlotMappings []SlotMapping
}

func (r *DeviceSlotMappingsResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM slot mappings: payload is truncated")
	}
	mapCount := binary.LittleEndian.Uint32(data[:4])
	refs, err := offsetSizeRefs(data, 4, mapCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM slot mappings: %w", err)
	}

	var mappings []SlotMapping
	if mapCount != 0 {
		mappings = make([]SlotMapping, mapCount)
	}
	for i, ref := range refs {
		if ref.size != 4 {
			return fmt.Errorf("parsing MBIM slot mappings: slot data size %d, want 4", ref.size)
		}
		mappings[i].Slot = binary.LittleEndian.Uint32(ref.bytes(data))
	}
	r.SlotMappings = mappings
	return nil
}
