//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"testing"
	"time"

	mbimproto "github.com/damonto/wwan-go/mbim"
	qmiproto "github.com/damonto/wwan-go/qcom/qmi"
)

type testFileInfo struct{}

func (testFileInfo) Name() string       { return "cdc-wdm0" }
func (testFileInfo) Size() int64        { return 0 }
func (testFileInfo) Mode() fs.FileMode  { return os.ModeDevice | os.ModeCharDevice }
func (testFileInfo) ModTime() time.Time { return time.Time{} }
func (testFileInfo) IsDir() bool        { return false }
func (testFileInfo) Sys() any           { return nil }

type testBackend struct {
	unsupportedBackend
	closed bool
}

func TestOpenProbeSelection(t *testing.T) {
	probeErr := errors.New("probe rejected")
	tests := []struct {
		name          string
		hint          Protocol
		requested     Access
		qmiErr        error
		mbimErr       error
		wantProtocol  Protocol
		wantAccess    Access
		wantAttempts  []Protocol
		wantOpenError bool
	}{
		{name: "explicit QMI direct", hint: ProtocolQMI, requested: AccessDirect, wantProtocol: ProtocolQMI, wantAccess: AccessDirect, wantAttempts: []Protocol{ProtocolQMI}},
		{name: "explicit MBIM proxy", hint: ProtocolMBIM, requested: AccessProxy, wantProtocol: ProtocolMBIM, wantAccess: AccessProxy, wantAttempts: []Protocol{ProtocolMBIM}},
		{name: "QMI driver auto selects proxy", hint: ProtocolQMI, requested: AccessAuto, wantProtocol: ProtocolQMI, wantAccess: AccessProxy, wantAttempts: []Protocol{ProtocolQMI}},
		{name: "MBIM driver auto selects direct", hint: ProtocolMBIM, requested: AccessAuto, wantProtocol: ProtocolMBIM, wantAccess: AccessDirect, wantAttempts: []Protocol{ProtocolMBIM}},
		{name: "unknown falls through to MBIM", hint: ProtocolUnknown, requested: AccessDirect, qmiErr: probeErr, wantProtocol: ProtocolMBIM, wantAccess: AccessDirect, wantAttempts: []Protocol{ProtocolQMI, ProtocolMBIM}},
		{name: "unknown continues after QMI probe timeout", hint: ProtocolUnknown, requested: AccessDirect, qmiErr: context.DeadlineExceeded, wantProtocol: ProtocolMBIM, wantAccess: AccessDirect, wantAttempts: []Protocol{ProtocolQMI, ProtocolMBIM}},
		{name: "QMI hint falls back to MBIM", hint: ProtocolQMI, requested: AccessAuto, qmiErr: probeErr, wantProtocol: ProtocolMBIM, wantAccess: AccessProxy, wantAttempts: []Protocol{ProtocolQMI, ProtocolMBIM}},
		{name: "MBIM hint falls back to QMI", hint: ProtocolMBIM, requested: AccessDirect, mbimErr: probeErr, wantProtocol: ProtocolQMI, wantAccess: AccessDirect, wantAttempts: []Protocol{ProtocolMBIM, ProtocolQMI}},
		{name: "known hint retains both failures", hint: ProtocolQMI, requested: AccessDirect, qmiErr: probeErr, mbimErr: probeErr, wantAttempts: []Protocol{ProtocolQMI, ProtocolMBIM}, wantOpenError: true},
		{name: "unknown retains both failures", hint: ProtocolUnknown, requested: AccessDirect, qmiErr: probeErr, mbimErr: probeErr, wantAttempts: []Protocol{ProtocolQMI, ProtocolMBIM}, wantOpenError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStat, oldProtocol := statDevice, protocolForNode
			oldQMI, oldMBIM := openQMIBackend, openMBIMBackend
			t.Cleanup(func() {
				statDevice, protocolForNode = oldStat, oldProtocol
				openQMIBackend, openMBIMBackend = oldQMI, oldMBIM
			})

			statDevice = func(string) (os.FileInfo, error) { return testFileInfo{}, nil }
			protocolForNode = func(string) Protocol { return tt.hint }
			var attempts []Protocol
			var accesses []Access
			openQMIBackend = func(_ context.Context, _ string, access Access) (backend, Access, error) {
				attempts = append(attempts, ProtocolQMI)
				accesses = append(accesses, access)
				if tt.qmiErr != nil {
					return nil, access, tt.qmiErr
				}
				return &testBackend{}, tt.wantAccess, nil
			}
			openMBIMBackend = func(_ context.Context, _ string, access Access) (backend, Access, error) {
				attempts = append(attempts, ProtocolMBIM)
				accesses = append(accesses, access)
				if tt.mbimErr != nil {
					return nil, access, tt.mbimErr
				}
				return &testBackend{}, tt.wantAccess, nil
			}

			got, err := Open(context.Background(), "/dev/cdc-wdm0", tt.requested)
			if tt.wantOpenError {
				var openErr *OpenError
				if !errors.As(err, &openErr) {
					t.Fatalf("Open() error = %v, want *OpenError", err)
				}
				if !errors.Is(err, probeErr) {
					t.Errorf("errors.Is(Open(), probeErr) = false")
				}
			} else {
				if err != nil {
					t.Fatalf("Open() error = %v", err)
				}
				if got.Protocol() != tt.wantProtocol || got.Access() != tt.wantAccess {
					t.Errorf("Open() = (%s, %s), want (%s, %s)", got.Protocol(), got.Access(), tt.wantProtocol, tt.wantAccess)
				}
				if err := got.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}
			if !reflect.DeepEqual(attempts, tt.wantAttempts) {
				t.Errorf("probe attempts = %v, want %v", attempts, tt.wantAttempts)
			}
			for _, access := range accesses {
				if access != tt.requested {
					t.Errorf("driver access = %s, want %s", access, tt.requested)
				}
			}
		})
	}
}

func TestOpenInputValidation(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "regular")
	if err != nil {
		t.Fatal(err)
	}
	if err := regular.Close(); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name   string
		ctx    context.Context
		device string
		access Access
	}{
		{name: "canceled", ctx: canceled, device: "/dev/null", access: AccessDirect},
		{name: "empty device", ctx: context.Background(), access: AccessDirect},
		{name: "invalid access", ctx: context.Background(), device: "/dev/null", access: Access(99)},
		{name: "regular file", ctx: context.Background(), device: regular.Name(), access: AccessDirect},
		{name: "missing file", ctx: context.Background(), device: regular.Name() + "-missing", access: AccessDirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Open(tt.ctx, tt.device, tt.access); err == nil {
				t.Fatal("Open() error = nil, want non-nil")
			}
		})
	}
}

func TestAccessFromDriverOpenError(t *testing.T) {
	probeErr := errors.New("probe rejected")
	tests := []struct {
		name      string
		requested Access
		err       error
		access    func(Access, error) Access
		want      Access
	}{
		{
			name:   "QMI auto proxy",
			err:    &qmiproto.OpenError{Proxy: true, Err: probeErr},
			access: qmiAccessFromError,
			want:   AccessProxy,
		},
		{
			name:   "QMI auto direct",
			err:    fmt.Errorf("wrapped: %w", &qmiproto.OpenError{Err: probeErr}),
			access: qmiAccessFromError,
			want:   AccessDirect,
		},
		{
			name:   "MBIM auto proxy",
			err:    &mbimproto.OpenError{Proxy: true, Err: probeErr},
			access: mbimAccessFromError,
			want:   AccessProxy,
		},
		{
			name:      "explicit access is preserved",
			requested: AccessDirect,
			err:       &mbimproto.OpenError{Proxy: true, Err: probeErr},
			access:    mbimAccessFromError,
			want:      AccessDirect,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.access(tt.requested, tt.err); got != tt.want {
				t.Errorf("accessFromError() = %s, want %s", got, tt.want)
			}
		})
	}
}
