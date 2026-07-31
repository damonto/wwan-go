package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type RegistrationParametersRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *RegistrationParametersInfo
}

func (r *RegistrationParametersRequest) Request() *Request {
	r.Response = &RegistrationParametersInfo{MBIMExVersion: registrationParametersVersion(r.MBIMExVersion)}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsRegistrationParameters,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type RegistrationParametersSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Parameters    RegistrationParametersInfo
	Response      *RegistrationParametersInfo
}

func (r *RegistrationParametersSetRequest) Request() *Request {
	parameters := r.Parameters
	parameters.MBIMExVersion = r.MBIMExVersion
	data, err := parameters.MarshalBinary()
	r.Response = &RegistrationParametersInfo{MBIMExVersion: registrationParametersVersion(r.MBIMExVersion)}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsRegistrationParameters,
			CommandTypeSet,
			data,
			err,
		),
		Response: r.Response,
	}
}

type RegistrationParametersInfo struct {
	MBIMExVersion      uint16
	MICOMode           MICOMode
	DRXParameters      DRXParameters
	LADNIndication     LADNIndication
	DefaultPDUHint     DefaultPDUHint
	ReRegisterIfNeeded uint32
	TLVs               TLVs
}

func (i RegistrationParametersInfo) MarshalBinary() ([]byte, error) {
	version := registrationParametersVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return nil, errors.New("encoding MBIM registration parameters: CID requires MBIMEx 3.0")
	}
	if err := i.validateSetFields(); err != nil {
		return nil, fmt.Errorf("encoding MBIM registration parameters: %w", err)
	}
	if err := validateRegistrationParametersTLVs(i.TLVs, version, true); err != nil {
		return nil, fmt.Errorf("encoding MBIM registration parameters: %w", err)
	}
	tlvData, err := i.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding MBIM registration parameters TLVs: %w", err)
	}
	data := i.marshalFixedFields()
	return append(data, tlvData...), nil
}

func (i RegistrationParametersInfo) marshalBinaryUnchecked() []byte {
	data := i.marshalFixedFields()
	return append(data, marshalTLVsUnchecked(i.TLVs)...)
}

func (i RegistrationParametersInfo) marshalFixedFields() []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(i.MICOMode))
	data = binary.LittleEndian.AppendUint32(data, uint32(i.DRXParameters))
	data = binary.LittleEndian.AppendUint32(data, uint32(i.LADNIndication))
	data = binary.LittleEndian.AppendUint32(data, uint32(i.DefaultPDUHint))
	return binary.LittleEndian.AppendUint32(data, i.ReRegisterIfNeeded)
}

func (i *RegistrationParametersInfo) UnmarshalBinary(data []byte) error {
	version := registrationParametersVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return errors.New("parsing MBIM registration parameters: CID requires MBIMEx 3.0")
	}
	if len(data) < 20 {
		return errors.New("parsing MBIM registration parameters: payload is truncated")
	}
	result := RegistrationParametersInfo{
		MBIMExVersion:      version,
		MICOMode:           MICOMode(binary.LittleEndian.Uint32(data[0:4])),
		DRXParameters:      DRXParameters(binary.LittleEndian.Uint32(data[4:8])),
		LADNIndication:     LADNIndication(binary.LittleEndian.Uint32(data[8:12])),
		DefaultPDUHint:     DefaultPDUHint(binary.LittleEndian.Uint32(data[12:16])),
		ReRegisterIfNeeded: binary.LittleEndian.Uint32(data[16:20]),
	}
	if err := result.validateResponseFields(); err != nil {
		return fmt.Errorf("parsing MBIM registration parameters: %w", err)
	}
	if err := result.TLVs.UnmarshalBinary(data[20:]); err != nil {
		return fmt.Errorf("parsing MBIM registration parameters TLVs: %w", err)
	}
	if err := validateRegistrationParametersTLVs(result.TLVs, version, false); err != nil {
		return fmt.Errorf("parsing MBIM registration parameters: %w", err)
	}
	*i = result
	return nil
}

func (i RegistrationParametersInfo) validateResponseFields() error {
	if i.MICOMode > MICOModeUnsupported {
		return fmt.Errorf("MICO mode is %d, want disabled, enabled, or unsupported", i.MICOMode)
	}
	if i.DRXParameters > DRXCycle256 {
		return fmt.Errorf("DRX parameters are %d, want a value in 0..%d", i.DRXParameters, DRXCycle256)
	}
	if i.LADNIndication > LADNInfoRequested {
		return fmt.Errorf("LADN indication is %d, want not needed or requested", i.LADNIndication)
	}
	if i.DefaultPDUHint > DefaultPDUActivationLikely {
		return fmt.Errorf("default PDU hint is %d, want unlikely or likely", i.DefaultPDUHint)
	}
	if i.ReRegisterIfNeeded > 1 {
		return fmt.Errorf("re-register indicator is %d, want 0 or 1", i.ReRegisterIfNeeded)
	}
	return nil
}

func (i RegistrationParametersInfo) validateSetFields() error {
	if i.MICOMode != MICOModeDisabled && i.MICOMode != MICOModeDefault {
		return fmt.Errorf("MICO mode is %d, want disabled or default", i.MICOMode)
	}
	if i.DRXParameters != DRXNotSpecified {
		return fmt.Errorf("DRX parameters are %d, want not specified", i.DRXParameters)
	}
	if i.LADNIndication != LADNInfoNotNeeded {
		return fmt.Errorf("LADN indication is %d, want not needed", i.LADNIndication)
	}
	if i.DefaultPDUHint != DefaultPDUActivationUnlikely && i.DefaultPDUHint != DefaultPDUActivationLikely {
		return fmt.Errorf("default PDU hint is %d, want unlikely or likely", i.DefaultPDUHint)
	}
	if i.ReRegisterIfNeeded > 1 {
		return fmt.Errorf("re-register indicator is %d, want 0 or 1", i.ReRegisterIfNeeded)
	}
	return nil
}

func validateRegistrationParametersTLVs(tlvs TLVs, version uint16, setRequest bool) error {
	preconfiguredCount := 0
	osidCount := 0
	for index, tlv := range tlvs {
		switch tlv.Type {
		case TLVTypePreconfiguredDefaultConfiguredNSSAI:
			preconfiguredCount++
			if preconfiguredCount > 1 {
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
			if setRequest {
				if err := validateSetPreconfiguredDefaultNSSAI(values); err != nil {
					return err
				}
			}
		case TLVTypeOSID:
			if !setRequest {
				return errors.New("OSID TLV is only valid in set requests")
			}
			if version < mbimExVersion40 {
				return errors.New("OSID TLV requires MBIMEx 4.0")
			}
			osidCount++
			if osidCount > 1 {
				return errors.New("more than one OSID TLV")
			}
			var osid OSID
			if err := osid.UnmarshalTLV(tlv); err != nil {
				return fmt.Errorf("OSID TLV %d: %w", index, err)
			}
		}
	}
	return nil
}

func validateMBIMEx3PreconfiguredDefaultNSSAI(values PreconfiguredDefaultNSSAIList) error {
	for accessIndex, value := range values {
		for snssaiIndex, snssai := range value.PreferredNSSAI {
			if snssai.HasMappedSliceServiceType || snssai.HasMappedSliceDifferentiator {
				return fmt.Errorf("preconfigured default NSSAI access list %d S-NSSAI %d: mapped values require MBIMEx 4.0", accessIndex, snssaiIndex)
			}
		}
	}
	return nil
}

func validateSetPreconfiguredDefaultNSSAI(values PreconfiguredDefaultNSSAIList) error {
	if len(values) != 1 {
		return fmt.Errorf("set request preconfigured default NSSAI has %d access lists, want 1", len(values))
	}
	if len(values[0].PreferredNSSAI) != 1 {
		return fmt.Errorf("set request preconfigured default NSSAI has %d S-NSSAIs, want 1", len(values[0].PreferredNSSAI))
	}
	snssai := values[0].PreferredNSSAI[0]
	if snssai.SliceServiceType != 1 {
		return fmt.Errorf("set request preconfigured default NSSAI SST is %d, want 1", snssai.SliceServiceType)
	}
	if snssai.HasSliceDifferentiator || snssai.HasMappedSliceServiceType || snssai.HasMappedSliceDifferentiator {
		return errors.New("set request preconfigured default NSSAI must contain only the eMBB SST")
	}
	return nil
}

func registrationParametersVersion(version uint16) uint16 {
	if version == 0 {
		return mbimExVersion30
	}
	return version
}

func (c *Client) RegistrationParameters(ctx context.Context) (RegistrationParametersInfo, error) {
	if c.mbimExVersion < mbimExVersion30 {
		return RegistrationParametersInfo{}, errors.New("reading MBIM registration parameters: CID requires MBIMEx 3.0")
	}
	request := RegistrationParametersRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return RegistrationParametersInfo{}, fmt.Errorf("reading MBIM registration parameters: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetRegistrationParameters(ctx context.Context, parameters RegistrationParametersInfo) (RegistrationParametersInfo, error) {
	if c.mbimExVersion < mbimExVersion30 {
		return RegistrationParametersInfo{}, errors.New("setting MBIM registration parameters: CID requires MBIMEx 3.0")
	}
	parameters.MBIMExVersion = c.mbimExVersion
	if _, err := parameters.MarshalBinary(); err != nil {
		return RegistrationParametersInfo{}, err
	}
	request := RegistrationParametersSetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Parameters:    parameters,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return RegistrationParametersInfo{}, fmt.Errorf("setting MBIM registration parameters: %w", err)
	}
	return *request.Response, nil
}
