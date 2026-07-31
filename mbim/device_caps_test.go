package mbim

import (
	"encoding/binary"
	"slices"
	"testing"
)

func TestDeviceCapsV2Request(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "MBIMEx 2", version: mbimExVersion20},
		{name: "MBIMEx 4", version: mbimExVersion40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := (&DeviceCapsV2Request{
				TransactionID: 7,
				MBIMExVersion: tt.version,
			}).Request()
			command := request.Command.(*Command)
			if command.ServiceID != ServiceMsBasicConnectExtensions || command.CommandID != CIDMsDeviceCapsV2 || command.CommandType != CommandTypeQuery {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if len(command.Data) != 0 {
				t.Fatalf("command data length = %d, want 0", len(command.Data))
			}
			if request.Response.(*DeviceCapsInfo).MBIMExVersion != tt.version {
				t.Fatalf("response version = %#x, want %#x", request.Response.(*DeviceCapsInfo).MBIMExVersion, tt.version)
			}
		})
	}
}

func TestDeviceCapsInfoUnmarshalBinaryVersions(t *testing.T) {
	v2 := deviceCapsPayloadV2ForTest(4, "custom", "device", "firmware", "hardware")
	v3 := deviceCapsPayloadV3ForTest(8)
	binary.LittleEndian.PutUint32(v3[40:44], 2)
	binary.LittleEndian.PutUint32(v3[44:48], 0x15)
	v3 = deviceCapsPayloadV3WithValuesForTest(
		8,
		[]uint16{1, 3, 78},
		[]uint16{1, 257},
		"custom",
		"device",
		"firmware",
		"hardware",
	)

	tests := []struct {
		name    string
		version uint16
		data    []byte
		want    DeviceCapsInfo
		wantErr bool
	}{
		{
			name:    "MBIMEx 2",
			version: mbimExVersion20,
			data:    v2,
			want: DeviceCapsInfo{
				MaxSessions:     4,
				ExecutorIndex:   3,
				CustomDataClass: "custom",
				DeviceID:        "device",
				FirmwareInfo:    "firmware",
				HardwareInfo:    "hardware",
			},
		},
		{
			name:    "MBIMEx 3",
			version: mbimExVersion30,
			data:    v3,
			want: DeviceCapsInfo{
				DataSubclass:    DataSubclass5GNR,
				MaxSessions:     8,
				ExecutorIndex:   2,
				WCDMABandClass:  0x15,
				LTEBandClasses:  []uint16{1, 3, 78},
				NRBandClasses:   []uint16{1, 257},
				CustomDataClass: "custom",
				DeviceID:        "device",
				FirmwareInfo:    "firmware",
				HardwareInfo:    "hardware",
			},
		},
		{name: "MBIMEx 2 truncated", version: mbimExVersion20, data: make([]byte, 67), wantErr: true},
		{name: "MBIMEx 3 truncated", version: mbimExVersion30, data: make([]byte, 47), wantErr: true},
		{
			name:    "MBIMEx 2 string points into header",
			version: mbimExVersion20,
			data: mutateBytes(v2, func(data []byte) {
				binary.LittleEndian.PutUint32(data[32:36], 4)
			}),
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 missing named TLV",
			version: mbimExVersion30,
			data:    v3[:len(v3)-8],
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 wrong named TLV type",
			version: mbimExVersion30,
			data: mutateBytes(v3, func(data []byte) {
				binary.LittleEndian.PutUint16(data[48:50], uint16(TLVTypePCO))
			}),
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 odd LTE table",
			version: mbimExVersion30,
			data: deviceCapsPayloadV3RawForTest(
				mbimTLV(TLVTypeUint16Table, []byte{1}),
				mbimTLV(TLVTypeUint16Table, nil),
				mbimTLV(TLVTypeWCharString, nil),
				mbimTLV(TLVTypeWCharString, nil),
				mbimTLV(TLVTypeWCharString, nil),
				mbimTLV(TLVTypeWCharString, nil),
			),
			wantErr: true,
		},
		{
			name:    "MBIMEx 3 odd string",
			version: mbimExVersion30,
			data: deviceCapsPayloadV3RawForTest(
				mbimTLV(TLVTypeUint16Table, nil),
				mbimTLV(TLVTypeUint16Table, nil),
				mbimTLV(TLVTypeWCharString, []byte{1}),
				mbimTLV(TLVTypeWCharString, nil),
				mbimTLV(TLVTypeWCharString, nil),
				mbimTLV(TLVTypeWCharString, nil),
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeviceCapsInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.MBIMExVersion != tt.version ||
				got.MaxSessions != tt.want.MaxSessions ||
				got.ExecutorIndex != tt.want.ExecutorIndex ||
				got.WCDMABandClass != tt.want.WCDMABandClass ||
				got.DataSubclass != tt.want.DataSubclass ||
				got.CustomDataClass != tt.want.CustomDataClass ||
				got.DeviceID != tt.want.DeviceID ||
				got.FirmwareInfo != tt.want.FirmwareInfo ||
				got.HardwareInfo != tt.want.HardwareInfo {
				t.Fatalf("UnmarshalBinary() = %+v, want selected fields %+v", got, tt.want)
			}
			if !slices.Equal(got.LTEBandClasses, tt.want.LTEBandClasses) {
				t.Fatalf("LTEBandClasses = %v, want %v", got.LTEBandClasses, tt.want.LTEBandClasses)
			}
			if !slices.Equal(got.NRBandClasses, tt.want.NRBandClasses) {
				t.Fatalf("NRBandClasses = %v, want %v", got.NRBandClasses, tt.want.NRBandClasses)
			}
		})
	}
}

func deviceCapsPayloadV2ForTest(maxSessions uint32, values ...string) []byte {
	data := make([]byte, 68)
	binary.LittleEndian.PutUint32(data[28:32], maxSessions)
	binary.LittleEndian.PutUint32(data[64:68], 3)
	for i, value := range values {
		data = appendRefValue(data, 32+i*8, utf16Bytes(value))
	}
	return data
}

func deviceCapsPayloadV3WithValuesForTest(maxSessions uint32, lte, nr []uint16, values ...string) []byte {
	data := make([]byte, 48)
	binary.LittleEndian.PutUint32(data[16:20], uint32(DataClass5G))
	binary.LittleEndian.PutUint64(data[28:36], uint64(DataSubclass5GNR))
	binary.LittleEndian.PutUint32(data[36:40], maxSessions)
	binary.LittleEndian.PutUint32(data[40:44], 2)
	binary.LittleEndian.PutUint32(data[44:48], 0x15)
	data = append(data, mbimTLV(TLVTypeUint16Table, uint16TableForTest(lte))...)
	data = append(data, mbimTLV(TLVTypeUint16Table, uint16TableForTest(nr))...)
	for _, value := range values {
		data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(value))...)
	}
	return data
}

func deviceCapsPayloadV3RawForTest(tlvs ...[]byte) []byte {
	data := make([]byte, 48)
	for _, tlv := range tlvs {
		data = append(data, tlv...)
	}
	return data
}

func uint16TableForTest(values []uint16) []byte {
	var data []byte
	for _, value := range values {
		data = binary.LittleEndian.AppendUint16(data, value)
	}
	return data
}

func mutateBytes(data []byte, mutate func([]byte)) []byte {
	result := slices.Clone(data)
	mutate(result)
	return result
}
