//go:build linux

package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	mbimproto "github.com/damonto/wwan-go/mbim"
	modemmbim "github.com/damonto/wwan-go/modem/mbim"
	modemqmi "github.com/damonto/wwan-go/modem/qmi"
	"github.com/damonto/wwan-go/qcom"
	qmiproto "github.com/damonto/wwan-go/qcom/qmi"
)

const probeTimeout = 5 * time.Second

var (
	statDevice      = os.Stat
	protocolForNode = systemProtocolForNode
	openQMIBackend  = openQMI
	openMBIMBackend = openMBIM
)

// Open detects the control protocol and opens deviceNode through the selected
// access method. It does not start a background state machine.
func Open(ctx context.Context, deviceNode string, access Access) (*Modem, error) {
	if err := validateOpenInput(ctx, deviceNode, access); err != nil {
		return nil, err
	}

	hint := protocolForNode(deviceNode)
	protocols := protocolProbeOrder(hint)

	openErr := &OpenError{Device: deviceNode}
	for _, protocol := range protocols {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		var b backend
		selected := access
		var err error
		switch protocol {
		case ProtocolQMI:
			b, selected, err = openQMIBackend(probeCtx, deviceNode, access)
		case ProtocolMBIM:
			b, selected, err = openMBIMBackend(probeCtx, deviceNode, access)
		}
		cancel()
		if err == nil {
			return newModem(deviceNode, protocol, selected, b), nil
		}
		openErr.Attempts = append(openErr.Attempts, ProbeError{Protocol: protocol, Access: selected, Err: err})
		if ctx.Err() != nil {
			return nil, openErr
		}
	}
	return nil, openErr
}

func protocolProbeOrder(hint Protocol) []Protocol {
	switch hint {
	case ProtocolQMI:
		return []Protocol{ProtocolQMI, ProtocolMBIM}
	case ProtocolMBIM:
		return []Protocol{ProtocolMBIM, ProtocolQMI}
	default:
		return []Protocol{ProtocolQMI, ProtocolMBIM}
	}
}

func validateOpenInput(ctx context.Context, deviceNode string, access Access) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deviceNode == "" {
		return errors.New("opening modem: device node is empty")
	}
	if access != AccessAuto && access != AccessProxy && access != AccessDirect {
		return fmt.Errorf("opening modem: access method %d is invalid", access)
	}
	info, err := statDevice(deviceNode)
	if err != nil {
		return fmt.Errorf("opening modem node %s: %w", deviceNode, err)
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("opening modem node %s: not a character device", deviceNode)
	}
	return nil
}

func systemProtocolForNode(deviceNode string) Protocol {
	name := filepath.Base(deviceNode)
	for _, class := range []string{"wwan", "usbmisc"} {
		entryPath := filepath.Join(defaultSysRoot, "class", class, name)
		if _, err := os.Stat(entryPath); err == nil {
			return protocolHint(entryPath, name)
		}
	}
	return protocolHint("", name)
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
		return nil, qmiAccessFromError(access, err), fmt.Errorf("opening QMI transport: %w", err)
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
		return nil, mbimAccessFromError(access, err), fmt.Errorf("opening MBIM client: %w", err)
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

func qmiAccessFromError(requested Access, err error) Access {
	if requested != AccessAuto {
		return requested
	}
	var openErr *qmiproto.OpenError
	if errors.As(err, &openErr) && openErr.Proxy {
		return AccessProxy
	}
	return AccessDirect
}

func mbimAccessFromError(requested Access, err error) Access {
	if requested != AccessAuto {
		return requested
	}
	var openErr *mbimproto.OpenError
	if errors.As(err, &openErr) && openErr.Proxy {
		return AccessProxy
	}
	return AccessDirect
}
