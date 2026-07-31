package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type ProvisionedContext struct {
	ContextID    uint32
	ContextType  ContextType
	AccessString string
	UserName     string
	Password     string
	Compression  Compression
	AuthProtocol AuthProtocol
}

type ProvisionedContextsRequest struct {
	TransactionID uint32
	Response      *ProvisionedContextsInfo
}

func (r *ProvisionedContextsRequest) Request() *Request {
	r.Response = new(ProvisionedContextsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDProvisionedContexts, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type ProvisionedContextSetRequest struct {
	TransactionID uint32
	Context       ProvisionedContext
	ProviderID    string
	Response      *ProvisionedContextsInfo
}

func (r *ProvisionedContextSetRequest) Request() *Request {
	r.Response = new(ProvisionedContextsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDProvisionedContexts,
			CommandTypeSet,
			marshalProvisionedContextSet(r.Context, r.ProviderID),
		),
		Response: r.Response,
	}
}

type ProvisionedContextsInfo struct {
	Contexts []ProvisionedContext
}

func (r *ProvisionedContextsInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM provisioned contexts: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM provisioned contexts: %w", err)
	}
	contexts := make([]ProvisionedContext, count)
	for i, ref := range refs {
		if err := contexts[i].unmarshalBinary(ref.bytes(data)); err != nil {
			return fmt.Errorf("parsing MBIM provisioned context %d: %w", i, err)
		}
	}
	r.Contexts = contexts
	return nil
}

func (r *ProvisionedContext) unmarshalBinary(data []byte) error {
	if len(data) < 52 {
		return errors.New("payload is truncated")
	}
	refs := make([]valueRef, 3)
	for i, offset := range []uint32{20, 28, 36} {
		ref, err := readOffsetSizeRef(data, offset)
		if err != nil {
			return fmt.Errorf("reading string reference: %w", err)
		}
		refs[i] = ref
	}
	if err := validateContextStringRefs(data, 52, refs); err != nil {
		return fmt.Errorf("validating strings: %w", err)
	}
	values := make([]string, 3)
	for i, ref := range refs {
		value, err := utf16String(data, ref)
		if err != nil {
			return fmt.Errorf("decoding string %d: %w", i, err)
		}
		values[i] = value
	}
	result := ProvisionedContext{
		ContextID:    binary.LittleEndian.Uint32(data[0:4]),
		AccessString: values[0],
		UserName:     values[1],
		Password:     values[2],
		Compression:  Compression(binary.LittleEndian.Uint32(data[44:48])),
		AuthProtocol: AuthProtocol(binary.LittleEndian.Uint32(data[48:52])),
	}
	if result.ContextID == ^uint32(0) {
		return errors.New("context ID is the set-only automatic value")
	}
	if !validCompression(result.Compression) {
		return fmt.Errorf("compression %d is outside 0..%d", result.Compression, CompressionEnable)
	}
	if !validAuthProtocol(result.AuthProtocol) {
		return fmt.Errorf("authentication protocol %d is outside 0..%d", result.AuthProtocol, AuthProtocolMSCHAPV2)
	}
	copy(result.ContextType[:], data[4:20])
	*r = result
	return nil
}

func validateProvisionedContextSet(provisioned ProvisionedContext, providerID string) error {
	if !validCompression(provisioned.Compression) {
		return fmt.Errorf("encoding MBIM provisioned context: compression %d is outside 0..%d", provisioned.Compression, CompressionEnable)
	}
	if !validAuthProtocol(provisioned.AuthProtocol) {
		return fmt.Errorf("encoding MBIM provisioned context: authentication protocol %d is outside 0..%d", provisioned.AuthProtocol, AuthProtocolMSCHAPV2)
	}
	if len(utf16Bytes(provisioned.AccessString)) > accessStringMaximumSize {
		return fmt.Errorf("encoding MBIM provisioned context: access string exceeds %d bytes", accessStringMaximumSize)
	}
	if len(utf16Bytes(provisioned.UserName)) > userNameMaximumSize {
		return fmt.Errorf("encoding MBIM provisioned context: user name exceeds %d bytes", userNameMaximumSize)
	}
	if len(utf16Bytes(provisioned.Password)) > passwordMaximumSize {
		return fmt.Errorf("encoding MBIM provisioned context: password exceeds %d bytes", passwordMaximumSize)
	}
	if err := validateProviderID(providerID); err != nil {
		return fmt.Errorf("encoding MBIM provisioned context: %w", err)
	}
	return nil
}

func marshalProvisionedContextSet(provisioned ProvisionedContext, providerID string) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[0:4], provisioned.ContextID)
	copy(data[4:20], provisioned.ContextType[:])
	binary.LittleEndian.PutUint32(data[44:48], uint32(provisioned.Compression))
	binary.LittleEndian.PutUint32(data[48:52], uint32(provisioned.AuthProtocol))
	data = appendRefValue(data, 20, utf16Bytes(provisioned.AccessString))
	data = appendRefValue(data, 28, utf16Bytes(provisioned.UserName))
	data = appendRefValue(data, 36, utf16Bytes(provisioned.Password))
	return appendRefValue(data, 52, utf16Bytes(providerID))
}

func (c *Client) ProvisionedContexts(ctx context.Context) ([]ProvisionedContext, error) {
	request := ProvisionedContextsRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM provisioned contexts: %w", err)
	}
	return slices.Clone(request.Response.Contexts), nil
}

func (c *Client) SetProvisionedContext(ctx context.Context, provisioned ProvisionedContext, providerID string) ([]ProvisionedContext, error) {
	if err := validateProvisionedContextSet(provisioned, providerID); err != nil {
		return nil, err
	}
	request := ProvisionedContextSetRequest{
		TransactionID: c.nextTransactionID(),
		Context:       provisioned,
		ProviderID:    providerID,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("setting MBIM provisioned context: %w", err)
	}
	return slices.Clone(request.Response.Contexts), nil
}
