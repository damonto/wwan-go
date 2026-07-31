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
	nasTLVEventReportSignalStrength = 0x10
	nasTLVEventReportRFBand         = 0x11
	nasTLVEventReportReject         = 0x12
	nasTLVEventReportRSSI           = 0x13
	nasTLVEventReportECIO           = 0x14
	nasTLVEventReportIO             = 0x15
	nasTLVEventReportSINR           = 0x16
	nasTLVEventReportErrorRate      = 0x17
	nasTLVEventReportRSRQ           = 0x18
	nasTLVEventReportLTESNR         = 0x19
	nasTLVEventReportLTERSRP        = 0x1A

	nasTLVSetEventReportSignalStrength = 0x10
	nasTLVSetEventReportRFBand         = 0x11
	nasTLVSetEventReportReject         = 0x12
	nasTLVSetEventReportRSSI           = 0x13
	nasTLVSetEventReportECIO           = 0x14
	nasTLVSetEventReportIO             = 0x15
	nasTLVSetEventReportSINR           = 0x16
	nasTLVSetEventReportErrorRate      = 0x17
	nasTLVSetEventReportRSRQ           = 0x18
	nasTLVSetEventReportECIOThreshold  = 0x19
	nasTLVSetEventReportSINRThreshold  = 0x1A
	nasTLVSetEventReportLTESNR         = 0x1B
	nasTLVSetEventReportLTERSRP        = 0x1C

	nasMaxEventSignalStrengthThresholds = 5
	nasMaxEventECIOThresholds           = 10
	nasMaxEventSINRThresholds           = 5
)

// NASLegacySignalThresholdConfig controls the old signal-strength bands.
type NASLegacySignalThresholdConfig struct {
	Report     bool
	Thresholds []int8
}

// NASLegacyDeltaConfig controls a legacy metric's report state and delta.
// The delta unit depends on the metric using it.
type NASLegacyDeltaConfig struct {
	Report bool
	Delta  uint8
}

// NASLegacyLTESNRDeltaConfig controls LTE SNR reports. Delta is in 0.1 dB
// units and follows the Qualcomm IDL's two-byte field.
type NASLegacyLTESNRDeltaConfig struct {
	Report bool
	Delta  uint16
}

// NASLegacyECIOThresholdConfig controls ECIO crossing thresholds.
type NASLegacyECIOThresholdConfig struct {
	Report     bool
	Thresholds []int16
}

// NASLegacySINRThresholdConfig controls EVDO SINR crossing thresholds.
type NASLegacySINRThresholdConfig struct {
	Report     bool
	Thresholds []uint8
}

// NASSetEventReportConfig selects deprecated NAS event-report indications.
// Nil fields are omitted so a caller can change only the settings it owns.
type NASSetEventReportConfig struct {
	SignalStrength     *NASLegacySignalThresholdConfig
	RFBandInfo         *bool
	RegistrationReject *bool
	RSSI               *NASLegacyDeltaConfig
	ECIO               *NASLegacyDeltaConfig
	IO                 *NASLegacyDeltaConfig
	SINR               *NASLegacyDeltaConfig
	ErrorRate          *bool
	RSRQ               *NASLegacyDeltaConfig
	ECIOThreshold      *NASLegacyECIOThresholdConfig
	SINRThreshold      *NASLegacySINRThresholdConfig
	LTESNR             *NASLegacyLTESNRDeltaConfig
	LTERSRP            *NASLegacyDeltaConfig
}

// NASSetEventReportRequest encodes Set Event Report.
type NASSetEventReportRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASSetEventReportConfig
}

// Request validates and converts the event-report configuration to QMI TLVs.
func (r NASSetEventReportRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASSetEventReport,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// NASRegistrationRejectReason is the reject data carried by a legacy event.
type NASRegistrationRejectReason struct {
	ServiceDomain NASNetworkServiceDomain
	Cause         uint16
}

// NASEventReport contains the optional values in one legacy NAS indication.
type NASEventReport struct {
	SignalStrength          NASSignalStrength
	SignalStrengthKnown     bool
	RFBands                 []NASRFBand
	RFBandsKnown            bool
	RegistrationReject      NASRegistrationRejectReason
	RegistrationRejectKnown bool
	RSSI                    NASRSSIMeasurement
	RSSIKnown               bool
	ECIO                    NASECIOMeasurement
	ECIOKnown               bool
	IO                      int32
	IOKnown                 bool
	SINRLevel               uint8
	SINRKnown               bool
	ErrorRate               NASErrorRateMeasurement
	ErrorRateKnown          bool
	RSRQ                    NASRSRQMeasurement
	RSRQKnown               bool
	LTESNR                  int16
	LTESNRKnown             bool
	LTERSRP                 int16
	LTERSRPKnown            bool
}

// UnmarshalTLVs parses a QMI NAS Event Report indication.
func (e *NASEventReport) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*e = NASEventReport{}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportSignalStrength); ok {
		strength, err := parseNASSignalStrength(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS event signal strength: %w", err)
		}
		e.SignalStrength = strength
		e.SignalStrengthKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportRFBand); ok {
		bands, err := parseNASLegacyEventRFBands(value)
		if err != nil {
			return err
		}
		e.RFBands = bands
		e.RFBandsKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportReject); ok {
		if len(value) != 3 {
			return fmt.Errorf("parsing QMI NAS event report: registration reject TLV length %d, want 3", len(value))
		}
		e.RegistrationReject = NASRegistrationRejectReason{
			ServiceDomain: NASNetworkServiceDomain(value[0]),
			Cause:         binary.LittleEndian.Uint16(value[1:]),
		}
		e.RegistrationRejectKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportRSSI); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS event report: RSSI TLV length %d, want 2", len(value))
		}
		e.RSSI = NASRSSIMeasurement{RSSI: value[0], RadioInterface: NASRadioInterface(value[1])}
		e.RSSIKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportECIO); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS event report: ECIO TLV length %d, want 2", len(value))
		}
		e.ECIO = NASECIOMeasurement{ECIO: value[0], RadioInterface: NASRadioInterface(value[1])}
		e.ECIOKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportIO); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS event report: IO TLV length %d, want 4", len(value))
		}
		e.IO = int32(binary.LittleEndian.Uint32(value))
		e.IOKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportSINR); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS event report: SINR TLV length %d, want 1", len(value))
		}
		e.SINRLevel = value[0]
		e.SINRKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportErrorRate); ok {
		if len(value) != 3 {
			return fmt.Errorf("parsing QMI NAS event report: error rate TLV length %d, want 3", len(value))
		}
		e.ErrorRate = NASErrorRateMeasurement{
			Rate:           binary.LittleEndian.Uint16(value[:2]),
			RadioInterface: NASRadioInterface(value[2]),
		}
		e.ErrorRateKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportRSRQ); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS event report: RSRQ TLV length %d, want 2", len(value))
		}
		e.RSRQ = NASRSRQMeasurement{RSRQ: int8(value[0]), RadioInterface: NASRadioInterface(value[1])}
		e.RSRQKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportLTESNR); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS event report: LTE SNR TLV length %d, want 2", len(value))
		}
		e.LTESNR = int16(binary.LittleEndian.Uint16(value))
		e.LTESNRKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVEventReportLTERSRP); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS event report: LTE RSRP TLV length %d, want 2", len(value))
		}
		e.LTERSRP = int16(binary.LittleEndian.Uint16(value))
		e.LTERSRPKnown = true
	}
	return nil
}

// NASSetEventReport configures legacy NAS state-change indications.
func (c *Client) NASSetEventReport(ctx context.Context, config NASSetEventReportConfig) error {
	req, err := (NASSetEventReportRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI NAS event reports: %w", err)
	}
	return nil
}

// MarshalTLVs encodes NAS event-report fields.
func (config NASSetEventReportConfig) MarshalTLVs() (tlv.TLVs, error) {
	var tlvs tlv.TLVs
	if config.SignalStrength != nil {
		if err := validateNASLegacyThresholds(config.SignalStrength.Report, config.SignalStrength.Thresholds, nasMaxEventSignalStrengthThresholds); err != nil {
			return nil, fmt.Errorf("encoding QMI NAS signal strength event thresholds: %w", err)
		}
		value := append([]byte{boolByte(config.SignalStrength.Report), byte(len(config.SignalStrength.Thresholds))}, encodeNASInt8s(config.SignalStrength.Thresholds)...)
		tlvs = append(tlvs, tlv.Bytes(nasTLVSetEventReportSignalStrength, value))
	}
	if config.RFBandInfo != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSetEventReportRFBand, boolByte(*config.RFBandInfo)))
	}
	if config.RegistrationReject != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSetEventReportReject, boolByte(*config.RegistrationReject)))
	}
	appendDelta := func(kind byte, config *NASLegacyDeltaConfig) {
		if config != nil {
			tlvs = append(tlvs, tlv.Bytes(kind, []byte{boolByte(config.Report), config.Delta}))
		}
	}
	appendDelta(nasTLVSetEventReportRSSI, config.RSSI)
	appendDelta(nasTLVSetEventReportECIO, config.ECIO)
	appendDelta(nasTLVSetEventReportIO, config.IO)
	appendDelta(nasTLVSetEventReportSINR, config.SINR)
	if config.ErrorRate != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVSetEventReportErrorRate, boolByte(*config.ErrorRate)))
	}
	appendDelta(nasTLVSetEventReportRSRQ, config.RSRQ)
	if config.ECIOThreshold != nil {
		if err := validateNASLegacyThresholds(config.ECIOThreshold.Report, config.ECIOThreshold.Thresholds, nasMaxEventECIOThresholds); err != nil {
			return nil, fmt.Errorf("encoding QMI NAS ECIO event thresholds: %w", err)
		}
		value := append([]byte{boolByte(config.ECIOThreshold.Report), byte(len(config.ECIOThreshold.Thresholds))}, encodeNASInt16s(config.ECIOThreshold.Thresholds)...)
		tlvs = append(tlvs, tlv.Bytes(nasTLVSetEventReportECIOThreshold, value))
	}
	if config.SINRThreshold != nil {
		if err := validateNASLegacyThresholds(config.SINRThreshold.Report, config.SINRThreshold.Thresholds, nasMaxEventSINRThresholds); err != nil {
			return nil, fmt.Errorf("encoding QMI NAS SINR event thresholds: %w", err)
		}
		value := append([]byte{boolByte(config.SINRThreshold.Report), byte(len(config.SINRThreshold.Thresholds))}, config.SINRThreshold.Thresholds...)
		tlvs = append(tlvs, tlv.Bytes(nasTLVSetEventReportSINRThreshold, value))
	}
	if config.LTESNR != nil {
		tlvs = append(tlvs, tlv.Bytes(nasTLVSetEventReportLTESNR, append([]byte{boolByte(config.LTESNR.Report)}, binary.LittleEndian.AppendUint16(nil, config.LTESNR.Delta)...)))
	}
	appendDelta(nasTLVSetEventReportLTERSRP, config.LTERSRP)
	return tlvs, nil
}

func validateNASLegacyThresholds[T ~int8 | ~int16 | ~uint8](report bool, values []T, maximum int) error {
	if len(values) > maximum {
		return fmt.Errorf("count %d exceeds maximum %d", len(values), maximum)
	}
	if report && len(values) == 0 {
		return errors.New("at least one threshold is required when reporting is enabled")
	}
	return nil
}

func encodeNASInt8s(values []int8) []byte {
	encoded := make([]byte, len(values))
	for i, value := range values {
		encoded[i] = byte(value)
	}
	return encoded
}

func parseNASLegacyEventRFBands(value []byte) ([]NASRFBand, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS event report: RF band count is truncated")
	}
	count := int(value[0])
	want := 1 + count*5
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS event report: RF band TLV length %d, want %d", len(value), want)
	}
	bands := make([]NASRFBand, count)
	for i := range count {
		offset := 1 + i*5
		bands[i] = NASRFBand{
			RadioInterface: NASRadioInterface(value[offset]),
			Band:           NASActiveBand(binary.LittleEndian.Uint16(value[offset+1 : offset+3])),
			Channel:        uint32(binary.LittleEndian.Uint16(value[offset+3 : offset+5])),
		}
	}
	return bands, nil
}
