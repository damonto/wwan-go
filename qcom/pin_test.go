package qcom

import (
	"context"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestPINOperations(t *testing.T) {
	keyReference := PINKeyReferenceApplication2
	tests := []struct {
		name       string
		message    MessageID
		wantPIN    []byte
		wantKeyTLV byte
		call       func(*Client) (PINOperationResult, error)
	}{
		{
			name:       "enable PIN1",
			message:    MessageSetPINProtection,
			wantPIN:    []byte{byte(PINIDPIN1), 1, 4, '1', '2', '3', '4'},
			wantKeyTLV: 0x10,
			call: func(c *Client) (PINOperationResult, error) {
				return c.SetPINProtection(context.Background(), PINProtectionRequest{Session: SessionPrimaryGWProvisioning, ID: PINIDPIN1, Enable: true, PIN: "1234", KeyReference: &keyReference})
			},
		},
		{
			name:       "verify PIN1",
			message:    MessageVerifyPIN,
			wantPIN:    []byte{byte(PINIDPIN1), 4, '1', '2', '3', '4'},
			wantKeyTLV: 0x11,
			call: func(c *Client) (PINOperationResult, error) {
				return c.VerifyPIN(context.Background(), PINVerifyRequest{Session: SessionPrimaryGWProvisioning, ID: PINIDPIN1, PIN: "1234", KeyReference: &keyReference})
			},
		},
		{
			name:       "unblock PIN1",
			message:    MessageUnblockPIN,
			wantPIN:    []byte{byte(PINIDPIN1), 8, '1', '2', '3', '4', '5', '6', '7', '8', 4, '4', '3', '2', '1'},
			wantKeyTLV: 0x10,
			call: func(c *Client) (PINOperationResult, error) {
				return c.UnblockPIN(context.Background(), PINUnblockRequest{Session: SessionPrimaryGWProvisioning, ID: PINIDPIN1, PUK: "12345678", NewPIN: "4321", KeyReference: &keyReference})
			},
		},
		{
			name:       "change PIN2",
			message:    MessageChangePIN,
			wantPIN:    []byte{byte(PINIDPIN2), 4, '1', '2', '3', '4', 4, '4', '3', '2', '1'},
			wantKeyTLV: 0x10,
			call: func(c *Client) (PINOperationResult, error) {
				return c.ChangePIN(context.Background(), PINChangeRequest{Session: SessionPrimaryGWProvisioning, ID: PINIDPIN2, OldPIN: "1234", NewPIN: "4321", KeyReference: &keyReference})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.MessageID != tt.message {
						t.Fatalf("request service/message = 0x%02X/0x%04X, want UIM/0x%04X", req.Service, req.MessageID, tt.message)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0})
					assertTLV(t, req.TLVs, 0x02, tt.wantPIN)
					assertTLV(t, req.TLVs, tt.wantKeyTLV, []byte{byte(keyReference)})
				},
				resp: successResponse(tt.message, tlv.Bytes(0x11, []byte{3, 0xAA, 0xBB, 0xCC}), tlv.Bytes(0x13, []byte{0x90, 0x00})),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			got, err := tt.call(client)
			if err != nil {
				t.Fatalf("PIN operation error = %v", err)
			}
			if len(got.EncryptedPIN1) != 3 || got.EncryptedPIN1[0] != 0xAA {
				t.Fatalf("PIN result = %+v", got)
			}
		})
	}
}

func TestPINOperationFailureReturnsRetries(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "incorrect PIN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: errorResponse(MessageVerifyPIN, QMIErrorIncorrectPin, tlv.Bytes(0x10, []byte{2, 9})),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			result, err := client.VerifyPIN(context.Background(), PINVerifyRequest{Session: SessionPrimaryGWProvisioning, ID: PINIDPIN1, PIN: "0000"})
			if err == nil {
				t.Fatal("VerifyPIN() error = nil, want incorrect PIN")
			}
			if !result.RetriesKnown || result.VerifyRetries != 2 || result.UnblockRetries != 9 {
				t.Fatalf("VerifyPIN() result = %+v, want retries 2/9", result)
			}
		})
	}
}

func TestPINValidation(t *testing.T) {
	tests := []struct {
		name    string
		call    func(*Client) error
		wantErr string
	}{
		{
			name: "empty PIN",
			call: func(c *Client) error {
				_, err := c.VerifyPIN(context.Background(), PINVerifyRequest{ID: PINIDPIN1})
				return err
			},
			wantErr: "validating PIN: value is empty",
		},
		{
			name: "hidden key cannot be unblocked",
			call: func(c *Client) error {
				_, err := c.UnblockPIN(context.Background(), PINUnblockRequest{ID: PINIDHiddenKey, PUK: "12345678", NewPIN: "1234"})
				return err
			},
			wantErr: "invalid PIN ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{transport: &fakeTransport{t: t}, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("operation error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
