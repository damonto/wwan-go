//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const ueventBufferSize = 64 * 1024

// Discover returns the current QMI/MBIM control-node candidates. A device may
// have ProtocolUnknown when its kernel metadata is inconclusive; Open will
// then perform active protocol handshakes.
func Discover(ctx context.Context) ([]Device, error) {
	return discover(ctx, defaultSysRoot, defaultDevRoot, true)
}

// WatchDevices reports an initial Present snapshot and later kernel-driven
// changes until ctx is canceled.
func WatchDevices(ctx context.Context) (<-chan Result[DeviceEvent], error) {
	fd, err := openUeventSocket()
	if err != nil {
		return nil, err
	}
	initial, err := Discover(ctx)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	out := make(chan Result[DeviceEvent], 32)
	go watchDevices(ctx, fd, initial, out)
	return out, nil
}

func openUeventSocket() (int, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return -1, fmt.Errorf("opening modem uevent socket: %w", err)
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, ueventBufferSize); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("setting modem uevent receive buffer: %w", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: 1}); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("binding modem uevent socket: %w", err)
	}
	return fd, nil
}

func watchDevices(ctx context.Context, fd int, initial []Device, out chan<- Result[DeviceEvent]) {
	defer close(out)
	defer unix.Close(fd)

	current := devicesByPath(initial)
	for _, device := range initial {
		if !sendDeviceResult(ctx, out, Result[DeviceEvent]{Value: DeviceEvent{Type: DevicePresent, Device: device}}) {
			return
		}
	}

	buf := make([]byte, ueventBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(pollFDs, 500)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: fmt.Errorf("waiting for modem uevent: %w", err)})
			return
		}
		if n == 0 {
			continue
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: errors.New("modem uevent socket stopped")})
			return
		}
		length, _, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: fmt.Errorf("reading modem uevent: %w", err)})
			return
		}
		if !modemUevent(buf[:length]) {
			continue
		}
		next, err := Discover(ctx)
		if err != nil {
			sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: err})
			return
		}
		for _, event := range diffDevices(current, devicesByPath(next)) {
			if !sendDeviceResult(ctx, out, Result[DeviceEvent]{Value: event}) {
				return
			}
		}
		current = devicesByPath(next)
	}
}

func modemUevent(data []byte) bool {
	for _, field := range strings.Split(string(data), "\x00") {
		if field == "SUBSYSTEM=wwan" || field == "SUBSYSTEM=usbmisc" || field == "SUBSYSTEM=net" || field == "SUBSYSTEM=tty" {
			return true
		}
	}
	return false
}

func diffDevices(current, next map[string]Device) []DeviceEvent {
	paths := make([]string, 0, len(current)+len(next))
	seen := make(map[string]struct{}, len(current)+len(next))
	for path := range current {
		seen[path] = struct{}{}
	}
	for path := range next {
		seen[path] = struct{}{}
	}
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	events := make([]DeviceEvent, 0)
	for _, path := range paths {
		before, existed := current[path]
		after, exists := next[path]
		switch {
		case !existed && exists:
			events = append(events, DeviceEvent{Type: DeviceAdded, Device: cloneDevice(after)})
		case existed && !exists:
			events = append(events, DeviceEvent{Type: DeviceRemoved, Device: cloneDevice(before)})
		case existed && exists && !sameDevice(before, after):
			events = append(events, DeviceEvent{Type: DeviceChanged, Device: cloneDevice(after)})
		}
	}
	return slices.Clip(events)
}

func sendDeviceResult(ctx context.Context, out chan<- Result[DeviceEvent], result Result[DeviceEvent]) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case out <- result:
		return true
	case <-ctx.Done():
		return false
	}
}
