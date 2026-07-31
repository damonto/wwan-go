package qcom

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSProfileCompleteRequests(t *testing.T) {
	cfg := completeWDSProfileConfigForTest()
	tests := []struct {
		name       string
		build      func() (Request, error)
		want       map[byte][]byte
		wantAbsent []byte
	}{
		{
			name: "create",
			build: func() (Request, error) {
				return (WDSCreateProfileRequest{ClientID: 7, Config: cfg}).Request()
			},
			want:       completeWDSProfileTLVsForTest(t, cfg, false),
			wantAbsent: []byte{wdsTLVProfileCLATEnabled, wdsTLVProfileIPv6PrefixDelegation},
		},
		{
			name: "modify",
			build: func() (Request, error) {
				return (WDSUpdateProfileRequest{
					ClientID: 7,
					Profile:  WDSProfileID{Type: cfg.Type, Index: 9},
					Update:   completeWDSProfileUpdateForTest(cfg),
				}).Request()
			},
			want: completeWDSProfileTLVsForTest(t, cfg, true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.build()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if len(req.TLVs) != len(tt.want) {
				t.Fatalf("TLV count = %d, want %d: %+v", len(req.TLVs), len(tt.want), req.TLVs)
			}
			for kind, want := range tt.want {
				assertTLV(t, req.TLVs, kind, want)
			}
			for _, kind := range tt.wantAbsent {
				if _, ok := tlv.Value(req.TLVs, kind); ok {
					t.Errorf("TLV 0x%02X is present, want absent", kind)
				}
			}
		})
	}
}

func TestWDSProfileSettingsComplete(t *testing.T) {
	cfg := completeWDSProfileConfigForTest()
	values := completeWDSProfileTLVsForTest(t, cfg, true)
	delete(values, wdsTLVProfileID)

	tests := []struct {
		name string
		tlvs tlv.TLVs
		want WDSProfileSettings
	}{
		{
			name: "all libqmi fields",
			tlvs: profileTLVsForTest(values),
			want: completeWDSProfileSettingsForTest(cfg),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WDSGetProfileSettingsResponse{
				Settings: WDSProfileSettings{ID: WDSProfileID{Type: cfg.Type, Index: 9}},
			}
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Settings != tt.want {
				t.Fatalf("settings = %+v, want %+v", got.Settings, tt.want)
			}
		})
	}
}

func TestWDSDefaultSettingsComplete(t *testing.T) {
	cfg := completeWDSProfileConfigForTest()
	values := completeWDSProfileTLVsForTest(t, cfg, false)
	delete(values, wdsTLVProfileID)
	delete(values, wdsTLVProfileRoamingDisallowed)
	delete(values, wdsTLVProfileAPNType)

	want := completeWDSProfileSettingsForTest(cfg)
	want.ID.Index = 0
	want.RoamingDisallowed = false
	want.RoamingDisallowedKnown = false
	want.APNType = 0
	want.APNTypeKnown = false
	want.CLATEnabled = false
	want.CLATEnabledKnown = false
	want.IPv6PrefixDelegation = false
	want.IPv6PrefixDelegationKnown = false

	tests := []struct {
		name string
		tlvs tlv.TLVs
		want WDSProfileSettings
	}{
		{name: "all libqmi default fields", tlvs: profileTLVsForTest(values), want: want},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WDSGetDefaultSettingsResponse{
				Settings: WDSProfileSettings{ID: WDSProfileID{Type: cfg.Type}},
			}
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Settings != tt.want {
				t.Fatalf("settings = %+v, want %+v", got.Settings, tt.want)
			}
		})
	}
}

func TestWDSProfileCompositeEncoding(t *testing.T) {
	cfg := completeWDSProfileConfigForTest()
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			name: "UMTS QoS",
			got:  mustMarshalBinary(t, *cfg.UMTSRequestedQoS),
			want: []byte{
				1,
				2, 0, 0, 0,
				3, 0, 0, 0,
				4, 0, 0, 0,
				5, 0, 0, 0,
				6,
				7, 0, 0, 0,
				8, 9, 10,
				11, 0, 0, 0,
				12, 0, 0, 0,
			},
		},
		{
			name: "GPRS QoS",
			got:  mustMarshalBinary(t, *cfg.GPRSRequestedQoS),
			want: []byte{
				13, 0, 0, 0,
				14, 0, 0, 0,
				15, 0, 0, 0,
				16, 0, 0, 0,
				17, 0, 0, 0,
			},
		},
		{
			name: "UMTS QoS with signaling",
			got:  mustMarshalBinary(t, *cfg.UMTSMinimumQoSWithSignaling),
			want: append([]byte{
				1,
				2, 0, 0, 0,
				3, 0, 0, 0,
				4, 0, 0, 0,
				5, 0, 0, 0,
				6,
				7, 0, 0, 0,
				8, 9, 10,
				11, 0, 0, 0,
				12, 0, 0, 0,
			}, 0xFF),
		},
		{
			name: "LTE QoS",
			got:  mustMarshalBinary(t, *cfg.LTEQoS),
			want: []byte{
				5,
				18, 0, 0, 0,
				19, 0, 0, 0,
				20, 0, 0, 0,
				21, 0, 0, 0,
			},
		},
		{name: "VLAN range", got: mustMarshalBinary(t, *cfg.VLAN), want: []byte{100, 0, 200, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("encoding = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestWDSProfileSettingsRejectMalformedFixedTLVs(t *testing.T) {
	tests := []struct {
		name string
		kind byte
		size int
	}{
		{name: "PDP type", kind: wdsTLVProfilePDPType, size: 1},
		{name: "header compression", kind: wdsTLVProfileHeaderCompression, size: 1},
		{name: "data compression", kind: wdsTLVProfileDataCompression, size: 1},
		{name: "primary IPv4 DNS", kind: wdsTLVProfilePrimaryIPv4DNS, size: 4},
		{name: "secondary IPv4 DNS", kind: wdsTLVProfileSecondaryIPv4DNS, size: 4},
		{name: "UMTS requested QoS", kind: wdsTLVProfileUMTSRequestedQoS, size: 33},
		{name: "UMTS minimum QoS", kind: wdsTLVProfileUMTSMinimumQoS, size: 33},
		{name: "GPRS requested QoS", kind: wdsTLVProfileGPRSRequestedQoS, size: 20},
		{name: "GPRS minimum QoS", kind: wdsTLVProfileGPRSMinimumQoS, size: 20},
		{name: "authentication", kind: wdsTLVProfileAuth, size: 1},
		{name: "IPv4 preference", kind: wdsTLVProfileIPv4Preference, size: 4},
		{name: "P-CSCF using PCO", kind: wdsTLVPCSCFUsingPCO, size: 1},
		{name: "PDP access control", kind: wdsTLVProfilePDPAccessControl, size: 1},
		{name: "P-CSCF using DHCP", kind: wdsTLVPCSCFUsingDHCP, size: 1},
		{name: "IMCN", kind: wdsTLVIMCNFlag, size: 1},
		{name: "PDP context number", kind: wdsTLVProfilePDPContextNumber, size: 1},
		{name: "PDP context secondary", kind: wdsTLVProfilePDPContextSecondary, size: 1},
		{name: "PDP context primary ID", kind: wdsTLVProfilePDPContextPrimaryID, size: 1},
		{name: "IPv6 preference", kind: wdsTLVProfileIPv6Preference, size: 16},
		{name: "UMTS requested QoS signaling", kind: wdsTLVProfileUMTSRequestedQoSSig, size: 34},
		{name: "UMTS minimum QoS signaling", kind: wdsTLVProfileUMTSMinimumQoSSig, size: 34},
		{name: "primary IPv6 DNS", kind: wdsTLVProfilePrimaryIPv6DNS, size: 16},
		{name: "secondary IPv6 DNS", kind: wdsTLVProfileSecondaryIPv6DNS, size: 16},
		{name: "address allocation", kind: wdsTLVProfileAddressAllocation, size: 1},
		{name: "LTE QoS", kind: wdsTLVProfileLTEQoS, size: 17},
		{name: "APN disabled", kind: wdsTLVProfileAPNDisabled, size: 1},
		{name: "roaming disallowed", kind: wdsTLVProfileRoamingDisallowed, size: 1},
		{name: "VLAN", kind: wdsTLVProfileVLAN, size: 4},
		{name: "APN type", kind: wdsTLVProfileAPNType, size: 8},
		{name: "CLAT enabled", kind: wdsTLVProfileCLATEnabled, size: 1},
		{name: "IPv6 prefix delegation", kind: wdsTLVProfileIPv6PrefixDelegation, size: 1},
	}

	for _, tt := range tests {
		for _, malformed := range []struct {
			name string
			size int
		}{
			{name: "truncated", size: tt.size - 1},
			{name: "trailing data", size: tt.size + 1},
		} {
			t.Run(tt.name+"/"+malformed.name, func(t *testing.T) {
				var got WDSGetProfileSettingsResponse
				err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(tt.kind, make([]byte, malformed.size))})
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
			})
		}
	}
}

func TestWDSProfileRequestValidation(t *testing.T) {
	invalidIPv4 := netip.MustParseAddr("2001:db8::1")
	invalidIPv6 := netip.MustParseAddr("192.0.2.1")
	tests := []struct {
		name  string
		build func() error
		want  string
	}{
		{name: "profile type", build: func() error {
			_, err := (WDSCreateProfileRequest{Config: WDSProfileConfig{Type: 3}}).Request()
			return err
		}, want: "profile type"},
		{name: "header compression", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{HeaderCompression: ptr(WDSPDPHeaderCompression(5))}}).Request()
			return err
		}, want: "header compression"},
		{name: "data compression", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{DataCompression: ptr(WDSPDPDataCompression(4))}}).Request()
			return err
		}, want: "data compression"},
		{name: "IPv4 family", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{PrimaryIPv4DNS: &invalidIPv4}}).Request()
			return err
		}, want: "not IPv4"},
		{name: "IPv6 family", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{PrimaryIPv6DNS: &invalidIPv6}}).Request()
			return err
		}, want: "not IPv6"},
		{name: "PDP access", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{PDPAccessControl: ptr(WDSPDPAccessControl(3))}}).Request()
			return err
		}, want: "PDP access"},
		{name: "address allocation", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{AddressAllocationPreference: ptr(WDSAddressAllocationPreference(2))}}).Request()
			return err
		}, want: "address allocation"},
		{name: "LTE QoS class", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{LTEQoS: &WDSLTEQoS{ClassIdentifier: 9}}}).Request()
			return err
		}, want: "LTE QoS class"},
		{name: "VLAN order", build: func() error {
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{VLAN: &WDSVLANRange{Start: 20, End: 10}}}).Request()
			return err
		}, want: "VLAN range"},
		{name: "APN type mask", build: func() error {
			mask := WDSAPNTypeMask(1 << 12)
			_, err := (WDSUpdateProfileRequest{Profile: WDSProfileID{}, Update: WDSProfileUpdate{APNType: &mask}}).Request()
			return err
		}, want: "APN type mask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Request() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func completeWDSProfileConfigForTest() WDSProfileConfig {
	umts := WDSUMTSGrantedQoS{
		TrafficClass:              1,
		MaximumUplinkBitrate:      2,
		MaximumDownlinkBitrate:    3,
		GuaranteedUplinkBitrate:   4,
		GuaranteedDownlinkBitrate: 5,
		DeliveryOrder:             6,
		MaximumSDUSize:            7,
		SDUErrorRatio:             8,
		ResidualBitErrorRatio:     9,
		ErroneousSDUDelivery:      10,
		TransferDelay:             11,
		TrafficHandlingPriority:   12,
	}
	gprs := WDSGPRSGrantedQoS{
		PrecedenceClass:     13,
		DelayClass:          14,
		ReliabilityClass:    15,
		PeakThroughputClass: 16,
		MeanThroughputClass: 17,
	}
	return WDSProfileConfig{
		Type:                          WDSProfileTypeEPC,
		Name:                          "carrier-data",
		APN:                           "internet",
		PDPType:                       WDSPDPTypeIPv4v6,
		Username:                      "subscriber",
		Password:                      "secret",
		Authentication:                WDSAuthenticationPAP | WDSAuthenticationCHAP,
		HeaderCompression:             ptr(WDSPDPHeaderCompressionRFC2507),
		DataCompression:               ptr(WDSPDPDataCompressionV42bis),
		PrimaryIPv4DNS:                netip.MustParseAddr("192.0.2.1"),
		SecondaryIPv4DNS:              netip.MustParseAddr("198.51.100.2"),
		UMTSRequestedQoS:              &umts,
		UMTSMinimumQoS:                &umts,
		GPRSRequestedQoS:              &gprs,
		GPRSMinimumQoS:                &gprs,
		IPv4AddressPreference:         netip.MustParseAddr("203.0.113.3"),
		PCSCFUsingPCO:                 ptr(true),
		PDPAccessControl:              ptr(WDSPDPAccessControlPermission),
		PCSCFUsingDHCP:                ptr(false),
		IMCN:                          ptr(true),
		PDPContextNumber:              ptr(uint8(4)),
		PDPContextSecondary:           ptr(true),
		PDPContextPrimaryID:           ptr(uint8(5)),
		IPv6AddressPreference:         netip.MustParseAddr("2001:db8::1"),
		UMTSRequestedQoSWithSignaling: &WDSUMTSQoSWithSignaling{QoS: umts, SignalingIndication: 1},
		UMTSMinimumQoSWithSignaling:   &WDSUMTSQoSWithSignaling{QoS: umts, SignalingIndication: -1},
		PrimaryIPv6DNS:                netip.MustParseAddr("2001:db8::53"),
		SecondaryIPv6DNS:              netip.MustParseAddr("2001:db8::54"),
		AddressAllocationPreference:   ptr(WDSAddressAllocationDHCP),
		LTEQoS:                        &WDSLTEQoS{ClassIdentifier: WDSQoSClassNonGuaranteedBitrate5, GuaranteedDownlinkBitrate: 18, MaximumDownlinkBitrate: 19, GuaranteedUplinkBitrate: 20, MaximumUplinkBitrate: 21},
		APNDisabled:                   ptr(false),
		RoamingDisallowed:             ptr(true),
		VLAN:                          &WDSVLANRange{Start: 100, End: 200},
		APNType:                       ptr(WDSAPNTypeDefault | WDSAPNTypeMMS),
	}
}

func completeWDSProfileUpdateForTest(cfg WDSProfileConfig) WDSProfileUpdate {
	update := wdsProfileUpdateFromConfig(cfg)
	update.CLATEnabled = ptr(true)
	update.IPv6PrefixDelegation = ptr(false)
	return update
}

func completeWDSProfileTLVsForTest(t *testing.T, cfg WDSProfileConfig, modify bool) map[byte][]byte {
	t.Helper()
	want := map[byte][]byte{
		wdsTLVProfileID:                  {byte(cfg.Type)},
		wdsTLVProfileName:                []byte(cfg.Name),
		wdsTLVProfilePDPType:             {byte(cfg.PDPType)},
		wdsTLVProfileHeaderCompression:   {byte(*cfg.HeaderCompression)},
		wdsTLVProfileDataCompression:     {byte(*cfg.DataCompression)},
		wdsTLVProfileAPN:                 []byte(cfg.APN),
		wdsTLVProfilePrimaryIPv4DNS:      {1, 2, 0, 192},
		wdsTLVProfileSecondaryIPv4DNS:    {2, 100, 51, 198},
		wdsTLVProfileUMTSRequestedQoS:    mustMarshalBinary(t, *cfg.UMTSRequestedQoS),
		wdsTLVProfileUMTSMinimumQoS:      mustMarshalBinary(t, *cfg.UMTSMinimumQoS),
		wdsTLVProfileGPRSRequestedQoS:    mustMarshalBinary(t, *cfg.GPRSRequestedQoS),
		wdsTLVProfileGPRSMinimumQoS:      mustMarshalBinary(t, *cfg.GPRSMinimumQoS),
		wdsTLVProfileUsername:            []byte(cfg.Username),
		wdsTLVProfilePassword:            []byte(cfg.Password),
		wdsTLVProfileAuth:                {byte(cfg.Authentication)},
		wdsTLVProfileIPv4Preference:      {3, 113, 0, 203},
		wdsTLVPCSCFUsingPCO:              {1},
		wdsTLVProfilePDPAccessControl:    {byte(*cfg.PDPAccessControl)},
		wdsTLVPCSCFUsingDHCP:             {0},
		wdsTLVIMCNFlag:                   {1},
		wdsTLVProfilePDPContextNumber:    {*cfg.PDPContextNumber},
		wdsTLVProfilePDPContextSecondary: {1},
		wdsTLVProfilePDPContextPrimaryID: {*cfg.PDPContextPrimaryID},
		wdsTLVProfileIPv6Preference:      cfg.IPv6AddressPreference.AsSlice(),
		wdsTLVProfileUMTSRequestedQoSSig: mustMarshalBinary(t, *cfg.UMTSRequestedQoSWithSignaling),
		wdsTLVProfileUMTSMinimumQoSSig:   mustMarshalBinary(t, *cfg.UMTSMinimumQoSWithSignaling),
		wdsTLVProfilePrimaryIPv6DNS:      cfg.PrimaryIPv6DNS.AsSlice(),
		wdsTLVProfileSecondaryIPv6DNS:    cfg.SecondaryIPv6DNS.AsSlice(),
		wdsTLVProfileAddressAllocation:   {byte(*cfg.AddressAllocationPreference)},
		wdsTLVProfileLTEQoS:              mustMarshalBinary(t, *cfg.LTEQoS),
		wdsTLVProfileAPNDisabled:         {0},
		wdsTLVProfileRoamingDisallowed:   {1},
		wdsTLVProfileVLAN:                binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, cfg.VLAN.Start), cfg.VLAN.End),
		wdsTLVProfileAPNType:             binary.LittleEndian.AppendUint64(nil, uint64(*cfg.APNType)),
	}
	if modify {
		want[wdsTLVProfileID] = []byte{byte(cfg.Type), 9}
		want[wdsTLVProfileCLATEnabled] = []byte{1}
		want[wdsTLVProfileIPv6PrefixDelegation] = []byte{0}
	}
	return want
}

func mustMarshalBinary(t *testing.T, value encoding.BinaryMarshaler) []byte {
	t.Helper()
	encoded, err := value.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	return encoded
}

func completeWDSProfileSettingsForTest(cfg WDSProfileConfig) WDSProfileSettings {
	return WDSProfileSettings{
		ID:                                 WDSProfileID{Type: cfg.Type, Index: 9},
		Name:                               cfg.Name,
		NameKnown:                          true,
		APN:                                cfg.APN,
		APNKnown:                           true,
		PDPType:                            cfg.PDPType,
		PDPKnown:                           true,
		Username:                           cfg.Username,
		UsernameKnown:                      true,
		Password:                           cfg.Password,
		PasswordKnown:                      true,
		Authentication:                     cfg.Authentication,
		AuthenticationKnown:                true,
		HeaderCompression:                  *cfg.HeaderCompression,
		HeaderCompressionKnown:             true,
		DataCompression:                    *cfg.DataCompression,
		DataCompressionKnown:               true,
		PrimaryIPv4DNS:                     cfg.PrimaryIPv4DNS,
		PrimaryIPv4DNSKnown:                true,
		SecondaryIPv4DNS:                   cfg.SecondaryIPv4DNS,
		SecondaryIPv4DNSKnown:              true,
		UMTSRequestedQoS:                   *cfg.UMTSRequestedQoS,
		UMTSRequestedQoSKnown:              true,
		UMTSMinimumQoS:                     *cfg.UMTSMinimumQoS,
		UMTSMinimumQoSKnown:                true,
		GPRSRequestedQoS:                   *cfg.GPRSRequestedQoS,
		GPRSRequestedQoSKnown:              true,
		GPRSMinimumQoS:                     *cfg.GPRSMinimumQoS,
		GPRSMinimumQoSKnown:                true,
		IPv4AddressPreference:              cfg.IPv4AddressPreference,
		IPv4AddressPreferenceKnown:         true,
		PCSCFUsingPCO:                      *cfg.PCSCFUsingPCO,
		PCSCFUsingPCOKnown:                 true,
		PDPAccessControl:                   *cfg.PDPAccessControl,
		PDPAccessControlKnown:              true,
		PCSCFUsingDHCP:                     *cfg.PCSCFUsingDHCP,
		PCSCFUsingDHCPKnown:                true,
		IMCN:                               *cfg.IMCN,
		IMCNKnown:                          true,
		PDPContextNumber:                   *cfg.PDPContextNumber,
		PDPContextNumberKnown:              true,
		PDPContextSecondary:                *cfg.PDPContextSecondary,
		PDPContextSecondaryKnown:           true,
		PDPContextPrimaryID:                *cfg.PDPContextPrimaryID,
		PDPContextPrimaryIDKnown:           true,
		IPv6AddressPreference:              cfg.IPv6AddressPreference,
		IPv6AddressPreferenceKnown:         true,
		UMTSRequestedQoSWithSignaling:      *cfg.UMTSRequestedQoSWithSignaling,
		UMTSRequestedQoSWithSignalingKnown: true,
		UMTSMinimumQoSWithSignaling:        *cfg.UMTSMinimumQoSWithSignaling,
		UMTSMinimumQoSWithSignalingKnown:   true,
		PrimaryIPv6DNS:                     cfg.PrimaryIPv6DNS,
		PrimaryIPv6DNSKnown:                true,
		SecondaryIPv6DNS:                   cfg.SecondaryIPv6DNS,
		SecondaryIPv6DNSKnown:              true,
		AddressAllocationPreference:        *cfg.AddressAllocationPreference,
		AddressAllocationPreferenceKnown:   true,
		LTEQoS:                             *cfg.LTEQoS,
		LTEQoSKnown:                        true,
		APNDisabled:                        *cfg.APNDisabled,
		APNDisabledKnown:                   true,
		RoamingDisallowed:                  *cfg.RoamingDisallowed,
		RoamingDisallowedKnown:             true,
		VLAN:                               *cfg.VLAN,
		VLANKnown:                          true,
		APNType:                            *cfg.APNType,
		APNTypeKnown:                       true,
		CLATEnabled:                        true,
		CLATEnabledKnown:                   true,
		IPv6PrefixDelegation:               false,
		IPv6PrefixDelegationKnown:          true,
	}
}

func profileTLVsForTest(values map[byte][]byte) tlv.TLVs {
	tlvs := make(tlv.TLVs, 0, len(values))
	for kind, value := range values {
		tlvs = append(tlvs, tlv.Bytes(kind, value))
	}
	return tlvs
}
