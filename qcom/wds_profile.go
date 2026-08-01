package qcom

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	wdsTLVProfileList                 = 0x01
	wdsTLVProfileID                   = 0x01
	wdsTLVProfileType                 = 0x10
	wdsTLVProfileName                 = 0x10
	wdsTLVProfilePDPType              = 0x11
	wdsTLVProfileHeaderCompression    = 0x12
	wdsTLVProfileDataCompression      = 0x13
	wdsTLVProfileAPN                  = 0x14
	wdsTLVProfilePrimaryIPv4DNS       = 0x15
	wdsTLVProfileSecondaryIPv4DNS     = 0x16
	wdsTLVProfileUMTSRequestedQoS     = 0x17
	wdsTLVProfileUMTSMinimumQoS       = 0x18
	wdsTLVProfileGPRSRequestedQoS     = 0x19
	wdsTLVProfileGPRSMinimumQoS       = 0x1A
	wdsTLVProfileUsername             = 0x1B
	wdsTLVProfilePassword             = 0x1C
	wdsTLVProfileAuth                 = 0x1D
	wdsTLVProfileIPv4Preference       = 0x1E
	wdsTLVPCSCFUsingPCO               = 0x1F
	wdsTLVProfilePDPAccessControl     = 0x20
	wdsTLVPCSCFUsingDHCP              = 0x21
	wdsTLVIMCNFlag                    = 0x22
	wdsTLVProfilePDPContextNumber     = 0x25
	wdsTLVProfilePDPContextSecondary  = 0x26
	wdsTLVProfilePDPContextPrimaryID  = 0x27
	wdsTLVProfileIPv6Preference       = 0x28
	wdsTLVProfileUMTSRequestedQoSSig  = 0x29
	wdsTLVProfileUMTSMinimumQoSSig    = 0x2A
	wdsTLVProfilePrimaryIPv6DNS       = 0x2B
	wdsTLVProfileSecondaryIPv6DNS     = 0x2C
	wdsTLVProfileAddressAllocation    = 0x2D
	wdsTLVProfileLTEQoS               = 0x2E
	wdsTLVProfileAPNDisabled          = 0x2F
	wdsTLVProfileRoamingDisallowed    = 0x3E
	wdsTLVProfileVLAN                 = 0x50
	wdsTLVProfileAPNType              = 0xDD
	wdsTLVProfileCLATEnabled          = 0xDE
	wdsTLVProfileIPv6PrefixDelegation = 0xDF
)

var ErrWDSProfileNotFound = errors.New("WDS profile not found")

type WDSCreateProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        WDSProfileConfig
}

func (r WDSCreateProfileRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI WDS create profile: %w", err)
	}
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSCreateProfile,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

type WDSDeleteProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profile       WDSProfileID
}

func (r WDSDeleteProfileRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSDeleteProfile,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(wdsTLVProfileID, []byte{byte(r.Profile.Type), r.Profile.Index})},
	}
}

type WDSGetProfileListRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	ProfileType   WDSProfileType
}

func (r WDSGetProfileListRequest) Request() Request {
	return Request{Service: ServiceWDS, ClientID: r.ClientID, TransactionID: r.TransactionID, MessageID: MessageWDSGetProfileList, Timeout: r.Timeout, TLVs: tlv.TLVs{tlv.Uint(wdsTLVProfileType, uint8(r.ProfileType))}}
}

type WDSGetProfileSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profile       WDSProfileID
}

type WDSModifyProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profile       WDSProfileID
	PCSCFUsingPCO bool
}

// WDSUpdateProfileRequest encodes optional changes for a stored WDS profile.
type WDSUpdateProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profile       WDSProfileID
	Update        WDSProfileUpdate
}

func (r WDSModifyProfileRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSModifyProfile,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Bytes(wdsTLVProfileID, []byte{byte(r.Profile.Type), r.Profile.Index}),
			tlv.Bytes(wdsTLVPCSCFUsingPCO, []byte{boolByte(r.PCSCFUsingPCO)}),
		},
	}
}

func (r WDSUpdateProfileRequest) Request() (Request, error) {
	if err := validateWDSProfileType(r.Profile.Type); err != nil {
		return Request{}, err
	}
	if wdsProfileUpdateEmpty(r.Update) {
		return Request{}, errors.New("no profile fields selected")
	}
	if err := r.Update.validate(); err != nil {
		return Request{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(wdsTLVProfileID, []byte{byte(r.Profile.Type), r.Profile.Index})}
	updateTLVs, err := r.Update.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	tlvs = append(tlvs, updateTLVs...)
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSModifyProfile,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

type WDSSetDefaultProfileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Profile       WDSProfileID
	Family        WDSProfileFamily
}

func (r WDSSetDefaultProfileRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSSetDefaultProfile,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Bytes(wdsTLVProfileID, []byte{byte(r.Profile.Type), byte(r.Family), r.Profile.Index}),
		},
	}
}

func (r WDSGetProfileSettingsRequest) Request() Request {
	return Request{Service: ServiceWDS, ClientID: r.ClientID, TransactionID: r.TransactionID, MessageID: MessageWDSGetProfileSettings, Timeout: r.Timeout, TLVs: tlv.TLVs{tlv.Bytes(wdsTLVProfileID, []byte{byte(r.Profile.Type), r.Profile.Index})}}
}

func (c *Client) WDSProfiles(ctx context.Context, profileType WDSProfileType) ([]WDSProfile, error) {
	var profiles []WDSProfile
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSGetProfileListRequest{ClientID: clientID, Timeout: DefaultRequestTimeout, ProfileType: profileType}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed WDSGetProfileListResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		profiles = parsed.Profiles
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("querying QMI WDS profiles: %w", err)
	}
	return profiles, nil
}

func (c *Client) WDSProfileSettings(ctx context.Context, id WDSProfileID) (WDSProfileSettings, error) {
	settings := WDSProfileSettings{ID: id}
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSGetProfileSettingsRequest{ClientID: clientID, Timeout: DefaultRequestTimeout, Profile: id}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed WDSGetProfileSettingsResponse
		parsed.Settings.ID = id
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		parsed.Settings.ID = id
		settings = parsed.Settings
		return nil
	})
	if err != nil {
		return WDSProfileSettings{}, fmt.Errorf("querying QMI WDS profile %d settings: %w", id.Index, err)
	}
	return settings, nil
}

// WDSModifyProfile sets P-CSCF address delivery through PCO for a stored profile.
func (c *Client) WDSModifyProfile(ctx context.Context, id WDSProfileID, pcscfUsingPCO bool) error {
	return c.WDSUpdateProfile(ctx, id, WDSProfileUpdate{PCSCFUsingPCO: &pcscfUsingPCO})
}

// WDSUpdateProfile applies selected changes to a stored profile.
func (c *Client) WDSUpdateProfile(ctx context.Context, id WDSProfileID, update WDSProfileUpdate) error {
	if err := validateWDSProfileType(id.Type); err != nil {
		return fmt.Errorf("modifying QMI WDS profile %d: %w", id.Index, err)
	}
	if wdsProfileUpdateEmpty(update) {
		return errors.New("modifying QMI WDS profile: no fields selected")
	}
	if err := update.validate(); err != nil {
		return fmt.Errorf("modifying QMI WDS profile %d: %w", id.Index, err)
	}

	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req, err := (WDSUpdateProfileRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Profile:  id,
			Update:   update,
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
		return fmt.Errorf("modify QMI WDS profile %d: %w", id.Index, err)
	}
	return nil
}

func wdsProfileUpdateEmpty(update WDSProfileUpdate) bool {
	return update.Name == nil && update.APN == nil && update.PDPType == nil &&
		update.Username == nil && update.Password == nil && update.Authentication == nil &&
		update.HeaderCompression == nil && update.DataCompression == nil &&
		update.PrimaryIPv4DNS == nil && update.SecondaryIPv4DNS == nil &&
		update.UMTSRequestedQoS == nil && update.UMTSMinimumQoS == nil &&
		update.GPRSRequestedQoS == nil && update.GPRSMinimumQoS == nil &&
		update.IPv4AddressPreference == nil && update.PCSCFUsingPCO == nil &&
		update.PDPAccessControl == nil && update.PCSCFUsingDHCP == nil && update.IMCN == nil &&
		update.PDPContextNumber == nil && update.PDPContextSecondary == nil &&
		update.PDPContextPrimaryID == nil && update.IPv6AddressPreference == nil &&
		update.UMTSRequestedQoSWithSignaling == nil && update.UMTSMinimumQoSWithSignaling == nil &&
		update.PrimaryIPv6DNS == nil && update.SecondaryIPv6DNS == nil &&
		update.AddressAllocationPreference == nil && update.LTEQoS == nil &&
		update.APNDisabled == nil && update.RoamingDisallowed == nil && update.VLAN == nil &&
		update.APNType == nil && update.CLATEnabled == nil && update.IPv6PrefixDelegation == nil
}

func (update *WDSProfileUpdate) validate() error {
	if update.Name != nil {
		if err := validateWDSString(*update.Name, wdsProfileNameMaxLength); err != nil {
			return fmt.Errorf("validating WDS profile name: %w", err)
		}
	}
	if update.APN != nil {
		if err := validateWDSString(*update.APN, wdsAPNMaxLength); err != nil {
			return fmt.Errorf("validating WDS APN: %w", err)
		}
	}
	if update.PDPType != nil && *update.PDPType > WDSPDPTypeNonIP {
		return fmt.Errorf("unsupported WDS PDP type %d", *update.PDPType)
	}
	if update.Username != nil {
		if err := validateWDSString(*update.Username, wdsUsernameMaxLength); err != nil {
			return fmt.Errorf("validating WDS username: %w", err)
		}
	}
	if update.Password != nil {
		if err := validateWDSString(*update.Password, wdsPasswordMaxLength); err != nil {
			return fmt.Errorf("validating WDS password: %w", err)
		}
	}
	if update.Authentication != nil {
		if err := validateWDSAuthentication(*update.Authentication); err != nil {
			return err
		}
	}
	if update.HeaderCompression != nil && *update.HeaderCompression > WDSPDPHeaderCompressionRFC3095 {
		return fmt.Errorf("WDS PDP header compression %d is out of range", *update.HeaderCompression)
	}
	if update.DataCompression != nil && *update.DataCompression > WDSPDPDataCompressionV44 {
		return fmt.Errorf("WDS PDP data compression %d is out of range", *update.DataCompression)
	}
	if err := validateWDSProfileIPv4(update.PrimaryIPv4DNS); err != nil {
		return fmt.Errorf("validating WDS primary IPv4 DNS: %w", err)
	}
	if err := validateWDSProfileIPv4(update.SecondaryIPv4DNS); err != nil {
		return fmt.Errorf("validating WDS secondary IPv4 DNS: %w", err)
	}
	if err := validateWDSProfileIPv4(update.IPv4AddressPreference); err != nil {
		return fmt.Errorf("validating WDS IPv4 address preference: %w", err)
	}
	if update.PDPAccessControl != nil && *update.PDPAccessControl > WDSPDPAccessControlPermission {
		return fmt.Errorf("WDS PDP access control %d is out of range", *update.PDPAccessControl)
	}
	if err := validateWDSProfileIPv6(update.IPv6AddressPreference); err != nil {
		return fmt.Errorf("validating WDS IPv6 address preference: %w", err)
	}
	if err := validateWDSProfileIPv6(update.PrimaryIPv6DNS); err != nil {
		return fmt.Errorf("validating WDS primary IPv6 DNS: %w", err)
	}
	if err := validateWDSProfileIPv6(update.SecondaryIPv6DNS); err != nil {
		return fmt.Errorf("validating WDS secondary IPv6 DNS: %w", err)
	}
	if update.AddressAllocationPreference != nil && *update.AddressAllocationPreference > WDSAddressAllocationDHCP {
		return fmt.Errorf("WDS address allocation preference %d is out of range", *update.AddressAllocationPreference)
	}
	if update.LTEQoS != nil && update.LTEQoS.ClassIdentifier > WDSQoSClassNonGuaranteedBitrate8 {
		return fmt.Errorf("WDS LTE QoS class %d is out of range", update.LTEQoS.ClassIdentifier)
	}
	if update.VLAN != nil && update.VLAN.Start > update.VLAN.End {
		return fmt.Errorf("WDS VLAN range start %d exceeds end %d", update.VLAN.Start, update.VLAN.End)
	}
	if update.APNType != nil && *update.APNType&^wdsAPNTypeAll != 0 {
		return fmt.Errorf("unsupported WDS APN type mask 0x%X", uint64(*update.APNType))
	}
	return nil
}

// WDSSetDefaultProfile selects a stored profile as the default profile in the given family.
func (c *Client) WDSSetDefaultProfile(ctx context.Context, id WDSProfileID, family WDSProfileFamily) error {
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSSetDefaultProfileRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Profile:  id,
			Family:   family,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("set default QMI WDS profile %d: %w", id.Index, err)
	}
	return nil
}

// WDSCreateProfile creates a persistent 3GPP profile with APN and PDP type.
// The caller owns the returned profile and should delete it when it is no
// longer needed.
func (c *Client) WDSCreateProfile(ctx context.Context, apn string, pdpType WDSPDPType) (uint8, error) {
	id, err := c.WDSCreateProfileWithConfig(ctx, WDSProfileConfig{APN: apn, PDPType: pdpType})
	if err != nil {
		return 0, err
	}
	return id.Index, nil
}

// WDSCreateProfileWithConfig creates a persistent 3GPP profile with optional
// authentication and IMS-related settings.
func (c *Client) WDSCreateProfileWithConfig(ctx context.Context, cfg WDSProfileConfig) (WDSProfileID, error) {
	cfg.APN = strings.TrimSpace(cfg.APN)
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.APN == "" {
		return WDSProfileID{}, errors.New("creating QMI WDS profile: APN is required")
	}
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("wwan-go-%d", cfg.PDPType)
	}
	if err := cfg.validate(); err != nil {
		return WDSProfileID{}, fmt.Errorf("creating QMI WDS profile: %w", err)
	}

	var id WDSProfileID
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req, err := (WDSCreateProfileRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   cfg,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, wdsTLVProfileID)
		if !ok {
			return errors.New("parsing QMI WDS create profile: profile identifier is missing")
		}
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS create profile: profile identifier length %d, want 2", len(value))
		}
		if WDSProfileType(value[0]) != cfg.Type {
			return fmt.Errorf("parsing QMI WDS create profile: profile type %d, want %d", value[0], cfg.Type)
		}
		id = WDSProfileID{Type: WDSProfileType(value[0]), Index: value[1]}
		return nil
	})
	if err != nil {
		return WDSProfileID{}, fmt.Errorf("creating QMI WDS profile: %w", err)
	}
	return id, nil
}

func (cfg *WDSProfileConfig) validate() error {
	if err := validateWDSProfileType(cfg.Type); err != nil {
		return err
	}
	update := wdsProfileUpdateFromConfig(*cfg)
	return update.validate()
}

func validateWDSProfileIPv4(address *netip.Addr) error {
	if address == nil {
		return nil
	}
	if !address.Unmap().Is4() {
		return fmt.Errorf("address %q is not IPv4", *address)
	}
	return nil
}

func validateWDSProfileIPv6(address *netip.Addr) error {
	if address == nil {
		return nil
	}
	if !address.Unmap().Is6() {
		return fmt.Errorf("address %q is not IPv6", *address)
	}
	return nil
}

// WDSDeleteProfile removes a persistent 3GPP profile.
func (c *Client) WDSDeleteProfile(ctx context.Context, index uint8) error {
	err := c.withServiceClient(ctx, ServiceWDS, func(clientID uint8) error {
		req := WDSDeleteProfileRequest{ClientID: clientID, Timeout: DefaultRequestTimeout, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: index}}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("deleting QMI WDS profile %d: %w", index, err)
	}
	return nil
}

// WDSProfileIndex returns the 3GPP profile whose APN matches apn.
func (c *Client) WDSProfileIndex(ctx context.Context, apn string) (uint8, error) {
	apn = strings.TrimSpace(apn)
	if apn == "" {
		return 0, errors.New("querying QMI WDS profile: APN is required")
	}
	profiles, err := c.WDSProfiles(ctx, WDSProfileType3GPP)
	if err != nil {
		return 0, err
	}
	for _, profile := range profiles {
		settings, err := c.WDSProfileSettings(ctx, profile.ID)
		if err != nil {
			return 0, err
		}
		if settings.APNKnown && strings.EqualFold(strings.TrimSpace(settings.APN), apn) {
			return profile.ID.Index, nil
		}
	}
	return 0, fmt.Errorf("%w: APN %q", ErrWDSProfileNotFound, apn)
}

type WDSGetProfileListResponse struct{ Profiles []WDSProfile }

func (r *WDSGetProfileListResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetProfileListResponse{}
	value, ok := tlv.Value(tlvs, wdsTLVProfileList)
	if !ok {
		return errors.New("parsing QMI WDS profile list: profile list TLV missing")
	}
	if len(value) < 1 {
		return errors.New("parsing QMI WDS profile list: profile list TLV is truncated")
	}
	count, rest := int(value[0]), value[1:]
	r.Profiles = make([]WDSProfile, 0, count)
	for range count {
		if len(rest) < 3 {
			return errors.New("parsing QMI WDS profile list: profile entry is truncated")
		}
		nameLen := int(rest[2])
		if len(rest) < 3+nameLen {
			return errors.New("parsing QMI WDS profile list: profile name is truncated")
		}
		r.Profiles = append(r.Profiles, WDSProfile{ID: WDSProfileID{Type: WDSProfileType(rest[0]), Index: rest[1]}, Name: string(rest[3 : 3+nameLen])})
		rest = rest[3+nameLen:]
	}
	if len(rest) != 0 {
		return errors.New("parsing QMI WDS profile list: trailing data")
	}
	return nil
}

type WDSGetProfileSettingsResponse struct{ Settings WDSProfileSettings }

func (r *WDSGetProfileSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	id := r.Settings.ID
	*r = WDSGetProfileSettingsResponse{Settings: WDSProfileSettings{ID: id}}
	return unmarshalWDSProfileSettings(tlvs, &r.Settings)
}
