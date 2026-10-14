package qmi

import (
	"context"
	"errors"

	"github.com/damonto/wwan-go/modem/contract"
	"github.com/damonto/wwan-go/qcom"
)

// primarySIMApplication uses the provisioning session, not array order.
func primarySIMApplication(status qcom.CardStatus) (qcom.Card, qcom.CardApplication, bool) {
	cardIndex, appIndex := int(status.IndexGWPrimary>>8), int(status.IndexGWPrimary&0xff)
	if cardIndex >= len(status.Cards) {
		return qcom.Card{}, qcom.CardApplication{}, false
	}
	card := status.Cards[cardIndex]
	if card.State != qcom.CardStatePresent || appIndex >= len(card.Applications) {
		return card, qcom.CardApplication{}, false
	}
	app := card.Applications[appIndex]
	return card, app, app.Type == qcom.ApplicationTypeSIM || app.Type == qcom.ApplicationTypeUSIM
}

func simPINStatus(status qcom.CardStatus) contract.SIMPINStatus {
	result := contract.SIMPINStatus{Lock: contract.SIMLockUnknown}
	card, app, ok := primarySIMApplication(status)
	if !ok {
		return result
	}
	result.Supported = true
	state, pin, puk := app.PIN1State, app.PIN1Retries, app.PUK1Retries
	if app.UPINReplacesPIN1 != 0 {
		state, pin, puk = card.UPINState, card.UPINRetries, card.UPUKRetries
	}
	result.PINRetries, result.PUKRetries = uint32(pin), uint32(puk)
	switch state {
	case qcom.PINStateEnabledNotVerified:
		result.Lock = contract.SIMLockPIN
	case qcom.PINStateEnabledVerified, qcom.PINStateDisabled:
		result.Lock = contract.SIMLockNone
	case qcom.PINStateBlocked:
		result.Lock = contract.SIMLockPUK
	case qcom.PINStatePermanentlyBlocked:
		result.Lock = contract.SIMLockBlocked
	default:
		// Application state can identify a lock before the PIN descriptor is ready.
		switch app.State {
		case qcom.ApplicationStatePIN1OrUPINRequired:
			result.Lock = contract.SIMLockPIN
		case qcom.ApplicationStatePUK1OrUPINRequired:
			result.Lock = contract.SIMLockPUK
		case qcom.ApplicationStatePIN1Blocked:
			result.Lock = contract.SIMLockBlocked
		}
	}
	if state >= qcom.PINStateEnabledNotVerified && state <= qcom.PINStatePermanentlyBlocked {
		result.EnabledKnown, result.Enabled = true, state != qcom.PINStateDisabled
		result.PINRetriesKnown, result.PUKRetriesKnown = true, true
	}
	if (result.Lock == contract.SIMLockNone || result.Lock == contract.SIMLockUnknown) && app.State == qcom.ApplicationStateCheckPersonalization {
		result.Lock = contract.SIMLockOther
	}
	return result
}

// SIMPINStatus reads PIN1 (or its replacing UPIN) for the primary GW application.
func (b *Backend) SIMPINStatus(ctx context.Context) (contract.SIMPINStatus, error) {
	status, err := b.client.CardStatus(ctx)
	if isUnsupported(err) {
		return contract.SIMPINStatus{}, contract.ErrNotSupported
	}
	if err != nil {
		return contract.SIMPINStatus{}, err
	}
	return simPINStatus(status), nil
}

func (b *Backend) simPINID(ctx context.Context) (qcom.PINID, error) {
	status, err := b.client.CardStatus(ctx)
	if err != nil {
		return 0, err
	}
	_, app, ok := primarySIMApplication(status)
	if !ok {
		return 0, contract.ErrNotSupported
	}
	if app.UPINReplacesPIN1 != 0 {
		return qcom.PINIDUniversal, nil
	}
	return qcom.PINIDPIN1, nil
}

func pinOperationError(result qcom.PINOperationResult, err error) error {
	if err == nil {
		return nil
	}
	status := contract.SIMPINStatus{Supported: true, Lock: contract.SIMLockUnknown,
		PINRetries: uint32(result.VerifyRetries), PUKRetries: uint32(result.UnblockRetries),
		PINRetriesKnown: result.RetriesKnown, PUKRetriesKnown: result.RetriesKnown}
	var reason error
	switch {
	case errors.Is(err, qcom.QMIErrorPINAlwaysBlocked):
		reason, status.Lock = contract.ErrPINPermanentlyBlocked, contract.SIMLockBlocked
	case errors.Is(err, qcom.QMIErrorPINBlocked), errors.Is(err, qcom.QMIErrorPUKRestriction):
		reason, status.Lock = contract.ErrPUKRequired, contract.SIMLockPUK
	case errors.Is(err, qcom.QMIErrorIncorrectPIN):
		reason = contract.ErrIncorrectPIN
	case errors.Is(err, qcom.QMIErrorPINRestriction):
		reason, status.Lock = contract.ErrPINRequired, contract.SIMLockPIN
	case isUnsupported(err):
		reason = contract.ErrNotSupported
	}
	return &contract.PINOperationError{Status: status, Reason: reason, Cause: err}
}
