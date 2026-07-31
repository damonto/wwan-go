package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

func TestLTEAttachRequests(t *testing.T) {
	configuration := LTEAttachConfiguration{
		IPType:       ContextIPTypeIPv4v6,
		Roaming:      LTEAttachRoamingControlHome,
		Source:       ContextSourceOperator,
		AccessString: "internet",
		UserName:     "user",
		Password:     "password",
		Compression:  CompressionNone,
		AuthProtocol: AuthProtocolPAP,
	}
	setData := marshalLTEAttachConfigurationSet(LTEAttachContextOperationDefault, []LTEAttachConfiguration{configuration})

	tests := []struct {
		name        string
		request     *Request
		cid         uint32
		commandType CommandType
		wantData    []byte
	}{
		{
			name:        "configuration query",
			request:     (&LTEAttachConfigurationRequest{TransactionID: 1}).Request(),
			cid:         CIDMsLteAttachConfiguration,
			commandType: CommandTypeQuery,
		},
		{
			name: "configuration set",
			request: (&LTEAttachConfigurationSetRequest{
				TransactionID:  1,
				Operation:      LTEAttachContextOperationDefault,
				Configurations: []LTEAttachConfiguration{configuration},
			}).Request(),
			cid:         CIDMsLteAttachConfiguration,
			commandType: CommandTypeSet,
			wantData:    setData,
		},
		{
			name:        "attach info query",
			request:     (&LTEAttachInfoRequest{TransactionID: 1, MBIMExVersion: mbimExVersion30}).Request(),
			cid:         CIDMsLteAttachInfo,
			commandType: CommandTypeQuery,
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
		})
	}
}

func TestLTEAttachConfigurationCodec(t *testing.T) {
	configurations := []LTEAttachConfiguration{
		{
			IPType:       ContextIPTypeIPv4v6,
			Roaming:      LTEAttachRoamingControlHome,
			Source:       ContextSourceOperator,
			AccessString: "internet",
			UserName:     "user",
			Password:     "password",
			Compression:  CompressionNone,
			AuthProtocol: AuthProtocolPAP,
		},
		{
			IPType:       ContextIPTypeIPv6,
			Roaming:      LTEAttachRoamingControlPartner,
			Source:       ContextSourceAdmin,
			AccessString: "ims",
			Compression:  CompressionEnable,
			AuthProtocol: AuthProtocolNone,
		},
	}
	validData, err := (LTEAttachConfigurationsInfo{Configurations: configurations}).MarshalBinary()
	if err != nil {
		t.Fatalf("valid MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name    string
		data    []byte
		marshal *LTEAttachConfiguration
		wantErr bool
	}{
		{name: "multiple configurations", data: validData},
		{name: "truncated header", data: validData[:3], wantErr: true},
		{
			name: "truncated reference table",
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[:4], 100)
			}),
			wantErr: true,
		},
		{
			name: "string points into fixed fields",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[12:16], 4)
			}),
			wantErr: true,
		},
		{
			name: "odd UTF-16 string",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[16:20], 1)
			}),
			wantErr: true,
		},
		{
			name:    "access string too long",
			marshal: &LTEAttachConfiguration{AccessString: string(make([]rune, 101))},
			wantErr: true,
		},
		{name: "reserved IP type", marshal: &LTEAttachConfiguration{IPType: ContextIPTypeIPv4AndIPv6 + 1}, wantErr: true},
		{name: "reserved roaming control", marshal: &LTEAttachConfiguration{Roaming: LTEAttachRoamingControlNonPartner + 1}, wantErr: true},
		{name: "reserved source", marshal: &LTEAttachConfiguration{Source: ContextSourceDevice + 1}, wantErr: true},
		{name: "reserved compression", marshal: &LTEAttachConfiguration{Compression: CompressionEnable + 1}, wantErr: true},
		{name: "reserved authentication protocol", marshal: &LTEAttachConfiguration{AuthProtocol: AuthProtocolMSCHAPV2 + 1}, wantErr: true},
		{
			name: "reserved IP type in response",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[0:4], uint32(ContextIPTypeIPv4AndIPv6)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved roaming control in response",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[4:8], uint32(LTEAttachRoamingControlNonPartner)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved source in response",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[8:12], uint32(ContextSourceDevice)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved compression in response",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[36:40], uint32(CompressionEnable)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved authentication protocol in response",
			data: mutateLTEAttachElementForTest(validData, func(element []byte) {
				binary.LittleEndian.PutUint32(element[40:44], uint32(AuthProtocolMSCHAPV2)+1)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.marshal != nil {
				_, err := tt.marshal.MarshalBinary()
				if (err != nil) != tt.wantErr {
					t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			var got LTEAttachConfigurationsInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got.Configurations) != len(configurations) {
				t.Fatalf("configuration count = %d, want %d", len(got.Configurations), len(configurations))
			}
			for index := range configurations {
				if got.Configurations[index] != configurations[index] {
					t.Fatalf("configuration %d = %+v, want %+v", index, got.Configurations[index], configurations[index])
				}
			}
		})
	}
}

func TestLTEAttachInfoUnmarshalBinaryVersions(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    LTEAttachInfo
		wantErr bool
	}{
		{
			name:    "legacy",
			version: mbimExVersion20,
			data:    lteAttachInfoPayloadForTest(mbimExVersion20, 0),
			want: LTEAttachInfo{
				State:        LTEAttachStateAttached,
				IPType:       ContextIPTypeIPv4v6,
				AccessString: "internet",
				UserName:     "user",
				Password:     "password",
				Compression:  CompressionNone,
				AuthProtocol: AuthProtocolPAP,
			},
		},
		{
			name:    "MBIMEx 3",
			version: mbimExVersion30,
			data:    lteAttachInfoPayloadForTest(mbimExVersion30, 42),
			want: LTEAttachInfo{
				State:        LTEAttachStateAttached,
				NwError:      42,
				IPType:       ContextIPTypeIPv4v6,
				AccessString: "internet",
				UserName:     "user",
				Password:     "password",
				Compression:  CompressionNone,
				AuthProtocol: AuthProtocolPAP,
			},
		},
		{name: "legacy truncated", version: mbimExVersion20, data: make([]byte, 39), wantErr: true},
		{name: "MBIMEx 3 truncated", version: mbimExVersion30, data: make([]byte, 43), wantErr: true},
		{
			name:    "MBIMEx 3 invalid string reference",
			version: mbimExVersion30,
			data: mutateBytes(lteAttachInfoPayloadForTest(mbimExVersion30, 0), func(data []byte) {
				binary.LittleEndian.PutUint32(data[12:16], 8)
			}),
			wantErr: true,
		},
		{
			name:    "reserved attach state",
			version: mbimExVersion30,
			data: mutateBytes(lteAttachInfoPayloadForTest(mbimExVersion30, 0), func(data []byte) {
				binary.LittleEndian.PutUint32(data[0:4], uint32(LTEAttachStateAttached)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved IP type",
			version: mbimExVersion30,
			data: mutateBytes(lteAttachInfoPayloadForTest(mbimExVersion30, 0), func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(ContextIPTypeIPv4AndIPv6)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved compression",
			version: mbimExVersion30,
			data: mutateBytes(lteAttachInfoPayloadForTest(mbimExVersion30, 0), func(data []byte) {
				binary.LittleEndian.PutUint32(data[36:40], uint32(CompressionEnable)+1)
			}),
			wantErr: true,
		},
		{
			name:    "reserved authentication protocol",
			version: mbimExVersion30,
			data: mutateBytes(lteAttachInfoPayloadForTest(mbimExVersion30, 0), func(data []byte) {
				binary.LittleEndian.PutUint32(data[40:44], uint32(AuthProtocolMSCHAPV2)+1)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LTEAttachInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			tt.want.MBIMExVersion = tt.version
			if got != tt.want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSetLTEAttachConfigurationsInputValidation(t *testing.T) {
	tests := []struct {
		name           string
		operation      LTEAttachContextOperation
		configurations []LTEAttachConfiguration
	}{
		{name: "reserved operation", operation: LTEAttachContextOperationRestoreFactory + 1},
		{name: "reserved configuration field", configurations: []LTEAttachConfiguration{{Source: ContextSourceDevice + 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := new(Client).SetLTEAttachConfigurations(context.Background(), tt.operation, tt.configurations); err == nil {
				t.Fatal("SetLTEAttachConfigurations() error = nil, want non-nil")
			}
		})
	}
}

func mutateLTEAttachElementForTest(data []byte, mutate func([]byte)) []byte {
	result := mutateBytes(data, func([]byte) {})
	offset := binary.LittleEndian.Uint32(result[4:8])
	size := binary.LittleEndian.Uint32(result[8:12])
	mutate(result[offset : offset+size])
	return result
}

func lteAttachInfoPayloadForTest(version uint16, nwError uint32) []byte {
	fixedLength := 40
	refOffsets := []int{8, 16, 24}
	ipTypeOffset := 4
	compressionOffset := 32
	authOffset := 36
	if version >= mbimExVersion30 {
		fixedLength = 44
		refOffsets = []int{12, 20, 28}
		ipTypeOffset = 8
		compressionOffset = 36
		authOffset = 40
	}
	data := make([]byte, fixedLength)
	binary.LittleEndian.PutUint32(data[0:4], uint32(LTEAttachStateAttached))
	if version >= mbimExVersion30 {
		binary.LittleEndian.PutUint32(data[4:8], nwError)
	}
	binary.LittleEndian.PutUint32(data[ipTypeOffset:ipTypeOffset+4], uint32(ContextIPTypeIPv4v6))
	binary.LittleEndian.PutUint32(data[compressionOffset:compressionOffset+4], uint32(CompressionNone))
	binary.LittleEndian.PutUint32(data[authOffset:authOffset+4], uint32(AuthProtocolPAP))
	for index, value := range []string{"internet", "user", "password"} {
		data = appendRefValue(data, refOffsets[index], utf16Bytes(value))
	}
	return data
}
