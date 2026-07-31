package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestCTLRequests(t *testing.T) {
	tests := []struct {
		name      string
		messageID MessageID
		response  Response
		check     func(*testing.T, Request)
		call      func(*testing.T, *Client)
	}{
		{
			name:      "set instance ID",
			messageID: MessageCTLSetInstanceID,
			response: successResponse(MessageCTLSetInstanceID,
				tlv.Uint(0x01, uint16(0x1234)),
			),
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0x07})
			},
			call: func(t *testing.T, client *Client) {
				linkID, err := client.CTLSetInstanceID(context.Background(), 7)
				if err != nil {
					t.Fatalf("CTLSetInstanceID() error = %v", err)
				}
				if linkID != 0x1234 {
					t.Fatalf("CTLSetInstanceID() = 0x%04X, want 0x1234", linkID)
				}
			},
		},
		{
			name:      "set data format",
			messageID: MessageCTLSetDataFormat,
			response: successResponse(MessageCTLSetDataFormat,
				tlv.Uint(0x10, uint16(CTLLinkProtocolRawIP)),
			),
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{byte(CTLQoSHeaderAbsent)})
				assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint16(nil, uint16(CTLLinkProtocolEthernet|CTLLinkProtocolRawIP)))
			},
			call: func(t *testing.T, client *Client) {
				protocol, err := client.CTLSetDataFormat(context.Background(), CTLDataFormatConfig{
					LinkProtocols: CTLLinkProtocolEthernet | CTLLinkProtocolRawIP,
				})
				if err != nil {
					t.Fatalf("CTLSetDataFormat() error = %v", err)
				}
				if protocol != CTLLinkProtocolRawIP {
					t.Fatalf("CTLSetDataFormat() = 0x%X, want raw IP", protocol)
				}
			},
		},
		{
			name:      "sync",
			messageID: MessageCTLSync,
			response:  successResponse(MessageCTLSync),
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLV count = %d, want 0", len(req.TLVs))
				}
			},
			call: func(t *testing.T, client *Client) {
				if err := client.CTLSync(context.Background()); err != nil {
					t.Fatalf("CTLSync() error = %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceControl || req.ClientID != 0 || req.MessageID != tt.messageID {
						t.Fatalf("request = %+v, want CTL message 0x%04X", req, tt.messageID)
					}
					if req.TransactionID == 0 || req.TransactionID > 0xff {
						t.Fatalf("transaction ID = %d, want 1..255", req.TransactionID)
					}
					tt.check(t, req)
				},
				resp: tt.response,
			}}}
			client := &Client{transport: transport}
			tt.call(t, client)
		})
	}
}

func TestCTLRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name  string
		calls []transportCall
		call  func(*Client) error
	}{
		{
			name: "QoS header out of range",
			call: func(client *Client) error {
				_, err := client.CTLSetDataFormat(context.Background(), CTLDataFormatConfig{QoSHeader: 2})
				return err
			},
		},
		{
			name: "unsupported link protocol",
			call: func(client *Client) error {
				_, err := client.CTLSetDataFormat(context.Background(), CTLDataFormatConfig{LinkProtocols: 1 << 8})
				return err
			},
		},
		{
			name:  "instance response missing link ID",
			calls: []transportCall{{resp: successResponse(MessageCTLSetInstanceID)}},
			call: func(client *Client) error {
				_, err := client.CTLSetInstanceID(context.Background(), 1)
				return err
			},
		},
		{
			name:  "data format response missing protocol",
			calls: []transportCall{{resp: successResponse(MessageCTLSetDataFormat)}},
			call: func(client *Client) error {
				_, err := client.CTLSetDataFormat(context.Background(), CTLDataFormatConfig{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport}
			if err := tt.call(client); err == nil {
				t.Fatal("call() error = nil, want validation or response error")
			}
		})
	}
}

func TestCTLWatchSync(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forwards synchronization event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &fakeIndicationTransport{fakeTransport: fakeTransport{t: t}}
			client := &Client{transport: transport}

			events, err := client.CTLWatchSync(ctx)
			if err != nil {
				t.Fatalf("CTLWatchSync() error = %v", err)
			}
			transport.emit(Indication{Service: ServiceControl, MessageID: MessageCTLSync})
			select {
			case <-events:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for CTL sync indication")
			}
		})
	}
}
