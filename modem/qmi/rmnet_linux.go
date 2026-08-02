//go:build linux

package qmi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"

	"golang.org/x/sys/unix"
)

const (
	netlinkHeaderLength = 16
	ifInfoMessageLength = 16
	routeAttrHeaderSize = 4
)

func createRMNetLink(ctx context.Context, baseName string, flags uint32) (*rmnetLink, error) {
	base, err := net.InterfaceByName(baseName)
	if err != nil {
		return nil, fmt.Errorf("looking up base interface %s: %w", baseName, err)
	}
	if base.Index <= 0 || base.Index > math.MaxInt32 {
		return nil, fmt.Errorf("creating rmnet link: base interface index %d is invalid", base.Index)
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing network interfaces: %w", err)
	}
	usedNames := make(map[string]struct{}, len(interfaces))
	for _, iface := range interfaces {
		usedNames[iface.Name] = struct{}{}
	}

	flags &= rmnetFlagMask
	flags |= rmnetFlagIngressDeaggregation
	for muxID := rmnetMuxIDMin; muxID <= rmnetMuxIDMax; muxID++ {
		name := rmnetLinkName(base.Index, uint8(muxID))
		if _, exists := usedNames[name]; exists {
			continue
		}
		if len(name) >= unix.IFNAMSIZ {
			return nil, fmt.Errorf("creating rmnet link: interface name %q exceeds %d bytes", name, unix.IFNAMSIZ-1)
		}

		err := sendRouteNetlink(ctx, newRMNetLinkMessage(base.Index, name, uint8(muxID), flags))
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("adding rmnet link %s with mux ID %d: %w", name, muxID, err)
		}

		child, err := net.InterfaceByName(name)
		if err != nil {
			return nil, fmt.Errorf("looking up created rmnet link %s: %w", name, err)
		}
		if child.Index <= 0 || child.Index > math.MaxInt32 {
			return nil, fmt.Errorf("creating rmnet link: child interface index %d is invalid", child.Index)
		}
		link := &rmnetLink{Name: name, MuxID: uint8(muxID), Index: child.Index}
		link.close = func(ctx context.Context) error {
			return deleteRMNetLink(ctx, link.Name, link.Index)
		}

		if base.Flags&net.FlagUp == 0 {
			if err := sendRouteNetlink(ctx, newSetLinkUpMessage(base.Index)); err != nil {
				deleteErr := link.Close()
				return nil, errors.Join(
					fmt.Errorf("bringing base interface %s up: %w", baseName, err),
					wrapRMNetRollbackError(link.Name, deleteErr),
				)
			}
		}
		return link, nil
	}
	return nil, fmt.Errorf("creating rmnet link on %s: no mux ID is available in %d..%d", baseName, rmnetMuxIDMin, rmnetMuxIDMax)
}

func wrapRMNetRollbackError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("rolling back rmnet link %s: %w", name, err)
}

func deleteRMNetLink(ctx context.Context, name string, expectedIndex int) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("listing network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Name != name {
			continue
		}
		if iface.Index != expectedIndex {
			return fmt.Errorf("rmnet link %s index changed from %d to %d", name, expectedIndex, iface.Index)
		}
		if err := sendRouteNetlink(ctx, newDeleteLinkMessage(iface.Index)); err != nil {
			return fmt.Errorf("removing interface index %d: %w", iface.Index, err)
		}
		return nil
	}
	return nil
}

func rmnetLinkName(baseIndex int, muxID uint8) string {
	return fmt.Sprintf("qmap%x.%x", baseIndex, muxID-1)
}

func newRMNetLinkMessage(baseIndex int, name string, muxID uint8, flags uint32) []byte {
	dataInfo := make([]byte, 0, 24)
	dataInfo = appendRouteAttribute(dataInfo, unix.IFLA_RMNET_MUX_ID, binary.NativeEndian.AppendUint16(nil, uint16(muxID)))
	rmnetFlags := binary.NativeEndian.AppendUint32(nil, flags)
	rmnetFlags = binary.NativeEndian.AppendUint32(rmnetFlags, rmnetFlagMask)
	dataInfo = appendRouteAttribute(dataInfo, unix.IFLA_RMNET_FLAGS, rmnetFlags)

	linkInfo := make([]byte, 0, 40)
	linkInfo = appendRouteAttribute(linkInfo, unix.IFLA_INFO_KIND, []byte("rmnet"))
	linkInfo = appendRouteAttribute(linkInfo, unix.IFLA_INFO_DATA, dataInfo)

	attrs := make([]byte, 0, 64)
	attrs = appendRouteAttribute(attrs, unix.IFLA_LINK, binary.NativeEndian.AppendUint32(nil, uint32(baseIndex)))
	attrs = appendRouteAttribute(attrs, unix.IFLA_IFNAME, []byte(name))
	attrs = appendRouteAttribute(attrs, unix.IFLA_LINKINFO, linkInfo)

	return newRouteLinkMessage(
		unix.RTM_NEWLINK,
		unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL,
		unix.ARPHRD_RAWIP,
		0,
		0,
		math.MaxUint32,
		attrs,
	)
}

func newDeleteLinkMessage(index int) []byte {
	return newRouteLinkMessage(
		unix.RTM_DELLINK,
		unix.NLM_F_REQUEST|unix.NLM_F_ACK,
		0,
		int32(index),
		0,
		0,
		nil,
	)
}

func newSetLinkUpMessage(index int) []byte {
	return newRouteLinkMessage(
		unix.RTM_NEWLINK,
		unix.NLM_F_REQUEST|unix.NLM_F_ACK,
		0,
		int32(index),
		unix.IFF_UP,
		unix.IFF_UP,
		nil,
	)
}

func newRouteLinkMessage(messageType, messageFlags, linkType uint16, index int32, flags, change uint32, attrs []byte) []byte {
	message := make([]byte, netlinkHeaderLength+ifInfoMessageLength, netlinkHeaderLength+ifInfoMessageLength+len(attrs))
	binary.NativeEndian.PutUint32(message[0:4], uint32(cap(message)))
	binary.NativeEndian.PutUint16(message[4:6], messageType)
	binary.NativeEndian.PutUint16(message[6:8], messageFlags)
	message[netlinkHeaderLength] = unix.AF_UNSPEC
	binary.NativeEndian.PutUint16(message[netlinkHeaderLength+2:netlinkHeaderLength+4], linkType)
	binary.NativeEndian.PutUint32(message[netlinkHeaderLength+4:netlinkHeaderLength+8], uint32(index))
	binary.NativeEndian.PutUint32(message[netlinkHeaderLength+8:netlinkHeaderLength+12], flags)
	binary.NativeEndian.PutUint32(message[netlinkHeaderLength+12:netlinkHeaderLength+16], change)
	return append(message, attrs...)
}

func appendRouteAttribute(dst []byte, attrType int, value []byte) []byte {
	length := routeAttrHeaderSize + len(value)
	alignedLength := alignRouteMessage(length)
	start := len(dst)
	dst = append(dst, make([]byte, alignedLength)...)
	binary.NativeEndian.PutUint16(dst[start:start+2], uint16(length))
	binary.NativeEndian.PutUint16(dst[start+2:start+4], uint16(attrType))
	copy(dst[start+routeAttrHeaderSize:], value)
	return dst
}

func alignRouteMessage(length int) int {
	return (length + 3) &^ 3
}

func sendRouteNetlink(ctx context.Context, message []byte) error {
	requestCtx, cancel := context.WithTimeout(ctx, rmnetOperationTimeout)
	defer cancel()

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("opening route netlink socket: %w", err)
	}
	defer func() {
		_ = unix.Close(fd) // The request is complete; a close error cannot change its result.
	}()
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return fmt.Errorf("binding route netlink socket: %w", err)
	}

	request := append([]byte(nil), message...)
	const sequence = uint32(1)
	binary.NativeEndian.PutUint32(request[8:12], sequence)
	for {
		err = unix.Sendto(fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK})
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			if err := waitRouteNetlink(requestCtx, fd, unix.POLLOUT); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("sending route netlink request: %w", err)
		}
		break
	}

	buffer := make([]byte, 8192)
	for {
		if err := waitRouteNetlink(requestCtx, fd, unix.POLLIN); err != nil {
			return err
		}
		length, _, err := unix.Recvfrom(fd, buffer, 0)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading route netlink acknowledgment: %w", err)
		}
		matched, err := routeNetlinkACK(buffer[:length], sequence)
		if matched {
			return err
		}
	}
}

func waitRouteNetlink(ctx context.Context, fd int, events int16) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: events}}
		n, err := unix.Poll(pollFDs, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("waiting for route netlink socket: %w", err)
		}
		if n == 0 {
			continue
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return fmt.Errorf("route netlink socket stopped: revents=0x%X", uint16(pollFDs[0].Revents))
		}
		if pollFDs[0].Revents&events != 0 {
			return nil
		}
	}
}

func routeNetlinkACK(data []byte, sequence uint32) (bool, error) {
	for len(data) >= netlinkHeaderLength {
		length := int(binary.NativeEndian.Uint32(data[0:4]))
		if length < netlinkHeaderLength || length > len(data) {
			return true, fmt.Errorf("parsing route netlink response: message length %d exceeds buffer %d", length, len(data))
		}
		messageType := binary.NativeEndian.Uint16(data[4:6])
		messageSequence := binary.NativeEndian.Uint32(data[8:12])
		if messageSequence == sequence {
			switch messageType {
			case unix.NLMSG_ERROR:
				if length < netlinkHeaderLength+4 {
					return true, fmt.Errorf("parsing route netlink acknowledgment: payload length %d, want at least 4", length-netlinkHeaderLength)
				}
				code := int32(binary.NativeEndian.Uint32(data[netlinkHeaderLength : netlinkHeaderLength+4]))
				if code == 0 {
					return true, nil
				}
				errno := int64(code)
				if errno < 0 {
					errno = -errno
				}
				return true, unix.Errno(errno)
			case unix.NLMSG_DONE:
				return true, nil
			case unix.NLMSG_OVERRUN:
				return true, errors.New("route netlink response was overrun")
			}
		}

		alignedLength := alignRouteMessage(length)
		if alignedLength > len(data) {
			if length == len(data) {
				return false, nil
			}
			return true, fmt.Errorf("parsing route netlink response: aligned length %d exceeds buffer %d", alignedLength, len(data))
		}
		data = data[alignedLength:]
	}
	if len(data) != 0 {
		return true, fmt.Errorf("parsing route netlink response: trailing length %d is smaller than header", len(data))
	}
	return false, nil
}
