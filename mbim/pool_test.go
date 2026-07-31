package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type poolErrorDialer struct {
	err    error
	calls  atomic.Int32
	dialed chan<- struct{}
}

func (d *poolErrorDialer) Dial(context.Context) (Conn, error) {
	d.calls.Add(1)
	if d.dialed != nil {
		d.dialed <- struct{}{}
	}
	return nil, d.err
}

type poolTrackingConn struct {
	closes atomic.Int32
}

func (*poolTrackingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*poolTrackingConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *poolTrackingConn) Close() error                   { c.closes.Add(1); return nil }
func (*poolTrackingConn) SetReadDeadline(time.Time) error  { return nil }
func (*poolTrackingConn) SetWriteDeadline(time.Time) error { return nil }

func TestDirectClientPoolLeaseLifecycle(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close() })

	owner := &Client{
		conn:               clientConn,
		maxControlTransfer: defaultMaxControlTransfer,
		clientState:        newClientState(),
	}
	key := "/dev/cdc-wdm0"
	pool := &directClientPool{entries: map[string]*directClientEntry{
		key: {owner: owner, refs: 2, closed: make(chan struct{})},
	}}
	first := pool.lease(key, owner, 1)
	second := pool.lease(key, owner, 2)
	if first.slot != 0 || second.slot != 1 {
		t.Fatalf("lease slots = %d, %d; want 0, 1", first.slot, second.slot)
	}
	if first.clientState != second.clientState {
		t.Fatal("leases do not share the MBIM dispatcher")
	}
	if got, want := first.nextTransactionID(), uint32(1); got != want {
		t.Fatalf("first transaction ID = %d, want %d", got, want)
	}
	if got, want := second.nextTransactionID(), uint32(2); got != want {
		t.Fatalf("second transaction ID = %d, want %d", got, want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	firstResults, err := first.WatchIndicationResults(ctx, ServiceBasicConnect, CIDRadioState)
	if err != nil {
		t.Fatalf("first WatchIndicationResults() error = %v", err)
	}
	secondResults, err := second.WatchIndicationResults(ctx, ServiceBasicConnect, CIDRadioState)
	if err != nil {
		t.Fatalf("second WatchIndicationResults() error = %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first second Close() error = %v", err)
	}
	select {
	case result := <-firstResults:
		if !errors.Is(result.Err, errClientLeaseClosed) {
			t.Fatalf("first watcher error = %v, want %v", result.Err, errClientLeaseClosed)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := first.WatchIndications(ctx, ServiceBasicConnect, CIDRadioState); !errors.Is(err, errClientLeaseClosed) {
		t.Fatalf("closed lease WatchIndications() error = %v, want %v", err, errClientLeaseClosed)
	}

	payload := []byte{1, 2, 3, 4}
	if _, err := serverConn.Write(mbimIndication(ServiceBasicConnect, CIDRadioState, payload)); err != nil {
		t.Fatalf("write indication: %v", err)
	}
	select {
	case result := <-secondResults:
		if result.Err != nil {
			t.Fatalf("second watcher error = %v", result.Err)
		}
		if got := result.Value.InformationBuffer; string(got) != string(payload) {
			t.Fatalf("second watcher payload = %x, want %x", got, payload)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	serverErr := make(chan error, 1)
	go func() {
		frame, err := readFrame(serverConn)
		if err != nil {
			serverErr <- err
			return
		}
		if got := MessageType(binary.LittleEndian.Uint32(frame[:4])); got != MessageTypeClose {
			serverErr <- fmt.Errorf("message type = %#x, want %#x", got, MessageTypeClose)
			return
		}
		transactionID := binary.LittleEndian.Uint32(frame[8:12])
		_, err = serverConn.Write(mbimCloseDoneForPoolTest(transactionID))
		serverErr <- err
	}()
	if err := second.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("serve MBIM close: %v", err)
	}
	if len(pool.entries) != 0 {
		t.Fatalf("pool entries after last Close() = %d, want 0", len(pool.entries))
	}
}

func TestDirectClientPoolReplacesFailedOwner(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "receiver failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := new(poolTrackingConn)
			state := newClientState()
			state.receiverErr = errors.New("malformed MBIM frame")
			owner := &Client{
				conn:               conn,
				maxControlTransfer: defaultMaxControlTransfer,
				clientState:        state,
			}
			key := "/dev/cdc-wdm0"
			pool := &directClientPool{entries: map[string]*directClientEntry{
				key: {owner: owner, refs: 1, closed: make(chan struct{})},
			}}
			dialErr := errors.New("replacement dial")
			dialer := &poolErrorDialer{err: dialErr}

			if _, err := pool.acquire(context.Background(), key, dialer, 1); !errors.Is(err, dialErr) {
				t.Fatalf("acquire() error = %v, want %v", err, dialErr)
			}
			if got := dialer.calls.Load(); got != 1 {
				t.Fatalf("Dial() calls = %d, want 1", got)
			}
			if got := conn.closes.Load(); got != 1 {
				t.Fatalf("failed owner connection closes = %d, want 1", got)
			}
			if len(pool.entries) != 0 {
				t.Fatalf("pool entries = %d, want 0", len(pool.entries))
			}
		})
	}
}

func TestDirectClientPoolWaitsForPhysicalCloseBeforeRedial(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "last lease close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = serverConn.Close() })
			owner := &Client{
				conn:               clientConn,
				maxControlTransfer: defaultMaxControlTransfer,
				clientState:        newClientState(),
			}
			key := "/dev/cdc-wdm0"
			pool := &directClientPool{entries: map[string]*directClientEntry{
				key: {owner: owner, refs: 1, closed: make(chan struct{})},
			}}
			lease := pool.lease(key, owner, 1)

			closeRequest := make(chan uint32, 1)
			allowClose := make(chan struct{})
			serverErr := make(chan error, 1)
			go func() {
				frame, err := readFrame(serverConn)
				if err != nil {
					serverErr <- err
					return
				}
				if got := MessageType(binary.LittleEndian.Uint32(frame[:4])); got != MessageTypeClose {
					serverErr <- fmt.Errorf("message type = %#x, want %#x", got, MessageTypeClose)
					return
				}
				transactionID := binary.LittleEndian.Uint32(frame[8:12])
				closeRequest <- transactionID
				<-allowClose
				_, err = serverConn.Write(mbimCloseDoneForPoolTest(transactionID))
				serverErr <- err
			}()

			closeResult := make(chan error, 1)
			go func() { closeResult <- lease.Close() }()
			<-closeRequest

			dialErr := errors.New("replacement dial")
			dialed := make(chan struct{}, 1)
			dialer := &poolErrorDialer{err: dialErr, dialed: dialed}
			acquireResult := make(chan error, 1)
			go func() {
				_, err := pool.acquire(context.Background(), key, dialer, 1)
				acquireResult <- err
			}()

			select {
			case <-dialed:
				t.Fatal("Dial() started before the previous connection finished closing")
			case <-time.After(50 * time.Millisecond):
			}

			close(allowClose)
			if err := <-serverErr; err != nil {
				t.Fatalf("serve MBIM close: %v", err)
			}
			if err := <-closeResult; err != nil {
				t.Fatalf("lease Close() error = %v", err)
			}
			select {
			case <-dialed:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for replacement Dial()")
			}
			if err := <-acquireResult; !errors.Is(err, dialErr) {
				t.Fatalf("acquire() error = %v, want %v", err, dialErr)
			}
		})
	}
}

func mbimCloseDoneForPoolTest(transactionID uint32) []byte {
	buf := binary.LittleEndian.AppendUint32(nil, uint32(MessageTypeCloseDone))
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint32(buf, transactionID)
	return binary.LittleEndian.AppendUint32(buf, uint32(StatusNone))
}
