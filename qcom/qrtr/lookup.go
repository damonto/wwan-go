package qrtr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/damonto/wwan-go/qcom"
	"golang.org/x/sys/unix"
)

const serviceLookupTimeout = 5 * time.Second

func (c *Conn) findService(ctx context.Context, serviceType qcom.ServiceType) (*service, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(serviceLookupTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	interruptDone := make(chan struct{})
	stopInterrupt := context.AfterFunc(ctx, func() {
		defer close(interruptDone)
		// Waking the poll is sufficient; the loop observes ctx.Err directly.
		_ = c.wakeReader()
	})
	defer func() {
		if !stopInterrupt() {
			<-interruptDone
		}
	}()

	if err := c.sendControlPacket(ctx, deadline, serviceType); err != nil {
		return nil, lookupContextError(ctx, err)
	}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		buf, _, err := c.recvPacketWithDeadline(deadline, c.currentDeadlineSeq())
		if err != nil {
			if errors.Is(err, errReadDeadlineChanged) {
				continue
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				break
			}
			return nil, lookupContextError(ctx, err)
		}
		if len(buf) < int(unsafe.Sizeof(controlPacket{})) {
			continue
		}
		if packetType(binary.LittleEndian.Uint32(buf[:4])) != packetTypeNewServer {
			continue
		}
		var service service
		if err := binary.Read(bytes.NewReader(buf[4:]), binary.LittleEndian, &service); err != nil {
			return nil, fmt.Errorf("read QRTR service announcement: %w", err)
		}
		if qcom.ServiceType(service.Service) == serviceType {
			return &service, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && !time.Now().Before(ctxDeadline) {
		return nil, context.DeadlineExceeded
	}
	return nil, fmt.Errorf("service %d not found", serviceType)
}

func (c *Conn) sendControlPacket(ctx context.Context, deadline time.Time, serviceType qcom.ServiceType) error {
	pkt := &controlPacket{
		Command: packetTypeNewLookup,
		Service: service{
			Service:  uint32(serviceType),
			Instance: 0,
			Node:     0,
			Port:     0,
		},
	}
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, pkt); err != nil {
		return fmt.Errorf("write QRTR control packet: %w", err)
	}
	addr, err := c.localAddr()
	if err != nil {
		return err
	}
	addr.Port = portControl
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err = c.sendTo(addr, buf.Bytes())
		if err == nil {
			return nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK) {
			return err
		}
		wait := min(writeRetryDelay, time.Until(deadline))
		if wait <= 0 {
			return os.ErrDeadlineExceeded
		}
		if err := waitLookupRetry(ctx, wait); err != nil {
			return err
		}
	}
}

func waitLookupRetry(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func lookupContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return err
}
