package usim_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
	"github.com/damonto/uicc-go/qcom/uim"
	"github.com/damonto/uicc-go/usim"
)

type imsPDNTransport struct {
	startAPN    string
	startFamily qcom.WDSIPFamily
	bindMessage qcom.MessageID
	stopped     bool
	released    []qcom.ServiceType
	nextClient  uint8
}

func (t *imsPDNTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	switch req.MessageID {
	case qcom.MessageAllocateClientID:
		service, err := serviceFromAllocateRequest(req.TLVs)
		if err != nil {
			return qcom.Response{}, err
		}
		t.nextClient++
		return imsSuccessResponse(req, tlv.Bytes(0x01, []byte{byte(service), t.nextClient})), nil
	case qcom.MessageReleaseClientID:
		service, err := serviceFromReleaseRequest(req.TLVs)
		if err != nil {
			return qcom.Response{}, err
		}
		t.released = append(t.released, service)
		return imsSuccessResponse(req), nil
	case qcom.MessageWDSStartNetworkInterface:
		apn, _ := tlv.Value(req.TLVs, 0x14)
		family, _ := tlv.Value(req.TLVs, 0x19)
		t.startAPN = string(apn)
		if len(family) > 0 {
			t.startFamily = qcom.WDSIPFamily(family[0])
		}
		return imsSuccessResponse(req, tlv.Uint(0x01, uint32(0x01020304))), nil
	case qcom.MessageWDSBindMuxDataPort, qcom.MessageWDSLegacyBindMuxDataPort:
		t.bindMessage = req.MessageID
		return imsSuccessResponse(req), nil
	case qcom.MessageWDSGetRuntimeSettings:
		localIPv6 := net.ParseIP("2001:db8::2").To16()
		pcscfIPv6 := net.ParseIP("2001:db8::1").To16()
		return imsSuccessResponse(req,
			tlv.Bytes(0x1E, []byte{2, 0, 0, 10}),
			tlv.Bytes(0x25, append(append([]byte(nil), localIPv6...), 64)),
			tlv.Bytes(0x23, ipv4ListForTest(net.IPv4(198, 51, 100, 10))),
			tlv.Bytes(0x2E, ipv6ListForTest(pcscfIPv6)),
			tlv.Bytes(0x2B, []byte{byte(qcom.WDSIPFamilyIPv4v6)}),
			tlv.Bytes(0x2C, []byte{1}),
		), nil
	case qcom.MessageWDSStopNetworkInterface:
		t.stopped = true
		return imsSuccessResponse(req), nil
	case qcom.MessageNASGetSysInfo:
		return imsSuccessResponse(req, tlv.Bytes(0x29, []byte{1})), nil
	default:
		return qcom.Response{}, fmt.Errorf("unexpected QMI message 0x%04X", req.MessageID)
	}
}

func (t *imsPDNTransport) Close() error {
	return nil
}

func TestQCOMOpenIMSPDN(t *testing.T) {
	tests := []struct {
		name       string
		cfg        usim.IMSPDNConfig
		wantAPN    string
		wantFamily qcom.WDSIPFamily
		wantBind   qcom.MessageID
	}{
		{
			name:       "defaults",
			wantAPN:    usim.DefaultIMSPDNAPN,
			wantFamily: qcom.WDSIPFamilyIPv4v6,
		},
		{
			name: "trims and normalizes",
			cfg: usim.IMSPDNConfig{
				APN:            " ims ",
				PDNType:        " IPV6 ",
				RequestTimeout: 3 * time.Second,
			},
			wantAPN:    "ims",
			wantFamily: qcom.WDSIPFamilyIPv6,
		},
		{
			name:       "modern mux data port",
			cfg:        usim.IMSPDNConfig{MuxDataPort: &qcom.WDSMuxDataPort{MuxID: 2}},
			wantAPN:    usim.DefaultIMSPDNAPN,
			wantFamily: qcom.WDSIPFamilyIPv4v6,
			wantBind:   qcom.MessageWDSBindMuxDataPort,
		},
		{
			name:       "legacy mux data port",
			cfg:        usim.IMSPDNConfig{LegacyMuxDataPort: qcom.WDSSIOPortA2MuxRMNET1},
			wantAPN:    usim.DefaultIMSPDNAPN,
			wantFamily: qcom.WDSIPFamilyIPv4v6,
			wantBind:   qcom.MessageWDSLegacyBindMuxDataPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &imsPDNTransport{}
			reader, err := uim.New(context.Background(), transport)
			if err != nil {
				t.Fatalf("uim.New() error = %v", err)
			}
			adapter, err := usim.NewQCOM(reader)
			if err != nil {
				t.Fatalf("NewQCOM() error = %v", err)
			}

			session, err := adapter.OpenIMSPDN(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			info := session.Info()
			if transport.startAPN != tt.wantAPN {
				t.Fatalf("start APN = %q, want %q", transport.startAPN, tt.wantAPN)
			}
			if transport.startFamily != tt.wantFamily {
				t.Fatalf("start family = %d, want %d", transport.startFamily, tt.wantFamily)
			}
			if transport.bindMessage != tt.wantBind {
				t.Fatalf("bind message = 0x%04X, want 0x%04X", transport.bindMessage, tt.wantBind)
			}
			if !info.LocalIPv4.Equal(net.IPv4(10, 0, 0, 2)) {
				t.Fatalf("LocalIPv4 = %v, want 10.0.0.2", info.LocalIPv4)
			}
			if len(info.PCSCFIPs) != 2 {
				t.Fatalf("PCSCFIPs len = %d, want 2", len(info.PCSCFIPs))
			}
			if !info.VoPSSupported {
				t.Fatal("VoPSSupported = false, want true")
			}
			if !info.PacketDataReady {
				t.Fatal("PacketDataReady = false, want true")
			}
			if err := session.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if !transport.stopped {
				t.Fatal("WDS stop network was not sent")
			}
			if !serviceReleased(transport.released, qcom.ServiceWDS) || !serviceReleased(transport.released, qcom.ServiceNAS) {
				t.Fatalf("released services = %v, want WDS and NAS", transport.released)
			}
		})
	}
}

func TestQCOMOpenIMSPDNRejectsUnsupportedPDNType(t *testing.T) {
	tests := []struct {
		name string
		cfg  usim.IMSPDNConfig
	}{
		{name: "invalid pdn type", cfg: usim.IMSPDNConfig{PDNType: "ipv5"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := uim.New(context.Background(), &imsPDNTransport{})
			if err != nil {
				t.Fatalf("uim.New() error = %v", err)
			}
			adapter, err := usim.NewQCOM(reader)
			if err != nil {
				t.Fatalf("NewQCOM() error = %v", err)
			}
			if _, err := adapter.OpenIMSPDN(context.Background(), tt.cfg); err == nil {
				t.Fatal("OpenIMSPDN() error = nil, want non-nil")
			}
		})
	}
}

func imsSuccessResponse(req qcom.Request, values ...tlv.TLV) qcom.Response {
	return qcom.Response{
		Service:   req.Service,
		ClientID:  req.ClientID,
		MessageID: req.MessageID,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(0x02, []byte{0x00, 0x00, 0x00, 0x00}),
		}, values...),
	}
}

func serviceFromAllocateRequest(tlvs tlv.TLVs) (qcom.ServiceType, error) {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) == 0 {
		return 0, fmt.Errorf("allocation service TLV missing")
	}
	return qcom.ServiceType(value[0]), nil
}

func serviceFromReleaseRequest(tlvs tlv.TLVs) (qcom.ServiceType, error) {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok || len(value) < 2 {
		return 0, fmt.Errorf("release service TLV missing")
	}
	return qcom.ServiceType(value[0]), nil
}

func serviceReleased(services []qcom.ServiceType, want qcom.ServiceType) bool {
	for _, service := range services {
		if service == want {
			return true
		}
	}
	return false
}

func ipv4ListForTest(ip net.IP) []byte {
	out := make([]byte, 5)
	out[0] = 1
	v4 := ip.To4()
	out[1], out[2], out[3], out[4] = v4[3], v4[2], v4[1], v4[0]
	return out
}

func ipv6ListForTest(ip net.IP) []byte {
	out := make([]byte, 17)
	out[0] = 1
	copy(out[1:], ip.To16())
	return out
}

func TestIMSPDNSessionInfoClonesAddresses(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "info clone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &imsPDNTransport{}
			reader, err := uim.New(context.Background(), transport)
			if err != nil {
				t.Fatalf("uim.New() error = %v", err)
			}
			adapter, err := usim.NewQCOM(reader)
			if err != nil {
				t.Fatalf("NewQCOM() error = %v", err)
			}
			session, err := adapter.OpenIMSPDN(context.Background(), usim.IMSPDNConfig{})
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			first := session.Info()
			first.LocalIPv4[0] = 192
			first.PCSCFIPs[0][0] = 203

			second := session.Info()
			if bytes.Equal(first.LocalIPv4, second.LocalIPv4) {
				t.Fatal("Info() returned aliased LocalIPv4")
			}
			if bytes.Equal(first.PCSCFIPs[0], second.PCSCFIPs[0]) {
				t.Fatal("Info() returned aliased P-CSCF address")
			}
		})
	}
}
