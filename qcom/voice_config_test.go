package qcom

import (
	"context"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestVoiceSetConfig(t *testing.T) {
	autoAnswer := true
	tty := VoiceTTYHCO
	domain := VoiceDomainPSPreferred
	uiTTY := VoiceTTYVCO
	tests := []struct {
		name    string
		update  VoiceConfigUpdate
		resp    Response
		wantErr string
	}{
		{
			name: "all common fields",
			update: VoiceConfigUpdate{
				AutoAnswer:  &autoAnswer,
				TTYMode:     &tty,
				VoiceDomain: &domain,
				UITTYMode:   &uiTTY,
			},
			resp: successResponse(MessageVoiceSetConfig,
				tlv.Bytes(0x10, []byte{0}),
				tlv.Bytes(0x13, []byte{0}),
				tlv.Bytes(0x15, []byte{0}),
				tlv.Bytes(0x16, []byte{0}),
			),
		},
		{
			name:    "field outcome error",
			update:  VoiceConfigUpdate{TTYMode: &tty},
			resp:    successResponse(MessageVoiceSetConfig, tlv.Bytes(0x13, []byte{1})),
			wantErr: "config TLV 0x13",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceVoice || req.MessageID != MessageVoiceSetConfig {
						t.Fatalf("request service/message = 0x%02X/0x%04X, want Voice/SetConfig", req.Service, req.MessageID)
					}
					if tt.update.AutoAnswer != nil {
						assertTLV(t, req.TLVs, 0x10, []byte{1})
					}
					if tt.update.TTYMode != nil {
						assertTLV(t, req.TLVs, 0x13, []byte{byte(*tt.update.TTYMode)})
					}
					if tt.update.VoiceDomain != nil {
						assertTLV(t, req.TLVs, 0x15, []byte{byte(*tt.update.VoiceDomain)})
					}
					if tt.update.UITTYMode != nil {
						assertTLV(t, req.TLVs, 0x16, []byte{byte(*tt.update.UITTYMode)})
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			err := client.VoiceSetConfig(context.Background(), tt.update)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("VoiceSetConfig() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("VoiceSetConfig() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestVoiceConfig(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		want    VoiceConfig
		wantErr bool
	}{
		{
			name: "all returned fields",
			resp: successResponse(MessageVoiceGetConfig,
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x13, []byte{byte(VoiceTTYHCO)}),
				tlv.Bytes(0x16, []byte{byte(VoicePrivacyEnhanced)}),
				tlv.Bytes(0x17, []byte{byte(VoiceDomainPSPreferred)}),
				tlv.Bytes(0x18, []byte{byte(VoiceTTYVCO)}),
			),
			want: VoiceConfig{
				AutoAnswer:       true,
				AutoAnswerKnown:  true,
				TTYMode:          VoiceTTYHCO,
				TTYModeKnown:     true,
				Privacy:          VoicePrivacyEnhanced,
				PrivacyKnown:     true,
				VoiceDomain:      VoiceDomainPSPreferred,
				VoiceDomainKnown: true,
				UITTYMode:        VoiceTTYVCO,
				UITTYModeKnown:   true,
			},
		},
		{
			name:    "malformed TTY mode",
			resp:    successResponse(MessageVoiceGetConfig, tlv.Bytes(0x13, nil)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceVoice || req.MessageID != MessageVoiceGetConfig {
						t.Fatalf("request service/message = 0x%02X/0x%04X, want Voice/GetConfig", req.Service, req.MessageID)
					}
					for _, kind := range []byte{0x10, 0x13, 0x16, 0x18, 0x19} {
						assertTLV(t, req.TLVs, kind, []byte{1})
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			got, err := client.VoiceConfig(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("VoiceConfig() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("VoiceConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("VoiceConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestVoiceSetPreferredPrivacy(t *testing.T) {
	tests := []struct {
		name    string
		privacy VoicePrivacy
		wantErr bool
	}{
		{name: "enhanced", privacy: VoicePrivacyEnhanced},
		{name: "invalid", privacy: VoicePrivacy(2), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []transportCall(nil)
			if !tt.wantErr {
				calls = []transportCall{{
					check: func(req Request) {
						if req.MessageID != MessageVoiceSetPreferredPrivacy {
							t.Fatalf("message = 0x%04X, want SetPreferredPrivacy", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.privacy)})
					},
					resp: successResponse(MessageVoiceSetPreferredPrivacy),
				}}
			}
			client := &Client{transport: &fakeTransport{t: t, calls: calls}, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			err := client.VoiceSetPreferredPrivacy(context.Background(), tt.privacy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VoiceSetPreferredPrivacy() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestVoiceALS(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		check   func(*testing.T, Request)
		resp    Response
		call    func(*Client) (uint8, error)
		want    uint8
	}{
		{
			name:    "allow line switching",
			message: MessageVoiceSetALSLineSwitching,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1})
			},
			resp: successResponse(MessageVoiceSetALSLineSwitching),
			call: func(c *Client) (uint8, error) {
				return 0, c.VoiceSetALSLineSwitching(context.Background(), true)
			},
		},
		{
			name:    "select line two",
			message: MessageVoiceSelectALSLine,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceALSLine2)})
			},
			resp: successResponse(MessageVoiceSelectALSLine),
			call: func(c *Client) (uint8, error) {
				return 0, c.VoiceSelectALSLine(context.Background(), VoiceALSLine2)
			},
		},
		{
			name:    "read line switching",
			message: MessageVoiceALSLineSwitching,
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs = %+v, want empty", req.TLVs)
				}
			},
			resp: successResponse(MessageVoiceALSLineSwitching, tlv.Bytes(0x10, []byte{1})),
			call: func(c *Client) (uint8, error) {
				allowed, err := c.VoiceALSLineSwitching(context.Background())
				return boolByte(allowed), err
			},
			want: 1,
		},
		{
			name:    "read selected line",
			message: MessageVoiceALSSelectedLine,
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs = %+v, want empty", req.TLVs)
				}
			},
			resp: successResponse(MessageVoiceALSSelectedLine, tlv.Bytes(0x10, []byte{byte(VoiceALSLine2)})),
			call: func(c *Client) (uint8, error) {
				line, err := c.VoiceALSSelectedLine(context.Background())
				return uint8(line), err
			},
			want: uint8(VoiceALSLine2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != tt.message {
						t.Fatalf("message = 0x%04X, want 0x%04X", req.MessageID, tt.message)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			got, err := tt.call(client)
			if err != nil {
				t.Fatalf("call() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("call() = %d, want %d", got, tt.want)
			}
		})
	}
}
