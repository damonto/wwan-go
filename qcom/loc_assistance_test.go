package qcom

import (
	"encoding/binary"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestLOCAssistanceRequestEncoding(t *testing.T) {
	ipv4 := LOCIPv4Server{Address: netip.MustParseAddr("192.0.2.9"), Port: 7275}
	ipv6 := LOCIPv6Server{Address: netip.MustParseAddr("2001:db8::9"), Port: 7276}
	url := "supl.example.test"
	addressTypes := LOCServerAddressIPv4 | LOCServerAddressIPv6 | LOCServerAddressURL
	format := LOCPredictedOrbitsDataFormatXTRA
	part := LOCAssistanceDataPart{TotalSize: 1500, TotalParts: 2, PartNumber: 1, Data: []byte{1, 2, 3}}
	ipv4Value := []byte{192, 0, 2, 9}
	ipv4Value = binary.LittleEndian.AppendUint16(ipv4Value, ipv4.Port)
	ipv6Bytes := ipv6.Address.As16()
	ipv6Value := append([]byte(nil), ipv6Bytes[:]...)
	ipv6Value = binary.LittleEndian.AppendUint32(ipv6Value, ipv6.Port)
	partValue := binary.LittleEndian.AppendUint16(nil, uint16(len(part.Data)))
	partValue = append(partValue, part.Data...)

	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "set server",
			request: func() (Request, error) {
				return (LOCSetServerRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Config: LOCServerConfig{
						Type: LOCServerUMTSSLP,
						IPv4: &ipv4,
						IPv6: &ipv6,
						URL:  &url,
					},
				}).Request()
			},
			wantMessage: MessageLOCSetServer,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(LOCServerUMTSSLP)),
				0x10: ipv4Value,
				0x11: ipv6Value,
				0x12: []byte(url),
			},
		},
		{
			name: "get server",
			request: func() (Request, error) {
				return (LOCGetServerRequest{
					ClientID:      7,
					TransactionID: 9,
					Timeout:       3 * time.Second,
					Query: LOCServerQuery{
						Type:         LOCServerUMTSSLP,
						AddressTypes: &addressTypes,
					},
				}).Request()
			},
			wantMessage: MessageLOCGetServer,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(LOCServerUMTSSLP)),
				0x10: {byte(addressTypes)},
			},
		},
		{
			name: "get predicted orbits source",
			request: func() (Request, error) {
				return (LOCGetPredictedOrbitsDataSourceRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
				}).Request(), nil
			},
			wantMessage: MessageLOCGetPredictedOrbitsDataSource,
		},
		{
			name: "inject predicted orbits part",
			request: func() (Request, error) {
				return (LOCInjectPredictedOrbitsDataRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Part: part, Format: &format,
				}).Request()
			},
			wantMessage: MessageLOCInjectPredictedOrbitsData,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, part.TotalSize),
				0x02: binary.LittleEndian.AppendUint16(nil, part.TotalParts),
				0x03: binary.LittleEndian.AppendUint16(nil, part.PartNumber),
				0x04: partValue,
				0x10: binary.LittleEndian.AppendUint32(nil, uint32(format)),
			},
		},
		{
			name: "inject XTRA part",
			request: func() (Request, error) {
				return (LOCInjectXTRADataRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Part: part,
				}).Request()
			},
			wantMessage: MessageLOCInjectXTRAData,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, part.TotalSize),
				0x02: binary.LittleEndian.AppendUint16(nil, part.TotalParts),
				0x03: binary.LittleEndian.AppendUint16(nil, part.PartNumber),
				0x04: partValue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceLOC || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
			}
			if got.Timeout != 3*time.Second {
				t.Fatalf("Timeout = %v, want 3s", got.Timeout)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				assertTLV(t, got.TLVs, kind, want)
			}
		})
	}
}

func TestLOCAssistanceRequestValidation(t *testing.T) {
	invalidAddressTypes := LOCServerAddressType(1 << 3)
	invalidFormat := LOCPredictedOrbitsDataFormat(1)
	tooLongURL := strings.Repeat("x", locServerURLMaxLength+1)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "set server type above range",
			call: func() error {
				_, err := (LOCSetServerRequest{Config: LOCServerConfig{Type: 5}}).Request()
				return err
			},
		},
		{
			name: "get server type above range",
			call: func() error {
				_, err := (LOCGetServerRequest{Query: LOCServerQuery{Type: 5}}).Request()
				return err
			},
		},
		{
			name: "get server address mask above range",
			call: func() error {
				_, err := (LOCGetServerRequest{Query: LOCServerQuery{AddressTypes: &invalidAddressTypes}}).Request()
				return err
			},
		},
		{
			name: "IPv4 field contains IPv6",
			call: func() error {
				server := LOCIPv4Server{Address: netip.MustParseAddr("2001:db8::1")}
				_, err := (LOCSetServerRequest{Config: LOCServerConfig{IPv4: &server}}).Request()
				return err
			},
		},
		{
			name: "IPv6 field contains IPv4",
			call: func() error {
				server := LOCIPv6Server{Address: netip.MustParseAddr("192.0.2.1")}
				_, err := (LOCSetServerRequest{Config: LOCServerConfig{IPv6: &server}}).Request()
				return err
			},
		},
		{
			name: "server URL too long",
			call: func() error {
				_, err := (LOCSetServerRequest{Config: LOCServerConfig{URL: &tooLongURL}}).Request()
				return err
			},
		},
		{
			name: "predicted orbits part too large",
			call: func() error {
				_, err := (LOCInjectPredictedOrbitsDataRequest{
					Part: LOCAssistanceDataPart{Data: make([]byte, locAssistanceDataPartMax+1)},
				}).Request()
				return err
			},
		},
		{
			name: "XTRA part too large",
			call: func() error {
				_, err := (LOCInjectXTRADataRequest{
					Part: LOCAssistanceDataPart{Data: make([]byte, locAssistanceDataPartMax+1)},
				}).Request()
				return err
			},
		},
		{
			name: "predicted orbits format unsupported",
			call: func() error {
				_, err := (LOCInjectPredictedOrbitsDataRequest{Format: &invalidFormat}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("call() error = nil, want non-nil")
			}
		})
	}
}

func TestLOCGetServerIndicationUnmarshalTLVs(t *testing.T) {
	url := "supl.example.test"
	ipv4 := LOCIPv4Server{Address: netip.MustParseAddr("198.51.100.8"), Port: 7275}
	ipv6 := LOCIPv6Server{Address: netip.MustParseAddr("2001:db8::8"), Port: 7276}
	ipv4Value := []byte{198, 51, 100, 8}
	ipv4Value = binary.LittleEndian.AppendUint16(ipv4Value, ipv4.Port)
	ipv6Bytes := ipv6.Address.As16()
	ipv6Value := append([]byte(nil), ipv6Bytes[:]...)
	ipv6Value = binary.LittleEndian.AppendUint32(ipv6Value, ipv6.Port)
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	want := LOCAssistanceServer{
		Type:      LOCServerUMTSSLP,
		TypeKnown: true,
		IPv4:      &ipv4,
		IPv6:      &ipv6,
		URL:       &url,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    LOCAssistanceServer
		wantErr bool
	}{
		{name: "status only", tlvs: tlv.TLVs{success}},
		{
			name: "all server fields",
			tlvs: tlv.TLVs{
				success,
				tlv.Uint(0x02, uint32(LOCServerUMTSSLP)),
				tlv.Bytes(0x10, ipv4Value),
				tlv.Bytes(0x11, ipv6Value),
				tlv.Bytes(0x12, []byte(url)),
			},
			want: want,
		},
		{name: "server type truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x02, []byte{1})}, wantErr: true},
		{name: "IPv4 truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x10, ipv4Value[:5])}, wantErr: true},
		{name: "IPv6 truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x11, ipv6Value[:19])}, wantErr: true},
		{name: "URL too long", tlvs: tlv.TLVs{success, tlv.Bytes(0x12, make([]byte, locServerURLMaxLength+1))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LOCGetServerIndication
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
			if !reflect.DeepEqual(got.Server, tt.want) {
				t.Fatalf("Server = %+v, want %+v", got.Server, tt.want)
			}
		})
	}
}

func TestLOCPredictedOrbitsSourceIndicationUnmarshalTLVs(t *testing.T) {
	allowedSizes := binary.LittleEndian.AppendUint32(nil, 4096)
	allowedSizes = binary.LittleEndian.AppendUint32(allowedSizes, 1024)
	serverList := []byte{2, 3, 'o', 'n', 'e', 3, 't', 'w', 'o'}
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	want := LOCPredictedOrbitsSource{
		MaxFileSize:       4096,
		MaxPartSize:       1024,
		AllowedSizesKnown: true,
		Servers:           []string{"one", "two"},
		ServersKnown:      true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		wantErr bool
	}{
		{
			name: "complete",
			tlvs: tlv.TLVs{success, tlv.Bytes(0x10, allowedSizes), tlv.Bytes(0x11, serverList)},
		},
		{name: "allowed sizes truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x10, allowedSizes[:7])}, wantErr: true},
		{name: "server count missing", tlvs: tlv.TLVs{success, tlv.Bytes(0x11, nil)}, wantErr: true},
		{name: "server length missing", tlvs: tlv.TLVs{success, tlv.Bytes(0x11, []byte{1})}, wantErr: true},
		{name: "server truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x11, []byte{1, 2, 'x'})}, wantErr: true},
		{name: "server trailing data", tlvs: tlv.TLVs{success, tlv.Bytes(0x11, []byte{0, 1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LOCGetPredictedOrbitsDataSourceIndication
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
			if !reflect.DeepEqual(got.Source, want) {
				t.Fatalf("Source = %+v, want %+v", got.Source, want)
			}
		})
	}
}

func TestLOCAssistanceDataPartIndicationUnmarshalTLVs(t *testing.T) {
	success := tlv.Uint(0x01, uint32(LOCIndicationSuccess))
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    LOCAssistanceDataPartResult
		wantErr bool
	}{
		{name: "status only", tlvs: tlv.TLVs{success}},
		{
			name: "part acknowledged",
			tlvs: tlv.TLVs{success, tlv.Uint(0x10, uint16(7))},
			want: LOCAssistanceDataPartResult{PartNumber: 7, PartNumberKnown: true},
		},
		{name: "part number truncated", tlvs: tlv.TLVs{success, tlv.Bytes(0x10, []byte{1})}, wantErr: true},
		{name: "part number trailing data", tlvs: tlv.TLVs{success, tlv.Bytes(0x10, []byte{1, 0, 0})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got LOCAssistanceDataPartIndication
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
			if got.Part != tt.want {
				t.Fatalf("Part = %+v, want %+v", got.Part, tt.want)
			}
		})
	}
}
