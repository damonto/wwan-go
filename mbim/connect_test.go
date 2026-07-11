package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
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
	tests := []struct {
		name string
		req  ConnectRequest
		want []byte
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
			name: "mbim ex 4 activate IMS",
			req: ConnectRequest{
				TransactionID:     1,
				MBIMExVersion:     mbimExVersion40,
				SessionID:         1,
				ActivationCommand: ActivationCommandActivate,
				AccessString:      "ims",
				IPType:            ContextIPTypeIPv4v6,
				ContextType:       ContextTypeIMS,
				MediaPreference:   AccessMediaType3GPP,
			},
			want: connectSetDataEx4ForTest(1, ActivationCommandActivate, ActivationOptionDefault, "ims", ContextIPTypeIPv4v6, ContextTypeIMS),
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
			},
			want: connectSetDataEx4ForTest(1, ActivationCommandActivate, ActivationOptionPerURSPRules, "", ContextIPTypeIPv4v6, ContextTypeIMS),
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
			want: connectSetDataEx4ForTest(1, ActivationCommandDeactivate, ActivationOptionDefault, "", ContextIPTypeIPv4v6, ContextTypeIMS),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req.Request()
			command := req.Command.(*Command)
			if command.ServiceID != ServiceBasicConnect || command.CommandID != CIDConnect || command.CommandType != CommandTypeSet {
				t.Fatalf("command = service % X cid %d type %d", command.ServiceID, command.CommandID, command.CommandType)
			}
			if req.Timeout != mbimConnectSetResponseTimeout {
				t.Fatalf("Timeout = %v, want %v", req.Timeout, mbimConnectSetResponseTimeout)
			}
			if !bytes.Equal(command.Data, tt.want) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.want)
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
			req := (&ConnectQueryRequest{
				TransactionID: 1,
				MBIMExVersion: tt.version,
				SessionID:     1,
			}).Request()
			command := req.Command.(*Command)
			if !bytes.Equal(command.Data, tt.wantPayload) {
				t.Fatalf("Data = %X, want %X", command.Data, tt.wantPayload)
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
	tests := []struct {
		name         string
		data         []byte
		wantErr      bool
		wantAccess   string
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
			name:         "mbim ex with pco",
			data:         connectInfoPayloadExForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, "ims", pcoWithConfig),
			wantAccess:   "ims",
			wantPCSCFLen: 2,
			wantDNSLen:   1,
			wantMTU:      1500,
			wantMTUOK:    true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ConnectInfo
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

func TestReaderOpenIMSPDN(t *testing.T) {
	tests := []struct {
		name          string
		mbimExVersion uint16
		packetState   PacketServiceState
		wantAttachSet bool
	}{
		{name: "already attached", packetState: PacketServiceStateAttached},
		{name: "detached attaches first", packetState: PacketServiceStateDetached, wantAttachSet: true},
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
				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDDeviceCaps, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDDeviceCaps, deviceCapsPayload(2))); err != nil {
					errc <- err
					return
				}
				transactionID++

				if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDPacketService, CommandTypeQuery, nil); err != nil {
					errc <- err
					return
				}
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDPacketService, packetServicePayloadForTest(tt.packetState))); err != nil {
					errc <- err
					return
				}
				transactionID++

				if tt.wantAttachSet {
					if err := expectMBIMCommandWithService(server, transactionID, ServiceBasicConnect, CIDPacketService, CommandTypeSet, []byte{0, 0, 0, 0}); err != nil {
						errc <- err
						return
					}
					if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDPacketService, packetServicePayloadForTest(PacketServiceStateAttached))); err != nil {
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
				connectInfo := connectInfoPayloadExForTest(1, ActivationStateActivated, ContextIPTypeIPv4v6, ContextTypeIMS, DefaultIMSPDNAPN, pco)
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
				if _, err := server.Write(mbimCommandDone(transactionID, ServiceBasicConnect, CIDConnect, connectInfoPayloadForTest(1, ActivationStateDeactivated, ContextIPTypeIPv4v6, ContextTypeIMS))); err != nil {
					errc <- err
					return
				}
			}()

			reader := &Reader{conn: client, mbimExVersion: tt.mbimExVersion}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			session, err := reader.OpenIMSPDN(ctx, IMSPDNConfig{})
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

func TestReaderOpenIMSPDNValidatesSessionCapacity(t *testing.T) {
	tests := []struct {
		name        string
		maxSessions uint32
		sessionID   uint32
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

			reader := &Reader{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := reader.OpenIMSPDN(ctx, IMSPDNConfig{SessionID: tt.sessionID})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("OpenIMSPDN() error = %v, want %q", err, tt.want)
			}
			if err := <-errc; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func connectSetDataForVersionForTest(version uint16, sessionID uint32, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	switch {
	case version >= mbimExVersion40:
		return connectSetDataEx4ForTest(sessionID, command, ActivationOptionDefault, accessString, ipType, contextType)
	case version >= mbimExVersion30:
		return connectSetDataEx3ForTest(sessionID, command, accessString, ipType, contextType)
	default:
		return connectSetDataV1ForTest(sessionID, command, accessString, ipType, contextType)
	}
}

func connectSetDataV1ForTest(sessionID uint32, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	access := utf16Bytes(accessString)
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
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

func connectSetDataEx3ForTest(sessionID uint32, command ActivationCommand, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	data := connectSetDataExForTest(40, sessionID, command, ipType, contextType)
	return appendConnectSetDataTLVsForTest(data, accessString, "", "")
}

func connectSetDataEx4ForTest(sessionID uint32, command ActivationCommand, option ActivationOption, accessString string, ipType ContextIPType, contextType ContextType) []byte {
	data := connectSetDataExForTest(44, sessionID, command, ipType, contextType)
	binary.LittleEndian.PutUint32(data[40:44], uint32(option))
	return appendConnectSetDataTLVsForTest(data, accessString, "", "")
}

func connectSetDataExForTest(size int, sessionID uint32, command ActivationCommand, ipType ContextIPType, contextType ContextType) []byte {
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
	binary.LittleEndian.PutUint32(data[4:8], uint32(command))
	binary.LittleEndian.PutUint32(data[16:20], uint32(ipType))
	copy(data[20:36], contextType[:])
	binary.LittleEndian.PutUint32(data[36:40], uint32(AccessMediaType3GPP))
	return data
}

func appendConnectSetDataTLVsForTest(data []byte, values ...string) []byte {
	for _, value := range values {
		data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(value))...)
	}
	return data
}

func connectQueryDataForTest(sessionID uint32, size int) []byte {
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
	return data
}

func connectInfoPayloadForTest(sessionID uint32, state ActivationState, ipType ContextIPType, contextType ContextType) []byte {
	data := make([]byte, 36)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
	binary.LittleEndian.PutUint32(data[4:8], uint32(state))
	binary.LittleEndian.PutUint32(data[8:12], uint32(VoiceCallStateNone))
	binary.LittleEndian.PutUint32(data[12:16], uint32(ipType))
	copy(data[16:32], contextType[:])
	return data
}

func connectInfoPayloadExForTest(sessionID uint32, state ActivationState, ipType ContextIPType, contextType ContextType, accessString string, pco []byte) []byte {
	data := connectInfoPayloadForTest(sessionID, state, ipType, contextType)
	data = binary.LittleEndian.AppendUint32(data, uint32(AccessMediaType3GPP))
	data = append(data, mbimTLV(TLVTypeWCharString, utf16Bytes(accessString))...)
	data = append(data, mbimTLV(TLVTypePCO, pco)...)
	return data
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

func ipConfigurationQueryDataForTest(sessionID uint32) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
	return data
}

func ipConfigurationPayloadForTest(sessionID uint32, ipv4 net.IP, ipv4Prefix uint32, ipv6 net.IP, ipv6Prefix uint32) []byte {
	data := make([]byte, 60)
	binary.LittleEndian.PutUint32(data[:4], sessionID)
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
