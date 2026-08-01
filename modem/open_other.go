//go:build !linux

package modem

import "context"

func Open(context.Context, Port, Access) (*Modem, error) {
	return nil, ErrNotSupported
}
