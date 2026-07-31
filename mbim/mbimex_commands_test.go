package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"
)

func TestMBIMExCommandRequests(t *testing.T) {
	queryTLVs := TLVs{{Type: TLVTypeSessionID, Data: binary.LittleEndian.AppendUint32(nil, 3)}}
	queryData, err := queryTLVs.MarshalBinary()
	if err != nil {
		t.Fatalf("query TLVs MarshalBinary() error = %v", err)
	}
	registration := RegistrationParametersInfo{
		MBIMExVersion:      mbimExVersion40,
		MICOMode:           MICOModeDisabled,
		DRXParameters:      DRXNotSpecified,
		LADNIndication:     LADNInfoNotNeeded,
		DefaultPDUHint:     DefaultPDUActivationLikely,
		ReRegisterIfNeeded: 1,
	}
	tests := []struct {
		name        string
		request     *Request
		cid         uint32
		commandType CommandType
		wantData    []byte
		wantTimeout time.Duration
	}{
		{
			name:        "modem configuration query",
			request:     (&ModemConfigurationRequest{TransactionID: 1, MBIMExVersion: mbimExVersion40}).Request(),
			cid:         CIDMsModemConfiguration,
			commandType: CommandTypeQuery,
			wantTimeout: mbimCIDResponseTimeout,
		},
		{
			name:        "registration parameters query",
			request:     (&RegistrationParametersRequest{TransactionID: 1, MBIMExVersion: mbimExVersion40}).Request(),
			cid:         CIDMsRegistrationParameters,
			commandType: CommandTypeQuery,
			wantTimeout: mbimCIDResponseTimeout,
		},
		{
			name: "registration parameters set",
			request: (&RegistrationParametersSetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				Parameters:    registration,
			}).Request(),
			cid:         CIDMsRegistrationParameters,
			commandType: CommandTypeSet,
			wantData:    registration.marshalBinaryUnchecked(),
			wantTimeout: mbimCIDResponseTimeout,
		},
		{
			name: "network parameters query",
			request: (&NetworkParametersRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				Query:         NetworkParametersQuery{TLVs: queryTLVs},
			}).Request(),
			cid:         CIDMsNetworkParameters,
			commandType: CommandTypeQuery,
			wantData:    queryData,
			wantTimeout: mbimCIDLongResponseTimeout,
		},
		{
			name:        "wake reason query",
			request:     (&WakeReasonRequest{TransactionID: 1}).Request(),
			cid:         CIDMsWakeReason,
			commandType: CommandTypeQuery,
			wantTimeout: mbimWakeReasonResponseTimeout,
		},
		{
			name: "UE policy query",
			request: (&UEPolicyRequest{
				TransactionID: 1,
				Query:         queryTLVs,
			}).Request(),
			cid:         CIDMsUEPolicy,
			commandType: CommandTypeQuery,
			wantData:    queryData,
			wantTimeout: mbimCIDLongResponseTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.request.Command.(*Command)
			if command.ServiceID != ServiceMsBasicConnectExtensions || command.CommandID != tt.cid || command.CommandType != tt.commandType {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if !bytes.Equal(command.Data, tt.wantData) {
				t.Fatalf("command data = %x, want %x", command.Data, tt.wantData)
			}
			if tt.request.Timeout != tt.wantTimeout {
				t.Fatalf("timeout = %v, want %v", tt.request.Timeout, tt.wantTimeout)
			}
			if tt.cid == CIDMsRegistrationParameters {
				response := tt.request.Response.(*RegistrationParametersInfo)
				if response.MBIMExVersion != mbimExVersion40 {
					t.Fatalf("response version = %#x, want %#x", response.MBIMExVersion, mbimExVersion40)
				}
			}
			if tt.cid == CIDMsModemConfiguration {
				response := tt.request.Response.(*ModemConfigurationInfo)
				if response.MBIMExVersion != mbimExVersion40 {
					t.Fatalf("response version = %#x, want %#x", response.MBIMExVersion, mbimExVersion40)
				}
			}
			if tt.cid == CIDMsNetworkParameters {
				response := tt.request.Response.(*NetworkParametersInfo)
				if response.MBIMExVersion != mbimExVersion40 {
					t.Fatalf("response version = %#x, want %#x", response.MBIMExVersion, mbimExVersion40)
				}
			}
		})
	}
}

func TestModemConfigurationInfoCodec(t *testing.T) {
	preconfigured := []PreconfiguredDefaultNSSAI{{
		AccessType:     AccessType3GPP,
		PreferredNSSAI: []SNSSAI{{SliceServiceType: 1}},
	}}
	preconfiguredTLV, err := NewPreconfiguredDefaultNSSAITLV(preconfigured)
	if err != nil {
		t.Fatalf("NewPreconfiguredDefaultNSSAITLV() error = %v", err)
	}
	valid := ModemConfigurationInfo{
		MBIMExVersion:             mbimExVersion30,
		Status:                    ModemConfigurationStatusCompleted,
		ConfigName:                "carrier",
		TLVs:                      TLVs{preconfiguredTLV},
		PreconfiguredDefaultNSSAI: preconfigured,
	}
	validData, err := valid.MarshalBinary()
	if err != nil {
		t.Fatalf("valid MarshalBinary() error = %v", err)
	}
	mappedTLV := mappedPreconfiguredDefaultNSSAITLVForTest()
	mappedData := binary.LittleEndian.AppendUint32(nil, uint32(ModemConfigurationStatusCompleted))
	mappedData = append(mappedData, mbimTLV(TLVTypeWCharString, utf16Bytes("carrier"))...)
	mappedData = append(mappedData, mbimTLV(mappedTLV.Type, mappedTLV.Data)...)

	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    ModemConfigurationInfo
		wantErr bool
	}{
		{name: "completed", version: mbimExVersion30, data: validData, want: valid},
		{name: "MBIMEx 4 mapped S-NSSAI", version: mbimExVersion40, data: mappedData, wantErr: true},
		{name: "MBIMEx 3 mapped S-NSSAI", version: mbimExVersion30, data: mappedData, wantErr: true},
		{name: "MBIMEx 2", version: mbimExVersion20, data: validData, wantErr: true},
		{name: "truncated", data: make([]byte, 11), wantErr: true},
		{
			name: "wrong name TLV",
			data: append(
				binary.LittleEndian.AppendUint32(nil, uint32(ModemConfigurationStatusStarted)),
				mbimTLV(TLVTypePCO, nil)...,
			),
			wantErr: true,
		},
		{
			name: "odd UTF-16 name",
			data: append(
				binary.LittleEndian.AppendUint32(nil, uint32(ModemConfigurationStatusStarted)),
				mbimTLV(TLVTypeWCharString, []byte{1})...,
			),
			wantErr: true,
		},
		{
			name: "completed empty name",
			data: append(
				binary.LittleEndian.AppendUint32(nil, uint32(ModemConfigurationStatusCompleted)),
				mbimTLV(TLVTypeWCharString, nil)...,
			),
			wantErr: true,
		},
		{
			name: "reserved status",
			data: append(
				binary.LittleEndian.AppendUint32(nil, 3),
				mbimTLV(TLVTypeWCharString, nil)...,
			),
			wantErr: true,
		},
		{
			name: "malformed preconfigured default NSSAI",
			data: append(
				append(binary.LittleEndian.AppendUint32(nil, uint32(ModemConfigurationStatusCompleted)), mbimTLV(TLVTypeWCharString, utf16Bytes("carrier"))...),
				mbimTLV(TLVTypePreconfiguredDefaultConfiguredNSSAI, []byte{1})...,
			),
			wantErr: true,
		},
		{
			name:    "duplicate preconfigured default NSSAI",
			data:    append(slices.Clone(validData), mbimTLV(preconfiguredTLV.Type, preconfiguredTLV.Data)...),
			wantErr: true,
		},
		{name: "malformed optional TLV", data: append(slices.Clone(validData), 1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModemConfigurationInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MBIMExVersion != tt.want.MBIMExVersion || got.Status != tt.want.Status || got.ConfigName != tt.want.ConfigName || len(got.TLVs) != len(tt.want.TLVs) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
			if len(got.PreconfiguredDefaultNSSAI) != len(tt.want.PreconfiguredDefaultNSSAI) {
				t.Fatalf("preconfigured default NSSAI count = %d, want %d", len(got.PreconfiguredDefaultNSSAI), len(tt.want.PreconfiguredDefaultNSSAI))
			}
			for index := range got.PreconfiguredDefaultNSSAI {
				if got.PreconfiguredDefaultNSSAI[index].AccessType != tt.want.PreconfiguredDefaultNSSAI[index].AccessType ||
					!slices.Equal(got.PreconfiguredDefaultNSSAI[index].PreferredNSSAI, tt.want.PreconfiguredDefaultNSSAI[index].PreferredNSSAI) {
					t.Fatalf("preconfigured default NSSAI %d = %+v, want %+v", index, got.PreconfiguredDefaultNSSAI[index], tt.want.PreconfiguredDefaultNSSAI[index])
				}
			}
		})
	}
}

func TestModemConfigurationInfoMarshalBinaryValidation(t *testing.T) {
	unmappedTLV, err := NewPreconfiguredDefaultNSSAITLV([]PreconfiguredDefaultNSSAI{{
		AccessType:     AccessType3GPP,
		PreferredNSSAI: []SNSSAI{{SliceServiceType: 1}},
	}})
	if err != nil {
		t.Fatalf("NewPreconfiguredDefaultNSSAITLV() unmapped error = %v", err)
	}
	mappedTLV := mappedPreconfiguredDefaultNSSAITLVForTest()
	tests := []struct {
		name string
		info ModemConfigurationInfo
	}{
		{name: "MBIMEx 2", info: ModemConfigurationInfo{MBIMExVersion: mbimExVersion20}},
		{name: "reserved status", info: ModemConfigurationInfo{Status: 3}},
		{name: "completed empty name", info: ModemConfigurationInfo{Status: ModemConfigurationStatusCompleted}},
		{name: "MBIMEx 3 mapped S-NSSAI", info: ModemConfigurationInfo{MBIMExVersion: mbimExVersion30, TLVs: TLVs{mappedTLV}}},
		{name: "duplicate preconfigured default NSSAI", info: ModemConfigurationInfo{TLVs: TLVs{unmappedTLV, unmappedTLV}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.info.MarshalBinary(); err == nil {
				t.Fatal("MarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestModemConfigurationClientVersionValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "MBIMEx 1", version: mbimExVersion10},
		{name: "MBIMEx 2", version: mbimExVersion20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{mbimExVersion: tt.version}
			if _, err := client.ModemConfiguration(context.Background()); err == nil {
				t.Fatal("ModemConfiguration() error = nil, want non-nil")
			}
		})
	}
}

func TestRegistrationParametersInfoCodec(t *testing.T) {
	preconfigured := func(values []PreconfiguredDefaultNSSAI) TLV {
		t.Helper()
		tlv, err := NewPreconfiguredDefaultNSSAITLV(values)
		if err != nil {
			t.Fatalf("NewPreconfiguredDefaultNSSAITLV() error = %v", err)
		}
		return tlv
	}
	setInfo := func(version uint16, tlvs TLVs) RegistrationParametersInfo {
		return RegistrationParametersInfo{
			MBIMExVersion:      version,
			MICOMode:           MICOModeDefault,
			DRXParameters:      DRXNotSpecified,
			LADNIndication:     LADNInfoNotNeeded,
			DefaultPDUHint:     DefaultPDUActivationLikely,
			ReRegisterIfNeeded: 1,
			TLVs:               tlvs,
		}
	}
	embb := PreconfiguredDefaultNSSAI{
		AccessType:     AccessType3GPP,
		PreferredNSSAI: []SNSSAI{{SliceServiceType: 1}},
	}
	osid := TLV{Type: TLVTypeOSID, Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}

	setTests := []struct {
		name    string
		info    RegistrationParametersInfo
		wantErr bool
	}{
		{name: "MBIMEx 3 eMBB NSSAI", info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{embb})})},
		{name: "MBIMEx 4 OSID", info: setInfo(mbimExVersion40, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{embb}), osid})},
		{name: "CID before MBIMEx 3", info: setInfo(mbimExVersion20, nil), wantErr: true},
		{name: "MICO enabled", info: func() RegistrationParametersInfo {
			value := setInfo(mbimExVersion30, nil)
			value.MICOMode = MICOModeEnabled
			return value
		}(), wantErr: true},
		{name: "DRX cycle", info: func() RegistrationParametersInfo {
			value := setInfo(mbimExVersion30, nil)
			value.DRXParameters = DRXCycle64
			return value
		}(), wantErr: true},
		{name: "LADN requested", info: func() RegistrationParametersInfo {
			value := setInfo(mbimExVersion30, nil)
			value.LADNIndication = LADNInfoRequested
			return value
		}(), wantErr: true},
		{name: "reserved default PDU hint", info: func() RegistrationParametersInfo {
			value := setInfo(mbimExVersion30, nil)
			value.DefaultPDUHint = 2
			return value
		}(), wantErr: true},
		{name: "reserved re-register indicator", info: func() RegistrationParametersInfo {
			value := setInfo(mbimExVersion30, nil)
			value.ReRegisterIfNeeded = 2
			return value
		}(), wantErr: true},
		{
			name: "duplicate preconfigured TLV",
			info: setInfo(mbimExVersion30, TLVs{
				preconfigured([]PreconfiguredDefaultNSSAI{embb}),
				preconfigured([]PreconfiguredDefaultNSSAI{embb}),
			}),
			wantErr: true,
		},
		{name: "empty preferred NSSAI", info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{{AccessType: AccessType3GPP}})}), wantErr: true},
		{
			name: "multiple preferred S-NSSAIs",
			info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{{
				AccessType: AccessType3GPP,
				PreferredNSSAI: []SNSSAI{
					{SliceServiceType: 1},
					{SliceServiceType: 1},
				},
			}})}),
			wantErr: true,
		},
		{
			name: "multiple access lists",
			info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{
				embb,
				{AccessType: AccessTypeNon3GPP},
			})}),
			wantErr: true,
		},
		{name: "non-eMBB SST", info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{{AccessType: AccessType3GPP, PreferredNSSAI: []SNSSAI{{SliceServiceType: 2}}}})}), wantErr: true},
		{name: "S-NSSAI with SD", info: setInfo(mbimExVersion30, TLVs{preconfigured([]PreconfiguredDefaultNSSAI{{AccessType: AccessType3GPP, PreferredNSSAI: []SNSSAI{{SliceServiceType: 1, HasSliceDifferentiator: true}}}})}), wantErr: true},
		{name: "S-NSSAI with mapped SST", info: setInfo(mbimExVersion40, TLVs{mappedPreconfiguredDefaultNSSAITLVForTest()}), wantErr: true},
		{name: "MBIMEx 3 OSID", info: setInfo(mbimExVersion30, TLVs{osid}), wantErr: true},
		{name: "invalid OSID length", info: setInfo(mbimExVersion40, TLVs{{Type: TLVTypeOSID, Data: []byte{1}}}), wantErr: true},
		{name: "duplicate OSID", info: setInfo(mbimExVersion40, TLVs{osid, osid}), wantErr: true},
		{name: "reserved TLV type", info: setInfo(mbimExVersion40, TLVs{{Type: 0}}), wantErr: true},
	}

	for _, tt := range setTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			data, err := tt.info.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(data) < 20 {
				t.Fatalf("MarshalBinary() length = %d, want at least 20", len(data))
			}
		})
	}

	responsePreconfigured := preconfigured([]PreconfiguredDefaultNSSAI{
		{
			AccessType: AccessType3GPP,
			PreferredNSSAI: []SNSSAI{{
				SliceServiceType:       1,
				SliceDifferentiator:    [3]byte{2, 3, 4},
				HasSliceDifferentiator: true,
			}},
		},
		{AccessType: AccessTypeNon3GPP},
	})
	validResponse := RegistrationParametersInfo{
		MBIMExVersion:  mbimExVersion40,
		MICOMode:       MICOModeEnabled,
		DRXParameters:  DRXCycle64,
		LADNIndication: LADNInfoRequested,
		DefaultPDUHint: DefaultPDUActivationLikely,
		TLVs:           TLVs{responsePreconfigured},
	}
	mappedResponse := RegistrationParametersInfo{
		MBIMExVersion: mbimExVersion30,
		TLVs:          TLVs{mappedPreconfiguredDefaultNSSAITLVForTest()},
	}
	duplicatePreconfigured := RegistrationParametersInfo{
		TLVs: TLVs{
			preconfigured([]PreconfiguredDefaultNSSAI{embb}),
			preconfigured([]PreconfiguredDefaultNSSAI{embb}),
		},
	}
	responseTests := []struct {
		name    string
		version uint16
		data    []byte
		want    RegistrationParametersInfo
		wantErr bool
	}{
		{name: "MBIMEx 4 response", version: mbimExVersion40, data: validResponse.marshalBinaryUnchecked(), want: validResponse},
		{name: "truncated", version: mbimExVersion40, data: make([]byte, 19), wantErr: true},
		{name: "CID before MBIMEx 3", version: mbimExVersion20, data: make([]byte, 20), wantErr: true},
		{name: "OSID is set-only", version: mbimExVersion40, data: setInfo(mbimExVersion40, TLVs{osid}).marshalBinaryUnchecked(), wantErr: true},
		{name: "duplicate preconfigured TLV", version: mbimExVersion30, data: duplicatePreconfigured.marshalBinaryUnchecked(), wantErr: true},
		{name: "MBIMEx 3 mapped NSSAI", version: mbimExVersion30, data: mappedResponse.marshalBinaryUnchecked(), wantErr: true},
		{name: "malformed preconfigured TLV", version: mbimExVersion40, data: setInfo(mbimExVersion40, TLVs{{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI}}).marshalBinaryUnchecked(), wantErr: true},
		{name: "malformed optional TLV", version: mbimExVersion40, data: append(validResponse.marshalBinaryUnchecked(), 1), wantErr: true},
		{
			name:    "set-only MICO default in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(MICOModeDefault))
			}),
			wantErr: true,
		},
		{
			name:    "reserved MICO mode in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(MICOModeDefault)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved DRX parameters in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], uint32(DRXCycle256)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved LADN indication in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(LADNInfoRequested)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved default PDU hint in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], uint32(DefaultPDUActivationLikely)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved re-register indicator in response",
			version: mbimExVersion40,
			data: mutateBytes(validResponse.marshalBinaryUnchecked(), func(data []byte) {
				binary.LittleEndian.PutUint32(data[16:20], 2)
			}),
			wantErr: true,
		},
	}

	for _, tt := range responseTests {
		t.Run("unmarshal "+tt.name, func(t *testing.T) {
			got := RegistrationParametersInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MBIMExVersion != tt.want.MBIMExVersion ||
				got.MICOMode != tt.want.MICOMode ||
				got.DRXParameters != tt.want.DRXParameters ||
				got.LADNIndication != tt.want.LADNIndication ||
				got.DefaultPDUHint != tt.want.DefaultPDUHint ||
				got.ReRegisterIfNeeded != tt.want.ReRegisterIfNeeded ||
				len(got.TLVs) != len(tt.want.TLVs) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
			for index := range got.TLVs {
				if got.TLVs[index].Type != tt.want.TLVs[index].Type || !bytes.Equal(got.TLVs[index].Data, tt.want.TLVs[index].Data) {
					t.Fatalf("TLV %d = %+v, want %+v", index, got.TLVs[index], tt.want.TLVs[index])
				}
			}
		})
	}
}

func TestNetworkParametersInfoCodec(t *testing.T) {
	allowedTLV, err := NewNSSAIListTLV(TLVTypeAllowedNSSAI, []SNSSAI{{SliceServiceType: 1}})
	if err != nil {
		t.Fatalf("NewNSSAIListTLV() error = %v", err)
	}
	taiTLV, err := NewTAITLV([]TAIList{{
		Type: TAIListTypeNonConsecutive,
		PLMN: PLMN{MCC: 0x0310, MNC: 0x0260},
		TACs: []uint32{1},
	}})
	if err != nil {
		t.Fatalf("NewTAITLV() error = %v", err)
	}
	valid := NetworkParametersInfo{
		MBIMExVersion:  mbimExVersion40,
		MICOIndication: MICOIndicationRegistrationAreaAllocated,
		DRXParameters:  DRXCycle128,
		TLVs:           TLVs{allowedTLV, taiTLV},
	}
	validData, err := valid.MarshalBinary()
	if err != nil {
		t.Fatalf("valid MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name    string
		version uint16
		data    []byte
		wantErr bool
	}{
		{name: "MBIMEx 3 round trip", version: mbimExVersion30, data: validData},
		{name: "MBIMEx 4 round trip", version: mbimExVersion40, data: validData},
		{name: "MBIMEx 2 unsupported", version: mbimExVersion20, data: validData, wantErr: true},
		{name: "truncated", version: mbimExVersion40, data: make([]byte, 7), wantErr: true},
		{name: "malformed TLV", version: mbimExVersion40, data: append(validData[:8:8], 1), wantErr: true},
		{name: "reserved MICO indication", version: mbimExVersion40, data: mutateBytes(validData, func(data []byte) {
			binary.LittleEndian.PutUint32(data[0:4], 2)
		}), wantErr: true},
		{name: "reserved DRX parameters", version: mbimExVersion40, data: mutateBytes(validData, func(data []byte) {
			binary.LittleEndian.PutUint32(data[4:8], 6)
		}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NetworkParametersInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MBIMExVersion != tt.version || got.MICOIndication != valid.MICOIndication || got.DRXParameters != valid.DRXParameters ||
				len(got.TLVs) != len(valid.TLVs) || len(got.AllowedNSSAI) != 1 || len(got.TAILists) != 1 {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, valid)
			}
		})
	}
}

func TestWakeReasonInfoCodec(t *testing.T) {
	command := WakeCommand{
		ServiceID: ServiceBasicConnect,
		CID:       CIDConnect,
		Payload:   binary.LittleEndian.AppendUint32(nil, uint32(ActivationCommandActivate)),
	}
	packet := WakePacket{
		FilterID:           7,
		OriginalPacketSize: 1500,
		SavedPacket:        []byte{0x45, 0x00, 0x00, 0x14},
	}
	commandReason := WakeReasonInfo{Type: WakeTypeCIDIndication, SessionID: 3, Command: &command}
	packetReason := WakeReasonInfo{Type: WakeTypePacket, SessionID: 4, Packet: &packet}
	commandData, err := commandReason.MarshalBinary()
	if err != nil {
		t.Fatalf("command reason MarshalBinary() error = %v", err)
	}
	packetData, err := packetReason.MarshalBinary()
	if err != nil {
		t.Fatalf("packet reason MarshalBinary() error = %v", err)
	}
	invalidCommand, err := command.MarshalBinary()
	if err != nil {
		t.Fatalf("command MarshalBinary() error = %v", err)
	}
	binary.LittleEndian.PutUint32(invalidCommand[20:24], 27)

	tests := []struct {
		name        string
		data        []byte
		wantType    WakeType
		wantCommand bool
		wantPacket  bool
		wantErr     bool
	}{
		{name: "CID indication", data: commandData, wantType: WakeTypeCIDIndication, wantCommand: true},
		{name: "packet", data: packetData, wantType: WakeTypePacket, wantPacket: true},
		{name: "truncated", data: make([]byte, 15), wantErr: true},
		{
			name: "wrong TLV type",
			data: append(
				binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, uint32(WakeTypePacket)), 1),
				mbimTLV(TLVTypeWakeCommand, invalidCommand)...,
			),
			wantErr: true,
		},
		{
			name: "command offset points into fixed fields",
			data: append(
				binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, uint32(WakeTypeCIDResponse)), 1),
				mbimTLV(TLVTypeWakeCommand, invalidCommand)...,
			),
			wantErr: true,
		},
		{
			name: "reserved wake type",
			data: append(
				binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, 3), 1),
				mbimTLV(TLVTypeWakePacket, nil)...,
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WakeReasonInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Type != tt.wantType || (got.Command != nil) != tt.wantCommand || (got.Packet != nil) != tt.wantPacket {
				t.Fatalf("UnmarshalBinary() = %+v", got)
			}
			if got.Command != nil && (!bytes.Equal(got.Command.Payload, command.Payload) || got.Command.ServiceID != command.ServiceID || got.Command.CID != command.CID) {
				t.Fatalf("wake command = %+v, want %+v", *got.Command, command)
			}
			if got.Packet != nil && (!bytes.Equal(got.Packet.SavedPacket, packet.SavedPacket) || got.Packet.FilterID != packet.FilterID || got.Packet.OriginalPacketSize != packet.OriginalPacketSize) {
				t.Fatalf("wake packet = %+v, want %+v", *got.Packet, packet)
			}
		})
	}
}

func TestUEPolicyInfoCodec(t *testing.T) {
	rules := []URSPTrafficDescriptor{
		{Precedence: 1, Data: []byte{0x01}},
		{Precedence: 9, Data: []byte{0x10, 0x01, 0x02}},
	}
	rulesData, err := marshalURSPTrafficDescriptors(rules)
	if err != nil {
		t.Fatalf("marshalURSPTrafficDescriptors() error = %v", err)
	}
	valid := TLVs{{Type: TLVTypeURSPRulesTDOnly, Data: rulesData}}
	validData, err := (UEPolicyInfo{TLVs: valid}).MarshalBinary()
	if err != nil {
		t.Fatalf("valid UEPolicyInfo MarshalBinary() error = %v", err)
	}
	emptyData := mbimTLV(TLVTypeURSPRulesTDOnly, nil)
	duplicateData := append(slices.Clone(validData), validData...)
	malformedData := mbimTLV(TLVTypeURSPRulesTDOnly, []byte{1, 0, 2, 0xaa})
	duplicatePrecedence := mbimTLV(TLVTypeURSPRulesTDOnly, []byte{1, 0, 1, 0xaa, 1, 0, 1, 0xbb})

	tests := []struct {
		name        string
		data        []byte
		wantRules   int
		wantPresent bool
		wantErr     bool
	}{
		{name: "rules", data: validData, wantRules: 2, wantPresent: true},
		{name: "no rules", data: emptyData, wantPresent: true},
		{name: "no URSP TLV", data: mbimTLV(TLVTypeSessionID, binary.LittleEndian.AppendUint32(nil, 1)), wantErr: true},
		{name: "duplicate URSP TLV", data: duplicateData, wantErr: true},
		{name: "truncated descriptor", data: malformedData, wantErr: true},
		{name: "duplicate precedence", data: duplicatePrecedence, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got UEPolicyInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.HasURSPRules != tt.wantPresent || len(got.URSPRules) != tt.wantRules {
				t.Fatalf("URSP rules present = %v length = %d, want %v/%d", got.HasURSPRules, len(got.URSPRules), tt.wantPresent, tt.wantRules)
			}
			if len(got.URSPRules) > 0 && (!bytes.Equal(got.URSPRules[1].Data, rules[1].Data) || got.URSPRules[1].Precedence != rules[1].Precedence) {
				t.Fatalf("URSP rule = %+v, want %+v", got.URSPRules[1], rules[1])
			}
		})
	}
}

func TestUEPolicyInfoMarshalBinaryValidation(t *testing.T) {
	tests := []struct {
		name string
		info UEPolicyInfo
	}{
		{name: "missing URSP TLV", info: UEPolicyInfo{TLVs: TLVs{{Type: TLVTypeSessionID, Data: make([]byte, 4)}}}},
		{name: "duplicate URSP TLV", info: UEPolicyInfo{TLVs: TLVs{{Type: TLVTypeURSPRulesTDOnly}, {Type: TLVTypeURSPRulesTDOnly}}}},
		{name: "malformed URSP TLV", info: UEPolicyInfo{TLVs: TLVs{{Type: TLVTypeURSPRulesTDOnly, Data: []byte{1}}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.info.MarshalBinary(); err == nil {
				t.Fatal("MarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestUEPolicyClientVersionValidation(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "MBIMEx 1", version: mbimExVersion10},
		{name: "MBIMEx 2", version: mbimExVersion20},
		{name: "MBIMEx 3", version: mbimExVersion30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{mbimExVersion: tt.version}
			if _, err := client.UEPolicy(context.Background(), nil); err == nil {
				t.Fatal("UEPolicy() error = nil, want non-nil")
			}
		})
	}
}

func mappedPreconfiguredDefaultNSSAITLVForTest() TLV {
	data := binary.LittleEndian.AppendUint32(nil, uint32(AccessType3GPP))
	data = append(data, mbimTLV(TLVTypeDefaultConfiguredNSSAI, []byte{2, 1, 1})...)
	return TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: data}
}
