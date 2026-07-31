package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type RegisterState uint32

type RegisterAction uint32

const (
	RegisterActionAutomatic RegisterAction = 0
	RegisterActionManual    RegisterAction = 1
)

const (
	RegisterStateUnknown RegisterState = iota
	RegisterStateDeregistered
	RegisterStateSearching
	RegisterStateHome
	RegisterStateRoaming
	RegisterStatePartner
	RegisterStateDenied
)

type RegisterMode uint32

const (
	RegisterModeUnknown RegisterMode = iota
	RegisterModeAutomatic
	RegisterModeManual
)

type RegistrationFlags uint32

const (
	RegistrationFlagManualSelectionNotAvailable RegistrationFlags = 1 << iota
	RegistrationFlagPacketServiceAutomaticAttach
)

type RegistrationStateInfo struct {
	MBIMExVersion        uint16
	NwError              uint32
	RegisterState        RegisterState
	RegisterMode         RegisterMode
	AvailableDataClasses uint32
	CurrentCellularClass uint32
	ProviderID           string
	ProviderName         string
	RoamingText          string
	RegistrationFlags    RegistrationFlags
	PreferredDataClasses DataClass
}

type RegistrationStateRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *RegistrationStateInfo
}

type RegistrationStateSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	ProviderID    string
	Action        RegisterAction
	DataClass     DataClass
	Response      *RegistrationStateInfo
}

func (r *RegistrationStateSetRequest) Request() *Request {
	providerID := utf16Bytes(r.ProviderID)
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[8:12], uint32(r.Action))
	binary.LittleEndian.PutUint32(data[12:16], uint32(r.DataClass))
	data = appendRefValue(data, 0, providerID)

	r.Response = &RegistrationStateInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDRegisterState, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (r *RegistrationStateRequest) Request() *Request {
	r.Response = &RegistrationStateInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDRegisterState, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *RegistrationStateInfo) UnmarshalBinary(data []byte) error {
	version := r.MBIMExVersion
	fixedLength := 48
	if version >= mbimExVersion20 {
		fixedLength = 52
	}
	if len(data) < fixedLength {
		return errors.New("parsing MBIM registration state: payload is truncated")
	}
	registerState := RegisterState(binary.LittleEndian.Uint32(data[4:8]))
	if registerState > RegisterStateDenied {
		return fmt.Errorf("parsing MBIM registration state: state %d is outside 0..%d", registerState, RegisterStateDenied)
	}
	registerMode := RegisterMode(binary.LittleEndian.Uint32(data[8:12]))
	if registerMode > RegisterModeManual {
		return fmt.Errorf("parsing MBIM registration state: mode %d is outside 0..%d", registerMode, RegisterModeManual)
	}
	availableDataClass := DataClass(binary.LittleEndian.Uint32(data[12:16]))
	if !validDataClass(version, availableDataClass) {
		return fmt.Errorf("parsing MBIM registration state: available data classes %#x contain bits reserved in MBIMEx %#x", availableDataClass, version)
	}
	registered := registerState == RegisterStateHome || registerState == RegisterStateRoaming || registerState == RegisterStatePartner
	if !registered && availableDataClass != DataClassNone {
		return fmt.Errorf("parsing MBIM registration state: available data classes are %#x while state is %d", availableDataClass, registerState)
	}
	currentCellularClass := CellularClass(binary.LittleEndian.Uint32(data[16:20]))
	if !validCellularClass(currentCellularClass) {
		return fmt.Errorf("parsing MBIM registration state: cellular class %#x contains reserved bits", currentCellularClass)
	}
	registrationFlags := RegistrationFlags(binary.LittleEndian.Uint32(data[44:48]))
	knownRegistrationFlags := RegistrationFlagManualSelectionNotAvailable | RegistrationFlagPacketServiceAutomaticAttach
	if registrationFlags&^knownRegistrationFlags != 0 {
		return fmt.Errorf("parsing MBIM registration state: flags %#x contain reserved bits", registrationFlags)
	}
	refs := make([]valueRef, 3)
	maximumSizes := [...]uint32{12, 40, 126}
	for i, offset := range []uint32{20, 28, 36} {
		ref, err := readOffsetSizeRef(data, offset)
		if err != nil {
			return fmt.Errorf("parsing MBIM registration state string reference: %w", err)
		}
		if ref.size > maximumSizes[i] {
			return fmt.Errorf("parsing MBIM registration state string %d: size %d exceeds %d bytes", i, ref.size, maximumSizes[i])
		}
		refs[i] = ref
	}
	if err := validateDataBufferRefs(data, uint32(fixedLength), refs); err != nil {
		return fmt.Errorf("parsing MBIM registration state data buffer: %w", err)
	}
	if err := validateUTF16Refs(data, refs); err != nil {
		return fmt.Errorf("parsing MBIM registration state strings: %w", err)
	}
	providerID, err := utf16String(data, refs[0])
	if err != nil {
		return fmt.Errorf("parsing MBIM registration provider ID: %w", err)
	}
	if err := validateProviderID(providerID); err != nil {
		return fmt.Errorf("parsing MBIM registration provider ID: %w", err)
	}
	providerName, err := utf16String(data, refs[1])
	if err != nil {
		return fmt.Errorf("parsing MBIM registration provider name: %w", err)
	}
	roamingText, err := utf16String(data, refs[2])
	if err != nil {
		return fmt.Errorf("parsing MBIM registration roaming text: %w", err)
	}
	*r = RegistrationStateInfo{
		MBIMExVersion:        version,
		NwError:              binary.LittleEndian.Uint32(data[0:4]),
		RegisterState:        registerState,
		RegisterMode:         registerMode,
		AvailableDataClasses: uint32(availableDataClass),
		CurrentCellularClass: uint32(currentCellularClass),
		ProviderID:           providerID,
		ProviderName:         providerName,
		RoamingText:          roamingText,
		RegistrationFlags:    registrationFlags,
	}
	if version >= mbimExVersion20 {
		preferredDataClasses := DataClass(binary.LittleEndian.Uint32(data[48:52]))
		if !validDataClass(version, preferredDataClasses) {
			return fmt.Errorf("parsing MBIM registration state: preferred data classes %#x contain bits reserved in MBIMEx %#x", preferredDataClasses, version)
		}
		r.PreferredDataClasses = preferredDataClasses
	}
	return nil
}

func (c *Client) RegistrationState(ctx context.Context) (RegistrationStateInfo, error) {
	request := RegistrationStateRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("reading MBIM registration state: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetRegistrationState(ctx context.Context, providerID string, action RegisterAction, dataClass DataClass) (RegistrationStateInfo, error) {
	if err := validateProviderID(providerID); err != nil {
		return RegistrationStateInfo{}, fmt.Errorf("setting MBIM registration state: %w", err)
	}
	if action > RegisterActionManual {
		return RegistrationStateInfo{}, fmt.Errorf("setting MBIM registration state: action %d is outside 0..%d", action, RegisterActionManual)
	}
	if action == RegisterActionManual && providerID == "" {
		return RegistrationStateInfo{}, errors.New("setting MBIM registration state: manual registration requires a provider ID")
	}
	if !validDataClass(c.mbimExVersion, dataClass) {
		return RegistrationStateInfo{}, fmt.Errorf("setting MBIM registration state: data class %#x contains bits reserved in MBIMEx %#x", dataClass, c.mbimExVersion)
	}
	request := RegistrationStateSetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		ProviderID:    providerID,
		Action:        action,
		DataClass:     dataClass,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("setting MBIM registration state: %w", err)
	}
	return *request.Response, nil
}
