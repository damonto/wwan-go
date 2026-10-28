package qmi

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"testing"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

type radioTestCall func(qcom.Request) (qcom.Response, error)

type radioTestTransport struct {
	t     *testing.T
	calls []radioTestCall
	next  int
}

func (*radioTestTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (t *radioTestTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.t.Helper()
	if t.next >= len(t.calls) {
		t.t.Fatalf("unexpected QMI request: service=%#x message=%#x", req.Service, req.MessageID)
	}
	call := t.calls[t.next]
	t.next++
	return call(req)
}

func (t *radioTestTransport) Close() error {
	t.t.Helper()
	if t.next != len(t.calls) {
		t.t.Errorf("QMI requests = %d, want %d", t.next, len(t.calls))
	}
	return nil
}

func newRadioTestBackend(t *testing.T, calls ...radioTestCall) *Backend {
	t.Helper()
	client, err := qcom.NewClient(&radioTestTransport{t: t, calls: calls})
	if err != nil {
		t.Fatalf("qcom.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Client.Close() error = %v", err)
		}
	})
	return New(client, "/dev/test")
}

func expectRadioRequest(
	t *testing.T,
	service qcom.ServiceType,
	message qcom.MessageID,
	tlvs ...tlv.TLV,
) radioTestCall {
	t.Helper()
	return func(req qcom.Request) (qcom.Response, error) {
		if req.Service != service || req.MessageID != message {
			t.Fatalf("QMI request = service %#x message %#x, want service %#x message %#x", req.Service, req.MessageID, service, message)
		}
		return successfulStatusResponse(req, tlvs...), nil
	}
}

func deviceCapabilitiesTLV(radios ...qcom.DMSRadioInterface) tlv.TLV {
	value := make([]byte, 11, 11+len(radios))
	value[8] = byte(qcom.DMSDataServicePSOnly)
	value[9] = byte(qcom.DMSSIMSupported)
	value[10] = byte(len(radios))
	for _, radio := range radios {
		value = append(value, byte(radio))
	}
	return tlv.Bytes(0x01, value)
}

func modePreferenceTLV(preference qcom.NASModePreference) tlv.TLV {
	return tlv.Uint(0x11, uint16(preference))
}

func acquisitionOrderTLV(radios ...qcom.NASRadioInterface) tlv.TLV {
	value := make([]byte, 1, 1+len(radios))
	value[0] = byte(len(radios))
	for _, radio := range radios {
		value = append(value, byte(radio))
	}
	return tlv.Bytes(0x1C, value)
}

func TestCanonicalModeTechnology(t *testing.T) {
	tests := []struct {
		name string
		in   Technology
		want Technology
	}{
		{name: "LTE variants", in: TechnologyLTECatM | TechnologyLTENB, want: TechnologyLTE},
		{name: "NR variants", in: TechnologyNR5GNSA, want: modeNR5G},
		{
			name: "all families",
			in:   TechnologyGSM | TechnologyUMTS | TechnologyLTE | TechnologyNR5GSA,
			want: TechnologyGSM | TechnologyUMTS | TechnologyLTE | modeNR5G,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalModeTechnology(tt.in); got != tt.want {
				t.Fatalf("canonicalModeTechnology(%#x) = %#x, want %#x", tt.in, got, tt.want)
			}
		})
	}
}

func TestModeCombinations(t *testing.T) {
	all := TechnologyGSM | TechnologyUMTS | TechnologyLTE | modeNR5G
	tests := []struct {
		name          string
		acquisition   Technology
		gwAcquisition bool
		want          []Mode
		wantNot       []Mode
	}{
		{
			name: "allowed modes without acquisition order",
			want: []Mode{
				{Allowed: all},
				{Allowed: TechnologyLTE},
				{Allowed: TechnologyLTE | modeNR5G},
				{Allowed: modeNR5G},
			},
			wantNot: []Mode{{Allowed: TechnologyLTE | modeNR5G, Preferred: modeNR5G}},
		},
		{
			name:        "preferred modes with acquisition order",
			acquisition: all,
			want: []Mode{
				{Allowed: TechnologyLTE},
				{Allowed: TechnologyLTE | modeNR5G, Preferred: TechnologyLTE},
				{Allowed: TechnologyLTE | modeNR5G, Preferred: modeNR5G},
			},
			wantNot: []Mode{{Allowed: TechnologyLTE | modeNR5G}},
		},
		{
			name:          "GSM UMTS acquisition fallback",
			gwAcquisition: true,
			want: []Mode{
				{Allowed: TechnologyGSM | TechnologyUMTS},
				{Allowed: TechnologyGSM | TechnologyUMTS, Preferred: TechnologyGSM},
				{Allowed: TechnologyGSM | TechnologyUMTS, Preferred: TechnologyUMTS},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modeCombinations(all, tt.acquisition, tt.gwAcquisition)
			for _, mode := range tt.want {
				if !slices.Contains(got, mode) {
					t.Errorf("modeCombinations() = %+v, does not contain %+v", got, mode)
				}
			}
			for _, mode := range tt.wantNot {
				if slices.Contains(got, mode) {
					t.Errorf("modeCombinations() = %+v, unexpectedly contains %+v", got, mode)
				}
			}
		})
	}
}

func TestPreferredMode(t *testing.T) {
	tests := []struct {
		name       string
		preference qcom.NASSystemSelectionPreference
		allowed    Technology
		want       Technology
	}{
		{
			name: "first enabled acquisition technology",
			preference: qcom.NASSystemSelectionPreference{
				AcquisitionOrderKnown: true,
				AcquisitionOrder: []qcom.NASRadioInterface{
					qcom.NASRadioInterfaceGSM,
					qcom.NASRadioInterfaceNR5G,
					qcom.NASRadioInterfaceLTE,
				},
			},
			allowed: TechnologyLTE | modeNR5G,
			want:    modeNR5G,
		},
		{
			name: "WCDMA acquisition fallback",
			preference: qcom.NASSystemSelectionPreference{
				GWAcquisitionOrderKnown: true,
				GWAcquisitionOrder:      qcom.NASGWAcquisitionWCDMAThenGSM,
			},
			allowed: TechnologyGSM | TechnologyUMTS,
			want:    TechnologyUMTS,
		},
		{
			name: "automatic GSM UMTS acquisition",
			preference: qcom.NASSystemSelectionPreference{
				GWAcquisitionOrderKnown: true,
				GWAcquisitionOrder:      qcom.NASGWAcquisitionAutomatic,
			},
			allowed: TechnologyGSM | TechnologyUMTS,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := preferredMode(tt.preference, tt.allowed); got != tt.want {
				t.Fatalf("preferredMode() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestModePreferenceRoundTrip(t *testing.T) {
	want := TechnologyGSM | TechnologyLTE | modeNR5G
	preference := modePreferenceFromTechnology(want)
	if preference != qcom.NASModePreferenceGSM|qcom.NASModePreferenceLTE|qcom.NASModePreferenceNR5G {
		t.Fatalf("modePreferenceFromTechnology(%#x) = %#x", want, preference)
	}
	if got := technologyFromModePreference(preference); got != want {
		t.Fatalf("technologyFromModePreference(%#x) = %#x, want %#x", preference, got, want)
	}
}

func TestBackendModesIncludesCurrentPreference(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRadioRequest(t, qcom.ServiceDMS, qcom.MessageDMSGetDeviceCapabilities,
			deviceCapabilitiesTLV(qcom.DMSRadioInterfaceLTE, qcom.DMSRadioInterfaceNR5G)),
		expectRadioRequest(t, qcom.ServiceNAS, qcom.MessageNASGetSystemSelectionPreference,
			modePreferenceTLV(qcom.NASModePreferenceLTE|qcom.NASModePreferenceNR5G),
			acquisitionOrderTLV(qcom.NASRadioInterfaceNR5G, qcom.NASRadioInterfaceLTE)),
	)

	supported, current, err := backend.Modes(t.Context())
	if err != nil {
		t.Fatalf("Modes() error = %v", err)
	}
	wantCurrent := Mode{Allowed: TechnologyLTE | modeNR5G, Preferred: modeNR5G}
	if current != wantCurrent {
		t.Fatalf("Modes() current = %+v, want %+v", current, wantCurrent)
	}
	if !slices.Contains(supported, current) {
		t.Fatalf("Modes() supported = %+v, does not contain current %+v", supported, current)
	}
}

func TestBackendModesRejectsUnavailablePreference(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRadioRequest(t, qcom.ServiceDMS, qcom.MessageDMSGetDeviceCapabilities,
			deviceCapabilitiesTLV(qcom.DMSRadioInterfaceLTE, qcom.DMSRadioInterfaceNR5G)),
		func(req qcom.Request) (qcom.Response, error) {
			if req.Service != qcom.ServiceNAS || req.MessageID != qcom.MessageNASGetSystemSelectionPreference {
				t.Fatalf("unexpected QMI request: service=%#x message=%#x", req.Service, req.MessageID)
			}
			return qcom.Response{}, qcom.QMIErrorNotSupported
		},
	)

	_, _, err := backend.Modes(t.Context())
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("Modes() error = %v, want %v", err, ErrNotSupported)
	}
}

func TestBackendSetModesWritesPermanentConcretePreference(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRadioRequest(t, qcom.ServiceDMS, qcom.MessageDMSGetDeviceCapabilities,
			deviceCapabilitiesTLV(qcom.DMSRadioInterfaceLTE, qcom.DMSRadioInterfaceNR5G)),
		expectRadioRequest(t, qcom.ServiceNAS, qcom.MessageNASGetSystemSelectionPreference,
			modePreferenceTLV(qcom.NASModePreferenceLTE|qcom.NASModePreferenceNR5G),
			acquisitionOrderTLV(qcom.NASRadioInterfaceLTE, qcom.NASRadioInterfaceNR5G)),
		func(req qcom.Request) (qcom.Response, error) {
			if req.Service != qcom.ServiceNAS || req.MessageID != qcom.MessageNASSetSystemSelectionPreference {
				t.Fatalf("unexpected QMI request: service=%#x message=%#x", req.Service, req.MessageID)
			}
			mode, ok := tlv.Value(req.TLVs, 0x11)
			wantMode := qcom.NASModePreferenceLTE | qcom.NASModePreferenceNR5G
			if !ok || len(mode) != 2 || qcom.NASModePreference(binary.LittleEndian.Uint16(mode)) != wantMode {
				t.Fatalf("mode preference TLV = % x", mode)
			}
			order, ok := tlv.Value(req.TLVs, 0x1E)
			wantOrder := []byte{2, byte(qcom.NASRadioInterfaceNR5G), byte(qcom.NASRadioInterfaceLTE)}
			if !ok || !slices.Equal(order, wantOrder) {
				t.Fatalf("acquisition order TLV = % x, want % x", order, wantOrder)
			}
			duration, ok := tlv.Value(req.TLVs, 0x17)
			if !ok || len(duration) != 1 || qcom.NASChangeDuration(duration[0]) != qcom.NASChangePermanent {
				t.Fatalf("change duration TLV = % x", duration)
			}
			return successfulStatusResponse(req), nil
		},
	)

	if err := backend.SetModes(t.Context(), Mode{Allowed: TechnologyLTE | modeNR5G, Preferred: modeNR5G}); err != nil {
		t.Fatalf("SetModes() error = %v", err)
	}
}

func TestModeSelectionConfigUsesGWAcquisitionOrder(t *testing.T) {
	tests := []struct {
		name      string
		preferred Technology
		want      qcom.NASGWAcquisitionOrder
	}{
		{name: "automatic", want: qcom.NASGWAcquisitionAutomatic},
		{name: "GSM first", preferred: TechnologyGSM, want: qcom.NASGWAcquisitionGSMThenWCDMA},
		{name: "UMTS first", preferred: TechnologyUMTS, want: qcom.NASGWAcquisitionWCDMAThenGSM},
	}
	preference := qcom.NASSystemSelectionPreference{GWAcquisitionOrderKnown: true}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := modeSelectionConfig(Mode{
				Allowed:   TechnologyGSM | TechnologyUMTS,
				Preferred: tt.preferred,
			}, preference)
			if err != nil {
				t.Fatalf("modeSelectionConfig() error = %v", err)
			}
			if config.GWAcquisitionOrder == nil || *config.GWAcquisitionOrder != tt.want {
				t.Fatalf("modeSelectionConfig() GW order = %v, want %d", config.GWAcquisitionOrder, tt.want)
			}
		})
	}
}

func TestModeSelectionConfigKeepsAcquisitionOrdersAligned(t *testing.T) {
	tests := []struct {
		name              string
		preferred         Technology
		wantAcquisition   []qcom.NASRadioInterface
		wantGWAcquisition qcom.NASGWAcquisitionOrder
	}{
		{
			name:              "GSM first",
			preferred:         TechnologyGSM,
			wantAcquisition:   []qcom.NASRadioInterface{qcom.NASRadioInterfaceGSM, qcom.NASRadioInterfaceLTE, qcom.NASRadioInterfaceUMTS},
			wantGWAcquisition: qcom.NASGWAcquisitionGSMThenWCDMA,
		},
		{
			name:              "LTE first keeps GSM WCDMA automatic",
			preferred:         TechnologyLTE,
			wantAcquisition:   []qcom.NASRadioInterface{qcom.NASRadioInterfaceLTE, qcom.NASRadioInterfaceUMTS, qcom.NASRadioInterfaceGSM},
			wantGWAcquisition: qcom.NASGWAcquisitionAutomatic,
		},
	}
	preference := qcom.NASSystemSelectionPreference{
		AcquisitionOrderKnown: true,
		AcquisitionOrder: []qcom.NASRadioInterface{
			qcom.NASRadioInterfaceLTE,
			qcom.NASRadioInterfaceUMTS,
			qcom.NASRadioInterfaceGSM,
		},
		GWAcquisitionOrderKnown: true,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := modeSelectionConfig(Mode{
				Allowed:   TechnologyGSM | TechnologyUMTS | TechnologyLTE,
				Preferred: tt.preferred,
			}, preference)
			if err != nil {
				t.Fatalf("modeSelectionConfig() error = %v", err)
			}
			if !slices.Equal(config.AcquisitionOrder, tt.wantAcquisition) {
				t.Fatalf("modeSelectionConfig() acquisition order = %v, want %v", config.AcquisitionOrder, tt.wantAcquisition)
			}
			if config.GWAcquisitionOrder == nil || *config.GWAcquisitionOrder != tt.wantGWAcquisition {
				t.Fatalf("modeSelectionConfig() GW order = %v, want %d", config.GWAcquisitionOrder, tt.wantGWAcquisition)
			}
		})
	}
}

func TestBackendSetModesRejectsInvalidPreferredMode(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
	}{
		{
			name: "preferred outside allowed",
			mode: Mode{Allowed: TechnologyLTE, Preferred: modeNR5G},
		},
		{
			name: "multiple preferred families",
			mode: Mode{Allowed: TechnologyLTE | modeNR5G, Preferred: TechnologyLTE | modeNR5G},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := new(Backend).SetModes(t.Context(), tt.mode); err == nil {
				t.Fatal("SetModes() error = nil, want non-nil")
			}
		})
	}
}
