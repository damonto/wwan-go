package mbim

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestHostShutdownRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *Request
		serviceID   [16]byte
		commandID   uint32
		commandType CommandType
	}{
		{
			name:        "notification",
			request:     (&HostShutdownRequest{TransactionID: 1}).Request(),
			serviceID:   ServiceMsHostShutdown,
			commandID:   CIDMsHostShutdownNotify,
			commandType: CommandTypeSet,
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

func TestClientNotifyHostShutdown(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(serverConn, 1, ServiceMsHostShutdown, CIDMsHostShutdownNotify, CommandTypeSet, nil); err != nil {
					errCh <- err
					return
				}
				_, err := serverConn.Write(mbimCommandDone(1, ServiceMsHostShutdown, CIDMsHostShutdownNotify, nil))
				errCh <- err
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := client.NotifyHostShutdown(ctx); err != nil {
				t.Fatalf("NotifyHostShutdown() error = %v", err)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
