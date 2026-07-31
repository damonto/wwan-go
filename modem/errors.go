package modem

import (
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/modem/contract"
)

var (
	ErrNotSupported    = contract.ErrNotSupported
	ErrClosed          = errors.New("modem is closed")
	ErrProtocolUnknown = errors.New("modem protocol is unknown")
)

// ProbeError records one protocol and access-method attempt made by Open.
type ProbeError struct {
	Protocol Protocol
	Access   Access
	Err      error
}

func (e ProbeError) Error() string {
	return fmt.Sprintf("probing %s over %s: %v", e.Protocol, e.Access, e.Err)
}

func (e ProbeError) Unwrap() error { return e.Err }

// OpenError keeps every failed protocol probe available to errors.Is and
// errors.As callers.
type OpenError struct {
	Device   string
	Attempts []ProbeError
}

func (e *OpenError) Error() string {
	if len(e.Attempts) == 0 {
		return fmt.Sprintf("opening modem %s: %v", e.Device, ErrProtocolUnknown)
	}
	return fmt.Sprintf("opening modem %s: %v", e.Device, errors.Join(e.attemptErrors()...))
}

func (e *OpenError) Unwrap() error {
	if len(e.Attempts) == 0 {
		return ErrProtocolUnknown
	}
	return errors.Join(e.attemptErrors()...)
}

func (e *OpenError) attemptErrors() []error {
	errs := make([]error, len(e.Attempts))
	for i := range e.Attempts {
		errs[i] = e.Attempts[i]
	}
	return errs
}
