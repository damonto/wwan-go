package uim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/qualcomm"
	"github.com/damonto/uicc-go/qualcomm/tlv"
)

func (r *Reader) SendEnvelope(ctx context.Context, envelope []byte) (EnvelopeResponse, error) {
	if len(envelope) > 0xFFFF {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: envelope length %d exceeds QMI CAT limit", len(envelope))
	}

	service, clientID, err := r.catClient(ctx)
	if err != nil {
		return EnvelopeResponse{}, err
	}

	value := binary.LittleEndian.AppendUint16(nil, envelopeCommandSMSPP)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(envelope)))
	value = append(value, envelope...)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if service == qualcomm.ServiceCAT2 {
		tlvs = append(tlvs, tlv.Uint(0x10, r.slot))
	}
	resp, err := r.requestService(ctx, service, clientID, qualcomm.MessageSendEnvelope, tlvs)
	if err != nil {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: %w", err)
	}

	result, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return EnvelopeResponse{SW1: 0x90, SW2: 0x00}, nil
	}
	if len(result) < 3 {
		return EnvelopeResponse{}, errors.New("running QMI CAT envelope: raw response TLV is truncated")
	}
	length := int(result[2])
	if len(result) < 3+length {
		return EnvelopeResponse{}, errors.New("running QMI CAT envelope: envelope response data is truncated")
	}
	return EnvelopeResponse{
		SW1:  result[0],
		SW2:  result[1],
		Data: slices.Clone(result[3 : 3+length]),
	}, nil
}

func (r *Reader) catClient(ctx context.Context) (qualcomm.ServiceType, uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return 0, 0, errReaderClosed
	}

	if r.catClientID != 0 {
		return r.catService, r.catClientID, nil
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

func (r *Reader) catServiceType(ctx context.Context) (qualcomm.ServiceType, error) {
	versions, err := r.serviceVersions(ctx)
	if err != nil {
		return 0, err
	}
	for _, version := range versions {
		if version.Service == qualcomm.ServiceCAT2 {
			return qualcomm.ServiceCAT2, nil
		}
	}
	for _, version := range versions {
		if version.Service == qualcomm.ServiceCAT {
			return qualcomm.ServiceCAT, nil
		}
	}
	return 0, errors.New("detecting QMI CAT service: CAT2/CAT service is not exposed")
}

func (r *Reader) serviceVersions(ctx context.Context) ([]serviceVersion, error) {
	resp, err := r.sendRequest(ctx, qualcomm.ServiceControl, 0, qualcomm.MessageGetVersionInfo, nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	if err := resultOK(resp); err != nil {
		return nil, err
	}
	return decodeServiceVersions(resp)
}
