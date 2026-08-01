//go:build linux

package cdcwdm

import (
	"errors"
	"net"
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

func TestDeadlineWakesBlockedIO(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T) *Conn
		run         func(*Conn) error
		setDeadline func(*Conn, time.Time) error
	}{
		{
			name:  "read",
			setup: newBlockedReadConn,
			run: func(conn *Conn) error {
				_, err := conn.Read(make([]byte, 1))
				return err
			},
			setDeadline: (*Conn).SetReadDeadline,
		},
		{
			name:  "write",
			setup: newBlockedWriteConn,
			run: func(conn *Conn) error {
				_, err := conn.Write([]byte{1})
				return err
			},
			setDeadline: (*Conn).SetWriteDeadline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := tt.setup(t)
			t.Cleanup(func() {
				if err := conn.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			})

			if err := tt.setDeadline(conn, time.Now().Add(time.Second)); err != nil {
				t.Fatalf("setting initial deadline: %v", err)
			}
			done := make(chan error, 1)
			go func() {
				done <- tt.run(conn)
			}()

			time.Sleep(10 * time.Millisecond)
			if err := tt.setDeadline(conn, time.Now()); err != nil {
				t.Fatalf("interrupting blocked I/O: %v", err)
			}

			select {
			case err := <-done:
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					t.Fatalf("I/O error = %v, want deadline exceeded", err)
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatal("deadline change did not wake blocked I/O")
			}
		})
	}
}

func TestCloseWakesBlockedIO(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) *Conn
		run   func(*Conn) error
	}{
		{
			name:  "read",
			setup: newBlockedReadConn,
			run: func(conn *Conn) error {
				_, err := conn.Read(make([]byte, 1))
				return err
			},
		},
		{
			name:  "write",
			setup: newBlockedWriteConn,
			run: func(conn *Conn) error {
				_, err := conn.Write([]byte{1})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := tt.setup(t)
			t.Cleanup(func() {
				if err := conn.Close(); err != nil {
					t.Errorf("cleanup Close() error = %v", err)
				}
			})

			ioDone := make(chan error, 1)
			go func() {
				ioDone <- tt.run(conn)
			}()

			time.Sleep(10 * time.Millisecond)
			closeDone := make(chan error, 1)
			go func() {
				closeDone <- conn.Close()
			}()

			select {
			case err := <-ioDone:
				if !errors.Is(err, net.ErrClosed) {
					t.Fatalf("I/O error = %v, want net.ErrClosed", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not wake blocked I/O")
			}
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not finish after I/O returned")
			}
		})
	}
}

func newBlockedReadConn(t *testing.T) *Conn {
	t.Helper()
	fds := make([]int, 2)
	if err := unix.Pipe2(fds, unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatalf("Pipe2() error = %v", err)
	}
	t.Cleanup(func() {
		if err := unix.Close(fds[1]); err != nil {
			t.Errorf("closing write pipe: %v", err)
		}
	})
	return newTestConn(t, fds[0])
}

func newBlockedWriteConn(t *testing.T) *Conn {
	t.Helper()
	fds := make([]int, 2)
	if err := unix.Pipe2(fds, unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatalf("Pipe2() error = %v", err)
	}
	t.Cleanup(func() {
		if err := unix.Close(fds[0]); err != nil {
			t.Errorf("closing read pipe: %v", err)
		}
	})

	fill := make([]byte, 4096)
	for {
		if _, err := unix.Write(fds[1], fill); err != nil {
			if errors.Is(err, unix.EAGAIN) {
				break
			}
			t.Fatalf("filling write pipe: %v", err)
		}
	}
	return newTestConn(t, fds[1])
}

func newTestConn(t *testing.T, fd int) *Conn {
	t.Helper()
	readWakeFD, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		if closeErr := unix.Close(fd); closeErr != nil {
			t.Errorf("closing connection fd: %v", closeErr)
		}
		t.Fatalf("creating read wake eventfd: %v", err)
	}
	writeWakeFD, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		if closeErr := unix.Close(readWakeFD); closeErr != nil {
			t.Errorf("closing read wake eventfd: %v", closeErr)
		}
		if closeErr := unix.Close(fd); closeErr != nil {
			t.Errorf("closing connection fd: %v", closeErr)
		}
		t.Fatalf("creating write wake eventfd: %v", err)
	}
	return newConnWithFDs(fd, readWakeFD, writeWakeFD)
}
