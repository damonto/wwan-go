package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

func TestBaseStationsRequest(t *testing.T) {
	maximum := BaseStationCounts{GSM: 1, UMTS: 2, TDSCDMA: 3, LTE: 4, CDMA: 5, NR: 6}
	tests := []struct {
		name    string
		version uint16
		want    []byte
	}{
		{
			name:    "legacy",
			version: mbimExVersion20,
			want:    baseStationCountBytesForTest(1, 2, 3, 4, 5),
		},
		{
			name:    "MBIMEx 3",
			version: mbimExVersion30,
			want:    baseStationCountBytesForTest(1, 2, 3, 4, 5, 6),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := (&BaseStationsRequest{
				TransactionID: 1,
				MBIMExVersion: tt.version,
				Maximum:       maximum,
			}).Request()
			command := request.Command.(*Command)
			if command.ServiceID != ServiceMsBasicConnectExtensions || command.CommandID != CIDMsBaseStationsInfo || command.CommandType != CommandTypeQuery {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("command data = %x, want %x", command.Data, tt.want)
			}
			if request.Timeout != mbimCIDResponseTimeout {
				t.Fatalf("timeout = %v, want %v", request.Timeout, mbimCIDResponseTimeout)
			}
		})
	}
}

func TestBaseStationsInfoUnmarshalBinary(t *testing.T) {
	legacyElements := baseStationLegacyElementsForTest()
	nrServing := baseStationRecordForTest(
		nrServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{16: 101, 20: 202, 24: 303, 28: 40, 32: 50, 36: 60},
		map[int]uint64{8: 0x123456789, 40: 700},
	)
	nrNeighbor := baseStationRecordForTest(
		nrNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 4, value: "00101"}},
		map[int]uint32{0: uint32(DataSubclass5GNR), 20: 11, 24: 22, 28: 33, 32: 44, 36: 55},
		nil,
	)
	nrNeighbor = appendRefValue(nrNeighbor, 12, utf16Bytes("1234567"))
	v3Elements := append(append([][]byte(nil), legacyElements...), baseStationArrayForTest(nrServing), baseStationArrayForTest(nrNeighbor))

	tests := []struct {
		name    string
		version uint16
		data    []byte
		check   func(*testing.T, BaseStationsInfo)
	}{
		{
			name:    "legacy cell families",
			version: mbimExVersion20,
			data:    baseStationsPayloadForTest(mbimExVersion20, DataClassLTE|DataClassUMTS, 0, legacyElements),
			check: func(t *testing.T, got BaseStationsInfo) {
				t.Helper()
				if got.SystemType != DataClassLTE|DataClassUMTS || got.SystemSubtype != DataSubclassNone {
					t.Fatalf("system = type %#x subtype %#x", got.SystemType, got.SystemSubtype)
				}
				if got.GSMServingCell == nil || got.GSMServingCell.ProviderID != "00101" || got.GSMServingCell.ARFCN != 4 {
					t.Fatalf("GSM serving cell = %+v", got.GSMServingCell)
				}
				if got.UMTSServingCell == nil || got.UMTSServingCell.RSCP != -10 || got.UMTSServingCell.EcNo != -20 {
					t.Fatalf("UMTS serving cell = %+v", got.UMTSServingCell)
				}
				if got.TDSCDMAServingCell == nil || got.TDSCDMAServingCell.CellParameterID != 4 {
					t.Fatalf("TDSCDMA serving cell = %+v", got.TDSCDMAServingCell)
				}
				if got.LTEServingCell == nil || got.LTEServingCell.PhysicalCellID != 3 || got.LTEServingCell.RSRQ != -12 {
					t.Fatalf("LTE serving cell = %+v", got.LTEServingCell)
				}
				if len(got.GSMNeighboringCells) != 1 || got.GSMNeighboringCells[0].RXLevel != 5 {
					t.Fatalf("GSM neighboring cells = %+v", got.GSMNeighboringCells)
				}
				if len(got.UMTSNeighboringCells) != 1 || got.UMTSNeighboringCells[0].PathLoss != 7 {
					t.Fatalf("UMTS neighboring cells = %+v", got.UMTSNeighboringCells)
				}
				if len(got.TDSCDMANeighboringCells) != 1 || got.TDSCDMANeighboringCells[0].TimingAdvance != 5 {
					t.Fatalf("TDSCDMA neighboring cells = %+v", got.TDSCDMANeighboringCells)
				}
				if len(got.LTENeighboringCells) != 1 || got.LTENeighboringCells[0].EARFCN != 2 {
					t.Fatalf("LTE neighboring cells = %+v", got.LTENeighboringCells)
				}
				if len(got.CDMACells) != 1 || got.CDMACells[0].ServingCellFlag != 1 || got.CDMACells[0].PilotStrength != 9 {
					t.Fatalf("CDMA cells = %+v", got.CDMACells)
				}
				if len(got.NRServingCells) != 0 || len(got.NRNeighboringCells) != 0 {
					t.Fatalf("legacy response contains NR cells: serving %+v neighboring %+v", got.NRServingCells, got.NRNeighboringCells)
				}
			},
		},
		{
			name:    "MBIMEx 3 NR cells",
			version: mbimExVersion30,
			data:    baseStationsPayloadForTest(mbimExVersion30, DataClass5G, DataSubclass5GENDC|DataSubclass5GNR, v3Elements),
			check: func(t *testing.T, got BaseStationsInfo) {
				t.Helper()
				if got.SystemType != DataClass5G || got.SystemSubtype != DataSubclass5GENDC|DataSubclass5GNR {
					t.Fatalf("system = type %#x subtype %#x", got.SystemType, got.SystemSubtype)
				}
				if len(got.NRServingCells) != 1 {
					t.Fatalf("NR serving cell count = %d, want 1", len(got.NRServingCells))
				}
				serving := got.NRServingCells[0]
				if serving.ProviderID != "00101" || serving.NCI != 0x123456789 || serving.NRARFCN != 202 || serving.TimingAdvance != 700 {
					t.Fatalf("NR serving cell = %+v", serving)
				}
				if len(got.NRNeighboringCells) != 1 {
					t.Fatalf("NR neighboring cell count = %d, want 1", len(got.NRNeighboringCells))
				}
				neighbor := got.NRNeighboringCells[0]
				if neighbor.SystemSubtype != DataSubclass5GNR || neighbor.ProviderID != "00101" || neighbor.CellID != "1234567" || neighbor.SINR != 55 {
					t.Fatalf("NR neighboring cell = %+v", neighbor)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BaseStationsInfo{MBIMExVersion: tt.version}
			if err := got.UnmarshalBinary(tt.data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestBaseStationsInfoRejectsMalformedPayloads(t *testing.T) {
	valid := baseStationsPayloadForTest(mbimExVersion20, DataClassLTE, 0, baseStationLegacyElementsForTest())
	validV3 := baseStationsPayloadForTest(mbimExVersion30, DataClass5G, DataSubclass5GNR, nil)
	gsmOffset := binary.LittleEndian.Uint32(valid[4:8])
	nonnumericProvider := baseStationRecordForTest(
		gsmServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00A01"}},
		nil,
		nil,
	)
	nrServing := baseStationRecordForTest(
		nrServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		nil,
		nil,
	)
	nrNeighbor := baseStationRecordForTest(
		nrNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 4, value: "00101"}},
		nil,
		nil,
	)
	nrNeighbor = appendRefValue(nrNeighbor, 12, utf16Bytes("1234567"))
	nrNeighborTooLong := baseStationRecordForTest(
		nrNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 4, value: "00101"}},
		nil,
		nil,
	)
	nrNeighborTooLong = appendRefValue(nrNeighborTooLong, 12, utf16Bytes("12345678"))
	nrServingRecords := make([][]byte, 33)
	for i := range nrServingRecords {
		nrServingRecords[i] = nrServing
	}
	nrNeighborRecords := make([][]byte, 9)
	for i := range nrNeighborRecords {
		nrNeighborRecords[i] = nrNeighbor
	}

	tests := []struct {
		name    string
		version uint16
		data    []byte
	}{
		{name: "legacy truncated header", version: mbimExVersion20, data: make([]byte, 75)},
		{name: "MBIMEx 3 truncated header", version: mbimExVersion30, data: make([]byte, 95)},
		{
			name:    "reserved system type",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], 1<<8)
			}),
		},
		{
			name:    "MBIMEx 3 reserved system subtype",
			version: mbimExVersion30,
			data: mutateBytes(validV3, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 1<<5)
			}),
		},
		{
			name:    "MBIMEx 3 subtype without 5G system type",
			version: mbimExVersion30,
			data:    baseStationsPayloadForTest(mbimExVersion30, DataClassLTE, DataSubclass5GNR, nil),
		},
		{
			name:    "nonnumeric provider ID",
			version: mbimExVersion20,
			data: baseStationsPayloadForTest(
				mbimExVersion20,
				DataClassGPRS,
				0,
				[][]byte{nonnumericProvider},
			),
		},
		{
			name:    "reference points into header",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 4)
			}),
		},
		{
			name:    "reference leaves sparse data buffer",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 80)
			}),
		},
		{
			name:    "reference has zero size",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], 0)
			}),
		},
		{
			name:    "provider string points into fixed fields",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[gsmOffset:gsmOffset+4], 8)
			}),
		},
		{
			name:    "provider string has odd length",
			version: mbimExVersion20,
			data: mutateBytes(valid, func(data []byte) {
				binary.LittleEndian.PutUint32(data[gsmOffset+4:gsmOffset+8], 1)
			}),
		},
		{
			name:    "neighbor array records truncated",
			version: mbimExVersion20,
			data: baseStationsPayloadForTest(
				mbimExVersion20,
				DataClassGPRS,
				0,
				[][]byte{nil, nil, nil, nil, binary.LittleEndian.AppendUint32(nil, 1)},
			),
		},
		{
			name:    "neighbor array trailing data",
			version: mbimExVersion20,
			data: baseStationsPayloadForTest(
				mbimExVersion20,
				DataClassGPRS,
				0,
				[][]byte{nil, nil, nil, nil, make([]byte, 8)},
			),
		},
		{
			name:    "NR neighbor cell ID too long",
			version: mbimExVersion30,
			data: baseStationsPayloadForTest(
				mbimExVersion30,
				DataClass5G,
				DataSubclass5GNR,
				[][]byte{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, baseStationArrayForTest(nrNeighborTooLong)},
			),
		},
		{
			name:    "too many NR serving cells",
			version: mbimExVersion30,
			data: baseStationsPayloadForTest(
				mbimExVersion30,
				DataClass5G,
				DataSubclass5GNR,
				[][]byte{nil, nil, nil, nil, nil, nil, nil, nil, nil, baseStationArrayForTest(nrServingRecords...)},
			),
		},
		{
			name:    "too many NR neighboring cells",
			version: mbimExVersion30,
			data: baseStationsPayloadForTest(
				mbimExVersion30,
				DataClass5G,
				DataSubclass5GNR,
				[][]byte{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, baseStationArrayForTest(nrNeighborRecords...)},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BaseStationsInfo{MBIMExVersion: tt.version}
			if err := got.UnmarshalBinary(tt.data); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestParseNRServingCellValidation(t *testing.T) {
	valid := baseStationRecordForTest(
		nrServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{16: 1007, 20: 3279165, 24: 1<<24 - 1, 28: 127, 32: 127, 36: 127},
		map[int]uint64{8: 1<<36 - 1},
	)
	unavailable := baseStationRecordForTest(
		nrServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{16: ^uint32(0), 20: ^uint32(0), 24: ^uint32(0), 28: ^uint32(0), 32: ^uint32(0), 36: ^uint32(0)},
		map[int]uint64{8: ^uint64(0)},
	)
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "maximum values", data: valid},
		{name: "unavailable values", data: unavailable},
		{
			name: "nonnumeric provider ID",
			data: baseStationRecordForTest(
				nrServingCellFixedSize,
				[]baseStationStringForTest{{fieldOffset: 0, value: "00A01"}},
				nil,
				nil,
			),
			wantErr: true,
		},
		{name: "NCI exceeds 36 bits", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint64(data[8:16], 1<<40) }), wantErr: true},
		{name: "physical cell ID exceeds 1007", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[16:20], 1008) }), wantErr: true},
		{name: "NRARFCN exceeds 3279165", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[20:24], 3279166) }), wantErr: true},
		{name: "TAC exceeds 24 bits", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[24:28], 1<<24) }), wantErr: true},
		{name: "RSRP exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[28:32], 128) }), wantErr: true},
		{name: "RSRQ exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[32:36], 128) }), wantErr: true},
		{name: "SINR exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[36:40], 128) }), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseNRServingCell(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseNRServingCell() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseNRNeighboringCellValidation(t *testing.T) {
	valid := baseStationRecordForTest(
		nrNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 4, value: "00101"}},
		map[int]uint32{0: uint32(DataSubclass5GNR), 20: 1007, 24: 1<<24 - 1, 28: 127, 32: 127, 36: 127},
		nil,
	)
	valid = appendRefValue(valid, 12, utf16Bytes("1234567"))
	unavailable := mutateBytes(valid, func(data []byte) {
		for _, offset := range []int{20, 24, 28, 32, 36} {
			binary.LittleEndian.PutUint32(data[offset:offset+4], ^uint32(0))
		}
	})
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "maximum values", data: valid},
		{name: "unavailable values", data: unavailable},
		{name: "reserved system subtype", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[0:4], 1<<5) }), wantErr: true},
		{
			name: "nonnumeric provider ID",
			data: baseStationRecordForTest(
				nrNeighboringCellFixedSize,
				[]baseStationStringForTest{{fieldOffset: 4, value: "00A01"}},
				map[int]uint32{0: uint32(DataSubclass5GNR)},
				nil,
			),
			wantErr: true,
		},
		{name: "physical cell ID exceeds 1007", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[20:24], 1008) }), wantErr: true},
		{name: "TAC exceeds 24 bits", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[24:28], 1<<24) }), wantErr: true},
		{name: "RSRP exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[28:32], 128) }), wantErr: true},
		{name: "RSRQ exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[32:36], 128) }), wantErr: true},
		{name: "SINR exceeds 127", data: mutateBytes(valid, func(data []byte) { binary.LittleEndian.PutUint32(data[36:40], 128) }), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseNRNeighboringCell(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseNRNeighboringCell() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseBaseStationValuesPadding(t *testing.T) {
	valid := baseStationRecordForTest(
		gsmServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "1"}},
		nil,
		nil,
	)
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "zero padding", data: valid},
		{
			name: "nonzero padding is ignored",
			data: mutateBytes(valid, func(data []byte) {
				data[len(data)-1] = 0xff
			}),
		},
		{name: "truncated padding", data: valid[:len(valid)-1], wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseBaseStationValues(tt.data, gsmServingCellFixedSize, []uint32{0}, []uint32{baseStationProviderIDMaximumSize})
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseBaseStationValues() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBaseStationsInfoValidatesNRCount(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		maximum BaseStationCounts
		wantErr bool
	}{
		{name: "legacy NR", version: mbimExVersion20, maximum: BaseStationCounts{NR: 1}, wantErr: true},
		{name: "NR count too large", version: mbimExVersion30, maximum: BaseStationCounts{NR: 41}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{mbimExVersion: tt.version}
			_, err := client.BaseStationsInfo(context.Background(), tt.maximum)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BaseStationsInfo() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type baseStationStringForTest struct {
	fieldOffset int
	value       string
}

func baseStationCountBytesForTest(values ...uint32) []byte {
	var data []byte
	for _, value := range values {
		data = binary.LittleEndian.AppendUint32(data, value)
	}
	return data
}

func baseStationSignedForTest(value int32) uint32 {
	return uint32(value)
}

func baseStationRecordForTest(fixedSize int, strings []baseStationStringForTest, values32 map[int]uint32, values64 map[int]uint64) []byte {
	data := make([]byte, fixedSize)
	for offset, value := range values32 {
		binary.LittleEndian.PutUint32(data[offset:offset+4], value)
	}
	for offset, value := range values64 {
		binary.LittleEndian.PutUint64(data[offset:offset+8], value)
	}
	for _, value := range strings {
		data = appendRefValue(data, value.fieldOffset, utf16Bytes(value.value))
	}
	return data
}

func baseStationArrayForTest(records ...[]byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(records)))
	for _, record := range records {
		data = append(data, record...)
	}
	return data
}

func baseStationsPayloadForTest(version uint16, systemType DataClass, systemSubtype DataSubclass, elements [][]byte) []byte {
	headerSize := 76
	refOffset := 4
	refCount := 9
	if version >= mbimExVersion30 {
		headerSize = 96
		refOffset = 8
		refCount = 11
	}
	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data[0:4], uint32(systemType))
	if version >= mbimExVersion30 {
		binary.LittleEndian.PutUint32(data[4:8], uint32(systemSubtype))
	}
	for index := 0; index < refCount && index < len(elements); index++ {
		data = appendRefValue(data, refOffset+index*8, elements[index])
	}
	return data
}

func baseStationLegacyElementsForTest() [][]byte {
	gsmServing := baseStationRecordForTest(
		gsmServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: 5, 28: 6},
		nil,
	)
	umtsServing := baseStationRecordForTest(
		umtsServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: 5, 28: 6, 32: 7, 36: baseStationSignedForTest(-10), 40: baseStationSignedForTest(-20), 44: 8},
		nil,
	)
	tdscdmaServing := baseStationRecordForTest(
		tdscdmaServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: 5, 28: baseStationSignedForTest(-6), 32: 7},
		nil,
	)
	lteServing := baseStationRecordForTest(
		lteServingCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: baseStationSignedForTest(-10), 28: baseStationSignedForTest(-12), 32: 7},
		nil,
	)
	gsmNeighbor := baseStationRecordForTest(
		gsmNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: 5},
		nil,
	)
	umtsNeighbor := baseStationRecordForTest(
		umtsNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: baseStationSignedForTest(-5), 28: baseStationSignedForTest(-6), 32: 7},
		nil,
	)
	tdscdmaNeighbor := baseStationRecordForTest(
		tdscdmaNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: 5, 28: baseStationSignedForTest(-6), 32: 7},
		nil,
	)
	lteNeighbor := baseStationRecordForTest(
		lteNeighboringCellFixedSize,
		[]baseStationStringForTest{{fieldOffset: 0, value: "00101"}},
		map[int]uint32{8: 1, 12: 2, 16: 3, 20: 4, 24: baseStationSignedForTest(-5), 28: baseStationSignedForTest(-6)},
		nil,
	)
	cdma := baseStationRecordForTest(
		cdmaCellFixedSize,
		nil,
		map[int]uint32{0: 1, 4: 2, 8: 3, 12: 4, 16: 5, 20: 6, 24: 7, 28: 8, 32: 9},
		nil,
	)
	return [][]byte{
		gsmServing,
		umtsServing,
		tdscdmaServing,
		lteServing,
		baseStationArrayForTest(gsmNeighbor),
		baseStationArrayForTest(umtsNeighbor),
		baseStationArrayForTest(tdscdmaNeighbor),
		baseStationArrayForTest(lteNeighbor),
		baseStationArrayForTest(cdma),
	}
}
