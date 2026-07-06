package uim

import (
	"bytes"
	"context"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestOpenIMSPDNNormalizesAPN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     IMSPDNConfig
		wantAPN string
	}{
		{name: "default", wantAPN: DefaultIMSPDNAPN},
		{name: "trimmed", cfg: IMSPDNConfig{APN: " ims "}, wantAPN: DefaultIMSPDNAPN},
		{name: "custom", cfg: IMSPDNConfig{APN: " sos "}, wantAPN: "sos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(qcom.ServiceUIM, 1)},
				{resp: allocatedClientResponse(qcom.ServiceWDS, 2)},
				{resp: allocatedClientResponse(qcom.ServiceNAS, 3)},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageWDSStartNetworkInterface {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageWDSStartNetworkInterface)
						}
						got, ok := tlv.Value(req.TLVs, 0x14)
						if !ok {
							t.Fatal("APN TLV missing")
						}
						if !bytes.Equal(got, []byte(tt.wantAPN)) {
							t.Fatalf("APN = %q, want %q", got, tt.wantAPN)
						}
					},
					resp: successResponse(qcom.MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{resp: successResponse(qcom.MessageWDSGetRuntimeSettings)},
				{resp: successResponse(qcom.MessageNASGetSysInfo, tlv.Bytes(0x29, []byte{1}))},
				{resp: successResponse(qcom.MessageWDSStopNetworkInterface)},
				{resp: successResponse(qcom.MessageReleaseClientID)},
				{resp: successResponse(qcom.MessageReleaseClientID)},
				{resp: successResponse(qcom.MessageReleaseClientID)},
			}}

			reader, err := New(context.Background(), transport)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			session, err := reader.OpenIMSPDN(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func allocatedClientResponse(service qcom.ServiceType, clientID uint8) qcom.Response {
	return successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(service), clientID}))
}
