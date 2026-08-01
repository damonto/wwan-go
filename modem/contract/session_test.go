package contract

import (
	"context"
	"testing"
	"time"
)

func TestPollStreamWaitsAfterQueryCompletion(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "slow query does not trigger catch-up polling"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			const interval = 50 * time.Millisecond
			started := make(chan time.Time, 2)
			releaseFirst := make(chan struct{})
			calls := 0
			stream := PollStream(ctx, interval, func(context.Context) (int, error) {
				calls++
				started <- time.Now()
				if calls == 1 {
					<-releaseFirst
				}
				return calls, nil
			})

			<-started
			time.Sleep(interval + 25*time.Millisecond)
			releasedAt := time.Now()
			close(releaseFirst)
			if result := <-stream; result.Err != nil || result.Value != 1 {
				t.Fatalf("first result = %+v, want value 1", result)
			}

			var secondStarted time.Time
			select {
			case secondStarted = <-started:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for the second query")
			}
			if elapsed := secondStarted.Sub(releasedAt); elapsed < interval {
				t.Fatalf("second query started after %s, want at least %s", elapsed, interval)
			}
		})
	}
}

func TestPollStreamRejectsNonPositiveInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "zero", interval: 0},
		{name: "negative", interval: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("PollStream() panic = nil, want non-nil")
				}
			}()
			PollStream(t.Context(), tt.interval, func(context.Context) (int, error) {
				return 0, nil
			})
		})
	}
}
