//go:build linux

package cdcwdm

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultMaxControlTransfer = 4096
	ioctlWDMMaxCommand        = 0x800248A0
)

// ErrDisconnected reports that the kernel control endpoint can no longer be
// used. Callers must reopen the device instead of retrying the same file
// descriptor.
var ErrDisconnected = errors.New("cdc-wdm device disconnected")

// DisconnectError preserves the poll flags which ended a cdc-wdm connection.
type DisconnectError struct {
	Revents int16
}

func (e *DisconnectError) Error() string {
	return fmt.Sprintf("%s: poll revents=0x%X", ErrDisconnected, uint16(e.Revents))
}

func (e *DisconnectError) Unwrap() error { return ErrDisconnected }

type Conn struct {
	mu            sync.RWMutex
	fd            int
	readDeadline  time.Time
	writeDeadline time.Time
}

func Open(path string) (*Conn, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_EXCL|unix.O_NONBLOCK|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("opening cdc-wdm device %s: %w", path, err)
	}
	return &Conn{fd: fd}, nil
}

func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		fd, deadline, err := c.state(true)
		if err != nil {
			return 0, err
		}
		if err := waitReady(fd, unix.POLLIN, deadline); err != nil {
			return 0, err
		}
		n, err := unix.Read(fd, p)
		if err == nil {
			if n == 0 {
				return 0, io.EOF
			}
			return n, nil
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func (c *Conn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		fd, deadline, err := c.state(false)
		if err != nil {
			return 0, err
		}
		if err := waitReady(fd, unix.POLLOUT, deadline); err != nil {
			return 0, err
		}
		n, err := unix.Write(fd, p)
		if err == nil {
			return n, nil
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func (c *Conn) Close() error {
	c.mu.Lock()
	fd := c.fd
	c.fd = -1
	c.mu.Unlock()
	if fd < 0 {
		return nil
	}
	return unix.Close(fd)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return net.ErrClosed
	}
	c.readDeadline = t
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return net.ErrClosed
	}
	c.writeDeadline = t
	return nil
}

func (c *Conn) MaxControlTransfer() int {
	c.mu.RLock()
	fd := c.fd
	c.mu.RUnlock()
	if fd < 0 {
		return defaultMaxControlTransfer
	}
	max, err := unix.IoctlGetInt(fd, ioctlWDMMaxCommand)
	if err != nil || max <= 0 {
		return defaultMaxControlTransfer
	}
	return max
}

func (c *Conn) state(read bool) (int, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fd < 0 {
		return -1, time.Time{}, net.ErrClosed
	}
	if read {
		return c.fd, c.readDeadline, nil
	}
	return c.fd, c.writeDeadline, nil
}

func waitReady(fd int, events int16, deadline time.Time) error {
	for {
		timeout := -1
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return os.ErrDeadlineExceeded
			}
			timeout = durationMillis(remaining)
		}

		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: events}}
		n, err := unix.Poll(pollFDs, timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return os.ErrDeadlineExceeded
		}

		revents := pollFDs[0].Revents
		if revents&unix.POLLNVAL != 0 {
			return net.ErrClosed
		}
		if revents&(unix.POLLERR|unix.POLLHUP) != 0 {
			return &DisconnectError{Revents: revents}
		}
		if revents&events != 0 {
			return nil
		}
	}
}

func durationMillis(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	ms := d / time.Millisecond
	if d%time.Millisecond != 0 {
		ms++
	}
	const maxInt32 time.Duration = 1<<31 - 1
	return int(min(ms, maxInt32))
}
