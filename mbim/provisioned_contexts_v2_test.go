package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"
)

func TestSNSSAICodec(t *testing.T) {
	tests := []struct {
		name  string
		value SNSSAI
		want  []byte
	}{
		{
			name:  "SST",
			value: SNSSAI{SliceServiceType: 1},
			want:  []byte{1, 1},
		},
		{
			name: "SST and mapped SST",
			value: SNSSAI{
				SliceServiceType:          1,
				MappedSliceServiceType:    2,
				HasMappedSliceServiceType: true,
			},
			want: []byte{2, 1, 2},
		},
		{
			name: "SST and SD",
			value: SNSSAI{
				SliceServiceType:       1,
				SliceDifferentiator:    [3]byte{2, 3, 4},
				HasSliceDifferentiator: true,
			},
			want: []byte{4, 1, 2, 3, 4},
		},
		{
			name: "SST SD and mapped SST",
			value: SNSSAI{
				SliceServiceType:          1,
				SliceDifferentiator:       [3]byte{2, 3, 4},
				HasSliceDifferentiator:    true,
				MappedSliceServiceType:    5,
				HasMappedSliceServiceType: true,
			},
			want: []byte{5, 1, 2, 3, 4, 5},
		},
		{
			name: "complete mapped S-NSSAI",
			value: SNSSAI{
				SliceServiceType:             1,
				SliceDifferentiator:          [3]byte{2, 3, 4},
				HasSliceDifferentiator:       true,
				MappedSliceServiceType:       5,
				HasMappedSliceServiceType:    true,
				MappedSliceDifferentiator:    [3]byte{6, 7, 8},
				HasMappedSliceDifferentiator: true,
			},
			want: []byte{8, 1, 2, 3, 4, 5, 6, 7, 8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.value.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(data, tt.want) {
				t.Fatalf("MarshalBinary() = %x, want %x", data, tt.want)
			}

			var got SNSSAI
			if err := got.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got != tt.value {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.value)
			}
		})
	}
}

func TestSNSSAIRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		value   *SNSSAI
		data    []byte
		wantErr bool
	}{
		{name: "empty payload", data: nil, wantErr: true},
		{name: "reserved length", data: []byte{3, 1, 2, 3}, wantErr: true},
		{name: "truncated contents", data: []byte{4, 1, 2, 3}, wantErr: true},
		{name: "trailing contents", data: []byte{1, 1, 2}, wantErr: true},
		{
			name: "mapped SD without SD",
			value: &SNSSAI{
				MappedSliceServiceType:       1,
				HasMappedSliceServiceType:    true,
				MappedSliceDifferentiator:    [3]byte{1, 2, 3},
				HasMappedSliceDifferentiator: true,
			},
			wantErr: true,
		},
		{
			name: "mapped SD without mapped SST",
			value: &SNSSAI{
				SliceDifferentiator:          [3]byte{1, 2, 3},
				HasSliceDifferentiator:       true,
				MappedSliceDifferentiator:    [3]byte{4, 5, 6},
				HasMappedSliceDifferentiator: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.value != nil {
				_, err = tt.value.MarshalBinary()
			} else {
				var value SNSSAI
				err = value.UnmarshalBinary(tt.data)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("codec error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSingleNSSAITLV(t *testing.T) {
	value := SNSSAI{
		SliceServiceType:       1,
		SliceDifferentiator:    [3]byte{2, 3, 4},
		HasSliceDifferentiator: true,
	}
	tests := []struct {
		name    string
		value   *SNSSAI
		tlv     *TLV
		wantNil bool
		wantErr bool
	}{
		{name: "empty", value: nil, wantNil: true},
		{name: "value", value: &value},
		{name: "wrong type", tlv: &TLV{Type: TLVTypePCO}, wantErr: true},
		{name: "invalid data", tlv: &TLV{Type: TLVTypeSingleNSSAI, Data: []byte{3}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv := TLV{}
			if tt.tlv != nil {
				tlv = *tt.tlv
			} else {
				var err error
				tlv, err = NewSingleNSSAITLV(tt.value)
				if err != nil {
					t.Fatalf("NewSingleNSSAITLV() error = %v", err)
				}
			}

			var got OptionalSNSSAI
			err := got.UnmarshalTLV(tlv)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalTLV() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if (got.Value == nil) != tt.wantNil {
				t.Fatalf("UnmarshalTLV() = %+v, wantNil %v", got.Value, tt.wantNil)
			}
			if got.Value != nil && *got.Value != *tt.value {
				t.Fatalf("UnmarshalTLV() = %+v, want %+v", *got.Value, *tt.value)
			}
		})
	}
}

func TestPreconfiguredDefaultNSSAITLV(t *testing.T) {
	tests := []struct {
		name     string
		values   []PreconfiguredDefaultNSSAI
		wantData []byte
	}{
		{
			name: "3GPP eMBB",
			values: []PreconfiguredDefaultNSSAI{{
				AccessType:     AccessType3GPP,
				PreferredNSSAI: []SNSSAI{{SliceServiceType: 1}},
			}},
			wantData: []byte{
				1, 0, 0, 0,
				5, 0, 0, 2, 2, 0, 0, 0,
				1, 1, 0, 0,
			},
		},
		{
			name: "both access types",
			values: []PreconfiguredDefaultNSSAI{
				{
					AccessType: AccessType3GPP,
					PreferredNSSAI: []SNSSAI{
						{SliceServiceType: 1},
						{SliceServiceType: 2},
					},
				},
				{AccessType: AccessTypeNon3GPP},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := NewPreconfiguredDefaultNSSAITLV(tt.values)
			if err != nil {
				t.Fatalf("NewPreconfiguredDefaultNSSAITLV() error = %v", err)
			}
			if tt.wantData != nil && !bytes.Equal(tlv.Data, tt.wantData) {
				t.Fatalf("TLV data = %x, want %x", tlv.Data, tt.wantData)
			}

			var got PreconfiguredDefaultNSSAIList
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if len(got) != len(tt.values) {
				t.Fatalf("access list count = %d, want %d", len(got), len(tt.values))
			}
			for index := range got {
				if got[index].AccessType != tt.values[index].AccessType || !slices.Equal(got[index].PreferredNSSAI, tt.values[index].PreferredNSSAI) {
					t.Fatalf("access list %d = %+v, want %+v", index, got[index], tt.values[index])
				}
			}
		})
	}
}

func TestPreconfiguredDefaultNSSAITLVRejectsInvalidValues(t *testing.T) {
	entry := func(accessType AccessType, preferredType TLVType, preferred []byte) []byte {
		data := binary.LittleEndian.AppendUint32(nil, uint32(accessType))
		return append(data, mbimTLV(preferredType, preferred)...)
	}
	validEntry := entry(AccessType3GPP, TLVTypeDefaultConfiguredNSSAI, []byte{1, 1})

	marshalTests := []struct {
		name   string
		values []PreconfiguredDefaultNSSAI
	}{
		{name: "empty"},
		{name: "more than two", values: make([]PreconfiguredDefaultNSSAI, 3)},
		{name: "unknown access type", values: []PreconfiguredDefaultNSSAI{{AccessType: AccessTypeUnknown}}},
		{
			name: "duplicate access type",
			values: []PreconfiguredDefaultNSSAI{
				{AccessType: AccessType3GPP},
				{AccessType: AccessType3GPP},
			},
		},
		{
			name: "invalid S-NSSAI",
			values: []PreconfiguredDefaultNSSAI{{
				AccessType: AccessType3GPP,
				PreferredNSSAI: []SNSSAI{{
					HasMappedSliceDifferentiator: true,
				}},
			}},
		},
	}
	for _, tt := range marshalTests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			if _, err := NewPreconfiguredDefaultNSSAITLV(tt.values); err == nil {
				t.Fatal("NewPreconfiguredDefaultNSSAITLV() error = nil, want non-nil")
			}
		})
	}

	parseTests := []struct {
		name string
		tlv  TLV
	}{
		{name: "wrong outer type", tlv: TLV{Type: TLVTypePCO}},
		{name: "empty access list", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI}},
		{name: "truncated access type", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: []byte{1, 0, 0}}},
		{name: "unknown access type", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: entry(AccessTypeUnknown, TLVTypeDefaultConfiguredNSSAI, nil)}},
		{name: "wrong preferred type", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: entry(AccessType3GPP, TLVTypePCO, nil)}},
		{name: "invalid S-NSSAI", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: entry(AccessType3GPP, TLVTypeDefaultConfiguredNSSAI, []byte{3, 1, 2, 3})}},
		{name: "duplicate access type", tlv: TLV{Type: TLVTypePreconfiguredDefaultConfiguredNSSAI, Data: append(slices.Clone(validEntry), validEntry...)}},
		{
			name: "more than two access lists",
			tlv: TLV{
				Type: TLVTypePreconfiguredDefaultConfiguredNSSAI,
				Data: append(
					append(slices.Clone(validEntry), entry(AccessTypeNon3GPP, TLVTypeDefaultConfiguredNSSAI, nil)...),
					validEntry...,
				),
			},
		},
	}
	for _, tt := range parseTests {
		t.Run("parse "+tt.name, func(t *testing.T) {
			var got PreconfiguredDefaultNSSAIList
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}

func TestProvisionedContextsV2Codec(t *testing.T) {
	snssai := &SNSSAI{
		SliceServiceType:       1,
		SliceDifferentiator:    [3]byte{2, 3, 4},
		HasSliceDifferentiator: true,
	}
	base := ProvisionedContextV2{
		ContextID:    7,
		ContextType:  ContextTypeInternet,
		IPType:       ContextIPTypeIPv4v6,
		State:        ContextStateEnabled,
		Roaming:      ContextRoamingControlHomeOnly,
		MediaType:    ContextMediaTypeCellularOnly,
		Source:       ContextSourceOperator,
		AccessString: "internet",
		UserName:     "user",
		Password:     "password",
		Compression:  CompressionNone,
		AuthProtocol: AuthProtocolPAP,
	}

	tests := []struct {
		name    string
		version uint16
		context ProvisionedContextV2
	}{
		{name: "MBIMEx 2", version: mbimExVersion20, context: base},
		{name: "MBIMEx 4 empty S-NSSAI", version: mbimExVersion40, context: base},
		{
			name:    "MBIMEx 4 S-NSSAI",
			version: mbimExVersion40,
			context: func() ProvisionedContextV2 {
				value := base
				value.SNSSAI = snssai
				return value
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.context
			want.MBIMExVersion = tt.version
			info := ProvisionedContextsV2Info{
				MBIMExVersion: tt.version,
				Contexts:      []ProvisionedContextV2{tt.context},
			}
			data, err := info.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}

			var got ProvisionedContextsV2Info
			got.MBIMExVersion = tt.version
			if err := got.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(got.Contexts) != 1 {
				t.Fatalf("context count = %d, want 1", len(got.Contexts))
			}
			if !equalProvisionedContextV2(got.Contexts[0], want) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got.Contexts[0], want)
			}
		})
	}
}

func TestProvisionedContextV2Validation(t *testing.T) {
	valid := ProvisionedContextV2{
		MBIMExVersion: mbimExVersion40,
		ContextType:   ContextTypeInternet,
		AccessString:  "internet",
		SNSSAI:        &SNSSAI{SliceServiceType: 1},
	}
	validData, err := valid.MarshalBinary()
	if err != nil {
		t.Fatalf("valid MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name    string
		version uint16
		data    []byte
		value   *ProvisionedContextV2
		wantErr bool
	}{
		{name: "truncated fixed fields", version: mbimExVersion40, data: make([]byte, 71), wantErr: true},
		{name: "missing S-NSSAI TLV", version: mbimExVersion40, data: make([]byte, 72), wantErr: true},
		{
			name:    "wrong named TLV",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint16(data[72:74], uint16(TLVTypePCO))
			}),
			wantErr: true,
		},
		{
			name:    "string points into fixed fields",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[40:44], 4)
			}),
			wantErr: true,
		},
		{
			name:    "string points into S-NSSAI TLV",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[40:44], 72)
			}),
			wantErr: true,
		},
		{
			name: "S-NSSAI before MBIMEx 4",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion30,
				SNSSAI:        &SNSSAI{SliceServiceType: 1},
			},
			wantErr: true,
		},
		{
			name: "access string too long",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				AccessString:  string(make([]rune, 101)),
			},
			wantErr: true,
		},
		{
			name: "reserved IP type",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				IPType:        ContextIPTypeIPv4AndIPv6 + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved state",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				State:         ContextStateEnabled + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved roaming control",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				Roaming:       ContextRoamingControlAllowAll + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved media type",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				MediaType:     ContextMediaTypeAll + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved source",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				Source:        ContextSourceDevice + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved compression",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				Compression:   CompressionEnable + 1,
			},
			wantErr: true,
		},
		{
			name: "reserved authentication protocol",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				AuthProtocol:  AuthProtocolMSCHAPV2 + 1,
			},
			wantErr: true,
		},
		{
			name: "mapped S-NSSAI",
			value: &ProvisionedContextV2{
				MBIMExVersion: mbimExVersion40,
				SNSSAI: &SNSSAI{
					SliceServiceType:          1,
					MappedSliceServiceType:    1,
					HasMappedSliceServiceType: true,
				},
			},
			wantErr: true,
		},
		{
			name:    "reserved IP type in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[20:24], uint32(ContextIPTypeIPv4AndIPv6)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved state in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[24:28], uint32(ContextStateEnabled)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved roaming control in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[28:32], uint32(ContextRoamingControlAllowAll)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved media type in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[32:36], uint32(ContextMediaTypeAll)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved source in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[36:40], uint32(ContextSourceDevice)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved compression in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[64:68], uint32(CompressionEnable)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved authentication protocol in response",
			version: mbimExVersion40,
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[68:72], uint32(AuthProtocolMSCHAPV2)+1)
			}),
			wantErr: true,
		},
		{
			name:    "mapped S-NSSAI in response",
			version: mbimExVersion40,
			data: marshalProvisionedContextV2(1, ProvisionedContextV2{
				SNSSAI: &SNSSAI{
					SliceServiceType:          1,
					MappedSliceServiceType:    1,
					HasMappedSliceServiceType: true,
				},
			}, mbimExVersion40),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.value != nil {
				_, err = tt.value.MarshalBinary()
			} else {
				value := ProvisionedContextV2{MBIMExVersion: tt.version}
				err = value.UnmarshalBinary(tt.data)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("codec error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetProvisionedContextV2InputValidation(t *testing.T) {
	tests := []struct {
		name      string
		operation ContextOperation
		context   ProvisionedContextV2
	}{
		{name: "reserved operation", operation: ContextOperationRestoreFactory + 1},
		{name: "reserved context field", context: ProvisionedContextV2{IPType: ContextIPTypeIPv4AndIPv6 + 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{mbimExVersion: mbimExVersion40}
			if _, err := client.SetProvisionedContextV2(context.Background(), tt.operation, tt.context); err == nil {
				t.Fatal("SetProvisionedContextV2() error = nil, want non-nil")
			}
		})
	}
}

func TestProvisionedContextsV2Request(t *testing.T) {
	context := ProvisionedContextV2{ContextID: 99, ContextType: ContextTypeIMS}
	tests := []struct {
		name        string
		request     *Request
		commandType CommandType
		wantFirst   uint32
	}{
		{
			name:        "query",
			request:     (&ProvisionedContextsV2Request{TransactionID: 1, MBIMExVersion: mbimExVersion40}).Request(),
			commandType: CommandTypeQuery,
		},
		{
			name: "set operation replaces context ID",
			request: (&ProvisionedContextV2SetRequest{
				TransactionID: 1,
				MBIMExVersion: mbimExVersion40,
				Operation:     ContextOperationDelete,
				Context:       context,
			}).Request(),
			commandType: CommandTypeSet,
			wantFirst:   uint32(ContextOperationDelete),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.request.Command.(*Command)
			if command.ServiceID != ServiceMsBasicConnectExtensions || command.CommandID != CIDMsProvisionedContexts || command.CommandType != tt.commandType {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if tt.commandType == CommandTypeQuery {
				if len(command.Data) != 0 {
					t.Fatalf("query data = %x, want empty", command.Data)
				}
				return
			}
			if got := binary.LittleEndian.Uint32(command.Data[:4]); got != tt.wantFirst {
				t.Fatalf("first field = %d, want operation %d", got, tt.wantFirst)
			}
		})
	}
}

func TestProvisionedContextsV2InfoRejectsTruncatedReferenceTable(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "missing count", data: make([]byte, 3)},
		{name: "truncated table", data: binary.LittleEndian.AppendUint32(nil, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info ProvisionedContextsV2Info
			if err := info.UnmarshalBinary(tt.data); err == nil {
				t.Fatal("UnmarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func equalProvisionedContextV2(got, want ProvisionedContextV2) bool {
	if got.MBIMExVersion != want.MBIMExVersion ||
		got.ContextID != want.ContextID ||
		got.ContextType != want.ContextType ||
		got.IPType != want.IPType ||
		got.State != want.State ||
		got.Roaming != want.Roaming ||
		got.MediaType != want.MediaType ||
		got.Source != want.Source ||
		got.AccessString != want.AccessString ||
		got.UserName != want.UserName ||
		got.Password != want.Password ||
		got.Compression != want.Compression ||
		got.AuthProtocol != want.AuthProtocol ||
		(got.SNSSAI == nil) != (want.SNSSAI == nil) {
		return false
	}
	return got.SNSSAI == nil || *got.SNSSAI == *want.SNSSAI
}
