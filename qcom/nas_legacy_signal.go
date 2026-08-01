package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasMaxLegacySignalStrengths = 2
	nasMaxLegacyRSSIValues      = 7
	nasMaxLegacyECIOValues      = 6
	nasMaxLegacyErrorRates      = 16
)

// NASSignalStrengthRequestMask selects optional measurements returned by the
// deprecated Get Signal Strength command.
type NASSignalStrengthRequestMask uint16

const (
	NASSignalStrengthRequestRSSI NASSignalStrengthRequestMask = 1 << iota
	NASSignalStrengthRequestECIO
	NASSignalStrengthRequestIO
	NASSignalStrengthRequestSINR
	NASSignalStrengthRequestErrorRate
	NASSignalStrengthRequestRSRQ
	NASSignalStrengthRequestLTESNR
	NASSignalStrengthRequestLTERSRP

	NASSignalStrengthRequestAll = NASSignalStrengthRequestRSSI |
		NASSignalStrengthRequestECIO |
		NASSignalStrengthRequestIO |
		NASSignalStrengthRequestSINR |
		NASSignalStrengthRequestErrorRate |
		NASSignalStrengthRequestRSRQ |
		NASSignalStrengthRequestLTESNR |
		NASSignalStrengthRequestLTERSRP
)

// NASSignalStrength is one dBm measurement associated with a radio interface.
type NASSignalStrength struct {
	Strength       int8
	RadioInterface NASRadioInterface
}

// NASRSSIMeasurement contains the positive magnitude used by legacy NAS.
// Negating RSSI yields the conventional dBm value.
type NASRSSIMeasurement struct {
	RSSI           uint8
	RadioInterface NASRadioInterface
}

// NASECIOMeasurement contains legacy negative half-dB units. Multiplying ECIO
// by -0.5 yields the value in dB.
type NASECIOMeasurement struct {
	ECIO           uint8
	RadioInterface NASRadioInterface
}

// NASErrorRateMeasurement is one RAT-specific legacy error-rate value.
type NASErrorRateMeasurement struct {
	Rate           uint16
	RadioInterface NASRadioInterface
}

// NASRSRQMeasurement is one RSRQ value in dB.
type NASRSRQMeasurement struct {
	RSRQ           int8
	RadioInterface NASRadioInterface
}

// NASAbortRequest encodes the library-level NAS Abort command. The target must
// use the same NAS client ID as the operation being aborted.
type NASAbortRequest struct {
	ClientID            uint8
	TransactionID       uint16
	Timeout             time.Duration
	TargetTransactionID uint16
}

// Request converts the abort operation into a QMI NAS request.
func (r NASAbortRequest) Request() Request {
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASAbort,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, r.TargetTransactionID)},
	}
}

// NASGetSignalStrengthRequest encodes the deprecated Get Signal Strength
// command retained by the libqmi basic collection.
type NASGetSignalStrengthRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mask          NASSignalStrengthRequestMask
}

// Request converts the signal query into a QMI NAS request.
func (r NASGetSignalStrengthRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Mask != 0 {
		tlvs = append(tlvs, tlv.Uint(0x10, uint16(r.Mask)))
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASGetSignalStrength,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// NASSignalStrengthResult contains the mandatory current measurement and all
// optional legacy metrics requested by the caller.
type NASSignalStrengthResult struct {
	Current NASSignalStrength

	Strengths       []NASSignalStrength
	StrengthsKnown  bool
	RSSI            []NASRSSIMeasurement
	RSSIKnown       bool
	ECIO            []NASECIOMeasurement
	ECIOKnown       bool
	IO              int32
	IOKnown         bool
	SINRLevel       uint8
	SINRKnown       bool
	ErrorRates      []NASErrorRateMeasurement
	ErrorRatesKnown bool
	RSRQ            NASRSRQMeasurement
	RSRQKnown       bool
	LTESNR          int16
	LTESNRKnown     bool
	LTERSRP         int16
	LTERSRPKnown    bool
}

// UnmarshalTLVs parses a QMI NAS Get Signal Strength response.
func (r *NASSignalStrengthResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASSignalStrengthResult{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI NAS signal strength: current signal TLV missing")
	}
	if err := r.Current.UnmarshalBinary(value); err != nil {
		return fmt.Errorf("parsing QMI NAS current signal strength: %w", err)
	}

	if value, ok := tlv.Value(tlvs, 0x10); ok {
		count, err := parseNASLegacyArray(value, 2, nasMaxLegacySignalStrengths)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS signal strength list: %w", err)
		}
		r.Strengths = make([]NASSignalStrength, count)
		for i := range count {
			offset := 2 + i*2
			r.Strengths[i] = NASSignalStrength{
				Strength:       int8(value[offset]),
				RadioInterface: NASRadioInterface(value[offset+1]),
			}
		}
		r.StrengthsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		count, err := parseNASLegacyArray(value, 2, nasMaxLegacyRSSIValues)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS RSSI list: %w", err)
		}
		r.RSSI = make([]NASRSSIMeasurement, count)
		for i := range count {
			offset := 2 + i*2
			r.RSSI[i] = NASRSSIMeasurement{RSSI: value[offset], RadioInterface: NASRadioInterface(value[offset+1])}
		}
		r.RSSIKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		count, err := parseNASLegacyArray(value, 2, nasMaxLegacyECIOValues)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS ECIO list: %w", err)
		}
		r.ECIO = make([]NASECIOMeasurement, count)
		for i := range count {
			offset := 2 + i*2
			r.ECIO[i] = NASECIOMeasurement{ECIO: value[offset], RadioInterface: NASRadioInterface(value[offset+1])}
		}
		r.ECIOKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS signal strength: IO TLV length %d, want 4", len(value))
		}
		r.IO = int32(binary.LittleEndian.Uint32(value))
		r.IOKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS signal strength: SINR TLV length %d, want 1", len(value))
		}
		r.SINRLevel = value[0]
		r.SINRKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		count, err := parseNASLegacyArray(value, 3, nasMaxLegacyErrorRates)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS error rate list: %w", err)
		}
		r.ErrorRates = make([]NASErrorRateMeasurement, count)
		for i := range count {
			offset := 2 + i*3
			r.ErrorRates[i] = NASErrorRateMeasurement{
				Rate:           binary.LittleEndian.Uint16(value[offset : offset+2]),
				RadioInterface: NASRadioInterface(value[offset+2]),
			}
		}
		r.ErrorRatesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS signal strength: RSRQ TLV length %d, want 2", len(value))
		}
		r.RSRQ = NASRSRQMeasurement{RSRQ: int8(value[0]), RadioInterface: NASRadioInterface(value[1])}
		r.RSRQKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS signal strength: LTE SNR TLV length %d, want 2", len(value))
		}
		r.LTESNR = int16(binary.LittleEndian.Uint16(value))
		r.LTESNRKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x18); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS signal strength: LTE RSRP TLV length %d, want 2", len(value))
		}
		r.LTERSRP = int16(binary.LittleEndian.Uint16(value))
		r.LTERSRPKnown = true
	}
	return nil
}

// SignalStrength returns the legacy NAS signal-strength response. New code
// should normally prefer SignalInfo, which has per-RAT fields and NR5G data.
func (c *Client) SignalStrength(ctx context.Context, mask NASSignalStrengthRequestMask) (NASSignalStrengthResult, error) {
	req := (NASGetSignalStrengthRequest{Timeout: DefaultRequestTimeout, Mask: mask}).Request()
	var result NASSignalStrengthResult
	if err := c.nasReadRequest(ctx, req, &result); err != nil {
		return NASSignalStrengthResult{}, fmt.Errorf("querying QMI NAS signal strength: %w", err)
	}
	return result, nil
}

func (strength NASSignalStrength) MarshalBinary() ([]byte, error) {
	return []byte{byte(strength.Strength), byte(strength.RadioInterface)}, nil
}

func (strength *NASSignalStrength) UnmarshalBinary(value []byte) error {
	if len(value) != 2 {
		return fmt.Errorf("signal strength length %d, want 2", len(value))
	}
	*strength = NASSignalStrength{Strength: int8(value[0]), RadioInterface: NASRadioInterface(value[1])}
	return nil
}

func parseNASLegacyArray(value []byte, entryLength, maximum int) (int, error) {
	if len(value) < 2 {
		return 0, errors.New("count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > maximum {
		return 0, fmt.Errorf("count %d exceeds maximum %d", count, maximum)
	}
	want := 2 + count*entryLength
	if len(value) != want {
		return 0, fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	return count, nil
}
