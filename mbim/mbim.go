package mbim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCloseTimeout = 5 * time.Second

type Reader struct {
	conn               Conn
	slot               uint32
	mbimExVersion      uint16
	txn                atomic.Uint32
	proxy              bool
	maxControlTransfer int

	mu              sync.Mutex
	writeMu         sync.Mutex
	closed          bool
	closing         bool
	receiverStarted bool
	receiverErr     error
	pending         map[uint32]*responseWaiter
	subs            map[indicationKey]map[chan Indication]struct{}
	waiters         map[indicationKey][]chan Indication
	indications     map[indicationKey][]Indication
	envelopeSupport *STKEnvelopeInfo
}

type Option func(*config)

type config struct {
	dialer Dialer
	slot   int
}

func WithDialer(d Dialer) Option {
	return func(c *config) {
		c.dialer = d
	}
}

func WithProxy(device string) Option {
	return func(c *config) {
		c.dialer = ProxyDialer{Device: device}
	}
}

func WithDirect(device string) Option {
	return func(c *config) {
		c.dialer = DirectDialer{Device: device}
	}
}

func WithSlot(slot int) Option {
	return func(c *config) {
		c.slot = slot
	}
}

func Open(ctx context.Context, opts ...Option) (*Reader, error) {
	cfg := config{slot: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.slot < 1 {
		return nil, fmt.Errorf("opening MBIM reader: slot %d is out of range", cfg.slot)
	}
	if cfg.dialer == nil {
		return nil, errors.New("opening MBIM reader: dialer is nil")
	}

	device := dialerDevice(cfg.dialer)
	if dialerUsesProxy(cfg.dialer) && device == "" {
		return nil, errors.New("opening MBIM proxy: device is empty")
	}

	conn, err := cfg.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	reader := &Reader{
		conn:               conn,
		slot:               uint32(cfg.slot - 1),
		proxy:              dialerUsesProxy(cfg.dialer),
		maxControlTransfer: connMaxControlTransfer(conn),
	}
	if err := reader.connect(ctx, device); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return reader, nil
}

func dialerDevice(d Dialer) string {
	device, ok := d.(deviceDialer)
	if ok {
		return device.device()
	}
	return ""
}

func (r *Reader) connect(ctx context.Context, device string) error {
	if r.proxy {
		if err := r.configureProxy(ctx, device); err != nil {
			return err
		}
	}
	if err := r.openDevice(ctx); err != nil {
		return err
	}
	if err := r.startReceiver(); err != nil {
		return err
	}
	if err := r.negotiateVersion(ctx); err != nil {
		return err
	}
	if !r.usesUiccSlotID() {
		if err := r.ensureSlotActivated(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reader) configureProxy(ctx context.Context, device string) error {
	request := ProxyConfigRequest{
		TransactionID: r.nextTransactionID(),
		DevicePath:    device,
		Timeout:       30,
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("opening MBIM reader: device %s is not connected", device)
		}
		return fmt.Errorf("configuring MBIM proxy for %s: %w", device, err)
	}
	return nil
}

func (r *Reader) openDevice(ctx context.Context) error {
	request := OpenDeviceRequest{
		TransactionID:      r.nextTransactionID(),
		MaxControlTransfer: uint32(r.maxControlTransfer),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return fmt.Errorf("opening MBIM device: %w", err)
	}
	return nil
}

func (r *Reader) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()

	if !r.beginClose() {
		return nil
	}

	request := CloseRequest{TransactionID: r.nextTransactionID()}
	err := r.transmitClosing(ctx, request.Request())
	closeErr := r.conn.Close()
	r.finishClose()
	return errors.Join(err, closeErr)
}

func (r *Reader) nextTransactionID() uint32 {
	return r.txn.Add(1)
}
