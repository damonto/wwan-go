package qcom

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestPDCRequests(t *testing.T) {
	enabled := true
	disabled := false
	token := uint32(0x11223344)
	subscription := uint32(2)
	slot := uint32(1)
	configType := PDCConfigurationDatabase
	activation := PDCActivationRefreshOnly
	multiSupport := true
	tests := []struct {
		name    string
		build   func() (Request, error)
		wantID  MessageID
		wantTLV map[byte][]byte
		wantErr string
	}{
		{
			name: "register",
			build: func() (Request, error) {
				return (PDCRegisterRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config: PDCIndicationConfig{ConfigChange: enabled, ClientRefresh: &disabled},
				}).Request(), nil
			},
			wantID: MessagePDCRegister,
			wantTLV: map[byte][]byte{
				0x10: {1},
				0x11: {0},
			},
		},
		{
			name: "get selected",
			build: func() (Request, error) {
				return (PDCGetSelectedConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Type: PDCConfigurationSoftware, Token: &token,
					Subscription: &subscription, Slot: &slot,
				}).Request()
			},
			wantID: MessagePDCGetSelectedConfig,
			wantTLV: map[byte][]byte{
				0x01: {1, 0, 0, 0},
				0x10: {0x44, 0x33, 0x22, 0x11},
				0x11: {2, 0, 0, 0},
				0x12: {1, 0, 0, 0},
			},
		},
		{
			name: "set selected",
			build: func() (Request, error) {
				return (PDCSetSelectedConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte{0xAA, 0xBB}},
					Token:  &token, Subscription: &subscription, Slot: &slot,
				}).Request()
			},
			wantID: MessagePDCSetSelectedConfig,
			wantTLV: map[byte][]byte{
				0x01: {1, 0, 0, 0, 2, 0xAA, 0xBB},
				0x10: {0x44, 0x33, 0x22, 0x11},
				0x11: {2, 0, 0, 0},
				0x12: {1, 0, 0, 0},
			},
		},
		{
			name: "list",
			build: func() (Request, error) {
				return (PDCListConfigsRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Token: &token, Type: configType, MultiSupport: &multiSupport,
				}).Request()
			},
			wantID: MessagePDCListConfigs,
			wantTLV: map[byte][]byte{
				0x10: {0x44, 0x33, 0x22, 0x11},
				0x11: {0x10, 0, 0, 0},
				0x12: {1},
			},
		},
		{
			name: "invalid list configuration type",
			build: func() (Request, error) {
				return (PDCListConfigsRequest{Type: 2}).Request()
			},
			wantErr: "configuration type",
		},
		{
			name: "activate",
			build: func() (Request, error) {
				return (PDCActivateConfigRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Type: PDCConfigurationSoftware, Token: &token, Activation: &activation,
					Subscription: &subscription, Slot: &slot,
				}).Request()
			},
			wantID: MessagePDCActivateConfig,
			wantTLV: map[byte][]byte{
				0x01: {1, 0, 0, 0},
				0x10: {0x44, 0x33, 0x22, 0x11},
				0x11: {1, 0, 0, 0},
				0x12: {2, 0, 0, 0},
				0x13: {1, 0, 0, 0},
			},
		},
		{
			name: "get info",
			build: func() (Request, error) {
				return (PDCGetConfigInfoRequest{
					ClientID: 7, TransactionID: 9, Timeout: time.Second,
					Config: PDCConfig{Type: PDCConfigurationPlatform, ID: []byte("id")}, Token: &token,
				}).Request()
			},
			wantID: MessagePDCGetConfigInfo,
			wantTLV: map[byte][]byte{
				0x01: {0, 0, 0, 0, 2, 'i', 'd'},
				0x10: {0x44, 0x33, 0x22, 0x11},
			},
		},
		{
			name: "invalid configuration type",
			build: func() (Request, error) {
				return (PDCGetSelectedConfigRequest{Type: 2}).Request()
			},
			wantErr: "configuration type",
		},
		{
			name: "configuration ID too long",
			build: func() (Request, error) {
				return (PDCSetSelectedConfigRequest{
					Config: PDCConfig{Type: PDCConfigurationSoftware, ID: make([]byte, pdcConfigIDMax+1)},
				}).Request()
			},
			wantErr: "ID length",
		},
		{
			name: "invalid activation type",
			build: func() (Request, error) {
				invalid := PDCActivationType(2)
				return (PDCActivateConfigRequest{Type: PDCConfigurationSoftware, Activation: &invalid}).Request()
			},
			wantErr: "activation type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Request() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServicePDC || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantID {
				t.Fatalf("Request() = %+v", got)
			}
			if got.Timeout != time.Second {
				t.Fatalf("Timeout = %v, want 1s", got.Timeout)
			}
			if len(got.TLVs) != len(tt.wantTLV) {
				t.Fatalf("TLVs len = %d, want %d", len(got.TLVs), len(tt.wantTLV))
			}
			for typ, want := range tt.wantTLV {
				assertTLV(t, got.TLVs, typ, want)
			}
		})
	}
}

func TestPDCIndicationsUnmarshalTLVs(t *testing.T) {
	result := pdcIndicationTLVs(QMIErrorNone, 7)
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "operation",
			run: func(t *testing.T) {
				var got PDCOperationIndication
				if err := got.UnmarshalTLVs(result); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.Result.Error != QMIErrorNone || !got.Result.TokenKnown || got.Result.Token != 7 {
					t.Fatalf("result = %+v", got.Result)
				}
			},
		},
		{
			name: "selected configuration",
			run: func(t *testing.T) {
				tlvs := append(slices.Clone(result),
					tlv.Bytes(0x11, []byte{2, 'a', '1'}),
					tlv.Bytes(0x12, []byte{2, 'p', '1'}),
				)
				var got PDCGetSelectedConfigIndication
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if !got.Selected.ActiveKnown || !slices.Equal(got.Selected.Active, []byte("a1")) ||
					!got.Selected.PendingKnown || !slices.Equal(got.Selected.Pending, []byte("p1")) {
					t.Fatalf("selected = %+v", got.Selected)
				}
			},
		},
		{
			name: "configuration list",
			run: func(t *testing.T) {
				list := []byte{2, 0, 0, 0, 0, 2, 'p', '0', 1, 0, 0, 0, 1, 's'}
				tlvs := append(slices.Clone(result), tlv.Bytes(0x11, list), tlv.Uint(0x12, uint8(1)))
				var got PDCListConfigsIndication
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := []PDCConfig{
					{Type: PDCConfigurationPlatform, ID: []byte("p0")},
					{Type: PDCConfigurationSoftware, ID: []byte("s")},
				}
				if !got.ConfigsKnown || !reflect.DeepEqual(got.Configs, want) || !got.MoreAvailableKnown || !got.MoreAvailable {
					t.Fatalf("list = %+v", got)
				}
			},
		},
		{
			name: "configuration info",
			run: func(t *testing.T) {
				path := []byte{'/', 0, 'm', 0, 0, 0}
				tlvs := append(slices.Clone(result),
					tlv.Uint(0x11, uint32(4096)),
					tlv.Bytes(0x12, []byte{4, 't', 'e', 's', 't'}),
					tlv.Uint(0x13, uint32(0x10203040)),
					tlv.Uint(0x14, uint32(PDCStorageRemote)),
					tlv.Bytes(0x15, path),
					tlv.Uint(0x16, uint32(0x01020304)),
					tlv.Bytes(0x17, []byte{2, 0xAA, 0xBB}),
				)
				var got PDCGetConfigInfoIndication
				if err := got.UnmarshalTLVs(tlvs); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := PDCConfigInfo{
					Size: 4096, SizeKnown: true,
					Description: "test", DescriptionKnown: true,
					Version: 0x10203040, VersionKnown: true,
					Storage: PDCStorageRemote, StorageKnown: true,
					Path: "/m", PathKnown: true,
					BaseVersion: 0x01020304, BaseVersionKnown: true,
					Header: []byte{0xAA, 0xBB}, HeaderKnown: true,
				}
				if !reflect.DeepEqual(got.Info, want) {
					t.Fatalf("info = %+v, want %+v", got.Info, want)
				}
			},
		},
		{
			name: "refresh",
			run: func(t *testing.T) {
				var got PDCRefreshIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Uint(0x01, uint32(PDCRefreshClient)),
					tlv.Uint(0x10, uint32(2)),
					tlv.Uint(0x11, uint32(1)),
				}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				want := PDCRefreshEvent{Type: PDCRefreshClient, Subscription: 2, SubscriptionKnown: true, Slot: 1, SlotKnown: true}
				if got.Event != want {
					t.Fatalf("event = %+v, want %+v", got.Event, want)
				}
			},
		},
		{
			name: "configuration change",
			run: func(t *testing.T) {
				var got PDCConfigChangeIndication
				if err := got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0, 0, 0, 2, 'i', 'd'})}); err != nil {
					t.Fatalf("UnmarshalTLVs() error = %v", err)
				}
				if got.Config.Type != PDCConfigurationSoftware || !slices.Equal(got.Config.ID, []byte("id")) {
					t.Fatalf("config = %+v", got.Config)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPDCIndicationsRejectMalformedTLVs(t *testing.T) {
	validResult := pdcIndicationTLVs(QMIErrorNone, 1)
	tests := []struct {
		name   string
		decode func() error
	}{
		{name: "result missing", decode: func() error { var got PDCOperationIndication; return got.UnmarshalTLVs(nil) }},
		{name: "result length", decode: func() error {
			var got PDCOperationIndication
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{0})})
		}},
		{name: "token length", decode: func() error {
			var got PDCOperationIndication
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{0, 0}), tlv.Bytes(0x10, []byte{1})})
		}},
		{name: "selected ID count", decode: func() error {
			var got PDCGetSelectedConfigIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, nil)))
		}},
		{name: "selected ID data", decode: func() error {
			var got PDCGetSelectedConfigIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x12, []byte{2, 1})))
		}},
		{name: "list count", decode: func() error {
			var got PDCListConfigsIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, nil)))
		}},
		{name: "list maximum", decode: func() error {
			var got PDCListConfigsIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, []byte{pdcConfigListMax + 1})))
		}},
		{name: "list item", decode: func() error {
			var got PDCListConfigsIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, []byte{1, 0, 0})))
		}},
		{name: "list trailing", decode: func() error {
			var got PDCListConfigsIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, []byte{0, 1})))
		}},
		{name: "more available", decode: func() error {
			var got PDCListConfigsIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x12, nil)))
		}},
		{name: "info size", decode: func() error {
			var got PDCGetConfigInfoIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x11, []byte{1})))
		}},
		{name: "info description count", decode: func() error {
			var got PDCGetConfigInfoIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x12, nil)))
		}},
		{name: "info description data", decode: func() error {
			var got PDCGetConfigInfoIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x12, []byte{2, 1})))
		}},
		{name: "info path", decode: func() error {
			var got PDCGetConfigInfoIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x15, []byte{1})))
		}},
		{name: "info header", decode: func() error {
			var got PDCGetConfigInfoIndication
			return got.UnmarshalTLVs(append(slices.Clone(validResult), tlv.Bytes(0x17, []byte{2, 1})))
		}},
		{name: "refresh missing", decode: func() error { var got PDCRefreshIndication; return got.UnmarshalTLVs(nil) }},
		{name: "refresh event", decode: func() error {
			var got PDCRefreshIndication
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1})})
		}},
		{name: "refresh subscription", decode: func() error {
			var got PDCRefreshIndication
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Uint(0x01, uint32(0)), tlv.Bytes(0x10, []byte{1})})
		}},
		{name: "change missing", decode: func() error { var got PDCConfigChangeIndication; return got.UnmarshalTLVs(nil) }},
		{name: "change config", decode: func() error {
			var got PDCConfigChangeIndication
			return got.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 0})})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.decode(); err == nil {
				t.Fatal("decode error = nil")
			}
		})
	}
}

func TestClientPDCOperations(t *testing.T) {
	configType := PDCConfigurationSoftware
	tests := []struct {
		name    string
		calls   []pdcTransportCall
		run     func(context.Context, *Client) error
		wantErr error
	}{
		{
			name: "selected configuration filters token",
			calls: []pdcTransportCall{{
				check: func(req Request) {
					if req.Service != ServicePDC || req.ClientID != 7 || req.MessageID != MessagePDCGetSelectedConfig {
						t.Fatalf("request = %+v", req)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{1, 0, 0, 0})
				},
				resp: successResponse(MessagePDCGetSelectedConfig),
				indications: []Indication{
					{MessageID: MessagePDCGetSelectedConfig, TLVs: pdcIndicationTLVs(QMIErrorNone, 9)},
					{MessageID: MessagePDCGetSelectedConfig, TLVs: append(pdcIndicationTLVs(QMIErrorNone, 1), tlv.Bytes(0x11, []byte{2, 'a', '1'}))},
				},
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.PDCSelectedConfiguration(ctx, PDCSelectionQuery{Type: PDCConfigurationSoftware})
				if err == nil && (!got.ActiveKnown || !slices.Equal(got.Active, []byte("a1"))) {
					t.Fatalf("selected = %+v", got)
				}
				return err
			},
		},
		{
			name: "select configuration",
			calls: []pdcTransportCall{{
				check: func(req Request) {
					if req.MessageID != MessagePDCSetSelectedConfig {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{1, 0, 0, 0, 2, 'i', 'd'})
				},
				resp:        successResponse(MessagePDCSetSelectedConfig),
				indications: []Indication{{MessageID: MessagePDCSetSelectedConfig, TLVs: pdcIndicationTLVs(QMIErrorNone, 1)}},
			}},
			run: func(ctx context.Context, client *Client) error {
				return client.PDCSelectConfiguration(ctx, PDCConfigSelection{
					Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
				})
			},
		},
		{
			name: "activate configuration",
			calls: []pdcTransportCall{{
				check: func(req Request) {
					if req.MessageID != MessagePDCActivateConfig {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{1, 0, 0, 0})
				},
				resp:        successResponse(MessagePDCActivateConfig),
				indications: []Indication{{MessageID: MessagePDCActivateConfig, TLVs: pdcIndicationTLVs(QMIErrorNone, 1)}},
			}},
			run: func(ctx context.Context, client *Client) error {
				return client.PDCActivateConfiguration(ctx, PDCConfigActivation{Type: PDCConfigurationSoftware})
			},
		},
		{
			name: "configuration info",
			calls: []pdcTransportCall{{
				check: func(req Request) {
					if req.MessageID != MessagePDCGetConfigInfo {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
				},
				resp: successResponse(MessagePDCGetConfigInfo),
				indications: []Indication{{MessageID: MessagePDCGetConfigInfo, TLVs: append(
					pdcIndicationTLVs(QMIErrorNone, 1),
					tlv.Uint(0x11, uint32(42)),
					tlv.Bytes(0x12, []byte{3, 'm', 'b', 'n'}),
				)}},
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.PDCConfigurationInfo(ctx, PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")})
				if err == nil && (!got.SizeKnown || got.Size != 42 || !got.DescriptionKnown || got.Description != "mbn") {
					t.Fatalf("info = %+v", got)
				}
				return err
			},
		},
		{
			name: "configuration list frames",
			calls: []pdcTransportCall{{
				check: func(req Request) {
					if req.MessageID != MessagePDCListConfigs {
						t.Fatalf("MessageID = 0x%04X", req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x11, []byte{1, 0, 0, 0})
					assertTLV(t, req.TLVs, 0x12, []byte{1})
					if len(req.TLVs) != 3 {
						t.Fatalf("TLVs len = %d, want 3", len(req.TLVs))
					}
				},
				resp: successResponse(MessagePDCListConfigs),
				indications: []Indication{
					{MessageID: MessagePDCListConfigs, TLVs: append(
						pdcIndicationTLVs(QMIErrorNone, 1),
						tlv.Bytes(0x11, []byte{1, 1, 0, 0, 0, 1, 'a'}),
						tlv.Uint(0x12, uint8(1)),
					)},
					{MessageID: MessagePDCListConfigs, TLVs: append(
						pdcIndicationTLVs(QMIErrorNone, 1),
						tlv.Bytes(0x11, []byte{1, 1, 0, 0, 0, 1, 'b'}),
						tlv.Uint(0x12, uint8(0)),
					)},
				},
			}},
			run: func(ctx context.Context, client *Client) error {
				got, err := client.PDCConfigurations(ctx, configType)
				if err == nil {
					want := []PDCConfig{
						{Type: PDCConfigurationSoftware, ID: []byte("a")},
						{Type: PDCConfigurationSoftware, ID: []byte("b")},
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("configurations = %+v, want %+v", got, want)
					}
				}
				return err
			},
		},
		{
			name: "indication error",
			calls: []pdcTransportCall{{
				resp:        successResponse(MessagePDCSetSelectedConfig),
				indications: []Indication{{MessageID: MessagePDCSetSelectedConfig, TLVs: pdcIndicationTLVs(QMIErrorInternal, 1)}},
			}},
			run: func(ctx context.Context, client *Client) error {
				return client.PDCSelectConfiguration(ctx, PDCConfigSelection{
					Config: PDCConfig{Type: PDCConfigurationSoftware, ID: []byte("id")},
				})
			},
			wantErr: QMIErrorInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pdcTestTransport{t: t, calls: tt.calls}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := tt.run(ctx, client)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("operation error = %v, want %v", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("operation error = %v", err)
			}
			if got := transport.callCount(); got != len(tt.calls) {
				t.Fatalf("Do() calls = %d, want %d", got, len(tt.calls))
			}
		})
	}
}

func TestPDCWatchRefresh(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "register event and unregister"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pdcTestTransport{t: t, calls: []pdcTransportCall{
				{
					check: func(req Request) {
						if req.MessageID != MessagePDCRegister || len(req.TLVs) != 2 {
							t.Fatalf("register request = %+v", req)
						}
						assertTLV(t, req.TLVs, 0x10, []byte{0})
						assertTLV(t, req.TLVs, 0x11, []byte{1})
					},
					resp: successResponse(MessagePDCRegister),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x10, []byte{0})
						assertTLV(t, req.TLVs, 0x11, []byte{0})
					},
					resp: successResponse(MessagePDCRegister),
				},
			}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithCancel(context.Background())
			events, err := client.PDCWatchRefresh(ctx)
			if err != nil {
				cancel()
				t.Fatalf("PDCWatchRefresh() error = %v", err)
			}
			transport.emit(Indication{Service: ServicePDC, ClientID: 7, MessageID: MessagePDCRefresh, TLVs: tlv.TLVs{
				tlv.Uint(0x01, uint32(PDCRefreshClient)),
				tlv.Uint(0x10, uint32(2)),
			}})
			select {
			case event := <-events:
				if event.Type != PDCRefreshClient || !event.SubscriptionKnown || event.Subscription != 2 {
					t.Fatalf("event = %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for refresh event")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func TestPDCWatchConfigChanges(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "register change and unregister"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &pdcTestTransport{t: t, calls: []pdcTransportCall{
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x10, []byte{1})
						assertTLV(t, req.TLVs, 0x11, []byte{0})
					},
					resp: successResponse(MessagePDCRegister),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x10, []byte{0})
						assertTLV(t, req.TLVs, 0x11, []byte{0})
					},
					resp: successResponse(MessagePDCRegister),
				},
			}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServicePDC: 7}}
			ctx, cancel := context.WithCancel(context.Background())
			configs, err := client.PDCWatchConfigChanges(ctx)
			if err != nil {
				cancel()
				t.Fatalf("PDCWatchConfigChanges() error = %v", err)
			}
			transport.emit(Indication{Service: ServicePDC, ClientID: 7, MessageID: MessagePDCConfigChange, TLVs: tlv.TLVs{
				tlv.Bytes(0x01, []byte{1, 0, 0, 0, 2, 'i', 'd'}),
			}})
			select {
			case config := <-configs:
				if config.Type != PDCConfigurationSoftware || !slices.Equal(config.ID, []byte("id")) {
					t.Fatalf("config = %+v", config)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for configuration change")
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}

func pdcIndicationTLVs(qmiErr QMIError, token uint32) tlv.TLVs {
	return tlv.TLVs{
		tlv.Uint(0x01, uint16(qmiErr)),
		tlv.Uint(0x10, token),
	}
}

type pdcTransportCall struct {
	check       func(Request)
	resp        Response
	err         error
	indications []Indication
}

type pdcTestTransport struct {
	mu       sync.Mutex
	t        testing.TB
	calls    []pdcTransportCall
	index    int
	channels map[MessageID][]chan Indication
}

func (t *pdcTestTransport) Do(_ context.Context, req Request) (Response, error) {
	t.t.Helper()
	t.mu.Lock()
	if t.index >= len(t.calls) {
		t.mu.Unlock()
		t.t.Fatalf("Do() got unexpected request: %+v", req)
	}
	call := t.calls[t.index]
	t.index++
	t.mu.Unlock()
	if call.check != nil {
		call.check(req)
	}
	for _, indication := range call.indications {
		t.emit(indication)
	}
	return call.resp, call.err
}

func (t *pdcTestTransport) Indications(ctx context.Context, _ ServiceType, _ uint8, id MessageID) (<-chan Indication, error) {
	ch := make(chan Indication, 8)
	t.mu.Lock()
	if t.channels == nil {
		t.channels = make(map[MessageID][]chan Indication)
	}
	t.channels[id] = append(t.channels[id], ch)
	t.mu.Unlock()
	go func() {
		<-ctx.Done()
		t.mu.Lock()
		channels := t.channels[id]
		for index, candidate := range channels {
			if candidate == ch {
				t.channels[id] = slices.Delete(channels, index, index+1)
				break
			}
		}
		close(ch)
		t.mu.Unlock()
	}()
	return ch, nil
}

func (t *pdcTestTransport) Close() error {
	return nil
}

func (t *pdcTestTransport) emit(indication Indication) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, ch := range t.channels[indication.MessageID] {
		ch <- indication
	}
}

func (t *pdcTestTransport) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.index
}

func (t *pdcTestTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want %d", t.callCount(), want)
}
