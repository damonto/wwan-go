package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type RegistrationStateRequest struct {
	TransactionID uint32
	Response      *RegistrationStateInfo
}

func (r *RegistrationStateRequest) Request() *Request {
	r.Response = new(RegistrationStateInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDRegisterState, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *RegistrationStateInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 48 {
		return errors.New("parsing MBIM registration state: payload is truncated")
	}
	refs := make([]valueRef, 3)
	for i, offset := range []uint32{20, 28, 36} {
		ref, err := readOffsetSizeRef(data, offset)
		if err != nil {
			return fmt.Errorf("parsing MBIM registration state string reference: %w", err)
		}
		refs[i] = ref
	}
	if err := validateUTF16Refs(data, refs); err != nil {
		return fmt.Errorf("parsing MBIM registration state strings: %w", err)
	}
	providerID, err := utf16String(data, refs[0])
	if err != nil {
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
		NwError:              binary.LittleEndian.Uint32(data[0:4]),
		RegisterState:        RegisterState(binary.LittleEndian.Uint32(data[4:8])),
		RegisterMode:         RegisterMode(binary.LittleEndian.Uint32(data[8:12])),
		AvailableDataClasses: binary.LittleEndian.Uint32(data[12:16]),
		CurrentCellularClass: binary.LittleEndian.Uint32(data[16:20]),
		ProviderID:           providerID,
		ProviderName:         providerName,
		RoamingText:          roamingText,
		RegistrationFlags:    RegistrationFlags(binary.LittleEndian.Uint32(data[44:48])),
	}
	return nil
}

func (r *Reader) RegistrationState(ctx context.Context) (RegistrationStateInfo, error) {
	request := RegistrationStateRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return RegistrationStateInfo{}, fmt.Errorf("reading MBIM registration state: %w", err)
	}
	return *request.Response, nil
}
