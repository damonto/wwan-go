package qcom

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWMSRoutesRequestEncoding(t *testing.T) {
	routes := []WMSRoute{{
		MessageType:  WMSMessageTypePointToPoint,
		MessageClass: WMSMessageClassNone,
		Storage:      WMSStorageNV,
		Action:       WMSReceiptStoreAndNotify,
	}}
	statusReport := WMSStatusReportToClient
	ims := true
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		check   func(*testing.T, Request)
		resp    Response
	}{
		{
			name:    "set routes",
			message: MessageWMSSetRoutes,
			call: func(_ *testing.T, c *Client) error {
				return c.WMSSetRoutes(context.Background(), routes, &statusReport)
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1, 0, 0, 4, 1, 1})
				assertTLV(t, req.TLVs, 0x10, []byte{1})
			},
			resp: successResponse(MessageWMSSetRoutes),
		},
		{
			name:    "get routes",
			message: MessageWMSGetRoutes,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSGetRoutes(context.Background())
				if err == nil && (len(got.Routes) != 1 || got.Routes[0] != routes[0] || !got.StatusReportKnown || got.StatusReportTransfer != statusReport) {
					t.Fatalf("routes = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) { assertNoRequestTLVs(t, req) },
			resp: successResponse(
				MessageWMSGetRoutes,
				tlv.Bytes(0x01, []byte{1, 0, 0, 4, 1, 1}),
				tlv.Bytes(0x10, []byte{1}),
			),
		},
		{
			name:    "send from store",
			message: MessageWMSSendFromMemoryStore,
			call: func(t *testing.T, c *Client) error {
				got, err := c.WMSSendFromStore(context.Background(), WMSSendFromStoreRequest{
					Reference:   WMSMessageReference{Storage: WMSStorageNV, Index: 0x11223344},
					MessageMode: WMSMessageModeGW,
					SMSOnIMS:    &ims,
				})
				if err == nil && (got.MessageID != 0x1234 || !got.CauseCodeKnown || got.CauseCode != 9 || !got.IMSRejectCauseKnown || got.IMSRejectCause != 10) {
					t.Fatalf("send result = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1, 0x44, 0x33, 0x22, 0x11, 1})
				assertTLV(t, req.TLVs, 0x10, []byte{1})
			},
			resp: successResponse(
				MessageWMSSendFromMemoryStore,
				tlv.Bytes(0x10, binary.LittleEndian.AppendUint16(nil, 0x1234)),
				tlv.Bytes(0x11, binary.LittleEndian.AppendUint16(nil, 9)),
				tlv.Bytes(0x18, binary.LittleEndian.AppendUint16(nil, 10)),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != tt.message {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.message)
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

func TestWMSRouteCodecRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "too many routes",
			fn: func() error {
				_, err := (WMSRoutes{Routes: make([]WMSRoute, wmsRouteMax+1)}).MarshalTLVs()
				return err
			},
		},
		{
			name: "count truncated",
			fn: func() error {
				_, err := decodeWMSRoutes([]byte{1})
				return err
			},
		},
		{
			name: "entry truncated",
			fn: func() error {
				_, err := decodeWMSRoutes([]byte{1, 0, 0, 1})
				return err
			},
		},
		{
			name: "count too large",
			fn: func() error {
				_, err := decodeWMSRoutes([]byte{11, 0})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("codec error = nil, want error")
			}
		})
	}
}
