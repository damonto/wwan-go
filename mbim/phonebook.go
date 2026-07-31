package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type PhonebookState uint32

const (
	PhonebookStateNotInitialized PhonebookState = 0
	PhonebookStateInitialized    PhonebookState = 1
)

type PhonebookConfigurationInfo struct {
	State           PhonebookState
	TotalEntries    uint32
	UsedEntries     uint32
	MaxNumberLength uint32
	MaxNameLength   uint32
}

type PhonebookFlag uint32

const (
	PhonebookFlagAll   PhonebookFlag = 0
	PhonebookFlagIndex PhonebookFlag = 1
)

type PhonebookEntry struct {
	Index  uint32
	Number string
	Name   string
}

type PhonebookWriteFlag uint32

const (
	PhonebookWriteFlagUnused PhonebookWriteFlag = 0
	PhonebookWriteFlagIndex  PhonebookWriteFlag = 1
)

func validatePhonebookIndex(flag PhonebookFlag, index uint32) error {
	if flag > PhonebookFlagIndex {
		return fmt.Errorf("flag %d is outside 0..%d", flag, PhonebookFlagIndex)
	}
	if flag == PhonebookFlagIndex {
		if index == 0 {
			return errors.New("index must be non-zero when the index flag is used")
		}
		return nil
	}
	if index != 0 {
		return errors.New("index must be zero when the all flag is used")
	}
	return nil
}

func validatePhonebookWriteIndex(flag PhonebookWriteFlag, index uint32) error {
	if flag > PhonebookWriteFlagIndex {
		return fmt.Errorf("flag %d is outside 0..%d", flag, PhonebookWriteFlagIndex)
	}
	if flag == PhonebookWriteFlagIndex {
		if index == 0 {
			return errors.New("index must be non-zero when the index flag is used")
		}
		return nil
	}
	if index != 0 {
		return errors.New("index must be zero when the unused-entry flag is used")
	}
	return nil
}

type PhonebookConfigurationRequest struct {
	TransactionID uint32
	Response      *PhonebookConfigurationInfo
}

func (r *PhonebookConfigurationRequest) Request() *Request {
	r.Response = new(PhonebookConfigurationInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServicePhonebook, CIDPhonebookConfiguration, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *PhonebookConfigurationInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 20 {
		return fmt.Errorf("parsing MBIM phonebook configuration: payload length is %d, want 20", len(data))
	}
	state := PhonebookState(binary.LittleEndian.Uint32(data[0:4]))
	if state > PhonebookStateInitialized {
		return fmt.Errorf("parsing MBIM phonebook configuration: state %d is outside 0..%d", state, PhonebookStateInitialized)
	}
	totalEntries := binary.LittleEndian.Uint32(data[4:8])
	usedEntries := binary.LittleEndian.Uint32(data[8:12])
	if usedEntries > totalEntries {
		return fmt.Errorf("parsing MBIM phonebook configuration: used entries %d exceed total entries %d", usedEntries, totalEntries)
	}
	*r = PhonebookConfigurationInfo{
		State:           state,
		TotalEntries:    totalEntries,
		UsedEntries:     usedEntries,
		MaxNumberLength: binary.LittleEndian.Uint32(data[12:16]),
		MaxNameLength:   binary.LittleEndian.Uint32(data[16:20]),
	}
	return nil
}

type PhonebookReadRequest struct {
	TransactionID uint32
	Flag          PhonebookFlag
	Index         uint32
	Response      *PhonebookReadInfo
}

func (r *PhonebookReadRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Flag))
	data = binary.LittleEndian.AppendUint32(data, r.Index)
	r.Response = new(PhonebookReadInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServicePhonebook, CIDPhonebookRead, CommandTypeQuery, data),
		Response:      r.Response,
	}
}

type PhonebookReadInfo struct {
	Entries []PhonebookEntry
}

func (r *PhonebookReadInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM phonebook entries: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM phonebook entries: %w", err)
	}
	entries := make([]PhonebookEntry, count)
	for i, ref := range refs {
		if err := entries[i].UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
			return fmt.Errorf("parsing MBIM phonebook entry %d: %w", i, err)
		}
	}
	r.Entries = entries
	return nil
}

func (e *PhonebookEntry) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("payload is truncated")
	}
	numberRef, err := readOffsetSizeRef(data, 4)
	if err != nil {
		return fmt.Errorf("number: %w", err)
	}
	nameRef, err := readOffsetSizeRef(data, 12)
	if err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if err := validateDataBufferRefs(data, 20, []valueRef{numberRef, nameRef}); err != nil {
		return fmt.Errorf("data buffer: %w", err)
	}
	if err := validateUTF16Refs(data, []valueRef{numberRef, nameRef}); err != nil {
		return fmt.Errorf("strings: %w", err)
	}
	number, err := utf16String(data, numberRef)
	if err != nil {
		return fmt.Errorf("number: %w", err)
	}
	name, err := utf16String(data, nameRef)
	if err != nil {
		return fmt.Errorf("name: %w", err)
	}
	*e = PhonebookEntry{
		Index:  binary.LittleEndian.Uint32(data[0:4]),
		Number: number,
		Name:   name,
	}
	return nil
}

type PhonebookDeleteRequest struct {
	TransactionID uint32
	Flag          PhonebookFlag
	Index         uint32
	Response      *emptyResponse
}

func (r *PhonebookDeleteRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Flag))
	data = binary.LittleEndian.AppendUint32(data, r.Index)
	r.Response = new(emptyResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServicePhonebook, CIDPhonebookDelete, CommandTypeSet, data),
		Response:      r.Response,
	}
}

type PhonebookWriteRequest struct {
	TransactionID uint32
	Flag          PhonebookWriteFlag
	Index         uint32
	Number        string
	Name          string
	Response      *emptyResponse
}

func (r *PhonebookWriteRequest) Request() *Request {
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.Flag))
	binary.LittleEndian.PutUint32(data[4:8], r.Index)
	data = appendRefValue(data, 8, utf16Bytes(r.Number))
	data = appendRefValue(data, 16, utf16Bytes(r.Name))
	r.Response = new(emptyResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServicePhonebook, CIDPhonebookWrite, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (c *Client) PhonebookConfiguration(ctx context.Context) (PhonebookConfigurationInfo, error) {
	request := PhonebookConfigurationRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return PhonebookConfigurationInfo{}, fmt.Errorf("reading MBIM phonebook configuration: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) ReadPhonebook(ctx context.Context, flag PhonebookFlag, index uint32) ([]PhonebookEntry, error) {
	if err := validatePhonebookIndex(flag, index); err != nil {
		return nil, fmt.Errorf("reading MBIM phonebook: %w", err)
	}
	request := PhonebookReadRequest{
		TransactionID: c.nextTransactionID(),
		Flag:          flag,
		Index:         index,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM phonebook: %w", err)
	}
	return slices.Clone(request.Response.Entries), nil
}

func (c *Client) DeletePhonebook(ctx context.Context, flag PhonebookFlag, index uint32) error {
	if err := validatePhonebookIndex(flag, index); err != nil {
		return fmt.Errorf("deleting MBIM phonebook entries: %w", err)
	}
	request := PhonebookDeleteRequest{
		TransactionID: c.nextTransactionID(),
		Flag:          flag,
		Index:         index,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("deleting MBIM phonebook entries: %w", err)
	}
	return nil
}

func (c *Client) WritePhonebook(ctx context.Context, flag PhonebookWriteFlag, index uint32, number, name string) error {
	if err := validatePhonebookWriteIndex(flag, index); err != nil {
		return fmt.Errorf("writing MBIM phonebook entry: %w", err)
	}
	request := PhonebookWriteRequest{
		TransactionID: c.nextTransactionID(),
		Flag:          flag,
		Index:         index,
		Number:        number,
		Name:          name,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("writing MBIM phonebook entry: %w", err)
	}
	return nil
}
