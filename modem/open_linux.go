//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	mbimproto "github.com/damonto/wwan-go/mbim"
	modemmbim "github.com/damonto/wwan-go/modem/mbim"
	modemqmi "github.com/damonto/wwan-go/modem/qmi"
	"github.com/damonto/wwan-go/qcom"
	qmiproto "github.com/damonto/wwan-go/qcom/qmi"
)

var (
	statDevice      = os.Stat
	openQMIBackend  = openQMI
	openMBIMBackend = openMBIM
)

// Open opens a QMI or MBIM control port through the selected access method.
// The port protocol must come from kernel discovery or be set explicitly by
// the caller. Open never probes or falls back to another protocol.
func Open(ctx context.Context, port Port, access Access) (*Modem, error) {
	if err := validateOpenInput(ctx, port, access); err != nil {
		return nil, err
	}

	var b backend
	selected := access
	var err error
	switch port.Type {
	case PortQMI:
		b, selected, err = openQMIBackend(ctx, port.Path, access)
	case PortMBIM:
		b, selected, err = openMBIMBackend(ctx, port.Path, access)
	}
	if err != nil {
		return nil, fmt.Errorf("opening modem port %s: %w", port.Path, err)
	}
	return newModem(port, selected, b), nil
}

func validateOpenInput(ctx context.Context, port Port, access Access) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if port.Path == "" {
		return errors.New("opening modem: control port path is empty")
	}
	if port.Protocol() == ProtocolUnknown {
		return fmt.Errorf("opening modem port %s: %w", port.Path, ErrProtocolUnknown)
	}
	if access != AccessAuto && access != AccessProxy && access != AccessDirect {
		return fmt.Errorf("opening modem: access method %d is invalid", access)
	}
	info, err := statDevice(port.Path)
	if err != nil {
		return fmt.Errorf("opening modem node %s: %w", port.Path, err)
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("opening modem node %s: not a character device", port.Path)
	}
	if err := validatePortMetadata(port); err != nil {
		return err
	}
	return nil
}

func validatePortMetadata(port Port) error {
	if port.SysPath == "" {
		return nil
	}

	wantProtocol := port.Protocol()
	if protocol := kernelProtocol(port.SysPath); protocol != wantProtocol {
		return fmt.Errorf("validating modem port %s metadata: %w: protocol is %s, want %s", port.Path, ErrPortChanged, protocol, wantProtocol)
	}
	if port.Driver == "" {
		return nil
	}
	if driver := symlinkBase(filepath.Join(port.SysPath, "device", "driver")); driver != port.Driver {
		return fmt.Errorf("validating modem port %s metadata: %w: driver is %q, want %q", port.Path, ErrPortChanged, driver, port.Driver)
	}
	return nil
}

func openQMI(ctx context.Context, device string, access Access) (backend, Access, error) {
	var option qmiproto.Option
	switch access {
	case AccessAuto:
		option = qmiproto.WithAutoDetect(device)
	case AccessProxy:
		option = qmiproto.WithProxy(device)
	case AccessDirect:
		option = qmiproto.WithDirect(device)
	}
	transport, err := qmiproto.Open(ctx, option)
	if err != nil {
		return nil, access, fmt.Errorf("opening QMI transport: %w", err)
	}
	selected := AccessDirect
	if transport.UsesProxy() {
		selected = AccessProxy
	}
	client, err := qcom.NewClient(transport)
	if err != nil {
		_ = transport.Close()
		return nil, selected, fmt.Errorf("creating QMI client: %w", err)
	}
	if _, err := client.DeviceCapabilities(ctx); err != nil {
		_ = client.Close()
		return nil, selected, fmt.Errorf("probing QMI device capabilities: %w", err)
	}
	return modemqmi.New(client, device), selected, nil
}

func openMBIM(ctx context.Context, device string, access Access) (backend, Access, error) {
	var option mbimproto.Option
	switch access {
	case AccessAuto:
		option = mbimproto.WithAutoDetect(device)
	case AccessProxy:
		option = mbimproto.WithProxy(device)
	case AccessDirect:
		option = mbimproto.WithDirect(device)
	}
	client, err := mbimproto.Open(ctx, option)
	if err != nil {
		return nil, access, fmt.Errorf("opening MBIM client: %w", err)
	}
	selected := AccessDirect
	if client.UsesProxy() {
		selected = AccessProxy
	}
	if _, err := client.DeviceServices(ctx); err != nil {
		_ = client.Close()
		return nil, selected, fmt.Errorf("probing MBIM device services: %w", err)
	}
	return modemmbim.New(client, device), selected, nil
}
