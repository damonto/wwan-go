package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type ModemConfigurationStatus uint32

const (
	ModemConfigurationStatusUnknown   ModemConfigurationStatus = 0
	ModemConfigurationStatusStarted   ModemConfigurationStatus = 1
	ModemConfigurationStatusCompleted ModemConfigurationStatus = 2
)

type ModemConfigurationRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *ModemConfigurationInfo
}

func (r *ModemConfigurationRequest) Request() *Request {
	r.Response = &ModemConfigurationInfo{MBIMExVersion: modemConfigurationVersion(r.MBIMExVersion)}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsModemConfiguration,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type ModemConfigurationInfo struct {
	MBIMExVersion             uint16
	Status                    ModemConfigurationStatus
	ConfigName                string
	TLVs                      TLVs
	PreconfiguredDefaultNSSAI PreconfiguredDefaultNSSAIList
}

func (i ModemConfigurationInfo) MarshalBinary() ([]byte, error) {
	version := modemConfigurationVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return nil, errors.New("encoding MBIM modem configuration: CID requires MBIMEx 3.0")
	}
	if err := validateModemConfigurationStatus(i.Status); err != nil {
		return nil, fmt.Errorf("encoding MBIM modem configuration: %w", err)
	}
	if i.Status == ModemConfigurationStatusCompleted && i.ConfigName == "" {
		return nil, errors.New("encoding MBIM modem configuration: completed configuration name is empty")
	}
	if err := i.unmarshalTLVs(i.TLVs, version); err != nil {
		return nil, fmt.Errorf("encoding MBIM modem configuration: %w", err)
	}
	optional, err := i.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding MBIM modem configuration TLVs: %w", err)
	}

	data := binary.LittleEndian.AppendUint32(nil, uint32(i.Status))
	data = append(data, marshalTLV(TLVTypeWCharString, utf16Bytes(i.ConfigName))...)
	data = append(data, optional...)
	return data, nil
}

func (i *ModemConfigurationInfo) UnmarshalBinary(data []byte) error {
	version := modemConfigurationVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return errors.New("parsing MBIM modem configuration: CID requires MBIMEx 3.0")
	}
	if len(data) < 12 {
		return errors.New("parsing MBIM modem configuration: payload is truncated")
	}
	configNameTLV, consumed, err := unmarshalTLVPrefix(data[4:])
	if err != nil {
		return fmt.Errorf("parsing MBIM modem configuration name: %w", err)
	}
	if configNameTLV.Type != TLVTypeWCharString {
		return fmt.Errorf("parsing MBIM modem configuration name: TLV type is %d, want %d", configNameTLV.Type, TLVTypeWCharString)
	}
	configName, err := utf16RawString(configNameTLV.Data)
	if err != nil {
		return fmt.Errorf("parsing MBIM modem configuration name: %w", err)
	}

	var optional TLVs
	if err := optional.UnmarshalBinary(data[4+consumed:]); err != nil {
		return fmt.Errorf("parsing MBIM modem configuration TLVs: %w", err)
	}
	status := ModemConfigurationStatus(binary.LittleEndian.Uint32(data[:4]))
	if err := validateModemConfigurationStatus(status); err != nil {
		return fmt.Errorf("parsing MBIM modem configuration: %w", err)
	}
	if status == ModemConfigurationStatusCompleted && configName == "" {
		return errors.New("parsing MBIM modem configuration: completed configuration name is empty")
	}
	info := ModemConfigurationInfo{
		MBIMExVersion: version,
		Status:        status,
		ConfigName:    configName,
		TLVs:          optional,
	}
	if err := info.unmarshalTLVs(optional, version); err != nil {
		return fmt.Errorf("parsing MBIM modem configuration: %w", err)
	}
	*i = info
	return nil
}

func (i *ModemConfigurationInfo) unmarshalTLVs(tlvs TLVs, version uint16) error {
	var preconfigured PreconfiguredDefaultNSSAIList
	for index, tlv := range tlvs {
		if tlv.Type != TLVTypePreconfiguredDefaultConfiguredNSSAI {
			continue
		}
		if preconfigured != nil {
			return errors.New("more than one preconfigured default NSSAI TLV")
		}
		var values PreconfiguredDefaultNSSAIList
		if err := values.UnmarshalTLV(tlv); err != nil {
			return fmt.Errorf("preconfigured default NSSAI TLV %d: %w", index, err)
		}
		if version < mbimExVersion40 {
			if err := validateMBIMEx3PreconfiguredDefaultNSSAI(values); err != nil {
				return err
			}
		}
		preconfigured = values
	}
	i.PreconfiguredDefaultNSSAI = preconfigured
	return nil
}

func validateModemConfigurationStatus(status ModemConfigurationStatus) error {
	if status > ModemConfigurationStatusCompleted {
		return fmt.Errorf("configuration status %d is reserved", status)
	}
	return nil
}

func modemConfigurationVersion(version uint16) uint16 {
	if version == 0 {
		return mbimExVersion30
	}
	return version
}

func (c *Client) ModemConfiguration(ctx context.Context) (ModemConfigurationInfo, error) {
	if c.mbimExVersion < mbimExVersion30 {
		return ModemConfigurationInfo{}, errors.New("reading MBIM modem configuration: CID requires MBIMEx 3.0")
	}
	request := ModemConfigurationRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return ModemConfigurationInfo{}, fmt.Errorf("reading MBIM modem configuration: %w", err)
	}
	return *request.Response, nil
}
