package mbim

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf16"
)

const (
	mbimCIDResponseTimeout     = 8 * time.Second
	mbimCIDLongResponseTimeout = 58 * time.Second
)

type ProxyConfigRequest struct {
	TransactionID uint32
	DevicePath    string
	Timeout       uint32
	Response      *ProxyConfigResponse
}

func (r *ProxyConfigRequest) Request() *Request {
	devicePath := utf16Bytes(r.DevicePath + "\x00")
	data := make([]byte, 0, 12+len(devicePath))
	data = binary.LittleEndian.AppendUint32(data, 12)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(devicePath)))
	data = binary.LittleEndian.AppendUint32(data, r.Timeout)
	data = append(data, devicePath...)

	r.Response = new(ProxyConfigResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMbimProxyControl,
			CIDProxyControlConfiguration,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type ProxyConfigResponse struct{}

func (r *ProxyConfigResponse) UnmarshalBinary([]byte) error { return nil }

type OpenDeviceRequest struct {
	TransactionID      uint32
	MaxControlTransfer uint32
	Response           *OpenDeviceResponse
}

func (r *OpenDeviceRequest) Request() *Request {
	r.Response = new(OpenDeviceResponse)
	return &Request{
		MessageType:   MessageTypeOpen,
		TransactionID: r.TransactionID,
		Command:       r,
		Response:      r.Response,
	}
}

func (r *OpenDeviceRequest) MarshalBinary() ([]byte, error) {
	maxControlTransfer := r.MaxControlTransfer
	if maxControlTransfer == 0 {
		maxControlTransfer = defaultMaxControlTransfer
	}
	return binary.LittleEndian.AppendUint32(nil, maxControlTransfer), nil
}

type OpenDeviceResponse struct{}

func (r *OpenDeviceResponse) UnmarshalBinary([]byte) error { return nil }

type CloseRequest struct {
	TransactionID uint32
	Response      *CloseResponse
}

func (r *CloseRequest) Request() *Request {
	r.Response = new(CloseResponse)
	return &Request{
		MessageType:   MessageTypeClose,
		TransactionID: r.TransactionID,
		Timeout:       2 * time.Second,
		Command:       r,
		Response:      r.Response,
	}
}

func (r *CloseRequest) MarshalBinary() ([]byte, error) { return nil, nil }

type CloseResponse struct{}

func (r *CloseResponse) UnmarshalBinary([]byte) error { return nil }

type DeviceServicesRequest struct {
	TransactionID uint32
	Response      *DeviceServicesResponse
}

func (r *DeviceServicesRequest) Request() *Request {
	r.Response = new(DeviceServicesResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceBasicConnect,
			CIDDeviceServices,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type DeviceServicesResponse struct {
	MaxDSSSessions uint32
	Services       []DeviceService
}

func (r DeviceServicesResponse) SupportsCID(serviceID [16]byte, cid uint32) bool {
	return slices.ContainsFunc(r.Services, func(service DeviceService) bool {
		return service.ServiceID == serviceID && slices.Contains(service.CIDs, cid)
	})
}

func (r *DeviceServicesResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return errors.New("parsing MBIM device services: payload is truncated")
	}
	serviceCount := binary.LittleEndian.Uint32(data[:4])
	r.MaxDSSSessions = binary.LittleEndian.Uint32(data[4:8])
	if serviceCount == 0 {
		r.Services = nil
		return nil
	}
	if serviceCount > uint32((len(data)-8)/8) {
		return errors.New("parsing MBIM device services: service table is truncated")
	}

	r.Services = make([]DeviceService, 0, serviceCount)
	for i := range serviceCount {
		entryOffset := uint32(8 + i*8)
		ref, err := readOffsetSizeRef(data, entryOffset)
		if err != nil {
			return fmt.Errorf("parsing MBIM device service %d: %w", i, err)
		}
		if err := ref.validate(data); err != nil {
			return fmt.Errorf("parsing MBIM device service %d: %w", i, err)
		}

		var service DeviceService
		if err := service.UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
			return fmt.Errorf("parsing MBIM device service %d: %w", i, err)
		}
		r.Services = append(r.Services, service)
	}
	return nil
}

type DeviceService struct {
	ServiceID       [16]byte
	DSSPayload      uint32
	MaxDSSInstances uint32
	CIDs            []uint32
}

func (s *DeviceService) UnmarshalBinary(data []byte) error {
	if len(data) < 28 {
		return errors.New("device service is truncated")
	}
	copy(s.ServiceID[:], data[:16])
	s.DSSPayload = binary.LittleEndian.Uint32(data[16:20])
	s.MaxDSSInstances = binary.LittleEndian.Uint32(data[20:24])
	cidCount := binary.LittleEndian.Uint32(data[24:28])
	if cidCount > uint32((len(data)-28)/4) {
		return errors.New("CID list is truncated")
	}

	s.CIDs = make([]uint32, cidCount)
	for i := range cidCount {
		offset := 28 + i*4
		s.CIDs[i] = binary.LittleEndian.Uint32(data[offset : offset+4])
	}
	return nil
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
	if len(data) < 4 {
		return errors.New("parsing MBIM version: payload is truncated")
	}
	r.MBIMVersion = binary.LittleEndian.Uint16(data[:2])
	r.MBIMExVersion = binary.LittleEndian.Uint16(data[2:4])
	return nil
}

type DeviceSlotMappingsRequest struct {
	TransactionID uint32
	SlotMappings  []SlotMapping
	Response      *DeviceSlotMappingsResponse
}

type SlotMapping struct {
	Slot uint32
}

func (r *DeviceSlotMappingsRequest) Request() *Request {
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

	commandType := CommandTypeQuery
	if mapCount > 0 {
		commandType = CommandTypeSet
	}
	r.Response = new(DeviceSlotMappingsResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDDeviceSlotMappings,
			commandType,
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
	if mapCount == 0 {
		r.SlotMappings = nil
		return nil
	}
	if mapCount > uint32((len(data)-4)/8) {
		return errors.New("parsing MBIM slot mappings: offset table is truncated")
	}

	r.SlotMappings = make([]SlotMapping, mapCount)
	for i := range mapCount {
		entryOffset := 4 + i*8
		slotDataOffset := binary.LittleEndian.Uint32(data[entryOffset : entryOffset+4])
		slotDataSize := binary.LittleEndian.Uint32(data[entryOffset+4 : entryOffset+8])
		if slotDataSize != 4 {
			return fmt.Errorf("parsing MBIM slot mappings: slot data size %d, want 4", slotDataSize)
		}
		if slotDataOffset > uint32(len(data)) || slotDataSize > uint32(len(data))-slotDataOffset {
			return errors.New("parsing MBIM slot mappings: slot data is truncated")
		}
		r.SlotMappings[i].Slot = binary.LittleEndian.Uint32(data[slotDataOffset : slotDataOffset+4])
	}
	return nil
}

type RadioStateRequest struct {
	TransactionID uint32
	Response      *RadioStateInfo
}

func (r *RadioStateRequest) Request() *Request {
	r.Response = new(RadioStateInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDRadioState,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type RadioStateSetRequest struct {
	TransactionID uint32
	State         RadioSwitchState
	Response      *RadioStateInfo
}

func (r *RadioStateSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.State))
	r.Response = new(RadioStateInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDRadioState,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

func (r *RadioStateInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return errors.New("parsing MBIM radio state: payload is truncated")
	}
	r.HwRadioState = RadioSwitchState(binary.LittleEndian.Uint32(data[:4]))
	r.SwRadioState = RadioSwitchState(binary.LittleEndian.Uint32(data[4:8]))
	return nil
}

type SubscriberReadyStatusRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Response      *SubscriberReadyStatusResponse
}

func (r *SubscriberReadyStatusRequest) Request() *Request {
	var data []byte
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(nil, r.SlotID)
	}

	r.Response = &SubscriberReadyStatusResponse{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDSubscriberReadyStatus,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type SubscriberReadyStatusResponse struct {
	MBIMExVersion         uint16
	ReadyState            SubscriberReadyState
	Flags                 SubscriberReadyStatusFlags
	SubscriberID          string
	SIMICCID              string
	ReadyInfo             ReadyInfo
	TelephoneNumbersCount uint32
	SlotID                uint32
	TelephoneNumbers      []string
}

func (r *SubscriberReadyStatusResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 28 {
		return errors.New("parsing MBIM subscriber ready status: payload is truncated")
	}
	subscriberRefOffset := uint32(4)
	simRefOffset := uint32(12)
	readyInfoOffset := uint32(20)
	countOffset := uint32(24)
	telephoneTableOffset := uint32(28)
	if r.MBIMExVersion >= mbimExVersion40 {
		if len(data) < 36 {
			return errors.New("parsing MBIM subscriber ready status: payload is truncated")
		}
		r.Flags = SubscriberReadyStatusFlags(binary.LittleEndian.Uint32(data[4:8]))
		r.SlotID = binary.LittleEndian.Uint32(data[32:36])
		subscriberRefOffset = 8
		simRefOffset = 16
		readyInfoOffset = 24
		countOffset = 28
		telephoneTableOffset = 36
	} else {
		r.Flags = SubscriberReadyStatusFlagNone
		r.SlotID = activeSubscriberSlot
	}

	r.ReadyState = SubscriberReadyState(binary.LittleEndian.Uint32(data[:4]))
	subscriberIDRef, err := readOffsetSizeRef(data, subscriberRefOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status subscriber ID: %w", err)
	}
	simICCIDRef, err := readOffsetSizeRef(data, simRefOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status SIM ICCID: %w", err)
	}
	r.ReadyInfo = ReadyInfo(binary.LittleEndian.Uint32(data[readyInfoOffset : readyInfoOffset+4]))
	r.TelephoneNumbersCount = binary.LittleEndian.Uint32(data[countOffset : countOffset+4])

	if r.TelephoneNumbersCount > uint32((len(data)-int(telephoneTableOffset))/8) {
		return errors.New("parsing MBIM subscriber ready status: telephone number table is truncated")
	}
	r.TelephoneNumbers = nil
	if r.TelephoneNumbersCount > 0 {
		r.TelephoneNumbers = make([]string, r.TelephoneNumbersCount)
	}

	refs := make([]valueRef, 0, 2+r.TelephoneNumbersCount)
	refs = append(refs, subscriberIDRef, simICCIDRef)
	for i := range r.TelephoneNumbersCount {
		entryOffset := telephoneTableOffset + i*8
		refs = append(refs, valueRef{
			offset: binary.LittleEndian.Uint32(data[entryOffset : entryOffset+4]),
			size:   binary.LittleEndian.Uint32(data[entryOffset+4 : entryOffset+8]),
		})
	}
	if err := validateUTF16Refs(data, refs); err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status strings: %w", err)
	}

	r.SubscriberID, err = utf16String(data, subscriberIDRef)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status subscriber ID: %w", err)
	}
	r.SIMICCID, err = utf16String(data, simICCIDRef)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status SIM ICCID: %w", err)
	}

	for i := range r.TelephoneNumbersCount {
		r.TelephoneNumbers[i], err = utf16String(data, refs[2+i])
		if err != nil {
			return fmt.Errorf("parsing MBIM subscriber ready status telephone number %d: %w", i, err)
		}
	}
	return nil
}

type ApplicationListRequest struct {
	TransactionID uint32
	Response      *ApplicationListResponse
}

func (r *ApplicationListRequest) Request() *Request {
	r.Response = new(ApplicationListResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccApplicationList,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type UICCApplication struct {
	Type                 UiccApplicationType
	AID                  []byte
	Label                string
	PinKeyReferenceCount uint32
	PinKeyReferences     []byte
}

var _ encoding.BinaryUnmarshaler = (*UICCApplication)(nil)

type ApplicationListResponse struct {
	Version                  uint32
	ActiveApplicationIndex   uint32
	ApplicationListSizeBytes uint32
	Applications             []UICCApplication
}

func (r *ApplicationListResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("parsing MBIM application list: payload is truncated")
	}
	r.Version = binary.LittleEndian.Uint32(data[:4])
	applicationCount := binary.LittleEndian.Uint32(data[4:8])
	r.ActiveApplicationIndex = binary.LittleEndian.Uint32(data[8:12])
	r.ApplicationListSizeBytes = binary.LittleEndian.Uint32(data[12:16])
	if applicationCount > uint32((len(data)-16)/8) {
		return errors.New("parsing MBIM application list: application table is truncated")
	}

	r.Applications = make([]UICCApplication, 0, applicationCount)
	for i := range applicationCount {
		entryOffset := 16 + i*8
		offset := binary.LittleEndian.Uint32(data[entryOffset : entryOffset+4])
		size := binary.LittleEndian.Uint32(data[entryOffset+4 : entryOffset+8])
		if offset > uint32(len(data)) || size > uint32(len(data))-offset {
			return errors.New("parsing MBIM application list: application entry is truncated")
		}
		var app UICCApplication
		if err := app.UnmarshalBinary(data[offset : offset+size]); err != nil {
			return fmt.Errorf("parsing MBIM application list entry %d: %w", i, err)
		}
		r.Applications = append(r.Applications, app)
	}
	return nil
}

func (a *UICCApplication) UnmarshalBinary(data []byte) error {
	if len(data) < 32 {
		return errors.New("application entry is truncated")
	}

	aid, err := byteArrayRef(data, data, 4)
	if err != nil {
		return fmt.Errorf("application ID: %w", err)
	}
	label, err := stringRef(data, data, 12)
	if err != nil {
		return fmt.Errorf("application name: %w", err)
	}
	pinKeyReferences, err := byteArrayRef(data, data, 24)
	if err != nil {
		return fmt.Errorf("PIN key references: %w", err)
	}

	*a = UICCApplication{
		Type:                 UiccApplicationType(binary.LittleEndian.Uint32(data[:4])),
		AID:                  aid,
		Label:                label,
		PinKeyReferenceCount: binary.LittleEndian.Uint32(data[20:24]),
		PinKeyReferences:     pinKeyReferences,
	}
	return nil
}

type FileStatusRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Response      *FileStatusResponse
}

func (r *FileStatusRequest) Request() *Request {
	data := refHeader(20, r.ApplicationID, r.FilePath)
	data = appendRefs(data, 20, r.ApplicationID, r.FilePath)

	r.Response = new(FileStatusResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccFileStatus,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type FileStatusResponse struct {
	Version                   uint32
	StatusWord1               uint32
	StatusWord2               uint32
	FileAccessibility         UiccFileAccessibility
	FileType                  UiccFileType
	FileStructure             UiccFileStructure
	FileItemCount             uint32
	FileItemSize              uint32
	AccessConditionRead       PinType
	AccessConditionUpdate     PinType
	AccessConditionActivate   PinType
	AccessConditionDeactivate PinType
}

func (r *FileStatusResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 48 {
		return errors.New("parsing MBIM file status: payload is truncated")
	}
	r.Version = binary.LittleEndian.Uint32(data[:4])
	r.StatusWord1 = binary.LittleEndian.Uint32(data[4:8])
	r.StatusWord2 = binary.LittleEndian.Uint32(data[8:12])
	r.FileAccessibility = UiccFileAccessibility(binary.LittleEndian.Uint32(data[12:16]))
	r.FileType = UiccFileType(binary.LittleEndian.Uint32(data[16:20]))
	r.FileStructure = UiccFileStructure(binary.LittleEndian.Uint32(data[20:24]))
	r.FileItemCount = binary.LittleEndian.Uint32(data[24:28])
	r.FileItemSize = binary.LittleEndian.Uint32(data[28:32])
	r.AccessConditionRead = PinType(binary.LittleEndian.Uint32(data[32:36]))
	r.AccessConditionUpdate = PinType(binary.LittleEndian.Uint32(data[36:40]))
	r.AccessConditionActivate = PinType(binary.LittleEndian.Uint32(data[40:44]))
	r.AccessConditionDeactivate = PinType(binary.LittleEndian.Uint32(data[44:48]))
	return nil
}

type ReadBinaryRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Offset        uint32
	Size          uint32
	Response      *ReadBinaryResponse
}

func (r *ReadBinaryRequest) Request() *Request {
	data := refHeader(44, r.ApplicationID, r.FilePath)
	data = binary.LittleEndian.AppendUint32(data, r.Offset)
	data = binary.LittleEndian.AppendUint32(data, r.Size)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = appendRefs(data, 44, r.ApplicationID, r.FilePath)

	r.Response = new(ReadBinaryResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReadBinary,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ReadBinaryResponse struct {
	Version     uint32
	StatusWord1 uint32
	StatusWord2 uint32
	Data        []byte
}

func (r *ReadBinaryResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM read binary: payload is truncated")
	}
	r.Version = binary.LittleEndian.Uint32(data[:4])
	r.StatusWord1 = binary.LittleEndian.Uint32(data[4:8])
	r.StatusWord2 = binary.LittleEndian.Uint32(data[8:12])
	value, err := byteArrayRef(data, data, 12)
	if err != nil {
		return fmt.Errorf("parsing MBIM read binary data: %w", err)
	}
	r.Data = value
	return nil
}

type ReadRecordRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Record        uint32
	Response      *ReadRecordResponse
}

func (r *ReadRecordRequest) Request() *Request {
	data := refHeader(40, r.ApplicationID, r.FilePath)
	data = binary.LittleEndian.AppendUint32(data, r.Record)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = appendRefs(data, 40, r.ApplicationID, r.FilePath)

	r.Response = new(ReadRecordResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReadRecord,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ReadRecordResponse struct {
	Version     uint32
	StatusWord1 uint32
	StatusWord2 uint32
	Data        []byte
}

func (r *ReadRecordResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM read record: payload is truncated")
	}
	r.Version = binary.LittleEndian.Uint32(data[:4])
	r.StatusWord1 = binary.LittleEndian.Uint32(data[4:8])
	r.StatusWord2 = binary.LittleEndian.Uint32(data[8:12])
	value, err := byteArrayRef(data, data, 12)
	if err != nil {
		return fmt.Errorf("parsing MBIM read record data: %w", err)
	}
	r.Data = value
	return nil
}

type AuthAKARequest struct {
	TransactionID uint32
	Rand          []byte
	AUTN          []byte
	Response      *AuthAKAResponse
}

func (r *AuthAKARequest) Request() *Request {
	data := make([]byte, 0, len(r.Rand)+len(r.AUTN))
	data = append(data, r.Rand...)
	data = append(data, r.AUTN...)

	r.Response = new(AuthAKAResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceAuth,
			CIDAuthAKA,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type AuthAKAResponse struct {
	RES  []byte
	CK   []byte
	IK   []byte
	AUTS []byte
}

func (r *AuthAKAResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 66 {
		return errors.New("parsing MBIM auth AKA: payload is truncated")
	}
	resLength := int(binary.LittleEndian.Uint32(data[16:20]))
	if resLength > 16 {
		return fmt.Errorf("parsing MBIM auth AKA: RES length %d exceeds 16", resLength)
	}
	r.RES = slices.Clone(data[:resLength])
	r.IK = slices.Clone(data[20:36])
	r.CK = slices.Clone(data[36:52])
	r.AUTS = slices.Clone(data[52:66])
	return nil
}

type UiccATRQueryRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Response      *UiccATRResponse
}

func (r *UiccATRQueryRequest) Request() *Request {
	var data []byte
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(nil, r.SlotID)
	}

	r.Response = new(UiccATRResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccATR,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type UiccATRResponse struct {
	ATR []byte
}

func (r *UiccATRResponse) UnmarshalBinary(data []byte) error {
	value, err := uiccByteArrayRef(data, 0)
	if err != nil {
		return fmt.Errorf("parsing MBIM UICC ATR: %w", err)
	}
	r.ATR = value
	return nil
}

type OpenChannelRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	ApplicationID []byte
	SelectP2Arg   uint32
	ChannelGroup  uint32
	Response      *OpenChannelResponse
}

func (r *OpenChannelRequest) Request() *Request {
	appIDOffset := 16
	if r.MBIMExVersion >= mbimExVersion40 {
		appIDOffset = 20
	}
	data := uiccRefHeader(appIDOffset, r.ApplicationID)
	data = binary.LittleEndian.AppendUint32(data, r.SelectP2Arg)
	data = binary.LittleEndian.AppendUint32(data, r.ChannelGroup)
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(data, r.SlotID)
	}
	data = append(data, r.ApplicationID...)

	r.Response = new(OpenChannelResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccOpenChannel,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type OpenChannelResponse struct {
	Status   uint32
	Channel  uint32
	Response []byte
}

func (r *OpenChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("parsing MBIM open channel: payload is truncated")
	}
	r.Status = binary.LittleEndian.Uint32(data[:4])
	r.Channel = binary.LittleEndian.Uint32(data[4:8])
	value, err := uiccByteArrayRef(data, 8)
	if err != nil {
		return fmt.Errorf("parsing MBIM open channel response: %w", err)
	}
	r.Response = value
	return nil
}

type CloseChannelRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Channel       uint32
	ChannelGroup  uint32
	Response      *CloseChannelResponse
}

func (r *CloseChannelRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, r.Channel)
	data = binary.LittleEndian.AppendUint32(data, r.ChannelGroup)
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(data, r.SlotID)
	}

	r.Response = new(CloseChannelResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccCloseChannel,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type CloseChannelResponse struct {
	Status uint32
}

func (r *CloseChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM close channel: payload is truncated")
	}
	r.Status = binary.LittleEndian.Uint32(data[:4])
	return nil
}

type APDURequest struct {
	TransactionID   uint32
	MBIMExVersion   uint16
	SlotID          uint32
	Channel         uint32
	SecureMessaging UiccSecureMessaging
	ClassByteType   UiccClassByteType
	Command         []byte
	Response        *APDUResponse
}

func (r *APDURequest) Request() *Request {
	commandOffset := uint32(20)
	if r.MBIMExVersion >= mbimExVersion40 {
		commandOffset = 24
	}

	data := binary.LittleEndian.AppendUint32(nil, r.Channel)
	data = binary.LittleEndian.AppendUint32(data, uint32(r.SecureMessaging))
	data = binary.LittleEndian.AppendUint32(data, uint32(r.ClassByteType))
	data = binary.LittleEndian.AppendUint32(data, uint32(len(r.Command)))
	data = binary.LittleEndian.AppendUint32(data, commandOffset)
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(data, r.SlotID)
	}
	data = append(data, r.Command...)

	r.Response = new(APDUResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccAPDU,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type APDUResponse struct {
	Status   uint32
	Response []byte
}

func (r *APDUResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 12 {
		return errors.New("parsing MBIM APDU: payload is truncated")
	}
	r.Status = binary.LittleEndian.Uint32(data[:4])
	value, err := uiccByteArrayRef(data, 4)
	if err != nil {
		return fmt.Errorf("parsing MBIM APDU response: %w", err)
	}
	r.Response = value
	return nil
}

type UiccTerminalCapabilitySetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Capabilities  [][]byte
}

func (r *UiccTerminalCapabilitySetRequest) Request() *Request {
	data := terminalCapabilityData(r.Capabilities)
	if r.MBIMExVersion >= mbimExVersion40 {
		data = terminalCapabilityDataEx4(r.SlotID, r.Capabilities)
	}

	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccTerminalCapability,
			CommandTypeSet,
			data,
		),
	}
}

type UiccTerminalCapabilityQueryRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Response      *UiccTerminalCapabilityResponse
}

func (r *UiccTerminalCapabilityQueryRequest) Request() *Request {
	var data []byte
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(nil, r.SlotID)
	}

	r.Response = new(UiccTerminalCapabilityResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccTerminalCapability,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type UiccTerminalCapabilityResponse struct {
	Capabilities [][]byte
}

func (r *UiccTerminalCapabilityResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM terminal capability: payload is truncated")
	}
	capabilityCount := binary.LittleEndian.Uint32(data[:4])
	if capabilityCount == 0 {
		r.Capabilities = nil
		return nil
	}
	if capabilityCount > uint32((len(data)-4)/8) {
		return errors.New("parsing MBIM terminal capability: capability table is truncated")
	}

	r.Capabilities = make([][]byte, capabilityCount)
	for i := range capabilityCount {
		entryOffset := 4 + i*8
		capabilityOffset := binary.LittleEndian.Uint32(data[entryOffset : entryOffset+4])
		capabilitySize := binary.LittleEndian.Uint32(data[entryOffset+4 : entryOffset+8])
		if capabilityOffset > uint32(len(data)) || capabilitySize > uint32(len(data))-capabilityOffset {
			return errors.New("parsing MBIM terminal capability: capability data is truncated")
		}
		r.Capabilities[i] = slices.Clone(data[capabilityOffset : capabilityOffset+capabilitySize])
	}
	return nil
}

type UiccResetSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Action        UiccPassThroughAction
	Response      *UiccResetResponse
}

func (r *UiccResetSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Action))
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(data, r.SlotID)
	}

	r.Response = new(UiccResetResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReset,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type UiccResetQueryRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SlotID        uint32
	Response      *UiccResetResponse
}

func (r *UiccResetQueryRequest) Request() *Request {
	var data []byte
	if r.MBIMExVersion >= mbimExVersion40 {
		data = binary.LittleEndian.AppendUint32(nil, r.SlotID)
	}

	r.Response = new(UiccResetResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReset,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type UiccResetResponse struct {
	PassThroughStatus UiccPassThroughStatus
}

func (r *UiccResetResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM UICC reset: payload is truncated")
	}
	r.PassThroughStatus = UiccPassThroughStatus(binary.LittleEndian.Uint32(data[:4]))
	return nil
}

const (
	stkPACHostControlLength  = 32
	stkPACSupportLength      = 256
	stkEnvelopeSupportLength = 32
)

type STKPACQueryRequest struct {
	TransactionID uint32
	Response      *STKPACInfo
}

func (r *STKPACQueryRequest) Request() *Request {
	r.Response = new(STKPACInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKPAC,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type STKPACSetRequest struct {
	TransactionID  uint32
	PacHostControl []byte
	Response       *STKPACInfo
}

func (r *STKPACSetRequest) Request() *Request {
	r.Response = new(STKPACInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKPAC,
			CommandTypeSet,
			r.PacHostControl,
		),
		Response: r.Response,
	}
}

type STKPACInfo struct {
	PacSupport [stkPACSupportLength]STKPACProfile
}

func (r *STKPACInfo) UnmarshalBinary(data []byte) error {
	if len(data) < stkPACSupportLength {
		return errors.New("parsing MBIM STK PAC info: payload is truncated")
	}
	for i, value := range data[:stkPACSupportLength] {
		r.PacSupport[i] = STKPACProfile(value)
	}
	return nil
}

type STKPAC struct {
	Type    STKPACType
	Command []byte
}

func (r *STKPAC) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM STK PAC: payload is truncated")
	}
	r.Type = STKPACType(binary.LittleEndian.Uint32(data[:4]))
	r.Command = slices.Clone(data[4:])
	return nil
}

type STKTerminalResponseRequest struct {
	TransactionID uint32
	Data          []byte
	Response      *STKTerminalResponseInfo
}

func (r *STKTerminalResponseRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(r.Data)))
	data = append(data, r.Data...)

	r.Response = new(STKTerminalResponseInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKTerminalResponse,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

type STKTerminalResponseInfo struct {
	ResultData  []byte
	StatusWords uint32
}

func (r *STKTerminalResponseInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 12 {
		return errors.New("parsing MBIM STK terminal response: payload is truncated")
	}
	result, err := byteArrayRef(data, data, 0)
	if err != nil {
		return fmt.Errorf("parsing MBIM STK terminal response data: %w", err)
	}
	r.ResultData = result
	r.StatusWords = binary.LittleEndian.Uint32(data[8:12])
	return nil
}

type STKEnvelopeQueryRequest struct {
	TransactionID uint32
	Response      *STKEnvelopeInfo
}

func (r *STKEnvelopeQueryRequest) Request() *Request {
	r.Response = new(STKEnvelopeInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKEnvelope,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type STKEnvelopeInfo struct {
	EnvelopeSupport [stkEnvelopeSupportLength]byte
}

func (r *STKEnvelopeInfo) Supports(tag byte) bool {
	mask := byte(1 << (tag % 8))
	return r.EnvelopeSupport[int(tag)/8]&mask != 0
}

func (r *STKEnvelopeInfo) UnmarshalBinary(data []byte) error {
	if len(data) < stkEnvelopeSupportLength {
		return errors.New("parsing MBIM STK envelope info: payload is truncated")
	}
	copy(r.EnvelopeSupport[:], data[:stkEnvelopeSupportLength])
	return nil
}

type STKEnvelopeRequest struct {
	TransactionID uint32
	Data          []byte
	Response      *STKEnvelopeResponse
}

func (r *STKEnvelopeRequest) Request() *Request {
	r.Response = new(STKEnvelopeResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceSTK,
			CIDSTKEnvelope,
			CommandTypeSet,
			r.Data,
		),
		Response: r.Response,
	}
}

type STKEnvelopeResponse struct{}

func (r *STKEnvelopeResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM STK envelope response: length %d, want 0", len(data))
	}
	return nil
}

func command(serviceID [16]byte, commandID uint32, commandType CommandType, data []byte) *Command {
	return &Command{
		FragmentTotal:   1,
		FragmentCurrent: 0,
		ServiceID:       serviceID,
		CommandID:       commandID,
		CommandType:     commandType,
		Data:            slices.Clone(data),
	}
}

func uiccRefHeader(offset int, value []byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(value)))
	data = binary.LittleEndian.AppendUint32(data, uint32(offset))
	return data
}

type valueRef struct {
	offset uint32
	size   uint32
}

func readOffsetSizeRef(data []byte, fieldOffset uint32) (valueRef, error) {
	if fieldOffset > uint32(len(data)) || 8 > uint32(len(data))-fieldOffset {
		return valueRef{}, errors.New("reference is truncated")
	}
	return valueRef{
		offset: binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4]),
		size:   binary.LittleEndian.Uint32(data[fieldOffset+4 : fieldOffset+8]),
	}, nil
}

func readSizeOffsetRef(data []byte, fieldOffset uint32) (valueRef, error) {
	if fieldOffset > uint32(len(data)) || 8 > uint32(len(data))-fieldOffset {
		return valueRef{}, errors.New("reference is truncated")
	}
	return valueRef{
		size:   binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4]),
		offset: binary.LittleEndian.Uint32(data[fieldOffset+4 : fieldOffset+8]),
	}, nil
}

func (r valueRef) validate(base []byte) error {
	if r.offset == 0 {
		if r.size != 0 {
			return errors.New("reference has zero offset with nonzero size")
		}
		return nil
	}
	if r.offset > uint32(len(base)) || r.size > uint32(len(base))-r.offset {
		return errors.New("value is truncated")
	}
	return nil
}

func (r valueRef) bytes(base []byte) []byte {
	if r.offset == 0 && r.size == 0 {
		return nil
	}
	return slices.Clone(base[r.offset : r.offset+r.size])
}

func uiccByteArrayRef(data []byte, fieldOffset uint32) ([]byte, error) {
	ref, err := readSizeOffsetRef(data, fieldOffset)
	if err != nil {
		return nil, err
	}
	if err := ref.validate(data); err != nil {
		return nil, err
	}
	return ref.bytes(data), nil
}

func terminalCapabilityData(capabilities [][]byte) []byte {
	capabilityCount := uint32(len(capabilities))
	data := binary.LittleEndian.AppendUint32(nil, capabilityCount)
	capabilityOffset := 4 + len(capabilities)*8
	for _, capability := range capabilities {
		data = binary.LittleEndian.AppendUint32(data, uint32(capabilityOffset))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(capability)))
		capabilityOffset = align4(capabilityOffset + len(capability))
	}
	for _, capability := range capabilities {
		data = append(data, capability...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return data
}

func terminalCapabilityDataEx4(slotID uint32, capabilities [][]byte) []byte {
	capabilityCount := uint32(len(capabilities))
	data := binary.LittleEndian.AppendUint32(nil, slotID)
	data = binary.LittleEndian.AppendUint32(data, capabilityCount)
	capabilityOffset := 8 + len(capabilities)*8
	for _, capability := range capabilities {
		data = binary.LittleEndian.AppendUint32(data, uint32(capabilityOffset))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(capability)))
		capabilityOffset = align4(capabilityOffset + len(capability))
	}
	for _, capability := range capabilities {
		data = append(data, capability...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return data
}

func refHeader(baseOffset int, refs ...[]byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, 1)
	offset := baseOffset
	for _, ref := range refs {
		data = binary.LittleEndian.AppendUint32(data, uint32(offset))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset = align4(offset + len(ref))
	}
	return data
}

func appendRefs(data []byte, baseOffset int, refs ...[]byte) []byte {
	for _, ref := range refs {
		data = append(data, ref...)
		for (baseOffset+len(ref))%4 != 0 {
			data = append(data, 0)
			baseOffset++
		}
		baseOffset += len(ref)
	}
	return data
}

func byteArrayRef(base, data []byte, fieldOffset uint32) ([]byte, error) {
	ref, err := readOffsetSizeRef(data, fieldOffset)
	if err != nil {
		return nil, err
	}
	if err := ref.validate(base); err != nil {
		return nil, err
	}
	return ref.bytes(base), nil
}

func stringRef(base, data []byte, fieldOffset uint32) (string, error) {
	raw, err := byteArrayRef(base, data, fieldOffset)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateUTF16Refs(base []byte, refs []valueRef) error {
	var previousEnd uint32
	var sawString bool
	for _, ref := range refs {
		if err := ref.validate(base); err != nil {
			return err
		}
		if ref.size%2 != 0 {
			return errors.New("UTF-16 string has odd byte length")
		}
		if ref.offset == 0 {
			continue
		}
		if sawString && ref.offset < previousEnd {
			return errors.New("string buffers overlap or are out of order")
		}
		previousEnd = ref.offset + ref.size
		sawString = true
	}
	return nil
}

func utf16String(data []byte, ref valueRef) (string, error) {
	if err := ref.validate(data); err != nil {
		return "", err
	}
	if ref.size == 0 {
		return "", nil
	}
	raw := data[ref.offset : ref.offset+ref.size]
	return utf16RawString(raw)
}

func utf16RawString(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", errors.New("UTF-16 string has odd byte length")
	}

	encoded := make([]uint16, len(raw)/2)
	for i := range encoded {
		encoded[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(encoded)), nil
}

func utf16Bytes(s string) []byte {
	encoded := utf16.Encode([]rune(s))
	buf := make([]byte, 0, len(encoded)*2)
	for _, v := range encoded {
		buf = binary.LittleEndian.AppendUint16(buf, v)
	}
	return buf
}
