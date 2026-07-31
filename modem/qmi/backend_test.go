package qmi

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestQMIErrorClassification(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		wantUnsupported    bool
		wantSARUnsupported bool
		wantNoSignal       bool
	}{
		{name: "not supported", err: fmt.Errorf("query: %w", qcom.QMIErrorNotSupported), wantUnsupported: true, wantSARUnsupported: true},
		{name: "invalid command", err: qcom.QMIErrorInvalidQmiCommand, wantUnsupported: true, wantSARUnsupported: true},
		{name: "SAR no memory", err: qcom.QMIErrorNoMemory, wantSARUnsupported: true},
		{name: "signal unavailable", err: qcom.QMIErrorInformationUnavailable, wantNoSignal: true},
		{name: "transport", err: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := qmiUnsupported(test.err); got != test.wantUnsupported {
				t.Errorf("qmiUnsupported() = %t, want %t", got, test.wantUnsupported)
			}
			if got := qmiSARUnsupported(test.err); got != test.wantSARUnsupported {
				t.Errorf("qmiSARUnsupported() = %t, want %t", got, test.wantSARUnsupported)
			}
			if got := qmiSignalUnavailable(test.err); got != test.wantNoSignal {
				t.Errorf("qmiSignalUnavailable() = %t, want %t", got, test.wantNoSignal)
			}
		})
	}
}

func TestApplyQMICellLocation(t *testing.T) {
	tests := []struct {
		name     string
		location qcom.NASCellLocationInfo
		want     uint32
	}{
		{name: "unknown frequency keeps existing value", location: qcom.NASCellLocationInfo{LTEIntraEARFCN: 1800}, want: 900},
		{name: "known LTE frequency updates ARFCN", location: qcom.NASCellLocationInfo{LTEIntraEARFCN: 1800, LTEIntraEARFCNKnown: true}, want: 1800},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cell := CellInfo{ARFCN: 900}
			applyQMICellLocation(&cell, tt.location)
			if cell.ARFCN != tt.want {
				t.Errorf("CellInfo.ARFCN = %d, want %d", cell.ARFCN, tt.want)
			}
		})
	}
}

type capabilityTransport struct {
	requests []qcom.Request
}

func (t *capabilityTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.requests = append(t.requests, req)
	return qcom.Response{
		Service: req.Service, ClientID: req.ClientID, TransactionID: req.TransactionID, MessageID: req.MessageID,
		TLVs: tlv.TLVs{tlv.Bytes(0x02, []byte{0, 0, 0, 0})},
	}, nil
}

func (*capabilityTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*capabilityTransport) Close() error { return nil }

func TestNetworkConfig(t *testing.T) {
	tests := []struct {
		name string
		info qcom.PDNInfo
		want NetworkConfig
	}{
		{
			name: "dual stack",
			info: qcom.PDNInfo{
				LocalIPv4: net.ParseIP("192.0.2.2"), IPv4SubnetMask: net.IPv4(255, 255, 255, 0),
				IPv4Gateway: net.ParseIP("192.0.2.1"), LocalIPv6: net.ParseIP("2001:db8::2"),
				IPv6PrefixLength: 64, IPv6Gateway: net.ParseIP("2001:db8::1"),
				DNS: []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2001:4860:4860::8888")}, MTU: 1500,
			},
			want: NetworkConfig{Interface: "wwan0", Addresses: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.2/24"), netip.MustParsePrefix("2001:db8::2/64"),
			}, Gateways: []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")},
				DNS: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2001:4860:4860::8888")}, MTU: 1500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qmiNetworkConfig("wwan0", tt.info); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("qmiNetworkConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeSIMIdentity(t *testing.T) {
	tests := []struct {
		name   string
		result SIMInfo
		want   SIMInfo
	}{
		{
			name: "uses card values when DMS is unavailable",
			want: SIMInfo{ICCID: "8986001234567890123", IMSI: "460001234567890"},
		},
		{
			name:   "preserves DMS values",
			result: SIMInfo{ICCID: "8986000000000000000", IMSI: "460000000000000"},
			want:   SIMInfo{ICCID: "8986000000000000000", IMSI: "460000000000000"},
		},
	}
	card := SIMInfo{ICCID: "8986001234567890123", IMSI: "460001234567890"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeSIMIdentity(&tt.result, card)
			if tt.result.ICCID != tt.want.ICCID || tt.result.IMSI != tt.want.IMSI {
				t.Fatalf("SIM identity = %+v, want %+v", tt.result, tt.want)
			}
		})
	}
}

func TestQMIIPPreferences(t *testing.T) {
	tests := []struct {
		name   string
		family IPFamily
		want   []qcom.WDSIPPreference
	}{
		{name: "default", family: IPFamilyUnknown, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceDefault}},
		{name: "IPv4", family: IPFamilyIPv4, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4}},
		{name: "IPv6", family: IPFamilyIPv6, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv6}},
		{name: "dual stack", family: IPFamilyIPv4v6, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4, qcom.WDSIPPreferenceIPv6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qmiIPPreferences(tt.family); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("qmiIPPreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeQMINetworkConfigs(t *testing.T) {
	tests := []struct {
		name  string
		infos []qcom.PDNInfo
		want  NetworkConfig
	}{
		{
			name: "combines families and uses the smaller MTU",
			infos: []qcom.PDNInfo{
				{
					LocalIPv4: net.ParseIP("192.0.2.2"), IPv4SubnetMask: net.IPv4(255, 255, 255, 0),
					IPv4Gateway: net.ParseIP("192.0.2.1"), DNS: []net.IP{net.ParseIP("1.1.1.1")}, MTU: 1500,
				},
				{
					LocalIPv6: net.ParseIP("2001:db8::2"), IPv6PrefixLength: 64,
					IPv6Gateway: net.ParseIP("2001:db8::1"), DNS: []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2001:4860:4860::8888")}, MTU: 1420,
				},
			},
			want: NetworkConfig{
				Interface: "wwan0",
				Addresses: []netip.Prefix{netip.MustParsePrefix("192.0.2.2/24"), netip.MustParsePrefix("2001:db8::2/64")},
				Gateways:  []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")},
				DNS:       []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2001:4860:4860::8888")},
				MTU:       1420,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeQMINetworkConfigs("wwan0", tt.infos); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeQMINetworkConfigs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestQMIFeatures(t *testing.T) {
	tests := []struct {
		name     string
		services []qcom.ServiceVersion
		want     Feature
	}{
		{name: "none"},
		{
			name: "advertised services only",
			services: []qcom.ServiceVersion{
				{Service: qcom.ServiceDMS}, {Service: qcom.ServiceWDS}, {Service: qcom.ServiceWMS},
				{Service: qcom.ServiceVoice}, {Service: qcom.ServiceSAR}, {Service: qcom.ServiceNAS},
			},
			want: FeatureFirmwareUpdate | FeatureFacilityLocks | FeatureProfileManagement |
				FeatureInitialEPSBearer | FeatureSMS | FeatureUSSD | FeatureSAR |
				FeatureSignalThresholds | FeatureCellInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qmiFeatures(tt.services); got != tt.want {
				t.Errorf("qmiFeatures() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestSetCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		technologies Technology
		wantErr      bool
		wantMessages []qcom.MessageID
	}{
		{
			name:         "sets a permanent preference then resets",
			technologies: TechnologyLTE | TechnologyNR5GSA,
			wantMessages: []qcom.MessageID{qcom.MessageNASSetSystemSelectionPreference, qcom.MessageDMSReset},
		},
		{name: "rejects empty capabilities", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := new(capabilityTransport)
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")
			err = backend.SetCapabilities(context.Background(), tt.technologies)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetCapabilities() error = %v, wantErr %t", err, tt.wantErr)
			}
			var messages []qcom.MessageID
			for _, request := range transport.requests {
				messages = append(messages, request.MessageID)
			}
			if !reflect.DeepEqual(messages, tt.wantMessages) {
				t.Errorf("SetCapabilities() messages = %v, want %v", messages, tt.wantMessages)
			}
			if !tt.wantErr {
				if _, ok := transport.requests[0].TLVs.Find(0x17); !ok {
					t.Error("SetCapabilities() omitted permanent change duration")
				}
			}
		})
	}
}

func TestAPNTypeMapping(t *testing.T) {
	tests := []struct {
		name  string
		value APNType
		want  qcom.WDSAPNTypeMask
	}{
		{name: "default", value: APNTypeDefault, want: qcom.WDSAPNTypeDefault},
		{name: "IMS", value: APNTypeIMS, want: qcom.WDSAPNTypeIMS},
		{name: "MMS", value: APNTypeMMS, want: qcom.WDSAPNTypeMMS},
		{name: "tethering", value: APNTypeTethering, want: qcom.WDSAPNTypeDUN},
		{name: "SUPL", value: APNTypeSUPL, want: qcom.WDSAPNTypeSUPL},
		{name: "emergency", value: APNTypeEmergency, want: qcom.WDSAPNTypeEmergency},
		{name: "mask", value: APNTypeDefault | APNTypeIMS, want: qcom.WDSAPNTypeDefault | qcom.WDSAPNTypeIMS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := apnTypeToQMI(tt.value)
			if value != tt.want {
				t.Errorf("apnTypeToQMI() = %#x, want %#x", value, tt.want)
			}
			if got := qmiAPNType(value); got != tt.value {
				t.Errorf("qmiAPNType() = %#x, want %#x", got, tt.value)
			}
		})
	}
}

func TestSignalReportConfigs(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantRate uint8
		wantNil  bool
		wantErr  bool
	}{
		{name: "disabled", wantNil: true},
		{name: "subsecond rounds up", interval: time.Nanosecond, wantRate: 1},
		{name: "one second", interval: time.Second, wantRate: 1},
		{name: "fractional seconds round up", interval: time.Second + time.Nanosecond, wantRate: 2},
		{name: "maximum", interval: 5 * time.Second, wantRate: 5},
		{name: "over maximum", interval: 5*time.Second + time.Nanosecond, wantErr: true},
		{name: "negative", interval: -time.Second, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lte, nr5g, err := qmiSignalReportConfigs(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Fatalf("qmiSignalReportConfigs() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNil {
				if lte != nil || nr5g != nil {
					t.Fatalf("qmiSignalReportConfigs() = (%v, %v), want nil", lte, nr5g)
				}
				return
			}
			if lte == nil || nr5g == nil || uint8(lte.Rate) != tt.wantRate || uint8(nr5g.Rate) != tt.wantRate {
				t.Errorf("qmiSignalReportConfigs() = (%v, %v), want rate %d", lte, nr5g, tt.wantRate)
			}
		})
	}
}
