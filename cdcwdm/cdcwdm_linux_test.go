//go:build linux

package cdcwdm

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDisconnectError(t *testing.T) {
	tests := []struct {
		name    string
		revents int16
		want    string
	}{
		{name: "poll error and hangup", revents: unix.POLLERR | unix.POLLHUP, want: "cdc-wdm device disconnected: poll revents=0x18"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &DisconnectError{Revents: tt.revents}
			if !errors.Is(err, ErrDisconnected) {
				t.Fatalf("errors.Is(%v, ErrDisconnected) = false", err)
			}
			disconnectErr, ok := errors.AsType[*DisconnectError](err)
			if !ok {
				t.Fatalf("errors.AsType[*DisconnectError](%v) = false", err)
			}
			if disconnectErr.Revents != tt.revents {
				t.Fatalf("Revents = 0x%X, want 0x%X", disconnectErr.Revents, tt.revents)
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWaitReadyReportsDisconnect(t *testing.T) {
	tests := []struct {
		name   string
		events int16
	}{
		{name: "peer closed", events: unix.POLLIN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe() error = %v", err)
			}
			t.Cleanup(func() {
				if err := reader.Close(); err != nil {
					t.Errorf("reader.Close() error = %v", err)
				}
			})
			if err := writer.Close(); err != nil {
				t.Fatalf("writer.Close() error = %v", err)
			}

			err = waitReady(int(reader.Fd()), tt.events, time.Now().Add(time.Second))
			if !errors.Is(err, ErrDisconnected) {
				t.Fatalf("waitReady() error = %v, want ErrDisconnected", err)
			}
			disconnectErr, ok := errors.AsType[*DisconnectError](err)
			if !ok {
				t.Fatalf("waitReady() error = %v, want *DisconnectError", err)
			}
			if disconnectErr.Revents&unix.POLLHUP == 0 {
				t.Fatalf("Revents = 0x%X, want POLLHUP", uint16(disconnectErr.Revents))
			}
		})
	}
}
