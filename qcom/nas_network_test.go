package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestNASPerformNetworkScanRequest(t *testing.T) {
	networkTypes := NASNetworkTypeLTE | NASNetworkTypeUMTS
	scanType := NASNetworkScanPLMN
	band := NASBandPreference(0x0102030405060708)
	lteBand := NASLTEBandPreference(0x1122334455667788)
	tdsBand := NASTDSCDMABandPreference(0x3F)
	scope := NASNetworkScanAcquisitionDatabase
	lteExtended := NASLTEBandPreferenceExtended{Bits1To64: 1, Bits65To128: 2, Bits129To192: 3, Bits193To256: 4}
	tests := []struct {
		name    string
		config  NASNetworkScanConfig
		check   func(*testing.T, Request)
		wantErr bool
	}{
		{
			name: "all filters",
			config: NASNetworkScanConfig{
				NetworkTypes:      &networkTypes,
				ScanType:          &scanType,
				BandPreference:    &band,
				LTEBandPreference: &lteBand,
				TDSBandPreference: &tdsBand,
				Scope:             &scope,
				LTEBandsExtended:  &lteExtended,
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, nasTLVScanNetworkTypes, []byte{0x06})
				assertTLV(t, req.TLVs, nasTLVScanType, []byte{0, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVScanBandPreference, binary.LittleEndian.AppendUint64(nil, uint64(band)))
				assertTLV(t, req.TLVs, nasTLVScanLTEBandPreference, binary.LittleEndian.AppendUint64(nil, uint64(lteBand)))
				assertTLV(t, req.TLVs, nasTLVScanTDSBandPreference, binary.LittleEndian.AppendUint64(nil, uint64(tdsBand)))
				assertTLV(t, req.TLVs, nasTLVScanScope, []byte{1, 0, 0, 0})
				assertTLV(t, req.TLVs, nasTLVScanLTEBandsExtended, mustMarshalBinary(t, lteExtended))
			},
		},
		{
			name: "empty filters",
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
				}
			},
		},
		{name: "scan type out of range", config: NASNetworkScanConfig{ScanType: ptr(NASNetworkScanCellSearch + 1)}, wantErr: true},
		{name: "scope out of range", config: NASNetworkScanConfig{Scope: ptr(NASNetworkScanAcquisitionDatabase + 1)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASPerformNetworkScanRequest{
				ClientID: 7, TransactionID: 9, Timeout: 2 * time.Minute, Config: tt.config,
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
				req.MessageID != MessageNASPerformNetworkScan || req.Timeout != 2*time.Minute {
				t.Fatalf("Request() = %+v", req)
			}
			tt.check(t, req)
		})
	}
}

func TestNASNetworkScanUnmarshalTLVs(t *testing.T) {
	networks := encodeNASVisibleNetworksForTest(
		NASVisibleNetwork{PLMN: NASPLMN{MCC: 460, MNC: 1, Description: "Carrier"}, Status: 0x65},
		NASVisibleNetwork{PLMN: NASPLMN{MCC: 310, MNC: 260, Description: "Away"}, Status: 0xAA},
	)
	radios := encodeNASNetworkRadiosForTest(
		nasNetworkRadioForTest{PLMN: NASPLMN{MCC: 460, MNC: 1}, Radio: NASRadioInterfaceLTE},
		nasNetworkRadioForTest{PLMN: NASPLMN{MCC: 460, MNC: 1}, Radio: NASRadioInterfaceNR5G},
		nasNetworkRadioForTest{PLMN: NASPLMN{MCC: 310, MNC: 260}, Radio: NASRadioInterfaceLTE},
	)
	mncDigits := encodeNASNetworkMNCDigitsForTest(
		NASPLMN{MCC: 460, MNC: 1, MNCThreeDigits: true},
		NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true},
	)
	nameSources := []byte{2}
	nameSources = binary.LittleEndian.AppendUint32(nameSources, uint32(NASNetworkNameSourceNITZ))
	nameSources = binary.LittleEndian.AppendUint32(nameSources, uint32(NASNetworkNameSourceMCCMNC))
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    NASNetworkScan
		wantErr bool
	}{
		{name: "no optional fields"},
		{
			name: "visible LTE and NR networks",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVScanNetworks, networks),
				tlv.Bytes(nasTLVScanNameSources, nameSources),
				tlv.Bytes(nasTLVScanRadioAccess, radios),
				tlv.Bytes(nasTLVScanMNCThreeDigit, mncDigits),
				tlv.Uint(nasTLVScanResult, uint32(NASNetworkScanSucceeded)),
			},
			want: NASNetworkScan{
				Networks: []NASVisibleNetwork{
					{
						PLMN:            NASPLMN{MCC: 460, MNC: 1, Description: "Carrier", MNCThreeDigits: true, MNCThreeDigitsKnown: true},
						Status:          0x65,
						RadioInterfaces: []NASRadioInterface{NASRadioInterfaceLTE, NASRadioInterfaceNR5G},
						NameSource:      NASNetworkNameSourceNITZ,
						NameSourceKnown: true,
					},
					{
						PLMN:            NASPLMN{MCC: 310, MNC: 260, Description: "Away", MNCThreeDigits: true, MNCThreeDigitsKnown: true},
						Status:          0xAA,
						RadioInterfaces: []NASRadioInterface{NASRadioInterfaceLTE},
						NameSource:      NASNetworkNameSourceMCCMNC,
						NameSourceKnown: true,
					},
				},
				Result:      NASNetworkScanSucceeded,
				ResultKnown: true,
			},
		},
		{name: "network count truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNetworks, []byte{1})}, wantErr: true},
		{name: "network count too large", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNetworks, []byte{nasMaxVisibleNetworks + 1, 0})}, wantErr: true},
		{name: "network entry truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNetworks, []byte{1, 0, 1, 0})}, wantErr: true},
		{name: "network description truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNetworks, []byte{1, 0, 1, 0, 2, 0, 0, 2, 'A'})}, wantErr: true},
		{name: "network trailing data", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNetworks, []byte{0, 0, 1})}, wantErr: true},
		{name: "name source count truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanNameSources, nil)}, wantErr: true},
		{
			name: "name source length mismatch",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVScanNetworks, encodeNASVisibleNetworksForTest(NASVisibleNetwork{PLMN: NASPLMN{MCC: 1, MNC: 2}})),
				tlv.Bytes(nasTLVScanNameSources, []byte{1, 0}),
			},
			wantErr: true,
		},
		{
			name: "name source count mismatch",
			tlvs: tlv.TLVs{
				tlv.Bytes(nasTLVScanNetworks, encodeNASVisibleNetworksForTest(NASVisibleNetwork{PLMN: NASPLMN{MCC: 1, MNC: 2}})),
				tlv.Bytes(nasTLVScanNameSources, []byte{0}),
			},
			wantErr: true,
		},
		{name: "radio count truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanRadioAccess, []byte{1})}, wantErr: true},
		{name: "radio list truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanRadioAccess, []byte{1, 0, 1})}, wantErr: true},
		{name: "MNC digit count truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanMNCThreeDigit, []byte{1})}, wantErr: true},
		{name: "MNC digit list truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanMNCThreeDigit, []byte{1, 0, 1})}, wantErr: true},
		{name: "result truncated", tlvs: tlv.TLVs{tlv.Bytes(nasTLVScanResult, make([]byte, 3))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASNetworkScan
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
			if !equalNASNetworkScan(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASNetworkStatusFields(t *testing.T) {
	tests := []struct {
		name      string
		status    NASNetworkStatus
		inUse     NASNetworkInUseStatus
		roaming   NASNetworkRoamingStatus
		forbidden NASNetworkForbiddenStatus
		preferred NASNetworkPreferredStatus
	}{
		{
			name: "current home allowed preferred", status: 0x65,
			inUse: NASNetworkInUseCurrent, roaming: NASNetworkRoamingHome,
			forbidden: NASNetworkAllowed, preferred: NASNetworkPreferred,
		},
		{
			name: "available roaming allowed not preferred", status: 0xAA,
			inUse: NASNetworkInUseAvailable, roaming: NASNetworkRoaming,
			forbidden: NASNetworkAllowed, preferred: NASNetworkNotPreferred,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status.InUse() != tt.inUse || tt.status.Roaming() != tt.roaming ||
				tt.status.Forbidden() != tt.forbidden || tt.status.Preferred() != tt.preferred {
				t.Fatalf("status 0x%02X decoded as %d/%d/%d/%d", tt.status, tt.status.InUse(), tt.status.Roaming(), tt.status.Forbidden(), tt.status.Preferred())
			}
		})
	}
}

func TestNASInitiateNetworkRegisterRequest(t *testing.T) {
	duration := NASChangePermanent
	tests := []struct {
		name         string
		registration NASNetworkRegistration
		check        func(*testing.T, Request)
		wantErr      bool
	}{
		{
			name:         "automatic",
			registration: NASNetworkRegistration{Action: NASRegisterAutomatically},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, nasTLVRegisterAction, []byte{1})
				if len(req.TLVs) != 1 {
					t.Fatalf("TLVs len = %d, want 1", len(req.TLVs))
				}
			},
		},
		{
			name: "manual",
			registration: NASNetworkRegistration{
				Action: NASRegisterManually,
				Manual: &NASManualRegistration{
					PLMN:           NASPLMN{MCC: 460, MNC: 1, MNCThreeDigits: true, MNCThreeDigitsKnown: true},
					RadioInterface: NASRadioInterfaceLTE,
				},
				ChangeDuration: &duration,
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, nasTLVRegisterAction, []byte{2})
				assertTLV(t, req.TLVs, nasTLVRegisterManualNetwork, []byte{0xCC, 0x01, 1, 0, 8})
				assertTLV(t, req.TLVs, nasTLVRegisterDuration, []byte{1})
				assertTLV(t, req.TLVs, nasTLVRegisterMNCThreeDigit, []byte{1})
			},
		},
		{name: "action out of range", registration: NASNetworkRegistration{}, wantErr: true},
		{name: "manual network missing", registration: NASNetworkRegistration{Action: NASRegisterManually}, wantErr: true},
		{
			name: "PLMN out of range",
			registration: NASNetworkRegistration{
				Action: NASRegisterManually,
				Manual: &NASManualRegistration{PLMN: NASPLMN{MNC: 1000}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (NASInitiateNetworkRegisterRequest{
				ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Registration: tt.registration,
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
				req.MessageID != MessageNASInitiateNetworkRegister || req.Timeout != 3*time.Second {
				t.Fatalf("Request() = %+v", req)
			}
			tt.check(t, req)
		})
	}
}

func TestClientNASNetworkMessageMapping(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
		want MessageID
		resp Response
	}{
		{
			name: "network scan",
			call: func(c *Client) error {
				_, err := c.NetworkScan(context.Background(), NASNetworkScanConfig{})
				return err
			},
			want: MessageNASPerformNetworkScan,
			resp: successResponse(MessageNASPerformNetworkScan),
		},
		{
			name: "network register",
			call: func(c *Client) error {
				return c.RegisterNetwork(context.Background(), NASNetworkRegistration{Action: NASRegisterAutomatically})
			},
			want: MessageNASInitiateNetworkRegister,
			resp: successResponse(MessageNASInitiateNetworkRegister),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceNAS || req.ClientID != 7 || req.MessageID != tt.want {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want NAS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.want)
					}
					if tt.want == MessageNASPerformNetworkScan && req.Timeout != DefaultNASNetworkScanTimeout {
						t.Fatalf("scan timeout = %v, want %v", req.Timeout, DefaultNASNetworkScanTimeout)
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

func encodeNASVisibleNetworksForTest(networks ...NASVisibleNetwork) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(networks)))
	for _, network := range networks {
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, network.PLMN.MNC)
		value = append(value, byte(network.Status), byte(len(network.PLMN.Description)))
		value = append(value, network.PLMN.Description...)
	}
	return value
}

type nasNetworkRadioForTest struct {
	PLMN  NASPLMN
	Radio NASRadioInterface
}

func encodeNASNetworkRadiosForTest(radios ...nasNetworkRadioForTest) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(radios)))
	for _, radio := range radios {
		value = binary.LittleEndian.AppendUint16(value, radio.PLMN.MCC)
		value = binary.LittleEndian.AppendUint16(value, radio.PLMN.MNC)
		value = append(value, byte(radio.Radio))
	}
	return value
}

func encodeNASNetworkMNCDigitsForTest(plmns ...NASPLMN) []byte {
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(plmns)))
	for _, plmn := range plmns {
		value = binary.LittleEndian.AppendUint16(value, plmn.MCC)
		value = binary.LittleEndian.AppendUint16(value, plmn.MNC)
		value = append(value, boolByte(plmn.MNCThreeDigits))
	}
	return value
}

func equalNASNetworkScan(got, want NASNetworkScan) bool {
	if got.Result != want.Result || got.ResultKnown != want.ResultKnown || len(got.Networks) != len(want.Networks) {
		return false
	}
	for i := range got.Networks {
		if got.Networks[i].PLMN != want.Networks[i].PLMN ||
			got.Networks[i].Status != want.Networks[i].Status ||
			!slices.Equal(got.Networks[i].RadioInterfaces, want.Networks[i].RadioInterfaces) ||
			got.Networks[i].NameSource != want.Networks[i].NameSource ||
			got.Networks[i].NameSourceKnown != want.Networks[i].NameSourceKnown {
			return false
		}
	}
	return true
}
