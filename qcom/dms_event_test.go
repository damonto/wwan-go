package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSSetEventReportRequestConfig(t *testing.T) {
	enabled := true
	disabled := false
	limits := DMSBatteryLevelLimits{Lower: 20, Upper: 80}
	tests := []struct {
		name string
		req  DMSSetEventReportRequest
		want map[byte][]byte
	}{
		{
			name: "all settings",
			req: DMSSetEventReportRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
				Config: &DMSEventReportConfig{
					PowerState:         &enabled,
					BatteryLevelLimits: &limits,
					PINState:           &enabled,
					ActivationState:    &disabled,
					OperatingMode:      &enabled,
					UIMState:           &enabled,
					WirelessDisable:    &disabled,
					PRLInit:            &enabled,
				},
			},
			want: map[byte][]byte{
				dmsTLVReportPowerState:    {1},
				dmsTLVBatteryLevelLimits:  {20, 80},
				dmsTLVReportPINState:      {1},
				dmsTLVReportActivation:    {0},
				dmsTLVReportOperatingMode: {1},
				dmsTLVReportUIMState:      {1},
				dmsTLVReportWireless:      {0},
				dmsTLVReportPRLInit:       {1},
			},
		},
		{
			name: "empty explicit config",
			req:  DMSSetEventReportRequest{Config: &DMSEventReportConfig{}},
			want: map[byte][]byte{},
		},
		{
			name: "legacy operating mode disabled",
			req:  DMSSetEventReportRequest{ReportOperatingMode: false},
			want: map[byte][]byte{dmsTLVReportOperatingMode: {0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req.Request()
			if req.Service != ServiceDMS || req.MessageID != MessageDMSSetEventReport {
				t.Fatalf("Request() = service 0x%02X message 0x%04X", req.Service, req.MessageID)
			}
			if len(req.TLVs) != len(tt.want) {
				t.Fatalf("TLVs len = %d, want %d", len(req.TLVs), len(tt.want))
			}
			for typ, value := range tt.want {
				assertTLV(t, req.TLVs, typ, value)
			}
		})
	}
}

func TestDMSSetEventReportRejectsBatteryLimits(t *testing.T) {
	tests := []struct {
		name   string
		limits DMSBatteryLevelLimits
	}{
		{name: "lower above upper", limits: DMSBatteryLevelLimits{Lower: 81, Upper: 80}},
		{name: "lower above 100", limits: DMSBatteryLevelLimits{Lower: 101, Upper: 101}},
		{name: "upper above 100", limits: DMSBatteryLevelLimits{Lower: 20, Upper: 101}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Client{}).DMSSetEventReport(context.Background(), DMSEventReportConfig{
				BatteryLevelLimits: &tt.limits,
			})
			if err == nil {
				t.Fatal("DMSSetEventReport() error = nil, want non-nil")
			}
		})
	}
}

func TestDMSEventUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
		want DMSEvent
	}{
		{name: "empty"},
		{
			name: "all fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVEventPowerState, []byte{byte(DMSPowerExternalSource | DMSPowerBatteryCharging), 74}),
				tlv.Bytes(dmsTLVEventPIN1Status, []byte{byte(DMSPINStatusEnabledVerified), 3, 10}),
				tlv.Bytes(dmsTLVEventPIN2Status, []byte{byte(DMSPINStatusBlocked), 0, 9}),
				tlv.Bytes(dmsTLVEventActivation, binary.LittleEndian.AppendUint16(nil, uint16(DMSActivationConnected))),
				tlv.Bytes(dmsTLVEventOperatingMode, []byte{byte(DMSOperatingModeLowPower)}),
				tlv.Bytes(dmsTLVEventUIMState, []byte{byte(DMSUIMStateUnavailable)}),
				tlv.Bytes(dmsTLVEventWireless, []byte{1}),
				tlv.Bytes(dmsTLVEventPRLInit, []byte{1}),
			},
			want: DMSEvent{
				PowerState: DMSPowerState{
					Status:       DMSPowerExternalSource | DMSPowerBatteryCharging,
					BatteryLevel: 74,
				},
				PowerStateKnown:       true,
				PIN1:                  DMSPINState{Status: DMSPINStatusEnabledVerified, VerifyRetries: 3, UnblockRetries: 10},
				PIN1Known:             true,
				PIN2:                  DMSPINState{Status: DMSPINStatusBlocked, UnblockRetries: 9},
				PIN2Known:             true,
				ActivationState:       DMSActivationConnected,
				ActivationStateKnown:  true,
				OperatingMode:         DMSOperatingModeLowPower,
				OperatingModeKnown:    true,
				UIMState:              DMSUIMStateUnavailable,
				UIMStateKnown:         true,
				WirelessDisabled:      true,
				WirelessDisabledKnown: true,
				PRLInitialized:        true,
				PRLInitializedKnown:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSEvent
			if err := got.UnmarshalTLVs(tt.tlvs); err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMSEventUnmarshalTLVsRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name  string
		typ   byte
		value []byte
	}{
		{name: "power state", typ: dmsTLVEventPowerState, value: []byte{1}},
		{name: "PIN1", typ: dmsTLVEventPIN1Status, value: []byte{1, 2}},
		{name: "PIN2", typ: dmsTLVEventPIN2Status, value: []byte{1, 2, 3, 4}},
		{name: "activation", typ: dmsTLVEventActivation, value: []byte{1}},
		{name: "operating mode", typ: dmsTLVEventOperatingMode, value: nil},
		{name: "UIM", typ: dmsTLVEventUIMState, value: []byte{1, 2}},
		{name: "wireless disable", typ: dmsTLVEventWireless, value: nil},
		{name: "PRL init", typ: dmsTLVEventPRLInit, value: []byte{1, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event DMSEvent
			if err := event.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(tt.typ, tt.value)}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestDMSEventReferences(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "shared event registration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) { assertDMSEventRegistration(t, req, 1) },
					resp:  successResponse(MessageDMSSetEventReport),
				},
				{
					check: func(req Request) { assertDMSEventRegistration(t, req, 0) },
					resp:  successResponse(MessageDMSSetEventReport),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}

			if err := client.acquireDMSEvents(context.Background()); err != nil {
				t.Fatalf("first acquireDMSEvents() error = %v", err)
			}
			if err := client.acquireDMSEvents(context.Background()); err != nil {
				t.Fatalf("second acquireDMSEvents() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after two acquires = %d, want 1", got)
			}

			client.releaseDMSEvents()
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls after first release = %d, want 1", got)
			}
			client.releaseDMSEvents()
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls after final release = %d, want 2", got)
			}
		})
	}
}

func TestDMSWatchEvents(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{
			name: "power operating mode and UIM",
			tlvs: tlv.TLVs{
				tlv.Bytes(dmsTLVEventPowerState, []byte{byte(DMSPowerExternalSource), 100}),
				tlv.Bytes(dmsTLVEventOperatingMode, []byte{byte(DMSOperatingModeOnline)}),
				tlv.Bytes(dmsTLVEventUIMState, []byte{byte(DMSUIMStateInitializationCompleted)}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &dmsIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{check: func(req Request) { assertDMSEventRegistration(t, req, 1) }, resp: successResponse(MessageDMSSetEventReport)},
				{check: func(req Request) { assertDMSEventRegistration(t, req, 0) }, resp: successResponse(MessageDMSSetEventReport)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}

			out, err := client.DMSWatchEvents(ctx)
			if err != nil {
				t.Fatalf("DMSWatchEvents() error = %v", err)
			}
			if transport.service != ServiceDMS || transport.clientID != 7 || transport.messageID != MessageDMSSetEventReport {
				t.Fatalf("Indications() = service 0x%02X client %d message 0x%04X", transport.service, transport.clientID, transport.messageID)
			}
			transport.emit(Indication{Service: ServiceDMS, ClientID: 7, MessageID: MessageDMSSetEventReport, TLVs: tt.tlvs})

			select {
			case got := <-out:
				if !got.PowerStateKnown || got.PowerState.BatteryLevel != 100 ||
					!got.OperatingModeKnown || got.OperatingMode != DMSOperatingModeOnline ||
					!got.UIMStateKnown || got.UIMState != DMSUIMStateInitializationCompleted {
					t.Fatalf("event = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for DMS event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func assertDMSEventRegistration(t *testing.T, req Request, want byte) {
	t.Helper()
	if req.Service != ServiceDMS || req.MessageID != MessageDMSSetEventReport {
		t.Fatalf("request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
	}
	for _, typ := range []byte{
		dmsTLVReportPowerState,
		dmsTLVReportPINState,
		dmsTLVReportActivation,
		dmsTLVReportOperatingMode,
		dmsTLVReportUIMState,
		dmsTLVReportWireless,
		dmsTLVReportPRLInit,
	} {
		assertTLV(t, req.TLVs, typ, []byte{want})
	}
}

type dmsIndicationTransport struct {
	fakeTransport
	indications chan Indication
	service     ServiceType
	clientID    uint8
	messageID   MessageID
}

func (t *dmsIndicationTransport) Indications(
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

func (t *dmsIndicationTransport) emit(indication Indication) {
	t.indications <- indication
}

func (t *dmsIndicationTransport) waitCalls(tb testing.TB, want int) {
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
