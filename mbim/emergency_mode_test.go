package mbim

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestEmergencyModeRequest(t *testing.T) {
	request := (&EmergencyModeRequest{TransactionID: 1}).Request()
	command := request.Command.(*Command)
	if command.ServiceID != ServiceBasicConnect || command.CommandID != CIDEmergencyMode || command.CommandType != CommandTypeQuery {
		t.Fatalf("command = service %x CID %d type %d", command.ServiceID, command.CommandID, command.CommandType)
	}
	if len(command.Data) != 0 {
		t.Fatalf("command data = %x, want empty", command.Data)
	}
}

func TestClientEmergencyMode(t *testing.T) {
	tests := []struct {
		name string
		mode EmergencyMode
	}{
		{name: "off", mode: EmergencyModeOff},
		{name: "on", mode: EmergencyModeOn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })
			payload := binary.LittleEndian.AppendUint32(nil, uint32(tt.mode))

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceBasicConnect, CIDEmergencyMode, CommandTypeQuery, nil); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceBasicConnect, CIDEmergencyMode, payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			got, err := client.EmergencyMode(ctx)
			if err != nil {
				t.Fatalf("EmergencyMode() error = %v", err)
			}
			if got != tt.mode {
				t.Fatalf("EmergencyMode() = %d, want %d", got, tt.mode)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
