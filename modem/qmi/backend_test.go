package qmi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name                 string
		err                  error
		wantUnsupported      bool
		wantSARUnsupported   bool
		wantRadioUnavailable bool
	}{
		{name: "not supported", err: fmt.Errorf("query: %w", qcom.QMIErrorNotSupported), wantUnsupported: true, wantSARUnsupported: true},
		{name: "invalid command", err: qcom.QMIErrorInvalidQMICommand, wantUnsupported: true, wantSARUnsupported: true},
		{name: "SAR no memory", err: qcom.QMIErrorNoMemory, wantSARUnsupported: true},
		{name: "information unavailable", err: qcom.QMIErrorInformationUnavailable, wantRadioUnavailable: true},
		{name: "no radio", err: qcom.QMIErrorNoRadio, wantRadioUnavailable: true},
		{name: "no network", err: qcom.QMIErrorNoNetworkFound, wantRadioUnavailable: true},
		{name: "hardware restricted", err: qcom.QMIErrorHardwareRestricted, wantRadioUnavailable: true},
		{name: "transport", err: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isUnsupported(test.err); got != test.wantUnsupported {
				t.Errorf("isUnsupported() = %t, want %t", got, test.wantUnsupported)
			}
			if got := isSARUnsupported(test.err); got != test.wantSARUnsupported {
				t.Errorf("isSARUnsupported() = %t, want %t", got, test.wantSARUnsupported)
			}
			if got := isRadioUnavailable(test.err); got != test.wantRadioUnavailable {
				t.Errorf("isRadioUnavailable() = %t, want %t", got, test.wantRadioUnavailable)
			}
		})
	}
}

func TestPowerState(t *testing.T) {
	tests := []struct {
		name string
		mode qcom.DMSOperatingMode
		want PowerState
	}{
		{name: "online", mode: qcom.DMSOperatingModeOnline, want: PowerStateOn},
		{name: "low power", mode: qcom.DMSOperatingModeLowPower, want: PowerStateLow},
		{name: "persistent low power", mode: qcom.DMSOperatingModePersistentLowPower, want: PowerStateLow},
		{name: "mode-only low power", mode: qcom.DMSOperatingModeModeOnlyLowPower, want: PowerStateLow},
		{name: "offline", mode: qcom.DMSOperatingModeOffline, want: PowerStateOff},
		{name: "shutting down", mode: qcom.DMSOperatingModeShuttingDown, want: PowerStateOff},
		{name: "factory test", mode: qcom.DMSOperatingModeFactoryTest, want: PowerStateUnknown},
		{name: "resetting", mode: qcom.DMSOperatingModeResetting, want: PowerStateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := powerState(tt.mode); got != tt.want {
				t.Errorf("powerState(%d) = %d, want %d", tt.mode, got, tt.want)
			}
		})
	}
}

func TestPowerStateFromInfo(t *testing.T) {
	tests := []struct {
		name string
		info qcom.DMSGetOperatingModeResponse
		want PowerState
	}{
		{
			name: "online",
			info: qcom.DMSGetOperatingModeResponse{Mode: qcom.DMSOperatingModeOnline},
			want: PowerStateOn,
		},
		{
			name: "hardware restriction overrides online",
			info: qcom.DMSGetOperatingModeResponse{
				Mode:                    qcom.DMSOperatingModeOnline,
				HardwareRestricted:      true,
				HardwareRestrictedKnown: true,
			},
			want: PowerStateLow,
		},
		{
			name: "hardware restriction preserves offline",
			info: qcom.DMSGetOperatingModeResponse{
				Mode:                    qcom.DMSOperatingModeOffline,
				HardwareRestricted:      true,
				HardwareRestrictedKnown: true,
			},
			want: PowerStateOff,
		},
		{
			name: "hardware restriction resolves unknown mode as radio off",
			info: qcom.DMSGetOperatingModeResponse{
				Mode:                    qcom.DMSOperatingModeResetting,
				HardwareRestricted:      true,
				HardwareRestrictedKnown: true,
			},
			want: PowerStateLow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := powerStateFromInfo(tt.info); got != tt.want {
				t.Errorf("powerStateFromInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPowerStateFromEvent(t *testing.T) {
	tests := []struct {
		name  string
		event qcom.DMSEvent
		want  PowerState
		ok    bool
	}{
		{
			name:  "operating mode",
			event: qcom.DMSEvent{OperatingMode: qcom.DMSOperatingModeLowPower, OperatingModeKnown: true},
			want:  PowerStateLow,
			ok:    true,
		},
		{
			name:  "wireless disabled",
			event: qcom.DMSEvent{WirelessDisabled: true, WirelessDisabledKnown: true},
			want:  PowerStateLow,
			ok:    true,
		},
		{
			name: "wireless disabled overrides online mode",
			event: qcom.DMSEvent{
				OperatingMode:         qcom.DMSOperatingModeOnline,
				OperatingModeKnown:    true,
				WirelessDisabled:      true,
				WirelessDisabledKnown: true,
			},
			want: PowerStateLow,
			ok:   true,
		},
		{
			name: "offline mode is preserved when wireless is disabled",
			event: qcom.DMSEvent{
				OperatingMode:         qcom.DMSOperatingModeOffline,
				OperatingModeKnown:    true,
				WirelessDisabled:      true,
				WirelessDisabledKnown: true,
			},
			want: PowerStateOff,
			ok:   true,
		},
		{
			name:  "wireless enabled requires operating mode query",
			event: qcom.DMSEvent{WirelessDisabledKnown: true},
		},
		{
			name: "unknown operating mode requires a query",
			event: qcom.DMSEvent{
				OperatingMode:      qcom.DMSOperatingModeFactoryTest,
				OperatingModeKnown: true,
			},
		},
		{name: "unrelated event", want: PowerStateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := powerStateFromEvent(tt.event)
			if got != tt.want || ok != tt.ok {
				t.Errorf("powerStateFromEvent() = %v, %t, want %v, %t", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPowerTransitionNeedsStatusRefresh(t *testing.T) {
	tests := []struct {
		name    string
		current PowerState
		next    PowerState
		want    bool
	}{
		{name: "low to on", current: PowerStateLow, next: PowerStateOn, want: true},
		{name: "off to on", current: PowerStateOff, next: PowerStateOn, want: true},
		{name: "unknown to on", current: PowerStateUnknown, next: PowerStateOn, want: true},
		{name: "on remains on", current: PowerStateOn, next: PowerStateOn},
		{name: "on to low", current: PowerStateOn, next: PowerStateLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := powerTransitionNeedsStatusRefresh(tt.current, tt.next); got != tt.want {
				t.Errorf("powerTransitionNeedsStatusRefresh() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestStatusIndicationsEmpty(t *testing.T) {
	tests := []struct {
		name        string
		indications statusIndications
		want        bool
	}{
		{name: "all unavailable", want: true},
		{name: "power available", indications: statusIndications{power: make(chan qcom.DMSEvent)}},
		{name: "card available", indications: statusIndications{card: make(chan qcom.CardStatus)}},
		{name: "network available", indications: statusIndications{network: make(chan qcom.NASServingSystem)}},
		{name: "signal available", indications: statusIndications{signal: make(chan qcom.NASSignalInfo)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.indications.empty(); got != tt.want {
				t.Errorf("statusIndications.empty() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestApplyPowerState(t *testing.T) {
	registered := Status{
		Power:         PowerStateOn,
		Registration:  RegistrationRoaming,
		PacketService: PacketServiceAttached,
		Technology:    TechnologyLTE,
		OperatorID:    "46001",
		OperatorName:  "UNICOM",
		SignalQuality: 80,
	}
	tests := []struct {
		name  string
		state PowerState
		want  Status
	}{
		{name: "online preserves radio status", state: PowerStateOn, want: registered},
		{
			name:  "low power clears radio status",
			state: PowerStateLow,
			want: Status{
				Power:         PowerStateLow,
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
		{
			name:  "offline clears radio status",
			state: PowerStateOff,
			want: Status{
				Power:         PowerStateOff,
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
		{
			name:  "unknown clears radio status",
			state: PowerStateUnknown,
			want: Status{
				Registration:  RegistrationIdle,
				PacketService: PacketServiceDetached,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registered
			applyPowerState(&got, tt.state)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("applyPowerState() status = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestApplyCellLocation(t *testing.T) {
	tests := []struct {
		name     string
		location qcom.NASCellLocationInfo
		want     uint32
	}{
		{name: "unknown frequency keeps existing value", location: qcom.NASCellLocationInfo{LTEIntraEARFCN: 1800}, want: 900},
		{name: "known LTE frequency updates ARFCN", location: qcom.NASCellLocationInfo{LTEIntraEARFCN: 1800, LTEIntraEARFCNKnown: true}, want: 1800},
		{name: "embedded LTE frequency updates ARFCN", location: qcom.NASCellLocationInfo{LTEIntra: qcom.NASLTEIntraFrequency{EARFCN: 1650}, LTEIntraKnown: true}, want: 1650},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cell := CellInfo{ARFCN: 900}
			applyCellLocation(&cell, tt.location)
			if cell.ARFCN != tt.want {
				t.Errorf("CellInfo.ARFCN = %d, want %d", cell.ARFCN, tt.want)
			}
		})
	}
}

func TestNetworkStatusFromServing(t *testing.T) {
	tests := []struct {
		name    string
		serving qcom.NASServingSystem
		want    NetworkStatus
	}{
		{
			name: "maps a roaming LTE serving system",
			serving: qcom.NASServingSystem{
				RegistrationState:     qcom.NASRegistrationRegistered,
				PSAttachState:         qcom.NASAttachAttached,
				RadioInterfaces:       []qcom.NASRadioInterface{qcom.NASRadioInterfaceLTE},
				RoamingIndicator:      qcom.NASRoamingIndicatorRoaming,
				RoamingIndicatorKnown: true,
				PLMN:                  qcom.NASPLMN{MCC: 460, MNC: 1, Description: "carrier"},
				PLMNKnown:             true,
				LocationAreaCode:      10,
				TrackingAreaCode:      20,
				CellID:                30,
			},
			want: NetworkStatus{
				Registration:     RegistrationRoaming,
				PacketService:    PacketServiceAttached,
				Technology:       TechnologyLTE,
				Available:        TechnologyLTE,
				OperatorID:       "46001",
				OperatorName:     "carrier",
				RoamingText:      "roaming",
				LocationAreaCode: 10,
				TrackingAreaCode: 20,
				CellID:           30,
			},
		},
		{
			name: "decodes a packed GSM7 roaming operator name",
			serving: qcom.NASServingSystem{
				RegistrationState:     qcom.NASRegistrationRegistered,
				RoamingIndicator:      qcom.NASRoamingIndicatorRoaming,
				RoamingIndicatorKnown: true,
				PLMN: qcom.NASPLMN{
					MCC: 222, MNC: 50,
					Description: string([]byte{0x49, 0x76, 0x3A, 0x4C, 0x06}),
				},
				PLMNKnown: true,
			},
			want: NetworkStatus{
				Registration: RegistrationRoaming,
				OperatorID:   "22250",
				OperatorName: "Iliad",
				RoamingText:  "roaming",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkStatusFromServing(tt.serving); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("networkStatusFromServing() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeNetworkDescription(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
		want  string
	}{
		{name: "ASCII", value: []byte("T-Mobile"), want: "T-Mobile"},
		{name: "UTF-8", value: []byte("中国移动"), want: "中国移动"},
		{name: "packed GSM7", value: []byte{0x49, 0x76, 0x3A, 0x4C, 0x06}, want: "Iliad"},
		{
			name: "UCS2 little endian",
			value: []byte{
				0x54, 0x00, 0x2D, 0x00, 0x4D, 0x00, 0x6F, 0x00,
				0x62, 0x00, 0x69, 0x00, 0x6C, 0x00, 0x65, 0x00,
			},
			want: "T-Mobile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeNetworkDescription(string(tt.value)); got != tt.want {
				t.Errorf("decodeNetworkDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignalFromInfo(t *testing.T) {
	tests := []struct {
		name string
		info qcom.NASSignalInfo
		want Signal
	}{
		{
			name: "maps LTE measurements",
			info: qcom.NASSignalInfo{
				LTEKnown: true,
				LTE:      qcom.NASLTESignalInfo{RSSI: -70, RSRQ: -10, RSRP: -95, SNR: 125},
			},
			want: Signal{
				Quality: 69,
				Radios: []RadioSignal{{
					Technology: TechnologyLTE,
					RSSI:       knownSignal(-70),
					RSRQ:       knownSignal(-10),
					RSRP:       knownSignal(-95),
					SNR:        knownSignal(12.5),
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := signalFromInfo(tt.info); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("signalFromInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type capabilityTransport struct {
	requests []qcom.Request
}

func (t *capabilityTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.requests = append(t.requests, req)
	return qcom.Response{
		Service: req.Service, ClientID: req.ClientID, TransactionID: req.TransactionID, MessageID: req.MessageID,
		TLVs: tlv.TLVs{tlv.Bytes(0x02, []byte{0, 0, 0, 0})},
	}, nil
}

func (*capabilityTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*capabilityTransport) Close() error { return nil }

type simIdentityTransport struct {
	t        *testing.T
	requests []qcom.Request
	data     []byte
}

func (t *simIdentityTransport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	t.requests = append(t.requests, req)
	if req.Service != qcom.ServiceUIM || req.MessageID != qcom.MessageReadTransparent {
		t.t.Fatalf("request = service 0x%02X message 0x%04X, want UIM read transparent", req.Service, req.MessageID)
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > qmiSIMICCIDTimeout {
		t.t.Fatalf("ICCID request deadline = %v, want at most %v", deadline, qmiSIMICCIDTimeout)
	}
	data := binary.LittleEndian.AppendUint16(nil, uint16(len(t.data)))
	data = append(data, t.data...)
	return qcom.Response{
		Service: req.Service, ClientID: req.ClientID, TransactionID: req.TransactionID, MessageID: req.MessageID,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x02, []byte{0, 0, 0, 0}),
			tlv.Bytes(0x10, []byte{0x90, 0x00}),
			tlv.Bytes(0x11, data),
		},
	}, nil
}

func (*simIdentityTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*simIdentityTransport) Close() error { return nil }

type simATRTransport struct {
	requests []qcom.Request
	appState qcom.ApplicationState
	atr      []byte
	atrErr   error
}

func (t *simATRTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.requests = append(t.requests, req)
	switch {
	case req.Service == qcom.ServiceUIM && req.MessageID == qcom.MessageGetCardStatus:
		status := make([]byte, 8)
		status = append(status, 1)
		status = append(status, byte(qcom.CardStatePresent), 0, 0, 0, 0, 1)
		status = append(status, byte(qcom.ApplicationTypeUSIM), byte(t.appState))
		status = append(status, make([]byte, 12)...)
		return qcom.Response{
			Service: req.Service, ClientID: req.ClientID, TransactionID: req.TransactionID, MessageID: req.MessageID,
			TLVs: tlv.TLVs{
				tlv.Bytes(0x02, []byte{0, 0, 0, 0}),
				tlv.Bytes(0x10, status),
			},
		}, nil
	case req.Service == qcom.ServiceUIM && req.MessageID == qcom.MessageGetATR:
		if t.atrErr != nil {
			return qcom.Response{}, t.atrErr
		}
		value := append([]byte{byte(len(t.atr))}, t.atr...)
		return qcom.Response{
			Service: req.Service, ClientID: req.ClientID, TransactionID: req.TransactionID, MessageID: req.MessageID,
			TLVs: tlv.TLVs{
				tlv.Bytes(0x02, []byte{0, 0, 0, 0}),
				tlv.Bytes(0x10, value),
			},
		}, nil
	default:
		return qcom.Response{}, errors.New("subscriber identity unavailable")
	}
}

func (*simATRTransport) ClientID(context.Context, qcom.ServiceType) (uint8, error) {
	return 7, nil
}

func (*simATRTransport) Close() error { return nil }

func TestSIMInfoReadsIndependentATR(t *testing.T) {
	errATR := errors.New("ATR unavailable")
	tests := []struct {
		name      string
		appState  qcom.ApplicationState
		atr       []byte
		atrErr    error
		wantState SIMState
		wantATR   []byte
	}{
		{
			name:      "locked SIM",
			appState:  qcom.ApplicationStatePIN1OrUPINRequired,
			atr:       []byte{0x3B, 0x80, 0x81, 0x2F, 0x82, 0xAC},
			wantState: SIMStateLocked,
			wantATR:   []byte{0x3B, 0x80, 0x81, 0x2F, 0x82, 0xAC},
		},
		{
			name:      "ready SIM",
			appState:  qcom.ApplicationStateReady,
			atr:       []byte{0x3B, 0x00},
			wantState: SIMStateReady,
			wantATR:   []byte{0x3B, 0x00},
		},
		{
			name:      "ATR unavailable",
			appState:  qcom.ApplicationStatePIN1OrUPINRequired,
			atrErr:    errATR,
			wantState: SIMStateLocked,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &simATRTransport{appState: tt.appState, atr: tt.atr, atrErr: tt.atrErr}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")

			got, err := backend.SIMInfo(t.Context())
			if err != nil {
				t.Fatalf("SIMInfo() error = %v", err)
			}
			if got.State != tt.wantState {
				t.Fatalf("SIM state = %d, want %d", got.State, tt.wantState)
			}
			if !bytes.Equal(got.ATR, tt.wantATR) {
				t.Fatalf("ATR = % X, want % X", got.ATR, tt.wantATR)
			}
			atrCalls := 0
			for _, req := range transport.requests {
				if req.Service == qcom.ServiceUIM && req.MessageID == qcom.MessageGetATR {
					atrCalls++
				}
			}
			if atrCalls != 1 {
				t.Fatalf("ATR request count = %d, want 1", atrCalls)
			}
		})
	}
}

func TestSIMInfoFromCardStatus(t *testing.T) {
	rawICCID := []byte{0x98, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0}
	tests := []struct {
		name       string
		appState   qcom.ApplicationState
		metadata   SIMInfo
		metadataID string
		want       SIMInfo
		wantRead   bool
	}{
		{
			name:     "refreshing USIM does not read files",
			appState: qcom.ApplicationStateDetected,
			want:     SIMInfo{State: SIMStateUnknown, Slot: 1, PINRetries: 3, PUKRetries: 10},
		},
		{
			name:     "ready USIM reads only ICCID",
			appState: qcom.ApplicationStateReady,
			want:     SIMInfo{State: SIMStateReady, Slot: 1, ICCID: "8986000000000000000", PINRetries: 3, PUKRetries: 10},
			wantRead: true,
		},
		{
			name:       "ready USIM applies cached metadata",
			appState:   qcom.ApplicationStateReady,
			metadataID: "8986000000000000000",
			metadata: SIMInfo{
				ICCID: "8986000000000000000", IMSI: "310260000000000",
				OperatorID: "310260", OperatorName: "Example", GID1: "AB", SPN: "Example",
			},
			want: SIMInfo{
				State: SIMStateReady, Slot: 1, ICCID: "8986000000000000000", IMSI: "310260000000000",
				OperatorID: "310260", OperatorName: "Example", GID1: "AB", SPN: "Example",
				PINRetries: 3, PUKRetries: 10,
			},
			wantRead: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &simIdentityTransport{t: t, data: rawICCID}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")
			backend.metadataKey = tt.metadataID
			backend.metadata = tt.metadata
			status := qcom.CardStatus{Cards: []qcom.Card{{
				State: qcom.CardStatePresent,
				Applications: []qcom.CardApplication{{
					Type: qcom.ApplicationTypeUSIM, State: tt.appState, PIN1Retries: 3, PUK1Retries: 10,
				}},
			}}}

			got := backend.simInfoFromCardStatus(context.Background(), status)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("simInfoFromCardStatus() = %+v, want %+v", got, tt.want)
			}
			if (len(transport.requests) == 1) != tt.wantRead {
				t.Fatalf("read count = %d, wantRead %t", len(transport.requests), tt.wantRead)
			}
			if tt.wantRead {
				fileValue, ok := tlv.Value(transport.requests[0].TLVs, 0x02)
				if !ok || !bytes.Equal(fileValue, []byte{0xE2, 0x2F, 0x02, 0x00, 0x3F}) {
					t.Fatalf("ICCID file TLV = % X, want EF_ICCID", fileValue)
				}
			}
		})
	}
}

func TestMetadataLoadLockHonorsContext(t *testing.T) {
	backend := &Backend{}
	if err := backend.lockMetadata(context.Background()); err != nil {
		t.Fatalf("lockMetadata() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := backend.lockMetadata(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lockMetadata() error = %v, want context deadline exceeded", err)
	}

	backend.unlockMetadata()
	if err := backend.lockMetadata(t.Context()); err != nil {
		t.Fatalf("lockMetadata() after release error = %v", err)
	}
	backend.unlockMetadata()
}

func TestSIMSlotsFromStatus(t *testing.T) {
	tests := []struct {
		name   string
		status qcom.SlotStatus
		want   []SIMSlot
	}{
		{
			name: "preserves ATR without trusting modem classification",
			status: qcom.SlotStatus{Slots: []qcom.Slot{{
				PhysicalCardStatus: qcom.PhysicalCardStatePresent,
				PhysicalSlotStatus: qcom.SlotStateActive,
				ATR:                []byte{0x3B, 0x80, 0x81, 0x2F, 0x82, 0xAC},
				IsEUICC:            true,
			}}},
			want: []SIMSlot{{
				Index:  1,
				Active: true,
				State:  SIMStateReady,
				ATR:    []byte{0x3B, 0x80, 0x81, 0x2F, 0x82, 0xAC},
			}},
		},
		{
			name:   "empty status",
			status: qcom.SlotStatus{},
			want:   []SIMSlot{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := simSlotsFromStatus(tt.status)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("simSlotsFromStatus() = %+v, want %+v", got, tt.want)
			}
			if len(got) == 0 || len(got[0].ATR) == 0 {
				return
			}
			original := tt.status.Slots[0].ATR[0]
			got[0].ATR[0] = 0
			if tt.status.Slots[0].ATR[0] != original {
				t.Fatal("simSlotsFromStatus() returned an aliased ATR")
			}
		})
	}
}

func TestNetworkConfig(t *testing.T) {
	tests := []struct {
		name string
		info qcom.PDNInfo
		want NetworkConfig
	}{
		{
			name: "dual stack",
			info: qcom.PDNInfo{
				LocalIPv4: net.ParseIP("192.0.2.2"), IPv4SubnetMask: net.IPv4(255, 255, 255, 0),
				IPv4Gateway: net.ParseIP("192.0.2.1"), LocalIPv6: net.ParseIP("2001:db8::2"),
				IPv6PrefixLength: 64, IPv6Gateway: net.ParseIP("2001:db8::1"),
				DNS: []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2001:4860:4860::8888")}, MTU: 1500,
			},
			want: NetworkConfig{Interface: "wwan0", Addresses: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.2/24"), netip.MustParsePrefix("2001:db8::2/64"),
			}, Gateways: []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")},
				DNS: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2001:4860:4860::8888")}, MTU: 1500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkConfig("wwan0", tt.info); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("networkConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeSIMIdentity(t *testing.T) {
	tests := []struct {
		name   string
		result SIMInfo
		want   SIMInfo
	}{
		{
			name: "uses card values when DMS is unavailable",
			want: SIMInfo{ICCID: "8986001234567890123", IMSI: "460001234567890"},
		},
		{
			name:   "preserves DMS values",
			result: SIMInfo{ICCID: "8986000000000000000", IMSI: "460000000000000"},
			want:   SIMInfo{ICCID: "8986000000000000000", IMSI: "460000000000000"},
		},
	}
	card := SIMInfo{ICCID: "8986001234567890123", IMSI: "460001234567890"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeSIMIdentity(&tt.result, card)
			if tt.result.ICCID != tt.want.ICCID || tt.result.IMSI != tt.want.IMSI {
				t.Fatalf("SIM identity = %+v, want %+v", tt.result, tt.want)
			}
		})
	}
}

func TestIPPreferences(t *testing.T) {
	tests := []struct {
		name   string
		family IPFamily
		want   []qcom.WDSIPPreference
	}{
		{name: "default", family: IPFamilyUnknown, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceDefault}},
		{name: "IPv4", family: IPFamilyIPv4, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4}},
		{name: "IPv6", family: IPFamilyIPv6, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv6}},
		{name: "dual stack", family: IPFamilyIPv4v6, want: []qcom.WDSIPPreference{qcom.WDSIPPreferenceIPv4, qcom.WDSIPPreferenceIPv6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipPreferences(tt.family); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ipPreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeNetworkConfigs(t *testing.T) {
	tests := []struct {
		name  string
		infos []qcom.PDNInfo
		want  NetworkConfig
	}{
		{
			name: "combines families and uses the smaller MTU",
			infos: []qcom.PDNInfo{
				{
					LocalIPv4: net.ParseIP("192.0.2.2"), IPv4SubnetMask: net.IPv4(255, 255, 255, 0),
					IPv4Gateway: net.ParseIP("192.0.2.1"), DNS: []net.IP{net.ParseIP("1.1.1.1")}, MTU: 1500,
				},
				{
					LocalIPv6: net.ParseIP("2001:db8::2"), IPv6PrefixLength: 64,
					IPv6Gateway: net.ParseIP("2001:db8::1"), DNS: []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2001:4860:4860::8888")}, MTU: 1420,
				},
			},
			want: NetworkConfig{
				Interface: "wwan0",
				Addresses: []netip.Prefix{netip.MustParsePrefix("192.0.2.2/24"), netip.MustParsePrefix("2001:db8::2/64")},
				Gateways:  []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")},
				DNS:       []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2001:4860:4860::8888")},
				MTU:       1420,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeNetworkConfigs("wwan0", tt.infos); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeNetworkConfigs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFeaturesFromVersions(t *testing.T) {
	tests := []struct {
		name     string
		services []qcom.ServiceVersion
		want     Feature
	}{
		{name: "none"},
		{
			name: "advertised services only",
			services: []qcom.ServiceVersion{
				{Service: qcom.ServiceDMS}, {Service: qcom.ServiceWDS}, {Service: qcom.ServiceWMS},
				{Service: qcom.ServiceVoice}, {Service: qcom.ServiceSAR}, {Service: qcom.ServiceNAS},
			},
			want: FeatureFirmwareUpdate | FeatureFacilityLocks | FeatureProfileManagement |
				FeatureInitialEPSBearer | FeatureSMS | FeatureUSSD | FeatureSAR |
				FeatureSignalThresholds | FeatureCellInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := featuresFromVersions(tt.services); got != tt.want {
				t.Errorf("featuresFromVersions() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestSetCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		technologies Technology
		wantErr      bool
		wantMessages []qcom.MessageID
	}{
		{
			name:         "sets a permanent preference then resets",
			technologies: TechnologyLTE | TechnologyNR5GSA,
			wantMessages: []qcom.MessageID{qcom.MessageNASSetSystemSelectionPreference, qcom.MessageDMSReset},
		},
		{name: "rejects empty capabilities", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := new(capabilityTransport)
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			backend := New(client, "/dev/test")
			err = backend.SetCapabilities(context.Background(), tt.technologies)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetCapabilities() error = %v, wantErr %t", err, tt.wantErr)
			}
			var messages []qcom.MessageID
			for _, request := range transport.requests {
				messages = append(messages, request.MessageID)
			}
			if !reflect.DeepEqual(messages, tt.wantMessages) {
				t.Errorf("SetCapabilities() messages = %v, want %v", messages, tt.wantMessages)
			}
			if !tt.wantErr {
				if _, ok := transport.requests[0].TLVs.Find(0x17); !ok {
					t.Error("SetCapabilities() omitted permanent change duration")
				}
			}
		})
	}
}

func TestAPNTypeMapping(t *testing.T) {
	tests := []struct {
		name  string
		value APNType
		want  qcom.WDSAPNTypeMask
	}{
		{name: "default", value: APNTypeDefault, want: qcom.WDSAPNTypeDefault},
		{name: "IMS", value: APNTypeIMS, want: qcom.WDSAPNTypeIMS},
		{name: "MMS", value: APNTypeMMS, want: qcom.WDSAPNTypeMMS},
		{name: "tethering", value: APNTypeTethering, want: qcom.WDSAPNTypeDUN},
		{name: "SUPL", value: APNTypeSUPL, want: qcom.WDSAPNTypeSUPL},
		{name: "emergency", value: APNTypeEmergency, want: qcom.WDSAPNTypeEmergency},
		{name: "mask", value: APNTypeDefault | APNTypeIMS, want: qcom.WDSAPNTypeDefault | qcom.WDSAPNTypeIMS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := apnTypeMask(tt.value)
			if value != tt.want {
				t.Errorf("apnTypeMask() = %#x, want %#x", value, tt.want)
			}
			if got := apnTypeFromMask(value); got != tt.value {
				t.Errorf("apnTypeFromMask() = %#x, want %#x", got, tt.value)
			}
		})
	}
}

func TestSignalReportConfigs(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantRate uint8
		wantNil  bool
		wantErr  bool
	}{
		{name: "disabled", wantNil: true},
		{name: "subsecond rounds up", interval: time.Nanosecond, wantRate: 1},
		{name: "one second", interval: time.Second, wantRate: 1},
		{name: "fractional seconds round up", interval: time.Second + time.Nanosecond, wantRate: 2},
		{name: "maximum", interval: 5 * time.Second, wantRate: 5},
		{name: "over maximum", interval: 5*time.Second + time.Nanosecond, wantErr: true},
		{name: "negative", interval: -time.Second, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lte, nr5g, err := signalReportConfigs(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Fatalf("signalReportConfigs() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNil {
				if lte != nil || nr5g != nil {
					t.Fatalf("signalReportConfigs() = (%v, %v), want nil", lte, nr5g)
				}
				return
			}
			if lte == nil || nr5g == nil || uint8(lte.Rate) != tt.wantRate || uint8(nr5g.Rate) != tt.wantRate {
				t.Errorf("signalReportConfigs() = (%v, %v), want rate %d", lte, nr5g, tt.wantRate)
			}
		})
	}
}

func TestNASRadioFromTechnology(t *testing.T) {
	tests := []struct {
		name       string
		technology Technology
		want       qcom.NASRadioInterface
	}{
		{name: "unset leaves the interface unchanged", technology: 0, want: qcom.NASRadioInterfaceNoChange},
		{name: "GSM", technology: TechnologyGSM, want: qcom.NASRadioInterfaceGSM},
		{name: "UMTS", technology: TechnologyUMTS, want: qcom.NASRadioInterfaceUMTS},
		{name: "LTE", technology: TechnologyLTE, want: qcom.NASRadioInterfaceLTE},
		{name: "5G SA", technology: TechnologyNR5GSA, want: qcom.NASRadioInterfaceNR5G},
		{name: "5G NSA registers on LTE", technology: TechnologyLTE | TechnologyNR5GNSA, want: qcom.NASRadioInterfaceLTE},
		{name: "unmappable leaves the interface unchanged", technology: TechnologyLTECatM, want: qcom.NASRadioInterfaceNoChange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nasRadioFromTechnology(tt.technology); got != tt.want {
				t.Errorf("nasRadioFromTechnology(%#x) = %#x, want %#x", tt.technology, got, tt.want)
			}
		})
	}
}

// expectRegisterSelection asserts the network-selection fields of a System
// Selection Preference write and reports err back to the caller.
func expectRegisterSelection(t *testing.T, wantSelection []byte, wantRadio []byte, err qcom.QMIError) radioTestCall {
	t.Helper()
	return func(req qcom.Request) (qcom.Response, error) {
		if req.Service != qcom.ServiceNAS || req.MessageID != qcom.MessageNASSetSystemSelectionPreference {
			t.Fatalf("QMI request = service %#x message %#x, want NAS Set System Selection Preference", req.Service, req.MessageID)
		}
		selection, ok := tlv.Value(req.TLVs, 0x16)
		if !ok {
			t.Fatal("Set System Selection Preference is missing the network selection TLV")
		}
		if !bytes.Equal(selection, wantSelection) {
			t.Errorf("network selection = %#v, want %#v", selection, wantSelection)
		}
		radio, hasRadio := tlv.Value(req.TLVs, 0x22)
		if wantRadio == nil {
			if hasRadio {
				t.Errorf("radio interface = %#v, want no TLV", radio)
			}
		} else if !bytes.Equal(radio, wantRadio) {
			t.Errorf("radio interface = %#v, want %#v", radio, wantRadio)
		}
		if err != 0 {
			return failedStatusResponse(req, err), nil
		}
		return successfulStatusResponse(req), nil
	}
}

func TestBackendRegisterPrefersSystemSelectionPreference(t *testing.T) {
	// Manual registration without a requested technology must not constrain the
	// radio interface.
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t, []byte{byte(qcom.NASNetworkSelectionManual), 0xCC, 0x01, 0x00, 0x00}, nil, 0),
	)

	if err := backend.Register(t.Context(), RegisterConfig{OperatorID: "46000"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestBackendRegisterConstrainsRequestedTechnology(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t,
			[]byte{byte(qcom.NASNetworkSelectionManual), 0xCC, 0x01, 0x01, 0x00},
			[]byte{byte(qcom.NASRadioInterfaceLTE)}, 0),
	)

	if err := backend.Register(t.Context(), RegisterConfig{OperatorID: "46001", Technology: TechnologyLTE}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestBackendRegisterAutomaticClearsManualSelection(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t, []byte{byte(qcom.NASNetworkSelectionAutomatic), 0x00, 0x00, 0x00, 0x00}, nil, 0),
	)

	if err := backend.Register(t.Context(), RegisterConfig{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestBackendRegisterFallsBackToInitiateNetworkRegister(t *testing.T) {
	// Firmware without the network-selection field must still register, and the
	// deprecated message must carry "no change" rather than "no service".
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t,
			[]byte{byte(qcom.NASNetworkSelectionManual), 0xCC, 0x01, 0x00, 0x00}, nil,
			qcom.QMIErrorNotSupported),
		func(req qcom.Request) (qcom.Response, error) {
			if req.Service != qcom.ServiceNAS || req.MessageID != qcom.MessageNASInitiateNetworkRegister {
				t.Fatalf("QMI request = service %#x message %#x, want NAS Initiate Network Register", req.Service, req.MessageID)
			}
			action, ok := tlv.Value(req.TLVs, 0x01)
			if !ok || len(action) != 1 || action[0] != byte(qcom.NASRegisterManually) {
				t.Errorf("register action = %#v, want manual", action)
			}
			manual, ok := tlv.Value(req.TLVs, 0x10)
			if !ok {
				t.Fatal("Initiate Network Register is missing the manual registration TLV")
			}
			want := []byte{0xCC, 0x01, 0x00, 0x00, byte(qcom.NASRadioInterfaceNoChange)}
			if !bytes.Equal(manual, want) {
				t.Errorf("manual registration = %#v, want %#v", manual, want)
			}
			return successfulStatusResponse(req), nil
		},
	)

	if err := backend.Register(t.Context(), RegisterConfig{OperatorID: "46000"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestBackendRegisterTreatsNoEffectAsRegistered(t *testing.T) {
	// "No effect" means the modem already uses the requested selection, so it
	// must not fall back and must not surface an error.
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t,
			[]byte{byte(qcom.NASNetworkSelectionManual), 0xCC, 0x01, 0x00, 0x00}, nil,
			qcom.QMIErrorNoEffect),
	)

	if err := backend.Register(t.Context(), RegisterConfig{OperatorID: "46000"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestBackendRegisterPropagatesRegistrationFailure(t *testing.T) {
	backend := newRadioTestBackend(t,
		expectRegisterSelection(t,
			[]byte{byte(qcom.NASNetworkSelectionManual), 0xCC, 0x01, 0x00, 0x00}, nil,
			qcom.QMIErrorNetworkUnsupported),
	)

	err := backend.Register(t.Context(), RegisterConfig{OperatorID: "46000"})
	if !errors.Is(err, qcom.QMIErrorNetworkUnsupported) {
		t.Fatalf("Register() error = %v, want %v", err, qcom.QMIErrorNetworkUnsupported)
	}
}
