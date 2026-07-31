package qcom

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestVoiceSupplementaryRequestEncoding(t *testing.T) {
	flashType := VoiceFlashActivateAnswerHold
	legacyClass := uint8(1)
	extendedClass := VoiceServiceClassVoice | VoiceServiceClassSMS
	timer := uint8(20)
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		check   func(*testing.T, Request)
		resp    Response
	}{
		{
			name:    "send flash",
			message: MessageVoiceSendFlash,
			call: func(_ *testing.T, c *Client) error {
				return c.VoiceSendFlash(context.Background(), VoiceFlashRequest{CallID: 3, Payload: "12", Type: &flashType})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{3})
				assertTLV(t, req.TLVs, 0x10, []byte("12"))
				assertTLV(t, req.TLVs, 0x11, []byte{1})
			},
			resp: successResponse(MessageVoiceSendFlash),
		},
		{
			name:    "set supplementary service",
			message: MessageVoiceSetSupplementary,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceSetSupplementaryService(context.Background(), VoiceSetSupplementaryRequest{
					Action:               VoiceSupplementaryRegister,
					Reason:               VoiceReasonForwardNoReply,
					ServiceClass:         &legacyClass,
					ExtendedServiceClass: &extendedClass,
					Password:             "1234",
					Number:               "+86123",
					NoReplyTimer:         &timer,
				})
				if err == nil && (!got.StatusKnown || got.Active != VoiceStatusActive || got.Provision != VoiceProvisionedPermanently || !got.SIPErrorCodeKnown || got.SIPErrorCode != 403) {
					t.Fatalf("supplementary result = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{3, 3})
				assertTLV(t, req.TLVs, 0x10, []byte{1})
				assertTLV(t, req.TLVs, 0x11, []byte("1234"))
				assertTLV(t, req.TLVs, 0x12, []byte("+86123"))
				assertTLV(t, req.TLVs, 0x13, []byte{20})
				assertTLV(t, req.TLVs, 0x15, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass)))
			},
			resp: successResponse(
				MessageVoiceSetSupplementary,
				tlv.Bytes(0x15, []byte{1, 1}),
				tlv.Bytes(0x18, binary.LittleEndian.AppendUint16(nil, 403)),
			),
		},
		{
			name:    "get call waiting",
			message: MessageVoiceGetCallWaiting,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCallWaiting(context.Background(), VoiceSupplementaryQuery{ServiceClass: &legacyClass, ExtendedServiceClass: &extendedClass})
				if err == nil && (!got.StatusKnown || got.Active != VoiceStatusActive || !got.ServiceClassKnown || got.ServiceClass != 1 || !got.ExtendedClassKnown || got.ExtendedServiceClass != extendedClass) {
					t.Fatalf("call waiting = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x10, []byte{1})
				assertTLV(t, req.TLVs, 0x11, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass)))
			},
			resp: successResponse(
				MessageVoiceGetCallWaiting,
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x16, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass))),
				tlv.Bytes(0x1A, []byte{1, 1}),
			),
		},
		{
			name:    "get call barring",
			message: MessageVoiceGetCallBarring,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCallBarring(context.Background(), VoiceReasonBarAllOutgoing, VoiceSupplementaryQuery{ExtendedServiceClass: &extendedClass})
				if err == nil && (!got.ExtendedClassKnown || got.ExtendedServiceClass != extendedClass) {
					t.Fatalf("call barring = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{7})
				assertTLV(t, req.TLVs, 0x11, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass)))
			},
			resp: successResponse(MessageVoiceGetCallBarring, tlv.Bytes(0x16, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass)))),
		},
		{
			name:    "get CLIP",
			message: MessageVoiceGetCLIP,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCLIP(context.Background())
				if err == nil && (!got.StatusKnown || got.Active != VoiceStatusActive || got.Provision != VoiceProvisionedPermanently) {
					t.Fatalf("CLIP = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageVoiceGetCLIP, tlv.Bytes(0x10, []byte{1, 1})),
		},
		{
			name:    "get CLIR",
			message: MessageVoiceGetCLIR,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCLIRStatus(context.Background())
				if err == nil && (!got.StatusKnown || got.Provision != VoiceProvisionPresentationRestricted) {
					t.Fatalf("CLIR = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageVoiceGetCLIR, tlv.Bytes(0x10, []byte{1, 2})),
		},
		{
			name:    "get COLP",
			message: MessageVoiceGetCOLP,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCOLP(context.Background())
				if err == nil && (!got.StatusKnown || got.Active != VoiceStatusActive || !got.SIPErrorCodeKnown || got.SIPErrorCode != 500) {
					t.Fatalf("COLP = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp: successResponse(
				MessageVoiceGetCOLP,
				tlv.Bytes(0x10, []byte{1, 1}),
				tlv.Bytes(0x18, binary.LittleEndian.AppendUint16(nil, 500)),
			),
		},
		{
			name:    "get COLR",
			message: MessageVoiceGetCOLR,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCOLR(context.Background())
				if err == nil && (!got.StatusKnown || !got.PresentationKnown || got.Presentation != VoiceCOLRRestricted || !got.RetryKnown || got.RetryDuration != 30) {
					t.Fatalf("COLR = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp: successResponse(
				MessageVoiceGetCOLR,
				tlv.Bytes(0x10, []byte{1, 1}),
				tlv.Bytes(0x16, binary.LittleEndian.AppendUint32(nil, uint32(VoiceCOLRRestricted))),
				tlv.Bytes(0x17, binary.LittleEndian.AppendUint16(nil, 30)),
			),
		},
		{
			name:    "get CNAP",
			message: MessageVoiceGetCNAP,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCNAP(context.Background())
				if err == nil && (!got.StatusKnown || got.Provision != VoiceProvisionPresentationAllowed) {
					t.Fatalf("CNAP = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageVoiceGetCNAP, tlv.Bytes(0x10, []byte{1, 3})),
		},
		{
			name:    "get call forwarding",
			message: MessageVoiceGetCallForwarding,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceCallForwarding(context.Background(), VoiceReasonForwardUnconditional, VoiceSupplementaryQuery{ExtendedServiceClass: &extendedClass})
				if err == nil && (len(got.Rules) != 1 || !got.Rules[0].Active || got.Rules[0].ServiceClass != VoiceServiceClassVoice || !got.Rules[0].ExtendedServiceClass || got.Rules[0].Number != "+123" || got.Rules[0].NoReplyTimer != 20) {
					t.Fatalf("call forwarding = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1})
				assertTLV(t, req.TLVs, 0x11, binary.LittleEndian.AppendUint32(nil, uint32(extendedClass)))
			},
			resp: successResponse(MessageVoiceGetCallForwarding, tlv.Bytes(0x17, []byte{
				1,
				1, 1, 0, 0, 0, 20,
				0, 3, 1, 1, 4, '+', '1', '2', '3',
			})),
		},
		{
			name:    "set barring password",
			message: MessageVoiceSetBarringPassword,
			call: func(t *testing.T, c *Client) error {
				got, err := c.VoiceSetCallBarringPassword(context.Background(), VoiceReasonBarAllOutgoing, "1234", "5678")
				if err == nil && (!got.RetryKnown || got.RetryDuration != 30) {
					t.Fatalf("password result = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{7, '1', '2', '3', '4', '5', '6', '7', '8', '5', '6', '7', '8'})
			},
			resp: successResponse(MessageVoiceSetBarringPassword, tlv.Bytes(0x15, binary.LittleEndian.AppendUint16(nil, 30))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceVoice || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want Voice/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.message)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestVoiceForwardingDecoding(t *testing.T) {
	tests := []struct {
		name  string
		tlvs  tlv.TLVs
		check func(*testing.T, []VoiceCallForwardingRule)
	}{
		{
			name: "legacy",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1, 1, 4, '+', '1', '2', '3', 15})},
			check: func(t *testing.T, rules []VoiceCallForwardingRule) {
				if len(rules) != 1 || rules[0].Number != "+123" || rules[0].NoReplyTimer != 15 || rules[0].ExtendedServiceClass {
					t.Fatalf("rules = %+v", rules)
				}
			},
		},
		{
			name: "extended2 preferred",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0}),
				tlv.Bytes(0x17, []byte{1, 1, 1, 0, 0, 0, 20, 0, 3, 1, 1, 2, '4', '2'}),
			},
			check: func(t *testing.T, rules []VoiceCallForwardingRule) {
				if len(rules) != 1 || rules[0].Number != "42" || !rules[0].ExtendedServiceClass {
					t.Fatalf("rules = %+v", rules)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rules VoiceCallForwardingRules
			if err := rules.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			tt.check(t, rules)
		})
	}
}

func TestVoiceSupplementaryRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "flash too long",
			fn: func() error {
				client := &Client{}
				return client.VoiceSendFlash(context.Background(), VoiceFlashRequest{Payload: string(make([]byte, voiceFlashPayloadMax+1))})
			},
		},
		{
			name: "password invalid",
			fn: func() error {
				client := &Client{}
				_, err := client.VoiceSetCallBarringPassword(context.Background(), VoiceReasonBarAll, "123", "5678")
				return err
			},
		},
		{
			name: "forwarding count missing",
			fn: func() error {
				_, err := decodeVoiceForwardingLegacy(nil)
				return err
			},
		},
		{
			name: "forwarding number truncated",
			fn: func() error {
				_, err := decodeVoiceForwardingLegacy([]byte{1, 1, 1, 4, '1'})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("call() error = nil, want error")
			}
		})
	}
}
