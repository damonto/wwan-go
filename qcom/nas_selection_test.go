package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASSetSystemSelectionPreferenceRequest(t *testing.T) {
	mode := NASModePreferenceLTE | NASModePreferenceNR5G
	band := NASBandPreference(0x0102030405060708)
	prl := NASPRLPreferenceAny
	roaming := NASRoamingPreferenceAny
	lteBand := NASLTEBandPreference(0x1122334455667788)
	radio := NASRadioInterfaceLTE
	duration := NASChangePermanent
	domain := NASServiceDomainPSOnly
	gwOrder := NASGWAcquisitionAutomatic
	tdsBands := NASTDSCDMABandPreference(0x3F)
	restriction := NASRegistrationUnrestricted
	usage := NASUsageDataCentric
	voice := NASVoiceDomainPSPreferred
	nrDisable := NASNR5GDisableNSA
	emergency := true
	lteExtended := NASLTEBandPreferenceExtended{Bits1To64: 1, Bits65To128: 2, Bits129To192: 3, Bits193To256: 4}
	nrBands := NASNR5GBandPreference{
		Bits1To64: 1, Bits65To128: 2, Bits129To192: 3, Bits193To256: 4,
		Bits257To320: 5, Bits321To384: 6, Bits385To448: 7, Bits449To512: 8,
	}
	tests := []struct {
		name    string
		config  NASSystemSelectionConfig
		check   func(*testing.T, Request)
		wantErr bool
	}{
		{
			name: "all practical fields",
			config: NASSystemSelectionConfig{
				EmergencyMode:     &emergency,
				ModePreference:    &mode,
				BandPreference:    &band,
				PRLPreference:     &prl,
				RoamingPreference: &roaming,
				LTEBandPreference: &lteBand,
				NetworkSelection: &NASNetworkSelection{
					Mode: NASNetworkSelectionManual,
					PLMN: NASPLMN{
						MCC: 460, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true,
					},
					RadioInterface: &radio,
				},
				ChangeDuration:        &duration,
				ServiceDomain:         &domain,
				GWAcquisitionOrder:    &gwOrder,
				TDSCDMABandPreference: &tdsBands,
				AcquisitionOrder:      []NASRadioInterface{NASRadioInterfaceNR5G, NASRadioInterfaceLTE},
				RegistrationRestrict:  &restriction,
				UsagePreference:       &usage,
				VoiceDomain:           &voice,
				LTEBandsExtended:      &lteExtended,
				NR5GBands:             &nrBands,
				NR5GDisableMode:       &nrDisable,
				NR5GSABands:           &nrBands,
				NR5GNSABands:          &nrBands,
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, nasTLVSelectionEmergencyMode, []byte{1})
				assertTLV(t, req.TLVs, nasTLVSelectionModePreference, []byte{0x50, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionBandPreference, binary.LittleEndian.AppendUint64(nil, uint64(band)))
				assertTLV(t, req.TLVs, nasTLVSelectionPRLPreference, []byte{0xFF, 0x3F})
				assertTLV(t, req.TLVs, nasTLVSelectionRoamingPreference, []byte{0xFF, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionLTEBandPreference, binary.LittleEndian.AppendUint64(nil, uint64(lteBand)))
				assertTLV(t, req.TLVs, nasTLVSelectionNetwork, []byte{1, 0xCC, 0x01, 1, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionMNCThreeDigits, []byte{1})
				assertTLV(t, req.TLVs, nasTLVSelectionSetRadioInterface, []byte{8})
				assertTLV(t, req.TLVs, nasTLVSelectionChangeDuration, []byte{1})
				assertTLV(t, req.TLVs, nasTLVSelectionServiceDomain, []byte{1, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionGWAcquisitionOrder, []byte{0, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionSetTDSCDMABands, binary.LittleEndian.AppendUint64(nil, uint64(tdsBands)))
				assertTLV(t, req.TLVs, nasTLVSelectionSetAcquisitionOrder, []byte{2, 12, 8})
				assertTLV(t, req.TLVs, nasTLVSelectionSetRegistrationRestrict, []byte{0, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionSetUsagePreference, []byte{2, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionSetVoiceDomain, []byte{3, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionSetLTEBandsExtended, mustMarshalBinary(t, lteExtended))
				assertTLV(t, req.TLVs, nasTLVSelectionSetNR5GBands, mustMarshalBinary(t, nrBands))
				assertTLV(t, req.TLVs, nasTLVSelectionSetNR5GDisableMode, []byte{2, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVSelectionSetNR5GSABands, mustMarshalBinary(t, nrBands))
				assertTLV(t, req.TLVs, nasTLVSelectionSetNR5GNSABands, mustMarshalBinary(t, nrBands))
			},
		},
		{
			name: "empty config",
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
				}
			},
		},
		{
			name: "acquisition order too long",
			config: NASSystemSelectionConfig{
				AcquisitionOrder: make([]NASRadioInterface, nasMaxAcquisitionOrder+1),
			},
			wantErr: true,
		},
		{
			name: "network mode out of range",
			config: NASSystemSelectionConfig{
				NetworkSelection: &NASNetworkSelection{Mode: NASNetworkSelectionManual + 1},
			},
			wantErr: true,
		},
		{
			name: "PLMN out of range",
			config: NASSystemSelectionConfig{
				NetworkSelection: &NASNetworkSelection{Mode: NASNetworkSelectionManual, PLMN: NASPLMN{MCC: 1000}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASSetSystemSelectionPreferenceRequest{
				ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Config: tt.config,
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
				req.MessageID != MessageNASSetSystemSelectionPreference || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			tt.check(t, req)
		})
	}
}

func TestNASSystemSelectionPreferenceUnmarshalTLVs(t *testing.T) {
	lteExtended := NASLTEBandPreferenceExtended{Bits1To64: 1, Bits65To128: 2, Bits129To192: 3, Bits193To256: 4}
	nrBands := NASNR5GBandPreference{
		Bits1To64: 1, Bits65To128: 2, Bits129To192: 3, Bits193To256: 4,
		Bits257To320: 5, Bits321To384: 6, Bits385To448: 7, Bits449To512: 8,
	}
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want NASSystemSelectionPreference
	}{
		{name: "no optional fields"},
		{
			name: "all practical fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVSelectionEmergencyMode, []byte{1}),
				tlv.Uint(nasTLVSelectionModePreference, uint16(NASModePreferenceLTE|NASModePreferenceNR5G)),
				nasUint64TLV(nasTLVSelectionBandPreference, 0x0102030405060708),
				tlv.Uint(nasTLVSelectionPRLPreference, uint16(NASPRLPreferenceAny)),
				tlv.Uint(nasTLVSelectionRoamingPreference, uint16(NASRoamingPreferenceAny)),
				nasUint64TLV(nasTLVSelectionLTEBandPreference, 0x1122334455667788),
				tlv.Bytes(nasTLVSelectionNetwork, []byte{byte(NASNetworkSelectionManual)}),
				tlv.Uint(nasTLVSelectionServiceDomain, uint32(NASServiceDomainPSOnly)),
				tlv.Uint(nasTLVSelectionGWAcquisitionOrder, uint32(NASGWAcquisitionAutomatic)),
				nasUint64TLV(nasTLVSelectionGetTDSCDMABands, 0x3F),
				tlv.Bytes(nasTLVSelectionGetManualPLMN, []byte{0xCC, 0x01, 1, 0, 1}),
				tlv.Bytes(nasTLVSelectionGetAcquisitionOrder, []byte{2, 12, 8}),
				tlv.Uint(nasTLVSelectionGetRegistrationRestrict, uint32(NASRegistrationUnrestricted)),
				tlv.Uint(nasTLVSelectionGetUsagePreference, uint32(NASUsageDataCentric)),
				tlv.Uint(nasTLVSelectionGetVoiceDomain, uint32(NASVoiceDomainPSPreferred)),
				tlv.Uint(nasTLVSelectionGetLTEDisableCause, uint32(NASLTEDisableUser)),
				tlv.Uint(nasTLVSelectionGetDisabledRATs, uint16(NASModePreferenceGSM)),
				tlv.Bytes(nasTLVSelectionGetLTEBandsExtended, mustMarshalBinary(t, lteExtended)),
				tlv.Bytes(nasTLVSelectionGetNR5GBands, mustMarshalBinary(t, nrBands)),
				tlv.Uint(nasTLVSelectionGetNR5GDisableMode, uint32(NASNR5GDisableNSA)),
				tlv.Bytes(nasTLVSelectionGetNR5GSABands, mustMarshalBinary(t, nrBands)),
				tlv.Bytes(nasTLVSelectionGetNR5GNSABands, mustMarshalBinary(t, nrBands)),
			},
			want: NASSystemSelectionPreference{
				EmergencyMode: true, EmergencyModeKnown: true,
				ModePreference: NASModePreferenceLTE | NASModePreferenceNR5G, ModePreferenceKnown: true,
				BandPreference: 0x0102030405060708, BandPreferenceKnown: true,
				PRLPreference: NASPRLPreferenceAny, PRLPreferenceKnown: true,
				RoamingPreference: NASRoamingPreferenceAny, RoamingPreferenceKnown: true,
				LTEBandPreference: 0x1122334455667788, LTEBandPreferenceKnown: true,
				NetworkSelection: NASNetworkSelectionManual, NetworkSelectionKnown: true,
				ServiceDomain: NASServiceDomainPSOnly, ServiceDomainKnown: true,
				GWAcquisitionOrder: NASGWAcquisitionAutomatic, GWAcquisitionOrderKnown: true,
				TDSCDMABandPreference: 0x3F, TDSCDMABandPreferenceKnown: true,
				ManualPLMN: NASPLMN{MCC: 460, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, ManualPLMNKnown: true,
				AcquisitionOrder: []NASRadioInterface{NASRadioInterfaceNR5G, NASRadioInterfaceLTE}, AcquisitionOrderKnown: true,
				RegistrationRestriction: NASRegistrationUnrestricted, RegistrationRestrictionKnown: true,
				UsagePreference: NASUsageDataCentric, UsagePreferenceKnown: true,
				VoiceDomain: NASVoiceDomainPSPreferred, VoiceDomainKnown: true,
				LTEDisableCause: NASLTEDisableUser, LTEDisableCauseKnown: true,
				DisabledRATs: NASModePreferenceGSM, DisabledRATsKnown: true,
				LTEBandsExtended: lteExtended, LTEBandsExtendedKnown: true,
				NR5GBands: nrBands, NR5GBandsKnown: true,
				NR5GDisableMode: NASNR5GDisableNSA, NR5GDisableModeKnown: true,
				NR5GSABands: nrBands, NR5GSABandsKnown: true,
				NR5GNSABands: nrBands, NR5GNSABandsKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSystemSelectionPreference
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !equalNASSystemSelectionPreference(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASSystemSelectionPreferenceRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlv  tlv.TLV
	}{
		{name: "emergency mode", tlv: tlv.Bytes(nasTLVSelectionEmergencyMode, nil)},
		{name: "mode preference", tlv: tlv.Bytes(nasTLVSelectionModePreference, []byte{1})},
		{name: "band preference", tlv: tlv.Bytes(nasTLVSelectionBandPreference, make([]byte, 7))},
		{name: "PRL preference", tlv: tlv.Bytes(nasTLVSelectionPRLPreference, []byte{1})},
		{name: "roaming preference", tlv: tlv.Bytes(nasTLVSelectionRoamingPreference, []byte{1})},
		{name: "LTE band preference", tlv: tlv.Bytes(nasTLVSelectionLTEBandPreference, make([]byte, 7))},
		{name: "network selection", tlv: tlv.Bytes(nasTLVSelectionNetwork, nil)},
		{name: "service domain", tlv: tlv.Bytes(nasTLVSelectionServiceDomain, make([]byte, 3))},
		{name: "GSM/WCDMA order", tlv: tlv.Bytes(nasTLVSelectionGWAcquisitionOrder, make([]byte, 3))},
		{name: "TD-SCDMA bands", tlv: tlv.Bytes(nasTLVSelectionGetTDSCDMABands, make([]byte, 7))},
		{name: "manual PLMN", tlv: tlv.Bytes(nasTLVSelectionGetManualPLMN, make([]byte, 4))},
		{name: "acquisition order count", tlv: tlv.Bytes(nasTLVSelectionGetAcquisitionOrder, nil)},
		{name: "acquisition order list", tlv: tlv.Bytes(nasTLVSelectionGetAcquisitionOrder, []byte{2, 8})},
		{name: "acquisition order too long", tlv: tlv.Bytes(nasTLVSelectionGetAcquisitionOrder, []byte{nasMaxAcquisitionOrder + 1})},
		{name: "registration restriction", tlv: tlv.Bytes(nasTLVSelectionGetRegistrationRestrict, make([]byte, 3))},
		{name: "usage preference", tlv: tlv.Bytes(nasTLVSelectionGetUsagePreference, make([]byte, 3))},
		{name: "voice domain", tlv: tlv.Bytes(nasTLVSelectionGetVoiceDomain, make([]byte, 3))},
		{name: "LTE disable cause", tlv: tlv.Bytes(nasTLVSelectionGetLTEDisableCause, make([]byte, 3))},
		{name: "disabled RAT mask", tlv: tlv.Bytes(nasTLVSelectionGetDisabledRATs, []byte{1})},
		{name: "extended LTE bands", tlv: tlv.Bytes(nasTLVSelectionGetLTEBandsExtended, make([]byte, 31))},
		{name: "NR5G bands", tlv: tlv.Bytes(nasTLVSelectionGetNR5GBands, make([]byte, 63))},
		{name: "NR5G disable mode", tlv: tlv.Bytes(nasTLVSelectionGetNR5GDisableMode, make([]byte, 3))},
		{name: "NR5G SA bands", tlv: tlv.Bytes(nasTLVSelectionGetNR5GSABands, make([]byte, 63))},
		{name: "NR5G NSA bands", tlv: tlv.Bytes(nasTLVSelectionGetNR5GNSABands, make([]byte, 63))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSystemSelectionPreference
			if err := got.UnmarshalTLVs(tlv.TLVs{tt.tlv}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestClientNASSystemSelectionMessageMapping(t *testing.T) {
	mode := NASModePreferenceLTE
	tests := []struct {
		name string
		call func(*Client) error
		want MessageID
		resp Response
	}{
		{
			name: "get",
			call: func(c *Client) error {
				_, err := c.SystemSelectionPreference(context.Background())
				return err
			},
			want: MessageNASGetSystemSelectionPreference,
			resp: successResponse(MessageNASGetSystemSelectionPreference),
		},
		{
			name: "set",
			call: func(c *Client) error {
				return c.SetSystemSelectionPreference(context.Background(), NASSystemSelectionConfig{ModePreference: &mode})
			},
			want: MessageNASSetSystemSelectionPreference,
			resp: successResponse(MessageNASSetSystemSelectionPreference),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != tt.want {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want NAS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.want)
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

func equalNASSystemSelectionPreference(got, want NASSystemSelectionPreference) bool {
	return got.EmergencyMode == want.EmergencyMode &&
		got.EmergencyModeKnown == want.EmergencyModeKnown &&
		got.ModePreference == want.ModePreference &&
		got.ModePreferenceKnown == want.ModePreferenceKnown &&
		got.BandPreference == want.BandPreference &&
		got.BandPreferenceKnown == want.BandPreferenceKnown &&
		got.PRLPreference == want.PRLPreference &&
		got.PRLPreferenceKnown == want.PRLPreferenceKnown &&
		got.RoamingPreference == want.RoamingPreference &&
		got.RoamingPreferenceKnown == want.RoamingPreferenceKnown &&
		got.LTEBandPreference == want.LTEBandPreference &&
		got.LTEBandPreferenceKnown == want.LTEBandPreferenceKnown &&
		got.NetworkSelection == want.NetworkSelection &&
		got.NetworkSelectionKnown == want.NetworkSelectionKnown &&
		got.ServiceDomain == want.ServiceDomain &&
		got.ServiceDomainKnown == want.ServiceDomainKnown &&
		got.GWAcquisitionOrder == want.GWAcquisitionOrder &&
		got.GWAcquisitionOrderKnown == want.GWAcquisitionOrderKnown &&
		got.TDSCDMABandPreference == want.TDSCDMABandPreference &&
		got.TDSCDMABandPreferenceKnown == want.TDSCDMABandPreferenceKnown &&
		got.ManualPLMN == want.ManualPLMN &&
		got.ManualPLMNKnown == want.ManualPLMNKnown &&
		slices.Equal(got.AcquisitionOrder, want.AcquisitionOrder) &&
		got.AcquisitionOrderKnown == want.AcquisitionOrderKnown &&
		got.RegistrationRestriction == want.RegistrationRestriction &&
		got.RegistrationRestrictionKnown == want.RegistrationRestrictionKnown &&
		got.UsagePreference == want.UsagePreference &&
		got.UsagePreferenceKnown == want.UsagePreferenceKnown &&
		got.VoiceDomain == want.VoiceDomain &&
		got.VoiceDomainKnown == want.VoiceDomainKnown &&
		got.LTEDisableCause == want.LTEDisableCause &&
		got.LTEDisableCauseKnown == want.LTEDisableCauseKnown &&
		got.DisabledRATs == want.DisabledRATs &&
		got.DisabledRATsKnown == want.DisabledRATsKnown &&
		got.LTEBandsExtended == want.LTEBandsExtended &&
		got.LTEBandsExtendedKnown == want.LTEBandsExtendedKnown &&
		got.NR5GBands == want.NR5GBands &&
		got.NR5GBandsKnown == want.NR5GBandsKnown &&
		got.NR5GDisableMode == want.NR5GDisableMode &&
		got.NR5GDisableModeKnown == want.NR5GDisableModeKnown &&
		got.NR5GSABands == want.NR5GSABands &&
		got.NR5GSABandsKnown == want.NR5GSABandsKnown &&
		got.NR5GNSABands == want.NR5GNSABands &&
		got.NR5GNSABandsKnown == want.NR5GNSABandsKnown
}
