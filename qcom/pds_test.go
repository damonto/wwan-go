package qcom

import (
	"context"
	"encoding/binary"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestPDSRequestEncoding(t *testing.T) {
	enabled := true
	disabled := false
	networkMode := PDSNetworkModeCDMA
	url := "supl.example.test"
	server := PDSAGPSServer{Address: netip.MustParseAddr("192.0.2.1"), Port: 7275}
	session := PDSDefaultTrackingSession{
		OperatingMode:          PDSOperatingModeMSBased,
		PositionTimeoutSeconds: 30,
		IntervalSeconds:        5,
		AccuracyMeters:         25,
	}
	sessionValue := []byte{byte(session.OperatingMode), session.PositionTimeoutSeconds}
	sessionValue = binary.LittleEndian.AppendUint32(sessionValue, session.IntervalSeconds)
	sessionValue = binary.LittleEndian.AppendUint32(sessionValue, session.AccuracyMeters)
	serverValue := []byte{192, 0, 2, 1}
	serverValue = binary.LittleEndian.AppendUint32(serverValue, server.Port)

	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "set every event report field",
			request: func() (Request, error) {
				return (PDSSetEventReportRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Config: PDSEventReportConfig{
						NMEAPosition:                    &enabled,
						ExtendedNMEAPosition:            &disabled,
						ParsedPosition:                  &enabled,
						ExternalXTRADataRequest:         &disabled,
						ExternalTimeInjectionRequest:    &enabled,
						ExternalWiFiPositionRequest:     &disabled,
						SatelliteInformation:            &enabled,
						VXNetworkInitiatedRequest:       &disabled,
						SUPLNetworkInitiatedPrompt:      &enabled,
						UMTSCPNetworkInitiatedPrompt:    &disabled,
						CommunicationEvent:              &enabled,
						AccelerometerStreamingReady:     &disabled,
						GyroStreamingReady:              &enabled,
						TimeSyncRequest:                 &disabled,
						PositionReliability:             &enabled,
						SensorDataUsage:                 &disabled,
						TimeSourceInformation:           &enabled,
						HeadingUncertainty:              &disabled,
						NMEADebugStrings:                &enabled,
						ExtendedExternalXTRADataRequest: &disabled,
					},
				}).Request(), nil
			},
			wantMessage: MessagePDSSetEventReport,
			wantTLVs: map[byte][]byte{
				0x10: {1}, 0x11: {0}, 0x12: {1}, 0x13: {0}, 0x14: {1},
				0x15: {0}, 0x16: {1}, 0x17: {0}, 0x18: {1}, 0x19: {0},
				0x1A: {1}, 0x1B: {0}, 0x1C: {1}, 0x1D: {0}, 0x1E: {1},
				0x1F: {0}, 0x20: {1}, 0x21: {0}, 0x22: {1}, 0x23: {0},
			},
		},
		{
			name: "get GPS service state",
			request: func() (Request, error) {
				return (PDSGetGPSServiceStateRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessagePDSGetGPSServiceState,
		},
		{
			name: "set GPS service state",
			request: func() (Request, error) {
				return (PDSSetGPSServiceStateRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Enabled: true}).Request(), nil
			},
			wantMessage: MessagePDSSetGPSServiceState,
			wantTLVs:    map[byte][]byte{0x01: {1}},
		},
		{
			name: "get default tracking session",
			request: func() (Request, error) {
				return (PDSGetDefaultTrackingSessionRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessagePDSGetDefaultTrackingSession,
		},
		{
			name: "set default tracking session",
			request: func() (Request, error) {
				return (PDSSetDefaultTrackingSessionRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Session: session,
				}).Request()
			},
			wantMessage: MessagePDSSetDefaultTrackingSession,
			wantTLVs:    map[byte][]byte{0x01: sessionValue},
		},
		{
			name: "get A-GPS configuration",
			request: func() (Request, error) {
				return (PDSGetAGPSConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, NetworkMode: PDSNetworkModeCDMA,
				}).Request()
			},
			wantMessage: MessagePDSGetAGPSConfig,
			wantTLVs:    map[byte][]byte{0x12: {byte(PDSNetworkModeCDMA)}},
		},
		{
			name: "set A-GPS configuration",
			request: func() (Request, error) {
				return (PDSSetAGPSConfigRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Config: PDSAGPSConfigUpdate{
						Server:      &server,
						URL:         &url,
						NetworkMode: &networkMode,
					},
				}).Request()
			},
			wantMessage: MessagePDSSetAGPSConfig,
			wantTLVs: map[byte][]byte{
				0x10: serverValue,
				0x11: append([]byte{byte(len(url))}, url...),
				0x14: {byte(PDSNetworkModeCDMA)},
			},
		},
		{
			name: "get automatic tracking state",
			request: func() (Request, error) {
				return (PDSGetAutoTrackingStateRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second}).Request(), nil
			},
			wantMessage: MessagePDSGetAutoTrackingState,
		},
		{
			name: "set automatic tracking state",
			request: func() (Request, error) {
				return (PDSSetAutoTrackingStateRequest{ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Enabled: false}).Request(), nil
			},
			wantMessage: MessagePDSSetAutoTrackingState,
			wantTLVs:    map[byte][]byte{0x01: {0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServicePDS || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
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

func TestPDSRequestValidation(t *testing.T) {
	invalidNetworkMode := PDSNetworkMode(2)
	tooLongURL := strings.Repeat("x", pdsURLMaxLength+1)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "unknown tracking operating mode",
			call: func() error {
				_, err := (PDSSetDefaultTrackingSessionRequest{Session: PDSDefaultTrackingSession{OperatingMode: PDSOperatingModeUnknown}}).Request()
				return err
			},
		},
		{
			name: "tracking operating mode above range",
			call: func() error {
				_, err := (PDSSetDefaultTrackingSessionRequest{Session: PDSDefaultTrackingSession{OperatingMode: 3}}).Request()
				return err
			},
		},
		{
			name: "get A-GPS network mode above range",
			call: func() error {
				_, err := (PDSGetAGPSConfigRequest{NetworkMode: invalidNetworkMode}).Request()
				return err
			},
		},
		{
			name: "set A-GPS network mode above range",
			call: func() error {
				_, err := (PDSSetAGPSConfigRequest{Config: PDSAGPSConfigUpdate{NetworkMode: &invalidNetworkMode}}).Request()
				return err
			},
		},
		{
			name: "A-GPS IPv6 server",
			call: func() error {
				server := PDSAGPSServer{Address: netip.MustParseAddr("2001:db8::1"), Port: 7275}
				_, err := (PDSSetAGPSConfigRequest{Config: PDSAGPSConfigUpdate{Server: &server}}).Request()
				return err
			},
		},
		{
			name: "A-GPS invalid server address",
			call: func() error {
				server := PDSAGPSServer{Port: 7275}
				_, err := (PDSSetAGPSConfigRequest{Config: PDSAGPSConfigUpdate{Server: &server}}).Request()
				return err
			},
		},
		{
			name: "A-GPS URL too long",
			call: func() error {
				_, err := (PDSSetAGPSConfigRequest{Config: PDSAGPSConfigUpdate{URL: &tooLongURL}}).Request()
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

func TestPDSEventUnmarshalTLVs(t *testing.T) {
	sentence := "$GPGGA,example"
	extended := []byte{byte(PDSOperatingModeMSBased)}
	extended = binary.LittleEndian.AppendUint16(extended, uint16(len(sentence)))
	extended = append(extended, sentence...)

	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PDSEvent
		wantErr bool
	}{
		{name: "empty"},
		{
			name: "all supported fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte("$GPRMC,example")),
				tlv.Bytes(0x11, extended),
				tlv.Bytes(0x12, []byte{byte(PDSPositionSessionInProgress)}),
			},
			want: PDSEvent{
				NMEA:               "$GPRMC,example",
				NMEAKnown:          true,
				ExtendedNMEA:       PDSExtendedNMEA{OperatingMode: PDSOperatingModeMSBased, Sentence: sentence},
				ExtendedNMEAKnown:  true,
				SessionStatus:      PDSPositionSessionInProgress,
				SessionStatusKnown: true,
			},
		},
		{
			name: "unknown extended operating mode",
			tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{0xFF, 0, 0})},
			want: PDSEvent{
				ExtendedNMEA:      PDSExtendedNMEA{OperatingMode: PDSOperatingModeUnknown},
				ExtendedNMEAKnown: true,
			},
		},
		{name: "NMEA too long", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, pdsNMEAMaxLength+1))}, wantErr: true},
		{name: "extended header truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 2})}, wantErr: true},
		{name: "extended length too large", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 201, 0})}, wantErr: true},
		{name: "extended sentence truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 2, 0, 'x'})}, wantErr: true},
		{name: "extended sentence trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 1, 0, 'x', 'y'})}, wantErr: true},
		{name: "session status missing", tlvs: tlv.TLVs{tlv.Bytes(0x12, nil)}, wantErr: true},
		{name: "session status trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 2})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PDSEvent
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

func TestPDSGPSServiceStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PDSGPSServiceState
		wantErr bool
	}{
		{
			name: "enabled and active",
			tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, byte(PDSTrackingSessionActive)})},
			want: PDSGPSServiceState{Enabled: true, TrackingSession: PDSTrackingSessionActive},
		},
		{name: "state missing", wantErr: true},
		{name: "state truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}, wantErr: true},
		{name: "state trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2, 3})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PDSGetGPSServiceStateResponse
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
			if got.State != tt.want {
				t.Fatalf("State = %+v, want %+v", got.State, tt.want)
			}
		})
	}
}

func TestPDSDefaultTrackingSessionResponseUnmarshalTLVs(t *testing.T) {
	want := PDSDefaultTrackingSession{
		OperatingMode:          PDSOperatingModeMSAssisted,
		PositionTimeoutSeconds: 45,
		IntervalSeconds:        10,
		AccuracyMeters:         50,
	}
	value := []byte{byte(want.OperatingMode), want.PositionTimeoutSeconds}
	value = binary.LittleEndian.AppendUint32(value, want.IntervalSeconds)
	value = binary.LittleEndian.AppendUint32(value, want.AccuracyMeters)
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{name: "complete", tlvs: tlv.TLVs{tlv.Bytes(0x01, value)}},
		{name: "info missing", wantErr: true},
		{name: "info truncated", tlvs: tlv.TLVs{tlv.Bytes(0x01, value[:9])}, wantErr: true},
		{name: "info trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x01, append(value, 0))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PDSGetDefaultTrackingSessionResponse
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
			if got.Session != want {
				t.Fatalf("Session = %+v, want %+v", got.Session, want)
			}
		})
	}
}

func TestPDSAGPSConfigResponseUnmarshalTLVs(t *testing.T) {
	url := "supl.example.test"
	server := PDSAGPSServer{Address: netip.MustParseAddr("198.51.100.7"), Port: 7275}
	serverValue := []byte{198, 51, 100, 7}
	serverValue = binary.LittleEndian.AppendUint32(serverValue, server.Port)
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PDSAGPSConfig
		wantErr bool
	}{
		{name: "empty"},
		{
			name: "address and URL",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, serverValue),
				tlv.Bytes(0x11, append([]byte{byte(len(url))}, url...)),
			},
			want: PDSAGPSConfig{Server: &server, URL: &url},
		},
		{name: "server truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, serverValue[:7])}, wantErr: true},
		{name: "server trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, append(serverValue, 0))}, wantErr: true},
		{name: "URL length missing", tlvs: tlv.TLVs{tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "URL truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{2, 'x'})}, wantErr: true},
		{name: "URL trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 'x', 'y'})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PDSGetAGPSConfigResponse
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
			if !reflect.DeepEqual(got.Config, tt.want) {
				t.Fatalf("Config = %+v, want %+v", got.Config, tt.want)
			}
		})
	}
}

func TestPDSAutoTrackingStateResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    bool
		wantErr bool
	}{
		{name: "enabled", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}, want: true},
		{name: "disabled", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{0})}},
		{name: "state missing", wantErr: true},
		{name: "state empty", tlvs: tlv.TLVs{tlv.Bytes(0x01, nil)}, wantErr: true},
		{name: "state trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PDSGetAutoTrackingStateResponse
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
			if got.Enabled != tt.want {
				t.Fatalf("Enabled = %t, want %t", got.Enabled, tt.want)
			}
		})
	}
}

func TestPDSEventReferences(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "shared event registration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertPDSEventRegistration(t, req, 1) }, resp: successResponse(MessagePDSSetEventReport)},
				{check: func(req Request) { assertPDSEventRegistration(t, req, 0) }, resp: successResponse(MessagePDSSetEventReport)},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServicePDS: 7}}

			if err := client.acquirePDSEvents(context.Background()); err != nil {
				t.Fatalf("first acquirePDSEvents() error = %v", err)
			}
			if err := client.acquirePDSEvents(context.Background()); err != nil {
				t.Fatalf("second acquirePDSEvents() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releasePDSEvents()
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releasePDSEvents()
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

func TestPDSWatchEvents(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forward extended NMEA and status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &pdsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertPDSEventRegistration(t, req, 1) }, resp: successResponse(MessagePDSSetEventReport)},
				{check: func(req Request) { assertPDSEventRegistration(t, req, 0) }, resp: successResponse(MessagePDSSetEventReport)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServicePDS: 7}}

			out, err := client.PDSWatchEvents(ctx)
			if err != nil {
				t.Fatalf("PDSWatchEvents() error = %v", err)
			}
			if transport.service != ServicePDS || transport.clientID != 7 || transport.messageID != MessagePDSEventReport {
				t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}

			sentence := "$GPGGA,watch"
			value := []byte{byte(PDSOperatingModeStandalone)}
			value = binary.LittleEndian.AppendUint16(value, uint16(len(sentence)))
			value = append(value, sentence...)
			transport.emit(Indication{
				Service:   ServicePDS,
				ClientID:  7,
				MessageID: MessagePDSEventReport,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x11, value),
					tlv.Bytes(0x12, []byte{byte(PDSPositionSessionSuccess)}),
				},
			})

			select {
			case got := <-out:
				if !got.ExtendedNMEAKnown || got.ExtendedNMEA.Sentence != sentence ||
					got.ExtendedNMEA.OperatingMode != PDSOperatingModeStandalone ||
					!got.SessionStatusKnown || got.SessionStatus != PDSPositionSessionSuccess {
					t.Fatalf("event = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for PDS event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestPDSWatchGPSReady(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "forward empty indication"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &pdsIndicationTransport{fakeTransport: fakeTransport{t: t}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDS: 7}}
			ready, err := client.PDSWatchGPSReady(ctx)
			if err != nil {
				t.Fatalf("PDSWatchGPSReady() error = %v", err)
			}
			if transport.service != ServicePDS || transport.clientID != 7 || transport.messageID != MessagePDSGPSReady {
				t.Fatalf("Indications() = service 0x%X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}
			transport.emit(Indication{Service: ServicePDS, ClientID: 7, MessageID: MessagePDSGPSReady})
			select {
			case <-ready:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for GPS Ready")
			}
		})
	}
}

func assertPDSEventRegistration(t *testing.T, req Request, want byte) {
	t.Helper()
	if req.Service != ServicePDS || req.MessageID != MessagePDSSetEventReport {
		t.Fatalf("request = service 0x%X message 0x%04X", req.Service, req.MessageID)
	}
	if len(req.TLVs) != 2 {
		t.Fatalf("TLVs len = %d, want 2", len(req.TLVs))
	}
	assertTLV(t, req.TLVs, 0x10, []byte{want})
	assertTLV(t, req.TLVs, 0x11, []byte{want})
}

type pdsIndicationTransport struct {
	fakeTransport
	indications chan Indication
	service     ServiceType
	clientID    uint8
	messageID   MessageID
}

func (t *pdsIndicationTransport) Indications(
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

func (t *pdsIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *pdsIndicationTransport) waitCalls(tb testing.TB, want int) {
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
