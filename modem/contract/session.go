package contract

import (
	"context"
	"slices"
	"time"
)

// Session is the lifecycle contract implemented by a protocol data session.
type Session interface {
	Info() BearerInfo
	Stats(context.Context) (BearerStats, error)
	Watch(context.Context) (<-chan Result[BearerEvent], error)
	Disconnect(context.Context) error
	Close() error
}

// PollStream emits an initial value and then polls until the context is done
// or a query returns an error.
func PollStream[T any](ctx context.Context, interval time.Duration, query func(context.Context) (T, error)) <-chan Result[T] {
	out := make(chan Result[T], 1)
	go func() {
		defer close(out)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			value, err := query(ctx)
			if ctx.Err() != nil {
				return
			}
			if !SendStreamResult(ctx, out, Result[T]{Value: value, Err: err}) || err != nil {
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// SendStreamResult sends one stream result unless the context has ended.
func SendStreamResult[T any](ctx context.Context, out chan<- Result[T], result Result[T]) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case out <- result:
		return true
	case <-ctx.Done():
		return false
	}
}

// CloneNetworkConfig returns an independent copy of the slice-backed fields.
func CloneNetworkConfig(config NetworkConfig) NetworkConfig {
	config.Addresses = slices.Clone(config.Addresses)
	config.Gateways = slices.Clone(config.Gateways)
	config.DNS = slices.Clone(config.DNS)
	return config
}
