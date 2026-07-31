package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDSDAPNRequestEncoding(t *testing.T) {
	preferred := DSDAPNTypePreferenceDefault | DSDAPNTypePreferenceMMS
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "get emergency APN",
			request: func() (Request, error) {
				return (DSDGetAPNInfoRequest{ClientID: 7, TransactionID: 9, Type: DSDAPNTypeEmergency}).Request()
			},
			wantMessage: MessageDSDGetAPNInfo,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(DSDAPNTypeEmergency)),
			},
		},
		{
			name: "set APN types and preference",
			request: func() (Request, error) {
				return (DSDSetAPNTypeRequest{
					ClientID:      7,
					TransactionID: 9,
					Config: DSDAPNTypeConfig{
						Name:           "internet",
						Types:          DSDAPNTypePreferenceDefault | DSDAPNTypePreferenceMMS | DSDAPNTypePreferenceDUN,
						PreferredTypes: &preferred,
					},
				}).Request()
			},
			wantMessage: MessageDSDSetAPNType,
			wantTLVs: map[byte][]byte{
				0x01: append(
					[]byte{8, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'},
					binary.LittleEndian.AppendUint64(nil, uint64(DSDAPNTypePreferenceDefault|DSDAPNTypePreferenceMMS|DSDAPNTypePreferenceDUN))...,
				),
				0x10: binary.LittleEndian.AppendUint64(nil, uint64(preferred)),
			},
		},
		{
			name: "set APN types without preference",
			request: func() (Request, error) {
				return (DSDSetAPNTypeRequest{ClientID: 7, TransactionID: 9, Config: DSDAPNTypeConfig{
					Name:  "ims",
					Types: DSDAPNTypePreferenceIMS,
				}}).Request()
			},
			wantMessage: MessageDSDSetAPNType,
			wantTLVs: map[byte][]byte{
				0x01: append([]byte{3, 'i', 'm', 's'}, binary.LittleEndian.AppendUint64(nil, uint64(DSDAPNTypePreferenceIMS))...),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServiceDSD || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = service 0x%X client %d transaction %d message 0x%04X", got.Service, got.ClientID, got.TransactionID, got.MessageID)
			}
			if len(got.TLVs) != len(tt.wantTLVs) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLVs))
			}
			for kind, want := range tt.wantTLVs {
				value, ok := tlv.Value(got.TLVs, kind)
				if !ok {
					t.Fatalf("TLV 0x%02X missing", kind)
				}
				if !bytes.Equal(value, want) {
					t.Fatalf("TLV 0x%02X = % X, want % X", kind, value, want)
				}
			}
		})
	}
}

func TestDSDAPNRequestValidation(t *testing.T) {
	unsupported := DSDAPNTypePreference(1 << 10)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "APN type out of range",
			call: func() error {
				_, err := (DSDGetAPNInfoRequest{Type: DSDAPNTypeEmergency + 1}).Request()
				return err
			},
		},
		{
			name: "APN name contains NUL",
			call: func() error {
				_, err := (DSDSetAPNTypeRequest{Config: DSDAPNTypeConfig{Name: "inter\x00net"}}).Request()
				return err
			},
		},
		{
			name: "APN name too long",
			call: func() error {
				_, err := (DSDSetAPNTypeRequest{Config: DSDAPNTypeConfig{Name: strings.Repeat("a", dsdAPNMaxLength+1)}}).Request()
				return err
			},
		},
		{
			name: "APN type mask contains unknown bit",
			call: func() error {
				_, err := (DSDSetAPNTypeRequest{Config: DSDAPNTypeConfig{Name: "internet", Types: unsupported}}).Request()
				return err
			},
		},
		{
			name: "preferred mask contains unknown bit",
			call: func() error {
				_, err := (DSDSetAPNTypeRequest{Config: DSDAPNTypeConfig{Name: "internet", PreferredTypes: &unsupported}}).Request()
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

func TestDSDGetAPNInfoResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DSDAPNInfo
		wantErr bool
	}{
		{
			name: "APN name",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte("internet"))},
			want: DSDAPNInfo{Name: "internet", NameKnown: true},
		},
		{name: "APN name absent"},
		{name: "APN name contains NUL", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte("inter\x00net"))}, wantErr: true},
		{name: "APN name too long", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte(strings.Repeat("a", dsdAPNMaxLength+1)))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response DSDGetAPNInfoResponse
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
			if response.Info != tt.want {
				t.Fatalf("Info = %+v, want %+v", response.Info, tt.want)
			}
		})
	}
}

func TestDSDAPNClientMethods(t *testing.T) {
	preferred := DSDAPNTypePreferenceDefault
	tests := []struct {
		name    string
		message MessageID
		call    func(*testing.T, *Client) error
		check   func(*testing.T, Request)
		resp    Response
	}{
		{
			name:    "get APN info",
			message: MessageDSDGetAPNInfo,
			call: func(t *testing.T, c *Client) error {
				got, err := c.DSDAPNInfo(context.Background(), DSDAPNTypeDefault)
				if err == nil && got != (DSDAPNInfo{Name: "internet", NameKnown: true}) {
					t.Fatalf("info = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
			},
			resp: successResponse(MessageDSDGetAPNInfo, tlv.Bytes(0x10, []byte("internet"))),
		},
		{
			name:    "set APN type",
			message: MessageDSDSetAPNType,
			call: func(_ *testing.T, c *Client) error {
				return c.DSDSetAPNType(context.Background(), DSDAPNTypeConfig{
					Name:           "internet",
					Types:          DSDAPNTypePreferenceDefault,
					PreferredTypes: &preferred,
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, append([]byte{8, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'}, 1, 0, 0, 0, 0, 0, 0, 0))
				assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 0, 0, 0, 0, 0, 0})
			},
			resp: successResponse(MessageDSDSetAPNType),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDSD || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X, client %d, message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDSD: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}
