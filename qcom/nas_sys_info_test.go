package qcom

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASSysInfoUnmarshalTLVs(t *testing.T) {
	lteSystem := nasTest3GPPSystemInfo(29, "310", "260", 0x1234, 0x12345678, 0xBEEF)
	nrSystem := nasTest3GPPSystemInfo(29, "460", "01", 0x4321, 0x87654321, 0xCAFE)

	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASSysInfo
		wantErr bool
	}{
		{name: "missing optional fields"},
		{
			name: "LTE and NR5G",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVSysInfoLTEService, []byte{2, 2, 1}),
				tlv.Bytes(nasTLVSysInfoLTE, lteSystem),
				tlv.Uint(nasTLVSysInfoLTEVoice, uint8(1)),
				tlv.Uint(nasTLVSysInfoLTEVoPS, uint8(1)),
				tlv.Uint(nasTLVSysInfoLTEVoiceDomain, uint32(NASVoiceDomainIMS)),
				tlv.Uint(nasTLVSysInfoLTESMSDomain, uint32(NASSMSDomain3GPP)),
				tlv.Bytes(nasTLVSysInfoNR5GService, []byte{2, 1, 0}),
				tlv.Bytes(nasTLVSysInfoNR5G, nrSystem),
				tlv.Uint(nasTLVSysInfoCPSMS, uint32(NASCPSMSAvailable)),
				tlv.Uint(nasTLVSysInfoENDC, uint8(1)),
				tlv.Uint(nasTLVSysInfoDCNRRestricted, uint8(0)),
				tlv.Bytes(nasTLVSysInfoNR5GTAC, []byte{0x01, 0x02, 0x03}),
				tlv.Uint(nasTLVSysInfoTARestricted, uint8(1)),
				tlv.Uint(nasTLVSysInfoN1SMS, uint8(1)),
				tlv.Uint(nasTLVSysInfoNR5GPCI, uint16(500)),
				tlv.Uint(nasTLVSysInfoNR5GVoiceDomain, uint32(NASVoiceDomainIMS)),
				tlv.Uint(nasTLVSysInfoNR5GSMSDomain, uint32(NASSMSDomainIMS)),
				tlv.Uint(nasTLVSysInfoNR5GVoice, uint8(1)),
				tlv.Uint(nasTLVSysInfoNR5GVoPS, uint8(1)),
			},
			want: NASSysInfo{
				LTE: NASRadioSystemInfo{
					ServiceStatus: NASServiceStatusAvailable, ServiceStatusKnown: true,
					TrueServiceStatus: NASServiceStatusAvailable, TrueServiceStatusKnown: true,
					PreferredDataPath: true, PreferredDataPathKnown: true,
					SystemInfoKnown: true,
					ServiceDomain:   NASServiceDomainCircuitAndPacketSwitched, ServiceDomainKnown: true,
					ServiceCapability: NASServiceDomainCircuitAndPacketSwitched, ServiceCapabilityKnown: true,
					RoamingStatus: NASRoamingStatusOn, RoamingStatusKnown: true,
					ForbiddenKnown: true,
					PLMN:           NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true}, PLMNKnown: true,
					LocationAreaCode: 0x1234, LocationAreaCodeKnown: true,
					CellID: 0x12345678, CellIDKnown: true,
					TrackingAreaCode: 0xBEEF, TrackingAreaCodeKnown: true,
					RegistrationRejectDomain: NASServiceDomainPacketSwitched,
					RegistrationRejectCause:  15,
					RegistrationRejectKnown:  true,
					VoiceDomain:              NASVoiceDomainIMS, VoiceDomainKnown: true,
					SMSDomain: NASSMSDomain3GPP, SMSDomainKnown: true,
					VoiceSupported: true, VoiceSupportedKnown: true,
					IMSVoiceAvailable: true, IMSVoiceKnown: true,
				},
				NR5G: NASRadioSystemInfo{
					ServiceStatus: NASServiceStatusAvailable, ServiceStatusKnown: true,
					TrueServiceStatus: NASServiceStatusLimited, TrueServiceStatusKnown: true,
					PreferredDataPathKnown: true,
					SystemInfoKnown:        true,
					ServiceDomain:          NASServiceDomainCircuitAndPacketSwitched, ServiceDomainKnown: true,
					ServiceCapability: NASServiceDomainCircuitAndPacketSwitched, ServiceCapabilityKnown: true,
					RoamingStatus: NASRoamingStatusOn, RoamingStatusKnown: true,
					ForbiddenKnown: true,
					PLMN:           NASPLMN{MCC: 460, MNC: 1, MNCThreeDigitsKnown: true}, PLMNKnown: true,
					LocationAreaCode: 0x4321, LocationAreaCodeKnown: true,
					CellID: 0x87654321, CellIDKnown: true,
					TrackingAreaCode: 0x010203, TrackingAreaCodeKnown: true,
					PhysicalCellID: 500, PhysicalCellIDKnown: true,
					RegistrationRejectDomain: NASServiceDomainPacketSwitched,
					RegistrationRejectCause:  15,
					RegistrationRejectKnown:  true,
					VoiceDomain:              NASVoiceDomainIMS, VoiceDomainKnown: true,
					SMSDomain: NASSMSDomainIMS, SMSDomainKnown: true,
					VoiceSupported: true, VoiceSupportedKnown: true,
					IMSVoiceAvailable: true, IMSVoiceKnown: true,
				},
				CPSMSServiceStatus: NASCPSMSAvailable, CPSMSServiceStatusKnown: true,
				ENDCAvailable: true, ENDCAvailableKnown: true,
				DCNRRestrictedKnown:    true,
				TrackingAreaRestricted: true, TrackingAreaRestrictedKnown: true,
				N1SMSRegistered: true, N1SMSRegisteredKnown: true,
				VoPSKnown: true, VoPSSupported: true,
				NRVoPSKnown: true, NRVoPSSupported: true,
			},
		},
		{name: "truncated 3GPP service", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoLTEService, []byte{2, 2})}, wantErr: true},
		{name: "truncated LTE system", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoLTE, make([]byte, 28))}, wantErr: true},
		{name: "invalid PLMN digit", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoLTE, nasTestInvalidPLMN(lteSystem))}, wantErr: true},
		{name: "truncated voice domain", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoLTEVoiceDomain, make([]byte, 3))}, wantErr: true},
		{name: "truncated NR5G TAC", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoNR5GTAC, make([]byte, 2))}, wantErr: true},
		{name: "truncated NR5G PCI", tlvs: tlv.TLVs{tlv.Bytes(nasTLVSysInfoNR5GPCI, make([]byte, 1))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSysInfo
			err := got.UnmarshalTLVs(tt.tlvs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalTLVs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNASSysInfoLegacyVoPS(t *testing.T) {
	tests := []struct {
		name      string
		available uint8
	}{
		{name: "unavailable"},
		{name: "available", available: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASSysInfo
			if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(nasTLVSysInfoLTEVoPS, tt.available)}); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !got.VoPSKnown || got.VoPSSupported != (tt.available != 0) {
				t.Fatalf("VoPS = (%v, %v), want (true, %v)", got.VoPSKnown, got.VoPSSupported, tt.available != 0)
			}
		})
	}
}

func nasTest3GPPSystemInfo(length int, mcc, mnc string, lac uint16, cellID uint32, tac uint16) []byte {
	value := make([]byte, length)
	copy(value[:8], []byte{1, byte(NASServiceDomainCircuitAndPacketSwitched), 1, byte(NASServiceDomainCircuitAndPacketSwitched), 1, byte(NASRoamingStatusOn), 1, 0})
	value[8] = 1
	binary.LittleEndian.PutUint16(value[9:11], lac)
	value[11] = 1
	binary.LittleEndian.PutUint32(value[12:16], cellID)
	value[16] = 1
	value[17] = byte(NASServiceDomainPacketSwitched)
	value[18] = 15
	value[19] = 1
	copy(value[20:23], mcc)
	copy(value[23:25], mnc)
	if len(mnc) == 3 {
		value[25] = mnc[2]
	} else {
		value[25] = 0xFF
	}
	value[26] = 1
	binary.LittleEndian.PutUint16(value[27:29], tac)
	return value
}

func nasTestInvalidPLMN(value []byte) []byte {
	invalid := append([]byte(nil), value...)
	invalid[20] = 'x'
	return invalid
}
