package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type PacketServiceAction uint32

const (
	PacketServiceActionAttach PacketServiceAction = iota
	PacketServiceActionDetach
)

type PacketServiceState uint32

const (
	PacketServiceStateUnknown PacketServiceState = iota
	PacketServiceStateAttaching
	PacketServiceStateAttached
	PacketServiceStateDetaching
	PacketServiceStateDetached
)

type PacketServiceInfo struct {
	MBIMExVersion             uint16
	NwError                   uint32
	PacketServiceState        PacketServiceState
	HighestAvailableDataClass uint32
	// CurrentDataClass is the MBIMEx name for HighestAvailableDataClass.
	// Both fields are populated from the same wire value.
	CurrentDataClass       DataClass
	UplinkSpeed            uint64
	DownlinkSpeed          uint64
	FrequencyRange         FrequencyRange
	CurrentDataSubclass    DataSubclass
	TrackingAreaIdentity   TrackingAreaIdentity
	ThreeGPPReleaseVersion ThreeGPPReleaseVersion
	Has3GPPReleaseVersion  bool
	TLVs                   TLVs
	expectedState          PacketServiceState
}

type PacketServiceRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *PacketServiceInfo
}

func (r *PacketServiceRequest) Request() *Request {
	r.Response = &PacketServiceInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDPacketService,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type PacketServiceSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Action        PacketServiceAction
	Response      *PacketServiceInfo
}

func (r *PacketServiceSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Action))

	expectedState := PacketServiceStateUnknown
	switch r.Action {
	case PacketServiceActionAttach:
		expectedState = PacketServiceStateAttached
	case PacketServiceActionDetach:
		expectedState = PacketServiceStateDetached
	}
	r.Response = &PacketServiceInfo{
		MBIMExVersion: r.MBIMExVersion,
		expectedState: expectedState,
	}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDPacketService,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

func (r *PacketServiceInfo) UnmarshalBinary(data []byte) error {
	version := r.MBIMExVersion
	expectedState := r.expectedState
	minimumLength := 28
	if version >= mbimExVersion30 {
		minimumLength = 44
	} else if version >= mbimExVersion20 {
		minimumLength = 32
	}
	if len(data) < minimumLength {
		return errors.New("parsing MBIM packet service: payload is truncated")
	}
	if version < mbimExVersion40 && len(data) != minimumLength {
		return errors.New("parsing MBIM packet service: payload has trailing data")
	}
	state := PacketServiceState(binary.LittleEndian.Uint32(data[4:8]))
	if state > PacketServiceStateDetached {
		return fmt.Errorf("parsing MBIM packet service: state %d is outside 0..%d", state, PacketServiceStateDetached)
	}
	if expectedState != PacketServiceStateUnknown && state != expectedState {
		return fmt.Errorf("parsing MBIM packet service: set response state is %d, want %d", state, expectedState)
	}
	dataClass := DataClass(binary.LittleEndian.Uint32(data[8:12]))
	if !validDataClass(version, dataClass) {
		return fmt.Errorf("parsing MBIM packet service: data class %#x contains bits reserved in MBIMEx %#x", dataClass, version)
	}
	if state != PacketServiceStateAttached && dataClass != DataClassNone {
		return fmt.Errorf("parsing MBIM packet service: data class is %#x while state is %d", dataClass, state)
	}
	result := PacketServiceInfo{
		MBIMExVersion:             version,
		NwError:                   binary.LittleEndian.Uint32(data[:4]),
		PacketServiceState:        state,
		HighestAvailableDataClass: uint32(dataClass),
		CurrentDataClass:          dataClass,
		UplinkSpeed:               binary.LittleEndian.Uint64(data[12:20]),
		DownlinkSpeed:             binary.LittleEndian.Uint64(data[20:28]),
	}
	if version >= mbimExVersion20 {
		frequencyRange := FrequencyRange(binary.LittleEndian.Uint32(data[28:32]))
		if frequencyRange&^(FrequencyRange1|FrequencyRange2) != 0 {
			return fmt.Errorf("parsing MBIM packet service: frequency range %#x contains reserved bits", frequencyRange)
		}
		if frequencyRange != FrequencyRangeUnknown && !dataClassHas5G(version, dataClass) {
			return fmt.Errorf("parsing MBIM packet service: frequency range is %#x without a 5G data class", frequencyRange)
		}
		result.FrequencyRange = frequencyRange
	}
	if version >= mbimExVersion30 {
		dataSubclass := DataSubclass(binary.LittleEndian.Uint32(data[32:36]))
		if !validDataSubclass(dataSubclass) {
			return fmt.Errorf("parsing MBIM packet service: data subclass %#x contains reserved bits", dataSubclass)
		}
		if dataSubclass != DataSubclassNone && !dataClassHas5G(version, dataClass) {
			return fmt.Errorf("parsing MBIM packet service: data subclass is %#x without a 5G data class", dataSubclass)
		}
		trackingAreaIdentity := TrackingAreaIdentity{
			PLMN: PLMN{
				MCC: binary.LittleEndian.Uint16(data[36:38]),
				MNC: binary.LittleEndian.Uint16(data[38:40]),
			},
			TAC: binary.LittleEndian.Uint32(data[40:44]),
		}
		if dataSubclassUses5GCore(dataSubclass) && trackingAreaIdentity.PLMN.MCC != 0 {
			if err := trackingAreaIdentity.PLMN.validate(); err != nil {
				return fmt.Errorf("parsing MBIM packet service tracking area identity: %w", err)
			}
			if err := validateTAC(trackingAreaIdentity.TAC); err != nil {
				return fmt.Errorf("parsing MBIM packet service tracking area identity: %w", err)
			}
		}
		result.CurrentDataSubclass = dataSubclass
		result.TrackingAreaIdentity = trackingAreaIdentity
	}
	if version >= mbimExVersion40 && len(data) > minimumLength {
		var tlvs TLVs
		if err := tlvs.UnmarshalBinary(data[minimumLength:]); err != nil {
			return fmt.Errorf("parsing MBIM packet service TLVs: %w", err)
		}
		result.TLVs = tlvs
		releaseCount := 0
		for _, tlv := range tlvs {
			if tlv.Type != TLVType3GPPReleaseVersion {
				continue
			}
			if state != PacketServiceStateAttached || !dataClassHas5G(version, dataClass) {
				return errors.New("parsing MBIM packet service: 3GPP release TLV is present without attached 5G service")
			}
			releaseCount++
			if releaseCount > 1 {
				return errors.New("parsing MBIM packet service: more than one 3GPP release TLV")
			}
			var release ThreeGPPReleaseVersion
			if err := release.UnmarshalTLV(tlv); err != nil {
				return fmt.Errorf("parsing MBIM packet service 3GPP release: %w", err)
			}
			result.ThreeGPPReleaseVersion = release
			result.Has3GPPReleaseVersion = true
		}
	}
	*r = result
	return nil
}

func (c *Client) PacketService(ctx context.Context) (PacketServiceInfo, error) {
	request := PacketServiceRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("reading MBIM packet service: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetPacketService(ctx context.Context, action PacketServiceAction) (PacketServiceInfo, error) {
	if action > PacketServiceActionDetach {
		return PacketServiceInfo{}, fmt.Errorf("setting MBIM packet service: action %d is outside 0..%d", action, PacketServiceActionDetach)
	}
	request := PacketServiceSetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Action:        action,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return *request.Response, fmt.Errorf("setting MBIM packet service: %w", err)
	}
	return *request.Response, nil
}
