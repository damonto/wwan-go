package qcom

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASGetServingSystemRequest(t *testing.T) {
	req := NASGetServingSystemRequest{ClientID: 7}.Request()
	if req.Service != ServiceNAS || req.MessageID != MessageNASGetServingSystem {
		t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
	}
	if len(req.TLVs) != 0 {
		t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
	}
}

func TestNASGetServingSystemResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASServingSystem
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "truncated aggregate", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 1})}, wantErr: true},
		{name: "truncated radio list", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 2, 8})}, wantErr: true},
		{name: "trailing radio data", tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8, 9})}, wantErr: true},
		{
			name: "registered LTE",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8})},
			want: NASServingSystem{
				RegistrationState: NASRegistrationRegistered,
				CSAttachState:     NASAttachAttached,
				PSAttachState:     NASAttachAttached,
				SelectedNetwork:   NASSelectedNetwork3GPP,
				RadioInterfaces:   []NASRadioInterface{NASRadioInterfaceLTE},
			},
		},
		{
			name: "multiple radios",
			tlvs: tlv.TLVs{tlv.Bytes(nasTLVServingSystem, []byte{1, 2, 1, 2, 2, 5, 8})},
			want: NASServingSystem{
				RegistrationState: NASRegistrationRegistered,
				CSAttachState:     NASAttachDetached,
				PSAttachState:     NASAttachAttached,
				SelectedNetwork:   NASSelectedNetwork3GPP,
				RadioInterfaces:   []NASRadioInterface{NASRadioInterfaceUMTS, NASRadioInterfaceLTE},
			},
		},
		{
			name: "3GPP optional fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8}),
				tlv.Bytes(nasTLVRoamingIndicator, []byte{1}),
				tlv.Bytes(nasTLVDataCapabilities, []byte{2, 0x0B, 0x02}),
				tlv.Bytes(nasTLVCurrentPLMN, []byte{0xCC, 0x01, 0x01, 0x00, 7, 'C', 'a', 'r', 'r', 'i', 'e', 'r'}),
				tlv.Bytes(nasTLVTimeZone, []byte{0xFC}),
				tlv.Bytes(nasTLVDaylightSaving, []byte{1}),
				tlv.Bytes(nasTLVLocationAreaCode, []byte{0x34, 0x12}),
				tlv.Bytes(nasTLVCellID, []byte{0x78, 0x56, 0x34, 0x12}),
				tlv.Bytes(nasTLVTrackingAreaCode, []byte{0xCD, 0xAB}),
				tlv.Bytes(nasTLVMNCIncludesPCSDigit, []byte{0xCC, 0x01, 0x01, 0x00, 1}),
				tlv.Bytes(nasTLVNetworkNameSource, []byte{3, 0, 0, 0}),
			},
			want: NASServingSystem{
				RegistrationState:      NASRegistrationRegistered,
				CSAttachState:          NASAttachAttached,
				PSAttachState:          NASAttachAttached,
				SelectedNetwork:        NASSelectedNetwork3GPP,
				RadioInterfaces:        []NASRadioInterface{NASRadioInterfaceLTE},
				RoamingIndicator:       NASRoamingIndicatorHome,
				RoamingIndicatorKnown:  true,
				DataCapabilities:       []NASDataCapability{NASDataCapabilityLTE, NASDataCapabilityEDGE},
				DataCapabilitiesKnown:  true,
				PLMN:                   NASPLMN{MCC: 460, MNC: 1, Description: "Carrier", MNCThreeDigits: true, MNCThreeDigitsKnown: true},
				PLMNKnown:              true,
				TimeZoneQuarterHours:   -4,
				TimeZoneKnown:          true,
				DaylightSavingHours:    1,
				DaylightSavingKnown:    true,
				LocationAreaCode:       0x1234,
				LocationAreaKnown:      true,
				CellID:                 0x12345678,
				CellIDKnown:            true,
				TrackingAreaCode:       0xABCD,
				TrackingAreaKnown:      true,
				NetworkNameSource:      NASNetworkNameSourceNITZ,
				NetworkNameSourceKnown: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASGetServingSystemResponse
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
			checkNASServingSystem(t, got.ServingSystem, tt.want)
		})
	}
}

func TestNASGetServingSystemResponseRejectsMalformedOptionalTLVs(t *testing.T) {
	mandatory := tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8})
	tests := []struct {
		name string
		tlv  tlv.TLV
	}{
		{name: "roaming indicator", tlv: tlv.Bytes(nasTLVRoamingIndicator, nil)},
		{name: "data capabilities count", tlv: tlv.Bytes(nasTLVDataCapabilities, nil)},
		{name: "data capabilities list", tlv: tlv.Bytes(nasTLVDataCapabilities, []byte{2, 0x0B})},
		{name: "too many data capabilities", tlv: tlv.Bytes(nasTLVDataCapabilities, []byte{nasMaxDataCapabilities + 1})},
		{name: "current PLMN", tlv: tlv.Bytes(nasTLVCurrentPLMN, []byte{0xCC, 0x01})},
		{name: "time zone", tlv: tlv.Bytes(nasTLVTimeZone, nil)},
		{name: "daylight saving", tlv: tlv.Bytes(nasTLVDaylightSaving, nil)},
		{name: "location area", tlv: tlv.Bytes(nasTLVLocationAreaCode, []byte{1})},
		{name: "cell ID", tlv: tlv.Bytes(nasTLVCellID, []byte{1, 2, 3})},
		{name: "tracking area", tlv: tlv.Bytes(nasTLVTrackingAreaCode, []byte{1})},
		{name: "MNC digit", tlv: tlv.Bytes(nasTLVMNCIncludesPCSDigit, []byte{1, 2, 3, 4})},
		{name: "network name source", tlv: tlv.Bytes(nasTLVNetworkNameSource, []byte{1, 2, 3})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASGetServingSystemResponse
			if err := got.UnmarshalTLVs(tlv.TLVs{mandatory, tt.tlv}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func checkNASServingSystem(t *testing.T, got, want NASServingSystem) {
	t.Helper()
	if !slices.Equal(got.RadioInterfaces, want.RadioInterfaces) {
		t.Fatalf("RadioInterfaces = %v, want %v", got.RadioInterfaces, want.RadioInterfaces)
	}
	if !slices.Equal(got.DataCapabilities, want.DataCapabilities) {
		t.Fatalf("DataCapabilities = %v, want %v", got.DataCapabilities, want.DataCapabilities)
	}
	if got.RegistrationState != want.RegistrationState ||
		got.CSAttachState != want.CSAttachState ||
		got.PSAttachState != want.PSAttachState ||
		got.SelectedNetwork != want.SelectedNetwork ||
		got.RoamingIndicator != want.RoamingIndicator ||
		got.RoamingIndicatorKnown != want.RoamingIndicatorKnown ||
		got.DataCapabilitiesKnown != want.DataCapabilitiesKnown ||
		got.PLMN != want.PLMN ||
		got.PLMNKnown != want.PLMNKnown ||
		got.TimeZoneQuarterHours != want.TimeZoneQuarterHours ||
		got.TimeZoneKnown != want.TimeZoneKnown ||
		got.DaylightSavingHours != want.DaylightSavingHours ||
		got.DaylightSavingKnown != want.DaylightSavingKnown ||
		got.LocationAreaCode != want.LocationAreaCode ||
		got.LocationAreaKnown != want.LocationAreaKnown ||
		got.CellID != want.CellID ||
		got.CellIDKnown != want.CellIDKnown ||
		got.TrackingAreaCode != want.TrackingAreaCode ||
		got.TrackingAreaKnown != want.TrackingAreaKnown ||
		got.NetworkNameSource != want.NetworkNameSource ||
		got.NetworkNameSourceKnown != want.NetworkNameSourceKnown {
		t.Fatalf("ServingSystem = %+v, want %+v", got, want)
	}
}

func TestClientNASServingSystem(t *testing.T) {
	transport := &fakeTransport{t: t, calls: []transportCall{
		{resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceNAS), 5}))},
		{
			check: func(req Request) {
				if req.Service != ServiceNAS || req.ClientID != 5 || req.MessageID != MessageNASGetServingSystem {
					t.Fatalf("unexpected NAS request: %+v", req)
				}
			},
			resp: successResponse(MessageNASGetServingSystem, tlv.Bytes(nasTLVServingSystem, []byte{1, 1, 1, 2, 1, 8})),
		},
		{resp: successResponse(MessageReleaseClientID)},
	}}
	reader := &Client{transport: transport, slot: 1}

	got, err := reader.NASServingSystem(context.Background())
	if err != nil {
		t.Fatalf("NASServingSystem() error = %v", err)
	}
	if got.RegistrationState != NASRegistrationRegistered || got.PSAttachState != NASAttachAttached {
		t.Fatalf("NASServingSystem() = %+v", got)
	}
}

func TestNASGetSysInfoRequest(t *testing.T) {
	tests := []struct {
		name string
		req  NASGetSysInfoRequest
	}{
		{
			name: "request fields",
			req: NASGetSysInfoRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.Request()
			if got.Service != ServiceNAS {
				t.Fatalf("Service = 0x%02X, want 0x%02X", got.Service, ServiceNAS)
			}
			if got.ClientID != tt.req.ClientID {
				t.Fatalf("ClientID = %d, want %d", got.ClientID, tt.req.ClientID)
			}
			if got.MessageID != MessageNASGetSysInfo {
				t.Fatalf("MessageID = 0x%04X, want 0x%04X", got.MessageID, MessageNASGetSysInfo)
			}
			if got.Timeout != tt.req.Timeout {
				t.Fatalf("Timeout = %v, want %v", got.Timeout, tt.req.Timeout)
			}
			if len(got.TLVs) != 0 {
				t.Fatalf("TLVs len = %d, want 0", len(got.TLVs))
			}
		})
	}
}

func TestNASGetSysInfoResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name          string
		tlvs          tlv.TLVs
		wantKnown     bool
		wantSupported bool
		wantErr       bool
	}{
		{name: "missing vops"},
		{name: "vops supported", tlvs: tlv.TLVs{tlv.Bytes(0x29, []byte{1})}, wantKnown: true, wantSupported: true},
		{name: "vops unsupported", tlvs: tlv.TLVs{tlv.Bytes(0x29, []byte{0})}, wantKnown: true},
		{name: "empty vops", tlvs: tlv.TLVs{tlv.Bytes(0x29, nil)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASGetSysInfoResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.SysInfo.VoPSKnown != tt.wantKnown {
				t.Fatalf("VoPSKnown = %v, want %v", got.SysInfo.VoPSKnown, tt.wantKnown)
			}
			if got.SysInfo.VoPSSupported != tt.wantSupported {
				t.Fatalf("VoPSSupported = %v, want %v", got.SysInfo.VoPSSupported, tt.wantSupported)
			}
		})
	}
}
