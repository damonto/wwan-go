package qmi

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
	var _ io.ReaderFrom = (*Response)(nil)
}

func TestMarshalRequest(t *testing.T) {
	tests := []struct {
		name string
		req  qcom.Request
		want []byte
	}{
		{
			name: "read transparent request",
			req: qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 3,
				MessageID:     qcom.MessageReadTransparent,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{0x06, 0x00}),
					tlv.Bytes(0x02, []byte{0x07, 0x6F, 0x00}),
					tlv.Bytes(0x03, []byte{0x00, 0x00, 0x09, 0x00}),
				},
			},
			want: []byte{
				0x01, 0x1E, 0x00, 0x00, 0x0B, 0x07,
				0x00, 0x03, 0x00, 0x20, 0x00, 0x12, 0x00,
				0x01, 0x02, 0x00, 0x06, 0x00,
				0x02, 0x03, 0x00, 0x07, 0x6F, 0x00,
				0x03, 0x04, 0x00, 0x00, 0x00, 0x09, 0x00,
			},
		},
		{
			name: "authenticate request",
			req: qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 4,
				MessageID:     qcom.MessageAuthenticate,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{0x00, 0x00}),
					tlv.Bytes(0x02, []byte{0x03, 0x04, 0x00, 0x10, 0x01, 0x10, 0x02}),
				},
			},
			want: []byte{
				0x01, 0x1B, 0x00, 0x00, 0x0B, 0x07,
				0x00, 0x04, 0x00, 0x34, 0x00, 0x0F, 0x00,
				0x01, 0x02, 0x00, 0x00, 0x00,
				0x02, 0x07, 0x00, 0x03, 0x04, 0x00, 0x10, 0x01, 0x10, 0x02,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalRequest(tt.req)
			if err != nil {
				t.Fatalf("MarshalRequest() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalRequest() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestMarshalRequestRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		req  qcom.Request
	}{
		{
			name: "zero transaction",
			req: qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 0,
				MessageID:     qcom.MessageReadTransparent,
			},
		},
		{
			name: "control client id",
			req: qcom.Request{
				Service:       qcom.ServiceControl,
				ClientID:      1,
				TransactionID: 1,
				MessageID:     qcom.MessageGetVersionInfo,
			},
		},
		{
			name: "control overflow",
			req: qcom.Request{
				Service:       qcom.ServiceControl,
				TransactionID: 256,
				MessageID:     qcom.MessageGetVersionInfo,
			},
		},
		{
			name: "extended QRTR service",
			req: qcom.Request{
				Service:       qcom.ServiceType(0x0302),
				TransactionID: 1,
				MessageID:     1,
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

func TestTransportEOFPreservesCause(t *testing.T) {
	tests := []struct {
		name string
		conn *deadlineConn
	}{
		{
			name: "read EOF",
			conn: &deadlineConn{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := New(tt.conn)
			_, err := transport.Do(context.Background(), qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 1,
				MessageID:     qcom.MessageGetCardStatus,
			})
			if !errors.Is(err, io.EOF) {
				t.Fatalf("Do() error = %v, want io.EOF", err)
			}
			if _, ok := errors.AsType[*TransportError](err); !ok {
				t.Fatalf("Do() error = %v, want *TransportError", err)
			}
			if got := transport.TerminalError(); !errors.Is(got, io.EOF) {
				t.Fatalf("TerminalError() = %v, want io.EOF", got)
			}
		})
	}
}

func TestTransportWriteFailureIsTerminal(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "write disconnected", err: errors.New("device disconnected")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &writeErrorConn{
				err:      tt.err,
				closed:   make(chan struct{}),
				readDone: make(chan struct{}),
			}
			transport := New(conn)
			_, err := transport.Do(context.Background(), qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 1,
				MessageID:     qcom.MessageGetCardStatus,
			})
			if !errors.Is(err, tt.err) {
				t.Fatalf("Do() error = %v, want %v", err, tt.err)
			}
			if _, ok := errors.AsType[*TransportError](err); !ok {
				t.Fatalf("Do() error = %v, want *TransportError", err)
			}
			if got := transport.TerminalError(); !errors.Is(got, tt.err) {
				t.Fatalf("TerminalError() = %v, want %v", got, tt.err)
			}
			if err := transport.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			select {
			case <-conn.readDone:
			case <-time.After(time.Second):
				t.Fatal("read loop did not stop after Close")
			}
		})
	}
}

func TestTransportPreservesWriteFailureDuringCancellation(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "device disconnect", err: errors.New("device disconnected")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newCancelRaceWriteConn(tt.err)
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() {
				_, err := transport.Do(ctx, qcom.Request{
					Service:       qcom.ServiceUIM,
					ClientID:      7,
					TransactionID: 1,
					MessageID:     qcom.MessageGetCardStatus,
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
				if _, ok := errors.AsType[*TransportError](err); !ok {
					t.Fatalf("Do() error = %v, want *TransportError", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Do() did not return")
			}
			if terminalErr := transport.TerminalError(); !errors.Is(terminalErr, tt.err) {
				t.Fatalf("TerminalError() = %v, want %v", terminalErr, tt.err)
			}
		})
	}
}

func TestMarshalRequestReturnsTLVError(t *testing.T) {
	req := qcom.Request{
		Service:       qcom.ServiceUIM,
		ClientID:      7,
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs:          tlv.TLVs{{Type: 0x01, Len: 2, Value: []byte{0x01}}},
	}

	if _, err := MarshalRequest(req); err == nil {
		t.Fatal("MarshalRequest() error = nil, want TLV error")
	}
}

func TestResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		frame   []byte
		service qcom.ServiceType
		client  uint8
		txn     uint16
		message qcom.MessageID
	}{
		{
			name: "service response",
			frame: []byte{
				0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
				0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x10, 0x02, 0x00, 0x90, 0x00,
			},
			service: qcom.ServiceUIM,
			client:  7,
			txn:     3,
			message: qcom.MessageReadTransparent,
		},
		{
			name: "control response",
			frame: []byte{
				0x01, 0x12, 0x00, 0x80, 0x00, 0x00,
				0x01, 0x01, 0x00, 0xFF, 0x07, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			service: qcom.ServiceControl,
			client:  0,
			txn:     1,
			message: qcom.MessageInternalProxyOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wire Response
			if err := wire.UnmarshalBinary(tt.frame); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			resp := wire.qcomResponse()
			if resp.Service != tt.service || resp.ClientID != tt.client || resp.TransactionID != tt.txn || resp.MessageID != tt.message {
				t.Fatalf("UnmarshalBinary() = %+v", resp)
			}
			if err := qcom.ResultError(resp.TLVs); err != nil {
				t.Fatalf("Result error = %v", err)
			}
		})
	}
}

func TestResponseClassifiesMessageTypes(t *testing.T) {
	tests := []struct {
		name           string
		frame          []byte
		wantResponse   bool
		wantIndication bool
	}{
		{
			name:           "control response",
			frame:          controlResultFrame(1, qcom.MessageCTLSync),
			wantResponse:   true,
			wantIndication: false,
		},
		{
			name:           "control indication",
			frame:          controlIndicationFrame(0, qcom.MessageCTLSync),
			wantResponse:   false,
			wantIndication: true,
		},
		{
			name:           "service response",
			frame:          serviceResultFrame(3, qcom.MessageReadTransparent),
			wantResponse:   true,
			wantIndication: false,
		},
		{
			name: "service indication",
			frame: []byte{
				0x01, 0x0C, 0x00, 0x80, byte(qcom.ServiceUIM), 0x07,
				byte(qcom.MessageTypeIndication), 0x00, 0x00, byte(qcom.MessageSlotStatus), byte(qcom.MessageSlotStatus >> 8), 0x00, 0x00,
			},
			wantResponse:   false,
			wantIndication: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wire Response
			if err := wire.UnmarshalBinary(tt.frame); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got := wire.isResponse(); got != tt.wantResponse {
				t.Errorf("isResponse() = %t, want %t", got, tt.wantResponse)
			}
			if got := wire.isIndication(); got != tt.wantIndication {
				t.Errorf("isIndication() = %t, want %t", got, tt.wantIndication)
			}
		})
	}
}

func TestTransportDispatchesIndications(t *testing.T) {
	mismatch := []byte{
		0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
		0x02, 0x09, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	indication := []byte{
		0x01, 0x0C, 0x00, 0x80, 0x0B, 0x07,
		0x04, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00,
	}
	match := []byte{
		0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	conn := newAsyncDeadlineConn()
	transport := New(conn)
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	indications, err := transport.Indications(ctx, qcom.ServiceUIM, 7, qcom.MessageSlotStatus)
	if err != nil {
		t.Fatalf("Indications() error = %v", err)
	}

	results := make(chan error, 1)
	go func() {
		_, err := transport.Do(context.Background(), qcom.Request{
			Service:       qcom.ServiceUIM,
			ClientID:      7,
			TransactionID: 3,
			MessageID:     qcom.MessageReadTransparent,
			Timeout:       time.Second,
		})
		results <- err
	}()
	conn.waitWrites(t, 1)
	conn.frames <- mismatch
	conn.frames <- indication
	conn.frames <- match

	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Do()")
	}

	select {
	case ind := <-indications:
		if ind.Service != qcom.ServiceUIM || ind.ClientID != 7 || ind.MessageID != qcom.MessageSlotStatus {
			t.Fatalf("indication = %+v, want slot status for client 7", ind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for indication")
	}
}

func TestTransportDoesNotDeliverControlIndicationAsResponse(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "sync indication before response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newAsyncDeadlineConn()
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			indications, err := transport.Indications(ctx, qcom.ServiceControl, 0, qcom.MessageCTLSync)
			if err != nil {
				t.Fatalf("Indications() error = %v", err)
			}

			results := make(chan error, 1)
			go func() {
				_, err := transport.Do(context.Background(), qcom.Request{
					Service:       qcom.ServiceControl,
					TransactionID: 1,
					MessageID:     qcom.MessageCTLSync,
					Timeout:       time.Second,
				})
				results <- err
			}()
			conn.waitWrites(t, 1)

			conn.frames <- controlIndicationFrame(1, qcom.MessageCTLSync)
			select {
			case ind := <-indications:
				if ind.Service != qcom.ServiceControl || ind.MessageID != qcom.MessageCTLSync {
					t.Fatalf("indication = %+v, want CTL sync", ind)
				}
			case err := <-results:
				t.Fatalf("Do() completed from indication with error %v", err)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for control indication")
			}

			conn.frames <- controlResultFrame(1, qcom.MessageCTLSync)
			select {
			case err := <-results:
				if err != nil {
					t.Fatalf("Do() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for control response")
			}
		})
	}
}

func TestTransportSkipsDirtyServiceMessageTypeFrames(t *testing.T) {
	tests := []struct {
		name  string
		dirty []byte
	}{
		{
			name: "unexpected service message type",
			dirty: func() []byte {
				frame := serviceResultFrame(99, qcom.MessageAuthenticate)
				frame[6] = 0x80
				return frame
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &deadlineConn{read: bytes.NewReader(joinFrames(tt.dirty, serviceResultFrame(3, qcom.MessageReadTransparent)))}
			transport := New(conn)

			_, err := transport.Do(context.Background(), qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 3,
				MessageID:     qcom.MessageReadTransparent,
				Timeout:       time.Second,
			})
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
		})
	}
}

func TestTransportFailsMalformedFrames(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
	}{
		{
			name: "unexpected QMUX marker",
			frame: func() []byte {
				frame := serviceResultFrame(99, qcom.MessageAuthenticate)
				frame[0] = 0xFF
				return frame
			}(),
		},
		{
			name: "unexpected control message type",
			frame: []byte{
				0x01, 0x12, 0x00, 0x80, 0x00, 0x00,
				0x04, 0x01, 0x00, 0xFF, 0x07, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &deadlineConn{read: bytes.NewReader(joinFrames(tt.frame, serviceResultFrame(3, qcom.MessageReadTransparent)))}
			transport := New(conn)

			_, err := transport.Do(context.Background(), qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 3,
				MessageID:     qcom.MessageReadTransparent,
				Timeout:       time.Second,
			})
			if err == nil {
				t.Fatal("Do() error = nil, want malformed frame error")
			}
		})
	}
}

func TestTransportClearsWriteDeadlineBeforeNextWrite(t *testing.T) {
	conn := newAsyncDeadlineConn()
	transport := New(conn)
	defer transport.Close()

	errs := make(chan error, 2)
	go func() {
		_, err := transport.Do(context.Background(), qcom.Request{
			Service:       qcom.ServiceUIM,
			ClientID:      7,
			TransactionID: 3,
			MessageID:     qcom.MessageReadTransparent,
			Timeout:       time.Second,
		})
		errs <- err
	}()
	conn.waitWrites(t, 1)

	go func() {
		_, err := transport.Do(context.Background(), qcom.Request{
			Service:       qcom.ServiceUIM,
			ClientID:      7,
			TransactionID: 4,
			MessageID:     qcom.MessageAuthenticate,
		})
		errs <- err
	}()
	conn.waitWrites(t, 2)

	deadlines := conn.deadlinesAtWrite()
	if deadlines[0].IsZero() {
		t.Fatal("first write deadline is zero, want request timeout deadline")
	}
	if !deadlines[1].IsZero() {
		t.Fatalf("second write deadline = %v, want zero", deadlines[1])
	}

	conn.frames <- serviceResultFrame(3, qcom.MessageReadTransparent)
	conn.frames <- serviceResultFrame(4, qcom.MessageAuthenticate)
	for range 2 {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for Do()")
		}
	}
}

func TestTransportCancelsBlockedWrite(t *testing.T) {
	tests := []struct {
		name    string
		service qcom.ServiceType
		client  uint8
		message qcom.MessageID
		tlvs    tlv.TLVs
	}{
		{
			name:    "ordinary request",
			service: qcom.ServiceUIM,
			client:  7,
			message: qcom.MessageReadTransparent,
		},
		{
			name:    "client ID allocation before any bytes are written",
			service: qcom.ServiceControl,
			message: qcom.MessageAllocateClientID,
			tlvs:    tlv.TLVs{tlv.Uint(0x01, uint8(qcom.ServiceDMS))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newBlockingWriteConn()
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() {
				_, err := transport.Do(ctx, qcom.Request{
					Service:       tt.service,
					ClientID:      tt.client,
					TransactionID: 3,
					MessageID:     tt.message,
					Timeout:       time.Minute,
					TLVs:          tt.tlvs,
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
			if err := transport.TerminalError(); err != nil {
				t.Fatalf("TerminalError() = %v, want nil for an interrupted zero-byte write", err)
			}
		})
	}
}

func TestTransportTerminatesOnUncertainClientIDAllocation(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "allocation response timeout after write"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newAsyncDeadlineConn()
			transport := New(conn)
			defer transport.Close()

			_, err := transport.Do(t.Context(), qcom.Request{
				Service:       qcom.ServiceControl,
				ClientID:      0,
				TransactionID: 1,
				MessageID:     qcom.MessageAllocateClientID,
				Timeout:       20 * time.Millisecond,
				TLVs:          tlv.TLVs{tlv.Uint(0x01, uint8(qcom.ServiceDMS))},
			})
			var transportErr *TransportError
			if !errors.As(err, &transportErr) {
				t.Fatalf("Do() error = %v, want terminal transport error", err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Do() error = %v, want deadline exceeded cause", err)
			}
			if terminalErr := transport.TerminalError(); terminalErr == nil {
				t.Fatal("TerminalError() = nil, want uncertain allocation failure")
			}

			_, nextErr := transport.Do(t.Context(), qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 2,
				MessageID:     qcom.MessageGetCardStatus,
				Timeout:       time.Second,
			})
			if !errors.As(nextErr, &transportErr) {
				t.Fatalf("second Do() error = %v, want terminal transport error", nextErr)
			}
		})
	}
}

func TestTransportCanUnsubscribeWhileDeliveringIndication(t *testing.T) {
	transport := New(&deadlineConn{})
	ind := qcom.Indication{
		Service:   qcom.ServiceUIM,
		ClientID:  7,
		MessageID: qcom.MessageSlotStatus,
	}

	for range 1000 {
		sub := newSubscription(context.Background(), qcom.ServiceUIM, 7, qcom.MessageSlotStatus)
		transport.mu.Lock()
		transport.nextSub++
		id := transport.nextSub
		transport.subs[id] = sub
		transport.mu.Unlock()

		done := make(chan struct{})
		go func() {
			defer close(done)
			transport.deliverIndication(ind)
		}()
		transport.removeSubscription(id)

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
			conn := newAsyncDeadlineConn()
			transport := New(conn)
			defer transport.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			indications, err := transport.Indications(ctx, qcom.ServiceUIM, 7, qcom.MessageSlotStatus)
			if err != nil {
				t.Fatalf("Indications() error = %v", err)
			}

			for i := range tt.count {
				transport.deliverIndication(qcom.Indication{
					Service:       qcom.ServiceUIM,
					ClientID:      7,
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

type deadlineConn struct {
	read *bytes.Reader
}

type blockingWriteConn struct {
	mu              sync.Mutex
	writeDeadline   time.Time
	deadlineChanged chan struct{}
	writeStarted    chan struct{}
	closed          chan struct{}
	startOnce       sync.Once
	closeOnce       sync.Once
}

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{
		deadlineChanged: make(chan struct{}, 1),
		writeStarted:    make(chan struct{}),
		closed:          make(chan struct{}),
	}
}

func (c *blockingWriteConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.EOF
}

func (c *blockingWriteConn) Write([]byte) (int, error) {
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

func (c *blockingWriteConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *blockingWriteConn) SetReadDeadline(time.Time) error { return nil }

func (c *blockingWriteConn) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.writeDeadline = deadline
	c.mu.Unlock()
	select {
	case c.deadlineChanged <- struct{}{}:
	default:
	}
	return nil
}

func (c *deadlineConn) Read(p []byte) (int, error) {
	if c.read == nil {
		return 0, io.EOF
	}
	return c.read.Read(p)
}

func (c *deadlineConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *deadlineConn) Close() error                     { return nil }
func (c *deadlineConn) SetReadDeadline(time.Time) error  { return nil }
func (c *deadlineConn) SetWriteDeadline(time.Time) error { return nil }

type writeErrorConn struct {
	err       error
	closed    chan struct{}
	readDone  chan struct{}
	closeOnce sync.Once
}

type cancelRaceWriteConn struct {
	err          error
	writeStarted chan struct{}
	writeRelease chan struct{}
	closed       chan struct{}
	startOnce    sync.Once
	closeOnce    sync.Once
}

func newCancelRaceWriteConn(err error) *cancelRaceWriteConn {
	return &cancelRaceWriteConn{
		err:          err,
		writeStarted: make(chan struct{}),
		writeRelease: make(chan struct{}),
		closed:       make(chan struct{}),
	}
}

func (c *cancelRaceWriteConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.ErrClosedPipe
}

func (c *cancelRaceWriteConn) Write([]byte) (int, error) {
	c.startOnce.Do(func() {
		close(c.writeStarted)
	})
	<-c.writeRelease
	return 0, c.err
}

func (c *cancelRaceWriteConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *cancelRaceWriteConn) SetReadDeadline(time.Time) error  { return nil }
func (c *cancelRaceWriteConn) SetWriteDeadline(time.Time) error { return nil }

func (c *writeErrorConn) Read([]byte) (int, error) {
	<-c.closed
	close(c.readDone)
	return 0, io.ErrClosedPipe
}

func (c *writeErrorConn) Write([]byte) (int, error)        { return 0, c.err }
func (c *writeErrorConn) SetReadDeadline(time.Time) error  { return nil }
func (c *writeErrorConn) SetWriteDeadline(time.Time) error { return nil }

func (c *writeErrorConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

type asyncDeadlineConn struct {
	mu             sync.Mutex
	frames         chan []byte
	readBuf        []byte
	writeDeadline  time.Time
	writeDeadlines []time.Time
	writeSignals   chan struct{}
	closeOnce      sync.Once
}

func newAsyncDeadlineConn() *asyncDeadlineConn {
	return &asyncDeadlineConn{
		frames:       make(chan []byte, 4),
		writeSignals: make(chan struct{}, 4),
	}
}

func (c *asyncDeadlineConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if len(c.readBuf) > 0 {
			n := copy(p, c.readBuf)
			c.readBuf = c.readBuf[n:]
			c.mu.Unlock()
			return n, nil
		}
		c.mu.Unlock()

		frame, ok := <-c.frames
		if !ok {
			return 0, io.EOF
		}
		c.mu.Lock()
		c.readBuf = append(c.readBuf, frame...)
		c.mu.Unlock()
	}
}

func (c *asyncDeadlineConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writeDeadlines = append(c.writeDeadlines, c.writeDeadline)
	c.mu.Unlock()

	select {
	case c.writeSignals <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (c *asyncDeadlineConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.frames)
	})
	return nil
}

func (c *asyncDeadlineConn) SetReadDeadline(time.Time) error { return nil }

func (c *asyncDeadlineConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *asyncDeadlineConn) waitWrites(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		got := len(c.writeDeadlines)
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

func (c *asyncDeadlineConn) deadlinesAtWrite() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Time(nil), c.writeDeadlines...)
}

func serviceResultFrame(txn uint16, message qcom.MessageID) []byte {
	return []byte{
		0x01, 0x13, 0x00, 0x80, byte(qcom.ServiceUIM), 0x07,
		byte(qcom.MessageTypeResponse), byte(txn), byte(txn >> 8), byte(message), byte(message >> 8), 0x07, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func controlResultFrame(txn uint8, message qcom.MessageID) []byte {
	return []byte{
		0x01, 0x12, 0x00, 0x80, byte(qcom.ServiceControl), 0x00,
		byte(controlMessageTypeResponse), txn, byte(message), byte(message >> 8), 0x07, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func controlIndicationFrame(txn uint8, message qcom.MessageID) []byte {
	return []byte{
		0x01, 0x0B, 0x00, 0x80, byte(qcom.ServiceControl), 0x00,
		byte(controlMessageTypeIndication), txn, byte(message), byte(message >> 8), 0x00, 0x00,
	}
}

func joinFrames(frames ...[]byte) []byte {
	var out []byte
	for _, frame := range frames {
		out = append(out, frame...)
	}
	return out
}
