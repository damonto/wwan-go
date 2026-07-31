package mbim

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestFirmwareIDRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *Request
		serviceID   [16]byte
		commandID   uint32
		commandType CommandType
	}{
		{
			name:        "query",
			request:     (&FirmwareIDRequest{TransactionID: 1}).Request(),
			serviceID:   ServiceMsFirmwareID,
			commandID:   CIDMsFirmwareIDGet,
			commandType: CommandTypeQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.request.Command.(*Command)
			if command.ServiceID != tt.serviceID || command.CommandID != tt.commandID || command.CommandType != tt.commandType {
				t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if len(command.Data) != 0 {
				t.Fatalf("command data = %x, want empty", command.Data)
			}
		})
	}
}

func TestFirmwareIDUnmarshalBinary(t *testing.T) {
	want := FirmwareID{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "libmbim vector", data: want[:]},
		{name: "truncated", data: make([]byte, 15), wantErr: true},
		{name: "trailing data", data: make([]byte, 17), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FirmwareID
			err := got.UnmarshalBinary(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != want {
				t.Fatalf("UnmarshalBinary() = %x, want %x", got, want)
			}
		})
	}
}

func TestClientFirmwareID(t *testing.T) {
	firmwareID := FirmwareID{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	tests := []struct {
		name     string
		response FirmwareID
	}{
		{name: "success", response: firmwareID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceMsFirmwareID, CIDMsFirmwareIDGet, CommandTypeQuery, nil); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceMsFirmwareID, CIDMsFirmwareIDGet, tt.response[:]))
				errCh <- err
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := client.FirmwareID(ctx)
			if err != nil {
				t.Fatalf("FirmwareID() error = %v", err)
			}
			if got != tt.response {
				t.Fatalf("FirmwareID() = %x, want %x", got, tt.response)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
