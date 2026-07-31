package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	slotReadyTimeout  = 5 * time.Second
	slotPollInterval  = 500 * time.Millisecond
	slotStatusTimeout = 1 * time.Second
)

func (c *Client) ActivateSlot(ctx context.Context) error {
	status, err := c.SlotStatus(ctx)
	if err != nil {
		if errors.Is(err, QMIErrorNotSupported) {
			return nil
		}
		return fmt.Errorf("activating slot %d: %w", c.slot, err)
	}
	if status.ActiveSlot == c.slot {
		return nil
	}
	if err := c.SwitchSlot(ctx, 1, uint32(c.slot)); err != nil {
		return fmt.Errorf("activating slot %d: %w", c.slot, err)
	}
	if err := c.waitForSlotReady(ctx); err != nil {
		return fmt.Errorf("activating slot %d: %w", c.slot, err)
	}
	return nil
}

func (c *Client) SlotStatus(ctx context.Context) (SlotStatus, error) {
	resp, err := c.requestWithTimeout(ctx, MessageGetSlotStatus, nil, slotStatusTimeout)
	if err != nil {
		return SlotStatus{}, err
	}
	if err := resultOK(resp); err != nil {
		return SlotStatus{}, err
	}
	var status SlotStatus
	if err := status.UnmarshalTLVs(resp.TLVs); err != nil {
		return SlotStatus{}, err
	}
	return status, nil
}

func (c *Client) SwitchSlot(ctx context.Context, logicalSlot uint8, physicalSlot uint32) error {
	resp, err := c.request(ctx, MessageSwitchSlot, tlv.TLVs{
		tlv.Uint(0x01, logicalSlot),
		tlv.Uint(0x02, physicalSlot),
	})
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *Client) waitForSlotReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, slotReadyTimeout)
	defer cancel()

	for {
		status, err := c.CardStatus(ctx)
		if err == nil && status.Ready() {
			return nil
		}
		if ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("waiting for card readiness: %w", err)
			}
			return errors.New("waiting for card readiness: timeout")
		}

		timer := time.NewTimer(slotPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if err != nil {
				return fmt.Errorf("waiting for card readiness: %w", err)
			}
			return errors.New("waiting for card readiness: timeout")
		case <-timer.C:
		}
	}
}

func (c *Client) CardStatus(ctx context.Context) (CardStatus, error) {
	resp, err := c.request(ctx, MessageGetCardStatus, nil)
	if err != nil {
		return CardStatus{}, err
	}
	if err := resultOK(resp); err != nil {
		return CardStatus{}, err
	}
	var status CardStatus
	if err := status.UnmarshalTLVs(resp.TLVs); err != nil {
		return CardStatus{}, err
	}
	return status, nil
}

func (c *Client) Slot() uint8 {
	return c.slot
}
