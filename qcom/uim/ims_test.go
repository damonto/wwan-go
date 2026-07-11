package uim

import (
	"bytes"
	"context"
	"errors"
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

func TestOpenIMSPDNBindsDataPortBeforeStartingNetwork(t *testing.T) {
	tests := []struct {
		name          string
		cfg           IMSPDNConfig
		wantMessageID qcom.MessageID
		wantTLVs      tlv.TLVs
	}{
		{
			name: "mux data port",
			cfg: IMSPDNConfig{MuxDataPort: &qcom.WDSMuxDataPort{
				Endpoint: &qcom.WDSDataEndpoint{Type: qcom.WDSDataEndpointBAMDMUX, InterfaceID: 1},
				MuxID:    2,
			}},
			wantMessageID: qcom.MessageWDSBindMuxDataPort,
			wantTLVs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0x05, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}),
				tlv.Bytes(0x11, []byte{0x02}),
			},
		},
		{
			name:          "legacy mux data port",
			cfg:           IMSPDNConfig{LegacyMuxDataPort: qcom.WDSSIOPortA2MuxRMNET1},
			wantMessageID: qcom.MessageWDSLegacyBindMuxDataPort,
			wantTLVs:      tlv.TLVs{tlv.Bytes(0x01, []byte{0x05, 0x0E})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(qcom.ServiceUIM, 1)},
				{resp: allocatedClientResponse(qcom.ServiceWDS, 2)},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceWDS {
							t.Fatalf("bind Service = 0x%02X, want 0x%02X", req.Service, qcom.ServiceWDS)
						}
						if req.ClientID != 2 {
							t.Fatalf("bind ClientID = %d, want 2", req.ClientID)
						}
						if req.MessageID != tt.wantMessageID {
							t.Fatalf("bind MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.wantMessageID)
						}
						for _, want := range tt.wantTLVs {
							got, ok := tlv.Value(req.TLVs, want.Type)
							if !ok || !bytes.Equal(got, want.Value) {
								t.Fatalf("bind TLV 0x%02X = % X, want % X", want.Type, got, want.Value)
							}
						}
					},
					resp: successResponse(tt.wantMessageID),
				},
				{resp: allocatedClientResponse(qcom.ServiceNAS, 3)},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageWDSStartNetworkInterface {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageWDSStartNetworkInterface)
						}
						if req.ClientID != 2 {
							t.Fatalf("start ClientID = %d, want bound WDS ClientID 2", req.ClientID)
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

func TestOpenIMSPDNBindDataPortFailureReleasesWDSClient(t *testing.T) {
	tests := []struct {
		name          string
		cfg           IMSPDNConfig
		wantMessageID qcom.MessageID
	}{
		{
			name:          "mux data port",
			cfg:           IMSPDNConfig{MuxDataPort: &qcom.WDSMuxDataPort{MuxID: 1}},
			wantMessageID: qcom.MessageWDSBindMuxDataPort,
		},
		{
			name:          "legacy mux data port",
			cfg:           IMSPDNConfig{LegacyMuxDataPort: qcom.WDSSIOPortA2MuxRMNET0},
			wantMessageID: qcom.MessageWDSLegacyBindMuxDataPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(qcom.ServiceUIM, 1)},
				{resp: allocatedClientResponse(qcom.ServiceWDS, 2)},
				{resp: errorResponse(tt.wantMessageID, qcom.QMIErrorNotSupported)},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageReleaseClientID {
							t.Fatalf("MessageID = 0x%04X, want release client", req.MessageID)
						}
						got, ok := tlv.Value(req.TLVs, 0x01)
						if !ok || !bytes.Equal(got, []byte{byte(qcom.ServiceWDS), 2}) {
							t.Fatalf("released client = % X, want WDS client 2", got)
						}
					},
					resp: successResponse(qcom.MessageReleaseClientID),
				},
				{resp: successResponse(qcom.MessageReleaseClientID)},
			}}

			reader, err := New(context.Background(), transport)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			_, err = reader.OpenIMSPDN(context.Background(), tt.cfg)
			if !errors.Is(err, qcom.QMIErrorNotSupported) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, qcom.QMIErrorNotSupported)
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

func TestOpenIMSPDNRejectsConflictingDataPorts(t *testing.T) {
	tests := []struct {
		name string
		cfg  IMSPDNConfig
		want string
	}{
		{
			name: "modern and legacy",
			cfg: IMSPDNConfig{
				MuxDataPort:       &qcom.WDSMuxDataPort{MuxID: 1},
				LegacyMuxDataPort: qcom.WDSSIOPortA2MuxRMNET0,
			},
			want: "opening IMS PDN: mux data port and legacy mux data port are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Reader{}).OpenIMSPDN(context.Background(), tt.cfg)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("OpenIMSPDN() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func allocatedClientResponse(service qcom.ServiceType, clientID uint8) qcom.Response {
	return successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(service), clientID}))
}
