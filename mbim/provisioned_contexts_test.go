package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProvisionedContextSetRequestData(t *testing.T) {
	tests := []struct {
		name       string
		context    ProvisionedContext
		providerID string
	}{
		{
			name: "all fields",
			context: ProvisionedContext{
				ContextID:    ^uint32(0),
				ContextType:  ContextTypeInternet,
				AccessString: "internet",
				UserName:     "alice",
				Password:     "secret",
				Compression:  CompressionEnable,
				AuthProtocol: AuthProtocolPAP,
			},
			providerID: "310260",
		},
		{
			name: "empty strings",
			context: ProvisionedContext{
				ContextID:   3,
				ContextType: ContextTypeIMS,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &ProvisionedContextSetRequest{
				TransactionID: 7,
				Context:       tt.context,
				ProviderID:    tt.providerID,
			}
			req := request.Request()
			command := req.Command.(*Command)
			if command.ServiceID != ServiceBasicConnect || command.CommandID != CIDProvisionedContexts || command.CommandType != CommandTypeSet {
				t.Fatalf("command = service % X cid %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if req.Timeout != mbimCIDResponseTimeout {
				t.Fatalf("Timeout = %v, want %v", req.Timeout, mbimCIDResponseTimeout)
			}
			want := provisionedContextSetPayloadForTest(tt.context, tt.providerID)
			if !bytes.Equal(command.Data, want) {
				t.Fatalf("Data = %X, want %X", command.Data, want)
			}
			if request.Response == nil {
				t.Fatal("Response = nil, want allocated response")
			}
		})
	}
}

func TestValidateProvisionedContextSet(t *testing.T) {
	tests := []struct {
		name       string
		context    ProvisionedContext
		providerID string
		wantErr    bool
	}{
		{name: "valid", context: ProvisionedContext{AccessString: strings.Repeat("a", 100), UserName: strings.Repeat("u", 255), Password: strings.Repeat("p", 255)}, providerID: "310260"},
		{name: "access string too long", context: ProvisionedContext{AccessString: strings.Repeat("a", 101)}, wantErr: true},
		{name: "user name too long", context: ProvisionedContext{UserName: strings.Repeat("u", 256)}, wantErr: true},
		{name: "password too long", context: ProvisionedContext{Password: strings.Repeat("p", 256)}, wantErr: true},
		{name: "provider ID too long", providerID: "1234567", wantErr: true},
		{name: "provider ID is not numeric", providerID: "31026A", wantErr: true},
		{name: "reserved compression", context: ProvisionedContext{Compression: CompressionEnable + 1}, wantErr: true},
		{name: "reserved authentication protocol", context: ProvisionedContext{AuthProtocol: AuthProtocolMSCHAPV2 + 1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProvisionedContextSet(tt.context, tt.providerID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateProvisionedContextSet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProvisionedContextsInfoUnmarshalBinaryReferences(t *testing.T) {
	want := []ProvisionedContext{
		{
			ContextID:    1,
			ContextType:  ContextTypeInternet,
			AccessString: "internet",
			UserName:     "alice",
			Password:     "secret",
			Compression:  CompressionEnable,
			AuthProtocol: AuthProtocolPAP,
		},
		{
			ContextID:    2,
			ContextType:  ContextTypeIMS,
			AccessString: "ims",
		},
	}
	valid := provisionedContextsInfoPayloadForTest(want...)
	tests := []struct {
		name    string
		data    []byte
		want    []ProvisionedContext
		wantErr bool
	}{
		{name: "multiple contexts", data: valid, want: want},
		{name: "empty", data: []byte{0, 0, 0, 0}},
		{name: "truncated header", data: []byte{1}, wantErr: true},
		{name: "truncated reference list", data: []byte{1, 0, 0, 0}, wantErr: true},
		{
			name: "reference points into list",
			data: mutateBytes(provisionedContextsInfoPayloadForTest(want[0]), func(data []byte) {
				binary.LittleEndian.PutUint32(data[4:8], 4)
			}),
			wantErr: true,
		},
		{
			name: "string points into fixed fields",
			data: mutateBytes(provisionedContextsInfoPayloadForTest(want[0]), func(data []byte) {
				offset := binary.LittleEndian.Uint32(data[4:8])
				binary.LittleEndian.PutUint32(data[offset+20:offset+24], 4)
			}),
			wantErr: true,
		},
		{
			name: "automatic context ID in response",
			data: mutateBytes(provisionedContextsInfoPayloadForTest(want[0]), func(data []byte) {
				offset := binary.LittleEndian.Uint32(data[4:8])
				binary.LittleEndian.PutUint32(data[offset:offset+4], ^uint32(0))
			}),
			wantErr: true,
		},
		{
			name: "reserved compression",
			data: mutateBytes(provisionedContextsInfoPayloadForTest(want[0]), func(data []byte) {
				offset := binary.LittleEndian.Uint32(data[4:8])
				binary.LittleEndian.PutUint32(data[offset+44:offset+48], uint32(CompressionEnable)+1)
			}),
			wantErr: true,
		},
		{
			name: "reserved authentication protocol",
			data: mutateBytes(provisionedContextsInfoPayloadForTest(want[0]), func(data []byte) {
				offset := binary.LittleEndian.Uint32(data[4:8])
				binary.LittleEndian.PutUint32(data[offset+48:offset+52], uint32(AuthProtocolMSCHAPV2)+1)
			}),
			wantErr: true,
		},
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
			if len(got.Contexts) != len(tt.want) {
				t.Fatalf("Contexts len = %d, want %d", len(got.Contexts), len(tt.want))
			}
			for i := range got.Contexts {
				if got.Contexts[i] != tt.want[i] {
					t.Fatalf("Contexts[%d] = %+v, want %+v", i, got.Contexts[i], tt.want[i])
				}
			}
		})
	}
}

func TestClientSetProvisionedContext(t *testing.T) {
	tests := []struct {
		name       string
		context    ProvisionedContext
		providerID string
		response   ProvisionedContext
	}{
		{
			name: "set and return updated list",
			context: ProvisionedContext{
				ContextID:    ^uint32(0),
				ContextType:  ContextTypeInternet,
				AccessString: "internet",
				AuthProtocol: AuthProtocolPAP,
			},
			providerID: "310260",
			response: ProvisionedContext{
				ContextID:    7,
				ContextType:  ContextTypeInternet,
				AccessString: "internet",
				AuthProtocol: AuthProtocolPAP,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				want := provisionedContextSetPayloadForTest(tt.context, tt.providerID)
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceBasicConnect, CIDProvisionedContexts, CommandTypeSet, want); err != nil {
					errCh <- err
					return
				}
				response := provisionedContextsInfoPayloadForTest(tt.response)
				if _, err := serverConn.Write(mbimCommandDone(1, ServiceBasicConnect, CIDProvisionedContexts, response)); err != nil {
					errCh <- err
				}
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := client.SetProvisionedContext(ctx, tt.context, tt.providerID)
			if err != nil {
				t.Fatalf("SetProvisionedContext() error = %v", err)
			}
			if len(got) != 1 || got[0] != tt.response {
				t.Fatalf("SetProvisionedContext() = %+v, want %+v", got, []ProvisionedContext{tt.response})
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func provisionedContextSetPayloadForTest(provisioned ProvisionedContext, providerID string) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[0:4], provisioned.ContextID)
	copy(data[4:20], provisioned.ContextType[:])
	binary.LittleEndian.PutUint32(data[44:48], uint32(provisioned.Compression))
	binary.LittleEndian.PutUint32(data[48:52], uint32(provisioned.AuthProtocol))
	data = appendProvisionedContextStringForTest(data, 20, provisioned.AccessString)
	data = appendProvisionedContextStringForTest(data, 28, provisioned.UserName)
	data = appendProvisionedContextStringForTest(data, 36, provisioned.Password)
	return appendProvisionedContextStringForTest(data, 52, providerID)
}

func provisionedContextsInfoPayloadForTest(contexts ...ProvisionedContext) []byte {
	elements := make([][]byte, len(contexts))
	for i, provisioned := range contexts {
		data := make([]byte, 52)
		binary.LittleEndian.PutUint32(data[0:4], provisioned.ContextID)
		copy(data[4:20], provisioned.ContextType[:])
		binary.LittleEndian.PutUint32(data[44:48], uint32(provisioned.Compression))
		binary.LittleEndian.PutUint32(data[48:52], uint32(provisioned.AuthProtocol))
		data = appendProvisionedContextStringForTest(data, 20, provisioned.AccessString)
		data = appendProvisionedContextStringForTest(data, 28, provisioned.UserName)
		elements[i] = appendProvisionedContextStringForTest(data, 36, provisioned.Password)
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

func appendProvisionedContextStringForTest(data []byte, fieldOffset int, value string) []byte {
	raw := utf16Bytes(value)
	if len(raw) == 0 {
		return data
	}
	binary.LittleEndian.PutUint32(data[fieldOffset:fieldOffset+4], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[fieldOffset+4:fieldOffset+8], uint32(len(raw)))
	data = append(data, raw...)
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	return data
}
