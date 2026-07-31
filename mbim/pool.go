package mbim

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type directClientPool struct {
	mu      sync.Mutex
	entries map[string]*directClientEntry
}

type directClientEntry struct {
	owner   *Client
	opening chan struct{}
	closed  chan struct{}
	refs    int
	closing bool
}

var directClients directClientPool

func (p *directClientPool) acquire(ctx context.Context, device string, dialer Dialer, slot int) (*Client, error) {
	key := mbimDeviceKey(device)
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
			if entry.owner != nil {
				if directOwnerAvailable(entry.owner) {
					entry.refs++
					client := p.lease(key, entry.owner, slot)
					p.mu.Unlock()
					if err := client.connectLease(ctx); err != nil {
						_ = client.Close()
						return nil, err
					}
					return client, nil
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
			p.entries = make(map[string]*directClientEntry)
		}
		entry := &directClientEntry{
			opening: make(chan struct{}),
			closed:  make(chan struct{}),
		}
		p.entries[key] = entry
		p.mu.Unlock()

		conn, err := dialer.Dial(ctx)
		var owner *Client
		if err == nil {
			owner = &Client{
				conn:               conn,
				slot:               uint32(slot - 1),
				maxControlTransfer: connMaxControlTransfer(conn),
				clientState:        newClientState(),
			}
			err = owner.connect(ctx, "")
		}
		if err != nil && conn != nil {
			_ = conn.Close()
		}

		p.mu.Lock()
		if err != nil {
			delete(p.entries, key)
			close(entry.opening)
			close(entry.closed)
			p.mu.Unlock()
			return nil, err
		}
		entry.owner = owner
		entry.refs = 1
		close(entry.opening)
		client := p.lease(key, owner, slot)
		p.mu.Unlock()
		return client, nil
	}
}

func (p *directClientPool) lease(key string, owner *Client, slot int) *Client {
	client := &Client{
		conn:               owner.conn,
		slot:               uint32(slot - 1),
		mbimExVersion:      owner.mbimExVersion,
		maxControlTransfer: owner.maxControlTransfer,
		clientState:        owner.clientState,
		lease:              new(clientLease),
	}
	client.release = func() error { return p.release(key, owner) }
	return client
}

func (p *directClientPool) release(key string, owner *Client) error {
	p.mu.Lock()
	entry := p.entries[key]
	if entry == nil || entry.owner != owner || entry.closing {
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

func (p *directClientPool) closeEntry(key string, entry *directClientEntry) error {
	err := entry.owner.closeDevice()

	p.mu.Lock()
	if p.entries[key] == entry {
		delete(p.entries, key)
		close(entry.closed)
	}
	p.mu.Unlock()
	return err
}

func directOwnerAvailable(owner *Client) bool {
	owner.ensureState()
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return !owner.closed && !owner.closing && owner.receiverErr == nil
}

func (c *Client) connectLease(ctx context.Context) error {
	if err := c.validateUiccSlotID(); err != nil {
		return fmt.Errorf("connecting MBIM client lease: %w", err)
	}
	if c.usesUiccSlotID() {
		return nil
	}
	return c.ensureSlotActivated(ctx)
}

func mbimDeviceKey(device string) string {
	device = filepath.Clean(strings.TrimSpace(device))
	resolved, err := filepath.EvalSymlinks(device)
	if err == nil {
		return resolved
	}
	return device
}
