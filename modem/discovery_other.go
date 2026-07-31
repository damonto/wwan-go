//go:build !linux

package modem

import "context"

func Discover(context.Context) ([]Device, error) {
	return nil, ErrNotSupported
}

func WatchDevices(context.Context) (<-chan Result[DeviceEvent], error) {
	return nil, ErrNotSupported
}
