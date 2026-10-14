package modem

import (
	"context"

	"github.com/damonto/wwan-go/modem/contract"
)

// SIMLock identifies the credential required by the active SIM.
type SIMLock = contract.SIMLock

// SIMPINStatus reports protection, current lock and optional retry counts.
type SIMPINStatus = contract.SIMPINStatus

// PINOperationError preserves retry counts from rejected credential commands.
type PINOperationError = contract.PINOperationError

const (
	SIMLockUnknown = contract.SIMLockUnknown
	SIMLockNone    = contract.SIMLockNone
	SIMLockPIN     = contract.SIMLockPIN
	SIMLockPUK     = contract.SIMLockPUK
	SIMLockBlocked = contract.SIMLockBlocked
	SIMLockOther   = contract.SIMLockOther
)

var (
	ErrIncorrectPIN          = contract.ErrIncorrectPIN
	ErrPINRequired           = contract.ErrPINRequired
	ErrPUKRequired           = contract.ErrPUKRequired
	ErrPINPermanentlyBlocked = contract.ErrPINPermanentlyBlocked
)

type simPINBackend interface {
	SIMPINStatus(context.Context) (SIMPINStatus, error)
}

// SIMPINStatus reads protection, current lock and available retry counts.
func (m *Modem) SIMPINStatus(ctx context.Context) (SIMPINStatus, error) {
	b, err := m.currentBackend()
	if err != nil {
		return SIMPINStatus{}, err
	}
	reader, ok := b.(simPINBackend)
	if !ok {
		return SIMPINStatus{}, ErrNotSupported
	}
	return reader.SIMPINStatus(ctx)
}
