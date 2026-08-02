package qcom

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestOpenIMSPDNNormalizesAPN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     IMSPDNConfig
		wantAPN string
	}{
		{name: "default", wantAPN: DefaultIMSPDNAPN},
		{name: "trimmed", cfg: IMSPDNConfig{APN: " ims "}, wantAPN: DefaultIMSPDNAPN},
		{name: "custom", cfg: IMSPDNConfig{APN: " sos "}, wantAPN: "sos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{
					check: func(req Request) {
						if req.MessageID != MessageWDSStartNetworkInterface {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageWDSStartNetworkInterface)
						}
						got, ok := tlv.Value(req.TLVs, 0x14)
						if !ok {
							t.Fatal("APN TLV missing")
						}
						if !bytes.Equal(got, []byte(tt.wantAPN)) {
							t.Fatalf("APN = %q, want %q", got, tt.wantAPN)
						}
						callType, ok := tlv.Value(req.TLVs, 0x35)
						if !ok || !bytes.Equal(callType, []byte{byte(WDSCallTypeEmbedded)}) {
							t.Fatalf("Call type = % X, want embedded", callType)
						}
					},
					resp: successResponse(MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{resp: successResponse(MessageWDSGetRuntimeSettings)},
				{resp: allocatedClientResponse(ServiceNAS, 3)},
				{resp: successResponse(MessageNASGetSysInfo, tlv.Bytes(0x29, []byte{1}))},
				{resp: successResponse(MessageWDSStopNetworkInterface)},
				{resp: successResponse(MessageReleaseClientID)},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			session, err := reader.OpenIMSPDN(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNNegotiatesNetworkIPFamilyRestriction(t *testing.T) {
	tests := []struct {
		name              string
		initialPreference WDSIPPreference
		reason            int16
		retryPreference   WDSIPPreference
		family            WDSIPFamily
	}{
		{
			name:              "default negotiates IPv4 only",
			initialPreference: WDSIPPreferenceDefault,
			reason:            WDSVerboseCallEndReason3GPPIPv4OnlyAllowed,
			retryPreference:   WDSIPPreferenceIPv4,
			family:            WDSIPFamilyIPv4,
		},
		{
			name:              "default negotiates IPv6 only",
			initialPreference: WDSIPPreferenceDefault,
			reason:            WDSVerboseCallEndReason3GPPIPv6OnlyAllowed,
			retryPreference:   WDSIPPreferenceIPv6,
			family:            WDSIPFamilyIPv6,
		},
		{
			name:              "unspecified negotiates IPv4 only",
			initialPreference: WDSIPPreferenceUnspecified,
			reason:            WDSVerboseCallEndReason3GPPIPv4OnlyAllowed,
			retryPreference:   WDSIPPreferenceIPv4,
			family:            WDSIPFamilyIPv4,
		},
		{
			name:              "unspecified negotiates IPv6 only",
			initialPreference: WDSIPPreferenceUnspecified,
			reason:            WDSVerboseCallEndReason3GPPIPv6OnlyAllowed,
			retryPreference:   WDSIPPreferenceIPv6,
			family:            WDSIPFamilyIPv6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{resp: successResponse(MessageWDSBindMuxDataPort)},
				{
					check: func(req Request) {
						if tt.initialPreference == WDSIPPreferenceDefault {
							if _, ok := tlv.Value(req.TLVs, 0x19); ok {
								t.Fatal("default WDS request included an IP Family TLV")
							}
							return
						}
						assertTLV(t, req.TLVs, 0x19, []byte{byte(tt.initialPreference)})
					},
					resp: errorResponse(
						MessageWDSStartNetworkInterface,
						QMIErrorCallFailed,
						tlv.Bytes(0x11, verboseCallEndReasonForTest(WDSVerboseCallEndReasonType3GPP, tt.reason)),
					),
				},
				{resp: successResponse(MessageReleaseClientID)},
				{resp: allocatedClientResponse(ServiceWDS, 3)},
				{
					check: func(req Request) {
						if req.MessageID != MessageWDSSetClientIPFamily {
							t.Fatalf("MessageID = 0x%04X, want Set Client IP Family", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.family)})
					},
					resp: successResponse(MessageWDSSetClientIPFamily),
				},
				{resp: successResponse(MessageWDSBindMuxDataPort)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x19, []byte{byte(tt.retryPreference)})
					},
					resp: successResponse(MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{resp: successResponse(MessageWDSGetRuntimeSettings, tlv.Bytes(0x2B, []byte{byte(tt.family)}))},
				{resp: allocatedClientResponse(ServiceNAS, 4)},
				{resp: successResponse(MessageNASGetSysInfo, tlv.Bytes(0x29, []byte{1}))},
				{resp: successResponse(MessageWDSStopNetworkInterface)},
				{resp: successResponse(MessageReleaseClientID)},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			session, err := reader.OpenIMSPDN(context.Background(), IMSPDNConfig{
				APN:          "ims",
				IPPreference: tt.initialPreference,
				MuxDataPort:  &WDSMuxDataPort{MuxID: 2},
			})
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			if got := session.Info().IPFamily; got != tt.family {
				t.Fatalf("IPFamily = %d, want %d", got, tt.family)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNKeepsExplicitIPPreference(t *testing.T) {
	tests := []struct {
		name        string
		preference  WDSIPPreference
		family      WDSIPFamily
		reason      int16
		restriction error
	}{
		{
			name:        "IPv4",
			preference:  WDSIPPreferenceIPv4,
			family:      WDSIPFamilyIPv4,
			reason:      WDSVerboseCallEndReason3GPPIPv6OnlyAllowed,
			restriction: ErrWDSIPv6Only,
		},
		{
			name:        "IPv6",
			preference:  WDSIPPreferenceIPv6,
			family:      WDSIPFamilyIPv6,
			reason:      WDSVerboseCallEndReason3GPPIPv4OnlyAllowed,
			restriction: ErrWDSIPv4Only,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{
					check: func(req Request) {
						if req.MessageID != MessageWDSSetClientIPFamily {
							t.Fatalf("MessageID = 0x%04X, want Set Client IP Family", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.family)})
					},
					resp: successResponse(MessageWDSSetClientIPFamily),
				},
				{resp: successResponse(MessageWDSBindMuxDataPort)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x19, []byte{byte(tt.preference)})
					},
					resp: errorResponse(
						MessageWDSStartNetworkInterface,
						QMIErrorCallFailed,
						tlv.Bytes(0x11, verboseCallEndReasonForTest(WDSVerboseCallEndReasonType3GPP, tt.reason)),
					),
				},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = reader.OpenIMSPDN(context.Background(), IMSPDNConfig{
				IPPreference: tt.preference,
				MuxDataPort:  &WDSMuxDataPort{MuxID: 2},
			})
			if !errors.Is(err, tt.restriction) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, tt.restriction)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
		})
	}
}

func TestOpenIMSPDNReleasesBothWDSClientsWhenNegotiationFails(t *testing.T) {
	tests := []struct {
		name        string
		reason      int16
		preference  WDSIPPreference
		family      WDSIPFamily
		restriction error
	}{
		{
			name:        "IPv4 only",
			reason:      WDSVerboseCallEndReason3GPPIPv4OnlyAllowed,
			preference:  WDSIPPreferenceIPv4,
			family:      WDSIPFamilyIPv4,
			restriction: ErrWDSIPv4Only,
		},
		{
			name:        "IPv6 only",
			reason:      WDSVerboseCallEndReason3GPPIPv6OnlyAllowed,
			preference:  WDSIPPreferenceIPv6,
			family:      WDSIPFamilyIPv6,
			restriction: ErrWDSIPv6Only,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{resp: successResponse(MessageWDSBindMuxDataPort)},
				{
					resp: errorResponse(
						MessageWDSStartNetworkInterface,
						QMIErrorCallFailed,
						tlv.Bytes(0x11, verboseCallEndReasonForTest(WDSVerboseCallEndReasonType3GPP, tt.reason)),
					),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDS), 2})
					},
					resp: successResponse(MessageReleaseClientID),
				},
				{resp: allocatedClientResponse(ServiceWDS, 3)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.family)})
					},
					resp: successResponse(MessageWDSSetClientIPFamily),
				},
				{resp: successResponse(MessageWDSBindMuxDataPort)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x19, []byte{byte(tt.preference)})
					},
					resp: errorResponse(MessageWDSStartNetworkInterface, QMIErrorInternal),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDS), 3})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = reader.OpenIMSPDN(context.Background(), IMSPDNConfig{
				IPPreference: WDSIPPreferenceUnspecified,
				MuxDataPort:  &WDSMuxDataPort{MuxID: 2},
			})
			if !errors.Is(err, tt.restriction) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, tt.restriction)
			}
			if !errors.Is(err, QMIErrorInternal) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, QMIErrorInternal)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
		})
	}
}

func TestOpenPDNUsesOnlyWDSAndOmitsOptionalDefaults(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "network defaults"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{
					check: func(req Request) {
						if req.Service != ServiceWDS || req.MessageID != MessageWDSStartNetworkInterface {
							t.Fatalf("start request = service 0x%02X message 0x%04X", req.Service, req.MessageID)
						}
						for _, kind := range []byte{0x14, 0x19, 0x30, 0x35} {
							if _, ok := tlv.Value(req.TLVs, kind); ok {
								t.Fatalf("optional TLV 0x%02X unexpectedly present", kind)
							}
						}
					},
					resp: successResponse(MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x10, uint32ValueForTest(uint32(WDSRuntimeRequestedNetworkSettings)))
					},
					resp: successResponse(MessageWDSGetRuntimeSettings,
						tlv.Bytes(0x1E, []byte{2, 0, 0, 10}),
						tlv.Bytes(0x2B, []byte{byte(WDSIPFamilyIPv4)}),
					),
				},
				{resp: successResponse(MessageWDSStopNetworkInterface)},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{byte(ServiceWDS), 2})
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}
			reader := &Client{transport: transport, slot: 1}

			session, err := reader.OpenPDN(context.Background(), PDNConfig{})
			if err != nil {
				t.Fatalf("OpenPDN() error = %v", err)
			}
			info := session.Info()
			if !info.LocalIPv4.Equal(net.IPv4(10, 0, 0, 2)) || info.IPFamily != WDSIPFamilyIPv4 || !info.PacketDataReady {
				t.Fatalf("Info() = %+v", info)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNBindsDataPortBeforeStartingNetwork(t *testing.T) {
	tests := []struct {
		name          string
		cfg           IMSPDNConfig
		wantMessageID MessageID
		wantTLVs      tlv.TLVs
	}{
		{
			name: "mux data port",
			cfg: IMSPDNConfig{MuxDataPort: &WDSMuxDataPort{
				Endpoint: &DataEndpoint{Type: DataEndpointBAMDMUX, InterfaceID: 1},
				MuxID:    2,
			}},
			wantMessageID: MessageWDSBindMuxDataPort,
			wantTLVs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0x05, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}),
				tlv.Bytes(0x11, []byte{0x02}),
			},
		},
		{
			name:          "legacy mux data port",
			cfg:           IMSPDNConfig{LegacyMuxDataPort: WDSSIOPortA2MuxRMNET1},
			wantMessageID: MessageWDSLegacyBindMuxDataPort,
			wantTLVs:      tlv.TLVs{tlv.Bytes(0x01, []byte{0x05, 0x0E})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{
					check: func(req Request) {
						if req.Service != ServiceWDS {
							t.Fatalf("bind Service = 0x%02X, want 0x%02X", req.Service, ServiceWDS)
						}
						if req.ClientID != 2 {
							t.Fatalf("bind ClientID = %d, want 2", req.ClientID)
						}
						if req.MessageID != tt.wantMessageID {
							t.Fatalf("bind MessageID = 0x%04X, want 0x%04X", req.MessageID, tt.wantMessageID)
						}
						for _, want := range tt.wantTLVs {
							got, ok := tlv.Value(req.TLVs, want.Type)
							if !ok || !bytes.Equal(got, want.Value) {
								t.Fatalf("bind TLV 0x%02X = % X, want % X", want.Type, got, want.Value)
							}
						}
					},
					resp: successResponse(tt.wantMessageID),
				},
				{
					check: func(req Request) {
						if req.MessageID != MessageWDSStartNetworkInterface {
							t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, MessageWDSStartNetworkInterface)
						}
						if req.ClientID != 2 {
							t.Fatalf("start ClientID = %d, want bound WDS ClientID 2", req.ClientID)
						}
					},
					resp: successResponse(MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{resp: successResponse(MessageWDSGetRuntimeSettings)},
				{resp: allocatedClientResponse(ServiceNAS, 3)},
				{resp: successResponse(MessageNASGetSysInfo, tlv.Bytes(0x29, []byte{1}))},
				{resp: successResponse(MessageWDSStopNetworkInterface)},
				{resp: successResponse(MessageReleaseClientID)},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			session, err := reader.OpenIMSPDN(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNLegacyMuxUsesProfileIPFamily(t *testing.T) {
	tests := []struct {
		name       string
		preference WDSIPPreference
	}{
		{name: "IPv4 preference ignored", preference: WDSIPPreferenceIPv4},
		{name: "IPv6 preference ignored", preference: WDSIPPreferenceIPv6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{resp: successResponse(MessageWDSLegacyBindMuxDataPort)},
				{
					check: func(req Request) {
						if req.MessageID != MessageWDSStartNetworkInterface {
							t.Fatalf("MessageID = 0x%04X, want Start Network", req.MessageID)
						}
						if _, ok := tlv.Value(req.TLVs, 0x19); ok {
							t.Fatal("IP family TLV unexpectedly present")
						}
					},
					resp: successResponse(MessageWDSStartNetworkInterface, tlv.Uint(0x01, uint32(0x01020304))),
				},
				{resp: successResponse(MessageWDSGetRuntimeSettings)},
				{resp: allocatedClientResponse(ServiceNAS, 3)},
				{resp: successResponse(MessageNASGetSysInfo, tlv.Bytes(0x29, []byte{1}))},
				{resp: successResponse(MessageWDSStopNetworkInterface)},
				{resp: successResponse(MessageReleaseClientID)},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			session, err := reader.OpenIMSPDN(context.Background(), IMSPDNConfig{
				IPPreference:      tt.preference,
				LegacyMuxDataPort: WDSSIOPortA2MuxRMNET0,
			})
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("session.Close() error = %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
		})
	}
}

func TestOpenIMSPDNLegacyMuxKeepsNetworkIPFamilyRestriction(t *testing.T) {
	tests := []struct {
		name        string
		reason      int16
		restriction error
	}{
		{
			name:        "IPv4 only",
			reason:      WDSVerboseCallEndReason3GPPIPv4OnlyAllowed,
			restriction: ErrWDSIPv4Only,
		},
		{
			name:        "IPv6 only",
			reason:      WDSVerboseCallEndReason3GPPIPv6OnlyAllowed,
			restriction: ErrWDSIPv6Only,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{resp: successResponse(MessageWDSLegacyBindMuxDataPort)},
				{
					check: func(req Request) {
						if _, ok := tlv.Value(req.TLVs, 0x19); ok {
							t.Fatal("legacy mux WDS request included an IP Family TLV")
						}
					},
					resp: errorResponse(
						MessageWDSStartNetworkInterface,
						QMIErrorCallFailed,
						tlv.Bytes(0x11, verboseCallEndReasonForTest(WDSVerboseCallEndReasonType3GPP, tt.reason)),
					),
				},
				{resp: successResponse(MessageReleaseClientID)},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = reader.OpenIMSPDN(context.Background(), IMSPDNConfig{
				IPPreference:      WDSIPPreferenceDefault,
				LegacyMuxDataPort: WDSSIOPortA2MuxRMNET0,
			})
			if !errors.Is(err, tt.restriction) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, tt.restriction)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNBindDataPortFailureReleasesWDSClient(t *testing.T) {
	tests := []struct {
		name          string
		cfg           IMSPDNConfig
		wantMessageID MessageID
	}{
		{
			name:          "mux data port",
			cfg:           IMSPDNConfig{MuxDataPort: &WDSMuxDataPort{MuxID: 1}},
			wantMessageID: MessageWDSBindMuxDataPort,
		},
		{
			name:          "legacy mux data port",
			cfg:           IMSPDNConfig{LegacyMuxDataPort: WDSSIOPortA2MuxRMNET0},
			wantMessageID: MessageWDSLegacyBindMuxDataPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{
				{resp: allocatedClientResponse(ServiceWDS, 2)},
				{resp: errorResponse(tt.wantMessageID, QMIErrorNotSupported)},
				{
					check: func(req Request) {
						if req.MessageID != MessageReleaseClientID {
							t.Fatalf("MessageID = 0x%04X, want release client", req.MessageID)
						}
						got, ok := tlv.Value(req.TLVs, 0x01)
						if !ok || !bytes.Equal(got, []byte{byte(ServiceWDS), 2}) {
							t.Fatalf("released client = % X, want WDS client 2", got)
						}
					},
					resp: successResponse(MessageReleaseClientID),
				},
			}}

			reader, err := NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = reader.OpenIMSPDN(context.Background(), tt.cfg)
			if !errors.Is(err, QMIErrorNotSupported) {
				t.Fatalf("OpenIMSPDN() error = %v, want %v", err, QMIErrorNotSupported)
			}
			if err := reader.Close(); err != nil {
				t.Fatalf("reader.Close() error = %v", err)
			}
			if got := transport.callCount(); got != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(transport.calls))
			}
		})
	}
}

func TestOpenIMSPDNRejectsConflictingDataPorts(t *testing.T) {
	tests := []struct {
		name string
		cfg  IMSPDNConfig
		want string
	}{
		{
			name: "modern and legacy",
			cfg: IMSPDNConfig{
				MuxDataPort:       &WDSMuxDataPort{MuxID: 1},
				LegacyMuxDataPort: WDSSIOPortA2MuxRMNET0,
			},
			want: "opening IMS PDN: mux data port and legacy mux data port are mutually exclusive",
		},
		{
			name: "fallback without legacy",
			cfg: IMSPDNConfig{
				LegacyMuxFallback: &WDSMuxDataPort{},
			},
			want: "opening IMS PDN: legacy mux fallback requires a legacy mux data port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Client{}).OpenIMSPDN(context.Background(), tt.cfg)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("OpenIMSPDN() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func allocatedClientResponse(service ServiceType, clientID uint8) Response {
	return successResponse(MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(service), clientID}))
}
