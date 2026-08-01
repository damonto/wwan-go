package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type DeviceType uint32

const (
	DeviceTypeUnknown   DeviceType = 0
	DeviceTypeEmbedded  DeviceType = 1
	DeviceTypeRemovable DeviceType = 2
	DeviceTypeRemote    DeviceType = 3
)

type VoiceClass uint32

const (
	VoiceClassUnknown               VoiceClass = 0
	VoiceClassNoVoice               VoiceClass = 1
	VoiceClassSeparateVoiceData     VoiceClass = 2
	VoiceClassSimultaneousVoiceData VoiceClass = 3
)

type SIMClass uint32

const (
	SIMClassNone      SIMClass = 0
	SIMClassLogical   SIMClass = 1 << 0
	SIMClassRemovable SIMClass = 1 << 1
)

type SMSCaps uint32

const (
	SMSCapsNone        SMSCaps = 0
	SMSCapsPDUReceive  SMSCaps = 1 << 0
	SMSCapsPDUSend     SMSCaps = 1 << 1
	SMSCapsTextReceive SMSCaps = 1 << 2
	SMSCapsTextSend    SMSCaps = 1 << 3
)

type ControlCaps uint32

const (
	ControlCapsNone                   ControlCaps = 0
	ControlCapsManualRegistration     ControlCaps = 1 << 0
	ControlCapsHardwareRadioSwitch    ControlCaps = 1 << 1
	ControlCapsCDMAMobileIP           ControlCaps = 1 << 2
	ControlCapsCDMASimpleIP           ControlCaps = 1 << 3
	ControlCapsMultiCarrier           ControlCaps = 1 << 4
	ControlCapsESIM                   ControlCaps = 1 << 5
	ControlCapsUEPolicyRouteSelection ControlCaps = 1 << 6
	ControlCapsSIMHotSwap             ControlCaps = 1 << 7
	ControlCapsUseURSPRuleOnEPC       ControlCaps = 1 << 8
)

const deviceCapsMaximumSessions = 256

// DeviceCapsRequest queries the mandatory MBIM Basic Connect device capabilities.
type DeviceCapsRequest struct {
	TransactionID uint32
	Response      *DeviceCapsInfo
}

func (r *DeviceCapsRequest) Request() *Request {
	r.Response = new(DeviceCapsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDDeviceCaps,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

// DeviceCapsV2Request queries the MBIM Basic Connect Extensions capability
// structure negotiated by MBIMEx version.
type DeviceCapsV2Request struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *DeviceCapsInfo
}

func (r *DeviceCapsV2Request) Request() *Request {
	r.Response = &DeviceCapsInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsDeviceCapsV2,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

// DeviceCapsInfo contains the IP-session capacity needed before MBIM_CID_CONNECT.
type DeviceCapsInfo struct {
	MBIMExVersion   uint16
	DeviceType      DeviceType
	CellularClass   CellularClass
	VoiceClass      VoiceClass
	SIMClass        SIMClass
	DataClass       DataClass
	DataSubclass    DataSubclass
	SMSCaps         SMSCaps
	ControlCaps     ControlCaps
	MaxSessions     uint32
	ExecutorIndex   uint32
	WCDMABandClass  uint32
	LTEBandClasses  []uint16
	NRBandClasses   []uint16
	CustomDataClass string
	DeviceID        string
	FirmwareInfo    string
	HardwareInfo    string
	TLVs            TLVs
}

func (r *DeviceCapsInfo) UnmarshalBinary(data []byte) error {
	version := r.MBIMExVersion
	switch {
	case version >= mbimExVersion30:
		return r.unmarshalV3(data, version)
	case version >= mbimExVersion20:
		return r.unmarshalV2(data, version)
	default:
		return r.unmarshalV1(data)
	}
}

func (r *DeviceCapsInfo) unmarshalV1(data []byte) error {
	if len(data) < 64 {
		return errors.New("parsing MBIM device capabilities: payload is truncated")
	}
	result := DeviceCapsInfo{
		DeviceType:    DeviceType(binary.LittleEndian.Uint32(data[0:4])),
		CellularClass: CellularClass(binary.LittleEndian.Uint32(data[4:8])),
		VoiceClass:    VoiceClass(binary.LittleEndian.Uint32(data[8:12])),
		SIMClass:      SIMClass(binary.LittleEndian.Uint32(data[12:16])),
		DataClass:     DataClass(binary.LittleEndian.Uint32(data[16:20])),
		SMSCaps:       SMSCaps(binary.LittleEndian.Uint32(data[20:24])),
		ControlCaps:   ControlCaps(binary.LittleEndian.Uint32(data[24:28])),
	}
	if err := result.validate(); err != nil {
		return fmt.Errorf("parsing MBIM device capabilities: %w", err)
	}
	maxSessions := binary.LittleEndian.Uint32(data[28:32])
	if maxSessions > deviceCapsMaximumSessions {
		return fmt.Errorf("parsing MBIM device capabilities: maximum sessions %d exceeds %d", maxSessions, deviceCapsMaximumSessions)
	}
	refs, err := deviceCapsStringRefs(data, 64, []uint32{32, 40, 48, 56})
	if err != nil {
		return err
	}
	values, err := deviceCapsStrings(data, refs)
	if err != nil {
		return err
	}

	result.MaxSessions = maxSessions
	result.CustomDataClass = values[0]
	result.DeviceID = values[1]
	result.FirmwareInfo = values[2]
	result.HardwareInfo = values[3]
	*r = result
	return nil
}

func (r *DeviceCapsInfo) unmarshalV2(data []byte, version uint16) error {
	if len(data) < 68 {
		return errors.New("parsing MBIMEx device capabilities: payload is truncated")
	}
	result := DeviceCapsInfo{
		MBIMExVersion: version,
		DeviceType:    DeviceType(binary.LittleEndian.Uint32(data[0:4])),
		CellularClass: CellularClass(binary.LittleEndian.Uint32(data[4:8])),
		VoiceClass:    VoiceClass(binary.LittleEndian.Uint32(data[8:12])),
		SIMClass:      SIMClass(binary.LittleEndian.Uint32(data[12:16])),
		DataClass:     DataClass(binary.LittleEndian.Uint32(data[16:20])),
		SMSCaps:       SMSCaps(binary.LittleEndian.Uint32(data[20:24])),
		ControlCaps:   ControlCaps(binary.LittleEndian.Uint32(data[24:28])),
	}
	if err := result.validate(); err != nil {
		return fmt.Errorf("parsing MBIMEx device capabilities: %w", err)
	}
	maxSessions := binary.LittleEndian.Uint32(data[28:32])
	if maxSessions > deviceCapsMaximumSessions {
		return fmt.Errorf("parsing MBIMEx device capabilities: maximum sessions %d exceeds %d", maxSessions, deviceCapsMaximumSessions)
	}
	refs, err := deviceCapsStringRefs(data, 68, []uint32{32, 40, 48, 56})
	if err != nil {
		return err
	}
	values, err := deviceCapsStrings(data, refs)
	if err != nil {
		return err
	}

	result.MaxSessions = maxSessions
	result.ExecutorIndex = binary.LittleEndian.Uint32(data[64:68])
	result.CustomDataClass = values[0]
	result.DeviceID = values[1]
	result.FirmwareInfo = values[2]
	result.HardwareInfo = values[3]
	*r = result
	return nil
}

func (r *DeviceCapsInfo) unmarshalV3(data []byte, version uint16) error {
	if len(data) < 48 {
		return errors.New("parsing MBIMEx device capabilities: payload is truncated")
	}
	result := DeviceCapsInfo{
		MBIMExVersion: version,
		DeviceType:    DeviceType(binary.LittleEndian.Uint32(data[0:4])),
		CellularClass: CellularClass(binary.LittleEndian.Uint32(data[4:8])),
		VoiceClass:    VoiceClass(binary.LittleEndian.Uint32(data[8:12])),
		SIMClass:      SIMClass(binary.LittleEndian.Uint32(data[12:16])),
		DataClass:     DataClass(binary.LittleEndian.Uint32(data[16:20])),
		SMSCaps:       SMSCaps(binary.LittleEndian.Uint32(data[20:24])),
		ControlCaps:   ControlCaps(binary.LittleEndian.Uint32(data[24:28])),
		DataSubclass:  DataSubclass(binary.LittleEndian.Uint64(data[28:36])),
	}
	if err := result.validate(); err != nil {
		return fmt.Errorf("parsing MBIMEx device capabilities: %w", err)
	}

	const namedTLVCount = 6
	values := make(TLVs, 0, namedTLVCount)
	rest := data[48:]
	for i := range namedTLVCount {
		tlv, consumed, err := unmarshalTLVPrefix(rest)
		if err != nil {
			return fmt.Errorf("parsing MBIMEx device capabilities TLV %d: %w", i, err)
		}
		values = append(values, tlv)
		rest = rest[consumed:]
	}
	if len(rest) > 0 {
		var optional TLVs
		if err := optional.UnmarshalBinary(rest); err != nil {
			return fmt.Errorf("parsing MBIMEx device capabilities optional TLVs: %w", err)
		}
		values = append(values, optional...)
	}

	wantTypes := [...]TLVType{
		TLVTypeUint16Table,
		TLVTypeUint16Table,
		TLVTypeWCharString,
		TLVTypeWCharString,
		TLVTypeWCharString,
		TLVTypeWCharString,
	}
	for i, want := range wantTypes {
		if values[i].Type != want {
			return fmt.Errorf("parsing MBIMEx device capabilities TLV %d: type is %d, want %d", i, values[i].Type, want)
		}
	}

	lteBands, err := decodeUint16Table(values[0].Data)
	if err != nil {
		return fmt.Errorf("parsing MBIMEx device capabilities LTE bands: %w", err)
	}
	nrBands, err := decodeUint16Table(values[1].Data)
	if err != nil {
		return fmt.Errorf("parsing MBIMEx device capabilities NR bands: %w", err)
	}
	strings := make([]string, 4)
	maximumSizes := [...]uint32{22, 36, 60, 60}
	for i := range strings {
		if uint32(len(values[i+2].Data)) > maximumSizes[i] {
			return fmt.Errorf("parsing MBIMEx device capabilities string %d: size %d exceeds %d bytes", i, len(values[i+2].Data), maximumSizes[i])
		}
		value, err := utf16RawString(values[i+2].Data)
		if err != nil {
			return fmt.Errorf("parsing MBIMEx device capabilities string %d: %w", i, err)
		}
		strings[i] = value
	}

	maxSessions := binary.LittleEndian.Uint32(data[36:40])
	if maxSessions > deviceCapsMaximumSessions {
		return fmt.Errorf("parsing MBIMEx device capabilities: maximum sessions %d exceeds %d", maxSessions, deviceCapsMaximumSessions)
	}
	result.MaxSessions = maxSessions
	result.ExecutorIndex = binary.LittleEndian.Uint32(data[40:44])
	result.WCDMABandClass = binary.LittleEndian.Uint32(data[44:48])
	result.LTEBandClasses = lteBands
	result.NRBandClasses = nrBands
	result.CustomDataClass = strings[0]
	result.DeviceID = strings[1]
	result.FirmwareInfo = strings[2]
	result.HardwareInfo = strings[3]
	result.TLVs = values
	*r = result
	return nil
}

func (info *DeviceCapsInfo) validate() error {
	if info.DeviceType > DeviceTypeRemote {
		return fmt.Errorf("device type %d is outside 0..%d", info.DeviceType, DeviceTypeRemote)
	}
	if !validCellularClass(info.CellularClass) {
		return fmt.Errorf("cellular class %#x contains reserved bits", info.CellularClass)
	}
	if info.VoiceClass > VoiceClassSimultaneousVoiceData {
		return fmt.Errorf("voice class %d is outside 0..%d", info.VoiceClass, VoiceClassSimultaneousVoiceData)
	}
	if info.SIMClass&^simClassMask != 0 {
		return fmt.Errorf("SIM class %#x contains reserved bits", info.SIMClass)
	}
	if !validDataClass(info.MBIMExVersion, info.DataClass) {
		return fmt.Errorf("data class %#x contains bits reserved in MBIMEx %#x", info.DataClass, info.MBIMExVersion)
	}
	if info.SMSCaps&^smsCapsMask != 0 {
		return fmt.Errorf("SMS capabilities %#x contain reserved bits", info.SMSCaps)
	}
	if !validControlCaps(info.MBIMExVersion, info.ControlCaps) {
		return fmt.Errorf("control capabilities %#x contain bits reserved in MBIMEx %#x", info.ControlCaps, info.MBIMExVersion)
	}
	if info.MBIMExVersion < mbimExVersion30 {
		return nil
	}
	if !validDataSubclass(info.DataSubclass) {
		return fmt.Errorf("data subclass %#x contains reserved bits", info.DataSubclass)
	}
	if dataClassHas5G(info.MBIMExVersion, info.DataClass) != (info.DataSubclass != DataSubclassNone) {
		return fmt.Errorf("5G data class %#x and data subclass %#x are inconsistent", info.DataClass, info.DataSubclass)
	}
	return nil
}

func deviceCapsStringRefs(data []byte, dataStart uint32, offsets []uint32) ([]valueRef, error) {
	refs := make([]valueRef, len(offsets))
	maximumSizes := [...]uint32{22, 36, 60, 60}
	for i, fieldOffset := range offsets {
		ref, err := readOffsetSizeRef(data, fieldOffset)
		if err != nil {
			return nil, fmt.Errorf("parsing MBIM device capabilities string %d: %w", i, err)
		}
		if ref.size > maximumSizes[i] {
			return nil, fmt.Errorf("parsing MBIM device capabilities string %d: size %d exceeds %d bytes", i, ref.size, maximumSizes[i])
		}
		refs[i] = ref
	}
	if err := validateDataBufferRefs(data, dataStart, refs); err != nil {
		return nil, fmt.Errorf("parsing MBIM device capabilities data buffer: %w", err)
	}
	if err := validateUTF16Refs(data, refs); err != nil {
		return nil, fmt.Errorf("parsing MBIM device capabilities strings: %w", err)
	}
	return refs, nil
}

func deviceCapsStrings(data []byte, refs []valueRef) ([]string, error) {
	values := make([]string, len(refs))
	for i, ref := range refs {
		value, err := utf16String(data, ref)
		if err != nil {
			return nil, fmt.Errorf("parsing MBIMEx device capabilities string %d: %w", i, err)
		}
		values[i] = value
	}
	return values, nil
}

func decodeUint16Table(data []byte) ([]uint16, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("UINT16 table has odd byte length")
	}
	values := make([]uint16, len(data)/2)
	for i := range values {
		values[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return values, nil
}

// DeviceCaps reads the modem's IP-session capacity.
func (c *Client) DeviceCaps(ctx context.Context) (DeviceCapsInfo, error) {
	if c.mbimExVersion >= mbimExVersion20 {
		request := DeviceCapsV2Request{
			TransactionID: c.nextTransactionID(),
			MBIMExVersion: c.mbimExVersion,
		}
		if err := c.transmit(ctx, request.Request()); err != nil {
			return DeviceCapsInfo{}, fmt.Errorf("reading MBIMEx device capabilities: %w", err)
		}
		response := *request.Response
		response.LTEBandClasses = slices.Clone(response.LTEBandClasses)
		response.NRBandClasses = slices.Clone(response.NRBandClasses)
		return response, nil
	}

	request := DeviceCapsRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return DeviceCapsInfo{}, fmt.Errorf("reading MBIM device capabilities: %w", err)
	}
	return *request.Response, nil
}
