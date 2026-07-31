package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wdsMaxPDNThrottleEntries = 8

// WDSPDNThrottleEntry describes IPv4 and IPv6 throttling for one APN.
type WDSPDNThrottleEntry struct {
	APN                       string
	IPv4Throttled             bool
	IPv6Throttled             bool
	IPv4RemainingMilliseconds uint32
	IPv6RemainingMilliseconds uint32
}

// WDSPDNThrottleExtendedEntry adds non-IP throttling for one APN.
type WDSPDNThrottleExtendedEntry struct {
	WDSPDNThrottleEntry
	NonIPThrottled             bool
	NonIPRemainingMilliseconds uint32
}

// WDSPDNThrottleAdditionalEntry contains network context for a throttled APN.
type WDSPDNThrottleAdditionalEntry struct {
	APN               string
	Emergency         bool
	BlockedOnAllPLMNs bool
	ThrottledPLMN     [3]byte
}

// WDSPDNThrottleInfo contains each optional generation of throttle data.
type WDSPDNThrottleInfo struct {
	Entries            []WDSPDNThrottleEntry
	EntriesKnown       bool
	Extended           []WDSPDNThrottleExtendedEntry
	ExtendedKnown      bool
	Additional         []WDSPDNThrottleAdditionalEntry
	AdditionalKnown    bool
	TransactionID      uint16
	TransactionIDKnown bool
}

// WDSGetPDNThrottleInfoRequest encodes Get PDN Throttle Info.
type WDSGetPDNThrottleInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	NetworkType   WDSDataSystemNetworkType
}

// Request converts the technology selector into a QMI request.
func (r WDSGetPDNThrottleInfoRequest) Request() Request {
	return Request{
		Service:       ServiceWDS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageWDSGetPDNThrottleInfo,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint8(r.NetworkType))},
	}
}

// WDSGetPDNThrottleInfoResponse contains the parsed throttle data.
type WDSGetPDNThrottleInfoResponse struct {
	Info WDSPDNThrottleInfo
}

// UnmarshalTLVs parses the base, extended, and additional throttle arrays.
func (r *WDSGetPDNThrottleInfoResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WDSGetPDNThrottleInfoResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		entries, err := decodeWDSPDNThrottleEntries(value)
		if err != nil {
			return err
		}
		r.Info.Entries, r.Info.EntriesKnown = entries, true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		entries, err := decodeWDSPDNThrottleExtendedEntries(value)
		if err != nil {
			return err
		}
		r.Info.Extended, r.Info.ExtendedKnown = entries, true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		entries, err := decodeWDSPDNThrottleAdditionalEntries(value)
		if err != nil {
			return err
		}
		r.Info.Additional, r.Info.AdditionalKnown = entries, true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WDS PDN throttle info: transaction ID TLV length %d, want 2", len(value))
		}
		r.Info.TransactionID = binary.LittleEndian.Uint16(value)
		r.Info.TransactionIDKnown = true
	}
	return nil
}

// WDSPDNThrottleInfo returns throttling state for the selected technology.
func (c *Client) WDSPDNThrottleInfo(ctx context.Context, networkType WDSDataSystemNetworkType) (WDSPDNThrottleInfo, error) {
	if err := validateWDSDataSystemNetworkType(networkType); err != nil {
		return WDSPDNThrottleInfo{}, fmt.Errorf("querying QMI WDS PDN throttle info: %w", err)
	}
	req := WDSGetPDNThrottleInfoRequest{
		Timeout:     DefaultRequestTimeout,
		NetworkType: networkType,
	}.Request()
	resp, err := c.wdsControlRequest(ctx, req.MessageID, req.TLVs)
	if err != nil {
		return WDSPDNThrottleInfo{}, fmt.Errorf("querying QMI WDS PDN throttle info: %w", err)
	}
	var parsed WDSGetPDNThrottleInfoResponse
	if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
		return WDSPDNThrottleInfo{}, err
	}
	return parsed.Info, nil
}

func validateWDSDataSystemNetworkType(networkType WDSDataSystemNetworkType) error {
	switch networkType {
	case WDSDataSystem3GPP, WDSDataSystem3GPP2:
		return nil
	default:
		return fmt.Errorf("network type %d is out of range", networkType)
	}
}

func decodeWDSPDNThrottleEntries(value []byte) ([]WDSPDNThrottleEntry, error) {
	count, rest, err := decodeWDSPDNThrottleCount(value)
	if err != nil {
		return nil, fmt.Errorf("parsing QMI WDS base PDN throttle entries: %w", err)
	}
	entries := make([]WDSPDNThrottleEntry, 0, count)
	for range count {
		if len(rest) < 11 {
			return nil, errors.New("parsing QMI WDS PDN throttle info: base entry is truncated")
		}
		apnLength := int(rest[10])
		if apnLength > wdsAPNMaxLength || len(rest) < 11+apnLength {
			return nil, errors.New("parsing QMI WDS PDN throttle info: base APN is truncated or too long")
		}
		entries = append(entries, WDSPDNThrottleEntry{
			IPv4Throttled:             rest[0] != 0,
			IPv6Throttled:             rest[1] != 0,
			IPv4RemainingMilliseconds: binary.LittleEndian.Uint32(rest[2:6]),
			IPv6RemainingMilliseconds: binary.LittleEndian.Uint32(rest[6:10]),
			APN:                       string(rest[11 : 11+apnLength]),
		})
		rest = rest[11+apnLength:]
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("parsing QMI WDS PDN throttle info: base TLV has %d trailing bytes", len(rest))
	}
	return entries, nil
}

func decodeWDSPDNThrottleExtendedEntries(value []byte) ([]WDSPDNThrottleExtendedEntry, error) {
	count, rest, err := decodeWDSPDNThrottleCount(value)
	if err != nil {
		return nil, fmt.Errorf("parsing QMI WDS extended PDN throttle entries: %w", err)
	}
	entries := make([]WDSPDNThrottleExtendedEntry, 0, count)
	for range count {
		if len(rest) < 16 {
			return nil, errors.New("parsing QMI WDS PDN throttle info: extended entry is truncated")
		}
		apnLength := int(rest[15])
		if apnLength > wdsAPNMaxLength || len(rest) < 16+apnLength {
			return nil, errors.New("parsing QMI WDS PDN throttle info: extended APN is truncated or too long")
		}
		entries = append(entries, WDSPDNThrottleExtendedEntry{
			WDSPDNThrottleEntry: WDSPDNThrottleEntry{
				IPv4Throttled:             rest[0] != 0,
				IPv6Throttled:             rest[1] != 0,
				IPv4RemainingMilliseconds: binary.LittleEndian.Uint32(rest[3:7]),
				IPv6RemainingMilliseconds: binary.LittleEndian.Uint32(rest[7:11]),
				APN:                       string(rest[16 : 16+apnLength]),
			},
			NonIPThrottled:             rest[2] != 0,
			NonIPRemainingMilliseconds: binary.LittleEndian.Uint32(rest[11:15]),
		})
		rest = rest[16+apnLength:]
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("parsing QMI WDS PDN throttle info: extended TLV has %d trailing bytes", len(rest))
	}
	return entries, nil
}

func decodeWDSPDNThrottleAdditionalEntries(value []byte) ([]WDSPDNThrottleAdditionalEntry, error) {
	count, rest, err := decodeWDSPDNThrottleCount(value)
	if err != nil {
		return nil, fmt.Errorf("parsing QMI WDS additional PDN throttle entries: %w", err)
	}
	entries := make([]WDSPDNThrottleAdditionalEntry, 0, count)
	for range count {
		if len(rest) < 6 {
			return nil, errors.New("parsing QMI WDS PDN throttle info: additional entry is truncated")
		}
		apnLength := int(rest[5])
		if apnLength > wdsAPNMaxLength || len(rest) < 6+apnLength {
			return nil, errors.New("parsing QMI WDS PDN throttle info: additional APN is truncated or too long")
		}
		entries = append(entries, WDSPDNThrottleAdditionalEntry{
			Emergency:         rest[0] != 0,
			BlockedOnAllPLMNs: rest[1] != 0,
			ThrottledPLMN:     [3]byte(rest[2:5]),
			APN:               string(rest[6 : 6+apnLength]),
		})
		rest = rest[6+apnLength:]
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("parsing QMI WDS PDN throttle info: additional TLV has %d trailing bytes", len(rest))
	}
	return entries, nil
}

func decodeWDSPDNThrottleCount(value []byte) (int, []byte, error) {
	if len(value) == 0 {
		return 0, nil, errors.New("count is missing")
	}
	count := int(value[0])
	if count > wdsMaxPDNThrottleEntries {
		return 0, nil, fmt.Errorf("count %d exceeds maximum %d", count, wdsMaxPDNThrottleEntries)
	}
	return count, value[1:], nil
}
