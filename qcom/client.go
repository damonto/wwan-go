package qcom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	DefaultRequestTimeout  = 30 * time.Second
	defaultCloseTimeout    = 5 * time.Second
	clientIDRequestTimeout = 10 * time.Second
)

// Client owns a QMI transport and every QMUX client ID allocated through it.
// Closing Client invalidates any stateful session created from it.
type Client struct {
	mu                 contextMutex
	lifecycleMu        sync.Mutex
	watchMu            sync.Mutex
	pdcMu              sync.Mutex
	locMu              sync.Mutex
	pbmMu              sync.Mutex
	wmsMu              sync.Mutex
	transport          Transport
	slot               uint8
	catService         ServiceType
	clientIDs          map[ServiceType]uint8
	allocatedClientIDs map[allocatedClientID]struct{}
	txn                uint16
	ctlTxn             uint8
	pdcToken           uint32
	wmsToken           uint32
	closeOnce          sync.Once
	closing            bool
	closed             bool
	requestStop        context.CancelFunc
	closeErr           error

	uimEventRefs              map[uint32]int
	uimRefreshWatcherActive   bool
	dmsEventRefs              int
	pdsEventRefs              int
	omaEventRefs              int
	locEventMask              LOCEventRegistration
	locEventRefs              map[LOCEventRegistration]int
	voiceIndicationRefs       map[voiceIndicationRegistration]int
	imsaIndicationRefs        map[imsaIndicationRegistration]int
	imsSettingsIndicationRefs map[imsSettingsIndicationRegistration]int
	wmsIndicationRefs         map[wmsIndicationRegistration]int
	dsdIndicationRefs         map[dsdIndicationRegistration]int
	nasIndicationRefs         map[nasIndicationRegistration]int
	pdcIndicationRefs         map[pdcIndicationRegistration]int
	wdsProfileEventRefs       map[WDSProfileID]int
	pbmIndicationRefs         map[PBMEventRegistrationMask]int
}

type allocatedClientID struct {
	service  ServiceType
	clientID uint8
}

// Option configures a Client.
type Option func(*config)

type config struct {
	slot uint8
}

type serviceBoundTransport interface {
	QMIService() ServiceType
}

// transportClientIDProvider is implemented by transports, such as QRTR,
// where the transport endpoint itself identifies the QMI client. Calling
// ClientID may also establish the service endpoint.
type transportClientIDProvider interface {
	ClientID(ctx context.Context, service ServiceType) (uint8, error)
}

type terminalErrorTransport interface {
	TerminalError() error
}

// WithSlot selects the physical UICC slot used by UIM and CAT operations.
func WithSlot(slot uint8) Option {
	return func(c *config) {
		c.slot = slot
	}
}

// NewClient creates a QCOM QMI client. Service client IDs are allocated on
// first use and released by Close.
func NewClient(transport Transport, opts ...Option) (*Client, error) {
	if transport == nil {
		return nil, errors.New("creating QCOM client: transport is nil")
	}

	cfg := config{slot: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.slot < 1 || cfg.slot > 5 {
		return nil, fmt.Errorf("creating QCOM client: slot %d is out of range", cfg.slot)
	}

	return &Client{
		transport: transport,
		slot:      cfg.slot,
	}, nil
}

// Close cancels in-flight work, releases allocated QMUX client IDs, and closes the
// transport. It is safe to call more than once.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.startClose()

		c.mu.Lock()
		defer c.mu.Unlock()

		transport := c.transport
		if transport == nil {
			c.clientIDs = nil
			c.allocatedClientIDs = nil
			c.catService = 0
			c.closed = true
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
		defer cancel()

		var releaseErr error
		if !transportManagesClientIDs(transport) && transportTerminalError(transport) == nil {
			allocated := make(map[allocatedClientID]struct{}, len(c.allocatedClientIDs)+len(c.clientIDs))
			for clientID := range c.allocatedClientIDs {
				allocated[clientID] = struct{}{}
			}
			for service, clientID := range c.clientIDs {
				allocated[allocatedClientID{service: service, clientID: clientID}] = struct{}{}
			}
			clientIDs := make([]allocatedClientID, 0, len(allocated))
			for clientID := range allocated {
				clientIDs = append(clientIDs, clientID)
			}
			slices.SortFunc(clientIDs, func(a, b allocatedClientID) int {
				if a.service != b.service {
					return int(a.service) - int(b.service)
				}
				return int(a.clientID) - int(b.clientID)
			})
			for _, allocated := range clientIDs {
				if transportTerminalError(transport) != nil {
					break
				}
				err := c.releaseServiceClientIDForCloseLocked(ctx, allocated.service, allocated.clientID)
				if err == nil {
					continue
				}
				if transportTerminalError(transport) != nil {
					break
				}
				releaseErr = errors.Join(releaseErr, err)
			}
		}
		c.clientIDs = nil
		c.allocatedClientIDs = nil
		c.catService = 0

		closeErr := transport.Close()
		c.transport = nil
		c.closed = true
		if releaseErr == nil {
			c.closeErr = closeErr
			return
		}
		c.closeErr = errors.Join(releaseErr, closeErr)
	})
	return c.closeErr
}

// startClose rejects new requests before waiting for the request mutex. The
// separate lifecycle lock lets Close cancel a request that currently owns mu.
func (c *Client) startClose() {
	c.lifecycleMu.Lock()
	c.closing = true
	stop := c.requestStop
	c.lifecycleMu.Unlock()

	if stop != nil {
		stop()
	}
}

// isOpenLocked reports whether callers may start transport work. c.mu must be
// held so transport and closed are read consistently with Close.
func (c *Client) isOpenLocked() bool {
	if c.closed || c.transport == nil {
		return false
	}

	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	return !c.closing
}

// beginRequest registers the one request serialized by c.mu so Close can
// cancel ordinary work before waiting for c.mu itself. Ownership transactions,
// such as allocating a CID, finish under their own short timeout so a canceled
// caller cannot orphan a resource that the modem already created.
func (c *Client) beginRequest(ctx context.Context, cancelOnClose bool) (context.Context, func(), error) {
	c.lifecycleMu.Lock()
	if c.closing {
		c.lifecycleMu.Unlock()
		return nil, nil, errClientClosed
	}
	requestCtx := ctx
	var stop context.CancelFunc
	if cancelOnClose {
		requestCtx, stop = context.WithCancel(ctx)
	}
	c.requestStop = stop
	c.lifecycleMu.Unlock()

	finish := func() {
		if stop != nil {
			stop()
		}
		c.lifecycleMu.Lock()
		c.requestStop = nil
		c.lifecycleMu.Unlock()
	}
	return requestCtx, finish, nil
}

func boundQMIService(transport Transport) (ServiceType, bool) {
	bound, ok := transport.(serviceBoundTransport)
	if !ok {
		return 0, false
	}
	return bound.QMIService(), true
}

func transportManagesClientIDs(transport Transport) bool {
	if _, ok := transport.(transportClientIDProvider); ok {
		return true
	}
	_, ok := boundQMIService(transport)
	return ok
}

func transportTerminalError(transport Transport) error {
	reporter, ok := transport.(terminalErrorTransport)
	if !ok {
		return nil
	}
	return reporter.TerminalError()
}

func (c *Client) nextTransactionID(service ServiceType) uint16 {
	if service == ServiceControl {
		c.ctlTxn++
		if c.ctlTxn == 0 {
			c.ctlTxn++
		}
		return uint16(c.ctlTxn)
	}

	c.txn++
	if c.txn == 0 {
		c.txn++
	}
	return c.txn
}
