package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type SubscriberReadyState uint32

const (
	SubscriberReadyStateNotInitialized SubscriberReadyState = iota
	SubscriberReadyStateInitialized
	SubscriberReadyStateSIMNotInserted
	SubscriberReadyStateBadSIM
	SubscriberReadyStateFailure
	SubscriberReadyStateNotActivated
	SubscriberReadyStateDeviceLocked
	SubscriberReadyStateNoESIMProfile
)

type ReadyInfo uint32

const (
	ReadyInfoNone            ReadyInfo = 0
	ReadyInfoProtectUniqueID ReadyInfo = 1 << 0
)

type SubscriberReadyStatusFlags uint32

const (
	SubscriberReadyStatusFlagNone                 SubscriberReadyStatusFlags = 0
	SubscriberReadyStatusFlagESIM                 SubscriberReadyStatusFlags = 1 << 0
	SubscriberReadyStatusFlagSIMRemovabilityKnown SubscriberReadyStatusFlags = 1 << 1
	SubscriberReadyStatusFlagSIMRemovable         SubscriberReadyStatusFlags = 1 << 2
	SubscriberReadyStatusFlagSIMSlotActive        SubscriberReadyStatusFlags = 1 << 3
)

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
	r.Flags = SubscriberReadyStatusFlagNone
	r.SlotID = activeSubscriberSlot

	switch {
	case r.MBIMExVersion >= mbimExVersion40:
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
	case r.MBIMExVersion >= mbimExVersion30:
		if len(data) < 32 {
			return errors.New("parsing MBIM subscriber ready status: payload is truncated")
		}
		r.Flags = SubscriberReadyStatusFlags(binary.LittleEndian.Uint32(data[4:8]))
		subscriberRefOffset = 8
		simRefOffset = 16
		readyInfoOffset = 24
		countOffset = 28
		telephoneTableOffset = 32
	}

	readyState := SubscriberReadyState(binary.LittleEndian.Uint32(data[:4]))
	maximumReadyState := SubscriberReadyStateDeviceLocked
	if r.MBIMExVersion >= mbimExVersion30 {
		maximumReadyState = SubscriberReadyStateNoESIMProfile
	}
	if readyState > maximumReadyState {
		return fmt.Errorf("parsing MBIM subscriber ready status: ready state %d is reserved in MBIMEx %#x", readyState, r.MBIMExVersion)
	}
	if r.MBIMExVersion >= mbimExVersion30 {
		flagsMask := SubscriberReadyStatusFlagESIM |
			SubscriberReadyStatusFlagSIMRemovabilityKnown |
			SubscriberReadyStatusFlagSIMRemovable
		if r.MBIMExVersion >= mbimExVersion40 {
			flagsMask |= SubscriberReadyStatusFlagSIMSlotActive
		}
		if r.Flags&^flagsMask != 0 {
			return fmt.Errorf("parsing MBIM subscriber ready status: flags %#x contain reserved bits", r.Flags)
		}
		if r.Flags&SubscriberReadyStatusFlagSIMRemovable != 0 &&
			r.Flags&SubscriberReadyStatusFlagSIMRemovabilityKnown == 0 {
			return errors.New("parsing MBIM subscriber ready status: SIM removable flag requires SIM removability known")
		}
		if r.Flags&SubscriberReadyStatusFlagESIM != 0 &&
			readyState != SubscriberReadyStateInitialized &&
			readyState != SubscriberReadyStateNoESIMProfile {
			return fmt.Errorf("parsing MBIM subscriber ready status: eSIM flag is invalid for ready state %d", readyState)
		}
		if r.Flags&SubscriberReadyStatusFlagSIMRemovable != 0 &&
			readyState != SubscriberReadyStateInitialized &&
			readyState != SubscriberReadyStateNoESIMProfile &&
			readyState != SubscriberReadyStateDeviceLocked {
			return fmt.Errorf("parsing MBIM subscriber ready status: SIM removable flag is invalid for ready state %d", readyState)
		}
	}
	if r.MBIMExVersion >= mbimExVersion40 && r.SlotID > 1 {
		return fmt.Errorf("parsing MBIM subscriber ready status: slot ID %d is reserved", r.SlotID)
	}

	subscriberIDRef, err := readOffsetSizeRef(data, subscriberRefOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status subscriber ID: %w", err)
	}
	simICCIDRef, err := readOffsetSizeRef(data, simRefOffset)
	if err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status SIM ICCID: %w", err)
	}
	readyInfo := ReadyInfo(binary.LittleEndian.Uint32(data[readyInfoOffset : readyInfoOffset+4]))
	if readyInfo&^ReadyInfoProtectUniqueID != 0 {
		return fmt.Errorf("parsing MBIM subscriber ready status: ready info %#x contains reserved bits", readyInfo)
	}
	telephoneNumbersCount := binary.LittleEndian.Uint32(data[countOffset : countOffset+4])
	if readyState != SubscriberReadyStateInitialized && telephoneNumbersCount != 0 {
		return fmt.Errorf("parsing MBIM subscriber ready status: ready state %d has %d telephone numbers", readyState, telephoneNumbersCount)
	}
	r.ReadyState = readyState
	r.ReadyInfo = readyInfo
	r.TelephoneNumbersCount = telephoneNumbersCount

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
	dataStart := telephoneTableOffset + r.TelephoneNumbersCount*8
	maximumSizes := make([]uint32, len(refs))
	maximumSizes[0] = 30
	maximumSizes[1] = 40
	for i := 2; i < len(maximumSizes); i++ {
		maximumSizes[i] = 44
	}
	for i, ref := range refs {
		if ref.size > maximumSizes[i] {
			return fmt.Errorf("parsing MBIM subscriber ready status string %d: size %d exceeds %d bytes", i, ref.size, maximumSizes[i])
		}
	}
	if err := validateDataBufferRefs(data, dataStart, refs); err != nil {
		return fmt.Errorf("parsing MBIM subscriber ready status data buffer: %w", err)
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

func (c *Client) SubscriberReadyStatus(ctx context.Context) (SubscriberReadyStatusResponse, error) {
	slotID := c.subscriberReadySlotID()
	if c.mbimExVersion >= mbimExVersion40 && slotID > 1 && slotID != activeSubscriberSlot {
		return SubscriberReadyStatusResponse{}, fmt.Errorf("reading MBIM subscriber ready status: slot ID %d is reserved", slotID)
	}
	request := SubscriberReadyStatusRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		SlotID:        slotID,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SubscriberReadyStatusResponse{}, fmt.Errorf("reading MBIM subscriber ready status: %w", err)
	}
	resp := *request.Response
	resp.TelephoneNumbers = slices.Clone(resp.TelephoneNumbers)
	return resp, nil
}
