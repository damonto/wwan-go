package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const maxQueuedIndications = 32

var (
	errReaderClosed    = errors.New("MBIM reader is closed")
	errReceiverStopped = errors.New("MBIM receiver stopped")
)

type responseWaiter struct {
	messageType   MessageType
	serviceID     [16]byte
	commandID     uint32
	expectCommand bool
	ch            chan responseResult
}

type responseResult struct {
	data []byte
	err  error
}

type indicationKey struct {
	serviceID [16]byte
	commandID uint32
}

func (r *Reader) startReceiver() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureReceiverLocked(false)
}

func (r *Reader) ensureReceiverLocked(allowClosing bool) error {
	switch {
	case r.closed:
		return errReaderClosed
	case r.closing && !allowClosing:
		return errReaderClosed
	case r.receiverErr != nil:
		return r.receiverErr
	case r.receiverStarted:
		return nil
	case r.conn == nil:
		return errors.New("MBIM reader connection is nil")
	}

	if r.pending == nil {
		r.pending = make(map[uint32]*responseWaiter)
	}
	if r.subs == nil {
		r.subs = make(map[indicationKey]map[chan Indication]struct{})
	}
	if r.waiters == nil {
		r.waiters = make(map[indicationKey][]chan Indication)
	}
	if r.indications == nil {
		r.indications = make(map[indicationKey][]Indication)
	}
	r.receiverStarted = true
	go r.receive()
	return nil
}

func (r *Reader) beginClose() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.closing {
		return false
	}
	r.closing = true
	return true
}

func (r *Reader) finishClose() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
}

func (r *Reader) transmit(ctx context.Context, request *Request) error {
	return r.transmitRequest(ctx, request, false)
}

func (r *Reader) transmitClosing(ctx context.Context, request *Request) error {
	return r.transmitRequest(ctx, request, true)
}

func (r *Reader) transmitRequest(ctx context.Context, request *Request, allowClosing bool) error {
	ctx, cancel := requestContext(ctx, request.timeout())
	defer cancel()

	results, unregister, err := r.registerResponse(request, allowClosing)
	if err != nil {
		return err
	}
	defer unregister()

	r.writeMu.Lock()
	_, err = request.writeConn(ctx, r.conn)
	r.writeMu.Unlock()
	if err != nil {
		return err
	}

	select {
	case result := <-results:
		if result.err != nil {
			return result.err
		}
		return request.unmarshalResponse(result.data)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline, _ := requestDeadline(ctx, timeout)
	return context.WithDeadline(ctx, deadline)
}

func (r *Reader) registerResponse(request *Request, allowClosing bool) (<-chan responseResult, func(), error) {
	messageType, ok := responseMessageType(request.MessageType)
	if !ok {
		return nil, nil, fmt.Errorf("registering MBIM response: unsupported request message type %#x", request.MessageType)
	}
	serviceID, commandID, expectCommand := request.expectedCommand()

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureReceiverLocked(allowClosing); err != nil {
		return nil, nil, err
	}
	if _, ok := r.pending[request.TransactionID]; ok {
		return nil, nil, fmt.Errorf("registering MBIM response: transaction ID %d is already pending", request.TransactionID)
	}

	ch := make(chan responseResult, 1)
	r.pending[request.TransactionID] = &responseWaiter{
		messageType:   messageType,
		serviceID:     serviceID,
		commandID:     commandID,
		expectCommand: expectCommand,
		ch:            ch,
	}
	return ch, func() { r.unregisterResponse(request.TransactionID, ch) }, nil
}

func (r *Reader) unregisterResponse(transactionID uint32, ch <-chan responseResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiter, ok := r.pending[transactionID]
	if ok && waiter.ch == ch {
		delete(r.pending, transactionID)
	}
}

func (r *Reader) receive() {
	var collector *fragmentCollector
	for {
		buf, err := readFrame(r.conn)
		if err != nil {
			if timeoutError(err) {
				continue
			}
			r.stopReceiver(fmt.Errorf("receiving MBIM message: %w", err))
			return
		}

		messageType := MessageType(binary.LittleEndian.Uint32(buf[:4]))
		if isFragmentMessage(messageType) {
			complete, err := collectFrame(&collector, buf)
			if err != nil {
				r.stopReceiver(err)
				return
			}
			if complete == nil {
				continue
			}
			buf = complete
			messageType = MessageType(binary.LittleEndian.Uint32(buf[:4]))
		}

		switch messageType {
		case MessageTypeOpenDone, MessageTypeCloseDone, MessageTypeCommandDone, MessageTypeFunctionError:
			r.deliverResponse(messageType, buf)
		case MessageTypeIndicateStatus:
			var indication Indication
			if err := indication.UnmarshalBinary(buf); err != nil {
				r.stopReceiver(err)
				return
			}
			r.publishIndication(indication)
		}
	}
}

func collectFrame(collector **fragmentCollector, buf []byte) ([]byte, error) {
	if *collector != nil {
		if err := (*collector).add(buf); err != nil {
			return nil, err
		}
		if !(*collector).complete() {
			return nil, nil
		}
		complete, err := (*collector).MarshalBinary()
		*collector = nil
		return complete, err
	}

	if len(buf) < 20 || binary.LittleEndian.Uint32(buf[12:16]) <= 1 {
		return buf, nil
	}
	next, err := newFragmentCollector(buf)
	if err != nil {
		return nil, err
	}
	if next.complete() {
		return next.MarshalBinary()
	}
	*collector = next
	return nil, nil
}

func (r *Reader) deliverResponse(messageType MessageType, data []byte) {
	transactionID := binary.LittleEndian.Uint32(data[8:12])

	r.mu.Lock()
	waiter := r.pending[transactionID]
	if waiter == nil || !waiter.matches(messageType, data) {
		r.mu.Unlock()
		return
	}
	delete(r.pending, transactionID)
	r.mu.Unlock()

	waiter.ch <- responseResult{data: data}
}

func (w *responseWaiter) matches(messageType MessageType, data []byte) bool {
	if messageType != w.messageType && messageType != MessageTypeFunctionError {
		return false
	}
	if messageType != MessageTypeCommandDone || !w.expectCommand {
		return true
	}

	var header commandDoneHeader
	if err := header.UnmarshalBinary(data); err != nil {
		return true
	}
	return header.ServiceID == w.serviceID && header.CommandID == w.commandID
}

func (r *Reader) stopReceiver(err error) {
	r.mu.Lock()
	if r.receiverErr == nil {
		r.receiverErr = err
	}
	pending := r.pending
	r.pending = make(map[uint32]*responseWaiter)
	subs := r.subs
	r.subs = make(map[indicationKey]map[chan Indication]struct{})
	waiters := r.waiters
	r.waiters = make(map[indicationKey][]chan Indication)
	r.receiverStarted = false
	r.mu.Unlock()

	for _, waiter := range pending {
		waiter.ch <- responseResult{err: err}
	}
	for _, set := range subs {
		for ch := range set {
			close(ch)
		}
	}
	for _, set := range waiters {
		for _, ch := range set {
			close(ch)
		}
	}
}

func (r *Reader) nextIndication(ctx context.Context, key indicationKey) (Indication, error) {
	r.mu.Lock()
	if r.closed || r.closing {
		r.mu.Unlock()
		return Indication{}, errReaderClosed
	}

	queue := r.indications[key]
	if len(queue) > 0 {
		indication := cloneIndication(queue[0])
		if len(queue) == 1 {
			delete(r.indications, key)
		} else {
			r.indications[key] = queue[1:]
		}
		r.mu.Unlock()
		return indication, nil
	}
	if r.receiverErr != nil {
		err := r.receiverErr
		r.mu.Unlock()
		return Indication{}, err
	}
	if err := r.ensureReceiverLocked(false); err != nil {
		r.mu.Unlock()
		return Indication{}, err
	}

	ch := make(chan Indication, 1)
	r.waiters[key] = append(r.waiters[key], ch)
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		if indication, ok := r.cancelIndicationWaiter(key, ch); ok {
			return indication, nil
		}
		return Indication{}, ctx.Err()
	case indication, ok := <-ch:
		if !ok {
			return Indication{}, errReceiverStopped
		}
		return indication, nil
	}
}

func (r *Reader) subscribeIndication(key indicationKey) (<-chan Indication, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.closing {
		return nil, nil, errReaderClosed
	}

	queued := r.indications[key]
	if r.receiverErr != nil {
		if len(queued) == 0 {
			return nil, nil, r.receiverErr
		}
		ch := make(chan Indication, len(queued))
		for _, indication := range queued {
			ch <- cloneIndication(indication)
		}
		close(ch)
		delete(r.indications, key)
		return ch, func() {}, nil
	}

	if err := r.ensureReceiverLocked(false); err != nil {
		return nil, nil, err
	}

	ch := make(chan Indication, maxQueuedIndications)
	if r.subs[key] == nil {
		r.subs[key] = make(map[chan Indication]struct{})
	}
	r.subs[key][ch] = struct{}{}
	for _, indication := range queued {
		ch <- cloneIndication(indication)
	}
	delete(r.indications, key)
	return ch, func() { r.unsubscribeIndication(key, ch) }, nil
}

func (r *Reader) unsubscribeIndication(key indicationKey, ch chan Indication) {
	r.mu.Lock()
	defer r.mu.Unlock()
	subs := r.subs[key]
	if subs == nil {
		return
	}
	delete(subs, ch)
	if len(subs) == 0 {
		delete(r.subs, key)
	}
}

func (r *Reader) cancelIndicationWaiter(key indicationKey, ch chan Indication) (Indication, bool) {
	r.mu.Lock()
	waiters := r.waiters[key]
	for i, waiter := range waiters {
		if waiter != ch {
			continue
		}
		waiters = append(waiters[:i], waiters[i+1:]...)
		if len(waiters) == 0 {
			delete(r.waiters, key)
		} else {
			r.waiters[key] = waiters
		}
		r.mu.Unlock()
		return Indication{}, false
	}
	r.mu.Unlock()

	select {
	case indication, ok := <-ch:
		if ok {
			return indication, true
		}
	default:
	}
	return Indication{}, false
}

func (r *Reader) publishIndication(indication Indication) {
	key := indicationKey{serviceID: indication.ServiceID, commandID: indication.CommandID}

	r.mu.Lock()
	subs := r.subs[key]
	waiters := r.waiters[key]
	var waiter chan Indication
	if len(waiters) > 0 {
		waiter = waiters[0]
		if len(waiters) == 1 {
			delete(r.waiters, key)
		} else {
			r.waiters[key] = waiters[1:]
		}
	}
	if len(subs) == 0 && waiter == nil {
		r.queueIndicationLocked(key, indication)
		r.mu.Unlock()
		return
	}
	for ch := range subs {
		deliverIndication(ch, cloneIndication(indication))
	}
	if waiter != nil {
		waiter <- cloneIndication(indication)
	}
	r.mu.Unlock()
}

func (r *Reader) queueIndicationLocked(key indicationKey, indication Indication) {
	queue := append(r.indications[key], cloneIndication(indication))
	if len(queue) > maxQueuedIndications {
		queue = queue[len(queue)-maxQueuedIndications:]
	}
	r.indications[key] = queue
}

func deliverIndication(ch chan Indication, indication Indication) {
	select {
	case ch <- indication:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- indication:
	default:
	}
}

func cloneIndication(indication Indication) Indication {
	indication.InformationBuffer = append([]byte(nil), indication.InformationBuffer...)
	return indication
}
