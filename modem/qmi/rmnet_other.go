//go:build !linux

package qmi

import (
	"context"
	"fmt"
)

func createRMNetLink(context.Context, string, uint32) (*rmnetLink, error) {
	return nil, fmt.Errorf("creating rmnet link: %w", ErrNotSupported)
}
