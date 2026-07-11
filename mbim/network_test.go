package mbim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNetworkQueryRequests(t *testing.T) {
	tests := []struct {
		name string
		req  *Request
		cid  uint32
	}{
		{"registration state", (&RegistrationStateRequest{TransactionID: 1}).Request(), CIDRegisterState},
		{"provisioned contexts", (&ProvisionedContextsRequest{TransactionID: 1}).Request(), CIDProvisionedContexts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, ok := tt.req.Command.(*Command)
			if !ok {
				t.Fatalf("Command type = %T, want *Command", tt.req.Command)
			}
			if command.ServiceID != ServiceBasicConnect || command.CommandID != tt.cid {
				t.Fatalf("command = (%X, %d), want (%X, %d)", command.ServiceID, command.CommandID, ServiceBasicConnect, tt.cid)
			}
			if command.CommandType != CommandTypeQuery || len(command.Data) != 0 {
				t.Fatalf("query = (type %d, data %X), want empty query", command.CommandType, command.Data)
			}
		})
	}
}

func TestRegistrationStateInfoUnmarshalBinary(t *testing.T) {
	valid := registrationStatePayloadForTest("310410", "AT&T", "Roaming")
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"valid", valid, false},
		{"truncated", valid[:47], true},
		{"invalid string reference", func() []byte {
			b := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(b[20:24], uint32(len(b)+1))
			return b
		}(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got RegistrationStateInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.RegisterState != RegisterStateHome || got.RegisterMode != RegisterModeAutomatic || got.ProviderID != "310410" || got.ProviderName != "AT&T" || got.RoamingText != "Roaming" {
				t.Fatalf("UnmarshalBinary() = %+v", got)
			}
		})
	}
}

func TestProvisionedContextsInfoUnmarshalBinary(t *testing.T) {
	valid := provisionedContextsPayloadForTest(
		provisionedContextPayloadForTest(1, ContextTypeInternet, "internet"),
		provisionedContextPayloadForTest(2, ContextTypeIMS, "ims"),
	)
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"valid", valid, false},
		{"truncated header", valid[:3], true},
		{"truncated reference list", []byte{2, 0, 0, 0}, true},
		{"context reference out of bounds", func() []byte {
			b := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)+1))
			return b
		}(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ProvisionedContextsInfo
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got.Contexts) != 2 || got.Contexts[1].ContextID != 2 || got.Contexts[1].ContextType != ContextTypeIMS || got.Contexts[1].AccessString != "ims" {
				t.Fatalf("UnmarshalBinary() = %+v", got)
			}
		})
	}
}

func registrationStatePayloadForTest(providerID, providerName, roamingText string) []byte {
	values := [][]byte{utf16Bytes(providerID), utf16Bytes(providerName), utf16Bytes(roamingText)}
	data := make([]byte, 48)
	binary.LittleEndian.PutUint32(data[4:8], uint32(RegisterStateHome))
	binary.LittleEndian.PutUint32(data[8:12], uint32(RegisterModeAutomatic))
	offset := 48
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[20+i*8:24+i*8], uint32(offset))
		binary.LittleEndian.PutUint32(data[24+i*8:28+i*8], uint32(len(value)))
		data = append(data, value...)
		offset += len(value)
	}
	return data
}

func provisionedContextsPayloadForTest(contexts ...[]byte) []byte {
	headerSize := 4 + len(contexts)*8
	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data[:4], uint32(len(contexts)))
	offset := headerSize
	for i, context := range contexts {
		binary.LittleEndian.PutUint32(data[4+i*8:8+i*8], uint32(offset))
		binary.LittleEndian.PutUint32(data[8+i*8:12+i*8], uint32(len(context)))
		data = append(data, context...)
		offset += len(context)
	}
	return data
}

func provisionedContextPayloadForTest(id uint32, contextType ContextType, accessString string) []byte {
	access := utf16Bytes(accessString)
	data := make([]byte, 52)
	binary.LittleEndian.PutUint32(data[:4], id)
	copy(data[4:20], contextType[:])
	binary.LittleEndian.PutUint32(data[20:24], 52)
	binary.LittleEndian.PutUint32(data[24:28], uint32(len(access)))
	return append(data, access...)
}
