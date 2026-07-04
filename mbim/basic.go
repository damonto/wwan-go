package mbim

import (
	"context"
	"fmt"
	"slices"
)

func (r *Reader) SubscriberReadyStatus(ctx context.Context) (SubscriberReadyStatusResponse, error) {
	request := SubscriberReadyStatusRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return SubscriberReadyStatusResponse{}, fmt.Errorf("reading MBIM subscriber ready status: %w", err)
	}
	resp := *request.Response
	resp.TelephoneNumbers = slices.Clone(resp.TelephoneNumbers)
	return resp, nil
}
