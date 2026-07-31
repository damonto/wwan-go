package qcom

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// VoiceTTYMode selects the modem and UI teletypewriter mode.
type VoiceTTYMode uint8

const (
	VoiceTTYFull VoiceTTYMode = iota
	VoiceTTYVCO
	VoiceTTYHCO
	VoiceTTYOff
)

// VoiceDomainPreference selects circuit-switched or packet-switched voice.
type VoiceDomainPreference uint8

const (
	VoiceDomainCSOnly VoiceDomainPreference = iota
	VoiceDomainPSOnly
	VoiceDomainCSPreferred
	VoiceDomainPSPreferred
)

// VoicePrivacy selects the 3GPP2 voice privacy level.
type VoicePrivacy uint8

const (
	VoicePrivacyStandard VoicePrivacy = iota
	VoicePrivacyEnhanced
)

// VoiceConfig contains the commonly useful QMI Voice configuration values.
// Known fields distinguish an unsupported or omitted value from its zero value.
type VoiceConfig struct {
	AutoAnswer       bool
	AutoAnswerKnown  bool
	TTYMode          VoiceTTYMode
	TTYModeKnown     bool
	Privacy          VoicePrivacy
	PrivacyKnown     bool
	VoiceDomain      VoiceDomainPreference
	VoiceDomainKnown bool
	UITTYMode        VoiceTTYMode
	UITTYModeKnown   bool
}

// VoiceConfigUpdate selects common QMI Voice configuration fields to change.
// Nil fields are omitted so callers can update one setting without replacing
// unrelated modem state.
type VoiceConfigUpdate struct {
	AutoAnswer  *bool
	TTYMode     *VoiceTTYMode
	VoiceDomain *VoiceDomainPreference
	UITTYMode   *VoiceTTYMode
}

// VoiceSetConfig updates common QMI Voice configuration fields.
func (c *Client) VoiceSetConfig(ctx context.Context, update VoiceConfigUpdate) error {
	var request tlv.TLVs
	if update.AutoAnswer != nil {
		request = append(request, tlv.Uint(0x10, boolByte(*update.AutoAnswer)))
	}
	if update.TTYMode != nil {
		if *update.TTYMode > VoiceTTYOff {
			return fmt.Errorf("writing QMI Voice TTY mode: value %d is out of range", *update.TTYMode)
		}
		request = append(request, tlv.Uint(0x13, uint8(*update.TTYMode)))
	}
	if update.VoiceDomain != nil {
		if *update.VoiceDomain > VoiceDomainPSPreferred {
			return fmt.Errorf("writing QMI Voice domain: value %d is out of range", *update.VoiceDomain)
		}
		request = append(request, tlv.Uint(0x15, uint8(*update.VoiceDomain)))
	}
	if update.UITTYMode != nil {
		if *update.UITTYMode > VoiceTTYOff {
			return fmt.Errorf("writing QMI Voice UI TTY mode: value %d is out of range", *update.UITTYMode)
		}
		request = append(request, tlv.Uint(0x16, uint8(*update.UITTYMode)))
	}

	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceSetConfig, request)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		outcomes := []struct {
			kind byte
		}{
			{kind: 0x10},
			{kind: 0x13},
			{kind: 0x15},
			{kind: 0x16},
		}
		for _, outcome := range outcomes {
			value, ok := tlv.Value(resp.TLVs, outcome.kind)
			if !ok {
				continue
			}
			if len(value) != 1 {
				return fmt.Errorf("reading QMI Voice config outcome: TLV 0x%02X length %d, want 1", outcome.kind, len(value))
			}
			if value[0] != 0 {
				return fmt.Errorf("writing QMI Voice config TLV 0x%02X: modem outcome %d", outcome.kind, value[0])
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("writing QMI Voice configuration: %w", err)
	}
	return nil
}

// VoiceConfig reads the common QMI Voice configuration fields supported by the modem.
func (c *Client) VoiceConfig(ctx context.Context) (VoiceConfig, error) {
	request := tlv.TLVs{
		tlv.Uint(0x10, uint8(1)),
		tlv.Uint(0x13, uint8(1)),
		tlv.Uint(0x16, uint8(1)),
		tlv.Uint(0x18, uint8(1)),
		tlv.Uint(0x19, uint8(1)),
	}

	var config VoiceConfig
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceGetConfig, request)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return config.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return VoiceConfig{}, fmt.Errorf("reading QMI Voice configuration: %w", err)
	}
	return config, nil
}

// UnmarshalTLVs parses the common QMI Voice configuration fields.
func (config *VoiceConfig) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*config = VoiceConfig{}
	var autoAnswer uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x10, &autoAnswer, &config.AutoAnswerKnown); err != nil {
		return err
	}
	config.AutoAnswer = autoAnswer == 1

	var ttyMode uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x13, &ttyMode, &config.TTYModeKnown); err != nil {
		return err
	}
	config.TTYMode = VoiceTTYMode(ttyMode)

	var privacy uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x16, &privacy, &config.PrivacyKnown); err != nil {
		return err
	}
	config.Privacy = VoicePrivacy(privacy)

	var domain uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x17, &domain, &config.VoiceDomainKnown); err != nil {
		return err
	}
	config.VoiceDomain = VoiceDomainPreference(domain)

	var uiTTYMode uint8
	if err := parseVoiceOptionalUint8(tlvs, 0x18, &uiTTYMode, &config.UITTYModeKnown); err != nil {
		return err
	}
	config.UITTYMode = VoiceTTYMode(uiTTYMode)
	return nil
}

func parseVoiceOptionalUint8(tlvs tlv.TLVs, kind byte, dst *uint8, known *bool) error {
	value, ok := tlv.Value(tlvs, kind)
	if !ok {
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI Voice TLV 0x%02X: length %d, want 1", kind, len(value))
	}
	*dst = value[0]
	*known = true
	return nil
}

// VoiceSetPreferredPrivacy changes the 3GPP2 preferred privacy level.
func (c *Client) VoiceSetPreferredPrivacy(ctx context.Context, privacy VoicePrivacy) error {
	if privacy > VoicePrivacyEnhanced {
		return fmt.Errorf("setting QMI Voice preferred privacy: value %d is out of range", privacy)
	}
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, MessageVoiceSetPreferredPrivacy, tlv.TLVs{
			tlv.Uint(0x01, uint8(privacy)),
		})
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("setting QMI Voice preferred privacy: %w", err)
	}
	return nil
}

// VoiceSetALSLineSwitching enables or disables alternate-line switching.
func (c *Client) VoiceSetALSLineSwitching(ctx context.Context, allowed bool) error {
	if err := c.voiceSetALS(ctx, MessageVoiceSetALSLineSwitching, boolByte(allowed)); err != nil {
		return fmt.Errorf("setting QMI Voice alternate-line switching: %w", err)
	}
	return nil
}

// VoiceSelectALSLine selects the active alternate-service line.
func (c *Client) VoiceSelectALSLine(ctx context.Context, line VoiceALS) error {
	if line != VoiceALSLine1 && line != VoiceALSLine2 {
		return fmt.Errorf("selecting QMI Voice alternate line: value %d is out of range", line)
	}
	if err := c.voiceSetALS(ctx, MessageVoiceSelectALSLine, uint8(line)); err != nil {
		return fmt.Errorf("selecting QMI Voice alternate line: %w", err)
	}
	return nil
}

func (c *Client) voiceSetALS(ctx context.Context, id MessageID, value uint8) error {
	return c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, id, tlv.TLVs{tlv.Uint(0x01, value)})
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

// VoiceALSLineSwitching reports whether alternate-line switching is allowed.
func (c *Client) VoiceALSLineSwitching(ctx context.Context) (bool, error) {
	value, err := c.voiceALSValue(ctx, MessageVoiceALSLineSwitching)
	if err != nil {
		return false, fmt.Errorf("reading QMI Voice alternate-line switching: %w", err)
	}
	return value != 0, nil
}

// VoiceALSSelectedLine returns the active alternate-service line.
func (c *Client) VoiceALSSelectedLine(ctx context.Context) (VoiceALS, error) {
	value, err := c.voiceALSValue(ctx, MessageVoiceALSSelectedLine)
	if err != nil {
		return 0, fmt.Errorf("reading QMI Voice selected alternate line: %w", err)
	}
	line := VoiceALS(value)
	if line != VoiceALSLine1 && line != VoiceALSLine2 {
		return 0, fmt.Errorf("reading QMI Voice selected alternate line: value %d is out of range", value)
	}
	return line, nil
}

func (c *Client) voiceALSValue(ctx context.Context, id MessageID) (uint8, error) {
	var value uint8
	err := c.withServiceClient(ctx, ServiceVoice, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceVoice, clientID, id, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		encoded, ok := tlv.Value(resp.TLVs, 0x10)
		if !ok {
			return errors.New("value TLV missing")
		}
		if len(encoded) != 1 {
			return fmt.Errorf("value TLV length %d, want 1", len(encoded))
		}
		value = encoded[0]
		return nil
	})
	return value, err
}
