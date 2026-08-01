package qcom

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSStorageRequests(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (Request, error)
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "read user data",
			build: func() (Request, error) {
				return (DMSReadUserDataRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSReadUserData,
		},
		{
			name: "write user data",
			build: func() (Request, error) {
				return (DMSWriteUserDataRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second, Data: []byte{1, 2, 3},
				}).Request()
			},
			message: MessageDMSWriteUserData,
			wantTLV: map[byte][]byte{0x01: {3, 0, 1, 2, 3}},
		},
		{
			name: "read ERI file",
			build: func() (Request, error) {
				return (DMSReadERIFileRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSReadERIFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := tt.build()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if request.Service != ServiceDMS || request.ClientID != 7 || request.TransactionID != 9 || request.MessageID != tt.message {
				t.Fatalf("Request() = %+v", request)
			}
			if request.Timeout != time.Second || len(request.TLVs) != len(tt.wantTLV) {
				t.Fatalf("Request() timeout = %v, TLVs = %v", request.Timeout, request.TLVs)
			}
			for kind, want := range tt.wantTLV {
				assertTLV(t, request.TLVs, kind, want)
			}
		})
	}
}

func TestDMSWriteUserDataRequestRejectsOversizedPayload(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "513 bytes", data: make([]byte, dmsUserDataMax+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := (DMSWriteUserDataRequest{Data: tt.data}).Request(); err == nil {
				t.Fatal("Request() error = nil")
			}
		})
	}
}

func TestDMSPersistentDataResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		max     int
		kind    string
		tlvs    tlv.TLVs
		want    []byte
		wantErr string
	}{
		{
			name: "user data", max: dmsUserDataMax, kind: "user data",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{3, 0, 1, 2, 3})},
			want: []byte{1, 2, 3},
		},
		{
			name: "empty ERI", max: dmsERIFileMax, kind: "ERI file",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{0, 0})}, want: []byte{},
		},
		{name: "missing TLV", max: dmsUserDataMax, kind: "user data", wantErr: "missing"},
		{
			name: "length missing", max: dmsUserDataMax, kind: "user data",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}, wantErr: "length",
		},
		{
			name: "length exceeds max", max: 2, kind: "user data",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{3, 0, 1, 2, 3})}, wantErr: "exceeds",
		},
		{
			name: "truncated", max: dmsUserDataMax, kind: "user data",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{3, 0, 1})}, wantErr: "value length",
		},
		{
			name: "trailing data", max: dmsUserDataMax, kind: "user data",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0, 1, 2})}, wantErr: "value length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DMSPersistentDataResponse{max: tt.max}
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got.Data, tt.want) {
				t.Fatalf("data = %v, want %v", got.Data, tt.want)
			}
		})
	}
}

func TestClientDMSStorage(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		check   func(*testing.T, Request)
		resp    Response
		run     func(context.Context, *Client) error
	}{
		{
			name: "read user data", message: MessageDMSReadUserData,
			check: func(t *testing.T, req Request) {},
			resp:  successResponse(MessageDMSReadUserData, tlv.Bytes(0x01, []byte{2, 0, 1, 2})),
			run: func(ctx context.Context, client *Client) error {
				data, err := client.DMSUserData(ctx)
				if err == nil && !reflect.DeepEqual(data, []byte{1, 2}) {
					t.Fatalf("DMSUserData() = %v", data)
				}
				return err
			},
		},
		{
			name: "write user data", message: MessageDMSWriteUserData,
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{2, 0, 1, 2}) },
			resp:  successResponse(MessageDMSWriteUserData),
			run: func(ctx context.Context, client *Client) error {
				return client.DMSWriteUserData(ctx, []byte{1, 2})
			},
		},
		{
			name: "read ERI file", message: MessageDMSReadERIFile,
			check: func(t *testing.T, req Request) {},
			resp:  successResponse(MessageDMSReadERIFile, tlv.Bytes(0x01, []byte{2, 0, 3, 4})),
			run: func(ctx context.Context, client *Client) error {
				data, err := client.DMSERIFile(ctx)
				if err == nil && !reflect.DeepEqual(data, []byte{3, 4}) {
					t.Fatalf("DMSERIFile() = %v", data)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDMS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = %+v", req)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("client call error = %v", err)
			}
		})
	}
}
