package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	wdsTLVSetEventChannelRate         = 0x10
	wdsTLVSetEventTransferStatistics  = 0x11
	wdsTLVSetEventDataBearer          = 0x12
	wdsTLVSetEventDormancy            = 0x13
	wdsTLVSetEventMIPStatus           = 0x14
	wdsTLVSetEventCurrentDataBearer   = 0x15
	wdsTLVSetEventDataCallStatus      = 0x17
	wdsTLVSetEventPreferredDataSystem = 0x18
	wdsTLVSetEventEVDOPageMonitor     = 0x19
	wdsTLVSetEventDataSystems         = 0x1A
	wdsTLVSetEventUplinkFlowControl   = 0x1B
	wdsTLVSetEventLimitedDataSystems  = 0x1C
	wdsTLVSetEventPDNFilterRemovals   = 0x1D
	wdsTLVSetEventExtendedDataBearer  = 0x1E

	wdsTLVEventTxPackets             = 0x10
	wdsTLVEventRxPackets             = 0x11
	wdsTLVEventTxErrors              = 0x12
	wdsTLVEventRxErrors              = 0x13
	wdsTLVEventTxOverflows           = 0x14
	wdsTLVEventRxOverflows           = 0x15
	wdsTLVEventChannelRates          = 0x16
	wdsTLVEventDataBearer            = 0x17
	wdsTLVEventDormancy              = 0x18
	wdsTLVEventTxBytes               = 0x19
	wdsTLVEventRxBytes               = 0x1A
	wdsTLVEventMIPStatus             = 0x1B
	wdsTLVEventCurrentDataBearer     = 0x1D
	wdsTLVEventDataCallStatus        = 0x1F
	wdsTLVEventPreferredDataSystem   = 0x20
	wdsTLVEventDataCallType          = 0x22
	wdsTLVEventEVDOPageMonitor       = 0x23
	wdsTLVEventDataSystems           = 0x24
	wdsTLVEventTxDropped             = 0x25
	wdsTLVEventRxDropped             = 0x26
	wdsTLVEventUplinkFlowControl     = 0x27
	wdsTLVEventDataCallAddressFamily = 0x28
	wdsTLVEventPDNFilterRemovals     = 0x29
	wdsTLVEventExtendedDataBearer    = 0x2A

	wdsMaxEventDataSystems = 16
	wdsMaxRemovedFilters   = 50
)

// WDSTransferStatisticsConfig controls periodic packet-counter indications.
type WDSTransferStatisticsConfig struct {
	IntervalSeconds uint8
	Indicators      WDSStatisticsMask
}

// WDSSetEventReportConfig selects legacy WDS event-report indications. Nil
// fields are omitted so callers do not overwrite settings they do not own.
type WDSSetEventReportConfig struct {
	ChannelRate                 *bool
	TransferStatistics          *WDSTransferStatisticsConfig
	DataBearerTechnology        *bool
	DormancyStatus              *bool
	MIPStatus                   *uint8
	CurrentDataBearerTechnology *bool
	DataCallStatus              *bool
	PreferredDataSystem         *bool
	EVDOPageMonitorChange       *bool
	DataSystems                 *bool
	UplinkFlowControl           *bool
	LimitedDataSystems          *bool
	PDNFilterRemovals           *bool
	ExtendedDataBearer          *bool
}

// WDSSetEventReportRequest encodes Set Event Report.
type WDSSetEventReportRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDSSetEventReportConfig
}

// Request validates and converts the WDS event configuration to QMI TLVs.
func (r WDSSetEventReportRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSSetEventReport,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// WDSDataCallStatus identifies activation and termination events.
type WDSDataCallStatus uint8

const (
	WDSDataCallStatusUnknown WDSDataCallStatus = iota
	WDSDataCallStatusActivated
	WDSDataCallStatusTerminated
)

// WDSPreferredDataSystem identifies the modem's preferred legacy data system.
type WDSPreferredDataSystem uint32

const (
	WDSPreferredDataSystemUnknown WDSPreferredDataSystem = iota
	WDSPreferredDataSystemCDMA1X
	WDSPreferredDataSystemEVDO
	WDSPreferredDataSystemGPRS
	WDSPreferredDataSystemWCDMA
	WDSPreferredDataSystemLTE
	WDSPreferredDataSystemTDSCDMA
)

// WDSEventDataCallType identifies the origin of a reported data call.
type WDSEventDataCallType uint8

const (
	WDSEventDataCallNone WDSEventDataCallType = iota
	WDSEventDataCallEmbedded
	WDSEventDataCallTethered
	WDSEventDataCallModemEmbedded
)

// WDSEventTetheredCallType identifies the tethering mechanism.
type WDSEventTetheredCallType uint8

const (
	WDSEventTetheredCallNone WDSEventTetheredCallType = iota
	WDSEventTetheredCallRMNET
	WDSEventTetheredCallDUN
)

// WDSEventDataCall describes one data-call type indication.
type WDSEventDataCall struct {
	Type         WDSEventDataCallType
	TetheredType WDSEventTetheredCallType
}

// WDSEVDOPageMonitorChange contains the new EVDO slot cycle and sleep flag.
type WDSEVDOPageMonitorChange struct {
	Period         uint8
	ForceLongSleep bool
}

// WDSDataSystemNetworkType identifies a 3GPP or 3GPP2 data-system family.
type WDSDataSystemNetworkType uint8

const (
	WDSDataSystem3GPP WDSDataSystemNetworkType = iota
	WDSDataSystem3GPP2
)

// WDSDataSystemNetwork contains one legacy RAT and service-option mask pair.
type WDSDataSystemNetwork struct {
	Type              WDSDataSystemNetworkType
	RATMask           uint32
	ServiceOptionMask uint32
}

// WDSDataSystems contains the preferred family and currently available data
// systems.
type WDSDataSystems struct {
	Preferred WDSDataSystemNetworkType
	Networks  []WDSDataSystemNetwork
}

// WDSDataCallAddressFamily is the four-byte IP-family value used in WDS event
// reports.
type WDSDataCallAddressFamily uint32

const (
	WDSDataCallAddressFamilyUnknown WDSDataCallAddressFamily = 0
	WDSDataCallAddressFamilyIPv4    WDSDataCallAddressFamily = 4
	WDSDataCallAddressFamilyIPv6    WDSDataCallAddressFamily = 6
)

// WDSEventReport contains all fields in the libqmi basic WDS Event Report.
type WDSEventReport struct {
	Statistics WDSPacketStatistics

	ChannelRates              WDSChannelRates
	ChannelRatesKnown         bool
	DataBearer                WDSDataBearerTechnology
	DataBearerKnown           bool
	Dormancy                  WDSDormancyStatus
	DormancyKnown             bool
	MIPStatus                 uint8
	MIPStatusKnown            bool
	CurrentDataBearer         WDSCurrentBearerTechnology
	CurrentDataBearerKnown    bool
	DataCallStatus            WDSDataCallStatus
	DataCallStatusKnown       bool
	PreferredDataSystem       WDSPreferredDataSystem
	PreferredDataSystemKnown  bool
	DataCall                  WDSEventDataCall
	DataCallKnown             bool
	EVDOPageMonitor           WDSEVDOPageMonitorChange
	EVDOPageMonitorKnown      bool
	DataSystems               WDSDataSystems
	DataSystemsKnown          bool
	UplinkFlowControlled      bool
	UplinkFlowControlKnown    bool
	AddressFamily             WDSDataCallAddressFamily
	AddressFamilyKnown        bool
	RemovedFilterHandles      []uint32
	RemovedFilterHandlesKnown bool
	ExtendedDataBearer        WDSBearerTechnology
	ExtendedDataBearerKnown   bool
}

// UnmarshalTLVs parses a QMI WDS Event Report indication.
func (e *WDSEventReport) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*e = WDSEventReport{}
	fields32 := []struct {
		kind  uint8
		value *uint32
		known *bool
	}{
		{wdsTLVEventTxPackets, &e.Statistics.TxPackets, &e.Statistics.TxPacketsKnown},
		{wdsTLVEventRxPackets, &e.Statistics.RxPackets, &e.Statistics.RxPacketsKnown},
		{wdsTLVEventTxErrors, &e.Statistics.TxErrors, &e.Statistics.TxErrorsKnown},
		{wdsTLVEventRxErrors, &e.Statistics.RxErrors, &e.Statistics.RxErrorsKnown},
		{wdsTLVEventTxOverflows, &e.Statistics.TxOverflows, &e.Statistics.TxOverflowsKnown},
		{wdsTLVEventRxOverflows, &e.Statistics.RxOverflows, &e.Statistics.RxOverflowsKnown},
		{wdsTLVEventTxDropped, &e.Statistics.TxDropped, &e.Statistics.TxDroppedKnown},
		{wdsTLVEventRxDropped, &e.Statistics.RxDropped, &e.Statistics.RxDroppedKnown},
	}
	for _, field := range fields32 {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS event report: TLV 0x%02X length %d, want 4", field.kind, len(value))
		}
		*field.value = binary.LittleEndian.Uint32(value)
		*field.known = true
	}
	fields64 := []struct {
		kind  uint8
		value *uint64
		known *bool
	}{
		{wdsTLVEventTxBytes, &e.Statistics.TxBytes, &e.Statistics.TxBytesKnown},
		{wdsTLVEventRxBytes, &e.Statistics.RxBytes, &e.Statistics.RxBytesKnown},
	}
	for _, field := range fields64 {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI WDS event report: TLV 0x%02X length %d, want 8", field.kind, len(value))
		}
		*field.value = binary.LittleEndian.Uint64(value)
		*field.known = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventChannelRates); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI WDS event report: channel rates TLV length %d, want 8", len(value))
		}
		e.ChannelRates = WDSChannelRates{
			Unit:      WDSChannelRateBitsPerSecond,
			CurrentTx: uint64(binary.LittleEndian.Uint32(value[:4])),
			CurrentRx: uint64(binary.LittleEndian.Uint32(value[4:])),
		}
		e.ChannelRatesKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDataBearer); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS event report: data bearer TLV length %d, want 1", len(value))
		}
		e.DataBearer = WDSDataBearerTechnology(int8(value[0]))
		e.DataBearerKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDormancy); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS event report: dormancy TLV length %d, want 1", len(value))
		}
		e.Dormancy = WDSDormancyStatus(value[0])
		e.DormancyKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventMIPStatus); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS event report: MIP status TLV length %d, want 1", len(value))
		}
		e.MIPStatus = value[0]
		e.MIPStatusKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventCurrentDataBearer); ok {
		if len(value) != 9 {
			return fmt.Errorf("parsing QMI WDS event report: current data bearer TLV length %d, want 9", len(value))
		}
		bearer, err := decodeWDSCurrentBearerTechnology(value)
		if err != nil {
			return err
		}
		e.CurrentDataBearer = bearer
		e.CurrentDataBearerKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDataCallStatus); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS event report: data call status TLV length %d, want 1", len(value))
		}
		e.DataCallStatus = WDSDataCallStatus(value[0])
		e.DataCallStatusKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventPreferredDataSystem); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS event report: preferred data system TLV length %d, want 4", len(value))
		}
		e.PreferredDataSystem = WDSPreferredDataSystem(binary.LittleEndian.Uint32(value))
		e.PreferredDataSystemKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDataCallType); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS event report: data call type TLV length %d, want 2", len(value))
		}
		e.DataCall = WDSEventDataCall{Type: WDSEventDataCallType(value[0]), TetheredType: WDSEventTetheredCallType(value[1])}
		e.DataCallKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventEVDOPageMonitor); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS event report: EVDO page monitor TLV length %d, want 2", len(value))
		}
		e.EVDOPageMonitor = WDSEVDOPageMonitorChange{Period: value[0], ForceLongSleep: value[1] != 0}
		e.EVDOPageMonitorKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDataSystems); ok {
		dataSystems, err := decodeWDSEventDataSystems(value)
		if err != nil {
			return err
		}
		e.DataSystems = dataSystems
		e.DataSystemsKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventUplinkFlowControl); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS event report: uplink flow control TLV length %d, want 1", len(value))
		}
		e.UplinkFlowControlled = value[0] != 0
		e.UplinkFlowControlKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventDataCallAddressFamily); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WDS event report: address family TLV length %d, want 4", len(value))
		}
		e.AddressFamily = WDSDataCallAddressFamily(binary.LittleEndian.Uint32(value))
		e.AddressFamilyKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventPDNFilterRemovals); ok {
		handles, err := decodeWDSRemovedFilterHandles(value)
		if err != nil {
			return err
		}
		e.RemovedFilterHandles = handles
		e.RemovedFilterHandlesKnown = true
	}
	if value, ok := tlv.Value(tlvs, wdsTLVEventExtendedDataBearer); ok {
		if len(value) != 16 {
			return fmt.Errorf("parsing QMI WDS event report: extended data bearer TLV length %d, want 16", len(value))
		}
		bearer, err := decodeWDSBearerTechnology(value)
		if err != nil {
			return err
		}
		e.ExtendedDataBearer = bearer
		e.ExtendedDataBearerKnown = true
	}
	return nil
}

// WDSSetEventReport configures event indications for the Client's WDS control
// point.
func (c *Client) WDSSetEventReport(ctx context.Context, config WDSSetEventReportConfig) error {
	req, err := (WDSSetEventReportRequest{Config: config}).Request()
	if err != nil {
		return err
	}
	if _, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs); err != nil {
		return fmt.Errorf("configuring QMI WDS event reports: %w", err)
	}
	return nil
}

// SetEventReport configures event indications for this packet-data session.
func (s *PDNSession) SetEventReport(ctx context.Context, config WDSSetEventReportConfig) error {
	req, err := (WDSSetEventReportRequest{Config: config}).Request()
	if err != nil {
		return err
	}
	if _, err := s.wdsControlRequest(ctx, req.MessageID, req.TLVs); err != nil {
		return fmt.Errorf("configuring QMI WDS session event reports: %w", err)
	}
	return nil
}

// MarshalTLVs encodes WDS event-report fields.
func (config WDSSetEventReportConfig) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	appendBool := func(kind uint8, value *bool) {
		if value != nil {
			tlvs = append(tlvs, tlv.Uint(kind, boolByte(*value)))
		}
	}
	appendBool(wdsTLVSetEventChannelRate, config.ChannelRate)
	if config.TransferStatistics != nil {
		if unknown := config.TransferStatistics.Indicators &^ WDSStatisticsAll; unknown != 0 {
			return nil, fmt.Errorf("encoding QMI WDS transfer statistics: indicator mask 0x%08X contains reserved bits", uint32(unknown))
		}
		value := binary.LittleEndian.AppendUint32([]byte{config.TransferStatistics.IntervalSeconds}, uint32(config.TransferStatistics.Indicators))
		tlvs = append(tlvs, tlv.Bytes(wdsTLVSetEventTransferStatistics, value))
	}
	appendBool(wdsTLVSetEventDataBearer, config.DataBearerTechnology)
	appendBool(wdsTLVSetEventDormancy, config.DormancyStatus)
	if config.MIPStatus != nil {
		tlvs = append(tlvs, tlv.Uint(wdsTLVSetEventMIPStatus, *config.MIPStatus))
	}
	appendBool(wdsTLVSetEventCurrentDataBearer, config.CurrentDataBearerTechnology)
	appendBool(wdsTLVSetEventDataCallStatus, config.DataCallStatus)
	appendBool(wdsTLVSetEventPreferredDataSystem, config.PreferredDataSystem)
	appendBool(wdsTLVSetEventEVDOPageMonitor, config.EVDOPageMonitorChange)
	appendBool(wdsTLVSetEventDataSystems, config.DataSystems)
	appendBool(wdsTLVSetEventUplinkFlowControl, config.UplinkFlowControl)
	appendBool(wdsTLVSetEventLimitedDataSystems, config.LimitedDataSystems)
	appendBool(wdsTLVSetEventPDNFilterRemovals, config.PDNFilterRemovals)
	appendBool(wdsTLVSetEventExtendedDataBearer, config.ExtendedDataBearer)
	return tlvs, nil
}

func decodeWDSEventDataSystems(value []byte) (WDSDataSystems, error) {
	if len(value) < 2 {
		return WDSDataSystems{}, errors.New("parsing QMI WDS event report: data systems header is truncated")
	}
	count := int(value[1])
	if count > wdsMaxEventDataSystems {
		return WDSDataSystems{}, fmt.Errorf("parsing QMI WDS event report: data systems count %d exceeds maximum %d", count, wdsMaxEventDataSystems)
	}
	want := 2 + count*9
	if len(value) != want {
		return WDSDataSystems{}, fmt.Errorf("parsing QMI WDS event report: data systems TLV length %d, want %d", len(value), want)
	}
	result := WDSDataSystems{
		Preferred: WDSDataSystemNetworkType(value[0]),
		Networks:  make([]WDSDataSystemNetwork, count),
	}
	for i := range count {
		offset := 2 + i*9
		result.Networks[i] = WDSDataSystemNetwork{
			Type:              WDSDataSystemNetworkType(value[offset]),
			RATMask:           binary.LittleEndian.Uint32(value[offset+1 : offset+5]),
			ServiceOptionMask: binary.LittleEndian.Uint32(value[offset+5 : offset+9]),
		}
	}
	return result, nil
}

func decodeWDSRemovedFilterHandles(value []byte) ([]uint32, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI WDS event report: removed filter count is truncated")
	}
	count := int(value[0])
	if count > wdsMaxRemovedFilters {
		return nil, fmt.Errorf("parsing QMI WDS event report: removed filter count %d exceeds maximum %d", count, wdsMaxRemovedFilters)
	}
	want := 1 + count*4
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI WDS event report: removed filter TLV length %d, want %d", len(value), want)
	}
	handles := make([]uint32, count)
	for i := range count {
		offset := 1 + i*4
		handles[i] = binary.LittleEndian.Uint32(value[offset : offset+4])
	}
	return handles, nil
}
