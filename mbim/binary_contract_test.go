package mbim

import (
	"bytes"
	"encoding"
	"testing"
)

type binaryCodec interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

func TestBinaryUnmarshalOwnsInput(t *testing.T) {
	tests := []struct {
		name   string
		frame  []byte
		decode func([]byte) ([]byte, error)
	}{
		{
			name:  "command response",
			frame: mbimCommandDone(1, ServiceBasicConnect, CIDRadioState, []byte{1, 2, 3, 4}),
			decode: func(data []byte) ([]byte, error) {
				var response CommandResponse
				if err := response.UnmarshalBinary(data); err != nil {
					return nil, err
				}
				return response.ResponseBuffer, nil
			},
		},
		{
			name:  "indication",
			frame: mbimIndication(ServiceBasicConnect, CIDRadioState, []byte{1, 2, 3, 4}),
			decode: func(data []byte) ([]byte, error) {
				var indication Indication
				if err := indication.UnmarshalBinary(data); err != nil {
					return nil, err
				}
				return indication.InformationBuffer, nil
			},
		},
		{
			name:  "fragment",
			frame: mbimIndication(ServiceBasicConnect, CIDRadioState, []byte{1, 2, 3, 4}),
			decode: func(data []byte) ([]byte, error) {
				var value fragment
				if err := value.UnmarshalBinary(data); err != nil {
					return nil, err
				}
				return value.payload, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retained, err := tt.decode(tt.frame)
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			want := bytes.Clone(retained)
			for i := range tt.frame {
				tt.frame[i] ^= 0xff
			}
			if !bytes.Equal(retained, want) {
				t.Fatalf("retained data changed to %x after input mutation, want %x", retained, want)
			}
		})
	}
}

func TestEmptyResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name   string
		decode func([]byte) error
	}{
		{name: "open", decode: new(OpenDeviceResponse).UnmarshalBinary},
		{name: "close", decode: new(CloseResponse).UnmarshalBinary},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.decode(nil); err != nil {
				t.Fatalf("UnmarshalBinary(nil) error = %v", err)
			}
			if err := tt.decode([]byte{0}); err == nil {
				t.Fatal("UnmarshalBinary(non-empty) error = nil, want non-nil")
			}
		})
	}
}

func TestBinaryRecordRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		value    binaryCodec
		newCodec func() binaryCodec
	}{
		{
			name:     "SAR configuration state",
			value:    &SARConfigState{AntennaIndex: 2, BackoffIndex: 7},
			newCodec: func() binaryCodec { return new(SARConfigState) },
		},
		{
			name:     "URSP traffic descriptor",
			value:    &URSPTrafficDescriptor{Precedence: 3, Data: []byte{0xaa, 0xbb}},
			newCodec: func() binaryCodec { return new(URSPTrafficDescriptor) },
		},
		{
			name: "rejected S-NSSAI",
			value: &RejectedSNSSAI{
				Cause: RejectedNSSAINotAvailableInRegistrationArea,
				SNSSAI: SNSSAI{
					SliceServiceType:       2,
					SliceDifferentiator:    [3]byte{1, 2, 3},
					HasSliceDifferentiator: true,
				},
			},
			newCodec: func() binaryCodec { return new(RejectedSNSSAI) },
		},
		{
			name:     "preconfigured default NSSAI without S-NSSAI",
			value:    &PreconfiguredDefaultNSSAI{AccessType: AccessTypeNon3GPP},
			newCodec: func() binaryCodec { return new(PreconfiguredDefaultNSSAI) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.value.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			decoded := tt.newCodec()
			if err := decoded.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			got, err := decoded.MarshalBinary()
			if err != nil {
				t.Fatalf("round-trip MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("round-trip data = %x, want %x", got, data)
			}
			if err := tt.newCodec().UnmarshalBinary(append(bytes.Clone(data), 0)); err == nil {
				t.Fatal("UnmarshalBinary(trailing data) error = nil, want non-nil")
			}
		})
	}
}

func TestRequestMarshalBinaryRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
	}{
		{name: "nil request"},
		{name: "nil command", request: new(Request)},
		{
			name: "nil wire command",
			request: &Request{
				Command: (*Command)(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.request.MarshalBinary(); err == nil {
				t.Fatal("MarshalBinary() error = nil, want non-nil")
			}
		})
	}
}

func TestPublicRequestBuildersPropagateEncodingErrors(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
	}{
		{
			name: "SAR configuration",
			request: (&SARConfigSetRequest{
				Config: SARConfig{Mode: SARControlModeOS + 1},
			}).Request(),
		},
		{
			name: "network parameters version",
			request: (&NetworkParametersRequest{
				MBIMExVersion: mbimExVersion20,
			}).Request(),
		},
		{
			name: "PCO type",
			request: (&PCORequest{
				Value: PCOValue{Type: PCOTypePartial + 1},
			}).Request(),
		},
		{
			name: "IP packet filter",
			request: (&IPPacketFiltersSetRequest{
				Filters: IPPacketFiltersInfo{
					Filters: []PacketFilter{{Filter: []byte{1}, Mask: []byte{1, 2}}},
				},
			}).Request(),
		},
		{
			name: "provider",
			request: (&HomeProviderSetRequest{
				Provider: Provider{ID: "invalid"},
			}).Request(),
		},
		{
			name: "connection",
			request: (&ConnectRequest{
				ActivationCommand: ActivationCommandActivate + 1,
			}).Request(),
		},
		{
			name: "visible providers action",
			request: (&VisibleProvidersRequest{
				Action: VisibleProvidersActionRestrictedScan + 1,
			}).Request(),
		},
		{
			name: "network idle hint",
			request: (&NetworkIdleHintSetRequest{
				Hint: NetworkIdleHintEnabled + 1,
			}).Request(),
		},
		{
			name: "UE policy TLV",
			request: (&UEPolicyRequest{
				Query: TLVs{{Type: 0}},
			}).Request(),
		},
		{
			name: "registration parameters version",
			request: (&RegistrationParametersSetRequest{
				MBIMExVersion: mbimExVersion20,
			}).Request(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.request.MarshalBinary(); err == nil {
				t.Fatal("MarshalBinary() error = nil, want non-nil")
			}
		})
	}
}
