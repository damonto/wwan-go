package qcom

import (
	"context"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestServiceSupportedMessages(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
	}{
		{name: "voice", service: ServiceVoice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != tt.service || req.MessageID != MessageGetSupportedMessages {
						t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
					if len(req.TLVs) != 0 {
						t.Fatalf("TLVs = %+v, want empty", req.TLVs)
					}
				},
				resp: successResponse(MessageGetSupportedMessages, tlv.Bytes(0x10, supportedMessageMask(MessageVoiceDialCall))),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{tt.service: 7}}

			got, err := client.ServiceSupportedMessages(context.Background(), tt.service)
			if err != nil {
				t.Fatalf("ServiceSupportedMessages() error = %v", err)
			}
			if !got.Supports(MessageVoiceDialCall) || got.Supports(MessageVoiceEndCall) {
				t.Fatalf("supported mask = % X", got.Mask())
			}
		})
	}
}

func TestServiceVersions(t *testing.T) {
	tests := []struct {
		name string
		want []ServiceVersion
	}{
		{
			name: "advertised services",
			want: []ServiceVersion{
				{Service: ServiceDMS, Major: 1, Minor: 2},
				{Service: ServiceWDS, Major: 3, Minor: 4},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceControl || req.MessageID != MessageGetVersionInfo {
						t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
					}
				},
				resp: successResponse(MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(tt.want...))),
			}}}
			client := &Client{transport: transport, slot: 1}
			got, err := client.ServiceVersions(context.Background())
			if err != nil {
				t.Fatalf("ServiceVersions() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ServiceVersions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestServiceSupportedMessagesRejectsUndefinedServices(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
	}{
		{name: "control", service: ServiceControl},
		{name: "QOS", service: ServiceQOS},
		{name: "PBM", service: ServicePBM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			if _, err := client.ServiceSupportedMessages(context.Background(), tt.service); err == nil {
				t.Fatal("ServiceSupportedMessages() error = nil, want error")
			}
		})
	}
}

func TestDecodeSupportedMessageMaskRejectsTruncation(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "short value", value: []byte{2, 0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeQMILength16Bytes(tt.value); err == nil {
				t.Fatal("decodeQMILength16Bytes() error = nil, want error")
			}
		})
	}
}

func supportedMessageMask(id MessageID) []byte {
	mask := make([]byte, int(id)/8+1)
	mask[int(id)/8] |= 1 << uint(id%8)
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(mask))), mask...)
}
