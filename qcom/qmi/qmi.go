package qmi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

const defaultProxyOpenTimeout = 5 * time.Second

type Conn interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type Dialer interface {
	Dial(ctx context.Context) (Conn, error)
}

type Option func(*config)

type config struct {
	dialer       Dialer
	autoDetect   bool
	sharedDirect bool
	device       string
}

// OpenError reports whether an open attempt had selected qmi-proxy before it
// failed. Err remains available through errors.Is and errors.As.
type OpenError struct {
	Proxy bool
	Err   error
}

func (e *OpenError) Error() string { return e.Err.Error() }

func (e *OpenError) Unwrap() error { return e.Err }

type ProxyDialer struct {
	Address string
	Device  string
}

type proxyDialer interface {
	usesProxy() bool
}

type deviceDialer interface {
	device() string
}

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

// WithAutoDetect uses qmi-proxy when its socket is reachable and otherwise
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

func Open(ctx context.Context, opts ...Option) (*Transport, error) {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.autoDetect {
		return openAuto(ctx, cfg.device)
	}
	if direct, ok := cfg.dialer.(DirectDialer); cfg.sharedDirect && ok {
		transport, err := directTransports.acquire(ctx, direct.Device, direct)
		if err != nil {
			return nil, &OpenError{Err: err}
		}
		return transport, nil
	}
	conn, proxy, device, err := dialConfigured(ctx, cfg)
	if err != nil {
		return nil, &OpenError{Proxy: proxy, Err: err}
	}
	transport := New(conn)
	transport.proxy = proxy
	if !proxy {
		return transport, nil
	}

	if err := transport.openProxy(ctx, device); err != nil {
		_ = conn.Close()
		return nil, &OpenError{Proxy: true, Err: fmt.Errorf("opening QMI proxy for %s: %w", device, err)}
	}
	return transport, nil
}

func openAuto(ctx context.Context, device string) (*Transport, error) {
	device = strings.TrimSpace(device)
	if device == "" {
		return nil, &OpenError{Err: errors.New("opening QMI transport: auto-detect device is empty")}
	}

	conn, err := (ProxyDialer{Device: device}).Dial(ctx)
	if err == nil {
		transport := New(conn)
		transport.proxy = true
		if err := transport.openProxy(ctx, device); err != nil {
			_ = transport.Close()
			return nil, &OpenError{Proxy: true, Err: fmt.Errorf("opening QMI proxy for %s: %w", device, err)}
		}
		return transport, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &OpenError{Proxy: true, Err: ctxErr}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, &OpenError{Proxy: true, Err: err}
	}

	transport, err := directTransports.acquire(ctx, device, DirectDialer{Device: device})
	if err != nil {
		return nil, &OpenError{Err: err}
	}
	return transport, nil
}

func dialConfigured(ctx context.Context, cfg config) (Conn, bool, string, error) {
	if cfg.autoDetect {
		device := strings.TrimSpace(cfg.device)
		if device == "" {
			return nil, false, "", errors.New("opening QMI transport: auto-detect device is empty")
		}
		conn, proxy, err := dialAuto(
			ctx,
			ProxyDialer{Device: device},
			DirectDialer{Device: device},
		)
		return conn, proxy, device, err
	}
	if cfg.dialer == nil {
		return nil, false, "", errors.New("opening QMI transport: dialer is nil")
	}

	proxy := dialerUsesProxy(cfg.dialer)
	device := ""
	if proxy {
		device = proxyDevice(cfg)
		if device == "" {
			return nil, true, "", errors.New("opening QMI proxy: device is empty")
		}
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

func dialerUsesProxy(d Dialer) bool {
	p, ok := d.(proxyDialer)
	return ok && p.usesProxy()
}

func (d ProxyDialer) Dial(ctx context.Context) (Conn, error) {
	address := d.Address
	if address == "" {
		address = "\x00qmi-proxy"
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (d ProxyDialer) usesProxy() bool { return true }

func (d ProxyDialer) device() string { return strings.TrimSpace(d.Device) }

func proxyDevice(cfg config) string {
	d, ok := cfg.dialer.(deviceDialer)
	if ok {
		return d.device()
	}
	return ""
}

func (t *Transport) openProxy(ctx context.Context, device string) error {
	req := qcom.InternalOpenRequest{
		TransactionID: 1,
		DevicePath:    []byte(device),
	}.Request()
	req.Timeout = defaultProxyOpenTimeout

	resp, err := t.Do(ctx, req)
	if err != nil {
		return err
	}
	return qcom.ResultError(resp.TLVs)
}

var _ qcom.Transport = (*Transport)(nil)
