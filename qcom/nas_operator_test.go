package qcom

import (
	"context"
	"encoding/binary"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestEncodeNASPLMNNameRequest(t *testing.T) {
	enabled := true
	disabled := false
	csgID := uint32(0x10203040)
	radio := NASRadioInterfaceLTE
	tests := []struct {
		name string
		req  NASPLMNNameRequest
		want map[byte][]byte
	}{
		{
			name: "required PLMN only",
			req:  NASPLMNNameRequest{PLMN: NASPLMN{MCC: 460, MNC: 1}},
			want: map[byte][]byte{0x01: {0xCC, 0x01, 0x01, 0x00}},
		},
		{
			name: "all options",
			req: NASPLMNNameRequest{
				PLMN: NASPLMN{
					MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true,
				},
				SuppressSIMError:   &enabled,
				AlwaysSend:         &disabled,
				UseStaticTableOnly: &enabled,
				CSGID:              &csgID,
				RadioInterface:     &radio,
				SendAllInformation: &enabled,
			},
			want: map[byte][]byte{
				0x01: {0x36, 0x01, 0x04, 0x01},
				0x10: {1},
				0x11: {1},
				0x12: {0},
				0x13: {1},
				0x14: {0x40, 0x30, 0x20, 0x10},
				0x15: {byte(NASRadioInterfaceLTE)},
				0x16: {1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeNASPLMNNameRequest(tt.req)
			if len(got) != len(tt.want) {
				t.Fatalf("TLVs len = %d, want %d", len(got), len(tt.want))
			}
			for typ, want := range tt.want {
				assertTLV(t, got, typ, want)
			}
		})
	}
}

func TestPLMNName(t *testing.T) {
	tests := []struct {
		name    string
		req     NASPLMNNameRequest
		calls   []transportCall
		want    NASPLMNName
		wantErr string
	}{
		{
			name: "query",
			req:  NASPLMNNameRequest{PLMN: NASPLMN{MCC: 460, MNC: 1}},
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageNASGetPLMNName {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageNASGetPLMNName)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{0xCC, 0x01, 0x01, 0x00})
				},
				resp: successResponse(MessageNASGetPLMNName, tlv.Uint(0x12, uint32(NASTriTrue))),
			}},
			want: NASPLMNName{HomeNetwork: NASTriTrue, HomeNetworkKnown: true},
		},
		{
			name:    "MCC out of range",
			req:     NASPLMNNameRequest{PLMN: NASPLMN{MCC: 1000, MNC: 1}},
			wantErr: "out of range",
		},
		{
			name:    "MNC out of range",
			req:     NASPLMNNameRequest{PLMN: NASPLMN{MCC: 460, MNC: 1000}},
			wantErr: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceNAS: 7}}
			got, err := client.PLMNName(context.Background(), tt.req)
			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("PLMNName() error = %v, want text %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("PLMNName() error = %v", err)
			case !reflect.DeepEqual(got, tt.want):
				t.Fatalf("PLMNName() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASPLMNNameUnmarshalTLVs(t *testing.T) {
	names := []byte{
		byte(NASNetworkDescriptionGSM7), 3, 'S', 'P', 'N',
		byte(NASNetworkDescriptionUCS2LE), byte(NASCountryInitialsAdd), 0, 4, 'C', 0, 'M', 0,
		byte(NASNetworkDescriptionGSM7), byte(NASCountryInitialsDoNotAdd), 2, 4, 'L', 'O', 'N', 'G',
	}
	localized := []byte{1, 2, 0x2D, 0x4E, 0xFD, 0x56, 1, 0x2D, 0x4E}
	localized = binary.LittleEndian.AppendUint32(localized, uint32(NASPLMNLanguageChineseSimplified))
	additional := []byte{2, 0x21, 0x00, 0x22, 0x00}
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want NASPLMNName
	}{
		{name: "empty", want: NASPLMNName{}},
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, names),
				tlv.Bytes(0x11, []byte{1, 0, 0, 0, 2, 0, 0, 0}),
				tlv.Uint(0x12, uint32(NASTriTrue)),
				tlv.Bytes(0x13, localized),
				tlv.Bytes(0x14, additional),
				tlv.Uint(0x15, uint32(NASNetworkNameSourceSE13)),
			},
			want: NASPLMNName{
				ServiceProvider: NASEncodedNetworkName{Encoding: NASNetworkDescriptionGSM7, Data: []byte("SPN")},
				Short: NASEncodedNetworkName{
					Encoding: NASNetworkDescriptionUCS2LE, CountryInitials: NASCountryInitialsAdd,
					Data: []byte{'C', 0, 'M', 0},
				},
				Long: NASEncodedNetworkName{
					Encoding: NASNetworkDescriptionGSM7, CountryInitials: NASCountryInitialsDoNotAdd,
					SpareBits: 2, Data: []byte("LONG"),
				},
				NamesKnown:             true,
				DisplayServiceProvider: NASTriTrue,
				DisplayPLMN:            NASTriUnknown,
				DisplayBitsKnown:       true,
				HomeNetwork:            NASTriTrue,
				HomeNetworkKnown:       true,
				Localized: []NASLocalizedPLMNName{{
					LongName: []uint16{0x4E2D, 0x56FD}, ShortName: []uint16{0x4E2D},
					Language: NASPLMNLanguageChineseSimplified,
				}},
				LocalizedKnown:      true,
				AdditionalInfo:      []uint16{0x21, 0x22},
				AdditionalInfoKnown: true,
				Source:              NASNetworkNameSourceSE13,
				SourceKnown:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASPLMNName
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASPLMNNameRejectsMalformedTLVs(t *testing.T) {
	validNames := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	tests := []struct {
		name string
		tlv  tlv.TLV
	}{
		{name: "service provider metadata", tlv: tlv.Bytes(0x10, []byte{0})},
		{name: "service provider data", tlv: tlv.Bytes(0x10, []byte{0, 2, 'A'})},
		{name: "short name metadata", tlv: tlv.Bytes(0x10, []byte{0, 0, 0})},
		{name: "long name metadata", tlv: tlv.Bytes(0x10, []byte{0, 0, 0, 0, 0})},
		{name: "name trailing data", tlv: tlv.Bytes(0x10, append(validNames, 1))},
		{name: "display bits", tlv: tlv.Bytes(0x11, make([]byte, 7))},
		{name: "home network", tlv: tlv.Bytes(0x12, make([]byte, 3))},
		{name: "localized count", tlv: tlv.Bytes(0x13, nil)},
		{name: "localized long name", tlv: tlv.Bytes(0x13, []byte{1, 1, 0})},
		{name: "localized short name", tlv: tlv.Bytes(0x13, []byte{1, 0})},
		{name: "localized language", tlv: tlv.Bytes(0x13, []byte{1, 0, 0, 1})},
		{name: "localized trailing data", tlv: tlv.Bytes(0x13, []byte{0, 1})},
		{name: "additional information count", tlv: tlv.Bytes(0x14, nil)},
		{name: "additional information data", tlv: tlv.Bytes(0x14, []byte{1, 0})},
		{name: "additional information trailing data", tlv: tlv.Bytes(0x14, []byte{0, 1})},
		{name: "source", tlv: tlv.Bytes(0x15, make([]byte, 3))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASPLMNName
			if err := got.UnmarshalTLVs(tlv.TLVs{tt.tlv}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want malformed TLV error")
			}
		})
	}
}

func TestNASNetworkRejectUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want NASNetworkReject
	}{
		{
			name: "required fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x01, uint8(NASRadioInterfaceLTE)),
				tlv.Uint(0x02, uint8(NASNetworkServicePS)),
				tlv.Uint(0x03, uint8(NASRejectPLMNNotAllowed)),
			},
			want: NASNetworkReject{
				RadioInterface: NASRadioInterfaceLTE,
				ServiceDomain:  NASNetworkServicePS,
				Cause:          NASRejectPLMNNotAllowed,
			},
		},
		{
			name: "optional fields",
			tlvs: tlv.TLVs{
				tlv.Uint(0x01, uint8(NASRadioInterfaceNR5G)),
				tlv.Uint(0x02, uint8(NASNetworkServiceCSPS)),
				tlv.Uint(0x03, uint8(NASRejectCongestion)),
				tlv.Bytes(0x10, []byte{0x36, 0x01, 0x04, 0x01, 1}),
				tlv.Uint(0x11, uint32(0x10203040)),
				tlv.Uint(0x12, uint32(2)),
			},
			want: NASNetworkReject{
				RadioInterface: NASRadioInterfaceNR5G,
				ServiceDomain:  NASNetworkServiceCSPS,
				Cause:          NASRejectCongestion,
				PLMN:           NASPLMN{MCC: 310, MNC: 260, MNCThreeDigits: true, MNCThreeDigitsKnown: true},
				PLMNKnown:      true,
				CSGID:          0x10203040, CSGIDKnown: true,
				CIoTLTEMode: 2, CIoTLTEModeKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASNetworkReject
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNASNetworkRejectRejectsMalformedTLVs(t *testing.T) {
	required := tlv.TLVs{
		tlv.Uint(0x01, uint8(NASRadioInterfaceLTE)),
		tlv.Uint(0x02, uint8(NASNetworkServicePS)),
		tlv.Uint(0x03, uint8(NASRejectCongestion)),
	}
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{name: "radio missing", tlvs: required[1:]},
		{name: "service domain missing", tlvs: tlv.TLVs{required[0], required[2]}},
		{name: "cause missing", tlvs: required[:2]},
		{name: "radio length", tlvs: append(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, required[1:]...)},
		{name: "PLMN length", tlvs: append(slices.Clone(required), tlv.Bytes(0x10, make([]byte, 4)))},
		{name: "CSG ID length", tlvs: append(slices.Clone(required), tlv.Bytes(0x11, make([]byte, 3)))},
		{name: "CIoT LTE mode length", tlvs: append(slices.Clone(required), tlv.Bytes(0x12, make([]byte, 3)))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got NASNetworkReject
			if err := got.UnmarshalTLVs(tt.tlvs); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want malformed TLV error")
			}
		})
	}
}
