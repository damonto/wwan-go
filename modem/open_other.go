//go:build !linux

package modem

import "context"

func Open(context.Context, string, Access) (*Modem, error) {
	return nil, ErrNotSupported
}
