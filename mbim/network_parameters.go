package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type MICOMode uint32

const (
	MICOModeDisabled    MICOMode = 0
	MICOModeEnabled     MICOMode = 1
	MICOModeUnsupported MICOMode = 2
	MICOModeDefault     MICOMode = 3
)

type DRXParameters uint32

const (
	DRXNotSpecified DRXParameters = 0
	DRXNotSupported DRXParameters = 1
	DRXCycle32      DRXParameters = 2
	DRXCycle64      DRXParameters = 3
	DRXCycle128     DRXParameters = 4
	DRXCycle256     DRXParameters = 5
)

type LADNIndication uint32

const (
	LADNInfoNotNeeded LADNIndication = 0
	LADNInfoRequested LADNIndication = 1
)

type DefaultPDUHint uint32

const (
	DefaultPDUActivationUnlikely DefaultPDUHint = 0
	DefaultPDUActivationLikely   DefaultPDUHint = 1
)

type MICOIndication uint32

const (
	MICOIndicationRegistrationAreaNotAllocated MICOIndication = 0
	MICOIndicationRegistrationAreaAllocated    MICOIndication = 1
	MICOIndicationNotAvailable                 MICOIndication = 0xFFFFFFFF
)

type NetworkParametersRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Query         NetworkParametersQuery
	Response      *NetworkParametersInfo
}

func (r *NetworkParametersRequest) Request() *Request {
	version := networkParametersVersion(r.MBIMExVersion)
	data, err := r.Query.marshalBinary(version)
	r.Response = &NetworkParametersInfo{MBIMExVersion: version}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: commandWithError(
			ServiceMsBasicConnectExtensions,
			CIDMsNetworkParameters,
			CommandTypeQuery,
			data,
			err,
		),
		Response: r.Response,
	}
}

type NetworkParametersQuery struct {
	ConfigurationsNeeded bool
	UEPoliciesNeeded     bool
	TLVs                 TLVs
}

func (q NetworkParametersQuery) marshalBinary(version uint16) ([]byte, error) {
	version = networkParametersVersion(version)
	if version < mbimExVersion30 {
		return nil, errors.New("encoding MBIM network parameters query: CID requires MBIMEx 3.0")
	}
	if version >= mbimExVersion40 && q.UEPoliciesNeeded {
		return nil, errors.New("encoding MBIM network parameters query: UE policies use MBIM_CID_MS_UE_POLICY in MBIMEx 4.0")
	}
	tlvData, err := q.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding MBIM network parameters query TLVs: %w", err)
	}
	if version >= mbimExVersion40 {
		return tlvData, nil
	}

	data := make([]byte, 4)
	if q.ConfigurationsNeeded {
		binary.LittleEndian.PutUint16(data[0:2], 1)
	}
	if q.UEPoliciesNeeded {
		binary.LittleEndian.PutUint16(data[2:4], 1)
	}
	return append(data, tlvData...), nil
}

type NetworkParametersInfo struct {
	MBIMExVersion          uint16
	MICOIndication         MICOIndication
	DRXParameters          DRXParameters
	TLVs                   TLVs
	AllowedNSSAI           NSSAIList
	ConfiguredNSSAI        NSSAIList
	DefaultConfiguredNSSAI NSSAIList
	RejectedNSSAI          RejectedNSSAIList
	LADNs                  LADNList
	TAILists               TAILists
}

func (i NetworkParametersInfo) MarshalBinary() ([]byte, error) {
	version := networkParametersVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return nil, errors.New("encoding MBIM network parameters: CID requires MBIMEx 3.0")
	}
	if err := validateNetworkParametersFields(i.MICOIndication, i.DRXParameters); err != nil {
		return nil, fmt.Errorf("encoding MBIM network parameters: %w", err)
	}
	if err := i.unmarshalTLVs(i.TLVs, version); err != nil {
		return nil, fmt.Errorf("encoding MBIM network parameters: %w", err)
	}
	tlvData, err := i.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding MBIM network parameters TLVs: %w", err)
	}
	data := binary.LittleEndian.AppendUint32(nil, uint32(i.MICOIndication))
	data = binary.LittleEndian.AppendUint32(data, uint32(i.DRXParameters))
	return append(data, tlvData...), nil
}

func (i *NetworkParametersInfo) UnmarshalBinary(data []byte) error {
	version := networkParametersVersion(i.MBIMExVersion)
	if version < mbimExVersion30 {
		return errors.New("parsing MBIM network parameters: CID requires MBIMEx 3.0")
	}
	if len(data) < 8 {
		return errors.New("parsing MBIM network parameters: payload is truncated")
	}
	mico := MICOIndication(binary.LittleEndian.Uint32(data[0:4]))
	drx := DRXParameters(binary.LittleEndian.Uint32(data[4:8]))
	if err := validateNetworkParametersFields(mico, drx); err != nil {
		return fmt.Errorf("parsing MBIM network parameters: %w", err)
	}
	var tlvs TLVs
	if err := tlvs.UnmarshalBinary(data[8:]); err != nil {
		return fmt.Errorf("parsing MBIM network parameters TLVs: %w", err)
	}
	info := NetworkParametersInfo{
		MBIMExVersion:  version,
		MICOIndication: mico,
		DRXParameters:  drx,
		TLVs:           tlvs,
	}
	if err := info.unmarshalTLVs(tlvs, version); err != nil {
		return fmt.Errorf("parsing MBIM network parameters: %w", err)
	}
	*i = info
	return nil
}

func (i *NetworkParametersInfo) unmarshalTLVs(tlvs TLVs, version uint16) error {
	var values NetworkParametersInfo
	uePoliciesSeen := false
	for index, tlv := range tlvs {
		var err error
		switch tlv.Type {
		case TLVTypeUEPolicies:
			if version >= mbimExVersion40 {
				return errors.New("UE policies TLV uses MBIM_CID_MS_UE_POLICY in MBIMEx 4.0")
			}
			if uePoliciesSeen {
				return errors.New("more than one UE policies TLV")
			}
			uePoliciesSeen = true
		case TLVTypeAllowedNSSAI:
			if values.AllowedNSSAI != nil {
				return errors.New("more than one allowed NSSAI TLV")
			}
			err = values.AllowedNSSAI.UnmarshalTLV(tlv)
		case TLVTypeConfiguredNSSAI:
			if values.ConfiguredNSSAI != nil {
				return errors.New("more than one configured NSSAI TLV")
			}
			err = values.ConfiguredNSSAI.UnmarshalTLV(tlv)
		case TLVTypeDefaultConfiguredNSSAI:
			if values.DefaultConfiguredNSSAI != nil {
				return errors.New("more than one default configured NSSAI TLV")
			}
			err = values.DefaultConfiguredNSSAI.UnmarshalTLV(tlv)
		case TLVTypeRejectedNSSAI:
			if values.RejectedNSSAI != nil {
				return errors.New("more than one rejected NSSAI TLV")
			}
			err = values.RejectedNSSAI.UnmarshalTLV(tlv)
		case TLVTypeLADN:
			if values.LADNs != nil {
				return errors.New("more than one LADN TLV")
			}
			err = values.LADNs.UnmarshalTLV(tlv, version)
		case TLVTypeTAI:
			if values.TAILists != nil {
				return errors.New("more than one TAI TLV")
			}
			err = values.TAILists.UnmarshalTLV(tlv)
		}
		if err != nil {
			return fmt.Errorf("TLV %d: %w", index, err)
		}
	}
	i.AllowedNSSAI = values.AllowedNSSAI
	i.ConfiguredNSSAI = values.ConfiguredNSSAI
	i.DefaultConfiguredNSSAI = values.DefaultConfiguredNSSAI
	i.RejectedNSSAI = values.RejectedNSSAI
	i.LADNs = values.LADNs
	i.TAILists = values.TAILists
	return nil
}

func validateNetworkParametersFields(mico MICOIndication, drx DRXParameters) error {
	if mico != MICOIndicationRegistrationAreaNotAllocated &&
		mico != MICOIndicationRegistrationAreaAllocated &&
		mico != MICOIndicationNotAvailable {
		return fmt.Errorf("MICO indication is %d, want 0, 1, or %#x", mico, uint32(MICOIndicationNotAvailable))
	}
	if drx > DRXCycle256 {
		return fmt.Errorf("DRX parameters are %d, want a value from 0 through %d", drx, DRXCycle256)
	}
	return nil
}

func networkParametersVersion(version uint16) uint16 {
	if version == 0 {
		return mbimExVersion30
	}
	return version
}

func (c *Client) NetworkParameters(ctx context.Context, query NetworkParametersQuery) (NetworkParametersInfo, error) {
	if c.mbimExVersion < mbimExVersion30 {
		return NetworkParametersInfo{}, errors.New("reading MBIM network parameters: CID requires MBIMEx 3.0")
	}
	if _, err := query.marshalBinary(c.mbimExVersion); err != nil {
		return NetworkParametersInfo{}, err
	}
	request := NetworkParametersRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Query:         query,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return NetworkParametersInfo{}, fmt.Errorf("reading MBIM network parameters: %w", err)
	}
	return *request.Response, nil
}
