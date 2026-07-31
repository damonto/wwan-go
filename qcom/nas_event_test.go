package qcom

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASSetEventReportRequest(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name    string
		config  NASSetEventReportConfig
		want    map[uint8][]byte
		wantErr string
	}{
		{name: "empty", want: map[uint8][]byte{}},
		{
			name: "all legacy metrics",
			config: NASSetEventReportConfig{
				SignalStrength:     &NASLegacySignalThresholdConfig{Report: true, Thresholds: []int8{-100, -80}},
				RFBandInfo:         &enabled,
				RegistrationReject: &disabled,
				RSSI:               &NASLegacyDeltaConfig{Report: true, Delta: 3},
				ECIO:               &NASLegacyDeltaConfig{Report: false, Delta: 4},
				IO:                 &NASLegacyDeltaConfig{Report: true, Delta: 5},
				SINR:               &NASLegacyDeltaConfig{Report: true, Delta: 6},
				ErrorRate:          &enabled,
				RSRQ:               &NASLegacyDeltaConfig{Report: true, Delta: 7},
				ECIOThreshold:      &NASLegacyECIOThresholdConfig{Report: true, Thresholds: []int16{-10, -20}},
				SINRThreshold:      &NASLegacySINRThresholdConfig{Report: true, Thresholds: []uint8{1, 2}},
				LTESNR:             &NASLegacyLTESNRDeltaConfig{Report: true, Delta: 25},
				LTERSRP:            &NASLegacyDeltaConfig{Report: true, Delta: 4},
			},
			want: map[uint8][]byte{
				0x10: {1, 2, 0x9C, 0xB0},
				0x11: {1},
				0x12: {0},
				0x13: {1, 3},
				0x14: {0, 4},
				0x15: {1, 5},
				0x16: {1, 6},
				0x17: {1},
				0x18: {1, 7},
				0x19: {1, 2, 0xF6, 0xFF, 0xEC, 0xFF},
				0x1A: {1, 2, 1, 2},
				0x1B: {1, 25, 0},
				0x1C: {1, 4},
			},
		},
		{
			name:    "enabled signal threshold empty",
			config:  NASSetEventReportConfig{SignalStrength: &NASLegacySignalThresholdConfig{Report: true}},
			wantErr: "at least one threshold",
		},
		{
			name:    "too many ECIO thresholds",
			config:  NASSetEventReportConfig{ECIOThreshold: &NASLegacyECIOThresholdConfig{Thresholds: make([]int16, nasMaxEventECIOThresholds+1)}},
			wantErr: "exceeds maximum",
		},
		{
			name:    "too many signal thresholds",
			config:  NASSetEventReportConfig{SignalStrength: &NASLegacySignalThresholdConfig{Thresholds: make([]int8, nasMaxEventSignalStrengthThresholds+1)}},
			wantErr: "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (NASSetEventReportRequest{
				ClientID: 7, TransactionID: 9, Timeout: 2 * time.Second, Config: tt.config,
			}).Request()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceNAS || got.ClientID != 7 || got.TransactionID != 9 ||
				got.MessageID != MessageNASSetEventReport || got.Timeout != 2*time.Second {
				t.Fatalf("Request() = %+v", got)
			}
			if len(got.TLVs) != len(tt.want) {
				t.Fatalf("TLV count = %d, want %d", len(got.TLVs), len(tt.want))
			}
			for typ, want := range tt.want {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestNASEventReportUnmarshalTLVs(t *testing.T) {
	var ioValue = nasLegacyInt32(-7000)
	tlvs := tlv.TLVs{
		tlv.Bytes(0x10, []byte{0xB0, byte(NASRadioInterfaceLTE)}),
		tlv.Bytes(0x11, []byte{1, byte(NASRadioInterfaceLTE), 41, 0, 0x72, 0x06}),
		tlv.Bytes(0x12, []byte{byte(NASNetworkServicePS), 0x0F, 0x00}),
		tlv.Bytes(0x13, []byte{70, byte(NASRadioInterfaceGSM)}),
		tlv.Bytes(0x14, []byte{14, byte(NASRadioInterfaceUMTS)}),
		tlv.Bytes(0x15, ioValue),
		tlv.Bytes(0x16, []byte{5}),
		tlv.Bytes(0x17, []byte{0x2C, 0x01, byte(NASRadioInterfaceGSM)}),
		tlv.Bytes(0x18, []byte{0xF4, byte(NASRadioInterfaceLTE)}),
		tlv.Bytes(0x19, nasTestInt16s(-160)),
		tlv.Bytes(0x1A, nasTestInt16s(-100)),
	}

	var got NASEventReport
	if err := got.UnmarshalTLVs(tlvs); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if !got.SignalStrengthKnown || got.SignalStrength.Strength != -80 || got.SignalStrength.RadioInterface != NASRadioInterfaceLTE {
		t.Fatalf("signal strength = %+v, known=%v", got.SignalStrength, got.SignalStrengthKnown)
	}
	if !got.RFBandsKnown || len(got.RFBands) != 1 || got.RFBands[0].Band != 41 || got.RFBands[0].Channel != 1650 {
		t.Fatalf("RF bands = %+v, known=%v", got.RFBands, got.RFBandsKnown)
	}
	if !got.RegistrationRejectKnown || got.RegistrationReject.ServiceDomain != NASNetworkServicePS || got.RegistrationReject.Cause != 15 {
		t.Fatalf("reject = %+v", got.RegistrationReject)
	}
	if !got.RSSIKnown || got.RSSI.RSSI != 70 || !got.ECIOKnown || got.ECIO.ECIO != 14 {
		t.Fatalf("RSSI/ECIO = %+v/%+v", got.RSSI, got.ECIO)
	}
	if !got.IOKnown || got.IO != -7000 || !got.SINRKnown || got.SINRLevel != 5 {
		t.Fatalf("IO/SINR = %d/%d", got.IO, got.SINRLevel)
	}
	if !got.ErrorRateKnown || got.ErrorRate.Rate != 300 || !got.RSRQKnown || got.RSRQ.RSRQ != -12 {
		t.Fatalf("error/RSRQ = %+v/%+v", got.ErrorRate, got.RSRQ)
	}
	if !got.LTESNRKnown || got.LTESNR != -160 || !got.LTERSRPKnown || got.LTERSRP != -100 {
		t.Fatalf("LTE metrics = %d/%d", got.LTESNR, got.LTERSRP)
	}
}

func TestNASEventReportRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want string
	}{
		{name: "signal length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, want: "event signal"},
		{name: "RF band length", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 2})}, want: "RF band TLV length"},
		{name: "reject length", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1})}, want: "registration reject"},
		{name: "RSSI length", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1})}, want: "RSSI TLV length"},
		{name: "ECIO length", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{1})}, want: "ECIO TLV length"},
		{name: "IO length", tlvs: tlv.TLVs{tlv.Bytes(0x15, []byte{1})}, want: "IO TLV length"},
		{name: "SINR length", tlvs: tlv.TLVs{tlv.Bytes(0x16, []byte{})}, want: "SINR TLV length"},
		{name: "error rate length", tlvs: tlv.TLVs{tlv.Bytes(0x17, []byte{1})}, want: "error rate TLV length"},
		{name: "RSRQ length", tlvs: tlv.TLVs{tlv.Bytes(0x18, []byte{1})}, want: "RSRQ TLV length"},
		{name: "LTE SNR length", tlvs: tlv.TLVs{tlv.Bytes(0x19, []byte{1})}, want: "LTE SNR TLV length"},
		{name: "LTE RSRP length", tlvs: tlv.TLVs{tlv.Bytes(0x1A, []byte{1})}, want: "LTE RSRP TLV length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASEventReport
			err := got.UnmarshalTLVs(tt.tlvs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestClientNASSetEventReport(t *testing.T) {
	enabled := true
	transport := &fakeTransport{t: t, calls: []transportCall{{
		check: func(req Request) {
			if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASSetEventReport {
				t.Fatalf("request = %+v", req)
			}
			assertTLV(t, req.TLVs, 0x11, []byte{1})
		},
		resp: successResponse(MessageNASSetEventReport),
	}}}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
	if err := client.NASSetEventReport(context.Background(), NASSetEventReportConfig{RFBandInfo: &enabled}); err != nil {
		t.Fatalf("NASSetEventReport() error = %v", err)
	}
}
