package qcom

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSLegacyPINRequestEncoding(t *testing.T) {
	tests := []struct {
		name string
		got  func() (Request, error)
		msg  MessageID
		want []byte
	}{
		{
			name: "set protection",
			got: func() (Request, error) {
				return (DMSUIMSetPINProtectionRequest{Info: DMSPINProtectionRequest{
					ID: DMSPINIDPIN1, Enabled: true, PIN: "1234",
				}}).Request()
			},
			msg:  MessageDMSUIMSetPINProtection,
			want: []byte{1, 1, 4, '1', '2', '3', '4'},
		},
		{
			name: "verify",
			got: func() (Request, error) {
				return (DMSUIMVerifyPINRequest{Info: DMSPINVerifyRequest{
					ID: DMSPINIDPIN2, PIN: "5678",
				}}).Request()
			},
			msg:  MessageDMSUIMVerifyPIN,
			want: []byte{2, 4, '5', '6', '7', '8'},
		},
		{
			name: "unblock",
			got: func() (Request, error) {
				return (DMSUIMUnblockPINRequest{Info: DMSPINUnblockRequest{
					ID: DMSPINIDPIN1, PUK: "12345678", NewPIN: "4321",
				}}).Request()
			},
			msg:  MessageDMSUIMUnblockPIN,
			want: []byte{1, 8, '1', '2', '3', '4', '5', '6', '7', '8', 4, '4', '3', '2', '1'},
		},
		{
			name: "change",
			got: func() (Request, error) {
				return (DMSUIMChangePINRequest{Info: DMSPINChangeRequest{
					ID: DMSPINIDPIN2, OldPIN: "1234", NewPIN: "4321",
				}}).Request()
			},
			msg:  MessageDMSUIMChangePIN,
			want: []byte{2, 4, '1', '2', '3', '4', 4, '4', '3', '2', '1'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.got()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if req.Service != ServiceDMS || req.MessageID != tt.msg {
				t.Fatalf("service/message = 0x%02X/0x%04X", req.Service, req.MessageID)
			}
			assertTLV(t, req.TLVs, dmsTLVUIMPINInfo, tt.want)
		})
	}
}

func TestDMSPINStatusDecoding(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want DMSPINStatusResponse
		err  bool
	}{
		{
			name: "both PINs",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVPIN1Status, []byte{byte(DMSPINStatusEnabledVerified), 3, 8}),
				tlv.Bytes(dmsTLVPIN2Status, []byte{byte(DMSPINStatusBlocked), 0, 2}),
			},
			want: DMSPINStatusResponse{
				PIN1: DMSPINState{Status: DMSPINStatusEnabledVerified, VerifyRetries: 3, UnblockRetries: 8}, PIN1Known: true,
				PIN2: DMSPINState{Status: DMSPINStatusBlocked, UnblockRetries: 2}, PIN2Known: true,
			},
		},
		{name: "optional fields omitted"},
		{
			name: "truncated",
			tlvs: tlv.TLVs{tlv.Bytes(dmsTLVPIN1Status, []byte{1, 2})},
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSPINStatusResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.err {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSLegacyPINOperationReturnsRetriesOnFailure(t *testing.T) {
	transport := &fakeTransport{t: t, calls: []transportCall{{
		check: func(req Request) {
			if req.Service != ServiceDMS || req.MessageID != MessageDMSUIMVerifyPIN {
				t.Fatalf("request = %+v", req)
			}
		},
		resp: errorResponse(MessageDMSUIMVerifyPIN, QMIErrorIncorrectPin, tlv.Bytes(dmsTLVPINRetries, []byte{2, 7})),
	}}}
	client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}
	result, err := client.DMSVerifyPIN(context.Background(), DMSPINVerifyRequest{ID: DMSPINIDPIN1, PIN: "0000"})
	if err == nil {
		t.Fatal("DMSVerifyPIN() error = nil, want non-nil")
	}
	if !result.RetriesKnown || result.VerifyRetries != 2 || result.UnblockRetries != 7 {
		t.Fatalf("DMSVerifyPIN() result = %+v, want retries 2/7", result)
	}
}

func TestDMSActivationAndFactoryRequestEncoding(t *testing.T) {
	haKey := "ha"
	aaaKey := "aaa"
	tests := []struct {
		name string
		got  func() (Request, error)
		msg  MessageID
		want map[byte][]byte
	}{
		{
			name: "automatic activation",
			got: func() (Request, error) {
				return (DMSActivateAutomaticRequest{ActivationCode: "ABC"}).Request()
			},
			msg:  MessageDMSActivateAutomatic,
			want: map[byte][]byte{dmsTLVActivationState: {3, 'A', 'B', 'C'}},
		},
		{
			name: "manual activation",
			got: func() (Request, error) {
				return (DMSActivateManualRequest{Info: DMSManualActivationRequest{
					SPC: "123456", SID: 0x1234, MDN: "mdn", MIN: "min",
					MNHAKey: &haKey, MNAAAKey: &aaaKey,
					PreferredRoamingList: &DMSPreferredRoamingList{TotalLength: 5, Segment: 2, Data: []byte{1, 2, 3}},
				}}).Request()
			},
			msg: MessageDMSActivateManual,
			want: map[byte][]byte{
				0x01: {'1', '2', '3', '4', '5', '6', 0x34, 0x12, 3, 'm', 'd', 'n', 3, 'm', 'i', 'n'},
				0x11: {2, 'h', 'a'},
				0x12: {3, 'a', 'a', 'a'},
				0x13: {5, 0, 3, 0, 2, 1, 2, 3},
			},
		},
		{
			name: "factory defaults",
			got: func() (Request, error) {
				return (DMSRestoreFactoryDefaultsRequest{SPC: "654321"}).Request()
			},
			msg:  MessageDMSRestoreFactoryDefaults,
			want: map[byte][]byte{0x01: {'6', '5', '4', '3', '2', '1'}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.got()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if req.Service != ServiceDMS || req.MessageID != tt.msg {
				t.Fatalf("service/message = 0x%02X/0x%04X", req.Service, req.MessageID)
			}
			if len(req.TLVs) != len(tt.want) {
				t.Fatalf("TLV count = %d, want %d", len(req.TLVs), len(tt.want))
			}
			for typ, want := range tt.want {
				assertTLV(t, req.TLVs, typ, want)
			}
		})
	}
}

func TestDMSLegacyValidation(t *testing.T) {
	longPIN := strings.Repeat("1", dmsPINValueMax+1)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "invalid PIN ID",
			call: func() error {
				_, err := (DMSUIMVerifyPINRequest{Info: DMSPINVerifyRequest{ID: 3, PIN: "1234"}}).Request()
				return err
			},
		},
		{
			name: "long PIN",
			call: func() error {
				_, err := (DMSUIMVerifyPINRequest{Info: DMSPINVerifyRequest{ID: DMSPINIDPIN1, PIN: longPIN}}).Request()
				return err
			},
		},
		{
			name: "short SPC",
			call: func() error {
				_, err := (DMSRestoreFactoryDefaultsRequest{SPC: "123"}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("Request() error = nil, want non-nil")
			}
		})
	}
}

func TestDMSCKStatusDecoding(t *testing.T) {
	value := []byte{byte(DMSFacilityActivated), 4, 5}
	var got DMSCKStatus
	err := got.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(dmsTLVFacilityStatus, value),
		tlv.Bytes(dmsTLVOperationBlocking, []byte{1}),
	})
	if err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	want := DMSCKStatus{FacilityState: DMSFacilityActivated, VerifyRetries: 4, UnblockRetries: 5, OperationBlocking: true, OperationBlockingKnown: true}
	if got != want {
		t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, want)
	}
}

func TestDMSActivationStateDecoding(t *testing.T) {
	value := binary.LittleEndian.AppendUint16(nil, uint16(DMSActivationConnected))
	var got DMSActivationState
	if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(dmsTLVActivationState, value)}); err != nil {
		t.Fatalf("decodeDMSActivationState() error = %v", err)
	}
	if got != DMSActivationConnected {
		t.Fatalf("decodeDMSActivationState() = %v, want %v", got, DMSActivationConnected)
	}
}
