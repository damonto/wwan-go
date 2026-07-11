package uim

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	imssTLVSetTestMode = 0x12
	imssTLVGetTestMode = 0x13
)

// IMSSSetTestModeRequest encodes QMI IMSS Set Registration Manager Config.
type IMSSSetTestModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Enabled       bool
}

// Request converts the request into a QMI IMSS request.
func (r IMSSSetTestModeRequest) Request() qcom.Request {
	enabled := uint8(0)
	if r.Enabled {
		enabled = 1
	}
	return qcom.Request{
		Service:       qcom.ServiceIMSS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageIMSSSetRegistrationManagerConfig,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(imssTLVSetTestMode, enabled)},
	}
}

// IMSSTestMode returns whether the modem's internal IMS registration is disabled.
func (r *Reader) IMSSTestMode(ctx context.Context) (bool, error) {
	var enabled bool
	err := r.withServiceClient(ctx, qcom.ServiceIMSS, func(clientID uint8) error {
		resp, err := r.requestService(ctx, qcom.ServiceIMSS, clientID, qcom.MessageIMSSGetRegistrationManagerConfig, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed IMSSTestModeResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		enabled = parsed.Enabled
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("querying QMI IMSS test mode: %w", err)
	}
	return enabled, nil
}

// SetIMSSTestMode controls the modem's internal IMS registration client.
func (r *Reader) SetIMSSTestMode(ctx context.Context, enabled bool) error {
	err := r.withServiceClient(ctx, qcom.ServiceIMSS, func(clientID uint8) error {
		req := IMSSSetTestModeRequest{ClientID: clientID, Timeout: DefaultRequestTimeout, Enabled: enabled}.Request()
		resp, err := r.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("setting QMI IMSS test mode: %w", err)
	}
	return nil
}

// IMSSTestModeResponse is the parsed registration manager configuration.
type IMSSTestModeResponse struct {
	Enabled bool
}

// UnmarshalTLVs parses QMI IMSS Get Registration Manager Config response TLVs.
func (r *IMSSTestModeResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSSTestModeResponse{}
	value, ok := tlv.Value(tlvs, imssTLVGetTestMode)
	if !ok {
		return errors.New("parsing QMI IMSS test mode: test mode TLV missing")
	}
	if len(value) < 1 {
		return errors.New("parsing QMI IMSS test mode: test mode TLV is truncated")
	}
	r.Enabled = value[0] != 0
	return nil
}
