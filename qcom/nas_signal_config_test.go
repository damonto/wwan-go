package qcom

import (
	"context"
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASConfigureSignalInfoRequest(t *testing.T) {
	tests := []struct {
		name    string
		config  NASSignalThresholdConfig
		wantErr bool
		check   func(*testing.T, Request)
	}{
		{
			name: "all threshold types",
			config: NASSignalThresholdConfig{
				RSSI:    []int8{-120, -100},
				ECIO:    []int16{2, 20},
				HDRSINR: []uint8{0, 8},
				LTESNR:  []int16{-160, 246},
				HDRIO:   []int32{-9000},
				LTERSRQ: []int8{-20, -3},
				LTERSRP: []int16{-140, -44},
				RSCP:    []int8{-120, -25},
				TDSSINR: []float32{-4.5, 12.25},
				LTEReport: &NASLTESignalReportConfig{
					Rate:          NASLTESignalReportRateTwoSeconds,
					AveragePeriod: NASLTESignalAverageFiveSeconds,
				},
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, nasTLVSignalConfigRSSI, []byte{2, 136, 156})
				assertTLV(t, req.TLVs, nasTLVSignalConfigECIO, []byte{2, 2, 0, 20, 0})
				assertTLV(t, req.TLVs, nasTLVSignalConfigHDRSINR, []byte{2, 0, 8})
				assertTLV(t, req.TLVs, nasTLVSignalConfigLTESNR, append([]byte{2}, nasTestInt16s(-160, 246)...))
				hdrIO := int32(-9000)
				assertTLV(t, req.TLVs, nasTLVSignalConfigHDRIO, append([]byte{1}, binary.LittleEndian.AppendUint32(nil, uint32(hdrIO))...))
				assertTLV(t, req.TLVs, nasTLVSignalConfigLTERSRQ, []byte{2, 236, 253})
				assertTLV(t, req.TLVs, nasTLVSignalConfigLTERSRP, append([]byte{2}, nasTestInt16s(-140, -44)...))
				assertTLV(t, req.TLVs, nasTLVSignalConfigLTEReport, []byte{2, 5})
				assertTLV(t, req.TLVs, nasTLVSignalConfigRSCP, []byte{2, 136, 231})
				floatValues := []byte{2}
				floatValues = binary.LittleEndian.AppendUint32(floatValues, math.Float32bits(-4.5))
				floatValues = binary.LittleEndian.AppendUint32(floatValues, math.Float32bits(12.25))
				assertTLV(t, req.TLVs, nasTLVSignalConfigTDSSINR, floatValues)
			},
		},
		{name: "empty configuration", wantErr: true},
		{name: "empty list", config: NASSignalThresholdConfig{RSSI: []int8{}}, wantErr: true},
		{name: "too many thresholds", config: NASSignalThresholdConfig{ECIO: make([]int16, 17)}, wantErr: true},
		{
			name:    "report rate out of range",
			config:  NASSignalThresholdConfig{LTEReport: &NASLTESignalReportConfig{Rate: 6}},
			wantErr: true,
		},
		{
			name:    "averaging period out of range",
			config:  NASSignalThresholdConfig{LTEReport: &NASLTESignalReportConfig{AveragePeriod: 11}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASConfigureSignalInfoRequest{
				ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Config: tt.config,
			}).Request()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Request() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if req.Service != ServiceNAS || req.ClientID != 7 || req.TransactionID != 9 ||
				req.MessageID != MessageNASConfigureSignalInfo || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			if tt.check != nil {
				tt.check(t, req)
			}
		})
	}
}

func TestClientConfigureSignalInfo(t *testing.T) {
	tests := []struct {
		name   string
		config NASSignalThresholdConfig
	}{
		{name: "RSSI", config: NASSignalThresholdConfig{RSSI: []int8{-100}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASConfigureSignalInfo {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					value, ok := tlv.Value(req.TLVs, nasTLVSignalConfigRSSI)
					if !ok || !slices.Equal(value, []byte{1, 156}) {
						t.Fatalf("RSSI TLV = %v, present %v", value, ok)
					}
				},
				resp: successResponse(MessageNASConfigureSignalInfo),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := client.ConfigureSignalInfo(context.Background(), tt.config); err != nil {
				t.Fatalf("ConfigureSignalInfo() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls = %d, want 1", got)
			}
		})
	}
}

func TestNASConfigureSignalInfo2Request(t *testing.T) {
	delta10 := uint16(10)
	delta20 := uint16(20)
	zero := uint16(0)
	tests := []struct {
		name    string
		config  NASSignalThresholdConfig2
		wantErr bool
		check   func(*testing.T, Request)
	}{
		{
			name: "LTE NR5G and WCDMA",
			config: NASSignalThresholdConfig2{
				LTERSSI:  NASScaledSignalConfig{Thresholds: []int16{-1200, -800}, Delta: &delta10},
				LTESNR:   NASScaledSignalConfig{Thresholds: []int16{-200, 300}, Delta: &delta20},
				LTERSRQ:  NASScaledSignalConfig{Thresholds: []int16{-200, -30}, Delta: &delta10},
				LTERSRP:  NASScaledSignalConfig{Thresholds: []int16{-1400, -440}, Delta: &delta20},
				NR5GSNR:  NASScaledSignalConfig{Thresholds: []int16{-200, 300}, Delta: &delta10},
				NR5GRSRQ: NASScaledSignalConfig{Thresholds: []int16{-200, -30}, Delta: &delta20},
				NR5GRSRP: NASScaledSignalConfig{Thresholds: []int16{-1400, -440}, Delta: &delta10},
				WCDMARSCP: NASScaledSignalConfig{
					Thresholds: []int16{-950, -800}, Delta: &delta20,
				},
				LTEReport: &NASLTESignalReportConfig{
					Rate: NASLTESignalReportRateThreeSeconds, AveragePeriod: NASLTESignalAverageSixSeconds,
				},
				NR5GReport: &NASNR5GSignalReportConfig{
					Rate: NASNR5GSignalReportRateTwentySeconds, AveragePeriod: NASNR5GSignalAverageThirtySeconds,
				},
			},
			check: func(t *testing.T, req Request) {
				thresholds := []struct {
					tlvType uint8
					values  []int16
				}{
					{nasTLVSignalConfig2LTERSSI, []int16{-1200, -800}},
					{nasTLVSignalConfig2LTESNR, []int16{-200, 300}},
					{nasTLVSignalConfig2LTERSRQ, []int16{-200, -30}},
					{nasTLVSignalConfig2LTERSRP, []int16{-1400, -440}},
					{nasTLVSignalConfig2NR5GSNR, []int16{-200, 300}},
					{nasTLVSignalConfig2NR5GRSRQ, []int16{-200, -30}},
					{nasTLVSignalConfig2NR5GRSRP, []int16{-1400, -440}},
					{nasTLVSignalConfig2WCDMARSCP, []int16{-950, -800}},
				}
				for _, threshold := range thresholds {
					assertTLV(t, req.TLVs, threshold.tlvType, append([]byte{2}, nasTestInt16s(threshold.values...)...))
				}
				deltas := []struct {
					tlvType uint8
					value   uint16
				}{
					{nasTLVSignalConfig2LTERSSIDelta, 10},
					{nasTLVSignalConfig2LTESNRDelta, 20},
					{nasTLVSignalConfig2LTERSRQDelta, 10},
					{nasTLVSignalConfig2LTERSRPDelta, 20},
					{nasTLVSignalConfig2NR5GSNRDelta, 10},
					{nasTLVSignalConfig2NR5GRSRQDelta, 20},
					{nasTLVSignalConfig2NR5GRSRPDelta, 10},
					{nasTLVSignalConfig2WCDMARSCPDelta, 20},
				}
				for _, delta := range deltas {
					assertTLV(t, req.TLVs, delta.tlvType, binary.LittleEndian.AppendUint16(nil, delta.value))
				}
				assertTLV(t, req.TLVs, nasTLVSignalConfig2LTEReport, []byte{3, 6})
				assertTLV(t, req.TLVs, nasTLVSignalConfig2NR5GReport, []byte{20, 30})
			},
		},
		{name: "empty configuration", wantErr: true},
		{
			name:    "empty thresholds",
			config:  NASSignalThresholdConfig2{LTERSRP: NASScaledSignalConfig{Thresholds: []int16{}}},
			wantErr: true,
		},
		{
			name:    "too many thresholds",
			config:  NASSignalThresholdConfig2{NR5GRSRP: NASScaledSignalConfig{Thresholds: make([]int16, 33)}},
			wantErr: true,
		},
		{
			name:    "zero delta",
			config:  NASSignalThresholdConfig2{LTERSSI: NASScaledSignalConfig{Delta: &zero}},
			wantErr: true,
		},
		{
			name:    "invalid LTE report",
			config:  NASSignalThresholdConfig2{LTEReport: &NASLTESignalReportConfig{Rate: 6}},
			wantErr: true,
		},
		{
			name:    "invalid NR5G report rate",
			config:  NASSignalThresholdConfig2{NR5GReport: &NASNR5GSignalReportConfig{Rate: 6}},
			wantErr: true,
		},
		{
			name:    "invalid NR5G averaging period",
			config:  NASSignalThresholdConfig2{NR5GReport: &NASNR5GSignalReportConfig{AveragePeriod: 11}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASConfigureSignalInfo2Request{
				ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Config: tt.config,
			}).Request()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Request() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if req.Service != ServiceNAS || req.ClientID != 7 || req.TransactionID != 9 ||
				req.MessageID != MessageNASConfigureSignalInfo2 || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			if tt.check != nil {
				tt.check(t, req)
			}
		})
	}
}

func TestClientConfigureSignalInfo2(t *testing.T) {
	tests := []struct {
		name   string
		config NASSignalThresholdConfig2
	}{
		{name: "NR5G RSRP", config: NASSignalThresholdConfig2{NR5GRSRP: NASScaledSignalConfig{Thresholds: []int16{-950}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASConfigureSignalInfo2 {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, nasTLVSignalConfig2NR5GRSRP, append([]byte{1}, nasTestInt16s(-950)...))
				},
				resp: successResponse(MessageNASConfigureSignalInfo2),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := client.ConfigureSignalInfo2(context.Background(), tt.config); err != nil {
				t.Fatalf("ConfigureSignalInfo2() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls = %d, want 1", got)
			}
		})
	}
}

func nasTestInt16s(values ...int16) []byte {
	encoded := make([]byte, 0, len(values)*2)
	for _, value := range values {
		encoded = binary.LittleEndian.AppendUint16(encoded, uint16(value))
	}
	return encoded
}
