package qmi

import (
	"context"
	"fmt"
	"time"
)

const (
	rmnetMuxIDMin = 1
	rmnetMuxIDMax = 0xfe
)

const (
	rmnetFlagIngressDeaggregation = 1 << iota
	rmnetFlagIngressMAPCommands
	rmnetFlagIngressMAPChecksumV4
	rmnetFlagEgressMAPChecksumV4
	rmnetFlagIngressMAPChecksumV5
	rmnetFlagEgressMAPChecksumV5

	rmnetFlagMask = rmnetFlagIngressDeaggregation |
		rmnetFlagIngressMAPChecksumV4 |
		rmnetFlagEgressMAPChecksumV4 |
		rmnetFlagIngressMAPChecksumV5 |
		rmnetFlagEgressMAPChecksumV5

	rmnetOperationTimeout = 5 * time.Second
)

type rmnetLink struct {
	Name  string
	MuxID uint8
	Index int
	close func(context.Context) error
}

func (l *rmnetLink) Close() error {
	if l == nil || l.close == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), rmnetOperationTimeout)
	defer cancel()
	if err := l.close(ctx); err != nil {
		return fmt.Errorf("deleting rmnet link %s: %w", l.Name, err)
	}
	return nil
}
