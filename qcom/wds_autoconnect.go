package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDSAutoconnectSetting is the modem's automatic packet-data policy.
type WDSAutoconnectSetting uint8

const (
	WDSAutoconnectDisabled WDSAutoconnectSetting = iota
	WDSAutoconnectEnabled
	WDSAutoconnectPaused
)

// WDSAutoconnectRoaming controls whether autoconnect is allowed while roaming.
type WDSAutoconnectRoaming uint8

const (
	WDSAutoconnectRoamingAllowed WDSAutoconnectRoaming = iota
	WDSAutoconnectHomeOnly
)

// WDSAutoconnectSettings contains the current autoconnect policy.
type WDSAutoconnectSettings struct {
	Status       WDSAutoconnectSetting
	Roaming      WDSAutoconnectRoaming
	RoamingKnown bool
}

// WDSGetAutoconnectSettingsRequest encodes Get Autoconnect Settings.
type WDSGetAutoconnectSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r WDSGetAutoconnectSettingsRequest) Request() Request {
	return wdsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageWDSGetAutoconnectSettings)
}

// WDSGetAutoconnectSettingsResponse is the parsed autoconnect response.
type WDSGetAutoconnectSettingsResponse struct {
	Settings WDSAutoconnectSettings
}

// UnmarshalTLVs parses the mandatory status and optional roaming policy.
func (r *WDSGetAutoconnectSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetAutoconnectSettingsResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS autoconnect settings: status TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS autoconnect settings: status TLV length %d, want 1", len(value))
	}
	r.Settings.Status = WDSAutoconnectSetting(value[0])
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WDS autoconnect settings: roaming TLV length %d, want 1", len(value))
		}
		r.Settings.Roaming = WDSAutoconnectRoaming(value[0])
		r.Settings.RoamingKnown = true
	}
	return nil
}

// WDSSetAutoconnectSettingsRequest encodes Set Autoconnect Settings.
type WDSSetAutoconnectSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Settings      WDSAutoconnectSettings
}

// Request validates and encodes the selected autoconnect policy.
func (r WDSSetAutoconnectSettingsRequest) Request() (Request, error) {
	if r.Settings.Status > WDSAutoconnectPaused {
		return Request{}, fmt.Errorf("encoding QMI WDS autoconnect settings: status %d is out of range", r.Settings.Status)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(r.Settings.Status))}
	if r.Settings.RoamingKnown {
		if r.Settings.Roaming > WDSAutoconnectHomeOnly {
			return Request{}, fmt.Errorf("encoding QMI WDS autoconnect settings: roaming policy %d is out of range", r.Settings.Roaming)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(r.Settings.Roaming)))
	}
	return Request{
		Service: ServiceWDS, ClientID: r.ClientID, TransactionID: r.TransactionID,
		MessageID: MessageWDSSetAutoconnectSettings, Timeout: r.Timeout, TLVs: tlvs,
	}, nil
}

// WDSAutoconnectSettings returns the modem's automatic packet-data policy.
func (c *Client) WDSAutoconnectSettings(ctx context.Context) (WDSAutoconnectSettings, error) {
	var settings WDSAutoconnectSettings
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSGetAutoconnectSettingsRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var decoded WDSGetAutoconnectSettingsResponse
		if err := decoded.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		settings = decoded.Settings
		return nil
	})
	if err != nil {
		return WDSAutoconnectSettings{}, fmt.Errorf("reading QMI WDS autoconnect settings: %w", err)
	}
	return settings, nil
}

// WDSSetAutoconnectSettings changes the modem's automatic packet-data policy.
func (c *Client) WDSSetAutoconnectSettings(ctx context.Context, settings WDSAutoconnectSettings) error {
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req, err := (WDSSetAutoconnectSettingsRequest{
			ClientID: clientID, Timeout: DefaultRequestTimeout, Settings: settings,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("setting QMI WDS autoconnect settings: %w", err)
	}
	return nil
}
