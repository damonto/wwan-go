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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return errReaderClosed
	}

	clientID, err := r.allocateServiceClientID(ctx, qualcomm.ServiceUIM)
	if err != nil {
		return err
	}
	r.clientID = clientID
	return nil
}

func (r *Reader) allocateServiceClientID(ctx context.Context, service qualcomm.ServiceType) (uint8, error) {
	resp, err := r.sendRequest(ctx, qualcomm.ServiceControl, 0, qualcomm.MessageAllocateClientID, tlv.TLVs{
		tlv.Uint(0x01, service),
	}, DefaultRequestTimeout)
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
	resp, err := r.sendRequest(ctx, qualcomm.ServiceControl, 0, qualcomm.MessageReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	}, DefaultRequestTimeout)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) request(
	ctx context.Context,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
) (qualcomm.Response, error) {
	return r.requestWithTimeout(ctx, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestWithTimeout(
	ctx context.Context,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qualcomm.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qualcomm.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, qualcomm.ServiceUIM, r.clientID, id, tlvs, timeout)
}

func (r *Reader) requestService(
	ctx context.Context,
	service qualcomm.ServiceType,
	clientID uint8,
	id qualcomm.MessageID,
	tlvs tlv.TLVs,
) (qualcomm.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qualcomm.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

// sendRequest assumes r.mu is held and r.transport is live.
func (r *Reader) sendRequest(
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
