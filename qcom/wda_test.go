package qcom

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestWDARequestEncoding(t *testing.T) {
	rawIP := WDALinkLayerRawIP
	qmap := WDAAggregationQMAP
	disabled := WDAAggregationDisabled
	flowControl := true
	inBandFlowControl := false
	padding := uint32(4)

	tests := []struct {
		name        string
		req         Request
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "get default data port",
			req: WDAGetDataFormatRequest{
				ClientID:      7,
				TransactionID: 9,
				Timeout:       3 * time.Second,
			}.Request(),
			wantMessage: MessageWDAGetDataFormat,
		},
		{
			name: "get BAM DMUX endpoint",
			req: WDAGetDataFormatRequest{
				ClientID:      8,
				TransactionID: 10,
				Endpoint: &DataEndpoint{
					Type:        DataEndpointBAMDMUX,
					InterfaceID: 1,
				},
			}.Request(),
			wantMessage: MessageWDAGetDataFormat,
			wantTLVs: map[byte][]byte{
				wdaTLVGetEndpoint: {0x05, 0, 0, 0, 1, 0, 0, 0},
			},
		},
		{
			name: "set raw IP",
			req: WDASetDataFormatRequest{
				ClientID:      9,
				TransactionID: 11,
				Config: WDADataFormatConfig{
					LinkLayerProtocol: &rawIP,
				},
			}.Request(),
			wantMessage: MessageWDASetDataFormat,
			wantTLVs: map[byte][]byte{
				wdaTLVLinkProtocol: {0x02, 0, 0, 0},
			},
		},
		{
			name: "set QMAP aggregation",
			req: WDASetDataFormatRequest{
				Config: WDADataFormatConfig{
					UplinkAggregation:            &qmap,
					DownlinkAggregation:          &disabled,
					DownlinkMinimumPadding:       &padding,
					TerminalEquipmentFlowControl: &flowControl,
				},
			}.Request(),
			wantMessage: MessageWDASetDataFormat,
			wantTLVs: map[byte][]byte{
				wdaTLVUplinkAggregation:   {0x05, 0, 0, 0},
				wdaTLVDownlinkAggregation: {0, 0, 0, 0},
				wdaTLVSetDownlinkPadding:  {4, 0, 0, 0},
				wdaTLVSetFlowControl:      {1},
			},
		},
		{
			name: "get QMAP settings for endpoint",
			req: WDAGetQMAPSettingsRequest{
				ClientID:      7,
				TransactionID: 12,
				Endpoint: &DataEndpoint{
					Type:        DataEndpointPCIe,
					InterfaceID: 2,
				},
			}.Request(),
			wantMessage: MessageWDAGetQMAPSettings,
			wantTLVs: map[byte][]byte{
				wdaTLVQMAPGetEndpoint: {0x03, 0, 0, 0, 2, 0, 0, 0},
			},
		},
		{
			name: "set QMAP flow control",
			req: WDASetQMAPSettingsRequest{
				ClientID:      7,
				TransactionID: 13,
				Config: WDAQMAPSettingsConfig{
					InBandFlowControl: &inBandFlowControl,
					DataFlowControl:   &flowControl,
				},
			}.Request(),
			wantMessage: MessageWDASetQMAPSettings,
			wantTLVs: map[byte][]byte{
				wdaTLVQMAPInBandFlowControl: {0},
				wdaTLVQMAPDataFlowControl:   {1},
			},
		},
		{
			name: "get Ethernet config",
			req: WDAGetEthernetConfigRequest{
				ClientID:      7,
				TransactionID: 14,
				Timeout:       3 * time.Second,
			}.Request(),
			wantMessage: MessageWDAGetEthernetConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Service != ServiceWDA {
				t.Fatalf("Service = 0x%02X, want 0x%02X", tt.req.Service, ServiceWDA)
			}
			if tt.req.MessageID != tt.wantMessage {
				t.Fatalf("MessageID = 0x%04X, want 0x%04X", tt.req.MessageID, tt.wantMessage)
			}
			if len(tt.req.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(tt.req.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				got, ok := tlv.Value(tt.req.TLVs, kind)
				if !ok {
					t.Fatalf("TLV 0x%02X missing", kind)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("TLV 0x%02X = % X, want % X", kind, got, want)
				}
			}
		})
	}
}

func TestWDAEthernetConfigResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDAEthernetConfigResponse
		wantErr bool
	}{
		{
			name: "VLAN IP",
			tlvs: tlv.TLVs{tlv.Uint(0x10, uint32(WDAEthernetHardwareVLANIP))},
			want: WDAEthernetConfigResponse{Config: WDAEthernetHardwareVLANIP, ConfigKnown: true},
		},
		{name: "optional field absent"},
		{name: "hardware config truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0, 0})}, wantErr: true},
		{name: "hardware config trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0, 0, 0, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDAEthernetConfigResponse
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
				t.Fatalf("response = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWDAEthernetHardwareConfig(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		want    WDAEthernetHardwareConfig
		wantErr bool
	}{
		{
			name: "non-VLAN IP",
			resp: successResponse(MessageWDAGetEthernetConfig, tlv.Uint(0x10, uint32(WDAEthernetHardwareNonVLANIP))),
			want: WDAEthernetHardwareNonVLANIP,
		},
		{name: "hardware config missing", resp: successResponse(MessageWDAGetEthernetConfig), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceWDA || req.ClientID != 5 || req.MessageID != MessageWDAGetEthernetConfig {
						t.Fatalf("request = service 0x%X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceWDA: 5}}

			got, err := client.WDAEthernetHardwareConfig(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("WDAEthernetHardwareConfig() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("WDAEthernetHardwareConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("WDAEthernetHardwareConfig() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWDADataFormatResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		check   func(*testing.T, WDADataFormat)
		wantErr bool
	}{
		{
			name: "raw IP and QMAP",
			tlvs: tlv.TLVs{
				tlv.Uint(wdaTLVQoSFormat, uint8(0)),
				tlv.Uint(wdaTLVLinkProtocol, uint32(WDALinkLayerRawIP)),
				tlv.Uint(wdaTLVUplinkAggregation, uint32(WDAAggregationQMAPv5)),
				tlv.Uint(wdaTLVDownlinkAggregation, uint32(WDAAggregationQMAPv5)),
				tlv.Uint(wdaTLVDownlinkMaxDatagrams, uint32(32)),
				tlv.Uint(wdaTLVDownlinkMaxSize, uint32(32768)),
				tlv.Uint(wdaTLVUplinkMaxDatagrams, uint32(16)),
				tlv.Uint(wdaTLVUplinkMaxSize, uint32(16384)),
				tlv.Uint(wdaTLVResponseQoSHeader, uint32(WDAQoSHeader6Bytes)),
				tlv.Uint(wdaTLVResponseDownlinkPadding, uint32(4)),
				tlv.Uint(wdaTLVResponseFlowControl, uint8(1)),
			},
			check: func(t *testing.T, got WDADataFormat) {
				if !got.LinkLayerProtocolKnown || got.LinkLayerProtocol != WDALinkLayerRawIP {
					t.Fatalf("LinkLayerProtocol = %d, known %v", got.LinkLayerProtocol, got.LinkLayerProtocolKnown)
				}
				if !got.UplinkAggregationKnown || got.UplinkAggregation != WDAAggregationQMAPv5 {
					t.Fatalf("UplinkAggregation = %d, known %v", got.UplinkAggregation, got.UplinkAggregationKnown)
				}
				if !got.DownlinkMaxSizeKnown || got.DownlinkMaxSize != 32768 {
					t.Fatalf("DownlinkMaxSize = %d, known %v", got.DownlinkMaxSize, got.DownlinkMaxSizeKnown)
				}
				if !got.UplinkMaxDatagramsKnown || got.UplinkMaxDatagrams != 16 {
					t.Fatalf("UplinkMaxDatagrams = %d, known %v", got.UplinkMaxDatagrams, got.UplinkMaxDatagramsKnown)
				}
				if !got.TerminalEquipmentFlowControlKnown || !got.TerminalEquipmentFlowControl {
					t.Fatalf("TerminalEquipmentFlowControl = %v, known %v", got.TerminalEquipmentFlowControl, got.TerminalEquipmentFlowControlKnown)
				}
			},
		},
		{
			name: "optional fields absent",
			check: func(t *testing.T, got WDADataFormat) {
				if got.LinkLayerProtocolKnown || got.QoSEnabledKnown {
					t.Fatalf("known flags set for empty response: %+v", got)
				}
			},
		},
		{
			name:    "truncated link protocol",
			tlvs:    tlv.TLVs{tlv.Bytes(wdaTLVLinkProtocol, []byte{2})},
			wantErr: true,
		},
		{
			name:    "truncated flow control",
			tlvs:    tlv.TLVs{tlv.Bytes(wdaTLVResponseFlowControl, nil)},
			wantErr: true,
		},
		{
			name:    "QoS format trailing byte",
			tlvs:    tlv.TLVs{tlv.Bytes(wdaTLVQoSFormat, []byte{1, 0})},
			wantErr: true,
		},
		{
			name:    "uint32 trailing byte",
			tlvs:    tlv.TLVs{tlv.Bytes(wdaTLVLinkProtocol, make([]byte, 5))},
			wantErr: true,
		},
		{
			name:    "flow control trailing byte",
			tlvs:    tlv.TLVs{tlv.Bytes(wdaTLVResponseFlowControl, []byte{1, 0})},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response WDADataFormatResponse
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
			tt.check(t, response.Format)
		})
	}
}

func TestWDAQMAPSettingsResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    WDAQMAPSettings
		wantErr bool
	}{
		{
			name: "both settings",
			tlvs: tlv.TLVs{
				tlv.Uint(wdaTLVQMAPInBandFlowControl, uint8(0)),
				tlv.Uint(wdaTLVQMAPResponseDataFlow, uint8(1)),
			},
			want: WDAQMAPSettings{
				InBandFlowControlKnown: true,
				DataFlowControl:        true,
				DataFlowControlKnown:   true,
			},
		},
		{name: "optional fields absent"},
		{name: "truncated in-band", tlvs: tlv.TLVs{tlv.Bytes(wdaTLVQMAPInBandFlowControl, nil)}, wantErr: true},
		{name: "truncated data flow", tlvs: tlv.TLVs{tlv.Bytes(wdaTLVQMAPResponseDataFlow, nil)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response WDAQMAPSettingsResponse
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
			if response.Settings != tt.want {
				t.Fatalf("Settings = %+v, want %+v", response.Settings, tt.want)
			}
		})
	}
}

func TestSetWDALinkLayerProtocolUsesWDA(t *testing.T) {
	tests := []struct {
		name     string
		protocol WDALinkLayerProtocol
	}{
		{name: "raw IP", protocol: WDALinkLayerRawIP},
		{name: "Ethernet", protocol: WDALinkLayerEthernet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{
				t: t,
				calls: []transportCall{
					{
						check: func(req Request) {
							assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDA)})
						},
						resp: successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(ServiceWDA), 5})),
					},
					{
						check: func(req Request) {
							if req.Service != ServiceWDA || req.ClientID != 5 || req.MessageID != MessageWDASetDataFormat {
								t.Fatalf("WDA request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
							}
							assertTLV(t, req.TLVs, wdaTLVLinkProtocol, uint32ValueForTest(uint32(tt.protocol)))
						},
						resp: successResponse(MessageWDASetDataFormat, tlv.Uint(wdaTLVLinkProtocol, uint32(tt.protocol))),
					},
					{
						check: func(req Request) {
							assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDA), 5})
						},
						resp: successResponse(MessageReleaseClientID),
					},
				},
			}
			reader := &Client{transport: transport, slot: 1}

			if err := reader.SetWDALinkLayerProtocol(context.Background(), tt.protocol); err != nil {
				t.Fatalf("SetWDALinkLayerProtocol() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := transport.callCount(); got != 3 {
				t.Fatalf("Do() calls = %d, want 3", got)
			}
		})
	}
}
