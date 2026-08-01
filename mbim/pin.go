package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	basicPinTypeMaximum = PinTypeCorporatePUK
	pinLengthUnknown    = 0x00ffffff
)

type PinType uint32

const (
	PinTypeNone PinType = iota
	PinTypeCustom
	PinTypePIN1
	PinTypePIN2
	PinTypeDeviceSIM
	PinTypeDeviceFirstSIM
	PinTypeNetwork
	PinTypeNetworkSubset
	PinTypeServiceProvider
	PinTypeCorporate
	PinTypeSubsidy
	PinTypePUK1
	PinTypePUK2
	PinTypeDeviceFirstSIMPUK
	PinTypeNetworkPUK
	PinTypeNetworkSubsetPUK
	PinTypeServiceProviderPUK
	PinTypeCorporatePUK
	PinTypeNEV
	PinTypeADM
)

const PinTypeUnknown = PinTypeNone

type PinMode uint32

const (
	PinModeNotSupported PinMode = 0
	PinModeEnabled      PinMode = 1
	PinModeDisabled     PinMode = 2
)

type PinFormat uint32

const (
	PinFormatUnknown      PinFormat = 0
	PinFormatNumeric      PinFormat = 1
	PinFormatAlphanumeric PinFormat = 2
)

type PinState uint32

const (
	PinStateUnlocked PinState = 0
	PinStateLocked   PinState = 1
)

type PinOperation uint32

const (
	PinOperationEnter   PinOperation = 0
	PinOperationEnable  PinOperation = 1
	PinOperationDisable PinOperation = 2
	PinOperationChange  PinOperation = 3
)

type PinDesc struct {
	Mode      PinMode
	Format    PinFormat
	LengthMin uint32
	LengthMax uint32
}

type PinInfo struct {
	Type              PinType
	State             PinState
	RemainingAttempts uint32
}

type PinListInfo struct {
	PIN1            PinDesc
	PIN2            PinDesc
	DeviceSIM       PinDesc
	DeviceFirstSIM  PinDesc
	Network         PinDesc
	NetworkSubset   PinDesc
	ServiceProvider PinDesc
	Corporate       PinDesc
	Subsidy         PinDesc
	Custom          PinDesc
}

func validBasicPinType(pinType PinType) bool {
	return pinType <= basicPinTypeMaximum
}

func validPinLength(length uint32) bool {
	return length <= 16 || length == pinLengthUnknown
}

func (desc PinDesc) validate() error {
	if desc.Mode > PinModeDisabled {
		return fmt.Errorf("mode %d is outside 0..%d", desc.Mode, PinModeDisabled)
	}
	if desc.Format > PinFormatAlphanumeric {
		return fmt.Errorf("format %d is outside 0..%d", desc.Format, PinFormatAlphanumeric)
	}
	if !validPinLength(desc.LengthMin) {
		return fmt.Errorf("minimum length %d exceeds 16 and is not the unknown value %#x", desc.LengthMin, pinLengthUnknown)
	}
	if !validPinLength(desc.LengthMax) {
		return fmt.Errorf("maximum length %d exceeds 16 and is not the unknown value %#x", desc.LengthMax, pinLengthUnknown)
	}
	if desc.LengthMin != pinLengthUnknown && desc.LengthMax != pinLengthUnknown && desc.LengthMin > desc.LengthMax {
		return fmt.Errorf("minimum length %d exceeds maximum length %d", desc.LengthMin, desc.LengthMax)
	}
	return nil
}

type PINRequest struct {
	TransactionID uint32
	Response      *PinInfo
}

func (r *PINRequest) Request() *Request {
	r.Response = new(PinInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDPin, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type PINSetRequest struct {
	TransactionID uint32
	Type          PinType
	Operation     PinOperation
	PIN           string
	NewPIN        string
	Response      *PinInfo
}

func (r *PINSetRequest) Request() *Request {
	pin := utf16Bytes(r.PIN)
	newPIN := utf16Bytes(r.NewPIN)
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.Type))
	binary.LittleEndian.PutUint32(data[4:8], uint32(r.Operation))
	data = appendRefValue(data, 8, pin)
	data = appendRefValue(data, 16, newPIN)

	r.Response = new(PinInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDPin, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (r *PinInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 12 {
		return fmt.Errorf("parsing MBIM PIN info: payload length is %d, want 12", len(data))
	}
	pinType := PinType(binary.LittleEndian.Uint32(data[0:4]))
	if !validBasicPinType(pinType) {
		return fmt.Errorf("parsing MBIM PIN info: type %d is outside 0..%d", pinType, basicPinTypeMaximum)
	}
	state := PinState(binary.LittleEndian.Uint32(data[4:8]))
	if state > PinStateLocked {
		return fmt.Errorf("parsing MBIM PIN info: state %d is outside 0..%d", state, PinStateLocked)
	}
	*r = PinInfo{
		Type:              pinType,
		State:             state,
		RemainingAttempts: binary.LittleEndian.Uint32(data[8:12]),
	}
	return nil
}

type PINListRequest struct {
	TransactionID uint32
	Response      *PinListInfo
}

func (r *PINListRequest) Request() *Request {
	r.Response = new(PinListInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDPinList, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *PinListInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 160 {
		return fmt.Errorf("parsing MBIM PIN list: payload length is %d, want 160", len(data))
	}
	names := [...]string{
		"PIN1", "PIN2", "device SIM", "device first SIM", "network",
		"network subset", "service provider", "corporate", "subsidy", "custom",
	}
	descs := make([]PinDesc, 10)
	for i := range descs {
		offset := i * 16
		descs[i] = PinDesc{
			Mode:      PinMode(binary.LittleEndian.Uint32(data[offset : offset+4])),
			Format:    PinFormat(binary.LittleEndian.Uint32(data[offset+4 : offset+8])),
			LengthMin: binary.LittleEndian.Uint32(data[offset+8 : offset+12]),
			LengthMax: binary.LittleEndian.Uint32(data[offset+12 : offset+16]),
		}
		if err := descs[i].validate(); err != nil {
			return fmt.Errorf("parsing MBIM PIN list %s descriptor: %w", names[i], err)
		}
	}
	*r = PinListInfo{
		PIN1:            descs[0],
		PIN2:            descs[1],
		DeviceSIM:       descs[2],
		DeviceFirstSIM:  descs[3],
		Network:         descs[4],
		NetworkSubset:   descs[5],
		ServiceProvider: descs[6],
		Corporate:       descs[7],
		Subsidy:         descs[8],
		Custom:          descs[9],
	}
	return nil
}

func (c *Client) PIN(ctx context.Context) (PinInfo, error) {
	request := PINRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("reading MBIM PIN state: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetPIN(ctx context.Context, pinType PinType, operation PinOperation, pin, newPIN string) (PinInfo, error) {
	if !validBasicPinType(pinType) {
		return PinInfo{}, fmt.Errorf("setting MBIM PIN: type %d is outside 0..%d", pinType, basicPinTypeMaximum)
	}
	if operation > PinOperationChange {
		return PinInfo{}, fmt.Errorf("setting MBIM PIN: operation %d is outside 0..%d", operation, PinOperationChange)
	}
	if size := len(utf16Bytes(pin)); size > 32 {
		return PinInfo{}, fmt.Errorf("setting MBIM PIN: PIN length %d exceeds 32 bytes", size)
	}
	if size := len(utf16Bytes(newPIN)); size > 32 {
		return PinInfo{}, fmt.Errorf("setting MBIM PIN: new PIN length %d exceeds 32 bytes", size)
	}
	newPINApplicable := operation == PinOperationChange ||
		(operation == PinOperationEnter && (pinType == PinTypePUK1 || pinType == PinTypePUK2))
	if newPIN != "" && !newPINApplicable {
		return PinInfo{}, errors.New("setting MBIM PIN: new PIN is only valid for change or PUK1/PUK2 enter operations")
	}
	request := PINSetRequest{
		TransactionID: c.nextTransactionID(),
		Type:          pinType,
		Operation:     operation,
		PIN:           pin,
		NewPIN:        newPIN,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("setting MBIM PIN: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) PINList(ctx context.Context) (PinListInfo, error) {
	request := PINListRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return PinListInfo{}, fmt.Errorf("reading MBIM PIN list: %w", err)
	}
	return *request.Response, nil
}
