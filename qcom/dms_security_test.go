package qcom

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSSecurityRequests(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (Request, error)
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "get user lock state",
			build: func() (Request, error) {
				return (DMSGetUserLockStateRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSGetUserLockState,
		},
		{
			name: "enable user lock",
			build: func() (Request, error) {
				return (DMSSetUserLockStateRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Enabled: true, Code: "1234",
				}).Request()
			},
			message: MessageDMSSetUserLockState,
			wantTLV: map[byte][]byte{0x01: {1, '1', '2', '3', '4'}},
		},
		{
			name: "replace user lock code",
			build: func() (Request, error) {
				return (DMSSetUserLockCodeRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					CurrentCode: "1234", NewCode: "5678",
				}).Request()
			},
			message: MessageDMSSetUserLockCode,
			wantTLV: map[byte][]byte{0x01: []byte("12345678")},
		},
		{
			name: "validate SPC",
			build: func() (Request, error) {
				return (DMSValidateSPCRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second, SPC: "123456",
				}).Request()
			},
			message: MessageDMSValidateSPC,
			wantTLV: map[byte][]byte{0x01: []byte("123456")},
		},
		{
			name: "replace SPC",
			build: func() (Request, error) {
				return (DMSSetSPCRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					CurrentSPC: "123456", NewSPC: "654321",
				}).Request()
			},
			message: MessageDMSSetSPC,
			wantTLV: map[byte][]byte{
				0x01: []byte("123456"),
				0x02: []byte("654321"),
			},
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

func TestDMSSecurityRequestsRejectInvalidCodes(t *testing.T) {
	tests := []struct {
		name    string
		build   func() error
		wantErr string
	}{
		{
			name: "short lock code",
			build: func() error {
				_, err := (DMSSetUserLockStateRequest{Code: "123"}).Request()
				return err
			},
			wantErr: "length",
		},
		{
			name: "non-digit lock code",
			build: func() error {
				_, err := (DMSSetUserLockCodeRequest{CurrentCode: "12a4", NewCode: "5678"}).Request()
				return err
			},
			wantErr: "non-digit",
		},
		{
			name: "short SPC",
			build: func() error {
				_, err := (DMSValidateSPCRequest{SPC: "123"}).Request()
				return err
			},
			wantErr: "length",
		},
		{
			name: "non-digit new SPC",
			build: func() error {
				_, err := (DMSSetSPCRequest{CurrentSPC: "123456", NewSPC: "12345x"}).Request()
				return err
			},
			wantErr: "non-digit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestDMSGetUserLockStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    bool
		wantErr string
	}{
		{name: "enabled", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(1))}, want: true},
		{name: "disabled", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(0))}},
		{name: "missing", wantErr: "missing"},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSGetUserLockStateResponse
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
			if got.Enabled != tt.want {
				t.Fatalf("enabled = %v, want %v", got.Enabled, tt.want)
			}
		})
	}
}

func TestClientDMSSecurity(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		wantTLV map[byte][]byte
		resp    Response
		run     func(context.Context, *Client) error
	}{
		{
			name: "get user lock", message: MessageDMSGetUserLockState,
			resp: successResponse(MessageDMSGetUserLockState, tlv.Uint(0x01, uint8(1))),
			run: func(ctx context.Context, client *Client) error {
				enabled, err := client.DMSUserLockState(ctx)
				if err == nil && !enabled {
					t.Fatal("DMSUserLockState() = false")
				}
				return err
			},
		},
		{
			name: "set user lock", message: MessageDMSSetUserLockState,
			wantTLV: map[byte][]byte{0x01: {1, '1', '2', '3', '4'}},
			resp:    successResponse(MessageDMSSetUserLockState),
			run: func(ctx context.Context, client *Client) error {
				return client.DMSSetUserLockState(ctx, true, "1234")
			},
		},
		{
			name: "replace user lock code", message: MessageDMSSetUserLockCode,
			wantTLV: map[byte][]byte{0x01: []byte("12345678")},
			resp:    successResponse(MessageDMSSetUserLockCode),
			run: func(ctx context.Context, client *Client) error {
				return client.DMSSetUserLockCode(ctx, "1234", "5678")
			},
		},
		{
			name: "validate SPC", message: MessageDMSValidateSPC,
			wantTLV: map[byte][]byte{0x01: []byte("123456")},
			resp:    successResponse(MessageDMSValidateSPC),
			run: func(ctx context.Context, client *Client) error {
				return client.DMSValidateSPC(ctx, "123456")
			},
		},
		{
			name: "replace SPC", message: MessageDMSSetSPC,
			wantTLV: map[byte][]byte{0x01: []byte("123456"), 0x02: []byte("654321")},
			resp:    successResponse(MessageDMSSetSPC),
			run: func(ctx context.Context, client *Client) error {
				return client.DMSSetSPC(ctx, "123456", "654321")
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
					if len(req.TLVs) != len(tt.wantTLV) {
						t.Fatalf("TLVs = %v, want %v", req.TLVs, tt.wantTLV)
					}
					for kind, want := range tt.wantTLV {
						assertTLV(t, req.TLVs, kind, want)
					}
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
