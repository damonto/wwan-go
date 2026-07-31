package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wmsBroadcastChannelMax = 50

// WMSLanguage identifies the language of a 3GPP2 broadcast category.
type WMSLanguage uint16

const (
	WMSLanguageUnknown WMSLanguage = iota
	WMSLanguageEnglish
	WMSLanguageFrench
	WMSLanguageSpanish
	WMSLanguageJapanese
	WMSLanguageKorean
	WMSLanguageChinese
	WMSLanguageHebrew
)

// WMSServiceCategory identifies a 3GPP2 broadcast service category.
type WMSServiceCategory uint16

const (
	WMSServiceCategoryUnknown WMSServiceCategory = iota
	WMSServiceCategoryEmergencyBroadcast
	WMSServiceCategoryAdministrative
	WMSServiceCategoryMaintenance
	WMSServiceCategoryGeneralNewsLocal
	WMSServiceCategoryGeneralNewsRegional
	WMSServiceCategoryGeneralNewsNational
	WMSServiceCategoryGeneralNewsInternational
	WMSServiceCategoryBusinessNewsLocal
	WMSServiceCategoryBusinessNewsRegional
	WMSServiceCategoryBusinessNewsNational
	WMSServiceCategoryBusinessNewsInternational
	WMSServiceCategorySportsNewsLocal
	WMSServiceCategorySportsNewsRegional
	WMSServiceCategorySportsNewsNational
	WMSServiceCategorySportsNewsInternational
	WMSServiceCategoryEntertainmentNewsLocal
	WMSServiceCategoryEntertainmentNewsRegional
	WMSServiceCategoryEntertainmentNewsNational
	WMSServiceCategoryEntertainmentNewsInternational
	WMSServiceCategoryLocalWeather
	WMSServiceCategoryTrafficReports
	WMSServiceCategoryLocalFlightSchedules
	WMSServiceCategoryRestaurants
	WMSServiceCategoryLodgings
	WMSServiceCategoryRetailDirectory
	WMSServiceCategoryAdvertisements
	WMSServiceCategoryStockQuotes
	WMSServiceCategoryEmploymentOpportunities
	WMSServiceCategoryMedical
	WMSServiceCategoryTechnologyNews
	WMSServiceCategoryMultiple
	WMSServiceCategoryCATPT
)

const (
	WMSServiceCategoryPresidentialAlert WMSServiceCategory = 0x1000 + iota
	WMSServiceCategoryExtremeThreat
	WMSServiceCategorySevereThreat
	WMSServiceCategoryAMBERAlert
	WMSServiceCategoryTestMessage
)

// WMS3GPPBroadcastChannel selects a range of 3GPP cell-broadcast message IDs.
type WMS3GPPBroadcastChannel struct {
	Start    uint16
	End      uint16
	Selected bool
}

// WMS3GPP2BroadcastChannel selects one 3GPP2 category and language pair.
type WMS3GPP2BroadcastChannel struct {
	ServiceCategory WMSServiceCategory
	Language        WMSLanguage
	Selected        bool
}

// WMSBroadcastActivation controls reception of broadcast SMS messages.
type WMSBroadcastActivation struct {
	Mode        WMSMessageMode
	Active      bool
	ActivateAll *bool
}

// WMSBroadcastChannelConfig contains the channel filters for one message mode.
// A GW configuration uses Channels3GPP; a CDMA configuration uses Channels3GPP2.
type WMSBroadcastChannelConfig struct {
	Mode          WMSMessageMode
	Channels3GPP  []WMS3GPPBroadcastChannel
	Channels3GPP2 []WMS3GPP2BroadcastChannel
}

// WMSBroadcastConfig contains the current activation and channel filters.
type WMSBroadcastConfig struct {
	Active        bool
	Mode          WMSMessageMode
	Channels3GPP  []WMS3GPPBroadcastChannel
	Channels3GPP2 []WMS3GPP2BroadcastChannel
	ConfigKnown   bool
}

// WMSSetBroadcastActivation enables or disables broadcast SMS reception.
func (c *Client) WMSSetBroadcastActivation(ctx context.Context, activation WMSBroadcastActivation) error {
	if err := validateWMSMessageMode(activation.Mode); err != nil {
		return fmt.Errorf("setting QMI WMS broadcast activation: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, []byte{byte(activation.Mode), boolByte(activation.Active)})}
	if activation.ActivateAll != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*activation.ActivateAll)))
	}
	if err := c.wmsResultRequest(ctx, MessageWMSSetBroadcastActivation, tlvs); err != nil {
		return fmt.Errorf("setting QMI WMS broadcast activation: %w", err)
	}
	return nil
}

// WMSSetBroadcastConfig replaces the channel filters for one message mode.
func (c *Client) WMSSetBroadcastConfig(ctx context.Context, config WMSBroadcastChannelConfig) error {
	if err := validateWMSMessageMode(config.Mode); err != nil {
		return fmt.Errorf("setting QMI WMS broadcast config: %w", err)
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(config.Mode))}
	switch config.Mode {
	case WMSMessageModeGW:
		if len(config.Channels3GPP2) != 0 {
			return errors.New("setting QMI WMS broadcast config: GW mode contains 3GPP2 channels")
		}
		value, err := encodeWMS3GPPBroadcastChannels(config.Channels3GPP)
		if err != nil {
			return fmt.Errorf("setting QMI WMS broadcast config: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	case WMSMessageModeCDMA:
		if len(config.Channels3GPP) != 0 {
			return errors.New("setting QMI WMS broadcast config: CDMA mode contains 3GPP channels")
		}
		value, err := encodeWMS3GPP2BroadcastChannels(config.Channels3GPP2)
		if err != nil {
			return fmt.Errorf("setting QMI WMS broadcast config: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}

	if err := c.wmsResultRequest(ctx, MessageWMSSetBroadcastConfig, tlvs); err != nil {
		return fmt.Errorf("setting QMI WMS broadcast config: %w", err)
	}
	return nil
}

// WMSBroadcastConfig reads the activation and channel filters for one mode.
func (c *Client) WMSBroadcastConfig(ctx context.Context, mode WMSMessageMode) (WMSBroadcastConfig, error) {
	if err := validateWMSMessageMode(mode); err != nil {
		return WMSBroadcastConfig{}, fmt.Errorf("reading QMI WMS broadcast config: %w", err)
	}

	config := WMSBroadcastConfig{Mode: mode}
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetBroadcastConfig, tlv.TLVs{
			tlv.Uint(0x01, uint8(mode)),
		})
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}

		switch mode {
		case WMSMessageModeGW:
			value, ok := tlv.Value(resp.TLVs, 0x10)
			if !ok {
				return errors.New("parsing QMI WMS broadcast config: 3GPP config TLV missing")
			}
			config.Active, config.Channels3GPP, err = decodeWMS3GPPBroadcastConfig(value)
		case WMSMessageModeCDMA:
			value, ok := tlv.Value(resp.TLVs, 0x11)
			if !ok {
				return errors.New("parsing QMI WMS broadcast config: 3GPP2 config TLV missing")
			}
			config.Active, config.Channels3GPP2, err = decodeWMS3GPP2BroadcastConfig(value)
		}
		config.ConfigKnown = err == nil
		return err
	})
	if err != nil {
		return WMSBroadcastConfig{}, fmt.Errorf("reading QMI WMS broadcast config: %w", err)
	}
	return config, nil
}

// WMSWatchBroadcastConfig subscribes to broadcast activation and channel
// configuration changes.
func (c *Client) WMSWatchBroadcastConfig(ctx context.Context) (<-chan WMSBroadcastConfig, error) {
	raw, err := c.watchWMSTLVs(ctx, MessageWMSBroadcastConfigChanged, wmsIndicationBroadcastConfig)
	if err != nil {
		return nil, fmt.Errorf("watching QMI WMS broadcast config: %w", err)
	}
	return unmarshalTLVStream[WMSBroadcastConfig](ctx, raw), nil
}

// UnmarshalTLVs parses a broadcast-configuration indication.
func (c *WMSBroadcastConfig) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS broadcast config indication: message mode TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI WMS broadcast config indication: message mode TLV length %d, want 1", len(value))
	}
	mode := WMSMessageMode(value[0])
	if err := validateWMSMessageMode(mode); err != nil {
		return fmt.Errorf("parsing QMI WMS broadcast config indication: %w", err)
	}

	config := WMSBroadcastConfig{Mode: mode}
	config3GPP, has3GPP := tlv.Value(tlvs, 0x10)
	config3GPP2, has3GPP2 := tlv.Value(tlvs, 0x11)
	switch mode {
	case WMSMessageModeGW:
		if has3GPP2 {
			return errors.New("parsing QMI WMS broadcast config indication: GW mode contains 3GPP2 config")
		}
		if !has3GPP {
			*c = config
			return nil
		}
		var err error
		config.Active, config.Channels3GPP, err = decodeWMS3GPPBroadcastConfig(config3GPP)
		if err != nil {
			return err
		}
	case WMSMessageModeCDMA:
		if has3GPP {
			return errors.New("parsing QMI WMS broadcast config indication: CDMA mode contains 3GPP config")
		}
		if !has3GPP2 {
			*c = config
			return nil
		}
		var err error
		config.Active, config.Channels3GPP2, err = decodeWMS3GPP2BroadcastConfig(config3GPP2)
		if err != nil {
			return err
		}
	}
	config.ConfigKnown = true
	*c = config
	return nil
}

func encodeWMS3GPPBroadcastChannels(channels []WMS3GPPBroadcastChannel) ([]byte, error) {
	if len(channels) > wmsBroadcastChannelMax {
		return nil, fmt.Errorf("3GPP broadcast channel count %d exceeds %d", len(channels), wmsBroadcastChannelMax)
	}
	value := make([]byte, 2, 2+len(channels)*5)
	binary.LittleEndian.PutUint16(value, uint16(len(channels)))
	for i, channel := range channels {
		if channel.Start > channel.End {
			return nil, fmt.Errorf("3GPP broadcast channel %d starts at %d after end %d", i, channel.Start, channel.End)
		}
		value = binary.LittleEndian.AppendUint16(value, channel.Start)
		value = binary.LittleEndian.AppendUint16(value, channel.End)
		value = append(value, boolByte(channel.Selected))
	}
	return value, nil
}

func encodeWMS3GPP2BroadcastChannels(channels []WMS3GPP2BroadcastChannel) ([]byte, error) {
	if len(channels) > wmsBroadcastChannelMax {
		return nil, fmt.Errorf("3GPP2 broadcast channel count %d exceeds %d", len(channels), wmsBroadcastChannelMax)
	}
	value := make([]byte, 2, 2+len(channels)*5)
	binary.LittleEndian.PutUint16(value, uint16(len(channels)))
	for i, channel := range channels {
		if channel.Language > WMSLanguageHebrew {
			return nil, fmt.Errorf("3GPP2 broadcast channel %d has language %d outside the supported range", i, channel.Language)
		}
		value = binary.LittleEndian.AppendUint16(value, uint16(channel.ServiceCategory))
		value = binary.LittleEndian.AppendUint16(value, uint16(channel.Language))
		value = append(value, boolByte(channel.Selected))
	}
	return value, nil
}

func decodeWMS3GPPBroadcastConfig(value []byte) (bool, []WMS3GPPBroadcastChannel, error) {
	active, count, entries, err := decodeWMSBroadcastHeader(value)
	if err != nil {
		return false, nil, fmt.Errorf("parsing QMI WMS 3GPP broadcast configuration: %w", err)
	}
	channels := make([]WMS3GPPBroadcastChannel, count)
	for i := range count {
		selected, err := decodeWMSBool(entries[4])
		if err != nil {
			return false, nil, fmt.Errorf("parsing QMI WMS 3GPP broadcast channel %d selection: %w", i, err)
		}
		channels[i] = WMS3GPPBroadcastChannel{
			Start:    binary.LittleEndian.Uint16(entries),
			End:      binary.LittleEndian.Uint16(entries[2:]),
			Selected: selected,
		}
		if channels[i].Start > channels[i].End {
			return false, nil, fmt.Errorf("parsing QMI WMS 3GPP broadcast channel %d: start %d is after end %d", i, channels[i].Start, channels[i].End)
		}
		entries = entries[5:]
	}
	return active, channels, nil
}

func decodeWMS3GPP2BroadcastConfig(value []byte) (bool, []WMS3GPP2BroadcastChannel, error) {
	active, count, entries, err := decodeWMSBroadcastHeader(value)
	if err != nil {
		return false, nil, fmt.Errorf("parsing QMI WMS 3GPP2 broadcast configuration: %w", err)
	}
	channels := make([]WMS3GPP2BroadcastChannel, count)
	for i := range count {
		language := WMSLanguage(binary.LittleEndian.Uint16(entries[2:]))
		if language > WMSLanguageHebrew {
			return false, nil, fmt.Errorf("parsing QMI WMS 3GPP2 broadcast channel %d: language %d is outside the supported range", i, language)
		}
		selected, err := decodeWMSBool(entries[4])
		if err != nil {
			return false, nil, fmt.Errorf("parsing QMI WMS 3GPP2 broadcast channel %d selection: %w", i, err)
		}
		channels[i] = WMS3GPP2BroadcastChannel{
			ServiceCategory: WMSServiceCategory(binary.LittleEndian.Uint16(entries)),
			Language:        language,
			Selected:        selected,
		}
		entries = entries[5:]
	}
	return active, channels, nil
}

func decodeWMSBroadcastHeader(value []byte) (bool, int, []byte, error) {
	if len(value) < 3 {
		return false, 0, nil, fmt.Errorf("TLV length %d is shorter than 3", len(value))
	}
	active, err := decodeWMSBool(value[0])
	if err != nil {
		return false, 0, nil, fmt.Errorf("activation: %w", err)
	}
	count := int(binary.LittleEndian.Uint16(value[1:]))
	if count > wmsBroadcastChannelMax {
		return false, 0, nil, fmt.Errorf("channel count %d exceeds %d", count, wmsBroadcastChannelMax)
	}
	want := 3 + count*5
	if len(value) != want {
		return false, 0, nil, fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	return active, count, value[3:], nil
}

func decodeWMSBool(value byte) (bool, error) {
	if value > 1 {
		return false, fmt.Errorf("boolean value %d, want 0 or 1", value)
	}
	return value == 1, nil
}

func validateWMSMessageMode(mode WMSMessageMode) error {
	if mode != WMSMessageModeCDMA && mode != WMSMessageModeGW {
		return fmt.Errorf("message mode %d is outside the supported range", mode)
	}
	return nil
}
