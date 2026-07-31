package qcom

import (
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestSSCControlRequest(t *testing.T) {
	small := SSCReportSmall
	invalid := SSCReportLarge + 1
	tests := []struct {
		name       string
		data       []byte
		reportType *SSCReportType
		wantReport bool
		wantError  bool
	}{
		{name: "data only", data: []byte{1, 2, 3}},
		{name: "small reports", data: []byte{4, 5}, reportType: &small, wantReport: true},
		{name: "invalid report type", reportType: &invalid, wantError: true},
		{name: "data too large", data: make([]byte, MaxQRTRServiceTLVLength-8), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (SSCControlRequest{ClientID: 7, TransactionID: 9, Data: tt.data, ReportType: tt.reportType}).Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if req.Service != ServiceSSC || req.ClientID != 7 || req.TransactionID != 9 || req.MessageID != MessageSSCControl {
				t.Fatalf("Request() = %+v, want SSC Control", req)
			}
			wantData := binary.LittleEndian.AppendUint16(nil, uint16(len(tt.data)))
			wantData = append(wantData, tt.data...)
			assertTLV(t, req.TLVs, 0x01, wantData)
			_, gotReport := tlv.Value(req.TLVs, 0x10)
			if gotReport != tt.wantReport {
				t.Fatalf("Report Type present = %v, want %v", gotReport, tt.wantReport)
			}
		})
	}
}

func TestSSCControlResponseUnmarshalTLVs(t *testing.T) {
	clientID := binary.LittleEndian.AppendUint64(nil, 0x0102030405060708)
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{name: "all fields", tlvs: tlv.TLVs{tlv.Bytes(0x10, clientID), tlv.Uint(0x11, uint32(9))}},
		{name: "optional fields absent"},
		{name: "client ID truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 7))}, wantError: true},
		{name: "client ID trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 9))}, wantError: true},
		{name: "response truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, make([]byte, 3))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SSCControlResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "all fields" && (!got.ClientIDKnown || got.ClientID != 0x0102030405060708 || !got.ResponseKnown || got.Response != 9) {
				t.Fatalf("UnmarshalTLVs() = %+v, want all fields", got)
			}
		})
	}
}

func TestSSCReportUnmarshalTLVs(t *testing.T) {
	data := binary.LittleEndian.AppendUint16(nil, 3)
	data = append(data, 1, 2, 3)
	clientID := binary.LittleEndian.AppendUint64(nil, 7)
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{name: "report", tlvs: tlv.TLVs{tlv.Bytes(0x01, clientID), tlv.Bytes(0x02, data)}},
		{name: "client ID missing", tlvs: tlv.TLVs{tlv.Bytes(0x02, data)}, wantError: true},
		{name: "client ID trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x01, make([]byte, 9)), tlv.Bytes(0x02, data)}, wantError: true},
		{name: "data missing", tlvs: tlv.TLVs{tlv.Bytes(0x01, clientID)}, wantError: true},
		{name: "data count mismatch", tlvs: tlv.TLVs{tlv.Bytes(0x01, clientID), tlv.Bytes(0x02, []byte{3, 0, 1, 2})}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SSCReport
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "report" && (got.ClientID != 7 || len(got.Data) != 3 || got.Data[2] != 3) {
				t.Fatalf("UnmarshalTLVs() = %+v, want decoded report", got)
			}
		})
	}
}
