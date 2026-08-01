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
	mu               sync.Mutex
	cond             *sync.Cond
	activeOps        int
	fd               int
	fdValid          bool
	readWakeFD       int
	readWakeFDValid  bool
	writeWakeFD      int
	writeWakeFDValid bool
	readDeadline     time.Time
	writeDeadline    time.Time
}

func Open(path string) (*Conn, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_EXCL|unix.O_NONBLOCK|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("opening cdc-wdm device %s: %w", path, err)
	}
	readWakeFD, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		var closeErr error
		if err := unix.Close(fd); err != nil {
			closeErr = fmt.Errorf("closing cdc-wdm device after wake eventfd error: %w", err)
		}
		return nil, errors.Join(fmt.Errorf("creating cdc-wdm read wake eventfd: %w", err), closeErr)
	}
	writeWakeFD, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		var closeErr error
		if err := unix.Close(readWakeFD); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("closing cdc-wdm read wake eventfd: %w", err))
		}
		if err := unix.Close(fd); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("closing cdc-wdm device after wake eventfd error: %w", err))
		}
		return nil, errors.Join(fmt.Errorf("creating cdc-wdm write wake eventfd: %w", err), closeErr)
	}
	return newConnWithFDs(fd, readWakeFD, writeWakeFD), nil
}

func newConnWithFDs(fd, readWakeFD, writeWakeFD int) *Conn {
	conn := &Conn{
		fd:               fd,
		fdValid:          true,
		readWakeFD:       readWakeFD,
		readWakeFDValid:  true,
		writeWakeFD:      writeWakeFD,
		writeWakeFDValid: true,
	}
	conn.cond = sync.NewCond(&conn.mu)
	return conn
}

func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		fd, wakeFD, deadline, release, err := c.acquireIO(true)
		if err != nil {
			return 0, err
		}
		if err := waitReadyWithWake(fd, wakeFD, unix.POLLIN, deadline); err != nil {
			release()
			if errors.Is(err, errIOStateChanged) {
				continue
			}
			return 0, err
		}
		n, err := unix.Read(fd, p)
		release()
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
		fd, wakeFD, deadline, release, err := c.acquireIO(false)
		if err != nil {
			return 0, err
		}
		if err := waitReadyWithWake(fd, wakeFD, unix.POLLOUT, deadline); err != nil {
			release()
			if errors.Is(err, errIOStateChanged) {
				continue
			}
			return 0, err
		}
		n, err := unix.Write(fd, p)
		release()
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
	if !c.fdValid {
		c.mu.Unlock()
		return nil
	}
	if err := notifyWakeFD(c.readWakeFD, c.readWakeFDValid); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("waking cdc-wdm reader: %w", err)
	}
	if err := notifyWakeFD(c.writeWakeFD, c.writeWakeFDValid); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("waking cdc-wdm writer: %w", err)
	}

	fd := c.fd
	readWakeFD := c.readWakeFD
	writeWakeFD := c.writeWakeFD
	c.fd = -1
	c.fdValid = false
	c.readWakeFD = -1
	c.readWakeFDValid = false
	c.writeWakeFD = -1
	c.writeWakeFDValid = false
	for c.activeOps > 0 {
		c.cond.Wait()
	}
	c.mu.Unlock()

	var closeErr error
	if err := unix.Close(fd); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("closing cdc-wdm device: %w", err))
	}
	if err := unix.Close(readWakeFD); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("closing cdc-wdm read wake eventfd: %w", err))
	}
	if err := unix.Close(writeWakeFD); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("closing cdc-wdm write wake eventfd: %w", err))
	}
	return closeErr
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fdValid {
		return net.ErrClosed
	}
	c.readDeadline = t
	return notifyWakeFD(c.readWakeFD, c.readWakeFDValid)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fdValid {
		return net.ErrClosed
	}
	c.writeDeadline = t
	return notifyWakeFD(c.writeWakeFD, c.writeWakeFDValid)
}

func (c *Conn) MaxControlTransfer() int {
	fd, release, err := c.acquireFD()
	if err != nil {
		return defaultMaxControlTransfer
	}
	defer release()

	max, err := unix.IoctlGetInt(fd, ioctlWDMMaxCommand)
	if err != nil || max <= 0 {
		return defaultMaxControlTransfer
	}
	return max
}

func (c *Conn) acquireIO(read bool) (int, int, time.Time, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fdValid {
		return -1, -1, time.Time{}, nil, net.ErrClosed
	}
	c.activeOps++
	if read {
		return c.fd, c.readWakeFD, c.readDeadline, c.releaseFD, nil
	}
	return c.fd, c.writeWakeFD, c.writeDeadline, c.releaseFD, nil
}

func (c *Conn) acquireFD() (int, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fdValid {
		return -1, nil, net.ErrClosed
	}
	c.activeOps++
	return c.fd, c.releaseFD, nil
}

func (c *Conn) releaseFD() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeOps--
	if c.activeOps == 0 {
		c.cond.Broadcast()
	}
}

var errIOStateChanged = errors.New("cdc-wdm I/O state changed")

func waitReady(fd int, events int16, deadline time.Time) error {
	return waitReadyWithWake(fd, -1, events, deadline)
}

func waitReadyWithWake(fd, wakeFD int, events int16, deadline time.Time) error {
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
		if wakeFD >= 0 {
			pollFDs = append(pollFDs, unix.PollFd{Fd: int32(wakeFD), Events: unix.POLLIN})
		}
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
		if wakeFD >= 0 && pollFDs[1].Revents&unix.POLLIN != 0 {
			drainWakeFD(wakeFD)
			return errIOStateChanged
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

func notifyWakeFD(fd int, valid bool) error {
	if !valid {
		return nil
	}
	for {
		_, err := unix.Write(fd, []byte{1, 0, 0, 0, 0, 0, 0, 0})
		if err == nil || errors.Is(err, unix.EAGAIN) {
			return nil
		}
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func drainWakeFD(fd int) {
	var buf [8]byte
	for {
		_, err := unix.Read(fd, buf[:])
		if err == nil || errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			return
		}
		return
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
