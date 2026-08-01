package qrtr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitLookupRetryHonorsContext(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "canceled while waiting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() {
				done <- waitLookupRetry(ctx, time.Minute)
			}()

			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("waitLookupRetry() error = %v, want context canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("waitLookupRetry did not return after cancellation")
			}
		})
	}
}
