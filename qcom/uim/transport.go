package uim

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

var errReaderClosed = errors.New("QMI UIM client is closed")

func (r *Reader) withServiceClient(ctx context.Context, service qcom.ServiceType, fn func(uint8) error) error {
	r.mu.Lock()
	transport := r.transport
	closed := r.closed || transport == nil
	r.mu.Unlock()
	if closed {
		return errReaderClosed
	}

	if boundService, ok := boundQMIService(transport); ok {
		if boundService != service {
			return fmt.Errorf("QMI transport is bound to service 0x%02X, want 0x%02X", boundService, service)
		}
		return fn(0)
	}

	clientID, err := r.allocateServiceClientID(ctx, service)
	if err != nil {
		return err
	}
	err = fn(clientID)
	releaseErr := r.releaseServiceClientID(ctx, service, clientID)
	if err != nil {
		return errors.Join(err, releaseErr)
	}
	return releaseErr
}

func (r *Reader) allocateClientID(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return errReaderClosed
	}

	clientID, err := r.allocateServiceClientID(ctx, qcom.ServiceUIM)
	if err != nil {
		return err
	}
	r.clientID = clientID
	return nil
}

func (r *Reader) allocateServiceClientID(ctx context.Context, service qcom.ServiceType) (uint8, error) {
	resp, err := r.sendRequest(ctx, qcom.ServiceControl, 0, qcom.MessageAllocateClientID, tlv.TLVs{
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

func (r *Reader) releaseServiceClientID(ctx context.Context, service qcom.ServiceType, clientID uint8) error {
	resp, err := r.sendRequest(ctx, qcom.ServiceControl, 0, qcom.MessageReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	}, DefaultRequestTimeout)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) request(
	ctx context.Context,
	id qcom.MessageID,
	tlvs tlv.TLVs,
) (qcom.Response, error) {
	return r.requestWithTimeout(ctx, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestWithTimeout(
	ctx context.Context,
	id qcom.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qcom.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qcom.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, qcom.ServiceUIM, r.clientID, id, tlvs, timeout)
}

func (r *Reader) requestService(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	id qcom.MessageID,
	tlvs tlv.TLVs,
) (qcom.Response, error) {
	return r.requestServiceWithTimeout(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestServiceWithTimeout(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	id qcom.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qcom.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qcom.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, service, clientID, id, tlvs, timeout)
}

// sendRequest assumes r.mu is held and r.transport is live.
func (r *Reader) sendRequest(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	id qcom.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qcom.Response, error) {
	return r.transport.Do(ctx, qcom.Request{
		Service:       service,
		ClientID:      clientID,
		TransactionID: r.nextTransactionID(service),
		MessageID:     id,
		Timeout:       timeout,
		TLVs:          tlvs,
	})
}
