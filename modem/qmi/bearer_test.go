package qmi

import (
	"testing"

	"github.com/damonto/wwan-go/qcom"
)

func TestPDNConfigUsesDiscoveredDataPort(t *testing.T) {
	tests := []struct {
		name            string
		port            Port
		muxID           uint8
		wantSIOPort     qcom.WDSSIOPort
		wantFallback    bool
		wantMux         bool
		wantInterfaceID uint32
	}{
		{name: "generic network port"},
		{
			name: "IPA mux network port",
			port: Port{QMIEndpoint: QMIEndpoint{
				Type:            QMIEndpointEmbedded,
				InterfaceNumber: 1,
			}},
			muxID:           3,
			wantMux:         true,
			wantInterfaceID: 1,
		},
		{
			name: "BAM-DMUX network port",
			port: Port{QMIEndpoint: QMIEndpoint{
				Type:            QMIEndpointBAMDMUX,
				InterfaceNumber: 3,
				SIOPort:         0x0e07,
			}},
			wantSIOPort:     qcom.WDSSIOPortA2MuxRMNET3,
			wantFallback:    true,
			wantInterfaceID: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pdnConfig(ConnectConfig{APN: "internet", ProfileID: 7}, qcom.WDSIPPreferenceIPv4, tt.port, tt.muxID)
			if cfg.LegacyMuxDataPort != tt.wantSIOPort {
				t.Errorf("LegacyMuxDataPort = %#x, want %#x", cfg.LegacyMuxDataPort, tt.wantSIOPort)
			}
			if (cfg.MuxDataPort != nil) != tt.wantMux {
				t.Fatalf("MuxDataPort present = %v, want %v", cfg.MuxDataPort != nil, tt.wantMux)
			}
			if cfg.MuxDataPort != nil {
				endpoint := cfg.MuxDataPort.Endpoint
				if endpoint == nil || endpoint.Type != qcom.DataEndpointEmbedded || endpoint.InterfaceID != tt.wantInterfaceID {
					t.Errorf("MuxDataPort endpoint = %#v, want embedded interface %d", endpoint, tt.wantInterfaceID)
				}
				if cfg.MuxDataPort.MuxID != tt.muxID {
					t.Errorf("MuxDataPort MuxID = %d, want %d", cfg.MuxDataPort.MuxID, tt.muxID)
				}
			}
			if (cfg.LegacyMuxFallback != nil) != tt.wantFallback {
				t.Fatalf("LegacyMuxFallback present = %v, want %v", cfg.LegacyMuxFallback != nil, tt.wantFallback)
			}
			if cfg.LegacyMuxFallback != nil {
				endpoint := cfg.LegacyMuxFallback.Endpoint
				if endpoint == nil || endpoint.Type != qcom.DataEndpointBAMDMUX || endpoint.InterfaceID != tt.wantInterfaceID {
					t.Errorf("LegacyMuxFallback endpoint = %#v, want BAM-DMUX interface %d", endpoint, tt.wantInterfaceID)
				}
				if cfg.LegacyMuxFallback.MuxID != 0 {
					t.Errorf("LegacyMuxFallback MuxID = %d, want unbound 0", cfg.LegacyMuxFallback.MuxID)
				}
			}
			if cfg.APN != "internet" || cfg.ProfileIndex != 7 || cfg.IPPreference != qcom.WDSIPPreferenceIPv4 {
				t.Errorf("PDN config = %#v, want APN/profile/IP preference preserved", cfg)
			}
		})
	}
}
