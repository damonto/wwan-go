//go:build linux

package qmi

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNewRMNetLinkMessage(t *testing.T) {
	tests := []struct {
		name  string
		flags uint32
	}{
		{name: "deaggregation only", flags: rmnetFlagIngressDeaggregation},
		{
			name: "MAPv4 checksum offload",
			flags: rmnetFlagIngressDeaggregation |
				rmnetFlagIngressMAPChecksumV4 |
				rmnetFlagEgressMAPChecksumV4,
		},
		{
			name: "MAPv5 checksum offload",
			flags: rmnetFlagIngressDeaggregation |
				rmnetFlagIngressMAPChecksumV5 |
				rmnetFlagEgressMAPChecksumV5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := newRMNetLinkMessage(17, "qmap11.2", 3, tt.flags)
			if got := binary.NativeEndian.Uint32(message[0:4]); got != uint32(len(message)) {
				t.Errorf("nlmsg_len = %d, want %d", got, len(message))
			}
			if got := binary.NativeEndian.Uint16(message[4:6]); got != unix.RTM_NEWLINK {
				t.Errorf("nlmsg_type = %d, want RTM_NEWLINK", got)
			}
			wantHeaderFlags := uint16(unix.NLM_F_REQUEST | unix.NLM_F_ACK | unix.NLM_F_CREATE | unix.NLM_F_EXCL)
			if got := binary.NativeEndian.Uint16(message[6:8]); got != wantHeaderFlags {
				t.Errorf("nlmsg_flags = %#x, want %#x", got, wantHeaderFlags)
			}
			if got := message[netlinkHeaderLength]; got != unix.AF_UNSPEC {
				t.Errorf("ifi_family = %d, want AF_UNSPEC", got)
			}
			if got := binary.NativeEndian.Uint16(message[netlinkHeaderLength+2 : netlinkHeaderLength+4]); got != unix.ARPHRD_RAWIP {
				t.Errorf("ifi_type = %d, want ARPHRD_RAWIP", got)
			}
			if got := binary.NativeEndian.Uint32(message[netlinkHeaderLength+12 : netlinkHeaderLength+16]); got != math.MaxUint32 {
				t.Errorf("ifi_change = %#x, want %#x", got, uint32(math.MaxUint32))
			}

			attrs := parseRouteAttributes(t, message[netlinkHeaderLength+ifInfoMessageLength:])
			if got := binary.NativeEndian.Uint32(attrs[unix.IFLA_LINK]); got != 17 {
				t.Errorf("IFLA_LINK = %d, want 17", got)
			}
			if got := string(attrs[unix.IFLA_IFNAME]); got != "qmap11.2" {
				t.Errorf("IFLA_IFNAME = %q, want %q", got, "qmap11.2")
			}

			linkInfo := parseRouteAttributes(t, attrs[unix.IFLA_LINKINFO])
			if got := string(linkInfo[unix.IFLA_INFO_KIND]); got != "rmnet" {
				t.Errorf("IFLA_INFO_KIND = %q, want rmnet", got)
			}
			dataInfo := parseRouteAttributes(t, linkInfo[unix.IFLA_INFO_DATA])
			if got := binary.NativeEndian.Uint16(dataInfo[unix.IFLA_RMNET_MUX_ID]); got != 3 {
				t.Errorf("IFLA_RMNET_MUX_ID = %d, want 3", got)
			}
			gotFlags := dataInfo[unix.IFLA_RMNET_FLAGS]
			if len(gotFlags) != 8 {
				t.Fatalf("IFLA_RMNET_FLAGS length = %d, want 8", len(gotFlags))
			}
			if got := binary.NativeEndian.Uint32(gotFlags[:4]); got != tt.flags {
				t.Errorf("rmnet flags = %#x, want %#x", got, tt.flags)
			}
			if got := binary.NativeEndian.Uint32(gotFlags[4:]); got != rmnetFlagMask {
				t.Errorf("rmnet mask = %#x, want %#x", got, uint32(rmnetFlagMask))
			}
		})
	}
}

func TestRMNetLinkName(t *testing.T) {
	tests := []struct {
		name      string
		baseIndex int
		muxID     uint8
		want      string
	}{
		{name: "first mux", baseIndex: 17, muxID: 1, want: "qmap11.0"},
		{name: "maximum values", baseIndex: math.MaxInt32, muxID: rmnetMuxIDMax, want: "qmap7fffffff.fd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rmnetLinkName(tt.baseIndex, tt.muxID)
			if got != tt.want {
				t.Errorf("rmnetLinkName() = %q, want %q", got, tt.want)
			}
			if len(got) >= unix.IFNAMSIZ {
				t.Errorf("rmnetLinkName() length = %d, want at most %d", len(got), unix.IFNAMSIZ-1)
			}
		})
	}
}

func TestNewSetLinkUpMessage(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "base interface", index: 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := newSetLinkUpMessage(tt.index)
			if got := binary.NativeEndian.Uint16(message[4:6]); got != unix.RTM_NEWLINK {
				t.Errorf("nlmsg_type = %d, want RTM_NEWLINK", got)
			}
			if got := int32(binary.NativeEndian.Uint32(message[netlinkHeaderLength+4 : netlinkHeaderLength+8])); got != int32(tt.index) {
				t.Errorf("ifi_index = %d, want %d", got, tt.index)
			}
			if got := binary.NativeEndian.Uint32(message[netlinkHeaderLength+8 : netlinkHeaderLength+12]); got != unix.IFF_UP {
				t.Errorf("ifi_flags = %#x, want IFF_UP", got)
			}
			if got := binary.NativeEndian.Uint32(message[netlinkHeaderLength+12 : netlinkHeaderLength+16]); got != unix.IFF_UP {
				t.Errorf("ifi_change = %#x, want IFF_UP", got)
			}
		})
	}
}

func TestRouteNetlinkACK(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		sequence    uint32
		wantMatched bool
		wantErr     error
	}{
		{name: "success", data: netlinkACKMessage(7, 0), sequence: 7, wantMatched: true},
		{name: "kernel error", data: netlinkACKMessage(7, -int32(unix.EEXIST)), sequence: 7, wantMatched: true, wantErr: unix.EEXIST},
		{name: "other sequence", data: netlinkACKMessage(8, 0), sequence: 7},
		{name: "short error payload", data: netlinkMessage(unix.NLMSG_ERROR, 7, nil), sequence: 7, wantMatched: true, wantErr: errors.New("short payload")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := routeNetlinkACK(tt.data, tt.sequence)
			if matched != tt.wantMatched {
				t.Errorf("routeNetlinkACK() matched = %v, want %v", matched, tt.wantMatched)
			}
			if tt.wantErr == nil && err != nil {
				t.Errorf("routeNetlinkACK() error = %v", err)
			}
			if tt.wantErr != nil && err == nil {
				t.Fatalf("routeNetlinkACK() error = nil, want %v", tt.wantErr)
			}
			if errors.Is(tt.wantErr, unix.EEXIST) && !errors.Is(err, unix.EEXIST) {
				t.Errorf("routeNetlinkACK() error = %v, want EEXIST", err)
			}
		})
	}
}

func parseRouteAttributes(t *testing.T, data []byte) map[int][]byte {
	t.Helper()
	attrs := make(map[int][]byte)
	for len(data) > 0 {
		if len(data) < routeAttrHeaderSize {
			t.Fatalf("attribute buffer length = %d, want at least %d", len(data), routeAttrHeaderSize)
		}
		length := int(binary.NativeEndian.Uint16(data[:2]))
		if length < routeAttrHeaderSize || length > len(data) {
			t.Fatalf("attribute length = %d, buffer = %d", length, len(data))
		}
		attrType := int(binary.NativeEndian.Uint16(data[2:4]))
		attrs[attrType] = append([]byte(nil), data[routeAttrHeaderSize:length]...)
		alignedLength := alignRouteMessage(length)
		if alignedLength > len(data) {
			t.Fatalf("aligned attribute length = %d, buffer = %d", alignedLength, len(data))
		}
		data = data[alignedLength:]
	}
	return attrs
}

func netlinkACKMessage(sequence uint32, code int32) []byte {
	payload := make([]byte, 4)
	binary.NativeEndian.PutUint32(payload, uint32(code))
	return netlinkMessage(unix.NLMSG_ERROR, sequence, payload)
}

func netlinkMessage(messageType uint16, sequence uint32, payload []byte) []byte {
	message := make([]byte, netlinkHeaderLength+len(payload))
	binary.NativeEndian.PutUint32(message[0:4], uint32(len(message)))
	binary.NativeEndian.PutUint16(message[4:6], messageType)
	binary.NativeEndian.PutUint32(message[8:12], sequence)
	copy(message[netlinkHeaderLength:], payload)
	return message
}

func TestRouteAttributeEncoding(t *testing.T) {
	tests := []struct {
		name      string
		attrType  int
		value     []byte
		wantBytes []byte
	}{
		{
			name:      "string excludes trailing NUL",
			attrType:  unix.IFLA_INFO_KIND,
			value:     []byte("rmnet"),
			wantBytes: []byte("rmnet"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := parseRouteAttributes(t, appendRouteAttribute(nil, tt.attrType, tt.value))
			if !reflect.DeepEqual(attrs[tt.attrType], tt.wantBytes) {
				t.Errorf("attribute value = %v, want %v", attrs[tt.attrType], tt.wantBytes)
			}
		})
	}
}
