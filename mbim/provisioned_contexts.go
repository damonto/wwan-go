package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

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

type ProvisionedContextsInfo struct {
	Contexts []ProvisionedContext
}

func (r *ProvisionedContextsInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM provisioned contexts: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[:4])
	if count > (uint32(len(data))-4)/8 {
		return errors.New("parsing MBIM provisioned contexts: reference list is truncated")
	}
	contexts := make([]ProvisionedContext, count)
	for i := range count {
		ref, err := readOffsetSizeRef(data, 4+i*8)
		if err != nil {
			return fmt.Errorf("parsing MBIM provisioned context %d reference: %w", i, err)
		}
		if err := ref.validate(data); err != nil {
			return fmt.Errorf("parsing MBIM provisioned context %d: %w", i, err)
		}
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
	if err := validateUTF16Refs(data, refs); err != nil {
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
	*r = ProvisionedContext{
		ContextID:    binary.LittleEndian.Uint32(data[0:4]),
		AccessString: values[0],
		UserName:     values[1],
		Password:     values[2],
		Compression:  Compression(binary.LittleEndian.Uint32(data[44:48])),
		AuthProtocol: AuthProtocol(binary.LittleEndian.Uint32(data[48:52])),
	}
	copy(r.ContextType[:], data[4:20])
	return nil
}

func (r *Reader) ProvisionedContexts(ctx context.Context) ([]ProvisionedContext, error) {
	request := ProvisionedContextsRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM provisioned contexts: %w", err)
	}
	return request.Response.Contexts, nil
}
