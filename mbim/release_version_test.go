package mbim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func Test3GPPReleaseVersionTLV(t *testing.T) {
	tests := []struct {
		name    string
		version ThreeGPPReleaseVersion
	}{
		{name: "pre-release 15", version: ThreeGPPReleaseVersionPre15},
		{name: "release 15", version: ThreeGPPReleaseVersion15},
		{name: "release 16", version: ThreeGPPReleaseVersion16},
		{name: "unknown", version: ThreeGPPReleaseVersionUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlv, err := New3GPPReleaseVersionTLV(tt.version)
			if err != nil {
				t.Fatalf("New3GPPReleaseVersionTLV() error = %v", err)
			}
			wantData := binary.LittleEndian.AppendUint32(nil, uint32(tt.version))
			if tlv.Type != TLVType3GPPReleaseVersion || !bytes.Equal(tlv.Data, wantData) {
				t.Fatalf("New3GPPReleaseVersionTLV() = %+v, want type %d data %x", tlv, TLVType3GPPReleaseVersion, wantData)
			}

			var got ThreeGPPReleaseVersion
			if err := got.UnmarshalTLV(tlv); err != nil {
				t.Fatalf("UnmarshalTLV() error = %v", err)
			}
			if got != tt.version {
				t.Fatalf("UnmarshalTLV() = %d, want %d", got, tt.version)
			}
		})
	}
}

func Test3GPPReleaseVersionTLVRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		version *ThreeGPPReleaseVersion
		tlv     TLV
	}{
		{name: "reserved encode value", version: threeGPPReleaseVersionPointer(17)},
		{name: "wrong type", tlv: TLV{Type: TLVTypePCO, Data: make([]byte, 4)}},
		{name: "truncated", tlv: TLV{Type: TLVType3GPPReleaseVersion, Data: make([]byte, 3)}},
		{name: "trailing data", tlv: TLV{Type: TLVType3GPPReleaseVersion, Data: make([]byte, 5)}},
		{
			name: "reserved parse value",
			tlv: TLV{
				Type: TLVType3GPPReleaseVersion,
				Data: binary.LittleEndian.AppendUint32(nil, 17),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.version != nil {
				if _, err := New3GPPReleaseVersionTLV(*tt.version); err == nil {
					t.Fatal("New3GPPReleaseVersionTLV() error = nil, want non-nil")
				}
				return
			}
			var got ThreeGPPReleaseVersion
			if err := got.UnmarshalTLV(tt.tlv); err == nil {
				t.Fatal("UnmarshalTLV() error = nil, want non-nil")
			}
		})
	}
}

func threeGPPReleaseVersionPointer(value ThreeGPPReleaseVersion) *ThreeGPPReleaseVersion {
	return &value
}

func TestClientVersion(t *testing.T) {
	tests := []struct {
		name          string
		mbimExVersion uint16
	}{
		{name: "MBIMEx 1", mbimExVersion: mbimExVersion10},
		{name: "MBIMEx 4", mbimExVersion: mbimExVersion40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{mbimExVersion: tt.mbimExVersion}
			got := client.Version()
			if got.MBIMVersion != mbimVersion10 || got.MBIMExVersion != tt.mbimExVersion {
				t.Fatalf("Version() = {%#x %#x}, want {%#x %#x}", got.MBIMVersion, got.MBIMExVersion, mbimVersion10, tt.mbimExVersion)
			}
		})
	}
}
