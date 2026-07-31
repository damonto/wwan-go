package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDSDRequestEncoding(t *testing.T) {
	enabled := true
	disabled := false
	temporary := DSDSwitchTemporary

	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "get system status",
			request: func() (Request, error) {
				return (DSDGetSystemStatusRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageDSDGetSystemStatus,
		},
		{
			name: "system status report",
			request: func() (Request, error) {
				return (DSDSystemStatusChangeRequest{ClientID: 7, TransactionID: 9, Config: DSDSystemStatusReportConfig{
					LimitServiceOptionChanges: &enabled,
					ReportChanges:             &enabled,
					PreferredTechnologyOnly:   &disabled,
					ReportNullBearerReason:    &enabled,
				}}).Request(), nil
			},
			wantMessage: MessageDSDSystemStatusChange,
			wantTLVs: map[byte][]byte{
				0x10: {1},
				0x11: {1},
				0x12: {0},
				0x13: {1},
			},
		},
		{
			name: "bind subscription",
			request: func() (Request, error) {
				return (DSDBindSubscriptionRequest{ClientID: 7, TransactionID: 9, Subscription: DSDSubscriptionSecondary}).Request()
			},
			wantMessage: MessageDSDBindSubscription,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(DSDSubscriptionSecondary)),
			},
		},
		{
			name: "get bound subscription",
			request: func() (Request, error) {
				return (DSDGetBindSubscriptionRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageDSDGetBindSubscription,
		},
		{
			name: "current DDS report",
			request: func() (Request, error) {
				return (DSDIndicationRegisterRequest{ClientID: 7, TransactionID: 9, Config: DSDIndicationConfig{CurrentDDS: &enabled}}).Request(), nil
			},
			wantMessage: MessageDSDIndicationRegister,
			wantTLVs:    map[byte][]byte{0x18: {1}},
		},
		{
			name: "switch DDS",
			request: func() (Request, error) {
				return (DSDSwitchDDSRequest{
					ClientID:      7,
					TransactionID: 9,
					Subscription:  DSDSubscriptionTertiary,
					SwitchType:    &temporary,
				}).Request()
			},
			wantMessage: MessageDSDSwitchDDS,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(DSDSubscriptionTertiary)),
				0x10: binary.LittleEndian.AppendUint32(nil, uint32(DSDSwitchTemporary)),
			},
		},
		{
			name: "get current DDS",
			request: func() (Request, error) {
				return (DSDGetCurrentDDSRequest{ClientID: 7, TransactionID: 9}).Request(), nil
			},
			wantMessage: MessageDSDGetCurrentDDS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceDSD || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				value, ok := tlv.Value(got.TLVs, kind)
				if !ok {
					t.Fatalf("TLV 0x%02X missing", kind)
				}
				if !bytes.Equal(value, want) {
					t.Fatalf("TLV 0x%02X = % X, want % X", kind, value, want)
				}
			}
		})
	}
}

func TestDSDRequestValidation(t *testing.T) {
	invalidSwitchType := DSDSwitchType(2)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "subscription zero",
			call: func() error {
				_, err := (DSDBindSubscriptionRequest{}).Request()
				return err
			},
		},
		{
			name: "subscription too high",
			call: func() error {
				_, err := (DSDSwitchDDSRequest{Subscription: 4}).Request()
				return err
			},
		},
		{
			name: "switch type too high",
			call: func() error {
				_, err := (DSDSwitchDDSRequest{Subscription: DSDSubscriptionPrimary, SwitchType: &invalidSwitchType}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("Request() error = nil, want non-nil")
			}
		})
	}
}

func TestDSDSystemStatusResponseUnmarshalTLVs(t *testing.T) {
	lte := DSDSystem{Network: DSDNetwork3GPP, RAT: DSDRATLTE, ServiceOptions: DSDServiceOptionLTEFDD | DSDServiceOptionLTECADownlink}
	nr := DSDSystem{Network: DSDNetwork3GPP, RAT: DSDRAT5G, ServiceOptions: DSDServiceOption5GSub6 | DSDServiceOption5GSA}
	available := append([]byte{2}, encodeDSDSystemForTest(lte)...)
	available = append(available, encodeDSDSystemForTest(nr)...)
	preferred := append(encodeDSDSystemForTest(nr), encodeDSDSystemForTest(lte)...)
	reason := DSDNullBearerReasonOutOfService | DSDNullBearerReasonAttachPending

	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DSDSystemStatus
		wantErr bool
	}{
		{
			name: "all global fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, available),
				tlv.Bytes(0x12, preferred),
				tlv.Bytes(0x14, binary.LittleEndian.AppendUint64(nil, uint64(reason))),
			},
			want: DSDSystemStatus{
				Available:             []DSDSystem{lte, nr},
				AvailableKnown:        true,
				Preferred:             DSDPreferredSystems{Current: nr, Recommended: lte},
				PreferredKnown:        true,
				NullBearerReason:      reason,
				NullBearerReasonKnown: true,
			},
		},
		{name: "optional fields absent"},
		{name: "available count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "available count excessive", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{dsdAvailableSystemMax + 1})}, wantErr: true},
		{name: "available list truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0})}, wantErr: true},
		{name: "available list trailing", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 0})}, wantErr: true},
		{name: "preferred systems truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, make([]byte, 31))}, wantErr: true},
		{name: "preferred systems trailing", tlvs: tlv.TLVs{tlv.Bytes(0x12, make([]byte, 33))}, wantErr: true},
		{name: "null reason truncated", tlvs: tlv.TLVs{tlv.Bytes(0x14, make([]byte, 7))}, wantErr: true},
		{name: "null reason trailing", tlvs: tlv.TLVs{tlv.Bytes(0x14, make([]byte, 9))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response DSDGetSystemStatusResponse
			err := response.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if !reflect.DeepEqual(response.Status, tt.want) {
				t.Fatalf("Status = %+v, want %+v", response.Status, tt.want)
			}
		})
	}
}

func TestDSDSubscriptionResponsesUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		call    func(tlv.TLVs) error
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "bound subscription",
			call: func(tlvs tlv.TLVs) error {
				var response DSDBindSubscriptionResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if !response.SubscriptionKnown || response.Subscription != DSDSubscriptionSecondary {
					t.Fatalf("response = %+v", response)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(DSDSubscriptionSecondary))},
		},
		{
			name: "current DDS response",
			call: func(tlvs tlv.TLVs) error {
				var response DSDGetCurrentDDSResponse
				if err := response.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if !response.Current.SubscriptionKnown || response.Current.Subscription != DSDSubscriptionPrimary ||
					!response.Current.SwitchTypeKnown || response.Current.SwitchType != DSDSwitchTemporary {
					t.Fatalf("response = %+v", response)
				}
				return nil
			},
			tlvs: tlv.TLVs{
				tlv.Uint(0x10, uint32(DSDSubscriptionPrimary)),
				tlv.Uint(0x11, uint32(DSDSwitchTemporary)),
			},
		},
		{
			name: "current DDS indication",
			call: func(tlvs tlv.TLVs) error {
				var indication DSDCurrentDDSIndication
				return indication.UnmarshalTLVs(tlvs)
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(DSDSubscriptionTertiary))},
		},
		{
			name: "switch result",
			call: func(tlvs tlv.TLVs) error {
				var indication DSDSwitchDDSIndication
				if err := indication.UnmarshalTLVs(tlvs); err != nil {
					return err
				}
				if indication.Result != DSDSwitchNotAllowed {
					t.Fatalf("Result = %d, want %d", indication.Result, DSDSwitchNotAllowed)
				}
				return nil
			},
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint32(DSDSwitchNotAllowed))},
		},
		{
			name: "bound subscription truncated",
			call: func(tlvs tlv.TLVs) error {
				var response DSDBindSubscriptionResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, []byte{1})},
			wantErr: true,
		},
		{
			name: "bound subscription trailing",
			call: func(tlvs tlv.TLVs) error {
				var response DSDBindSubscriptionResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 5))},
			wantErr: true,
		},
		{
			name: "current DDS subscription trailing",
			call: func(tlvs tlv.TLVs) error {
				var response DSDGetCurrentDDSResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x10, make([]byte, 5))},
			wantErr: true,
		},
		{
			name: "current DDS switch type trailing",
			call: func(tlvs tlv.TLVs) error {
				var response DSDGetCurrentDDSResponse
				return response.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x11, make([]byte, 5))},
			wantErr: true,
		},
		{
			name: "current indication missing subscription",
			call: func(tlvs tlv.TLVs) error {
				var indication DSDCurrentDDSIndication
				return indication.UnmarshalTLVs(tlvs)
			},
			wantErr: true,
		},
		{
			name: "switch result missing",
			call: func(tlvs tlv.TLVs) error {
				var indication DSDSwitchDDSIndication
				return indication.UnmarshalTLVs(tlvs)
			},
			wantErr: true,
		},
		{
			name: "switch result trailing",
			call: func(tlvs tlv.TLVs) error {
				var indication DSDSwitchDDSIndication
				return indication.UnmarshalTLVs(tlvs)
			},
			tlvs:    tlv.TLVs{tlv.Bytes(0x01, make([]byte, 5))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(tt.tlvs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
		})
	}
}

func TestClientDSDOperations(t *testing.T) {
	enabled := true
	lte := DSDSystem{Network: DSDNetwork3GPP, RAT: DSDRATLTE, ServiceOptions: DSDServiceOptionLTEFDD}
	tests := []struct {
		name    string
		message MessageID
		call    func(*Client) error
		resp    Response
	}{
		{
			name:    "system status",
			message: MessageDSDGetSystemStatus,
			call: func(c *Client) error {
				status, err := c.DSDSystemStatus(context.Background())
				if err == nil && (!status.AvailableKnown || len(status.Available) != 1 || status.Available[0] != lte) {
					t.Fatalf("DSDSystemStatus() = %+v", status)
				}
				return err
			},
			resp: successResponse(MessageDSDGetSystemStatus, tlv.Bytes(0x10, append([]byte{1}, encodeDSDSystemForTest(lte)...))),
		},
		{
			name:    "system status report",
			message: MessageDSDSystemStatusChange,
			call: func(c *Client) error {
				return c.DSDSetSystemStatusReport(context.Background(), DSDSystemStatusReportConfig{ReportChanges: &enabled})
			},
			resp: successResponse(MessageDSDSystemStatusChange),
		},
		{
			name:    "bind subscription",
			message: MessageDSDBindSubscription,
			call: func(c *Client) error {
				return c.DSDBindSubscription(context.Background(), DSDSubscriptionSecondary)
			},
			resp: successResponse(MessageDSDBindSubscription),
		},
		{
			name:    "bound subscription",
			message: MessageDSDGetBindSubscription,
			call: func(c *Client) error {
				subscription, err := c.DSDBoundSubscription(context.Background())
				if err == nil && subscription != DSDSubscriptionTertiary {
					t.Fatalf("DSDBoundSubscription() = %d, want %d", subscription, DSDSubscriptionTertiary)
				}
				return err
			},
			resp: successResponse(MessageDSDGetBindSubscription, tlv.Uint(0x10, uint32(DSDSubscriptionTertiary))),
		},
		{
			name:    "indication report",
			message: MessageDSDIndicationRegister,
			call: func(c *Client) error {
				return c.DSDSetIndicationReport(context.Background(), DSDIndicationConfig{CurrentDDS: &enabled})
			},
			resp: successResponse(MessageDSDIndicationRegister),
		},
		{
			name:    "request DDS switch",
			message: MessageDSDSwitchDDS,
			call: func(c *Client) error {
				return c.DSDRequestSwitchDDS(context.Background(), DSDSubscriptionPrimary, DSDSwitchPermanent)
			},
			resp: successResponse(MessageDSDSwitchDDS),
		},
		{
			name:    "current DDS",
			message: MessageDSDGetCurrentDDS,
			call: func(c *Client) error {
				current, err := c.DSDCurrentDDS(context.Background())
				if err == nil && (!current.SubscriptionKnown || current.Subscription != DSDSubscriptionSecondary) {
					t.Fatalf("DSDCurrentDDS() = %+v", current)
				}
				return err
			},
			resp: successResponse(MessageDSDGetCurrentDDS, tlv.Uint(0x10, uint32(DSDSubscriptionSecondary))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDSD || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDSD: 7}}
			if err := tt.call(client); err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}

func TestDSDIndicationRegistrationReferences(t *testing.T) {
	tests := []struct {
		name         string
		registration dsdIndicationRegistration
		message      MessageID
		kinds        []byte
	}{
		{
			name:         "system status",
			registration: dsdIndicationSystemStatus,
			message:      MessageDSDSystemStatusChange,
			kinds:        []byte{0x11, 0x13},
		},
		{
			name:         "current DDS",
			registration: dsdIndicationCurrentDDS,
			message:      MessageDSDIndicationRegister,
			kinds:        []byte{0x18},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						if req.MessageID != tt.message {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.message)
						}
						for _, kind := range tt.kinds {
							assertTLV(t, req.TLVs, kind, []byte{1})
						}
					},
					resp: successResponse(tt.message),
				},
				{
					check: func(req Request) {
						if req.MessageID != tt.message {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.message)
						}
						for _, kind := range tt.kinds {
							assertTLV(t, req.TLVs, kind, []byte{0})
						}
					},
					resp: successResponse(tt.message),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDSD: 7}}

			if err := client.acquireDSDIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("first acquireDSDIndication() error = %v", err)
			}
			if err := client.acquireDSDIndication(context.Background(), tt.registration); err != nil {
				t.Fatalf("second acquireDSDIndication() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseDSDIndication(tt.registration)
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseDSDIndication(tt.registration)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

func TestDSDWatchers(t *testing.T) {
	lte := DSDSystem{Network: DSDNetwork3GPP, RAT: DSDRATLTE, ServiceOptions: DSDServiceOptionLTEFDD}
	tests := []struct {
		name         string
		message      MessageID
		register     MessageID
		watch        func(context.Context, *Client) (<-chan bool, error)
		indication   Indication
		cleanupCalls int
	}{
		{
			name:     "system status",
			message:  MessageDSDSystemStatus,
			register: MessageDSDSystemStatusChange,
			watch: func(ctx context.Context, c *Client) (<-chan bool, error) {
				statuses, err := c.DSDWatchSystemStatus(ctx)
				if err != nil {
					return nil, err
				}
				out := make(chan bool, 1)
				go func() {
					status, ok := <-statuses
					out <- ok && status.AvailableKnown && len(status.Available) == 1 && status.Available[0] == lte
				}()
				return out, nil
			},
			indication: Indication{
				MessageID: MessageDSDSystemStatus,
				TLVs:      tlv.TLVs{tlv.Bytes(0x10, append([]byte{1}, encodeDSDSystemForTest(lte)...))},
			},
			cleanupCalls: 2,
		},
		{
			name:     "current DDS",
			message:  MessageDSDCurrentDDS,
			register: MessageDSDIndicationRegister,
			watch: func(ctx context.Context, c *Client) (<-chan bool, error) {
				updates, err := c.DSDWatchCurrentDDS(ctx)
				if err != nil {
					return nil, err
				}
				out := make(chan bool, 1)
				go func() {
					current, ok := <-updates
					out <- ok && current.SubscriptionKnown && current.Subscription == DSDSubscriptionSecondary
				}()
				return out, nil
			},
			indication: Indication{
				MessageID: MessageDSDCurrentDDS,
				TLVs:      tlv.TLVs{tlv.Uint(0x01, uint32(DSDSubscriptionSecondary))},
			},
			cleanupCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &dsdIndicationTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{
					{check: func(req Request) {
						if req.MessageID != tt.register {
							t.Fatalf("register MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.register)
						}
					}, resp: successResponse(tt.register)},
					{check: func(req Request) {
						if req.MessageID != tt.register {
							t.Fatalf("unregister MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.register)
						}
					}, resp: successResponse(tt.register)},
				}},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDSD: 7}}
			updates, err := tt.watch(ctx, client)
			if err != nil {
				cancel()
				t.Fatalf("watch error = %v", err)
			}
			transport.emit(tt.message, tt.indication)
			select {
			case ok := <-updates:
				if !ok {
					t.Fatal("watch update did not match indication")
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for DSD indication")
			}
			cancel()
			transport.waitCalls(t, tt.cleanupCalls)
		})
	}
}

func TestDSDSwitchDDSWaitsForResult(t *testing.T) {
	transport := &dsdIndicationTransport{
		fakeTransport: fakeTransport{t: t, calls: []transportCall{{
			check: func(req Request) {
				if req.MessageID != MessageDSDSwitchDDS {
					t.Fatalf("MessageID = 0x%04X, want Switch DDS", req.MessageID)
				}
				assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, uint32(DSDSubscriptionSecondary)))
				assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint32(nil, uint32(DSDSwitchTemporary)))
			},
			resp: successResponse(MessageDSDSwitchDDS),
		}}},
	}
	transport.afterDo = func() {
		transport.emit(MessageDSDSwitchDDS, Indication{
			MessageID: MessageDSDSwitchDDS,
			TLVs:      tlv.TLVs{tlv.Uint(0x01, uint32(DSDSwitchAllowed))},
		})
	}
	client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDSD: 7}}
	result, err := client.DSDSwitchDDS(context.Background(), DSDSubscriptionSecondary, DSDSwitchTemporary)
	if err != nil {
		t.Fatalf("DSDSwitchDDS() error = %v", err)
	}
	if result != DSDSwitchAllowed {
		t.Fatalf("DSDSwitchDDS() = %d, want %d", result, DSDSwitchAllowed)
	}
}

type dsdIndicationTransport struct {
	fakeTransport
	indicationMu sync.Mutex
	indications  map[MessageID]chan Indication
	afterDo      func()
}

func (t *dsdIndicationTransport) Do(ctx context.Context, req Request) (Response, error) {
	resp, err := t.fakeTransport.Do(ctx, req)
	if t.afterDo != nil {
		t.afterDo()
	}
	return resp, err
}

func (t *dsdIndicationTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, id MessageID) (<-chan Indication, error) {
	t.indicationMu.Lock()
	if t.indications == nil {
		t.indications = make(map[MessageID]chan Indication)
	}
	indications := make(chan Indication, 4)
	t.indications[id] = indications
	t.indicationMu.Unlock()
	go func() {
		<-ctx.Done()
		close(indications)
	}()
	return indications, nil
}

func (t *dsdIndicationTransport) emit(id MessageID, indication Indication) {
	t.indicationMu.Lock()
	indications := t.indications[id]
	t.indicationMu.Unlock()
	indications <- indication
}

func (t *dsdIndicationTransport) waitCalls(tb testing.TB, want int) {
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

func encodeDSDSystemForTest(system DSDSystem) []byte {
	value := binary.LittleEndian.AppendUint32(nil, uint32(system.Network))
	value = binary.LittleEndian.AppendUint32(value, uint32(system.RAT))
	return binary.LittleEndian.AppendUint64(value, uint64(system.ServiceOptions))
}
