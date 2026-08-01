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
	DefaultRequestTimeout = 30 * time.Second
	defaultCloseTimeout   = 5 * time.Second
)

// Client owns a QMI transport and lazily allocated service client IDs.
type Client struct {
	mu         sync.Mutex
	watchMu    sync.Mutex
	pdcMu      sync.Mutex
	locMu      sync.Mutex
	pbmMu      sync.Mutex
	wmsMu      sync.Mutex
	transport  Transport
	slot       uint8
	catService ServiceType
	clientIDs  map[ServiceType]uint8
	txn        uint16
	ctlTxn     uint8
	pdcToken   uint32
	wmsToken   uint32
	closeOnce  sync.Once
	closed     bool
	closeErr   error

	uimEventRefs              map[uint32]int
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

func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()
	c.closeOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		transport := c.transport
		if transport == nil {
			c.closed = true
			return
		}

		var releaseErr error
		if !transportManagesClientIDs(transport) && transportTerminalError(transport) == nil {
			services := make([]ServiceType, 0, len(c.clientIDs))
			for service := range c.clientIDs {
				services = append(services, service)
			}
			slices.Sort(services)
			for _, service := range services {
				if transportTerminalError(transport) != nil {
					break
				}
				err := c.releaseServiceClientIDLocked(ctx, service, c.clientIDs[service])
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
