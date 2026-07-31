package qcom

import (
	"context"
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASSignalInfoUnmarshalTLVs(t *testing.T) {
	tds := binary.LittleEndian.AppendUint32(nil, math.Float32bits(-75.5))
	tds = binary.LittleEndian.AppendUint32(tds, math.Float32bits(-91.25))
	tds = binary.LittleEndian.AppendUint32(tds, math.Float32bits(-8.5))
	tds = binary.LittleEndian.AppendUint32(tds, math.Float32bits(4.25))
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASSignalInfo
		wantErr bool
	}{
		{
			name: "all radio interfaces",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVSignalCDMA, []byte{0xBF, 0xF4, 0xFF}),
				tlv.Bytes(nasTLVSignalHDR, []byte{0xBA, 0xF2, 0xFF, 5, 0xD8, 0xDC, 0xFF, 0xFF}),
				tlv.Bytes(nasTLVSignalGSM, []byte{0xB5}),
				tlv.Bytes(nasTLVSignalWCDMA, []byte{0xB0, 0xEE, 0xFF}),
				tlv.Bytes(nasTLVSignalLTE, []byte{0xB0, 0xF6, 0xE6, 0xFB, 0x7D, 0x00}),
				tlv.Bytes(nasTLVSignalTDSRSCP, []byte{0xA5}),
				tlv.Bytes(nasTLVSignalTDS, tds),
				tlv.Bytes(nasTLVSignalNR5G, []byte{0x4A, 0xFC, 0xB4, 0x00}),
				tlv.Bytes(nasTLVSignalNR5GRSRQ, []byte{0x88, 0xFF}),
				tlv.Bytes(nasTLVSignalUMTSRSCP, []byte{0xAE, 0xFC}),
			},
			want: NASSignalInfo{
				CDMA:             NASCommonSignalInfo{RSSI: -65, ECIO: -12},
				CDMAKnown:        true,
				HDR:              NASHDRSignalInfo{NASCommonSignalInfo: NASCommonSignalInfo{RSSI: -70, ECIO: -14}, SINRLevel: 5, IO: -9000},
				HDRKnown:         true,
				GSM:              -75,
				GSMKnown:         true,
				WCDMA:            NASCommonSignalInfo{RSSI: -80, ECIO: -18},
				WCDMAKnown:       true,
				LTE:              NASLTESignalInfo{RSSI: -80, RSRQ: -10, RSRP: -1050, SNR: 125},
				LTEKnown:         true,
				TDSCDMARSCP:      -91,
				TDSCDMARSCPKnown: true,
				TDSCDMA:          NASTDSCDMASignalInfo{RSSI: -75.5, RSCP: -91.25, ECIO: -8.5, SINR: 4.25},
				TDSCDMAKnown:     true,
				NR5G:             NASNR5GSignalInfo{RSRP: -950, SNR: 180},
				NR5GKnown:        true,
				NR5GRSRQ:         -120,
				NR5GRSRQKnown:    true,
				UMTSRSCP:         -850,
				UMTSRSCPKnown:    true,
			},
		},
		{name: "CDMA truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalCDMA, make([]byte, 2))}, wantErr: true},
		{name: "HDR truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalHDR, make([]byte, 7))}, wantErr: true},
		{name: "GSM truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalGSM, nil)}, wantErr: true},
		{name: "WCDMA truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalWCDMA, make([]byte, 2))}, wantErr: true},
		{name: "LTE truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalLTE, make([]byte, 5))}, wantErr: true},
		{name: "TD-SCDMA RSCP truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalTDSRSCP, nil)}, wantErr: true},
		{name: "TD-SCDMA truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalTDS, make([]byte, 15))}, wantErr: true},
		{name: "NR5G truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalNR5G, make([]byte, 3))}, wantErr: true},
		{name: "NR5G RSRQ truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalNR5GRSRQ, make([]byte, 1))}, wantErr: true},
		{name: "UMTS RSCP truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSignalUMTSRSCP, make([]byte, 1))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSignalInfo
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
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASRFBandInfoUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASRFBandInfo
		wantErr bool
	}{
		{
			name: "legacy channel",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 8, 41, 0, 0x72, 0x06})},
			want: NASRFBandInfo{Bands: []NASRFBand{{RadioInterface: NASRadioInterfaceLTE, Band: 41, Channel: 1650}}},
		},
		{
			name: "extended channels and bandwidths",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0}),
				tlv.Bytes(0x10, []byte{1, 8, 42, 0}),
				tlv.Bytes(nasTLVRFBandsExtended, []byte{2, 8, 3, 0, 8, 7, 0, 0, 12, 78, 0, 0, 0xC4, 9, 0}),
				tlv.Bytes(nasTLVRFBandwidths, []byte{2, 8, 6, 0, 0, 0, 12, 13, 0, 0, 0}),
				tlv.Bytes(0x13, []byte{2, 0, 0, 0}),
			},
			want: NASRFBandInfo{
				Bands: []NASRFBand{
					{RadioInterface: NASRadioInterfaceLTE, Band: 3, Channel: 1800},
					{RadioInterface: NASRadioInterfaceNR5G, Band: 78, Channel: 640000},
				},
				Extended:            true,
				DedicatedBands:      []NASRFDedicatedBand{{RadioInterface: NASRadioInterfaceLTE, Band: 42}},
				DedicatedBandsKnown: true,
				Bandwidths: []NASRFBandwidth{
					{RadioInterface: NASRadioInterfaceLTE, Bandwidth: 6},
					{RadioInterface: NASRadioInterfaceNR5G, Bandwidth: 13},
				},
				BandwidthsKnown:  true,
				CIoTLTEMode:      NASCIoTLTEModeM1,
				CIoTLTEModeKnown: true,
			},
		},
		{name: "missing primary bands", wantErr: true},
		{name: "primary count truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, nil)}, wantErr: true},
		{name: "primary count too large", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{nasMaxRFBands + 1})}, wantErr: true},
		{name: "primary list truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 8})}, wantErr: true},
		{
			name: "extended list truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0}),
				tlv.Bytes(nasTLVRFBandsExtended, []byte{1, 8, 3, 0}),
			},
			wantErr: true,
		},
		{
			name: "dedicated list truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0}),
				tlv.Bytes(0x10, []byte{1, 8}),
			},
			wantErr: true,
		},
		{
			name: "bandwidth list truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0}),
				tlv.Bytes(nasTLVRFBandwidths, []byte{1, 8}),
			},
			wantErr: true,
		},
		{
			name: "LTE mode width",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0}),
				tlv.Bytes(0x13, []byte{1}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASRFBandInfo
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
			if !slices.Equal(got.Bands, tt.want.Bands) ||
				got.Extended != tt.want.Extended ||
				!slices.Equal(got.DedicatedBands, tt.want.DedicatedBands) ||
				got.DedicatedBandsKnown != tt.want.DedicatedBandsKnown ||
				!slices.Equal(got.Bandwidths, tt.want.Bandwidths) ||
				got.BandwidthsKnown != tt.want.BandwidthsKnown ||
				got.CIoTLTEMode != tt.want.CIoTLTEMode || got.CIoTLTEModeKnown != tt.want.CIoTLTEModeKnown {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASHomeNetworkUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASHomeNetwork
		wantErr bool
	}{
		{
			name: "3GPP home network",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{0x36, 0x01, 0x04, 0x01, 4, 'H', 'o', 'm', 'e'}),
				tlv.Bytes(nasTLVHomeMNCIncludesPCSDigit, []byte{1, 1}),
				tlv.Bytes(nasTLVHomeNetworkNameSource, []byte{5, 0, 0, 0}),
			},
			want: NASHomeNetwork{
				PLMN:            NASPLMN{MCC: 310, MNC: 260, Description: "Home", MNCThreeDigits: true, MNCThreeDigitsKnown: true},
				Is3GPP:          true,
				Is3GPPKnown:     true,
				NameSource:      NASNetworkNameSourceMCCMNC,
				NameSourceKnown: true,
			},
		},
		{name: "missing home network", wantErr: true},
		{name: "PLMN truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, make([]byte, 4))}, wantErr: true},
		{name: "PLMN description truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 0, 2, 0, 2, 'A'})}, wantErr: true},
		{
			name: "MNC digit truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{1, 0, 2, 0, 0}),
				tlv.Bytes(nasTLVHomeMNCIncludesPCSDigit, []byte{1}),
			},
			wantErr: true,
		},
		{
			name: "name source truncated",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{1, 0, 2, 0, 0}),
				tlv.Bytes(nasTLVHomeNetworkNameSource, make([]byte, 3)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASHomeNetwork
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
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASPLMNString(t *testing.T) {
	tests := []struct {
		name string
		plmn NASPLMN
		want string
	}{
		{name: "two digit MNC", plmn: NASPLMN{MCC: 460, MNC: 1}, want: "46001"},
		{name: "three digit MNC", plmn: NASPLMN{MCC: 460, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, want: "460001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plmn.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNASNetworkTimesUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASNetworkTimes
		wantErr bool
	}{
		{
			name: "3GPP and 3GPP2",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVNetworkTime3GPP2, []byte{0xE9, 0x07, 12, 31, 23, 59, 58, 3, 0xE0, 0, 1}),
				tlv.Bytes(nasTLVNetworkTime3GPP, []byte{0xEA, 0x07, 7, 29, 12, 34, 56, 4, 32, 1, 8}),
			},
			want: NASNetworkTimes{
				ThreeGPP2: NASNetworkTime{
					Year: 2025, Month: 12, Day: 31, Hour: 23, Minute: 59, Second: 58, DayOfWeek: 3,
					TimeZoneQuarterHours: -32, RadioInterface: NASRadioInterfaceCDMA1X,
				},
				ThreeGPP2Known: true,
				ThreeGPP: NASNetworkTime{
					Year: 2026, Month: 7, Day: 29, Hour: 12, Minute: 34, Second: 56, DayOfWeek: 4,
					TimeZoneQuarterHours: 32, DaylightSavingHours: 1, RadioInterface: NASRadioInterfaceLTE,
				},
				ThreeGPPKnown: true,
			},
		},
		{name: "3GPP2 truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVNetworkTime3GPP2, make([]byte, 10))}, wantErr: true},
		{name: "3GPP oversized", tlvs: tlv.TLVs{tlv.Bytes(nasTLVNetworkTime3GPP, make([]byte, 12))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASNetworkTimes
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
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASNetworkTimeUpdateUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASNetworkTimeUpdate
		wantErr bool
	}{
		{
			name: "complete indication",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVNetworkTimeUniversal, []byte{0xEA, 0x07, 7, 29, 12, 34, 56, 2}),
				tlv.Uint(nasTLVNetworkTimeZone, uint8(0xE0)),
				tlv.Uint(nasTLVNetworkTimeDaylightSaving, uint8(1)),
				tlv.Uint(nasTLVNetworkTimeRadioInterface, uint8(NASRadioInterfaceNR5G)),
			},
			want: NASNetworkTimeUpdate{
				Time: NASNetworkTime{
					Year: 2026, Month: 7, Day: 29, Hour: 12, Minute: 34, Second: 56, DayOfWeek: 2,
					TimeZoneQuarterHours: -32, DaylightSavingHours: 1, RadioInterface: NASRadioInterfaceNR5G,
				},
				TimeZoneKnown: true, DaylightSavingKnown: true, RadioInterfaceKnown: true,
			},
		},
		{
			name: "only mandatory time",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVNetworkTimeUniversal, []byte{0xE9, 0x07, 1, 2, 3, 4, 5, 6})},
			want: NASNetworkTimeUpdate{Time: NASNetworkTime{Year: 2025, Month: 1, Day: 2, Hour: 3, Minute: 4, Second: 5, DayOfWeek: 6}},
		},
		{name: "missing mandatory time", wantErr: true},
		{name: "truncated time", tlvs: tlv.TLVs{tlv.Bytes(nasTLVNetworkTimeUniversal, make([]byte, 7))}, wantErr: true},
		{
			name: "truncated time zone",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVNetworkTimeUniversal, make([]byte, 8)),
				tlv.Bytes(nasTLVNetworkTimeZone, nil),
			},
			wantErr: true,
		},
		{
			name: "truncated daylight saving",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVNetworkTimeUniversal, make([]byte, 8)),
				tlv.Bytes(nasTLVNetworkTimeDaylightSaving, nil),
			},
			wantErr: true,
		},
		{
			name: "truncated radio interface",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVNetworkTimeUniversal, make([]byte, 8)),
				tlv.Bytes(nasTLVNetworkTimeRadioInterface, nil),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASNetworkTimeUpdate
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASBindSubscriptionRequest(t *testing.T) {
	tests := []struct {
		name         string
		subscription NASSubscription
		wantErr      bool
	}{
		{name: "primary", subscription: NASSubscriptionPrimary},
		{name: "secondary", subscription: NASSubscriptionSecondary},
		{name: "tertiary", subscription: NASSubscriptionTertiary},
		{name: "out of range", subscription: NASSubscriptionTertiary + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASBindSubscriptionRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
				Subscription:  tt.subscription,
			}).Request()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Request() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if req.Service != ServiceNAS || req.ClientID != 7 || req.TransactionID != 9 ||
				req.MessageID != MessageNASBindSubscription || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			value, ok := tlv.Value(req.TLVs, nasTLVServingSystem)
			if !ok || !slices.Equal(value, []byte{byte(tt.subscription)}) {
				t.Fatalf("subscription TLV = %v, present %v", value, ok)
			}
		})
	}
}

func TestClientNASInfoMessageMapping(t *testing.T) {
	tests := []struct {
		name  string
		call  func(*Client) error
		want  MessageID
		check func(*testing.T, Request)
		resp  Response
	}{
		{
			name: "system information",
			call: func(c *Client) error {
				_, err := c.SystemInfo(context.Background())
				return err
			},
			want: MessageNASGetSysInfo,
			resp: successResponse(MessageNASGetSysInfo),
		},
		{
			name: "signal information",
			call: func(c *Client) error {
				_, err := c.SignalInfo(context.Background())
				return err
			},
			want: MessageNASGetSignalInfo,
			resp: successResponse(MessageNASGetSignalInfo),
		},
		{
			name: "RF band information",
			call: func(c *Client) error {
				_, err := c.RFBandInfo(context.Background())
				return err
			},
			want: MessageNASGetRFBandInfo,
			resp: successResponse(MessageNASGetRFBandInfo, tlv.Bytes(nasTLVServingSystem, []byte{0})),
		},
		{
			name: "home network",
			call: func(c *Client) error {
				_, err := c.HomeNetwork(context.Background())
				return err
			},
			want: MessageNASGetHomeNetwork,
			resp: successResponse(MessageNASGetHomeNetwork, tlv.Bytes(nasTLVServingSystem, []byte{0, 0, 0, 0, 0})),
		},
		{
			name: "network time",
			call: func(c *Client) error {
				_, err := c.NetworkTime(context.Background())
				return err
			},
			want: MessageNASGetNetworkTime,
			resp: successResponse(MessageNASGetNetworkTime),
		},
		{
			name: "bind subscription",
			call: func(c *Client) error {
				return c.NASBindSubscription(context.Background(), NASSubscriptionSecondary)
			},
			want: MessageNASBindSubscription,
			check: func(t *testing.T, req Request) {
				value, ok := tlv.Value(req.TLVs, nasTLVServingSystem)
				if !ok || !slices.Equal(value, []byte{byte(NASSubscriptionSecondary)}) {
					t.Fatalf("subscription TLV = %v, present %v", value, ok)
				}
			},
			resp: successResponse(MessageNASBindSubscription),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != tt.want {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want NAS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.want)
					}
					if tt.check != nil {
						tt.check(t, req)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}
