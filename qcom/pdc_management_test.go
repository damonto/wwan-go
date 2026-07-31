package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestPDCManagementRequestEncoding(t *testing.T) {
	token := uint32(0x10203040)
	storage := PDCStorageRemote
	subscription := uint32(2)
	chunk := []byte{0xAA, 0xBB, 0xCC}
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "reset",
			request: func() (Request, error) {
				return (PDCResetRequest{ClientID: 7, TransactionID: 9, Timeout: time.Second}).Request(), nil
			},
			wantMessage: MessagePDCReset,
		},
		{
			name: "delete configuration",
			request: func() (Request, error) {
				return (PDCDeleteConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, Token: &token,
				}).Request()
			},
			wantMessage: MessagePDCDeleteConfig,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(PDCConfigurationSoftware)),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
				0x11: {2, 'i', 'd'},
			},
		},
		{
			name: "delete without configuration ID",
			request: func() (Request, error) {
				return (PDCDeleteConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config: PDCConfig{Type: PDCConfigurationDatabase}, Token: &token,
				}).Request()
			},
			wantMessage: MessagePDCDeleteConfig,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(PDCConfigurationDatabase)),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
			},
		},
		{
			name: "load configuration frame",
			request: func() (Request, error) {
				return (PDCLoadConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config:    PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
					TotalSize: 9, Chunk: chunk, Token: &token, Storage: &storage,
				}).Request()
			},
			wantMessage: MessagePDCLoadConfig,
			wantTLVs: map[byte][]byte{
				0x01: append(
					[]byte{1, 0, 0, 0, 2, 'i', 'd', 9, 0, 0, 0, 3, 0},
					chunk...,
				),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
				0x11: binary.LittleEndian.AppendUint32(nil, uint32(PDCStorageRemote)),
			},
		},
		{
			name: "get configuration limits",
			request: func() (Request, error) {
				return (PDCGetConfigLimitsRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Type: PDCConfigurationDatabase, Token: &token,
				}).Request()
			},
			wantMessage: MessagePDCGetConfigLimits,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(PDCConfigurationDatabase)),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
			},
		},
		{
			name: "get default configuration info",
			request: func() (Request, error) {
				return (PDCGetDefaultConfigInfoRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Type: PDCConfigurationPlatform, Token: &token,
				}).Request()
			},
			wantMessage: MessagePDCGetDefaultConfigInfo,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(PDCConfigurationPlatform)),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
			},
		},
		{
			name: "deactivate configuration",
			request: func() (Request, error) {
				return (PDCDeactivateConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Type: PDCConfigurationSoftware, Token: &token, Subscription: &subscription,
				}).Request()
			},
			wantMessage: MessagePDCDeactivateConfig,
			wantTLVs: map[byte][]byte{
				0x01: binary.LittleEndian.AppendUint32(nil, uint32(PDCConfigurationSoftware)),
				0x10: binary.LittleEndian.AppendUint32(nil, token),
				0x11: binary.LittleEndian.AppendUint32(nil, subscription),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServicePDC || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
				t.Fatalf("Request() = %+v", got)
			}
			if got.Timeout != time.Second {
				t.Fatalf("Timeout = %v, want 1s", got.Timeout)
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

func TestPDCManagementRequestValidation(t *testing.T) {
	invalidStorage := PDCStorage(2)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "delete invalid type",
			call: func() error {
				_, err := (PDCDeleteConfigRequest{Config: PDCConfig{Type: 2}}).Request()
				return err
			},
		},
		{
			name: "load zero total size",
			call: func() error {
				_, err := (PDCLoadConfigRequest{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, Chunk: []byte{1}}).Request()
				return err
			},
		},
		{
			name: "load empty frame",
			call: func() error {
				_, err := (PDCLoadConfigRequest{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, TotalSize: 1}).Request()
				return err
			},
		},
		{
			name: "load frame too large",
			call: func() error {
				_, err := (PDCLoadConfigRequest{
					Config:    PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
					TotalSize: pdcLoadFrameMax + 1, Chunk: make([]byte, pdcLoadFrameMax+1),
				}).Request()
				return err
			},
		},
		{
			name: "load frame exceeds total",
			call: func() error {
				_, err := (PDCLoadConfigRequest{
					Config:    PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
					TotalSize: 1, Chunk: []byte{1, 2},
				}).Request()
				return err
			},
		},
		{
			name: "load storage out of range",
			call: func() error {
				_, err := (PDCLoadConfigRequest{
					Config:    PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
					TotalSize: 1, Chunk: []byte{1}, Storage: &invalidStorage,
				}).Request()
				return err
			},
		},
		{
			name: "limits type unsupported",
			call: func() error {
				_, err := (PDCGetConfigLimitsRequest{Type: 2}).Request()
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

func TestPDCManagementIndicationsUnmarshalTLVs(t *testing.T) {
	base := pdcIndicationTLVs(QMIErrorNone, 7)
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "load response frame reset",
			run: func(t *testing.T) {
				var got PDCLoadConfigResponse
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint8(1))}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if !got.FrameResetKnown || !got.FrameReset {
					t.Fatalf("response = %+v", got)
				}
			},
		},
		{
			name: "load indication",
			run: func(t *testing.T) {
				var got PDCLoadConfigIndication
				tlvs := append(base,
					tlv.Uint(0x11, uint32(1024)),
					tlv.Uint(0x12, uint32(3)),
					tlv.Uint(0x13, uint8(0)),
				)
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.Result.Token != 7 || !got.ReceivedKnown || got.Received != 1024 ||
					!got.RemainingKnown || got.Remaining != 3 || !got.FrameResetKnown || got.FrameReset {
					t.Fatalf("indication = %+v", got)
				}
			},
		},
		{
			name: "configuration limits",
			run: func(t *testing.T) {
				var got PDCGetConfigLimitsIndication
				tlvs := append(base,
					tlv.Bytes(0x11, binary.LittleEndian.AppendUint64(nil, 1<<32)),
					tlv.Bytes(0x12, binary.LittleEndian.AppendUint64(nil, 1234)),
				)
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := PDCConfigLimits{Maximum: 1 << 32, MaximumKnown: true, Current: 1234, CurrentKnown: true}
				if got.Limits != want {
					t.Fatalf("Limits = %+v, want %+v", got.Limits, want)
				}
			},
		},
		{
			name: "default configuration info",
			run: func(t *testing.T) {
				var got PDCGetDefaultConfigInfoIndication
				tlvs := append(base,
					tlv.Uint(0x11, uint32(0x10203040)),
					tlv.Uint(0x12, uint32(4096)),
					tlv.Bytes(0x13, []byte{3, 'm', 'b', 'n'}),
				)
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := PDCConfigInfo{
					Version: 0x10203040, VersionKnown: true,
					Size: 4096, SizeKnown: true,
					Description: "mbn", DescriptionKnown: true,
				}
				if !reflect.DeepEqual(got.Info, want) {
					t.Fatalf("Info = %+v, want %+v", got.Info, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPDCManagementRejectsMalformedTLVs(t *testing.T) {
	base := pdcIndicationTLVs(QMIErrorNone, 7)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "load response boolean length",
			call: func() error {
				var got PDCLoadConfigResponse
				return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, nil)})
			},
		},
		{
			name: "load response boolean value",
			call: func() error {
				var got PDCLoadConfigResponse
				return got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x10, uint8(2))})
			},
		},
		{
			name: "load received size",
			call: func() error {
				var got PDCLoadConfigIndication
				return got.UnmarshalTLVs(append(base, tlv.Bytes(0x11, []byte{1})))
			},
		},
		{
			name: "load frame reset",
			call: func() error {
				var got PDCLoadConfigIndication
				return got.UnmarshalTLVs(append(base, tlv.Uint(0x13, uint8(2))))
			},
		},
		{
			name: "limits uint64",
			call: func() error {
				var got PDCGetConfigLimitsIndication
				return got.UnmarshalTLVs(append(base, tlv.Bytes(0x11, make([]byte, 7))))
			},
		},
		{
			name: "default description",
			call: func() error {
				var got PDCGetDefaultConfigInfoIndication
				return got.UnmarshalTLVs(append(base, tlv.Bytes(0x13, []byte{2, 'x'})))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestClientPDCManagementOperations(t *testing.T) {
	subscription := uint32(2)
	tests := []struct {
		name    string
		message MessageID
		call    func(context.Context, *Client) error
		check   func(*testing.T, Request)
		resp    Response
		ind     tlv.TLVs
	}{
		{
			name:    "reset",
			message: MessagePDCReset,
			call: func(ctx context.Context, c *Client) error {
				return c.PDCReset(ctx)
			},
			check: func(t *testing.T, req Request) {
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
				}
			},
			resp: successResponse(MessagePDCReset),
		},
		{
			name:    "delete configuration",
			message: MessagePDCDeleteConfig,
			call: func(ctx context.Context, c *Client) error {
				return c.PDCDeleteConfiguration(ctx, PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x11, []byte{2, 'i', 'd'})
			},
			resp: successResponse(MessagePDCDeleteConfig),
			ind:  pdcIndicationTLVs(QMIErrorNone, 1),
		},
		{
			name:    "configuration limits",
			message: MessagePDCGetConfigLimits,
			call: func(ctx context.Context, c *Client) error {
				got, err := c.PDCConfigurationLimits(ctx, PDCConfigurationSoftware)
				if err == nil && (got.Maximum != 8192 || !got.MaximumKnown || got.Current != 1024 || !got.CurrentKnown) {
					t.Fatalf("limits = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{1, 0, 0, 0})
			},
			resp: successResponse(MessagePDCGetConfigLimits),
			ind: append(pdcIndicationTLVs(QMIErrorNone, 1),
				tlv.Bytes(0x11, binary.LittleEndian.AppendUint64(nil, 8192)),
				tlv.Bytes(0x12, binary.LittleEndian.AppendUint64(nil, 1024)),
			),
		},
		{
			name:    "default configuration info",
			message: MessagePDCGetDefaultConfigInfo,
			call: func(ctx context.Context, c *Client) error {
				got, err := c.PDCDefaultConfigurationInfo(ctx, PDCConfigurationPlatform)
				if err == nil && (!got.VersionKnown || got.Version != 3 || !got.DescriptionKnown || got.Description != "base") {
					t.Fatalf("info = %+v", got)
				}
				return err
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
			},
			resp: successResponse(MessagePDCGetDefaultConfigInfo),
			ind: append(pdcIndicationTLVs(QMIErrorNone, 1),
				tlv.Uint(0x11, uint32(3)),
				tlv.Bytes(0x13, []byte{4, 'b', 'a', 's', 'e'}),
			),
		},
		{
			name:    "deactivate configuration",
			message: MessagePDCDeactivateConfig,
			call: func(ctx context.Context, c *Client) error {
				return c.PDCDeactivateConfiguration(ctx, PDCConfigDeactivation{
					Type: PDCConfigurationSoftware, Subscription: &subscription,
				})
			},
			check: func(t *testing.T, req Request) {
				assertTLV(t, req.TLVs, 0x11, []byte{2, 0, 0, 0})
			},
			resp: successResponse(MessagePDCDeactivateConfig),
			ind:  pdcIndicationTLVs(QMIErrorNone, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := pdcTransportCall{
				check: func(req Request) {
					if req.Service != ServicePDC || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = %+v", req)
					}
					tt.check(t, req)
				},
				resp: tt.resp,
			}
			if tt.ind != nil {
				call.indications = []Indication{{Service: ServicePDC, ClientID: 7, MessageID: tt.message, TLVs: tt.ind}}
			}
			transport := &pdcTestTransport{t: t, calls: []pdcTransportCall{call}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := tt.call(ctx, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
		})
	}
}

func TestClientPDCLoadConfiguration(t *testing.T) {
	data := []byte(strings.Repeat("a", pdcLoadChunkSize) + "xyz")
	tests := []struct {
		name string
	}{
		{name: "two frames"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := PDCStorageRemote
			transport := &pdcTestTransport{t: t, calls: []pdcTransportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessagePDCLoadConfig {
							t.Fatalf("MessageID = 0x%04X", req.MessageID)
						}
						value, ok := tlv.Value(req.TLVs, 0x01)
						if !ok || len(value) != 4+1+2+4+2+pdcLoadChunkSize {
							t.Fatalf("frame length = %d", len(value))
						}
						if binary.LittleEndian.Uint16(value[11:13]) != pdcLoadChunkSize || !bytes.Equal(value[13:], data[:pdcLoadChunkSize]) {
							t.Fatal("first frame data mismatch")
						}
						assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 0, 0})
						assertTLV(t, req.TLVs, 0x11, []byte{1, 0, 0, 0})
					},
					resp: successResponse(MessagePDCLoadConfig, tlv.Uint(0x10, uint8(0))),
					indications: []Indication{{MessageID: MessagePDCLoadConfig, TLVs: append(
						pdcIndicationTLVs(QMIErrorNone, 1),
						tlv.Uint(0x11, uint32(pdcLoadChunkSize)),
						tlv.Uint(0x12, uint32(3)),
					)}},
				},
				{
					check: func(req Request) {
						value, ok := tlv.Value(req.TLVs, 0x01)
						if !ok || binary.LittleEndian.Uint16(value[11:13]) != 3 || !bytes.Equal(value[13:], []byte("xyz")) {
							t.Fatalf("second frame = % X", value)
						}
						assertTLV(t, req.TLVs, 0x10, []byte{2, 0, 0, 0})
					},
					resp: successResponse(MessagePDCLoadConfig),
					indications: []Indication{{MessageID: MessagePDCLoadConfig, TLVs: append(
						pdcIndicationTLVs(QMIErrorNone, 2),
						tlv.Uint(0x11, uint32(len(data))),
						tlv.Uint(0x12, uint32(0)),
					)}},
				},
			}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := client.PDCLoadConfiguration(ctx, PDCConfigLoad{
				Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
				Data:   data, Storage: &storage,
			})
			if err != nil {
				t.Fatalf("PDCLoadConfiguration() error = %v", err)
			}
			if got := transport.callCount(); got != 2 {
				t.Fatalf("Do() calls = %d, want 2", got)
			}
		})
	}
}

func TestClientPDCLoadConfigurationErrors(t *testing.T) {
	tests := []struct {
		name    string
		load    PDCConfigLoad
		calls   []pdcTransportCall
		wantErr string
	}{
		{
			name:    "empty data",
			load:    PDCConfigLoad{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}},
			wantErr: "data is empty",
		},
		{
			name:    "response frame reset",
			load:    PDCConfigLoad{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, Data: []byte{1}},
			calls:   []pdcTransportCall{{resp: successResponse(MessagePDCLoadConfig, tlv.Uint(0x10, uint8(1)))}},
			wantErr: "reset accumulated",
		},
		{
			name: "remaining size missing",
			load: PDCConfigLoad{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, Data: []byte{1}},
			calls: []pdcTransportCall{{
				resp:        successResponse(MessagePDCLoadConfig),
				indications: []Indication{{MessageID: MessagePDCLoadConfig, TLVs: pdcIndicationTLVs(QMIErrorNone, 1)}},
			}},
			wantErr: "remaining configuration size is missing",
		},
		{
			name: "received size mismatch",
			load: PDCConfigLoad{Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")}, Data: []byte{1}},
			calls: []pdcTransportCall{{
				resp: successResponse(MessagePDCLoadConfig),
				indications: []Indication{{MessageID: MessagePDCLoadConfig, TLVs: append(
					pdcIndicationTLVs(QMIErrorNone, 1),
					tlv.Uint(0x11, uint32(0)),
					tlv.Uint(0x12, uint32(0)),
				)}},
			}},
			wantErr: "received configuration size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pdcTestTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := client.PDCLoadConfiguration(ctx, tt.load)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("PDCLoadConfiguration() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientPDCManagementIndicationError(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "delete indication error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pdcTestTransport{t: t, calls: []pdcTransportCall{{
				resp:        successResponse(MessagePDCDeleteConfig),
				indications: []Indication{{MessageID: MessagePDCDeleteConfig, TLVs: pdcIndicationTLVs(QMIErrorInternal, 1)}},
			}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := client.PDCDeleteConfiguration(ctx, PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")})
			if !errors.Is(err, QMIErrorInternal) {
				t.Fatalf("PDCDeleteConfiguration() error = %v, want %v", err, QMIErrorInternal)
			}
		})
	}
}

func TestClientPDCDeleteConfigurationValidation(t *testing.T) {
	tests := []struct {
		name   string
		config PDCConfig
	}{
		{name: "empty ID", config: PDCConfig{Type: PDCConfigurationSoftware}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			if err := client.PDCDeleteConfiguration(context.Background(), tt.config); err == nil {
				t.Fatal("PDCDeleteConfiguration() error = nil, want non-nil")
			}
		})
	}
}
