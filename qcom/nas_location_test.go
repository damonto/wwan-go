package qcom

import (
	"context"
	"encoding/binary"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDecodeNASLocationPLMN(t *testing.T) {
	tests := []struct {
		name      string
		value     []byte
		want      NASPLMN
		wantKnown bool
		wantErr   bool
	}{
		{
			name: "two-digit MNC", value: []byte{0x64, 0xF0, 0x10},
			want: NASPLMN{MCC: 460, MNC: 1, MNCThreeDigitsKnown: true}, wantKnown: true,
		},
		{
			name: "three-digit MNC", value: []byte{0x13, 0x00, 0x62},
			want: NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, wantKnown: true,
		},
		{name: "unknown", value: []byte{0xFF, 0xFF, 0xFF}},
		{name: "invalid MCC digit", value: []byte{0x6A, 0xF0, 0x10}, wantErr: true},
		{name: "invalid MNC digit", value: []byte{0x64, 0xA0, 0x10}, wantErr: true},
		{name: "truncated", value: []byte{0x64, 0xF0}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known, err := decodeNASLocationPLMN(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("decodeNASLocationPLMN() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeNASLocationPLMN() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) || known != tt.wantKnown {
				t.Fatalf("decodeNASLocationPLMN() = (%+v, %t), want (%+v, %t)", got, known, tt.want, tt.wantKnown)
			}
		})
	}
}

func TestNASCellLocationInfoUnmarshalTLVs(t *testing.T) {
	geran := binary.LittleEndian.AppendUint32(nil, 0x01020304)
	geran = append(geran, 0x64, 0xF0, 0x10)
	geran = binary.LittleEndian.AppendUint16(geran, 0x1122)
	geran = binary.LittleEndian.AppendUint16(geran, 512)
	geran = append(geran, 0x2A)
	geran = binary.LittleEndian.AppendUint32(geran, 7)
	geran = binary.LittleEndian.AppendUint16(geran, 55)
	geran = append(geran, 1)
	geran = binary.LittleEndian.AppendUint32(geran, 0xAABBCCDD)
	geran = append(geran, 0x13, 0x00, 0x62)
	geran = binary.LittleEndian.AppendUint16(geran, 0x3344)
	geran = binary.LittleEndian.AppendUint16(geran, 600)
	geran = append(geran, 0x1B)
	geran = binary.LittleEndian.AppendUint16(geran, 44)

	umts := binary.LittleEndian.AppendUint16(nil, 0x1234)
	umts = append(umts, 0x13, 0x00, 0x62)
	umts = binary.LittleEndian.AppendUint16(umts, 0x5678)
	umts = binary.LittleEndian.AppendUint16(umts, 10688)
	umts = binary.LittleEndian.AppendUint16(umts, 321)
	umts = appendLocationInt16Test(umts, -85)
	umts = appendLocationInt16Test(umts, -12)
	umts = append(umts, 1)
	umts = binary.LittleEndian.AppendUint16(umts, 10712)
	umts = binary.LittleEndian.AppendUint16(umts, 222)
	umts = appendLocationInt16Test(umts, -91)
	umts = appendLocationInt16Test(umts, -16)
	umts = append(umts, 1)
	umts = binary.LittleEndian.AppendUint16(umts, 700)
	umts = append(umts, 3, 4)
	umts = appendLocationInt16Test(umts, -77)

	cdma := binary.LittleEndian.AppendUint16(nil, 42)
	cdma = binary.LittleEndian.AppendUint16(cdma, 43)
	cdma = binary.LittleEndian.AppendUint16(cdma, 44)
	cdma = binary.LittleEndian.AppendUint16(cdma, 45)
	cdma = binary.LittleEndian.AppendUint32(cdma, 46)
	cdma = binary.LittleEndian.AppendUint32(cdma, 47)

	lteIntra := []byte{1, 0x13, 0x00, 0x62}
	lteIntra = binary.LittleEndian.AppendUint16(lteIntra, 0x1020)
	lteIntra = binary.LittleEndian.AppendUint32(lteIntra, 0x01234567)
	lteIntra = binary.LittleEndian.AppendUint16(lteIntra, 1800)
	lteIntra = binary.LittleEndian.AppendUint16(lteIntra, 123)
	lteIntra = append(lteIntra, 7, 8, 9, 10, 1)
	lteIntra = appendLTECellTest(lteIntra, 123, -120, -950, -700, 20)

	lteInter := []byte{1, 1}
	lteInter = binary.LittleEndian.AppendUint16(lteInter, 2850)
	lteInter = append(lteInter, 5, 15, 6, 1)
	lteInter = appendLTECellTest(lteInter, 222, -130, -1020, -800, 15)

	lteGERAN := []byte{1, 1, 6, 20, 10, 0xAA, 1}
	lteGERAN = binary.LittleEndian.AppendUint16(lteGERAN, 750)
	lteGERAN = append(lteGERAN, 1, 1, 0x2C)
	lteGERAN = appendLocationInt16Test(lteGERAN, -820)
	lteGERAN = appendLocationInt16Test(lteGERAN, 12)

	lteWCDMA := []byte{1, 1}
	lteWCDMA = binary.LittleEndian.AppendUint16(lteWCDMA, 10688)
	lteWCDMA = append(lteWCDMA, 5)
	lteWCDMA = binary.LittleEndian.AppendUint16(lteWCDMA, 20)
	lteWCDMA = binary.LittleEndian.AppendUint16(lteWCDMA, 10)
	lteWCDMA = append(lteWCDMA, 1)
	lteWCDMA = binary.LittleEndian.AppendUint16(lteWCDMA, 301)
	lteWCDMA = appendLocationInt16Test(lteWCDMA, -920)
	lteWCDMA = appendLocationInt16Test(lteWCDMA, -75)
	lteWCDMA = appendLocationInt16Test(lteWCDMA, 8)

	umtsLTE := binary.LittleEndian.AppendUint32(nil, uint32(NASWCDMARRCCellDCH))
	umtsLTE = append(umtsLTE, 1)
	umtsLTE = binary.LittleEndian.AppendUint16(umtsLTE, 1650)
	umtsLTE = binary.LittleEndian.AppendUint16(umtsLTE, 321)
	umtsLTE = binary.LittleEndian.AppendUint32(umtsLTE, math.Float32bits(-95.5))
	umtsLTE = binary.LittleEndian.AppendUint32(umtsLTE, math.Float32bits(-12.25))
	umtsLTE = appendLocationInt16Test(umtsLTE, 9)
	umtsLTE = append(umtsLTE, 1)

	nr5g := []byte{0x13, 0x00, 0x62, 0x11, 0x22, 0x33}
	nr5g = binary.LittleEndian.AppendUint64(nr5g, 0x0102030405060708)
	nr5g = binary.LittleEndian.AppendUint16(nr5g, 501)
	nr5g = appendLocationInt16Test(nr5g, -125)
	nr5g = appendLocationInt16Test(nr5g, -970)
	nr5g = appendLocationInt16Test(nr5g, 135)

	tests := []struct {
		name  string
		tlvs  tlv.TLVs
		check func(*testing.T, NASCellLocationInfo)
	}{
		{
			name: "GERAN",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationGERAN, geran)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				want := NASGERANCellLocation{
					CellID: 0x01020304, CellIDKnown: true,
					PLMN: NASPLMN{MCC: 460, MNC: 1, MNCThreeDigitsKnown: true}, PLMNKnown: true,
					LAC: 0x1122, ARFCN: 512, BSIC: 0x2A,
					TimingAdvance: 7, TimingAdvanceKnown: true, RXLevel: 55,
					Neighbors: []NASGERANNeighborCell{{
						CellID: 0xAABBCCDD, CellIDKnown: true,
						PLMN:      NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true},
						PLMNKnown: true, LAC: 0x3344, ARFCN: 600, BSIC: 0x1B, RXLevel: 44,
					}},
				}
				if !got.GERANKnown || !reflect.DeepEqual(got.GERAN, want) {
					t.Fatalf("GERAN = %+v, known %t, want %+v", got.GERAN, got.GERANKnown, want)
				}
			},
		},
		{
			name: "UMTS",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationUMTS, umts)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				if !got.UMTSKnown || got.UMTS.CellID != 0x1234 || got.UMTS.PLMN.MNC != 260 ||
					!reflect.DeepEqual(got.UMTS.Neighbors, []NASUMTSNeighborCell{{10712, 222, -91, -16}}) ||
					!reflect.DeepEqual(got.UMTS.GERANNeighbors, []NASUMTSGERANNeighborCell{{700, 3, 4, -77}}) {
					t.Fatalf("UMTS = %+v, known %t", got.UMTS, got.UMTSKnown)
				}
			},
		},
		{
			name: "CDMA",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationCDMA, cdma)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				want := NASCDMACellLocation{42, 43, 44, 45, 46, 47}
				if !got.CDMAKnown || got.CDMA != want {
					t.Fatalf("CDMA = %+v, known %t, want %+v", got.CDMA, got.CDMAKnown, want)
				}
			},
		},
		{
			name: "LTE intrafrequency",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationLTEIntra, lteIntra)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				if !got.LTEIntraKnown || !got.LTEIntra.UEInIdle || got.LTEIntra.PLMN.MNC != 260 ||
					got.LTEIntra.GlobalCellID != 0x01234567 || got.LTEIntra.EARFCN != 1800 ||
					!reflect.DeepEqual(got.LTEIntra.Cells, []NASLTECellMeasurement{{123, -120, -950, -700, 20}}) {
					t.Fatalf("LTEIntra = %+v, known %t", got.LTEIntra, got.LTEIntraKnown)
				}
			},
		},
		{
			name: "LTE interfrequency",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationLTEInter, lteInter)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				want := []NASLTEInterFrequencyEntry{{
					EARFCN: 2850, LowThreshold: 5, HighThreshold: 15, CellReselectionPriority: 6,
					Cells: []NASLTECellMeasurement{{222, -130, -1020, -800, 15}},
				}}
				if !got.LTEInterKnown || !got.LTEInter.UEInIdle || !reflect.DeepEqual(got.LTEInter.Frequencies, want) {
					t.Fatalf("LTEInter = %+v, known %t", got.LTEInter, got.LTEInterKnown)
				}
			},
		},
		{
			name: "LTE GERAN neighbors",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationLTEGERAN, lteGERAN)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				if !got.LTEGERANKnown || len(got.LTEGERAN.Frequencies) != 1 ||
					!reflect.DeepEqual(got.LTEGERAN.Frequencies[0].Cells, []NASLTEGERANCell{{750, true, true, 0x2C, -820, 12}}) {
					t.Fatalf("LTEGERAN = %+v, known %t", got.LTEGERAN, got.LTEGERANKnown)
				}
			},
		},
		{
			name: "LTE WCDMA neighbors",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationLTEWCDMA, lteWCDMA)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				if !got.LTEWCDMAKnown || len(got.LTEWCDMA.Frequencies) != 1 ||
					!reflect.DeepEqual(got.LTEWCDMA.Frequencies[0].Cells, []NASLTEWCDMACell{{301, -920, -75, 8}}) {
					t.Fatalf("LTEWCDMA = %+v, known %t", got.LTEWCDMA, got.LTEWCDMAKnown)
				}
			},
		},
		{
			name: "UMTS LTE neighbors",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVLocationUMTSLTE, umtsLTE)},
			check: func(t *testing.T, got NASCellLocationInfo) {
				want := []NASUMTSLTENeighbor{{1650, 321, -95.5, -12.25, 9, true}}
				if !got.UMTSLTEKnown || got.UMTSLTE.RRCState != NASWCDMARRCCellDCH || !reflect.DeepEqual(got.UMTSLTE.Cells, want) {
					t.Fatalf("UMTSLTE = %+v, known %t", got.UMTSLTE, got.UMTSLTEKnown)
				}
			},
		},
		{
			name: "extended channels and NR5G",
			tlvs: tlv.TLVs{
				tlv.Uint(nasTLVLocationUMTSCellID, uint32(0x12345678)),
				tlv.Uint(nasTLVLocationLTETimingAdvance, uint32(15)),
				tlv.Uint(nasTLVLocationLTEIntraEARFCN, uint32(66000)),
				tlv.Bytes(nasTLVLocationLTEInterEARFCNs, appendUint32ArrayTest(70000, 71000)),
				tlv.Bytes(nasTLVLocationUMTSLTEEARFCNs, appendUint32ArrayTest(72000)),
				tlv.Uint(nasTLVLocationNR5GARFCN, uint32(640000)),
				tlv.Bytes(nasTLVLocationNR5GCell, nr5g),
			},
			check: func(t *testing.T, got NASCellLocationInfo) {
				if !got.UMTSCellIDKnown || got.UMTSCellID != 0x12345678 ||
					!got.LTETimingAdvanceKnown || got.LTETimingAdvance != 15 ||
					!got.LTEIntraEARFCNKnown || got.LTEIntraEARFCN != 66000 ||
					!reflect.DeepEqual(got.LTEInterEARFCNs, []uint32{70000, 71000}) ||
					!reflect.DeepEqual(got.UMTSLTEEARFCNs, []uint32{72000}) ||
					!got.NR5GARFCNKnown || got.NR5GARFCN != 640000 || !got.NR5GKnown ||
					got.NR5G.PLMN.MNC != 260 || got.NR5G.TrackingAreaCode != [3]byte{0x11, 0x22, 0x33} ||
					got.NR5G.GlobalCellID != 0x0102030405060708 || got.NR5G.PhysicalCellID != 501 ||
					got.NR5G.RSRQ != -125 || got.NR5G.RSRP != -970 || got.NR5G.SNR != 135 {
					t.Fatalf("cell location = %+v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASCellLocationInfo
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestNASCellLocationInfoRejectsMalformedTLVs(t *testing.T) {
	validGERAN := binary.LittleEndian.AppendUint32(nil, 1)
	validGERAN = append(validGERAN, 0x64, 0xF0, 0x10)
	validGERAN = binary.LittleEndian.AppendUint16(validGERAN, 2)
	validGERAN = binary.LittleEndian.AppendUint16(validGERAN, 3)
	validGERAN = append(validGERAN, 4)
	validGERAN = binary.LittleEndian.AppendUint32(validGERAN, 5)
	validGERAN = binary.LittleEndian.AppendUint16(validGERAN, 6)
	validGERAN = append(validGERAN, 0)

	validUMTS := binary.LittleEndian.AppendUint16(nil, 1)
	validUMTS = append(validUMTS, 0x64, 0xF0, 0x10)
	for range 5 {
		validUMTS = binary.LittleEndian.AppendUint16(validUMTS, 2)
	}
	validUMTS = append(validUMTS, 0, 0)

	validLTEIntra := []byte{1, 0x64, 0xF0, 0x10}
	validLTEIntra = binary.LittleEndian.AppendUint16(validLTEIntra, 1)
	validLTEIntra = binary.LittleEndian.AppendUint32(validLTEIntra, 2)
	validLTEIntra = binary.LittleEndian.AppendUint16(validLTEIntra, 3)
	validLTEIntra = binary.LittleEndian.AppendUint16(validLTEIntra, 4)
	validLTEIntra = append(validLTEIntra, 5, 6, 7, 8, 0)

	tests := []struct {
		name string
		tlv  tlv.TLV
	}{
		{name: "GERAN truncated", tlv: tlv.Bytes(nasTLVLocationGERAN, validGERAN[:len(validGERAN)-1])},
		{name: "GERAN neighbor truncated", tlv: tlv.Bytes(nasTLVLocationGERAN, append(slices.Clone(validGERAN[:len(validGERAN)-1]), 1))},
		{name: "GERAN trailing data", tlv: tlv.Bytes(nasTLVLocationGERAN, append(slices.Clone(validGERAN), 1))},
		{name: "UMTS truncated", tlv: tlv.Bytes(nasTLVLocationUMTS, validUMTS[:len(validUMTS)-1])},
		{name: "UMTS monitored cell truncated", tlv: tlv.Bytes(nasTLVLocationUMTS, append(slices.Clone(validUMTS[:len(validUMTS)-2]), 1))},
		{name: "CDMA length", tlv: tlv.Bytes(nasTLVLocationCDMA, make([]byte, 15))},
		{name: "LTE intrafrequency truncated", tlv: tlv.Bytes(nasTLVLocationLTEIntra, validLTEIntra[:len(validLTEIntra)-1])},
		{name: "LTE intrafrequency cell truncated", tlv: tlv.Bytes(nasTLVLocationLTEIntra, append(slices.Clone(validLTEIntra[:len(validLTEIntra)-1]), 1))},
		{name: "LTE interfrequency truncated", tlv: tlv.Bytes(nasTLVLocationLTEInter, []byte{1, 1})},
		{name: "LTE GERAN truncated", tlv: tlv.Bytes(nasTLVLocationLTEGERAN, []byte{1, 1})},
		{name: "LTE WCDMA truncated", tlv: tlv.Bytes(nasTLVLocationLTEWCDMA, []byte{1, 1})},
		{name: "UMTS cell ID length", tlv: tlv.Bytes(nasTLVLocationUMTSCellID, make([]byte, 3))},
		{name: "UMTS LTE neighbor truncated", tlv: tlv.Bytes(nasTLVLocationUMTSLTE, []byte{0, 0, 0, 0, 1})},
		{name: "LTE timing advance length", tlv: tlv.Bytes(nasTLVLocationLTETimingAdvance, make([]byte, 3))},
		{name: "LTE intra EARFCN length", tlv: tlv.Bytes(nasTLVLocationLTEIntraEARFCN, make([]byte, 3))},
		{name: "LTE inter EARFCN count missing", tlv: tlv.Bytes(nasTLVLocationLTEInterEARFCNs, nil)},
		{name: "LTE inter EARFCN truncated", tlv: tlv.Bytes(nasTLVLocationLTEInterEARFCNs, []byte{1, 0, 0, 0})},
		{name: "UMTS LTE EARFCN trailing data", tlv: tlv.Bytes(nasTLVLocationUMTSLTEEARFCNs, []byte{0, 1})},
		{name: "NR5G ARFCN length", tlv: tlv.Bytes(nasTLVLocationNR5GARFCN, make([]byte, 3))},
		{name: "NR5G cell length", tlv: tlv.Bytes(nasTLVLocationNR5GCell, make([]byte, 21))},
		{name: "invalid BCD PLMN", tlv: tlv.Bytes(nasTLVLocationLTEIntra, append([]byte{1, 0x6A}, validLTEIntra[2:]...))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASCellLocationInfo
			if err := got.UnmarshalTLVs(tlv.TLVs{tt.tlv}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want malformed TLV error")
			}
		})
	}
}

func TestCellLocationInfo(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		wantErr string
	}{
		{
			name: "success",
			resp: successResponse(MessageNASGetCellLocationInfo, tlv.Uint(nasTLVLocationNR5GARFCN, uint32(640000))),
		},
		{
			name:    "malformed response",
			resp:    successResponse(MessageNASGetCellLocationInfo, tlv.Bytes(nasTLVLocationNR5GARFCN, []byte{1})),
			wantErr: "NR5G ARFCN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.MessageID != MessageNASGetCellLocationInfo || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			got, err := client.CellLocationInfo(context.Background())
			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("CellLocationInfo() error = %v, want text %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("CellLocationInfo() error = %v", err)
			case !got.NR5GARFCNKnown || got.NR5GARFCN != 640000:
				t.Fatalf("CellLocationInfo() = %+v", got)
			}
		})
	}
}

func appendLocationInt16Test(value []byte, number int16) []byte {
	return binary.LittleEndian.AppendUint16(value, uint16(number))
}

func appendLTECellTest(value []byte, pci uint16, rsrq, rsrp, rssi, srxlev int16) []byte {
	value = binary.LittleEndian.AppendUint16(value, pci)
	value = appendLocationInt16Test(value, rsrq)
	value = appendLocationInt16Test(value, rsrp)
	value = appendLocationInt16Test(value, rssi)
	return appendLocationInt16Test(value, srxlev)
}

func appendUint32ArrayTest(values ...uint32) []byte {
	encoded := []byte{byte(len(values))}
	for _, value := range values {
		encoded = binary.LittleEndian.AppendUint32(encoded, value)
	}
	return encoded
}
