package qcom

import (
	"context"
	"encoding"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

var (
	_ encoding.BinaryMarshaler   = IMSPolicyManagerAPNs{}
	_ encoding.BinaryUnmarshaler = (*IMSPolicyManagerAPNs)(nil)
)

func TestIMSRegisterIndicationsRequest(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name      string
		config    IMSIndicationConfig
		wantTLVs  int
		checkTLVs func(*testing.T, tlv.TLVs)
	}{
		{
			name:     "all supported indications",
			config:   IMSIndicationConfig{PolicyManager: &enabled, ServicesEnabled: &disabled},
			wantTLVs: 2,
			checkTLVs: func(t *testing.T, tlvs tlv.TLVs) {
				assertTLV(t, tlvs, 0x1B, []byte{1})
				assertTLV(t, tlvs, 0x2D, []byte{0})
			},
		},
		{name: "no changes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (IMSRegisterIndicationsRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
				Config:        tt.config,
			}).Request()
			if got.Service != ServiceIMS || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != MessageIMSRegisterIndications {
				t.Fatalf("Request() = %+v, want IMS Settings indication registration", got)
			}
			if got.Timeout != 3*time.Second {
				t.Fatalf("Timeout = %v, want 3s", got.Timeout)
			}
			if len(got.TLVs) != tt.wantTLVs {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), tt.wantTLVs)
			}
			if tt.checkTLVs != nil {
				tt.checkTLVs(t, got.TLVs)
			}
		})
	}
}

func TestIMSSetPolicyManagerSettingsRequest(t *testing.T) {
	acs := uint8(1)
	isim := uint8(2)
	nv := uint8(3)
	pco := uint8(4)
	mask := IMSServiceVoLTE | IMSServiceSMS | IMSServicePresence
	apns := IMSPolicyManagerAPNs{"ims", "sos", "rcs", "xcap", "", "internet"}
	tests := []struct {
		name      string
		config    IMSPolicyManagerConfig
		wantTLVs  int
		wantError bool
	}{
		{
			name: "all fields",
			config: IMSPolicyManagerConfig{
				ACSPriority: &acs, ISIMPriority: &isim, NVPriority: &nv, PCOPriority: &pco,
				ServiceMask: &mask, APNs: &apns,
			},
			wantTLVs: 6,
		},
		{name: "all fields omitted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (IMSSetPolicyManagerSettingsRequest{ClientID: 7, TransactionID: 9, Config: tt.config}).Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				return
			}
			if got.Service != ServiceIMS || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != MessageIMSSetPolicyManagerSettings {
				t.Fatalf("Request() = %+v, want IMS Set Policy Manager Settings", got)
			}
			if len(got.TLVs) != tt.wantTLVs {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), tt.wantTLVs)
			}
			if tt.name != "all fields" {
				return
			}
			assertTLV(t, got.TLVs, 0x14, []byte{1})
			assertTLV(t, got.TLVs, 0x15, []byte{2})
			assertTLV(t, got.TLVs, 0x16, []byte{3})
			assertTLV(t, got.TLVs, 0x17, []byte{4})
			assertTLV(t, got.TLVs, 0x18, binary.LittleEndian.AppendUint64(nil, uint64(mask)))
			assertTLV(t, got.TLVs, 0x19, []byte{
				3, 'i', 'm', 's',
				3, 's', 'o', 's',
				3, 'r', 'c', 's',
				4, 'x', 'c', 'a', 'p',
				0,
				8, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't',
			})
		})
	}
}

func TestIMSPolicyManagerAPNsMarshalBinary(t *testing.T) {
	tests := []struct {
		name      string
		apns      IMSPolicyManagerAPNs
		wantError bool
	}{
		{name: "empty entries"},
		{name: "maximum length", apns: IMSPolicyManagerAPNs{strings.Repeat("a", imsPolicyManagerAPNMaxLength)}},
		{name: "too long", apns: IMSPolicyManagerAPNs{strings.Repeat("a", imsPolicyManagerAPNMaxLength+1)}, wantError: true},
		{name: "NUL byte", apns: IMSPolicyManagerAPNs{"im\x00s"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.apns.MarshalBinary()
			if (err != nil) != tt.wantError {
				t.Fatalf("MarshalBinary() error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && len(got) < imsPolicyManagerAPNCount {
				t.Fatalf("MarshalBinary() length = %d, want at least %d", len(got), imsPolicyManagerAPNCount)
			}
		})
	}
}

func TestIMSPolicyManagerAPNsUnmarshalBinary(t *testing.T) {
	want := IMSPolicyManagerAPNs{"ims", "sos", "rcs", "xcap", "", "internet"}
	encoded, err := want.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	tests := []struct {
		name      string
		value     []byte
		want      IMSPolicyManagerAPNs
		wantError bool
	}{
		{name: "round trip", value: encoded, want: want},
		{name: "missing entry", value: []byte{0, 0, 0, 0, 0}, wantError: true},
		{name: "entry too long", value: []byte{imsPolicyManagerAPNMaxLength + 1}, wantError: true},
		{name: "truncated entry", value: []byte{3, 'i'}, wantError: true},
		{name: "trailing data", value: append(append([]byte(nil), encoded...), 0), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IMSPolicyManagerAPNs{"stale"}
			err := got.UnmarshalBinary(tt.value)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalBinary() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				if got != (IMSPolicyManagerAPNs{}) {
					t.Errorf("UnmarshalBinary() receiver = %q after error, want zero value", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("UnmarshalBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIMSPolicyManagerSettingsUnmarshalTLVs(t *testing.T) {
	apns := []byte{3, 'i', 'm', 's', 3, 's', 'o', 's', 0, 0, 0, 0}
	mask := IMSServiceVoLTE | IMSServiceVideoTelephony
	tests := []struct {
		name      string
		decode    func(tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error)
		tlvs      tlv.TLVs
		wantError bool
	}{
		{
			name: "get response",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSGetPolicyManagerSettingsResponse
				err := got.UnmarshalTLVs(tlvs)
				return got.SettingsError, got.SettingsErrorKnown, got.Settings, err
			},
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{byte(IMSSettingsNotReady)}),
				tlv.Bytes(0x15, []byte{1}), tlv.Bytes(0x16, []byte{2}),
				tlv.Bytes(0x17, []byte{3}), tlv.Bytes(0x18, []byte{4}),
				tlv.Bytes(0x19, binary.LittleEndian.AppendUint64(nil, uint64(mask))),
				tlv.Bytes(0x1A, apns),
			},
		},
		{
			name: "indication",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSPolicyManagerSettingsIndication
				err := got.UnmarshalTLVs(tlvs)
				return 0, false, got.Settings, err
			},
			tlvs: tlv.TLVs{
				tlv.Bytes(0x14, []byte{1}), tlv.Bytes(0x15, []byte{2}),
				tlv.Bytes(0x16, []byte{3}), tlv.Bytes(0x17, []byte{4}),
				tlv.Bytes(0x18, binary.LittleEndian.AppendUint64(nil, uint64(mask))),
				tlv.Bytes(0x19, apns),
			},
		},
		{
			name: "settings error wrong libqmi width",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSGetPolicyManagerSettingsResponse
				err := got.UnmarshalTLVs(tlvs)
				return got.SettingsError, got.SettingsErrorKnown, got.Settings, err
			},
			tlvs:      tlv.TLVs{tlv.Uint(0x10, uint32(IMSSettingsNotReady))},
			wantError: true,
		},
		{
			name: "priority trailing byte",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSPolicyManagerSettingsIndication
				err := got.UnmarshalTLVs(tlvs)
				return 0, false, got.Settings, err
			},
			tlvs:      tlv.TLVs{tlv.Bytes(0x14, []byte{1, 0})},
			wantError: true,
		},
		{
			name: "mask trailing byte",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSPolicyManagerSettingsIndication
				err := got.UnmarshalTLVs(tlvs)
				return 0, false, got.Settings, err
			},
			tlvs:      tlv.TLVs{tlv.Bytes(0x18, make([]byte, 9))},
			wantError: true,
		},
		{
			name: "APN trailing byte",
			decode: func(tlvs tlv.TLVs) (IMSSettingsError, bool, IMSPolicyManagerSettings, error) {
				var got IMSPolicyManagerSettingsIndication
				err := got.UnmarshalTLVs(tlvs)
				return 0, false, got.Settings, err
			},
			tlvs:      tlv.TLVs{tlv.Bytes(0x19, append(apns, 0))},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsError, settingsErrorKnown, got, err := tt.decode(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil || (tt.name != "get response" && tt.name != "indication") {
				return
			}
			if tt.name == "get response" && (!settingsErrorKnown || settingsError != IMSSettingsNotReady) {
				t.Fatalf("settings error = %d known %v, want not ready", settingsError, settingsErrorKnown)
			}
			if !got.ACSPriorityKnown || got.ACSPriority != 1 || !got.ISIMPriorityKnown || got.ISIMPriority != 2 ||
				!got.NVPriorityKnown || got.NVPriority != 3 || !got.PCOPriorityKnown || got.PCOPriority != 4 ||
				!got.ServiceMaskKnown || got.ServiceMask != mask || !got.APNsKnown || got.APNs[0] != "ims" || got.APNs[1] != "sos" {
				t.Fatalf("UnmarshalTLVs() = %+v, want all Policy Manager fields", got)
			}
		})
	}
}

func TestIMSSetServicesEnabledRequest(t *testing.T) {
	enabled := true
	disabled := false
	callMode := IMSCallModeWiFiPreferred
	invalidCallMode := IMSCallModeIMS + 1
	tests := []struct {
		name      string
		config    IMSServicesEnabledConfig
		wantError bool
	}{
		{
			name: "all libqmi fields",
			config: IMSServicesEnabledConfig{
				VoiceOverLTE: &enabled, VideoTelephony: &disabled, VoiceWiFi: &enabled,
				CallMode: &callMode, IMS: &enabled, UT: &enabled, SMS: &enabled, USSD: &disabled,
				Presence: &enabled, Autoconfig: &disabled, XDM: &enabled, RCS: &enabled, CarrierConfig: &disabled,
			},
		},
		{name: "empty update"},
		{name: "invalid call mode", config: IMSServicesEnabledConfig{CallMode: &invalidCallMode}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (IMSSetServicesEnabledRequest{ClientID: 7, TransactionID: 9, Config: tt.config}).Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				return
			}
			if got.Service != ServiceIMS || got.MessageID != MessageIMSSetServicesEnabled {
				t.Fatalf("Request() = service 0x%02X message 0x%04X", got.Service, got.MessageID)
			}
			if tt.name == "empty update" {
				if len(got.TLVs) != 0 {
					t.Fatalf("TLVs = %v, want none", got.TLVs)
				}
				return
			}
			assertTLV(t, got.TLVs, 0x10, []byte{1})
			assertTLV(t, got.TLVs, 0x11, []byte{0})
			assertTLV(t, got.TLVs, 0x14, []byte{1})
			assertTLV(t, got.TLVs, 0x15, binary.LittleEndian.AppendUint32(nil, uint32(callMode)))
			assertTLV(t, got.TLVs, 0x25, []byte{0})
		})
	}
}

func TestIMSGetServicesEnabledResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
		check     func(*testing.T, IMSServicesEnabled)
	}{
		{
			name: "Qualcomm response IDs",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x11, []byte{1}), tlv.Bytes(0x12, []byte{0}), tlv.Bytes(0x15, []byte{1}),
				tlv.Bytes(0x19, []byte{1}), tlv.Bytes(0x1A, []byte{0}), tlv.Bytes(0x1B, []byte{1}), tlv.Bytes(0x1D, []byte{1}),
			},
			check: func(t *testing.T, got IMSServicesEnabled) {
				if !got.VoiceOverLTEKnown || !got.VoiceOverLTE || !got.VideoTelephonyKnown || got.VideoTelephony ||
					!got.VoiceWiFiKnown || !got.VoiceWiFi || !got.IMSKnown || !got.IMS ||
					!got.UTKnown || got.UT || !got.SMSKnown || !got.SMS || !got.USSDKnown || !got.USSD {
					t.Fatalf("services = %+v, want all Qualcomm response fields", got)
				}
			},
		},
		{name: "boolean trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0})}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSGetServicesEnabledResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && tt.check != nil {
				tt.check(t, got.Services)
			}
		})
	}
}

func TestIMSServicesEnabledIndicationUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name      string
		tlvs      tlv.TLVs
		wantError bool
	}{
		{
			name: "extended fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{1}), tlv.Bytes(0x11, []byte{1}), tlv.Bytes(0x14, []byte{1}),
				tlv.Bytes(0x18, []byte{1}), tlv.Bytes(0x19, []byte{1}), tlv.Bytes(0x1A, []byte{1}),
				tlv.Bytes(0x1C, []byte{1}), tlv.Bytes(0x1E, []byte{1}), tlv.Bytes(0x1F, []byte{1}),
				tlv.Bytes(0x20, []byte{1}), tlv.Bytes(0x21, []byte{1}), tlv.Bytes(0x25, []byte{1}),
			},
		},
		{name: "carrier config trailing byte", tlvs: tlv.TLVs{tlv.Bytes(0x25, []byte{1, 0})}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IMSServicesEnabledIndication
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantError {
				t.Fatalf("UnmarshalTLVs() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.name == "extended fields" && (!got.Services.PresenceKnown || !got.Services.Presence || !got.Services.AutoconfigKnown || !got.Services.XDMKnown || !got.Services.RCSKnown || !got.Services.CarrierConfigKnown) {
				t.Fatalf("UnmarshalTLVs() = %+v, want extended indication fields", got.Services)
			}
		})
	}
}

func TestIMSBindRequest(t *testing.T) {
	tests := []struct {
		name         string
		subscription IMSSubscription
		want         []byte
		wantError    bool
	}{
		{name: "none", subscription: IMSSubscriptionNone, want: []byte{0xff, 0xff, 0xff, 0xff}},
		{name: "primary", subscription: IMSSubscriptionPrimary, want: []byte{0, 0, 0, 0}},
		{name: "tertiary", subscription: IMSSubscriptionTertiary, want: []byte{2, 0, 0, 0}},
		{name: "below range", subscription: IMSSubscriptionNone - 1, wantError: true},
		{name: "above range", subscription: IMSSubscriptionTertiary + 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (IMSBindRequest{ClientID: 7, TransactionID: 9, Subscription: tt.subscription}).Request()
			if (err != nil) != tt.wantError {
				t.Fatalf("Request() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				return
			}
			if got.Service != ServiceIMS || got.MessageID != MessageIMSBind {
				t.Fatalf("Request() = service 0x%02X message 0x%04X", got.Service, got.MessageID)
			}
			assertTLV(t, got.TLVs, 0x01, tt.want)
		})
	}
}

func TestIMSSettingsWatchers(t *testing.T) {
	tests := []struct {
		name        string
		messageID   MessageID
		registerTLV uint8
		watch       func(context.Context, *Client) (<-chan bool, error)
		indication  Indication
	}{
		{
			name:        "Policy Manager",
			messageID:   MessageIMSPolicyManagerSettings,
			registerTLV: 0x1B,
			watch: func(ctx context.Context, c *Client) (<-chan bool, error) {
				updates, err := c.IMSWatchPolicyManagerSettings(ctx)
				if err != nil {
					return nil, err
				}
				out := make(chan bool, 1)
				go func() {
					defer close(out)
					settings, ok := <-updates
					if ok {
						out <- settings.ACSPriorityKnown && settings.ACSPriority == 4
					}
				}()
				return out, nil
			},
			indication: Indication{TLVs: tlv.TLVs{tlv.Bytes(0x14, []byte{4})}},
		},
		{
			name:        "services enabled",
			messageID:   MessageIMSServicesEnabled,
			registerTLV: 0x2D,
			watch: func(ctx context.Context, c *Client) (<-chan bool, error) {
				updates, err := c.IMSWatchServicesEnabled(ctx)
				if err != nil {
					return nil, err
				}
				out := make(chan bool, 1)
				go func() {
					defer close(out)
					services, ok := <-updates
					if ok {
						out <- services.VoiceOverLTEKnown && services.VoiceOverLTE
					}
				}()
				return out, nil
			},
			indication: Indication{TLVs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &dsdIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						if req.Service != ServiceIMS || req.ClientID != 7 || req.MessageID != MessageIMSRegisterIndications {
							t.Fatalf("register request = service 0x%X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, tt.registerTLV, []byte{1})
					},
					resp: successResponse(MessageIMSRegisterIndications),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, tt.registerTLV, []byte{0})
					},
					resp: successResponse(MessageIMSRegisterIndications),
				},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceIMS: 7}}
			updates, err := tt.watch(ctx, client)
			if err != nil {
				cancel()
				t.Fatalf("watch() error = %v", err)
			}
			transport.emit(tt.messageID, tt.indication)
			select {
			case matches := <-updates:
				if !matches {
					t.Fatal("decoded indication does not match")
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for IMS Settings indication")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestIMSSettingsIndicationReferenceCounting(t *testing.T) {
	tests := []struct {
		name         string
		registration imsSettingsIndicationRegistration
		tlvType      uint8
	}{
		{name: "Policy Manager", registration: imsSettingsIndicationPolicyManager, tlvType: 0x1B},
		{name: "services enabled", registration: imsSettingsIndicationServicesEnabled, tlvType: 0x2D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) { assertTLV(t, req.TLVs, tt.tlvType, []byte{1}) },
					resp:  successResponse(MessageIMSRegisterIndications),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, tt.tlvType, []byte{0}) },
					resp:  successResponse(MessageIMSRegisterIndications),
				},
			}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceIMS: 7}}
			if err := client.acquireIMSSettingsIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("first acquireIMSSettingsIndication() error = %v", err)
			}
			if err := client.acquireIMSSettingsIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("second acquireIMSSettingsIndication() error = %v", err)
			}
			client.releaseIMSSettingsIndication(tt.registration)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("calls after first release = %d, want 1", got)
			}
			client.releaseIMSSettingsIndication(tt.registration)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("calls after final release = %d, want 2", got)
			}
		})
	}
}
