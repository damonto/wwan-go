package qcom

import (
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASServingSystemUnmarshalIndicationTLVs(t *testing.T) {
	// Wire IDs come from Qualcomm nas_serving_system_ind_msg_data_v01,
	// independently of the response constants used by the implementation.
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASServingSystem
		wantErr bool
	}{
		{name: "mandatory fields only"},
		{
			name: "MNC digits and network name source",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x12, []byte{0xCC, 0x01, 1, 0, 0}),
				tlv.Bytes(0x29, []byte{0xCC, 0x01, 1, 0, 1}),
				tlv.Uint(0x2B, uint32(3)),
			},
			want: NASServingSystem{
				PLMN:              NASPLMN{MCC: 460, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true},
				PLMNKnown:         true,
				NetworkNameSource: NASNetworkNameSourceNITZ, NetworkNameSourceKnown: true,
			},
		},
		{
			name: "time aggregate is not LAC",
			tlvs: tlv.TLVs{tlv.Bytes(0x1C, []byte{0xEA, 7, 9, 29, 10, 25, 0, 32})},
		},
		{
			name: "LAC and cell ID",
			tlvs: tlv.TLVs{tlv.Uint(0x1D, uint16(0x1234)), tlv.Uint(0x1E, uint32(0x12345678))},
			want: NASServingSystem{
				LocationAreaCode: 0x1234, LocationAreaKnown: true,
				CellID: 0x12345678, CellIDKnown: true,
			},
		},
		{
			name: "HDR personality and TAC",
			tlvs: tlv.TLVs{tlv.Uint(0x24, uint8(1)), tlv.Uint(0x25, uint16(0xABCD))},
			want: NASServingSystem{TrackingAreaCode: 0xABCD, TrackingAreaKnown: true},
		},
		{
			name: "unconsumed indication fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x27, uint8(1)),    // Serving system unchanged.
				tlv.Uint(0x28, uint16(100)), // UMTS PSC.
				tlv.Uint(0x2A, uint8(1)),    // HS call status.
				tlv.Bytes(0xFE, []byte{1, 2, 3}),
			},
		},
		{name: "truncated LAC", tlvs: tlv.TLVs{tlv.Bytes(0x1D, []byte{1})}, wantErr: true},
		{name: "truncated cell ID", tlvs: tlv.TLVs{tlv.Bytes(0x1E, []byte{1, 2})}, wantErr: true},
		{name: "truncated TAC", tlvs: tlv.TLVs{tlv.Bytes(0x25, []byte{1})}, wantErr: true},
		{name: "truncated MNC digits", tlvs: tlv.TLVs{tlv.Bytes(0x29, make([]byte, 4))}, wantErr: true},
		{name: "truncated name source", tlvs: tlv.TLVs{tlv.Bytes(0x2B, make([]byte, 3))}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlvs := append(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 1, 1, 2, 1, 8})}, tt.tlvs...)
			var got NASServingSystem
			err := got.UnmarshalIndicationTLVs(tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalIndicationTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := tt.want
			want.RegistrationState = NASRegistrationRegistered
			want.CSAttachState = NASAttachAttached
			want.PSAttachState = NASAttachAttached
			want.SelectedNetwork = NASSelectedNetwork3GPP
			want.RadioInterfaces = []NASRadioInterface{NASRadioInterfaceLTE}
			checkNASServingSystem(t, got, want)
		})
	}
}

func TestNASSysInfoUnmarshalIndicationTLVs(t *testing.T) {
	// Wire IDs come from Qualcomm nas_sys_info_ind_msg_data_v01. Adjacent
	// voice/SMS fields use distinct values to expose same-length misdecodes.
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASSysInfo
		wantErr bool
	}{
		{name: "empty"},
		{
			name: "LTE IMS voice available",
			tlvs: tlv.TLVs{tlv.Uint(0x2A, uint8(1)), tlv.Uint(0x2B, uint32(1))},
			want: NASSysInfo{
				LTE:       NASRadioSystemInfo{IMSVoiceAvailable: true, IMSVoiceKnown: true, VoiceDomain: NASVoiceDomainIMS, VoiceDomainKnown: true},
				VoPSKnown: true, VoPSSupported: true,
			},
		},
		{
			name: "LTE IMS voice unavailable",
			tlvs: tlv.TLVs{tlv.Uint(0x2A, uint8(0)), tlv.Uint(0x2B, uint32(0))},
			want: NASSysInfo{LTE: NASRadioSystemInfo{IMSVoiceKnown: true, VoiceDomainKnown: true}, VoPSKnown: true},
		},
		{
			name: "voice and SMS domains",
			tlvs: tlv.TLVs{
				tlv.Uint(0x37, uint32(2)), tlv.Uint(0x38, uint32(1)),
				tlv.Uint(0x2B, uint32(1)), tlv.Uint(0x39, uint32(3)),
				tlv.Uint(0x3B, uint32(3)), tlv.Uint(0x3C, uint32(0)),
				tlv.Uint(0x3D, uint32(0)), tlv.Uint(0x3E, uint32(3)),
				tlv.Uint(0x40, uint32(2)), tlv.Uint(0x41, uint32(0)),
				tlv.Uint(0x42, uint32(3)), tlv.Uint(0x43, uint32(1)),
				tlv.Uint(0x58, uint32(1)), tlv.Uint(0x59, uint32(0)),
			},
			want: NASSysInfo{
				HDR:     NASRadioSystemInfo{VoiceDomain: NASVoiceDomainOneX, VoiceDomainKnown: true, SMSDomain: NASSMSDomainIMS, SMSDomainKnown: true},
				LTE:     NASRadioSystemInfo{VoiceDomain: NASVoiceDomainIMS, VoiceDomainKnown: true, SMSDomain: NASSMSDomain3GPP, SMSDomainKnown: true},
				GSM:     NASRadioSystemInfo{VoiceDomain: NASVoiceDomain3GPP, VoiceDomainKnown: true, SMSDomainKnown: true},
				WCDMA:   NASRadioSystemInfo{VoiceDomainKnown: true, SMSDomain: NASSMSDomain3GPP, SMSDomainKnown: true},
				CDMA:    NASRadioSystemInfo{VoiceDomain: NASVoiceDomainOneX, VoiceDomainKnown: true, SMSDomainKnown: true},
				TDSCDMA: NASRadioSystemInfo{VoiceDomain: NASVoiceDomain3GPP, VoiceDomainKnown: true, SMSDomain: NASSMSDomainIMS, SMSDomainKnown: true},
				NR5G:    NASRadioSystemInfo{VoiceDomain: NASVoiceDomainIMS, VoiceDomainKnown: true, SMSDomainKnown: true},
			},
		},
		{
			name: "TDSCDMA service and system",
			tlvs: tlv.TLVs{tlv.Bytes(0x25, []byte{2, 1, 0}), tlv.Bytes(0x26, make([]byte, 50))},
			want: NASSysInfo{TDSCDMA: NASRadioSystemInfo{
				ServiceStatus: NASServiceStatusAvailable, ServiceStatusKnown: true,
				TrueServiceStatus: NASServiceStatusLimited, TrueServiceStatusKnown: true,
				PreferredDataPathKnown: true, SystemInfoKnown: true,
			}},
		},
		{
			name: "NR5G service cell and capabilities",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x4C, []byte{2, 2, 1}), tlv.Bytes(0x4D, make([]byte, 29)),
				tlv.Uint(0x4F, uint32(2)), tlv.Uint(0x50, uint8(1)), tlv.Uint(0x51, uint8(0)),
				tlv.Bytes(0x52, []byte{1, 2, 3}), tlv.Uint(0x53, uint8(1)), tlv.Uint(0x54, uint8(0)),
				tlv.Uint(0x56, uint16(500)), tlv.Uint(0x5A, uint8(0)), tlv.Uint(0x5B, uint8(1)),
			},
			want: NASSysInfo{
				NR5G: NASRadioSystemInfo{
					ServiceStatus: NASServiceStatusAvailable, ServiceStatusKnown: true,
					TrueServiceStatus: NASServiceStatusAvailable, TrueServiceStatusKnown: true,
					PreferredDataPath: true, PreferredDataPathKnown: true, SystemInfoKnown: true,
					TrackingAreaCode: 0x010203, TrackingAreaCodeKnown: true,
					PhysicalCellID: 500, PhysicalCellIDKnown: true,
					VoiceSupportedKnown: true, IMSVoiceAvailable: true, IMSVoiceKnown: true,
				},
				CPSMSServiceStatus: NASCPSMSAvailable, CPSMSServiceStatusKnown: true,
				ENDCAvailable: true, ENDCAvailableKnown: true, DCNRRestrictedKnown: true,
				TrackingAreaRestricted: true, TrackingAreaRestrictedKnown: true, N1SMSRegisteredKnown: true,
				NRVoPSKnown: true, NRVoPSSupported: true,
			},
		},
		{
			name: "unconsumed indication fields do not become response fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x24, uint8(1)),  // System info unchanged, not TDS service.
				tlv.Uint(0x29, uint8(1)),  // E-UTRA detection, not LTE VoPS.
				tlv.Uint(0x3A, uint32(1)), // Emergency bearer support, not GSM voice.
				tlv.Uint(0x3F, uint32(1)), // Emergency access barred, not CDMA voice.
				tlv.Uint(0x4B, uint32(1)), // CIoT mode, not NR5G system info.
				tlv.Uint(0x4E, uint32(1)), // NR5G cell status, not ENDC.
				tlv.Uint(0x57, uint8(1)),  // PLMN list availability, not NR5G SMS.
				tlv.Bytes(0xFE, []byte{1, 2, 3}),
			},
		},
		{name: "truncated LTE IMS voice", tlvs: tlv.TLVs{tlv.Bytes(0x2A, nil)}, wantErr: true},
		{name: "truncated LTE voice domain", tlvs: tlv.TLVs{tlv.Bytes(0x2B, []byte{1})}, wantErr: true},
		{name: "truncated NR5G system", tlvs: tlv.TLVs{tlv.Bytes(0x4D, make([]byte, 28))}, wantErr: true},
		{name: "truncated NR5G TAC", tlvs: tlv.TLVs{tlv.Bytes(0x52, []byte{1, 2})}, wantErr: true},
		{name: "truncated NR5G PCI", tlvs: tlv.TLVs{tlv.Bytes(0x56, []byte{1})}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSysInfo
			err := got.UnmarshalIndicationTLVs(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalIndicationTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("UnmarshalIndicationTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
