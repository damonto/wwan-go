package qcom

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSDeviceCapabilitiesUnmarshalTLVs(t *testing.T) {
	core := encodeDMSDeviceCapabilitiesCore(
		1_000_000,
		2_000_000,
		DMSDataServicePSOnly,
		DMSSIMSupported,
		DMSRadioInterfaceLTE,
		DMSRadioInterfaceNR5G,
	)
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSDeviceCapabilities
		wantErr bool
	}{
		{
			name: "common capabilities",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPrimaryValue, core),
				tlv.Uint(dmsTLVDeviceServiceCapability, uint32(DMSDeviceServiceSimultaneousVoiceData)),
				dmsUint64TLV(dmsTLVVoiceSupportCapability, uint64(DMSVoiceSupportGWCSFB|DMSVoiceSupportVoLTE)),
				dmsUint64TLV(dmsTLVSimultaneousVoiceData, uint64(DMSSimultaneousVoiceDataSVLTE)),
				tlv.Bytes(dmsTLVCurrentMultiSIMCapability, []byte{3, 2}),
				tlv.Bytes(dmsTLVCurrentSubscriptionCaps, encodeDMSSubscriptionCapabilities(
					DMSSubscriptionCapabilityLTE|DMSSubscriptionCapabilityNR5G,
					DMSSubscriptionCapabilityLTE,
				)),
				tlv.Bytes(dmsTLVSubscriptionVoiceDataCaps, encodeDMSSubscriptionVoiceData(
					DMSSubscriptionVoiceData{Capability: DMSSubscriptionVoiceDataSVLTE, Concurrent: true},
					DMSSubscriptionVoiceData{Capability: DMSSubscriptionVoiceDataCSFB},
				)),
				tlv.Bytes(dmsTLVSubscriptionFeatureModes, encodeDMSSubscriptionFeatureModes(
					DMSSubscriptionFeatureSVLTE,
					DMSSubscriptionFeatureNormal,
				)),
				tlv.Bytes(dmsTLVMaxActiveDataSubscriptions, []byte{2}),
				tlv.Bytes(dmsTLVMaximumSubscriptionCaps, encodeDMSSubscriptionCapabilities(
					DMSSubscriptionCapabilityLTE|DMSSubscriptionCapabilityNR5G,
					DMSSubscriptionCapabilityLTE|DMSSubscriptionCapabilityNR5G,
				)),
				tlv.Bytes(dmsTLVIMSCapabilities, encodeDMSIMSCapabilities(
					DMSIMSCapability{Subscription: DMSSubscriptionPrimary, Enabled: true},
					DMSIMSCapability{Subscription: DMSSubscriptionSecondary},
				)),
				tlv.Bytes(dmsTLVMaxIMSInstances, []byte{1}),
			},
			want: DMSDeviceCapabilities{
				MaxTXRate:       1_000_000,
				MaxRXRate:       2_000_000,
				DataService:     DMSDataServicePSOnly,
				SIM:             DMSSIMSupported,
				RadioInterfaces: []DMSRadioInterface{DMSRadioInterfaceLTE, DMSRadioInterfaceNR5G},

				DeviceService:              DMSDeviceServiceSimultaneousVoiceData,
				DeviceServiceKnown:         true,
				VoiceSupport:               DMSVoiceSupportGWCSFB | DMSVoiceSupportVoLTE,
				VoiceSupportKnown:          true,
				SimultaneousVoiceData:      DMSSimultaneousVoiceDataSVLTE,
				SimultaneousVoiceDataKnown: true,

				CurrentMultiSIM:      DMSMultiSIMCapability{MaxSubscriptions: 3, MaxActive: 2},
				CurrentMultiSIMKnown: true,

				CurrentSubscriptionCapabilities: []DMSSubscriptionCapability{
					DMSSubscriptionCapabilityLTE | DMSSubscriptionCapabilityNR5G,
					DMSSubscriptionCapabilityLTE,
				},
				CurrentSubscriptionCapabilitiesKnown: true,
				SubscriptionVoiceData: []DMSSubscriptionVoiceData{
					{Capability: DMSSubscriptionVoiceDataSVLTE, Concurrent: true},
					{Capability: DMSSubscriptionVoiceDataCSFB},
				},
				SubscriptionVoiceDataKnown: true,
				SubscriptionFeatureModes: []DMSSubscriptionFeatureMode{
					DMSSubscriptionFeatureSVLTE,
					DMSSubscriptionFeatureNormal,
				},
				SubscriptionFeatureModesKnown: true,

				MaxActiveDataSubscriptions:      2,
				MaxActiveDataSubscriptionsKnown: true,
				MaximumSubscriptionCapabilities: []DMSSubscriptionCapability{
					DMSSubscriptionCapabilityLTE | DMSSubscriptionCapabilityNR5G,
					DMSSubscriptionCapabilityLTE | DMSSubscriptionCapabilityNR5G,
				},
				MaximumSubscriptionCapabilitiesKnown: true,

				IMSCapabilities: []DMSIMSCapability{
					{Subscription: DMSSubscriptionPrimary, Enabled: true},
					{Subscription: DMSSubscriptionSecondary},
				},
				IMSCapabilitiesKnown: true,
				MaxIMSInstances:      1,
				MaxIMSInstancesKnown: true,
			},
		},
		{name: "missing core", wantErr: true},
		{name: "truncated core", tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, make([]byte, 10))}, wantErr: true},
		{
			name:    "too many radio interfaces",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, append(make([]byte, 10), dmsMaxRadioInterfaces+1))},
			wantErr: true,
		},
		{
			name:    "radio interface length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, append(make([]byte, 10), 2, byte(DMSRadioInterfaceLTE)))},
			wantErr: true,
		},
		{
			name:    "truncated device service",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVDeviceServiceCapability, make([]byte, 3))},
			wantErr: true,
		},
		{
			name:    "truncated voice support",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVVoiceSupportCapability, make([]byte, 7))},
			wantErr: true,
		},
		{
			name:    "oversized simultaneous voice data",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVSimultaneousVoiceData, make([]byte, 9))},
			wantErr: true,
		},
		{
			name:    "truncated multi-SIM",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVCurrentMultiSIMCapability, []byte{2})},
			wantErr: true,
		},
		{
			name:    "missing current capability count",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVCurrentSubscriptionCaps, nil)},
			wantErr: true,
		},
		{
			name:    "too many current capabilities",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVCurrentSubscriptionCaps, []byte{dmsMaxSubscriptions + 1})},
			wantErr: true,
		},
		{
			name:    "current capability length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVCurrentSubscriptionCaps, []byte{1})},
			wantErr: true,
		},
		{
			name:    "voice data length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVSubscriptionVoiceDataCaps, []byte{1, 1})},
			wantErr: true,
		},
		{
			name:    "feature mode length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVSubscriptionFeatureModes, []byte{1, 1})},
			wantErr: true,
		},
		{
			name:    "missing max active data subscriptions",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVMaxActiveDataSubscriptions, nil)},
			wantErr: true,
		},
		{
			name:    "maximum capability length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVMaximumSubscriptionCaps, []byte{1})},
			wantErr: true,
		},
		{
			name:    "IMS capability length mismatch",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVIMSCapabilities, []byte{1, 1})},
			wantErr: true,
		},
		{
			name:    "oversized max IMS instances",
			tlvs:    tlv.TLVs{tlv.Bytes(dmsTLVPrimaryValue, core), tlv.Bytes(dmsTLVMaxIMSInstances, []byte{1, 2})},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSDeviceCapabilities
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
			if !equalDMSDeviceCapabilities(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func equalDMSDeviceCapabilities(got, want DMSDeviceCapabilities) bool {
	return got.MaxTXRate == want.MaxTXRate &&
		got.MaxRXRate == want.MaxRXRate &&
		got.DataService == want.DataService &&
		got.SIM == want.SIM &&
		slices.Equal(got.RadioInterfaces, want.RadioInterfaces) &&
		got.DeviceService == want.DeviceService &&
		got.DeviceServiceKnown == want.DeviceServiceKnown &&
		got.VoiceSupport == want.VoiceSupport &&
		got.VoiceSupportKnown == want.VoiceSupportKnown &&
		got.SimultaneousVoiceData == want.SimultaneousVoiceData &&
		got.SimultaneousVoiceDataKnown == want.SimultaneousVoiceDataKnown &&
		got.CurrentMultiSIM == want.CurrentMultiSIM &&
		got.CurrentMultiSIMKnown == want.CurrentMultiSIMKnown &&
		slices.Equal(got.CurrentSubscriptionCapabilities, want.CurrentSubscriptionCapabilities) &&
		got.CurrentSubscriptionCapabilitiesKnown == want.CurrentSubscriptionCapabilitiesKnown &&
		slices.Equal(got.SubscriptionVoiceData, want.SubscriptionVoiceData) &&
		got.SubscriptionVoiceDataKnown == want.SubscriptionVoiceDataKnown &&
		slices.Equal(got.SubscriptionFeatureModes, want.SubscriptionFeatureModes) &&
		got.SubscriptionFeatureModesKnown == want.SubscriptionFeatureModesKnown &&
		got.MaxActiveDataSubscriptions == want.MaxActiveDataSubscriptions &&
		got.MaxActiveDataSubscriptionsKnown == want.MaxActiveDataSubscriptionsKnown &&
		slices.Equal(got.MaximumSubscriptionCapabilities, want.MaximumSubscriptionCapabilities) &&
		got.MaximumSubscriptionCapabilitiesKnown == want.MaximumSubscriptionCapabilitiesKnown &&
		slices.Equal(got.IMSCapabilities, want.IMSCapabilities) &&
		got.IMSCapabilitiesKnown == want.IMSCapabilitiesKnown &&
		got.MaxIMSInstances == want.MaxIMSInstances &&
		got.MaxIMSInstancesKnown == want.MaxIMSInstancesKnown
}

func encodeDMSDeviceCapabilitiesCore(
	maxTXRate uint32,
	maxRXRate uint32,
	dataService DMSDataServiceCapability,
	sim DMSSIMCapability,
	radios ...DMSRadioInterface,
) []byte {
	value := binary.LittleEndian.AppendUint32(nil, maxTXRate)
	value = binary.LittleEndian.AppendUint32(value, maxRXRate)
	value = append(value, byte(dataService), byte(sim), byte(len(radios)))
	for _, radio := range radios {
		value = append(value, byte(radio))
	}
	return value
}

func encodeDMSSubscriptionCapabilities(capabilities ...DMSSubscriptionCapability) []byte {
	value := []byte{byte(len(capabilities))}
	for _, capability := range capabilities {
		value = binary.LittleEndian.AppendUint64(value, uint64(capability))
	}
	return value
}

func encodeDMSSubscriptionVoiceData(capabilities ...DMSSubscriptionVoiceData) []byte {
	value := []byte{byte(len(capabilities))}
	for _, capability := range capabilities {
		value = binary.LittleEndian.AppendUint32(value, uint32(capability.Capability))
		value = append(value, boolByte(capability.Concurrent))
	}
	return value
}

func encodeDMSSubscriptionFeatureModes(modes ...DMSSubscriptionFeatureMode) []byte {
	value := []byte{byte(len(modes))}
	for _, mode := range modes {
		value = binary.LittleEndian.AppendUint32(value, uint32(mode))
	}
	return value
}

func encodeDMSIMSCapabilities(capabilities ...DMSIMSCapability) []byte {
	value := []byte{byte(len(capabilities))}
	for _, capability := range capabilities {
		value = binary.LittleEndian.AppendUint32(value, uint32(capability.Subscription))
		value = append(value, boolByte(capability.Enabled))
	}
	return value
}
