package qcom

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	imsPolicyManagerAPNCount     = 6
	imsPolicyManagerAPNMaxLength = 49
)

type imsSettingsIndicationRegistration uint8

const (
	imsSettingsIndicationPolicyManager imsSettingsIndicationRegistration = iota
	imsSettingsIndicationServicesEnabled
)

// IMSSettingsError is an IMS Settings service-specific response code.
// Qualcomm's IDL encodes this enum in one byte.
type IMSSettingsError uint8

const (
	IMSSettingsNoError IMSSettingsError = iota
	IMSSettingsNotReady
	IMSSettingsFileNotAvailable
	IMSSettingsReadFailure
	IMSSettingsWriteFailure
	IMSSettingsInternalError
	IMSSettingsValueNotSupported
)

// IMSIndicationConfig contains optional IMS Settings indication updates.
type IMSIndicationConfig struct {
	PolicyManager   *bool
	ServicesEnabled *bool
}

// IMSRegisterIndicationsRequest encodes IMS Settings Config Indication Register.
type IMSRegisterIndicationsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        IMSIndicationConfig
}

// Request converts indication updates into QMI TLVs.
func (r IMSRegisterIndicationsRequest) Request() Request {
	var tlvs tlv.TLVs
	for _, field := range []struct {
		typ   uint8
		value *bool
	}{
		{0x1B, r.Config.PolicyManager},
		{0x2D, r.Config.ServicesEnabled},
	} {
		if field.value != nil {
			tlvs = append(tlvs, tlv.Uint(field.typ, boolByte(*field.value)))
		}
	}
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSRegisterIndications,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// IMSServiceMask selects IMS services in policy-manager settings.
type IMSServiceMask uint64

const (
	IMSServiceVoLTE IMSServiceMask = 1 << iota
	IMSServiceVideoTelephony
	IMSServiceSMS
	IMSServiceInstantMessaging
	IMSServiceVideoShare
	IMSServiceImageShare
	IMSServiceMSRP
	IMSServiceGeoLocation
	IMSServicePresence
	IMSServiceFileTransfer
	IMSServiceRCS
	IMSServiceStandaloneMessaging
	IMSServiceFileTransferThumbnail
	IMSServiceFileTransferStoreForward
	IMSServiceFileTransferHTTP
	IMSServiceDefault
	IMSServiceVideoShareDuringCS
	IMSServiceSocialPresence
	IMSServiceCapabilityDiscovery
	IMSServiceGeoLocationPush
	IMSServiceGeoLocationPull
	IMSServiceFullGroupChat
)

// IMSPolicyManagerAPNs is the fixed six-entry APN list used by IMS Settings.
type IMSPolicyManagerAPNs [imsPolicyManagerAPNCount]string

// IMSPolicyManagerConfig contains optional Policy Manager updates.
type IMSPolicyManagerConfig struct {
	ACSPriority  *uint8
	ISIMPriority *uint8
	NVPriority   *uint8
	PCOPriority  *uint8
	ServiceMask  *IMSServiceMask
	APNs         *IMSPolicyManagerAPNs
}

// IMSPolicyManagerSettings contains Policy Manager settings reported by a modem.
type IMSPolicyManagerSettings struct {
	ACSPriorityKnown  bool
	ACSPriority       uint8
	ISIMPriorityKnown bool
	ISIMPriority      uint8
	NVPriorityKnown   bool
	NVPriority        uint8
	PCOPriorityKnown  bool
	PCOPriority       uint8
	ServiceMaskKnown  bool
	ServiceMask       IMSServiceMask
	APNsKnown         bool
	APNs              IMSPolicyManagerAPNs
}

// IMSSetPolicyManagerSettingsRequest encodes Set IMS Policy Manager Settings.
type IMSSetPolicyManagerSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        IMSPolicyManagerConfig
}

// Request validates and converts Policy Manager updates into QMI TLVs.
func (r IMSSetPolicyManagerSettingsRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	for _, field := range []struct {
		typ   uint8
		value *uint8
	}{
		{0x14, r.Config.ACSPriority},
		{0x15, r.Config.ISIMPriority},
		{0x16, r.Config.NVPriority},
		{0x17, r.Config.PCOPriority},
	} {
		if field.value != nil {
			tlvs = append(tlvs, tlv.Uint(field.typ, *field.value))
		}
	}
	if r.Config.ServiceMask != nil {
		value := binary.LittleEndian.AppendUint64(nil, uint64(*r.Config.ServiceMask))
		tlvs = append(tlvs, tlv.Bytes(0x18, value))
	}
	if r.Config.APNs != nil {
		value, err := r.Config.APNs.MarshalBinary()
		if err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(0x19, value))
	}
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSSetPolicyManagerSettings,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// MarshalBinary encodes six size-prefixed APN strings.
func (a IMSPolicyManagerAPNs) MarshalBinary() ([]byte, error) {
	value := make([]byte, 0, imsPolicyManagerAPNCount)
	for index, apn := range a {
		if strings.IndexByte(apn, 0) >= 0 {
			return nil, fmt.Errorf("IMS Policy Manager APN %d contains a NUL byte", index)
		}
		if len(apn) > imsPolicyManagerAPNMaxLength {
			return nil, fmt.Errorf("IMS Policy Manager APN %d length %d exceeds maximum %d", index, len(apn), imsPolicyManagerAPNMaxLength)
		}
		value = append(value, byte(len(apn)))
		value = append(value, apn...)
	}
	return value, nil
}

// UnmarshalBinary decodes six size-prefixed APN strings.
func (a *IMSPolicyManagerAPNs) UnmarshalBinary(value []byte) error {
	*a = IMSPolicyManagerAPNs{}

	var decoded IMSPolicyManagerAPNs
	offset := 0
	for index := range imsPolicyManagerAPNCount {
		if offset >= len(value) {
			return fmt.Errorf("APN %d length is missing", index)
		}
		length := int(value[offset])
		offset++
		if length > imsPolicyManagerAPNMaxLength {
			return fmt.Errorf("APN %d length %d exceeds maximum %d", index, length, imsPolicyManagerAPNMaxLength)
		}
		if len(value)-offset < length {
			return fmt.Errorf("APN %d data is truncated", index)
		}
		decoded[index] = string(value[offset : offset+length])
		offset += length
	}
	if offset != len(value) {
		return fmt.Errorf("length %d, decoded %d", len(value), offset)
	}
	*a = decoded
	return nil
}

// IMSGetPolicyManagerSettingsRequest encodes Get IMS Policy Manager Settings.
type IMSGetPolicyManagerSettingsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r IMSGetPolicyManagerSettingsRequest) Request() Request {
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSGetPolicyManagerSettings,
		Timeout:       r.Timeout,
	}
}

// IMSSetPolicyManagerSettingsResponse contains the settings-specific result.
type IMSSetPolicyManagerSettingsResponse struct {
	SettingsError      IMSSettingsError
	SettingsErrorKnown bool
}

// UnmarshalTLVs parses Set IMS Policy Manager Settings response TLVs.
func (r *IMSSetPolicyManagerSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSSetPolicyManagerSettingsResponse{}
	return decodeIMSSettingsError(tlvs, &r.SettingsError, &r.SettingsErrorKnown)
}

// IMSGetPolicyManagerSettingsResponse contains current Policy Manager settings.
type IMSGetPolicyManagerSettingsResponse struct {
	SettingsError      IMSSettingsError
	SettingsErrorKnown bool
	Settings           IMSPolicyManagerSettings
}

// UnmarshalTLVs parses Get IMS Policy Manager Settings response TLVs.
func (r *IMSGetPolicyManagerSettingsResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var response IMSGetPolicyManagerSettingsResponse
	if err := decodeIMSSettingsError(tlvs, &response.SettingsError, &response.SettingsErrorKnown); err != nil {
		return err
	}
	if err := response.Settings.unmarshalTLVs(tlvs, imsPolicyManagerResponseTLVs); err != nil {
		return err
	}
	*r = response
	return nil
}

// IMSPolicyManagerSettingsIndication is a Policy Manager settings change.
type IMSPolicyManagerSettingsIndication struct {
	Settings IMSPolicyManagerSettings
}

// UnmarshalTLVs parses IMS Policy Manager Settings indication TLVs.
func (i *IMSPolicyManagerSettingsIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var settings IMSPolicyManagerSettings
	if err := settings.unmarshalTLVs(tlvs, imsPolicyManagerIndicationTLVs); err != nil {
		return err
	}
	*i = IMSPolicyManagerSettingsIndication{Settings: settings}
	return nil
}

type imsPolicyManagerTLVs struct {
	acsPriority  uint8
	isimPriority uint8
	nvPriority   uint8
	pcoPriority  uint8
	serviceMask  uint8
	apns         uint8
}

var (
	imsPolicyManagerResponseTLVs   = imsPolicyManagerTLVs{0x15, 0x16, 0x17, 0x18, 0x19, 0x1A}
	imsPolicyManagerIndicationTLVs = imsPolicyManagerTLVs{0x14, 0x15, 0x16, 0x17, 0x18, 0x19}
)

func (settings *IMSPolicyManagerSettings) unmarshalTLVs(tlvs tlv.TLVs, ids imsPolicyManagerTLVs) error {
	for _, field := range []struct {
		typ   uint8
		value *uint8
		known *bool
	}{
		{ids.acsPriority, &settings.ACSPriority, &settings.ACSPriorityKnown},
		{ids.isimPriority, &settings.ISIMPriority, &settings.ISIMPriorityKnown},
		{ids.nvPriority, &settings.NVPriority, &settings.NVPriorityKnown},
		{ids.pcoPriority, &settings.PCOPriority, &settings.PCOPriorityKnown},
	} {
		if err := decodeIMSByte(tlvs, field.typ, field.value, field.known); err != nil {
			return err
		}
	}
	if value, ok := tlv.Value(tlvs, ids.serviceMask); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI IMS Policy Manager service mask: TLV length %d, want 8", len(value))
		}
		settings.ServiceMask = IMSServiceMask(binary.LittleEndian.Uint64(value))
		settings.ServiceMaskKnown = true
	}
	if value, ok := tlv.Value(tlvs, ids.apns); ok {
		var apns IMSPolicyManagerAPNs
		if err := apns.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI IMS Policy Manager APNs: %w", err)
		}
		settings.APNs = apns
		settings.APNsKnown = true
	}
	return nil
}

// IMSCallModePreference selects the preferred access for IMS calls.
// Qualcomm's IDL encodes this enum in four bytes.
type IMSCallModePreference uint32

const (
	IMSCallModeNone IMSCallModePreference = iota
	IMSCallModeCellularPreferred
	IMSCallModeWiFiPreferred
	IMSCallModeWiFiOnly
	IMSCallModeCellularOnly
	IMSCallModeIMS
)

// IMSServicesEnabledConfig contains optional IMS service updates.
type IMSServicesEnabledConfig struct {
	VoiceOverLTE   *bool
	VideoTelephony *bool
	VoiceWiFi      *bool
	CallMode       *IMSCallModePreference
	IMS            *bool
	UT             *bool
	SMS            *bool
	USSD           *bool
	Presence       *bool
	Autoconfig     *bool
	XDM            *bool
	RCS            *bool
	CarrierConfig  *bool
}

// IMSServicesEnabled contains service settings reported by the modem.
type IMSServicesEnabled struct {
	VoiceOverLTEKnown   bool
	VoiceOverLTE        bool
	VideoTelephonyKnown bool
	VideoTelephony      bool
	VoiceWiFiKnown      bool
	VoiceWiFi           bool
	CallModeKnown       bool
	CallMode            IMSCallModePreference
	IMSKnown            bool
	IMS                 bool
	UTKnown             bool
	UT                  bool
	SMSKnown            bool
	SMS                 bool
	USSDKnown           bool
	USSD                bool
	PresenceKnown       bool
	Presence            bool
	AutoconfigKnown     bool
	Autoconfig          bool
	XDMKnown            bool
	XDM                 bool
	RCSKnown            bool
	RCS                 bool
	CarrierConfigKnown  bool
	CarrierConfig       bool
}

// IMSSetServicesEnabledRequest encodes Set IMS Services Enabled Setting.
type IMSSetServicesEnabledRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        IMSServicesEnabledConfig
}

// Request validates and converts IMS service updates into QMI TLVs.
func (r IMSSetServicesEnabledRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	for _, field := range []struct {
		typ   uint8
		value *bool
	}{
		{0x10, r.Config.VoiceOverLTE},
		{0x11, r.Config.VideoTelephony},
		{0x14, r.Config.VoiceWiFi},
		{0x18, r.Config.IMS},
		{0x19, r.Config.UT},
		{0x1A, r.Config.SMS},
		{0x1C, r.Config.USSD},
		{0x1E, r.Config.Presence},
		{0x1F, r.Config.Autoconfig},
		{0x20, r.Config.XDM},
		{0x21, r.Config.RCS},
		{0x25, r.Config.CarrierConfig},
	} {
		if field.value != nil {
			tlvs = append(tlvs, tlv.Uint(field.typ, boolByte(*field.value)))
		}
	}
	if r.Config.CallMode != nil {
		if *r.Config.CallMode > IMSCallModeIMS {
			return Request{}, fmt.Errorf("IMS call mode preference %d is out of range", *r.Config.CallMode)
		}
		tlvs = append(tlvs, tlv.Uint(0x15, uint32(*r.Config.CallMode)))
	}
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSSetServicesEnabled,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// IMSGetServicesEnabledRequest encodes Get IMS Services Enabled Setting.
type IMSGetServicesEnabledRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r IMSGetServicesEnabledRequest) Request() Request {
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSGetServicesEnabled,
		Timeout:       r.Timeout,
	}
}

// IMSSetServicesEnabledResponse contains the settings-specific result.
type IMSSetServicesEnabledResponse struct {
	SettingsError      IMSSettingsError
	SettingsErrorKnown bool
}

// UnmarshalTLVs parses Set IMS Services Enabled response TLVs.
func (r *IMSSetServicesEnabledResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSSetServicesEnabledResponse{}
	return decodeIMSSettingsError(tlvs, &r.SettingsError, &r.SettingsErrorKnown)
}

// IMSGetServicesEnabledResponse contains current IMS service settings.
type IMSGetServicesEnabledResponse struct {
	SettingsError      IMSSettingsError
	SettingsErrorKnown bool
	Services           IMSServicesEnabled
}

// UnmarshalTLVs parses Get IMS Services Enabled response TLVs.
func (r *IMSGetServicesEnabledResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSGetServicesEnabledResponse{}
	if err := decodeIMSSettingsError(tlvs, &r.SettingsError, &r.SettingsErrorKnown); err != nil {
		return err
	}
	// These response IDs come from Qualcomm's IDL. Several later fields in
	// libqmi's JSON incorrectly reuse the request IDs.
	for _, field := range []struct {
		typ   uint8
		value *bool
		known *bool
	}{
		{0x11, &r.Services.VoiceOverLTE, &r.Services.VoiceOverLTEKnown},
		{0x12, &r.Services.VideoTelephony, &r.Services.VideoTelephonyKnown},
		{0x15, &r.Services.VoiceWiFi, &r.Services.VoiceWiFiKnown},
		{0x19, &r.Services.IMS, &r.Services.IMSKnown},
		{0x1A, &r.Services.UT, &r.Services.UTKnown},
		{0x1B, &r.Services.SMS, &r.Services.SMSKnown},
		{0x1D, &r.Services.USSD, &r.Services.USSDKnown},
	} {
		if err := decodeIMSBool(tlvs, field.typ, field.value, field.known); err != nil {
			return err
		}
	}
	return nil
}

// IMSServicesEnabledIndication is an IMS service-settings change.
type IMSServicesEnabledIndication struct {
	Services IMSServicesEnabled
}

// UnmarshalTLVs parses IMS Services Enabled Setting indication TLVs.
func (i *IMSServicesEnabledIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = IMSServicesEnabledIndication{}
	for _, field := range []struct {
		typ   uint8
		value *bool
		known *bool
	}{
		{0x10, &i.Services.VoiceOverLTE, &i.Services.VoiceOverLTEKnown},
		{0x11, &i.Services.VideoTelephony, &i.Services.VideoTelephonyKnown},
		{0x14, &i.Services.VoiceWiFi, &i.Services.VoiceWiFiKnown},
		{0x18, &i.Services.IMS, &i.Services.IMSKnown},
		{0x19, &i.Services.UT, &i.Services.UTKnown},
		{0x1A, &i.Services.SMS, &i.Services.SMSKnown},
		{0x1C, &i.Services.USSD, &i.Services.USSDKnown},
		{0x1E, &i.Services.Presence, &i.Services.PresenceKnown},
		{0x1F, &i.Services.Autoconfig, &i.Services.AutoconfigKnown},
		{0x20, &i.Services.XDM, &i.Services.XDMKnown},
		{0x21, &i.Services.RCS, &i.Services.RCSKnown},
		{0x25, &i.Services.CarrierConfig, &i.Services.CarrierConfigKnown},
	} {
		if err := decodeIMSBool(tlvs, field.typ, field.value, field.known); err != nil {
			return err
		}
	}
	return nil
}

// IMSSubscription identifies a modem subscription for IMS Settings.
type IMSSubscription int32

const (
	IMSSubscriptionNone      IMSSubscription = -1
	IMSSubscriptionPrimary   IMSSubscription = 0
	IMSSubscriptionSecondary IMSSubscription = 1
	IMSSubscriptionTertiary  IMSSubscription = 2
)

// IMSBindRequest encodes QMI IMS Bind.
type IMSBindRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  IMSSubscription
}

// Request validates and converts the subscription into a QMI request.
func (r IMSBindRequest) Request() (Request, error) {
	if r.Subscription < IMSSubscriptionNone || r.Subscription > IMSSubscriptionTertiary {
		return Request{}, fmt.Errorf("IMS subscription %d is out of range", r.Subscription)
	}
	return Request{
		Service:       ServiceIMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSBind,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Subscription))},
	}, nil
}

// SetIMSPolicyManagerSettings updates IMS Policy Manager settings.
func (c *Client) SetIMSPolicyManagerSettings(ctx context.Context, config IMSPolicyManagerConfig) (IMSSetPolicyManagerSettingsResponse, error) {
	req, err := (IMSSetPolicyManagerSettingsRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return IMSSetPolicyManagerSettingsResponse{}, fmt.Errorf("setting QMI IMS Policy Manager settings: %w", err)
	}
	var result IMSSetPolicyManagerSettingsResponse
	if err := c.imsSettingsRequest(ctx, req, &result); err != nil {
		return IMSSetPolicyManagerSettingsResponse{}, fmt.Errorf("setting QMI IMS Policy Manager settings: %w", err)
	}
	return result, nil
}

// IMSRegisterIndications updates IMS Settings indication subscriptions.
func (c *Client) IMSRegisterIndications(ctx context.Context, config IMSIndicationConfig) error {
	req := IMSRegisterIndicationsRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.imsSettingsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI IMS Settings indications: %w", err)
	}
	return nil
}

// IMSPolicyManagerSettings returns current IMS Policy Manager settings.
func (c *Client) IMSPolicyManagerSettings(ctx context.Context) (IMSGetPolicyManagerSettingsResponse, error) {
	var result IMSGetPolicyManagerSettingsResponse
	req := IMSGetPolicyManagerSettingsRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.imsSettingsRequest(ctx, req, &result); err != nil {
		return IMSGetPolicyManagerSettingsResponse{}, fmt.Errorf("querying QMI IMS Policy Manager settings: %w", err)
	}
	return result, nil
}

// SetIMSServicesEnabled updates IMS service settings.
func (c *Client) SetIMSServicesEnabled(ctx context.Context, config IMSServicesEnabledConfig) (IMSSetServicesEnabledResponse, error) {
	req, err := (IMSSetServicesEnabledRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return IMSSetServicesEnabledResponse{}, fmt.Errorf("setting QMI IMS services: %w", err)
	}
	var result IMSSetServicesEnabledResponse
	if err := c.imsSettingsRequest(ctx, req, &result); err != nil {
		return IMSSetServicesEnabledResponse{}, fmt.Errorf("setting QMI IMS services: %w", err)
	}
	return result, nil
}

// IMSServices returns current IMS service settings.
func (c *Client) IMSServices(ctx context.Context) (IMSGetServicesEnabledResponse, error) {
	var result IMSGetServicesEnabledResponse
	req := IMSGetServicesEnabledRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.imsSettingsRequest(ctx, req, &result); err != nil {
		return IMSGetServicesEnabledResponse{}, fmt.Errorf("querying QMI IMS services: %w", err)
	}
	return result, nil
}

// IMSWatchPolicyManagerSettings subscribes to Policy Manager setting changes.
func (c *Client) IMSWatchPolicyManagerSettings(ctx context.Context) (<-chan IMSPolicyManagerSettings, error) {
	raw, err := c.watchIMSSettingsTLVs(ctx, MessageIMSPolicyManagerSettings, imsSettingsIndicationPolicyManager)
	if err != nil {
		return nil, err
	}
	indications := unmarshalTLVStream[IMSPolicyManagerSettingsIndication](ctx, raw)
	out := make(chan IMSPolicyManagerSettings, 8)
	go func() {
		defer close(out)
		for indication := range indications {
			select {
			case out <- indication.Settings:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// IMSWatchServicesEnabled subscribes to IMS service-setting changes.
func (c *Client) IMSWatchServicesEnabled(ctx context.Context) (<-chan IMSServicesEnabled, error) {
	raw, err := c.watchIMSSettingsTLVs(ctx, MessageIMSServicesEnabled, imsSettingsIndicationServicesEnabled)
	if err != nil {
		return nil, err
	}
	indications := unmarshalTLVStream[IMSServicesEnabledIndication](ctx, raw)
	out := make(chan IMSServicesEnabled, 8)
	go func() {
		defer close(out)
		for indication := range indications {
			select {
			case out <- indication.Services:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// IMSBindSubscription binds the IMS Settings client to one modem subscription.
func (c *Client) IMSBindSubscription(ctx context.Context, subscription IMSSubscription) error {
	req, err := (IMSBindRequest{Timeout: DefaultRequestTimeout, Subscription: subscription}).Request()
	if err != nil {
		return fmt.Errorf("binding QMI IMS subscription: %w", err)
	}
	if err := c.imsSettingsRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("binding QMI IMS subscription: %w", err)
	}
	return nil
}

func (c *Client) watchIMSSettingsTLVs(
	ctx context.Context,
	messageID MessageID,
	registration imsSettingsIndicationRegistration,
) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMS Settings: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServiceIMS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMS Settings: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceIMS, clientID, messageID)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI IMS Settings: subscribe: %w", err)
	}
	if err := c.acquireIMSSettingsIndication(ctx, registration); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI IMS Settings: %w", err)
	}

	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseIMSSettingsIndication(registration)
		for indication := range indications {
			select {
			case out <- indication.TLVs:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) acquireIMSSettingsIndication(ctx context.Context, registration imsSettingsIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.imsSettingsIndicationRefs == nil {
		c.imsSettingsIndicationRefs = make(map[imsSettingsIndicationRegistration]int)
	}
	if c.imsSettingsIndicationRefs[registration] > 0 {
		c.imsSettingsIndicationRefs[registration]++
		return nil
	}
	c.imsSettingsIndicationRefs[registration] = 1
	if err := c.setIMSSettingsIndicationRegistration(ctx, registration, true); err != nil {
		delete(c.imsSettingsIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseIMSSettingsIndication(registration imsSettingsIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.imsSettingsIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.imsSettingsIndicationRefs[registration]--
		return
	}
	delete(c.imsSettingsIndicationRefs, registration)
	_ = c.setIMSSettingsIndicationRegistration(ctx, registration, false)
}

func (c *Client) setIMSSettingsIndicationRegistration(ctx context.Context, registration imsSettingsIndicationRegistration, enabled bool) error {
	switch registration {
	case imsSettingsIndicationPolicyManager:
		return c.IMSRegisterIndications(ctx, IMSIndicationConfig{PolicyManager: &enabled})
	case imsSettingsIndicationServicesEnabled:
		return c.IMSRegisterIndications(ctx, IMSIndicationConfig{ServicesEnabled: &enabled})
	default:
		return fmt.Errorf("configuring QMI IMS Settings indications: registration %d is unknown", registration)
	}
}

func (c *Client) imsSettingsRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceIMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceIMS, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if dst == nil {
			return nil
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func decodeIMSSettingsError(tlvs tlv.TLVs, value *IMSSettingsError, known *bool) error {
	wire, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(wire) != 1 {
		return fmt.Errorf("parsing QMI IMS settings error: TLV length %d, want 1", len(wire))
	}
	*value = IMSSettingsError(wire[0])
	*known = true
	return nil
}

func decodeIMSByte(tlvs tlv.TLVs, typ uint8, value *uint8, known *bool) error {
	wire, ok := tlv.Value(tlvs, typ)
	if !ok {
		return nil
	}
	if len(wire) != 1 {
		return fmt.Errorf("parsing QMI IMS TLV 0x%02X: length %d, want 1", typ, len(wire))
	}
	*value = wire[0]
	*known = true
	return nil
}

func decodeIMSBool(tlvs tlv.TLVs, typ uint8, value *bool, known *bool) error {
	var wire uint8
	if err := decodeIMSByte(tlvs, typ, &wire, known); err != nil {
		return err
	}
	if *known {
		*value = wire != 0
	}
	return nil
}
