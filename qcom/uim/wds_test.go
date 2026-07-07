package uim

import (
	"bytes"
	"encoding/binary"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestWDSRequestEncoding(t *testing.T) {
	tests := []struct {
		name          string
		req           qcom.Request
		wantService   qcom.ServiceType
		wantClientID  uint8
		wantMessageID qcom.MessageID
		wantTimeout   time.Duration
		wantTLV       byte
		wantValue     []byte
	}{
		{
			name: "start network",
			req: WDSStartNetworkInterfaceRequest{
				ClientID:             7,
				TransactionID:        9,
				Timeout:              3 * time.Second,
				APN:                  "ims",
				IPFamily:             qcom.WDSIPFamilyIPv4v6,
				TechnologyPreference: qcom.WDSTechnologyPreference3GPP,
			}.Request(),
			wantService:   qcom.ServiceWDS,
			wantClientID:  7,
			wantMessageID: qcom.MessageWDSStartNetworkInterface,
			wantTimeout:   3 * time.Second,
			wantTLV:       0x14,
			wantValue:     []byte("ims"),
		},
		{
			name: "stop network",
			req: WDSStopNetworkInterfaceRequest{
				ClientID:         8,
				TransactionID:    10,
				Timeout:          4 * time.Second,
				PacketDataHandle: 0x01020304,
			}.Request(),
			wantService:   qcom.ServiceWDS,
			wantClientID:  8,
			wantMessageID: qcom.MessageWDSStopNetworkInterface,
			wantTimeout:   4 * time.Second,
			wantTLV:       0x01,
			wantValue:     []byte{0x04, 0x03, 0x02, 0x01},
		},
		{
			name: "runtime settings",
			req: WDSGetRuntimeSettingsRequest{
				ClientID:      9,
				TransactionID: 11,
				Timeout:       5 * time.Second,
			}.Request(),
			wantService:   qcom.ServiceWDS,
			wantClientID:  9,
			wantMessageID: qcom.MessageWDSGetRuntimeSettings,
			wantTimeout:   5 * time.Second,
			wantTLV:       0x10,
			wantValue:     uint32ValueForTest(uint32(qcom.WDSRuntimeRequestedIMSSettings)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Service != tt.wantService {
				t.Fatalf("Service = 0x%02X, want 0x%02X", tt.req.Service, tt.wantService)
			}
			if tt.req.ClientID != tt.wantClientID {
				t.Fatalf("ClientID = %d, want %d", tt.req.ClientID, tt.wantClientID)
			}
			if tt.req.MessageID != tt.wantMessageID {
				t.Fatalf("MessageID = 0x%04X, want 0x%04X", tt.req.MessageID, tt.wantMessageID)
			}
			if tt.req.Timeout != tt.wantTimeout {
				t.Fatalf("Timeout = %v, want %v", tt.req.Timeout, tt.wantTimeout)
			}
			value, ok := tlv.Value(tt.req.TLVs, tt.wantTLV)
			if !ok {
				t.Fatalf("TLV 0x%02X missing", tt.wantTLV)
			}
			if !bytes.Equal(value, tt.wantValue) {
				t.Fatalf("TLV 0x%02X = % X, want % X", tt.wantTLV, value, tt.wantValue)
			}
		})
	}
}

func TestWDSGetRuntimeSettingsResponseUnmarshalTLVs(t *testing.T) {
	localIPv6 := net.ParseIP("2001:db8::1").To16()
	pcscfIPv6 := net.ParseIP("2001:db8::2").To16()

	tests := []struct {
		name       string
		tlvs       tlv.TLVs
		wantErr    bool
		wantIPv4   net.IP
		wantIPv6   net.IP
		wantPCSCF  []net.IP
		wantFamily qcom.WDSIPFamily
		wantIMCN   bool
	}{
		{
			name: "runtime addresses and pcscf",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x1E, []byte{3, 2, 1, 10}),
				tlv.Bytes(0x25, append(slices.Clone(localIPv6), 64)),
				tlv.Bytes(0x23, ipv4ListForTest(net.IPv4(198, 51, 100, 10))),
				tlv.Bytes(0x2E, ipv6ListForTest(pcscfIPv6)),
				tlv.Bytes(0x2B, []byte{byte(qcom.WDSIPFamilyIPv4v6)}),
				tlv.Bytes(0x2C, []byte{1}),
			},
			wantIPv4:   net.IPv4(10, 1, 2, 3),
			wantIPv6:   localIPv6,
			wantPCSCF:  []net.IP{net.IPv4(198, 51, 100, 10), pcscfIPv6},
			wantFamily: qcom.WDSIPFamilyIPv4v6,
			wantIMCN:   true,
		},
		{
			name:    "short local ipv4 fails",
			tlvs:    tlv.TLVs{tlv.Bytes(0x1E, []byte{10, 1, 2})},
			wantErr: true,
		},
		{
			name:    "truncated pcscf ipv4 list fails",
			tlvs:    tlv.TLVs{tlv.Bytes(0x23, []byte{1, 198})},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSGetRuntimeSettingsResponse
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
			if !got.Settings.LocalIPv4.Equal(tt.wantIPv4) {
				t.Fatalf("LocalIPv4 = %v, want %v", got.Settings.LocalIPv4, tt.wantIPv4)
			}
			if !got.Settings.LocalIPv6.Equal(tt.wantIPv6) {
				t.Fatalf("LocalIPv6 = %v, want %v", got.Settings.LocalIPv6, tt.wantIPv6)
			}
			if got.Settings.IPFamily != tt.wantFamily {
				t.Fatalf("IPFamily = %d, want %d", got.Settings.IPFamily, tt.wantFamily)
			}
			if got.Settings.IMCN != tt.wantIMCN {
				t.Fatalf("IMCN = %v, want %v", got.Settings.IMCN, tt.wantIMCN)
			}
			if len(got.Settings.PCSCFIPs) != len(tt.wantPCSCF) {
				t.Fatalf("PCSCFIPs len = %d, want %d", len(got.Settings.PCSCFIPs), len(tt.wantPCSCF))
			}
			for i, want := range tt.wantPCSCF {
				if !got.Settings.PCSCFIPs[i].Equal(want) {
					t.Fatalf("PCSCFIPs[%d] = %v, want %v", i, got.Settings.PCSCFIPs[i], want)
				}
			}
		})
	}
}

func TestWDSStartNetworkInterfaceResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name           string
		tlvs           tlv.TLVs
		wantErr        bool
		want           uint32
		wantCallEnd    qcom.WDSCallEndReason
		wantVerbose    qcom.WDSVerboseCallEndReason
		wantHasCallEnd bool
		wantHasVerbose bool
	}{
		{name: "handle present", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{4, 3, 2, 1})}, want: 0x01020304},
		{name: "handle missing"},
		{
			name: "call end reasons without handle",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, uint16ValueForTest(uint16(qcom.WDSCallEndReasonGenericUnspecified))),
				tlv.Bytes(0x11, verboseCallEndReasonForTest(qcom.WDSVerboseCallEndReasonTypeInternal, 241)),
			},
			wantCallEnd:    qcom.WDSCallEndReasonGenericUnspecified,
			wantVerbose:    qcom.WDSVerboseCallEndReason{Type: qcom.WDSVerboseCallEndReasonTypeInternal, Reason: 241},
			wantHasCallEnd: true,
			wantHasVerbose: true,
		},
		{name: "short handle fails", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: true},
		{name: "short call end reason fails", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, wantErr: true},
		{name: "short verbose call end reason fails", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{2, 0, 1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got WDSStartNetworkInterfaceResponse
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
			if got.PacketDataHandle != tt.want {
				t.Fatalf("PacketDataHandle = 0x%08X, want 0x%08X", got.PacketDataHandle, tt.want)
			}
			if got.HasCallEndReason != tt.wantHasCallEnd {
				t.Fatalf("HasCallEndReason = %v, want %v", got.HasCallEndReason, tt.wantHasCallEnd)
			}
			if got.CallEndReason != tt.wantCallEnd {
				t.Fatalf("CallEndReason = %d, want %d", got.CallEndReason, tt.wantCallEnd)
			}
			if got.HasVerboseCallEndReason != tt.wantHasVerbose {
				t.Fatalf("HasVerboseCallEndReason = %v, want %v", got.HasVerboseCallEndReason, tt.wantHasVerbose)
			}
			if got.VerboseCallEndReason != tt.wantVerbose {
				t.Fatalf("VerboseCallEndReason = %+v, want %+v", got.VerboseCallEndReason, tt.wantVerbose)
			}
		})
	}
}

func uint16ValueForTest(value uint16) []byte {
	out := make([]byte, 2)
	binary.LittleEndian.PutUint16(out, value)
	return out
}

func uint32ValueForTest(value uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, value)
	return out
}

func verboseCallEndReasonForTest(reasonType qcom.WDSVerboseCallEndReasonType, reason int16) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint16(out[:2], uint16(reasonType))
	binary.LittleEndian.PutUint16(out[2:], uint16(reason))
	return out
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
