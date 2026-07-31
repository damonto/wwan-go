package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

var errClientClosed = errors.New("QCOM client is closed")

// RequestDeadline returns the earlier of the context deadline and the request
// timeout. A non-positive timeout leaves an existing context deadline intact.
func RequestDeadline(ctx context.Context, timeout time.Duration) (time.Time, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		if timeout <= 0 {
			return deadline, true
		}

		timeoutDeadline := time.Now().Add(timeout)
		if deadline.Before(timeoutDeadline) {
			return deadline, true
		}
		return timeoutDeadline, true
	}
	if timeout <= 0 {
		return time.Time{}, false
	}
	return time.Now().Add(timeout), true
}

func (c *Client) withServiceClient(ctx context.Context, service ServiceType, fn func(uint8) error) error {
	clientID, err := c.serviceClientID(ctx, service)
	if err != nil {
		return err
	}

	err = fn(clientID)
	if !errors.Is(err, QMIErrorInvalidClientId) {
		return err
	}

	// A modem reset invalidates allocated CIDs. Forget only the stale CID and
	// retry once; resetting the whole shared QMI endpoint would disrupt peers.
	if !c.forgetServiceClientID(service, clientID) {
		return err
	}
	clientID, allocateErr := c.serviceClientID(ctx, service)
	if allocateErr != nil {
		return errors.Join(err, allocateErr)
	}
	return fn(clientID)
}

func (c *Client) serviceClientID(ctx context.Context, service ServiceType) (uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return 0, errClientClosed
	}
	return c.serviceClientIDLocked(ctx, service)
}

func (c *Client) serviceClientIDLocked(ctx context.Context, service ServiceType) (uint8, error) {
	if transport, ok := c.transport.(transportClientIDProvider); ok {
		return transport.ClientID(ctx, service)
	}

	boundService, serviceBound := boundQMIService(c.transport)
	if serviceBound {
		if boundService != service {
			return 0, fmt.Errorf("QMI transport is bound to service 0x%02X, want 0x%02X", boundService, service)
		}
		return 0, nil
	}

	if clientID := c.clientIDs[service]; clientID != 0 {
		return clientID, nil
	}

	clientID, err := c.allocateServiceClientIDLocked(ctx, service)
	if err != nil {
		return 0, err
	}
	if c.clientIDs == nil {
		c.clientIDs = make(map[ServiceType]uint8)
	}
	c.clientIDs[service] = clientID
	return clientID, nil
}

func (c *Client) forgetServiceClientID(service ServiceType, clientID uint8) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if transportManagesClientIDs(c.transport) {
		return false
	}
	if c.clientIDs[service] == clientID {
		delete(c.clientIDs, service)
	}
	return true
}

func (c *Client) uimClientID(ctx context.Context) (uint8, error) {
	return c.serviceClientID(ctx, ServiceUIM)
}

func (c *Client) allocateServiceClientID(ctx context.Context, service ServiceType) (uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return 0, errClientClosed
	}
	return c.allocateServiceClientIDLocked(ctx, service)
}

// sessionServiceClientID returns a service endpoint suitable for a stateful
// session. QMUX sessions receive a dedicated CID that the session must
// release; QRTR sessions are identified by their service socket instead.
func (c *Client) sessionServiceClientID(ctx context.Context, service ServiceType) (uint8, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return 0, false, errClientClosed
	}
	if transportManagesClientIDs(c.transport) {
		clientID, err := c.serviceClientIDLocked(ctx, service)
		return clientID, false, err
	}
	clientID, err := c.allocateServiceClientIDLocked(ctx, service)
	return clientID, err == nil, err
}

func (c *Client) allocateServiceClientIDLocked(ctx context.Context, service ServiceType) (uint8, error) {
	if service > 0xff {
		return 0, fmt.Errorf("allocating QMI client ID: service 0x%X exceeds QMUX 8-bit limit", service)
	}
	resp, err := c.sendRequest(ctx, ServiceControl, 0, MessageAllocateClientID, tlv.TLVs{
		tlv.Uint(0x01, uint8(service)),
	}, DefaultRequestTimeout)
	if err != nil {
		return 0, err
	}
	if err := resultOK(resp); err != nil {
		return 0, err
	}

	value, ok := tlv.Value(resp.TLVs, 0x01)
	if !ok {
		return 0, errors.New("allocating QMI client ID: allocated client TLV missing")
	}
	if len(value) != 2 {
		return 0, fmt.Errorf("allocating QMI client ID: allocated client TLV length %d, want 2", len(value))
	}
	return value[1], nil
}

func (c *Client) releaseServiceClientID(ctx context.Context, service ServiceType, clientID uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return errClientClosed
	}
	return c.releaseServiceClientIDLocked(ctx, service, clientID)
}

func (c *Client) releaseServiceClientIDLocked(ctx context.Context, service ServiceType, clientID uint8) error {
	if service > 0xff {
		return fmt.Errorf("releasing QMI client ID: service 0x%X exceeds QMUX 8-bit limit", service)
	}
	resp, err := c.sendRequest(ctx, ServiceControl, 0, MessageReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	}, DefaultRequestTimeout)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *Client) request(
	ctx context.Context,
	id MessageID,
	tlvs tlv.TLVs,
) (Response, error) {
	return c.requestWithTimeout(ctx, id, tlvs, DefaultRequestTimeout)
}

func (c *Client) requestWithTimeout(
	ctx context.Context,
	id MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return Response{}, errClientClosed
	}
	clientID, err := c.serviceClientIDLocked(ctx, ServiceUIM)
	if err != nil {
		return Response{}, err
	}
	return c.sendRequest(ctx, ServiceUIM, clientID, id, tlvs, timeout)
}

func (c *Client) requestService(
	ctx context.Context,
	service ServiceType,
	clientID uint8,
	id MessageID,
	tlvs tlv.TLVs,
) (Response, error) {
	return c.requestServiceWithTimeout(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

func (c *Client) requestServiceWithTimeout(
	ctx context.Context,
	service ServiceType,
	clientID uint8,
	id MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return Response{}, errClientClosed
	}
	return c.sendRequest(ctx, service, clientID, id, tlvs, timeout)
}

// sendRequest assumes c.mu is held and c.transport is live.
func (c *Client) sendRequest(
	ctx context.Context,
	service ServiceType,
	clientID uint8,
	id MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (Response, error) {
	return c.transport.Do(ctx, Request{
		Service:       service,
		ClientID:      clientID,
		TransactionID: c.nextTransactionID(service),
		MessageID:     id,
		Timeout:       timeout,
		TLVs:          tlvs,
	})
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
