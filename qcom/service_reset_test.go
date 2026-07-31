package qcom

import (
	"context"
	"testing"
	"time"
)

func TestServiceResetRequests(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		service ServiceType
		message MessageID
	}{
		{
			name:    "PDS",
			request: (PDSResetRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(),
			service: ServicePDS,
			message: MessagePDSReset,
		},
		{
			name:    "QOS",
			request: (QOSResetRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(),
			service: ServiceQOS,
			message: MessageQOSReset,
		},
		{
			name:    "WDS",
			request: (WDSResetRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(),
			service: ServiceWDS,
			message: MessageWDSReset,
		},
		{
			name:    "DMS",
			request: (DMSResetRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(),
			service: ServiceDMS,
			message: MessageDMSReset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request.Service != tt.service || tt.request.ClientID != 7 || tt.request.TransactionID != 9 || tt.request.MessageID != tt.message {
				t.Fatalf("Request() = %+v", tt.request)
			}
			if tt.request.Timeout != time.Second || len(tt.request.TLVs) != 0 {
				t.Fatalf("Request() timeout = %v, TLVs = %v", tt.request.Timeout, tt.request.TLVs)
			}
		})
	}
}

func TestServiceResetClientMethods(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
		message MessageID
		call    func(*Client) error
	}{
		{
			name: "PDS", service: ServicePDS, message: MessagePDSReset,
			call: func(c *Client) error { return c.PDSReset(context.Background()) },
		},
		{
			name: "QOS", service: ServiceQOS, message: MessageQOSReset,
			call: func(c *Client) error { return c.QOSReset(context.Background()) },
		},
		{
			name: "WDS", service: ServiceWDS, message: MessageWDSReset,
			call: func(c *Client) error { return c.WDSReset(context.Background()) },
		},
		{
			name: "DMS", service: ServiceDMS, message: MessageDMSReset,
			call: func(c *Client) error { return c.DMSReset(context.Background()) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != tt.service || req.ClientID != 7 || req.MessageID != tt.message || len(req.TLVs) != 0 {
						t.Fatalf("request = %+v", req)
					}
				},
				resp: successResponse(tt.message),
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{tt.service: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("reset error = %v", err)
			}
		})
	}
}
