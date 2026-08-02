package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// VoiceUSSDNoWaitResult is the asynchronous result of Originate USSD No Wait.
// Err reports malformed indication data; modem and network failures remain in
// ProtocolError and FailureCause so callers can inspect their numeric values.
type VoiceUSSDNoWaitResult struct {
	VoiceUSSDResult
	ProtocolError      QMIError
	ProtocolErrorKnown bool
	Alpha              VoiceAlphaIdentifier
	AlphaKnown         bool
	FailureCauseText   []uint16
	Err                error
}

// VoiceOriginateUSSDNoWait starts a 3GPP USSD operation without holding the
// request open. The returned channel delivers at most one asynchronous result
// and then closes. The indication subscription is installed before the request
// is sent, so the modem cannot race the caller's listener setup.
func (c *Client) VoiceOriginateUSSDNoWait(ctx context.Context, data VoiceUSSDData) (<-chan VoiceUSSDNoWaitResult, error) {
	value, err := data.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("originating QMI Voice USSD without waiting: %w", err)
	}
	return c.voiceOriginateUSSDNoWait(ctx, value, true)
}

func (c *Client) voiceOriginateUSSDNoWait(ctx context.Context, value []byte, retry bool) (<-chan VoiceUSSDNoWaitResult, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("originating QMI Voice USSD without waiting: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServiceVoice)
	if err != nil {
		return nil, fmt.Errorf("originating QMI Voice USSD without waiting: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceVoice, clientID, MessageVoiceOriginateUSSDNoWait)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("originating QMI Voice USSD without waiting: subscribe result: %w", err)
	}
	resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceOriginateUSSDNoWait, tlv.TLVs{
		tlv.Bytes(0x01, value),
	})
	if err == nil {
		err = resultOK(resp)
	}
	if err != nil {
		cancel()
		if retry && errors.Is(err, QMIErrorInvalidClientId) && c.forgetServiceClientID(ctx, ServiceVoice, clientID) {
			return c.voiceOriginateUSSDNoWait(ctx, value, false)
		}
		return nil, fmt.Errorf("originating QMI Voice USSD without waiting: %w", err)
	}

	out := make(chan VoiceUSSDNoWaitResult, 1)
	go func() {
		defer close(out)
		defer cancel()
		select {
		case indication, ok := <-indications:
			if !ok {
				return
			}
			var result VoiceUSSDNoWaitResult
			result.Err = result.UnmarshalTLVs(indication.TLVs)
			select {
			case out <- result:
			case <-watchCtx.Done():
			}
		case <-watchCtx.Done():
		}
	}()
	return out, nil
}

// UnmarshalTLVs parses an Originate USSD No Wait result indication.
func (r *VoiceUSSDNoWaitResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = VoiceUSSDNoWaitResult{}
	var result VoiceUSSDNoWaitResult
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: error TLV length %d, want 2", len(value))
		}
		result.ProtocolError = QMIError(binary.LittleEndian.Uint16(value))
		result.ProtocolErrorKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: failure cause TLV length %d, want 2", len(value))
		}
		result.FailureCause = binary.LittleEndian.Uint16(value)
		result.FailureCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		var data VoiceUSSDData
		if err := data.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: %w", err)
		}
		result.Data = data
		result.DataKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if err := result.Alpha.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: %w", err)
		}
		result.AlphaKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		uss, err := decodeVoiceUint16Array(value, false)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: UTF-16 data: %w", err)
		}
		result.UTF16 = uss
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		text, err := decodeVoiceUint16Array(value, true)
		if err != nil {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: failure text: %w", err)
		}
		result.FailureCauseText = text
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI Voice USSD no-wait result: SIP error TLV length %d, want 2", len(value))
		}
		result.SIPErrorCode = binary.LittleEndian.Uint16(value)
		result.SIPErrorCodeKnown = true
	}
	*r = result
	return nil
}
