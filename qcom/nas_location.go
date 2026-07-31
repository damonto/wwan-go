package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVLocationGERAN            = 0x10
	nasTLVLocationUMTS             = 0x11
	nasTLVLocationCDMA             = 0x12
	nasTLVLocationLTEIntra         = 0x13
	nasTLVLocationLTEInter         = 0x14
	nasTLVLocationLTEGERAN         = 0x15
	nasTLVLocationLTEWCDMA         = 0x16
	nasTLVLocationUMTSCellID       = 0x17
	nasTLVLocationUMTSLTE          = 0x18
	nasTLVLocationLTETimingAdvance = 0x1E
	nasTLVLocationLTEIntraEARFCN   = 0x27
	nasTLVLocationLTEInterEARFCNs  = 0x28
	nasTLVLocationUMTSLTEEARFCNs   = 0x29
	nasTLVLocationNR5GARFCN        = 0x2E
	nasTLVLocationNR5GCell         = 0x2F
)

// NASGERANNeighborCell contains one measured GSM neighbor.
type NASGERANNeighborCell struct {
	CellID      uint32
	CellIDKnown bool
	PLMN        NASPLMN
	PLMNKnown   bool
	LAC         uint16
	ARFCN       uint16
	BSIC        uint8
	RXLevel     uint16
}

// NASGERANCellLocation contains the GSM serving cell and its neighbors.
type NASGERANCellLocation struct {
	CellID             uint32
	CellIDKnown        bool
	PLMN               NASPLMN
	PLMNKnown          bool
	LAC                uint16
	ARFCN              uint16
	BSIC               uint8
	TimingAdvance      uint32
	TimingAdvanceKnown bool
	RXLevel            uint16
	Neighbors          []NASGERANNeighborCell
}

// NASUMTSNeighborCell contains one monitored WCDMA cell.
type NASUMTSNeighborCell struct {
	UARFCN uint16
	PSC    uint16
	RSCP   int16
	ECIO   int16
}

// NASUMTSGERANNeighborCell contains one GSM neighbor measured by WCDMA.
type NASUMTSGERANNeighborCell struct {
	ARFCN uint16
	NCC   uint8
	BCC   uint8
	RSSI  int16
}

// NASUMTSCellLocation contains the WCDMA serving cell and its neighbors.
type NASUMTSCellLocation struct {
	CellID         uint16
	PLMN           NASPLMN
	PLMNKnown      bool
	LAC            uint16
	UARFCN         uint16
	PSC            uint16
	RSCP           int16
	ECIO           int16
	Neighbors      []NASUMTSNeighborCell
	GERANNeighbors []NASUMTSGERANNeighborCell
}

// NASCDMACellLocation contains the current CDMA base-station identity.
type NASCDMACellLocation struct {
	SystemID      uint16
	NetworkID     uint16
	BaseStationID uint16
	ReferencePN   uint16
	Latitude      uint32
	Longitude     uint32
}

// NASLTECellMeasurement contains one measured LTE cell.
type NASLTECellMeasurement struct {
	PhysicalCellID   uint16
	RSRQ             int16
	RSRP             int16
	RSSI             int16
	SelectionRXLevel int16
}

// NASLTEIntraFrequency contains the serving LTE frequency and measured cells.
type NASLTEIntraFrequency struct {
	UEInIdle                bool
	PLMN                    NASPLMN
	PLMNKnown               bool
	TrackingAreaCode        uint16
	GlobalCellID            uint32
	EARFCN                  uint16
	ServingCellID           uint16
	CellReselectionPriority uint8
	NonIntraSearchThreshold uint8
	ServingCellLowThreshold uint8
	IntraSearchThreshold    uint8
	Cells                   []NASLTECellMeasurement
}

// NASLTEInterFrequencyEntry contains cells measured on one other LTE frequency.
type NASLTEInterFrequencyEntry struct {
	EARFCN                  uint16
	LowThreshold            uint8
	HighThreshold           uint8
	CellReselectionPriority uint8
	Cells                   []NASLTECellMeasurement
}

// NASLTEInterFrequency contains LTE interfrequency measurements.
type NASLTEInterFrequency struct {
	UEInIdle    bool
	Frequencies []NASLTEInterFrequencyEntry
}

// NASLTEGERANCell contains one GSM cell measured by LTE.
type NASLTEGERANCell struct {
	ARFCN            uint16
	Band1900         bool
	CellIDValid      bool
	BSIC             uint8
	RSSI             int16
	SelectionRXLevel int16
}

// NASLTEGERANFrequency contains one LTE-to-GSM neighbor group.
type NASLTEGERANFrequency struct {
	CellReselectionPriority uint8
	HighThreshold           uint8
	LowThreshold            uint8
	NCCPermitted            uint8
	Cells                   []NASLTEGERANCell
}

// NASLTEGERANNeighbors contains GSM neighbors measured by LTE.
type NASLTEGERANNeighbors struct {
	UEInIdle    bool
	Frequencies []NASLTEGERANFrequency
}

// NASLTEWCDMACell contains one WCDMA cell measured by LTE.
type NASLTEWCDMACell struct {
	PSC              uint16
	CPICHRSCP        int16
	CPICHECNO        int16
	SelectionRXLevel int16
}

// NASLTEWCDMAFrequency contains one LTE-to-WCDMA neighbor group.
type NASLTEWCDMAFrequency struct {
	UARFCN                  uint16
	CellReselectionPriority uint8
	HighThreshold           uint16
	LowThreshold            uint16
	Cells                   []NASLTEWCDMACell
}

// NASLTEWCDMANeighbors contains WCDMA neighbors measured by LTE.
type NASLTEWCDMANeighbors struct {
	UEInIdle    bool
	Frequencies []NASLTEWCDMAFrequency
}

// NASWCDMARRCState identifies the current WCDMA RRC state.
type NASWCDMARRCState uint32

const (
	NASWCDMARRCDisconnected NASWCDMARRCState = iota
	NASWCDMARRCCellPCH
	NASWCDMARRCURAPCH
	NASWCDMARRCCellFACH
	NASWCDMARRCCellDCH
)

// NASUMTSLTENeighbor contains one LTE cell measured by WCDMA.
type NASUMTSLTENeighbor struct {
	EARFCN           uint16
	PhysicalCellID   uint16
	RSRP             float32
	RSRQ             float32
	SelectionRXLevel int16
	TDD              bool
}

// NASUMTSLTENeighbors contains LTE neighbors measured by WCDMA.
type NASUMTSLTENeighbors struct {
	RRCState NASWCDMARRCState
	Cells    []NASUMTSLTENeighbor
}

// NASNR5GCellLocation contains the current NR serving-cell measurements.
type NASNR5GCellLocation struct {
	PLMN             NASPLMN
	PLMNKnown        bool
	TrackingAreaCode [3]byte
	GlobalCellID     uint64
	PhysicalCellID   uint16
	RSRQ             int16
	RSRP             int16
	SNR              int16
}

// NASCellLocationInfo contains serving-cell and neighbor measurements by RAT.
type NASCellLocationInfo struct {
	GERAN      NASGERANCellLocation
	GERANKnown bool
	UMTS       NASUMTSCellLocation
	UMTSKnown  bool
	CDMA       NASCDMACellLocation
	CDMAKnown  bool

	LTEIntra      NASLTEIntraFrequency
	LTEIntraKnown bool
	LTEInter      NASLTEInterFrequency
	LTEInterKnown bool
	LTEGERAN      NASLTEGERANNeighbors
	LTEGERANKnown bool
	LTEWCDMA      NASLTEWCDMANeighbors
	LTEWCDMAKnown bool

	UMTSCellID      uint32
	UMTSCellIDKnown bool
	UMTSLTE         NASUMTSLTENeighbors
	UMTSLTEKnown    bool

	LTETimingAdvance      int32
	LTETimingAdvanceKnown bool
	LTEIntraEARFCN        uint32
	LTEIntraEARFCNKnown   bool
	LTEInterEARFCNs       []uint32
	LTEInterEARFCNsKnown  bool
	UMTSLTEEARFCNs        []uint32
	UMTSLTEEARFCNsKnown   bool

	NR5GARFCN      uint32
	NR5GARFCNKnown bool
	NR5G           NASNR5GCellLocation
	NR5GKnown      bool
}

// CellLocationInfo returns current serving-cell and neighbor measurements.
func (c *Client) CellLocationInfo(ctx context.Context) (NASCellLocationInfo, error) {
	var result NASCellLocationInfo
	if err := c.nasRead(ctx, MessageNASGetCellLocationInfo, &result); err != nil {
		return NASCellLocationInfo{}, fmt.Errorf("querying QMI NAS cell location information: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses a QMI NAS Get Cell Location Info response.
func (i *NASCellLocationInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = NASCellLocationInfo{}
	if value, ok := tlv.Value(tlvs, nasTLVLocationGERAN); ok {
		parsed, err := decodeNASGERANLocation(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS GERAN location: %w", err)
		}
		i.GERAN, i.GERANKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationUMTS); ok {
		parsed, err := decodeNASUMTSLocation(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS UMTS location: %w", err)
		}
		i.UMTS, i.UMTSKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationCDMA); ok {
		parsed, err := decodeNASCDMALocation(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS CDMA location: %w", err)
		}
		i.CDMA, i.CDMAKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEIntra); ok {
		parsed, err := decodeNASLTEIntraFrequency(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE intrafrequency location: %w", err)
		}
		i.LTEIntra, i.LTEIntraKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEInter); ok {
		parsed, err := decodeNASLTEInterFrequency(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE interfrequency location: %w", err)
		}
		i.LTEInter, i.LTEInterKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEGERAN); ok {
		parsed, err := decodeNASLTEGERANNeighbors(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE GERAN-neighbor location: %w", err)
		}
		i.LTEGERAN, i.LTEGERANKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEWCDMA); ok {
		parsed, err := decodeNASLTEWCDMANeighbors(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE WCDMA-neighbor location: %w", err)
		}
		i.LTEWCDMA, i.LTEWCDMAKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationUMTSCellID); ok {
		parsed, err := decodeNASLocationUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS UMTS cell ID: %w", err)
		}
		i.UMTSCellID, i.UMTSCellIDKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationUMTSLTE); ok {
		parsed, err := decodeNASUMTSLTENeighbors(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS UMTS LTE-neighbor location: %w", err)
		}
		i.UMTSLTE, i.UMTSLTEKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTETimingAdvance); ok {
		parsed, err := decodeNASLocationUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE timing advance: %w", err)
		}
		i.LTETimingAdvance, i.LTETimingAdvanceKnown = int32(parsed), true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEIntraEARFCN); ok {
		parsed, err := decodeNASLocationUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE intrafrequency EARFCN: %w", err)
		}
		i.LTEIntraEARFCN, i.LTEIntraEARFCNKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationLTEInterEARFCNs); ok {
		parsed, err := decodeNASLocationUint32Array(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS LTE interfrequency EARFCNs: %w", err)
		}
		i.LTEInterEARFCNs, i.LTEInterEARFCNsKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationUMTSLTEEARFCNs); ok {
		parsed, err := decodeNASLocationUint32Array(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS UMTS LTE-neighbor EARFCNs: %w", err)
		}
		i.UMTSLTEEARFCNs, i.UMTSLTEEARFCNsKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationNR5GARFCN); ok {
		parsed, err := decodeNASLocationUint32(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS NR5G ARFCN: %w", err)
		}
		i.NR5GARFCN, i.NR5GARFCNKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationNR5GCell); ok {
		parsed, err := decodeNASNR5GLocation(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS NR5G location: %w", err)
		}
		i.NR5G, i.NR5GKnown = parsed, true
	}
	return nil
}

func decodeNASGERANLocation(value []byte) (NASGERANCellLocation, error) {
	c := nasLocationCursor{value: value}
	cellID, err := c.uint32()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	rawPLMN, err := c.take(3)
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	plmn, plmnKnown, err := decodeNASLocationPLMN(rawPLMN)
	if err != nil && cellID != math.MaxUint32 {
		return NASGERANCellLocation{}, fmt.Errorf("parsing QMI NAS GERAN information PLMN: %w", err)
	}
	lac, err := c.uint16()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	arfcn, err := c.uint16()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	bsic, err := c.uint8()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	timingAdvance, err := c.uint32()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	rxLevel, err := c.uint16()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	count, err := c.uint8()
	if err != nil {
		return NASGERANCellLocation{}, err
	}
	neighbors := make([]NASGERANNeighborCell, int(count))
	for index := range neighbors {
		neighborID, err := c.uint32()
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighborPLMNRaw, err := c.take(3)
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighborPLMN, neighborPLMNKnown, err := decodeNASLocationPLMN(neighborPLMNRaw)
		if err != nil && neighborID != math.MaxUint32 {
			return NASGERANCellLocation{}, fmt.Errorf("parsing QMI NAS GERAN neighbor %d PLMN: %w", index, err)
		}
		neighborLAC, err := c.uint16()
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighborARFCN, err := c.uint16()
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighborBSIC, err := c.uint8()
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighborRXLevel, err := c.uint16()
		if err != nil {
			return NASGERANCellLocation{}, err
		}
		neighbors[index] = NASGERANNeighborCell{
			CellID: neighborID, CellIDKnown: neighborID != math.MaxUint32,
			PLMN: neighborPLMN, PLMNKnown: neighborPLMNKnown && neighborID != math.MaxUint32,
			LAC: neighborLAC, ARFCN: neighborARFCN, BSIC: neighborBSIC, RXLevel: neighborRXLevel,
		}
	}
	if err := c.finish(); err != nil {
		return NASGERANCellLocation{}, err
	}
	return NASGERANCellLocation{
		CellID: cellID, CellIDKnown: cellID != math.MaxUint32,
		PLMN: plmn, PLMNKnown: plmnKnown && cellID != math.MaxUint32,
		LAC: lac, ARFCN: arfcn, BSIC: bsic,
		TimingAdvance: timingAdvance, TimingAdvanceKnown: timingAdvance != math.MaxUint32,
		RXLevel: rxLevel, Neighbors: neighbors,
	}, nil
}

func decodeNASUMTSLocation(value []byte) (NASUMTSCellLocation, error) {
	c := nasLocationCursor{value: value}
	cellID, err := c.uint16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	rawPLMN, err := c.take(3)
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	plmn, plmnKnown, err := decodeNASLocationPLMN(rawPLMN)
	if err != nil {
		return NASUMTSCellLocation{}, fmt.Errorf("parsing QMI NAS UMTS information PLMN: %w", err)
	}
	lac, err := c.uint16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	uarfcn, err := c.uint16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	psc, err := c.uint16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	rscp, err := c.int16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	ecio, err := c.int16()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	monitoredCount, err := c.uint8()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	neighbors := make([]NASUMTSNeighborCell, int(monitoredCount))
	for index := range neighbors {
		neighbors[index], err = decodeNASUMTSNeighbor(&c)
		if err != nil {
			return NASUMTSCellLocation{}, err
		}
	}
	geranCount, err := c.uint8()
	if err != nil {
		return NASUMTSCellLocation{}, err
	}
	geranNeighbors := make([]NASUMTSGERANNeighborCell, int(geranCount))
	for index := range geranNeighbors {
		neighborARFCN, err := c.uint16()
		if err != nil {
			return NASUMTSCellLocation{}, err
		}
		ncc, err := c.uint8()
		if err != nil {
			return NASUMTSCellLocation{}, err
		}
		bcc, err := c.uint8()
		if err != nil {
			return NASUMTSCellLocation{}, err
		}
		rssi, err := c.int16()
		if err != nil {
			return NASUMTSCellLocation{}, err
		}
		geranNeighbors[index] = NASUMTSGERANNeighborCell{ARFCN: neighborARFCN, NCC: ncc, BCC: bcc, RSSI: rssi}
	}
	if err := c.finish(); err != nil {
		return NASUMTSCellLocation{}, err
	}
	return NASUMTSCellLocation{
		CellID: cellID, PLMN: plmn, PLMNKnown: plmnKnown, LAC: lac, UARFCN: uarfcn,
		PSC: psc, RSCP: rscp, ECIO: ecio, Neighbors: neighbors, GERANNeighbors: geranNeighbors,
	}, nil
}

func decodeNASUMTSNeighbor(c *nasLocationCursor) (NASUMTSNeighborCell, error) {
	uarfcn, err := c.uint16()
	if err != nil {
		return NASUMTSNeighborCell{}, err
	}
	psc, err := c.uint16()
	if err != nil {
		return NASUMTSNeighborCell{}, err
	}
	rscp, err := c.int16()
	if err != nil {
		return NASUMTSNeighborCell{}, err
	}
	ecio, err := c.int16()
	if err != nil {
		return NASUMTSNeighborCell{}, err
	}
	return NASUMTSNeighborCell{UARFCN: uarfcn, PSC: psc, RSCP: rscp, ECIO: ecio}, nil
}

func decodeNASCDMALocation(value []byte) (NASCDMACellLocation, error) {
	if len(value) != 16 {
		return NASCDMACellLocation{}, fmt.Errorf("parsing QMI NAS CDMA information: TLV length %d, want 16", len(value))
	}
	return NASCDMACellLocation{
		SystemID: binary.LittleEndian.Uint16(value[0:2]), NetworkID: binary.LittleEndian.Uint16(value[2:4]),
		BaseStationID: binary.LittleEndian.Uint16(value[4:6]), ReferencePN: binary.LittleEndian.Uint16(value[6:8]),
		Latitude: binary.LittleEndian.Uint32(value[8:12]), Longitude: binary.LittleEndian.Uint32(value[12:16]),
	}, nil
}

func decodeNASLTEIntraFrequency(value []byte) (NASLTEIntraFrequency, error) {
	c := nasLocationCursor{value: value}
	idle, err := c.uint8()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	rawPLMN, err := c.take(3)
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	plmn, plmnKnown, err := decodeNASLocationPLMN(rawPLMN)
	if err != nil {
		return NASLTEIntraFrequency{}, fmt.Errorf("parsing QMI NAS LTE intrafrequency PLMN: %w", err)
	}
	tac, err := c.uint16()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	globalCellID, err := c.uint32()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	earfcn, err := c.uint16()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	servingCellID, err := c.uint16()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	priority, err := c.uint8()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	nonIntra, err := c.uint8()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	servingLow, err := c.uint8()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	intra, err := c.uint8()
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	cells, err := decodeNASLTECells(&c)
	if err != nil {
		return NASLTEIntraFrequency{}, err
	}
	if err := c.finish(); err != nil {
		return NASLTEIntraFrequency{}, err
	}
	return NASLTEIntraFrequency{
		UEInIdle: idle != 0, PLMN: plmn, PLMNKnown: plmnKnown, TrackingAreaCode: tac,
		GlobalCellID: globalCellID, EARFCN: earfcn, ServingCellID: servingCellID,
		CellReselectionPriority: priority, NonIntraSearchThreshold: nonIntra,
		ServingCellLowThreshold: servingLow, IntraSearchThreshold: intra, Cells: cells,
	}, nil
}

func decodeNASLTEInterFrequency(value []byte) (NASLTEInterFrequency, error) {
	c := nasLocationCursor{value: value}
	idle, err := c.uint8()
	if err != nil {
		return NASLTEInterFrequency{}, err
	}
	count, err := c.uint8()
	if err != nil {
		return NASLTEInterFrequency{}, err
	}
	frequencies := make([]NASLTEInterFrequencyEntry, int(count))
	for index := range frequencies {
		earfcn, err := c.uint16()
		if err != nil {
			return NASLTEInterFrequency{}, err
		}
		low, err := c.uint8()
		if err != nil {
			return NASLTEInterFrequency{}, err
		}
		high, err := c.uint8()
		if err != nil {
			return NASLTEInterFrequency{}, err
		}
		priority, err := c.uint8()
		if err != nil {
			return NASLTEInterFrequency{}, err
		}
		cells, err := decodeNASLTECells(&c)
		if err != nil {
			return NASLTEInterFrequency{}, err
		}
		frequencies[index] = NASLTEInterFrequencyEntry{
			EARFCN: earfcn, LowThreshold: low, HighThreshold: high,
			CellReselectionPriority: priority, Cells: cells,
		}
	}
	if err := c.finish(); err != nil {
		return NASLTEInterFrequency{}, err
	}
	return NASLTEInterFrequency{UEInIdle: idle != 0, Frequencies: frequencies}, nil
}

func decodeNASLTECells(c *nasLocationCursor) ([]NASLTECellMeasurement, error) {
	count, err := c.uint8()
	if err != nil {
		return nil, err
	}
	cells := make([]NASLTECellMeasurement, int(count))
	for index := range cells {
		pci, err := c.uint16()
		if err != nil {
			return nil, err
		}
		rsrq, err := c.int16()
		if err != nil {
			return nil, err
		}
		rsrp, err := c.int16()
		if err != nil {
			return nil, err
		}
		rssi, err := c.int16()
		if err != nil {
			return nil, err
		}
		srxlev, err := c.int16()
		if err != nil {
			return nil, err
		}
		cells[index] = NASLTECellMeasurement{
			PhysicalCellID: pci, RSRQ: rsrq, RSRP: rsrp, RSSI: rssi, SelectionRXLevel: srxlev,
		}
	}
	return cells, nil
}

func decodeNASLTEGERANNeighbors(value []byte) (NASLTEGERANNeighbors, error) {
	c := nasLocationCursor{value: value}
	idle, err := c.uint8()
	if err != nil {
		return NASLTEGERANNeighbors{}, err
	}
	count, err := c.uint8()
	if err != nil {
		return NASLTEGERANNeighbors{}, err
	}
	frequencies := make([]NASLTEGERANFrequency, int(count))
	for index := range frequencies {
		priority, err := c.uint8()
		if err != nil {
			return NASLTEGERANNeighbors{}, err
		}
		high, err := c.uint8()
		if err != nil {
			return NASLTEGERANNeighbors{}, err
		}
		low, err := c.uint8()
		if err != nil {
			return NASLTEGERANNeighbors{}, err
		}
		ncc, err := c.uint8()
		if err != nil {
			return NASLTEGERANNeighbors{}, err
		}
		cellCount, err := c.uint8()
		if err != nil {
			return NASLTEGERANNeighbors{}, err
		}
		cells := make([]NASLTEGERANCell, int(cellCount))
		for cellIndex := range cells {
			arfcn, err := c.uint16()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			band1900, err := c.uint8()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			cellIDValid, err := c.uint8()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			bsic, err := c.uint8()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			rssi, err := c.int16()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			srxlev, err := c.int16()
			if err != nil {
				return NASLTEGERANNeighbors{}, err
			}
			cells[cellIndex] = NASLTEGERANCell{
				ARFCN: arfcn, Band1900: band1900 != 0, CellIDValid: cellIDValid != 0,
				BSIC: bsic, RSSI: rssi, SelectionRXLevel: srxlev,
			}
		}
		frequencies[index] = NASLTEGERANFrequency{
			CellReselectionPriority: priority, HighThreshold: high,
			LowThreshold: low, NCCPermitted: ncc, Cells: cells,
		}
	}
	if err := c.finish(); err != nil {
		return NASLTEGERANNeighbors{}, err
	}
	return NASLTEGERANNeighbors{UEInIdle: idle != 0, Frequencies: frequencies}, nil
}

func decodeNASLTEWCDMANeighbors(value []byte) (NASLTEWCDMANeighbors, error) {
	c := nasLocationCursor{value: value}
	idle, err := c.uint8()
	if err != nil {
		return NASLTEWCDMANeighbors{}, err
	}
	count, err := c.uint8()
	if err != nil {
		return NASLTEWCDMANeighbors{}, err
	}
	frequencies := make([]NASLTEWCDMAFrequency, int(count))
	for index := range frequencies {
		uarfcn, err := c.uint16()
		if err != nil {
			return NASLTEWCDMANeighbors{}, err
		}
		priority, err := c.uint8()
		if err != nil {
			return NASLTEWCDMANeighbors{}, err
		}
		high, err := c.uint16()
		if err != nil {
			return NASLTEWCDMANeighbors{}, err
		}
		low, err := c.uint16()
		if err != nil {
			return NASLTEWCDMANeighbors{}, err
		}
		cellCount, err := c.uint8()
		if err != nil {
			return NASLTEWCDMANeighbors{}, err
		}
		cells := make([]NASLTEWCDMACell, int(cellCount))
		for cellIndex := range cells {
			psc, err := c.uint16()
			if err != nil {
				return NASLTEWCDMANeighbors{}, err
			}
			rscp, err := c.int16()
			if err != nil {
				return NASLTEWCDMANeighbors{}, err
			}
			ecno, err := c.int16()
			if err != nil {
				return NASLTEWCDMANeighbors{}, err
			}
			srxlev, err := c.int16()
			if err != nil {
				return NASLTEWCDMANeighbors{}, err
			}
			cells[cellIndex] = NASLTEWCDMACell{PSC: psc, CPICHRSCP: rscp, CPICHECNO: ecno, SelectionRXLevel: srxlev}
		}
		frequencies[index] = NASLTEWCDMAFrequency{
			UARFCN: uarfcn, CellReselectionPriority: priority,
			HighThreshold: high, LowThreshold: low, Cells: cells,
		}
	}
	if err := c.finish(); err != nil {
		return NASLTEWCDMANeighbors{}, err
	}
	return NASLTEWCDMANeighbors{UEInIdle: idle != 0, Frequencies: frequencies}, nil
}

func decodeNASUMTSLTENeighbors(value []byte) (NASUMTSLTENeighbors, error) {
	c := nasLocationCursor{value: value}
	rrc, err := c.uint32()
	if err != nil {
		return NASUMTSLTENeighbors{}, err
	}
	count, err := c.uint8()
	if err != nil {
		return NASUMTSLTENeighbors{}, err
	}
	cells := make([]NASUMTSLTENeighbor, int(count))
	for index := range cells {
		earfcn, err := c.uint16()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		pci, err := c.uint16()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		rsrp, err := c.float32()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		rsrq, err := c.float32()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		srxlev, err := c.int16()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		tdd, err := c.uint8()
		if err != nil {
			return NASUMTSLTENeighbors{}, err
		}
		cells[index] = NASUMTSLTENeighbor{
			EARFCN: earfcn, PhysicalCellID: pci, RSRP: rsrp, RSRQ: rsrq,
			SelectionRXLevel: srxlev, TDD: tdd != 0,
		}
	}
	if err := c.finish(); err != nil {
		return NASUMTSLTENeighbors{}, err
	}
	return NASUMTSLTENeighbors{RRCState: NASWCDMARRCState(rrc), Cells: cells}, nil
}

func decodeNASNR5GLocation(value []byte) (NASNR5GCellLocation, error) {
	if len(value) != 22 {
		return NASNR5GCellLocation{}, fmt.Errorf("parsing QMI NAS NR5G cell information: TLV length %d, want 22", len(value))
	}
	plmn, plmnKnown, err := decodeNASLocationPLMN(value[:3])
	if err != nil {
		return NASNR5GCellLocation{}, fmt.Errorf("parsing QMI NAS NR5G cell PLMN: %w", err)
	}
	return NASNR5GCellLocation{
		PLMN: plmn, PLMNKnown: plmnKnown,
		TrackingAreaCode: [3]byte(value[3:6]),
		GlobalCellID:     binary.LittleEndian.Uint64(value[6:14]),
		PhysicalCellID:   binary.LittleEndian.Uint16(value[14:16]),
		RSRQ:             int16(binary.LittleEndian.Uint16(value[16:18])),
		RSRP:             int16(binary.LittleEndian.Uint16(value[18:20])),
		SNR:              int16(binary.LittleEndian.Uint16(value[20:22])),
	}, nil
}

func decodeNASLocationPLMN(value []byte) (NASPLMN, bool, error) {
	if len(value) != 3 {
		return NASPLMN{}, false, fmt.Errorf("BCD PLMN length %d, want 3", len(value))
	}
	if value[0] == 0xFF && value[1] == 0xFF && value[2] == 0xFF {
		return NASPLMN{}, false, nil
	}
	mcc1, mcc2, mcc3 := value[0]&0x0F, value[0]>>4, value[1]&0x0F
	mnc1, mnc2, mnc3 := value[2]&0x0F, value[2]>>4, value[1]>>4
	if mcc1 > 9 || mcc2 > 9 || mcc3 > 9 || mnc1 > 9 || mnc2 > 9 || (mnc3 > 9 && mnc3 != 0x0F) {
		return NASPLMN{}, false, fmt.Errorf("invalid BCD digits % X", value)
	}
	mnc := uint16(mnc1)*10 + uint16(mnc2)
	threeDigits := mnc3 != 0x0F
	if threeDigits {
		mnc = uint16(mnc1)*100 + uint16(mnc2)*10 + uint16(mnc3)
	}
	return NASPLMN{
		MCC: uint16(mcc1)*100 + uint16(mcc2)*10 + uint16(mcc3), MNC: mnc,
		MNCThreeDigits: threeDigits, MNCThreeDigitsKnown: true,
	}, true, nil
}

func decodeNASLocationUint32(value []byte) (uint32, error) {
	if len(value) != 4 {
		return 0, fmt.Errorf("TLV length %d, want 4", len(value))
	}
	return binary.LittleEndian.Uint32(value), nil
}

func decodeNASLocationUint32Array(value []byte) ([]uint32, error) {
	if len(value) == 0 {
		return nil, errors.New("count is missing")
	}
	count := int(value[0])
	want := 1 + count*4
	if len(value) != want {
		return nil, fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	values := make([]uint32, count)
	for index := range values {
		offset := 1 + index*4
		values[index] = binary.LittleEndian.Uint32(value[offset : offset+4])
	}
	return values, nil
}

type nasLocationCursor struct {
	value  []byte
	offset int
}

func (c *nasLocationCursor) take(length int) ([]byte, error) {
	if len(c.value)-c.offset < length {
		return nil, fmt.Errorf(
			"data truncated at byte %d: need %d, have %d",
			c.offset, length, len(c.value)-c.offset,
		)
	}
	value := c.value[c.offset : c.offset+length]
	c.offset += length
	return value, nil
}

func (c *nasLocationCursor) uint8() (uint8, error) {
	value, err := c.take(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}

func (c *nasLocationCursor) uint16() (uint16, error) {
	value, err := c.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(value), nil
}

func (c *nasLocationCursor) int16() (int16, error) {
	value, err := c.uint16()
	return int16(value), err
}

func (c *nasLocationCursor) uint32() (uint32, error) {
	value, err := c.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}

func (c *nasLocationCursor) float32() (float32, error) {
	value, err := c.uint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(value), nil
}

func (c *nasLocationCursor) finish() error {
	if c.offset != len(c.value) {
		return fmt.Errorf("%d trailing bytes", len(c.value)-c.offset)
	}
	return nil
}
