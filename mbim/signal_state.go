package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type SignalStateInfo struct {
	MBIMExVersion          uint16
	RSSI                   uint32
	ErrorRate              uint32
	SignalStrengthInterval uint32
	RSSIThreshold          uint32
	ErrorRateThreshold     uint32
	RsrpSnr                []RsrpSnrInfo
}

type RsrpSnrInfo struct {
	RSRP          uint32
	SNR           uint32
	RSRPThreshold uint32
	SNRThreshold  uint32
	SystemType    DataClass
}

type SignalStateSet struct {
	SignalStrengthInterval uint32
	RSSIThreshold          uint32
	ErrorRateThreshold     uint32
}

type SignalStateRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Response      *SignalStateInfo
}

func (r *SignalStateRequest) Request() *Request {
	r.Response = &SignalStateInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDSignalState, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type SignalStateSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	State         SignalStateSet
	Response      *SignalStateInfo
}

func (r *SignalStateSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, r.State.SignalStrengthInterval)
	data = binary.LittleEndian.AppendUint32(data, r.State.RSSIThreshold)
	data = binary.LittleEndian.AppendUint32(data, r.State.ErrorRateThreshold)
	r.Response = &SignalStateInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDSignalState, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (r *SignalStateInfo) UnmarshalBinary(data []byte) error {
	version := r.MBIMExVersion
	if version == 0 && len(data) > 20 {
		// Preserve decoding for callers that constructed SignalStateInfo before
		// it carried the negotiated MBIMEx version.
		version = mbimExVersion20
	}
	fixedLength := 20
	if version >= mbimExVersion20 {
		fixedLength = 28
	}
	if len(data) < fixedLength {
		return errors.New("parsing MBIM signal state: payload is truncated")
	}
	if version < mbimExVersion20 && len(data) != fixedLength {
		return errors.New("parsing MBIM signal state: payload has trailing data")
	}
	rssi := binary.LittleEndian.Uint32(data[0:4])
	if !validRSSI(rssi) {
		return fmt.Errorf("parsing MBIM signal state: RSSI %d is outside 0..31 and is not the unknown value 99", rssi)
	}
	errorRate := binary.LittleEndian.Uint32(data[4:8])
	if !validErrorRate(errorRate) {
		return fmt.Errorf("parsing MBIM signal state: error rate %d is outside 0..7 and is not the unknown value 99", errorRate)
	}
	*r = SignalStateInfo{
		MBIMExVersion:          version,
		RSSI:                   rssi,
		ErrorRate:              errorRate,
		SignalStrengthInterval: binary.LittleEndian.Uint32(data[8:12]),
		RSSIThreshold:          binary.LittleEndian.Uint32(data[12:16]),
		ErrorRateThreshold:     binary.LittleEndian.Uint32(data[16:20]),
	}
	if version < mbimExVersion20 {
		return nil
	}
	ref, err := readOffsetSizeRef(data, 20)
	if err != nil {
		return fmt.Errorf("parsing MBIM signal state RSRP/SNR: %w", err)
	}
	if err := validateDataBufferRefs(data, 28, []valueRef{ref}); err != nil {
		return fmt.Errorf("parsing MBIM signal state RSRP/SNR: %w", err)
	}
	if ref.size == 0 {
		return nil
	}
	if ref.size < 4 {
		return errors.New("parsing MBIM signal state RSRP/SNR: payload is truncated")
	}
	rsrpSNR := data[ref.offset : ref.offset+ref.size]
	count := binary.LittleEndian.Uint32(rsrpSNR[0:4])
	wantLength := uint64(4) + uint64(count)*20
	if uint64(len(rsrpSNR)) != wantLength {
		return fmt.Errorf("parsing MBIM signal state RSRP/SNR: payload length %d, want %d for %d entries", len(rsrpSNR), wantLength, count)
	}
	if count != 0 && r.RSSI != 99 {
		return fmt.Errorf("parsing MBIM signal state RSRP/SNR: RSSI is %d, want 99 when RSRP/SNR is reported", r.RSSI)
	}
	r.RsrpSnr = make([]RsrpSnrInfo, count)
	for i := range count {
		offset := 4 + i*20
		info := RsrpSnrInfo{
			RSRP:          binary.LittleEndian.Uint32(rsrpSNR[offset : offset+4]),
			SNR:           binary.LittleEndian.Uint32(rsrpSNR[offset+4 : offset+8]),
			RSRPThreshold: binary.LittleEndian.Uint32(rsrpSNR[offset+8 : offset+12]),
			SNRThreshold:  binary.LittleEndian.Uint32(rsrpSNR[offset+12 : offset+16]),
			SystemType:    DataClass(binary.LittleEndian.Uint32(rsrpSNR[offset+16 : offset+20])),
		}
		if info.RSRP > 127 {
			return fmt.Errorf("parsing MBIM signal state RSRP/SNR entry %d: RSRP %d exceeds 127", i, info.RSRP)
		}
		if info.SNR > 128 {
			return fmt.Errorf("parsing MBIM signal state RSRP/SNR entry %d: SNR %d exceeds 128", i, info.SNR)
		}
		if !validSignalSystemType(version, info.SystemType) {
			return fmt.Errorf("parsing MBIM signal state RSRP/SNR entry %d: system type %#x is not LTE or a negotiated 5G data class", i, info.SystemType)
		}
		r.RsrpSnr[i] = info
	}
	return nil
}

func validSignalSystemType(version uint16, systemType DataClass) bool {
	if systemType == DataClassLTE {
		return true
	}
	if version >= mbimExVersion30 {
		return systemType == DataClass5G
	}
	return systemType == DataClass5GNSA || systemType == DataClass5GSA
}

func (c *Client) SignalState(ctx context.Context) (SignalStateInfo, error) {
	request := SignalStateRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SignalStateInfo{}, fmt.Errorf("reading MBIM signal state: %w", err)
	}
	response := *request.Response
	response.RsrpSnr = slices.Clone(response.RsrpSnr)
	return response, nil
}

func (c *Client) SetSignalState(ctx context.Context, state SignalStateSet) (SignalStateInfo, error) {
	request := SignalStateSetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		State:         state,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SignalStateInfo{}, fmt.Errorf("setting MBIM signal state: %w", err)
	}
	response := *request.Response
	response.RsrpSnr = slices.Clone(response.RsrpSnr)
	return response, nil
}
