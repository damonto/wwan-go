package contract

import "errors"

// ErrNotSupported reports an operation unavailable in the selected protocol.
var ErrNotSupported = errors.New("operation is not supported")
