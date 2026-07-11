package uim

import (
	"context"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestIMSSSetTestModeRequest(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    byte
	}{
		{name: "enable", enabled: true, want: 1},
		{name: "disable", enabled: false, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := IMSSSetTestModeRequest{ClientID: 7, Enabled: tt.enabled}.Request()
			if req.Service != qcom.ServiceIMSS || req.MessageID != qcom.MessageIMSSSetRegistrationManagerConfig {
				t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
			}
			assertTLV(t, req.TLVs, imssTLVSetTestMode, []byte{tt.want})
		})
	}
}

func TestIMSSTestModeResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    bool
		wantErr bool
	}{
		{name: "enabled", tlvs: tlv.TLVs{tlv.Bytes(imssTLVGetTestMode, []byte{1})}, want: true},
		{name: "disabled", tlvs: tlv.TLVs{tlv.Bytes(imssTLVGetTestMode, []byte{0})}},
		{name: "missing", wantErr: true},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(imssTLVGetTestMode, nil)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSSTestModeResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Enabled != tt.want {
				t.Fatalf("Enabled = %v, want %v", got.Enabled, tt.want)
			}
		})
	}
}

func TestReaderIMSSTestMode(t *testing.T) {
	transport := &fakeTransport{t: t, calls: []transportCall{
		{resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceIMSS), 5}))},
		{
			check: func(req qcom.Request) {
				if req.Service != qcom.ServiceIMSS || req.ClientID != 5 || req.MessageID != qcom.MessageIMSSGetRegistrationManagerConfig {
					t.Fatalf("unexpected IMSS get request: %+v", req)
				}
			},
			resp: successResponse(qcom.MessageIMSSGetRegistrationManagerConfig, tlv.Bytes(imssTLVGetTestMode, []byte{1})),
		},
		{resp: successResponse(qcom.MessageReleaseClientID)},
	}}
	reader := &Reader{transport: transport, slot: 1}

	enabled, err := reader.IMSSTestMode(context.Background())
	if err != nil {
		t.Fatalf("IMSSTestMode() error = %v", err)
	}
	if !enabled {
		t.Fatal("IMSSTestMode() = false, want true")
	}
}

func TestReaderSetIMSSTestMode(t *testing.T) {
	transport := &fakeTransport{t: t, calls: []transportCall{
		{resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceIMSS), 6}))},
		{
			check: func(req qcom.Request) {
				if req.Service != qcom.ServiceIMSS || req.ClientID != 6 || req.MessageID != qcom.MessageIMSSSetRegistrationManagerConfig {
					t.Fatalf("unexpected IMSS set request: %+v", req)
				}
				assertTLV(t, req.TLVs, imssTLVSetTestMode, []byte{1})
			},
			resp: successResponse(qcom.MessageIMSSSetRegistrationManagerConfig),
		},
		{resp: successResponse(qcom.MessageReleaseClientID)},
	}}
	reader := &Reader{transport: transport, slot: 1}

	if err := reader.SetIMSSTestMode(context.Background(), true); err != nil {
		t.Fatalf("SetIMSSTestMode() error = %v", err)
	}
}
