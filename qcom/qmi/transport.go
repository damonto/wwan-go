package qmi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

type QMUXHeader struct {
	IfType       uint8
	Length       uint16
	ControlFlags uint8
	ServiceType  uint8
	ClientID     uint8
}

type Header[T uint8 | uint16] struct {
	MessageType   qcom.MessageType
	TransactionID T
	MessageID     qcom.MessageID
	MessageLength uint16
}

type Request struct {
	qcom.Request
}

// TransportError reports a terminal transport failure. A QMI transport cannot
// preserve transactions or client IDs after this error and must be reopened.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return e.Err.Error() }

func (e *TransportError) Unwrap() error { return e.Err }

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

type Transport struct {
	*transportCore

	proxy     bool
	shared    bool
	lease     *transportLease
	closeOnce sync.Once
	closeErr  error
	release   func() error
}

type transportLease struct {
	closed atomic.Bool
}

type transportCore struct {
	conn Conn

	writeGate     chan struct{}
	readOnce      sync.Once
	mu            sync.Mutex
	pending       map[messageKey]chan responseResult
	pendingOwners map[messageKey]*transportLease
	subs          map[uint64]*subscription
	nextSub       uint64
	terminalErr   error
	closed        bool
	txn           atomic.Uint32
	ctlTxn        atomic.Uint32
}

// UsesProxy reports whether the transport was opened through qmi-proxy.
func (t *Transport) UsesProxy() bool { return t.proxy }

// TerminalError returns the error which made the transport unusable, if any.
func (t *Transport) TerminalError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.terminalErr
}

func New(conn Conn) *Transport {
	core := newTransportCore(conn)
	transport := &Transport{transportCore: core}
	transport.release = transport.closeCore
	return transport
}

func newTransportCore(conn Conn) *transportCore {
	return &transportCore{
		conn:          conn,
		writeGate:     newWriteGate(),
		pending:       make(map[messageKey]chan responseResult),
		pendingOwners: make(map[messageKey]*transportLease),
		subs:          make(map[uint64]*subscription),
	}
}

func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		if t.lease != nil {
			t.closeLease()
		}
		if t.release != nil {
			t.closeErr = t.release()
		}
	})
	return t.closeErr
}

func (t *Transport) closeLease() {
	if !t.lease.closed.CompareAndSwap(false, true) {
		return
	}

	t.mu.Lock()
	var pending []chan responseResult
	for key, owner := range t.pendingOwners {
		if owner != t.lease {
			continue
		}
		pending = append(pending, t.pending[key])
		delete(t.pending, key)
		delete(t.pendingOwners, key)
	}
	var subs []*subscription
	for id, sub := range t.subs {
		if sub.owner != t.lease {
			continue
		}
		subs = append(subs, sub)
		delete(t.subs, id)
	}
	t.mu.Unlock()

	err := errors.New("QMI transport lease is closed")
	for _, ch := range pending {
		ch <- responseResult{err: err}
	}
	for _, sub := range subs {
		sub.stop()
	}
}

func (t *Transport) closeCore() error {
	err := t.conn.Close()
	t.fail(errors.New("QMI transport is closed"))
	return err
}

func (t *Transport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	if t.shared {
		req.TransactionID = t.nextTransactionID(req.Service)
	}
	packet, err := (Request{Request: req}).MarshalBinary()
	if err != nil {
		return qcom.Response{}, err
	}

	waitCtx, cancel := requestContext(ctx, req.Timeout)
	defer cancel()

	key := messageKey{
		service: qcom.ServiceControl,
		client:  req.ClientID,
		txn:     req.TransactionID,
		message: req.MessageID,
	}
	if req.Service != qcom.ServiceControl {
		key.service = req.Service
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
		if err := t.conn.SetWriteDeadline(deadline); err != nil {
			releaseWrite(t.writeGate)
			t.removePending(key)
			return qcom.Response{}, fmt.Errorf("setting QMI write deadline: %w", err)
		}
	}
	interruptDone := make(chan struct{})
	stopInterrupt := context.AfterFunc(waitCtx, func() {
		defer close(interruptDone)
		// The write result remains authoritative; this deadline only wakes a
		// syscall whose request context has already ended.
		_ = t.conn.SetWriteDeadline(time.Now())
	})
	written, writeErr := writeFull(t.conn, packet)
	if !stopInterrupt() {
		<-interruptDone
	}
	// A stale deadline would incorrectly fail the next writer sharing this core.
	_ = t.conn.SetWriteDeadline(time.Time{})
	releaseWrite(t.writeGate)
	if writeErr != nil {
		if !t.expirePending(key, result) {
			outcome := <-result
			return outcome.resp, outcome.err
		}
		if ctxErr := waitCtx.Err(); ctxErr != nil && writeInterruptedByContext(writeErr) && (written == 0 || written == len(packet)) {
			if written == len(packet) && isClientIDAllocation(req) {
				return qcom.Response{}, t.failUncertainClientIDAllocation(ctxErr)
			}
			return qcom.Response{}, ctxErr
		}
		terminalErr := &TransportError{Err: fmt.Errorf("writing QMI request: %w", writeErr)}
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
		if isClientIDAllocation(req) {
			return qcom.Response{}, t.failUncertainClientIDAllocation(waitCtx.Err())
		}
		return qcom.Response{}, waitCtx.Err()
	}
}

func isClientIDAllocation(req qcom.Request) bool {
	return req.Service == qcom.ServiceControl && req.MessageID == qcom.MessageAllocateClientID
}

func writeInterruptedByContext(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, os.ErrDeadlineExceeded)
}

func (t *Transport) failUncertainClientIDAllocation(err error) error {
	terminalErr := &TransportError{Err: fmt.Errorf("completing QMI client ID allocation: result unknown: %w", err)}
	t.fail(terminalErr)
	return terminalErr
}

func (t *Transport) nextTransactionID(service qcom.ServiceType) uint16 {
	if service == qcom.ServiceControl {
		for {
			txn := uint8(t.ctlTxn.Add(1))
			if txn != 0 {
				return uint16(txn)
			}
		}
	}
	for {
		txn := uint16(t.txn.Add(1))
		if txn != 0 {
			return txn
		}
	}
}

func (t *Transport) Indications(ctx context.Context, service qcom.ServiceType, clientID uint8, id qcom.MessageID) (<-chan qcom.Indication, error) {
	sub := newSubscription(ctx, service, clientID, id)
	sub.owner = t.lease

	t.mu.Lock()
	if t.lease != nil && t.lease.closed.Load() {
		t.mu.Unlock()
		sub.stop()
		return nil, errors.New("QMI transport lease is closed")
	}
	if t.terminalErr != nil {
		t.mu.Unlock()
		sub.stop()
		return nil, t.terminalErr
	}
	if t.closed {
		t.mu.Unlock()
		sub.stop()
		return nil, errors.New("QMI transport is closed")
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
	service qcom.ServiceType
	client  uint8
	txn     uint16
	message qcom.MessageID
}

type responseResult struct {
	resp qcom.Response
	err  error
}

type subscription struct {
	service qcom.ServiceType
	client  uint8
	message qcom.MessageID
	ch      chan qcom.Indication
	notify  chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc
	owner   *transportLease

	stopOnce sync.Once
	mu       sync.Mutex
	queue    []qcom.Indication
	stopped  bool
}

func newSubscription(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	message qcom.MessageID,
) *subscription {
	subCtx, cancel := context.WithCancel(ctx)
	sub := &subscription{
		service: service,
		client:  clientID,
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

func (t *Transport) addPending(key messageKey, ch chan responseResult) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lease != nil && t.lease.closed.Load() {
		return errors.New("QMI transport lease is closed")
	}
	if t.terminalErr != nil {
		return t.terminalErr
	}
	if t.closed {
		return errors.New("QMI transport is closed")
	}
	if _, ok := t.pending[key]; ok {
		return errors.New("QMI request is already pending")
	}
	t.pending[key] = ch
	t.pendingOwners[key] = t.lease
	return nil
}

func (t *Transport) removePending(key messageKey) {
	t.mu.Lock()
	delete(t.pending, key)
	delete(t.pendingOwners, key)
	t.mu.Unlock()
}

func (t *Transport) expirePending(key messageKey, ch chan responseResult) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending[key] != ch {
		return false
	}
	delete(t.pending, key)
	delete(t.pendingOwners, key)
	return true
}

func (t *Transport) removeFinishedSubscription(id uint64, sub *subscription) {
	<-sub.done
	t.mu.Lock()
	if t.subs[id] == sub {
		delete(t.subs, id)
	}
	t.mu.Unlock()
}

func (t *Transport) removeSubscription(id uint64) {
	t.mu.Lock()
	sub := t.subs[id]
	delete(t.subs, id)
	t.mu.Unlock()
	if sub != nil {
		sub.stop()
	}
}

func (t *Transport) startReader() {
	t.readOnce.Do(func() {
		go t.readLoop()
	})
}

func (t *Transport) readLoop() {
	for {
		var wire Response
		if _, err := wire.ReadFrom(t.conn); err != nil {
			if errors.Is(err, errUnexpectedServiceMessageType) {
				continue
			}
			t.fail(&TransportError{Err: fmt.Errorf("reading QMI message: %w", err)})
			return
		}
		switch {
		case wire.isResponse():
			t.deliverResponse(wire.qcomResponse())
		case wire.isIndication():
			t.deliverIndication(wire.qcomIndication())
		}
	}
}

func (t *Transport) deliverResponse(resp qcom.Response) {
	key := messageKey{
		service: resp.Service,
		client:  resp.ClientID,
		txn:     resp.TransactionID,
		message: resp.MessageID,
	}
	if resp.Service == qcom.ServiceControl {
		key.client = 0
	}

	t.mu.Lock()
	ch, ok := t.pending[key]
	if ok {
		delete(t.pending, key)
		delete(t.pendingOwners, key)
	}
	t.mu.Unlock()
	if ok {
		ch <- responseResult{resp: resp}
	}
}

func (t *Transport) deliverIndication(ind qcom.Indication) {
	t.mu.Lock()
	subs := make([]*subscription, 0, len(t.subs))
	for _, sub := range t.subs {
		if sub.service == ind.Service && sub.client == ind.ClientID && sub.message == ind.MessageID {
			subs = append(subs, sub)
		}
	}
	t.mu.Unlock()

	for _, sub := range subs {
		sub.enqueue(ind)
	}
}

func (t *Transport) fail(err error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.terminalErr = err
	pending := t.pending
	t.pending = make(map[messageKey]chan responseResult)
	t.pendingOwners = make(map[messageKey]*transportLease)
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
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	payload, err := req.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal QMI TLVs: %w", err)
	}
	maxPayloadLength := qcom.MaxQMUXServiceTLVLength
	if req.Service == qcom.ServiceControl {
		maxPayloadLength = qcom.MaxQMUXControlTLVLength
	}
	if len(payload) > maxPayloadLength {
		return nil, fmt.Errorf("QMI message TLVs length %d exceeds limit %d", len(payload), maxPayloadLength)
	}

	sdu := new(bytes.Buffer)
	if req.Service == qcom.ServiceControl {
		if err := binary.Write(sdu, binary.LittleEndian, Header[uint8]{
			MessageType:   qcom.MessageTypeRequest,
			TransactionID: uint8(req.TransactionID),
			MessageID:     req.MessageID,
			MessageLength: uint16(len(payload)),
		}); err != nil {
			return nil, fmt.Errorf("write control QMI header: %w", err)
		}
	} else {
		if err := binary.Write(sdu, binary.LittleEndian, Header[uint16]{
			MessageType:   qcom.MessageTypeRequest,
			TransactionID: req.TransactionID,
			MessageID:     req.MessageID,
			MessageLength: uint16(len(payload)),
		}); err != nil {
			return nil, fmt.Errorf("write service QMI header: %w", err)
		}
	}
	if _, err := sdu.Write(payload); err != nil {
		return nil, fmt.Errorf("write QMI payload: %w", err)
	}

	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, QMUXHeader{
		IfType:       qcom.QMUXIfType,
		Length:       uint16(sdu.Len() + 5),
		ControlFlags: qcom.QMUXControlFlagRequest,
		ServiceType:  uint8(req.Service),
		ClientID:     req.ClientID,
	}); err != nil {
		return nil, fmt.Errorf("write QMUX header: %w", err)
	}
	if _, err := out.Write(sdu.Bytes()); err != nil {
		return nil, fmt.Errorf("write QMUX payload: %w", err)
	}
	return out.Bytes(), nil
}

func validateRequest(req qcom.Request) error {
	if req.Service > 0xFF {
		return fmt.Errorf("QMUX service ID 0x%X exceeds 8-bit wire limit", req.Service)
	}
	if req.Service == qcom.ServiceControl && req.ClientID != 0 {
		return fmt.Errorf("QMI control client ID %d is not zero", req.ClientID)
	}
	if req.TransactionID == 0 {
		return errors.New("QMI transaction ID is zero")
	}
	if req.Service == qcom.ServiceControl && req.TransactionID > 0xFF {
		return fmt.Errorf("QMI control transaction ID %d exceeds limit 255", req.TransactionID)
	}
	return nil
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

func writeFull(w io.Writer, p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n, err := w.Write(p)
		written += n
		p = p[n:]
		if err != nil {
			return written, err
		}
		if n <= 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}
