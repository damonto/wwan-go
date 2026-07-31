package qcom

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestUIMConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		mask     UIMConfigurationMask
		response tlv.TLVs
		want     UIMConfiguration
	}{
		{
			name: "all supported fields",
			mask: UIMConfigurationAutomaticSelection |
				UIMConfigurationPersonalizationStatus |
				UIMConfigurationHaltSubscription,
			response: tlv.TLVs{
				tlv.Bytes(0x10, []byte{1}),
				tlv.Bytes(0x11, []byte{
					2,
					byte(UIMPersonalizationGWNetwork), 3, 8,
					byte(UIMPersonalizationGWServiceProvider), 4, 9,
				}),
				tlv.Bytes(0x12, []byte{0}),
				tlv.Bytes(0x13, []byte{
					2,
					1, byte(UIMPersonalizationGWCorporate), 2, 7,
					0,
				}),
			},
			want: UIMConfiguration{
				AutomaticSelection:      true,
				AutomaticSelectionKnown: true,
				PersonalizationStatus: []UIMPersonalizationStatus{
					{Feature: UIMPersonalizationGWNetwork, VerifyRetries: 3, UnblockRetries: 8},
					{Feature: UIMPersonalizationGWServiceProvider, VerifyRetries: 4, UnblockRetries: 9},
				},
				PersonalizationStatusKnown: true,
				HaltSubscriptionKnown:      true,
				OtherSlotsPersonalization: [][]UIMPersonalizationStatus{
					{{Feature: UIMPersonalizationGWCorporate, VerifyRetries: 2, UnblockRetries: 7}},
					{},
				},
				OtherSlotsPersonalizationKnown: true,
			},
		},
		{name: "request all with omitted mask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.ClientID != 7 || req.MessageID != MessageGetConfiguration {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					if tt.mask == 0 {
						if len(req.TLVs) != 0 {
							t.Fatalf("TLVs len = %d, want 0", len(req.TLVs))
						}
						return
					}
					assertTLV(t, req.TLVs, 0x10, binary.LittleEndian.AppendUint32(nil, uint32(tt.mask)))
				},
				resp: successResponse(MessageGetConfiguration, tt.response...),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			got, err := client.UIMConfiguration(context.Background(), tt.mask)
			if err != nil {
				t.Fatalf("UIMConfiguration() error = %v", err)
			}
			if !equalUIMConfiguration(got, tt.want) {
				t.Fatalf("UIMConfiguration() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUIMConfigurationRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name  string
		typ   byte
		value []byte
	}{
		{name: "automatic selection", typ: 0x10, value: nil},
		{name: "personalization count missing", typ: 0x11, value: nil},
		{name: "personalization truncated", typ: 0x11, value: []byte{1, 0, 3}},
		{name: "personalization count too large", typ: 0x11, value: []byte{21}},
		{name: "halt subscription", typ: 0x12, value: []byte{0, 1}},
		{name: "other-slot count missing", typ: 0x13, value: nil},
		{name: "other-slot truncated", typ: 0x13, value: []byte{1, 1, 0}},
		{name: "other-slot trailing data", typ: 0x13, value: []byte{0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config UIMConfiguration
			if err := config.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(tt.typ, tt.value)}); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestDepersonalize(t *testing.T) {
	tests := []struct {
		name    string
		req     UIMDepersonalizationRequest
		resp    Response
		want    UIMDepersonalizationResult
		wantErr bool
	}{
		{
			name: "deactivate network lock",
			req: UIMDepersonalizationRequest{
				Feature:    UIMPersonalizationGWNetwork,
				Operation:  UIMDepersonalizationDeactivate,
				ControlKey: "12345678",
			},
			resp: successResponse(MessageDepersonalization),
		},
		{
			name: "incorrect control key returns retries",
			req: UIMDepersonalizationRequest{
				Feature:    UIMPersonalizationGWServiceProvider,
				Operation:  UIMDepersonalizationUnblock,
				ControlKey: "00000000",
			},
			resp:    errorResponse(MessageDepersonalization, QMIErrorIncorrectPin, tlv.Bytes(0x10, []byte{2, 5})),
			want:    UIMDepersonalizationResult{VerifyRetries: 2, UnblockRetries: 5, RetriesKnown: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(request Request) {
					if request.Service != ServiceUIM || request.ClientID != 7 || request.MessageID != MessageDepersonalization {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", request.Service, request.ClientID, request.MessageID)
					}
					value := []byte{byte(tt.req.Feature), byte(tt.req.Operation), byte(len(tt.req.ControlKey))}
					value = append(value, tt.req.ControlKey...)
					assertTLV(t, request.TLVs, 0x01, value)
					assertTLV(t, request.TLVs, 0x10, []byte{2})
				},
				resp: tt.resp,
			}}}
			client := &Client{transport: transport, slot: 2, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			got, err := client.Depersonalize(context.Background(), tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Depersonalize() error = nil, want non-nil")
				}
			} else if err != nil {
				t.Fatalf("Depersonalize() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Depersonalize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDepersonalizeValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     UIMDepersonalizationRequest
		wantErr string
	}{
		{name: "feature", req: UIMDepersonalizationRequest{Feature: 0xFF, ControlKey: "1234"}, wantErr: "feature"},
		{name: "operation", req: UIMDepersonalizationRequest{Operation: 0xFF, ControlKey: "1234"}, wantErr: "operation"},
		{name: "empty key", req: UIMDepersonalizationRequest{}, wantErr: "control key is empty"},
		{name: "long key", req: UIMDepersonalizationRequest{ControlKey: strings.Repeat("1", 17)}, wantErr: "exceeds 16"},
		{name: "non ASCII key", req: UIMDepersonalizationRequest{ControlKey: "密码"}, wantErr: "non-ASCII"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Client{}).Depersonalize(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Depersonalize() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRemoteUnlock(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantTLV byte
	}{
		{name: "standard blob", data: []byte{0xAA, 0xBB, 0xCC}, wantTLV: 0x10},
		{name: "largest standard blob", data: bytes.Repeat([]byte{0x5A}, 1024), wantTLV: 0x10},
		{name: "extended blob", data: bytes.Repeat([]byte{0xA5}, 1025), wantTLV: 0x12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.ClientID != 7 || req.MessageID != MessageRemoteUnlock {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					want := binary.LittleEndian.AppendUint16(nil, uint16(len(tt.data)))
					want = append(want, tt.data...)
					assertTLV(t, req.TLVs, tt.wantTLV, want)
				},
				resp: successResponse(MessageRemoteUnlock),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			if err := client.RemoteUnlock(context.Background(), tt.data); err != nil {
				t.Fatalf("RemoteUnlock() error = %v", err)
			}
		})
	}
}

func TestRemoteUnlockValidation(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "empty", wantErr: "empty"},
		{name: "too long", data: make([]byte, 4097), wantErr: "exceeds 4096"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Client{}).RemoteUnlock(context.Background(), tt.data)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("RemoteUnlock() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func equalUIMConfiguration(got, want UIMConfiguration) bool {
	if got.AutomaticSelection != want.AutomaticSelection ||
		got.AutomaticSelectionKnown != want.AutomaticSelectionKnown ||
		got.PersonalizationStatusKnown != want.PersonalizationStatusKnown ||
		got.HaltSubscription != want.HaltSubscription ||
		got.HaltSubscriptionKnown != want.HaltSubscriptionKnown ||
		got.OtherSlotsPersonalizationKnown != want.OtherSlotsPersonalizationKnown {
		return false
	}
	return slices.Equal(got.PersonalizationStatus, want.PersonalizationStatus) &&
		slices.EqualFunc(got.OtherSlotsPersonalization, want.OtherSlotsPersonalization, func(a, b []UIMPersonalizationStatus) bool {
			return slices.Equal(a, b)
		})
}
