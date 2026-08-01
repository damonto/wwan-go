package qmi

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

type directPool struct {
	mu      sync.Mutex
	entries map[string]*directEntry
}

type directEntry struct {
	core    *transportCore
	opening chan struct{}
	closed  chan struct{}
	refs    int
	closing bool
}

var directTransports directPool

func (p *directPool) acquire(ctx context.Context, device string, dialer Dialer) (*Transport, error) {
	key := directDeviceKey(device)
	for {
		p.mu.Lock()
		if entry := p.entries[key]; entry != nil {
			if entry.closing {
				closed := entry.closed
				p.mu.Unlock()
				select {
				case <-closed:
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			if entry.core != nil {
				if directCoreAvailable(entry.core) {
					entry.refs++
					transport := p.lease(key, entry.core)
					p.mu.Unlock()
					return transport, nil
				}

				entry.closing = true
				p.mu.Unlock()
				_ = p.closeEntry(key, entry)
				continue
			}
			opening := entry.opening
			p.mu.Unlock()
			select {
			case <-opening:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if p.entries == nil {
			p.entries = make(map[string]*directEntry)
		}
		entry := &directEntry{
			opening: make(chan struct{}),
			closed:  make(chan struct{}),
		}
		p.entries[key] = entry
		p.mu.Unlock()

		conn, err := dialer.Dial(ctx)
		p.mu.Lock()
		if err != nil {
			delete(p.entries, key)
			close(entry.opening)
			close(entry.closed)
			p.mu.Unlock()
			return nil, err
		}
		entry.core = &transportCore{
			conn:          conn,
			pending:       make(map[messageKey]chan responseResult),
			pendingOwners: make(map[messageKey]*transportLease),
			subs:          make(map[uint64]*subscription),
		}
		entry.refs = 1
		close(entry.opening)
		transport := p.lease(key, entry.core)
		p.mu.Unlock()
		return transport, nil
	}
}

func (p *directPool) lease(key string, core *transportCore) *Transport {
	transport := &Transport{transportCore: core, shared: true, lease: new(transportLease)}
	transport.release = func() error { return p.release(key, core) }
	return transport
}

func (p *directPool) release(key string, core *transportCore) error {
	p.mu.Lock()
	entry := p.entries[key]
	if entry == nil || entry.core != core || entry.closing {
		p.mu.Unlock()
		return nil
	}
	entry.refs--
	if entry.refs > 0 {
		p.mu.Unlock()
		return nil
	}
	entry.closing = true
	p.mu.Unlock()
	return p.closeEntry(key, entry)
}

func (p *directPool) closeEntry(key string, entry *directEntry) error {
	transport := &Transport{transportCore: entry.core}
	err := transport.closeCore()

	p.mu.Lock()
	if p.entries[key] == entry {
		delete(p.entries, key)
		close(entry.closed)
	}
	p.mu.Unlock()
	return err
}

func directCoreAvailable(core *transportCore) bool {
	core.mu.Lock()
	defer core.mu.Unlock()
	return !core.closed && core.terminalErr == nil
}

func directDeviceKey(device string) string {
	device = filepath.Clean(strings.TrimSpace(device))
	resolved, err := filepath.EvalSymlinks(device)
	if err == nil {
		return resolved
	}
	return device
}
