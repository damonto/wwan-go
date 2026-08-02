package qmi

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRMNetLinkCloseContext(t *testing.T) {
	errDelete := errors.New("delete rejected")
	tests := []struct {
		name     string
		closeErr error
		wantErr  bool
	}{
		{name: "active bounded cleanup context"},
		{name: "preserves cleanup error", closeErr: errDelete, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := &rmnetLink{
				Name: "qmap1.0",
				close: func(ctx context.Context) error {
					if err := ctx.Err(); err != nil {
						t.Errorf("cleanup context error = %v", err)
					}
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > rmnetOperationTimeout {
						t.Errorf("cleanup deadline = %v, want within %v", deadline, rmnetOperationTimeout)
					}
					return tt.closeErr
				},
			}
			err := link.Close()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Close() error = %v, want error %v", err, tt.wantErr)
			}
			if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
				t.Errorf("Close() error = %v, want %v", err, tt.closeErr)
			}
		})
	}
}
