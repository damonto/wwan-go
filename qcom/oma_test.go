package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestOMARequestEncoding(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "reset",
			request: func() (Request, error) {
				return (OMAResetRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageOMAReset,
		},
		{
			name: "set event report",
			request: func() (Request, error) {
				return (OMASetEventReportRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Config: OMAEventReportConfig{NetworkInitiatedAlerts: &enabled, SessionState: &disabled},
				}).Request(), nil
			},
			wantMessage: MessageOMASetEventReport,
			wantTLVs:    map[byte][]byte{0x10: {1}, 0x11: {0}},
		},
		{
			name: "start session",
			request: func() (Request, error) {
				return (OMAStartSessionRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					SessionType: OMASessionClientHandsFreeActivation,
				}).Request()
			},
			wantMessage: MessageOMAStartSession,
			wantTLVs:    map[byte][]byte{0x10: {byte(OMASessionClientHandsFreeActivation)}},
		},
		{
			name: "cancel session",
			request: func() (Request, error) {
				return (OMACancelSessionRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageOMACancelSession,
		},
		{
			name: "get session info",
			request: func() (Request, error) {
				return (OMAGetSessionInfoRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageOMAGetSessionInfo,
		},
		{
			name: "send selection",
			request: func() (Request, error) {
				return (OMASendSelectionRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Accept: true, SessionID: 0x1234,
				}).Request(), nil
			},
			wantMessage: MessageOMASendSelection,
			wantTLVs:    map[byte][]byte{0x10: {1, 0x34, 0x12}},
		},
		{
			name: "get feature settings",
			request: func() (Request, error) {
				return (OMAGetFeatureSettingsRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageOMAGetFeatureSetting,
		},
		{
			name: "set feature settings",
			request: func() (Request, error) {
				return (OMASetFeatureSettingsRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Update: OMAFeatureSettingsUpdate{
						DeviceProvisioning: &enabled,
						PRLUpdate:          &disabled,
						HFA:                &enabled,
					},
				}).Request(), nil
			},
			wantMessage: MessageOMASetFeatureSetting,
			wantTLVs:    map[byte][]byte{0x10: {1}, 0x11: {0}, 0x12: {1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceOMA || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
			}
			if got.Timeout != 3*time.Second {
				t.Fatalf("Timeout = %v, want 3s", got.Timeout)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				assertTLV(t, got.TLVs, kind, want)
			}
		})
	}
}

func TestOMARequestValidation(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "session type above range",
			call: func() error {
				_, err := (OMAStartSessionRequest{SessionType: OMASessionDevicePRLUpdate + 1}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestOMAEventUnmarshalTLVs(t *testing.T) {
	alert := []byte{byte(OMASessionNetworkPRLUpdate), 0x34, 0x12}
	want := OMAEvent{
		NetworkInitiatedAlert: OMANetworkInitiatedAlert{
			SessionType: OMASessionNetworkPRLUpdate,
			SessionID:   0x1234,
		},
		NetworkInitiatedAlertKnown: true,
		SessionState:               OMASessionFailed,
		SessionStateKnown:          true,
		FailureReason:              OMASessionFailureServerUnavailable,
		FailureReasonKnown:         true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    OMAEvent
		wantErr bool
	}{
		{
			name: "complete",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, alert),
				tlv.Uint(0x11, uint8(OMASessionFailed)),
				tlv.Uint(0x12, uint8(OMASessionFailureServerUnavailable)),
			},
			want: want,
		},
		{name: "empty"},
		{name: "alert truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, alert[:2])}, wantErr: true},
		{name: "state truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "reason trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OMAEvent
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
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

func TestOMASessionInfoUnmarshalTLVs(t *testing.T) {
	retry := []byte{3}
	retry = binary.LittleEndian.AppendUint16(retry, 120)
	retry = binary.LittleEndian.AppendUint16(retry, 45)
	alert := []byte{byte(OMASessionNetworkDeviceConfigure), 0x78, 0x56}
	want := OMASessionInfo{
		State:              OMASessionRetrying,
		Type:               OMASessionClientPRLUpdate,
		FailureReason:      OMASessionFailureNetworkUnavailable,
		FailureReasonKnown: true,
		Retry:              OMARetryInfo{Count: 3, PauseTimer: 120, PauseRemaining: 45},
		RetryKnown:         true,
		NetworkInitiatedAlert: OMANetworkInitiatedAlert{
			SessionType: OMASessionNetworkDeviceConfigure,
			SessionID:   0x5678,
		},
		NetworkInitiatedAlertKnown: true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    OMASessionInfo
		wantErr bool
	}{
		{
			name: "complete",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{byte(OMASessionRetrying), byte(OMASessionClientPRLUpdate)}),
				tlv.Uint(0x11, uint8(OMASessionFailureNetworkUnavailable)),
				tlv.Bytes(0x12, retry),
				tlv.Bytes(0x13, alert),
			},
			want: want,
		},
		{
			name: "required only",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{byte(OMASessionConnected), byte(OMASessionClientDeviceConfigure)})},
			want: OMASessionInfo{State: OMASessionConnected, Type: OMASessionClientDeviceConfigure},
		},
		{name: "session info missing", wantErr: true},
		{name: "session info truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, wantErr: true},
		{
			name:    "failure reason trailing data",
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1}), tlv.Bytes(0x11, []byte{1, 0})},
			wantErr: true,
		},
		{
			name:    "retry truncated",
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1}), tlv.Bytes(0x12, retry[:4])},
			wantErr: true,
		},
		{
			name:    "alert trailing data",
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, []byte{1, 1}), tlv.Bytes(0x13, append(alert, 0))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OMASessionInfo
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
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

func TestOMAFeatureSettingsUnmarshalTLVs(t *testing.T) {
	want := OMAFeatureSettings{
		DeviceProvisioning:      true,
		DeviceProvisioningKnown: true,
		PRLUpdateKnown:          true,
		HFA:                     true,
		HFAKnown:                true,
		HFADoneState:            OMAHFAFeatureSucceeded,
		HFADoneStateKnown:       true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    OMAFeatureSettings
		wantErr bool
	}{
		{
			name: "complete",
			tlvs: tlv.TLVs{
				tlv.Uint(0x10, uint8(1)),
				tlv.Uint(0x11, uint8(0)),
				tlv.Uint(0x12, uint8(1)),
				tlv.Uint(0x13, uint8(OMAHFAFeatureSucceeded)),
			},
			want: want,
		},
		{name: "empty"},
		{name: "provisioning truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "PRL trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0})}, wantErr: true},
		{name: "HFA truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, nil)}, wantErr: true},
		{name: "done state trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OMAFeatureSettings
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
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

func TestClientOMAOperations(t *testing.T) {
	enabled := true
	tests := []struct {
		name     string
		message  MessageID
		response Response
		call     func(*testing.T, *Client) error
	}{
		{name: "reset", message: MessageOMAReset, response: successResponse(MessageOMAReset), call: func(_ *testing.T, c *Client) error { return c.OMAReset(context.Background()) }},
		{
			name: "set event report", message: MessageOMASetEventReport, response: successResponse(MessageOMASetEventReport),
			call: func(_ *testing.T, c *Client) error {
				return c.OMASetEventReport(context.Background(), OMAEventReportConfig{SessionState: &enabled})
			},
		},
		{
			name: "start session", message: MessageOMAStartSession, response: successResponse(MessageOMAStartSession),
			call: func(_ *testing.T, c *Client) error {
				return c.OMAStartSession(context.Background(), OMASessionClientDeviceConfigure)
			},
		},
		{name: "cancel session", message: MessageOMACancelSession, response: successResponse(MessageOMACancelSession), call: func(_ *testing.T, c *Client) error { return c.OMACancelSession(context.Background()) }},
		{
			name:    "get session info",
			message: MessageOMAGetSessionInfo,
			response: successResponse(
				MessageOMAGetSessionInfo,
				tlv.Bytes(0x10, []byte{byte(OMASessionConnected), byte(OMASessionClientPRLUpdate)}),
			),
			call: func(t *testing.T, c *Client) error {
				got, err := c.OMASessionInfo(context.Background())
				if err == nil && (got.State != OMASessionConnected || got.Type != OMASessionClientPRLUpdate) {
					t.Fatalf("OMASessionInfo() = %+v", got)
				}
				return err
			},
		},
		{
			name: "send selection", message: MessageOMASendSelection, response: successResponse(MessageOMASendSelection),
			call: func(_ *testing.T, c *Client) error { return c.OMASendSelection(context.Background(), 12, true) },
		},
		{
			name:    "get feature settings",
			message: MessageOMAGetFeatureSetting,
			response: successResponse(
				MessageOMAGetFeatureSetting,
				tlv.Uint(0x10, uint8(1)),
			),
			call: func(t *testing.T, c *Client) error {
				got, err := c.OMAFeatureSettings(context.Background())
				if err == nil && (!got.DeviceProvisioningKnown || !got.DeviceProvisioning) {
					t.Fatalf("OMAFeatureSettings() = %+v", got)
				}
				return err
			},
		},
		{
			name: "set feature settings", message: MessageOMASetFeatureSetting, response: successResponse(MessageOMASetFeatureSetting),
			call: func(_ *testing.T, c *Client) error {
				return c.OMASetFeatureSettings(context.Background(), OMAFeatureSettingsUpdate{HFA: &enabled})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &serviceBoundFakeTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{
						check: func(req Request) {
							if req.Service != ServiceOMA || req.MessageID != tt.message {
								t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
							}
						},
						resp: tt.response,
					},
				}},
				service: ServiceOMA,
			}
			client, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestOMAWatchEvents(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forward event and release registration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &omaIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertOMAEventRegistration(t, req, 1) }, resp: successResponse(MessageOMASetEventReport)},
				{check: func(req Request) { assertOMAEventRegistration(t, req, 0) }, resp: successResponse(MessageOMASetEventReport)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceOMA: 7}}

			out, err := client.OMAWatchEvents(ctx)
			if err != nil {
				t.Fatalf("OMAWatchEvents() error = %v", err)
			}
			if transport.service != ServiceOMA || transport.clientID != 7 || transport.messageID != MessageOMAEventReport {
				t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}

			transport.emit(Indication{
				Service:   ServiceOMA,
				ClientID:  7,
				MessageID: MessageOMAEventReport,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x10, []byte{byte(OMASessionNetworkPRLUpdate), 0x34, 0x12}),
					tlv.Uint(0x11, uint8(OMASessionConnecting)),
				},
			})

			select {
			case got := <-out:
				want := OMAEvent{
					NetworkInitiatedAlert: OMANetworkInitiatedAlert{
						SessionType: OMASessionNetworkPRLUpdate,
						SessionID:   0x1234,
					},
					NetworkInitiatedAlertKnown: true,
					SessionState:               OMASessionConnecting,
					SessionStateKnown:          true,
				}
				if got != want {
					t.Fatalf("event = %+v, want %+v", got, want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for OMA event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func assertOMAEventRegistration(t *testing.T, req Request, want byte) {
	t.Helper()
	if req.Service != ServiceOMA || req.MessageID != MessageOMASetEventReport {
		t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
	}
	if len(req.TLVs) != 2 {
		t.Fatalf("TLVs len = %d, want 2", len(req.TLVs))
	}
	assertTLV(t, req.TLVs, 0x10, []byte{want})
	assertTLV(t, req.TLVs, 0x11, []byte{want})
}

type omaIndicationTransport struct {
	fakeTransport
	indications chan Indication
	service     ServiceType
	clientID    uint8
	messageID   MessageID
}

func (t *omaIndicationTransport) Indications(
	ctx context.Context,
	service ServiceType,
	clientID uint8,
	messageID MessageID,
) (<-chan Indication, error) {
	t.service = service
	t.clientID = clientID
	t.messageID = messageID
	t.indications = make(chan Indication, 4)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *omaIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *omaIndicationTransport) waitCalls(tb testing.TB, want int) {
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
