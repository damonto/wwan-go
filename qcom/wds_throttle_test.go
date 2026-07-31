package qcom

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDSThrottleRequests(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "PDN throttle",
			request: (WDSGetPDNThrottleInfoRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second,
				NetworkType: WDSDataSystem3GPP2,
			}).Request(),
			message: MessageWDSGetPDNThrottleInfo,
			wantTLV: map[byte][]byte{0x01: {byte(WDSDataSystem3GPP2)}},
		},
		{
			name: "maximum LTE attach PDNs",
			request: (WDSGetMaxLTEAttachPDNNumberRequest{
				ClientID: 7, TransactionID: 9, Timeout: time.Second,
			}).Request(),
			message: MessageWDSGetMaxLTEAttachPDNNumber,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Service != ServiceWDS || tt.request.ClientID != 7 || tt.request.TransactionID != 9 || tt.request.MessageID != tt.message {
				t.Fatalf("Request() = %+v", tt.request)
			}
			if tt.request.Timeout != time.Second || len(tt.request.TLVs) != len(tt.wantTLV) {
				t.Fatalf("Request() timeout = %v, TLVs = %v", tt.request.Timeout, tt.request.TLVs)
			}
			for kind, want := range tt.wantTLV {
				assertTLV(t, tt.request.TLVs, kind, want)
			}
		})
	}
}

func TestWDSGetPDNThrottleInfoResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDSPDNThrottleInfo
		wantErr string
	}{
		{name: "empty response"},
		{
			name: "all generations",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{
					1,
					1, 0, 100, 0, 0, 0, 200, 0, 0, 0, 3, 'i', 'm', 's',
				}),
				tlv.Bytes(0x11, []byte{
					1,
					1, 1, 1, 10, 0, 0, 0, 20, 0, 0, 0, 30, 0, 0, 0,
					8, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't',
				}),
				tlv.Bytes(0x12, []byte{
					1,
					1, 0, 0x13, 0x00, 0x62, 3, 'i', 'm', 's',
				}),
				tlv.Uint(0x13, uint16(0x1234)),
			},
			want: WDSPDNThrottleInfo{
				Entries: []WDSPDNThrottleEntry{{
					APN: "ims", IPv4Throttled: true,
					IPv4RemainingMilliseconds: 100, IPv6RemainingMilliseconds: 200,
				}},
				EntriesKnown: true,
				Extended: []WDSPDNThrottleExtendedEntry{{
					WDSPDNThrottleEntry: WDSPDNThrottleEntry{
						APN: "internet", IPv4Throttled: true, IPv6Throttled: true,
						IPv4RemainingMilliseconds: 10, IPv6RemainingMilliseconds: 20,
					},
					NonIPThrottled: true, NonIPRemainingMilliseconds: 30,
				}},
				ExtendedKnown: true,
				Additional: []WDSPDNThrottleAdditionalEntry{{
					APN: "ims", Emergency: true, ThrottledPLMN: [3]byte{0x13, 0x00, 0x62},
				}},
				AdditionalKnown: true,
				TransactionID:   0x1234, TransactionIDKnown: true,
			},
		},
		{
			name: "known empty arrays",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0}),
				tlv.Bytes(0x11, []byte{0}),
				tlv.Bytes(0x12, []byte{0}),
			},
			want: WDSPDNThrottleInfo{
				Entries: []WDSPDNThrottleEntry{}, EntriesKnown: true,
				Extended: []WDSPDNThrottleExtendedEntry{}, ExtendedKnown: true,
				Additional: []WDSPDNThrottleAdditionalEntry{}, AdditionalKnown: true,
			},
		},
		{name: "base count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: "count"},
		{name: "base count too large", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{wdsMaxPDNThrottleEntries + 1})}, wantErr: "exceeds"},
		{name: "base entry truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0})}, wantErr: "truncated"},
		{
			name: "base APN truncated",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{
				1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 'x',
			})},
			wantErr: "APN",
		},
		{name: "base trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 1})}, wantErr: "trailing"},
		{name: "extended entry truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0})}, wantErr: "extended"},
		{name: "additional entry truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0})}, wantErr: "additional"},
		{name: "transaction length", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1})}, wantErr: "transaction ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetPDNThrottleInfoResponse
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
			if !reflect.DeepEqual(got.Info, tt.want) {
				t.Fatalf("info = %+v, want %+v", got.Info, tt.want)
			}
		})
	}
}

func TestWDSGetMaxLTEAttachPDNNumberResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    uint8
		wantErr string
	}{
		{name: "eight", tlvs: tlv.TLVs{tlv.Uint(0x10, uint8(8))}, want: 8},
		{name: "missing", wantErr: "missing"},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetMaxLTEAttachPDNNumberResponse
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
			if got.Maximum != tt.want {
				t.Fatalf("maximum = %d, want %d", got.Maximum, tt.want)
			}
		})
	}
}

func TestClientWDSThrottleAndAttachLimit(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		check   func(*testing.T, Request)
		resp    Response
		run     func(context.Context, *Client) error
	}{
		{
			name:    "PDN throttle",
			message: MessageWDSGetPDNThrottleInfo,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(WDSDataSystem3GPP)})
			},
			resp: successResponse(MessageWDSGetPDNThrottleInfo, tlv.Bytes(0x10, []byte{0})),
			run: func(ctx context.Context, client *Client) error {
				info, err := client.WDSPDNThrottleInfo(ctx, WDSDataSystem3GPP)
				if err == nil && (!info.EntriesKnown || len(info.Entries) != 0) {
					t.Fatalf("WDSPDNThrottleInfo() = %+v", info)
				}
				return err
			},
		},
		{
			name:    "maximum LTE attach PDNs",
			message: MessageWDSGetMaxLTEAttachPDNNumber,
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("request TLVs = %v, want empty", req.TLVs)
				}
			},
			resp: successResponse(MessageWDSGetMaxLTEAttachPDNNumber, tlv.Uint(0x10, uint8(8))),
			run: func(ctx context.Context, client *Client) error {
				maximum, err := client.WDSMaxLTEAttachPDNs(ctx)
				if err == nil && maximum != 8 {
					t.Fatalf("WDSMaxLTEAttachPDNs() = %d, want 8", maximum)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWDS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = %+v", req)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("client call error = %v", err)
			}
		})
	}
}

func TestClientWDSPDNThrottleInfoRejectsInvalidNetworkType(t *testing.T) {
	tests := []struct {
		name        string
		networkType WDSDataSystemNetworkType
	}{
		{name: "unknown", networkType: WDSDataSystemNetworkType(2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWDS: 7}}
			if _, err := client.WDSPDNThrottleInfo(context.Background(), tt.networkType); err == nil {
				t.Fatal("WDSPDNThrottleInfo() error = nil")
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
		})
	}
}
