package mbim

import (
	"context"
	"fmt"
	"slices"
)

func (r *Reader) RadioState(ctx context.Context) (RadioStateInfo, error) {
	request := RadioStateRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return RadioStateInfo{}, fmt.Errorf("reading MBIM radio state: %w", err)
	}
	return *request.Response, nil
}

func (r *Reader) SetRadioState(ctx context.Context, state RadioSwitchState) (RadioStateInfo, error) {
	request := RadioStateSetRequest{
		TransactionID: r.nextTransactionID(),
		State:         state,
	}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return RadioStateInfo{}, fmt.Errorf("setting MBIM radio state: %w", err)
	}
	return *request.Response, nil
}

func (r *Reader) SubscriberReadyStatus(ctx context.Context) (SubscriberReadyStatusResponse, error) {
	request := SubscriberReadyStatusRequest{
		TransactionID: r.nextTransactionID(),
		MBIMExVersion: r.mbimExVersion,
		SlotID:        r.subscriberReadySlotID(),
	}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return SubscriberReadyStatusResponse{}, fmt.Errorf("reading MBIM subscriber ready status: %w", err)
	}
	resp := *request.Response
	resp.TelephoneNumbers = slices.Clone(resp.TelephoneNumbers)
	return resp, nil
}
