package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type LTEAttachContextOperation uint32

const (
	LTEAttachContextOperationDefault        LTEAttachContextOperation = 0
	LTEAttachContextOperationRestoreFactory LTEAttachContextOperation = 1
)

type LTEAttachRoamingControl uint32

const (
	LTEAttachRoamingControlHome       LTEAttachRoamingControl = 0
	LTEAttachRoamingControlPartner    LTEAttachRoamingControl = 1
	LTEAttachRoamingControlNonPartner LTEAttachRoamingControl = 2
)

type LTEAttachState uint32

const (
	LTEAttachStateDetached LTEAttachState = 0
	LTEAttachStateAttached LTEAttachState = 1
)

type LTEAttachConfigurationRequest struct {
	TransactionID uint32
	Response      *LTEAttachConfigurationsInfo
}

func (r *LTEAttachConfigurationRequest) Request() *Request {
	r.Response = new(LTEAttachConfigurationsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsLteAttachConfiguration,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type LTEAttachConfigurationSetRequest struct {
	TransactionID  uint32
	Operation      LTEAttachContextOperation
	Configurations []LTEAttachConfiguration
	Response       *LTEAttachConfigurationsInfo
}

func (r *LTEAttachConfigurationSetRequest) Request() *Request {
	data, err := marshalLTEAttachConfigurationSetChecked(r.Operation, r.Configurations)
	r.Response = new(LTEAttachConfigurationsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsLteAttachConfiguration,
			CommandTypeSet,
			data,
			err,
		),
		Response: r.Response,
	}
}

func marshalLTEAttachConfigurationSetChecked(operation LTEAttachContextOperation, configurations []LTEAttachConfiguration) ([]byte, error) {
	if operation > LTEAttachContextOperationRestoreFactory {
		return nil, fmt.Errorf("encoding MBIM LTE attach configurations: operation %d is outside 0..%d", operation, LTEAttachContextOperationRestoreFactory)
	}
	for index, configuration := range configurations {
		if err := configuration.validate(); err != nil {
			return nil, fmt.Errorf("encoding MBIM LTE attach configuration %d: %w", index, err)
		}
	}
	return marshalLTEAttachConfigurationSet(operation, configurations), nil
}

type LTEAttachConfiguration struct {
	IPType       ContextIPType
	Roaming      LTEAttachRoamingControl
	Source       ContextSource
	AccessString string
	UserName     string
	Password     string
	Compression  Compression
	AuthProtocol AuthProtocol
}

func (c LTEAttachConfiguration) MarshalBinary() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("encoding MBIM LTE attach configuration: %w", err)
	}
	accessString := utf16Bytes(c.AccessString)
	userName := utf16Bytes(c.UserName)
	password := utf16Bytes(c.Password)
	return c.marshalBinary(accessString, userName, password), nil
}

func (c LTEAttachConfiguration) validate() error {
	if !validContextIPType(c.IPType) {
		return fmt.Errorf("IP type %d is outside 0..%d", c.IPType, ContextIPTypeIPv4AndIPv6)
	}
	if c.Roaming > LTEAttachRoamingControlNonPartner {
		return fmt.Errorf("roaming control %d is outside 0..%d", c.Roaming, LTEAttachRoamingControlNonPartner)
	}
	if !validContextSource(c.Source) {
		return fmt.Errorf("source %d is outside 0..%d", c.Source, ContextSourceDevice)
	}
	if !validCompression(c.Compression) {
		return fmt.Errorf("compression %d is outside 0..%d", c.Compression, CompressionEnable)
	}
	if !validAuthProtocol(c.AuthProtocol) {
		return fmt.Errorf("authentication protocol %d is outside 0..%d", c.AuthProtocol, AuthProtocolMSCHAPV2)
	}
	if len(utf16Bytes(c.AccessString)) > accessStringMaximumSize {
		return fmt.Errorf("access string exceeds %d bytes", accessStringMaximumSize)
	}
	if len(utf16Bytes(c.UserName)) > userNameMaximumSize {
		return fmt.Errorf("user name exceeds %d bytes", userNameMaximumSize)
	}
	if len(utf16Bytes(c.Password)) > passwordMaximumSize {
		return fmt.Errorf("password exceeds %d bytes", passwordMaximumSize)
	}
	return nil
}

func (c LTEAttachConfiguration) marshalBinary(accessString, userName, password []byte) []byte {
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data[0:4], uint32(c.IPType))
	binary.LittleEndian.PutUint32(data[4:8], uint32(c.Roaming))
	binary.LittleEndian.PutUint32(data[8:12], uint32(c.Source))
	binary.LittleEndian.PutUint32(data[36:40], uint32(c.Compression))
	binary.LittleEndian.PutUint32(data[40:44], uint32(c.AuthProtocol))
	data = appendRefValue(data, 12, accessString)
	data = appendRefValue(data, 20, userName)
	return appendRefValue(data, 28, password)
}

func (c *LTEAttachConfiguration) UnmarshalBinary(data []byte) error {
	if len(data) < 44 {
		return errors.New("LTE attach configuration is truncated")
	}
	refs, err := lteAttachStringRefs(data, 44, []uint32{12, 20, 28})
	if err != nil {
		return err
	}
	values, err := lteAttachStrings(data, refs)
	if err != nil {
		return err
	}
	result := LTEAttachConfiguration{
		IPType:       ContextIPType(binary.LittleEndian.Uint32(data[0:4])),
		Roaming:      LTEAttachRoamingControl(binary.LittleEndian.Uint32(data[4:8])),
		Source:       ContextSource(binary.LittleEndian.Uint32(data[8:12])),
		AccessString: values[0],
		UserName:     values[1],
		Password:     values[2],
		Compression:  Compression(binary.LittleEndian.Uint32(data[36:40])),
		AuthProtocol: AuthProtocol(binary.LittleEndian.Uint32(data[40:44])),
	}
	if err := result.validate(); err != nil {
		return fmt.Errorf("parsing MBIM LTE attach configuration: %w", err)
	}
	*c = result
	return nil
}

type LTEAttachConfigurationsInfo struct {
	Configurations []LTEAttachConfiguration
}

func (i LTEAttachConfigurationsInfo) MarshalBinary() ([]byte, error) {
	elements := make([][]byte, len(i.Configurations))
	for index, configuration := range i.Configurations {
		data, err := configuration.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding MBIM LTE attach configuration %d: %w", index, err)
		}
		elements[index] = data
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements), nil
}

func (i *LTEAttachConfigurationsInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM LTE attach configurations: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM LTE attach configurations: %w", err)
	}
	configurations := make([]LTEAttachConfiguration, count)
	for index, ref := range refs {
		if err := configurations[index].UnmarshalBinary(ref.bytes(data)); err != nil {
			return fmt.Errorf("parsing MBIM LTE attach configuration %d: %w", index, err)
		}
	}
	i.Configurations = configurations
	return nil
}

func marshalLTEAttachConfigurationSet(operation LTEAttachContextOperation, configurations []LTEAttachConfiguration) []byte {
	elements := make([][]byte, len(configurations))
	for index, configuration := range configurations {
		elements[index] = configuration.marshalBinary(
			utf16Bytes(configuration.AccessString),
			utf16Bytes(configuration.UserName),
			utf16Bytes(configuration.Password),
		)
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(operation))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

type LTEAttachInfoRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *LTEAttachInfo
}

func (r *LTEAttachInfoRequest) Request() *Request {
	r.Response = &LTEAttachInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsLteAttachInfo,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type LTEAttachInfo struct {
	MBIMExVersion uint16
	State         LTEAttachState
	NwError       uint32
	IPType        ContextIPType
	AccessString  string
	UserName      string
	Password      string
	Compression   Compression
	AuthProtocol  AuthProtocol
}

func (i *LTEAttachInfo) UnmarshalBinary(data []byte) error {
	version := i.MBIMExVersion
	fixedLength := 40
	ipTypeOffset := 4
	refOffsets := []uint32{8, 16, 24}
	compressionOffset := 32
	authOffset := 36
	if version >= mbimExVersion30 {
		fixedLength = 44
		ipTypeOffset = 8
		refOffsets = []uint32{12, 20, 28}
		compressionOffset = 36
		authOffset = 40
	}
	if len(data) < fixedLength {
		return errors.New("parsing MBIM LTE attach info: payload is truncated")
	}
	refs, err := lteAttachStringRefs(data, uint32(fixedLength), refOffsets)
	if err != nil {
		return fmt.Errorf("parsing MBIM LTE attach info: %w", err)
	}
	values, err := lteAttachStrings(data, refs)
	if err != nil {
		return fmt.Errorf("parsing MBIM LTE attach info: %w", err)
	}
	result := LTEAttachInfo{
		MBIMExVersion: version,
		State:         LTEAttachState(binary.LittleEndian.Uint32(data[0:4])),
		IPType:        ContextIPType(binary.LittleEndian.Uint32(data[ipTypeOffset : ipTypeOffset+4])),
		AccessString:  values[0],
		UserName:      values[1],
		Password:      values[2],
		Compression:   Compression(binary.LittleEndian.Uint32(data[compressionOffset : compressionOffset+4])),
		AuthProtocol:  AuthProtocol(binary.LittleEndian.Uint32(data[authOffset : authOffset+4])),
	}
	if result.State > LTEAttachStateAttached {
		return fmt.Errorf("parsing MBIM LTE attach info: state %d is outside 0..%d", result.State, LTEAttachStateAttached)
	}
	if !validContextIPType(result.IPType) {
		return fmt.Errorf("parsing MBIM LTE attach info: IP type %d is outside 0..%d", result.IPType, ContextIPTypeIPv4AndIPv6)
	}
	if !validCompression(result.Compression) {
		return fmt.Errorf("parsing MBIM LTE attach info: compression %d is outside 0..%d", result.Compression, CompressionEnable)
	}
	if !validAuthProtocol(result.AuthProtocol) {
		return fmt.Errorf("parsing MBIM LTE attach info: authentication protocol %d is outside 0..%d", result.AuthProtocol, AuthProtocolMSCHAPV2)
	}
	if version >= mbimExVersion30 {
		result.NwError = binary.LittleEndian.Uint32(data[4:8])
	}
	*i = result
	return nil
}

func lteAttachStringRefs(data []byte, fixedLength uint32, offsets []uint32) ([]valueRef, error) {
	refs := make([]valueRef, len(offsets))
	for index, offset := range offsets {
		ref, err := readOffsetSizeRef(data, offset)
		if err != nil {
			return nil, fmt.Errorf("string %d reference: %w", index, err)
		}
		refs[index] = ref
	}
	if err := validateContextStringRefs(data, fixedLength, refs); err != nil {
		return nil, fmt.Errorf("validating strings: %w", err)
	}
	return refs, nil
}

func lteAttachStrings(data []byte, refs []valueRef) ([]string, error) {
	values := make([]string, len(refs))
	for index, ref := range refs {
		value, err := utf16String(data, ref)
		if err != nil {
			return nil, fmt.Errorf("decoding string %d: %w", index, err)
		}
		values[index] = value
	}
	return values, nil
}

func (c *Client) LTEAttachConfigurations(ctx context.Context) ([]LTEAttachConfiguration, error) {
	request := LTEAttachConfigurationRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM LTE attach configurations: %w", err)
	}
	return slices.Clone(request.Response.Configurations), nil
}

func (c *Client) SetLTEAttachConfigurations(ctx context.Context, operation LTEAttachContextOperation, configurations []LTEAttachConfiguration) ([]LTEAttachConfiguration, error) {
	if operation > LTEAttachContextOperationRestoreFactory {
		return nil, fmt.Errorf("setting MBIM LTE attach configurations: operation %d is outside 0..%d", operation, LTEAttachContextOperationRestoreFactory)
	}
	for index, configuration := range configurations {
		if _, err := configuration.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("encoding MBIM LTE attach configuration %d: %w", index, err)
		}
	}
	request := LTEAttachConfigurationSetRequest{
		TransactionID:  c.nextTransactionID(),
		Operation:      operation,
		Configurations: configurations,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM LTE attach configurations: %w", err)
	}
	return slices.Clone(request.Response.Configurations), nil
}

func (c *Client) LTEAttachInfo(ctx context.Context) (LTEAttachInfo, error) {
	request := LTEAttachInfoRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return LTEAttachInfo{}, fmt.Errorf("reading MBIM LTE attach info: %w", err)
	}
	return *request.Response, nil
}
