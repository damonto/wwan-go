package qcom

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestPBMRequestEncoding(t *testing.T) {
	mask := PBMEventPhonebookReady | PBMEventEmergencyNumberList
	tests := []struct {
		name        string
		request     func() (Request, error)
		wantMessage MessageID
		wantTLVs    map[byte][]byte
	}{
		{
			name: "indication register",
			request: func() (Request, error) {
				return (PBMIndicationRegisterRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second, Mask: mask,
				}).Request(), nil
			},
			wantMessage: MessagePBMIndicationRegister,
			wantTLVs:    map[byte][]byte{0x01: {0x06, 0x00, 0x00, 0x00}},
		},
		{
			name: "get capabilities",
			request: func() (Request, error) {
				return (PBMGetCapabilitiesRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
					Session: PBMSessionGWSecondary, Phonebook: PBMPhonebookADN | PBMPhonebookFDN,
				}).Request()
			},
			wantMessage: MessagePBMGetCapabilities,
			wantTLVs:    map[byte][]byte{0x01: {0x02, 0x03, 0x00}},
		},
		{
			name: "get all capabilities",
			request: func() (Request, error) {
				return (PBMGetAllCapabilitiesRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
				}).Request(), nil
			},
			wantMessage: MessagePBMGetAllCapabilities,
		},
		{
			name: "get emergency list",
			request: func() (Request, error) {
				return (PBMGetEmergencyListRequest{
					ClientID: 7, TransactionID: 9, Timeout: 3 * time.Second,
				}).Request(), nil
			},
			wantMessage: MessagePBMGetEmergencyList,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.request()
			if err != nil {
				t.Fatalf("Request() error = %v", err)
			}
			if got.Service != ServicePBM || got.ClientID != 7 || got.TransactionID != 9 || got.MessageID != tt.wantMessage {
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

func TestPBMRequestValidation(t *testing.T) {
	tests := []struct {
		name string
		req  PBMGetCapabilitiesRequest
	}{
		{
			name: "session above range",
			req: PBMGetCapabilitiesRequest{
				Session: PBMSessionGlobalPhonebookSlot3 + 1, Phonebook: PBMPhonebookADN,
			},
		},
		{
			name: "empty phonebook mask",
			req:  PBMGetCapabilitiesRequest{Session: PBMSessionGWPrimary},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.req.Request(); err == nil {
				t.Fatal("Request() error = nil, want non-nil")
			}
		})
	}
}

func TestPBMIndicationRegisterResponseUnmarshalTLVs(t *testing.T) {
	wantMask := PBMEventRecordUpdate | PBMEventGASUpdate
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PBMIndicationRegisterResponse
		wantErr bool
	}{
		{
			name: "mask",
			tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0x21, 0x00, 0x00, 0x00})},
			want: PBMIndicationRegisterResponse{Mask: wantMask, MaskKnown: true},
		},
		{name: "mask omitted"},
		{name: "mask truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0x21, 0x00, 0x00})}, wantErr: true},
		{name: "mask trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0x21, 0x00, 0x00, 0x00, 0x00})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PBMIndicationRegisterResponse
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
			if got != tt.want {
				t.Fatalf("UnmarshalTLVs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPBMCapabilitiesUnmarshalTLVs(t *testing.T) {
	complete := tlv.TLVs{
		tlv.Bytes(0x10, []byte{0x02, 0x03, 0x00, 0x05, 0x00, 0x14, 0x00, 0x20, 0x28}),
		tlv.Bytes(0x11, []byte{0x05, 0x14}),
		tlv.Bytes(0x12, []byte{0x02, 0x1E, 0x0F}),
		tlv.Bytes(0x13, []byte{0x04, 0x3C}),
		tlv.Bytes(0x14, []byte{0x28}),
		tlv.Bytes(0x15, []byte{0x01}),
		tlv.Bytes(0x16, []byte{0x28, 0x03, 0x32}),
		tlv.Bytes(0x17, []byte{0x14, 0x02, 0x1E}),
	}
	want := PBMCapabilities{
		Basic: PBMBasicCapabilities{
			Session: PBMSessionGWSecondary,
			Phonebooks: []PBMPhonebookCapability{{
				Phonebook:           PBMPhonebookADN | PBMPhonebookFDN,
				UsedRecords:         5,
				MaximumRecords:      20,
				MaximumNumberLength: 32,
				MaximumNameLength:   40,
			}},
		},
		BasicKnown:            true,
		Group:                 PBMGroupCapability{MaximumGroups: 5, MaximumGroupTagLength: 20},
		GroupKnown:            true,
		AdditionalNumber:      PBMAdditionalNumberCapability{MaximumNumbers: 2, MaximumNumberLength: 30, MaximumNumberTagLength: 15},
		AdditionalNumberKnown: true,
		Email:                 PBMEmailCapability{MaximumEmails: 4, MaximumAddressLength: 60},
		EmailKnown:            true,
		SecondName:            PBMSecondNameCapability{MaximumLength: 40},
		SecondNameKnown:       true,
		HiddenRecords:         PBMHiddenRecordsCapability{Supported: true},
		HiddenRecordsKnown:    true,
		GAS:                   PBMAlphaStringCapability{MaximumRecords: 40, UsedRecords: 3, MaximumStringLength: 50},
		GASKnown:              true,
		AAS:                   PBMAlphaStringCapability{MaximumRecords: 20, UsedRecords: 2, MaximumStringLength: 30},
		AASKnown:              true,
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PBMCapabilities
		wantErr bool
	}{
		{name: "complete", tlvs: complete, want: want},
		{name: "empty"},
		{name: "basic truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 8))}, wantErr: true},
		{name: "basic trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, make([]byte, 10))}, wantErr: true},
		{name: "group truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1})}, wantErr: true},
		{name: "additional trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 2, 3, 4})}, wantErr: true},
		{name: "email truncated", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1})}, wantErr: true},
		{name: "second name trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{1, 2})}, wantErr: true},
		{name: "hidden records truncated", tlvs: tlv.TLVs{tlv.Bytes(0x15, nil)}, wantErr: true},
		{name: "GAS trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x16, []byte{1, 2, 3, 4})}, wantErr: true},
		{name: "AAS truncated", tlvs: tlv.TLVs{tlv.Bytes(0x17, []byte{1, 2})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PBMCapabilities
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPBMAllCapabilitiesUnmarshalTLVs(t *testing.T) {
	complete := tlv.TLVs{
		tlv.Bytes(0x10, []byte{
			0x02,
			0x00, 0x02,
			0x01, 0x00, 0x01, 0x00, 0x64, 0x00, 0x14, 0x1E,
			0x02, 0x00, 0x02, 0x00, 0x0A, 0x00, 0x0F, 0x14,
			0x08, 0x01,
			0x00, 0x01, 0x03, 0x00, 0x28, 0x00, 0x00, 0x32,
		}),
		tlv.Bytes(0x11, []byte{0x02, 0x00, 0x05, 0x14, 0x08, 0x06, 0x15}),
		tlv.Bytes(0x12, []byte{0x02, 0x00, 0x02, 0x1E, 0x0F, 0x08, 0x03, 0x1F, 0x10}),
		tlv.Bytes(0x13, []byte{0x02, 0x00, 0x04, 0x3C, 0x08, 0x05, 0x3D}),
		tlv.Bytes(0x14, []byte{0x02, 0x00, 0x28, 0x08, 0x29}),
		tlv.Bytes(0x15, []byte{0x02, 0x00, 0x01, 0x08, 0x00}),
		tlv.Bytes(0x16, []byte{0x02, 0x00, 0x28, 0x03, 0x32, 0x08, 0x29, 0x04, 0x33}),
		tlv.Bytes(0x17, []byte{0x02, 0x00, 0x14, 0x02, 0x1E, 0x08, 0x15, 0x03, 0x1F}),
	}
	want := PBMAllCapabilities{
		Basic: []PBMBasicCapabilities{
			{
				Session: PBMSessionGWPrimary,
				Phonebooks: []PBMPhonebookCapability{
					{Phonebook: PBMPhonebookADN, UsedRecords: 1, MaximumRecords: 100, MaximumNumberLength: 20, MaximumNameLength: 30},
					{Phonebook: PBMPhonebookFDN, UsedRecords: 2, MaximumRecords: 10, MaximumNumberLength: 15, MaximumNameLength: 20},
				},
			},
			{
				Session: PBMSessionGWTertiary,
				Phonebooks: []PBMPhonebookCapability{
					{Phonebook: PBMPhonebookGAS, UsedRecords: 3, MaximumRecords: 40, MaximumNameLength: 50},
				},
			},
		},
		Groups: []PBMSessionGroupCapability{
			{Session: PBMSessionGWPrimary, PBMGroupCapability: PBMGroupCapability{MaximumGroups: 5, MaximumGroupTagLength: 20}},
			{Session: PBMSessionGWTertiary, PBMGroupCapability: PBMGroupCapability{MaximumGroups: 6, MaximumGroupTagLength: 21}},
		},
		AdditionalNumbers: []PBMSessionAdditionalNumberCapability{
			{Session: PBMSessionGWPrimary, PBMAdditionalNumberCapability: PBMAdditionalNumberCapability{MaximumNumbers: 2, MaximumNumberLength: 30, MaximumNumberTagLength: 15}},
			{Session: PBMSessionGWTertiary, PBMAdditionalNumberCapability: PBMAdditionalNumberCapability{MaximumNumbers: 3, MaximumNumberLength: 31, MaximumNumberTagLength: 16}},
		},
		Emails: []PBMSessionEmailCapability{
			{Session: PBMSessionGWPrimary, PBMEmailCapability: PBMEmailCapability{MaximumEmails: 4, MaximumAddressLength: 60}},
			{Session: PBMSessionGWTertiary, PBMEmailCapability: PBMEmailCapability{MaximumEmails: 5, MaximumAddressLength: 61}},
		},
		SecondNames: []PBMSessionSecondNameCapability{
			{Session: PBMSessionGWPrimary, PBMSecondNameCapability: PBMSecondNameCapability{MaximumLength: 40}},
			{Session: PBMSessionGWTertiary, PBMSecondNameCapability: PBMSecondNameCapability{MaximumLength: 41}},
		},
		HiddenRecords: []PBMSessionHiddenRecordsCapability{
			{Session: PBMSessionGWPrimary, PBMHiddenRecordsCapability: PBMHiddenRecordsCapability{Supported: true}},
			{Session: PBMSessionGWTertiary, PBMHiddenRecordsCapability: PBMHiddenRecordsCapability{Supported: false}},
		},
		GAS: []PBMSessionAlphaStringCapability{
			{Session: PBMSessionGWPrimary, PBMAlphaStringCapability: PBMAlphaStringCapability{MaximumRecords: 40, UsedRecords: 3, MaximumStringLength: 50}},
			{Session: PBMSessionGWTertiary, PBMAlphaStringCapability: PBMAlphaStringCapability{MaximumRecords: 41, UsedRecords: 4, MaximumStringLength: 51}},
		},
		AAS: []PBMSessionAlphaStringCapability{
			{Session: PBMSessionGWPrimary, PBMAlphaStringCapability: PBMAlphaStringCapability{MaximumRecords: 20, UsedRecords: 2, MaximumStringLength: 30}},
			{Session: PBMSessionGWTertiary, PBMAlphaStringCapability: PBMAlphaStringCapability{MaximumRecords: 21, UsedRecords: 3, MaximumStringLength: 31}},
		},
	}
	emptyArrays := PBMAllCapabilities{
		Basic:             []PBMBasicCapabilities{},
		Groups:            []PBMSessionGroupCapability{},
		AdditionalNumbers: []PBMSessionAdditionalNumberCapability{},
		Emails:            []PBMSessionEmailCapability{},
		SecondNames:       []PBMSessionSecondNameCapability{},
		HiddenRecords:     []PBMSessionHiddenRecordsCapability{},
		GAS:               []PBMSessionAlphaStringCapability{},
		AAS:               []PBMSessionAlphaStringCapability{},
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PBMAllCapabilities
		wantErr bool
	}{
		{name: "complete", tlvs: complete, want: want},
		{name: "omitted arrays"},
		{
			name: "present empty arrays",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0}), tlv.Bytes(0x11, []byte{0}),
				tlv.Bytes(0x12, []byte{0}), tlv.Bytes(0x13, []byte{0}),
				tlv.Bytes(0x14, []byte{0}), tlv.Bytes(0x15, []byte{0}),
				tlv.Bytes(0x16, []byte{0}), tlv.Bytes(0x17, []byte{0}),
			},
			want: emptyArrays,
		},
		{name: "basic count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "basic phonebook count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0})}, wantErr: true},
		{name: "basic phonebook truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0, 1, 1, 0})}, wantErr: true},
		{name: "basic trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{0, 1})}, wantErr: true},
		{name: "group truncated", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1, 0, 1})}, wantErr: true},
		{name: "additional trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{0, 1})}, wantErr: true},
		{name: "email truncated", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1, 0, 1})}, wantErr: true},
		{name: "second name trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{0, 1})}, wantErr: true},
		{name: "hidden records truncated", tlvs: tlv.TLVs{tlv.Bytes(0x15, []byte{1, 0})}, wantErr: true},
		{name: "GAS truncated", tlvs: tlv.TLVs{tlv.Bytes(0x16, []byte{1, 0, 1, 2})}, wantErr: true},
		{name: "AAS trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x17, []byte{0, 1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PBMAllCapabilities
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPBMEmergencyListUnmarshalTLVs(t *testing.T) {
	complete := tlv.TLVs{
		tlv.Bytes(0x10, []byte{0x02, 0x03, '1', '1', '2', 0x03, '9', '1', '1'}),
		tlv.Bytes(0x11, []byte{0x01, 0x03, '9', '9', '9'}),
		tlv.Bytes(0x12, []byte{
			0x02,
			0x00, 0x02,
			0x01, 0x03, '1', '1', '0',
			0x04, 0x03, '1', '1', '9',
			0x08, 0x01,
			0x00, 0x03, '1', '1', '2',
		}),
		tlv.Bytes(0x13, []byte{
			0x01,
			0x02, 0x02,
			0x02, 0x03, '1', '2', '0',
			0x08, 0x03, '1', '2', '2',
		}),
	}
	want := PBMEmergencyList{
		Hardcoded: []string{"112", "911"},
		NV:        []string{"999"},
		Card: []PBMSessionEmergencyNumbers{
			{
				Session: PBMSessionGWPrimary,
				Numbers: []PBMEmergencyNumber{
					{Flags: PBMEmergencyPolice, Number: "110"},
					{Flags: PBMEmergencyFireBrigade, Number: "119"},
				},
			},
			{
				Session: PBMSessionGWTertiary,
				Numbers: []PBMEmergencyNumber{{Number: "112"}},
			},
		},
		Network: []PBMSessionEmergencyNumbers{
			{
				Session: PBMSessionGWSecondary,
				Numbers: []PBMEmergencyNumber{
					{Flags: PBMEmergencyAmbulance, Number: "120"},
					{Flags: PBMEmergencyMarineGuard, Number: "122"},
				},
			},
		},
	}
	emptyLists := PBMEmergencyList{
		Hardcoded: []string{},
		NV:        []string{},
		Card:      []PBMSessionEmergencyNumbers{},
		Network:   []PBMSessionEmergencyNumbers{},
	}
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    PBMEmergencyList
		wantErr bool
	}{
		{name: "complete", tlvs: complete, want: want},
		{name: "omitted lists"},
		{
			name: "present empty lists",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, []byte{0}), tlv.Bytes(0x11, []byte{0}),
				tlv.Bytes(0x12, []byte{0}), tlv.Bytes(0x13, []byte{0}),
			},
			want: emptyLists,
		},
		{name: "hardcoded count missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, nil)}, wantErr: true},
		{name: "hardcoded string length missing", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}, wantErr: true},
		{name: "hardcoded string truncated", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1, 3, '1', '1'})}, wantErr: true},
		{name: "NV trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{0, 1})}, wantErr: true},
		{name: "card session missing", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1})}, wantErr: true},
		{name: "card number count missing", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0})}, wantErr: true},
		{name: "card flags missing", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0, 1})}, wantErr: true},
		{name: "card number length missing", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0, 1, 1})}, wantErr: true},
		{name: "card number truncated", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 0, 1, 1, 3, '1'})}, wantErr: true},
		{name: "network trailing data", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{0, 1})}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PBMEmergencyList
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnmarshalTLVs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestClientPBMOperations(t *testing.T) {
	mask := PBMEventPhonebookReady | PBMEventEmergencyNumberList
	tests := []struct {
		name     string
		message  MessageID
		response Response
		call     func(*testing.T, *Client) error
	}{
		{
			name:     "set indication registration",
			message:  MessagePBMIndicationRegister,
			response: successResponse(MessagePBMIndicationRegister, tlv.Bytes(0x10, []byte{0x06, 0x00, 0x00, 0x00})),
			call: func(_ *testing.T, c *Client) error {
				return c.PBMSetIndicationRegistration(context.Background(), mask)
			},
		},
		{
			name:    "get capabilities",
			message: MessagePBMGetCapabilities,
			response: successResponse(
				MessagePBMGetCapabilities,
				tlv.Bytes(0x10, []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x64, 0x00, 0x14, 0x1E}),
			),
			call: func(t *testing.T, c *Client) error {
				got, err := c.PBMCapabilities(context.Background(), PBMSessionGWPrimary, PBMPhonebookADN)
				if err == nil && (!got.BasicKnown || len(got.Basic.Phonebooks) != 1 || got.Basic.Phonebooks[0].MaximumRecords != 100) {
					t.Fatalf("PBMCapabilities() = %+v", got)
				}
				return err
			},
		},
		{
			name:     "get all capabilities",
			message:  MessagePBMGetAllCapabilities,
			response: successResponse(MessagePBMGetAllCapabilities, tlv.Bytes(0x11, []byte{0x01, 0x00, 0x05, 0x14})),
			call: func(t *testing.T, c *Client) error {
				got, err := c.PBMAllCapabilities(context.Background())
				if err == nil && (len(got.Groups) != 1 || got.Groups[0].MaximumGroups != 5) {
					t.Fatalf("PBMAllCapabilities() = %+v", got)
				}
				return err
			},
		},
		{
			name:     "get emergency list",
			message:  MessagePBMGetEmergencyList,
			response: successResponse(MessagePBMGetEmergencyList, tlv.Bytes(0x10, []byte{0x01, 0x03, '1', '1', '2'})),
			call: func(t *testing.T, c *Client) error {
				got, err := c.PBMEmergencyList(context.Background())
				if err == nil && (len(got.Hardcoded) != 1 || got.Hardcoded[0] != "112") {
					t.Fatalf("PBMEmergencyList() = %+v", got)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServicePBM || req.ClientID != 7 || req.MessageID != tt.message {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
				},
				resp: tt.response,
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServicePBM: 7}}
			if err := tt.call(t, client); err != nil {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}

func TestClientPBMSetIndicationRegistrationRejectsUnknownBits(t *testing.T) {
	tests := []struct {
		name string
		mask PBMEventRegistrationMask
	}{
		{name: "unknown bit", mask: 1 << 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			err := client.PBMSetIndicationRegistration(context.Background(), tt.mask)
			if err == nil || !strings.Contains(err.Error(), "contains unknown bits") {
				t.Fatalf("PBMSetIndicationRegistration() error = %v", err)
			}
		})
	}
}
