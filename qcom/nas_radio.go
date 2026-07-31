package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasMaxCDMABaseStations      = 10
	nasMaxLTESecondaryCells     = 4
	nasRXChainInfoLength        = 21
	nasCDMABaseStationLength    = 30
	nasLTEPrimaryCellLength     = 10
	nasLTESecondaryCellLength   = 14
	nasLTESecondaryCellExLength = 15
)

// NASRXChainInfo contains raw receive measurements for one modem RF chain.
type NASRXChainInfo struct {
	Index   uint8
	Tuned   bool
	RXPower int32
	ECIO    int32
	RSCP    int32
	RSRP    int32
	Phase   uint32
}

// NASTXInfo contains current transmit state and raw power.
type NASTXInfo struct {
	InTraffic bool
	Power     int32
}

// NASTxRxInfo contains per-chain receive data and optional transmit data.
type NASTxRxInfo struct {
	RXChains []NASRXChainInfo
	TX       NASTXInfo
	TXKnown  bool
}

// NASCDMAPilotType identifies an active or neighboring CDMA pilot.
type NASCDMAPilotType uint32

const (
	NASCDMAPilotActive NASCDMAPilotType = iota
	NASCDMAPilotNeighbor
)

// NASCDMABaseStation contains one CDMA pilot position report.
type NASCDMABaseStation struct {
	PilotType     NASCDMAPilotType
	SystemID      uint16
	NetworkID     uint16
	BaseStationID uint16
	PilotPN       uint16
	PilotStrength uint16
	Latitude      int32
	Longitude     int32
	GPSTimeMillis uint64
}

// NASCDMAPositionInfo contains the serving and neighboring CDMA positions.
type NASCDMAPositionInfo struct {
	Known        bool
	UEInIdle     bool
	BaseStations []NASCDMABaseStation
}

// NASDRX is the legacy circuit-switched discontinuous-reception cycle.
type NASDRX uint32

const (
	NASDRXUnknown NASDRX = 0x00
	NASDRXCN6T32  NASDRX = 0x06
	NASDRXCN7T64  NASDRX = 0x07
	NASDRXCN8T128 NASDRX = 0x08
	NASDRXCN9T256 NASDRX = 0x09
)

// NASSecondaryCellState describes an LTE secondary carrier state.
type NASSecondaryCellState uint32

const (
	NASSecondaryCellDeconfigured NASSecondaryCellState = iota
	NASSecondaryCellConfiguredDeactivated
	NASSecondaryCellConfiguredActivated
)

// NASLTECarrierAggregationCell contains one primary or secondary LTE carrier.
type NASLTECarrierAggregationCell struct {
	PhysicalCellID uint16
	RXChannel      uint16
	DLBandwidth    NASBandwidth
	Band           NASActiveBand
	State          NASSecondaryCellState
	StateKnown     bool
	Index          uint8
	IndexKnown     bool
}

// NASLTECarrierAggregationInfo contains LTE primary and secondary carriers.
type NASLTECarrierAggregationInfo struct {
	DLBandwidth      NASBandwidth
	DLBandwidthKnown bool
	PrimaryCell      NASLTECarrierAggregationCell
	PrimaryCellKnown bool
	SecondaryCells   []NASLTECarrierAggregationCell
}

// NASENDCConfig describes LTE/NR dual-connectivity configuration.
type NASENDCConfig struct {
	Enabled                  bool
	EnabledKnown             bool
	ImmediateSCGRelease      bool
	ImmediateSCGReleaseKnown bool
}

// NASReset resets the NAS service state.
func (c *Client) NASReset(ctx context.Context) error {
	if err := c.nasRead(ctx, MessageNASReset, nil); err != nil {
		return fmt.Errorf("resetting QMI NAS service: %w", err)
	}
	return nil
}

// TxRxInfo returns raw RF-chain measurements for one radio interface.
func (c *Client) TxRxInfo(ctx context.Context, radio NASRadioInterface) (NASTxRxInfo, error) {
	req := nasEmptyRequest(0, 0, DefaultRequestTimeout, MessageNASGetTxRxInfo)
	req.TLVs = tlv.TLVs{tlv.Uint(0x01, uint8(radio))}
	var result NASTxRxInfo
	if err := c.nasReadRequest(ctx, req, &result); err != nil {
		return NASTxRxInfo{}, fmt.Errorf("querying QMI NAS Tx/Rx information: %w", err)
	}
	return result, nil
}

// CDMAPositionInfo returns serving and neighboring CDMA base-station positions.
func (c *Client) CDMAPositionInfo(ctx context.Context) (NASCDMAPositionInfo, error) {
	var result NASCDMAPositionInfo
	if err := c.nasRead(ctx, MessageNASGetCDMAPositionInfo, &result); err != nil {
		return NASCDMAPositionInfo{}, fmt.Errorf("querying QMI NAS CDMA position information: %w", err)
	}
	return result, nil
}

// ForceNetworkSearch asks the modem to immediately search for service.
func (c *Client) ForceNetworkSearch(ctx context.Context) error {
	if err := c.nasRead(ctx, MessageNASForceNetworkSearch, nil); err != nil {
		return fmt.Errorf("forcing QMI NAS network search: %w", err)
	}
	return nil
}

// DRX returns the modem's configured legacy DRX cycle.
func (c *Client) DRX(ctx context.Context) (NASDRX, error) {
	var result NASDRX
	if err := c.nasRead(ctx, MessageNASGetDRX, &result); err != nil {
		return 0, fmt.Errorf("querying QMI NAS DRX: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses the QMI NAS legacy DRX cycle.
func (drx *NASDRX) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*drx = NASDRXUnknown
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return errors.New("parsing QMI NAS DRX: info TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI NAS DRX: info TLV length %d, want 4", len(value))
	}
	*drx = NASDRX(binary.LittleEndian.Uint32(value))
	if !validNASDRX(*drx) {
		return fmt.Errorf("parsing QMI NAS DRX: value %d is outside the supported range", *drx)
	}
	return nil
}

func validNASDRX(value NASDRX) bool {
	return value == NASDRXUnknown || value >= NASDRXCN6T32 && value <= NASDRXCN9T256
}

// LTECarrierAggregationInfo returns the active LTE primary and secondary carriers.
func (c *Client) LTECarrierAggregationInfo(ctx context.Context) (NASLTECarrierAggregationInfo, error) {
	var result NASLTECarrierAggregationInfo
	if err := c.nasRead(ctx, MessageNASGetLTECarrierAggregationInfo, &result); err != nil {
		return NASLTECarrierAggregationInfo{}, fmt.Errorf("querying QMI NAS LTE carrier aggregation: %w", err)
	}
	return result, nil
}

// ENDCConfig returns LTE/NR dual-connectivity configuration.
func (c *Client) ENDCConfig(ctx context.Context) (NASENDCConfig, error) {
	var result NASENDCConfig
	if err := c.nasRead(ctx, MessageNASGetENDCConfig, &result); err != nil {
		return NASENDCConfig{}, fmt.Errorf("querying QMI NAS EN-DC configuration: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses QMI NAS Get Tx Rx Info response TLVs.
func (i *NASTxRxInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = NASTxRxInfo{}
	chainTLVs := [...]byte{0x10, 0x11, 0x15, 0x16}
	for index, kind := range chainTLVs {
		value, ok := tlv.Value(tlvs, kind)
		if !ok {
			continue
		}
		chain, err := decodeNASRXChain(uint8(index), value)
		if err != nil {
			return err
		}
		i.RXChains = append(i.RXChains, chain)
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI NAS Tx/Rx information: TX TLV length %d, want 5", len(value))
		}
		i.TX = NASTXInfo{
			InTraffic: value[0] != 0,
			Power:     int32(binary.LittleEndian.Uint32(value[1:5])),
		}
		i.TXKnown = true
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get CDMA Position Info response TLVs.
func (i *NASCDMAPositionInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = NASCDMAPositionInfo{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) < 2 {
		return errors.New("parsing QMI NAS CDMA position information: header is truncated")
	}
	i.Known = true
	count := int(value[1])
	if count > nasMaxCDMABaseStations {
		return fmt.Errorf("parsing QMI NAS CDMA position information: base-station count %d exceeds %d", count, nasMaxCDMABaseStations)
	}
	want := 2 + count*nasCDMABaseStationLength
	if len(value) != want {
		return fmt.Errorf("parsing QMI NAS CDMA position information: TLV length %d, want %d", len(value), want)
	}

	i.UEInIdle = value[0] != 0
	i.BaseStations = make([]NASCDMABaseStation, count)
	for index := range count {
		offset := 2 + index*nasCDMABaseStationLength
		i.BaseStations[index] = NASCDMABaseStation{
			PilotType:     NASCDMAPilotType(binary.LittleEndian.Uint32(value[offset : offset+4])),
			SystemID:      binary.LittleEndian.Uint16(value[offset+4 : offset+6]),
			NetworkID:     binary.LittleEndian.Uint16(value[offset+6 : offset+8]),
			BaseStationID: binary.LittleEndian.Uint16(value[offset+8 : offset+10]),
			PilotPN:       binary.LittleEndian.Uint16(value[offset+10 : offset+12]),
			PilotStrength: binary.LittleEndian.Uint16(value[offset+12 : offset+14]),
			Latitude:      int32(binary.LittleEndian.Uint32(value[offset+14 : offset+18])),
			Longitude:     int32(binary.LittleEndian.Uint32(value[offset+18 : offset+22])),
			GPSTimeMillis: binary.LittleEndian.Uint64(value[offset+22 : offset+30]),
		}
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get LTE CPHY CA Info response TLVs.
func (i *NASLTECarrierAggregationInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = NASLTECarrierAggregationInfo{}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS LTE carrier aggregation: bandwidth TLV length %d, want 4", len(value))
		}
		i.DLBandwidth = NASBandwidth(binary.LittleEndian.Uint32(value))
		i.DLBandwidthKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		cell, err := decodeNASLTECarrierCell(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE carrier aggregation primary cell: %w", err)
		}
		i.PrimaryCell = cell
		i.PrimaryCellKnown = true
	}

	var legacy *NASLTECarrierAggregationCell
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		cell, err := decodeNASLTECarrierCell(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE carrier aggregation secondary cell: %w", err)
		}
		legacy = &cell
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS LTE carrier aggregation: secondary-cell index TLV length %d, want 1", len(value))
		}
		if legacy != nil {
			legacy.Index = value[0]
			legacy.IndexKnown = true
		}
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		cells, err := decodeNASLTESecondaryCells(value)
		if err != nil {
			return err
		}
		i.SecondaryCells = cells
	} else if legacy != nil {
		i.SecondaryCells = []NASLTECarrierAggregationCell{*legacy}
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get ENDC Config response TLVs.
func (c *NASENDCConfig) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*c = NASENDCConfig{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS EN-DC configuration: enabled TLV length %d, want 1", len(value))
		}
		if value[0] > 1 {
			return fmt.Errorf("parsing QMI NAS EN-DC configuration: invalid enabled boolean %d", value[0])
		}
		c.Enabled = value[0] == 1
		c.EnabledKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS EN-DC configuration: immediate-release TLV length %d, want 1", len(value))
		}
		if value[0] > 1 {
			return fmt.Errorf("parsing QMI NAS EN-DC configuration: invalid immediate-release boolean %d", value[0])
		}
		c.ImmediateSCGRelease = value[0] == 1
		c.ImmediateSCGReleaseKnown = true
	}
	return nil
}

func decodeNASRXChain(index uint8, value []byte) (NASRXChainInfo, error) {
	if len(value) != nasRXChainInfoLength {
		return NASRXChainInfo{}, fmt.Errorf("parsing QMI NAS Tx/Rx information: RX chain %d TLV length %d, want %d", index, len(value), nasRXChainInfoLength)
	}
	return NASRXChainInfo{
		Index:   index,
		Tuned:   value[0] != 0,
		RXPower: int32(binary.LittleEndian.Uint32(value[1:5])),
		ECIO:    int32(binary.LittleEndian.Uint32(value[5:9])),
		RSCP:    int32(binary.LittleEndian.Uint32(value[9:13])),
		RSRP:    int32(binary.LittleEndian.Uint32(value[13:17])),
		Phase:   binary.LittleEndian.Uint32(value[17:21]),
	}, nil
}

func decodeNASLTECarrierCell(value []byte, secondary bool) (NASLTECarrierAggregationCell, error) {
	want := nasLTEPrimaryCellLength
	if secondary {
		want = nasLTESecondaryCellLength
	}
	if len(value) != want {
		return NASLTECarrierAggregationCell{}, fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	cell := NASLTECarrierAggregationCell{
		PhysicalCellID: binary.LittleEndian.Uint16(value[0:2]),
		RXChannel:      binary.LittleEndian.Uint16(value[2:4]),
		DLBandwidth:    NASBandwidth(binary.LittleEndian.Uint32(value[4:8])),
		Band:           NASActiveBand(binary.LittleEndian.Uint16(value[8:10])),
	}
	if secondary {
		cell.State = NASSecondaryCellState(binary.LittleEndian.Uint32(value[10:14]))
		cell.StateKnown = true
	}
	return cell, nil
}

func decodeNASLTESecondaryCells(value []byte) ([]NASLTECarrierAggregationCell, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS LTE carrier aggregation: secondary-cell count is missing")
	}
	count := int(value[0])
	if count > nasMaxLTESecondaryCells {
		return nil, fmt.Errorf("parsing QMI NAS LTE carrier aggregation: secondary-cell count %d exceeds %d", count, nasMaxLTESecondaryCells)
	}
	want := 1 + count*nasLTESecondaryCellExLength
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS LTE carrier aggregation: secondary-cell TLV length %d, want %d", len(value), want)
	}
	cells := make([]NASLTECarrierAggregationCell, count)
	for index := range count {
		offset := 1 + index*nasLTESecondaryCellExLength
		cell, err := decodeNASLTECarrierCell(value[offset:offset+nasLTESecondaryCellLength], true)
		if err != nil {
			return nil, err
		}
		cell.Index = value[offset+nasLTESecondaryCellLength]
		cell.IndexKnown = true
		cells[index] = cell
	}
	return cells, nil
}
