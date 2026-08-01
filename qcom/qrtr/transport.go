package qrtr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

var errTransportClosed = errors.New("QRTR transport is closed")

type packetConn interface {
	io.ReadWriter
	SetReadDeadline(time.Time) error
	Close() error
}

type writeDeadlineSetter interface {
	SetWriteDeadline(time.Time) error
}

type Header struct {
	MessageType   qcom.MessageType
	TransactionID uint16
	MessageID     qcom.MessageID
	MessageLength uint16
}

type Request struct {
	qcom.Request
}

func (r Request) MarshalBinary() ([]byte, error) {
	return marshalRequest(r.Request)
}

func (r Request) WriteTo(w io.Writer) (int64, error) {
	data, err := r.MarshalBinary()
	if err != nil {
		return 0, err
	}
	var written int64
	for len(data) > 0 {
		n, err := w.Write(data)
		written += int64(n)
		data = data[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

// Transport multiplexes QMI services over separate QRTR client sockets.
// Qualcomm's QRTR QCCI transport opens one socket for each service client;
// the map here gives a qcom.Client the same multi-service view as QMUX.
type Transport struct {
	dialer Dialer

	mu       sync.Mutex
	services map[qcom.ServiceType]*serviceTransport
	closed   bool
}

// New creates a transport bound to a caller-provided UIM connection. It is
// primarily useful for tests and custom connection ownership. Open should be
// used when the client needs more than one QMI service.
func New(conn packetConn) *Transport {
	return newTransport(conn, qcom.ServiceUIM)
}

func newTransport(conn packetConn, service qcom.ServiceType) *Transport {
	return &Transport{
		services: map[qcom.ServiceType]*serviceTransport{
			service: newServiceTransport(conn, service),
		},
	}
}

func newDialingTransport(dialer Dialer, service qcom.ServiceType, conn packetConn) *Transport {
	t := newTransport(conn, service)
	t.dialer = dialer
	return t
}

// ClientID reports the implicit QRTR client ID. Establishing the service
// socket here lets qcom.Client avoid the QMUX-only CTL Allocate Client ID flow.
func (t *Transport) ClientID(ctx context.Context, service qcom.ServiceType) (uint8, error) {
	if _, err := t.endpoint(ctx, service); err != nil {
		return 0, err
	}
	return 0, nil
}

func (t *Transport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errTransportClosed
	}
	t.closed = true
	services := t.services
	t.services = nil
	t.mu.Unlock()

	var closeErr error
	for _, service := range services {
		closeErr = errors.Join(closeErr, service.Close())
	}
	return closeErr
}

func (t *Transport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	service, err := t.endpoint(ctx, req.Service)
	if err != nil {
		return qcom.Response{}, err
	}
	return service.Do(ctx, req)
}

func (t *Transport) Indications(
	ctx context.Context,
	service qcom.ServiceType,
	_ uint8,
	id qcom.MessageID,
) (<-chan qcom.Indication, error) {
	endpoint, err := t.endpoint(ctx, service)
	if err != nil {
		return nil, err
	}
	return endpoint.Indications(ctx, id)
}

func (t *Transport) endpoint(ctx context.Context, service qcom.ServiceType) (*serviceTransport, error) {
	if service == qcom.ServiceControl {
		return nil, errors.New("QRTR does not expose the QMI control service")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, errTransportClosed
	}
	if endpoint := t.services[service]; endpoint != nil {
		return endpoint, nil
	}
	if t.dialer == nil {
		return nil, fmt.Errorf("QRTR transport is not configured to open service 0x%X", service)
	}

	conn, err := t.dialer.Dial(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("opening QRTR service 0x%X: %w", service, err)
	}
	endpoint := newServiceTransport(conn, service)
	t.services[service] = endpoint
	return endpoint, nil
}

type serviceTransport struct {
	conn    packetConn
	service qcom.ServiceType

	writeGate chan struct{}
	readOnce  sync.Once
	mu        sync.Mutex
	pending   map[messageKey]chan responseResult
	subs      map[uint64]*subscription
	nextSub   uint64
	readErr   error
	closed    bool
}

func newServiceTransport(conn packetConn, service qcom.ServiceType) *serviceTransport {
	return &serviceTransport{
		conn:      conn,
		service:   service,
		writeGate: newWriteGate(),
		pending:   make(map[messageKey]chan responseResult),
		subs:      make(map[uint64]*subscription),
	}
}

func (t *serviceTransport) Close() error {
	err := t.conn.Close()
	t.fail(errTransportClosed)
	return err
}

func (t *serviceTransport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	if req.Service != t.service {
		return qcom.Response{}, fmt.Errorf("QRTR service endpoint 0x%X received request for service 0x%X", t.service, req.Service)
	}
	deadlineWriter, ok := t.conn.(writeDeadlineSetter)
	if !ok {
		return qcom.Response{}, errors.New("QRTR connection does not support write deadlines")
	}

	packet, err := (Request{Request: req}).MarshalBinary()
	if err != nil {
		return qcom.Response{}, err
	}

	waitCtx, cancel := requestContext(ctx, req.Timeout)
	defer cancel()

	key := messageKey{
		txn:     req.TransactionID,
		message: req.MessageID,
	}
	result := make(chan responseResult, 1)
	if err := t.addPending(key, result); err != nil {
		return qcom.Response{}, err
	}
	t.startReader()

	if err := acquireWrite(waitCtx, t.writeGate); err != nil {
		t.removePending(key)
		return qcom.Response{}, err
	}
	if err := waitCtx.Err(); err != nil {
		releaseWrite(t.writeGate)
		t.removePending(key)
		return qcom.Response{}, err
	}

	deadline, hasDeadline := waitCtx.Deadline()
	if hasDeadline {
		if err := deadlineWriter.SetWriteDeadline(deadline); err != nil {
			releaseWrite(t.writeGate)
			t.removePending(key)
			return qcom.Response{}, fmt.Errorf("setting QRTR write deadline: %w", err)
		}
	}
	interruptDone := make(chan struct{})
	stopInterrupt := context.AfterFunc(waitCtx, func() {
		defer close(interruptDone)
		// The write result remains authoritative; this deadline only wakes a
		// syscall whose request context has already ended.
		_ = deadlineWriter.SetWriteDeadline(time.Now())
	})
	written, writeErr := t.conn.Write(packet)
	if writeErr == nil && written != len(packet) {
		writeErr = io.ErrShortWrite
	}
	if !stopInterrupt() {
		<-interruptDone
	}
	// A stale deadline would incorrectly fail the next request on this service.
	_ = deadlineWriter.SetWriteDeadline(time.Time{})
	releaseWrite(t.writeGate)
	if writeErr != nil {
		if !t.expirePending(key, result) {
			outcome := <-result
			return outcome.resp, outcome.err
		}
		if ctxErr := waitCtx.Err(); ctxErr != nil && writeInterruptedByContext(writeErr) && (written == 0 || written == len(packet)) {
			return qcom.Response{}, ctxErr
		}
		terminalErr := fmt.Errorf("writing QRTR request: %w", writeErr)
		t.fail(terminalErr)
		return qcom.Response{}, terminalErr
	}

	select {
	case result := <-result:
		return result.resp, result.err
	case <-waitCtx.Done():
		if !t.expirePending(key, result) {
			outcome := <-result
			return outcome.resp, outcome.err
		}
		return qcom.Response{}, waitCtx.Err()
	}
}

func writeInterruptedByContext(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, os.ErrDeadlineExceeded)
}

func (t *serviceTransport) Indications(ctx context.Context, id qcom.MessageID) (<-chan qcom.Indication, error) {
	sub := newSubscription(ctx, id)

	t.mu.Lock()
	if t.readErr != nil {
		t.mu.Unlock()
		sub.stop()
		return nil, t.readErr
	}
	if t.closed {
		t.mu.Unlock()
		sub.stop()
		return nil, errTransportClosed
	}
	t.nextSub++
	idn := t.nextSub
	t.subs[idn] = sub
	t.mu.Unlock()

	t.startReader()
	go t.removeFinishedSubscription(idn, sub)
	return sub.ch, nil
}

type messageKey struct {
	txn     uint16
	message qcom.MessageID
}

type responseResult struct {
	resp qcom.Response
	err  error
}

type subscription struct {
	message qcom.MessageID
	ch      chan qcom.Indication
	notify  chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc

	stopOnce sync.Once
	mu       sync.Mutex
	queue    []qcom.Indication
	stopped  bool
}

func newSubscription(ctx context.Context, message qcom.MessageID) *subscription {
	subCtx, cancel := context.WithCancel(ctx)
	sub := &subscription{
		message: message,
		ch:      make(chan qcom.Indication, 16),
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
		cancel:  cancel,
	}
	go sub.run(subCtx)
	return sub
}

func (s *subscription) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.ch)

	for {
		if ind, ok := s.next(); ok {
			select {
			case s.ch <- ind:
			case <-ctx.Done():
				s.flushBuffered(ind)
				return
			}
			continue
		}

		select {
		case <-s.notify:
		case <-ctx.Done():
			s.flushBuffered()
			return
		}
	}
}

func (s *subscription) flushBuffered(first ...qcom.Indication) {
	for _, ind := range first {
		select {
		case s.ch <- ind:
		default:
			return
		}
	}
	for {
		ind, ok := s.next()
		if !ok {
			return
		}
		select {
		case s.ch <- ind:
		default:
			return
		}
	}
}

func (s *subscription) next() (qcom.Indication, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return qcom.Indication{}, false
	}

	ind := s.queue[0]
	s.queue[0] = qcom.Indication{}
	s.queue = s.queue[1:]
	if len(s.queue) == 0 {
		s.queue = nil
	}
	return ind, true
}

func (s *subscription) enqueue(ind qcom.Indication) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, ind)
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *subscription) stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		s.cancel()
	})
	<-s.done
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := qcom.RequestDeadline(ctx, timeout)
	if !ok {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, deadline)
}

func (t *serviceTransport) addPending(key messageKey, ch chan responseResult) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.readErr != nil {
		return t.readErr
	}
	if t.closed {
		return errTransportClosed
	}
	if _, ok := t.pending[key]; ok {
		return errors.New("QRTR request is already pending")
	}
	t.pending[key] = ch
	return nil
}

func (t *serviceTransport) removePending(key messageKey) {
	t.mu.Lock()
	delete(t.pending, key)
	t.mu.Unlock()
}

func (t *serviceTransport) expirePending(key messageKey, ch chan responseResult) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending[key] != ch {
		return false
	}
	delete(t.pending, key)
	return true
}

func (t *serviceTransport) removeFinishedSubscription(id uint64, sub *subscription) {
	<-sub.done
	t.mu.Lock()
	if t.subs[id] == sub {
		delete(t.subs, id)
	}
	t.mu.Unlock()
}

func (t *serviceTransport) removeSubscription(id uint64) {
	t.mu.Lock()
	sub := t.subs[id]
	delete(t.subs, id)
	t.mu.Unlock()
	if sub != nil {
		sub.stop()
	}
}

func (t *serviceTransport) startReader() {
	t.readOnce.Do(func() {
		go t.readLoop()
	})
}

func (t *serviceTransport) readLoop() {
	for {
		buf := make([]byte, qcom.MaxQRTRQMIMessageLength)
		n, err := t.conn.Read(buf)
		if err != nil {
			t.fail(fmt.Errorf("reading QRTR QMI message: %w", err))
			return
		}

		var wire Response
		if err := wire.UnmarshalBinary(buf[:n]); err != nil {
			t.fail(err)
			return
		}
		switch wire.MessageType {
		case qcom.MessageTypeResponse:
			t.deliverResponse(wire.qcomResponse(t.service))
		case qcom.MessageTypeIndication:
			t.deliverIndication(wire.qcomIndication(t.service))
		}
	}
}

func (t *serviceTransport) deliverResponse(resp qcom.Response) {
	key := messageKey{
		txn:     resp.TransactionID,
		message: resp.MessageID,
	}

	t.mu.Lock()
	ch, ok := t.pending[key]
	if ok {
		delete(t.pending, key)
	}
	t.mu.Unlock()
	if ok {
		ch <- responseResult{resp: resp}
	}
}

func (t *serviceTransport) deliverIndication(ind qcom.Indication) {
	t.mu.Lock()
	subs := make([]*subscription, 0, len(t.subs))
	for _, sub := range t.subs {
		if sub.message == ind.MessageID {
			subs = append(subs, sub)
		}
	}
	t.mu.Unlock()

	for _, sub := range subs {
		sub.enqueue(ind)
	}
}

func (t *serviceTransport) fail(err error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.readErr = err
	pending := t.pending
	t.pending = make(map[messageKey]chan responseResult)
	subs := t.subs
	t.subs = make(map[uint64]*subscription)
	t.mu.Unlock()

	for _, ch := range pending {
		ch <- responseResult{err: err}
	}
	for _, sub := range subs {
		sub.stop()
	}
}

func MarshalRequest(req qcom.Request) ([]byte, error) {
	return (Request{Request: req}).MarshalBinary()
}

func marshalRequest(req qcom.Request) ([]byte, error) {
	if req.TransactionID == 0 {
		return nil, errors.New("QRTR QMI transaction ID is zero")
	}

	payload, err := req.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal QRTR QMI TLVs: %w", err)
	}
	if len(payload) > qcom.MaxQRTRServiceTLVLength {
		return nil, fmt.Errorf("QRTR QMI message TLVs length %d exceeds limit %d", len(payload), qcom.MaxQRTRServiceTLVLength)
	}

	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, Header{
		MessageType:   qcom.MessageTypeRequest,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		MessageLength: uint16(len(payload)),
	}); err != nil {
		return nil, fmt.Errorf("write QRTR QMI header: %w", err)
	}
	if _, err := out.Write(payload); err != nil {
		return nil, fmt.Errorf("write QRTR QMI payload: %w", err)
	}
	return out.Bytes(), nil
}

func newWriteGate() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}

func acquireWrite(ctx context.Context, gate <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func releaseWrite(gate chan<- struct{}) {
	gate <- struct{}{}
}
