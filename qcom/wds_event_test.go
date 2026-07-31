package qcom

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSSetEventReportRequest(t *testing.T) {
	enabled := true
	disabled := false
	mipStatus := uint8(7)
	tests := []struct {
		name    string
		config  WDSSetEventReportConfig
		want    map[uint8][]byte
		wantErr string
	}{
		{name: "empty", want: map[uint8][]byte{}},
		{
			name: "all basic fields",
			config: WDSSetEventReportConfig{
				ChannelRate:                 &enabled,
				TransferStatistics:          &WDSTransferStatisticsConfig{IntervalSeconds: 5, Indicators: WDSStatisticsTxPackets | WDSStatisticsRxBytes},
				DataBearerTechnology:        &disabled,
				DormancyStatus:              &enabled,
				MIPStatus:                   &mipStatus,
				CurrentDataBearerTechnology: &enabled,
				DataCallStatus:              &enabled,
				PreferredDataSystem:         &disabled,
				EVDOPageMonitorChange:       &enabled,
				DataSystems:                 &enabled,
				UplinkFlowControl:           &disabled,
				LimitedDataSystems:          &enabled,
				PDNFilterRemovals:           &enabled,
				ExtendedDataBearer:          &enabled,
			},
			want: map[uint8][]byte{
				0x10: {1},
				0x11: {5, 0x81, 0x00, 0x00, 0x00},
				0x12: {0},
				0x13: {1},
				0x14: {7},
				0x15: {1},
				0x17: {1},
				0x18: {0},
				0x19: {1},
				0x1A: {1},
				0x1B: {0},
				0x1C: {1},
				0x1D: {1},
				0x1E: {1},
			},
		},
		{
			name: "reserved statistics mask",
			config: WDSSetEventReportConfig{
				TransferStatistics: &WDSTransferStatisticsConfig{Indicators: WDSStatisticsMask(1 << 31)},
			},
			wantErr: "reserved bits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (WDSSetEventReportRequest{
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
			if got.Service != ServiceWDS || got.ClientID != 7 || got.TransactionID != 9 ||
				got.MessageID != MessageWDSSetEventReport || got.Timeout != 2*time.Second {
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

func TestWDSEventReportUnmarshalTLVs(t *testing.T) {
	currentBearer := []byte{byte(WDSCurrentBearerNetwork3GPP), 0x20, 0, 0, 0, 0x01, 0, 0, 0}
	dataSystems := []byte{byte(WDSDataSystem3GPP), 1, byte(WDSDataSystem3GPP2), 0x20, 0, 0, 0, 0x02, 0, 0, 0}
	filters := append([]byte{2}, binary.LittleEndian.AppendUint32(nil, 11)...)
	filters = append(filters, binary.LittleEndian.AppendUint32(nil, 12)...)
	extendedBearer := append(binary.LittleEndian.AppendUint32(nil, uint32(WDSBearerNetwork3GPP)), binary.LittleEndian.AppendUint32(nil, uint32(WDSBearerRATLTE))...)
	extendedBearer = binary.LittleEndian.AppendUint64(extendedBearer, uint64(WDSBearerServiceOptionLTEFDD))
	tlvs := tlv.TLVs{
		tlv.Bytes(0x10, binary.LittleEndian.AppendUint32(nil, 10)),
		tlv.Bytes(0x11, binary.LittleEndian.AppendUint32(nil, 20)),
		tlv.Bytes(0x12, binary.LittleEndian.AppendUint32(nil, 3)),
		tlv.Bytes(0x13, binary.LittleEndian.AppendUint32(nil, 4)),
		tlv.Bytes(0x14, binary.LittleEndian.AppendUint32(nil, 5)),
		tlv.Bytes(0x15, binary.LittleEndian.AppendUint32(nil, 6)),
		tlv.Bytes(0x16, append(binary.LittleEndian.AppendUint32(nil, 1000), binary.LittleEndian.AppendUint32(nil, 2000)...)),
		tlv.Bytes(0x17, []byte{byte(WDSDataBearerLTE)}),
		tlv.Bytes(0x18, []byte{byte(WDSDormancyActive)}),
		tlv.Bytes(0x19, binary.LittleEndian.AppendUint64(nil, 1000)),
		tlv.Bytes(0x1A, binary.LittleEndian.AppendUint64(nil, 2000)),
		tlv.Bytes(0x1B, []byte{0}),
		tlv.Bytes(0x1D, currentBearer),
		tlv.Bytes(0x1F, []byte{1}),
		tlv.Bytes(0x20, binary.LittleEndian.AppendUint32(nil, uint32(WDSPreferredDataSystemLTE))),
		tlv.Bytes(0x22, []byte{byte(WDSEventDataCallTethered), byte(WDSEventTetheredCallRMNET)}),
		tlv.Bytes(0x23, []byte{3, 1}),
		tlv.Bytes(0x24, dataSystems),
		tlv.Bytes(0x25, binary.LittleEndian.AppendUint32(nil, 7)),
		tlv.Bytes(0x26, binary.LittleEndian.AppendUint32(nil, 8)),
		tlv.Bytes(0x27, []byte{1}),
		tlv.Bytes(0x28, binary.LittleEndian.AppendUint32(nil, uint32(WDSDataCallAddressFamilyIPv6))),
		tlv.Bytes(0x29, filters),
		tlv.Bytes(0x2A, extendedBearer),
	}

	var got WDSEventReport
	if err := got.UnmarshalTLVs(tlvs); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if got.Statistics.TxPackets != 10 || got.Statistics.RxPackets != 20 || got.Statistics.TxDropped != 7 || got.Statistics.RxDropped != 8 || got.Statistics.TxBytes != 1000 || got.Statistics.RxBytes != 2000 {
		t.Fatalf("statistics = %+v", got.Statistics)
	}
	if !got.ChannelRatesKnown || got.ChannelRates.CurrentTx != 1000 || got.ChannelRates.CurrentRx != 2000 {
		t.Fatalf("channel rates = %+v", got.ChannelRates)
	}
	if !got.DataBearerKnown || got.DataBearer != WDSDataBearerLTE || !got.DormancyKnown || got.Dormancy != WDSDormancyActive || !got.MIPStatusKnown {
		t.Fatalf("basic state = %+v", got)
	}
	if !got.CurrentDataBearerKnown || got.CurrentDataBearer.Network != WDSCurrentBearerNetwork3GPP || !got.DataCallStatusKnown || got.DataCallStatus != WDSDataCallStatusActivated {
		t.Fatalf("bearer/status = %+v", got)
	}
	if !got.PreferredDataSystemKnown || got.PreferredDataSystem != WDSPreferredDataSystemLTE || !got.DataCallKnown || got.DataCall.TetheredType != WDSEventTetheredCallRMNET {
		t.Fatalf("call metadata = %+v", got)
	}
	if !got.EVDOPageMonitorKnown || !got.EVDOPageMonitor.ForceLongSleep || !got.DataSystemsKnown || len(got.DataSystems.Networks) != 1 {
		t.Fatalf("data systems = %+v", got)
	}
	if !got.UplinkFlowControlKnown || !got.UplinkFlowControlled || !got.AddressFamilyKnown || got.AddressFamily != WDSDataCallAddressFamilyIPv6 {
		t.Fatalf("flow/address = %+v", got)
	}
	if !got.RemovedFilterHandlesKnown || len(got.RemovedFilterHandles) != 2 || got.RemovedFilterHandles[1] != 12 || !got.ExtendedDataBearerKnown || got.ExtendedDataBearer.RAT != WDSBearerRATLTE {
		t.Fatalf("filters/extended bearer = %+v/%+v", got.RemovedFilterHandles, got.ExtendedDataBearer)
	}
}

func TestWDSEventReportRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want string
	}{
		{name: "counter length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, want: "TLV 0x10 length"},
		{name: "bytes length", tlvs: tlv.TLVs{tlv.Bytes(0x19, []byte{1})}, want: "TLV 0x19 length"},
		{name: "channel rates length", tlvs: tlv.TLVs{tlv.Bytes(0x16, []byte{1})}, want: "channel rates TLV length"},
		{name: "current bearer length", tlvs: tlv.TLVs{tlv.Bytes(0x1D, []byte{1})}, want: "current data bearer TLV length"},
		{name: "data systems length", tlvs: tlv.TLVs{tlv.Bytes(0x24, []byte{0, 1})}, want: "data systems TLV length"},
		{name: "filter length", tlvs: tlv.TLVs{tlv.Bytes(0x29, []byte{1})}, want: "removed filter TLV length"},
		{name: "extended bearer length", tlvs: tlv.TLVs{tlv.Bytes(0x2A, []byte{1})}, want: "extended data bearer TLV length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSEventReport
			err := got.UnmarshalTLVs(tt.tlvs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestClientWDSSetEventReport(t *testing.T) {
	enabled := true
	transport := &fakeTransport{t: t, calls: []transportCall{{
		check: func(req Request) {
			if req.Service != ServiceWDS || req.ClientID != 7 || req.MessageID != MessageWDSSetEventReport {
				t.Fatalf("request = %+v", req)
			}
			assertTLV(t, req.TLVs, 0x10, []byte{1})
		},
		resp: successResponse(MessageWDSSetEventReport),
	}}}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
	if err := client.WDSSetEventReport(context.Background(), WDSSetEventReportConfig{ChannelRate: &enabled}); err != nil {
		t.Fatalf("WDSSetEventReport() error = %v", err)
	}
}
