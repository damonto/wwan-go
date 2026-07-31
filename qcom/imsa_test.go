package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestIMSARequestEncoding(t *testing.T) {
	tests := []struct {
		name          string
		req           Request
		wantMessageID MessageID
	}{
		{
			name: "registration status",
			req: IMSAGetRegistrationStatusRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
			}.Request(),
			wantMessageID: MessageIMSAGetRegistrationStatus,
		},
		{
			name: "service status",
			req: IMSAGetServiceStatusRequest{
				ClientID:      8,
				TransactionID: 10,
				Timeout:       4 * time.Second,
			}.Request(),
			wantMessageID: MessageIMSAGetServiceStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Service != ServiceIMSA {
				t.Fatalf("Service = 0x%02X, want 0x%02X", tt.req.Service, ServiceIMSA)
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
		wantStatus  IMSRegistrationStatus
		wantFailure bool
		wantCode    uint16
	}{
		{name: "missing status"},
		{
			name:       "new status registered",
			tlvs:       tlv.TLVs{tlv.Uint(imsaTLVRegStatus, uint32(IMSRegistrationStatusRegistered))},
			wantKnown:  true,
			wantStatus: IMSRegistrationStatusRegistered,
		},
		{
			name:       "new status wins over old boolean",
			tlvs:       tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, []byte{0}), tlv.Uint(imsaTLVRegStatus, uint32(IMSRegistrationStatusRegistering))},
			wantKnown:  true,
			wantStatus: IMSRegistrationStatusRegistering,
		},
		{
			name:       "old boolean registered",
			tlvs:       tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, []byte{1})},
			wantKnown:  true,
			wantStatus: IMSRegistrationStatusRegistered,
		},
		{
			name:        "failure code",
			tlvs:        tlv.TLVs{tlv.Uint(imsaTLVRegStatus, uint32(IMSRegistrationStatusNotRegistered)), tlv.Uint(imsaTLVFailureCode, uint16(403))},
			wantKnown:   true,
			wantFailure: true,
			wantCode:    403,
		},
		{name: "truncated new status", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegStatus, []byte{1})}, wantErr: true},
		{name: "new status trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegStatus, make([]byte, 5))}, wantErr: true},
		{name: "truncated old boolean", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, nil)}, wantErr: true},
		{name: "old boolean trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVIMSRegistered, []byte{1, 0})}, wantErr: true},
		{name: "truncated failure code", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVFailureCode, []byte{1})}, wantErr: true},
		{name: "failure code trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVFailureCode, make([]byte, 3))}, wantErr: true},
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
		wantService     IMSServiceStatus
		wantServiceSeen bool
		wantRAT         IMSServiceRAT
		wantRATSeen     bool
	}{
		{name: "missing service"},
		{
			name:            "volte service",
			tlvs:            tlv.TLVs{tlv.Uint(imsaTLVVoIPService, uint32(IMSServiceStatusFullService)), tlv.Uint(imsaTLVVoIPRAT, uint32(IMSServiceRATWWAN))},
			wantService:     IMSServiceStatusFullService,
			wantServiceSeen: true,
			wantRAT:         IMSServiceRATWWAN,
			wantRATSeen:     true,
		},
		{name: "truncated service", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPService, []byte{1})}, wantErr: true},
		{name: "service trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPService, make([]byte, 5))}, wantErr: true},
		{name: "truncated rat", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPRAT, []byte{1})}, wantErr: true},
		{name: "rat trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVVoIPRAT, make([]byte, 5))}, wantErr: true},
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

func TestIMSARegistrationDetails(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{
			name: "error and RAT",
			tlvs: tlv.TLVs{
				tlv.Bytes(imsaTLVRegistrationError, []byte("forbidden")),
				tlv.Uint(imsaTLVRegistrationRAT, uint32(IMSServiceRATIWLAN)),
			},
		},
		{name: "RAT truncated", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegistrationRAT, make([]byte, 3))}, wantError: true},
		{name: "RAT trailing byte", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegistrationRAT, make([]byte, 5))}, wantError: true},
		{name: "error message too long", tlvs: tlv.TLVs{tlv.Bytes(imsaTLVRegistrationError, make([]byte, imsaRegistrationErrorMaxLength+1))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSARegistrationStatusResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "error and RAT" && (!got.Status.RegistrationErrorMessageKnown || got.Status.RegistrationErrorMessage != "forbidden" || !got.Status.RegistrationRATKnown || got.Status.RegistrationRAT != IMSServiceRATIWLAN) {
				t.Fatalf("UnmarshalTLVs() = %+v, want error and RAT", got.Status)
			}
		})
	}
}

func TestIMSARegistrationStatusIndicationUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x10, uint16(403)),
				tlv.Uint(0x11, uint32(IMSRegistrationStatusLimited)),
				tlv.Bytes(0x12, []byte("limited")),
				tlv.Uint(0x13, uint32(IMSServiceRATWWAN)),
			},
		},
		{name: "legacy registered", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}},
		{name: "modern status wins", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{0}), tlv.Uint(0x11, uint32(IMSRegistrationStatusRegistering))}},
		{name: "legacy status trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})}, wantError: true},
		{name: "status trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x11, make([]byte, 5))}, wantError: true},
		{name: "failure code trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 3))}, wantError: true},
		{name: "RAT trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x13, make([]byte, 5))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSARegistrationStatusIndication
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "all fields" && (!got.Status.RegistrationKnown || got.Status.Registration != IMSRegistrationStatusLimited || !got.Status.FailureCodeKnown || got.Status.FailureCode != 403 || !got.Status.RegistrationErrorMessageKnown || got.Status.RegistrationErrorMessage != "limited" || !got.Status.RegistrationRATKnown || got.Status.RegistrationRAT != IMSServiceRATWWAN) {
				t.Fatalf("UnmarshalTLVs() = %+v, want all fields", got.Status)
			}
			if tt.name == "legacy registered" && (!got.Status.RegistrationKnown || got.Status.Registration != IMSRegistrationStatusRegistered) {
				t.Fatalf("UnmarshalTLVs() = %+v, want legacy registered", got.Status)
			}
			if tt.name == "modern status wins" && got.Status.Registration != IMSRegistrationStatusRegistering {
				t.Fatalf("Registration = %d, want registering", got.Status.Registration)
			}
		})
	}
}

func TestIMSAAllServiceStatusFields(t *testing.T) {
	tlvs := tlv.TLVs{
		tlv.Uint(imsaTLVSMSService, uint32(IMSServiceStatusLimitedService)),
		tlv.Uint(imsaTLVVoIPService, uint32(IMSServiceStatusFullService)),
		tlv.Uint(imsaTLVVTService, uint32(IMSServiceStatusNoService)),
		tlv.Uint(imsaTLVSMSRAT, uint32(IMSServiceRATWLAN)),
		tlv.Uint(imsaTLVVoIPRAT, uint32(IMSServiceRATWWAN)),
		tlv.Uint(imsaTLVVTRAT, uint32(IMSServiceRATIWLAN)),
		tlv.Uint(imsaTLVUTService, uint32(IMSServiceStatusFullService)),
		tlv.Uint(imsaTLVUTRAT, uint32(IMSServiceRATWWAN)),
		tlv.Uint(imsaTLVVSService, uint32(IMSServiceStatusLimitedService)),
		tlv.Uint(imsaTLVVSRAT, uint32(IMSServiceRATWLAN)),
	}
	tests := []struct {
		name   string
		decode func(tlv.TLVs) (IMSAStatus, error)
	}{
		{
			name: "response",
			decode: func(tlvs tlv.TLVs) (IMSAStatus, error) {
				var got IMSAServiceStatusResponse
				err := got.UnmarshalTLVs(tlvs)
				return got.Status, err
			},
		},
		{
			name: "indication",
			decode: func(tlvs tlv.TLVs) (IMSAStatus, error) {
				var got IMSAServiceStatusIndication
				err := got.UnmarshalTLVs(tlvs)
				return got.Status, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode(tlvs)
			if err != nil {
				t.Fatalf("decode() error = %v", err)
			}
			if !got.SMSServiceKnown || got.SMSService != IMSServiceStatusLimitedService || !got.SMSRATKnown || got.SMSRAT != IMSServiceRATWLAN ||
				!got.VoIPServiceKnown || got.VoIPService != IMSServiceStatusFullService || !got.VoIPRATKnown || got.VoIPRAT != IMSServiceRATWWAN ||
				!got.VTServiceKnown || got.VTService != IMSServiceStatusNoService || !got.VTRATKnown || got.VTRAT != IMSServiceRATIWLAN ||
				!got.UTServiceKnown || got.UTService != IMSServiceStatusFullService || !got.UTRATKnown || got.UTRAT != IMSServiceRATWWAN ||
				!got.VSServiceKnown || got.VSService != IMSServiceStatusLimitedService || !got.VSRATKnown || got.VSRAT != IMSServiceRATWLAN {
				t.Fatalf("decode() = %+v, want every service field", got)
			}
		})
	}
}

func TestIMSAControlRequestEncoding(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[uint8][]byte
		wantError   bool
	}{
		{
			name: "register indications",
			request: func() (Request, error) {
				return (IMSARegisterIndicationsRequest{Config: IMSAIndicationConfig{RegistrationStatus: &enabled, ServiceStatus: &disabled}}).Request(), nil
			},
			wantMessage: MessageIMSARegisterIndications,
			wantTLVs:    map[uint8][]byte{0x10: {1}, 0x11: {0}},
		},
		{
			name: "bind secondary",
			request: func() (Request, error) {
				return (IMSABindRequest{Subscription: IMSASubscriptionSecondary}).Request()
			},
			wantMessage: MessageIMSABind,
			wantTLVs:    map[uint8][]byte{0x10: binary.LittleEndian.AppendUint32(nil, uint32(IMSASubscriptionSecondary))},
		},
		{
			name: "get bind",
			request: func() (Request, error) {
				return (IMSAGetBindRequest{}).Request(), nil
			},
			wantMessage: MessageIMSAGetBind,
		},
		{
			name: "invalid binding",
			request: func() (Request, error) {
				return (IMSABindRequest{Subscription: IMSASubscriptionTertiary + 1}).Request()
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if got.Service != ServiceIMSA || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%02X message 0x%04X", got.Service, got.MessageID)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for typ, want := range tt.wantTLVs {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestIMSAGetBindResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		want      IMSAGetBindResponse
		wantError bool
	}{
		{name: "not reported"},
		{name: "tertiary", tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(IMSASubscriptionTertiary))}, want: IMSAGetBindResponse{Subscription: IMSASubscriptionTertiary, SubscriptionKnown: true}},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 3))}, wantError: true},
		{name: "trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 5))}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSAGetBindResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIMSAWatchers(t *testing.T) {
	tests := []struct {
		name        string
		message     MessageID
		registerTLV uint8
		watch       func(context.Context, *Client) (<-chan IMSAStatus, error)
		indication  Indication
		matches     func(IMSAStatus) bool
	}{
		{
			name:        "registration",
			message:     MessageIMSARegistrationChanged,
			registerTLV: 0x10,
			watch: func(ctx context.Context, c *Client) (<-chan IMSAStatus, error) {
				return c.WatchIMSARegistrationStatus(ctx)
			},
			indication: Indication{MessageID: MessageIMSARegistrationChanged, TLVs: tlv.TLVs{tlv.Uint(0x11, uint32(IMSRegistrationStatusRegistered))}},
			matches:    func(status IMSAStatus) bool { return status.IMSRegistered() },
		},
		{
			name:        "services",
			message:     MessageIMSAServiceStatusChanged,
			registerTLV: 0x11,
			watch:       func(ctx context.Context, c *Client) (<-chan IMSAStatus, error) { return c.WatchIMSAServiceStatus(ctx) },
			indication: Indication{MessageID: MessageIMSAServiceStatusChanged, TLVs: tlv.TLVs{
				tlv.Uint(imsaTLVVoIPService, uint32(IMSServiceStatusFullService)),
				tlv.Uint(imsaTLVVoIPRAT, uint32(IMSServiceRATWWAN)),
			}},
			matches: func(status IMSAStatus) bool {
				return status.VoIPServiceKnown && status.VoIPService == IMSServiceStatusFullService && status.VoIPRATKnown && status.VoIPRAT == IMSServiceRATWWAN
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &dsdIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessageIMSARegisterIndications {
							t.Fatalf("register MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageIMSARegisterIndications)
						}
						assertTLV(t, req.TLVs, tt.registerTLV, []byte{1})
					},
					resp: successResponse(MessageIMSARegisterIndications),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, tt.registerTLV, []byte{0})
					},
					resp: successResponse(MessageIMSARegisterIndications),
				},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceIMSA: 7}}
			updates, err := tt.watch(ctx, client)
			if err != nil {
				cancel()
				t.Fatalf("watch() error = %v", err)
			}
			transport.emit(tt.message, tt.indication)
			select {
			case status := <-updates:
				if !tt.matches(status) {
					t.Fatalf("status = %+v, want matching update", status)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for IMSA indication")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestClientIMSAStatus(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req Request) {
					if req.Service != ServiceControl || req.MessageID != MessageAllocateClientID {
						t.Fatalf("allocate request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceIMSA)})
				},
				resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceIMSA), 5})),
			},
			{
				check: func(req Request) {
					if req.Service != ServiceIMSA || req.ClientID != 5 || req.MessageID != MessageIMSAGetRegistrationStatus {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X, want IMSA registration", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: successResponse(MessageIMSAGetRegistrationStatus, tlv.Uint(imsaTLVRegStatus, uint32(IMSRegistrationStatusRegistered))),
			},
			{
				check: func(req Request) {
					if req.Service != ServiceIMSA || req.ClientID != 5 || req.MessageID != MessageIMSAGetServiceStatus {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X, want IMSA service", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: successResponse(MessageIMSAGetServiceStatus,
					tlv.Uint(imsaTLVVoIPService, uint32(IMSServiceStatusFullService)),
					tlv.Uint(imsaTLVVoIPRAT, uint32(IMSServiceRATWWAN))),
			},
			{
				check: func(req Request) {
					if req.Service != ServiceControl || req.MessageID != MessageReleaseClientID {
						t.Fatalf("release request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceIMSA), 5})
				},
				resp: successResponse(MessageReleaseClientID),
			},
		},
	}
	reader := &Client{
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
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := transport.callCount(); got != 4 {
		t.Fatalf("Do() calls = %d, want 4", got)
	}
}
