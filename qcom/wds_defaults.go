package qcom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WDSGetDefaultSettingsRequest encodes Get Default Settings.
type WDSGetDefaultSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	ProfileType   WDSProfileType
}

// Request converts the profile selector into a QMI request.
func (r WDSGetDefaultSettingsRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetDefaultSettings,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint8(r.ProfileType))},
	}
}

// WDSGetDefaultSettingsResponse contains the practical profile fields exposed
// by WDSProfileSettings. ID.Type identifies the requested profile technology;
// ID.Index has no meaning for default settings.
type WDSGetDefaultSettingsResponse struct {
	Settings WDSProfileSettings
}

// UnmarshalTLVs parses the profile fields shared with Get Profile Settings.
func (r *WDSGetDefaultSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	profileType := r.Settings.ID.Type
	parsed := WDSGetProfileSettingsResponse{
		Settings: WDSProfileSettings{ID: WDSProfileID{Type: profileType}},
	}
	if err := parsed.UnmarshalTLVs(tlvs); err != nil {
		return fmt.Errorf("parsing QMI WDS default settings: %w", err)
	}
	r.Settings = parsed.Settings
	return nil
}

// WDSGetDefaultProfileRequest encodes Get Default Profile Number.
type WDSGetDefaultProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	ProfileType   WDSProfileType
	Family        WDSProfileFamily
}

// Request converts the profile family into a QMI request.
func (r WDSGetDefaultProfileRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetDefaultProfile,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte{byte(r.ProfileType), byte(r.Family)}),
		},
	}
}

// WDSGetDefaultProfileResponse contains the selected profile index.
type WDSGetDefaultProfileResponse struct {
	Index uint8
}

// UnmarshalTLVs parses the mandatory default profile index.
func (r *WDSGetDefaultProfileResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetDefaultProfileResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WDS default profile: profile index TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WDS default profile: profile index TLV length %d, want 1", len(value))
	}
	r.Index = value[0]
	return nil
}

// WDSDefaultSettings returns the modem defaults for a profile technology.
func (c *Client) WDSDefaultSettings(ctx context.Context, profileType WDSProfileType) (WDSProfileSettings, error) {
	if err := validateWDSProfileType(profileType); err != nil {
		return WDSProfileSettings{}, fmt.Errorf("querying QMI WDS default settings: %w", err)
	}

	req := WDSGetDefaultSettingsRequest{
		Timeout:     DefaultRequestTimeout,
		ProfileType: profileType,
	}.Request()
	resp, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs)
	if err != nil {
		return WDSProfileSettings{}, fmt.Errorf("querying QMI WDS default settings: %w", err)
	}
	parsed := WDSGetDefaultSettingsResponse{
		Settings: WDSProfileSettings{ID: WDSProfileID{Type: profileType}},
	}
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSProfileSettings{}, err
	}
	return parsed.Settings, nil
}

// WDSDefaultProfile returns the default profile in the selected family.
func (c *Client) WDSDefaultProfile(ctx context.Context, profileType WDSProfileType, family WDSProfileFamily) (WDSProfileID, error) {
	if err := validateWDSProfileType(profileType); err != nil {
		return WDSProfileID{}, fmt.Errorf("querying QMI WDS default profile: %w", err)
	}
	if err := validateWDSProfileFamily(family); err != nil {
		return WDSProfileID{}, fmt.Errorf("querying QMI WDS default profile: %w", err)
	}

	req := WDSGetDefaultProfileRequest{
		Timeout:     DefaultRequestTimeout,
		ProfileType: profileType,
		Family:      family,
	}.Request()
	resp, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs)
	if err != nil {
		return WDSProfileID{}, fmt.Errorf("querying QMI WDS default profile: %w", err)
	}
	var parsed WDSGetDefaultProfileResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSProfileID{}, err
	}
	return WDSProfileID{Type: profileType, Index: parsed.Index}, nil
}

func validateWDSProfileType(profileType WDSProfileType) error {
	switch profileType {
	case WDSProfileType3GPP, WDSProfileType3GPP2, WDSProfileTypeEPC:
		return nil
	default:
		return fmt.Errorf("profile type %d is out of range", profileType)
	}
}

func validateWDSProfileFamily(family WDSProfileFamily) error {
	switch family {
	case WDSProfileFamilyEmbedded, WDSProfileFamilyTethered:
		return nil
	default:
		return fmt.Errorf("profile family %d is out of range", family)
	}
}
