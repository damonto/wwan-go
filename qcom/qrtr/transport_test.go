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

func (c *asyncPacketConn) SetReadDeadline(time.Time) error { return nil }

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
