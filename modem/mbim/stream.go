package mbim

import (
	"context"
	"time"

	"github.com/damonto/wwan-go/modem/contract"
)

const watchPollInterval = 2 * time.Second

func pollStream[T any](ctx context.Context, query func(context.Context) (T, error)) <-chan Result[T] {
	return contract.PollStream(ctx, watchPollInterval, query)
}

func knownSignal(db float64) SignalValue { return SignalValue{DB: db, Known: true} }
