package mbim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestMSBasicConnectExtensionRequests(t *testing.T) {
	pco := PCOValue{SessionID: 3, Type: PCOTypePartial, Data: []byte{0x80, 0x00}}
	pcoData, err := pco.MarshalBinary()
	if err != nil {
		t.Fatalf("PCO MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name        string
		request     *Request
		cid         uint32
		commandType CommandType
		wantData    []byte
	}{
		{
			name:        "system capabilities",
			request:     (&SystemCapabilitiesRequest{TransactionID: 1}).Request(),
			cid:         CIDMsSystemCapabilities,
			commandType: CommandTypeQuery,
		},
		{
			name:        "slot info status",
			request:     (&SlotInfoStatusRequest{TransactionID: 1, SlotIndex: 2}).Request(),
			cid:         CIDMsSlotInfoStatus,
			commandType: CommandTypeQuery,
			wantData:    binary.LittleEndian.AppendUint32(nil, 2),
		},
		{
			name:        "device slot mappings query",
			request:     (&DeviceSlotMappingsRequest{TransactionID: 1}).Request(),
			cid:         CIDDeviceSlotMappings,
			commandType: CommandTypeQuery,
		},
		{
			name: "device slot mappings set",
			request: (&DeviceSlotMappingsSetRequest{
				TransactionID: 1,
				SlotMappings:  []SlotMapping{{Slot: 2}},
			}).Request(),
			cid:         CIDDeviceSlotMappings,
			commandType: CommandTypeSet,
			wantData:    slotMappingsPayload(2),
		},
		{
			name:        "PCO",
			request:     (&PCORequest{TransactionID: 1, Value: pco}).Request(),
			cid:         CIDMsPCO,
			commandType: CommandTypeQuery,
			wantData:    pcoData,
		},
		{
			name:        "device reset",
			request:     (&DeviceResetRequest{TransactionID: 1}).Request(),
			cid:         CIDMsDeviceReset,
			commandType: CommandTypeSet,
		},
		{
			name:        "location info status",
			request:     (&LocationInfoStatusRequest{TransactionID: 1}).Request(),
			cid:         CIDMsLocationInfoStatus,
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
			if tt.request.Timeout != mbimCIDResponseTimeout {
				t.Fatalf("timeout = %v, want %v", tt.request.Timeout, mbimCIDResponseTimeout)
			}
		})
	}
}

func TestSystemCapabilitiesInfoUnmarshalBinary(t *testing.T) {
	valid := make([]byte, 20)
	binary.LittleEndian.PutUint32(valid[0:4], 2)
	binary.LittleEndian.PutUint32(valid[4:8], 3)
	binary.LittleEndian.PutUint32(valid[8:12], 1)
	binary.LittleEndian.PutUint64(valid[12:20], 0x1122334455667788)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{name: "truncated", data: valid[:19], wantErr: true},
		{name: "trailing data", data: append(bytes.Clone(valid), 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SystemCapabilitiesInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := SystemCapabilitiesInfo{Executors: 2, Slots: 3, Concurrency: 1, ModemID: 0x1122334455667788}
			if got != want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSlotInfoStatusUnmarshalBinary(t *testing.T) {
	valid := binary.LittleEndian.AppendUint32(nil, 2)
	valid = binary.LittleEndian.AppendUint32(valid, uint32(UICCSlotStateActiveESIM))

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "active eSIM", data: valid},
		{
			name: "reserved state",
			data: func() []byte {
				data := bytes.Clone(valid)
				binary.LittleEndian.PutUint32(data[4:8], uint32(UICCSlotStateActiveESIMNoProfiles+1))
				return data
			}(),
			wantErr: true,
		},
		{name: "truncated", data: valid[:7], wantErr: true},
		{name: "trailing data", data: append(bytes.Clone(valid), 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SlotInfoStatus
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.SlotIndex != 2 || got.State != UICCSlotStateActiveESIM {
				t.Fatalf("UnmarshalBinary() = %+v", got)
			}
		})
	}
}

func TestPCOValueCodec(t *testing.T) {
	valid := PCOValue{SessionID: 4, Type: PCOTypeComplete, Data: []byte{0x80, 0x00, 0x0d}}
	validData, err := valid.MarshalBinary()
	if err != nil {
		t.Fatalf("valid MarshalBinary() error = %v", err)
	}
	if len(validData) != 16 {
		t.Fatalf("MarshalBinary() length = %d, want 16", len(validData))
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "round trip", data: validData},
		{
			name: "nonzero padding",
			data: mutateBytes(validData, func(data []byte) {
				data[15] = 0xff
			}),
		},
		{name: "truncated", data: validData[:11], wantErr: true},
		{
			name: "declared data too long",
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 5)
			}),
			wantErr: true,
		},
		{
			name:    "missing final padding",
			data:    validData[:15],
			wantErr: true,
		},
		{
			name:    "trailing data",
			data:    append(bytes.Clone(validData), make([]byte, 4)...),
			wantErr: true,
		},
		{
			name: "zero size with trailing data",
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 0)
			}),
			wantErr: true,
		},
		{
			name: "invalid PCO type",
			data: mutateBytes(validData, func(data []byte) {
				binary.LittleEndian.PutUint32(data[8:12], uint32(PCOTypePartial)+1)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PCOValue
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.SessionID != valid.SessionID || got.Type != valid.Type || !bytes.Equal(got.Data, valid.Data) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, valid)
			}
		})
	}
}

func TestPCOValueMarshalBinaryValidation(t *testing.T) {
	tests := []struct {
		name    string
		typ     PCOType
		wantErr bool
	}{
		{name: "complete", typ: PCOTypeComplete},
		{name: "partial", typ: PCOTypePartial},
		{name: "invalid", typ: PCOTypePartial + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (PCOValue{Type: tt.typ}).MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeviceResetResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "empty"},
		{name: "unexpected data", data: []byte{0}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DeviceResetResponse
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocationInfoStatusUnmarshalBinary(t *testing.T) {
	valid := binary.LittleEndian.AppendUint32(nil, 0x1234)
	valid = binary.LittleEndian.AppendUint32(valid, 0x10203)
	valid = binary.LittleEndian.AppendUint32(valid, 0xabcdef)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid", data: valid},
		{name: "truncated", data: valid[:11], wantErr: true},
		{name: "trailing data", data: append(bytes.Clone(valid), 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LocationInfoStatus
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := LocationInfoStatus{LocationAreaCode: 0x1234, TrackingAreaCode: 0x10203, CellID: 0xabcdef}
			if got != want {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", got, want)
			}
		})
	}
}
