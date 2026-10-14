package contract

import "errors"

// SIMLock identifies the credential currently required by the active SIM.
type SIMLock string

const (
	SIMLockUnknown SIMLock = "unknown"
	SIMLockNone    SIMLock = "none"
	SIMLockPIN     SIMLock = "pin"
	SIMLockPUK     SIMLock = "puk"
	SIMLockBlocked SIMLock = "permanently_blocked"
	SIMLockOther   SIMLock = "other"
)

// SIMPINStatus separates PIN protection from verification in the current session.
// Retry counts and Enabled are meaningful only when their Known field is true.
type SIMPINStatus struct {
	Supported       bool
	Enabled         bool
	EnabledKnown    bool
	Lock            SIMLock
	PINRetries      uint32
	PINRetriesKnown bool
	PUKRetries      uint32
	PUKRetriesKnown bool
}

var (
	ErrIncorrectPIN          = errors.New("incorrect SIM credential")
	ErrPINRequired           = errors.New("SIM PIN is required")
	ErrPUKRequired           = errors.New("SIM PUK is required")
	ErrPINPermanentlyBlocked = errors.New("SIM is permanently blocked")
)

// PINOperationError preserves response retry counts even when a command fails.
// Neither its message nor Status contains the submitted credential.
type PINOperationError struct {
	Status SIMPINStatus
	Reason error
	Cause  error
}

func (e *PINOperationError) Error() string { return "SIM PIN operation did not complete" }

// Unwrap preserves both the portable reason and the original protocol error.
func (e *PINOperationError) Unwrap() []error {
	var result []error
	if e.Reason != nil {
		result = append(result, e.Reason)
	}
	if e.Cause != nil {
		result = append(result, e.Cause)
	}
	return result
}
