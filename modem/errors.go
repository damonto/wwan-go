package modem

import (
	"errors"

	"github.com/damonto/wwan-go/modem/contract"
)

var (
	ErrNotSupported    = contract.ErrNotSupported
	ErrClosed          = errors.New("modem is closed")
	ErrProtocolUnknown = errors.New("modem control port protocol is unknown")
	ErrPortChanged     = errors.New("modem control port changed since discovery")
)
