package qcom

import (
	"context"
	"errors"
	"fmt"
)

func (c *Client) SendEnvelope(ctx context.Context, envelope []byte) (EnvelopeResponse, error) {
	return c.sendCATEnvelope(ctx, envelope, envelopeCommandSMSPP)
}

func (c *Client) catClient(ctx context.Context) (ServiceType, uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return 0, 0, errClientClosed
	}

	if c.catService != 0 {
		clientID, err := c.serviceClientIDLocked(ctx, c.catService)
		return c.catService, clientID, err
	}
	if service, ok := boundQMIService(c.transport); ok {
		if _, managed := c.transport.(transportClientIDProvider); !managed {
			return 0, 0, fmt.Errorf("running QMI CAT envelope: transport is bound to service 0x%02X and cannot switch to CAT/CAT2", service)
		}
	}
	if c.catService == 0 {
		service, err := c.catServiceType(ctx)
		if err != nil {
			return 0, 0, err
		}
		c.catService = service
	}

	clientID, err := c.serviceClientIDLocked(ctx, c.catService)
	if err != nil {
		return 0, 0, err
	}
	return c.catService, clientID, nil
}

func (c *Client) releaseCATClient(ctx context.Context, service ServiceType, clientID uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return nil
	}
	if c.catService != service || c.clientIDs[service] != clientID {
		return nil
	}

	if !transportManagesClientIDs(c.transport) {
		if err := c.releaseServiceClientIDLocked(ctx, service, clientID); err != nil {
			return err
		}
	}
	delete(c.clientIDs, service)
	return nil
}

func (c *Client) catServiceType(ctx context.Context) (ServiceType, error) {
	if transport, ok := c.transport.(transportClientIDProvider); ok {
		if _, err := c.transportClientIDLocked(ctx, transport, ServiceCAT2); err == nil {
			return ServiceCAT2, nil
		}
		if _, err := c.transportClientIDLocked(ctx, transport, ServiceCAT); err == nil {
			return ServiceCAT, nil
		}
		return 0, errors.New("detecting QMI CAT service: CAT2/CAT service is not exposed")
	}

	versions, err := c.serviceVersionsLocked(ctx)
	if err != nil {
		return 0, err
	}
	for _, version := range versions {
		if version.Service == ServiceCAT2 {
			return ServiceCAT2, nil
		}
	}
	for _, version := range versions {
		if version.Service == ServiceCAT {
			return ServiceCAT, nil
		}
	}
	return 0, errors.New("detecting QMI CAT service: CAT2/CAT service is not exposed")
}

// ServiceVersions returns the QMI services advertised by the device.
func (c *Client) ServiceVersions(ctx context.Context) ([]ServiceVersion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return nil, errClientClosed
	}
	return c.serviceVersionsLocked(ctx)
}

func (c *Client) serviceVersionsLocked(ctx context.Context) ([]serviceVersion, error) {
	resp, err := c.sendRequest(ctx, ServiceControl, 0, MessageGetVersionInfo, nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	if err := resultOK(resp); err != nil {
		return nil, err
	}
	var versions ServiceVersionList
	if err := versions.UnmarshalTLVs(resp.TLVs); err != nil {
		return nil, err
	}
	return versions, nil
}
