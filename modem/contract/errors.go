package contract

import (
	"errors"
	"fmt"
)

// ErrNotSupported reports an operation unavailable in the selected protocol.
var ErrNotSupported = errors.New("operation is not supported")

// MessageDecodeError reports a malformed SMS without implying a transport
// failure. Ref identifies a stored message; unknown storage means an unstored
// message. PDU retains an owned copy of the modem payload for diagnosis; QMI
// transfer-route payloads are TPDUs without an SMSC prefix.
type MessageDecodeError struct {
	Ref MessageRef
	PDU []byte
	Err error
}

func (e *MessageDecodeError) Error() string {
	if e.Ref.Storage == MessageStorageUnknown {
		return fmt.Sprintf("decoding unstored message: %v", e.Err)
	}
	return fmt.Sprintf("decoding message %d in storage %d: %v", e.Ref.ID, e.Ref.Storage, e.Err)
}

// Unwrap returns the PDU decoding error.
func (e *MessageDecodeError) Unwrap() error { return e.Err }

// MessageListError reports malformed records omitted from a decoded batch.
// ListMessages returns this error alongside usable messages. Transport errors
// never use this type.
type MessageListError struct {
	Errors []*MessageDecodeError
}

func (e *MessageListError) Error() string {
	return fmt.Sprintf("decoding message list: %v", errors.Join(e.Unwrap()...))
}

// Unwrap returns the individual decoding errors.
func (e *MessageListError) Unwrap() []error {
	errs := make([]error, len(e.Errors))
	for i, err := range e.Errors {
		errs[i] = err
	}
	return errs
}
