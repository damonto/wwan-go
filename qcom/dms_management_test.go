package qcom

import (
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestDMSManagementRequests(t *testing.T) {
	reference := DMSTimeReferenceUser
	tests := []struct {
		name    string
		build   func() (Request, error)
		message MessageID
		wantTLV map[byte][]byte
	}{
		{
			name: "PRL version",
			build: func() (Request, error) {
				return (DMSGetPRLVersionRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSGetPRLVersion,
		},
		{
			name: "set time",
			build: func() (Request, error) {
				return (DMSSetTimeRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Milliseconds: 0x0102030405060708, Reference: reference,
				}).Request()
			},
			message: MessageDMSSetTime,
			wantTLV: map[byte][]byte{
				0x01: {0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01},
				0x10: {0, 0, 0, 0},
			},
		},
		{
			name: "get alternate network config",
			build: func() (Request, error) {
				return (DMSGetAltNetworkConfigRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			message: MessageDMSGetAltNetworkConfig,
		},
		{
			name: "set alternate network config",
			build: func() (Request, error) {
				return (DMSSetAltNetworkConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second, Enabled: true,
				}).Request(), nil
			},
			message: MessageDMSSetAltNetworkConfig,
			wantTLV: map[byte][]byte{0x01: {1}},
		},
		{
			name: "WLAN MAC address",
			build: func() (Request, error) {
				return (DMSGetMACAddressRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second, Device: DMSMACDeviceWLAN,
				}).Request()
			},
			message: MessageDMSGetMACAddress,
			wantTLV: map[byte][]byte{0x01: {0, 0, 0, 0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := tt.build()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if request.Service != ServiceDMS || request.ClientID != 7 || request.TransactionID != 9 || request.MessageID != tt.message {
				t.Fatalf("Request() = %+v", request)
			}
			if request.Timeout != time.Second || len(request.TLVs) != len(tt.wantTLV) {
				t.Fatalf("Request() timeout = %v, TLVs = %v", request.Timeout, request.TLVs)
			}
			for kind, want := range tt.wantTLV {
				assertTLV(t, request.TLVs, kind, want)
			}
		})
	}
}

func TestDMSManagementRequestsRejectInvalidEnums(t *testing.T) {
	tests := []struct {
		name  string
		build func() error
	}{
		{
			name: "time reference",
			build: func() error {
				invalid := DMSTimeReference(1)
				_, err := (DMSSetTimeRequest{Reference: invalid}).Request()
				return err
			},
		},
		{
			name: "MAC device",
			build: func() error {
				_, err := (DMSGetMACAddressRequest{Device: DMSMACDevice(2)}).Request()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.build(); err == nil {
				t.Fatal("Request() error = nil")
			}
		})
	}
}

func TestDMSGetPRLVersionResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    DMSPRLVersion
		wantErr string
	}{
		{
			name: "version and preference",
			tlvs: tlv.TLVs{tlv.Uint(0x01, uint16(0x1234)), tlv.Uint(0x10, uint8(1))},
			want: DMSPRLVersion{Version: 0x1234, PRLOnly: true, PRLOnlyKnown: true},
		},
		{name: "version only", tlvs: tlv.TLVs{tlv.Uint(0x01, uint16(7))}, want: DMSPRLVersion{Version: 7}},
		{name: "missing version", wantErr: "missing"},
		{name: "version length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1})}, wantErr: "version TLV length"},
		{
			name:    "preference length",
			tlvs:    tlv.TLVs{tlv.Uint(0x01, uint16(7)), tlv.Bytes(0x10, []byte{1, 2})},
			wantErr: "PRL-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSGetPRLVersionResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Info != tt.want {
				t.Fatalf("info = %+v, want %+v", got.Info, tt.want)
			}
		})
	}
}

func TestDMSGetAltNetworkConfigResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    bool
		wantErr string
	}{
		{name: "enabled", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(1))}, want: true},
		{name: "disabled", tlvs: tlv.TLVs{tlv.Uint(0x01, uint8(0))}},
		{name: "missing", wantErr: "missing"},
		{name: "wrong length", tlvs: tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})}, wantErr: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSGetAltNetworkConfigResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Enabled != tt.want {
				t.Fatalf("enabled = %v, want %v", got.Enabled, tt.want)
			}
		})
	}
}

func TestDMSGetMACAddressResponseUnmarshalTLVs(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    net.HardwareAddr
		known   bool
		wantErr string
	}{
		{
			name: "six byte address",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{6, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55})},
			want: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, known: true,
		},
		{name: "omitted"},
		{name: "length missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: "missing"},
		{name: "zero length", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0})}, wantErr: "out of range"},
		{name: "too long", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{dmsMACAddressMax + 1})}, wantErr: "out of range"},
		{name: "truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{6, 1})}, wantErr: "TLV length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DMSGetMACAddressResponse
			err := got.UnmarshalTLVs(tt.tlvs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalTLVs() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTLVs() error = %v", err)
			}
			if got.Known != tt.known || !reflect.DeepEqual(got.Address, tt.want) {
				t.Fatalf("response = %+v, want address %v known %v", got, tt.want, tt.known)
			}
		})
	}
}

func TestClientDMSManagement(t *testing.T) {
	tests := []struct {
		name    string
		message MessageID
		check   func(*testing.T, Request)
		resp    Response
		run     func(context.Context, *Client) error
	}{
		{
			name: "PRL version", message: MessageDMSGetPRLVersion,
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs = %v, want empty", req.TLVs)
				}
			},
			resp: successResponse(MessageDMSGetPRLVersion, tlv.Uint(0x01, uint16(7))),
			run: func(ctx context.Context, client *Client) error {
				info, err := client.DMSPRLVersion(ctx)
				if err == nil && info.Version != 7 {
					t.Fatalf("DMSPRLVersion() = %+v", info)
				}
				return err
			},
		},
		{
			name: "set time", message: MessageDMSSetTime,
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{9, 0, 0, 0, 0, 0, 0, 0})
				assertTLV(t, req.TLVs, 0x10, []byte{0, 0, 0, 0})
			},
			resp: successResponse(MessageDMSSetTime),
			run:  func(ctx context.Context, client *Client) error { return client.DMSSetTime(ctx, 9) },
		},
		{
			name: "get alternate network", message: MessageDMSGetAltNetworkConfig,
			check: func(t *testing.T, req Request) {},
			resp:  successResponse(MessageDMSGetAltNetworkConfig, tlv.Uint(0x01, uint8(1))),
			run: func(ctx context.Context, client *Client) error {
				enabled, err := client.DMSAltNetworkConfig(ctx)
				if err == nil && !enabled {
					t.Fatal("DMSAltNetworkConfig() = false")
				}
				return err
			},
		},
		{
			name: "set alternate network", message: MessageDMSSetAltNetworkConfig,
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1}) },
			resp:  successResponse(MessageDMSSetAltNetworkConfig),
			run:   func(ctx context.Context, client *Client) error { return client.DMSSetAltNetworkConfig(ctx, true) },
		},
		{
			name: "Bluetooth MAC", message: MessageDMSGetMACAddress,
			check: func(t *testing.T, req Request) { assertTLV(t, req.TLVs, 0x01, []byte{1, 0, 0, 0}) },
			resp:  successResponse(MessageDMSGetMACAddress, tlv.Bytes(0x10, []byte{6, 0, 1, 2, 3, 4, 5})),
			run: func(ctx context.Context, client *Client) error {
				address, err := client.DMSMACAddress(ctx, DMSMACDeviceBluetooth)
				if err == nil && !reflect.DeepEqual(address, net.HardwareAddr{0, 1, 2, 3, 4, 5}) {
					t.Fatalf("DMSMACAddress() = %v", address)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceDMS || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = %+v", req)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceDMS: 7}}
			if err := tt.run(context.Background(), client); err != nil {
				t.Fatalf("client call error = %v", err)
			}
		})
	}
}
