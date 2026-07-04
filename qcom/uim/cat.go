package uim

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/uicc-go/qcom"
)

func (r *Reader) SendEnvelope(ctx context.Context, envelope []byte) (EnvelopeResponse, error) {
	return r.sendCATEnvelope(ctx, envelope, envelopeCommandSMSPP)
}

func (r *Reader) catClient(ctx context.Context) (qcom.ServiceType, uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return 0, 0, errReaderClosed
	}

	if r.catClientID != 0 {
		return r.catService, r.catClientID, nil
	}
	if service, ok := boundQMIService(r.transport); ok {
		return 0, 0, fmt.Errorf("running QMI CAT envelope: transport is bound to service 0x%02X and cannot switch to CAT/CAT2", service)
	}
	if r.catService == 0 {
		service, err := r.catServiceType(ctx)
		if err != nil {
			return 0, 0, err
		}
		r.catService = service
	}

	clientID, err := r.allocateServiceClientID(ctx, r.catService)
	if err != nil {
		return 0, 0, err
	}
	r.catClientID = clientID
	return r.catService, r.catClientID, nil
}

func (r *Reader) releaseCATClient(ctx context.Context, service qcom.ServiceType, clientID uint8) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return nil
	}
	if r.catService != service || r.catClientID != clientID {
		return nil
	}

	if _, serviceBound := boundQMIService(r.transport); !serviceBound {
		if err := r.releaseServiceClientID(ctx, service, clientID); err != nil {
			return err
		}
	}
	r.catClientID = 0
	return nil
}

func (r *Reader) catServiceType(ctx context.Context) (qcom.ServiceType, error) {
	versions, err := r.serviceVersions(ctx)
	if err != nil {
		return 0, err
	}
	for _, version := range versions {
		if version.Service == qcom.ServiceCAT2 {
			return qcom.ServiceCAT2, nil
		}
	}
	for _, version := range versions {
		if version.Service == qcom.ServiceCAT {
			return qcom.ServiceCAT, nil
		}
	}
	return 0, errors.New("detecting QMI CAT service: CAT2/CAT service is not exposed")
}

func (r *Reader) serviceVersions(ctx context.Context) ([]serviceVersion, error) {
	resp, err := r.sendRequest(ctx, qcom.ServiceControl, 0, qcom.MessageGetVersionInfo, nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	if err := resultOK(resp); err != nil {
		return nil, err
	}
	return decodeServiceVersions(resp)
}
