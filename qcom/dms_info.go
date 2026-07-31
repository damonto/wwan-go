package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVPrimaryValue = 0x01

	dmsTLVRevisionBootCode = 0x10
	dmsTLVRevisionPRI      = 0x11

	dmsTLVSerialESN    = 0x10
	dmsTLVSerialIMEI   = 0x11
	dmsTLVSerialMEID   = 0x12
	dmsTLVSerialIMEISV = 0x13

	dmsTLVSystemTime = 0x10
	dmsTLVUserTime   = 0x11

	dmsTLVDeprecatedLTEBandMask = 0x10
	dmsTLVTDSBandMask           = 0x11
	dmsTLVSupportedLTEBands     = 0x12
	dmsTLVSupportedNR5GBands    = 0x13

	dmsTLVImageVersions            = 0x10
	dmsTLVExtendedSoftwareVersion  = 0x11
	dmsTLVSecondarySoftwareVersion = 0x12
	dmsTLVBoundSubscription        = 0x10

	dmsMaxEncryptedSerialNumber = 255
	dmsMaxImageVersions         = 32
	dmsMaxImageVersion          = 128
	dmsMaxLTEBands              = 256
	dmsMaxNR5GBands             = 512
)

// DMSSubscription identifies a modem subscription for the DMS service.
type DMSSubscription uint32

const (
	DMSSubscriptionPrimary DMSSubscription = 1 + iota
	DMSSubscriptionSecondary
	DMSSubscriptionTertiary
)

// DMSRevisionInfo contains firmware, boot-code, and PRI revisions.
type DMSRevisionInfo struct {
	Revision      string
	BootCode      string
	BootCodeKnown bool
	PRI           string
	PRIKnown      bool
}

// UnmarshalTLVs parses QMI DMS Get Device Revision ID response TLVs.
func (r *DMSRevisionInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSRevisionInfo{}
	revision, err := dmsRequiredString(tlvs, dmsTLVPrimaryValue)
	if err != nil {
		return fmt.Errorf("parsing QMI DMS revision: %w", err)
	}
	r.Revision = revision
	if value, ok := tlv.Value(tlvs, dmsTLVRevisionBootCode); ok {
		r.BootCode = string(value)
		r.BootCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVRevisionPRI); ok {
		r.PRI = string(value)
		r.PRIKnown = true
	}
	return nil
}

// DMSSerialNumbers contains the hardware identifiers reported by DMS.
type DMSSerialNumbers struct {
	ESN         string
	ESNKnown    bool
	IMEI        string
	IMEIKnown   bool
	MEID        string
	MEIDKnown   bool
	IMEISV      string
	IMEISVKnown bool
}

// UnmarshalTLVs parses QMI DMS Get Device Serial Numbers response TLVs.
func (r *DMSSerialNumbers) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSSerialNumbers{}
	if value, ok := tlv.Value(tlvs, dmsTLVSerialESN); ok {
		r.ESN = string(value)
		r.ESNKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSerialIMEI); ok {
		r.IMEI = string(value)
		r.IMEIKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSerialMEID); ok {
		r.MEID = string(value)
		r.MEIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSerialIMEISV); ok {
		r.IMEISV = string(value)
		r.IMEISVKnown = true
	}
	return nil
}

// DMSEncryptedSerialNumbers contains the opaque encrypted device identifiers.
type DMSEncryptedSerialNumbers struct {
	ESN         []byte
	ESNKnown    bool
	IMEI        []byte
	IMEIKnown   bool
	MEID        []byte
	MEIDKnown   bool
	IMEISV      []byte
	IMEISVKnown bool
}

// UnmarshalTLVs parses QMI DMS Get Encrypted Device Serial Numbers response TLVs.
func (r *DMSEncryptedSerialNumbers) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSEncryptedSerialNumbers{}
	fields := []struct {
		typ   uint8
		value *[]byte
		known *bool
	}{
		{dmsTLVSerialESN, &r.ESN, &r.ESNKnown},
		{dmsTLVSerialIMEI, &r.IMEI, &r.IMEIKnown},
		{dmsTLVSerialMEID, &r.MEID, &r.MEIDKnown},
		{dmsTLVSerialIMEISV, &r.IMEISV, &r.IMEISVKnown},
	}
	for _, field := range fields {
		value, ok := tlv.Value(tlvs, field.typ)
		if !ok {
			continue
		}
		if len(value) > dmsMaxEncryptedSerialNumber {
			return fmt.Errorf("parsing QMI DMS encrypted serial number TLV 0x%02X: length %d exceeds maximum %d", field.typ, len(value), dmsMaxEncryptedSerialNumber)
		}
		*field.value = slices.Clone(value)
		*field.known = true
	}
	return nil
}

// DMSBindSubscriptionRequest encodes Bind Subscription.
type DMSBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  DMSSubscription
}

// Request validates and converts the subscription into a QMI request.
func (r DMSBindSubscriptionRequest) Request() (Request, error) {
	if !validDMSSubscription(r.Subscription) {
		return Request{}, fmt.Errorf("DMS subscription %d is out of range", r.Subscription)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSBindSubscription,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(dmsTLVPrimaryValue, uint32(r.Subscription))},
	}, nil
}

// DMSGetBindSubscriptionRequest encodes Get Bind Subscription.
type DMSGetBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSGetBindSubscriptionRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetBindSubscription)
}

// DMSGetBindSubscriptionResponse contains the subscription bound to the client.
type DMSGetBindSubscriptionResponse struct {
	Subscription      DMSSubscription
	SubscriptionKnown bool
}

// UnmarshalTLVs parses a Get Bind Subscription response.
func (r *DMSGetBindSubscriptionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetBindSubscriptionResponse{}
	value, ok := tlv.Value(tlvs, dmsTLVBoundSubscription)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI DMS bound subscription: TLV length %d, want 4", len(value))
	}
	r.Subscription = DMSSubscription(binary.LittleEndian.Uint32(value))
	r.SubscriptionKnown = true
	return nil
}

// DMSPowerStatus is the power-state bitmask reported by DMS.
type DMSPowerStatus uint8

const (
	DMSPowerExternalSource DMSPowerStatus = 1 << iota
	DMSPowerBatteryConnected
	DMSPowerBatteryCharging
	DMSPowerFault
)

// DMSPowerState contains the current power source and battery level.
type DMSPowerState struct {
	Status       DMSPowerStatus
	BatteryLevel uint8
}

// UnmarshalTLVs parses QMI DMS Get Power State response TLVs.
func (r *DMSPowerState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSPowerState{}
	value, ok := tlv.Value(tlvs, dmsTLVPrimaryValue)
	if !ok {
		return errors.New("parsing QMI DMS power state: power-state TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI DMS power state: power-state TLV length %d, want 2", len(value))
	}
	r.Status = DMSPowerStatus(value[0])
	r.BatteryLevel = value[1]
	return nil
}

// DMSTimeSource identifies the source used for a DMS device timestamp.
type DMSTimeSource uint16

const (
	DMSTimeSourceDeviceClock DMSTimeSource = iota
	DMSTimeSourceCDMA
	DMSTimeSourceHDR
)

// DMSDeviceTime contains raw GPS-epoch timestamps reported by DMS.
type DMSDeviceTime struct {
	// TimeCount is the number of 1.25 ms intervals since January 6, 1980.
	TimeCount uint64
	Source    DMSTimeSource

	SystemMilliseconds      uint64
	SystemMillisecondsKnown bool
	UserMilliseconds        uint64
	UserMillisecondsKnown   bool
}

// UnmarshalTLVs parses QMI DMS Get Time response TLVs.
func (r *DMSDeviceTime) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSDeviceTime{}
	value, ok := tlv.Value(tlvs, dmsTLVPrimaryValue)
	if !ok {
		return errors.New("parsing QMI DMS device time: device-time TLV missing")
	}
	if len(value) != 8 {
		return fmt.Errorf("parsing QMI DMS device time: device-time TLV length %d, want 8", len(value))
	}
	for i, b := range value[:6] {
		r.TimeCount |= uint64(b) << (8 * i)
	}
	r.Source = DMSTimeSource(binary.LittleEndian.Uint16(value[6:8]))

	if value, ok := tlv.Value(tlvs, dmsTLVSystemTime); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS device time: system-time TLV length %d, want 8", len(value))
		}
		r.SystemMilliseconds = binary.LittleEndian.Uint64(value)
		r.SystemMillisecondsKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVUserTime); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS device time: user-time TLV length %d, want 8", len(value))
		}
		r.UserMilliseconds = binary.LittleEndian.Uint64(value)
		r.UserMillisecondsKnown = true
	}
	return nil
}

// DMSBandCapabilities contains legacy masks and explicit LTE/NR5G bands.
type DMSBandCapabilities struct {
	BandMask uint64

	// LTEBandMask is deprecated by Qualcomm; use LTEBands when present.
	LTEBandMask      uint64
	LTEBandMaskKnown bool
	TDSBandMask      uint64
	TDSBandMaskKnown bool

	LTEBands       []uint16
	LTEBandsKnown  bool
	NR5GBands      []uint16
	NR5GBandsKnown bool
}

// UnmarshalTLVs parses QMI DMS Get Band Capability response TLVs.
func (r *DMSBandCapabilities) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSBandCapabilities{}
	value, ok := tlv.Value(tlvs, dmsTLVPrimaryValue)
	if !ok {
		return errors.New("parsing QMI DMS band capabilities: band-mask TLV missing")
	}
	if len(value) != 8 {
		return fmt.Errorf("parsing QMI DMS band capabilities: band-mask TLV length %d, want 8", len(value))
	}
	r.BandMask = binary.LittleEndian.Uint64(value)

	if value, ok := tlv.Value(tlvs, dmsTLVDeprecatedLTEBandMask); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS band capabilities: LTE band-mask TLV length %d, want 8", len(value))
		}
		r.LTEBandMask = binary.LittleEndian.Uint64(value)
		r.LTEBandMaskKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVTDSBandMask); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS band capabilities: TDS band-mask TLV length %d, want 8", len(value))
		}
		r.TDSBandMask = binary.LittleEndian.Uint64(value)
		r.TDSBandMaskKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSupportedLTEBands); ok {
		bands, err := parseDMSBandList(value, dmsMaxLTEBands)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS LTE bands: %w", err)
		}
		r.LTEBands = bands
		r.LTEBandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSupportedNR5GBands); ok {
		bands, err := parseDMSBandList(value, dmsMaxNR5GBands)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS NR5G bands: %w", err)
		}
		r.NR5GBands = bands
		r.NR5GBandsKnown = true
	}
	return nil
}

// DMSImageType identifies one firmware image returned by Get Software Version.
type DMSImageType uint32

const (
	DMSImageUnknown DMSImageType = iota
	DMSImageSBL
	DMSImageTZ
	DMSImageTZSecureApp
	DMSImageRPM
	DMSImageSDI
	DMSImageHypervisor
	DMSImageAPPSBL
	DMSImageApplications
	DMSImageMPSS
	DMSImageADSP
	DMSImageWCNS
	DMSImageVenus
)

// DMSImageVersion identifies the version of one device firmware image.
type DMSImageVersion struct {
	Type    DMSImageType
	Version string
}

// DMSSoftwareVersion contains the modem and component software versions.
type DMSSoftwareVersion struct {
	Version string

	Images         []DMSImageVersion
	ImagesKnown    bool
	Extended       string
	ExtendedKnown  bool
	Secondary      string
	SecondaryKnown bool
}

// UnmarshalTLVs parses QMI DMS Get Software Version response TLVs.
func (r *DMSSoftwareVersion) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSSoftwareVersion{}
	version, err := dmsRequiredString(tlvs, dmsTLVPrimaryValue)
	if err != nil {
		return fmt.Errorf("parsing QMI DMS software version: %w", err)
	}
	r.Version = version
	if value, ok := tlv.Value(tlvs, dmsTLVImageVersions); ok {
		images, err := parseDMSImageVersions(value)
		if err != nil {
			return err
		}
		r.Images = images
		r.ImagesKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVExtendedSoftwareVersion); ok {
		r.Extended = string(value)
		r.ExtendedKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSecondarySoftwareVersion); ok {
		r.Secondary = string(value)
		r.SecondaryKnown = true
	}
	return nil
}

// Manufacturer returns the modem manufacturer reported by QMI DMS.
func (c *Client) Manufacturer(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSGetManufacturer)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS manufacturer: %w", err)
	}
	return value, nil
}

// ModelID returns the modem model identifier reported by QMI DMS.
func (c *Client) ModelID(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSGetModelID)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS model ID: %w", err)
	}
	return value, nil
}

// RevisionInfo returns firmware, boot-code, and PRI revisions.
func (c *Client) RevisionInfo(ctx context.Context) (DMSRevisionInfo, error) {
	var result DMSRevisionInfo
	if err := c.dmsRead(ctx, MessageDMSGetRevisionID, &result); err != nil {
		return DMSRevisionInfo{}, fmt.Errorf("querying QMI DMS revision information: %w", err)
	}
	return result, nil
}

// SerialNumbers returns device ESN, IMEI, MEID, and IMEISV identifiers.
func (c *Client) SerialNumbers(ctx context.Context) (DMSSerialNumbers, error) {
	var result DMSSerialNumbers
	if err := c.dmsRead(ctx, MessageDMSGetSerialNumbers, &result); err != nil {
		return DMSSerialNumbers{}, fmt.Errorf("querying QMI DMS serial numbers: %w", err)
	}
	return result, nil
}

// EncryptedSerialNumbers returns opaque encrypted device identifiers.
func (c *Client) EncryptedSerialNumbers(ctx context.Context) (DMSEncryptedSerialNumbers, error) {
	var result DMSEncryptedSerialNumbers
	if err := c.dmsRead(ctx, MessageDMSGetEncryptedSerialNumbers, &result); err != nil {
		return DMSEncryptedSerialNumbers{}, fmt.Errorf("querying QMI DMS encrypted serial numbers: %w", err)
	}
	return result, nil
}

// PowerState returns the current device power and battery state.
func (c *Client) PowerState(ctx context.Context) (DMSPowerState, error) {
	var result DMSPowerState
	if err := c.dmsRead(ctx, MessageDMSGetPowerState, &result); err != nil {
		return DMSPowerState{}, fmt.Errorf("querying QMI DMS power state: %w", err)
	}
	return result, nil
}

// HardwareRevision returns the modem hardware revision.
func (c *Client) HardwareRevision(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSGetHardwareRevision)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS hardware revision: %w", err)
	}
	return value, nil
}

// DeviceTime returns raw GPS-epoch time values reported by the modem.
func (c *Client) DeviceTime(ctx context.Context) (DMSDeviceTime, error) {
	var result DMSDeviceTime
	if err := c.dmsRead(ctx, MessageDMSGetTime, &result); err != nil {
		return DMSDeviceTime{}, fmt.Errorf("querying QMI DMS device time: %w", err)
	}
	return result, nil
}

// BandCapabilities returns legacy masks and explicit LTE/NR5G bands.
func (c *Client) BandCapabilities(ctx context.Context) (DMSBandCapabilities, error) {
	var result DMSBandCapabilities
	if err := c.dmsRead(ctx, MessageDMSGetBandCapabilities, &result); err != nil {
		return DMSBandCapabilities{}, fmt.Errorf("querying QMI DMS band capabilities: %w", err)
	}
	return result, nil
}

// FactorySKU returns the factory-provisioned SKU string.
func (c *Client) FactorySKU(ctx context.Context) (string, error) {
	value, err := c.dmsString(ctx, MessageDMSGetFactorySKU)
	if err != nil {
		return "", fmt.Errorf("querying QMI DMS factory SKU: %w", err)
	}
	return value, nil
}

// SoftwareVersion returns modem and component software versions.
func (c *Client) SoftwareVersion(ctx context.Context) (DMSSoftwareVersion, error) {
	var result DMSSoftwareVersion
	if err := c.dmsRead(ctx, MessageDMSGetSoftwareVersion, &result); err != nil {
		return DMSSoftwareVersion{}, fmt.Errorf("querying QMI DMS software version: %w", err)
	}
	return result, nil
}

// DMSBindSubscription associates the DMS client with a modem subscription.
func (c *Client) DMSBindSubscription(ctx context.Context, subscription DMSSubscription) error {
	req, err := (DMSBindSubscriptionRequest{
		Timeout:      DefaultRequestTimeout,
		Subscription: subscription,
	}).Request()
	if err != nil {
		return fmt.Errorf("binding QMI DMS subscription: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("binding QMI DMS subscription: %w", err)
	}
	return nil
}

// DMSBoundSubscription returns the subscription associated with the DMS client.
func (c *Client) DMSBoundSubscription(ctx context.Context) (DMSGetBindSubscriptionResponse, error) {
	var result DMSGetBindSubscriptionResponse
	if err := c.dmsRead(ctx, MessageDMSGetBindSubscription, &result); err != nil {
		return DMSGetBindSubscriptionResponse{}, fmt.Errorf("querying QMI DMS bound subscription: %w", err)
	}
	return result, nil
}

func (c *Client) dmsString(ctx context.Context, message MessageID) (string, error) {
	var result dmsStringResponse
	err := c.dmsRead(ctx, message, &result)
	return string(result), err
}

type dmsStringResponse string

func (r *dmsStringResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, err := dmsRequiredString(tlvs, dmsTLVPrimaryValue)
	if err != nil {
		return err
	}
	*r = dmsStringResponse(value)
	return nil
}

func (c *Client) dmsRead(ctx context.Context, message MessageID, dst tlvUnmarshaler) error {
	req := dmsEmptyRequest(0, 0, DefaultRequestTimeout, message)
	return c.dmsReadRequest(ctx, req, dst)
}

func (c *Client) dmsReadRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, req.MessageID, req.TLVs, req.Timeout)
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

func dmsEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServiceDMS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func dmsRequiredString(tlvs tlv.TLVs, typ byte) (string, error) {
	value, ok := tlv.Value(tlvs, typ)
	if !ok {
		return "", errors.New("value TLV missing")
	}
	return string(value), nil
}

func validDMSSubscription(subscription DMSSubscription) bool {
	return subscription >= DMSSubscriptionPrimary && subscription <= DMSSubscriptionTertiary
}

func parseDMSBandList(value []byte, maxBands int) ([]uint16, error) {
	if len(value) < 2 {
		return nil, errors.New("count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > maxBands {
		return nil, fmt.Errorf("count %d exceeds maximum %d", count, maxBands)
	}
	want := 2 + count*2
	if len(value) != want {
		return nil, fmt.Errorf("TLV length %d, want %d for %d bands", len(value), want, count)
	}
	bands := make([]uint16, count)
	for i := range count {
		offset := 2 + i*2
		bands[i] = binary.LittleEndian.Uint16(value[offset : offset+2])
	}
	return bands, nil
}

func parseDMSImageVersions(value []byte) ([]DMSImageVersion, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS image versions: count is truncated")
	}
	count := int(value[0])
	if count > dmsMaxImageVersions {
		return nil, fmt.Errorf("parsing QMI DMS image versions: count %d exceeds maximum %d", count, dmsMaxImageVersions)
	}
	value = value[1:]
	images := make([]DMSImageVersion, 0, count)
	for range count {
		if len(value) < 5 {
			return nil, errors.New("parsing QMI DMS image versions: image header is truncated")
		}
		imageType := DMSImageType(binary.LittleEndian.Uint32(value[:4]))
		versionLength := int(value[4])
		value = value[5:]
		if versionLength > dmsMaxImageVersion {
			return nil, fmt.Errorf("parsing QMI DMS image versions: version length %d exceeds maximum %d", versionLength, dmsMaxImageVersion)
		}
		if len(value) < versionLength {
			return nil, errors.New("parsing QMI DMS image versions: version is truncated")
		}
		images = append(images, DMSImageVersion{
			Type:    imageType,
			Version: string(value[:versionLength]),
		})
		value = value[versionLength:]
	}
	if len(value) != 0 {
		return nil, fmt.Errorf("parsing QMI DMS image versions: %d trailing bytes", len(value))
	}
	return images, nil
}
