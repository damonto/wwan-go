//go:build linux

package modem

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
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
}

func TestOpenUsesDeclaredPortProtocol(t *testing.T) {
	openErr := errors.New("open rejected")
	tests := []struct {
		name          string
		portType      PortType
		requested     Access
		selected      Access
		qmiErr        error
		mbimErr       error
		wantQMICalls  int
		wantMBIMCalls int
		wantErr       bool
	}{
		{name: "QMI direct", portType: PortQMI, requested: AccessDirect, selected: AccessDirect, wantQMICalls: 1},
		{name: "MBIM proxy", portType: PortMBIM, requested: AccessProxy, selected: AccessProxy, wantMBIMCalls: 1},
		{name: "QMI auto resolves proxy", portType: PortQMI, requested: AccessAuto, selected: AccessProxy, wantQMICalls: 1},
		{name: "MBIM auto resolves direct", portType: PortMBIM, requested: AccessAuto, selected: AccessDirect, wantMBIMCalls: 1},
		{name: "QMI error does not probe MBIM", portType: PortQMI, requested: AccessDirect, selected: AccessDirect, qmiErr: openErr, wantQMICalls: 1, wantErr: true},
		{name: "MBIM error does not probe QMI", portType: PortMBIM, requested: AccessDirect, selected: AccessDirect, mbimErr: openErr, wantMBIMCalls: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStat := statDevice
			oldQMI, oldMBIM := openQMIBackend, openMBIMBackend
			t.Cleanup(func() {
				statDevice = oldStat
				openQMIBackend, openMBIMBackend = oldQMI, oldMBIM
			})

			statDevice = func(string) (os.FileInfo, error) { return testFileInfo{}, nil }
			qmiCalls := 0
			mbimCalls := 0
			openQMIBackend = func(_ context.Context, device string, access Access) (backend, Access, error) {
				qmiCalls++
				if device != "/dev/cdc-wdm0" || access != tt.requested {
					t.Errorf("QMI endpoint = (%q, %s), want (/dev/cdc-wdm0, %s)", device, access, tt.requested)
				}
				return &testBackend{}, tt.selected, tt.qmiErr
			}
			openMBIMBackend = func(_ context.Context, device string, access Access) (backend, Access, error) {
				mbimCalls++
				if device != "/dev/cdc-wdm0" || access != tt.requested {
					t.Errorf("MBIM endpoint = (%q, %s), want (/dev/cdc-wdm0, %s)", device, access, tt.requested)
				}
				return &testBackend{}, tt.selected, tt.mbimErr
			}

			port := Port{Type: tt.portType, Name: "cdc-wdm0", Path: "/dev/cdc-wdm0"}
			got, err := Open(context.Background(), port, tt.requested)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Open() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, openErr) {
					t.Fatalf("Open() error = %v, want %v", err, openErr)
				}
			} else {
				if got.Port() != port || got.Protocol() != port.Protocol() || got.Access() != tt.selected {
					t.Errorf("Open() = (%+v, %s, %s), want (%+v, %s, %s)", got.Port(), got.Protocol(), got.Access(), port, port.Protocol(), tt.selected)
				}
				if err := got.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}
			if qmiCalls != tt.wantQMICalls || mbimCalls != tt.wantMBIMCalls {
				t.Errorf("open calls = (QMI %d, MBIM %d), want (QMI %d, MBIM %d)", qmiCalls, mbimCalls, tt.wantQMICalls, tt.wantMBIMCalls)
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
		port   Port
		access Access
		wantIs error
	}{
		{name: "canceled", ctx: canceled, port: Port{Type: PortQMI, Path: "/dev/null"}, access: AccessDirect, wantIs: context.Canceled},
		{name: "empty path", ctx: context.Background(), port: Port{Type: PortQMI}, access: AccessDirect},
		{name: "unknown protocol", ctx: context.Background(), port: Port{Type: PortUnknown, Path: "/dev/null"}, access: AccessDirect, wantIs: ErrProtocolUnknown},
		{name: "AT port", ctx: context.Background(), port: Port{Type: PortAT, Path: "/dev/null"}, access: AccessDirect, wantIs: ErrProtocolUnknown},
		{name: "network port", ctx: context.Background(), port: Port{Type: PortNetwork, Path: "/dev/null"}, access: AccessDirect, wantIs: ErrProtocolUnknown},
		{name: "invalid access", ctx: context.Background(), port: Port{Type: PortQMI, Path: "/dev/null"}, access: Access(99)},
		{name: "regular file", ctx: context.Background(), port: Port{Type: PortQMI, Path: regular.Name()}, access: AccessDirect},
		{name: "missing file", ctx: context.Background(), port: Port{Type: PortQMI, Path: regular.Name() + "-missing"}, access: AccessDirect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(tt.ctx, tt.port, tt.access)
			if err == nil {
				t.Fatal("Open() error = nil, want non-nil")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Open() error = %v, want errors.Is(%v)", err, tt.wantIs)
			}
		})
	}
}

func TestOpenValidatesPortMetadata(t *testing.T) {
	tests := []struct {
		name             string
		manual           bool
		createMetadata   bool
		discoveredDriver string
		currentProtocol  string
		currentDriver    string
		wantErr          bool
		wantCalls        int
	}{
		{
			name:             "metadata matches",
			createMetadata:   true,
			discoveredDriver: "qmi_wwan",
			currentProtocol:  "QMI",
			currentDriver:    "qmi_wwan",
			wantCalls:        1,
		},
		{
			name:             "protocol changed",
			createMetadata:   true,
			discoveredDriver: "qmi_wwan",
			currentProtocol:  "MBIM",
			currentDriver:    "cdc_mbim",
			wantErr:          true,
		},
		{
			name:             "driver changed",
			createMetadata:   true,
			discoveredDriver: "qmi_wwan",
			currentProtocol:  "QMI",
			currentDriver:    "cdc_mbim",
			wantErr:          true,
		},
		{
			name:             "metadata disappeared",
			discoveredDriver: "qmi_wwan",
			wantErr:          true,
		},
		{
			name:             "explicit port skips kernel metadata",
			manual:           true,
			discoveredDriver: "qmi_wwan",
			wantCalls:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryPath := filepath.Join(t.TempDir(), "sys", "class", "usbmisc", "cdc-wdm0")
			if tt.createMetadata {
				writePortMetadata(t, entryPath, tt.currentProtocol, tt.currentDriver)
			}
			port := Port{
				Type:   PortQMI,
				Name:   "cdc-wdm0",
				Path:   "/dev/cdc-wdm0",
				Driver: tt.discoveredDriver,
			}
			if !tt.manual {
				port.SysPath = entryPath
			}

			oldStat, oldQMI := statDevice, openQMIBackend
			t.Cleanup(func() {
				statDevice, openQMIBackend = oldStat, oldQMI
			})
			statDevice = func(string) (os.FileInfo, error) { return testFileInfo{}, nil }
			calls := 0
			openQMIBackend = func(context.Context, string, Access) (backend, Access, error) {
				calls++
				return &testBackend{}, AccessDirect, nil
			}

			got, err := Open(context.Background(), port, AccessDirect)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Open() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrPortChanged) {
				t.Errorf("Open() error = %v, want errors.Is(ErrPortChanged)", err)
			}
			if calls != tt.wantCalls {
				t.Errorf("QMI open calls = %d, want %d", calls, tt.wantCalls)
			}
			if got != nil {
				if err := got.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}
		})
	}
}

func writePortMetadata(t *testing.T, entryPath, protocol, driver string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(entryPath, "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	if protocol != "" {
		if err := os.WriteFile(filepath.Join(entryPath, "type"), []byte(protocol+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if driver == "" {
		return
	}
	driverPath := filepath.Join(t.TempDir(), driver)
	if err := os.MkdirAll(driverPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driverPath, filepath.Join(entryPath, "device", "driver")); err != nil {
		t.Fatal(err)
	}
}
