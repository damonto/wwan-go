package qcom

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSBroadcastRequests(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		check   func(*testing.T, Request)
		resp    Response
	}{
		{
			name:    "activate 3GPP broadcasts",
			message: MessageWMSSetBroadcastActivation,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetBroadcastActivation(context.Background(), WMSBroadcastActivation{
					Mode:        WMSMessageModeGW,
					Active:      true,
					ActivateAll: ptr(false),
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1, 1})
				assertTLV(t, req.TLVs, 0x10, []byte{0})
			},
			resp: successResponse(MessageWMSSetBroadcastActivation),
		},
		{
			name:    "set 3GPP channels",
			message: MessageWMSSetBroadcastConfig,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetBroadcastConfig(context.Background(), WMSBroadcastChannelConfig{
					Mode: WMSMessageModeGW,
					Channels3GPP: []WMS3GPPBroadcastChannel{
						{Start: 1, End: 5, Selected: true},
						{Start: 0x10, End: 0x20},
					},
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1})
				assertTLV(t, req.TLVs, 0x10, []byte{2, 0, 1, 0, 5, 0, 1, 0x10, 0, 0x20, 0, 0})
			},
			resp: successResponse(MessageWMSSetBroadcastConfig),
		},
		{
			name:    "set 3GPP2 channels",
			message: MessageWMSSetBroadcastConfig,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetBroadcastConfig(context.Background(), WMSBroadcastChannelConfig{
					Mode: WMSMessageModeCDMA,
					Channels3GPP2: []WMS3GPP2BroadcastChannel{{
						ServiceCategory: WMSServiceCategoryPresidentialAlert,
						Language:        WMSLanguageEnglish,
						Selected:        true,
					}},
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0})
				assertTLV(t, req.TLVs, 0x11, []byte{1, 0, 0, 0x10, 1, 0, 1})
			},
			resp: successResponse(MessageWMSSetBroadcastConfig),
		},
		{
			name:    "get 3GPP channels",
			message: MessageWMSGetBroadcastConfig,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSBroadcastConfig(context.Background(), WMSMessageModeGW)
				want := []WMS3GPPBroadcastChannel{{Start: 0x1234, End: 0x5678, Selected: true}}
				if err == nil && (!got.ConfigKnown || !got.Active || got.Mode != WMSMessageModeGW || !slices.Equal(got.Channels3GPP, want)) {
					t.Fatalf("config = %+v, want active 3GPP %+v", got, want)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp: successResponse(MessageWMSGetBroadcastConfig,
				tlv.Bytes(0x10, []byte{1, 1, 0, 0x34, 0x12, 0x78, 0x56, 1}),
			),
		},
		{
			name:    "get 3GPP2 channels",
			message: MessageWMSGetBroadcastConfig,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSBroadcastConfig(context.Background(), WMSMessageModeCDMA)
				want := []WMS3GPP2BroadcastChannel{{
					ServiceCategory: WMSServiceCategoryPresidentialAlert,
					Language:        WMSLanguageEnglish,
					Selected:        true,
				}}
				if err == nil && (!got.ConfigKnown || got.Active || got.Mode != WMSMessageModeCDMA || !slices.Equal(got.Channels3GPP2, want)) {
					t.Fatalf("config = %+v, want inactive 3GPP2 %+v", got, want)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{0}) },
			resp: successResponse(MessageWMSGetBroadcastConfig,
				tlv.Bytes(0x11, []byte{0, 1, 0, 0, 0x10, 1, 0, 1}),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWMS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want WMS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.message)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestWMSBroadcastValidation(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "activation mode",
			call: func() error {
				return (&Client{}).WMSSetBroadcastActivation(context.Background(), WMSBroadcastActivation{Mode: 2})
			},
		},
		{
			name: "3GPP range reversed",
			call: func() error {
				_, err := encodeWMS3GPPBroadcastChannels([]WMS3GPPBroadcastChannel{{Start: 2, End: 1}})
				return err
			},
		},
		{
			name: "too many 3GPP channels",
			call: func() error {
				_, err := encodeWMS3GPPBroadcastChannels(make([]WMS3GPPBroadcastChannel, wmsBroadcastChannelMax+1))
				return err
			},
		},
		{
			name: "3GPP2 language",
			call: func() error {
				_, err := encodeWMS3GPP2BroadcastChannels([]WMS3GPP2BroadcastChannel{{Language: WMSLanguageHebrew + 1}})
				return err
			},
		},
		{
			name: "GW with 3GPP2 channels",
			call: func() error {
				return (&Client{}).WMSSetBroadcastConfig(context.Background(), WMSBroadcastChannelConfig{
					Mode:          WMSMessageModeGW,
					Channels3GPP2: []WMS3GPP2BroadcastChannel{{}},
				})
			},
		},
		{
			name: "CDMA with 3GPP channels",
			call: func() error {
				return (&Client{}).WMSSetBroadcastConfig(context.Background(), WMSBroadcastChannelConfig{
					Mode:         WMSMessageModeCDMA,
					Channels3GPP: []WMS3GPPBroadcastChannel{{}},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestWMSBroadcastRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "3GPP header truncated", call: func() error { _, _, err := decodeWMS3GPPBroadcastConfig([]byte{1, 0}); return err }},
		{name: "3GPP invalid activation", call: func() error { _, _, err := decodeWMS3GPPBroadcastConfig([]byte{2, 0, 0}); return err }},
		{name: "3GPP entry truncated", call: func() error { _, _, err := decodeWMS3GPPBroadcastConfig([]byte{1, 1, 0, 1}); return err }},
		{name: "3GPP reversed range", call: func() error { _, _, err := decodeWMS3GPPBroadcastConfig([]byte{1, 1, 0, 2, 0, 1, 0, 1}); return err }},
		{name: "3GPP invalid selection", call: func() error { _, _, err := decodeWMS3GPPBroadcastConfig([]byte{1, 1, 0, 1, 0, 2, 0, 2}); return err }},
		{name: "3GPP2 language", call: func() error { _, _, err := decodeWMS3GPP2BroadcastConfig([]byte{1, 1, 0, 0, 0, 8, 0, 1}); return err }},
		{name: "3GPP2 invalid selection", call: func() error { _, _, err := decodeWMS3GPP2BroadcastConfig([]byte{1, 1, 0, 0, 0, 1, 0, 2}); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("decode() error = nil, want non-nil")
			}
		})
	}
}

func TestWMSBroadcastConfigIndicationDecoding(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want WMSBroadcastConfig
	}{
		{
			name: "3GPP config",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(WMSMessageModeGW)}),
				tlv.Bytes(0x10, []byte{1, 1, 0, 0x34, 0x12, 0x78, 0x56, 1}),
			},
			want: WMSBroadcastConfig{
				Active: true, Mode: WMSMessageModeGW, ConfigKnown: true,
				Channels3GPP: []WMS3GPPBroadcastChannel{{Start: 0x1234, End: 0x5678, Selected: true}},
			},
		},
		{
			name: "3GPP2 config omitted",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{byte(WMSMessageModeCDMA)})},
			want: WMSBroadcastConfig{Mode: WMSMessageModeCDMA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WMSBroadcastConfig
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Active != tt.want.Active || got.Mode != tt.want.Mode || got.ConfigKnown != tt.want.ConfigKnown ||
				!slices.Equal(got.Channels3GPP, tt.want.Channels3GPP) || !slices.Equal(got.Channels3GPP2, tt.want.Channels3GPP2) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWMSBroadcastConfigIndicationRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{name: "mode missing"},
		{name: "mode truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{0, 1})}},
		{name: "mode out of range", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{2})}},
		{
			name: "GW with 3GPP2 config",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{byte(WMSMessageModeGW)}), tlv.Bytes(0x11, []byte{0, 0, 0})},
		},
		{
			name: "CDMA with 3GPP config",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{byte(WMSMessageModeCDMA)}), tlv.Bytes(0x10, []byte{0, 0, 0})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config WMSBroadcastConfig
			if err := config.UnmarshalTLVs(tt.tlvs); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestWMSWatchBroadcastConfig(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forwards 3GPP config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &wmsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageWMSIndicationRegister)},
				{resp: successResponse(MessageWMSIndicationRegister)},
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			out, err := client.WMSWatchBroadcastConfig(ctx)
			if err != nil {
				t.Fatalf("WMSWatchBroadcastConfig() error = %v", err)
			}
			transport.emit(Indication{Service: ServiceWMS, ClientID: 7, MessageID: MessageWMSBroadcastConfigChanged, TLVs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(WMSMessageModeGW)}),
				tlv.Bytes(0x10, []byte{1, 1, 0, 1, 0, 2, 0, 1}),
			}})
			select {
			case got := <-out:
				want := []WMS3GPPBroadcastChannel{{Start: 1, End: 2, Selected: true}}
				if !got.ConfigKnown || !got.Active || !slices.Equal(got.Channels3GPP, want) {
					t.Fatalf("broadcast config = %+v, want %+v", got, want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for WMS broadcast-config event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}
