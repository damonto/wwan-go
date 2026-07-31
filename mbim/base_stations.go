package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type BaseStationCounts struct {
	GSM     uint32
	UMTS    uint32
	TDSCDMA uint32
	LTE     uint32
	CDMA    uint32
	NR      uint32
}

type BaseStationsRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Maximum       BaseStationCounts
	Response      *BaseStationsInfo
}

func (r *BaseStationsRequest) Request() *Request {
	r.Response = &BaseStationsInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsBaseStationsInfo,
			CommandTypeQuery,
			marshalBaseStationCounts(r.Maximum, r.MBIMExVersion),
		),
		Response: r.Response,
	}
}

func marshalBaseStationCounts(counts BaseStationCounts, version uint16) []byte {
	data := binary.LittleEndian.AppendUint32(nil, counts.GSM)
	data = binary.LittleEndian.AppendUint32(data, counts.UMTS)
	data = binary.LittleEndian.AppendUint32(data, counts.TDSCDMA)
	data = binary.LittleEndian.AppendUint32(data, counts.LTE)
	data = binary.LittleEndian.AppendUint32(data, counts.CDMA)
	if version >= mbimExVersion30 {
		data = binary.LittleEndian.AppendUint32(data, counts.NR)
	}
	return data
}

type BaseStationsInfo struct {
	MBIMExVersion uint16
	SystemType    DataClass
	SystemSubtype DataSubclass

	GSMServingCell     *GSMServingCell
	UMTSServingCell    *UMTSServingCell
	TDSCDMAServingCell *TDSCDMAServingCell
	LTEServingCell     *LTEServingCell

	GSMNeighboringCells     []GSMNeighboringCell
	UMTSNeighboringCells    []UMTSNeighboringCell
	TDSCDMANeighboringCells []TDSCDMANeighboringCell
	LTENeighboringCells     []LTENeighboringCell
	CDMACells               []CDMACell
	NRServingCells          []NRServingCell
	NRNeighboringCells      []NRNeighboringCell
}

func (i *BaseStationsInfo) UnmarshalBinary(data []byte) error {
	version := i.MBIMExVersion
	headerSize := uint32(76)
	refOffset := uint32(4)
	refCount := uint32(9)
	if version >= mbimExVersion30 {
		headerSize = 96
		refOffset = 8
		refCount = 11
	}
	if uint32(len(data)) < headerSize {
		return errors.New("parsing MBIM base stations info: payload is truncated")
	}

	refs, err := baseStationRefs(data, refOffset, refCount, headerSize)
	if err != nil {
		return fmt.Errorf("parsing MBIM base stations info: %w", err)
	}

	systemType := DataClass(binary.LittleEndian.Uint32(data[0:4]))
	if !validDataClass(version, systemType) {
		return fmt.Errorf("parsing MBIM base stations info: system type %#x contains bits reserved in MBIMEx %#x", systemType, version)
	}
	systemSubtype := DataSubclassNone
	if version >= mbimExVersion30 {
		systemSubtype = DataSubclass(binary.LittleEndian.Uint32(data[4:8]))
		if !validDataSubclass(systemSubtype) {
			return fmt.Errorf("parsing MBIM base stations info: system subtype %#x contains reserved bits", systemSubtype)
		}
		if !dataClassHas5G(version, systemType) && systemSubtype != DataSubclassNone {
			return fmt.Errorf("parsing MBIM base stations info: system subtype %#x requires a 5G system type", systemSubtype)
		}
	}
	result := BaseStationsInfo{
		MBIMExVersion: version,
		SystemType:    systemType,
		SystemSubtype: systemSubtype,
	}

	result.GSMServingCell, err = parseBaseStationStruct(data, refs[0], parseGSMServingCell)
	if err != nil {
		return fmt.Errorf("parsing GSM serving cell: %w", err)
	}
	result.UMTSServingCell, err = parseBaseStationStruct(data, refs[1], parseUMTSServingCell)
	if err != nil {
		return fmt.Errorf("parsing UMTS serving cell: %w", err)
	}
	result.TDSCDMAServingCell, err = parseBaseStationStruct(data, refs[2], parseTDSCDMAServingCell)
	if err != nil {
		return fmt.Errorf("parsing TDSCDMA serving cell: %w", err)
	}
	result.LTEServingCell, err = parseBaseStationStruct(data, refs[3], parseLTEServingCell)
	if err != nil {
		return fmt.Errorf("parsing LTE serving cell: %w", err)
	}

	result.GSMNeighboringCells, err = parseBaseStationArray(data, refs[4], gsmNeighboringCellFixedSize, parseGSMNeighboringCell)
	if err != nil {
		return fmt.Errorf("parsing GSM neighboring cells: %w", err)
	}
	result.UMTSNeighboringCells, err = parseBaseStationArray(data, refs[5], umtsNeighboringCellFixedSize, parseUMTSNeighboringCell)
	if err != nil {
		return fmt.Errorf("parsing UMTS neighboring cells: %w", err)
	}
	result.TDSCDMANeighboringCells, err = parseBaseStationArray(data, refs[6], tdscdmaNeighboringCellFixedSize, parseTDSCDMANeighboringCell)
	if err != nil {
		return fmt.Errorf("parsing TDSCDMA neighboring cells: %w", err)
	}
	result.LTENeighboringCells, err = parseBaseStationArray(data, refs[7], lteNeighboringCellFixedSize, parseLTENeighboringCell)
	if err != nil {
		return fmt.Errorf("parsing LTE neighboring cells: %w", err)
	}
	result.CDMACells, err = parseBaseStationArray(data, refs[8], cdmaCellFixedSize, parseCDMACell)
	if err != nil {
		return fmt.Errorf("parsing CDMA cells: %w", err)
	}
	if version >= mbimExVersion30 {
		result.NRServingCells, err = parseBaseStationArray(data, refs[9], nrServingCellFixedSize, parseNRServingCell)
		if err != nil {
			return fmt.Errorf("parsing NR serving cells: %w", err)
		}
		if len(result.NRServingCells) > 32 {
			return fmt.Errorf("parsing NR serving cells: count %d exceeds 32", len(result.NRServingCells))
		}
		result.NRNeighboringCells, err = parseBaseStationArray(data, refs[10], nrNeighboringCellFixedSize, parseNRNeighboringCell)
		if err != nil {
			return fmt.Errorf("parsing NR neighboring cells: %w", err)
		}
		if len(result.NRNeighboringCells) > 8 {
			return fmt.Errorf("parsing NR neighboring cells: count %d exceeds 8", len(result.NRNeighboringCells))
		}
	}

	*i = result
	return nil
}

func baseStationRefs(data []byte, fieldOffset, count, dataStart uint32) ([]valueRef, error) {
	refs := make([]valueRef, count)
	for index := range count {
		ref, err := readOffsetSizeRef(data, fieldOffset+index*8)
		if err != nil {
			return nil, fmt.Errorf("reference %d: %w", index, err)
		}
		if ref.offset != 0 && ref.size == 0 {
			return nil, fmt.Errorf("reference %d has nonzero offset with zero size", index)
		}
		refs[index] = ref
	}
	if err := validateDataBufferRefs(data, dataStart, refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func parseBaseStationStruct[T any](data []byte, ref valueRef, parse func([]byte) (T, int, error)) (*T, error) {
	if ref.offset == 0 {
		return nil, nil
	}
	raw := data[ref.offset : ref.offset+ref.size]
	value, consumed, err := parse(raw)
	if err != nil {
		return nil, err
	}
	if consumed != len(raw) {
		return nil, fmt.Errorf("structure consumed %d bytes, want %d", consumed, len(raw))
	}
	return &value, nil
}

func parseBaseStationArray[T any](data []byte, ref valueRef, minimumSize int, parse func([]byte) (T, int, error)) ([]T, error) {
	if ref.offset == 0 {
		return nil, nil
	}
	raw := data[ref.offset : ref.offset+ref.size]
	if len(raw) < 4 {
		return nil, errors.New("array header is truncated")
	}
	count := binary.LittleEndian.Uint32(raw[:4])
	if count > uint32((len(raw)-4)/minimumSize) {
		return nil, errors.New("array records are truncated")
	}

	values := make([]T, count)
	offset := 4
	for index := range count {
		value, consumed, err := parse(raw[offset:])
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", index, err)
		}
		if consumed <= 0 || consumed > len(raw)-offset {
			return nil, fmt.Errorf("record %d has invalid encoded size %d", index, consumed)
		}
		values[index] = value
		offset += consumed
	}
	if offset != len(raw) {
		return nil, errors.New("array has trailing data")
	}
	return values, nil
}

const (
	gsmServingCellFixedSize          = 32
	umtsServingCellFixedSize         = 48
	tdscdmaServingCellFixedSize      = 36
	lteServingCellFixedSize          = 36
	gsmNeighboringCellFixedSize      = 28
	umtsNeighboringCellFixedSize     = 36
	tdscdmaNeighboringCellFixedSize  = 36
	lteNeighboringCellFixedSize      = 32
	cdmaCellFixedSize                = 36
	nrServingCellFixedSize           = 48
	nrNeighboringCellFixedSize       = 40
	baseStationProviderIDMaximumSize = 12
	nrNeighborCellIDMaximumSize      = 14
)

func parseBaseStationValues(data []byte, fixedSize int, fields []uint32, maximumSizes []uint32) ([][]byte, int, error) {
	if len(data) < fixedSize {
		return nil, 0, errors.New("record is truncated")
	}
	if len(fields) != len(maximumSizes) {
		return nil, 0, errors.New("string field metadata is inconsistent")
	}

	values := make([][]byte, len(fields))
	end := fixedSize
	for index, field := range fields {
		ref, err := readOffsetSizeRef(data, field)
		if err != nil {
			return nil, 0, fmt.Errorf("string %d reference: %w", index, err)
		}
		if err := ref.validate(data); err != nil {
			return nil, 0, fmt.Errorf("string %d: %w", index, err)
		}
		if ref.offset == 0 {
			continue
		}
		if ref.size == 0 {
			return nil, 0, fmt.Errorf("string %d has nonzero offset with zero size", index)
		}
		if maximumSizes[index] != 0 && ref.size > maximumSizes[index] {
			return nil, 0, fmt.Errorf("string %d exceeds %d bytes", index, maximumSizes[index])
		}
		if ref.offset != uint32(end) {
			return nil, 0, fmt.Errorf("string %d offset is %d, want %d", index, ref.offset, end)
		}
		values[index] = ref.bytes(data)
		valueEnd := int(ref.offset + ref.size)
		end = align4(valueEnd)
		if end > len(data) {
			return nil, 0, fmt.Errorf("string %d padding is truncated", index)
		}
	}
	return values, end, nil
}

func parseBaseStationStrings(data []byte, fixedSize int, fields []uint32, maximumSizes []uint32) ([]string, int, error) {
	rawValues, consumed, err := parseBaseStationValues(data, fixedSize, fields, maximumSizes)
	if err != nil {
		return nil, 0, err
	}
	values := make([]string, len(rawValues))
	for index, raw := range rawValues {
		value, err := utf16RawString(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("string %d: %w", index, err)
		}
		values[index] = value
	}
	return values, consumed, nil
}

func parseBaseStationProviderID(data []byte, fixedSize int) (string, int, error) {
	strings, consumed, err := parseBaseStationStrings(
		data,
		fixedSize,
		[]uint32{0},
		[]uint32{baseStationProviderIDMaximumSize},
	)
	if err != nil {
		return "", 0, err
	}
	if err := validateProviderID(strings[0]); err != nil {
		return "", 0, fmt.Errorf("provider ID: %w", err)
	}
	return strings[0], consumed, nil
}

type GSMServingCell struct {
	ProviderID       string
	LocationAreaCode uint32
	CellID           uint32
	TimingAdvance    uint32
	ARFCN            uint32
	BaseStationID    uint32
	RXLevel          uint32
}

func parseGSMServingCell(data []byte) (GSMServingCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, gsmServingCellFixedSize)
	if err != nil {
		return GSMServingCell{}, 0, err
	}
	return GSMServingCell{
		ProviderID:       providerID,
		LocationAreaCode: binary.LittleEndian.Uint32(data[8:12]),
		CellID:           binary.LittleEndian.Uint32(data[12:16]),
		TimingAdvance:    binary.LittleEndian.Uint32(data[16:20]),
		ARFCN:            binary.LittleEndian.Uint32(data[20:24]),
		BaseStationID:    binary.LittleEndian.Uint32(data[24:28]),
		RXLevel:          binary.LittleEndian.Uint32(data[28:32]),
	}, consumed, nil
}

type UMTSServingCell struct {
	ProviderID            string
	LocationAreaCode      uint32
	CellID                uint32
	FrequencyInfoUL       uint32
	FrequencyInfoDL       uint32
	FrequencyInfoNT       uint32
	UARFCN                uint32
	PrimaryScramblingCode uint32
	RSCP                  int32
	EcNo                  int32
	PathLoss              uint32
}

func parseUMTSServingCell(data []byte) (UMTSServingCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, umtsServingCellFixedSize)
	if err != nil {
		return UMTSServingCell{}, 0, err
	}
	return UMTSServingCell{
		ProviderID:            providerID,
		LocationAreaCode:      binary.LittleEndian.Uint32(data[8:12]),
		CellID:                binary.LittleEndian.Uint32(data[12:16]),
		FrequencyInfoUL:       binary.LittleEndian.Uint32(data[16:20]),
		FrequencyInfoDL:       binary.LittleEndian.Uint32(data[20:24]),
		FrequencyInfoNT:       binary.LittleEndian.Uint32(data[24:28]),
		UARFCN:                binary.LittleEndian.Uint32(data[28:32]),
		PrimaryScramblingCode: binary.LittleEndian.Uint32(data[32:36]),
		RSCP:                  int32(binary.LittleEndian.Uint32(data[36:40])),
		EcNo:                  int32(binary.LittleEndian.Uint32(data[40:44])),
		PathLoss:              binary.LittleEndian.Uint32(data[44:48]),
	}, consumed, nil
}

type TDSCDMAServingCell struct {
	ProviderID       string
	LocationAreaCode uint32
	CellID           uint32
	UARFCN           uint32
	CellParameterID  uint32
	TimingAdvance    uint32
	RSCP             int32
	PathLoss         uint32
}

func parseTDSCDMAServingCell(data []byte) (TDSCDMAServingCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, tdscdmaServingCellFixedSize)
	if err != nil {
		return TDSCDMAServingCell{}, 0, err
	}
	return TDSCDMAServingCell{
		ProviderID:       providerID,
		LocationAreaCode: binary.LittleEndian.Uint32(data[8:12]),
		CellID:           binary.LittleEndian.Uint32(data[12:16]),
		UARFCN:           binary.LittleEndian.Uint32(data[16:20]),
		CellParameterID:  binary.LittleEndian.Uint32(data[20:24]),
		TimingAdvance:    binary.LittleEndian.Uint32(data[24:28]),
		RSCP:             int32(binary.LittleEndian.Uint32(data[28:32])),
		PathLoss:         binary.LittleEndian.Uint32(data[32:36]),
	}, consumed, nil
}

type LTEServingCell struct {
	ProviderID     string
	CellID         uint32
	EARFCN         uint32
	PhysicalCellID uint32
	TAC            uint32
	RSRP           int32
	RSRQ           int32
	TimingAdvance  uint32
}

func parseLTEServingCell(data []byte) (LTEServingCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, lteServingCellFixedSize)
	if err != nil {
		return LTEServingCell{}, 0, err
	}
	return LTEServingCell{
		ProviderID:     providerID,
		CellID:         binary.LittleEndian.Uint32(data[8:12]),
		EARFCN:         binary.LittleEndian.Uint32(data[12:16]),
		PhysicalCellID: binary.LittleEndian.Uint32(data[16:20]),
		TAC:            binary.LittleEndian.Uint32(data[20:24]),
		RSRP:           int32(binary.LittleEndian.Uint32(data[24:28])),
		RSRQ:           int32(binary.LittleEndian.Uint32(data[28:32])),
		TimingAdvance:  binary.LittleEndian.Uint32(data[32:36]),
	}, consumed, nil
}

type GSMNeighboringCell struct {
	ProviderID       string
	LocationAreaCode uint32
	CellID           uint32
	ARFCN            uint32
	BaseStationID    uint32
	RXLevel          uint32
}

func parseGSMNeighboringCell(data []byte) (GSMNeighboringCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, gsmNeighboringCellFixedSize)
	if err != nil {
		return GSMNeighboringCell{}, 0, err
	}
	return GSMNeighboringCell{
		ProviderID:       providerID,
		LocationAreaCode: binary.LittleEndian.Uint32(data[8:12]),
		CellID:           binary.LittleEndian.Uint32(data[12:16]),
		ARFCN:            binary.LittleEndian.Uint32(data[16:20]),
		BaseStationID:    binary.LittleEndian.Uint32(data[20:24]),
		RXLevel:          binary.LittleEndian.Uint32(data[24:28]),
	}, consumed, nil
}

type UMTSNeighboringCell struct {
	ProviderID            string
	LocationAreaCode      uint32
	CellID                uint32
	UARFCN                uint32
	PrimaryScramblingCode uint32
	RSCP                  int32
	EcNo                  int32
	PathLoss              uint32
}

func parseUMTSNeighboringCell(data []byte) (UMTSNeighboringCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, umtsNeighboringCellFixedSize)
	if err != nil {
		return UMTSNeighboringCell{}, 0, err
	}
	return UMTSNeighboringCell{
		ProviderID:            providerID,
		LocationAreaCode:      binary.LittleEndian.Uint32(data[8:12]),
		CellID:                binary.LittleEndian.Uint32(data[12:16]),
		UARFCN:                binary.LittleEndian.Uint32(data[16:20]),
		PrimaryScramblingCode: binary.LittleEndian.Uint32(data[20:24]),
		RSCP:                  int32(binary.LittleEndian.Uint32(data[24:28])),
		EcNo:                  int32(binary.LittleEndian.Uint32(data[28:32])),
		PathLoss:              binary.LittleEndian.Uint32(data[32:36]),
	}, consumed, nil
}

type TDSCDMANeighboringCell struct {
	ProviderID       string
	LocationAreaCode uint32
	CellID           uint32
	UARFCN           uint32
	CellParameterID  uint32
	TimingAdvance    uint32
	RSCP             int32
	PathLoss         uint32
}

func parseTDSCDMANeighboringCell(data []byte) (TDSCDMANeighboringCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, tdscdmaNeighboringCellFixedSize)
	if err != nil {
		return TDSCDMANeighboringCell{}, 0, err
	}
	return TDSCDMANeighboringCell{
		ProviderID:       providerID,
		LocationAreaCode: binary.LittleEndian.Uint32(data[8:12]),
		CellID:           binary.LittleEndian.Uint32(data[12:16]),
		UARFCN:           binary.LittleEndian.Uint32(data[16:20]),
		CellParameterID:  binary.LittleEndian.Uint32(data[20:24]),
		TimingAdvance:    binary.LittleEndian.Uint32(data[24:28]),
		RSCP:             int32(binary.LittleEndian.Uint32(data[28:32])),
		PathLoss:         binary.LittleEndian.Uint32(data[32:36]),
	}, consumed, nil
}

type LTENeighboringCell struct {
	ProviderID     string
	CellID         uint32
	EARFCN         uint32
	PhysicalCellID uint32
	TAC            uint32
	RSRP           int32
	RSRQ           int32
}

func parseLTENeighboringCell(data []byte) (LTENeighboringCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, lteNeighboringCellFixedSize)
	if err != nil {
		return LTENeighboringCell{}, 0, err
	}
	return LTENeighboringCell{
		ProviderID:     providerID,
		CellID:         binary.LittleEndian.Uint32(data[8:12]),
		EARFCN:         binary.LittleEndian.Uint32(data[12:16]),
		PhysicalCellID: binary.LittleEndian.Uint32(data[16:20]),
		TAC:            binary.LittleEndian.Uint32(data[20:24]),
		RSRP:           int32(binary.LittleEndian.Uint32(data[24:28])),
		RSRQ:           int32(binary.LittleEndian.Uint32(data[28:32])),
	}, consumed, nil
}

type CDMACell struct {
	ServingCellFlag uint32
	NID             uint32
	SID             uint32
	BaseStationID   uint32
	BaseLatitude    uint32
	BaseLongitude   uint32
	RefPN           uint32
	GPSSeconds      uint32
	PilotStrength   uint32
}

func parseCDMACell(data []byte) (CDMACell, int, error) {
	if len(data) < cdmaCellFixedSize {
		return CDMACell{}, 0, errors.New("record is truncated")
	}
	return CDMACell{
		ServingCellFlag: binary.LittleEndian.Uint32(data[0:4]),
		NID:             binary.LittleEndian.Uint32(data[4:8]),
		SID:             binary.LittleEndian.Uint32(data[8:12]),
		BaseStationID:   binary.LittleEndian.Uint32(data[12:16]),
		BaseLatitude:    binary.LittleEndian.Uint32(data[16:20]),
		BaseLongitude:   binary.LittleEndian.Uint32(data[20:24]),
		RefPN:           binary.LittleEndian.Uint32(data[24:28]),
		GPSSeconds:      binary.LittleEndian.Uint32(data[28:32]),
		PilotStrength:   binary.LittleEndian.Uint32(data[32:36]),
	}, cdmaCellFixedSize, nil
}

type NRServingCell struct {
	ProviderID     string
	NCI            uint64
	PhysicalCellID uint32
	NRARFCN        uint32
	TAC            uint32
	RSRP           uint32
	RSRQ           uint32
	SINR           uint32
	TimingAdvance  uint64
}

func parseNRServingCell(data []byte) (NRServingCell, int, error) {
	providerID, consumed, err := parseBaseStationProviderID(data, nrServingCellFixedSize)
	if err != nil {
		return NRServingCell{}, 0, err
	}
	nci := binary.LittleEndian.Uint64(data[8:16])
	if nci >= 1<<36 && nci != ^uint64(0) {
		return NRServingCell{}, 0, fmt.Errorf("NCI %#x exceeds 36 bits", nci)
	}
	physicalCellID := binary.LittleEndian.Uint32(data[16:20])
	if err := validateNRCellValue("physical cell ID", physicalCellID, 1007); err != nil {
		return NRServingCell{}, 0, err
	}
	nrarfcn := binary.LittleEndian.Uint32(data[20:24])
	if err := validateNRCellValue("NRARFCN", nrarfcn, 3279165); err != nil {
		return NRServingCell{}, 0, err
	}
	tac := binary.LittleEndian.Uint32(data[24:28])
	if err := validateNRCellValue("TAC", tac, 1<<24-1); err != nil {
		return NRServingCell{}, 0, err
	}
	rsrp := binary.LittleEndian.Uint32(data[28:32])
	if err := validateNRCellValue("RSRP", rsrp, 127); err != nil {
		return NRServingCell{}, 0, err
	}
	rsrq := binary.LittleEndian.Uint32(data[32:36])
	if err := validateNRCellValue("RSRQ", rsrq, 127); err != nil {
		return NRServingCell{}, 0, err
	}
	sinr := binary.LittleEndian.Uint32(data[36:40])
	if err := validateNRCellValue("SINR", sinr, 127); err != nil {
		return NRServingCell{}, 0, err
	}
	return NRServingCell{
		ProviderID:     providerID,
		NCI:            nci,
		PhysicalCellID: physicalCellID,
		NRARFCN:        nrarfcn,
		TAC:            tac,
		RSRP:           rsrp,
		RSRQ:           rsrq,
		SINR:           sinr,
		TimingAdvance:  binary.LittleEndian.Uint64(data[40:48]),
	}, consumed, nil
}

type NRNeighboringCell struct {
	SystemSubtype  DataSubclass
	ProviderID     string
	CellID         string
	PhysicalCellID uint32
	TAC            uint32
	RSRP           uint32
	RSRQ           uint32
	SINR           uint32
}

func parseNRNeighboringCell(data []byte) (NRNeighboringCell, int, error) {
	values, consumed, err := parseBaseStationValues(
		data,
		nrNeighboringCellFixedSize,
		[]uint32{4, 12},
		[]uint32{baseStationProviderIDMaximumSize, nrNeighborCellIDMaximumSize},
	)
	if err != nil {
		return NRNeighboringCell{}, 0, err
	}
	providerID, err := utf16RawString(values[0])
	if err != nil {
		return NRNeighboringCell{}, 0, fmt.Errorf("provider ID: %w", err)
	}
	if err := validateProviderID(providerID); err != nil {
		return NRNeighboringCell{}, 0, fmt.Errorf("provider ID: %w", err)
	}
	cellID, err := utf16RawString(values[1])
	if err != nil {
		return NRNeighboringCell{}, 0, fmt.Errorf("cell ID: %w", err)
	}
	systemSubtype := DataSubclass(binary.LittleEndian.Uint32(data[0:4]))
	if !validDataSubclass(systemSubtype) {
		return NRNeighboringCell{}, 0, fmt.Errorf("system subtype %#x contains reserved bits", systemSubtype)
	}
	physicalCellID := binary.LittleEndian.Uint32(data[20:24])
	if err := validateNRCellValue("physical cell ID", physicalCellID, 1007); err != nil {
		return NRNeighboringCell{}, 0, err
	}
	tac := binary.LittleEndian.Uint32(data[24:28])
	if err := validateNRCellValue("TAC", tac, 1<<24-1); err != nil {
		return NRNeighboringCell{}, 0, err
	}
	rsrp := binary.LittleEndian.Uint32(data[28:32])
	if err := validateNRCellValue("RSRP", rsrp, 127); err != nil {
		return NRNeighboringCell{}, 0, err
	}
	rsrq := binary.LittleEndian.Uint32(data[32:36])
	if err := validateNRCellValue("RSRQ", rsrq, 127); err != nil {
		return NRNeighboringCell{}, 0, err
	}
	sinr := binary.LittleEndian.Uint32(data[36:40])
	if err := validateNRCellValue("SINR", sinr, 127); err != nil {
		return NRNeighboringCell{}, 0, err
	}
	return NRNeighboringCell{
		SystemSubtype:  systemSubtype,
		ProviderID:     providerID,
		CellID:         cellID,
		PhysicalCellID: physicalCellID,
		TAC:            tac,
		RSRP:           rsrp,
		RSRQ:           rsrq,
		SINR:           sinr,
	}, consumed, nil
}

func validateNRCellValue(name string, value, maximum uint32) error {
	if value > maximum && value != ^uint32(0) {
		return fmt.Errorf("%s %d exceeds %d and is not the unavailable value", name, value, maximum)
	}
	return nil
}

func (c *Client) BaseStationsInfo(ctx context.Context, maximum BaseStationCounts) (BaseStationsInfo, error) {
	if c.mbimExVersion < mbimExVersion30 && maximum.NR != 0 {
		return BaseStationsInfo{}, errors.New("reading MBIM base stations info: NR count requires MBIMEx 3.0")
	}
	if maximum.NR > 40 {
		return BaseStationsInfo{}, fmt.Errorf("reading MBIM base stations info: NR count %d exceeds 40", maximum.NR)
	}
	request := BaseStationsRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Maximum:       maximum,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return BaseStationsInfo{}, fmt.Errorf("reading MBIM base stations info: %w", err)
	}
	return *request.Response, nil
}
