package modem

import (
	"context"
	"fmt"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/qcom"
	qmiproto "github.com/damonto/wwan-go/qcom/qmi"
)

var (
	openModemQMIClient  = openQMIClient
	openModemMBIMClient = openMBIMClient
)

// QMIClient opens an independently owned QMI client using the modem's
// resolved device and access method. The caller must close the returned
// client. Direct clients share the process-wide transport but own their QMI
// service client IDs.
func (m *Modem) QMIClient(ctx context.Context, slot uint8) (*qcom.Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.backend == nil {
		return nil, ErrClosed
	}
	if m.port.Type != PortQMI {
		return nil, fmt.Errorf("opening QMI client: modem protocol is %s", m.port.Protocol())
	}
	if m.access != AccessProxy && m.access != AccessDirect {
		return nil, unresolvedAccessError("QMI")
	}
	return openModemQMIClient(ctx, m.port.Path, m.access, slot)
}

// MBIMClient opens an independently owned MBIM client using the modem's
// resolved device and access method. The caller must close the returned
// client. Direct clients share the process-wide MBIM dispatcher.
func (m *Modem) MBIMClient(ctx context.Context, slot uint8) (*mbimproto.Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.backend == nil {
		return nil, ErrClosed
	}
	if m.port.Type != PortMBIM {
		return nil, fmt.Errorf("opening MBIM client: modem protocol is %s", m.port.Protocol())
	}
	if m.access != AccessProxy && m.access != AccessDirect {
		return nil, unresolvedAccessError("MBIM")
	}
	return openModemMBIMClient(ctx, m.port.Path, m.access, slot)
}

func openQMIClient(ctx context.Context, device string, access Access, slot uint8) (*qcom.Client, error) {
	var option qmiproto.Option
	switch access {
	case AccessProxy:
		option = qmiproto.WithProxy(device)
	case AccessDirect:
		option = qmiproto.WithDirect(device)
	default:
		return nil, unresolvedAccessError("QMI")
	}

	transport, err := qmiproto.Open(ctx, option)
	if err != nil {
		return nil, fmt.Errorf("opening QMI client transport: %w", err)
	}
	client, err := qcom.NewClient(transport, qcom.WithSlot(slot))
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	return client, nil
}

func openMBIMClient(ctx context.Context, device string, access Access, slot uint8) (*mbimproto.Client, error) {
	var option mbimproto.Option
	switch access {
	case AccessProxy:
		option = mbimproto.WithProxy(device)
	case AccessDirect:
		option = mbimproto.WithDirect(device)
	default:
		return nil, unresolvedAccessError("MBIM")
	}

	client, err := mbimproto.Open(ctx, option, mbimproto.WithSlot(int(slot)))
	if err != nil {
		return nil, fmt.Errorf("opening MBIM client: %w", err)
	}
	return client, nil
}

func unresolvedAccessError(protocol string) error {
	return fmt.Errorf("opening %s client: modem access method is unresolved", protocol)
}
