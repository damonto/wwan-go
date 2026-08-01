//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	ueventBufferSize  = 64 * 1024
	ueventSettleDelay = 50 * time.Millisecond
)

type kernelUevent struct {
	action    string
	subsystem string
	devName   string
	devPath   string
}

func (e kernelUevent) removesControlNode() bool {
	return e.action == "remove" && (e.subsystem == "wwan" || e.subsystem == "usbmisc")
}

func (e kernelUevent) controlNodeName() string {
	if name := strings.TrimSpace(e.devName); name != "" {
		return filepath.Base(name)
	}
	return filepath.Base(strings.TrimSpace(e.devPath))
}

type deviceUeventBatch struct {
	removals []kernelUevent
	rescan   bool
	stopped  bool
	err      error
}

type deviceUeventQueue struct {
	mu           sync.Mutex
	notify       chan struct{}
	removals     []kernelUevent
	removalNames map[string]struct{}
	rescan       bool
	stopped      bool
	err          error
}

func newDeviceUeventQueue() *deviceUeventQueue {
	return &deviceUeventQueue{
		notify:       make(chan struct{}, 1),
		removalNames: make(map[string]struct{}),
	}
}

func (q *deviceUeventQueue) push(event kernelUevent) {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	if event.removesControlNode() {
		name := event.controlNodeName()
		if name != "" && name != "." {
			if _, exists := q.removalNames[name]; !exists {
				q.removalNames[name] = struct{}{}
				q.removals = append(q.removals, event)
			}
		}
	}
	q.rescan = true
	q.mu.Unlock()
	q.signal()
}

func (q *deviceUeventQueue) stop(err error) {
	q.mu.Lock()
	if !q.stopped {
		q.stopped = true
		q.err = err
	}
	q.mu.Unlock()
	q.signal()
}

func (q *deviceUeventQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *deviceUeventQueue) wait(ctx context.Context) bool {
	for {
		q.mu.Lock()
		ready := q.rescan || len(q.removals) > 0 || q.stopped
		q.mu.Unlock()
		if ready {
			return true
		}

		select {
		case <-q.notify:
		case <-ctx.Done():
			return false
		}
	}
}

func (q *deviceUeventQueue) take() deviceUeventBatch {
	q.mu.Lock()
	defer q.mu.Unlock()
	batch := deviceUeventBatch{
		removals: q.removals,
		rescan:   q.rescan,
		stopped:  q.stopped,
		err:      q.err,
	}
	q.removals = nil
	clear(q.removalNames)
	q.rescan = false
	return batch
}

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

	current := devicesByPath(initial)
	readerCtx, cancelReader := context.WithCancel(ctx)
	queue := newDeviceUeventQueue()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		readModemUevents(readerCtx, fd, queue)
	}()
	defer func() {
		cancelReader()
		<-readerDone
	}()

	for _, device := range initial {
		if !sendDeviceResult(ctx, out, Result[DeviceEvent]{Value: DeviceEvent{Type: DevicePresent, Device: device}}) {
			return
		}
	}

	for {
		if !queue.wait(ctx) || !waitUeventSettle(ctx) {
			return
		}
		batch := queue.take()
		if batch.rescan {
			next, err := Discover(ctx)
			if err != nil {
				sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: err})
				return
			}
			var events []DeviceEvent
			current, events = reconcileDeviceEvents(current, batch.removals, next)
			for _, event := range events {
				if !sendDeviceResult(ctx, out, Result[DeviceEvent]{Value: event}) {
					return
				}
			}
		}
		if batch.err != nil {
			sendDeviceResult(ctx, out, Result[DeviceEvent]{Err: batch.err})
			return
		}
		if batch.stopped {
			return
		}
	}
}

func readModemUevents(ctx context.Context, fd int, queue *deviceUeventQueue) {
	defer func() {
		// The reader owns the socket, and no useful recovery is possible here.
		_ = unix.Close(fd)
	}()
	buf := make([]byte, ueventBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			queue.stop(nil)
			return
		}
		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(pollFDs, 500)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			queue.stop(fmt.Errorf("waiting for modem uevent: %w", err))
			return
		}
		if n == 0 {
			continue
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			queue.stop(fmt.Errorf("modem uevent socket stopped: revents=0x%X", uint16(pollFDs[0].Revents)))
			return
		}
		length, _, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			queue.stop(fmt.Errorf("reading modem uevent: %w", err))
			return
		}
		event, ok := parseModemUevent(buf[:length])
		if !ok {
			continue
		}
		queue.push(event)
	}
}

func modemUevent(data []byte) bool {
	_, ok := parseModemUevent(data)
	return ok
}

func parseModemUevent(data []byte) (kernelUevent, bool) {
	fields := strings.Split(string(data), "\x00")
	var event kernelUevent
	if len(fields) > 0 {
		event.action, event.devPath, _ = strings.Cut(fields[0], "@")
	}
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "ACTION":
			event.action = value
		case "SUBSYSTEM":
			event.subsystem = value
		case "DEVNAME":
			event.devName = value
		case "DEVPATH":
			event.devPath = value
		}
	}
	switch event.subsystem {
	case "wwan", "usbmisc", "net", "tty":
		return event, true
	default:
		return kernelUevent{}, false
	}
}

func waitUeventSettle(ctx context.Context) bool {
	timer := time.NewTimer(ueventSettleDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func reconcileDeviceEvents(current map[string]Device, removals []kernelUevent, next []Device) (map[string]Device, []DeviceEvent) {
	events := make([]DeviceEvent, 0, len(removals)+len(next))
	for _, removal := range removals {
		events = append(events, removeControlDevice(current, removal)...)
	}
	nextByPath := devicesByPath(next)
	events = append(events, diffDevices(current, nextByPath)...)
	return nextByPath, slices.Clip(events)
}

func removeControlDevice(current map[string]Device, event kernelUevent) []DeviceEvent {
	name := event.controlNodeName()
	if name == "" || name == "." {
		return nil
	}
	paths := make([]string, 0, len(current))
	for path := range current {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var events []DeviceEvent
	for _, path := range paths {
		device := current[path]
		if !deviceHasControlNode(device, name) {
			continue
		}
		events = append(events, DeviceEvent{Type: DeviceRemoved, Device: cloneDevice(device)})
		delete(current, path)
	}
	return slices.Clip(events)
}

func deviceHasControlNode(device Device, name string) bool {
	if filepath.Base(strings.TrimSpace(device.Path)) == name {
		return true
	}
	for _, port := range device.Ports {
		if strings.TrimSpace(port.Name) == name || filepath.Base(strings.TrimSpace(port.Path)) == name || filepath.Base(strings.TrimSpace(port.SysPath)) == name {
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
