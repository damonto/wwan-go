package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVDeviceServiceCapability    = 0x10
	dmsTLVVoiceSupportCapability     = 0x11
	dmsTLVSimultaneousVoiceData      = 0x12
	dmsTLVCurrentMultiSIMCapability  = 0x14
	dmsTLVCurrentSubscriptionCaps    = 0x15
	dmsTLVSubscriptionVoiceDataCaps  = 0x16
	dmsTLVSubscriptionFeatureModes   = 0x17
	dmsTLVMaxActiveDataSubscriptions = 0x18
	dmsTLVMaximumSubscriptionCaps    = 0x19
	dmsTLVIMSCapabilities            = 0x1C
	dmsTLVMaxIMSInstances            = 0x1D

	dmsMaxRadioInterfaces = 20
	dmsMaxSubscriptions   = 6
)

// DMSDataServiceCapability is the legacy circuit/packet service capability.
type DMSDataServiceCapability uint8

const (
	DMSDataServiceNone DMSDataServiceCapability = iota
	DMSDataServiceCSOnly
	DMSDataServicePSOnly
	DMSDataServiceSimultaneousCSPS
	DMSDataServiceNonSimultaneousCSPS
)

// DMSSIMCapability reports whether the device supports a SIM.
type DMSSIMCapability uint8

const (
	DMSSIMNotSupported DMSSIMCapability = 1 + iota
	DMSSIMSupported
)

// DMSRadioInterface identifies a radio interface supported by the modem.
type DMSRadioInterface uint8

const (
	DMSRadioInterfaceCDMA1X  DMSRadioInterface = 0x01
	DMSRadioInterfaceEVDO    DMSRadioInterface = 0x02
	DMSRadioInterfaceGSM     DMSRadioInterface = 0x04
	DMSRadioInterfaceUMTS    DMSRadioInterface = 0x05
	DMSRadioInterfaceLTE     DMSRadioInterface = 0x08
	DMSRadioInterfaceTDSCDMA DMSRadioInterface = 0x09
	DMSRadioInterfaceNR5G    DMSRadioInterface = 0x0A
)

// DMSDeviceServiceCapability describes voice and data concurrency.
type DMSDeviceServiceCapability uint32

const (
	DMSDeviceServiceDataOnly DMSDeviceServiceCapability = 1 + iota
	DMSDeviceServiceVoiceOnly
	DMSDeviceServiceSimultaneousVoiceData
	DMSDeviceServiceNonSimultaneousVoiceData
)

// DMSVoiceSupportCapability is the voice technology support mask.
type DMSVoiceSupportCapability uint64

const (
	DMSVoiceSupportGWCSFB DMSVoiceSupportCapability = 1 << iota
	DMSVoiceSupportCDMA1XCSFB
	DMSVoiceSupportVoLTE
)

// DMSSimultaneousVoiceDataCapability is the simultaneous voice/data mask.
type DMSSimultaneousVoiceDataCapability uint64

const (
	DMSSimultaneousVoiceDataSVLTE DMSSimultaneousVoiceDataCapability = 1 << iota
	DMSSimultaneousVoiceDataSVDO
	DMSSimultaneousVoiceDataSGLTE
)

// DMSSubscriptionCapability is the RAT capability mask for one subscription.
type DMSSubscriptionCapability uint64

const (
	DMSSubscriptionCapabilityAMPS DMSSubscriptionCapability = 1 << iota
	DMSSubscriptionCapabilityCDMA
	DMSSubscriptionCapabilityHDR
	DMSSubscriptionCapabilityGSM
	DMSSubscriptionCapabilityWCDMA
	DMSSubscriptionCapabilityLTE
	DMSSubscriptionCapabilityTDSCDMA
	DMSSubscriptionCapabilityNR5G
)

// DMSSubscriptionVoiceDataCapability describes one subscription's voice/data mode.
type DMSSubscriptionVoiceDataCapability uint32

const (
	DMSSubscriptionVoiceDataNormal DMSSubscriptionVoiceDataCapability = 1 + iota
	DMSSubscriptionVoiceDataSGLTE
	DMSSubscriptionVoiceDataCSFB
	DMSSubscriptionVoiceDataSVLTE
	DMSSubscriptionVoiceDataSRLTE
)

// DMSSubscriptionVoiceData contains the mode and concurrency flag for one subscription.
type DMSSubscriptionVoiceData struct {
	Capability DMSSubscriptionVoiceDataCapability
	Concurrent bool
}

// DMSSubscriptionFeatureMode identifies a modem subscription feature mode.
type DMSSubscriptionFeatureMode uint32

const (
	DMSSubscriptionFeatureNormal DMSSubscriptionFeatureMode = iota
	DMSSubscriptionFeatureSGLTE
	DMSSubscriptionFeatureSVLTE
	DMSSubscriptionFeatureSRLTE
	DMSSubscriptionFeatureDualMultimode
)

// DMSMultiSIMCapability contains the current subscription concurrency limits.
type DMSMultiSIMCapability struct {
	MaxSubscriptions uint8
	MaxActive        uint8
}

// DMSIMSCapability reports IMS support for one subscription.
type DMSIMSCapability struct {
	Subscription DMSSubscription
	Enabled      bool
}

// DMSDeviceCapabilities contains the commonly useful QMI DMS device capabilities.
//
// Qualcomm also exposes deprecated configuration lists and detailed RAT release
// data in this response. Those optional TLVs are intentionally ignored here.
type DMSDeviceCapabilities struct {
	MaxTXRate uint32
	MaxRXRate uint32

	// DataService is deprecated by Qualcomm in favor of DeviceService.
	DataService     DMSDataServiceCapability
	SIM             DMSSIMCapability
	RadioInterfaces []DMSRadioInterface

	DeviceService              DMSDeviceServiceCapability
	DeviceServiceKnown         bool
	VoiceSupport               DMSVoiceSupportCapability
	VoiceSupportKnown          bool
	SimultaneousVoiceData      DMSSimultaneousVoiceDataCapability
	SimultaneousVoiceDataKnown bool

	CurrentMultiSIM      DMSMultiSIMCapability
	CurrentMultiSIMKnown bool

	CurrentSubscriptionCapabilities      []DMSSubscriptionCapability
	CurrentSubscriptionCapabilitiesKnown bool
	SubscriptionVoiceData                []DMSSubscriptionVoiceData
	SubscriptionVoiceDataKnown           bool
	SubscriptionFeatureModes             []DMSSubscriptionFeatureMode
	SubscriptionFeatureModesKnown        bool

	MaxActiveDataSubscriptions           uint8
	MaxActiveDataSubscriptionsKnown      bool
	MaximumSubscriptionCapabilities      []DMSSubscriptionCapability
	MaximumSubscriptionCapabilitiesKnown bool

	IMSCapabilities      []DMSIMSCapability
	IMSCapabilitiesKnown bool
	MaxIMSInstances      uint8
	MaxIMSInstancesKnown bool
}

// UnmarshalTLVs parses the common QMI DMS Get Device Capabilities TLVs.
func (r *DMSDeviceCapabilities) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSDeviceCapabilities{}
	value, ok := tlv.Value(tlvs, dmsTLVPrimaryValue)
	if !ok {
		return errors.New("parsing QMI DMS device capabilities: capabilities TLV missing")
	}
	if err := r.unmarshalCore(value); err != nil {
		return err
	}

	if value, ok := tlv.Value(tlvs, dmsTLVDeviceServiceCapability); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI DMS device capabilities: device-service TLV length %d, want 4", len(value))
		}
		r.DeviceService = DMSDeviceServiceCapability(binary.LittleEndian.Uint32(value))
		r.DeviceServiceKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVVoiceSupportCapability); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS device capabilities: voice-support TLV length %d, want 8", len(value))
		}
		r.VoiceSupport = DMSVoiceSupportCapability(binary.LittleEndian.Uint64(value))
		r.VoiceSupportKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSimultaneousVoiceData); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI DMS device capabilities: simultaneous voice/data TLV length %d, want 8", len(value))
		}
		r.SimultaneousVoiceData = DMSSimultaneousVoiceDataCapability(binary.LittleEndian.Uint64(value))
		r.SimultaneousVoiceDataKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVCurrentMultiSIMCapability); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI DMS device capabilities: current multi-SIM TLV length %d, want 2", len(value))
		}
		r.CurrentMultiSIM = DMSMultiSIMCapability{MaxSubscriptions: value[0], MaxActive: value[1]}
		r.CurrentMultiSIMKnown = true
	}

	if value, ok := tlv.Value(tlvs, dmsTLVCurrentSubscriptionCaps); ok {
		capabilities, err := parseDMSSubscriptionCapabilities(value)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS current subscription capabilities: %w", err)
		}
		r.CurrentSubscriptionCapabilities = capabilities
		r.CurrentSubscriptionCapabilitiesKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSubscriptionVoiceDataCaps); ok {
		capabilities, err := parseDMSSubscriptionVoiceData(value)
		if err != nil {
			return err
		}
		r.SubscriptionVoiceData = capabilities
		r.SubscriptionVoiceDataKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVSubscriptionFeatureModes); ok {
		modes, err := parseDMSSubscriptionFeatureModes(value)
		if err != nil {
			return err
		}
		r.SubscriptionFeatureModes = modes
		r.SubscriptionFeatureModesKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVMaxActiveDataSubscriptions); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DMS device capabilities: max active data subscriptions TLV length %d, want 1", len(value))
		}
		r.MaxActiveDataSubscriptions = value[0]
		r.MaxActiveDataSubscriptionsKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVMaximumSubscriptionCaps); ok {
		capabilities, err := parseDMSSubscriptionCapabilities(value)
		if err != nil {
			return fmt.Errorf("parsing QMI DMS maximum subscription capabilities: %w", err)
		}
		r.MaximumSubscriptionCapabilities = capabilities
		r.MaximumSubscriptionCapabilitiesKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVIMSCapabilities); ok {
		capabilities, err := parseDMSIMSCapabilities(value)
		if err != nil {
			return err
		}
		r.IMSCapabilities = capabilities
		r.IMSCapabilitiesKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVMaxIMSInstances); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DMS device capabilities: max IMS instances TLV length %d, want 1", len(value))
		}
		r.MaxIMSInstances = value[0]
		r.MaxIMSInstancesKnown = true
	}
	return nil
}

func (r *DMSDeviceCapabilities) unmarshalCore(value []byte) error {
	if len(value) < 11 {
		return fmt.Errorf("parsing QMI DMS device capabilities: core aggregate length %d, want at least 11", len(value))
	}
	radioCount := int(value[10])
	if radioCount > dmsMaxRadioInterfaces {
		return fmt.Errorf("parsing QMI DMS device capabilities: radio interface count %d exceeds maximum %d", radioCount, dmsMaxRadioInterfaces)
	}
	want := 11 + radioCount
	if len(value) != want {
		return fmt.Errorf("parsing QMI DMS device capabilities: core aggregate length %d, want %d", len(value), want)
	}
	r.MaxTXRate = binary.LittleEndian.Uint32(value[:4])
	r.MaxRXRate = binary.LittleEndian.Uint32(value[4:8])
	r.DataService = DMSDataServiceCapability(value[8])
	r.SIM = DMSSIMCapability(value[9])
	r.RadioInterfaces = make([]DMSRadioInterface, radioCount)
	for i, radio := range value[11:] {
		r.RadioInterfaces[i] = DMSRadioInterface(radio)
	}
	return nil
}

// DeviceCapabilities returns the modem's common data, voice, SIM, RAT, and IMS capabilities.
func (c *Client) DeviceCapabilities(ctx context.Context) (DMSDeviceCapabilities, error) {
	var result DMSDeviceCapabilities
	if err := c.dmsRead(ctx, MessageDMSGetDeviceCapabilities, &result); err != nil {
		return DMSDeviceCapabilities{}, fmt.Errorf("querying QMI DMS device capabilities: %w", err)
	}
	return result, nil
}

func parseDMSSubscriptionCapabilities(value []byte) ([]DMSSubscriptionCapability, error) {
	if len(value) < 1 {
		return nil, errors.New("count is truncated")
	}
	count := int(value[0])
	if count > dmsMaxSubscriptions {
		return nil, fmt.Errorf("count %d exceeds maximum %d", count, dmsMaxSubscriptions)
	}
	want := 1 + count*8
	if len(value) != want {
		return nil, fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	capabilities := make([]DMSSubscriptionCapability, count)
	for i := range count {
		offset := 1 + i*8
		capabilities[i] = DMSSubscriptionCapability(binary.LittleEndian.Uint64(value[offset : offset+8]))
	}
	return capabilities, nil
}

func parseDMSSubscriptionVoiceData(value []byte) ([]DMSSubscriptionVoiceData, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS subscription voice/data capabilities: count is truncated")
	}
	count := int(value[0])
	if count > dmsMaxSubscriptions {
		return nil, fmt.Errorf("parsing QMI DMS subscription voice/data capabilities: count %d exceeds maximum %d", count, dmsMaxSubscriptions)
	}
	want := 1 + count*5
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI DMS subscription voice/data capabilities: TLV length %d, want %d", len(value), want)
	}
	capabilities := make([]DMSSubscriptionVoiceData, count)
	for i := range count {
		offset := 1 + i*5
		capabilities[i] = DMSSubscriptionVoiceData{
			Capability: DMSSubscriptionVoiceDataCapability(binary.LittleEndian.Uint32(value[offset : offset+4])),
			Concurrent: value[offset+4] != 0,
		}
	}
	return capabilities, nil
}

func parseDMSSubscriptionFeatureModes(value []byte) ([]DMSSubscriptionFeatureMode, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS subscription feature modes: count is truncated")
	}
	count := int(value[0])
	if count > dmsMaxSubscriptions {
		return nil, fmt.Errorf("parsing QMI DMS subscription feature modes: count %d exceeds maximum %d", count, dmsMaxSubscriptions)
	}
	want := 1 + count*4
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI DMS subscription feature modes: TLV length %d, want %d", len(value), want)
	}
	modes := make([]DMSSubscriptionFeatureMode, count)
	for i := range count {
		offset := 1 + i*4
		modes[i] = DMSSubscriptionFeatureMode(binary.LittleEndian.Uint32(value[offset : offset+4]))
	}
	return modes, nil
}

func parseDMSIMSCapabilities(value []byte) ([]DMSIMSCapability, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS IMS capabilities: count is truncated")
	}
	count := int(value[0])
	if count > dmsMaxSubscriptions {
		return nil, fmt.Errorf("parsing QMI DMS IMS capabilities: count %d exceeds maximum %d", count, dmsMaxSubscriptions)
	}
	want := 1 + count*5
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI DMS IMS capabilities: TLV length %d, want %d", len(value), want)
	}
	capabilities := make([]DMSIMSCapability, count)
	for i := range count {
		offset := 1 + i*5
		capabilities[i] = DMSIMSCapability{
			Subscription: DMSSubscription(binary.LittleEndian.Uint32(value[offset : offset+4])),
			Enabled:      value[offset+4] != 0,
		}
	}
	return capabilities, nil
}
