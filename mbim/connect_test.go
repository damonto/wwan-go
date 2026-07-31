package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTLVsUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    TLVs
		wantErr bool
	}{
		{
			name: "padded values",
			data: append(mbimTLV(TLVTypePCO, []byte{0x80, 0x00}), mbimTLV(TLVTypeWCharString, utf16Bytes("ims"))...),
			want: TLVs{
				{Type: TLVTypePCO, Data: []byte{0x80, 0x00}},
				{Type: TLVTypeWCharString, Data: utf16Bytes("ims")},
			},
		},
		{
			name:    "truncated header",
			data:    []byte{0x0d},
			wantErr: true,
		},
		{
			name:    "invalid padding length",
			data:    []byte{0x0d, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "reserved type zero",
			data:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "reserved high type",
			data:    []byte{0xf1, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "nonzero reserved byte",
			data:    []byte{0x0d, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "incorrect padding length",
			data:    []byte{0x0d, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0xaa, 0xbb},
			wantErr: true,
		},
		{
			name:    "nonzero padding",
			data:    []byte{0x0d, 0x00, 0x00, 0x03, 0x01, 0x00, 0x00, 0x00, 0xaa, 0x01, 0x02, 0x03},
			wantErr: true,
		},
		{
			name:    "truncated data",
			data:    []byte{0x0d, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0xaa},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got TLVs
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("TLVs len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Type != tt.want[i].Type || !bytes.Equal(got[i].Data, tt.want[i].Data) {
					t.Fatalf("TLVs[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTLVsMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		values  TLVs
		want    []byte
		wantErr bool
	}{
		{
			name: "multiple values",
			values: TLVs{
				{Type: TLVTypePCO, Data: []byte{0x80, 0x00}},
				{Type: TLVTypeWCharString, Data: utf16Bytes("ims")},
			},
			want: append(mbimTLV(TLVTypePCO, []byte{0x80, 0x00}), mbimTLV(TLVTypeWCharString, utf16Bytes("ims"))...),
		},
		{
			name:    "reserved type",
			values:  TLVs{{Type: 0}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.values.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = %x, want %x", got, tt.want)
			}

			var roundTrip TLVs
			if err := roundTrip.UnmarshalBinary(got); err != nil {
				t.Fatalf("round trip UnmarshalBinary() error = %v", err)
			}
			if len(roundTrip) != len(tt.values) {
				t.Fatalf("round trip length = %d, want %d", len(roundTrip), len(tt.values))
			}
			for i := range roundTrip {
				if roundTrip[i].Type != tt.values[i].Type || !bytes.Equal(roundTrip[i].Data, tt.values[i].Data) {
					t.Fatalf("round trip value %d = %+v, want %+v", i, roundTrip[i], tt.values[i])
				}
			}
		})
	}
}

func TestTLVsUnmarshalBinaryResetsReceiver(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "reset existing values"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tlvs TLVs
			if err := tlvs.UnmarshalBinary(mbimTLV(TLVTypePCO, []byte{0x80})); err != nil {
				t.Fatalf("first UnmarshalBinary() error = %v", err)
			}
			if err := tlvs.UnmarshalBinary(nil); err != nil {
				t.Fatalf("second UnmarshalBinary() error = %v", err)
			}
			if len(tlvs) != 0 {
				t.Fatalf("TLVs len = %d, want 0", len(tlvs))
			}
		})
	}
}

func TestProtocolConfigurationOptionsUnmarshalBinary(t *testing.T) {
	pcscfIPv6 := net.ParseIP("2001:db8::1").To16()
	tests := []struct {
		name          string
		data          []byte
		wantParseErr  bool
		wantOption    int
		wantExtension bool
		wantProtocol  byte
		wantPCSCF     []net.IP
	}{
		{
			name:          "pcscf addresses",
			data:          pcoPayloadForTest(net.IPv4(198, 51, 100, 10), pcscfIPv6, net.IPv4(198, 51, 100, 10)),
			wantOption:    3,
			wantExtension: true,
			wantPCSCF:     []net.IP{net.IPv4(198, 51, 100, 10), pcscfIPv6},
		},
		{
			name:          "two byte length option",
			data:          []byte{0x80, 0x00, 0x23, 0x00, 0x01, 0xaa},
			wantOption:    1,
			wantExtension: true,
		},
		{
			name:          "configuration protocol strips spare bits",
			data:          []byte{0x83},
			wantExtension: true,
			wantProtocol:  3,
		},
		{
			name:         "empty",
			data:         nil,
			wantParseErr: true,
		},
		{
			name:         "truncated option",
			data:         []byte{0x80, 0x00},
			wantParseErr: true,
		},
		{
			name:         "truncated option data",
			data:         []byte{0x80, 0x00, 0x0c, 0x04, 0xc6},
			wantParseErr: true,
		},
		{
			name:         "bad pcscf length",
			data:         []byte{0x80, 0x00, 0x0c, 0x03, 0xc6, 0x33, 0x64},
			wantParseErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pco ProtocolConfigurationOptions
			err := pco.UnmarshalBinary(tt.data)
			if tt.wantParseErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(pco.Options) != tt.wantOption {
				t.Fatalf("options len = %d, want %d", len(pco.Options), tt.wantOption)
			}
			if pco.Extension != tt.wantExtension {
				t.Fatalf("Extension = %v, want %v", pco.Extension, tt.wantExtension)
			}
			if pco.ConfigurationProtocol != tt.wantProtocol {
				t.Fatalf("ConfigurationProtocol = %d, want %d", pco.ConfigurationProtocol, tt.wantProtocol)
			}
			if len(pco.PCSCFIPs) != len(tt.wantPCSCF) {
				t.Fatalf("pco.PCSCFIPs len = %d, want %d", len(pco.PCSCFIPs), len(tt.wantPCSCF))
			}
			gotPCSCF, err := PCSCFIPsFromPCO(tt.data)
			if err != nil {
				t.Fatalf("PCSCFIPsFromPCO() error = %v", err)
			}
			if len(gotPCSCF) != len(tt.wantPCSCF) {
				t.Fatalf("PCSCFIPs len = %d, want %d", len(gotPCSCF), len(tt.wantPCSCF))
			}
			for i, want := range tt.wantPCSCF {
				if !gotPCSCF[i].Equal(want) {
					t.Fatalf("PCSCFIPs[%d] = %v, want %v", i, gotPCSCF[i], want)
				}
			}
		})
	}
}

func TestProtocolConfigurationOptionsUnmarshalBinaryResetsReceiver(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "reset existing options"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pco ProtocolConfigurationOptions
			if err := pco.UnmarshalBinary([]byte{0x80, 0x00, 0x0c, 0x04, 192, 0, 2, 1}); err != nil {
				t.Fatalf("first UnmarshalBinary() error = %v", err)
			}
			if err := pco.UnmarshalBinary([]byte{0x83}); err != nil {
				t.Fatalf("second UnmarshalBinary() error = %v", err)
			}
			if !pco.Extension {
				t.Fatal("Extension = false, want true")
			}
			if pco.ConfigurationProtocol != 3 {
				t.Fatalf("ConfigurationProtocol = %d, want 3", pco.ConfigurationProtocol)
			}
			if len(pco.Options) != 0 {
				t.Fatalf("Options len = %d, want 0", len(pco.Options))
			}
		})
	}
}

func TestPCOExtractors(t *testing.T) {
	dnsIPv6 := net.ParseIP("2001:db8::53").To16()
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		wantDNS []net.IP
		wantMTU uint16
		wantOK  bool
	}{
		{
			name: "dns and mtu",
			data: pcoPayloadWithOptionsForTest(
				pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{8, 8, 8, 8}},
				pcoOptionForTest{id: pcoOptionDNSIPv6, value: dnsIPv6},
				pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{8, 8, 8, 8}},
				pcoOptionForTest{id: pcoOptionIPv4MTU, value: []byte{0x05, 0xdc}},
			),
			wantDNS: []net.IP{net.IPv4(8, 8, 8, 8), dnsIPv6},
			wantMTU: 1500,
			wantOK:  true,
		},
		{
			name:    "bad dns length",
			data:    pcoPayloadWithOptionsForTest(pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{8, 8, 8}}),
			wantErr: true,
		},
		{
			name:    "bad mtu length",
			data:    pcoPayloadWithOptionsForTest(pcoOptionForTest{id: pcoOptionIPv4MTU, value: []byte{0x05}}),
			wantErr: true,
		},
		{
			name: "missing mtu",
			data: pcoPayloadWithOptionsForTest(pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{1, 1, 1, 1}}),
			wantDNS: []net.IP{
				net.IPv4(1, 1, 1, 1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pco ProtocolConfigurationOptions
			err := pco.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if len(pco.DNSIPs) != len(tt.wantDNS) {
				t.Fatalf("DNS IPs len = %d, want %d", len(pco.DNSIPs), len(tt.wantDNS))
			}
			for i, want := range tt.wantDNS {
				if !pco.DNSIPs[i].Equal(want) {
					t.Fatalf("DNS IPs[%d] = %v, want %v", i, pco.DNSIPs[i], want)
				}
			}
			if pco.IPv4LinkMTUKnown != tt.wantOK {
				t.Fatalf("IPv4LinkMTUKnown = %v, want %v", pco.IPv4LinkMTUKnown, tt.wantOK)
			}
			if pco.IPv4LinkMTU != tt.wantMTU {
				t.Fatalf("IPv4LinkMTU = %d, want %d", pco.IPv4LinkMTU, tt.wantMTU)
			}
		})
	}
}

func TestConnectRequestData(t *testing.T) {
	trafficParameters := TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}}
	tests := []struct {
		name    string
		req     ConnectRequest
		want    []byte
		wantErr bool
	}{
		{
			name: "mbim 1 activate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
			},
			want: connectSetDataV1ForTest(1, ActivationCommandActivate, "ims", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name: "mbim 1 deactivate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				SessionID:         1,
				ActivationCommand: ActivationCommandDeactivate,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
			},
			want: connectSetDataV1ForTest(1, ActivationCommandDeactivate, "", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name: "mbim ex 3 activate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion30,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
			},
			want: connectSetDataEx3ForTest(1, ActivationCommandActivate, "ims", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name: "mbim ex 3 unnamed TLV",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion30,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				TLVs:              TLVs{{Type: TLVTypePCO, Data: []byte{0x80}}},
			},
			want: connectSetDataEx3RequestForTest(ConnectRequest{
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				TLVs:              TLVs{{Type: TLVTypePCO, Data: []byte{0x80}}},
			}),
		},
		{
			name: "mbim ex 3 deactivate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion30,
				SessionID:         1,
				ActivationCommand: ActivationCommandDeactivate,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
			},
			want: connectSetDataEx3ForTest(1, ActivationCommandDeactivate, "", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name: "mbim ex 4 activate IMS with S-NSSAI",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion40,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				UserName:          "alice",
				Password:          "secret",
				Compression:       CompressionEnable,
				AuthProtocol:      AuthProtocolPAP,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				SNSSAI: &SNSSAI{
					SliceServiceType:       1,
					SliceDifferentiator:    [3]byte{0x11, 0x22, 0x33},
					HasSliceDifferentiator: true,
				},
				TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 1, 3}}},
			},
			want: connectSetDataEx4RequestForTest(ConnectRequest{
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				UserName:          "alice",
				Password:          "secret",
				Compression:       CompressionEnable,
				AuthProtocol:      AuthProtocolPAP,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				SNSSAI: &SNSSAI{
					SliceServiceType:       1,
					SliceDifferentiator:    [3]byte{0x11, 0x22, 0x33},
					HasSliceDifferentiator: true,
				},
				TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 1, 3}}},
			}),
		},
		{
			name: "mbim ex 4 activate with URSP option",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion40,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				ActivationOption:  ActivationOptionPerURSPRules,
				TLVs:              trafficParameters,
			},
			want: append(
				connectSetDataEx4ForTest(1, ActivationCommandActivate, ActivationOptionPerURSPRules, "", ContextIPTypeIPv4v6, ContextTypeIMS),
				marshalTLVsUnchecked(trafficParameters)...,
			),
		},
		{
			name: "mbim ex 4 deactivate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion40,
				SessionID:         1,
				ActivationCommand: ActivationCommandDeactivate,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
			},
			want: connectSetDataEx4ForTest(1, ActivationCommandDeactivate, ActivationOptionDefault, "", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name: "mbim ex 4 deactivate ignores URSP option",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion40,
				SessionID:         1,
				ActivationCommand: ActivationCommandDeactivate,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
				ActivationOption:  ActivationOptionPerURSPRules,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req.Request()
			_, err := req.MarshalBinary()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			command := req.Command.(*Command)
			if command.ServiceID != ServiceBasicConnect || command.CommandID != CIDConnect || command.CommandType != CommandTypeSet {
				t.Fatalf("command = service % X cid %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if req.Timeout != mbimConnectSetResponseTimeout {
				t.Fatalf("Timeout = %v, want %v", req.Timeout, mbimConnectSetResponseTimeout)
			}
			if tt.req.Response.MBIMExVersion != tt.req.MBIMExVersion {
				t.Fatalf("response version = %#x, want %#x", tt.req.Response.MBIMExVersion, tt.req.MBIMExVersion)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
			}
		})
	}
}

func TestConnectRequestEx4FixedFields(t *testing.T) {
	tests := []struct {
		name string
		req  ConnectRequest
	}{
		{
			name: "official field offsets",
			req: ConnectRequest{
				MBIMExVersion:     mbimExVersion40,
				SessionID:         0x11,
				ActivationCommand: ActivationCommandActivate,
				ActivationOption:  ActivationOptionPerURSPRules,
				Compression:       CompressionEnable,
				AuthProtocol:      AuthProtocolCHAP,
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeCustom,
				MediaPreference:   AccessMediaType3GPPPreferred,
				TLVs:              TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := tt.req.Request()
			if _, err := request.MarshalBinary(); err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			data := request.Command.(*Command).Data
			if len(data) < 44 {
				t.Fatalf("Data len = %d, want at least 44", len(data))
			}
			fields := []struct {
				name   string
				offset int
				want   uint32
			}{
				{name: "SessionID", offset: 0, want: uint32(tt.req.SessionID)},
				{name: "ActivationCommand", offset: 4, want: uint32(tt.req.ActivationCommand)},
				{name: "ActivationOption", offset: 8, want: uint32(tt.req.ActivationOption)},
				{name: "Compression", offset: 12, want: uint32(tt.req.Compression)},
				{name: "AuthProtocol", offset: 16, want: uint32(tt.req.AuthProtocol)},
				{name: "ContextSessionType", offset: 20, want: uint32(tt.req.IPType)},
				{name: "MediaPreference", offset: 40, want: uint32(tt.req.MediaPreference)},
			}
			for _, field := range fields {
				if got := binary.LittleEndian.Uint32(data[field.offset : field.offset+4]); got != field.want {
					t.Errorf("%s at offset %d = %#x, want %#x", field.name, field.offset, got, field.want)
				}
			}
			if !bytes.Equal(data[24:40], tt.req.ContextType[:]) {
				t.Errorf("ContextPurposeType at offset 24 = %x, want %x", data[24:40], tt.req.ContextType)
			}
		})
	}
}

func TestPacketServiceRequestData(t *testing.T) {
	tests := []struct {
		name        string
		req         *Request
		commandType CommandType
		wantPayload []byte
		wantTimeout time.Duration
	}{
		{
			name:        "query",
			req:         (&PacketServiceRequest{TransactionID: 1}).Request(),
			commandType: CommandTypeQuery,
			wantTimeout: mbimCIDResponseTimeout,
		},
		{
			name:        "attach",
			req:         (&PacketServiceSetRequest{TransactionID: 1, Action: PacketServiceActionAttach}).Request(),
			commandType: CommandTypeSet,
			wantPayload: []byte{0, 0, 0, 0},
			wantTimeout: mbimCIDLongResponseTimeout,
		},
		{
			name:        "detach",
			req:         (&PacketServiceSetRequest{TransactionID: 1, Action: PacketServiceActionDetach}).Request(),
			commandType: CommandTypeSet,
			wantPayload: []byte{1, 0, 0, 0},
			wantTimeout: mbimCIDLongResponseTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := tt.req.Command.(*Command)
			if command.ServiceID != ServiceBasicConnect || command.CommandID != CIDPacketService || command.CommandType != tt.commandType {
				t.Fatalf("command = service % X cid %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if tt.req.Timeout != tt.wantTimeout {
				t.Fatalf("Timeout = %v, want %v", tt.req.Timeout, tt.wantTimeout)
			}
			if !bytes.Equal(command.Data, tt.wantPayload) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.wantPayload)
			}
		})
	}
}

func TestPacketServiceInfoUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "attached", data: packetServicePayloadForTest(PacketServiceStateAttached)},
		{name: "truncated", data: []byte{1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PacketServiceInfo
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.PacketServiceState != PacketServiceStateAttached {
				t.Fatalf("PacketServiceState = %d, want %d", got.PacketServiceState, PacketServiceStateAttached)
			}
		})
	}
}

func TestConnectQueryRequestDataUsesVersionShape(t *testing.T) {
	tests := []struct {
		name        string
		version     uint16
		wantPayload []byte
	}{
		{name: "mbim 1", version: mbimExVersion10, wantPayload: connectQueryDataForTest(1, 36)},
		{name: "mbim ex 3", version: mbimExVersion30, wantPayload: connectQueryDataForTest(1, 4)},
		{name: "mbim ex 4", version: mbimExVersion40, wantPayload: connectQueryDataForTest(1, 4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &ConnectQueryRequest{
				TransactionID: 1,
				MBIMExVersion: tt.version,
				SessionID:     1,
			}
			req := request.Request()
			command := req.Command.(*Command)
			if !bytes.Equal(command.Data, tt.wantPayload) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.wantPayload)
			}
			if request.Response.MBIMExVersion != tt.version {
				t.Fatalf("response version = %#x, want %#x", request.Response.MBIMExVersion, tt.version)
			}
		})
	}
}

func TestConnectInfoUnmarshalBinary(t *testing.T) {
	pcscfIPv6 := net.ParseIP("2001:db8::1").To16()
	pcoWithConfig := pcoPayloadWithOptionsForTest(
		pcoOptionForTest{id: pcoOptionPCSCFIPv4, value: []byte{198, 51, 100, 10}},
		pcoOptionForTest{id: pcoOptionPCSCFIPv6, value: pcscfIPv6},
		pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{8, 8, 8, 8}},
		pcoOptionForTest{id: pcoOptionIPv4MTU, value: []byte{0x05, 0xdc}},
	)
	snssai := &SNSSAI{
		SliceServiceType:       1,
		SliceDifferentiator:    [3]byte{0x11, 0x22, 0x33},
		HasSliceDifferentiator: true,
	}
	tests := []struct {
		name         string
		version      uint16
		data         []byte
		wantErr      bool
		wantAccess   string
		wantSNSSAI   *SNSSAI
		wantTraffic  []byte
		wantTLVs     int
		wantPCSCFLen int
		wantDNSLen   int
		wantMTU      uint16
		wantMTUOK    bool
	}{
		{
			name: "mbim 1",
			data: connectInfoPayloadForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS),
		},
		{
			name:         "mbim ex 3 with unnamed TLVs",
			version:      mbimExVersion30,
			data:         connectInfoPayloadEx3ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", TLVs{{Type: TLVTypeWCharString, Data: utf16Bytes("unnamed")}, {Type: TLVTypePCO, Data: pcoWithConfig}}),
			wantAccess:   "ims",
			wantTLVs:     2,
			wantPCSCFLen: 2,
			wantDNSLen:   1,
			wantMTU:      1500,
			wantMTUOK:    true,
		},
		{
			name:       "mbim ex 4 with S-NSSAI",
			version:    mbimExVersion40,
			data:       connectInfoPayloadEx4ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", snssai, TLVs{{Type: TLVTypePCO, Data: []byte{0x80}}}),
			wantAccess: "ims",
			wantSNSSAI: snssai,
			wantTLVs:   1,
		},
		{
			name:       "mbim ex 4 without S-NSSAI",
			version:    mbimExVersion40,
			data:       connectInfoPayloadEx4ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", nil, nil),
			wantAccess: "ims",
		},
		{
			name:        "mbim ex 4 with traffic parameters",
			version:     mbimExVersion40,
			data:        connectInfoPayloadEx4ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", nil, TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 2, 0xaa, 0xbb}}}),
			wantAccess:  "ims",
			wantTraffic: []byte{0xaa, 0xbb},
			wantTLVs:    1,
		},
		{
			name:    "truncated",
			data:    []byte{1},
			wantErr: true,
		},
		{
			name:    "truncated ex",
			data:    append(connectInfoPayloadForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS), 0),
			wantErr: true,
		},
		{
			name:    "mbim 1 trailing data",
			version: mbimExVersion10,
			data:    append(connectInfoPayloadForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS), make([]byte, 4)...),
			wantErr: true,
		},
		{
			name:    "missing access string TLV",
			version: mbimExVersion30,
			data:    connectInfoPayloadExHeaderForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS),
			wantErr: true,
		},
		{
			name:    "incorrect access string TLV type",
			version: mbimExVersion30,
			data:    append(connectInfoPayloadExHeaderForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS), mbimTLV(TLVTypePCO, []byte{0x80})...),
			wantErr: true,
		},
		{
			name:    "incorrect S-NSSAI TLV type",
			version: mbimExVersion40,
			data: append(
				append(connectInfoPayloadExHeaderForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS), mbimTLV(TLVTypeWCharString, utf16Bytes("ims"))...),
				mbimTLV(TLVTypePCO, []byte{0x80})...,
			),
			wantErr: true,
		},
		{
			name:    "truncated S-NSSAI",
			version: mbimExVersion40,
			data: append(
				append(connectInfoPayloadExHeaderForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS), mbimTLV(TLVTypeWCharString, utf16Bytes("ims"))...),
				mbimTLV(TLVTypeSingleNSSAI, []byte{4, 1, 0x11})...,
			),
			wantErr: true,
		},
		{
			name:    "malformed unnamed TLV",
			version: mbimExVersion30,
			data:    append(connectInfoPayloadEx3ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", nil), 0x0d),
			wantErr: true,
		},
		{
			name:    "malformed traffic parameters",
			version: mbimExVersion40,
			data:    connectInfoPayloadEx4ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", nil, TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 2, 0xaa}}}),
			wantErr: true,
		},
		{
			name:    "duplicate traffic parameters",
			version: mbimExVersion40,
			data: connectInfoPayloadEx4ForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", nil, TLVs{
				{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}},
				{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}},
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConnectInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.SessionID != 1 || got.ActivationState != ActivationStateActivated || got.IPType != ContextIPTypeIPv4v6 || got.ContextType != ContextTypeIMS {
				t.Fatalf("UnmarshalBinary() = %+v", got)
			}
			if got.AccessString != tt.wantAccess {
				t.Fatalf("AccessString = %q, want %q", got.AccessString, tt.wantAccess)
			}
			if (got.SNSSAI == nil) != (tt.wantSNSSAI == nil) {
				t.Fatalf("SNSSAI = %+v, want %+v", got.SNSSAI, tt.wantSNSSAI)
			}
			if got.SNSSAI != nil && *got.SNSSAI != *tt.wantSNSSAI {
				t.Fatalf("SNSSAI = %+v, want %+v", *got.SNSSAI, *tt.wantSNSSAI)
			}
			if (got.TrafficParameters == nil) != (tt.wantTraffic == nil) {
				t.Fatalf("TrafficParameters = %+v, want descriptor %x", got.TrafficParameters, tt.wantTraffic)
			}
			if got.TrafficParameters != nil && !bytes.Equal(got.TrafficParameters.TrafficDescriptor, tt.wantTraffic) {
				t.Fatalf("TrafficDescriptor = %x, want %x", got.TrafficParameters.TrafficDescriptor, tt.wantTraffic)
			}
			if len(got.TLVs) != tt.wantTLVs {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), tt.wantTLVs)
			}
			if len(got.PCSCFIPs) != tt.wantPCSCFLen {
				t.Fatalf("P-CSCF len = %d, want %d", len(got.PCSCFIPs), tt.wantPCSCFLen)
			}
			if len(got.DNSIPs) != tt.wantDNSLen {
				t.Fatalf("DNS len = %d, want %d", len(got.DNSIPs), tt.wantDNSLen)
			}
			if got.IPv4LinkMTUKnown != tt.wantMTUOK {
				t.Fatalf("IPv4LinkMTUKnown = %v, want %v", got.IPv4LinkMTUKnown, tt.wantMTUOK)
			}
			if got.IPv4LinkMTU != tt.wantMTU {
				t.Fatalf("IPv4LinkMTU = %d, want %d", got.IPv4LinkMTU, tt.wantMTU)
			}
		})
	}
}

func TestConnectInfoSessionIDTLVValidation(t *testing.T) {
	tests := []struct {
		name      string
		version   uint16
		tlvs      TLVs
		wantID    SessionID
		wantIDSet bool
		wantErr   bool
	}{
		{
			name:      "MBIMEx 4",
			version:   mbimExVersion40,
			tlvs:      TLVs{NewSessionIDTLV(7)},
			wantID:    7,
			wantIDSet: true,
		},
		{
			name:    "MBIMEx 3 ignores newer TLV",
			version: mbimExVersion30,
			tlvs:    TLVs{NewSessionIDTLV(7)},
		},
		{
			name:    "malformed",
			version: mbimExVersion40,
			tlvs:    TLVs{{Type: TLVTypeSessionID, Data: make([]byte, 3)}},
			wantErr: true,
		},
		{
			name:    "duplicate",
			version: mbimExVersion40,
			tlvs:    TLVs{NewSessionIDTLV(7), NewSessionIDTLV(8)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := connectInfoPayloadForVersionForTest(
				tt.version,
				1,
				ActivationStateDeactivated,
				ContextIPTypeDefault,
				ContextTypeNone,
				"",
				nil,
				tt.tlvs,
			)
			got := ConnectInfo{MBIMExVersion: tt.version}
			err := got.UnmarshalBinary(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if (got.MatchingSessionID != nil) != tt.wantIDSet {
				t.Fatalf("MatchingSessionID = %v, want set %v", got.MatchingSessionID, tt.wantIDSet)
			}
			if got.MatchingSessionID != nil && *got.MatchingSessionID != tt.wantID {
				t.Fatalf("MatchingSessionID = %d, want %d", *got.MatchingSessionID, tt.wantID)
			}
		})
	}
}

func TestIPConfigurationInfoUnmarshalBinary(t *testing.T) {
	ipv6 := net.ParseIP("2001:db8::2").To16()
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "addresses", data: ipConfigurationPayloadForTest(1, net.IPv4(10, 0, 0, 2), 24, ipv6, 64)},
		{name: "truncated", data: []byte{1}, wantErr: true},
		{name: "truncated IPv4 table", data: corruptIPConfigurationPayloadForTest(func(data []byte) {
			binary.LittleEndian.PutUint32(data[12:16], 4)
		}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IPConfigurationInfo
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if got.SessionID != 1 {
				t.Fatalf("SessionID = %d, want 1", got.SessionID)
			}
			if len(got.IPv4Addresses) != 1 || !got.IPv4Addresses[0].IP.Equal(net.IPv4(10, 0, 0, 2)) || got.IPv4Addresses[0].PrefixLength != 24 {
				t.Fatalf("IPv4Addresses = %+v", got.IPv4Addresses)
			}
			if len(got.IPv6Addresses) != 1 || !got.IPv6Addresses[0].IP.Equal(ipv6) || got.IPv6Addresses[0].PrefixLength != 64 {
				t.Fatalf("IPv6Addresses = %+v", got.IPv6Addresses)
			}
		})
	}
}

func TestClientOpenIMSPDN(t *testing.T) {
	tests := []struct {
		name          string
		mbimExVersion uint16
		packetState   PacketServiceState
		wantAttachSet bool
	}{
		{name: "mbim ex 3 already attached", mbimExVersion: mbimExVersion30, packetState: PacketServiceStateAttached},
		{name: "mbim ex 3 detached attaches first", mbimExVersion: mbimExVersion30, packetState: PacketServiceStateDetached, wantAttachSet: true},
		{name: "mbim ex 4 already attached", mbimExVersion: mbimExVersion40, packetState: PacketServiceStateAttached},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			pcscfIPv6 := net.ParseIP("2001:db8::1").To16()
			dnsIPv6 := net.ParseIP("2001:db8::53").To16()
			localIPv6 := net.ParseIP("2001:db8::2").To16()

			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()

				transactionID := uint32(1)
				if tt.mbimExVersion >= mbimExVersion20 {
					if err := expectMBIMCommandWithService(server, transactionID, ServiceMsBasicConnectExtensions, CIDMsDeviceCapsV2, CommandTypeQuery, nil); err != nil {
						errc <- err
						return
					}
					if _, err := server.Write(mbimCommandDone(transactionID, ServiceMsBasicConnectExtensions, CIDMsDeviceCapsV2, deviceCapsPayloadV3ForTest(2))); err != nil {
						errc <- err
						return
					}
				} else {
					if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDDeviceCaps, CommandTypeQuery, nil); err != nil {
						errc <- err
						return
					}
					if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDDeviceCaps, deviceCapsPayload(2))); err != nil {
						errc <- err
						return
					}
				}
				transactionID++

				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDPacketService, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDPacketService, packetServicePayloadForVersionForTest(tt.mbimExVersion, tt.packetState))); err != nil {
					errc <- err
					return
				}
				transactionID++

				if tt.wantAttachSet {
					if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDPacketService, CommandTypeSet, []byte{0, 0, 0, 0}); err != nil {
						errc <- err
						return
					}
					if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDPacketService, packetServicePayloadForVersionForTest(tt.mbimExVersion, PacketServiceStateAttached))); err != nil {
						errc <- err
						return
					}
					transactionID++
				}

				wantConnect := connectSetDataForVersionForTest(tt.mbimExVersion, 1, ActivationCommandActivate, DefaultIMSPDNAPN, ContextIPTypeIPv4v6, ContextTypeIMS)
				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDConnect, CommandTypeSet, wantConnect); err != nil {
					errc <- err
					return
				}
				pco := pcoPayloadWithOptionsForTest(
					pcoOptionForTest{id: pcoOptionPCSCFIPv4, value: []byte{198, 51, 100, 10}},
					pcoOptionForTest{id: pcoOptionPCSCFIPv6, value: pcscfIPv6},
					pcoOptionForTest{id: pcoOptionDNSIPv4, value: []byte{8, 8, 8, 8}},
					pcoOptionForTest{id: pcoOptionDNSIPv6, value: dnsIPv6},
					pcoOptionForTest{id: pcoOptionIPv4MTU, value: []byte{0x05, 0xdc}},
				)
				connectInfo := connectInfoPayloadForVersionForTest(
					tt.mbimExVersion,
					1,
					ActivationStateActivated,
					ContextIPTypeIPv4v6,
					ContextTypeIMS,
					DefaultIMSPDNAPN,
					nil,
					TLVs{{Type: TLVTypePCO, Data: pco}},
				)
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDConnect, connectInfo)); err != nil {
					errc <- err
					return
				}
				transactionID++

				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDIPConfiguration, CommandTypeQuery, ipConfigurationQueryDataForTest(1)); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDIPConfiguration, ipConfigurationPayloadForTest(1, net.IPv4(10, 0, 0, 2), 24, localIPv6, 64))); err != nil {
					errc <- err
					return
				}
				transactionID++

				wantDeactivate := connectSetDataForVersionForTest(tt.mbimExVersion, 1, ActivationCommandDeactivate, "", ContextIPTypeIPv4v6, ContextTypeIMS)
				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDConnect, CommandTypeSet, wantDeactivate); err != nil {
					errc <- err
					return
				}
				deactivated := connectInfoPayloadForVersionForTest(
					tt.mbimExVersion,
					1,
					ActivationStateDeactivated,
					ContextIPTypeIPv4v6,
					ContextTypeIMS,
					"",
					nil,
					nil,
				)
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDConnect, deactivated)); err != nil {
					errc <- err
					return
				}
			}()

			mbimClient := &Client{conn: client, mbimExVersion: tt.mbimExVersion}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			session, err := mbimClient.OpenIMSPDN(ctx, IMSPDNConfig{})
			if err != nil {
				t.Fatalf("OpenIMSPDN() error = %v", err)
			}
			info := session.Info()
			if !info.LocalIPv4.Equal(net.IPv4(10, 0, 0, 2)) {
				t.Fatalf("LocalIPv4 = %v, want 10.0.0.2", info.LocalIPv4)
			}
			if !info.LocalIPv6.Equal(localIPv6) {
				t.Fatalf("LocalIPv6 = %v, want %v", info.LocalIPv6, localIPv6)
			}
			if len(info.PCSCFIPs) != 2 {
				t.Fatalf("PCSCFIPs len = %d, want 2", len(info.PCSCFIPs))
			}
			if len(info.DNSIPs) != 2 {
				t.Fatalf("DNSIPs len = %d, want 2", len(info.DNSIPs))
			}
			if !info.IPv4LinkMTUKnown || info.IPv4LinkMTU != 1500 {
				t.Fatalf("IPv4LinkMTU = %d known %v, want 1500 known true", info.IPv4LinkMTU, info.IPv4LinkMTUKnown)
			}
			if info.VoPSKnown || info.VoPSSupported {
				t.Fatalf("VoPS = known %v supported %v, want unknown", info.VoPSKnown, info.VoPSSupported)
			}
			if !info.PacketDataReady {
				t.Fatal("PacketDataReady = false, want true")
			}
			if err := session.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateConnectConfig(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		cfg     ConnectConfig
		wantErr bool
	}{
		{name: "MBIM 1 valid", version: mbimExVersion10},
		{name: "MBIMEx 4 valid", version: mbimExVersion40, cfg: ConnectConfig{MediaPreference: AccessMediaType3GPP, SNSSAI: &SNSSAI{SliceServiceType: 1}, TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}}}},
		{name: "access string too long", version: mbimExVersion40, cfg: ConnectConfig{AccessString: strings.Repeat("a", 101)}, wantErr: true},
		{name: "user name too long", version: mbimExVersion40, cfg: ConnectConfig{UserName: strings.Repeat("a", 256)}, wantErr: true},
		{name: "password too long", version: mbimExVersion40, cfg: ConnectConfig{Password: strings.Repeat("a", 256)}, wantErr: true},
		{name: "MBIM 1 media preference", version: mbimExVersion10, cfg: ConnectConfig{MediaPreference: AccessMediaType3GPP}, wantErr: true},
		{name: "MBIM 1 unnamed TLV", version: mbimExVersion10, cfg: ConnectConfig{TLVs: TLVs{{Type: TLVTypePCO}}}, wantErr: true},
		{name: "MBIMEx 3 activation option", version: mbimExVersion30, cfg: ConnectConfig{ActivationOption: ActivationOptionPerURSPRules}, wantErr: true},
		{name: "MBIMEx 3 S-NSSAI", version: mbimExVersion30, cfg: ConnectConfig{SNSSAI: &SNSSAI{SliceServiceType: 1}}, wantErr: true},
		{name: "invalid S-NSSAI", version: mbimExVersion40, cfg: ConnectConfig{SNSSAI: &SNSSAI{HasMappedSliceDifferentiator: true}}, wantErr: true},
		{name: "reserved TLV type", version: mbimExVersion30, cfg: ConnectConfig{TLVs: TLVs{{Type: 0}}}, wantErr: true},
		{name: "reserved activation command", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: 2}, wantErr: true},
		{name: "reserved activation option", version: mbimExVersion40, cfg: ConnectConfig{ActivationOption: 4}, wantErr: true},
		{name: "malformed traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{TLVs: TLVs{{Type: TLVTypeTrafficParameters}}}, wantErr: true},
		{name: "default with duplicate traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: ActivationCommandActivate, TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}, {Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}}}, wantErr: true},
		{name: "non-default URSP without traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: ActivationCommandActivate, ActivationOption: ActivationOptionPerNonDefaultURSPRules}, wantErr: true},
		{name: "combined URSP with duplicate traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: ActivationCommandActivate, ActivationOption: ActivationOptionPerURSPRules, TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}, {Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}}}, wantErr: true},
		{name: "default URSP without traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: ActivationCommandActivate, ActivationOption: ActivationOptionPerDefaultURSPRule}},
		{name: "combined URSP with traffic parameters", version: mbimExVersion40, cfg: ConnectConfig{ActivationCommand: ActivationCommandActivate, ActivationOption: ActivationOptionPerURSPRules, TLVs: TLVs{{Type: TLVTypeTrafficParameters, Data: []byte{0, 0}}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectConfig(tt.cfg, tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConnectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientQueryConnect(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
	}{
		{name: "MBIM 1", version: mbimExVersion10},
		{name: "MBIMEx 3", version: mbimExVersion30},
		{name: "MBIMEx 4", version: mbimExVersion40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errC := make(chan error, 1)
			go func() {
				defer close(errC)
				defer server.Close()
				wantQuery := connectQueryDataForTest(7, 36)
				if tt.version >= mbimExVersion30 {
					wantQuery = connectQueryDataForTest(7, 4)
				}
				if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDConnect, CommandTypeQuery, wantQuery); err != nil {
					errC <- err
					return
				}
				payload := connectInfoPayloadForVersionForTest(tt.version, 7, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeInternet, "internet", nil, nil)
				if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDConnect, payload)); err != nil {
					errC <- err
				}
			}()

			mbimClient := &Client{conn: client, mbimExVersion: tt.version}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := mbimClient.QueryConnect(ctx, 7)
			if err != nil {
				t.Fatalf("QueryConnect() error = %v", err)
			}
			if got.SessionID != 7 || got.ActivationState != ActivationStateActivated || got.MBIMExVersion != tt.version {
				t.Fatalf("QueryConnect() = %+v", got)
			}
			if err := <-errC; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientSetConnect(t *testing.T) {
	tests := []struct {
		name    string
		version uint16
		cfg     ConnectConfig
	}{
		{
			name:    "MBIM 1",
			version: mbimExVersion10,
			cfg:     ConnectConfig{SessionID: 2, ActivationCommand: ActivationCommandActivate, AccessString: "internet", IPType: ContextIPTypeIPv4, ContextType: ContextTypeInternet},
		},
		{
			name:    "MBIMEx 3",
			version: mbimExVersion30,
			cfg:     ConnectConfig{SessionID: 2, ActivationCommand: ActivationCommandActivate, AccessString: "internet", IPType: ContextIPTypeIPv4, ContextType: ContextTypeInternet, MediaPreference: AccessMediaType3GPP},
		},
		{
			name:    "MBIMEx 4",
			version: mbimExVersion40,
			cfg:     ConnectConfig{SessionID: 2, ActivationCommand: ActivationCommandActivate, AccessString: "internet", IPType: ContextIPTypeIPv4, ContextType: ContextTypeInternet, MediaPreference: AccessMediaType3GPP, SNSSAI: &SNSSAI{SliceServiceType: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errC := make(chan error, 1)
			go func() {
				defer close(errC)
				defer server.Close()
				request := ConnectRequest{
					MBIMExVersion:     tt.version,
					SessionID:         tt.cfg.SessionID,
					ActivationCommand: tt.cfg.ActivationCommand,
					AccessString:      tt.cfg.AccessString,
					IPType:            tt.cfg.IPType,
					ContextType:       tt.cfg.ContextType,
					MediaPreference:   tt.cfg.MediaPreference,
					SNSSAI:            tt.cfg.SNSSAI,
				}
				wantData := connectSetDataForRequestForTest(request)
				if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDConnect, CommandTypeSet, wantData); err != nil {
					errC <- err
					return
				}
				payload := connectInfoPayloadForVersionForTest(tt.version, 2, ActivationStateActivated, ContextIPTypeIPv4, ContextTypeInternet, "internet", tt.cfg.SNSSAI, nil)
				if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDConnect, payload)); err != nil {
					errC <- err
				}
			}()

			mbimClient := &Client{conn: client, mbimExVersion: tt.version}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := mbimClient.SetConnect(ctx, tt.cfg)
			if err != nil {
				t.Fatalf("SetConnect() error = %v", err)
			}
			if got.SessionID != 2 || got.ActivationState != ActivationStateActivated || got.MBIMExVersion != tt.version {
				t.Fatalf("SetConnect() = %+v", got)
			}
			if err := <-errC; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientSetConnectMatchingPDUSession(t *testing.T) {
	tests := []struct {
		name            string
		activationState ActivationState
		tlvs            TLVs
		wantResponse    bool
		wantErrorText   string
	}{
		{
			name:            "matching session",
			activationState: ActivationStateDeactivated,
			tlvs:            TLVs{NewSessionIDTLV(7)},
			wantResponse:    true,
		},
		{
			name:            "missing session ID",
			activationState: ActivationStateDeactivated,
			wantErrorText:   "missing session ID TLV",
		},
		{
			name:            "not deactivated",
			activationState: ActivationStateActivated,
			tlvs:            TLVs{NewSessionIDTLV(7)},
			wantErrorText:   "activation state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			cfg := ConnectConfig{
				SessionID:         2,
				ActivationCommand: ActivationCommandActivate,
				ActivationOption:  ActivationOptionPerDefaultURSPRule,
			}
			errC := make(chan error, 1)
			go func() {
				defer close(errC)
				defer server.Close()
				request := ConnectRequest{
					MBIMExVersion:     mbimExVersion40,
					SessionID:         cfg.SessionID,
					ActivationCommand: cfg.ActivationCommand,
					ActivationOption:  cfg.ActivationOption,
				}
				if err := expectMBIMCommandWithService(
					server,
					1,
					ServiceBasicConnect,
					CIDConnect,
					CommandTypeSet,
					connectSetDataForRequestForTest(request),
				); err != nil {
					errC <- err
					return
				}
				payload := connectInfoPayloadForVersionForTest(
					mbimExVersion40,
					cfg.SessionID,
					tt.activationState,
					ContextIPTypeDefault,
					ContextTypeNone,
					"",
					nil,
					tt.tlvs,
				)
				if _, err := server.Write(mbimCommandDoneStatus(
					1,
					ServiceBasicConnect,
					CIDConnect,
					StatusMatchingPDUSessionFound,
					payload,
				)); err != nil {
					errC <- err
				}
			}()

			mbimClient := &Client{conn: client, mbimExVersion: mbimExVersion40}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := mbimClient.SetConnect(ctx, cfg)
			if !errors.Is(err, StatusMatchingPDUSessionFound) {
				t.Fatalf("SetConnect() error = %v, want StatusMatchingPDUSessionFound", err)
			}
			if tt.wantErrorText != "" && !strings.Contains(err.Error(), tt.wantErrorText) {
				t.Fatalf("SetConnect() error = %v, want text %q", err, tt.wantErrorText)
			}
			if tt.wantResponse {
				if got.SessionID != cfg.SessionID || got.ActivationState != ActivationStateDeactivated {
					t.Fatalf("SetConnect() = %+v", got)
				}
				if got.MatchingSessionID == nil || *got.MatchingSessionID != 7 {
					t.Fatalf("MatchingSessionID = %v, want 7", got.MatchingSessionID)
				}
			} else if got.SessionID != 0 || got.MatchingSessionID != nil {
				t.Fatalf("SetConnect() = %+v, want zero response", got)
			}
			if err := <-errC; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientSetConnectRejectsInvalidConfigBeforeTransmit(t *testing.T) {
	tests := []struct {
		name string
		cfg  ConnectConfig
	}{
		{name: "access string", cfg: ConnectConfig{AccessString: strings.Repeat("a", 101)}},
		{name: "TLV", cfg: ConnectConfig{TLVs: TLVs{{Type: 0}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{mbimExVersion: mbimExVersion40}
			_, err := client.SetConnect(context.Background(), tt.cfg)
			if err == nil {
				t.Fatal("SetConnect() error = nil, want non-nil")
			}
			if errors.Is(err, context.Canceled) {
				t.Fatalf("SetConnect() error = %v, want validation error", err)
			}
		})
	}
}

func TestConnectTimeout(t *testing.T) {
	tests := []struct {
		name               string
		timeout            time.Duration
		pduActivationCount uint32
		want               time.Duration
	}{
		{name: "default", want: mbimConnectSetResponseTimeout},
		{name: "one activation", pduActivationCount: 1, want: mbimConnectSetResponseTimeout},
		{name: "three activations", pduActivationCount: 3, want: 3 * mbimConnectSetResponseTimeout},
		{name: "explicit timeout", timeout: 7 * time.Second, pduActivationCount: 3, want: 7 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectTimeout(tt.timeout, tt.pduActivationCount); got != tt.want {
				t.Fatalf("connectTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConnectConfigRejectsTimeoutOverflow(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ConnectConfig
		wantErr bool
	}{
		{name: "maximum count overflows", cfg: ConnectConfig{PDUActivationCount: ^uint32(0)}, wantErr: true},
		{name: "explicit timeout ignores count", cfg: ConnectConfig{Timeout: time.Second, PDUActivationCount: ^uint32(0)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectConfig(tt.cfg, mbimExVersion40)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConnectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeIMSPDNConfigExplicitZeroValues(t *testing.T) {
	tests := []struct {
		name          string
		cfg           IMSPDNConfig
		wantIPType    ContextIPType
		wantSessionID SessionID
	}{
		{
			name:          "defaults",
			wantIPType:    ContextIPTypeIPv4v6,
			wantSessionID: DefaultIMSPDNSessionID,
		},
		{
			name: "explicit zero values",
			cfg: IMSPDNConfig{
				IPTypeSet:    true,
				SessionIDSet: true,
			},
			wantIPType:    ContextIPTypeDefault,
			wantSessionID: 0,
		},
		{
			name: "explicit nonzero values",
			cfg: IMSPDNConfig{
				IPType:    ContextIPTypeIPv6,
				SessionID: 3,
			},
			wantIPType:    ContextIPTypeIPv6,
			wantSessionID: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeIMSPDNConfig(tt.cfg)
			if got.IPType != tt.wantIPType || got.SessionID != tt.wantSessionID {
				t.Fatalf("normalizeIMSPDNConfig() IP type/session = %d/%d, want %d/%d", got.IPType, got.SessionID, tt.wantIPType, tt.wantSessionID)
			}
		})
	}
}

func TestClientOpenIMSPDNValidatesSessionCapacity(t *testing.T) {
	tests := []struct {
		name        string
		maxSessions uint32
		sessionID   SessionID
		want        string
	}{
		{
			name:        "zero sessions",
			maxSessions: 0,
			want:        "opening MBIM IMS PDN: device reports zero IP sessions",
		},
		{
			name:        "single session cannot open IMS session one",
			maxSessions: 1,
			sessionID:   1,
			want:        "opening MBIM IMS PDN: session ID 1 is out of range for 1 supported sessions",
		},
		{
			name:        "session equals capacity",
			maxSessions: 2,
			sessionID:   2,
			want:        "opening MBIM IMS PDN: session ID 2 is out of range for 2 supported sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })

			errc := make(chan error, 1)
			go func() {
				defer close(errc)
				defer server.Close()
				if err := expectMBIMCommandWithService(server, 1, ServiceBasicConnect, CIDDeviceCaps, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(1, ServiceBasicConnect, CIDDeviceCaps, deviceCapsPayload(tt.maxSessions))); err != nil {
					errc <- err
				}
			}()

			mbimClient := &Client{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := mbimClient.OpenIMSPDN(ctx, IMSPDNConfig{SessionID: tt.sessionID})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("OpenIMSPDN() error = %v, want %q", err, tt.want)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func connectSetDataForVersionForTest(version uint16, sessionID SessionID, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	switch {
	case version >= mbimExVersion40:
		return connectSetDataEx4ForTest(sessionID, command, ActivationOptionDefault, accessString, ipType, contextType)
	case version >= mbimExVersion30:
		return connectSetDataEx3ForTest(sessionID, command, accessString, ipType, contextType)
	default:
		return connectSetDataV1ForTest(sessionID, command, accessString, ipType, contextType)
	}
}

func connectSetDataForRequestForTest(request ConnectRequest) []byte {
	switch {
	case request.MBIMExVersion >= mbimExVersion40:
		return connectSetDataEx4RequestForTest(request)
	case request.MBIMExVersion >= mbimExVersion30:
		return connectSetDataEx3RequestForTest(request)
	default:
		return connectSetDataV1ForTest(request.SessionID, request.ActivationCommand, request.AccessString, request.IPType, request.ContextType)
	}
}

func connectSetDataV1ForTest(sessionID SessionID, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	access := utf16Bytes(accessString)
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], uint32(sessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(command))
	if len(access) != 0 {
		binary.LittleEndian.PutUint32(data[8:12], 60)
		binary.LittleEndian.PutUint32(data[12:16], uint32(len(access)))
	}
	binary.LittleEndian.PutUint32(data[40:44], uint32(ipType))
	copy(data[44:60], contextType[:])
	data = append(data, access...)
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	return data
}

func connectSetDataEx3ForTest(sessionID SessionID, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	return connectSetDataEx3RequestForTest(ConnectRequest{
		SessionID:         sessionID,
		ActivationCommand: command,
		AccessString:      accessString,
		IPType:            ipType,
		ContextType:       contextType,
		MediaPreference:   AccessMediaType3GPP,
	})
}

func connectSetDataEx4ForTest(sessionID SessionID, command ActivationCommand, option ActivationOption, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	return connectSetDataEx4RequestForTest(ConnectRequest{
		SessionID:         sessionID,
		ActivationCommand: command,
		ActivationOption:  option,
		AccessString:      accessString,
		IPType:            ipType,
		ContextType:       contextType,
		MediaPreference:   AccessMediaType3GPP,
	})
}

func connectSetDataEx3RequestForTest(request ConnectRequest) []byte {
	data := make([]byte, 40)
	binary.LittleEndian.PutUint32(data[0:4], uint32(request.SessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(request.ActivationCommand))
	binary.LittleEndian.PutUint32(data[8:12], uint32(request.Compression))
	binary.LittleEndian.PutUint32(data[12:16], uint32(request.AuthProtocol))
	binary.LittleEndian.PutUint32(data[16:20], uint32(request.IPType))
	copy(data[20:36], request.ContextType[:])
	binary.LittleEndian.PutUint32(data[36:40], uint32(request.MediaPreference))
	data = appendConnectSetDataTLVsForTest(data, request.AccessString, request.UserName, request.Password)
	return append(data, marshalTLVsUnchecked(request.TLVs)...)
}

func connectSetDataEx4RequestForTest(request ConnectRequest) []byte {
	option := request.ActivationOption
	if request.ActivationCommand == ActivationCommandDeactivate {
		option = ActivationOptionDefault
	}
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data[0:4], uint32(request.SessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(request.ActivationCommand))
	binary.LittleEndian.PutUint32(data[8:12], uint32(option))
	binary.LittleEndian.PutUint32(data[12:16], uint32(request.Compression))
	binary.LittleEndian.PutUint32(data[16:20], uint32(request.AuthProtocol))
	binary.LittleEndian.PutUint32(data[20:24], uint32(request.IPType))
	copy(data[24:40], request.ContextType[:])
	binary.LittleEndian.PutUint32(data[40:44], uint32(request.MediaPreference))
	data = appendConnectSetDataTLVsForTest(data, request.AccessString, request.UserName, request.Password)
	var snssai []byte
	if request.SNSSAI != nil {
		snssai = request.SNSSAI.marshalBinaryUnchecked()
	}
	data = append(data, mbimTLV(TLVTypeSingleNSSAI, snssai)...)
	return append(data, marshalTLVsUnchecked(request.TLVs)...)
}

func appendConnectSetDataTLVsForTest(data []byte, values ...string) []byte {
	for _, value := range values {
		data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(value))...)
	}
	return data
}

func connectQueryDataForTest(sessionID SessionID, size int) []byte {
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[:4], uint32(sessionID))
	return data
}

func connectInfoPayloadForTest(sessionID SessionID, state ActivationState, ipType ContextIPType, contextType ContextType) []byte {
	data := make([]byte, 36)
	binary.LittleEndian.PutUint32(data[:4], uint32(sessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(state))
	binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceCallStateNone))
	binary.LittleEndian.PutUint32(data[12:16], uint32(ipType))
	copy(data[16:32], contextType[:])
	return data
}

func connectInfoPayloadExHeaderForTest(sessionID SessionID, state ActivationState, ipType ContextIPType, contextType ContextType) []byte {
	data := connectInfoPayloadForTest(sessionID, state, ipType, contextType)
	return binary.LittleEndian.AppendUint32(data, uint32(AccessMediaType3GPP))
}

func connectInfoPayloadEx3ForTest(sessionID SessionID, state ActivationState, ipType ContextIPType, contextType ContextType, accessString string, tlvs TLVs) []byte {
	data := connectInfoPayloadExHeaderForTest(sessionID, state, ipType, contextType)
	data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(accessString))...)
	return append(data, marshalTLVsUnchecked(tlvs)...)
}

func connectInfoPayloadEx4ForTest(sessionID SessionID, state ActivationState, ipType ContextIPType, contextType ContextType, accessString string, snssai *SNSSAI, tlvs TLVs) []byte {
	data := connectInfoPayloadExHeaderForTest(sessionID, state, ipType, contextType)
	data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(accessString))...)
	var snssaiData []byte
	if snssai != nil {
		snssaiData = snssai.marshalBinaryUnchecked()
	}
	data = append(data, mbimTLV(TLVTypeSingleNSSAI, snssaiData)...)
	return append(data, marshalTLVsUnchecked(tlvs)...)
}

func connectInfoPayloadForVersionForTest(version uint16, sessionID SessionID, state ActivationState, ipType ContextIPType, contextType ContextType, accessString string, snssai *SNSSAI, tlvs TLVs) []byte {
	switch {
	case version >= mbimExVersion40:
		return connectInfoPayloadEx4ForTest(sessionID, state, ipType, contextType, accessString, snssai, tlvs)
	case version >= mbimExVersion30:
		return connectInfoPayloadEx3ForTest(sessionID, state, ipType, contextType, accessString, tlvs)
	default:
		return connectInfoPayloadForTest(sessionID, state, ipType, contextType)
	}
}

func pcoPayloadForTest(ips ...net.IP) []byte {
	data := []byte{0x80}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			data = append(data, 0x00, 0x0c, 0x04)
			data = append(data, v4...)
			continue
		}
		v6 := ip.To16()
		data = append(data, 0x00, 0x01, 0x10)
		data = append(data, v6...)
	}
	return data
}

type pcoOptionForTest struct {
	id    uint16
	value []byte
}

func pcoPayloadWithOptionsForTest(options ...pcoOptionForTest) []byte {
	data := []byte{0x80}
	for _, option := range options {
		data = binary.BigEndian.AppendUint16(data, option.id)
		data = append(data, byte(len(option.value)))
		data = append(data, option.value...)
	}
	return data
}

func packetServicePayloadForTest(state PacketServiceState) []byte {
	data := make([]byte, 28)
	binary.LittleEndian.PutUint32(data[4:8], uint32(state))
	binary.LittleEndian.PutUint64(data[12:20], 1000000)
	binary.LittleEndian.PutUint64(data[20:28], 1000000)
	return data
}

func packetServicePayloadForVersionForTest(version uint16, state PacketServiceState) []byte {
	data := packetServicePayloadForTest(state)
	if version >= mbimExVersion20 {
		data = append(data, make([]byte, 4)...)
	}
	if version >= mbimExVersion30 {
		data = append(data, make([]byte, 12)...)
	}
	return data
}

func deviceCapsPayloadV3ForTest(maxSessions uint32) []byte {
	data := make([]byte, 48)
	binary.LittleEndian.PutUint32(data[16:20], uint32(DataClassLTE|DataClass5G))
	binary.LittleEndian.PutUint64(data[28:36], uint64(DataSubclass5GNR))
	binary.LittleEndian.PutUint32(data[36:40], maxSessions)
	data = append(data, mbimTLV(TLVTypeUint16Table, nil)...)
	data = append(data, mbimTLV(TLVTypeUint16Table, nil)...)
	for range 4 {
		data = append(data, mbimTLV(TLVTypeWCharString, nil)...)
	}
	return data
}

func ipConfigurationQueryDataForTest(sessionID SessionID) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], uint32(sessionID))
	return data
}

func ipConfigurationPayloadForTest(sessionID SessionID, ipv4 net.IP, ipv4Prefix uint32, ipv6 net.IP, ipv6Prefix uint32) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], uint32(sessionID))
	binary.LittleEndian.PutUint32(data[4:8], uint32(IPConfigurationAvailableAddress|IPConfigurationAvailableGateway|IPConfigurationAvailableMTU))
	binary.LittleEndian.PutUint32(data[8:12], uint32(IPConfigurationAvailableAddress|IPConfigurationAvailableGateway|IPConfigurationAvailableMTU))
	binary.LittleEndian.PutUint32(data[12:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 60)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], 68)
	binary.LittleEndian.PutUint32(data[52:56], 1500)
	binary.LittleEndian.PutUint32(data[56:60], 1500)
	data = binary.LittleEndian.AppendUint32(data, ipv4Prefix)
	data = append(data, ipv4.To4()...)
	data = binary.LittleEndian.AppendUint32(data, ipv6Prefix)
	data = append(data, ipv6.To16()...)
	return data
}

func corruptIPConfigurationPayloadForTest(mutate func([]byte)) []byte {
	data := ipConfigurationPayloadForTest(1, net.IPv4(10, 0, 0, 2), 24, net.ParseIP("2001:db8::2").To16(), 64)
	mutate(data)
	return data
}
