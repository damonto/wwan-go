package qcom

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestVoiceRequestEncoding(t *testing.T) {
	callType := VoiceCallTypeVoiceIP
	clir := VoiceCLIRInvocation
	emergency := uint8(0x05)
	service := VoiceDialServiceLTE
	endCause := uint32(0x11223344)
	rejectCause := uint32(2)
	sipRejectCause := uint16(603)
	callID := uint8(7)
	audioAttributes := VoiceCallAttributeTX | VoiceCallAttributeRX
	videoAttributes := VoiceCallAttributeRX
	presentation := VoiceIPPresentationRestricted
	callPull := true
	codecProfile := uint8(2)
	secure := true
	rttMode := VoiceRTTFull
	secondary := true
	tests := []struct {
		name  string
		call  func(*Client) error
		id    MessageID
		check func(*testing.T, Request)
		resp  Response
	}{
		{
			name: "dial with overflow SIP URI",
			call: func(c *Client) error {
				_, err := c.VoiceDial(context.Background(), "123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890", VoiceDialOptions{
					CallType:          &callType,
					CLIR:              &clir,
					EmergencyCategory: &emergency,
					Service:           &service,
					AudioAttributes:   &audioAttributes,
					VideoAttributes:   &videoAttributes,
					Presentation:      &presentation,
					CallPull:          &callPull,
					CodecProfile:      &codecProfile,
					Secure:            &secure,
					RTTMode:           &rttMode,
					OriginationNumber: "5551234",
					Secondary:         &secondary,
				})
				return err
			},
			id: MessageVoiceDialCall,
			check: func(t *testing.T, req Request) {
				if value, _ := tlv.Value(req.TLVs, 0x01); len(value) != voiceNumberMax {
					t.Fatalf("calling number length = %d, want %d", len(value), voiceNumberMax)
				}
				if value, _ := tlv.Value(req.TLVs, 0x17); len(value) != 9 {
					t.Fatalf("overflow length = %d, want 9", len(value))
				}
				assertTLV(t, req.TLVs, 0x10, []byte{byte(callType)})
				assertTLV(t, req.TLVs, 0x11, []byte{byte(clir)})
				assertTLV(t, req.TLVs, 0x14, []byte{emergency})
				assertTLV(t, req.TLVs, 0x16, binary.LittleEndian.AppendUint32(nil, uint32(service)))
				assertTLV(t, req.TLVs, 0x18, binary.LittleEndian.AppendUint64(nil, uint64(audioAttributes)))
				assertTLV(t, req.TLVs, 0x19, binary.LittleEndian.AppendUint64(nil, uint64(videoAttributes)))
				assertTLV(t, req.TLVs, 0x1A, binary.LittleEndian.AppendUint32(nil, uint32(presentation)))
				assertTLV(t, req.TLVs, 0x20, []byte{1})
				assertTLV(t, req.TLVs, 0x21, []byte{codecProfile})
				assertTLV(t, req.TLVs, 0x22, []byte{1})
				assertTLV(t, req.TLVs, 0x23, binary.LittleEndian.AppendUint32(nil, uint32(rttMode)))
				assertTLV(t, req.TLVs, 0x24, []byte("5551234"))
				assertTLV(t, req.TLVs, 0x25, []byte{1})
			},
			resp: successResponse(MessageVoiceDialCall, tlv.Bytes(0x10, []byte{9})),
		},
		{
			name: "end with cause",
			call: func(c *Client) error {
				return c.VoiceEnd(context.Background(), callID, &endCause)
			},
			id: MessageVoiceEndCall,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{callID})
				assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint32(nil, endCause))
			},
			resp: successResponse(MessageVoiceEndCall),
		},
		{
			name: "answer reject",
			call: func(c *Client) error {
				return c.VoiceAnswer(context.Background(), callID, VoiceAnswerOptions{
					CallType:        &callType,
					AudioAttributes: &audioAttributes,
					VideoAttributes: &videoAttributes,
					Presentation:    &presentation,
					Reject:          true,
					RejectCause:     &rejectCause,
					SIPRejectCause:  &sipRejectCause,
					CodecProfile:    &codecProfile,
					RTTMode:         &rttMode,
				})
			},
			id: MessageVoiceAnswerCall,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{callID})
				assertTLV(t, req.TLVs, 0x10, []byte{byte(callType)})
				assertTLV(t, req.TLVs, 0x11, binary.LittleEndian.AppendUint64(nil, uint64(audioAttributes)))
				assertTLV(t, req.TLVs, 0x12, binary.LittleEndian.AppendUint64(nil, uint64(videoAttributes)))
				assertTLV(t, req.TLVs, 0x13, binary.LittleEndian.AppendUint32(nil, uint32(presentation)))
				assertTLV(t, req.TLVs, 0x15, []byte{1})
				assertTLV(t, req.TLVs, 0x16, binary.LittleEndian.AppendUint32(nil, rejectCause))
				assertTLV(t, req.TLVs, 0x17, binary.LittleEndian.AppendUint16(nil, sipRejectCause))
				assertTLV(t, req.TLVs, 0x18, []byte{codecProfile})
				assertTLV(t, req.TLVs, 0x19, binary.LittleEndian.AppendUint32(nil, uint32(rttMode)))
			},
			resp: successResponse(MessageVoiceAnswerCall),
		},
		{
			name: "indication register",
			call: func(c *Client) error {
				on := true
				return c.VoiceSetIndicationReport(context.Background(), VoiceIndicationConfig{
					CallEvents:                      &on,
					DTMFEvents:                      &on,
					USSDEvents:                      &on,
					SupplementaryNotificationEvents: &on,
					SupplementaryResultEvents:       &on,
					ModificationEvents:              &on,
					HandoverEvents:                  &on,
					SpeechEvents:                    &on,
				})
			},
			id: MessageVoiceIndicationRegister,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x10, []byte{1})
				assertTLV(t, req.TLVs, 0x12, []byte{1})
				assertTLV(t, req.TLVs, 0x13, []byte{1})
				assertTLV(t, req.TLVs, 0x14, []byte{1})
				assertTLV(t, req.TLVs, 0x15, []byte{1})
				assertTLV(t, req.TLVs, 0x16, []byte{1})
				assertTLV(t, req.TLVs, 0x17, []byte{1})
				assertTLV(t, req.TLVs, 0x18, []byte{1})
			},
			resp: successResponse(MessageVoiceIndicationRegister),
		},
		{
			name: "manage",
			call: func(c *Client) error {
				return c.VoiceManage(context.Background(), VoiceManageRequest{Operation: VoiceManageReleaseSpecified, CallID: &callID, RejectCause: &rejectCause})
			},
			id: MessageVoiceManageCalls,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceManageReleaseSpecified)})
				assertTLV(t, req.TLVs, 0x10, []byte{callID})
				assertTLV(t, req.TLVs, 0x11, binary.LittleEndian.AppendUint32(nil, rejectCause))
			},
			resp: successResponse(MessageVoiceManageCalls),
		},
		{
			name: "burst DTMF",
			call: func(c *Client) error {
				return c.VoiceBurstDTMF(context.Background(), callID, "12#", &VoiceDTMFLengths{On: VoiceDTMFOn200ms, Off: VoiceDTMFOff150ms})
			},
			id: MessageVoiceBurstDTMF,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{callID, 3, '1', '2', '#'})
				assertTLV(t, req.TLVs, 0x10, []byte{byte(VoiceDTMFOn200ms), byte(VoiceDTMFOff150ms)})
			},
			resp: successResponse(MessageVoiceBurstDTMF),
		},
		{
			name: "start DTMF",
			call: func(c *Client) error {
				return c.VoiceStartDTMF(context.Background(), callID, 'A')
			},
			id: MessageVoiceStartContinuousDTMF,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{callID, 'A'})
			},
			resp: successResponse(MessageVoiceStartContinuousDTMF),
		},
		{
			name: "stop DTMF",
			call: func(c *Client) error {
				return c.VoiceStopDTMF(context.Background(), callID)
			},
			id: MessageVoiceStopContinuousDTMF,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{callID})
			},
			resp: successResponse(MessageVoiceStopContinuousDTMF),
		},
		{
			name: "originate USSD",
			call: func(c *Client) error {
				_, err := c.VoiceOriginateUSSD(context.Background(), VoiceUSSDData{Encoding: VoiceUSSDEncodingASCII, Data: []byte("*123#")})
				return err
			},
			id: MessageVoiceOriginateUSSD,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceUSSDEncodingASCII), 5, '*', '1', '2', '3', '#'})
			},
			resp: successResponse(MessageVoiceOriginateUSSD),
		},
		{
			name: "answer USSD",
			call: func(c *Client) error {
				return c.VoiceAnswerUSSD(context.Background(), VoiceUSSDData{Encoding: VoiceUSSDEncoding8Bit, Data: []byte{1, 2}})
			},
			id: MessageVoiceAnswerUSSD,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceUSSDEncoding8Bit), 2, 1, 2})
			},
			resp: successResponse(MessageVoiceAnswerUSSD),
		},
		{
			name: "cancel USSD",
			call: func(c *Client) error {
				return c.VoiceCancelUSSD(context.Background())
			},
			id: MessageVoiceCancelUSSD,
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs = %+v, want empty", req.TLVs)
				}
			},
			resp: successResponse(MessageVoiceCancelUSSD),
		},
		{
			name: "bind subscription",
			call: func(c *Client) error {
				return c.VoiceBindSubscription(context.Background(), VoiceSubscriptionSecondary)
			},
			id: MessageVoiceBindSubscription,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceSubscriptionSecondary)})
			},
			resp: successResponse(MessageVoiceBindSubscription),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceVoice || req.ClientID != 7 || req.MessageID != tt.id {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want Voice/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.id)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestVoiceDialRejectsMalformedCallID(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "truncated"},
		{name: "trailing byte", value: []byte{9, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: successResponse(MessageVoiceDialCall, tlv.Bytes(0x10, tt.value)),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			if _, err := client.VoiceDial(context.Background(), "123", VoiceDialOptions{}); err == nil {
				t.Fatal("VoiceDial() error = nil, want non-nil")
			}
		})
	}
}

func TestVoiceDecodeCalls(t *testing.T) {
	callValue := []byte{1, 7, byte(VoiceCallStateConversation), byte(VoiceCallTypeVoiceIP), byte(VoiceCallDirectionMT), byte(VoiceCallModeLTE), 1, byte(VoiceALSLine2)}
	numberValue := []byte{1, 7, byte(VoicePresentationAllowed), 4, '1', '2', '3', '4'}
	var calls VoiceCalls
	if err := calls.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, callValue), tlv.Bytes(0x11, numberValue)}); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if len(calls) != 1 || calls[0].ID != 7 || !calls[0].Multiparty || calls[0].ALS != VoiceALSLine2 || calls[0].Number != "1234" || !calls[0].NumberKnown {
		t.Fatalf("calls = %+v", calls)
	}

	if err := calls.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1})}); err == nil {
		t.Fatal("UnmarshalTLVs() error = nil, want truncated-record error")
	}

	var callInfo VoiceCall
	if err := callInfo.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(0x10, []byte{7, byte(VoiceCallStateIncoming), byte(VoiceCallTypeVoice), byte(VoiceCallDirectionMT), byte(VoiceCallModeGSM)}),
		tlv.Bytes(0x11, []byte{byte(VoicePresentationRestricted), 3, 'x', 'y', 'z'}),
	}); err != nil {
		t.Fatalf("UnmarshalTLVs() error = %v", err)
	}
	if callInfo.Number != "xyz" || callInfo.Presentation != VoicePresentationRestricted {
		t.Fatalf("callInfo = %+v", callInfo)
	}
}

func TestVoiceDecodeCallTableRejectsMalformedLength(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "truncated", value: []byte{1, 7}},
		{name: "trailing byte", value: []byte{0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls VoiceCalls
			if err := calls.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, tt.value)}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestVoiceDecodeCallDetails(t *testing.T) {
	tests := []struct {
		name         string
		callsTLV     byte
		numbersTLV   byte
		endReasonTLV byte
		audioTLV     byte
		videoTLV     byte
		sipURITLV    byte
		nameTLV      byte
		ipNameTLV    byte
		namePITLV    byte
		endTextTLV   byte
		secureTLV    byte
		sipErrorTLV  byte
		rttTLV       byte
		wantSecure   bool
	}{
		{
			name:         "get all call info response",
			callsTLV:     0x10,
			numbersTLV:   0x11,
			endReasonTLV: 0x18,
			nameTLV:      0x12,
			audioTLV:     0x1F,
			videoTLV:     0x20,
			sipURITLV:    0x22,
			ipNameTLV:    0x27,
			endTextTLV:   0x28,
			sipErrorTLV:  0x2A,
			rttTLV:       0x2E,
		},
		{
			name:         "all call status indication",
			callsTLV:     0x01,
			numbersTLV:   0x10,
			endReasonTLV: 0x14,
			nameTLV:      0x11,
			audioTLV:     0x1B,
			videoTLV:     0x1C,
			sipURITLV:    0x1E,
			ipNameTLV:    0x2D,
			endTextTLV:   0x2E,
			namePITLV:    0x2F,
			secureTLV:    0x32,
			sipErrorTLV:  0x33,
			rttTLV:       0x38,
			wantSecure:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callValue := []byte{1, 7, byte(VoiceCallStateConversation), byte(VoiceCallTypeVoiceIP), byte(VoiceCallDirectionMT), byte(VoiceCallModeLTE), 0, byte(VoiceALSLine1)}
			audio := binary.LittleEndian.AppendUint64([]byte{1, 7}, uint64(VoiceCallAttributeTX|VoiceCallAttributeRX))
			video := binary.LittleEndian.AppendUint64([]byte{1, 7}, uint64(VoiceCallAttributeRX))
			rtt := binary.LittleEndian.AppendUint32([]byte{1, 7}, uint32(VoiceRTTFull))
			tlvs := tlv.TLVs{
				tlv.Bytes(tt.callsTLV, callValue),
				tlv.Bytes(tt.endReasonTLV, []byte{1, 7, 0x34, 0x12}),
				tlv.Bytes(tt.nameTLV, []byte{1, 7, byte(VoicePresentationAllowed), 0, 3, 'B', 'o', 'b'}),
				tlv.Bytes(tt.audioTLV, audio),
				tlv.Bytes(tt.videoTLV, video),
				tlv.Bytes(tt.sipURITLV, []byte{1, 7, 7, 's', 'i', 'p', ':', '1', '2', '3'}),
				tlv.Bytes(tt.ipNameTLV, []byte{1, 7, 2, 'I', 0, 'P', 0}),
				tlv.Bytes(tt.endTextTLV, []byte{1, 7, 2, 'O', 0, 'K', 0}),
				tlv.Bytes(tt.sipErrorTLV, []byte{1, 7, 0x57, 0x02}),
				tlv.Bytes(tt.rttTLV, rtt),
			}
			if tt.secureTLV != 0 {
				tlvs = append(tlvs, tlv.Bytes(tt.secureTLV, []byte{1, 7, 1}))
			}
			if tt.namePITLV != 0 {
				tlvs = append(tlvs, tlv.Bytes(tt.namePITLV, []byte{1, 7, byte(VoicePresentationRestricted)}))
			}

			var calls VoiceCalls
			if err := calls.unmarshalTLVs(tlvs, tt.callsTLV, tt.numbersTLV); err != nil {
				t.Fatalf("unmarshalTLVs() error = %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("len(calls) = %d, want 1", len(calls))
			}
			call := calls[0]
			if !call.EndReasonKnown || call.EndReason != 0x1234 {
				t.Errorf("end reason = 0x%04X, known %t; want 0x1234, true", call.EndReason, call.EndReasonKnown)
			}
			if !call.AudioAttributesKnown || call.AudioAttributes != VoiceCallAttributeTX|VoiceCallAttributeRX {
				t.Errorf("audio attributes = 0x%X, known %t; want TX|RX, true", call.AudioAttributes, call.AudioAttributesKnown)
			}
			if !call.VideoAttributesKnown || call.VideoAttributes != VoiceCallAttributeRX {
				t.Errorf("video attributes = 0x%X, known %t; want RX, true", call.VideoAttributes, call.VideoAttributesKnown)
			}
			if !call.SIPURIKnown || call.SIPURI != "sip:123" {
				t.Errorf("SIP URI = %q, known %t; want sip:123, true", call.SIPURI, call.SIPURIKnown)
			}
			if !call.CallerNameKnown || call.CallerName != "IP" || !call.CallerNameCodingSchemeKnown {
				t.Errorf("caller name = %q, known %t, coding known %t; want IP, true, true", call.CallerName, call.CallerNameKnown, call.CallerNameCodingSchemeKnown)
			}
			wantNamePresentation := VoicePresentationAllowed
			if tt.namePITLV != 0 {
				wantNamePresentation = VoicePresentationRestricted
			}
			if !call.CallerNamePresentationKnown || call.CallerNamePresentation != wantNamePresentation {
				t.Errorf("caller name presentation = %d, known %t; want %d, true", call.CallerNamePresentation, call.CallerNamePresentationKnown, wantNamePresentation)
			}
			if len(call.EndReasonText) != 2 || call.EndReasonText[0] != 'O' || call.EndReasonText[1] != 'K' {
				t.Errorf("end reason text = %v, want [O K]", call.EndReasonText)
			}
			if !call.SIPErrorCodeKnown || call.SIPErrorCode != 599 {
				t.Errorf("SIP error = %d, known %t; want 599, true", call.SIPErrorCode, call.SIPErrorCodeKnown)
			}
			if !call.RTTModeKnown || call.RTTMode != VoiceRTTFull {
				t.Errorf("RTT mode = %d, known %t; want full, true", call.RTTMode, call.RTTModeKnown)
			}
			if call.SecureKnown != tt.wantSecure || call.Secure != tt.wantSecure {
				t.Errorf("secure = %t, known %t; want %t", call.Secure, call.SecureKnown, tt.wantSecure)
			}
		})
	}
}

func TestVoiceDecodeCallInfoDetails(t *testing.T) {
	base := tlv.TLVs{
		tlv.Bytes(0x10, []byte{7, byte(VoiceCallStateConversation), byte(VoiceCallTypeVoiceIP), byte(VoiceCallDirectionMT), byte(VoiceCallModeLTE)}),
		tlv.Bytes(0x11, []byte{byte(VoicePresentationAllowed), 3, '1', '2', '3'}),
		tlv.Bytes(0x13, []byte{byte(VoicePrivacyEnhanced)}),
		tlv.Bytes(0x15, []byte{byte(VoicePresentationAllowed), 0, 3, 'B', 'o', 'b'}),
		tlv.Bytes(0x1C, binary.LittleEndian.AppendUint64(nil, uint64(VoiceCallAttributeTX|VoiceCallAttributeRX))),
		tlv.Bytes(0x1D, binary.LittleEndian.AppendUint64(nil, uint64(VoiceCallAttributeRX))),
		tlv.Bytes(0x1F, []byte("sip:7")),
		tlv.Bytes(0x20, []byte{1}),
		tlv.Bytes(0x23, []byte{2, 'I', 0, 'P', 0}),
		tlv.Bytes(0x24, []byte{2, 'O', 0, 'K', 0}),
		tlv.Bytes(0x26, binary.LittleEndian.AppendUint16(nil, 599)),
		tlv.Bytes(0x28, binary.LittleEndian.AppendUint32(nil, uint32(VoiceRTTFull))),
	}
	malformed := slices.Clone(base)
	for i := range malformed {
		if malformed[i].Type == 0x1C {
			malformed[i] = tlv.Bytes(0x1C, []byte{1})
			break
		}
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{name: "all common details", tlvs: base},
		{name: "malformed audio attributes", tlvs: malformed, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var call VoiceCall
			err := call.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if call.Number != "123" || !call.NumberKnown || call.Privacy != VoicePrivacyEnhanced || !call.PrivacyKnown {
				t.Errorf("number/privacy = %+v", call)
			}
			if call.CallerName != "IP" || !call.CallerNameKnown || !call.CallerNameCodingSchemeKnown {
				t.Errorf("caller name = %q, known %t, coding known %t", call.CallerName, call.CallerNameKnown, call.CallerNameCodingSchemeKnown)
			}
			if call.AudioAttributes != VoiceCallAttributeTX|VoiceCallAttributeRX || !call.AudioAttributesKnown || call.VideoAttributes != VoiceCallAttributeRX || !call.VideoAttributesKnown {
				t.Errorf("media attributes = %+v", call)
			}
			if call.SIPURI != "sip:7" || !call.SIPURIKnown || !call.SRVCC || !call.SRVCCKnown {
				t.Errorf("SIP/SRVCC = %+v", call)
			}
			if len(call.EndReasonText) != 2 || call.EndReasonText[0] != 'O' || call.EndReasonText[1] != 'K' || call.SIPErrorCode != 599 || !call.SIPErrorCodeKnown || call.RTTMode != VoiceRTTFull || !call.RTTModeKnown {
				t.Errorf("end/RTT details = %+v", call)
			}
		})
	}
}

func TestVoiceDecodeDTMFAndUSSD(t *testing.T) {
	var dtmf VoiceDTMFEvent
	err := dtmf.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(0x01, []byte{7, byte(VoiceDTMFEventForwardBurst), 2, '1', '#'}),
		tlv.Bytes(0x10, []byte{byte(VoiceDTMFOn150ms)}),
		tlv.Bytes(0x11, []byte{byte(VoiceDTMFOff100ms)}),
		tlv.Bytes(0x12, binary.LittleEndian.AppendUint16(nil, 10)),
	})
	if err != nil {
		t.Fatalf("VoiceDTMFEvent.UnmarshalTLVs() error = %v", err)
	}
	if dtmf.CallID != 7 || dtmf.Digits != "1#" || !dtmf.VolumeKnown || dtmf.Volume != 10 {
		t.Fatalf("dtmf = %+v", dtmf)
	}

	var data VoiceUSSDData
	err = data.UnmarshalBinary([]byte{byte(VoiceUSSDEncodingASCII), 3, 'a', 'b', 'c'})
	if err != nil || string(data.Data) != "abc" {
		t.Fatalf("VoiceUSSDData.UnmarshalBinary() = %+v, error = %v", data, err)
	}
	var result VoiceUSSDResult
	err = result.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(0x12, []byte{byte(VoiceUSSDEncodingASCII), 1, 'x'}),
		tlv.Bytes(0x16, []byte{2, 0x41, 0, 0x42, 0}),
		tlv.Bytes(0x18, binary.LittleEndian.AppendUint16(nil, 500)),
	})
	if err != nil || !result.DataKnown || string(result.Data.Data) != "x" || len(result.UTF16) != 2 || result.SIPErrorCode != 500 {
		t.Fatalf("result = %+v, error = %v", result, err)
	}

	var event VoiceUSSDEvent
	err = event.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(VoiceUSSDActionRequired)}),
		tlv.Bytes(0x10, []byte{byte(VoiceUSSDEncoding8Bit), 1, 0xFF}),
	})
	if err != nil || event.Action != VoiceUSSDActionRequired || !event.DataKnown || event.Data.Data[0] != 0xFF {
		t.Fatalf("event = %+v, error = %v", event, err)
	}
}

func TestVoiceWatchCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &voiceIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{resp: successResponse(MessageVoiceIndicationRegister)},
				{resp: successResponse(MessageVoiceIndicationRegister)},
			},
		},
	}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
	out, err := client.VoiceWatchCalls(ctx)
	if err != nil {
		t.Fatalf("VoiceWatchCalls() error = %v", err)
	}
	transport.emit(Indication{
		Service:   ServiceVoice,
		ClientID:  7,
		MessageID: MessageVoiceAllCallStatus,
		TLVs:      tlv.TLVs{tlv.Bytes(0x01, []byte{1, 7, byte(VoiceCallStateConversation), byte(VoiceCallTypeVoice), byte(VoiceCallDirectionMO), byte(VoiceCallModeLTE), 0, 0})},
	})
	select {
	case calls := <-out:
		if len(calls) != 1 || calls[0].ID != 7 {
			t.Fatalf("calls = %+v", calls)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Voice call indication")
	}
	cancel()
	transport.waitCalls(t, 2)
}

func TestVoiceIndicationRegistrationReferences(t *testing.T) {
	tests := []struct {
		name         string
		registration voiceIndicationRegistration
		tlvType      byte
	}{
		{name: "calls", registration: voiceIndicationCalls, tlvType: 0x13},
		{name: "DTMF", registration: voiceIndicationDTMF, tlvType: 0x10},
		{name: "USSD", registration: voiceIndicationUSSD, tlvType: 0x16},
		{name: "supplementary notification", registration: voiceIndicationSupplementaryNotification, tlvType: 0x12},
		{name: "supplementary result", registration: voiceIndicationSupplementaryResult, tlvType: 0x17},
		{name: "modification", registration: voiceIndicationModification, tlvType: 0x18},
		{name: "handover", registration: voiceIndicationHandover, tlvType: 0x14},
		{name: "speech", registration: voiceIndicationSpeech, tlvType: 0x15},
		{name: "privacy", registration: voiceIndicationPrivacy, tlvType: 0x11},
		{name: "TTY", registration: voiceIndicationTTY, tlvType: 0x20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) { assertTLV(t, req.TLVs, tt.tlvType, []byte{1}) },
					resp:  successResponse(MessageVoiceIndicationRegister),
				},
				{
					check: func(req Request) { assertTLV(t, req.TLVs, tt.tlvType, []byte{0}) },
					resp:  successResponse(MessageVoiceIndicationRegister),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}

			if err := client.acquireVoiceIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("first acquireVoiceIndication() error = %v", err)
			}
			if err := client.acquireVoiceIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("second acquireVoiceIndication() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseVoiceIndication(tt.registration)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseVoiceIndication(tt.registration)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

type voiceIndicationTransport struct {
	fakeTransport
	indications chan Indication
}

func (t *voiceIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, _ MessageID) (<-chan Indication, error) {
	t.indications = make(chan Indication, 4)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *voiceIndicationTransport) emit(ind Indication) {
	t.indications <- ind
}

func (t *voiceIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}
