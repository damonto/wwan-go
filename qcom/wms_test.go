package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSRequestEncoding(t *testing.T) {
	mode := WMSMessageModeGW
	tag := WMSTagMONotSent
	index := uint32(0x11223344)
	ims := true
	forceOnDC := WMSCDMAForceOnDC{Force: true, ServiceOption: WMSCDMAServiceOption14}
	failure3GPP2 := WMSAck3GPP2Failure{ErrorClass: 2, CauseCode: 0x60}
	failure3GPP := WMSAck3GPPFailure{RPCause: 0x21, TPCause: 0xD3}
	tests := []struct {
		name  string
		call  func(*Client) error
		want  MessageID
		check func(*testing.T, Request)
		resp  Response
	}{
		{
			name: "raw send",
			call: func(c *Client) error {
				_, err := c.WMSSendRaw(context.Background(), WMSMessageFormatGWPointToPoint, []byte{0x01, 0x02}, WMSSendOptions{
					ForceOnDC:      &forceOnDC,
					FollowOnDC:     true,
					LinkTimer:      ptr(uint8(7)),
					SMSOnIMS:       &ims,
					RetryMessageID: ptr(uint32(9)),
					CommandTPDU:    ptr(true),
				})
				return err
			},
			want: MessageWMSRawSend,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0x06, 0x02, 0x00, 0x01, 0x02})
				assertTLV(t, req.TLVs, 0x10, []byte{0x01, 0x0E})
				assertTLV(t, req.TLVs, 0x11, []byte{0x01})
				assertTLV(t, req.TLVs, 0x12, []byte{0x07})
				assertTLV(t, req.TLVs, 0x13, []byte{0x01})
				assertTLV(t, req.TLVs, 0x14, []byte{0x01})
				assertTLV(t, req.TLVs, 0x15, binary.LittleEndian.AppendUint32(nil, 9))
				assertTLV(t, req.TLVs, 0x18, []byte{0x01})
			},
			resp: successResponse(MessageWMSRawSend, tlv.Bytes(0x01, []byte{0x34, 0x12})),
		},
		{
			name: "raw write",
			call: func(c *Client) error {
				_, err := c.WMSWriteRaw(context.Background(), WMSWriteRequest{
					Storage: WMSStorageNV,
					Format:  WMSMessageFormatCDMA,
					Data:    []byte{0xAA},
					Tag:     &tag,
				})
				return err
			},
			want: MessageWMSRawWrite,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0x01, 0x00, 0x01, 0x00, 0xAA})
				assertTLV(t, req.TLVs, 0x10, []byte{byte(tag)})
			},
			resp: successResponse(MessageWMSRawWrite, tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, 12))),
		},
		{
			name: "raw read",
			call: func(c *Client) error {
				_, err := c.WMSReadRaw(context.Background(), WMSReadRequest{
					Reference:   WMSMessageReference{Storage: WMSStorageUIM, Index: index},
					MessageMode: &mode,
					SMSOnIMS:    &ims,
				})
				return err
			},
			want: MessageWMSRawRead,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, append([]byte{0x00}, binary.LittleEndian.AppendUint32(nil, index)...))
				assertTLV(t, req.TLVs, 0x10, []byte{byte(mode)})
				assertTLV(t, req.TLVs, 0x11, []byte{0x01})
			},
			resp: successResponse(MessageWMSRawRead, tlv.Bytes(0x01, []byte{byte(WMSTagMTRead), byte(WMSMessageFormatCDMA), 1, 0, 0xAA})),
		},
		{
			name: "delete",
			call: func(c *Client) error {
				return c.WMSDelete(context.Background(), WMSDeleteRequest{Storage: WMSStorageNV, Index: &index, Tag: &tag, MessageMode: &mode})
			},
			want: MessageWMSDelete,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0x01})
				assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint32(nil, index))
				assertTLV(t, req.TLVs, 0x11, []byte{byte(tag)})
				assertTLV(t, req.TLVs, 0x12, []byte{byte(mode)})
			},
			resp: successResponse(MessageWMSDelete),
		},
		{
			name: "list",
			call: func(c *Client) error {
				_, err := c.WMSListMessages(context.Background(), WMSListRequest{Storage: WMSStorageNV, Tag: &tag, MessageMode: &mode})
				return err
			},
			want: MessageWMSListMessages,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0x01})
				assertTLV(t, req.TLVs, 0x10, []byte{byte(tag)})
				assertTLV(t, req.TLVs, 0x11, []byte{byte(mode)})
			},
			resp: successResponse(MessageWMSListMessages, tlv.Bytes(0x01, []byte{0, 0, 0, 0})),
		},
		{
			name: "event report",
			call: func(c *Client) error {
				return c.WMSSetEventReport(context.Background(), WMSEventReportConfig{MTMessages: &ims, LowerLayerErrors: &ims})
			},
			want: MessageWMSSetEventReport,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x10, []byte{0x01})
				assertTLV(t, req.TLVs, 0x13, []byte{0x01})
			},
			resp: successResponse(MessageWMSSetEventReport),
		},
		{
			name: "ack",
			call: func(c *Client) error {
				return c.WMSAcknowledge(context.Background(), WMSAckRequest{
					TransactionID: 0xAABBCCDD,
					Protocol:      WMSMessageProtocolWCDMA,
					Success:       false,
					Failure3GPP2:  &failure3GPP2,
					Failure3GPP:   &failure3GPP,
					SMSOnIMS:      &ims,
				})
			},
			want: MessageWMSSendAck,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0xDD, 0xCC, 0xBB, 0xAA, 0x01, 0x00})
				assertTLV(t, req.TLVs, 0x10, []byte{0x02, 0x60})
				assertTLV(t, req.TLVs, 0x11, []byte{0x21, 0xD3})
				assertTLV(t, req.TLVs, 0x12, []byte{0x01})
			},
			resp: successResponse(MessageWMSSendAck),
		},
		{
			name: "set SMSC",
			call: func(c *Client) error {
				return c.WMSSetSMSCAddress(context.Background(), WMSSMSCAddress{Type: "145", Digits: "+8613800"}, ptr(uint8(2)))
			},
			want: MessageWMSSetSMSCAddress,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte("+8613800"))
				assertTLV(t, req.TLVs, 0x10, []byte("145"))
				assertTLV(t, req.TLVs, 0x11, []byte{0x02})
			},
			resp: successResponse(MessageWMSSetSMSCAddress),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWMS || req.ClientID != 7 || req.MessageID != tt.want {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want WMS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.want)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestWMSWriteRawRejectsMalformedStorageIndex(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "truncated", value: make([]byte, 3)},
		{name: "trailing byte", value: make([]byte, 5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: successResponse(MessageWMSRawWrite, tlv.Bytes(0x01, tt.value)),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			_, err := client.WMSWriteRaw(context.Background(), WMSWriteRequest{
				Storage: WMSStorageNV,
				Format:  WMSMessageFormatCDMA,
				Data:    []byte{0xAA},
			})
			if err == nil {
				t.Fatal("WMSWriteRaw() error = nil, want non-nil")
			}
		})
	}
}

func TestWMSResponseDecoding(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "raw read data truncated",
			fn: func() error {
				return new(WMSRawMessage).UnmarshalBinary([]byte{0, 6, 2, 0, 0x01})
			},
		},
		{
			name: "raw read trailing data",
			fn: func() error {
				return new(WMSRawMessage).UnmarshalBinary([]byte{0, 6, 1, 0, 0x01, 0x02})
			},
		},
		{
			name: "message list entry truncated",
			fn: func() error {
				_, err := decodeWMSMessageList(WMSStorageUIM, []byte{1, 0, 0, 0, 1, 2})
				return err
			},
		},
		{
			name: "message list trailing data",
			fn: func() error {
				_, err := decodeWMSMessageList(WMSStorageUIM, []byte{0, 0, 0, 0, 1})
				return err
			},
		},
		{
			name: "incoming transfer truncated",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x11, []byte{0, 1, 0, 0, 0, 6, 2, 0, 1})})
			},
		},
		{
			name: "incoming transfer trailing data",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x11, []byte{0, 1, 0, 0, 0, 6, 1, 0, 1, 2})})
			},
		},
		{
			name: "incoming message mode trailing data",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x10, []byte{0, 1, 0, 0, 0}),
					tlv.Bytes(0x12, []byte{1, 2}),
				})
			},
		},
		{
			name: "ETWS trailing data",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x13, []byte{0, 1, 0, 0xAA, 0xBB})})
			},
		},
		{
			name: "ETWS PLMN truncated",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x13, []byte{0, 0, 0}),
					tlv.Bytes(0x14, []byte{1, 0, 2}),
				})
			},
		},
		{
			name: "SMS on IMS trailing data",
			fn: func() error {
				return new(WMSIncomingMessage).UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x10, []byte{0, 1, 0, 0, 0}),
					tlv.Bytes(0x16, []byte{1, 0}),
				})
			},
		},
		{
			name: "SMSC digits truncated",
			fn: func() error {
				return new(WMSSMSCAddress).UnmarshalBinary([]byte{'1', '4', '5', 4, '+', '8'})
			},
		},
		{
			name: "SMSC trailing data",
			fn: func() error {
				return new(WMSSMSCAddress).UnmarshalBinary([]byte{'1', '4', '5', 1, '+', '8'})
			},
		},
		{
			name: "raw send GSM cause trailing data",
			fn: func() error {
				var result WMSSendResult
				return result.unmarshalRawSendTLVs(tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0, 2, 3})})
			},
		},
		{
			name: "send from store delivery failure trailing data",
			fn: func() error {
				var result WMSSendResult
				return result.unmarshalSendFromStoreTLVs(tlv.TLVs{tlv.Bytes(0x14, []byte{1, 0})})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("decode() error = nil, want error")
			}
		})
	}
}

func TestWMSSendResultDecoding(t *testing.T) {
	tests := []struct {
		name   string
		decode func() (WMSSendResult, error)
	}{
		{
			name: "raw send",
			decode: func() (WMSSendResult, error) {
				var result WMSSendResult
				err := result.unmarshalRawSendTLVs(tlv.TLVs{
					tlv.Bytes(0x01, []byte{0x34, 0x12}),
					tlv.Bytes(0x10, []byte{0x09, 0x00}),
					tlv.Bytes(0x11, []byte{0x02}),
					tlv.Bytes(0x12, []byte{0x21, 0x00, 0xD3}),
					tlv.Bytes(0x13, []byte{byte(WMSDeliveryFailurePermanent)}),
					tlv.Bytes(0x14, []byte{byte(WMSDeliveryBlockedByVoiceOrDataCall)}),
					tlv.Bytes(0x15, []byte{3, 'S', 'M', 'S'}),
					tlv.Bytes(0x16, []byte{1, 0, 0, 0, 0x42}),
					tlv.Bytes(0x17, []byte{0x88, 0x77}),
				})
				return result, err
			},
		},
		{
			name: "send from store",
			decode: func() (WMSSendResult, error) {
				var result WMSSendResult
				err := result.unmarshalSendFromStoreTLVs(tlv.TLVs{
					tlv.Bytes(0x10, []byte{0x34, 0x12}),
					tlv.Bytes(0x11, []byte{0x09, 0x00}),
					tlv.Bytes(0x12, []byte{0x02}),
					tlv.Bytes(0x13, []byte{0x21, 0x00, 0xD3}),
					tlv.Bytes(0x14, []byte{byte(WMSDeliveryFailurePermanent)}),
					tlv.Bytes(0x15, []byte{1, 0, 0, 0, 0x42}),
					tlv.Bytes(0x16, []byte{byte(WMSDeliveryBlockedByVoiceOrDataCall)}),
					tlv.Bytes(0x17, []byte{3, 'S', 'M', 'S'}),
					tlv.Bytes(0x18, []byte{0x88, 0x77}),
				})
				return result, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode()
			if err != nil {
				t.Fatalf("decode() error = %v", err)
			}
			if !got.MessageIDKnown || got.MessageID != 0x1234 || !got.CauseCodeKnown || got.CauseCode != 9 || !got.ErrorClassKnown || got.ErrorClass != 2 {
				t.Fatalf("CDMA result = %+v", got)
			}
			if !got.GSMCauseKnown || got.GSMCause != (WMSGSMCauseInfo{RPCause: 0x21, TPCause: 0xD3}) {
				t.Fatalf("GSM result = %+v", got)
			}
			if !got.DeliveryFailureKnown || got.DeliveryFailure != WMSDeliveryFailurePermanent {
				t.Fatalf("delivery result = %+v", got)
			}
			if !got.DeliveryCauseKnown || got.DeliveryCause != WMSDeliveryBlockedByVoiceOrDataCall {
				t.Fatalf("delivery cause = %+v", got)
			}
			if !got.CallControlKnown || !bytes.Equal(got.CallControlAlphaID, []byte("SMS")) {
				t.Fatalf("call-control result = %+v", got)
			}
			if !got.RejectCauseKnown || got.RejectCause != (WMSRejectCause{Type: 1, Value: 0x42}) {
				t.Fatalf("reject cause = %+v", got)
			}
			if !got.IMSRejectCauseKnown || got.IMSRejectCause != 0x7788 {
				t.Fatalf("IMS reject cause = %+v", got)
			}
		})
	}
}

func TestWMSSendResultDecodingRejectsMalformedOptionalFields(t *testing.T) {
	tests := []struct {
		name    string
		decode  func(tlv.TLVs) error
		tlvs    tlv.TLVs
		wantErr string
	}{
		{
			name: "raw delivery cause",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalRawSendTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x14, []byte{0, 1})},
			wantErr: "delivery failure cause TLV length 2, want 1",
		},
		{
			name: "raw call-control",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalRawSendTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x15, []byte{2, 'A'})},
			wantErr: "call-control alpha ID value length 2, want 3",
		},
		{
			name: "raw reject cause",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalRawSendTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x16, []byte{1, 0, 0, 0})},
			wantErr: "reject cause length 4, want 5",
		},
		{
			name: "send-from-store reject cause",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalSendFromStoreTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x15, []byte{1, 0, 0, 0})},
			wantErr: "reject cause length 4, want 5",
		},
		{
			name: "send-from-store delivery cause",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalSendFromStoreTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Uint(0x16, uint8(2))},
			wantErr: "delivery failure cause 2 is out of range",
		},
		{
			name: "send-from-store call-control",
			decode: func(tlvs tlv.TLVs) error {
				var result WMSSendResult
				return result.unmarshalSendFromStoreTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x17, nil)},
			wantErr: "call-control alpha ID length is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.decode(tt.tlvs)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("decode() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWMSAcknowledgeFailure(t *testing.T) {
	tests := []struct {
		name      string
		response  Response
		wantTyped bool
	}{
		{
			name: "network released link",
			response: errorResponse(
				MessageWMSSendAck,
				QMIErrorAckNotSent,
				tlv.Bytes(0x10, []byte{byte(WMSAckFailureNetworkReleasedLink)}),
			),
			wantTyped: true,
		},
		{
			name:      "malformed failure cause",
			response:  errorResponse(MessageWMSSendAck, QMIErrorAckNotSent, tlv.Bytes(0x10, []byte{0, 1})),
			wantTyped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{resp: tt.response}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			err := client.WMSAcknowledge(context.Background(), WMSAckRequest{})
			if err == nil {
				t.Fatal("WMSAcknowledge() error = nil, want non-nil")
			}
			var ackErr *WMSAckError
			if errors.As(err, &ackErr) != tt.wantTyped {
				t.Fatalf("errors.As(WMSAckError) = %t, want %t; error = %v", ackErr != nil, tt.wantTyped, err)
			}
			if tt.wantTyped {
				if !errors.Is(err, QMIErrorAckNotSent) || !ackErr.FailureCauseKnown || ackErr.FailureCause != WMSAckFailureNetworkReleasedLink {
					t.Fatalf("WMSAcknowledge() error = %#v", ackErr)
				}
			}
		})
	}
}

func TestWMSDecodeIncomingMetadata(t *testing.T) {
	tests := []struct {
		name  string
		tlvs  tlv.TLVs
		check func(*testing.T, WMSIncomingMessage)
	}{
		{
			name: "transfer metadata",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x11, []byte{0, 1, 0, 0, 0, 6, 1, 0, 0xAA}),
				tlv.Bytes(0x12, []byte{byte(WMSMessageModeGW)}),
				tlv.Bytes(0x15, []byte("+8613800")),
			},
			check: func(t *testing.T, got WMSIncomingMessage) {
				if !got.MessageModeKnown || got.MessageMode != WMSMessageModeGW || !got.SMSCAddressKnown || got.SMSCAddress != "+8613800" {
					t.Fatalf("incoming metadata = %+v", got)
				}
			},
		},
		{
			name: "ETWS only",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x13, []byte{byte(WMSETWSNotificationPrimary), 2, 0, 0xAA, 0xBB}),
				tlv.Bytes(0x14, []byte{0xCC, 0x01, 0x01, 0x00}),
			},
			check: func(t *testing.T, got WMSIncomingMessage) {
				if !got.ETWSKnown || got.ETWSNotification != WMSETWSNotificationPrimary || !bytes.Equal(got.ETWSData, []byte{0xAA, 0xBB}) {
					t.Fatalf("ETWS message = %+v", got)
				}
				if !got.ETWSPLMNKnown || got.ETWSPLMN != (WMSPLMN{MCC: 460, MNC: 1}) {
					t.Fatalf("ETWS PLMN = %+v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WMSIncomingMessage
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestWMSDecodeIncomingVariants(t *testing.T) {
	var stored WMSIncomingMessage
	err := stored.UnmarshalTLVs(tlv.TLVs{
		tlv.Bytes(0x10, []byte{byte(WMSStorageNV), 1, 0, 0, 0}),
		tlv.Bytes(0x16, []byte{1}),
	})
	if err != nil {
		t.Fatalf("stored decode error = %v", err)
	}
	if !stored.Stored || stored.Reference != (WMSMessageReference{Storage: WMSStorageNV, Index: 1}) || !stored.SMSOnIMS || !stored.SMSOnIMSKnown {
		t.Fatalf("stored = %+v", stored)
	}

	transferValue := []byte{byte(WMSAckRequired), 1, 0, 0, 0, byte(WMSMessageFormatGWPointToPoint), 2, 0, 0xAA, 0xBB}
	var transfer WMSIncomingMessage
	err = transfer.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x11, transferValue)})
	if err != nil {
		t.Fatalf("transfer decode error = %v", err)
	}
	if transfer.Stored || transfer.AckIndicator != WMSAckRequired || transfer.TransactionID != 1 || !bytes.Equal(transfer.Data, []byte{0xAA, 0xBB}) {
		t.Fatalf("transfer = %+v", transfer)
	}
}

func TestWMSWatchIncomingReadsStoredMessages(t *testing.T) {
	tests := []struct {
		name             string
		messageModeKnown bool
		messageMode      WMSMessageMode
		smsOnIMSKnown    bool
		smsOnIMS         bool
		wantMode         WMSMessageMode
		wantFormat       WMSMessageFormat
	}{
		{
			name:       "default to GW message mode",
			wantMode:   WMSMessageModeGW,
			wantFormat: WMSMessageFormatGWPointToPoint,
		},
		{
			name:             "preserve indication metadata",
			messageModeKnown: true,
			messageMode:      WMSMessageModeCDMA,
			smsOnIMSKnown:    true,
			smsOnIMS:         true,
			wantMode:         WMSMessageModeCDMA,
			wantFormat:       WMSMessageFormatCDMA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			transport := &wmsIndicationTransport{
				fakeTransport: fakeTransport{
					t: t,
					calls: []transportCall{
						{resp: successResponse(MessageWMSSetEventReport)},
						{
							check: func(req Request) {
								if req.MessageID != MessageWMSRawRead {
									t.Fatalf("read MessageID = 0x%04X, want raw read", req.MessageID)
								}
								assertTLV(t, req.TLVs, 0x01, []byte{byte(WMSStorageNV), 3, 0, 0, 0})
								assertTLV(t, req.TLVs, 0x10, []byte{byte(tt.wantMode)})
								ims, hasIMS := tlv.Value(req.TLVs, 0x11)
								if !tt.smsOnIMSKnown {
									if hasIMS {
										t.Fatalf("SMS on IMS TLV = % X, want omitted", ims)
									}
								} else {
									assertTLV(t, req.TLVs, 0x11, []byte{boolByte(tt.smsOnIMS)})
								}
							},
							resp: successResponse(MessageWMSRawRead, tlv.Bytes(0x01, []byte{byte(WMSTagMTNotRead), byte(tt.wantFormat), 2, 0, 0x01, 0x02})),
						},
						{resp: successResponse(MessageWMSSetEventReport)},
					},
				},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			out, err := client.WMSWatchIncoming(ctx)
			if err != nil {
				cancel()
				t.Fatalf("WMSWatchIncoming() error = %v", err)
			}

			indicationTLVs := tlv.TLVs{tlv.Bytes(0x10, []byte{byte(WMSStorageNV), 3, 0, 0, 0})}
			if tt.messageModeKnown {
				indicationTLVs = append(indicationTLVs, tlv.Uint(0x12, uint8(tt.messageMode)))
			}
			if tt.smsOnIMSKnown {
				indicationTLVs = append(indicationTLVs, tlv.Uint(0x16, boolByte(tt.smsOnIMS)))
			}
			transport.emit(Indication{
				Service:   ServiceWMS,
				ClientID:  7,
				MessageID: MessageWMSEventReport,
				TLVs:      indicationTLVs,
			})

			select {
			case got := <-out:
				if !got.Stored || got.Reference.Index != 3 || got.Tag != WMSTagMTNotRead || got.Format != tt.wantFormat || !bytes.Equal(got.Data, []byte{1, 2}) {
					t.Fatalf("incoming = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for WMS incoming message")
			}
			cancel()
			transport.waitCalls(t, 3)
		})
	}
}

func TestWMSIndicationRegistrationReferences(t *testing.T) {
	tests := []struct {
		name         string
		registration wmsIndicationRegistration
		messageID    MessageID
		tlvType      byte
	}{
		{name: "mobile terminated messages", registration: wmsIndicationMT, messageID: MessageWMSSetEventReport, tlvType: 0x10},
		{name: "transport layer", registration: wmsIndicationTransportLayer, messageID: MessageWMSIndicationRegister, tlvType: 0x10},
		{name: "transport network", registration: wmsIndicationTransportNetwork, messageID: MessageWMSIndicationRegister, tlvType: 0x11},
		{name: "service ready", registration: wmsIndicationServiceReady, messageID: MessageWMSIndicationRegister, tlvType: 0x13},
		{name: "SMSC address", registration: wmsIndicationSMSCAddress, messageID: MessageWMSIndicationRegister, tlvType: 0x17},
		{name: "memory full", registration: wmsIndicationMemoryFull, messageID: MessageWMSIndicationRegister, tlvType: 0x18},
		{name: "message waiting", registration: wmsIndicationMessageWaiting, messageID: MessageWMSSetEventReport, tlvType: 0x12},
		{name: "call status", registration: wmsIndicationCallStatus, messageID: MessageWMSIndicationRegister, tlvType: 0x12},
		{name: "broadcast config", registration: wmsIndicationBroadcastConfig, messageID: MessageWMSIndicationRegister, tlvType: 0x14},
		{name: "transport MWI", registration: wmsIndicationTransportMWI, messageID: MessageWMSIndicationRegister, tlvType: 0x15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != tt.messageID {
							t.Fatalf("register MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.messageID)
						}
						assertTLV(t, req.TLVs, tt.tlvType, []byte{1})
					},
					resp: successResponse(tt.messageID),
				},
				{
					check: func(req Request) {
						if req.MessageID != tt.messageID {
							t.Fatalf("unregister MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.messageID)
						}
						assertTLV(t, req.TLVs, tt.tlvType, []byte{0})
					},
					resp: successResponse(tt.messageID),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}

			if err := client.acquireWMSIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("first acquireWMSIndication() error = %v", err)
			}
			if err := client.acquireWMSIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("second acquireWMSIndication() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseWMSIndication(tt.registration)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseWMSIndication(tt.registration)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

type wmsIndicationTransport struct {
	fakeTransport
	indications chan Indication
}

func (t *wmsIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, _ MessageID) (<-chan Indication, error) {
	t.indications = make(chan Indication, 4)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *wmsIndicationTransport) emit(ind Indication) {
	t.indications <- ind
}

func (t *wmsIndicationTransport) waitCalls(tb testing.TB, want int) {
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
