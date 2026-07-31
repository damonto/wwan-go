package qcom

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSDefaultRequests(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "settings",
			request: (WDSGetDefaultSettingsRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second,
				ProfileType: WDSProfileTypeEPC,
			}).Request(),
			message: MessageWDSGetDefaultSettings,
			wantTLV: map[byte][]byte{0x01: {byte(WDSProfileTypeEPC)}},
		},
		{
			name: "profile number",
			request: (WDSGetDefaultProfileRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second,
				ProfileType: WDSProfileType3GPP, Family: WDSProfileFamilyTethered,
			}).Request(),
			message: MessageWDSGetDefaultProfile,
			wantTLV: map[byte][]byte{
				0x01: {byte(WDSProfileType3GPP), byte(WDSProfileFamilyTethered)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Service != ServiceWDS || tt.request.ClientID != 7 || tt.request.TransactionID != 9 || tt.request.MessageID != tt.message {
				t.Fatalf("Request() = %+v", tt.request)
			}
			if tt.request.Timeout != time.Second || len(tt.request.TLVs) != len(tt.wantTLV) {
				t.Fatalf("Request() timeout = %v, TLVs = %v", tt.request.Timeout, tt.request.TLVs)
			}
			for kind, want := range tt.wantTLV {
				assertTLV(t, tt.request.TLVs, kind, want)
			}
		})
	}
}

func TestWDSGetDefaultSettingsResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSProfileSettings
		wantErr string
	}{
		{
			name: "practical fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(wdsTLVProfileName, []byte("carrier")),
				tlv.Uint(wdsTLVProfilePDPType, uint8(WDSPDPTypeIPv4v6)),
				tlv.Bytes(wdsTLVProfileAPN, []byte("internet")),
				tlv.Bytes(wdsTLVProfileUsername, []byte("subscriber")),
				tlv.Bytes(wdsTLVProfilePassword, []byte("secret")),
				tlv.Uint(wdsTLVProfileAuth, uint8(WDSAuthenticationCHAP)),
				tlv.Uint(wdsTLVPCSCFUsingPCO, uint8(1)),
				tlv.Uint(wdsTLVPCSCFUsingDHCP, uint8(0)),
				tlv.Uint(wdsTLVIMCNFlag, uint8(1)),
			},
			want: WDSProfileSettings{
				ID:   WDSProfileID{Type: WDSProfileTypeEPC},
				Name: "carrier", NameKnown: true,
				PDPType: WDSPDPTypeIPv4v6, PDPKnown: true,
				APN: "internet", APNKnown: true,
				Username: "subscriber", UsernameKnown: true,
				Password: "secret", PasswordKnown: true,
				Authentication: WDSAuthenticationCHAP, AuthenticationKnown: true,
				PCSCFUsingPCO: true, PCSCFUsingPCOKnown: true,
				PCSCFUsingDHCPKnown: true,
				IMCN:                true, IMCNKnown: true,
			},
		},
		{
			name: "empty response",
			want: WDSProfileSettings{ID: WDSProfileID{Type: WDSProfileTypeEPC}},
		},
		{
			name:    "truncated PDP type",
			tlvs:    tlv.TLVs{tlv.Bytes(wdsTLVProfilePDPType, nil)},
			wantErr: "PDP type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WDSGetDefaultSettingsResponse{
				Settings: WDSProfileSettings{ID: WDSProfileID{Type: WDSProfileTypeEPC}},
			}
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got.Settings, tt.want) {
				t.Fatalf("settings = %+v, want %+v", got.Settings, tt.want)
			}
		})
	}
}

func TestWDSGetDefaultProfileResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    uint8
		wantErr string
	}{
		{name: "profile seven", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(7))}, want: 7},
		{name: "missing", wantErr: "missing"},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetDefaultProfileResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Index != tt.want {
				t.Fatalf("index = %d, want %d", got.Index, tt.want)
			}
		})
	}
}

func TestClientWDSDefaults(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		check   func(*testing.T, Request)
		resp    Response
		run     func(context.Context, *Client) error
	}{
		{
			name:    "settings",
			message: MessageWDSGetDefaultSettings,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(WDSProfileTypeEPC)})
			},
			resp: successResponse(
				MessageWDSGetDefaultSettings,
				tlv.Bytes(wdsTLVProfileAPN, []byte("internet")),
			),
			run: func(ctx context.Context, client *Client) error {
				settings, err := client.WDSDefaultSettings(ctx, WDSProfileTypeEPC)
				if err == nil && (settings.ID.Type != WDSProfileTypeEPC || !settings.APNKnown || settings.APN != "internet") {
					t.Fatalf("WDSDefaultSettings() = %+v", settings)
				}
				return err
			},
		},
		{
			name:    "profile",
			message: MessageWDSGetDefaultProfile,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(WDSProfileType3GPP), byte(WDSProfileFamilyEmbedded)})
			},
			resp: successResponse(MessageWDSGetDefaultProfile, tlv.Uint(0x01, uint8(7))),
			run: func(ctx context.Context, client *Client) error {
				profile, err := client.WDSDefaultProfile(ctx, WDSProfileType3GPP, WDSProfileFamilyEmbedded)
				if err == nil && profile != (WDSProfileID{Type: WDSProfileType3GPP, Index: 7}) {
					t.Fatalf("WDSDefaultProfile() = %+v", profile)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWDS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = %+v", req)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("client call error = %v", err)
			}
		})
	}
}

func TestClientWDSDefaultsRejectInvalidSelectors(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{
			name: "settings profile type",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.WDSDefaultSettings(ctx, WDSProfileType(3))
				return err
			},
		},
		{
			name: "default profile type",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.WDSDefaultProfile(ctx, WDSProfileType(3), WDSProfileFamilyEmbedded)
				return err
			},
		},
		{
			name: "default profile family",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.WDSDefaultProfile(ctx, WDSProfileType3GPP, WDSProfileFamily(2))
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if err := tt.run(context.Background(), client); err == nil {
				t.Fatal("client call error = nil")
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
		})
	}
}
