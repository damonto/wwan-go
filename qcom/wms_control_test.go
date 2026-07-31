package qcom

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSControlRequestEncoding(t *testing.T) {
	mode := WMSMessageModeGW
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		check   func(*testing.T, Request)
		resp    Response
	}{
		{
			name:    "reset",
			message: MessageWMSReset,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSReset(context.Background())
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSReset),
		},
		{
			name:    "modify tag",
			message: MessageWMSModifyTag,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSModifyTag(context.Background(), WMSModifyTagRequest{
					Reference:   WMSMessageReference{Storage: WMSStorageNV, Index: 0x11223344},
					Tag:         WMSTagMTRead,
					MessageMode: &mode,
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1, 0x44, 0x33, 0x22, 0x11, 0})
				assertTLV(t, req.TLVs, 0x10, []byte{1})
			},
			resp: successResponse(MessageWMSModifyTag),
		},
		{
			name:    "message protocol",
			message: MessageWMSGetMessageProtocol,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSCurrentMessageProtocol(context.Background())
				if err == nil && got != WMSMessageProtocolWCDMA {
					t.Fatalf("protocol = %d, want WCDMA", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSGetMessageProtocol, tlv.Bytes(0x01, []byte{1})),
		},
		{
			name:    "store capacity",
			message: MessageWMSGetStoreMaxSize,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSStoreCapacity(context.Background(), WMSStoreCapacityRequest{Storage: WMSStorageUIM, MessageMode: &mode})
				if err == nil && (got.Maximum != 100 || got.Free != 25 || !got.FreeKnown) {
					t.Fatalf("capacity = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0})
				assertTLV(t, req.TLVs, 0x10, []byte{1})
			},
			resp: successResponse(
				MessageWMSGetStoreMaxSize,
				tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, 100)),
				tlv.Bytes(0x10, binary.LittleEndian.AppendUint32(nil, 25)),
			),
		},
		{
			name:    "set memory available",
			message: MessageWMSSetMemoryStatus,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetMemoryAvailable(context.Background(), true)
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp:  successResponse(MessageWMSSetMemoryStatus),
		},
		{
			name:    "get memory available",
			message: MessageWMSGetMemoryStatus,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSMemoryAvailable(context.Background())
				if err == nil && !got {
					t.Fatal("memory available = false, want true")
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSGetMemoryStatus, tlv.Bytes(0x10, []byte{1})),
		},
		{
			name:    "get domain preference",
			message: MessageWMSGetDomainPreference,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSDomainPreference(context.Background())
				if err == nil && got != WMSDomainPreferencePSOnly {
					t.Fatalf("domain preference = %d, want PS only", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSGetDomainPreference, tlv.Bytes(0x01, []byte{3})),
		},
		{
			name:    "set domain preference",
			message: MessageWMSSetDomainPreference,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetDomainPreference(context.Background(), WMSDomainPreferencePS)
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp:  successResponse(MessageWMSSetDomainPreference),
		},
		{
			name:    "set primary client",
			message: MessageWMSSetPrimaryClient,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetPrimaryClient(context.Background(), true)
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp:  successResponse(MessageWMSSetPrimaryClient),
		},
		{
			name:    "get primary client",
			message: MessageWMSGetPrimaryClient,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSPrimaryClient(context.Background())
				if err == nil && !got {
					t.Fatal("primary client = false, want true")
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSGetPrimaryClient, tlv.Bytes(0x10, []byte{1})),
		},
		{
			name:    "bind subscription",
			message: MessageWMSBindSubscription,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSBindSubscription(context.Background(), WMSSubscriptionSecondary)
			},
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp:  successResponse(MessageWMSBindSubscription),
		},
		{
			name:    "get subscription binding",
			message: MessageWMSGetSubscription,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSBoundSubscription(context.Background())
				if err == nil && got != WMSSubscriptionTertiary {
					t.Fatalf("subscription = %d, want tertiary", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp:  successResponse(MessageWMSGetSubscription, tlv.Bytes(0x10, []byte{2})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWMS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X; want WMS/7/0x%04X", req.Service, req.ClientID, req.MessageID, tt.message)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestWMSControlRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		call    func(*Client) error
		resp    Response
	}{
		{
			name:    "protocol missing",
			message: MessageWMSGetMessageProtocol,
			call: func(c *Client) error {
				_, err := c.WMSCurrentMessageProtocol(context.Background())
				return err
			},
			resp: successResponse(MessageWMSGetMessageProtocol),
		},
		{
			name:    "capacity truncated",
			message: MessageWMSGetStoreMaxSize,
			call: func(c *Client) error {
				_, err := c.WMSStoreCapacity(context.Background(), WMSStoreCapacityRequest{})
				return err
			},
			resp: successResponse(MessageWMSGetStoreMaxSize, tlv.Bytes(0x01, []byte{1, 2})),
		},
		{
			name:    "optional byte truncated",
			message: MessageWMSGetMemoryStatus,
			call: func(c *Client) error {
				_, err := c.WMSMemoryAvailable(context.Background())
				return err
			},
			resp: successResponse(MessageWMSGetMemoryStatus, tlv.Bytes(0x10, []byte{1, 0})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{resp: tt.resp}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWMS: 7}}
			if err := tt.call(client); err == nil {
				t.Fatal("call() error = nil, want error")
			}
		})
	}
}

func assertNoRequestTLVs(t *testing.T, req Request) {
	t.Helper()
	if len(req.TLVs) != 0 {
		t.Fatalf("request TLVs = %v, want none", req.TLVs)
	}
}
