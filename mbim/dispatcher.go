package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	maxQueuedIndications = 32
	fragmentTimeout      = time.Second
)

var (
	errClientClosed      = errors.New("MBIM client is closed")
	errClientLeaseClosed = errors.New("MBIM client lease is closed")
	errReceiverStopped   = errors.New("MBIM receiver stopped")
)

type responseWaiter struct {
	messageType   MessageType
	serviceID     [16]byte
	commandID     uint32
	expectCommand bool
	ch            chan responseResult
	owner         *clientLease
}

type indicationWaiter struct {
	ch    chan WatchResult[Indication]
	owner *clientLease
}

type responseResult struct {
	data []byte
	err  error
}

type frameResult struct {
	data []byte
	err  error
}

// WatchResult is a value or the terminal error from a notification stream.
// A result with a non-nil Err is always the last result sent by the stream.
type WatchResult[T any] struct {
	Value T
	Err   error
}

type fragmentKey struct {
	messageType   MessageType
	transactionID uint32
}

type incomingFragmentCollection struct {
	collector *fragmentCollector
	deadline  time.Time
}

type indicationKey struct {
	serviceID [16]byte
	commandID uint32
}

func (c *Client) startReceiver() error {
	c.ensureState()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureReceiverLocked(false)
}

func (c *Client) ensureReceiverLocked(allowClosing bool) error {
	switch {
	case c.lease != nil && c.lease.closed.Load():
		return errClientLeaseClosed
	case c.closed:
		return errClientClosed
	case c.closing && !allowClosing:
		return errClientClosed
	case c.receiverErr != nil:
		return c.receiverErr
	case c.receiverStarted:
		return nil
	case c.conn == nil:
		return errors.New("MBIM client connection is nil")
	}

	if c.pending == nil {
		c.pending = make(map[uint32]*responseWaiter)
	}
	if c.subs == nil {
		c.subs = make(map[indicationKey]map[chan WatchResult[Indication]]*clientLease)
	}
	if c.waiters == nil {
		c.waiters = make(map[indicationKey][]*indicationWaiter)
	}
	if c.indications == nil {
		c.indications = make(map[indicationKey][]Indication)
	}
	c.receiverStarted = true
	go c.receive()
	return nil
}

func (c *Client) beginClose() bool {
	c.ensureState()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.closing {
		return false
	}
	c.closing = true
	return true
}

func (c *Client) finishClose() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

func (c *Client) transmit(ctx context.Context, request *Request) error {
	return c.transmitRequest(ctx, request, false)
}

func (c *Client) transmitClosing(ctx context.Context, request *Request) error {
	return c.transmitRequest(ctx, request, true)
}

func (c *Client) sendOneWay(ctx context.Context, request *Request) error {
	c.ensureState()
	ctx, cancel := requestContext(ctx, request.timeout())
	defer cancel()

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	switch {
	case c.lease != nil && c.lease.closed.Load():
		c.mu.Unlock()
		return errClientLeaseClosed
	case c.closed || c.closing:
		c.mu.Unlock()
		return errClientClosed
	case c.conn == nil:
		c.mu.Unlock()
		return errors.New("MBIM client connection is nil")
	}
	conn := c.conn
	c.mu.Unlock()

	_, err := request.writeConn(ctx, conn)
	return err
}

func (c *Client) transmitRequest(ctx context.Context, request *Request, allowClosing bool) error {
	ctx, cancel := requestContext(ctx, request.timeout())
	defer cancel()

	results, unregister, err := c.registerResponse(request, allowClosing)
	if err != nil {
		return err
	}
	defer unregister()

	c.writeMu.Lock()
	_, err = request.writeConn(ctx, c.conn)
	c.writeMu.Unlock()
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

func (c *Client) registerResponse(request *Request, allowClosing bool) (<-chan responseResult, func(), error) {
	c.ensureState()
	messageType, ok := responseMessageType(request.MessageType)
	if !ok {
		return nil, nil, fmt.Errorf("registering MBIM response: unsupported request message type %#x", request.MessageType)
	}
	serviceID, commandID, expectCommand := request.expectedCommand()

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureReceiverLocked(allowClosing); err != nil {
		return nil, nil, err
	}
	if _, ok := c.pending[request.TransactionID]; ok {
		return nil, nil, fmt.Errorf("registering MBIM response: transaction ID %d is already pending", request.TransactionID)
	}

	ch := make(chan responseResult, 1)
	c.pending[request.TransactionID] = &responseWaiter{
		messageType:   messageType,
		serviceID:     serviceID,
		commandID:     commandID,
		expectCommand: expectCommand,
		ch:            ch,
		owner:         c.lease,
	}
	return ch, func() { c.unregisterResponse(request.TransactionID, ch) }, nil
}

func (c *Client) unregisterResponse(transactionID uint32, ch <-chan responseResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiter, ok := c.pending[transactionID]
	if ok && waiter.ch == ch {
		delete(c.pending, transactionID)
	}
}

func (c *Client) receive() {
	frames := make(chan frameResult, 1)
	done := make(chan struct{})
	defer close(done)
	go readFrames(c.conn, frames, done)

	collectors := make(map[fragmentKey]*incomingFragmentCollection)
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		timeout := resetFragmentTimer(timer, collectors)
		select {
		case result := <-frames:
			if result.err != nil {
				c.stopReceiver(fmt.Errorf("receiving MBIM message: %w", result.err))
				return
			}

			buf := result.data
			messageType := MessageType(binary.LittleEndian.Uint32(buf[:4]))
			if isFragmentMessage(messageType) {
				complete, fault := collectReceivedFrame(collectors, buf, time.Now())
				if fault != nil {
					if !c.reportFragmentError(fault.transactionID, fault.status) {
						return
					}
					continue
				}
				if complete == nil {
					continue
				}
				buf = complete
				messageType = MessageType(binary.LittleEndian.Uint32(buf[:4]))
			}

			switch messageType {
			case MessageTypeOpenDone, MessageTypeCloseDone, MessageTypeCommandDone, MessageTypeFunctionError:
				c.deliverResponse(messageType, buf)
			case MessageTypeIndicateStatus:
				var indication Indication
				if err := indication.UnmarshalBinary(buf); err != nil {
					c.stopReceiver(err)
					return
				}
				c.publishIndication(indication)
			}
		case now := <-timeout:
			for _, transactionID := range expireFragmentCollectors(collectors, now) {
				if !c.reportFragmentError(transactionID, ProtocolErrorTimeoutFragment) {
					return
				}
			}
		}
	}
}

func readFrames(conn Conn, frames chan<- frameResult, done <-chan struct{}) {
	for {
		data, err := readFrame(conn)
		if err != nil && timeoutError(err) {
			continue
		}
		select {
		case frames <- frameResult{data: data, err: err}:
		case <-done:
			return
		}
		if err != nil {
			return
		}
	}
}

type fragmentFault struct {
	transactionID uint32
	status        ProtocolError
}

func collectReceivedFrame(
	collectors map[fragmentKey]*incomingFragmentCollection,
	buf []byte,
	now time.Time,
) ([]byte, *fragmentFault) {
	messageType := MessageType(binary.LittleEndian.Uint32(buf[:4]))
	transactionID := binary.LittleEndian.Uint32(buf[8:12])
	key := fragmentKey{messageType: messageType, transactionID: transactionID}

	var f fragment
	if err := f.UnmarshalBinary(buf); err != nil {
		delete(collectors, key)
		return nil, &fragmentFault{transactionID: transactionID, status: ProtocolErrorFragmentOutOfSequence}
	}
	if f.total == 1 {
		return buf, nil
	}

	collection := collectors[key]
	if collection == nil {
		if f.current != 0 {
			return nil, &fragmentFault{transactionID: transactionID, status: ProtocolErrorFragmentOutOfSequence}
		}
		collector, err := newFragmentCollector(buf)
		if err != nil {
			return nil, &fragmentFault{transactionID: transactionID, status: ProtocolErrorFragmentOutOfSequence}
		}
		collectors[key] = &incomingFragmentCollection{
			collector: collector,
			deadline:  now.Add(fragmentTimeout),
		}
		return nil, nil
	}

	if err := collection.collector.add(buf); err != nil {
		delete(collectors, key)
		return nil, &fragmentFault{transactionID: transactionID, status: ProtocolErrorFragmentOutOfSequence}
	}
	collection.deadline = now.Add(fragmentTimeout)
	if !collection.collector.complete() {
		return nil, nil
	}

	complete, err := collection.collector.MarshalBinary()
	delete(collectors, key)
	if err != nil {
		return nil, &fragmentFault{transactionID: transactionID, status: ProtocolErrorFragmentOutOfSequence}
	}
	return complete, nil
}

func resetFragmentTimer(timer *time.Timer, collectors map[fragmentKey]*incomingFragmentCollection) <-chan time.Time {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if len(collectors) == 0 {
		return nil
	}

	deadline, _ := nextFragmentDeadline(collectors)
	delay := time.Until(deadline)
	if delay < 0 {
		delay = 0
	}
	timer.Reset(delay)
	return timer.C
}

func nextFragmentDeadline(collectors map[fragmentKey]*incomingFragmentCollection) (time.Time, bool) {
	var deadline time.Time
	for _, collection := range collectors {
		if deadline.IsZero() || collection.deadline.Before(deadline) {
			deadline = collection.deadline
		}
	}
	return deadline, !deadline.IsZero()
}

func expireFragmentCollectors(
	collectors map[fragmentKey]*incomingFragmentCollection,
	now time.Time,
) []uint32 {
	var transactionIDs []uint32
	seen := make(map[uint32]struct{})
	for key, collection := range collectors {
		if collection.deadline.After(now) {
			continue
		}
		delete(collectors, key)
		if _, ok := seen[key.transactionID]; ok {
			continue
		}
		seen[key.transactionID] = struct{}{}
		transactionIDs = append(transactionIDs, key.transactionID)
	}
	return transactionIDs
}

func (c *Client) reportFragmentError(transactionID uint32, status ProtocolError) bool {
	c.failResponse(transactionID, status)

	ctx, cancel := context.WithTimeout(context.Background(), fragmentTimeout)
	defer cancel()
	if err := c.SendHostError(ctx, transactionID, status); err != nil {
		c.stopReceiver(err)
		return false
	}
	return true
}

func (c *Client) failResponse(transactionID uint32, err error) {
	c.mu.Lock()
	waiter := c.pending[transactionID]
	if waiter != nil {
		delete(c.pending, transactionID)
	}
	c.mu.Unlock()
	if waiter != nil {
		waiter.ch <- responseResult{err: err}
	}
}

func (c *Client) deliverResponse(messageType MessageType, data []byte) {
	transactionID := binary.LittleEndian.Uint32(data[8:12])

	c.mu.Lock()
	waiter := c.pending[transactionID]
	if waiter == nil || !waiter.matches(messageType, data) {
		c.mu.Unlock()
		return
	}
	delete(c.pending, transactionID)
	c.mu.Unlock()

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

func (c *Client) stopReceiver(err error) {
	c.mu.Lock()
	if c.receiverErr == nil {
		c.receiverErr = err
	}
	pending := c.pending
	c.pending = make(map[uint32]*responseWaiter)
	subs := c.subs
	c.subs = make(map[indicationKey]map[chan WatchResult[Indication]]*clientLease)
	waiters := c.waiters
	c.waiters = make(map[indicationKey][]*indicationWaiter)
	c.receiverStarted = false
	c.mu.Unlock()

	for _, waiter := range pending {
		waiter.ch <- responseResult{err: err}
	}
	for _, set := range subs {
		for ch := range set {
			deliverWatchResult(ch, WatchResult[Indication]{Err: err})
			close(ch)
		}
	}
	for _, set := range waiters {
		for _, waiter := range set {
			select {
			case waiter.ch <- WatchResult[Indication]{Err: err}:
			default:
			}
			close(waiter.ch)
		}
	}
}

func (c *Client) closeLease() {
	if c.lease == nil || !c.lease.closed.CompareAndSwap(false, true) {
		return
	}

	c.ensureState()
	c.mu.Lock()
	var pending []*responseWaiter
	for transactionID, waiter := range c.pending {
		if waiter.owner != c.lease {
			continue
		}
		pending = append(pending, waiter)
		delete(c.pending, transactionID)
	}
	var subscriptions []chan WatchResult[Indication]
	for key, set := range c.subs {
		for ch, owner := range set {
			if owner != c.lease {
				continue
			}
			subscriptions = append(subscriptions, ch)
			delete(set, ch)
		}
		if len(set) == 0 {
			delete(c.subs, key)
		}
	}
	var waiters []*indicationWaiter
	for key, set := range c.waiters {
		remaining := set[:0]
		for _, waiter := range set {
			if waiter.owner == c.lease {
				waiters = append(waiters, waiter)
				continue
			}
			remaining = append(remaining, waiter)
		}
		if len(remaining) == 0 {
			delete(c.waiters, key)
		} else {
			c.waiters[key] = remaining
		}
	}
	c.mu.Unlock()

	for _, waiter := range pending {
		waiter.ch <- responseResult{err: errClientLeaseClosed}
	}
	for _, ch := range subscriptions {
		deliverWatchResult(ch, WatchResult[Indication]{Err: errClientLeaseClosed})
	}
	for _, waiter := range waiters {
		waiter.ch <- WatchResult[Indication]{Err: errClientLeaseClosed}
	}
}

// NextIndication waits for the next unsolicited indication matching serviceID
// and commandID. Indications received before the call remain queued and are
// returned in arrival order.
func (c *Client) NextIndication(ctx context.Context, serviceID [16]byte, commandID uint32) (Indication, error) {
	indication, err := c.nextIndication(ctx, indicationKey{serviceID: serviceID, commandID: commandID})
	if err != nil {
		return Indication{}, fmt.Errorf("reading MBIM indication for service %x CID %d: %w", serviceID, commandID, err)
	}
	return indication, nil
}

// WatchIndications streams unsolicited indications matching serviceID and
// commandID until ctx is done or the MBIM receiver stops.
func (c *Client) WatchIndications(ctx context.Context, serviceID [16]byte, commandID uint32) (<-chan Indication, error) {
	results, err := c.WatchIndicationResults(ctx, serviceID, commandID)
	if err != nil {
		return nil, err
	}
	return watchValues(ctx, results), nil
}

// WatchIndicationResults streams unsolicited indications and reports receiver
// failures through the terminal result.
func (c *Client) WatchIndicationResults(
	ctx context.Context,
	serviceID [16]byte,
	commandID uint32,
) (<-chan WatchResult[Indication], error) {
	indications, unsubscribe, err := c.subscribeIndication(indicationKey{serviceID: serviceID, commandID: commandID})
	if err != nil {
		return nil, fmt.Errorf("watching MBIM indications for service %x CID %d: %w", serviceID, commandID, err)
	}

	out := make(chan WatchResult[Indication], maxQueuedIndications)
	go func() {
		defer close(out)
		defer unsubscribe()

		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-indications:
				if !ok {
					return
				}
				if result.Err != nil {
					result.Err = fmt.Errorf(
						"watching MBIM indications for service %x CID %d: %w",
						serviceID,
						commandID,
						result.Err,
					)
				}
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
				if result.Err != nil {
					return
				}
			}
		}
	}()
	return out, nil
}

func (c *Client) nextIndication(ctx context.Context, key indicationKey) (Indication, error) {
	c.ensureState()
	c.mu.Lock()
	if c.lease != nil && c.lease.closed.Load() {
		c.mu.Unlock()
		return Indication{}, errClientLeaseClosed
	}
	if c.closed || c.closing {
		c.mu.Unlock()
		return Indication{}, errClientClosed
	}

	queue := c.indications[key]
	if len(queue) > 0 {
		indication := cloneIndication(queue[0])
		if len(queue) == 1 {
			delete(c.indications, key)
		} else {
			c.indications[key] = queue[1:]
		}
		c.mu.Unlock()
		return indication, nil
	}
	if c.receiverErr != nil {
		err := c.receiverErr
		c.mu.Unlock()
		return Indication{}, err
	}
	if err := c.ensureReceiverLocked(false); err != nil {
		c.mu.Unlock()
		return Indication{}, err
	}

	ch := make(chan WatchResult[Indication], 1)
	c.waiters[key] = append(c.waiters[key], &indicationWaiter{ch: ch, owner: c.lease})
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		if result, ok := c.cancelIndicationWaiter(key, ch); ok {
			if result.Err != nil {
				return Indication{}, result.Err
			}
			return result.Value, nil
		}
		return Indication{}, ctx.Err()
	case result, ok := <-ch:
		if !ok {
			return Indication{}, errReceiverStopped
		}
		if result.Err != nil {
			return Indication{}, result.Err
		}
		return result.Value, nil
	}
}

func (c *Client) subscribeIndication(key indicationKey) (<-chan WatchResult[Indication], func(), error) {
	c.ensureState()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease != nil && c.lease.closed.Load() {
		return nil, nil, errClientLeaseClosed
	}
	if c.closed || c.closing {
		return nil, nil, errClientClosed
	}

	queued := c.indications[key]
	if c.receiverErr != nil {
		if len(queued) == 0 {
			return nil, nil, c.receiverErr
		}
		ch := make(chan WatchResult[Indication], len(queued)+1)
		for _, indication := range queued {
			ch <- WatchResult[Indication]{Value: cloneIndication(indication)}
		}
		ch <- WatchResult[Indication]{Err: c.receiverErr}
		close(ch)
		delete(c.indications, key)
		return ch, func() {}, nil
	}

	if err := c.ensureReceiverLocked(false); err != nil {
		return nil, nil, err
	}

	ch := make(chan WatchResult[Indication], maxQueuedIndications)
	if c.subs[key] == nil {
		c.subs[key] = make(map[chan WatchResult[Indication]]*clientLease)
	}
	c.subs[key][ch] = c.lease
	for _, indication := range queued {
		ch <- WatchResult[Indication]{Value: cloneIndication(indication)}
	}
	delete(c.indications, key)
	return ch, func() { c.unsubscribeIndication(key, ch) }, nil
}

func (c *Client) unsubscribeIndication(key indicationKey, ch chan WatchResult[Indication]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	subs := c.subs[key]
	if subs == nil {
		return
	}
	delete(subs, ch)
	if len(subs) == 0 {
		delete(c.subs, key)
	}
}

func (c *Client) cancelIndicationWaiter(
	key indicationKey,
	ch chan WatchResult[Indication],
) (WatchResult[Indication], bool) {
	c.mu.Lock()
	waiters := c.waiters[key]
	for i, waiter := range waiters {
		if waiter.ch != ch {
			continue
		}
		waiters = append(waiters[:i], waiters[i+1:]...)
		if len(waiters) == 0 {
			delete(c.waiters, key)
		} else {
			c.waiters[key] = waiters
		}
		c.mu.Unlock()
		return WatchResult[Indication]{}, false
	}
	c.mu.Unlock()

	select {
	case result, ok := <-ch:
		if ok {
			return result, true
		}
	default:
	}
	return WatchResult[Indication]{}, false
}

func (c *Client) publishIndication(indication Indication) {
	key := indicationKey{serviceID: indication.ServiceID, commandID: indication.CommandID}

	c.mu.Lock()
	subs := c.subs[key]
	waiters := c.waiters[key]
	var waiter *indicationWaiter
	if len(waiters) > 0 {
		waiter = waiters[0]
		if len(waiters) == 1 {
			delete(c.waiters, key)
		} else {
			c.waiters[key] = waiters[1:]
		}
	}
	if len(subs) == 0 && waiter == nil {
		c.queueIndicationLocked(key, indication)
		c.mu.Unlock()
		return
	}
	for ch := range subs {
		deliverWatchResult(ch, WatchResult[Indication]{Value: cloneIndication(indication)})
	}
	if waiter != nil {
		waiter.ch <- WatchResult[Indication]{Value: cloneIndication(indication)}
	}
	c.mu.Unlock()
}

func (c *Client) queueIndicationLocked(key indicationKey, indication Indication) {
	queue := append(c.indications[key], cloneIndication(indication))
	if len(queue) > maxQueuedIndications {
		queue = queue[len(queue)-maxQueuedIndications:]
	}
	c.indications[key] = queue
}

func deliverWatchResult[T any](ch chan WatchResult[T], result WatchResult[T]) {
	select {
	case ch <- result:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- result:
	default:
	}
}

func watchValues[T any](ctx context.Context, results <-chan WatchResult[T]) <-chan T {
	out := make(chan T, maxQueuedIndications)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-results:
				if !ok || result.Err != nil {
					return
				}
				select {
				case out <- result.Value:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func watchDecoded[T any](
	ctx context.Context,
	indications <-chan WatchResult[Indication],
	operation string,
	decode func([]byte) (T, error),
) <-chan WatchResult[T] {
	out := make(chan WatchResult[T], maxQueuedIndications)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case indication, ok := <-indications:
				if !ok {
					return
				}
				var result WatchResult[T]
				if indication.Err != nil {
					result.Err = fmt.Errorf("%s: %w", operation, indication.Err)
				} else {
					result.Value, result.Err = decode(indication.Value.InformationBuffer)
					if result.Err != nil {
						result.Err = fmt.Errorf("%s: %w", operation, result.Err)
					}
				}
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
				if result.Err != nil {
					return
				}
			}
		}
	}()
	return out
}

func cloneIndication(indication Indication) Indication {
	indication.InformationBuffer = append([]byte(nil), indication.InformationBuffer...)
	return indication
}
