package qcom

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func (c *Client) Reset(ctx context.Context) error {
	resp, err := c.request(ctx, MessageReset, nil)
	if err != nil {
		return fmt.Errorf("resetting QMI UIM service: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("resetting QMI UIM service: %w", err)
	}
	return nil
}

func (c *Client) PowerOffSIM(ctx context.Context, slot uint8) error {
	if slot == 0 {
		return errors.New("powering off QMI UIM SIM: slot is zero")
	}

	resp, err := c.request(ctx, MessagePowerOffSIM, tlv.TLVs{
		tlv.Uint(0x01, slot),
	})
	if err != nil {
		return fmt.Errorf("powering off QMI UIM SIM slot %d: %w", slot, err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("powering off QMI UIM SIM slot %d: %w", slot, err)
	}
	return nil
}

func (c *Client) PowerOnSIM(ctx context.Context, req PowerOnSIMRequest) error {
	if req.Slot == 0 {
		return errors.New("powering on QMI UIM SIM: slot is zero")
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, req.Slot)}
	if req.IgnoreHotSwapSwitch {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(1)))
	}

	resp, err := c.request(ctx, MessagePowerOnSIM, tlvs)
	if err != nil {
		return fmt.Errorf("powering on QMI UIM SIM slot %d: %w", req.Slot, err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("powering on QMI UIM SIM slot %d: %w", req.Slot, err)
	}
	return nil
}

func (c *Client) ChangeProvisioningSession(ctx context.Context, req ChangeProvisioningSessionRequest) error {
	if err := validateUIMAIDLength(req.AID); err != nil {
		return fmt.Errorf("changing QMI UIM provisioning session: %w", err)
	}

	activate := uint8(0)
	if req.Activate {
		activate = 1
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(req.Session), activate}),
	}
	if req.Slot != 0 || len(req.AID) > 0 {
		app := []byte{req.Slot, byte(len(req.AID))}
		app = append(app, req.AID...)
		tlvs = append(tlvs, tlv.Bytes(0x10, app))
	}

	resp, err := c.request(ctx, MessageChangeProvisioningSession, tlvs)
	if err != nil {
		return fmt.Errorf("changing QMI UIM provisioning session: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("changing QMI UIM provisioning session: %w", err)
	}
	return nil
}
