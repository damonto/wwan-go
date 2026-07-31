package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCloseTimeout = 5 * time.Second

type Client struct {
	conn               Conn
	slot               uint32
	mbimExVersion      uint16
	proxy              bool
	maxControlTransfer int
	envelopeSupport    *STKEnvelopeInfo

	*clientState
	lease     *clientLease
	stateOnce sync.Once
	closeOnce sync.Once
	closeErr  error
	release   func() error
}

type clientLease struct {
	closed atomic.Bool
}

type clientState struct {
	txn             atomic.Uint32
	mu              sync.Mutex
	writeMu         sync.Mutex
	closed          bool
	closing         bool
	receiverStarted bool
	receiverErr     error
	pending         map[uint32]*responseWaiter
	subs            map[indicationKey]map[chan WatchResult[Indication]]*clientLease
	waiters         map[indicationKey][]*indicationWaiter
	indications     map[indicationKey][]Indication
}

type Option func(*config)

type config struct {
	dialer       Dialer
	slot         int
	autoDetect   bool
	sharedDirect bool
	device       string
}

// OpenError reports whether an open attempt had selected mbim-proxy before it
// failed. Err remains available through errors.Is and errors.As.
type OpenError struct {
	Proxy bool
	Err   error
}

func (e *OpenError) Error() string { return e.Err.Error() }

func (e *OpenError) Unwrap() error { return e.Err }

func WithDialer(d Dialer) Option {
	return func(c *config) {
		c.setDialer(d)
	}
}

func WithProxy(device string) Option {
	return func(c *config) {
		c.setDialer(ProxyDialer{Device: device})
	}
}

func WithDirect(device string) Option {
	return func(c *config) {
		c.setDialer(DirectDialer{Device: device})
		c.sharedDirect = true
	}
}

// WithAutoDetect uses mbim-proxy when its socket is reachable and otherwise
// opens device directly. A proxy connection that rejects device is not retried
// using direct access.
func WithAutoDetect(device string) Option {
	return func(c *config) {
		c.dialer = nil
		c.autoDetect = true
		c.sharedDirect = false
		c.device = device
	}
}

func (c *config) setDialer(d Dialer) {
	c.dialer = d
	c.autoDetect = false
	c.sharedDirect = false
	c.device = ""
}

func WithSlot(slot int) Option {
	return func(c *config) {
		c.slot = slot
	}
}

func Open(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := config{slot: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.slot < 1 {
		return nil, fmt.Errorf("opening MBIM client: slot %d is out of range", cfg.slot)
	}
	if cfg.autoDetect {
		return openAuto(ctx, cfg)
	}
	if direct, ok := cfg.dialer.(DirectDialer); cfg.sharedDirect && ok {
		return directClients.acquire(ctx, direct.Device, direct, cfg.slot)
	}
	conn, proxy, device, err := dialConfigured(ctx, cfg)
	if err != nil {
		return nil, &OpenError{Proxy: proxy, Err: err}
	}

	client := &Client{
		conn:               conn,
		slot:               uint32(cfg.slot - 1),
		proxy:              proxy,
		maxControlTransfer: connMaxControlTransfer(conn),
		clientState:        newClientState(),
	}
	if err := client.connect(ctx, device); err != nil {
		_ = conn.Close()
		return nil, &OpenError{Proxy: proxy, Err: err}
	}
	return client, nil
}

func newClientState() *clientState {
	return &clientState{
		pending:     make(map[uint32]*responseWaiter),
		subs:        make(map[indicationKey]map[chan WatchResult[Indication]]*clientLease),
		waiters:     make(map[indicationKey][]*indicationWaiter),
		indications: make(map[indicationKey][]Indication),
	}
}

func (c *Client) ensureState() *clientState {
	c.stateOnce.Do(func() {
		if c.clientState == nil {
			c.clientState = newClientState()
		}
	})
	return c.clientState
}

func openAuto(ctx context.Context, cfg config) (*Client, error) {
	device := strings.TrimSpace(cfg.device)
	if device == "" {
		return nil, &OpenError{Err: errors.New("opening MBIM client: auto-detect device is empty")}
	}

	conn, err := (ProxyDialer{Device: device}).Dial(ctx)
	if err == nil {
		client := &Client{
			conn:               conn,
			slot:               uint32(cfg.slot - 1),
			proxy:              true,
			maxControlTransfer: connMaxControlTransfer(conn),
			clientState:        newClientState(),
		}
		if err := client.connect(ctx, device); err != nil {
			_ = conn.Close()
			return nil, &OpenError{Proxy: true, Err: err}
		}
		return client, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &OpenError{Proxy: true, Err: ctxErr}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, &OpenError{Proxy: true, Err: err}
	}
	return directClients.acquire(ctx, device, DirectDialer{Device: device}, cfg.slot)
}

func dialConfigured(ctx context.Context, cfg config) (Conn, bool, string, error) {
	if cfg.autoDetect {
		device := strings.TrimSpace(cfg.device)
		if device == "" {
			return nil, false, "", errors.New("opening MBIM client: auto-detect device is empty")
		}
		conn, proxy, err := dialAuto(
			ctx,
			ProxyDialer{Device: device},
			DirectDialer{Device: device},
		)
		return conn, proxy, device, err
	}
	if cfg.dialer == nil {
		return nil, false, "", errors.New("opening MBIM client: dialer is nil")
	}

	device := dialerDevice(cfg.dialer)
	proxy := dialerUsesProxy(cfg.dialer)
	if proxy && device == "" {
		return nil, true, "", errors.New("opening MBIM proxy: device is empty")
	}
	conn, err := cfg.dialer.Dial(ctx)
	return conn, proxy, device, err
}

func dialAuto(ctx context.Context, proxy, direct Dialer) (Conn, bool, error) {
	conn, err := proxy.Dial(ctx)
	if err == nil {
		return conn, true, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, true, ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, true, err
	}
	conn, err = direct.Dial(ctx)
	return conn, false, err
}

// UsesProxy reports whether the client was opened through mbim-proxy.
func (c *Client) UsesProxy() bool { return c.proxy }

func dialerDevice(d Dialer) string {
	device, ok := d.(deviceDialer)
	if ok {
		return device.device()
	}
	return ""
}

func (c *Client) connect(ctx context.Context, device string) error {
	if c.proxy {
		if err := c.configureProxy(ctx, device); err != nil {
			return err
		}
	}
	if err := c.openDevice(ctx); err != nil {
		return err
	}
	if err := c.startReceiver(); err != nil {
		return err
	}
	if err := c.negotiateVersion(ctx); err != nil {
		return err
	}
	if err := c.validateUiccSlotID(); err != nil {
		return fmt.Errorf("connecting MBIM client: %w", err)
	}
	if !c.usesUiccSlotID() {
		if err := c.ensureSlotActivated(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) openDevice(ctx context.Context) error {
	request := OpenDeviceRequest{
		TransactionID:      c.nextTransactionID(),
		MaxControlTransfer: uint32(c.maxControlTransfer),
	}
	if err := request.Request().Transmit(ctx, c.conn); err != nil {
		return fmt.Errorf("opening MBIM device: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if c.lease != nil {
			c.closeLease()
		}
		if c.release != nil {
			c.closeErr = c.release()
			return
		}
		c.closeErr = c.closeDevice()
	})
	return c.closeErr
}

func (c *Client) closeDevice() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()

	if !c.beginClose() {
		return nil
	}

	request := CloseRequest{TransactionID: c.nextTransactionID()}
	err := c.transmitClosing(ctx, request.Request())
	closeErr := c.conn.Close()
	c.finishClose()
	return errors.Join(err, closeErr)
}

func (c *Client) nextTransactionID() uint32 {
	return c.ensureState().txn.Add(1)
}

type OpenDeviceRequest struct {
	TransactionID      uint32
	MaxControlTransfer uint32
	Response           *OpenDeviceResponse
}

func (r *OpenDeviceRequest) Request() *Request {
	r.Response = new(OpenDeviceResponse)
	return &Request{
		MessageType:   MessageTypeOpen,
		TransactionID: r.TransactionID,
		Command:       r,
		Response:      r.Response,
	}
}

func (r *OpenDeviceRequest) MarshalBinary() ([]byte, error) {
	maxControlTransfer := r.MaxControlTransfer
	if maxControlTransfer == 0 {
		maxControlTransfer = defaultMaxControlTransfer
	}
	if maxControlTransfer < 64 {
		return nil, fmt.Errorf("encoding MBIM open request: maximum control transfer %d is less than 64", maxControlTransfer)
	}
	return binary.LittleEndian.AppendUint32(nil, maxControlTransfer), nil
}

type OpenDeviceResponse struct{}

func (r *OpenDeviceResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM open response: payload length %d, want 0", len(data))
	}
	return nil
}

type CloseRequest struct {
	TransactionID uint32
	Response      *CloseResponse
}

func (r *CloseRequest) Request() *Request {
	r.Response = new(CloseResponse)
	return &Request{
		MessageType:   MessageTypeClose,
		TransactionID: r.TransactionID,
		Timeout:       2 * time.Second,
		Command:       r,
		Response:      r.Response,
	}
}

func (r *CloseRequest) MarshalBinary() ([]byte, error) { return nil, nil }

type CloseResponse struct{}

func (r *CloseResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing MBIM close response: payload length %d, want 0", len(data))
	}
	return nil
}
