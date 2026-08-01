package qcom

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	uimControlKeyMax          = 16
	uimPersonalizationMax     = 20
	uimOtherSlotsMax          = 5
	uimRemoteUnlockDataMax    = 1024
	uimRemoteUnlockDataExtMax = 4096
)

// UIMConfigurationMask selects fields returned by UIMConfiguration. A zero
// mask omits the request TLV and asks the modem for all supported fields.
type UIMConfigurationMask uint32

const (
	UIMConfigurationAutomaticSelection UIMConfigurationMask = 1 << iota
	UIMConfigurationPersonalizationStatus
	UIMConfigurationHaltSubscription
)

// UIMPersonalizationFeature identifies a SIM or R-UIM network-lock category.
type UIMPersonalizationFeature uint8

const (
	UIMPersonalizationGWNetwork UIMPersonalizationFeature = iota
	UIMPersonalizationGWNetworkSubset
	UIMPersonalizationGWServiceProvider
	UIMPersonalizationGWCorporate
	UIMPersonalizationGWUIM
	UIMPersonalization1XNetworkType1
	UIMPersonalization1XNetworkType2
	UIMPersonalization1XHRPD
	UIMPersonalization1XServiceProvider
	UIMPersonalization1XCorporate
	UIMPersonalization1XRUIM
	UIMPersonalizationGWServiceProviderName
	UIMPersonalizationGWSPEHPLMN
	UIMPersonalizationGWICCID
	UIMPersonalizationGWIMPI
	UIMPersonalizationGWNetworkSubsetServiceProvider
	UIMPersonalizationGWCarrier
)

// UIMPersonalizationStatus contains remaining attempts for one active lock.
type UIMPersonalizationStatus struct {
	Feature        UIMPersonalizationFeature
	VerifyRetries  uint8
	UnblockRetries uint8
}

// UIMConfiguration contains common UIM provisioning and personalization
// settings. Known fields distinguish omitted response TLVs from false values.
type UIMConfiguration struct {
	AutomaticSelection             bool
	AutomaticSelectionKnown        bool
	PersonalizationStatus          []UIMPersonalizationStatus
	PersonalizationStatusKnown     bool
	HaltSubscription               bool
	HaltSubscriptionKnown          bool
	OtherSlotsPersonalization      [][]UIMPersonalizationStatus
	OtherSlotsPersonalizationKnown bool
}

// UnmarshalTLVs parses QMI UIM Get Configuration response TLVs.
func (c *UIMConfiguration) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*c = UIMConfiguration{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		parsed, err := decodeUIMConfigurationBool(value)
		if err != nil {
			return fmt.Errorf("parsing QMI UIM automatic selection: %w", err)
		}
		c.AutomaticSelection = parsed
		c.AutomaticSelectionKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodeUIMPersonalizationStatuses(value)
		if err != nil {
			return fmt.Errorf("parsing QMI UIM configuration personalization status: %w", err)
		}
		c.PersonalizationStatus = parsed
		c.PersonalizationStatusKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		parsed, err := decodeUIMConfigurationBool(value)
		if err != nil {
			return fmt.Errorf("parsing QMI UIM halt subscription: %w", err)
		}
		c.HaltSubscription = parsed
		c.HaltSubscriptionKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		parsed, err := decodeUIMOtherSlotsPersonalization(value)
		if err != nil {
			return fmt.Errorf("parsing QMI UIM configuration other-slot personalization: %w", err)
		}
		c.OtherSlotsPersonalization = parsed
		c.OtherSlotsPersonalizationKnown = true
	}
	return nil
}

// UIMDepersonalizationOperation selects how a blocked personalization feature
// is changed.
type UIMDepersonalizationOperation uint8

const (
	UIMDepersonalizationDeactivate UIMDepersonalizationOperation = iota
	UIMDepersonalizationUnblock
)

// UIMDepersonalizationRequest deactivates or unblocks one network-lock feature.
type UIMDepersonalizationRequest struct {
	Feature    UIMPersonalizationFeature
	Operation  UIMDepersonalizationOperation
	ControlKey string
}

// UIMDepersonalizationResult contains retry counters returned after an
// unsuccessful depersonalization attempt.
type UIMDepersonalizationResult struct {
	VerifyRetries  uint8
	UnblockRetries uint8
	RetriesKnown   bool
}

// UIMConfiguration reads common provisioning and network-lock settings.
func (c *Client) UIMConfiguration(ctx context.Context, mask UIMConfigurationMask) (UIMConfiguration, error) {
	var requestTLVs tlv.TLVs
	if mask != 0 {
		requestTLVs = append(requestTLVs, tlv.Uint(0x10, uint32(mask)))
	}
	resp, err := c.request(ctx, MessageGetConfiguration, requestTLVs)
	if err != nil {
		return UIMConfiguration{}, fmt.Errorf("reading QMI UIM configuration: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return UIMConfiguration{}, fmt.Errorf("reading QMI UIM configuration: %w", err)
	}
	var config UIMConfiguration
	if err := config.UnmarshalTLVs(resp.TLVs); err != nil {
		return UIMConfiguration{}, err
	}
	return config, nil
}

// Depersonalize deactivates or unblocks one SIM network-lock category.
func (c *Client) Depersonalize(ctx context.Context, req UIMDepersonalizationRequest) (UIMDepersonalizationResult, error) {
	if req.Feature > UIMPersonalizationGWCarrier {
		return UIMDepersonalizationResult{}, fmt.Errorf("depersonalizing QMI UIM: feature %d is out of range", req.Feature)
	}
	if req.Operation > UIMDepersonalizationUnblock {
		return UIMDepersonalizationResult{}, fmt.Errorf("depersonalizing QMI UIM: operation %d is out of range", req.Operation)
	}
	if err := validateUIMControlKey(req.ControlKey); err != nil {
		return UIMDepersonalizationResult{}, fmt.Errorf("depersonalizing QMI UIM: %w", err)
	}

	value := []byte{byte(req.Feature), byte(req.Operation), byte(len(req.ControlKey))}
	value = append(value, req.ControlKey...)
	resp, err := c.request(ctx, MessageDepersonalization, tlv.TLVs{
		tlv.Bytes(0x01, value),
		tlv.Uint(0x10, c.slot),
	})
	if err != nil {
		return UIMDepersonalizationResult{}, fmt.Errorf("depersonalizing QMI UIM: %w", err)
	}
	var result UIMDepersonalizationResult
	decodeErr := result.UnmarshalTLVs(resp.TLVs)
	if err := resultOK(resp); err != nil {
		return result, fmt.Errorf("depersonalizing QMI UIM: %w", err)
	}
	if decodeErr != nil {
		return UIMDepersonalizationResult{}, decodeErr
	}
	return result, nil
}

// RemoteUnlock submits an opaque carrier SimLock blob. Data up to 1024 bytes
// uses the original field; larger data uses the extended field.
func (c *Client) RemoteUnlock(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return errors.New("remotely unlocking QMI UIM: SimLock data is empty")
	}
	if len(data) > uimRemoteUnlockDataExtMax {
		return fmt.Errorf("remotely unlocking QMI UIM: SimLock data length %d exceeds %d", len(data), uimRemoteUnlockDataExtMax)
	}

	typ := byte(0x10)
	if len(data) > uimRemoteUnlockDataMax {
		typ = 0x12
	}
	value, err := qmiLength16Bytes(data).MarshalBinary()
	if err != nil {
		return fmt.Errorf("remotely unlocking QMI UIM: %w", err)
	}
	resp, err := c.request(ctx, MessageRemoteUnlock, tlv.TLVs{tlv.Bytes(typ, value)})
	if err != nil {
		return fmt.Errorf("remotely unlocking QMI UIM: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("remotely unlocking QMI UIM: %w", err)
	}
	return nil
}

func decodeUIMConfigurationBool(value []byte) (bool, error) {
	if len(value) != 1 {
		return false, fmt.Errorf("boolean TLV length %d, want 1", len(value))
	}
	return value[0] != 0, nil
}

func decodeUIMPersonalizationStatuses(value []byte) ([]UIMPersonalizationStatus, error) {
	if len(value) == 0 {
		return nil, errors.New("status count is missing")
	}
	count := int(value[0])
	if count > uimPersonalizationMax {
		return nil, fmt.Errorf("status count %d exceeds %d", count, uimPersonalizationMax)
	}
	want := 1 + count*3
	if len(value) != want {
		return nil, fmt.Errorf("status length %d, want %d for %d entries", len(value), want, count)
	}
	statuses := make([]UIMPersonalizationStatus, count)
	for i := range count {
		offset := 1 + i*3
		statuses[i] = UIMPersonalizationStatus{
			Feature:        UIMPersonalizationFeature(value[offset]),
			VerifyRetries:  value[offset+1],
			UnblockRetries: value[offset+2],
		}
	}
	return statuses, nil
}

func decodeUIMOtherSlotsPersonalization(value []byte) ([][]UIMPersonalizationStatus, error) {
	if len(value) == 0 {
		return nil, errors.New("slot count is missing")
	}
	count := int(value[0])
	if count > uimOtherSlotsMax {
		return nil, fmt.Errorf("slot count %d exceeds %d", count, uimOtherSlotsMax)
	}
	offset := 1
	statuses := make([][]UIMPersonalizationStatus, count)
	for i := range count {
		if offset >= len(value) {
			return nil, fmt.Errorf("slot %d status count is missing", i+1)
		}
		featureCount := int(value[offset])
		if featureCount > uimPersonalizationMax {
			return nil, fmt.Errorf("slot %d status count %d exceeds %d", i+1, featureCount, uimPersonalizationMax)
		}
		length := 1 + featureCount*3
		if offset+length > len(value) {
			return nil, fmt.Errorf("slot %d status data is truncated", i+1)
		}
		decoded, err := decodeUIMPersonalizationStatuses(value[offset : offset+length])
		if err != nil {
			return nil, fmt.Errorf("slot %d: %w", i+1, err)
		}
		statuses[i] = decoded
		offset += length
	}
	if offset != len(value) {
		return nil, fmt.Errorf("status data has %d trailing bytes", len(value)-offset)
	}
	return statuses, nil
}

func validateUIMControlKey(key string) error {
	if key == "" {
		return errors.New("control key is empty")
	}
	if len(key) > uimControlKeyMax {
		return fmt.Errorf("control key length %d exceeds %d", len(key), uimControlKeyMax)
	}
	for i := range len(key) {
		if key[i] > 0x7F {
			return errors.New("control key contains a non-ASCII character")
		}
	}
	return nil
}

// UnmarshalTLVs decodes UIM depersonalization retry counters.
func (r *UIMDepersonalizationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		*r = UIMDepersonalizationResult{}
		return nil
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI UIM depersonalization retries: TLV length %d, want 2", len(value))
	}
	*r = UIMDepersonalizationResult{
		VerifyRetries:  value[0],
		UnblockRetries: value[1],
		RetriesKnown:   true,
	}
	return nil
}
