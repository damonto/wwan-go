package qrtr

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestResponseImplementsStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = Request{}
	var _ encoding.BinaryUnmarshaler = (*Response)(nil)
	var _ io.WriterTo = Request{}
}

func TestMarshalRequest(t *testing.T) {
	req := qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte{0x06, 0x00}),
			tlv.Bytes(0x02, []byte{0x07, 0x6F, 0x00}),
			tlv.Bytes(0x03, []byte{0x00, 0x00, 0x09, 0x00}),
		},
	}
	want := []byte{
		0x00, 0x03, 0x00, 0x20, 0x00, 0x12, 0x00,
		0x01, 0x02, 0x00, 0x06, 0x00,
		0x02, 0x03, 0x00, 0x07, 0x6F, 0x00,
		0x03, 0x04, 0x00, 0x00, 0x00, 0x09, 0x00,
	}
	got, err := MarshalRequest(req)
	if err != nil {
		t.Fatalf("MarshalRequest() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("MarshalRequest() = % X, want % X", got, want)
	}
}

func TestMarshalRequestReturnsTLVError(t *testing.T) {
	req := qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs:          tlv.TLVs{{Type: 0x01, Len: 2, Value: []byte{0x01}}},
	}

	if _, err := MarshalRequest(req); err == nil {
		t.Fatal("MarshalRequest() error = nil, want TLV error")
	}
}

func TestMarshalRequestRejectsZeroTransactionID(t *testing.T) {
	tests := []struct {
		name string
		req  qcom.Request
	}{
		{
			name: "zero transaction",
			req: qcom.Request{
				TransactionID: 0,
				MessageID:     qcom.MessageReadTransparent,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := MarshalRequest(tt.req); err == nil {
				t.Fatal("MarshalRequest() error = nil, want transaction ID error")
			}
		})
	}
}

func TestResponseUnmarshalBinary(t *testing.T) {
	frame := []byte{
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}

	var wire Response
	if err := wire.UnmarshalBinary(frame); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	resp := wire.qcomResponse(qcom.ServiceUIM)
	if resp.TransactionID != 3 || resp.MessageID != qcom.MessageReadTransparent {
		t.Fatalf("UnmarshalBinary() = %+v", resp)
	}
	if err := qcom.ResultError(resp.TLVs); err != nil {
		t.Fatalf("Result error = %v", err)
	}
}

func TestTransportDispatchesIndications(t *testing.T) {
	mismatch := []byte{
		0x02, 0x09, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	indication := []byte{
		0x04, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00,
	}
	match := []byte{
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	conn := newAsyncPacketConn()
	transport := New(conn)
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	indications, err := transport.Indications(ctx, qcom.ServiceUIM, 0, qcom.MessageSlotStatus)
	if err != nil {
		t.Fatalf("Indications() error = %v", err)
	}

	errs := make(chan error, 1)
	go func() {
		_, err := transport.Do(context.Background(), qcom.Request{
			Service:       qcom.ServiceUIM,
			TransactionID: 3,
			MessageID:     qcom.MessageReadTransparent,
			Timeout:       time.Second,
		})
		errs <- err
	}()
	conn.waitWrites(t, 1)
	conn.frames <- mismatch
	conn.frames <- indication
	conn.frames <- match

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}

	select {
	case ind := <-indications:
		if ind.Service != qcom.ServiceUIM || ind.MessageID != qcom.MessageSlotStatus {
			t.Fatalf("indication = %+v, want slot status", ind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for indication")
	}
}

func TestTransportCancelsBlockedWrite(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "request context canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newBlockingPacketConn()
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() {
				_, err := transport.Do(ctx, qcom.Request{
					Service:       qcom.ServiceUIM,
					TransactionID: 3,
					MessageID:     qcom.MessageReadTransparent,
					Timeout:       time.Minute,
				})
				result <- err
			}()

			select {
			case <-conn.writeStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for blocked write")
			}
			cancel()

			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Do() error = %v, want context canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Do() did not return after cancellation")
			}
		})
	}
}

func TestTransportPreservesWriteFailureDuringCancellation(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "service disconnect", err: errors.New("service disconnected")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newCancelRacePacketConn(tt.err)
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() {
				_, err := transport.Do(ctx, qcom.Request{
					Service:       qcom.ServiceUIM,
					TransactionID: 3,
					MessageID:     qcom.MessageReadTransparent,
					Timeout:       time.Minute,
				})
				result <- err
			}()

			select {
			case <-conn.writeStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for write")
			}
			cancel()
			close(conn.writeRelease)

			select {
			case err := <-result:
				if !errors.Is(err, tt.err) {
					t.Fatalf("Do() error = %v, want %v", err, tt.err)
				}
				if errors.Is(err, context.Canceled) {
					t.Fatalf("Do() error = %v, should preserve the write failure", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Do() did not return")
			}

			_, nextErr := transport.Do(t.Context(), qcom.Request{
				Service:       qcom.ServiceUIM,
				TransactionID: 4,
				MessageID:     qcom.MessageGetCardStatus,
			})
			if !errors.Is(nextErr, tt.err) {
				t.Fatalf("second Do() error = %v, want terminal %v", nextErr, tt.err)
			}
		})
	}
}

func TestTransportReturnsResponseClaimedAtCancellation(t *testing.T) {
	tests := []struct {
		name       string
		iterations int
	}{
		{name: "response wins after pending claim", iterations: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range tt.iterations {
				conn := newCallbackPacketConn()
				transport := New(conn)
				ctx, cancel := context.WithCancel(t.Context())
				conn.onWrite = func() {
					cancel()
					transport.mu.Lock()
					endpoint := transport.services[qcom.ServiceUIM]
					transport.mu.Unlock()
					endpoint.deliverResponse(qcom.Response{
						Service:       qcom.ServiceUIM,
						TransactionID: 3,
						MessageID:     qcom.MessageReadTransparent,
					})
				}

				resp, err := transport.Do(ctx, qcom.Request{
					Service:       qcom.ServiceUIM,
					TransactionID: 3,
					MessageID:     qcom.MessageReadTransparent,
					Timeout:       time.Minute,
				})
				if err != nil {
					t.Fatalf("Do() error = %v, want claimed response", err)
				}
				if resp.MessageID != qcom.MessageReadTransparent {
					t.Fatalf("response MessageID = 0x%04X, want 0x%04X", resp.MessageID, qcom.MessageReadTransparent)
				}
				if err := transport.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
		})
	}
}

func TestTransportRejectsWrongService(t *testing.T) {
	transport := New(&deadlinePacketConn{})

	_, err := transport.Do(context.Background(), qcom.Request{
		Service:       qcom.ServiceControl,
		TransactionID: 1,
		MessageID:     qcom.MessageGetVersionInfo,
	})
	if err == nil {
		t.Fatal("Do() error = nil, want service mismatch")
	}
}

func TestTransportRejectsWrongIndicationService(t *testing.T) {
	transport := New(&deadlinePacketConn{})

	_, err := transport.Indications(context.Background(), qcom.ServiceCAT2, 0, qcom.MessageSendEnvelope)
	if err == nil {
		t.Fatal("Indications() error = nil, want service mismatch")
	}
}

func TestTransportRequiresWriteDeadlineSupport(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "custom connection without write deadline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := New(readDeadlineOnlyPacketConn{})
			defer transport.Close()
			_, err := transport.Do(t.Context(), qcom.Request{
				Service:       qcom.ServiceUIM,
				TransactionID: 1,
				MessageID:     qcom.MessageGetCardStatus,
			})
			if err == nil {
				t.Fatal("Do() error = nil, want missing write deadline support")
			}
		})
	}
}

func TestTransportEOFPreservesCause(t *testing.T) {
	tests := []struct {
		name string
		conn *deadlinePacketConn
	}{
		{
			name: "read EOF",
			conn: &deadlinePacketConn{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := New(tt.conn)
			_, err := transport.Do(context.Background(), qcom.Request{
				Service:       qcom.ServiceUIM,
				TransactionID: 1,
				MessageID:     qcom.MessageGetCardStatus,
			})
			if !errors.Is(err, io.EOF) {
				t.Fatalf("Do() error = %v, want io.EOF", err)
			}
		})
	}
}

func TestTransportUsesBoundServiceInResponses(t *testing.T) {
	frame := []byte{
		0x02, 0x03, 0x00, byte(qcom.MessageSendEnvelope), byte(qcom.MessageSendEnvelope >> 8), 0x07, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	transport := newTransport(&deadlinePacketConn{frames: [][]byte{frame}}, qcom.ServiceCAT2)

	resp, err := transport.Do(context.Background(), qcom.Request{
		Service:       qcom.ServiceCAT2,
		TransactionID: 3,
		MessageID:     qcom.MessageSendEnvelope,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.Service != qcom.ServiceCAT2 {
		t.Fatalf("response service = %#x, want %#x", resp.Service, qcom.ServiceCAT2)
	}
}

func TestTransportCanUnsubscribeWhileDeliveringIndication(t *testing.T) {
	transport := New(&deadlinePacketConn{})
	transport.mu.Lock()
	endpoint := transport.services[qcom.ServiceUIM]
	transport.mu.Unlock()
	ind := qcom.Indication{
		Service:   qcom.ServiceUIM,
		MessageID: qcom.MessageSlotStatus,
	}

	for range 1000 {
		sub := newSubscription(context.Background(), qcom.MessageSlotStatus)
		endpoint.mu.Lock()
		endpoint.nextSub++
		id := endpoint.nextSub
		endpoint.subs[id] = sub
		endpoint.mu.Unlock()

		done := make(chan struct{})
		go func() {
			defer close(done)
			endpoint.deliverIndication(ind)
		}()
		endpoint.removeSubscription(id)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for indication delivery")
		}
	}
}

func TestTransportQueuesSlowSubscriberIndications(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "more than channel capacity", count: 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newAsyncPacketConn()
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			indications, err := transport.Indications(ctx, qcom.ServiceUIM, 0, qcom.MessageSlotStatus)
			if err != nil {
				t.Fatalf("Indications() error = %v", err)
			}

			transport.mu.Lock()
			endpoint := transport.services[qcom.ServiceUIM]
			transport.mu.Unlock()
			for i := range tt.count {
				endpoint.deliverIndication(qcom.Indication{
					Service:       qcom.ServiceUIM,
					TransactionID: uint16(i + 1),
					MessageID:     qcom.MessageSlotStatus,
				})
			}

			for i := range tt.count {
				select {
				case ind := <-indications:
					if want := uint16(i + 1); ind.TransactionID != want {
						t.Fatalf("indication %d transaction = %d, want %d", i, ind.TransactionID, want)
					}
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for indication %d", i)
				}
			}
		})
	}
}

type deadlinePacketConn struct {
	frames [][]byte
}

type readDeadlineOnlyPacketConn struct{}

func (readDeadlineOnlyPacketConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (readDeadlineOnlyPacketConn) Write(p []byte) (int, error)     { return len(p), nil }
func (readDeadlineOnlyPacketConn) Close() error                    { return nil }
func (readDeadlineOnlyPacketConn) SetReadDeadline(time.Time) error { return nil }

type blockingPacketConn struct {
	mu              sync.Mutex
	writeDeadline   time.Time
	deadlineChanged chan struct{}
	writeStarted    chan struct{}
	closed          chan struct{}
	startOnce       sync.Once
	closeOnce       sync.Once
}

type cancelRacePacketConn struct {
	err          error
	writeStarted chan struct{}
	writeRelease chan struct{}
	closed       chan struct{}
	startOnce    sync.Once
	closeOnce    sync.Once
}

func newCancelRacePacketConn(err error) *cancelRacePacketConn {
	return &cancelRacePacketConn{
		err:          err,
		writeStarted: make(chan struct{}),
		writeRelease: make(chan struct{}),
		closed:       make(chan struct{}),
	}
}

func (c *cancelRacePacketConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.ErrClosedPipe
}

func (c *cancelRacePacketConn) Write([]byte) (int, error) {
	c.startOnce.Do(func() {
		close(c.writeStarted)
	})
	<-c.writeRelease
	return 0, c.err
}

func (c *cancelRacePacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *cancelRacePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *cancelRacePacketConn) SetWriteDeadline(time.Time) error { return nil }

type callbackPacketConn struct {
	onWrite   func()
	closed    chan struct{}
	closeOnce sync.Once
}

func newCallbackPacketConn() *callbackPacketConn {
	return &callbackPacketConn{closed: make(chan struct{})}
}

func (c *callbackPacketConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.EOF
}

func (c *callbackPacketConn) Write(p []byte) (int, error) {
	c.onWrite()
	return len(p), nil
}

func (c *callbackPacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *callbackPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *callbackPacketConn) SetWriteDeadline(time.Time) error { return nil }

func newBlockingPacketConn() *blockingPacketConn {
	return &blockingPacketConn{
		deadlineChanged: make(chan struct{}, 1),
		writeStarted:    make(chan struct{}),
		closed:          make(chan struct{}),
	}
}

func (c *blockingPacketConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.EOF
}

func (c *blockingPacketConn) Write([]byte) (int, error) {
	c.startOnce.Do(func() {
		close(c.writeStarted)
	})
	for {
		c.mu.Lock()
		deadline := c.writeDeadline
		c.mu.Unlock()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, context.DeadlineExceeded
		}
		select {
		case <-c.deadlineChanged:
		case <-c.closed:
			return 0, io.ErrClosedPipe
		}
	}
}

func (c *blockingPacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *blockingPacketConn) SetReadDeadline(time.Time) error { return nil }

func (c *blockingPacketConn) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.writeDeadline = deadline
	c.mu.Unlock()
	select {
	case c.deadlineChanged <- struct{}{}:
	default:
	}
	return nil
}

func (c *deadlinePacketConn) Read(p []byte) (int, error) {
	if len(c.frames) == 0 {
		return 0, io.EOF
	}
	frame := c.frames[0]
	c.frames = c.frames[1:]
	return copy(p, frame), nil
}

func (c *deadlinePacketConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *deadlinePacketConn) Close() error                { return nil }
func (c *deadlinePacketConn) SetReadDeadline(time.Time) error {
	return nil
}
func (c *deadlinePacketConn) SetWriteDeadline(time.Time) error { return nil }

type asyncPacketConn struct {
	mu           sync.Mutex
	frames       chan []byte
	writes       int
	writeSignals chan struct{}
	closeOnce    sync.Once
}

func newAsyncPacketConn() *asyncPacketConn {
	return &asyncPacketConn{
		frames:       make(chan []byte, 4),
		writeSignals: make(chan struct{}, 4),
	}
}

func (c *asyncPacketConn) Read(p []byte) (int, error) {
	frame, ok := <-c.frames
	if !ok {
		return 0, io.EOF
	}
	return copy(p, frame), nil
}

func (c *asyncPacketConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes++
	c.mu.Unlock()

	select {
	case c.writeSignals <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (c *asyncPacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.frames)
	})
	return nil
}

func (c *asyncPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *asyncPacketConn) SetWriteDeadline(time.Time) error { return nil }

func (c *asyncPacketConn) waitWrites(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		got := c.writes
		c.mu.Unlock()
		if got >= want {
			return
		}
		select {
		case <-c.writeSignals:
		case <-deadline:
			tb.Fatalf("writes = %d, want at least %d", got, want)
		}
	}
}
