package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASAttachDetach(t *testing.T) {
	tests := []struct {
		name   string
		action NASPSAttachAction
	}{
		{name: "attach", action: NASPSAttach},
		{name: "detach", action: NASPSDetach},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASAttachDetach {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{byte(tt.action)})
				},
				resp: successResponse(MessageNASAttachDetach),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := client.NASAttachDetach(context.Background(), tt.action); err != nil {
				t.Fatalf("NASAttachDetach() error = %v", err)
			}
		})
	}
}

func TestNASAttachDetachValidation(t *testing.T) {
	tests := []struct {
		name   string
		action NASPSAttachAction
	}{
		{name: "zero", action: 0},
		{name: "above range", action: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := (&Client{}).NASAttachDetach(context.Background(), tt.action); err == nil {
				t.Fatal("NASAttachDetach() error = nil, want non-nil")
			}
		})
	}
}

func TestNASPreferredNetworksUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want NASPreferredNetworks
	}{
		{name: "empty"},
		{
			name: "user and static lists",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, preferredNetworkListValue(
					NASPreferredNetwork{PLMN: NASPLMN{MCC: 460, MNC: 0}, AccessTechnology: NASPLMNAccessEUTRAN},
					NASPreferredNetwork{PLMN: NASPLMN{MCC: 310, MNC: 260}, AccessTechnology: NASPLMNAccessNGRAN | NASPLMNAccessEUTRAN},
				)),
				tlv.Bytes(0x11, preferredNetworkListValue(
					NASPreferredNetwork{PLMN: NASPLMN{MCC: 001, MNC: 1}, AccessTechnology: NASPLMNAccessAll},
				)),
				tlv.Bytes(0x12, mncDigitStatusValue(
					NASPLMN{MCC: 460, MNC: 0, MNCThreeDigits: false},
					NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true},
				)),
				tlv.Bytes(0x13, mncDigitStatusValue(
					NASPLMN{MCC: 001, MNC: 1, MNCThreeDigits: true},
				)),
			},
			want: NASPreferredNetworks{
				Networks: []NASPreferredNetwork{
					{PLMN: NASPLMN{MCC: 460, MNC: 0, MNCThreeDigitsKnown: true}, AccessTechnology: NASPLMNAccessEUTRAN},
					{PLMN: NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, AccessTechnology: NASPLMNAccessNGRAN | NASPLMNAccessEUTRAN},
				},
				Known: true,
				Static: []NASPreferredNetwork{
					{PLMN: NASPLMN{MCC: 001, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, AccessTechnology: NASPLMNAccessAll},
				},
				StaticKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASPreferredNetworks
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Known != tt.want.Known || got.StaticKnown != tt.want.StaticKnown ||
				!slices.Equal(got.Networks, tt.want.Networks) || !slices.Equal(got.Static, tt.want.Static) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASPreferredNetworksRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name  string
		typ   byte
		value []byte
	}{
		{name: "network count missing", typ: 0x10, value: []byte{1}},
		{name: "network truncated", typ: 0x10, value: []byte{1, 0, 0}},
		{name: "static count too large", typ: 0x11, value: []byte{41, 0}},
		{name: "MNC status count missing", typ: 0x12},
		{name: "MNC status truncated", typ: 0x12, value: []byte{1, 0}},
		{name: "static MNC status trailing", typ: 0x13, value: []byte{0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var networks NASPreferredNetworks
			if err := networks.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(tt.typ, tt.value)}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestSetPreferredNetworks(t *testing.T) {
	clear := true
	tests := []struct {
		name   string
		config NASPreferredNetworksConfig
	}{
		{
			name: "replace list",
			config: NASPreferredNetworksConfig{
				Networks: []NASPreferredNetwork{
					{
						PLMN:             NASPLMN{MCC: 460, MNC: 1, MNCThreeDigitsKnown: true},
						AccessTechnology: NASPLMNAccessEUTRAN,
					},
					{
						PLMN:             NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true},
						AccessTechnology: NASPLMNAccessNGRAN,
					},
				},
				ClearPrevious: &clear,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != MessageNASSetPreferredNetworks {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, preferredNetworkListValue(tt.config.Networks...))
					assertTLV(t, req.TLVs, 0x11, mncDigitStatusValue(
						tt.config.Networks[0].PLMN,
						tt.config.Networks[1].PLMN,
					))
					assertTLV(t, req.TLVs, 0x12, []byte{1})
				},
				resp: successResponse(MessageNASSetPreferredNetworks),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := client.SetPreferredNetworks(context.Background(), tt.config); err != nil {
				t.Fatalf("SetPreferredNetworks() error = %v", err)
			}
		})
	}
}

func TestSetPreferredNetworksValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  NASPreferredNetworksConfig
		wantErr string
	}{
		{
			name:    "too many",
			config:  NASPreferredNetworksConfig{Networks: make([]NASPreferredNetwork, nasPreferredNetworksMax+1)},
			wantErr: "exceeds",
		},
		{
			name:    "MCC out of range",
			config:  NASPreferredNetworksConfig{Networks: []NASPreferredNetwork{{PLMN: NASPLMN{MCC: 1000}}}},
			wantErr: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Client{}).SetPreferredNetworks(context.Background(), tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SetPreferredNetworks() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNASTechnologyPreferences(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want NASTechnologyPreferences
	}{
		{
			name: "active and persistent",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{byte(NASTechnology3GPP | NASTechnologyLTE), 0, byte(NASPreferencePowerCycle)}),
				tlv.Uint(0x10, uint16(NASTechnology3GPP)),
			},
			want: NASTechnologyPreferences{
				Active:          NASTechnology3GPP | NASTechnologyLTE,
				Duration:        NASPreferencePowerCycle,
				Persistent:      NASTechnology3GPP,
				PersistentKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageNASGetTechnologyPreference || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(MessageNASGetTechnologyPreference, tt.tlvs...),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			got, err := client.TechnologyPreferences(context.Background())
			if err != nil {
				t.Fatalf("TechnologyPreferences() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("TechnologyPreferences() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSetTechnologyPreference(t *testing.T) {
	tests := []struct {
		name       string
		preference NASTechnologyPreference
		duration   NASPreferenceDuration
	}{
		{name: "3GPP LTE until power cycle", preference: NASTechnology3GPP | NASTechnologyLTE, duration: NASPreferencePowerCycle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageNASSetTechnologyPreference {
						t.Fatalf("message ID = 0x%04X", req.MessageID)
					}
					want := binary.LittleEndian.AppendUint16(nil, uint16(tt.preference))
					want = append(want, byte(tt.duration))
					assertTLV(t, req.TLVs, 0x01, want)
				},
				resp: successResponse(MessageNASSetTechnologyPreference),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			if err := client.SetTechnologyPreference(context.Background(), tt.preference, tt.duration); err != nil {
				t.Fatalf("SetTechnologyPreference() error = %v", err)
			}
		})
	}
}

func TestNASTechnologyPreferenceMalformed(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{name: "active missing"},
		{name: "active truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}},
		{name: "persistent truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0, 0}), tlv.Bytes(0x10, []byte{1})}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var preference NASTechnologyPreferences
			if err := preference.UnmarshalTLVs(tt.tlvs); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func preferredNetworkListValue(networks ...NASPreferredNetwork) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(networks)))
	for _, network := range networks {
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MNC)
		value = binary.LittleEndian.AppendUint16(value, uint16(network.AccessTechnology))
	}
	return value
}

func mncDigitStatusValue(plmns ...NASPLMN) []byte {
	value := []byte{byte(len(plmns))}
	for _, plmn := range plmns {
		value = binary.LittleEndian.AppendUint16(value, plmn.MCC)
		value = binary.LittleEndian.AppendUint16(value, plmn.MNC)
		value = append(value, boolByte(plmn.MNCThreeDigits))
	}
	return value
}
