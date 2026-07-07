package uim

import (
	"context"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestIMSARequestEncoding(t *testing.T) {
	tests := []struct {
		name          string
		req           qcom.Request
		wantMessageID qcom.MessageID
	}{
		{
			name: "registration status",
			req: IMSAGetRegistrationStatusRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
			}.Request(),
			wantMessageID: qcom.MessageIMSAGetRegistrationStatus,
		},
		{
			name: "service status",
			req: IMSAGetServiceStatusRequest{
				ClientID:      8,
				TransactionID: 10,
				Timeout:       4 * time.Second,
			}.Request(),
			wantMessageID: qcom.MessageIMSAGetServiceStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Service != qcom.ServiceIMSA {
				t.Fatalf("Service = 0x%02X, want 0x%02X", tt.req.Service, qcom.ServiceIMSA)
			}
			if tt.req.MessageID != tt.wantMessageID {
				t.Fatalf("MessageID = 0x%04X, want 0x%04X", tt.req.MessageID, tt.wantMessageID)
			}
			if len(tt.req.TLVs) != 0 {
				t.Fatalf("TLVs len = %d, want 0", len(tt.req.TLVs))
			}
		})
	}
}

func TestIMSARegistrationStatusResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name        string
		tlvs        tlv.TLVs
		wantErr     bool
		wantKnown   bool
		wantStatus  qcom.IMSRegistrationStatus
		wantFailure bool
		wantCode    uint16
	}{
		{name: "missing status"},
		{
			name:       "new status registered",
			tlvs:       tlv.TLVs{tlv.Uint(imsaTLVRegStatus, uint32(qcom.IMSRegistrationStatusRegistered))},
			wantKnown:  true,
			wantStatus: qcom.IMSRegistrationStatusRegistered,
		},
		{
			name:       "new status wins over old boolean",
			tlvs:       tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, []byte{0}), tlv.Uint(imsaTLVRegStatus, uint32(qcom.IMSRegistrationStatusRegistering))},
			wantKnown:  true,
			wantStatus: qcom.IMSRegistrationStatusRegistering,
		},
		{
			name:       "old boolean registered",
			tlvs:       tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, []byte{1})},
			wantKnown:  true,
			wantStatus: qcom.IMSRegistrationStatusRegistered,
		},
		{
			name:        "failure code",
			tlvs:        tlv.TLVs{tlv.Uint(imsaTLVRegStatus, uint32(qcom.IMSRegistrationStatusNotRegistered)), tlv.Uint(imsaTLVFailureCode, uint16(403))},
			wantKnown:   true,
			wantFailure: true,
			wantCode:    403,
		},
		{name: "truncated new status", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegStatus, []byte{1})}, wantErr: true},
		{name: "truncated old boolean", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, nil)}, wantErr: true},
		{name: "truncated failure code", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVFailureCode, []byte{1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSARegistrationStatusResponse
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
			if got.Status.RegistrationKnown != tt.wantKnown {
				t.Fatalf("RegistrationKnown = %v, want %v", got.Status.RegistrationKnown, tt.wantKnown)
			}
			if got.Status.Registration != tt.wantStatus {
				t.Fatalf("Registration = %d, want %d", got.Status.Registration, tt.wantStatus)
			}
			if got.Status.FailureCodeKnown != tt.wantFailure {
				t.Fatalf("FailureCodeKnown = %v, want %v", got.Status.FailureCodeKnown, tt.wantFailure)
			}
			if got.Status.FailureCode != tt.wantCode {
				t.Fatalf("FailureCode = %d, want %d", got.Status.FailureCode, tt.wantCode)
			}
		})
	}
}

func TestIMSAServiceStatusResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name            string
		tlvs            tlv.TLVs
		wantErr         bool
		wantService     qcom.IMSServiceStatus
		wantServiceSeen bool
		wantRAT         qcom.IMSServiceRAT
		wantRATSeen     bool
	}{
		{name: "missing service"},
		{
			name:            "volte service",
			tlvs:            tlv.TLVs{tlv.Uint(imsaTLVVoIPService, uint32(qcom.IMSServiceStatusFullService)), tlv.Uint(imsaTLVVoIPRAT, uint32(qcom.IMSServiceRATWWAN))},
			wantService:     qcom.IMSServiceStatusFullService,
			wantServiceSeen: true,
			wantRAT:         qcom.IMSServiceRATWWAN,
			wantRATSeen:     true,
		},
		{name: "truncated service", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPService, []byte{1})}, wantErr: true},
		{name: "truncated rat", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPRAT, []byte{1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSAServiceStatusResponse
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
			if got.Status.VoIPServiceKnown != tt.wantServiceSeen {
				t.Fatalf("VoIPServiceKnown = %v, want %v", got.Status.VoIPServiceKnown, tt.wantServiceSeen)
			}
			if got.Status.VoIPService != tt.wantService {
				t.Fatalf("VoIPService = %d, want %d", got.Status.VoIPService, tt.wantService)
			}
			if got.Status.VoIPRATKnown != tt.wantRATSeen {
				t.Fatalf("VoIPRATKnown = %v, want %v", got.Status.VoIPRATKnown, tt.wantRATSeen)
			}
			if got.Status.VoIPRAT != tt.wantRAT {
				t.Fatalf("VoIPRAT = %d, want %d", got.Status.VoIPRAT, tt.wantRAT)
			}
		})
	}
}

func TestReaderIMSAStatus(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageAllocateClientID {
						t.Fatalf("allocate request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceIMSA)})
				},
				resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceIMSA), 5})),
			},
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceIMSA || req.ClientID != 5 || req.MessageID != qcom.MessageIMSAGetRegistrationStatus {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X, want IMSA registration", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: successResponse(qcom.MessageIMSAGetRegistrationStatus, tlv.Uint(imsaTLVRegStatus, uint32(qcom.IMSRegistrationStatusRegistered))),
			},
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceIMSA || req.ClientID != 5 || req.MessageID != qcom.MessageIMSAGetServiceStatus {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X, want IMSA service", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: successResponse(qcom.MessageIMSAGetServiceStatus,
					tlv.Uint(imsaTLVVoIPService, uint32(qcom.IMSServiceStatusFullService)),
					tlv.Uint(imsaTLVVoIPRAT, uint32(qcom.IMSServiceRATWWAN))),
			},
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageReleaseClientID {
						t.Fatalf("release request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceIMSA), 5})
				},
				resp: successResponse(qcom.MessageReleaseClientID),
			},
		},
	}
	reader := &Reader{
		transport: transport,
		slot:      1,
	}

	got, err := reader.IMSAStatus(context.Background())
	if err != nil {
		t.Fatalf("IMSAStatus() error = %v", err)
	}
	if !got.IMSRegistered() {
		t.Fatal("IMSRegistered() = false, want true")
	}
	if !got.VoLTERegistered() {
		t.Fatal("VoLTERegistered() = false, want true")
	}
	if got := transport.callCount(); got != 4 {
		t.Fatalf("Do() calls = %d, want 4", got)
	}
}
