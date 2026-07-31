package qcom

import (
	"context"
	"testing"
	"time"
)

func TestDMSDeleteStoredImageRequest(t *testing.T) {
	var uniqueID [dmsFirmwareUniqueIDLength]byte
	for i := range uniqueID {
		uniqueID[i] = byte(i)
	}
	tests := []struct {
		name    string
		image   DMSFirmwareImage
		want    []byte
		wantErr bool
	}{
		{
			name:  "modem image",
			image: DMSFirmwareImage{Type: DMSFirmwareImageModem, UniqueID: uniqueID, BuildID: "build"},
			want:  append(append([]byte{byte(DMSFirmwareImageModem)}, uniqueID[:]...), 5, 'b', 'u', 'i', 'l', 'd'),
		},
		{name: "invalid type", image: DMSFirmwareImage{Type: DMSFirmwareImageType(2)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := (DMSDeleteStoredImageRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second, Image: tt.image,
			}).Request()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Request() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if request.Service != ServiceDMS || request.ClientID != 7 || request.TransactionID != 9 || request.MessageID != MessageDMSDeleteStoredImage || request.Timeout != time.Second {
				t.Fatalf("Request() = %+v", request)
			}
			assertTLV(t, request.TLVs, dmsTLVFirmwareList, tt.want)
		})
	}
}

func TestClientDeleteStoredImage(t *testing.T) {
	var uniqueID [dmsFirmwareUniqueIDLength]byte
	uniqueID[0] = 0xAA
	image := DMSFirmwareImage{Type: DMSFirmwareImagePRI, UniqueID: uniqueID, BuildID: "pri"}
	tests := []struct {
		name string
		resp Response
	}{
		{name: "success", resp: successResponse(MessageDMSDeleteStoredImage)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDMS || req.ClientID != 7 || req.MessageID != MessageDMSDeleteStoredImage {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}
			if err := client.DeleteStoredImage(context.Background(), image); err != nil {
				t.Fatalf("DeleteStoredImage() error = %v", err)
			}
		})
	}
}
