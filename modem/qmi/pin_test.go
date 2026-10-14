package qmi

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/damonto/wwan-go/modem/contract"
	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestSIMPINStatus(t *testing.T) {
	tests := []struct {
		name    string
		state   qcom.PINState
		lock    contract.SIMLock
		enabled bool
		known   bool
	}{
		{"disabled", qcom.PINStateDisabled, contract.SIMLockNone, false, true},
		{"verified protection remains enabled", qcom.PINStateEnabledVerified, contract.SIMLockNone, true, true},
		{"PIN", qcom.PINStateEnabledNotVerified, contract.SIMLockPIN, true, true},
		{"PUK", qcom.PINStateBlocked, contract.SIMLockPUK, true, true},
		{"permanent", qcom.PINStatePermanentlyBlocked, contract.SIMLockBlocked, true, true},
		{"unknown", qcom.PINStateNotInitialized, contract.SIMLockUnknown, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := qcom.CardStatus{IndexGWPrimary: 0x0101, Cards: []qcom.Card{
				{State: qcom.CardStatePresent, Applications: []qcom.CardApplication{{Type: qcom.ApplicationTypeUSIM, PIN1State: qcom.PINStateDisabled}}},
				{State: qcom.CardStatePresent, Applications: []qcom.CardApplication{
					{Type: qcom.ApplicationTypeISIM},
					{Type: qcom.ApplicationTypeUSIM, PIN1State: tt.state, PIN1Retries: 2, PUK1Retries: 9},
				}},
			}}
			got := simPINStatus(status)
			want := contract.SIMPINStatus{Supported: true, Enabled: tt.enabled, EnabledKnown: tt.known,
				Lock: tt.lock, PINRetries: 2, PUKRetries: 9, PINRetriesKnown: tt.known, PUKRetriesKnown: tt.known}
			if got != want {
				t.Fatalf("status = %+v, want %+v", got, want)
			}
		})
	}
}

func TestUPINAndInvalidApplication(t *testing.T) {
	tests := []struct {
		name      string
		index     uint16
		upin      bool
		want      contract.SIMLock
		supported bool
	}{
		{"UPIN replaces PIN1", 0, true, contract.SIMLockPUK, true},
		{"invalid card", 0xffff, false, contract.SIMLockUnknown, false},
		{"invalid app", 1, false, contract.SIMLockUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := qcom.CardApplication{Type: qcom.ApplicationTypeUSIM, PIN1State: qcom.PINStateDisabled}
			if tt.upin {
				app.UPINReplacesPIN1 = 1
			}
			got := simPINStatus(qcom.CardStatus{IndexGWPrimary: tt.index, Cards: []qcom.Card{{
				State: qcom.CardStatePresent, UPINState: qcom.PINStateBlocked, UPUKRetries: 8,
				Applications: []qcom.CardApplication{app},
			}}})
			if got.Lock != tt.want || got.Supported != tt.supported {
				t.Fatalf("status = %+v", got)
			}
			if tt.upin && (!got.PUKRetriesKnown || got.PUKRetries != 8) {
				t.Fatalf("UPIN retries = %+v", got)
			}
		})
	}
}

type pinTestTransport struct {
	simInfoTestTransport
	writes []qcom.MessageID
	values []byte
	err    error
	upin   bool
}

func (p *pinTestTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	if req.MessageID == qcom.MessageGetCardStatus {
		value := encodeSIMCardStatus(qcom.ApplicationStateReady)
		// The final seven bytes are UPIN replacement, PIN1 state/retries and PIN2 state/retries.
		if p.upin {
			value[len(value)-7] = 1
		}
		return simInfoTestResponse(req, tlv.Bytes(0x10, value)), nil
	}
	p.writes = append(p.writes, req.MessageID)
	p.values, _ = tlv.Value(req.TLVs, 0x02)
	if p.err != nil {
		return qcom.Response{}, p.err
	}
	resp := simInfoTestErrorResponse(req, qcom.QMIErrorIncorrectPIN)
	resp.TLVs = append(resp.TLVs, tlv.Bytes(0x10, []byte{1, 8}))
	return resp, nil
}

func TestPINCommandsKeepRetryResultAndNeverReplay(t *testing.T) {
	tests := []struct {
		name    string
		run     func(*Backend) error
		message qcom.MessageID
		timeout bool
		upin    bool
	}{
		{"verify", func(b *Backend) error { return b.SendPIN(context.Background(), "0123") }, qcom.MessageVerifyPIN, false, false},
		{"unblock", func(b *Backend) error { return b.SendPUK(context.Background(), "01234567", "0123") }, qcom.MessageUnblockPIN, false, false},
		{"enable", func(b *Backend) error { return b.EnablePIN(context.Background(), "0123", true) }, qcom.MessageSetPINProtection, false, false},
		{"disable", func(b *Backend) error { return b.EnablePIN(context.Background(), "0123", false) }, qcom.MessageSetPINProtection, false, false},
		{"change UPIN", func(b *Backend) error { return b.ChangePIN(context.Background(), "0123", "4567") }, qcom.MessageChangePIN, false, true},
		{"timeout", func(b *Backend) error { return b.SendPIN(context.Background(), "0123") }, qcom.MessageVerifyPIN, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pinTestTransport{upin: tt.upin}
			if tt.timeout {
				transport.err = context.DeadlineExceeded
			}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = tt.run(New(client, "/dev/test"))
			var operation *contract.PINOperationError
			if !errors.As(err, &operation) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(transport.writes, []qcom.MessageID{tt.message}) {
				t.Fatalf("writes = %v", transport.writes)
			}
			id := qcom.PINIDPIN1
			if tt.upin {
				id = qcom.PINIDUniversal
			}
			if len(transport.values) == 0 || transport.values[0] != byte(id) {
				t.Fatalf("PIN ID = %v", transport.values)
			}
			if tt.timeout {
				if !errors.Is(err, context.DeadlineExceeded) || operation.Status.PINRetriesKnown {
					t.Fatalf("timeout result = %+v", operation)
				}
			} else if !errors.Is(err, contract.ErrIncorrectPIN) || !operation.Status.PINRetriesKnown || operation.Status.PINRetries != 1 || operation.Status.PUKRetries != 8 {
				t.Fatalf("retry result = %+v", operation)
			}
		})
	}
}

func TestAbsentSIMWithoutProvisioningSession(t *testing.T) {
	status := qcom.CardStatus{IndexGWPrimary: 0xffff, Cards: []qcom.Card{{State: qcom.CardStateAbsent}}}
	if got := simStateFromCardStatus(status); got != SIMStateAbsent {
		t.Fatalf("absent SIM = %v", got)
	}
	status.Cards[0].State = qcom.CardStatePresent
	if got := simStateFromCardStatus(status); got != SIMStateUnknown {
		t.Fatalf("unprovisioned SIM = %v", got)
	}
}
