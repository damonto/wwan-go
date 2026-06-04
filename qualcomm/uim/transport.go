package uim

import (
	"context"
	"errors"
	"time"

	"github.com/damonto/uicc-go/qualcomm"
	"github.com/damonto/uicc-go/qualcomm/tlv"
)

var errReaderClosed = errors.New("QMI UIM client is closed")

func (r *Reader) allocateClientID(ctx context.Context) error {
	clientID, err := r.allocateServiceClientID(ctx, qualcomm.QMIServiceUIM)
	if err != nil {
		return err
	}
	r.clientID = clientID
	return nil
}

func (r *Reader) releaseClientID(ctx context.Context) error {
	return r.releaseServiceClientID(ctx, qualcomm.QMIServiceUIM, r.clientID)
}

func (r *Reader) allocateServiceClientID(ctx context.Context, service qualcomm.ServiceType) (uint8, error) {
	resp, err := r.request(ctx, qualcomm.QMIServiceControl, 0, qualcomm.QMICtlCmdAllocateClientID, tlv.TLVs{
		tlv.Uint(0x01, service),
	})
	if err != nil {
		return 0, err
	}
	if err := resultOK(resp); err != nil {
		return 0, err
	}

	value, ok := tlv.Value(resp.TLVs, 0x01)
	if !ok || len(value) < 2 {
		return 0, errors.New("allocating QMI client ID: allocated client TLV missing")
	}
	return value[1], nil
}

func (r *Reader) allocateServiceClientIDLocked(ctx context.Context, service qualcomm.ServiceType) (uint8, error) {
	resp, err := r.requestLocked(ctx, qualcomm.QMIServiceControl, 0, qualcomm.QMICtlCmdAllocateClientID, tlv.TLVs{
		tlv.Uint(0x01, service),
	})
	if err != nil {
		return 0, err
	}
	if err := resultOK(resp); err != nil {
		return 0, err
	}

	value, ok := tlv.Value(resp.TLVs, 0x01)
	if !ok || len(value) < 2 {
		return 0, errors.New("allocating QMI client ID: allocated client TLV missing")
	}
	return value[1], nil
}

func (r *Reader) releaseServiceClientID(ctx context.Context, service qualcomm.ServiceType, clientID uint8) error {
	resp, err := r.request(ctx, qualcomm.QMIServiceControl, 0, qualcomm.QMICtlCmdReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	})
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) releaseServiceClientIDLocked(ctx context.Context, service qualcomm.ServiceType, clientID uint8) error {
	resp, err := r.requestLocked(ctx, qualcomm.QMIServiceControl, 0, qualcomm.QMICtlCmdReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	})
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) request(
	ctx context.Context,
	service qualcomm.ServiceType,
	clientID uint8,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
) (qualcomm.Response, error) {
	return r.requestWithTimeout(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestWithTimeout(
	ctx context.Context,
	service qualcomm.ServiceType,
	clientID uint8,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qualcomm.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qualcomm.Response{}, errReaderClosed
	}
	return r.requestLockedWithTimeout(ctx, service, clientID, id, tlvs, timeout)
}

func (r *Reader) requestLocked(
	ctx context.Context,
	service qualcomm.ServiceType,
	clientID uint8,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
) (qualcomm.Response, error) {
	return r.requestLockedWithTimeout(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestLockedWithTimeout(
	ctx context.Context,
	service qualcomm.ServiceType,
	clientID uint8,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qualcomm.Response, error) {
	return r.transport.Do(ctx, qualcomm.Request{
		Service:       service,
		ClientID:      clientID,
		TransactionID: uint16(r.txn.Add(1)),
		MessageID:     id,
		Timeout:       timeout,
		TLVs:          tlvs,
	})
}
