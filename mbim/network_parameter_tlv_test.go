package mbim

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
)

func TestNSSAIListTLV(t *testing.T) {
	values := []SNSSAI{
		{SliceServiceType: 1},
		{SliceServiceType: 2, SliceDifferentiator: [3]byte{1, 2, 3}, HasSliceDifferentiator: true},
	}
	tests := []struct {
		name string
		typ  TLVType
	}{
		{name: "allowed", typ: TLVTypeAllowedNSSAI},
		{name: "configured", typ: TLVTypeConfiguredNSSAI},
		{name: "default configured", typ: TLVTypeDefaultConfiguredNSSAI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewNSSAIListTLV(tt.typ, values)
			if err != nil {
				t.Fatalf("NewNSSAIListTLV() error = %v", err)
			}
			if tlv.Type != tt.typ || !bytes.Equal(tlv.Data, []byte{1, 1, 4, 2, 1, 2, 3}) {
				t.Fatalf("NewNSSAIListTLV() = %+v", tlv)
			}
			var got NSSAIList
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if !slices.Equal(got, values) {
				t.Fatalf("UnmarshalTLV() = %+v, want %+v", got, values)
			}
		})
	}
}

func TestNSSAIListTLVValidation(t *testing.T) {
	marshalTests := []struct {
		name   string
		typ    TLVType
		values []SNSSAI
	}{
		{name: "wrong type", typ: TLVTypePCO, values: []SNSSAI{{SliceServiceType: 1}}},
		{name: "empty", typ: TLVTypeAllowedNSSAI},
		{name: "invalid S-NSSAI", typ: TLVTypeAllowedNSSAI, values: []SNSSAI{{HasMappedSliceDifferentiator: true}}},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewNSSAIListTLV(tt.typ, tt.values); err == nil {
				t.Fatal("NewNSSAIListTLV() error = nil, want non-nil")
			}
		})
	}

	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO}},
		{name: "empty", tlv: TLV{Type: TLVTypeAllowedNSSAI}},
		{name: "reserved length", tlv: TLV{Type: TLVTypeAllowedNSSAI, Data: []byte{3, 1, 2, 3}}},
		{name: "truncated", tlv: TLV{Type: TLVTypeAllowedNSSAI, Data: []byte{4, 1, 2}}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got NSSAIList
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}

func TestRejectedNSSAITLV(t *testing.T) {
	values := []RejectedSNSSAI{
		{Cause: RejectedNSSAINotAvailableInPLMN, SNSSAI: SNSSAI{SliceServiceType: 1}},
		{
			Cause: RejectedNSSAINotAvailableInRegistrationArea,
			SNSSAI: SNSSAI{
				SliceServiceType:       2,
				SliceDifferentiator:    [3]byte{1, 2, 3},
				HasSliceDifferentiator: true,
			},
		},
	}
	tests := []struct {
		name string
	}{
		{name: "SST and SST with SD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewRejectedNSSAITLV(values)
			if err != nil {
				t.Fatalf("NewRejectedNSSAITLV() error = %v", err)
			}
			if !bytes.Equal(tlv.Data, []byte{1, 0, 1, 4, 1, 2, 1, 2, 3}) {
				t.Fatalf("TLV data = %X", tlv.Data)
			}
			var got RejectedNSSAIList
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if !slices.Equal(got, values) {
				t.Fatalf("UnmarshalTLV() = %+v, want %+v", got, values)
			}
		})
	}
}

func TestRejectedNSSAITLVValidation(t *testing.T) {
	valid := RejectedSNSSAI{Cause: RejectedNSSAINotAvailableInPLMN, SNSSAI: SNSSAI{SliceServiceType: 1}}
	marshalTests := []struct {
		name   string
		values []RejectedSNSSAI
	}{
		{name: "empty"},
		{name: "reserved cause", values: []RejectedSNSSAI{{Cause: 2, SNSSAI: SNSSAI{SliceServiceType: 1}}}},
		{name: "reserved SST", values: []RejectedSNSSAI{{SNSSAI: SNSSAI{SliceServiceType: 4}}}},
		{name: "mapped SST", values: []RejectedSNSSAI{{SNSSAI: SNSSAI{SliceServiceType: 1, HasMappedSliceServiceType: true}}}},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewRejectedNSSAITLV(tt.values); err == nil {
				t.Fatal("NewRejectedNSSAITLV() error = nil, want non-nil")
			}
		})
	}

	validTLV, err := NewRejectedNSSAITLV([]RejectedSNSSAI{valid})
	if err != nil {
		t.Fatalf("NewRejectedNSSAITLV() error = %v", err)
	}
	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO}},
		{name: "empty", tlv: TLV{Type: TLVTypeRejectedNSSAI}},
		{name: "truncated", tlv: TLV{Type: TLVTypeRejectedNSSAI, Data: []byte{1, 0}}},
		{name: "reserved length", tlv: TLV{Type: TLVTypeRejectedNSSAI, Data: []byte{2, 0, 1, 2}}},
		{name: "reserved cause", tlv: TLV{Type: TLVTypeRejectedNSSAI, Data: []byte{1, 2, 1}}},
		{name: "reserved SST", tlv: TLV{Type: TLVTypeRejectedNSSAI, Data: []byte{1, 0, 4}}},
		{name: "trailing truncated entry", tlv: TLV{Type: TLVTypeRejectedNSSAI, Data: append(validTLV.Data, 1)}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got RejectedNSSAIList
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}

func TestTAITLV(t *testing.T) {
	lists := []TAIList{
		{Type: TAIListTypeNonConsecutive, PLMN: PLMN{MCC: 0x0310, MNC: 0x0260}, TACs: []uint32{1, 3}},
		{Type: TAIListTypeConsecutive, PLMN: PLMN{MCC: 0x0311, MNC: 0x8026}, TACs: []uint32{4, 5, 6}},
		{
			Type: TAIListTypeMultiplePLMNs,
			TAIs: []TrackingAreaIdentity{
				{PLMN: PLMN{MCC: 0x0312, MNC: 0x0261}, TAC: 7},
				{PLMN: PLMN{MCC: 0x0313, MNC: 0x8027}, TAC: 8},
			},
		},
	}
	tests := []struct {
		name string
	}{
		{name: "all list types"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewTAITLV(lists)
			if err != nil {
				t.Fatalf("NewTAITLV() error = %v", err)
			}
			wantPrefix := []byte{0, 0x10, 0x03, 0x60, 0x02, 2}
			wantPrefix = binary.LittleEndian.AppendUint32(wantPrefix, 1)
			wantPrefix = binary.LittleEndian.AppendUint32(wantPrefix, 3)
			if !bytes.HasPrefix(tlv.Data, wantPrefix) {
				t.Fatalf("TLV data prefix = %X, want %X", tlv.Data, wantPrefix)
			}
			var got TAILists
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if len(got) != len(lists) {
				t.Fatalf("TAI list count = %d, want %d", len(got), len(lists))
			}
			for index := range got {
				if got[index].Type != lists[index].Type || got[index].PLMN != lists[index].PLMN ||
					!slices.Equal(got[index].TACs, lists[index].TACs) || !slices.Equal(got[index].TAIs, lists[index].TAIs) {
					t.Fatalf("TAI list %d = %+v, want %+v", index, got[index], lists[index])
				}
			}
		})
	}
}

func TestTAITLVValidation(t *testing.T) {
	validPLMN := PLMN{MCC: 0x0310, MNC: 0x0260}
	marshalTests := []struct {
		name  string
		lists []TAIList
	}{
		{name: "empty"},
		{name: "reserved type", lists: []TAIList{{Type: 3}}},
		{name: "empty TAC list", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: validPLMN}}},
		{name: "too many TACs", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: validPLMN, TACs: make([]uint32, 17)}}},
		{name: "nonconsecutive TACs", lists: []TAIList{{Type: TAIListTypeConsecutive, PLMN: validPLMN, TACs: []uint32{1, 3}}}},
		{name: "TAC exceeds 24 bits", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: validPLMN, TACs: []uint32{0x1000000}}}},
		{name: "invalid MCC BCD", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: PLMN{MCC: 0xA10}, TACs: []uint32{1}}}},
		{name: "invalid MNC unused bits", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: PLMN{MCC: 0x310, MNC: 0x8100}, TACs: []uint32{1}}}},
		{name: "single PLMN with TAIs", lists: []TAIList{{Type: TAIListTypeNonConsecutive, PLMN: validPLMN, TACs: []uint32{1}, TAIs: []TrackingAreaIdentity{{}}}}},
		{name: "empty multi PLMN", lists: []TAIList{{Type: TAIListTypeMultiplePLMNs}}},
		{name: "multi PLMN with single fields", lists: []TAIList{{Type: TAIListTypeMultiplePLMNs, PLMN: validPLMN, TAIs: []TrackingAreaIdentity{{PLMN: validPLMN, TAC: 1}}}}},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewTAITLV(tt.lists); err == nil {
				t.Fatal("NewTAITLV() error = nil, want non-nil")
			}
		})
	}

	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO}},
		{name: "empty", tlv: TLV{Type: TLVTypeTAI}},
		{name: "reserved list type", tlv: TLV{Type: TLVTypeTAI, Data: []byte{3}}},
		{name: "single PLMN header truncated", tlv: TLV{Type: TLVTypeTAI, Data: []byte{0, 1}}},
		{name: "zero TAC count", tlv: TLV{Type: TLVTypeTAI, Data: []byte{0, 0x10, 0x03, 0x60, 0x02, 0}}},
		{name: "TAC values truncated", tlv: TLV{Type: TLVTypeTAI, Data: []byte{0, 0x10, 0x03, 0x60, 0x02, 1, 1}}},
		{name: "multi PLMN header truncated", tlv: TLV{Type: TLVTypeTAI, Data: []byte{2}}},
		{name: "zero TAI count", tlv: TLV{Type: TLVTypeTAI, Data: []byte{2, 0}}},
		{name: "TAI values truncated", tlv: TLV{Type: TLVTypeTAI, Data: []byte{2, 1, 1}}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got TAILists
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}

func TestTAIListRejectsTrailingData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "second list", data: []byte{0, 0x10, 0x03, 0x60, 0x02, 1, 1, 0, 0, 0, 2, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var list TAIList
			if err := list.UnmarshalBinary(tt.data); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestLADNTLV(t *testing.T) {
	values := []LADN{
		{
			DNN: "ims",
			TAILists: []TAIList{{
				Type: TAIListTypeNonConsecutive,
				PLMN: PLMN{MCC: 0x0310, MNC: 0x0260},
				TACs: []uint32{1, 3},
			}},
		},
		{
			DNN: "internet",
			TAILists: []TAIList{{
				Type: TAIListTypeMultiplePLMNs,
				TAIs: []TrackingAreaIdentity{{PLMN: PLMN{MCC: 0x0311, MNC: 0x8026}, TAC: 4}},
			}},
		},
	}
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "MBIMEx 3", version: mbimExVersion30},
		{name: "MBIMEx 4", version: mbimExVersion40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewLADNTLV(values, tt.version)
			if err != nil {
				t.Fatalf("NewLADNTLV() error = %v", err)
			}
			if tt.version == mbimExVersion30 && !bytes.HasPrefix(tlv.Data, []byte{3, 'i', 'm', 's', 0}) {
				t.Fatalf("MBIMEx 3 TLV data = %X", tlv.Data)
			}
			if tt.version == mbimExVersion40 && !bytes.HasPrefix(tlv.Data, []byte{10, 0, 0, 2, 6, 0, 0, 0}) {
				t.Fatalf("MBIMEx 4 TLV data = %X", tlv.Data)
			}

			var got LADNList
			if err := got.UnmarshalTLV(tlv, tt.version); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if len(got) != len(values) {
				t.Fatalf("LADN count = %d, want %d", len(got), len(values))
			}
			for index := range got {
				if got[index].DNN != values[index].DNN || len(got[index].TAILists) != len(values[index].TAILists) {
					t.Fatalf("LADN %d = %+v, want %+v", index, got[index], values[index])
				}
			}
		})
	}
}

func TestLADNTLVValidation(t *testing.T) {
	validList := TAIList{
		Type: TAIListTypeNonConsecutive,
		PLMN: PLMN{MCC: 0x0310, MNC: 0x0260},
		TACs: []uint32{1},
	}
	marshalTests := []struct {
		name    string
		version uint16
		values  []LADN
	}{
		{name: "MBIMEx 2", version: mbimExVersion20, values: []LADN{{DNN: "ims", TAILists: []TAIList{validList}}}},
		{name: "empty", version: mbimExVersion30},
		{name: "MBIMEx 3 short DNN", version: mbimExVersion30, values: []LADN{{DNN: "ab", TAILists: []TAIList{validList}}}},
		{name: "MBIMEx 3 long DNN", version: mbimExVersion30, values: []LADN{{DNN: string(make([]byte, maximumLADNDNNLength+1)), TAILists: []TAIList{validList}}}},
		{name: "empty TAI list", version: mbimExVersion40, values: []LADN{{DNN: "ims"}}},
		{name: "invalid TAI list", version: mbimExVersion40, values: []LADN{{DNN: "ims", TAILists: []TAIList{{Type: 3}}}}},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewLADNTLV(tt.values, tt.version); err == nil {
				t.Fatal("NewLADNTLV() error = nil, want non-nil")
			}
		})
	}

	validTAIData, err := validList.MarshalBinary()
	if err != nil {
		t.Fatalf("TAIList.MarshalBinary() error = %v", err)
	}
	parseTests := []struct {
		name    string
		version uint16
		tlv     TLV
	}{
		{name: "MBIMEx 2", version: mbimExVersion20, tlv: TLV{Type: TLVTypeLADN}},
		{name: "wrong outer type", version: mbimExVersion40, tlv: TLV{Type: TLVTypePCO}},
		{name: "empty", version: mbimExVersion40, tlv: TLV{Type: TLVTypeLADN}},
		{name: "MBIMEx 3 short DNN", version: mbimExVersion30, tlv: TLV{Type: TLVTypeLADN, Data: []byte{2, 'a', 'b'}}},
		{name: "MBIMEx 3 truncated DNN", version: mbimExVersion30, tlv: TLV{Type: TLVTypeLADN, Data: []byte{3, 'i'}}},
		{name: "MBIMEx 3 missing TAI list", version: mbimExVersion30, tlv: TLV{Type: TLVTypeLADN, Data: []byte{3, 'i', 'm', 's'}}},
		{name: "MBIMEx 4 wrong DNN type", version: mbimExVersion40, tlv: TLV{Type: TLVTypeLADN, Data: mbimTLV(TLVTypePCO, nil)}},
		{name: "MBIMEx 4 odd DNN", version: mbimExVersion40, tlv: TLV{Type: TLVTypeLADN, Data: append(mbimTLV(TLVTypeWCharString, []byte{1}), validTAIData...)}},
		{name: "MBIMEx 4 missing TAI list", version: mbimExVersion40, tlv: TLV{Type: TLVTypeLADN, Data: mbimTLV(TLVTypeWCharString, utf16Bytes("ims"))}},
		{name: "malformed TAI list", version: mbimExVersion40, tlv: TLV{Type: TLVTypeLADN, Data: append(mbimTLV(TLVTypeWCharString, utf16Bytes("ims")), 0)}},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got LADNList
			if err := got.UnmarshalTLV(tt.tlv, tt.version); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}
