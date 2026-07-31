package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVSignalConfigRSSI      = 0x10
	nasTLVSignalConfigECIO      = 0x11
	nasTLVSignalConfigHDRSINR   = 0x12
	nasTLVSignalConfigLTESNR    = 0x13
	nasTLVSignalConfigHDRIO     = 0x14
	nasTLVSignalConfigLTERSRQ   = 0x15
	nasTLVSignalConfigLTERSRP   = 0x16
	nasTLVSignalConfigLTEReport = 0x17
	nasTLVSignalConfigRSCP      = 0x18
	nasTLVSignalConfigTDSSINR   = 0x19

	nasMaxSignalThresholds  = 16
	nasMaxSignalThresholds2 = 32

	nasTLVSignalConfig2LTERSSI        = 0x22
	nasTLVSignalConfig2LTERSSIDelta   = 0x23
	nasTLVSignalConfig2LTESNR         = 0x24
	nasTLVSignalConfig2LTESNRDelta    = 0x25
	nasTLVSignalConfig2LTERSRQ        = 0x26
	nasTLVSignalConfig2LTERSRQDelta   = 0x27
	nasTLVSignalConfig2LTERSRP        = 0x28
	nasTLVSignalConfig2LTERSRPDelta   = 0x29
	nasTLVSignalConfig2LTEReport      = 0x2A
	nasTLVSignalConfig2NR5GSNR        = 0x33
	nasTLVSignalConfig2NR5GSNRDelta   = 0x34
	nasTLVSignalConfig2NR5GRSRP       = 0x35
	nasTLVSignalConfig2NR5GRSRPDelta  = 0x36
	nasTLVSignalConfig2NR5GReport     = 0x37
	nasTLVSignalConfig2NR5GRSRQ       = 0x38
	nasTLVSignalConfig2NR5GRSRQDelta  = 0x39
	nasTLVSignalConfig2WCDMARSCP      = 0x3A
	nasTLVSignalConfig2WCDMARSCPDelta = 0x3B
)

// NASLTESignalReportRate controls how often LTE signal state is checked.
type NASLTESignalReportRate uint8

const (
	NASLTESignalReportRateDefault NASLTESignalReportRate = iota
	NASLTESignalReportRateOneSecond
	NASLTESignalReportRateTwoSeconds
	NASLTESignalReportRateThreeSeconds
	NASLTESignalReportRateFourSeconds
	NASLTESignalReportRateFiveSeconds
)

// NASLTESignalAveragePeriod controls the LTE measurement averaging window.
type NASLTESignalAveragePeriod uint8

const (
	NASLTESignalAverageDefault NASLTESignalAveragePeriod = iota
	NASLTESignalAverageOneSecond
	NASLTESignalAverageTwoSeconds
	NASLTESignalAverageThreeSeconds
	NASLTESignalAverageFourSeconds
	NASLTESignalAverageFiveSeconds
	NASLTESignalAverageSixSeconds
	NASLTESignalAverageSevenSeconds
	NASLTESignalAverageEightSeconds
	NASLTESignalAverageNineSeconds
	NASLTESignalAverageTenSeconds
)

// NASLTESignalReportConfig controls LTE report cadence and averaging.
type NASLTESignalReportConfig struct {
	Rate          NASLTESignalReportRate
	AveragePeriod NASLTESignalAveragePeriod
}

// NASSignalThresholdConfig selects crossing thresholds for Signal Info indications.
// Nil slices are omitted. A non-nil slice must contain between 1 and 16 values.
type NASSignalThresholdConfig struct {
	RSSI    []int8
	ECIO    []int16
	HDRSINR []uint8
	LTESNR  []int16
	HDRIO   []int32
	LTERSRQ []int8
	LTERSRP []int16
	RSCP    []int8

	TDSSINR   []float32
	LTEReport *NASLTESignalReportConfig
}

// NASScaledSignalConfig configures thresholds and an optional crossing delta.
// Values use the 0.1 dB or 0.1 dBm units defined by the corresponding metric.
type NASScaledSignalConfig struct {
	Thresholds []int16
	Delta      *uint16
}

// NASNR5GSignalReportRate controls how often NR5G signal state is checked.
type NASNR5GSignalReportRate uint8

const (
	NASNR5GSignalReportRateDefault       NASNR5GSignalReportRate = 0
	NASNR5GSignalReportRateOneSecond     NASNR5GSignalReportRate = 1
	NASNR5GSignalReportRateTwoSeconds    NASNR5GSignalReportRate = 2
	NASNR5GSignalReportRateThreeSeconds  NASNR5GSignalReportRate = 3
	NASNR5GSignalReportRateFourSeconds   NASNR5GSignalReportRate = 4
	NASNR5GSignalReportRateFiveSeconds   NASNR5GSignalReportRate = 5
	NASNR5GSignalReportRateTenSeconds    NASNR5GSignalReportRate = 10
	NASNR5GSignalReportRateTwentySeconds NASNR5GSignalReportRate = 20
	NASNR5GSignalReportRateThirtySeconds NASNR5GSignalReportRate = 30
)

// NASNR5GSignalAveragePeriod controls the NR5G measurement averaging window.
type NASNR5GSignalAveragePeriod uint8

const (
	NASNR5GSignalAverageDefault       NASNR5GSignalAveragePeriod = 0
	NASNR5GSignalAverageOneSecond     NASNR5GSignalAveragePeriod = 1
	NASNR5GSignalAverageTwoSeconds    NASNR5GSignalAveragePeriod = 2
	NASNR5GSignalAverageThreeSeconds  NASNR5GSignalAveragePeriod = 3
	NASNR5GSignalAverageFourSeconds   NASNR5GSignalAveragePeriod = 4
	NASNR5GSignalAverageFiveSeconds   NASNR5GSignalAveragePeriod = 5
	NASNR5GSignalAverageSixSeconds    NASNR5GSignalAveragePeriod = 6
	NASNR5GSignalAverageSevenSeconds  NASNR5GSignalAveragePeriod = 7
	NASNR5GSignalAverageEightSeconds  NASNR5GSignalAveragePeriod = 8
	NASNR5GSignalAverageNineSeconds   NASNR5GSignalAveragePeriod = 9
	NASNR5GSignalAverageTenSeconds    NASNR5GSignalAveragePeriod = 10
	NASNR5GSignalAverageTwentySeconds NASNR5GSignalAveragePeriod = 20
	NASNR5GSignalAverageThirtySeconds NASNR5GSignalAveragePeriod = 30
)

// NASNR5GSignalReportConfig controls NR5G report cadence and averaging.
type NASNR5GSignalReportConfig struct {
	Rate          NASNR5GSignalReportRate
	AveragePeriod NASNR5GSignalAveragePeriod
}

// NASSignalThresholdConfig2 configures per-RAT signal thresholds supported by
// the nondeprecated Config Signal Info2 command.
type NASSignalThresholdConfig2 struct {
	LTERSSI NASScaledSignalConfig
	LTESNR  NASScaledSignalConfig
	LTERSRQ NASScaledSignalConfig
	LTERSRP NASScaledSignalConfig

	NR5GSNR  NASScaledSignalConfig
	NR5GRSRQ NASScaledSignalConfig
	NR5GRSRP NASScaledSignalConfig

	WCDMARSCP NASScaledSignalConfig

	LTEReport  *NASLTESignalReportConfig
	NR5GReport *NASNR5GSignalReportConfig
}

// NASConfigureSignalInfoRequest encodes NAS Configure Signal Info.
type NASConfigureSignalInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASSignalThresholdConfig
}

// NASConfigureSignalInfo2Request encodes NAS Configure Signal Info2.
type NASConfigureSignalInfo2Request struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASSignalThresholdConfig2
}

// Request converts the threshold configuration into a QMI NAS request.
func (r NASConfigureSignalInfoRequest) Request() (Request, error) {
	var tlvs tlv.TLVs
	appendThresholds := func(tlvType uint8, count int, encode func() []byte) error {
		if count == 0 {
			return errors.New("threshold list is empty")
		}
		if count > nasMaxSignalThresholds {
			return fmt.Errorf("threshold count %d exceeds %d", count, nasMaxSignalThresholds)
		}
		value := append([]byte{byte(count)}, encode()...)
		tlvs = append(tlvs, tlv.Bytes(tlvType, value))
		return nil
	}

	if r.Config.RSSI != nil {
		if err := appendThresholds(nasTLVSignalConfigRSSI, len(r.Config.RSSI), func() []byte {
			value := make([]byte, len(r.Config.RSSI))
			for i, threshold := range r.Config.RSSI {
				value[i] = byte(threshold)
			}
			return value
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS RSSI thresholds: %w", err)
		}
	}
	if r.Config.ECIO != nil {
		if err := appendThresholds(nasTLVSignalConfigECIO, len(r.Config.ECIO), func() []byte {
			return encodeNASInt16s(r.Config.ECIO)
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS ECIO thresholds: %w", err)
		}
	}
	if r.Config.HDRSINR != nil {
		if err := appendThresholds(nasTLVSignalConfigHDRSINR, len(r.Config.HDRSINR), func() []byte {
			return append([]byte(nil), r.Config.HDRSINR...)
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS HDR SINR thresholds: %w", err)
		}
	}
	if r.Config.LTESNR != nil {
		if err := appendThresholds(nasTLVSignalConfigLTESNR, len(r.Config.LTESNR), func() []byte {
			return encodeNASInt16s(r.Config.LTESNR)
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS LTE SNR thresholds: %w", err)
		}
	}
	if r.Config.HDRIO != nil {
		if err := appendThresholds(nasTLVSignalConfigHDRIO, len(r.Config.HDRIO), func() []byte {
			value := make([]byte, 0, len(r.Config.HDRIO)*4)
			for _, threshold := range r.Config.HDRIO {
				value = binary.LittleEndian.AppendUint32(value, uint32(threshold))
			}
			return value
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS HDR IO thresholds: %w", err)
		}
	}
	if r.Config.LTERSRQ != nil {
		if err := appendThresholds(nasTLVSignalConfigLTERSRQ, len(r.Config.LTERSRQ), func() []byte {
			value := make([]byte, len(r.Config.LTERSRQ))
			for i, threshold := range r.Config.LTERSRQ {
				value[i] = byte(threshold)
			}
			return value
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS LTE RSRQ thresholds: %w", err)
		}
	}
	if r.Config.LTERSRP != nil {
		if err := appendThresholds(nasTLVSignalConfigLTERSRP, len(r.Config.LTERSRP), func() []byte {
			return encodeNASInt16s(r.Config.LTERSRP)
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS LTE RSRP thresholds: %w", err)
		}
	}
	if r.Config.LTEReport != nil {
		if r.Config.LTEReport.Rate > NASLTESignalReportRateFiveSeconds {
			return Request{}, fmt.Errorf("encoding QMI NAS LTE signal report: rate %d is out of range", r.Config.LTEReport.Rate)
		}
		if r.Config.LTEReport.AveragePeriod > NASLTESignalAverageTenSeconds {
			return Request{}, fmt.Errorf("encoding QMI NAS LTE signal report: averaging period %d is out of range", r.Config.LTEReport.AveragePeriod)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSignalConfigLTEReport, []byte{
			byte(r.Config.LTEReport.Rate), byte(r.Config.LTEReport.AveragePeriod),
		}))
	}
	if r.Config.RSCP != nil {
		if err := appendThresholds(nasTLVSignalConfigRSCP, len(r.Config.RSCP), func() []byte {
			value := make([]byte, len(r.Config.RSCP))
			for i, threshold := range r.Config.RSCP {
				value[i] = byte(threshold)
			}
			return value
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS RSCP thresholds: %w", err)
		}
	}
	if r.Config.TDSSINR != nil {
		if err := appendThresholds(nasTLVSignalConfigTDSSINR, len(r.Config.TDSSINR), func() []byte {
			value := make([]byte, 0, len(r.Config.TDSSINR)*4)
			for _, threshold := range r.Config.TDSSINR {
				value = binary.LittleEndian.AppendUint32(value, math.Float32bits(threshold))
			}
			return value
		}); err != nil {
			return Request{}, fmt.Errorf("encoding QMI NAS TD-SCDMA SINR thresholds: %w", err)
		}
	}
	if len(tlvs) == 0 {
		return Request{}, errors.New("encoding QMI NAS signal thresholds: configuration is empty")
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASConfigureSignalInfo,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// ConfigureSignalInfo sets threshold crossings that trigger Signal Info indications.
func (c *Client) ConfigureSignalInfo(ctx context.Context, config NASSignalThresholdConfig) error {
	req, err := (NASConfigureSignalInfoRequest{
		Timeout: DefaultRequestTimeout,
		Config:  config,
	}).Request()
	if err != nil {
		return err
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI NAS signal information: %w", err)
	}
	return nil
}

// Request converts the per-RAT threshold configuration into a QMI NAS request.
func (r NASConfigureSignalInfo2Request) Request() (Request, error) {
	var tlvs tlv.TLVs
	metrics := []struct {
		thresholdType uint8
		deltaType     uint8
		config        NASScaledSignalConfig
	}{
		{nasTLVSignalConfig2LTERSSI, nasTLVSignalConfig2LTERSSIDelta, r.Config.LTERSSI},
		{nasTLVSignalConfig2LTESNR, nasTLVSignalConfig2LTESNRDelta, r.Config.LTESNR},
		{nasTLVSignalConfig2LTERSRQ, nasTLVSignalConfig2LTERSRQDelta, r.Config.LTERSRQ},
		{nasTLVSignalConfig2LTERSRP, nasTLVSignalConfig2LTERSRPDelta, r.Config.LTERSRP},
		{nasTLVSignalConfig2NR5GSNR, nasTLVSignalConfig2NR5GSNRDelta, r.Config.NR5GSNR},
		{nasTLVSignalConfig2NR5GRSRQ, nasTLVSignalConfig2NR5GRSRQDelta, r.Config.NR5GRSRQ},
		{nasTLVSignalConfig2NR5GRSRP, nasTLVSignalConfig2NR5GRSRPDelta, r.Config.NR5GRSRP},
		{nasTLVSignalConfig2WCDMARSCP, nasTLVSignalConfig2WCDMARSCPDelta, r.Config.WCDMARSCP},
	}
	for _, metric := range metrics {
		if metric.config.Thresholds != nil {
			value, err := encodeNASSignalThresholds2(metric.config.Thresholds)
			if err != nil {
				return Request{}, fmt.Errorf("encoding QMI NAS threshold TLV 0x%02X: %w", metric.thresholdType, err)
			}
			tlvs = append(tlvs, tlv.Bytes(metric.thresholdType, value))
		}
		if metric.config.Delta != nil {
			if *metric.config.Delta == 0 {
				return Request{}, fmt.Errorf("encoding QMI NAS delta TLV 0x%02X: value is zero", metric.deltaType)
			}
			tlvs = append(tlvs, tlv.Uint(metric.deltaType, *metric.config.Delta))
		}
	}
	if r.Config.LTEReport != nil {
		if err := validateNASLTEReportConfig(*r.Config.LTEReport); err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSignalConfig2LTEReport, []byte{
			byte(r.Config.LTEReport.Rate), byte(r.Config.LTEReport.AveragePeriod),
		}))
	}
	if r.Config.NR5GReport != nil {
		if !validNASNR5GReportRate(r.Config.NR5GReport.Rate) {
			return Request{}, fmt.Errorf("encoding QMI NAS NR5G signal report: rate %d is out of range", r.Config.NR5GReport.Rate)
		}
		if !validNASNR5GAveragePeriod(r.Config.NR5GReport.AveragePeriod) {
			return Request{}, fmt.Errorf("encoding QMI NAS NR5G signal report: averaging period %d is out of range", r.Config.NR5GReport.AveragePeriod)
		}
		tlvs = append(tlvs, tlv.Bytes(nasTLVSignalConfig2NR5GReport, []byte{
			byte(r.Config.NR5GReport.Rate), byte(r.Config.NR5GReport.AveragePeriod),
		}))
	}
	if len(tlvs) == 0 {
		return Request{}, errors.New("encoding QMI NAS per-RAT signal thresholds: configuration is empty")
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASConfigureSignalInfo2,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// ConfigureSignalInfo2 sets per-RAT LTE, NR5G, and WCDMA signal thresholds.
func (c *Client) ConfigureSignalInfo2(ctx context.Context, config NASSignalThresholdConfig2) error {
	req, err := (NASConfigureSignalInfo2Request{
		Timeout: DefaultRequestTimeout,
		Config:  config,
	}).Request()
	if err != nil {
		return err
	}
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("configuring QMI NAS per-RAT signal information: %w", err)
	}
	return nil
}

func encodeNASInt16s(values []int16) []byte {
	encoded := make([]byte, 0, len(values)*2)
	for _, value := range values {
		encoded = binary.LittleEndian.AppendUint16(encoded, uint16(value))
	}
	return encoded
}

func encodeNASSignalThresholds2(values []int16) ([]byte, error) {
	if len(values) == 0 {
		return nil, errors.New("list is empty")
	}
	if len(values) > nasMaxSignalThresholds2 {
		return nil, fmt.Errorf("count %d exceeds %d", len(values), nasMaxSignalThresholds2)
	}
	return append([]byte{byte(len(values))}, encodeNASInt16s(values)...), nil
}

func validateNASLTEReportConfig(config NASLTESignalReportConfig) error {
	if config.Rate > NASLTESignalReportRateFiveSeconds {
		return fmt.Errorf("encoding QMI NAS LTE signal report: rate %d is out of range", config.Rate)
	}
	if config.AveragePeriod > NASLTESignalAverageTenSeconds {
		return fmt.Errorf("encoding QMI NAS LTE signal report: averaging period %d is out of range", config.AveragePeriod)
	}
	return nil
}

func validNASNR5GReportRate(rate NASNR5GSignalReportRate) bool {
	return rate <= NASNR5GSignalReportRateFiveSeconds ||
		rate == NASNR5GSignalReportRateTenSeconds ||
		rate == NASNR5GSignalReportRateTwentySeconds ||
		rate == NASNR5GSignalReportRateThirtySeconds
}

func validNASNR5GAveragePeriod(period NASNR5GSignalAveragePeriod) bool {
	return period <= NASNR5GSignalAverageTenSeconds ||
		period == NASNR5GSignalAverageTwentySeconds ||
		period == NASNR5GSignalAverageThirtySeconds
}
