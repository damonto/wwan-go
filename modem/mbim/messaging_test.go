package mbim

import (
	"errors"
	"slices"
	"testing"

	mbimproto "github.com/damonto/wwan-go/mbim"
	smscodec "github.com/damonto/wwan-go/modem/sms"
)

func TestFlashMessagePartsKeepsValidRecords(t *testing.T) {
	tests := []struct {
		name    string
		invalid []int
	}{
		{name: "first record malformed", invalid: []int{0}},
		{name: "first and last records malformed", invalid: []int{0, 2}},
		{name: "all records malformed", invalid: []int{0, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := mbimproto.SMSReadInfo{Format: mbimproto.SMSFormatPDU}
			for i := range 3 {
				pdu := messagingTestPDU([]byte("hello"))
				if slices.Contains(tt.invalid, i) {
					pdu = []byte{0}
				}
				read.PDURecords = append(read.PDURecords, mbimproto.SMSPDURecord{MessageStatus: mbimproto.SMSStatusNew, PDU: pdu})
			}
			parts, err := flashMessageParts(read)
			listErr, ok := errors.AsType[*MessageListError](err)
			if !ok || len(listErr.Errors) != len(tt.invalid) {
				t.Fatalf("flashMessageParts() error = %v, want %d decoding errors", err, len(tt.invalid))
			}
			if len(parts) != 3-len(tt.invalid) {
				t.Fatalf("flashMessageParts() returned %d parts, want %d", len(parts), 3-len(tt.invalid))
			}
			for _, part := range parts {
				if part.Message.Text != "hello" || part.Message.Storage != MessageStorageUnknown || len(part.Message.Refs) != 0 {
					t.Errorf("flash message = %+v, want unstored hello", part.Message)
				}
			}
		})
	}
}

func TestFlashMessageParts(t *testing.T) {
	pdus, err := smscodec.EncodePDUs(MessageConfig{Number: "+15551234", Text: "hello"})
	if err != nil {
		t.Fatalf("EncodePDUs() error = %v", err)
	}

	tests := []struct {
		name    string
		read    mbimproto.SMSReadInfo
		wantErr bool
	}{
		{
			name: "decodes PDU flash message",
			read: mbimproto.SMSReadInfo{
				Format: mbimproto.SMSFormatPDU,
				PDURecords: []mbimproto.SMSPDURecord{{
					MessageStatus: mbimproto.SMSStatusNew,
					PDU:           pdus[0],
				}},
			},
		},
		{
			name:    "rejects unsupported CDMA format",
			read:    mbimproto.SMSReadInfo{Format: mbimproto.SMSFormatCDMA},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := flashMessageParts(tt.read)
			if (err != nil) != tt.wantErr {
				t.Fatalf("flashMessageParts() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(parts) != 1 {
				t.Fatalf("flashMessageParts() returned %d parts, want 1", len(parts))
			}
			message := parts[0].Message
			if message.Text != "hello" || message.State != MessageStateReceivedUnread {
				t.Fatalf("flash message = %+v, want text hello and unread state", message)
			}
			if message.Storage != MessageStorageUnknown || message.ID != 0 || len(message.Refs) != 0 {
				t.Fatalf("flash message storage metadata = %+v, want unstored message", message)
			}
		})
	}
}
