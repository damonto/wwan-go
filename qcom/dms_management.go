package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const dmsMACAddressMax = 8

// DMSPRLVersion contains the active Preferred Roaming List version.
type DMSPRLVersion struct {
	Version      uint16
	PRLOnly      bool
	PRLOnlyKnown bool
}

// DMSMACDevice selects a device whose factory MAC address is requested.
type DMSMACDevice uint32

const (
	DMSMACDeviceWLAN DMSMACDevice = iota
	DMSMACDeviceBluetooth
)

// DMSTimeReference selects the time base updated by Set Time.
type DMSTimeReference uint32

const DMSTimeReferenceUser DMSTimeReference = 0

// DMSResetRequest encodes QMI DMS Reset.
type DMSResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r DMSResetRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSReset)
}

// DMSGetPRLVersionRequest encodes Get PRL Version.
type DMSGetPRLVersionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSGetPRLVersionRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetPRLVersion)
}

// DMSGetPRLVersionResponse contains the parsed active PRL version.
type DMSGetPRLVersionResponse struct {
	Info DMSPRLVersion
}

// UnmarshalTLVs parses the mandatory version and optional PRL-only flag.
func (r *DMSGetPRLVersionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetPRLVersionResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI DMS PRL version: version TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI DMS PRL version: version TLV length %d, want 2", len(value))
	}
	r.Info.Version = binary.LittleEndian.Uint16(value)
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DMS PRL version: PRL-only TLV length %d, want 1", len(value))
		}
		r.Info.PRLOnly = value[0] != 0
		r.Info.PRLOnlyKnown = true
	}
	return nil
}

// DMSSetTimeRequest encodes Set Time in GPS-epoch milliseconds.
type DMSSetTimeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Milliseconds  uint64
	Reference     DMSTimeReference
}

// Request validates and converts the timestamp into a QMI request.
func (r DMSSetTimeRequest) Request() (Request, error) {
	if r.Reference != DMSTimeReferenceUser {
		return Request{}, fmt.Errorf("DMS time reference %d is out of range", r.Reference)
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, binary.LittleEndian.AppendUint64(nil, r.Milliseconds)),
		tlv.Uint(0x10, uint32(r.Reference)),
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetTime,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DMSGetAltNetworkConfigRequest encodes Get Alt Net Config.
type DMSGetAltNetworkConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSGetAltNetworkConfigRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetAltNetworkConfig)
}

// DMSGetAltNetworkConfigResponse contains the alternate-network flag.
type DMSGetAltNetworkConfigResponse struct {
	Enabled bool
}

// UnmarshalTLVs parses the mandatory alternate-network flag.
func (r *DMSGetAltNetworkConfigResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetAltNetworkConfigResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI DMS alternate network config: config TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS alternate network config: config TLV length %d, want 1", len(value))
	}
	r.Enabled = value[0] != 0
	return nil
}

// DMSSetAltNetworkConfigRequest encodes Set Alt Net Config.
type DMSSetAltNetworkConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the alternate-network flag into a QMI request.
func (r DMSSetAltNetworkConfigRequest) Request() Request {
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetAltNetworkConfig,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, boolByte(r.Enabled))},
	}
}

// DMSGetMACAddressRequest encodes Get MAC Address.
type DMSGetMACAddressRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Device        DMSMACDevice
}

// Request validates and converts the device selector into a QMI request.
func (r DMSGetMACAddressRequest) Request() (Request, error) {
	if r.Device != DMSMACDeviceWLAN && r.Device != DMSMACDeviceBluetooth {
		return Request{}, fmt.Errorf("DMS MAC device %d is out of range", r.Device)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSGetMACAddress,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Device))},
	}, nil
}

// DMSGetMACAddressResponse contains an optional hardware address.
type DMSGetMACAddressResponse struct {
	Address net.HardwareAddr
	Known   bool
}

// UnmarshalTLVs parses the length-prefixed hardware address.
func (r *DMSGetMACAddressResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetMACAddressResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) == 0 {
		return errors.New("parsing QMI DMS MAC address: address length is missing")
	}
	length := int(value[0])
	if length == 0 || length > dmsMACAddressMax {
		return fmt.Errorf("parsing QMI DMS MAC address: address length %d is out of range", length)
	}
	if len(value) != 1+length {
		return fmt.Errorf("parsing QMI DMS MAC address: TLV length %d, want %d", len(value), 1+length)
	}
	r.Address = append(net.HardwareAddr(nil), value[1:]...)
	r.Known = true
	return nil
}

// DMSReset resets state owned by this DMS client.
func (c *Client) DMSReset(ctx context.Context) error {
	if err := c.dmsRead(ctx, MessageDMSReset, nil); err != nil {
		return fmt.Errorf("resetting QMI DMS control point: %w", err)
	}
	return nil
}

// DMSPRLVersion returns the active Preferred Roaming List version.
func (c *Client) DMSPRLVersion(ctx context.Context) (DMSPRLVersion, error) {
	var result DMSGetPRLVersionResponse
	if err := c.dmsRead(ctx, MessageDMSGetPRLVersion, &result); err != nil {
		return DMSPRLVersion{}, fmt.Errorf("querying QMI DMS PRL version: %w", err)
	}
	return result.Info, nil
}

// DMSSetTime sets the user clock in milliseconds since the GPS epoch.
func (c *Client) DMSSetTime(ctx context.Context, milliseconds uint64) error {
	req, err := (DMSSetTimeRequest{
		Timeout:      DefaultRequestTimeout,
		Milliseconds: milliseconds,
		Reference:    DMSTimeReferenceUser,
	}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DMS device time: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI DMS device time: %w", err)
	}
	return nil
}

// DMSAltNetworkConfig returns the legacy alternate-network configuration.
func (c *Client) DMSAltNetworkConfig(ctx context.Context) (bool, error) {
	var result DMSGetAltNetworkConfigResponse
	if err := c.dmsRead(ctx, MessageDMSGetAltNetworkConfig, &result); err != nil {
		return false, fmt.Errorf("querying QMI DMS alternate network config: %w", err)
	}
	return result.Enabled, nil
}

// DMSSetAltNetworkConfig updates the legacy alternate-network configuration.
func (c *Client) DMSSetAltNetworkConfig(ctx context.Context, enabled bool) error {
	req := DMSSetAltNetworkConfigRequest{
		Timeout: DefaultRequestTimeout,
		Enabled: enabled,
	}.Request()
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI DMS alternate network config: %w", err)
	}
	return nil
}

// DMSMACAddress returns the factory address for WLAN or Bluetooth.
func (c *Client) DMSMACAddress(ctx context.Context, device DMSMACDevice) (net.HardwareAddr, error) {
	req, err := (DMSGetMACAddressRequest{
		Timeout: DefaultRequestTimeout,
		Device:  device,
	}).Request()
	if err != nil {
		return nil, fmt.Errorf("querying QMI DMS MAC address: %w", err)
	}
	var result DMSGetMACAddressResponse
	if err := c.dmsReadRequest(ctx, req, &result); err != nil {
		return nil, fmt.Errorf("querying QMI DMS MAC address: %w", err)
	}
	if !result.Known {
		return nil, errors.New("querying QMI DMS MAC address: address TLV missing")
	}
	return result.Address, nil
}
