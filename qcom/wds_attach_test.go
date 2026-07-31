package qcom

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSLTEAttachRequests(t *testing.T) {
	action := WDSAttachPDNListDetachOrDisconnect
	tests := []struct {
		name    string
		build   func() (Request, error)
		wantID  MessageID
		wantTLV map[byte][]byte
		wantErr string
	}{
		{
			name: "get parameters",
			build: func() (Request, error) {
				return (WDSGetLTEAttachParametersRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			wantID: MessageWDSGetLTEAttachParameters,
		},
		{
			name: "get PDN list",
			build: func() (Request, error) {
				return (WDSGetLTEAttachPDNListRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			wantID: MessageWDSGetLTEAttachPDNList,
		},
		{
			name: "set PDN list",
			build: func() (Request, error) {
				return (WDSSetLTEAttachPDNListRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Profiles: []uint16{1, 0x1234}, Action: &action,
				}).Request()
			},
			wantID: MessageWDSSetLTEAttachPDNList,
			wantTLV: map[byte][]byte{
				0x01: {2, 1, 0, 0x34, 0x12},
				0x10: {2, 0, 0, 0},
			},
		},
		{
			name: "clear PDN list",
			build: func() (Request, error) {
				return (WDSSetLTEAttachPDNListRequest{}).Request()
			},
			wantID:  MessageWDSSetLTEAttachPDNList,
			wantTLV: map[byte][]byte{0x01: {0}},
		},
		{
			name: "too many profiles",
			build: func() (Request, error) {
				return (WDSSetLTEAttachPDNListRequest{Profiles: make([]uint16, wdsMaxLTEAttachPDNs+1)}).Request()
			},
			wantErr: "exceeds maximum",
		},
		{
			name: "invalid action",
			build: func() (Request, error) {
				invalid := WDSAttachPDNListAction(0)
				return (WDSSetLTEAttachPDNListRequest{Action: &invalid}).Request()
			},
			wantErr: "action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceWDS || got.MessageID != tt.wantID {
				t.Fatalf("Request() = %+v", got)
			}
			if len(got.TLVs) != len(tt.wantTLV) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLV))
			}
			for typ, want := range tt.wantTLV {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestWDSGetLTEAttachParametersResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSLTEAttachParameters
		wantErr bool
	}{
		{name: "empty", want: WDSLTEAttachParameters{}},
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte("internet")),
				tlv.Uint(0x11, uint8(WDSIPSupportIPv4v6)),
				tlv.Uint(0x12, uint8(1)),
			},
			want: WDSLTEAttachParameters{
				APN: "internet", APNKnown: true, IPSupport: WDSIPSupportIPv4v6,
				IPSupportKnown: true, OTAAttachPerformed: true, OTAAttachKnown: true,
			},
		},
		{name: "IP support length", tlvs: tlv.TLVs{tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "OTA attach length", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 2})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetLTEAttachParametersResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got.Parameters, tt.want) {
				t.Fatalf("parameters = %+v, want %+v", got.Parameters, tt.want)
			}
		})
	}
}

func TestWDSGetLTEAttachPDNListResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSLTEAttachPDNList
		wantErr bool
	}{
		{name: "empty response", want: WDSLTEAttachPDNList{}},
		{
			name: "current and pending",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{2, 1, 0, 2, 0}),
				tlv.Bytes(0x11, []byte{1, 3, 0}),
			},
			want: WDSLTEAttachPDNList{
				Current: []uint16{1, 2}, CurrentKnown: true,
				Pending: []uint16{3}, PendingKnown: true,
			},
		},
		{
			name: "empty known lists",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0}), tlv.Bytes(0x11, []byte{0})},
			want: WDSLTEAttachPDNList{Current: []uint16{}, CurrentKnown: true, Pending: []uint16{}, PendingKnown: true},
		},
		{name: "current count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "current truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1})}, wantErr: true},
		{name: "pending trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{0, 1})}, wantErr: true},
		{name: "count exceeds maximum", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{wdsMaxLTEAttachPDNs + 1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetLTEAttachPDNListResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(got.List, tt.want) {
				t.Fatalf("list = %+v, want %+v", got.List, tt.want)
			}
		})
	}
}

func TestClientWDSLTEAttach(t *testing.T) {
	action := WDSAttachPDNListNoAction
	tests := []struct {
		name  string
		calls []transportCall
		run   func(context.Context, *Client) error
	}{
		{
			name: "get parameters",
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSGetLTEAttachParameters || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(MessageWDSGetLTEAttachParameters, tlv.Bytes(0x10, []byte("ims"))),
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.WDSLTEAttachParameters(ctx)
				if err == nil && (!got.APNKnown || got.APN != "ims") {
					t.Fatalf("parameters = %+v", got)
				}
				return err
			},
		},
		{
			name: "get PDN list",
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSGetLTEAttachPDNList || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(MessageWDSGetLTEAttachPDNList, tlv.Bytes(0x10, []byte{1, 7, 0})),
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.WDSLTEAttachPDNList(ctx)
				if err == nil && (!got.CurrentKnown || !slices.Equal(got.Current, []uint16{7})) {
					t.Fatalf("list = %+v", got)
				}
				return err
			},
		},
		{
			name: "set PDN list",
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageWDSSetLTEAttachPDNList {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{2, 7, 0, 8, 0})
					assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 0, 0})
				},
				resp: successResponse(MessageWDSSetLTEAttachPDNList),
			}},
			run: func(ctx context.Context, client *Client) error {
				return client.WDSSetLTEAttachPDNList(ctx, []uint16{7, 8}, &action)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}
