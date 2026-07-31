package qcom

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSAutoconnectRequests(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (Request, error)
		wantID  MessageID
		wantTLV map[byte][]byte
		wantErr string
	}{
		{
			name: "get",
			build: func() (Request, error) {
				return (WDSGetAutoconnectSettingsRequest{
					ClientID: 7, TransactionID: 9, Timeout: 2 * time.Second,
				}).Request(), nil
			},
			wantID: MessageWDSGetAutoconnectSettings,
		},
		{
			name: "set status only",
			build: func() (Request, error) {
				return (WDSSetAutoconnectSettingsRequest{
					ClientID: 7, TransactionID: 9, Timeout: 2 * time.Second,
					Settings: WDSAutoconnectSettings{Status: WDSAutoconnectPaused},
				}).Request()
			},
			wantID:  MessageWDSSetAutoconnectSettings,
			wantTLV: map[byte][]byte{0x01: {byte(WDSAutoconnectPaused)}},
		},
		{
			name: "set roaming",
			build: func() (Request, error) {
				return (WDSSetAutoconnectSettingsRequest{
					Settings: WDSAutoconnectSettings{
						Status: WDSAutoconnectEnabled, Roaming: WDSAutoconnectHomeOnly, RoamingKnown: true,
					},
				}).Request()
			},
			wantID:  MessageWDSSetAutoconnectSettings,
			wantTLV: map[byte][]byte{0x01: {1}, 0x10: {1}},
		},
		{
			name: "invalid status",
			build: func() (Request, error) {
				return (WDSSetAutoconnectSettingsRequest{
					Settings: WDSAutoconnectSettings{Status: WDSAutoconnectSetting(3)},
				}).Request()
			},
			wantErr: "status",
		},
		{
			name: "invalid roaming",
			build: func() (Request, error) {
				return (WDSSetAutoconnectSettingsRequest{
					Settings: WDSAutoconnectSettings{
						Status: WDSAutoconnectEnabled, Roaming: WDSAutoconnectRoaming(2), RoamingKnown: true,
					},
				}).Request()
			},
			wantErr: "roaming",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceWDS || got.MessageID != tt.wantID {
				t.Fatalf("Request() = %+v", got)
			}
			if tt.name == "get" && (got.ClientID != 7 || got.TransactionID != 9 || got.Timeout != 2*time.Second) {
				t.Fatalf("Request() metadata = %+v", got)
			}
			if len(got.TLVs) != len(tt.wantTLV) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLV))
			}
			for typ, want := range tt.wantTLV {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestWDSGetAutoconnectSettingsResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSAutoconnectSettings
		wantErr bool
	}{
		{
			name: "status and roaming",
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(WDSAutoconnectEnabled)), tlv.Uint(0x10, uint8(WDSAutoconnectHomeOnly))},
			want: WDSAutoconnectSettings{Status: WDSAutoconnectEnabled, Roaming: WDSAutoconnectHomeOnly, RoamingKnown: true},
		},
		{
			name: "status only",
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(WDSAutoconnectPaused))},
			want: WDSAutoconnectSettings{Status: WDSAutoconnectPaused},
		},
		{name: "status missing", wantErr: true},
		{name: "status length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: true},
		{
			name: "roaming length",
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(1)), tlv.Bytes(0x10, nil)}, wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetAutoconnectSettingsResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
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

func TestClientWDSAutoconnectSettings(t *testing.T) {
	tests := []struct {
		name    string
		calls   []transportCall
		run     func(context.Context, *Client) error
		wantErr string
	}{
		{
			name: "get",
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSGetAutoconnectSettings || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(MessageWDSGetAutoconnectSettings, tlv.Uint(0x01, uint8(WDSAutoconnectEnabled))),
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.WDSAutoconnectSettings(ctx)
				if err == nil && got.Status != WDSAutoconnectEnabled {
					t.Fatalf("settings = %+v", got)
				}
				return err
			},
		},
		{
			name: "set",
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSSetAutoconnectSettings {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{1})
					assertTLV(t, req.TLVs, 0x10, []byte{1})
				},
				resp: successResponse(MessageWDSSetAutoconnectSettings),
			}},
			run: func(ctx context.Context, client *Client) error {
				return client.WDSSetAutoconnectSettings(ctx, WDSAutoconnectSettings{
					Status: WDSAutoconnectEnabled, Roaming: WDSAutoconnectHomeOnly, RoamingKnown: true,
				})
			},
		},
		{
			name:  "modem error",
			calls: []transportCall{{resp: errorResponse(MessageWDSSetAutoconnectSettings, QMIErrorNotSupported)}},
			run: func(ctx context.Context, client *Client) error {
				return client.WDSSetAutoconnectSettings(ctx, WDSAutoconnectSettings{Status: WDSAutoconnectDisabled})
			},
			wantErr: "not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			err := tt.run(context.Background(), client)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
					t.Fatalf("operation error = %v, want text %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}
