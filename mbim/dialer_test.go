package mbim

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type stubDialer struct{}

func (stubDialer) Dial(context.Context) (Conn, error) {
	return nil, nil
}

type trackingDialer struct {
	conn   Conn
	err    error
	called bool
}

type proxyTestDialer struct {
	trackingDialer
	devicePath string
}

func (*proxyTestDialer) usesProxy() bool { return true }

func (d *proxyTestDialer) device() string { return d.devicePath }

func (d *trackingDialer) Dial(context.Context) (Conn, error) {
	d.called = true
	return d.conn, d.err
}

type dialTestConn struct{}

func (*dialTestConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*dialTestConn) Write(p []byte) (int, error)      { return len(p), nil }
func (*dialTestConn) Close() error                     { return nil }
func (*dialTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*dialTestConn) SetWriteDeadline(time.Time) error { return nil }

func TestDialerUsesProxy(t *testing.T) {
	tests := []struct {
		name string
		in   Dialer
		want bool
	}{
		{"proxy", ProxyDialer{}, true},
		{"proxy pointer", &ProxyDialer{}, true},
		{"custom", stubDialer{}, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dialerUsesProxy(tt.in); got != tt.want {
				t.Fatalf("dialerUsesProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenOptionsSetDialer(t *testing.T) {
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
		{name: "last option wins", opts: []Option{WithAutoDetect("auto"), WithProxy("proxy")}, wantDialer: ProxyDialer{Device: "proxy"}},
		{name: "auto resets direct sharing", opts: []Option{WithDirect("direct"), WithAutoDetect("auto")}, wantAuto: true, wantDevice: "auto"},
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
		{name: "direct", dialer: &trackingDialer{err: probeErr}},
		{
			name:      "proxy",
			dialer:    &proxyTestDialer{trackingDialer: trackingDialer{err: probeErr}, devicePath: "/dev/cdc-wdm0"},
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
		{name: "canceled", ctx: canceledDialContext, proxyErr: errUnavailable, wantProxy: true, wantErr: context.Canceled},
		{name: "proxy timeout", ctx: context.Background, proxyErr: context.DeadlineExceeded, wantProxy: true, wantErr: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyConn := &dialTestConn{}
			directConn := &dialTestConn{}
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

func canceledDialContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestUsesProxy(t *testing.T) {
	tests := []struct {
		name string
		in   *Client
		want bool
	}{
		{name: "proxy", in: &Client{proxy: true}, want: true},
		{name: "direct", in: &Client{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.UsesProxy(); got != tt.want {
				t.Fatalf("UsesProxy() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestOpenOptionsSetSlot(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want int
	}{
		{"default", nil, 1},
		{"custom", []Option{WithSlot(2)}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{slot: 1}
			for _, opt := range tt.opts {
				opt(&cfg)
			}
			if cfg.slot != tt.want {
				t.Fatalf("slot = %d, want %d", cfg.slot, tt.want)
			}
		})
	}
}
