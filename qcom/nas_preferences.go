package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasPreferredNetworksMax       = 85
	nasStaticPreferredNetworksMax = 40
)

// NASPSAttachAction requests an immediate packet-domain attach or detach.
type NASPSAttachAction uint8

const (
	NASPSAttach NASPSAttachAction = 1 + iota
	NASPSDetach
)

// NASPLMNAccessTechnology is the preferred RAT mask stored with one PLMN.
type NASPLMNAccessTechnology uint16

const (
	NASPLMNAccessGSMCompact NASPLMNAccessTechnology = 1 << 6
	NASPLMNAccessGSM        NASPLMNAccessTechnology = 1 << 7
	NASPLMNAccessNGRAN      NASPLMNAccessTechnology = 1 << 11
	NASPLMNAccessEUTRAN     NASPLMNAccessTechnology = 1 << 14
	NASPLMNAccessUTRAN      NASPLMNAccessTechnology = 1 << 15
	NASPLMNAccessAll        NASPLMNAccessTechnology = 0xFFFF
)

// NASPreferredNetwork is one ordered user or static preferred PLMN entry.
type NASPreferredNetwork struct {
	PLMN             NASPLMN
	AccessTechnology NASPLMNAccessTechnology
}

// NASPreferredNetworks contains user-controlled and modem-provided static
// preferred PLMN lists.
type NASPreferredNetworks struct {
	Networks    []NASPreferredNetwork
	Known       bool
	Static      []NASPreferredNetwork
	StaticKnown bool
}

// NASPreferredNetworksConfig updates the user-controlled preferred PLMN list.
// A nil Networks slice omits the list; an empty non-nil slice sends an empty
// list. ClearPrevious is optional because some modems append by default.
type NASPreferredNetworksConfig struct {
	Networks      []NASPreferredNetwork
	ClearPrevious *bool
}

// NASTechnologyPreference is the legacy NAS radio-technology bitmask. New code
// should normally use NASModePreference through SystemSelectionPreference.
type NASTechnologyPreference uint16

const (
	NASTechnology3GPP2 NASTechnologyPreference = 1 << iota
	NASTechnology3GPP
	NASTechnologyAMPSOrGSM
	NASTechnologyCDMAOrWCDMA
	NASTechnologyHDR
	NASTechnologyLTE
)

// NASPreferenceDuration controls the lifetime of a legacy technology setting.
type NASPreferenceDuration uint8

const (
	NASPreferencePermanent NASPreferenceDuration = iota
	NASPreferencePowerCycle
	NASPreferenceOneCall
	NASPreferenceOneCallOrTime
	NASPreferenceInternalOneCall1
	NASPreferenceInternalOneCall2
	NASPreferenceInternalOneCall3
)

// NASTechnologyPreferences contains active and persistent legacy preferences.
type NASTechnologyPreferences struct {
	Active          NASTechnologyPreference
	Duration        NASPreferenceDuration
	Persistent      NASTechnologyPreference
	PersistentKnown bool
}

// NASAttachDetach requests an immediate packet-domain attach or detach.
func (c *Client) NASAttachDetach(ctx context.Context, action NASPSAttachAction) error {
	if action < NASPSAttach || action > NASPSDetach {
		return fmt.Errorf("changing QMI NAS packet attach state: action %d is out of range", action)
	}
	req := nasEmptyRequest(0, 0, DefaultRequestTimeout, MessageNASAttachDetach)
	req.TLVs = tlv.TLVs{tlv.Uint(0x10, uint8(action))}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("changing QMI NAS packet attach state: %w", err)
	}
	return nil
}

// PreferredNetworks returns user and static preferred PLMN lists.
func (c *Client) PreferredNetworks(ctx context.Context) (NASPreferredNetworks, error) {
	var result NASPreferredNetworks
	if err := c.nasRead(ctx, MessageNASGetPreferredNetworks, &result); err != nil {
		return NASPreferredNetworks{}, fmt.Errorf("reading QMI NAS preferred networks: %w", err)
	}
	return result, nil
}

// SetPreferredNetworks updates the user-controlled preferred PLMN list.
func (c *Client) SetPreferredNetworks(ctx context.Context, config NASPreferredNetworksConfig) error {
	tlvs, err := config.MarshalTLVs()
	if err != nil {
		return err
	}
	req := nasEmptyRequest(0, 0, DefaultRequestTimeout, MessageNASSetPreferredNetworks)
	req.TLVs = tlvs
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI NAS preferred networks: %w", err)
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get Preferred Networks response TLVs.
func (n *NASPreferredNetworks) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*n = NASPreferredNetworks{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		parsed, err := decodeNASPreferredNetworkList(value, nasPreferredNetworksMax)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS preferred networks: %w", err)
		}
		n.Networks = parsed
		n.Known = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodeNASPreferredNetworkList(value, nasStaticPreferredNetworksMax)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS static preferred networks: %w", err)
		}
		n.Static = parsed
		n.StaticKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if err := applyNASMNCDigitStatus(n.Networks, value); err != nil {
			return fmt.Errorf("parsing QMI NAS preferred-network MNC digits: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if err := applyNASMNCDigitStatus(n.Static, value); err != nil {
			return fmt.Errorf("parsing QMI NAS static preferred-network MNC digits: %w", err)
		}
	}
	return nil
}

// TechnologyPreferences returns the legacy active and persistent RAT policy.
func (c *Client) TechnologyPreferences(ctx context.Context) (NASTechnologyPreferences, error) {
	var result NASTechnologyPreferences
	if err := c.nasRead(ctx, MessageNASGetTechnologyPreference, &result); err != nil {
		return NASTechnologyPreferences{}, fmt.Errorf("reading QMI NAS technology preference: %w", err)
	}
	return result, nil
}

// SetTechnologyPreference changes the legacy RAT policy.
func (c *Client) SetTechnologyPreference(
	ctx context.Context,
	preference NASTechnologyPreference,
	duration NASPreferenceDuration,
) error {
	if duration > NASPreferenceInternalOneCall3 {
		return fmt.Errorf("setting QMI NAS technology preference: duration %d is out of range", duration)
	}
	value := binary.LittleEndian.AppendUint16(nil, uint16(preference))
	value = append(value, byte(duration))
	req := nasEmptyRequest(0, 0, DefaultRequestTimeout, MessageNASSetTechnologyPreference)
	req.TLVs = tlv.TLVs{tlv.Bytes(0x01, value)}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI NAS technology preference: %w", err)
	}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get Technology Preference response TLVs.
func (p *NASTechnologyPreferences) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*p = NASTechnologyPreferences{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI NAS technology preference: active preference TLV missing")
	}
	if len(value) != 3 {
		return fmt.Errorf("parsing QMI NAS technology preference: active preference TLV length %d, want 3", len(value))
	}
	p.Active = NASTechnologyPreference(binary.LittleEndian.Uint16(value[:2]))
	p.Duration = NASPreferenceDuration(value[2])
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS technology preference: persistent preference TLV length %d, want 2", len(value))
		}
		p.Persistent = NASTechnologyPreference(binary.LittleEndian.Uint16(value))
		p.PersistentKnown = true
	}
	return nil
}

// MarshalTLVs encodes preferred-network fields.
func (config NASPreferredNetworksConfig) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	if config.Networks != nil {
		if len(config.Networks) > nasPreferredNetworksMax {
			return nil, fmt.Errorf("setting QMI NAS preferred networks: network count %d exceeds %d", len(config.Networks), nasPreferredNetworksMax)
		}
		for i, network := range config.Networks {
			if network.PLMN.MCC > 999 || network.PLMN.MNC > 999 {
				return nil, fmt.Errorf("setting QMI NAS preferred networks: network %d PLMN %d/%d is out of range", i, network.PLMN.MCC, network.PLMN.MNC)
			}
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, encodeNASPreferredNetworkList(config.Networks)))
		if statuses := encodeNASMNCDigitStatus(config.Networks); statuses != nil {
			tlvs = append(tlvs, tlv.Bytes(0x11, statuses))
		}
	}
	if config.ClearPrevious != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*config.ClearPrevious)))
	}
	return tlvs, nil
}

func encodeNASPreferredNetworkList(networks []NASPreferredNetwork) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(networks)))
	for _, network := range networks {
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MNC)
		value = binary.LittleEndian.AppendUint16(value, uint16(network.AccessTechnology))
	}
	return value
}

func decodeNASPreferredNetworkList(value []byte, maxCount int) ([]NASPreferredNetwork, error) {
	if len(value) < 2 {
		return nil, errors.New("network count is missing")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > maxCount {
		return nil, fmt.Errorf("network count %d exceeds %d", count, maxCount)
	}
	want := 2 + count*6
	if len(value) != want {
		return nil, fmt.Errorf("network list length %d, want %d for %d entries", len(value), want, count)
	}
	networks := make([]NASPreferredNetwork, count)
	for i := range count {
		offset := 2 + i*6
		networks[i] = NASPreferredNetwork{
			PLMN: NASPLMN{
				MCC: binary.LittleEndian.Uint16(value[offset : offset+2]),
				MNC: binary.LittleEndian.Uint16(value[offset+2 : offset+4]),
			},
			AccessTechnology: NASPLMNAccessTechnology(binary.LittleEndian.Uint16(value[offset+4 : offset+6])),
		}
	}
	return networks, nil
}

func encodeNASMNCDigitStatus(networks []NASPreferredNetwork) []byte {
	count := 0
	for _, network := range networks {
		if network.PLMN.MNCThreeDigitsKnown {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	value := make([]byte, 1, 1+count*5)
	value[0] = byte(count)
	for _, network := range networks {
		if !network.PLMN.MNCThreeDigitsKnown {
			continue
		}
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MNC)
		value = append(value, boolByte(network.PLMN.MNCThreeDigits))
	}
	return value
}

func applyNASMNCDigitStatus(networks []NASPreferredNetwork, value []byte) error {
	if len(value) == 0 {
		return errors.New("status count is missing")
	}
	count := int(value[0])
	if count > nasPreferredNetworksMax {
		return fmt.Errorf("status count %d exceeds %d", count, nasPreferredNetworksMax)
	}
	want := 1 + count*5
	if len(value) != want {
		return fmt.Errorf("status length %d, want %d for %d entries", len(value), want, count)
	}
	for i := range count {
		offset := 1 + i*5
		mcc := binary.LittleEndian.Uint16(value[offset : offset+2])
		mnc := binary.LittleEndian.Uint16(value[offset+2 : offset+4])
		index := slices.IndexFunc(networks, func(network NASPreferredNetwork) bool {
			return network.PLMN.MCC == mcc && network.PLMN.MNC == mnc
		})
		if index < 0 {
			continue
		}
		networks[index].PLMN.MNCThreeDigits = value[offset+4] != 0
		networks[index].PLMN.MNCThreeDigitsKnown = true
	}
	return nil
}
