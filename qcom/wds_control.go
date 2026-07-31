package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDSChannelRateUnit identifies the unit used by extended channel rates.
type WDSChannelRateUnit uint32

const (
	WDSChannelRateBitsPerSecond WDSChannelRateUnit = iota
	WDSChannelRateKilobitsPerSecond
	WDSChannelRateMegabitsPerSecond
	WDSChannelRateGigabitsPerSecond
)

// WDSChannelRates contains instantaneous and maximum link rates.
type WDSChannelRates struct {
	Unit      WDSChannelRateUnit
	CurrentTx uint64
	CurrentRx uint64
	MaximumTx uint64
	MaximumRx uint64
	Extended  bool
}

// WDSStatisticsMask selects counters returned by Get Packet Statistics.
type WDSStatisticsMask uint32

const (
	WDSStatisticsTxPackets   WDSStatisticsMask = 1 << 0
	WDSStatisticsRxPackets   WDSStatisticsMask = 1 << 1
	WDSStatisticsTxErrors    WDSStatisticsMask = 1 << 2
	WDSStatisticsRxErrors    WDSStatisticsMask = 1 << 3
	WDSStatisticsTxOverflows WDSStatisticsMask = 1 << 4
	WDSStatisticsRxOverflows WDSStatisticsMask = 1 << 5
	WDSStatisticsTxBytes     WDSStatisticsMask = 1 << 6
	WDSStatisticsRxBytes     WDSStatisticsMask = 1 << 7
	WDSStatisticsTxDropped   WDSStatisticsMask = 1 << 8
	WDSStatisticsRxDropped   WDSStatisticsMask = 1 << 9

	WDSStatisticsAll = WDSStatisticsTxPackets |
		WDSStatisticsRxPackets |
		WDSStatisticsTxErrors |
		WDSStatisticsRxErrors |
		WDSStatisticsTxOverflows |
		WDSStatisticsRxOverflows |
		WDSStatisticsTxBytes |
		WDSStatisticsRxBytes |
		WDSStatisticsTxDropped |
		WDSStatisticsRxDropped
)

// WDSResetRequest encodes QMI WDS Reset.
type WDSResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r WDSResetRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSReset)
}

// WDSPacketStatistics contains counters accumulated by a WDS control point.
type WDSPacketStatistics struct {
	TxPackets            uint32
	TxPacketsKnown       bool
	RxPackets            uint32
	RxPacketsKnown       bool
	TxErrors             uint32
	TxErrorsKnown        bool
	RxErrors             uint32
	RxErrorsKnown        bool
	TxOverflows          uint32
	TxOverflowsKnown     bool
	RxOverflows          uint32
	RxOverflowsKnown     bool
	TxBytes              uint64
	TxBytesKnown         bool
	RxBytes              uint64
	RxBytesKnown         bool
	LastCallTxBytes      uint64
	LastCallTxBytesKnown bool
	LastCallRxBytes      uint64
	LastCallRxBytesKnown bool
	TxDropped            uint32
	TxDroppedKnown       bool
	RxDropped            uint32
	RxDroppedKnown       bool
}

// WDSGoDormantConfig controls an optional delayed dormancy request.
type WDSGoDormantConfig struct {
	Delay    *time.Duration
	SendSCRI *bool
}

// WDSDormancyStatus is the current traffic-channel state.
type WDSDormancyStatus uint8

const (
	WDSDormancyDormant WDSDormancyStatus = 0x01
	WDSDormancyActive  WDSDormancyStatus = 0x02
)

// WDSDataBearerTechnology is the compact legacy bearer enumeration.
type WDSDataBearerTechnology int8

const (
	WDSDataBearer1X                  WDSDataBearerTechnology = 0x01
	WDSDataBearer1XEVDO              WDSDataBearerTechnology = 0x02
	WDSDataBearerGSM                 WDSDataBearerTechnology = 0x03
	WDSDataBearerUMTS                WDSDataBearerTechnology = 0x04
	WDSDataBearerEVDORevA            WDSDataBearerTechnology = 0x05
	WDSDataBearerEDGE                WDSDataBearerTechnology = 0x06
	WDSDataBearerHSDPAWCDMA          WDSDataBearerTechnology = 0x07
	WDSDataBearerWCDMAHSUPA          WDSDataBearerTechnology = 0x08
	WDSDataBearerHSDPAHSUPA          WDSDataBearerTechnology = 0x09
	WDSDataBearerLTE                 WDSDataBearerTechnology = 0x0A
	WDSDataBearerEHRPD               WDSDataBearerTechnology = 0x0B
	WDSDataBearerHSDPAPlusWCDMA      WDSDataBearerTechnology = 0x0C
	WDSDataBearerHSDPAPlusHSUPA      WDSDataBearerTechnology = 0x0D
	WDSDataBearerDCHSDPAPlusWCDMA    WDSDataBearerTechnology = 0x0E
	WDSDataBearerDCHSDPAPlusHSUPA    WDSDataBearerTechnology = 0x0F
	WDSDataBearerHSDPAPlus64QAM      WDSDataBearerTechnology = 0x10
	WDSDataBearerHSDPAPlus64QAMHSUPA WDSDataBearerTechnology = 0x11
	WDSDataBearerTDSCDMA             WDSDataBearerTechnology = 0x12
	WDSDataBearerTDSCDMAHSDPA        WDSDataBearerTechnology = 0x13
	WDSDataBearerTDSCDMAHSUPA        WDSDataBearerTechnology = 0x14
	WDSDataBearerIWLANS2B            WDSDataBearerTechnology = 0x15
	WDSDataBearerUnknown             WDSDataBearerTechnology = -1
)

// WDSDataBearerTechnologyInfo contains the current and optional last bearer.
type WDSDataBearerTechnologyInfo struct {
	Current   WDSDataBearerTechnology
	Last      WDSDataBearerTechnology
	LastKnown bool
}

// WDSCurrentBearerNetwork identifies the network namespace of legacy masks.
type WDSCurrentBearerNetwork uint8

const (
	WDSCurrentBearerNetworkUnknown WDSCurrentBearerNetwork = iota
	WDSCurrentBearerNetwork3GPP2
	WDSCurrentBearerNetwork3GPP
)

// WDSCurrentBearerTechnology contains the legacy network, RAT, and SO masks.
type WDSCurrentBearerTechnology struct {
	Network           WDSCurrentBearerNetwork
	RATMask           uint32
	ServiceOptionMask uint32
}

// WDSCurrentBearerTechnologyInfo contains current and optional last masks.
type WDSCurrentBearerTechnologyInfo struct {
	Current   WDSCurrentBearerTechnology
	Last      WDSCurrentBearerTechnology
	LastKnown bool
}

// WDSBearerNetwork identifies the network family in extended bearer data.
type WDSBearerNetwork uint32

const (
	WDSBearerNetwork3GPP WDSBearerNetwork = iota
	WDSBearerNetwork3GPP2
)

// WDSBearerRAT identifies the RAT in extended bearer data.
type WDSBearerRAT uint32

const (
	WDSBearerRATNull      WDSBearerRAT = 0x00
	WDSBearerRATWCDMA     WDSBearerRAT = 0x01
	WDSBearerRATGERAN     WDSBearerRAT = 0x02
	WDSBearerRATLTE       WDSBearerRAT = 0x03
	WDSBearerRATTDSCDMA   WDSBearerRAT = 0x04
	WDSBearerRAT3GPPWLAN  WDSBearerRAT = 0x05
	WDSBearerRAT5G        WDSBearerRAT = 0x06
	WDSBearerRAT1X        WDSBearerRAT = 0x65
	WDSBearerRATHRPD      WDSBearerRAT = 0x66
	WDSBearerRATEHRPD     WDSBearerRAT = 0x67
	WDSBearerRAT3GPP2WLAN WDSBearerRAT = 0x68
)

// WDSBearerServiceOptionMask describes active bearer capabilities.
type WDSBearerServiceOptionMask uint64

const (
	WDSBearerServiceOptionWCDMA         WDSBearerServiceOptionMask = 1 << 0
	WDSBearerServiceOptionHSDPA         WDSBearerServiceOptionMask = 1 << 1
	WDSBearerServiceOptionHSUPA         WDSBearerServiceOptionMask = 1 << 2
	WDSBearerServiceOptionHSDPAPlus     WDSBearerServiceOptionMask = 1 << 3
	WDSBearerServiceOptionGPRS          WDSBearerServiceOptionMask = 1 << 7
	WDSBearerServiceOptionEDGE          WDSBearerServiceOptionMask = 1 << 8
	WDSBearerServiceOptionGSM           WDSBearerServiceOptionMask = 1 << 9
	WDSBearerServiceOptionS2B           WDSBearerServiceOptionMask = 1 << 10
	WDSBearerServiceOptionLTEFDD        WDSBearerServiceOptionMask = 1 << 12
	WDSBearerServiceOptionLTETDD        WDSBearerServiceOptionMask = 1 << 13
	WDSBearerServiceOptionTDSCDMA       WDSBearerServiceOptionMask = 1 << 14
	WDSBearerServiceOptionLTECADownlink WDSBearerServiceOptionMask = 1 << 16
	WDSBearerServiceOptionLTECAUplink   WDSBearerServiceOptionMask = 1 << 17
	WDSBearerServiceOption5GTDD         WDSBearerServiceOptionMask = 1 << 40
	WDSBearerServiceOption5GSub6        WDSBearerServiceOptionMask = 1 << 41
	WDSBearerServiceOption5GMMWave      WDSBearerServiceOptionMask = 1 << 42
	WDSBearerServiceOption5GNSA         WDSBearerServiceOptionMask = 1 << 43
	WDSBearerServiceOption5GSA          WDSBearerServiceOptionMask = 1 << 44
)

// WDSBearerTechnology contains 5G-capable extended bearer information.
type WDSBearerTechnology struct {
	Network        WDSBearerNetwork
	RAT            WDSBearerRAT
	ServiceOptions WDSBearerServiceOptionMask
}

// WDSBearerTechnologyInfo contains current and optional last extended bearer.
type WDSBearerTechnologyInfo struct {
	Current      WDSBearerTechnology
	CurrentKnown bool
	Last         WDSBearerTechnology
	LastKnown    bool
}

// WDSSubscription identifies the subscription bound to a WDS control point.
type WDSSubscription uint32

const (
	WDSSubscriptionDefault   WDSSubscription = 0x0000
	WDSSubscriptionPrimary   WDSSubscription = 0x0001
	WDSSubscriptionSecondary WDSSubscription = 0x0002
	WDSSubscriptionTertiary  WDSSubscription = 0x0003
	WDSSubscriptionDontCare  WDSSubscription = 0x00FF
)

// WDSGetCurrentChannelRateRequest encodes Get Current Channel Rate.
type WDSGetCurrentChannelRateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetCurrentChannelRateRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetCurrentChannelRate)
}

// WDSGetCurrentChannelRateResponse is the parsed channel-rate response.
type WDSGetCurrentChannelRateResponse struct {
	Rates WDSChannelRates
}

// UnmarshalTLVs parses legacy rates and prefers extended rates when present.
func (r *WDSGetCurrentChannelRateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetCurrentChannelRateResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS channel rates: rates TLV missing")
	}
	if len(value) != 16 {
		return fmt.Errorf("parsing QMI WDS channel rates: rates TLV length %d, want 16", len(value))
	}
	r.Rates = WDSChannelRates{
		Unit:      WDSChannelRateBitsPerSecond,
		CurrentTx: uint64(binary.LittleEndian.Uint32(value[:4])),
		CurrentRx: uint64(binary.LittleEndian.Uint32(value[4:8])),
		MaximumTx: uint64(binary.LittleEndian.Uint32(value[8:12])),
		MaximumRx: uint64(binary.LittleEndian.Uint32(value[12:16])),
	}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 36 {
			return fmt.Errorf("parsing QMI WDS channel rates: extended rates TLV length %d, want 36", len(value))
		}
		r.Rates = WDSChannelRates{
			Unit:      WDSChannelRateUnit(binary.LittleEndian.Uint32(value[:4])),
			CurrentTx: binary.LittleEndian.Uint64(value[4:12]),
			CurrentRx: binary.LittleEndian.Uint64(value[12:20]),
			MaximumTx: binary.LittleEndian.Uint64(value[20:28]),
			MaximumRx: binary.LittleEndian.Uint64(value[28:36]),
			Extended:  true,
		}
	}
	return nil
}

// WDSGetPacketStatisticsRequest encodes Get Packet Statistics.
type WDSGetPacketStatisticsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mask          WDSStatisticsMask
}

// Request converts the selected counters into a QMI request. A zero mask asks
// for every standard counter.
func (r WDSGetPacketStatisticsRequest) Request() Request {
	mask := r.Mask
	if mask == 0 {
		mask = WDSStatisticsAll
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetPacketStatistics,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(mask))},
	}
}

// WDSGetPacketStatisticsResponse is the parsed packet-statistics response.
type WDSGetPacketStatisticsResponse struct {
	Statistics WDSPacketStatistics
}

// UnmarshalTLVs parses every optional statistics counter.
func (r *WDSGetPacketStatisticsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetPacketStatisticsResponse{}
	fields32 := []struct {
		kind  byte
		value *uint32
		known *bool
	}{
		{0x10, &r.Statistics.TxPackets, &r.Statistics.TxPacketsKnown},
		{0x11, &r.Statistics.RxPackets, &r.Statistics.RxPacketsKnown},
		{0x12, &r.Statistics.TxErrors, &r.Statistics.TxErrorsKnown},
		{0x13, &r.Statistics.RxErrors, &r.Statistics.RxErrorsKnown},
		{0x14, &r.Statistics.TxOverflows, &r.Statistics.TxOverflowsKnown},
		{0x15, &r.Statistics.RxOverflows, &r.Statistics.RxOverflowsKnown},
		{0x1D, &r.Statistics.TxDropped, &r.Statistics.TxDroppedKnown},
		{0x1E, &r.Statistics.RxDropped, &r.Statistics.RxDroppedKnown},
	}
	for _, field := range fields32 {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS packet statistics: TLV 0x%02X length %d, want 4", field.kind, len(value))
		}
		*field.value = binary.LittleEndian.Uint32(value[:4])
		*field.known = true
	}
	fields64 := []struct {
		kind  byte
		value *uint64
		known *bool
	}{
		{0x19, &r.Statistics.TxBytes, &r.Statistics.TxBytesKnown},
		{0x1A, &r.Statistics.RxBytes, &r.Statistics.RxBytesKnown},
		{0x1B, &r.Statistics.LastCallTxBytes, &r.Statistics.LastCallTxBytesKnown},
		{0x1C, &r.Statistics.LastCallRxBytes, &r.Statistics.LastCallRxBytesKnown},
	}
	for _, field := range fields64 {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI WDS packet statistics: TLV 0x%02X length %d, want 8", field.kind, len(value))
		}
		*field.value = binary.LittleEndian.Uint64(value[:8])
		*field.known = true
	}
	return nil
}

// WDSGoDormantRequest encodes Go Dormant.
type WDSGoDormantRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDSGoDormantConfig
}

// Request validates the optional delay and converts it into QMI TLVs.
func (r WDSGoDormantRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	if r.Config.Delay != nil {
		if *r.Config.Delay < 0 {
			return Request{}, errors.New("encoding QMI WDS go dormant: delay is negative")
		}
		milliseconds := r.Config.Delay.Milliseconds()
		if milliseconds > 1<<32-1 {
			return Request{}, errors.New("encoding QMI WDS go dormant: delay exceeds uint32 milliseconds")
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint32(milliseconds)))
	}
	if r.Config.SendSCRI != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*r.Config.SendSCRI)))
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGoDormant,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// WDSGoActiveRequest encodes Go Active.
type WDSGoActiveRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the command into a QMI request.
func (r WDSGoActiveRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGoActive)
}

// WDSGetDormancyStatusRequest encodes Get Dormancy Status.
type WDSGetDormancyStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetDormancyStatusRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetDormancyStatus)
}

// WDSGetDormancyStatusResponse is the parsed dormancy response.
type WDSGetDormancyStatusResponse struct {
	Status WDSDormancyStatus
}

// UnmarshalTLVs parses the mandatory dormancy state.
func (r *WDSGetDormancyStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetDormancyStatusResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS dormancy status: status TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS dormancy status: status TLV length %d, want 1", len(value))
	}
	r.Status = WDSDormancyStatus(value[0])
	return nil
}

// WDSGetDataBearerTechnologyRequest encodes the compact legacy bearer query.
type WDSGetDataBearerTechnologyRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetDataBearerTechnologyRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetDataBearerTechnology)
}

// WDSGetDataBearerTechnologyResponse is the parsed compact bearer response.
type WDSGetDataBearerTechnologyResponse struct {
	Technology WDSDataBearerTechnologyInfo
}

// UnmarshalTLVs parses current and optional last-call bearer values.
func (r *WDSGetDataBearerTechnologyResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetDataBearerTechnologyResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS data bearer technology: current bearer TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS data bearer technology: current bearer TLV length %d, want 1", len(value))
	}
	r.Technology.Current = WDSDataBearerTechnology(int8(value[0]))
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS data bearer technology: last bearer TLV length %d, want 1", len(value))
		}
		r.Technology.Last = WDSDataBearerTechnology(int8(value[0]))
		r.Technology.LastKnown = true
	}
	return nil
}

// WDSGetCurrentDataBearerTechnologyRequest encodes the legacy mask query.
type WDSGetCurrentDataBearerTechnologyRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetCurrentDataBearerTechnologyRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetCurrentDataBearerTechnology)
}

// WDSGetCurrentDataBearerTechnologyResponse is the parsed legacy mask response.
type WDSGetCurrentDataBearerTechnologyResponse struct {
	Technology WDSCurrentBearerTechnologyInfo
}

// UnmarshalTLVs parses current and optional last-call bearer masks.
func (r *WDSGetCurrentDataBearerTechnologyResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetCurrentDataBearerTechnologyResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS current bearer technology: current bearer TLV missing")
	}
	current, err := decodeWDSCurrentBearerTechnology(value)
	if err != nil {
		return err
	}
	r.Technology.Current = current
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		last, err := decodeWDSCurrentBearerTechnology(value)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS last bearer technology: %w", err)
		}
		r.Technology.Last = last
		r.Technology.LastKnown = true
	}
	return nil
}

// WDSGetDataBearerTechnologyExRequest encodes the 5G-capable bearer query.
type WDSGetDataBearerTechnologyExRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into an empty QMI WDS request.
func (r WDSGetDataBearerTechnologyExRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetDataBearerTechnologyEx)
}

// WDSGetDataBearerTechnologyExResponse is the parsed extended bearer
// response.
type WDSGetDataBearerTechnologyExResponse struct {
	Technology WDSBearerTechnologyInfo
}

// UnmarshalTLVs parses the optional current and last-call bearer aggregates.
func (r *WDSGetDataBearerTechnologyExResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetDataBearerTechnologyExResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		current, err := decodeWDSBearerTechnology(value)
		if err != nil {
			return err
		}
		r.Technology.Current = current
		r.Technology.CurrentKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		last, err := decodeWDSBearerTechnology(value)
		if err != nil {
			return fmt.Errorf("parsing QMI WDS last extended bearer technology: %w", err)
		}
		r.Technology.Last = last
		r.Technology.LastKnown = true
	}
	return nil
}

// WDSBindSubscriptionRequest encodes Bind Subscription.
type WDSBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  WDSSubscription
}

// Request validates the subscription and converts it into a QMI request.
func (r WDSBindSubscriptionRequest) Request() (Request, error) {
	if err := validateWDSSubscription(r.Subscription); err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSBindSubscription,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Subscription))},
	}, nil
}

// WDSGetBindSubscriptionRequest encodes Get Bind Subscription.
type WDSGetBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetBindSubscriptionRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetBindSubscription)
}

// WDSGetBindSubscriptionResponse is the parsed subscription response.
type WDSGetBindSubscriptionResponse struct {
	Subscription      WDSSubscription
	SubscriptionKnown bool
}

// UnmarshalTLVs parses the optional bound subscription.
func (r *WDSGetBindSubscriptionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetBindSubscriptionResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI WDS bound subscription: subscription TLV length %d, want 4", len(value))
	}
	r.Subscription = WDSSubscription(binary.LittleEndian.Uint32(value[:4]))
	r.SubscriptionKnown = true
	return nil
}

// WDSChannelRates reads rates associated with the Client's WDS control point.
func (c *Client) WDSChannelRates(ctx context.Context) (WDSChannelRates, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetCurrentChannelRate, nil)
	if err != nil {
		return WDSChannelRates{}, fmt.Errorf("reading QMI WDS channel rates: %w", err)
	}
	var parsed WDSGetCurrentChannelRateResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSChannelRates{}, err
	}
	return parsed.Rates, nil
}

// WDSReset resets state owned by this WDS client.
func (c *Client) WDSReset(ctx context.Context) error {
	if _, err := c.wdsControlRequest(ctx, MessageWDSReset, nil); err != nil {
		return fmt.Errorf("resetting QMI WDS control point: %w", err)
	}
	return nil
}

// ChannelRates reads rates associated with this packet-data session.
func (s *PDNSession) ChannelRates(ctx context.Context) (WDSChannelRates, error) {
	resp, err := s.wdsControlRequest(ctx, MessageWDSGetCurrentChannelRate, nil)
	if err != nil {
		return WDSChannelRates{}, fmt.Errorf("reading QMI WDS session channel rates: %w", err)
	}
	var parsed WDSGetCurrentChannelRateResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSChannelRates{}, err
	}
	return parsed.Rates, nil
}

// WDSPacketStatistics reads counters from the Client's WDS control point.
func (c *Client) WDSPacketStatistics(ctx context.Context, mask WDSStatisticsMask) (WDSPacketStatistics, error) {
	req := (WDSGetPacketStatisticsRequest{Mask: mask}).Request()
	resp, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs)
	if err != nil {
		return WDSPacketStatistics{}, fmt.Errorf("reading QMI WDS packet statistics: %w", err)
	}
	var parsed WDSGetPacketStatisticsResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSPacketStatistics{}, err
	}
	return parsed.Statistics, nil
}

// PacketStatistics reads counters associated with this packet-data session.
func (s *PDNSession) PacketStatistics(ctx context.Context, mask WDSStatisticsMask) (WDSPacketStatistics, error) {
	req := (WDSGetPacketStatisticsRequest{Mask: mask}).Request()
	resp, err := s.wdsControlRequest(ctx, req.MessageID, req.TLVs)
	if err != nil {
		return WDSPacketStatistics{}, fmt.Errorf("reading QMI WDS session packet statistics: %w", err)
	}
	var parsed WDSGetPacketStatisticsResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSPacketStatistics{}, err
	}
	return parsed.Statistics, nil
}

// WDSGoDormant asks the Client's WDS control point to release its traffic channel.
func (c *Client) WDSGoDormant(ctx context.Context, config WDSGoDormantConfig) error {
	req, err := (WDSGoDormantRequest{Config: config}).Request()
	if err != nil {
		return err
	}
	if _, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs); err != nil {
		return fmt.Errorf("placing QMI WDS channel into dormancy: %w", err)
	}
	return nil
}

// GoDormant asks this packet-data session to release its traffic channel.
func (s *PDNSession) GoDormant(ctx context.Context, config WDSGoDormantConfig) error {
	req, err := (WDSGoDormantRequest{Config: config}).Request()
	if err != nil {
		return err
	}
	if _, err := s.wdsControlRequest(ctx, req.MessageID, req.TLVs); err != nil {
		return fmt.Errorf("placing QMI WDS session into dormancy: %w", err)
	}
	return nil
}

// WDSGoActive asks the Client's WDS control point to acquire a traffic channel.
func (c *Client) WDSGoActive(ctx context.Context) error {
	if _, err := c.wdsControlRequest(ctx, MessageWDSGoActive, nil); err != nil {
		return fmt.Errorf("activating QMI WDS channel: %w", err)
	}
	return nil
}

// GoActive asks this packet-data session to acquire a traffic channel.
func (s *PDNSession) GoActive(ctx context.Context) error {
	if _, err := s.wdsControlRequest(ctx, MessageWDSGoActive, nil); err != nil {
		return fmt.Errorf("activating QMI WDS session channel: %w", err)
	}
	return nil
}

// WDSDormancyStatus reads the Client's WDS traffic-channel state.
func (c *Client) WDSDormancyStatus(ctx context.Context) (WDSDormancyStatus, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetDormancyStatus, nil)
	if err != nil {
		return 0, fmt.Errorf("reading QMI WDS dormancy status: %w", err)
	}
	var parsed WDSGetDormancyStatusResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return 0, err
	}
	return parsed.Status, nil
}

// DormancyStatus reads this packet-data session's traffic-channel state.
func (s *PDNSession) DormancyStatus(ctx context.Context) (WDSDormancyStatus, error) {
	resp, err := s.wdsControlRequest(ctx, MessageWDSGetDormancyStatus, nil)
	if err != nil {
		return 0, fmt.Errorf("reading QMI WDS session dormancy status: %w", err)
	}
	var parsed WDSGetDormancyStatusResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return 0, err
	}
	return parsed.Status, nil
}

// WDSDataBearerTechnology reads the compact legacy bearer enumeration.
func (c *Client) WDSDataBearerTechnology(ctx context.Context) (WDSDataBearerTechnologyInfo, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetDataBearerTechnology, nil)
	if err != nil {
		return WDSDataBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS data bearer technology: %w", err)
	}
	var parsed WDSGetDataBearerTechnologyResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSDataBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// DataBearerTechnology reads the compact legacy bearer for this session.
func (s *PDNSession) DataBearerTechnology(ctx context.Context) (WDSDataBearerTechnologyInfo, error) {
	resp, err := s.wdsControlRequest(ctx, MessageWDSGetDataBearerTechnology, nil)
	if err != nil {
		return WDSDataBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS session data bearer technology: %w", err)
	}
	var parsed WDSGetDataBearerTechnologyResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSDataBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// WDSCurrentDataBearerTechnology reads legacy network, RAT, and SO masks.
func (c *Client) WDSCurrentDataBearerTechnology(ctx context.Context) (WDSCurrentBearerTechnologyInfo, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetCurrentDataBearerTechnology, nil)
	if err != nil {
		return WDSCurrentBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS current bearer technology: %w", err)
	}
	var parsed WDSGetCurrentDataBearerTechnologyResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSCurrentBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// CurrentDataBearerTechnology reads legacy bearer masks for this session.
func (s *PDNSession) CurrentDataBearerTechnology(ctx context.Context) (WDSCurrentBearerTechnologyInfo, error) {
	resp, err := s.wdsControlRequest(ctx, MessageWDSGetCurrentDataBearerTechnology, nil)
	if err != nil {
		return WDSCurrentBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS session current bearer technology: %w", err)
	}
	var parsed WDSGetCurrentDataBearerTechnologyResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSCurrentBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// WDSDataBearerTechnologyExtended reads the 5G-capable bearer information.
func (c *Client) WDSDataBearerTechnologyExtended(ctx context.Context) (WDSBearerTechnologyInfo, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetDataBearerTechnologyEx, nil)
	if err != nil {
		return WDSBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS extended bearer technology: %w", err)
	}
	var parsed WDSGetDataBearerTechnologyExResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// DataBearerTechnologyExtended reads 5G-capable bearer information for this
// session.
func (s *PDNSession) DataBearerTechnologyExtended(ctx context.Context) (WDSBearerTechnologyInfo, error) {
	resp, err := s.wdsControlRequest(ctx, MessageWDSGetDataBearerTechnologyEx, nil)
	if err != nil {
		return WDSBearerTechnologyInfo{}, fmt.Errorf("reading QMI WDS session extended bearer technology: %w", err)
	}
	var parsed WDSGetDataBearerTechnologyExResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSBearerTechnologyInfo{}, err
	}
	return parsed.Technology, nil
}

// WDSBindSubscription binds the Client's WDS control point to a subscription.
func (c *Client) WDSBindSubscription(ctx context.Context, subscription WDSSubscription) error {
	req, err := (WDSBindSubscriptionRequest{Subscription: subscription}).Request()
	if err != nil {
		return err
	}
	if _, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs); err != nil {
		return fmt.Errorf("binding QMI WDS subscription: %w", err)
	}
	return nil
}

// WDSBoundSubscription reads the Client's current WDS subscription binding.
func (c *Client) WDSBoundSubscription(ctx context.Context) (WDSSubscription, error) {
	resp, err := c.wdsControlRequest(ctx, MessageWDSGetBindSubscription, nil)
	if err != nil {
		return 0, fmt.Errorf("reading QMI WDS bound subscription: %w", err)
	}
	var parsed WDSGetBindSubscriptionResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return 0, err
	}
	if !parsed.SubscriptionKnown {
		return 0, errors.New("reading QMI WDS bound subscription: subscription TLV missing")
	}
	return parsed.Subscription, nil
}

func (c *Client) wdsControlRequest(ctx context.Context, id MessageID, requestTLVs tlv.TLVs) (Response, error) {
	var resp Response
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		var err error
		resp, err = c.requestService(ctx, ServiceWDS, clientID, id, requestTLVs)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	return resp, err
}

func (s *PDNSession) wdsControlRequest(ctx context.Context, id MessageID, requestTLVs tlv.TLVs) (Response, error) {
	if s == nil {
		return Response{}, errors.New("QMI WDS session is nil")
	}
	s.mu.RLock()
	client := s.client
	clientID := s.wdsClientID
	ready := s.wdsClientReady
	timeout := s.timeout
	s.mu.RUnlock()
	if client == nil || !ready {
		return Response{}, errors.New("QMI WDS session is closed")
	}
	resp, err := client.requestServiceWithTimeout(ctx, ServiceWDS, clientID, id, requestTLVs, timeout)
	if err != nil {
		return Response{}, err
	}
	if err := resultOK(resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

func decodeWDSCurrentBearerTechnology(value []byte) (WDSCurrentBearerTechnology, error) {
	if len(value) != 9 {
		return WDSCurrentBearerTechnology{}, fmt.Errorf("current bearer TLV length %d, want 9", len(value))
	}
	return WDSCurrentBearerTechnology{
		Network:           WDSCurrentBearerNetwork(value[0]),
		RATMask:           binary.LittleEndian.Uint32(value[1:5]),
		ServiceOptionMask: binary.LittleEndian.Uint32(value[5:9]),
	}, nil
}

func decodeWDSBearerTechnology(value []byte) (WDSBearerTechnology, error) {
	if len(value) != 16 {
		return WDSBearerTechnology{}, fmt.Errorf("parsing QMI WDS extended bearer technology: bearer TLV length %d, want 16", len(value))
	}
	return WDSBearerTechnology{
		Network:        WDSBearerNetwork(binary.LittleEndian.Uint32(value[:4])),
		RAT:            WDSBearerRAT(binary.LittleEndian.Uint32(value[4:8])),
		ServiceOptions: WDSBearerServiceOptionMask(binary.LittleEndian.Uint64(value[8:16])),
	}, nil
}

func validateWDSSubscription(subscription WDSSubscription) error {
	switch subscription {
	case WDSSubscriptionDefault,
		WDSSubscriptionPrimary,
		WDSSubscriptionSecondary,
		WDSSubscriptionTertiary,
		WDSSubscriptionDontCare:
		return nil
	default:
		return fmt.Errorf("encoding QMI WDS subscription: subscription %d is out of range", subscription)
	}
}

func wdsEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, id MessageID) Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     id,
		Timeout:       timeout,
	}
}
