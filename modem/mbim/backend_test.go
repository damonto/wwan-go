package mbim

import (
	"context"
	"errors"
	"math"
	"net"
	"net/netip"
	"reflect"
	"testing"

	mbimproto "github.com/damonto/wwan-go/mbim"
)

func TestPopulateActiveMBIMSIMSlot(t *testing.T) {
	tests := []struct {
		name  string
		slots []SIMSlot
		sim   SIMInfo
		want  []SIMSlot
	}{
		{
			name:  "copies identity only to active slot",
			slots: []SIMSlot{{Index: 1}, {Index: 2, Active: true}},
			sim:   SIMInfo{ICCID: "8986001234567890123", EID: "89049032000000000000000000000001"},
			want:  []SIMSlot{{Index: 1}, {Index: 2, Active: true, ICCID: "8986001234567890123", EID: "89049032000000000000000000000001"}},
		},
		{
			name:  "does not guess when mapping has no active slot",
			slots: []SIMSlot{{Index: 1}, {Index: 2}},
			sim:   SIMInfo{ICCID: "8986001234567890123"},
			want:  []SIMSlot{{Index: 1}, {Index: 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			populateActiveMBIMSIMSlot(tt.slots, tt.sim)
			if !reflect.DeepEqual(tt.slots, tt.want) {
				t.Errorf("SIM slots = %+v, want %+v", tt.slots, tt.want)
			}
		})
	}
}

func TestApplyMBIMLTEServingCell(t *testing.T) {
	tests := []struct {
		name    string
		serving *mbimproto.LTEServingCell
		want    CellInfo
	}{
		{name: "missing serving cell keeps network values", want: CellInfo{OperatorID: "00101", CellID: 1, TrackingAreaCode: 2, PhysicalCellID: 3, ARFCN: 4}},
		{
			name: "serving cell supplies LTE identity and frequency",
			serving: &mbimproto.LTEServingCell{
				ProviderID: "46000", CellID: 0x12345, TAC: 0x2345, PhysicalCellID: 321, EARFCN: 38950,
			},
			want: CellInfo{OperatorID: "46000", CellID: 0x12345, TrackingAreaCode: 0x2345, PhysicalCellID: 321, ARFCN: 38950},
		},
		{
			name: "unavailable values keep network identity",
			serving: &mbimproto.LTEServingCell{
				CellID: math.MaxUint32, TAC: math.MaxUint32, PhysicalCellID: math.MaxUint32, EARFCN: math.MaxUint32,
			},
			want: CellInfo{OperatorID: "00101", CellID: 1, TrackingAreaCode: 2, PhysicalCellID: 3, ARFCN: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cell := CellInfo{OperatorID: "00101", CellID: 1, TrackingAreaCode: 2, PhysicalCellID: 3, ARFCN: 4}
			applyMBIMLTEServingCell(&cell, tt.serving)
			if !reflect.DeepEqual(cell, tt.want) {
				t.Errorf("CellInfo = %+v, want %+v", cell, tt.want)
			}
		})
	}
}

type staleSessionClient struct {
	states        map[uint32]mbimproto.ActivationState
	queryErrors   map[uint32]error
	disconnectErr error
	disconnected  []uint32
}

func (c *staleSessionClient) QueryConnect(_ context.Context, sessionID mbimproto.SessionID) (mbimproto.ConnectInfo, error) {
	id := uint32(sessionID)
	return mbimproto.ConnectInfo{ActivationState: c.states[id]}, c.queryErrors[id]
}

func (c *staleSessionClient) SetConnect(_ context.Context, cfg mbimproto.ConnectConfig) (mbimproto.ConnectInfo, error) {
	c.disconnected = append(c.disconnected, uint32(cfg.SessionID))
	return mbimproto.ConnectInfo{}, c.disconnectErr
}

func TestNetworkConfig(t *testing.T) {
	tests := []struct {
		name string
		info mbimproto.IPConfigurationInfo
		want NetworkConfig
	}{
		{
			name: "dual stack",
			info: mbimproto.IPConfigurationInfo{
				IPv4Addresses: []mbimproto.IPAddress{{IP: net.ParseIP("198.51.100.2"), PrefixLength: 25}},
				IPv6Addresses: []mbimproto.IPAddress{{IP: net.ParseIP("2001:db8:1::2"), PrefixLength: 56}},
				IPv4Gateway:   net.ParseIP("198.51.100.1"), IPv6Gateway: net.ParseIP("2001:db8:1::1"),
				IPv4DNSServers: []net.IP{net.ParseIP("8.8.8.8")}, IPv6DNSServers: []net.IP{net.ParseIP("2001:4860:4860::8844")},
				IPv4MTU: 1420, IPv6MTU: 1500,
			},
			want: NetworkConfig{Interface: "wwan1", Addresses: []netip.Prefix{
				netip.MustParsePrefix("198.51.100.2/25"), netip.MustParsePrefix("2001:db8:1::2/56"),
			}, Gateways: []netip.Addr{netip.MustParseAddr("198.51.100.1"), netip.MustParseAddr("2001:db8:1::1")},
				DNS: []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("2001:4860:4860::8844")}, MTU: 1500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mbimNetworkConfig("wwan1", tt.info); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mbimNetworkConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAPNTypeMapping(t *testing.T) {
	tests := []struct {
		name     string
		value    APNType
		want     mbimproto.ContextType
		wantBack APNType
	}{
		{name: "default", value: APNTypeDefault, want: mbimproto.ContextTypeInternet, wantBack: APNTypeDefault},
		{name: "IMS", value: APNTypeIMS, want: mbimproto.ContextTypeIMS, wantBack: APNTypeIMS},
		{name: "MMS", value: APNTypeMMS, want: mbimproto.ContextTypeMMS, wantBack: APNTypeMMS},
		{name: "tethering", value: APNTypeTethering, want: mbimproto.ContextTypeTethering, wantBack: APNTypeTethering},
		{name: "SUPL", value: APNTypeSUPL, want: mbimproto.ContextTypeSUPL, wantBack: APNTypeSUPL},
		{name: "emergency", value: APNTypeEmergency, want: mbimproto.ContextTypeEmergencyCalling, wantBack: APNTypeEmergency},
		{name: "best mask match", value: APNTypeDefault | APNTypeIMS, want: mbimproto.ContextTypeInternet, wantBack: APNTypeDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := apnTypeToMBIM(tt.value)
			if value != tt.want {
				t.Errorf("apnTypeToMBIM() = %x, want %x", value, tt.want)
			}
			if got := mbimAPNType(value); got != tt.wantBack {
				t.Errorf("mbimAPNType() = %#x, want %#x", got, tt.wantBack)
			}
		})
	}
}

func TestSessionAllocation(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "lowest free ID and requested collision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &Backend{}
			first, err := backend.reserveSessionID(3, nil)
			if err != nil || first != 0 {
				t.Fatalf("first reserve = (%d, %v), want (0, nil)", first, err)
			}
			second, err := backend.reserveSessionID(3, nil)
			if err != nil || second != 1 {
				t.Fatalf("second reserve = (%d, %v), want (1, nil)", second, err)
			}
			requested := uint32(1)
			if _, err := backend.reserveSessionID(3, &requested); err == nil {
				t.Fatal("reserving used ID error = nil, want non-nil")
			}
			backend.releaseSession(first)
			reused, err := backend.reserveSessionID(3, nil)
			if err != nil || reused != 0 {
				t.Fatalf("reused reserve = (%d, %v), want (0, nil)", reused, err)
			}
		})
	}
}

func TestCleanupStaleMBIMSessions(t *testing.T) {
	tests := []struct {
		name             string
		client           *staleSessionClient
		wantDisconnected []uint32
		wantErr          bool
	}{
		{
			name: "disconnects only confirmed active sessions",
			client: &staleSessionClient{states: map[uint32]mbimproto.ActivationState{
				0: mbimproto.ActivationStateActivated,
				1: mbimproto.ActivationStateDeactivated,
				2: mbimproto.ActivationStateActivating,
				3: mbimproto.ActivationStateDeactivating,
			}},
			wantDisconnected: []uint32{0, 2, 3},
		},
		{
			name: "ignores context not activated",
			client: &staleSessionClient{
				states:      map[uint32]mbimproto.ActivationState{},
				queryErrors: map[uint32]error{0: mbimproto.StatusContextNotActivated},
			},
		},
		{
			name: "returns query error",
			client: &staleSessionClient{
				states:      map[uint32]mbimproto.ActivationState{},
				queryErrors: map[uint32]error{0: errors.New("transport stopped")},
			},
			wantErr: true,
		},
		{
			name: "returns disconnect error",
			client: &staleSessionClient{
				states:        map[uint32]mbimproto.ActivationState{0: mbimproto.ActivationStateActivated},
				disconnectErr: errors.New("disconnect rejected"),
			},
			wantDisconnected: []uint32{0},
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cleanupStaleMBIMSessions(context.Background(), tt.client, 4)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cleanupStaleMBIMSessions() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.client.disconnected, tt.wantDisconnected) {
				t.Fatalf("disconnected sessions = %v, want %v", tt.client.disconnected, tt.wantDisconnected)
			}
		})
	}
}

func TestMBIMFeatures(t *testing.T) {
	all := mbimproto.DeviceServicesResponse{Services: []mbimproto.DeviceService{
		{ServiceID: mbimproto.ServiceBasicConnect, CIDs: []uint32{
			mbimproto.CIDProvisionedContexts, mbimproto.CIDSignalState, mbimproto.CIDPin, mbimproto.CIDPinList,
		}},
		{ServiceID: mbimproto.ServiceSMS, CIDs: []uint32{mbimproto.CIDSMSRead, mbimproto.CIDSMSSend, mbimproto.CIDSMSDelete}},
		{ServiceID: mbimproto.ServiceUSSD, CIDs: []uint32{mbimproto.CIDUSSD}},
		{ServiceID: mbimproto.ServiceMsSAR, CIDs: []uint32{mbimproto.CIDMsSARConfig}},
		{ServiceID: mbimproto.ServiceMsFirmwareID, CIDs: []uint32{mbimproto.CIDMsFirmwareIDGet}},
		{ServiceID: mbimproto.ServiceMsBasicConnectExtensions, CIDs: []uint32{
			mbimproto.CIDMsBaseStationsInfo, mbimproto.CIDMsLteAttachConfiguration, mbimproto.CIDMsLteAttachInfo,
			mbimproto.CIDMsSystemCapabilities, mbimproto.CIDDeviceSlotMappings, mbimproto.CIDMsSlotInfoStatus,
		}},
	}}
	tests := []struct {
		name     string
		services mbimproto.DeviceServicesResponse
		want     Feature
	}{
		{name: "none"},
		{
			name:     "advertised CIDs only",
			services: all,
			want: FeatureProfileManagement | FeatureSignalThresholds | FeatureFacilityLocks |
				FeatureSMS | FeatureUSSD | FeatureSAR | FeatureFirmwareUpdate | FeatureCellInfo |
				FeatureInitialEPSBearer | FeatureMultiSIM,
		},
		{
			name: "partial SMS service",
			services: mbimproto.DeviceServicesResponse{Services: []mbimproto.DeviceService{
				{ServiceID: mbimproto.ServiceSMS, CIDs: []uint32{mbimproto.CIDSMSRead, mbimproto.CIDSMSSend}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mbimFeatures(tt.services); got != tt.want {
				t.Errorf("mbimFeatures() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestSetCapabilities(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "native MBIM does not switch capabilities"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := new(Backend).SetCapabilities(context.Background(), TechnologyLTE)
			if !errors.Is(err, ErrNotSupported) {
				t.Fatalf("SetCapabilities() error = %v, want ErrNotSupported", err)
			}
		})
	}
}
