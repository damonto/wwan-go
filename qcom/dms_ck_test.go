package qcom

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSCKRequests(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (Request, error)
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "activate network facility",
			build: func() (Request, error) {
				return (DMSUIMSetCKProtectionRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Info: DMSCKProtectionRequest{
						Facility: DMSUIMFacilityNetwork, State: DMSFacilityActivated, Key: "12345678",
					},
				}).Request()
			},
			message: MessageDMSUIMSetCKProtection,
			wantTLV: map[byte][]byte{
				0x01: {byte(DMSUIMFacilityNetwork), byte(DMSFacilityActivated), 8, '1', '2', '3', '4', '5', '6', '7', '8'},
			},
		},
		{
			name: "unblock UIM facility",
			build: func() (Request, error) {
				return (DMSUIMUnblockCKRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Info: DMSCKUnblockRequest{Facility: DMSUIMFacilityUIM, Key: "87654321"},
				}).Request()
			},
			message: MessageDMSUIMUnblockCK,
			wantTLV: map[byte][]byte{
				0x01: {byte(DMSUIMFacilityUIM), 8, '8', '7', '6', '5', '4', '3', '2', '1'},
			},
		},
		{
			name: "get UIM state",
			build: func() (Request, error) {
				return (DMSUIMGetStateRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSUIMGetState,
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

func TestDMSCKRequestsRejectInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		build   func() error
		wantErr string
	}{
		{
			name: "facility",
			build: func() error {
				_, err := (DMSUIMSetCKProtectionRequest{Info: DMSCKProtectionRequest{
					Facility: DMSUIMFacility(5), State: DMSFacilityActivated, Key: "1234",
				}}).Request()
				return err
			},
			wantErr: "facility",
		},
		{
			name: "blocked state",
			build: func() error {
				_, err := (DMSUIMSetCKProtectionRequest{Info: DMSCKProtectionRequest{
					Facility: DMSUIMFacilityNetwork, State: DMSFacilityBlocked, Key: "1234",
				}}).Request()
				return err
			},
			wantErr: "state",
		},
		{
			name: "empty key",
			build: func() error {
				_, err := (DMSUIMUnblockCKRequest{Info: DMSCKUnblockRequest{Facility: DMSUIMFacilityNetwork}}).Request()
				return err
			},
			wantErr: "empty",
		},
		{
			name: "long key",
			build: func() error {
				_, err := (DMSUIMUnblockCKRequest{Info: DMSCKUnblockRequest{
					Facility: DMSUIMFacilityNetwork, Key: "123456789",
				}}).Request()
				return err
			},
			wantErr: "exceeds",
		},
		{
			name: "NUL key",
			build: func() error {
				_, err := (DMSUIMUnblockCKRequest{Info: DMSCKUnblockRequest{
					Facility: DMSUIMFacilityNetwork, Key: "12\x0034",
				}}).Request()
				return err
			},
			wantErr: "NUL",
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

func TestDMSCKOperationResult(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSCKOperationResult
		wantErr string
	}{
		{name: "omitted"},
		{name: "three retries", tlvs: tlv.TLVs{tlv.Uint(0x10, uint8(3))}, want: DMSCKOperationResult{Retries: 3, RetriesKnown: true}},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSCKOperationResult
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("decode error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("result = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSUIMGetStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSUIMState
		wantErr string
	}{
		{name: "initialized", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(DMSUIMStateInitializationCompleted))}},
		{name: "unavailable", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(DMSUIMStateUnavailable))}, want: DMSUIMStateUnavailable},
		{name: "missing", wantErr: "missing"},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSUIMGetStateResponse
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
			if got.State != tt.want {
				t.Fatalf("state = %d, want %d", got.State, tt.want)
			}
		})
	}
}

func TestClientDMSCKAndUIMState(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		resp    Response
		run     func(context.Context, *Client) error
		wantErr bool
	}{
		{
			name: "set protection", message: MessageDMSUIMSetCKProtection,
			resp: successResponse(MessageDMSUIMSetCKProtection, tlv.Uint(0x10, uint8(3))),
			run: func(ctx context.Context, client *Client) error {
				result, err := client.DMSSetCKProtection(ctx, DMSCKProtectionRequest{
					Facility: DMSUIMFacilityNetwork, State: DMSFacilityActivated, Key: "1234",
				})
				if result != (DMSCKOperationResult{Retries: 3, RetriesKnown: true}) {
					t.Fatalf("DMSSetCKProtection() result = %+v", result)
				}
				return err
			},
		},
		{
			name: "unblock error preserves retries", message: MessageDMSUIMUnblockCK,
			resp: errorResponse(MessageDMSUIMUnblockCK, QMIErrorIncorrectPin, tlv.Uint(0x10, uint8(2))),
			run: func(ctx context.Context, client *Client) error {
				result, err := client.DMSUnblockCK(ctx, DMSCKUnblockRequest{
					Facility: DMSUIMFacilityNetwork, Key: "1234",
				})
				if result != (DMSCKOperationResult{Retries: 2, RetriesKnown: true}) {
					t.Fatalf("DMSUnblockCK() result = %+v", result)
				}
				if err == nil || !errors.Is(err, QMIErrorIncorrectPin) {
					t.Fatalf("DMSUnblockCK() error = %v", err)
				}
				return err
			},
			wantErr: true,
		},
		{
			name: "UIM state", message: MessageDMSUIMGetState,
			resp: successResponse(MessageDMSUIMGetState, tlv.Uint(0x01, uint8(DMSUIMStateNotPresent))),
			run: func(ctx context.Context, client *Client) error {
				state, err := client.DMSUIMState(ctx)
				if err == nil && state != DMSUIMStateNotPresent {
					t.Fatalf("DMSUIMState() = %d", state)
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
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}
			err := tt.run(context.Background(), client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("client call error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
