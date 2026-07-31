package qcom

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSFirmwarePreferenceEncoding(t *testing.T) {
	var modemID [dmsFirmwareUniqueIDLength]byte
	for i := range modemID {
		modemID[i] = byte(i)
	}
	override := true
	index := uint8(3)
	tests := []struct {
		name string
		info DMSFirmwarePreferenceRequest
		want []byte
	}{
		{
			name: "one image with options",
			info: DMSFirmwarePreferenceRequest{
				Images:            []DMSFirmwareImage{{Type: DMSFirmwareImageModem, UniqueID: modemID, BuildID: "build"}},
				DownloadOverride:  &override,
				ModemStorageIndex: &index,
			},
			want: []byte{1, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 5, 'b', 'u', 'i', 'l', 'd'},
		},
		{
			name: "empty list",
			info: DMSFirmwarePreferenceRequest{},
			want: []byte{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := (DMSSetFirmwarePreferenceRequest{Info: tt.info}).Request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			assertTLV(t, req.TLVs, dmsTLVFirmwareList, tt.want)
			if tt.info.DownloadOverride != nil {
				assertTLV(t, req.TLVs, dmsTLVDownloadOverride, []byte{1})
			}
			if tt.info.ModemStorageIndex != nil {
				assertTLV(t, req.TLVs, dmsTLVModemStorageIndex, []byte{3})
			}
		})
	}
}

func TestDMSFirmwarePreferenceDecoding(t *testing.T) {
	var id [dmsFirmwareUniqueIDLength]byte
	id[0] = 0xAA
	value := append([]byte{1, byte(DMSFirmwareImagePRI)}, id[:]...)
	value = append(value, 4, 'p', 'r', 'i', '1')
	var got DMSGetFirmwarePreferenceResponse
	if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, value)}); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	want := DMSFirmwareImage{Type: DMSFirmwareImagePRI, UniqueID: id, BuildID: "pri1"}
	if len(got.Images) != 1 || got.Images[0] != want {
		t.Fatalf("UnmarshalTLVs() = %+v, want one image %+v", got.Images, want)
	}
}

func TestDMSSetFirmwarePreferenceResponseOptional(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want DMSSetFirmwarePreferenceResponse
	}{
		{name: "no optional list"},
		{
			name: "download list and maximum",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVFirmwareList, []byte{2, 0, 1}),
				tlv.Bytes(dmsTLVMaximumBuildIDLength, []byte{42}),
			},
			want: DMSSetFirmwarePreferenceResponse{
				ImageDownloadList:      []DMSFirmwareImageType{DMSFirmwareImageModem, DMSFirmwareImagePRI},
				ImageDownloadListKnown: true,
				MaximumBuildIDLength:   42,
				MaximumBuildIDKnown:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSSetFirmwarePreferenceResponse
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if len(got.ImageDownloadList) != len(tt.want.ImageDownloadList) ||
				got.ImageDownloadListKnown != tt.want.ImageDownloadListKnown ||
				got.MaximumBuildIDLength != tt.want.MaximumBuildIDLength ||
				got.MaximumBuildIDKnown != tt.want.MaximumBuildIDKnown {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
			for i := range got.ImageDownloadList {
				if got.ImageDownloadList[i] != tt.want.ImageDownloadList[i] {
					t.Fatalf("image %d = %d, want %d", i, got.ImageDownloadList[i], tt.want.ImageDownloadList[i])
				}
			}
		})
	}
}

func TestDMSStoredImageDecoding(t *testing.T) {
	var id [dmsFirmwareUniqueIDLength]byte
	id[15] = 0xFE
	value := []byte{1, byte(DMSFirmwareImageModem), 4, 0, 1, 9, 7}
	value = append(value, id[:]...)
	value = append(value, 5, 'b', 'o', 'o', 't', '1')
	var got DMSListStoredImagesResponse
	if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, value)}); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if len(got.Images) != 1 || len(got.Images[0].Images) != 1 {
		t.Fatalf("UnmarshalTLVs() = %+v, want one image group and image", got)
	}
	image := got.Images[0].Images[0]
	if image.StorageIndex != 9 || image.FailureCount != 7 || image.UniqueID != id || image.BuildID != "boot1" {
		t.Fatalf("stored image = %+v, want index 9/failure 7/build boot1", image)
	}
}

func TestDMSStoredImageInfoEncodingAndDecoding(t *testing.T) {
	var id [dmsFirmwareUniqueIDLength]byte
	id[0] = 1
	request, err := (DMSStoredImageInfoRequest{Image: DMSFirmwareImage{
		Type: DMSFirmwareImagePRI, UniqueID: id, BuildID: "pri",
	}}).Request()
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	want := append([]byte{1}, id[:]...)
	want = append(want, 3, 'p', 'r', 'i')
	assertTLV(t, request.TLVs, dmsTLVFirmwareList, want)

	pri := make([]byte, 36)
	binary.LittleEndian.PutUint32(pri, 77)
	copy(pri[4:], "pri-info")
	var got DMSStoredImageInfo
	if err := got.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(dmsTLVBootVersion, []byte{2, 0, 3, 0}),
		tlv.Bytes(dmsTLVPRIVersion, pri),
		tlv.Bytes(dmsTLVOEMLockID, []byte{4, 3, 2, 1}),
	}); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if got.BootMajorVersion != 2 || got.BootMinorVersion != 3 || !got.BootVersionKnown ||
		got.PRIVersion != 77 || got.PRIInfo != "pri-info" || !got.PRIVersionKnown ||
		got.OEMLockID != 0x01020304 || !got.OEMLockIDKnown {
		t.Fatalf("stored image info = %+v", got)
	}
}

func TestDMSFirmwareValidation(t *testing.T) {
	longBuild := strings.Repeat("x", dmsFirmwareBuildIDMax+1)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "invalid image type",
			call: func() error {
				_, err := (DMSSetFirmwarePreferenceRequest{Info: DMSFirmwarePreferenceRequest{
					Images: []DMSFirmwareImage{{Type: 2}},
				}}).Request()
				return err
			},
		},
		{
			name: "long build ID",
			call: func() error {
				_, err := (DMSSetFirmwarePreferenceRequest{Info: DMSFirmwarePreferenceRequest{
					Images: []DMSFirmwareImage{{BuildID: longBuild}},
				}}).Request()
				return err
			},
		},
		{
			name: "malformed stored list",
			call: func() error {
				var response DMSListStoredImagesResponse
				return response.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, []byte{1})})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("operation error = nil, want non-nil")
			}
		})
	}
}
