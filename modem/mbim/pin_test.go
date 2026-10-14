package mbim

import (
	"context"
	"errors"
	"math"
	"testing"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/contract"
)

func TestSIMPINStatus(t *testing.T) {
	tests := []struct {
		name     string
		kind     mbimproto.PINType
		attempts uint32
		lock     contract.SIMLock
		known    bool
	}{
		{"PIN", mbimproto.PINTypePIN1, 2, contract.SIMLockPIN, true},
		{"PUK", mbimproto.PINTypePUK1, 9, contract.SIMLockPUK, true},
		{"permanent", mbimproto.PINTypePUK1, 0, contract.SIMLockBlocked, true},
		{"unknown PUK count", mbimproto.PINTypePUK1, 0xffffff, contract.SIMLockPUK, false},
		{"unknown PIN count", mbimproto.PINTypePIN1, math.MaxUint32, contract.SIMLockPIN, false},
		{"PIN2", mbimproto.PINTypePIN2, 0, contract.SIMLockOther, false},
		{"network lock", mbimproto.PINTypeNetwork, 0, contract.SIMLockOther, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := simPINStatus(mbimproto.PINInfo{Type: tt.kind, State: mbimproto.PINStateLocked, RemainingAttempts: tt.attempts})
			if got.Lock != tt.lock || (got.PINRetriesKnown || got.PUKRetriesKnown) != tt.known {
				t.Fatalf("status = %+v", got)
			}
			if tt.known && got.PINRetries+got.PUKRetries != tt.attempts {
				t.Fatalf("retries = %+v", got)
			}
		})
	}
}

type pinStatusReader struct {
	pin     mbimproto.PINInfo
	mode    mbimproto.PINMode
	listErr error
	pinErr  error
}

func (r pinStatusReader) PIN(context.Context) (mbimproto.PINInfo, error) { return r.pin, r.pinErr }
func (r pinStatusReader) PINList(context.Context) (mbimproto.PINListInfo, error) {
	return mbimproto.PINListInfo{PIN1: mbimproto.PINDesc{Mode: r.mode}}, r.listErr
}

func TestProtectionIsIndependentOfUnlock(t *testing.T) {
	tests := []struct {
		name           string
		reader         pinStatusReader
		enabled, known bool
		lock           contract.SIMLock
	}{
		{"enabled and verified", pinStatusReader{mode: mbimproto.PINModeEnabled}, true, true, contract.SIMLockNone},
		{"disabled", pinStatusReader{mode: mbimproto.PINModeDisabled}, false, true, contract.SIMLockNone},
		{"unknown mode", pinStatusReader{mode: mbimproto.PINMode(99)}, false, false, contract.SIMLockNone},
		{"list unavailable", pinStatusReader{listErr: errors.New("unavailable")}, false, false, contract.SIMLockNone},
		{"PIN required without descriptor", pinStatusReader{pinErr: mbimproto.StatusPINRequired, mode: mbimproto.PINModeEnabled}, true, true, contract.SIMLockUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readSIMPINStatus(t.Context(), tt.reader)
			if err != nil || got.Enabled != tt.enabled || got.EnabledKnown != tt.known || got.Lock != tt.lock {
				t.Fatalf("status = %+v, %v", got, err)
			}
		})
	}
}

func TestPINFailurePreservesResponse(t *testing.T) {
	tests := []struct {
		name        string
		pin         mbimproto.PINInfo
		err, reason error
	}{
		{"incorrect PIN", mbimproto.PINInfo{Type: mbimproto.PINTypePIN1, State: mbimproto.PINStateLocked, RemainingAttempts: 2}, mbimproto.StatusFailure, contract.ErrIncorrectPIN},
		{"PUK required", mbimproto.PINInfo{Type: mbimproto.PINTypePUK1, State: mbimproto.PINStateLocked, RemainingAttempts: 10}, mbimproto.StatusFailure, contract.ErrPUKRequired},
		{"permanent", mbimproto.PINInfo{Type: mbimproto.PINTypePUK1, State: mbimproto.PINStateLocked}, mbimproto.StatusFailure, contract.ErrPINPermanentlyBlocked},
		{"timeout does not imply wrong PIN", mbimproto.PINInfo{}, context.DeadlineExceeded, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pinOperationError(tt.pin, tt.err)
			var result *contract.PINOperationError
			if !errors.As(err, &result) || !errors.Is(err, tt.reason) {
				t.Fatalf("error = %v", err)
			}
			if tt.pin.Type == mbimproto.PINTypePIN1 && result.Status.PINRetries != 2 {
				t.Fatalf("status = %+v", result.Status)
			}
			if tt.pin.Type == mbimproto.PINTypeNone && (result.Status.PINRetriesKnown || result.Status.PUKRetriesKnown) {
				t.Fatal("missing response reported zero retries")
			}
		})
	}
}
