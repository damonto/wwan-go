package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestLOCRequestEncoding(t *testing.T) {
	clientString := "wwan"
	clientType := LOCClientNonFramework
	notify := true
	recurrence := LOCFixPeriodic
	intermediate := LOCIntermediateReportEnabled
	interval := uint32(1000)
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "register events",
			request: func() (Request, error) {
				return (LOCRegisterEventsRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Config: LOCRegisterEventsConfig{
						Mask:                           LOCEventNMEA | LOCEventPositionReport,
						ClientString:                   &clientString,
						ClientType:                     &clientType,
						PositioningRequestNotification: &notify,
					},
				}).Request()
			},
			wantMessage: MessageLOCRegisterEvents,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint64(nil, uint64(LOCEventNMEA|LOCEventPositionReport)),
				0x10: []byte(clientString),
				0x11: binary.LittleEndian.AppendUint32(nil, uint32(clientType)),
				0x12: {1},
			},
		},
		{
			name: "start",
			request: func() (Request, error) {
				return (LOCStartRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Config: LOCStartConfig{
						SessionID:                         4,
						FixRecurrence:                     &recurrence,
						IntermediateReports:               &intermediate,
						MinimumReportIntervalMilliseconds: &interval,
					},
				}).Request()
			},
			wantMessage: MessageLOCStart,
			wantTLVs: map[byte][]byte{
				0x01: {4},
				0x10: binary.LittleEndian.AppendUint32(nil, uint32(recurrence)),
				0x12: binary.LittleEndian.AppendUint32(nil, uint32(intermediate)),
				0x13: binary.LittleEndian.AppendUint32(nil, interval),
			},
		},
		{
			name: "stop",
			request: func() (Request, error) {
				return (LOCStopRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, SessionID: 4}).Request(), nil
			},
			wantMessage: MessageLOCStop,
			wantTLVs:    map[byte][]byte{0x01: {4}},
		},
		{
			name: "set NMEA types",
			request: func() (Request, error) {
				return (LOCSetNMEATypesRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Types: LOCNMEAGGA | LOCNMEARMC,
				}).Request(), nil
			},
			wantMessage: MessageLOCSetNMEATypes,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(LOCNMEAGGA|LOCNMEARMC)),
			},
		},
		{
			name: "get NMEA types",
			request: func() (Request, error) {
				return (LOCGetNMEATypesRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageLOCGetNMEATypes,
		},
		{
			name: "set operation mode",
			request: func() (Request, error) {
				return (LOCSetOperationModeRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Mode: LOCOperationModeStandalone,
				}).Request()
			},
			wantMessage: MessageLOCSetOperationMode,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(LOCOperationModeStandalone)),
			},
		},
		{
			name: "get operation mode",
			request: func() (Request, error) {
				return (LOCGetOperationModeRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessageLOCGetOperationMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceLOC || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
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

func TestLOCRequestValidation(t *testing.T) {
	tooLongClientString := "abcde"
	invalidClientType := LOCClientType(4)
	invalidRecurrence := LOCFixRecurrence(3)
	invalidIntermediate := LOCIntermediateReportState(3)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "client string too long",
			call: func() error {
				_, err := (LOCRegisterEventsRequest{Config: LOCRegisterEventsConfig{ClientString: &tooLongClientString}}).Request()
				return err
			},
		},
		{
			name: "client type above range",
			call: func() error {
				_, err := (LOCRegisterEventsRequest{Config: LOCRegisterEventsConfig{ClientType: &invalidClientType}}).Request()
				return err
			},
		},
		{
			name: "fix recurrence above range",
			call: func() error {
				_, err := (LOCStartRequest{Config: LOCStartConfig{FixRecurrence: &invalidRecurrence}}).Request()
				return err
			},
		},
		{
			name: "intermediate report state above range",
			call: func() error {
				_, err := (LOCStartRequest{Config: LOCStartConfig{IntermediateReports: &invalidIntermediate}}).Request()
				return err
			},
		},
		{
			name: "operation mode zero",
			call: func() error {
				_, err := (LOCSetOperationModeRequest{}).Request()
				return err
			},
		},
		{
			name: "operation mode above range",
			call: func() error {
				_, err := (LOCSetOperationModeRequest{Mode: 7}).Request()
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

func TestLOCIndicationsUnmarshalTLVs(t *testing.T) {
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	tests := []struct {
		name    string
		decode  func() error
		check   func(*testing.T)
		wantErr bool
	}{
		{
			name: "status only",
			decode: func() error {
				var got LOCOperationIndication
				return got.UnmarshalTLVs(tlv.TLVs{success})
			},
		},
		{
			name: "NMEA",
			decode: func() error {
				var got LOCNMEAIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte("$GPGGA\r\n"))}); err != nil {
					return err
				}
				if got.Sentence != "$GPGGA\r\n" {
					t.Fatalf("Sentence = %q", got.Sentence)
				}
				return nil
			},
		},
		{
			name: "NMEA types",
			decode: func() error {
				var got LOCNMEATypesIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{
					success,
					tlv.Uint(0x10, uint32(LOCNMEAGGA|LOCNMEARMC)),
				}); err != nil {
					return err
				}
				if !got.TypesKnown || got.Types != LOCNMEAGGA|LOCNMEARMC {
					t.Fatalf("indication = %+v", got)
				}
				return nil
			},
		},
		{
			name: "operation mode",
			decode: func() error {
				var got LOCOperationModeIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{
					success,
					tlv.Uint(0x10, uint32(LOCOperationModeStandalone)),
				}); err != nil {
					return err
				}
				if !got.ModeKnown || got.Mode != LOCOperationModeStandalone {
					t.Fatalf("indication = %+v", got)
				}
				return nil
			},
		},
		{name: "status missing", decode: func() error { var got LOCOperationIndication; return got.UnmarshalTLVs(nil) }, wantErr: true},
		{
			name: "status truncated",
			decode: func() error {
				var got LOCOperationIndication
				return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{0})})
			},
			wantErr: true,
		},
		{name: "NMEA missing", decode: func() error { var got LOCNMEAIndication; return got.UnmarshalTLVs(nil) }, wantErr: true},
		{
			name: "NMEA types truncated",
			decode: func() error {
				var got LOCNMEATypesIndication
				return got.UnmarshalTLVs(tlv.TLVs{success, tlv.Bytes(0x10, []byte{1})})
			},
			wantErr: true,
		},
		{
			name: "operation mode truncated",
			decode: func() error {
				var got LOCOperationModeIndication
				return got.UnmarshalTLVs(tlv.TLVs{success, tlv.Bytes(0x10, []byte{1})})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.decode()
			if tt.wantErr {
				if err == nil {
					t.Fatal("decode() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("decode() error = %v", err)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}

func TestLOCOperationMode(t *testing.T) {
	tests := []struct {
		name       string
		indication tlv.TLVs
		want       LOCOperationMode
		wantErr    error
	}{
		{
			name: "success",
			indication: tlv.TLVs{
				tlv.Uint(0x01, uint32(LOCIndicationSuccess)),
				tlv.Uint(0x10, uint32(LOCOperationModeMSBased)),
			},
			want: LOCOperationModeMSBased,
		},
		{
			name:       "asynchronous failure",
			indication: tlv.TLVs{tlv.Uint(0x01, uint32(LOCIndicationEngineBusy))},
			wantErr:    LOCIndicationEngineBusy,
		},
		{
			name:       "mode missing",
			indication: tlv.TLVs{tlv.Uint(0x01, uint32(LOCIndicationSuccess))},
			wantErr:    errors.New("mode missing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &locAsyncTransport{
				fakeTransport: fakeTransport{t: t, calls: []transportCall{{
					check: func(req Request) {
						if req.Service != ServiceLOC || req.MessageID != MessageLOCGetOperationMode {
							t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
						}
					},
					resp: successResponse(MessageLOCGetOperationMode),
				}}},
				indication: Indication{Service: ServiceLOC, ClientID: 7, MessageID: MessageLOCGetOperationMode, TLVs: tt.indication},
			}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceLOC: 7}}

			got, err := client.LOCOperationMode(context.Background())
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("LOCOperationMode() error = nil, want non-nil")
				}
				if status, ok := tt.wantErr.(LOCIndicationStatus); ok && !errors.Is(err, status) {
					t.Fatalf("LOCOperationMode() error = %v, want status %v", err, status)
				}
				return
			}
			if err != nil {
				t.Fatalf("LOCOperationMode() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("LOCOperationMode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLOCEventReferences(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "watcher preserves caller mask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertLOCEventMask(t, req, LOCEventPositionReport) }, resp: successResponse(MessageLOCRegisterEvents)},
				{check: func(req Request) { assertLOCEventMask(t, req, LOCEventPositionReport|LOCEventNMEA) }, resp: successResponse(MessageLOCRegisterEvents)},
				{check: func(req Request) { assertLOCEventMask(t, req, LOCEventPositionReport) }, resp: successResponse(MessageLOCRegisterEvents)},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceLOC: 7}}

			if err := client.LOCRegisterEvents(context.Background(), LOCRegisterEventsConfig{Mask: LOCEventPositionReport}); err != nil {
				t.Fatalf("LOCRegisterEvents() error = %v", err)
			}
			if err := client.acquireLOCEvents(context.Background(), LOCEventNMEA); err != nil {
				t.Fatalf("first acquireLOCEvents() error = %v", err)
			}
			if err := client.acquireLOCEvents(context.Background(), LOCEventNMEA); err != nil {
				t.Fatalf("second acquireLOCEvents() error = %v", err)
			}
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after two acquires = %d, want 2", got)
			}

			client.releaseLOCEvents(LOCEventNMEA)
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after first release = %d, want 2", got)
			}
			client.releaseLOCEvents(LOCEventNMEA)
			if got := transport.callCount(); got != 3 {
				t.Fatalf("Do() calls after final release = %d, want 3", got)
			}
		})
	}
}

func TestLOCWatchNMEA(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forward sentence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &locNMEAIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertLOCEventMask(t, req, LOCEventNMEA) }, resp: successResponse(MessageLOCRegisterEvents)},
				{check: func(req Request) { assertLOCEventMask(t, req, 0) }, resp: successResponse(MessageLOCRegisterEvents)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceLOC: 7}}

			out, err := client.LOCWatchNMEA(ctx)
			if err != nil {
				t.Fatalf("LOCWatchNMEA() error = %v", err)
			}
			if transport.service != ServiceLOC || transport.clientID != 7 || transport.messageID != MessageLOCNMEA {
				t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}
			transport.emit(Indication{
				Service: ServiceLOC, ClientID: 7, MessageID: MessageLOCNMEA,
				TLVs: tlv.TLVs{tlv.Bytes(0x01, []byte("$GPGGA,watch\r\n"))},
			})

			select {
			case got := <-out:
				if got != "$GPGGA,watch\r\n" {
					t.Fatalf("sentence = %q", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for LOC NMEA")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func assertLOCEventMask(t *testing.T, req Request, want LOCEventRegistration) {
	t.Helper()
	if req.Service != ServiceLOC || req.MessageID != MessageLOCRegisterEvents {
		t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
	}
	assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint64(nil, uint64(want)))
}

type locAsyncTransport struct {
	fakeTransport
	indications chan Indication
	indication  Indication
}

func (t *locAsyncTransport) Indications(
	ctx context.Context,
	_ ServiceType,
	_ uint8,
	_ MessageID,
) (<-chan Indication, error) {
	t.indications = make(chan Indication, 1)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *locAsyncTransport) Do(ctx context.Context, req Request) (Response, error) {
	resp, err := t.fakeTransport.Do(ctx, req)
	if err == nil {
		t.indications <- t.indication
	}
	return resp, err
}

type locNMEAIndicationTransport struct {
	fakeTransport
	indications chan Indication
	service     ServiceType
	clientID    uint8
	messageID   MessageID
}

func (t *locNMEAIndicationTransport) Indications(
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

func (t *locNMEAIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *locNMEAIndicationTransport) waitCalls(tb testing.TB, want int) {
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
