package mbim

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestHostErrorRequestMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		status  ProtocolError
		wantErr bool
	}{
		{name: "fragment timeout", status: ProtocolErrorTimeoutFragment},
		{name: "cancel", status: ProtocolErrorCancel},
		{name: "max transfer", status: ProtocolErrorMaxTransfer},
		{name: "zero is invalid", status: ProtocolErrorInvalid, wantErr: true},
		{name: "reserved", status: ProtocolErrorMaxTransfer + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := (&HostErrorRequest{TransactionID: 23, Status: tt.status}).Request()
			got, err := request.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != 16 {
				t.Fatalf("MarshalBinary() length = %d, want 16", len(got))
			}
			if messageType := MessageType(binary.LittleEndian.Uint32(got[0:4])); messageType != MessageTypeHostError {
				t.Fatalf("message type = %#x, want %#x", messageType, MessageTypeHostError)
			}
			if transactionID := binary.LittleEndian.Uint32(got[8:12]); transactionID != 23 {
				t.Fatalf("transaction ID = %d, want 23", transactionID)
			}
			if status := ProtocolError(binary.LittleEndian.Uint32(got[12:16])); status != tt.status {
				t.Fatalf("status = %d, want %d", status, tt.status)
			}
		})
	}
}

func TestHostErrorTransmission(t *testing.T) {
	tests := []struct {
		name       string
		transmit   func(context.Context, *Client, Conn) error
		wantStatus ProtocolError
	}{
		{
			name: "request transmit",
			transmit: func(ctx context.Context, _ *Client, conn Conn) error {
				return (&HostErrorRequest{TransactionID: 31, Status: ProtocolErrorLengthMismatch}).Request().Transmit(ctx, conn)
			},
			wantStatus: ProtocolErrorLengthMismatch,
		},
		{
			name: "client cancel transaction",
			transmit: func(ctx context.Context, client *Client, _ Conn) error {
				return client.CancelTransaction(ctx, 31)
			},
			wantStatus: ProtocolErrorCancel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			frameCh := make(chan []byte, 1)
			errCh := make(chan error, 1)
			go func() {
				defer close(frameCh)
				defer close(errCh)
				defer serverConn.Close()
				frame, err := readFrame(serverConn)
				if err != nil {
					errCh <- err
					return
				}
				frameCh <- frame
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := tt.transmit(ctx, client, clientConn); err != nil {
				t.Fatalf("transmit() error = %v", err)
			}

			frame := <-frameCh
			if len(frame) != 16 {
				t.Fatalf("frame length = %d, want 16", len(frame))
			}
			if transactionID := binary.LittleEndian.Uint32(frame[8:12]); transactionID != 31 {
				t.Fatalf("transaction ID = %d, want 31", transactionID)
			}
			if status := ProtocolError(binary.LittleEndian.Uint32(frame[12:16])); status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
