package qmi

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

type poolDialer struct {
	mu      sync.Mutex
	conns   []*poolConn
	newConn func(index int) *poolConn
	dialed  chan<- int
}

func (d *poolDialer) Dial(context.Context) (Conn, error) {
	d.mu.Lock()
	index := len(d.conns)
	conn := new(poolConn)
	if d.newConn != nil {
		conn = d.newConn(index)
	}
	d.conns = append(d.conns, conn)
	dialed := d.dialed
	d.mu.Unlock()
	if dialed != nil {
		dialed <- index
	}
	return conn, nil
}

func (d *poolDialer) connections() []*poolConn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*poolConn(nil), d.conns...)
}

type poolConn struct {
	closes       atomic.Int32
	closeStarted chan struct{}
	closeRelease <-chan struct{}
	closeOnce    sync.Once
}

func (*poolConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (*poolConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *poolConn) Close() error {
	c.closes.Add(1)
	if c.closeStarted != nil {
		c.closeOnce.Do(func() { close(c.closeStarted) })
	}
	if c.closeRelease != nil {
		<-c.closeRelease
	}
	return nil
}
func (*poolConn) SetReadDeadline(time.Time) error  { return nil }
func (*poolConn) SetWriteDeadline(time.Time) error { return nil }

func newTestDirectPool() *directPool {
	return &directPool{initialize: func(context.Context, *Transport) error { return nil }}
}

func TestDirectPoolSynchronizesNewCoreOnce(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "two leases on one direct endpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			conn := newAsyncDeadlineConn()
			conn.frames <- controlResultFrame(1, qcom.MessageCTLSync)
			pool := new(directPool)
			dialer := fakeDialer{conn: conn}

			first, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
			if err != nil {
				t.Fatalf("acquire(first) error = %v", err)
			}
			second, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
			if err != nil {
				t.Fatalf("acquire(second) error = %v", err)
			}

			conn.mu.Lock()
			writes := len(conn.writeDeadlines)
			conn.mu.Unlock()
			if writes != 1 {
				t.Fatalf("direct initialization writes = %d, want one CTL sync", writes)
			}
			if err := first.Close(); err != nil {
				t.Fatalf("first Close() error = %v", err)
			}
			if err := second.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
		})
	}
}

func TestDirectPoolClosesCoreAfterInitializationFailure(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "CTL sync rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initErr := errors.New("sync rejected")
			pool := &directPool{initialize: func(context.Context, *Transport) error { return initErr }}
			dialer := new(poolDialer)

			if _, err := pool.acquire(t.Context(), "/dev/wwan0qmi0", dialer); !errors.Is(err, initErr) {
				t.Fatalf("acquire() error = %v, want initialization error", err)
			}
			conns := dialer.connections()
			if len(conns) != 1 {
				t.Fatalf("Dial() calls = %d, want 1", len(conns))
			}
			if got := conns[0].closes.Load(); got != 1 {
				t.Fatalf("connection closes = %d, want 1", got)
			}
			pool.mu.Lock()
			remaining := len(pool.entries)
			pool.mu.Unlock()
			if remaining != 0 {
				t.Fatalf("pool entries = %d, want empty", remaining)
			}
		})
	}
}

func TestDirectPoolGroupsLeasesByDevice(t *testing.T) {
	ctx := context.Background()
	pool := newTestDirectPool()
	dialer := new(poolDialer)

	data5a, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
	if err != nil {
		t.Fatalf("acquire(DATA5 first) error = %v", err)
	}
	data5b, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
	if err != nil {
		t.Fatalf("acquire(DATA5 second) error = %v", err)
	}
	data6, err := pool.acquire(ctx, "/dev/wwan0qmi1", dialer)
	if err != nil {
		t.Fatalf("acquire(DATA6) error = %v", err)
	}

	if data5a.transportCore != data5b.transportCore {
		t.Fatal("same device did not share the QMI transport core")
	}
	if data5a.transportCore == data6.transportCore {
		t.Fatal("different devices shared the QMI transport core")
	}
	if got, want := data5a.nextTransactionID(qcom.ServiceUIM), uint16(1); got != want {
		t.Fatalf("DATA5 first transaction ID = %d, want %d", got, want)
	}
	if got, want := data5b.nextTransactionID(qcom.ServiceUIM), uint16(2); got != want {
		t.Fatalf("DATA5 second transaction ID = %d, want %d", got, want)
	}
	if got, want := data6.nextTransactionID(qcom.ServiceUIM), uint16(1); got != want {
		t.Fatalf("DATA6 first transaction ID = %d, want %d", got, want)
	}

	conns := dialer.connections()
	if len(conns) != 2 {
		t.Fatalf("Dial() calls = %d, want 2", len(conns))
	}
	if err := data5a.Close(); err != nil {
		t.Fatalf("DATA5 first Close() error = %v", err)
	}
	if err := data5a.Close(); err != nil {
		t.Fatalf("DATA5 first second Close() error = %v", err)
	}
	if got := conns[0].closes.Load(); got != 0 {
		t.Fatalf("DATA5 connection closes after first lease = %d, want 0", got)
	}
	if err := data5b.Close(); err != nil {
		t.Fatalf("DATA5 second Close() error = %v", err)
	}
	if got := conns[0].closes.Load(); got != 1 {
		t.Fatalf("DATA5 connection closes after last lease = %d, want 1", got)
	}
	if err := data6.Close(); err != nil {
		t.Fatalf("DATA6 Close() error = %v", err)
	}
	if got := conns[1].closes.Load(); got != 1 {
		t.Fatalf("DATA6 connection closes = %d, want 1", got)
	}
}

func TestDirectPoolCloseRemovesOnlyLeaseSubscriptions(t *testing.T) {
	ctx := context.Background()
	pool := newTestDirectPool()
	dialer := new(poolDialer)
	first, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
	if err != nil {
		t.Fatalf("acquire(first) error = %v", err)
	}
	second, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
	if err != nil {
		t.Fatalf("acquire(second) error = %v", err)
	}

	// The pool lifecycle is under test here; suppress the wire reader so the
	// synthetic connection does not terminate both subscriptions with EOF.
	first.readOnce.Do(func() {})
	firstIndications, err := first.Indications(ctx, qcom.ServiceUIM, 1, qcom.MessageGetCardStatus)
	if err != nil {
		t.Fatalf("first Indications() error = %v", err)
	}
	secondIndications, err := second.Indications(ctx, qcom.ServiceUIM, 2, qcom.MessageGetCardStatus)
	if err != nil {
		t.Fatalf("second Indications() error = %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	assertQMIChannelClosed(t, firstIndications)
	if _, err := first.Indications(ctx, qcom.ServiceUIM, 1, qcom.MessageGetCardStatus); err == nil {
		t.Fatal("closed lease Indications() error = nil")
	}
	first.mu.Lock()
	remaining := len(first.subs)
	first.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("subscriptions after first Close() = %d, want 1", remaining)
	}

	if err := second.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	assertQMIChannelClosed(t, secondIndications)
}

func TestDirectPoolReplacesFailedCore(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "read failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := newTestDirectPool()
			dialer := new(poolDialer)

			first, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
			if err != nil {
				t.Fatalf("acquire(first) error = %v", err)
			}
			first.fail(errors.New("malformed QMUX frame"))

			second, err := pool.acquire(ctx, "/dev/wwan0qmi0", dialer)
			if err != nil {
				t.Fatalf("acquire(second) error = %v", err)
			}
			if first.transportCore == second.transportCore {
				t.Fatal("failed QMI transport core was leased again")
			}

			conns := dialer.connections()
			if len(conns) != 2 {
				t.Fatalf("Dial() calls = %d, want 2", len(conns))
			}
			if got := conns[0].closes.Load(); got != 1 {
				t.Fatalf("failed connection closes = %d, want 1", got)
			}
			if err := first.Close(); err != nil {
				t.Fatalf("first Close() error = %v", err)
			}
			if err := second.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
		})
	}
}

func TestDirectPoolWaitsForPhysicalCloseBeforeRedial(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "last lease close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closeStarted := make(chan struct{})
			closeRelease := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(closeRelease) }) })

			dialed := make(chan int, 2)
			dialer := &poolDialer{
				dialed: dialed,
				newConn: func(index int) *poolConn {
					if index == 0 {
						return &poolConn{closeStarted: closeStarted, closeRelease: closeRelease}
					}
					return new(poolConn)
				},
			}
			pool := newTestDirectPool()
			first, err := pool.acquire(context.Background(), "/dev/wwan0qmi0", dialer)
			if err != nil {
				t.Fatalf("acquire(first) error = %v", err)
			}
			<-dialed

			closeResult := make(chan error, 1)
			go func() { closeResult <- first.Close() }()
			<-closeStarted

			type acquireResult struct {
				transport *Transport
				err       error
			}
			acquired := make(chan acquireResult, 1)
			go func() {
				transport, err := pool.acquire(context.Background(), "/dev/wwan0qmi0", dialer)
				acquired <- acquireResult{transport: transport, err: err}
			}()

			select {
			case <-dialed:
				t.Fatal("Dial() started before the previous connection finished closing")
			case <-time.After(50 * time.Millisecond):
			}

			releaseOnce.Do(func() { close(closeRelease) })
			if err := <-closeResult; err != nil {
				t.Fatalf("first Close() error = %v", err)
			}
			if index := <-dialed; index != 1 {
				t.Fatalf("second Dial() index = %d, want 1", index)
			}
			result := <-acquired
			if result.err != nil {
				t.Fatalf("acquire(second) error = %v", result.err)
			}
			if err := result.transport.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
		})
	}
}

func assertQMIChannelClosed(t *testing.T, ch <-chan qcom.Indication) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("indication channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for indication channel to close")
	}
}
