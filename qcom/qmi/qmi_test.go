package qmi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

type fakeDialer struct {
	conn Conn
	err  error
}

func (d fakeDialer) Dial(context.Context) (Conn, error) {
	return d.conn, d.err
}

type trackingDialer struct {
	conn   Conn
	err    error
	called bool
}

func (d *trackingDialer) Dial(context.Context) (Conn, error) {
	d.called = true
	return d.conn, d.err
}

type fakeConn struct{}

func (fakeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (fakeConn) Close() error                     { return nil }
func (fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeConn) SetWriteDeadline(time.Time) error { return nil }

type fakeProxyDialer struct {
	conn       Conn
	err        error
	devicePath string
	dialed     bool
}

func (d *fakeProxyDialer) Dial(context.Context) (Conn, error) {
	d.dialed = true
	return d.conn, d.err
}

func (d *fakeProxyDialer) usesProxy() bool { return true }

func (d *fakeProxyDialer) device() string { return d.devicePath }

type scriptConn struct {
	read   *bytes.Reader
	write  bytes.Buffer
	ready  chan struct{}
	once   sync.Once
	closed bool
}

func newScriptConn(data []byte) *scriptConn {
	return &scriptConn{read: bytes.NewReader(data), ready: make(chan struct{})}
}

func (c *scriptConn) Read(p []byte) (int, error) {
	if c.read == nil {
		return 0, io.EOF
	}
	<-c.ready
	return c.read.Read(p)
}

func (c *scriptConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.ready) })
	return c.write.Write(p)
}

func (c *scriptConn) Close() error {
	c.closed = true
	c.once.Do(func() { close(c.ready) })
	return nil
}

func (c *scriptConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptConn) SetWriteDeadline(time.Time) error { return nil }

func TestOpenUsesDialer(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr bool
	}{
		{"custom dialer", []Option{WithDialer(fakeDialer{conn: fakeConn{}})}, false},
		{"missing mode", nil, true},
		{"nil dialer", []Option{WithDialer(nil)}, true},
		{"dial error", []Option{WithDialer(fakeDialer{err: errors.New("boom")})}, true},
		{"empty auto-detect device", []Option{WithAutoDetect(" ")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Open(context.Background(), tt.opts...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Open() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if got == nil {
				t.Fatal("Open() = nil, want transport")
			}
			if got.UsesProxy() {
				t.Fatal("UsesProxy() = true, want false")
			}
		})
	}
}

func TestOpenOptionsSetAccess(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantDialer Dialer
		wantAuto   bool
		wantShared bool
		wantDevice string
	}{
		{name: "proxy", opts: []Option{WithProxy("/dev/cdc-wdm0")}, wantDialer: ProxyDialer{Device: "/dev/cdc-wdm0"}},
		{name: "direct", opts: []Option{WithDirect("/dev/cdc-wdm0")}, wantDialer: DirectDialer{Device: "/dev/cdc-wdm0"}, wantShared: true},
		{name: "custom direct remains exclusive", opts: []Option{WithDialer(DirectDialer{Device: "/dev/cdc-wdm0"})}, wantDialer: DirectDialer{Device: "/dev/cdc-wdm0"}},
		{name: "auto", opts: []Option{WithAutoDetect("/dev/cdc-wdm0")}, wantAuto: true, wantDevice: "/dev/cdc-wdm0"},
		{name: "direct last option wins", opts: []Option{WithAutoDetect("auto"), WithDirect("direct")}, wantDialer: DirectDialer{Device: "direct"}, wantShared: true},
		{name: "auto last option wins", opts: []Option{WithDirect("direct"), WithAutoDetect("auto")}, wantAuto: true, wantDevice: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{}
			for _, opt := range tt.opts {
				opt(&cfg)
			}
			if cfg.dialer != tt.wantDialer || cfg.autoDetect != tt.wantAuto || cfg.sharedDirect != tt.wantShared || cfg.device != tt.wantDevice {
				t.Fatalf("config = %#v, want dialer=%#v auto=%t shared=%t device=%q", cfg, tt.wantDialer, tt.wantAuto, tt.wantShared, tt.wantDevice)
			}
		})
	}
}

func TestOpenErrorReportsSelectedAccess(t *testing.T) {
	probeErr := errors.New("probe rejected")
	tests := []struct {
		name      string
		dialer    Dialer
		wantProxy bool
	}{
		{name: "direct", dialer: fakeDialer{err: probeErr}},
		{
			name:      "proxy",
			dialer:    &fakeProxyDialer{err: probeErr, devicePath: "/dev/cdc-wdm0"},
			wantProxy: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(context.Background(), WithDialer(tt.dialer))
			var openErr *OpenError
			if !errors.As(err, &openErr) {
				t.Fatalf("Open() error = %v, want *OpenError", err)
			}
			if !errors.Is(err, probeErr) {
				t.Errorf("errors.Is(Open(), probeErr) = false")
			}
			if openErr.Proxy != tt.wantProxy {
				t.Errorf("OpenError.Proxy = %t, want %t", openErr.Proxy, tt.wantProxy)
			}
		})
	}
}

func TestDialAuto(t *testing.T) {
	errUnavailable := errors.New("proxy unavailable")
	tests := []struct {
		name       string
		ctx        func() context.Context
		proxyErr   error
		directErr  error
		wantProxy  bool
		wantDirect bool
		wantErr    error
	}{
		{name: "proxy", ctx: context.Background, wantProxy: true},
		{name: "direct fallback", ctx: context.Background, proxyErr: errUnavailable, wantDirect: true},
		{name: "canceled", ctx: canceledContext, proxyErr: errUnavailable, wantProxy: true, wantErr: context.Canceled},
		{name: "proxy timeout", ctx: context.Background, proxyErr: context.DeadlineExceeded, wantProxy: true, wantErr: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyConn := &fakeConn{}
			directConn := &fakeConn{}
			proxy := &trackingDialer{conn: proxyConn, err: tt.proxyErr}
			direct := &trackingDialer{conn: directConn, err: tt.directErr}
			got, usesProxy, err := dialAuto(tt.ctx(), proxy, direct)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("dialAuto() error = %v, want %v", err, tt.wantErr)
			}
			if usesProxy != tt.wantProxy {
				t.Errorf("dialAuto() proxy = %t, want %t", usesProxy, tt.wantProxy)
			}
			if direct.called != tt.wantDirect {
				t.Errorf("direct dialed = %t, want %t", direct.called, tt.wantDirect)
			}
			if err == nil {
				want := Conn(directConn)
				if tt.wantProxy {
					want = proxyConn
				}
				if got != want {
					t.Errorf("dialAuto() conn = %p, want %p", got, want)
				}
			}
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestOpenConfiguresProxy(t *testing.T) {
	tests := []struct {
		name       string
		device     string
		read       []byte
		wantErr    bool
		wantClosed bool
	}{
		{
			name:   "proxy device",
			device: "/dev/cdc-wdm0",
			read: []byte{
				0x01, 0x12, 0x00, 0x80, 0x00, 0x00,
				0x01, 0x01, 0x00, 0xFF, 0x07, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:    "missing device",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newScriptConn(tt.read)
			dialer := &fakeProxyDialer{conn: conn, devicePath: tt.device}
			got, err := Open(context.Background(),
				WithDialer(dialer),
			)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Open() error = nil, want error")
				}
				if tt.device == "" && dialer.dialed {
					t.Fatal("Open() dialed proxy before validating device")
				}
				if conn.closed != tt.wantClosed {
					t.Fatalf("conn closed = %v, want %v", conn.closed, tt.wantClosed)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if got == nil {
				t.Fatal("Open() = nil, want transport")
			}
			if !got.UsesProxy() {
				t.Fatal("UsesProxy() = false, want true")
			}

			var req Response
			if err := req.UnmarshalBinary(conn.write.Bytes()); err == nil {
				t.Fatalf("proxy request parsed as response: %v", err)
			}
			frame := conn.write.Bytes()
			if !bytes.Contains(frame, []byte(tt.device)) {
				t.Fatalf("proxy open frame = % X, want device path %q", frame, tt.device)
			}
			if qcom.MessageID(frame[8])|qcom.MessageID(frame[9])<<8 != qcom.MessageInternalProxyOpen {
				t.Fatalf("proxy open message = 0x%04X, want 0x%04X", qcom.MessageID(frame[8])|qcom.MessageID(frame[9])<<8, qcom.MessageInternalProxyOpen)
			}
		})
	}
}
