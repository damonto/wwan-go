package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

const (
	mbimVersion10        uint16 = 0x0100
	mbimExVersion10      uint16 = 0x0100
	mbimExVersion20      uint16 = 0x0200
	mbimExVersion30      uint16 = 0x0300
	mbimExVersion40      uint16 = 0x0400
	hostMBIMExVersion           = mbimExVersion40
	activeSubscriberSlot        = 0xFFFFFFFF
	uiccSlotIDMaximum           = 1
)

// Version returns the MBIM and MBIMEx versions negotiated with the device.
// The values use the BCD wire representation defined by MBIM_CID_VERSION.
func (c *Client) Version() VersionInfo {
	return VersionInfo{
		MBIMVersion:   mbimVersion10,
		MBIMExVersion: c.mbimExVersion,
	}
}

func (c *Client) negotiateVersion(ctx context.Context) error {
	c.mbimExVersion = mbimExVersion10

	services, err := c.DeviceServices(ctx)
	if err != nil {
		return fmt.Errorf("negotiating MBIM version: %w", err)
	}
	if !services.SupportsCID(ServiceMsBasicConnectExtensions, CIDVersion) {
		return nil
	}

	version := VersionRequest{
		TransactionID: c.nextTransactionID(),
		MBIMVersion:   mbimVersion10,
		MBIMExVersion: hostMBIMExVersion,
	}
	if err := c.transmit(ctx, version.Request()); err != nil {
		return fmt.Errorf("negotiating MBIM version: %w", err)
	}
	c.mbimExVersion = min(version.Response.MBIMExVersion, hostMBIMExVersion)
	return nil
}

func (c *Client) usesUiccSlotID() bool {
	return c.mbimExVersion >= mbimExVersion40
}

func (c *Client) validateUiccSlotID() error {
	if c.usesUiccSlotID() && c.slot > uiccSlotIDMaximum {
		return fmt.Errorf(
			"MBIMEx 4 UICC slot ID %d is outside 0..%d: %w",
			c.slot,
			uiccSlotIDMaximum,
			StatusInvalidSlot,
		)
	}
	return nil
}

func (c *Client) subscriberReadySlotID() uint32 {
	if c.usesUiccSlotID() {
		return c.slot
	}
	return activeSubscriberSlot
}

type VersionRequest struct {
	TransactionID uint32
	MBIMVersion   uint16
	MBIMExVersion uint16
	Response      *VersionInfo
}

func (r *VersionRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint16(nil, r.MBIMVersion)
	data = binary.LittleEndian.AppendUint16(data, r.MBIMExVersion)

	r.Response = new(VersionInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDVersion,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type VersionInfo struct {
	MBIMVersion   uint16
	MBIMExVersion uint16
}

func (r *VersionInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("parsing MBIM version: payload length is %d, want 4", len(data))
	}
	mbimVersion := binary.LittleEndian.Uint16(data[:2])
	if !validBCDNibbles(mbimVersion, 4) {
		return fmt.Errorf("parsing MBIM version: MBIM release %#04x contains a non-decimal BCD digit", mbimVersion)
	}
	mbimExVersion := binary.LittleEndian.Uint16(data[2:4])
	if !validBCDNibbles(mbimExVersion, 4) {
		return fmt.Errorf("parsing MBIM version: MBIM extensions release %#04x contains a non-decimal BCD digit", mbimExVersion)
	}
	r.MBIMVersion = mbimVersion
	r.MBIMExVersion = mbimExVersion
	return nil
}
