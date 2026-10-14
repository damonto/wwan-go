package mbim

import (
	"context"
	"errors"
	"math"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/contract"
)

func simPINStatus(pin mbimproto.PINInfo) contract.SIMPINStatus {
	result := contract.SIMPINStatus{Supported: true, Lock: contract.SIMLockNone}
	if pin.State == mbimproto.PINStateLocked {
		switch pin.Type {
		case mbimproto.PINTypePIN1:
			result.Lock = contract.SIMLockPIN
		case mbimproto.PINTypePUK1:
			result.Lock = contract.SIMLockPUK
		default:
			result.Lock = contract.SIMLockOther
		}
		if result.Lock == contract.SIMLockPIN || result.Lock == contract.SIMLockPUK {
			result.Enabled, result.EnabledKnown = true, true
		}
	}
	if pin.RemainingAttempts == unknownPINAttempts || pin.RemainingAttempts == math.MaxUint32 {
		return result
	}
	switch pin.Type {
	case mbimproto.PINTypePIN1:
		result.PINRetries, result.PINRetriesKnown = pin.RemainingAttempts, true
	case mbimproto.PINTypePUK1:
		result.PUKRetries, result.PUKRetriesKnown = pin.RemainingAttempts, true
		if pin.State == mbimproto.PINStateLocked && pin.RemainingAttempts == 0 {
			result.Lock = contract.SIMLockBlocked
		}
	}
	return result
}

// SIMPINStatus combines the current lock with PIN1's protection descriptor.
func (b *Backend) SIMPINStatus(ctx context.Context) (contract.SIMPINStatus, error) {
	return readSIMPINStatus(ctx, b.client)
}

type simPINStatusReader interface {
	PIN(context.Context) (mbimproto.PINInfo, error)
	PINList(context.Context) (mbimproto.PINListInfo, error)
}

func readSIMPINStatus(ctx context.Context, reader simPINStatusReader) (contract.SIMPINStatus, error) {
	pin, err := reader.PIN(ctx)
	if errors.Is(err, mbimproto.StatusNoDeviceSupport) {
		return contract.SIMPINStatus{}, contract.ErrNotSupported
	}
	if err != nil && !errors.Is(err, mbimproto.StatusPINRequired) {
		return contract.SIMPINStatus{}, err
	}
	result := simPINStatus(pin)
	if err != nil && pin.Type == mbimproto.PINTypeNone {
		result.Lock = contract.SIMLockUnknown
	}
	list, listErr := reader.PINList(ctx)
	if listErr == nil {
		result.Supported = list.PIN1.Mode != mbimproto.PINModeNotSupported
		if result.Supported {
			result.EnabledKnown, result.Enabled = true, list.PIN1.Mode == mbimproto.PINModeEnabled
		}
	} else if ctx.Err() != nil {
		return contract.SIMPINStatus{}, ctx.Err()
	}
	// A failed PIN_LIST read must not erase a usable PIN/PUK unlock result.
	return result, nil
}

func pinOperationError(result mbimproto.PINInfo, err error) error {
	if err == nil {
		return nil
	}
	status := simPINStatus(result)
	var reason error
	switch {
	case errors.Is(err, mbimproto.StatusNoDeviceSupport):
		reason = contract.ErrNotSupported
	case errors.Is(err, mbimproto.StatusPINRequired):
		reason = contract.ErrPINRequired
	case errors.Is(err, mbimproto.StatusFailure):
		// The returned PIN_INFO is authoritative even on command failure.
		switch status.Lock {
		case contract.SIMLockBlocked:
			reason = contract.ErrPINPermanentlyBlocked
		case contract.SIMLockPUK:
			reason = contract.ErrPUKRequired
		case contract.SIMLockPIN:
			reason = contract.ErrIncorrectPIN
		}
	}
	if result.Type == mbimproto.PINTypeNone {
		status = contract.SIMPINStatus{Lock: contract.SIMLockUnknown}
	}
	return &contract.PINOperationError{Status: status, Reason: reason, Cause: err}
}
