package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

type fakeTransport struct {
	mu         sync.Mutex
	t          *testing.T
	calls      []transportCall
	idx        int
	closeCalls int
	closeErr   error
}

type transportCall struct {
	check func(Request)
	resp  Response
	err   error
}

func (t *fakeTransport) Do(_ context.Context, req Request) (Response, error) {
	t.t.Helper()
	t.mu.Lock()
	if t.idx >= len(t.calls) {
		t.mu.Unlock()
		t.t.Fatalf("Do() got unexpected request: %+v", req)
	}

	call := t.calls[t.idx]
	t.idx++
	t.mu.Unlock()

	if call.check != nil {
		call.check(req)
	}
	if call.err != nil {
		return Response{}, call.err
	}
	return call.resp, nil
}

func (t *fakeTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeCalls++
	return t.closeErr
}

func (t *fakeTransport) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.idx
}

type serviceBoundFakeTransport struct {
	fakeTransport
	service ServiceType
}

func (t *serviceBoundFakeTransport) QMIService() ServiceType {
	return t.service
}

type terminalFakeTransport struct {
	fakeTransport
	terminalErr error
	failOnDo    error
}

func (t *terminalFakeTransport) Do(_ context.Context, _ Request) (Response, error) {
	t.t.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.idx++
	if t.terminalErr == nil {
		t.terminalErr = t.failOnDo
	}
	return Response{}, t.terminalErr
}

func (t *terminalFakeTransport) TerminalError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.terminalErr
}

type lockCheckingTransport struct {
	t      *testing.T
	reader *Client
	calls  int
}

func (t *lockCheckingTransport) Do(_ context.Context, req Request) (Response, error) {
	t.t.Helper()
	if t.reader.mu.TryLock() {
		t.reader.mu.Unlock()
		t.t.Fatal("Client mutex is not held while sending QMI request")
	}
	t.calls++
	switch {
	case req.Service == ServiceControl && req.MessageID == MessageAllocateClientID:
		return successResponse(req.MessageID, tlv.Bytes(0x01, []byte{byte(ServiceDMS), 5})), nil
	case req.Service == ServiceDMS && req.MessageID == MessageDMSGetMSISDN:
		return successResponse(req.MessageID, tlv.Bytes(dmsTLVVoiceNumber, []byte("+8613800138000"))), nil
	case req.Service == ServiceControl && req.MessageID == MessageReleaseClientID:
		return successResponse(req.MessageID), nil
	default:
		t.t.Fatalf("unexpected QMI request: %+v", req)
		return Response{}, nil
	}
}

func (t *lockCheckingTransport) Close() error { return nil }

type closeCancelTransport struct {
	mu             sync.Mutex
	blockedMessage MessageID
	requestStarted chan struct{}
	startOnce      sync.Once
	requests       []Request
	closeCalls     int
}

type allocationCloseTransport struct {
	mu                 sync.Mutex
	service            ServiceType
	clientID           uint8
	requestStarted     chan struct{}
	continueAllocation chan struct{}
	startOnce          sync.Once
	requests           []Request
	closeCalls         int
}

type serializedRequestTransport struct {
	requestStarted chan struct{}
	releaseRequest chan struct{}
	startOnce      sync.Once
}

func (t *serializedRequestTransport) Do(ctx context.Context, req Request) (Response, error) {
	t.startOnce.Do(func() {
		close(t.requestStarted)
	})
	select {
	case <-t.releaseRequest:
		return successResponse(req.MessageID, tlv.Bytes(0x10, encodeCardStatus(true))), nil
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
}

func (*serializedRequestTransport) Close() error { return nil }

func (t *allocationCloseTransport) Do(ctx context.Context, req Request) (Response, error) {
	t.mu.Lock()
	t.requests = append(t.requests, req)
	t.mu.Unlock()

	switch req.MessageID {
	case MessageAllocateClientID:
		t.startOnce.Do(func() {
			close(t.requestStarted)
		})
		select {
		case <-t.continueAllocation:
			return successResponse(req.MessageID, tlv.Bytes(0x01, []byte{byte(t.service), t.clientID})), nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	case MessageReleaseClientID:
		return successResponse(req.MessageID), nil
	default:
		return Response{}, errors.New("unexpected QMI request during allocation close test")
	}
}

func (t *allocationCloseTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeCalls++
	return nil
}

func (t *allocationCloseTransport) snapshot() ([]Request, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.requests), t.closeCalls
}

func (t *closeCancelTransport) Do(ctx context.Context, req Request) (Response, error) {
	t.mu.Lock()
	t.requests = append(t.requests, req)
	t.mu.Unlock()

	if req.MessageID == MessageReleaseClientID {
		return successResponse(req.MessageID), nil
	}
	if req.MessageID != t.blockedMessage {
		return Response{}, errors.New("unexpected QMI request during close cancellation test")
	}

	t.startOnce.Do(func() {
		close(t.requestStarted)
	})
	<-ctx.Done()
	return Response{}, ctx.Err()
}

func (t *closeCancelTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeCalls++
	return nil
}

func (t *closeCancelTransport) snapshot() ([]Request, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.requests), t.closeCalls
}

type cancelingClientIDTransport struct {
	mu             sync.Mutex
	requestStarted chan struct{}
	startOnce      sync.Once
	closeCalls     int
}

func (t *cancelingClientIDTransport) ClientID(ctx context.Context, _ ServiceType) (uint8, error) {
	t.startOnce.Do(func() {
		close(t.requestStarted)
	})
	<-ctx.Done()
	return 0, ctx.Err()
}

func (t *cancelingClientIDTransport) Do(context.Context, Request) (Response, error) {
	return Response{}, errors.New("unexpected QMI request while opening transport service")
}

func (t *cancelingClientIDTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeCalls++
	return nil
}

func (t *cancelingClientIDTransport) closeCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeCalls
}

func TestNewClientDoesNotAllocateServiceClients(t *testing.T) {
	tests := []struct {
		name         string
		newTransport func(*testing.T) (Transport, func() int)
	}{
		{
			name: "unbound transport",
			newTransport: func(t *testing.T) (Transport, func() int) {
				transport := &fakeTransport{t: t}
				return transport, transport.callCount
			},
		},
		{
			name: "service-bound transport",
			newTransport: func(t *testing.T) (Transport, func() int) {
				transport := &serviceBoundFakeTransport{fakeTransport: fakeTransport{t: t}, service: ServiceUIM}
				return transport, transport.callCount
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, callCount := tt.newTransport(t)
			client, err := NewClient(transport, WithSlot(1))
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if len(client.clientIDs) != 0 {
				t.Fatalf("clientIDs = %v, want empty", client.clientIDs)
			}
			if got := callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := callCount(); got != 0 {
				t.Fatalf("Do() calls after Close = %d, want 0", got)
			}
		})
	}
}

func TestNewClientAcceptsServiceBoundTransport(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
	}{
		{name: "UIM", service: ServiceUIM},
		{name: "DMS", service: ServiceDMS},
		{name: "WDA", service: ServiceWDA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &serviceBoundFakeTransport{
				fakeTransport: fakeTransport{t: t},
				service:       tt.service,
			}

			client, err := NewClient(transport, WithSlot(1))
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if client == nil {
				t.Fatal("NewClient() client = nil")
			}
		})
	}
}

func TestClientRejectsUIMRequestOnOtherBoundService(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
	}{
		{name: "DMS", service: ServiceDMS},
		{name: "WDA", service: ServiceWDA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &serviceBoundFakeTransport{fakeTransport: fakeTransport{t: t}, service: tt.service}
			client, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			_, err = client.request(context.Background(), MessageGetATR, nil)
			if err == nil || !strings.Contains(err.Error(), "bound to service") {
				t.Fatalf("request() error = %v, want bound-service mismatch", err)
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
		})
	}
}

func TestClientLazilyAllocatesUIMClient(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "reuse UIM client until close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceUIM, 5)},
				{
					check: func(req Request) {
						if req.Service != ServiceUIM || req.ClientID != 5 {
							t.Fatalf("first UIM request = service 0x%02X client %d", req.Service, req.ClientID)
						}
					},
					resp: successResponse(MessageGetATR),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceUIM || req.ClientID != 5 {
							t.Fatalf("second UIM request = service 0x%02X client %d", req.Service, req.ClientID)
						}
					},
					resp: successResponse(MessageGetATR),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceUIM), 5})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}
			client, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			for range 2 {
				if _, err := client.request(context.Background(), MessageGetATR, nil); err != nil {
					t.Fatalf("request() error = %v", err)
				}
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestWithServiceClientSerializesEverySend(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "DMS allocate request close release"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &lockCheckingTransport{t: t}
			reader := &Client{transport: transport, slot: 1}
			transport.reader = reader

			if _, err := reader.MSISDN(context.Background()); err != nil {
				t.Fatalf("MSISDN() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if transport.calls != 3 {
				t.Fatalf("Do() calls = %d, want 3", transport.calls)
			}
		})
	}
}

func TestRequestContextStopsWaitingForClientLock(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{
			name: "UIM request",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.CardStatus(ctx)
				return err
			},
		},
		{
			name: "service request",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.MSISDN(ctx)
				return err
			},
		},
		{
			name: "indication transport",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.indicationTransportContext(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &serializedRequestTransport{
				requestStarted: make(chan struct{}),
				releaseRequest: make(chan struct{}),
			}
			client := &Client{
				transport: transport,
				slot:      1,
				clientIDs: map[ServiceType]uint8{ServiceUIM: 7, ServiceDMS: 8},
			}
			firstDone := make(chan error, 1)
			go func() {
				_, err := client.CardStatus(t.Context())
				firstDone <- err
			}()

			select {
			case <-transport.requestStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for first request")
			}

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if err := tt.call(ctx, client); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("call() error = %v, want context deadline exceeded", err)
			}

			close(transport.releaseRequest)
			select {
			case err := <-firstDone:
				if err != nil {
					t.Fatalf("first CardStatus() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for first request to finish")
			}
		})
	}
}

func TestClientReusesServiceClientUntilClose(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "DMS client"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceDMS), 5}))},
				{resp: successResponse(MessageDMSGetMSISDN, tlv.Bytes(dmsTLVVoiceNumber, []byte("first")))},
				{resp: successResponse(MessageDMSGetMSISDN, tlv.Bytes(dmsTLVVoiceNumber, []byte("second")))},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceDMS), 5})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}
			reader := &Client{transport: transport, slot: 1}

			for _, want := range []string{"first", "second"} {
				got, err := reader.MSISDN(context.Background())
				if err != nil {
					t.Fatalf("MSISDN() error = %v", err)
				}
				if got.VoiceNumber != want {
					t.Fatalf("MSISDN().VoiceNumber = %q, want %q", got.VoiceNumber, want)
				}
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != 4 {
				t.Fatalf("Do() calls = %d, want 4", got)
			}
		})
	}
}

func TestAllocateServiceClientIDRejectsMalformedTLV(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "truncated", value: []byte{byte(ServiceDMS)}},
		{name: "trailing byte", value: []byte{byte(ServiceDMS), 5, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, tt.value)),
			}}}
			client := &Client{transport: transport}
			if _, err := client.allocateServiceClientIDLocked(context.Background(), ServiceDMS); err == nil {
				t.Fatal("allocateServiceClientIDLocked() error = nil, want non-nil")
			}
		})
	}
}

func TestAllocateServiceClientIDValidatesOwnership(t *testing.T) {
	tests := []struct {
		name        string
		value       []byte
		wantRelease bool
	}{
		{
			name:        "service mismatch releases returned client",
			value:       []byte{byte(ServiceNAS), 5},
			wantRelease: true,
		},
		{
			name:  "reserved client ID",
			value: []byte{byte(ServiceDMS), 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []transportCall{{
				resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, tt.value)),
			}}
			if tt.wantRelease {
				calls = append(calls, transportCall{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageReleaseClientID {
							t.Fatalf("cleanup request = %+v, want CID release", req)
						}
						assertTLV(t, req.TLVs, 0x01, tt.value)
					},
					resp: successResponse(MessageReleaseClientID),
				})
			}
			transport := &fakeTransport{t: t, calls: calls}
			client := &Client{transport: transport}

			if _, err := client.allocateServiceClientIDLocked(t.Context(), ServiceDMS); err == nil {
				t.Fatal("allocateServiceClientIDLocked() error = nil, want ownership validation error")
			}
			if got := transport.callCount(); got != len(calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(calls))
			}
			if len(client.allocatedClientIDs) != 0 {
				t.Fatalf("allocated client IDs = %v, want empty", client.allocatedClientIDs)
			}
		})
	}
}

func TestQMUXClientIDRejectsExtendedService(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{
			name: "allocate",
			run: func(ctx context.Context, client *Client) error {
				_, err := client.allocateServiceClientID(ctx, ServiceSSC)
				return err
			},
		},
		{
			name: "release",
			run: func(ctx context.Context, client *Client) error {
				return client.releaseServiceClientID(ctx, ServiceIMSDCM, 7)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			client := &Client{transport: transport}

			err := tt.run(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), "QMUX 8-bit limit") {
				t.Fatalf("client ID operation error = %v, want QMUX service limit", err)
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
		})
	}
}

func TestReleaseServiceClientIDAcceptsAlreadyInvalid(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "modem already discarded client ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: errorResponse(MessageReleaseClientID, QMIErrorInvalidClientId),
			}}}
			allocated := allocatedClientID{service: ServiceDMS, clientID: 5}
			client := &Client{
				transport:          transport,
				clientIDs:          map[ServiceType]uint8{ServiceDMS: 5},
				allocatedClientIDs: map[allocatedClientID]struct{}{allocated: {}},
			}

			if err := client.releaseServiceClientID(context.Background(), ServiceDMS, 5); err != nil {
				t.Fatalf("releaseServiceClientID() error = %v", err)
			}
			if len(client.allocatedClientIDs) != 0 {
				t.Fatalf("allocated client IDs = %v, want empty", client.allocatedClientIDs)
			}
		})
	}
}

func TestClientReallocatesInvalidServiceClient(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "DMS client invalidated by modem"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceDMS), 5}))},
				{resp: errorResponse(MessageDMSGetMSISDN, QMIErrorInvalidClientId)},
				{resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceDMS), 6}))},
				{resp: successResponse(MessageDMSGetMSISDN, tlv.Bytes(dmsTLVVoiceNumber, []byte("recovered")))},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceDMS), 6})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}
			reader := &Client{transport: transport, slot: 1}

			got, err := reader.MSISDN(context.Background())
			if err != nil {
				t.Fatalf("MSISDN() error = %v", err)
			}
			if got.VoiceNumber != "recovered" {
				t.Fatalf("MSISDN().VoiceNumber = %q, want recovered", got.VoiceNumber)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestClientNextTransactionIDSkipsZero(t *testing.T) {
	tests := []struct {
		name    string
		ctlTxn  uint8
		txn     uint16
		service ServiceType
		want    []uint16
	}{
		{
			name:    "control wraps after 255",
			ctlTxn:  0xFE,
			service: ServiceControl,
			want:    []uint16{0xFF, 0x01},
		},
		{
			name:    "service wraps after 65535",
			txn:     0xFFFE,
			service: ServiceUIM,
			want:    []uint16{0xFFFF, 0x0001},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := Client{ctlTxn: tt.ctlTxn, txn: tt.txn}
			for i, want := range tt.want {
				if got := reader.nextTransactionID(tt.service); got != want {
					t.Fatalf("nextTransactionID() call %d = %#04x, want %#04x", i+1, got, want)
				}
			}
		})
	}
}

func TestSendEnvelopeRejectsServiceBoundTransport(t *testing.T) {
	transport := &serviceBoundFakeTransport{
		fakeTransport: fakeTransport{t: t},
		service:       ServiceUIM,
	}
	reader := &Client{
		transport: transport,
		slot:      1,
	}

	_, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
	if err == nil || !strings.Contains(err.Error(), "cannot switch to CAT/CAT2") {
		t.Fatalf("SendEnvelope() error = %v, want service-bound CAT error", err)
	}
	if got := transport.callCount(); got != 0 {
		t.Fatalf("Do() calls = %d, want 0", got)
	}
}

func TestSendEnvelopeRejectsLongEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		envelope []byte
		wantErr  string
	}{
		{
			name:    "empty",
			wantErr: "envelope length 0 is too short",
		},
		{
			name:     "one byte",
			envelope: []byte{0xD1},
			wantErr:  "envelope length 1 is too short",
		},
		{
			name:     "above raw envelope limit",
			envelope: bytes.Repeat([]byte{0xD1}, catRawEnvelopeMaxLength+1),
			wantErr:  "exceeds QMI CAT raw envelope limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			reader := &Client{
				transport: transport,
				slot:      1,
			}

			_, err := reader.SendEnvelope(context.Background(), tt.envelope)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SendEnvelope() error = %v, want text %q", err, tt.wantErr)
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
		})
	}
}

func TestSendEnvelopeRequiresRawResponseTLV(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr string
	}{
		{
			name:    "missing response",
			wantErr: "raw response TLV missing",
		},
		{
			name:    "truncated response",
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, []byte{0x90, 0x00})},
			wantErr: "raw response TLV is truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Client{
				transport: &fakeTransport{
					t: t,
					calls: []transportCall{{
						resp: successResponse(MessageSendEnvelope, tt.tlvs...),
					}},
				},
				slot:       1,
				catService: ServiceCAT,
				clientIDs:  map[ServiceType]uint8{ServiceCAT: 10},
			}

			_, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SendEnvelope() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientUIMMessages(t *testing.T) {
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessageGetFileAttributes {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageGetFileAttributes)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0xE2, 0x2F, 0x02, 0x00, 0x3F})
					},
					resp: successResponse(MessageGetFileAttributes,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeFileAttributes(10, 0x2FE2, 0, 0, 0, []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x0A})),
					),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageReadTransparent {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageReadTransparent)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0x07, 0x6F, 0x04, 0x00, 0x3F, 0xFF, 0x7F})
						assertTLV(t, req.TLVs, 0x03, []byte{0x00, 0x00, 0x09, 0x00})
					},
					resp: successResponse(MessageReadTransparent,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98})),
					),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, append([]byte{byte(SessionNonProvisioningSlot1), byte(len(isimAID))}, isimAID...))
						assertTLV(t, req.TLVs, 0x02, []byte{0x04, 0x6F, 0x00})
						assertTLV(t, req.TLVs, 0x03, []byte{0x01, 0x00, 0x20, 0x00})
					},
					resp: successResponse(MessageReadRecord,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed(tlvTextRecord("sip:alice@ims.example.com", 32))),
					),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageAuthenticate {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageAuthenticate)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{
							byte(AuthContext3G),
							0x22, 0x00,
							0x10, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
							0x10, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
						})
					},
					resp: successResponse(MessageAuthenticate,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0xDC, 0x00})),
					),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	attrs, err := reader.FileAttributes(context.Background(), File{
		Session: SessionPrimaryGWProvisioning,
		Path:    []byte{0x3F, 0x00, 0x2F, 0xE2},
	})
	if err != nil {
		t.Fatalf("FileAttributes() error = %v", err)
	}
	if attrs.FileStructure != FileStructureTransparent || attrs.FileSize != 10 {
		t.Fatalf("FileAttributes() = %+v", attrs)
	}

	imsiRaw, err := reader.ReadTransparent(context.Background(), TransparentRead{
		File:   File{Session: SessionPrimaryGWProvisioning, Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x07}},
		Length: 9,
	})
	if err != nil {
		t.Fatalf("ReadTransparent() error = %v", err)
	}
	if !bytes.Equal(imsiRaw, []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98}) {
		t.Fatalf("ReadTransparent() = %X", imsiRaw)
	}

	impuRaw, err := reader.ReadRecord(context.Background(), RecordRead{
		File:   File{Session: SessionNonProvisioningSlot1, AID: isimAID, Path: []byte{0x6F, 0x04}},
		Record: 1,
		Length: 32,
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}
	if !bytes.Equal(impuRaw, tlvTextRecord("sip:alice@ims.example.com", 32)) {
		t.Fatalf("ReadRecord() = %X", impuRaw)
	}

	auth, err := reader.Authenticate(context.Background(), AuthenticateRequest{
		Session: SessionPrimaryGWProvisioning,
		Context: AuthContext3G,
		Rand:    bytes.Repeat([]byte{0x01}, 16),
		AUTN:    bytes.Repeat([]byte{0x02}, 16),
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !bytes.Equal(auth, []byte{0xDC, 0x00}) {
		t.Fatalf("Authenticate() = %X, want DC00", auth)
	}
}

func TestClientAuthenticateUsesISIMContext(t *testing.T) {
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{{
				check: func(req Request) {
					if req.MessageID != MessageAuthenticate {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, MessageAuthenticate)
					}
					assertTLV(t, req.TLVs, 0x01, append([]byte{byte(SessionCardSlot1), byte(len(isimAID))}, isimAID...))
					assertTLV(t, req.TLVs, 0x02, []byte{
						byte(AuthContextIMSAKA),
						0x22, 0x00,
						0x10, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
						0x10, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
					})
				},
				resp: successResponse(MessageAuthenticate,
					tlv.Bytes(0x10, []byte{0x90, 0x00}),
					tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0xDC, 0x00})),
				),
			}},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	auth, err := reader.Authenticate(context.Background(), AuthenticateRequest{
		Session: SessionCardSlot1,
		AID:     isimAID,
		Context: AuthContextIMSAKA,
		Rand:    bytes.Repeat([]byte{0x01}, 16),
		AUTN:    bytes.Repeat([]byte{0x02}, 16),
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !bytes.Equal(auth, []byte{0xDC, 0x00}) {
		t.Fatalf("Authenticate() = %X, want DC00", auth)
	}
}

func TestReadTransparentRejectsLongResponse(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					resp: errorResponse(
						MessageReadTransparent,
						QMIErrorInsufficientResources,
						tlv.Uint(0x15, uint32(1)),
					),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	_, err := reader.ReadTransparent(context.Background(), TransparentRead{
		File:   File{Session: SessionPrimaryGWProvisioning, Path: []byte{0x3F, 0x00, 0x6F, 0x07}},
		Length: 9,
	})
	if err == nil || !strings.Contains(err.Error(), "long response is not supported") {
		t.Fatalf("ReadTransparent() error = %v, want long response error", err)
	}
}

func TestReadRecordRejectsResponseIndication(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					resp: successResponse(MessageReadRecord, tlv.Uint(0x13, uint32(7))),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	_, err := reader.ReadRecord(context.Background(), RecordRead{
		File:   File{Session: SessionPrimaryGWProvisioning, Path: []byte{0x3F, 0x00, 0x6F, 0x04}},
		Record: 1,
		Length: 32,
	})
	if err == nil || !strings.Contains(err.Error(), "response indication is not supported") {
		t.Fatalf("ReadRecord() error = %v, want indication error", err)
	}
}

func TestClientSMSPPDownloadUsesCATEnvelope(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: ServiceCAT2, Major: 2, Minor: 24},
					))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT2 client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceCAT2)})
					},
					resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceCAT2), 9})),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 9 || req.MessageID != MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT2 envelope", req.Service, req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{
							0x09, 0x00,
							0x10, 0x00,
							0xD1, 0x0E,
							0x82, 0x02, 0x83, 0x81,
							0x86, 0x03, 0x91, 0x21, 0x43,
							0x8B, 0x03, 0x00, 0x7F, 0xF6,
						})
						assertTLV(t, req.TLVs, 0x10, []byte{0x01})
					},
					resp: successResponse(MessageSendEnvelope, tlv.Bytes(0x10, []byte{0x90, 0x00, 0x00})),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	got, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
	if err != nil {
		t.Fatalf("SendEnvelope() error = %v", err)
	}
	if got.SW1 != 0x90 || got.SW2 != 0x00 {
		t.Fatalf("SendEnvelope() status = %02X%02X, want 9000", got.SW1, got.SW2)
	}
}

func TestClientSMSPPDownloadUsesCATWhenOnlyCATIsExposed(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: ServiceCAT, Major: 1, Minor: 0},
					))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceCAT)})
					},
					resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceCAT), 10})),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT || req.ClientID != 10 || req.MessageID != MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT envelope", req.Service, req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{
							0x09, 0x00,
							0x10, 0x00,
							0xD1, 0x0E,
							0x82, 0x02, 0x83, 0x81,
							0x86, 0x03, 0x91, 0x21, 0x43,
							0x8B, 0x03, 0x00, 0x7F, 0xF6,
						})
						assertTLV(t, req.TLVs, 0x10, []byte{0x01})
					},
					resp: successResponse(MessageSendEnvelope, tlv.Bytes(0x10, []byte{0x90, 0x00, 0x00})),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	got, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
	if err != nil {
		t.Fatalf("SendEnvelope() error = %v", err)
	}
	if got.SW1 != 0x90 || got.SW2 != 0x00 {
		t.Fatalf("SendEnvelope() status = %02X%02X, want 9000", got.SW1, got.SW2)
	}
	if catClientID := reader.clientIDs[reader.catService]; reader.catService != ServiceCAT || catClientID != 10 {
		t.Fatalf("CAT client = service %#x client %d, want CAT client 10", reader.catService, catClientID)
	}
}

func TestClientSMSPPDownloadDoesNotFallbackAfterCAT2EnvelopeError(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: ServiceCAT2, Major: 2, Minor: 24},
						serviceVersion{Service: ServiceCAT, Major: 1, Minor: 0},
					))),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceControl || req.MessageID != MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT2 client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceCAT2)})
					},
					resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceCAT2), 9})),
				},
				{
					check: func(req Request) {
						if req.Service != ServiceCAT2 || req.ClientID != 9 || req.MessageID != MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT2 envelope", req.Service, req.ClientID, req.MessageID)
						}
					},
					resp: errorResponse(MessageSendEnvelope, QMIErrorInvalidOperation),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	if _, err := reader.SendEnvelope(context.Background(), smsPPEnvelope()); err == nil || !strings.Contains(err.Error(), "Invalid operation") {
		t.Fatalf("SendEnvelope() error = %v, want Invalid operation", err)
	}
}

func TestEnsureSlotActivated(t *testing.T) {
	tests := []struct {
		name    string
		slot    uint8
		ctx     func() context.Context
		calls   []transportCall
		wantErr string
	}{
		{
			name: "already active",
			slot: 2,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: successResponse(MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(2)))},
			},
		},
		{
			name: "switch then ready",
			slot: 2,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: successResponse(MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(1)))},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{0x01})
						assertTLV(t, req.TLVs, 0x02, []byte{0x02, 0x00, 0x00, 0x00})
					},
					resp: successResponse(MessageSwitchSlot),
				},
				{resp: successResponse(MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(false)))},
				{resp: successResponse(MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(true)))},
			},
		},
		{
			name: "unsupported get slot status",
			slot: 1,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: errorResponse(MessageGetSlotStatus, QMIErrorNotSupported)},
			},
		},
		{
			name: "timeout waiting for app readiness",
			slot: 2,
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			calls: []transportCall{
				{resp: successResponse(MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(1)))},
				{resp: successResponse(MessageSwitchSlot)},
				{resp: successResponse(MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(false)))},
			},
			wantErr: "waiting for card readiness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Client{
				transport: &fakeTransport{t: t, calls: tt.calls},
				slot:      tt.slot,
				clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
			}

			err := reader.ActivateSlot(tt.ctx())
			switch {
			case tt.wantErr == "":
				if err != nil {
					t.Fatalf("ActivateSlot() error = %v", err)
				}
			case err == nil || !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("ActivateSlot() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req Request) {
					if req.Service != ServiceControl {
						t.Fatalf("Service = %v, want %v", req.Service, ServiceControl)
					}
					if req.ClientID != 0 {
						t.Fatalf("ClientID = %d, want 0", req.ClientID)
					}
					if req.MessageID != MessageReleaseClientID {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageReleaseClientID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceUIM), 0x07})
				},
				resp: Response{
					Service:   ServiceControl,
					MessageID: MessageReleaseClientID,
					TLVs: tlv.TLVs{
						tlv.Bytes(qmiTLVResult, []byte{0x00, 0x00, 0x00, 0x00}),
					},
				},
			},
		},
	}
	reader := &Client{
		transport: transport,
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if transport.idx != 1 {
		t.Fatalf("Do() calls = %d, want 1", transport.idx)
	}
	if transport.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", transport.closeCalls)
	}
	if len(reader.clientIDs) != 0 {
		t.Fatalf("clientIDs = %v, want empty", reader.clientIDs)
	}
	if reader.transport != nil {
		t.Fatal("Transport was not cleared")
	}
}

func TestClientCloseAcceptsAlreadyInvalidClientID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "modem discarded client before close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: errorResponse(MessageReleaseClientID, QMIErrorInvalidClientId),
			}}}
			client := &Client{
				transport: transport,
				clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
			}

			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if transport.closeCalls != 1 {
				t.Fatalf("transport Close() calls = %d, want 1", transport.closeCalls)
			}
		})
	}
}

func TestClientCloseCancelsInFlightRequests(t *testing.T) {
	tests := []struct {
		name           string
		service        ServiceType
		blockedMessage MessageID
		call           func(context.Context, *Client) error
	}{
		{
			name:           "service request",
			service:        ServiceDMS,
			blockedMessage: MessageDMSGetMSISDN,
			call: func(ctx context.Context, client *Client) error {
				_, err := client.MSISDN(ctx)
				return err
			},
		},
		{
			name:           "incremental NAS request",
			service:        ServiceNAS,
			blockedMessage: MessageNASIncrementalNetworkScan,
			call: func(ctx context.Context, client *Client) error {
				_, _, err := client.startNASIncrementalNetworkScan(ctx, 7, nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &closeCancelTransport{
				blockedMessage: tt.blockedMessage,
				requestStarted: make(chan struct{}),
			}
			client := &Client{
				transport: transport,
				slot:      1,
				clientIDs: map[ServiceType]uint8{tt.service: 7},
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			requestErr := make(chan error, 1)
			go func() {
				requestErr <- tt.call(ctx, client)
			}()

			select {
			case <-transport.requestStarted:
			case <-ctx.Done():
				t.Fatalf("waiting for request start: %v", ctx.Err())
			}

			closeErr := make(chan error, 1)
			go func() {
				closeErr <- client.Close()
			}()

			select {
			case err := <-requestErr:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request error = %v, want context canceled", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for canceled request: %v", ctx.Err())
			}
			select {
			case err := <-closeErr:
				if err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for Close(): %v", ctx.Err())
			}

			if err := client.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
			requests, closeCalls := transport.snapshot()
			if len(requests) != 2 {
				t.Fatalf("Do() requests = %+v, want blocked request and CID release", requests)
			}
			if requests[0].MessageID != tt.blockedMessage {
				t.Fatalf("first MessageID = 0x%04X, want 0x%04X", requests[0].MessageID, tt.blockedMessage)
			}
			if requests[1].Service != ServiceControl || requests[1].MessageID != MessageReleaseClientID {
				t.Fatalf("second request = %+v, want CID release", requests[1])
			}
			assertTLV(t, requests[1].TLVs, 0x01, []byte{byte(tt.service), 7})
			if closeCalls != 1 {
				t.Fatalf("transport Close() calls = %d, want 1", closeCalls)
			}
		})
	}
}

func TestClientCloseReleasesInFlightClientIDAllocation(t *testing.T) {
	tests := []struct {
		name     string
		service  ServiceType
		clientID uint8
	}{
		{name: "DMS allocation", service: ServiceDMS, clientID: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &allocationCloseTransport{
				service:            tt.service,
				clientID:           tt.clientID,
				requestStarted:     make(chan struct{}),
				continueAllocation: make(chan struct{}),
			}
			client := &Client{transport: transport, slot: 1}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			requestErr := make(chan error, 1)
			go func() {
				_, err := client.serviceClientID(ctx, tt.service)
				requestErr <- err
			}()

			select {
			case <-transport.requestStarted:
			case <-ctx.Done():
				t.Fatalf("waiting for allocation start: %v", ctx.Err())
			}

			client.startClose()
			closeErr := make(chan error, 1)
			go func() {
				closeErr <- client.Close()
			}()
			close(transport.continueAllocation)

			select {
			case err := <-requestErr:
				if !errors.Is(err, errClientClosed) {
					t.Fatalf("allocation error = %v, want client closed", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for allocation completion: %v", ctx.Err())
			}
			select {
			case err := <-closeErr:
				if err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for Close(): %v", ctx.Err())
			}

			requests, closeCalls := transport.snapshot()
			if len(requests) != 2 {
				t.Fatalf("Do() requests = %+v, want allocation and release", requests)
			}
			if requests[0].MessageID != MessageAllocateClientID {
				t.Fatalf("first request = %+v, want client ID allocation", requests[0])
			}
			if requests[1].MessageID != MessageReleaseClientID {
				t.Fatalf("second request = %+v, want client ID release", requests[1])
			}
			assertTLV(t, requests[1].TLVs, 0x01, []byte{byte(tt.service), tt.clientID})
			if closeCalls != 1 {
				t.Fatalf("transport Close() calls = %d, want 1", closeCalls)
			}
			if len(client.allocatedClientIDs) != 0 {
				t.Fatalf("allocated client IDs = %v, want empty", client.allocatedClientIDs)
			}
		})
	}
}

func TestClientIDAllocationSurvivesCallerCancellation(t *testing.T) {
	tests := []struct {
		name     string
		service  ServiceType
		clientID uint8
	}{
		{name: "DMS allocation", service: ServiceDMS, clientID: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &allocationCloseTransport{
				service:            tt.service,
				clientID:           tt.clientID,
				requestStarted:     make(chan struct{}),
				continueAllocation: make(chan struct{}),
			}
			client := &Client{transport: transport, slot: 1}
			ctx, cancel := context.WithCancel(t.Context())

			requestErr := make(chan error, 1)
			go func() {
				_, err := client.serviceClientID(ctx, tt.service)
				requestErr <- err
			}()

			select {
			case <-transport.requestStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for allocation start")
			}
			cancel()
			close(transport.continueAllocation)

			select {
			case err := <-requestErr:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("allocation error = %v, want context canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("allocation did not finish after response")
			}
			if got := client.clientIDs[tt.service]; got != tt.clientID {
				t.Fatalf("cached client ID = %d, want %d", got, tt.clientID)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			requests, _ := transport.snapshot()
			if len(requests) != 2 || requests[1].MessageID != MessageReleaseClientID {
				t.Fatalf("Do() requests = %+v, want allocation followed by release", requests)
			}
			assertTLV(t, requests[1].TLVs, 0x01, []byte{byte(tt.service), tt.clientID})
		})
	}
}

func TestClientCloseCancelsTransportClientID(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceType
	}{
		{name: "QRTR service open", service: ServiceDMS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &cancelingClientIDTransport{requestStarted: make(chan struct{})}
			client := &Client{transport: transport, slot: 1}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			requestErr := make(chan error, 1)
			go func() {
				_, err := client.serviceClientID(ctx, tt.service)
				requestErr <- err
			}()

			select {
			case <-transport.requestStarted:
			case <-ctx.Done():
				t.Fatalf("waiting for service open: %v", ctx.Err())
			}

			closeErr := make(chan error, 1)
			go func() {
				closeErr <- client.Close()
			}()

			select {
			case err := <-requestErr:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("service open error = %v, want context canceled", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for canceled service open: %v", ctx.Err())
			}
			select {
			case err := <-closeErr:
				if err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for Close(): %v", ctx.Err())
			}
			if got := transport.closeCount(); got != 1 {
				t.Fatalf("transport Close() calls = %d, want 1", got)
			}
		})
	}
}

func TestClientRejectsWorkWhileClosing(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{
			name: "QMI request",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.CardStatus(ctx)
				return err
			},
		},
		{
			name: "service client allocation",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.serviceClientID(ctx, ServiceDMS)
				return err
			},
		},
		{
			name: "indication transport",
			call: func(_ context.Context, client *Client) error {
				_, err := client.indicationTransport()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			client := &Client{transport: transport, slot: 1}
			client.startClose()

			if err := tt.call(t.Context(), client); !errors.Is(err, errClientClosed) {
				t.Fatalf("call() error = %v, want client closed", err)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != 0 {
				t.Fatalf("Do() calls = %d, want 0", got)
			}
			if transport.closeCalls != 1 {
				t.Fatalf("transport Close() calls = %d, want 1", transport.closeCalls)
			}
		})
	}
}

func TestClientCloseSkipsReleaseAfterTransportFailure(t *testing.T) {
	tests := []struct {
		name          string
		terminalErr   error
		failOnRelease error
		wantCalls     int
	}{
		{
			name:        "transport already failed",
			terminalErr: errors.New("transport disconnected"),
			wantCalls:   0,
		},
		{
			name:          "transport fails during first release",
			failOnRelease: errors.New("transport disconnected"),
			wantCalls:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &terminalFakeTransport{
				fakeTransport: fakeTransport{t: t},
				terminalErr:   tt.terminalErr,
				failOnDo:      tt.failOnRelease,
			}
			client := &Client{
				transport: transport,
				slot:      1,
				clientIDs: map[ServiceType]uint8{
					ServiceUIM: 7,
					ServiceWMS: 8,
				},
			}

			if err := client.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if transport.idx != tt.wantCalls {
				t.Fatalf("Do() calls = %d, want %d", transport.idx, tt.wantCalls)
			}
			if transport.closeCalls != 1 {
				t.Fatalf("Close() calls = %d, want 1", transport.closeCalls)
			}
		})
	}
}

func TestClientRejectsRequestsAfterClose(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req Request) {
					if req.MessageID != MessageReleaseClientID {
						t.Fatalf("MessageID = 0x%04X, want release client ID", req.MessageID)
					}
				},
				resp: successResponse(MessageReleaseClientID),
			},
		},
	}
	reader := &Client{
		transport: transport,
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := reader.CardStatus(context.Background()); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("CardStatus() after Close() error = %v, want closed", err)
	}
	if transport.idx != 1 {
		t.Fatalf("Do() calls = %d, want only release client ID", transport.idx)
	}
}

func assertTLV(t *testing.T, tlvs tlv.TLVs, typ byte, want []byte) {
	t.Helper()
	got, ok := tlv.Value(tlvs, typ)
	if !ok {
		t.Fatalf("TLV 0x%02X is missing", typ)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("TLV 0x%02X = % X, want % X", typ, got, want)
	}
}

func assertRequestTimeout(t *testing.T, req Request, want time.Duration) {
	t.Helper()
	if req.Timeout != want {
		t.Fatalf("Timeout = %v, want %v", req.Timeout, want)
	}
}

func successResponse(id MessageID, tlvs ...tlv.TLV) Response {
	return Response{
		Service:   ServiceUIM,
		ClientID:  7,
		MessageID: id,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(qmiTLVResult, []byte{0x00, 0x00, 0x00, 0x00}),
		}, tlvs...),
	}
}

func errorResponse(id MessageID, err QMIError, tlvs ...tlv.TLV) Response {
	return Response{
		Service:   ServiceUIM,
		ClientID:  7,
		MessageID: id,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(qmiTLVResult, []byte{0x01, 0x00, byte(err), byte(uint16(err) >> 8)}),
		}, tlvs...),
	}
}

func encodeLengthPrefixed(data []byte) []byte {
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(data))), data...)
}

func encodeServiceVersions(versions ...serviceVersion) []byte {
	out := []byte{byte(len(versions))}
	for _, version := range versions {
		out = append(out, byte(version.Service))
		out = binary.LittleEndian.AppendUint16(out, version.Major)
		out = binary.LittleEndian.AppendUint16(out, version.Minor)
	}
	return out
}

func encodeFileAttributes(fileSize, fileID uint16, fileType byte, recordSize, recordCount uint16, raw []byte) []byte {
	value := binary.LittleEndian.AppendUint16(nil, fileSize)
	value = binary.LittleEndian.AppendUint16(value, fileID)
	value = append(value, fileType)
	value = binary.LittleEndian.AppendUint16(value, recordSize)
	value = binary.LittleEndian.AppendUint16(value, recordCount)
	for range 5 {
		value = append(value, 0x00)
		value = binary.LittleEndian.AppendUint16(value, 0x0000)
	}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(raw)))
	value = append(value, raw...)
	return value
}

func encodeSlotStatus(activeSlot uint8) []byte {
	value := []byte{0x02}
	for slot := uint8(1); slot <= 2; slot++ {
		value = binary.LittleEndian.AppendUint32(value, 2)
		slotState := uint32(0)
		if slot == activeSlot {
			slotState = 1
		}
		value = binary.LittleEndian.AppendUint32(value, slotState)
		value = append(value, 0x01, 0x00)
	}
	return value
}

func encodeSlotInformation() []byte {
	value := []byte{0x02}
	value = binary.LittleEndian.AppendUint32(value, uint32(CardProtocolICC))
	value = append(value, 0x01, 0x01, 0x3B, 0x00)
	value = binary.LittleEndian.AppendUint32(value, uint32(CardProtocolUICC))
	value = append(value, 0x03, 0x02, 0x3B, 0x9F, 0x01)
	return value
}

func encodeCardStatus(ready bool) []byte {
	value := make([]byte, 0, 64)
	value = append(value, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	value = append(value, 0x01)
	value = append(value, 0x01)
	value = append(value, 0x00, 0x00, 0x00, 0x00)
	value = append(value, 0x01)
	state := byte(0x01)
	if ready {
		state = 0x07
	}
	value = append(value, 0x02, state)
	value = append(value, make([]byte, 12)...)
	return value
}

func tlvTextRecord(value string, size int) []byte {
	record := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(record) < size {
		record = append(record, 0xFF)
	}
	return record
}

func smsPPEnvelope() []byte {
	return []byte{
		0xD1, 0x0E,
		0x82, 0x02, 0x83, 0x81,
		0x86, 0x03, 0x91, 0x21, 0x43,
		0x8B, 0x03, 0x00, 0x7F, 0xF6,
	}
}
