package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type ProvisionedContextsV2Request struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *ProvisionedContextsV2Info
}

func (r *ProvisionedContextsV2Request) Request() *Request {
	r.Response = &ProvisionedContextsV2Info{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsProvisionedContexts,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type ProvisionedContextV2SetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Operation     ContextOperation
	Context       ProvisionedContextV2
	Response      *ProvisionedContextsV2Info
}

func (r *ProvisionedContextV2SetRequest) Request() *Request {
	data, err := marshalProvisionedContextV2Set(r.Operation, r.Context, r.MBIMExVersion)
	r.Response = &ProvisionedContextsV2Info{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsProvisionedContexts,
			CommandTypeSet,
			data,
			err,
		),
		Response: r.Response,
	}
}

func marshalProvisionedContextV2Set(operation ContextOperation, context ProvisionedContextV2, version uint16) ([]byte, error) {
	if operation > ContextOperationRestoreFactory {
		return nil, fmt.Errorf("encoding MBIM provisioned context V2: operation %d is outside 0..%d", operation, ContextOperationRestoreFactory)
	}
	context.MBIMExVersion = version
	if err := context.validate(); err != nil {
		return nil, fmt.Errorf("encoding MBIM provisioned context V2: %w", err)
	}
	return marshalProvisionedContextV2(uint32(operation), context, version), nil
}

type ProvisionedContextV2 struct {
	MBIMExVersion uint16
	ContextID     uint32
	ContextType   ContextType
	IPType        ContextIPType
	State         ContextState
	Roaming       ContextRoamingControl
	MediaType     ContextMediaType
	Source        ContextSource
	AccessString  string
	UserName      string
	Password      string
	Compression   Compression
	AuthProtocol  AuthProtocol
	SNSSAI        *SNSSAI
}

func (c ProvisionedContextV2) MarshalBinary() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("encoding MBIM provisioned context V2: %w", err)
	}
	return marshalProvisionedContextV2(c.ContextID, c, c.MBIMExVersion), nil
}

func (c ProvisionedContextV2) validate() error {
	if !validContextIPType(c.IPType) {
		return fmt.Errorf("IP type %d is outside 0..%d", c.IPType, ContextIPTypeIPv4AndIPv6)
	}
	if c.State > ContextStateEnabled {
		return fmt.Errorf("state %d is outside 0..%d", c.State, ContextStateEnabled)
	}
	if c.Roaming > ContextRoamingControlAllowAll {
		return fmt.Errorf("roaming control %d is outside 0..%d", c.Roaming, ContextRoamingControlAllowAll)
	}
	if c.MediaType > ContextMediaTypeAll {
		return fmt.Errorf("media type %d is outside 0..%d", c.MediaType, ContextMediaTypeAll)
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
	if c.SNSSAI != nil && c.MBIMExVersion < mbimExVersion40 {
		return errors.New("S-NSSAI requires MBIMEx 4.0")
	}
	if c.SNSSAI != nil {
		if c.SNSSAI.HasMappedSliceServiceType || c.SNSSAI.HasMappedSliceDifferentiator {
			return errors.New("S-NSSAI must not contain mapped values")
		}
		if _, err := c.SNSSAI.MarshalBinary(); err != nil {
			return fmt.Errorf("S-NSSAI: %w", err)
		}
	}
	return nil
}

func marshalProvisionedContextV2(firstField uint32, context ProvisionedContextV2, version uint16) []byte {
	data := make([]byte, 72)
	binary.LittleEndian.PutUint32(data[0:4], firstField)
	copy(data[4:20], context.ContextType[:])
	binary.LittleEndian.PutUint32(data[20:24], uint32(context.IPType))
	binary.LittleEndian.PutUint32(data[24:28], uint32(context.State))
	binary.LittleEndian.PutUint32(data[28:32], uint32(context.Roaming))
	binary.LittleEndian.PutUint32(data[32:36], uint32(context.MediaType))
	binary.LittleEndian.PutUint32(data[36:40], uint32(context.Source))
	binary.LittleEndian.PutUint32(data[64:68], uint32(context.Compression))
	binary.LittleEndian.PutUint32(data[68:72], uint32(context.AuthProtocol))
	if version >= mbimExVersion40 {
		var snssaiData []byte
		if context.SNSSAI != nil {
			snssaiData = context.SNSSAI.marshalBinaryUnchecked()
		}
		data = append(data, marshalTLV(TLVTypeSingleNSSAI, snssaiData)...)
	}
	data = appendRefValue(data, 40, utf16Bytes(context.AccessString))
	data = appendRefValue(data, 48, utf16Bytes(context.UserName))
	return appendRefValue(data, 56, utf16Bytes(context.Password))
}

func (c *ProvisionedContextV2) UnmarshalBinary(data []byte) error {
	version := c.MBIMExVersion
	if len(data) < 72 {
		return errors.New("parsing MBIM provisioned context V2: payload is truncated")
	}
	dataStart := uint32(72)
	var snssai *SNSSAI
	if version >= mbimExVersion40 {
		tlv, consumed, err := unmarshalTLVPrefix(data[72:])
		if err != nil {
			return fmt.Errorf("parsing MBIM provisioned context V2 S-NSSAI: %w", err)
		}
		var value OptionalSNSSAI
		if err := value.UnmarshalTLV(tlv); err != nil {
			return err
		}
		snssai = value.Value
		dataStart += uint32(consumed)
	}

	refs := make([]valueRef, 3)
	for index, offset := range []uint32{40, 48, 56} {
		ref, err := readOffsetSizeRef(data, offset)
		if err != nil {
			return fmt.Errorf("parsing MBIM provisioned context V2 string %d reference: %w", index, err)
		}
		refs[index] = ref
	}
	if err := validateContextStringRefs(data, dataStart, refs); err != nil {
		return fmt.Errorf("parsing MBIM provisioned context V2 strings: %w", err)
	}
	values := make([]string, 3)
	for index, ref := range refs {
		value, err := utf16String(data, ref)
		if err != nil {
			return fmt.Errorf("parsing MBIM provisioned context V2 string %d: %w", index, err)
		}
		values[index] = value
	}

	result := ProvisionedContextV2{
		MBIMExVersion: version,
		ContextID:     binary.LittleEndian.Uint32(data[0:4]),
		IPType:        ContextIPType(binary.LittleEndian.Uint32(data[20:24])),
		State:         ContextState(binary.LittleEndian.Uint32(data[24:28])),
		Roaming:       ContextRoamingControl(binary.LittleEndian.Uint32(data[28:32])),
		MediaType:     ContextMediaType(binary.LittleEndian.Uint32(data[32:36])),
		Source:        ContextSource(binary.LittleEndian.Uint32(data[36:40])),
		AccessString:  values[0],
		UserName:      values[1],
		Password:      values[2],
		Compression:   Compression(binary.LittleEndian.Uint32(data[64:68])),
		AuthProtocol:  AuthProtocol(binary.LittleEndian.Uint32(data[68:72])),
		SNSSAI:        snssai,
	}
	if err := result.validate(); err != nil {
		return fmt.Errorf("parsing MBIM provisioned context V2: %w", err)
	}
	copy(result.ContextType[:], data[4:20])
	*c = result
	return nil
}

type ProvisionedContextsV2Info struct {
	MBIMExVersion uint16
	Contexts      []ProvisionedContextV2
}

func (i ProvisionedContextsV2Info) MarshalBinary() ([]byte, error) {
	elements := make([][]byte, len(i.Contexts))
	for index, context := range i.Contexts {
		context.MBIMExVersion = i.MBIMExVersion
		data, err := context.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding MBIM provisioned context V2 %d: %w", index, err)
		}
		elements[index] = data
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements), nil
}

func (i *ProvisionedContextsV2Info) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM provisioned contexts V2: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM provisioned contexts V2: %w", err)
	}
	contexts := make([]ProvisionedContextV2, count)
	for index, ref := range refs {
		contexts[index].MBIMExVersion = i.MBIMExVersion
		if err := contexts[index].UnmarshalBinary(ref.bytes(data)); err != nil {
			return fmt.Errorf("parsing MBIM provisioned context V2 %d: %w", index, err)
		}
	}
	i.Contexts = contexts
	return nil
}

func cloneProvisionedContextsV2(contexts []ProvisionedContextV2) []ProvisionedContextV2 {
	result := make([]ProvisionedContextV2, len(contexts))
	copy(result, contexts)
	for index, context := range result {
		if context.SNSSAI == nil {
			continue
		}
		value := *context.SNSSAI
		result[index].SNSSAI = &value
	}
	return result
}

func (c *Client) ProvisionedContextsV2(ctx context.Context) ([]ProvisionedContextV2, error) {
	request := ProvisionedContextsV2Request{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM provisioned contexts V2: %w", err)
	}
	return cloneProvisionedContextsV2(request.Response.Contexts), nil
}

func (c *Client) SetProvisionedContextV2(ctx context.Context, operation ContextOperation, context ProvisionedContextV2) ([]ProvisionedContextV2, error) {
	if operation > ContextOperationRestoreFactory {
		return nil, fmt.Errorf("setting MBIM provisioned context V2: operation %d is outside 0..%d", operation, ContextOperationRestoreFactory)
	}
	context.MBIMExVersion = c.mbimExVersion
	if err := context.validate(); err != nil {
		return nil, fmt.Errorf("setting MBIM provisioned context V2: %w", err)
	}
	request := ProvisionedContextV2SetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Operation:     operation,
		Context:       context,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM provisioned context V2: %w", err)
	}
	return cloneProvisionedContextsV2(request.Response.Contexts), nil
}
