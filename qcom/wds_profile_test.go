package qcom

import (
	"context"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSProfileRequests(t *testing.T) {
	tests := []struct {
		name  string
		build func() (Request, error)
		want  MessageID
	}{
		{name: "list", build: func() (Request, error) {
			return WDSGetProfileListRequest{ClientID: 3, ProfileType: WDSProfileType3GPP}.Request(), nil
		}, want: MessageWDSGetProfileList},
		{name: "settings", build: func() (Request, error) {
			return WDSGetProfileSettingsRequest{ClientID: 4, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 2}}.Request(), nil
		}, want: MessageWDSGetProfileSettings},
		{name: "modify", build: func() (Request, error) {
			return WDSModifyProfileRequest{ClientID: 5, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 2}, PCSCFUsingPCO: true}.Request(), nil
		}, want: MessageWDSModifyProfile},
		{name: "update", build: func() (Request, error) {
			return (WDSUpdateProfileRequest{ClientID: 5, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 2}, Update: WDSProfileUpdate{APN: ptr("internet")}}).Request()
		}, want: MessageWDSModifyProfile},
		{name: "set default", build: func() (Request, error) {
			return WDSSetDefaultProfileRequest{ClientID: 6, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 2}, Family: WDSProfileFamilyTethered}.Request(), nil
		}, want: MessageWDSSetDefaultProfile},
		{name: "create", build: func() (Request, error) {
			return (WDSCreateProfileRequest{ClientID: 5, Config: WDSProfileConfig{APN: "ims", PDPType: WDSPDPTypeIPv4v6}}).Request()
		}, want: MessageWDSCreateProfile},
		{name: "delete", build: func() (Request, error) {
			return WDSDeleteProfileRequest{ClientID: 6, Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 3}}.Request(), nil
		}, want: MessageWDSDeleteProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.build()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if req.Service != ServiceWDS || req.MessageID != tt.want {
				t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
			}
		})
	}
}

func TestWDSProfileMutationRequests(t *testing.T) {
	tests := []struct {
		name  string
		build func() (Request, error)
		want  map[byte][]byte
	}{
		{
			name: "enable P-CSCF via PCO",
			build: func() (Request, error) {
				return WDSModifyProfileRequest{
					Profile:       WDSProfileID{Type: WDSProfileType3GPP, Index: 7},
					PCSCFUsingPCO: true,
				}.Request(), nil
			},
			want: map[byte][]byte{
				wdsTLVProfileID:     {byte(WDSProfileType3GPP), 7},
				wdsTLVPCSCFUsingPCO: {1},
			},
		},
		{
			name: "set tethered default profile",
			build: func() (Request, error) {
				return WDSSetDefaultProfileRequest{
					Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 7},
					Family:  WDSProfileFamilyTethered,
				}.Request(), nil
			},
			want: map[byte][]byte{
				wdsTLVProfileID: {byte(WDSProfileType3GPP), byte(WDSProfileFamilyTethered), 7},
			},
		},
		{
			name: "update authentication and IMS fields",
			build: func() (Request, error) {
				return (WDSUpdateProfileRequest{
					Profile: WDSProfileID{Type: WDSProfileType3GPP, Index: 7},
					Update: WDSProfileUpdate{
						APN:            ptr("ims"),
						PDPType:        ptr(WDSPDPTypeIPv4v6),
						Username:       ptr("subscriber"),
						Password:       ptr("secret"),
						Authentication: ptr(WDSAuthenticationPAP | WDSAuthenticationCHAP),
						PCSCFUsingPCO:  ptr(true),
						PCSCFUsingDHCP: ptr(false),
						IMCN:           ptr(true),
					},
				}).Request()
			},
			want: map[byte][]byte{
				wdsTLVProfileID:       {byte(WDSProfileType3GPP), 7},
				wdsTLVProfilePDPType:  {byte(WDSPDPTypeIPv4v6)},
				wdsTLVProfileAPN:      []byte("ims"),
				wdsTLVProfileUsername: []byte("subscriber"),
				wdsTLVProfilePassword: []byte("secret"),
				wdsTLVProfileAuth:     {byte(WDSAuthenticationPAP | WDSAuthenticationCHAP)},
				wdsTLVPCSCFUsingPCO:   {1},
				wdsTLVPCSCFUsingDHCP:  {0},
				wdsTLVIMCNFlag:        {1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.build()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			for kind, want := range tt.want {
				assertTLV(t, req.TLVs, kind, want)
			}
		})
	}
}

func TestClientWDSProfileMutations(t *testing.T) {
	profile := WDSProfileID{Type: WDSProfileType3GPP, Index: 7}
	tests := []struct {
		name    string
		message MessageID
		apply   func(context.Context, *Client) error
		resp    Response
		wantErr bool
	}{
		{
			name:    "modify profile",
			message: MessageWDSModifyProfile,
			apply:   func(ctx context.Context, client *Client) error { return client.WDSModifyProfile(ctx, profile, true) },
			resp:    successResponse(MessageWDSModifyProfile),
		},
		{
			name:    "update profile",
			message: MessageWDSModifyProfile,
			apply: func(ctx context.Context, client *Client) error {
				return client.WDSUpdateProfile(ctx, profile, WDSProfileUpdate{
					Authentication: ptr(WDSAuthenticationCHAP),
					Username:       ptr("subscriber"),
				})
			},
			resp: successResponse(MessageWDSModifyProfile),
		},
		{
			name:    "default profile error",
			message: MessageWDSSetDefaultProfile,
			apply: func(ctx context.Context, client *Client) error {
				return client.WDSSetDefaultProfile(ctx, profile, WDSProfileFamilyTethered)
			},
			resp:    errorResponse(MessageWDSSetDefaultProfile, QMIErrorNotSupported),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 5)},
				{
					check: func(req Request) {
						if req.ClientID != 5 || req.MessageID != tt.message {
							t.Fatalf("mutation request = client %d message 0x%04X", req.ClientID, req.MessageID)
						}
					},
					resp: tt.resp,
				},
				{resp: successResponse(MessageReleaseClientID)},
			}}
			client := &Client{transport: transport, slot: 1}

			err := tt.apply(context.Background(), client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("apply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestClientWDSProfileLifecycleReusesClient(t *testing.T) {
	tests := []struct {
		name    string
		apn     string
		pdpType WDSPDPType
	}{
		{name: "IPv4v6 profile", apn: " ims ", pdpType: WDSPDPTypeIPv4v6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 5)},
				{
					check: func(req Request) {
						if req.ClientID != 5 || req.MessageID != MessageWDSCreateProfile {
							t.Fatalf("create request = client %d message 0x%04X", req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, wdsTLVProfileID, []byte{byte(WDSProfileType3GPP)})
						assertTLV(t, req.TLVs, wdsTLVProfilePDPType, []byte{byte(tt.pdpType)})
						assertTLV(t, req.TLVs, wdsTLVProfileAPN, []byte("ims"))
					},
					resp: successResponse(MessageWDSCreateProfile, tlv.Bytes(wdsTLVProfileID, []byte{byte(WDSProfileType3GPP), 7})),
				},
				{
					check: func(req Request) {
						if req.ClientID != 5 || req.MessageID != MessageWDSDeleteProfile {
							t.Fatalf("delete request = client %d message 0x%04X", req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, wdsTLVProfileID, []byte{byte(WDSProfileType3GPP), 7})
					},
					resp: successResponse(MessageWDSDeleteProfile),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDS), 5})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}
			reader := &Client{transport: transport, slot: 1}

			index, err := reader.WDSCreateProfile(context.Background(), tt.apn, tt.pdpType)
			if err != nil {
				t.Fatalf("WDSCreateProfile() error = %v", err)
			}
			if index != 7 {
				t.Fatalf("WDSCreateProfile() = %d, want 7", index)
			}
			if err := reader.WDSDeleteProfile(context.Background(), index); err != nil {
				t.Fatalf("WDSDeleteProfile() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestClientWDSCreateProfileWithConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  WDSProfileConfig
	}{
		{
			name: "authenticated IMS profile",
			cfg: WDSProfileConfig{
				Name:           "carrier-ims",
				APN:            " ims ",
				PDPType:        WDSPDPTypeIPv4v6,
				Username:       "subscriber",
				Password:       "secret",
				Authentication: WDSAuthenticationPAP | WDSAuthenticationCHAP,
				PCSCFUsingPCO:  ptr(true),
				PCSCFUsingDHCP: ptr(false),
				IMCN:           ptr(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 5)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, wdsTLVProfileName, []byte("carrier-ims"))
						assertTLV(t, req.TLVs, wdsTLVProfileAPN, []byte("ims"))
						assertTLV(t, req.TLVs, wdsTLVProfileUsername, []byte("subscriber"))
						assertTLV(t, req.TLVs, wdsTLVProfilePassword, []byte("secret"))
						assertTLV(t, req.TLVs, wdsTLVProfileAuth, []byte{byte(WDSAuthenticationPAP | WDSAuthenticationCHAP)})
						assertTLV(t, req.TLVs, wdsTLVPCSCFUsingPCO, []byte{1})
						assertTLV(t, req.TLVs, wdsTLVPCSCFUsingDHCP, []byte{0})
						assertTLV(t, req.TLVs, wdsTLVIMCNFlag, []byte{1})
					},
					resp: successResponse(MessageWDSCreateProfile, tlv.Bytes(wdsTLVProfileID, []byte{byte(WDSProfileType3GPP), 7})),
				},
				{resp: successResponse(MessageReleaseClientID)},
			}}
			client := &Client{transport: transport, slot: 1}

			id, err := client.WDSCreateProfileWithConfig(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("WDSCreateProfileWithConfig() error = %v", err)
			}
			if id != (WDSProfileID{Type: WDSProfileType3GPP, Index: 7}) {
				t.Fatalf("profile ID = %+v", id)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestClientWDSProfileConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  WDSProfileConfig
		want string
	}{
		{name: "missing APN", cfg: WDSProfileConfig{}, want: "APN is required"},
		{name: "profile name too long", cfg: WDSProfileConfig{APN: "internet", Name: strings.Repeat("n", wdsProfileNameMaxLength+1)}, want: "validating WDS profile name: value length"},
		{name: "APN too long", cfg: WDSProfileConfig{APN: strings.Repeat("a", wdsAPNMaxLength+1)}, want: "validating WDS APN: value length"},
		{name: "username too long", cfg: WDSProfileConfig{APN: "internet", Username: strings.Repeat("u", wdsUsernameMaxLength+1)}, want: "validating WDS username: value length"},
		{name: "password too long", cfg: WDSProfileConfig{APN: "internet", Password: strings.Repeat("p", wdsPasswordMaxLength+1)}, want: "validating WDS password: value length"},
		{name: "unsupported PDP type", cfg: WDSProfileConfig{APN: "internet", PDPType: 9}, want: "unsupported WDS PDP type"},
		{name: "unsupported authentication", cfg: WDSProfileConfig{APN: "internet", Authentication: 0x80}, want: "unsupported WDS authentication mask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Client{}).WDSCreateProfileWithConfig(context.Background(), tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WDSCreateProfileWithConfig() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWDSProfileValidationAcceptsNonIP(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
	}{
		{
			name: "create",
			validate: func() error {
				return validateWDSProfileConfig(WDSProfileConfig{APN: "nonip", PDPType: WDSPDPTypeNonIP})
			},
		},
		{
			name: "update",
			validate: func() error {
				return validateWDSProfileUpdate(WDSProfileUpdate{PDPType: ptr(WDSPDPTypeNonIP)})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.validate(); err != nil {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestClientWDSProfileUpdateValidation(t *testing.T) {
	tests := []struct {
		name    string
		profile WDSProfileID
		update  WDSProfileUpdate
		want    string
	}{
		{name: "no changes", profile: WDSProfileID{Type: WDSProfileType3GPP}, want: "no fields selected"},
		{name: "unsupported profile type", profile: WDSProfileID{Type: 3}, update: WDSProfileUpdate{APN: ptr("internet")}, want: "profile type"},
		{name: "APN too long", profile: WDSProfileID{Type: WDSProfileType3GPP}, update: WDSProfileUpdate{APN: ptr(strings.Repeat("a", wdsAPNMaxLength+1))}, want: "validating WDS APN: value length"},
		{name: "unsupported authentication", profile: WDSProfileID{Type: WDSProfileType3GPP}, update: WDSProfileUpdate{Authentication: ptr(WDSAuthenticationMask(0x80))}, want: "unsupported WDS authentication mask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Client{}).WDSUpdateProfile(context.Background(), tt.profile, tt.update)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WDSUpdateProfile() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestClientWDSProfileIndex(t *testing.T) {
	tests := []struct {
		name string
		apn  string
		want uint8
	}{
		{name: "case-insensitive APN", apn: " IMS ", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileList := []byte{2, 0, 1, 3, 'n', 'e', 't', 0, 2, 3, 'i', 'm', 's'}
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 5)},
				{resp: successResponse(MessageWDSGetProfileList, tlv.Bytes(wdsTLVProfileList, profileList))},
				{resp: successResponse(MessageWDSGetProfileSettings, tlv.Bytes(wdsTLVProfileAPN, []byte("internet")))},
				{resp: successResponse(MessageWDSGetProfileSettings, tlv.Bytes(wdsTLVProfileAPN, []byte("ims")))},
				{resp: successResponse(MessageReleaseClientID)},
			}}
			reader := &Client{transport: transport, slot: 1}

			got, err := reader.WDSProfileIndex(context.Background(), tt.apn)
			if err != nil {
				t.Fatalf("WDSProfileIndex() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("WDSProfileIndex() = %d, want %d", got, tt.want)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestWDSGetProfileListResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileList, []byte{1, 0})}, wantErr: true},
		{name: "trailing data", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileList, []byte{0, 1})}, wantErr: true},
		{name: "profiles", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileList, []byte{2, 0, 1, 3, 'n', 'e', 't', 0, 2, 3, 'i', 'm', 's'})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetProfileListResponse
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
			if len(got.Profiles) != 2 || got.Profiles[1].ID.Index != 2 || got.Profiles[1].Name != "ims" {
				t.Fatalf("Profiles = %+v", got.Profiles)
			}
		})
	}
}

func TestWDSGetProfileSettingsResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{name: "empty"},
		{name: "IMS", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileAPN, []byte("ims")), tlv.Uint(wdsTLVProfilePDPType, uint8(WDSPDPTypeIPv6)), tlv.Bytes(wdsTLVProfileUsername, []byte("subscriber")), tlv.Bytes(wdsTLVProfilePassword, []byte("secret")), tlv.Uint(wdsTLVProfileAuth, uint8(WDSAuthenticationCHAP)), tlv.Bytes(wdsTLVPCSCFUsingPCO, []byte{1}), tlv.Bytes(wdsTLVIMCNFlag, []byte{1})}},
		{name: "truncated PDP type", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfilePDPType, nil)}, wantErr: true},
		{name: "PDP type trailing data", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfilePDPType, make([]byte, 2))}, wantErr: true},
		{name: "truncated authentication", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileAuth, nil)}, wantErr: true},
		{name: "authentication trailing data", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVProfileAuth, make([]byte, 2))}, wantErr: true},
		{name: "truncated bool", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVPCSCFUsingPCO, nil)}, wantErr: true},
		{name: "bool trailing data", tlvs: tlv.TLVs{tlv.Bytes(wdsTLVPCSCFUsingPCO, make([]byte, 2))}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WDSGetProfileSettingsResponse{Settings: WDSProfileSettings{ID: WDSProfileID{Index: 2}}}
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
			if tt.name == "IMS" && (!got.Settings.APNKnown || got.Settings.APN != "ims" ||
				!got.Settings.PDPKnown || got.Settings.PDPType != WDSPDPTypeIPv6 ||
				!got.Settings.UsernameKnown || got.Settings.Username != "subscriber" ||
				!got.Settings.PasswordKnown || got.Settings.Password != "secret" ||
				!got.Settings.AuthenticationKnown || got.Settings.Authentication != WDSAuthenticationCHAP ||
				!got.Settings.PCSCFUsingPCO || !got.Settings.IMCN) {
				t.Fatalf("Settings = %+v", got.Settings)
			}
		})
	}
}
