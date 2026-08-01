package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestVoiceOriginateUSSDNoWait(t *testing.T) {
	tests := []struct {
		name string
		data VoiceUSSDData
	}{
		{name: "asynchronous USSD result", data: VoiceUSSDData{Encoding: VoiceUSSDEncodingASCII, Data: []byte("*123#")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := &voiceUSSDNoWaitTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if req.Service != ServiceVoice || req.ClientID != 7 || req.MessageID != MessageVoiceOriginateUSSDNoWait {
						t.Fatalf("request = service 0x%02X client %d message 0x%04X", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(VoiceUSSDEncodingASCII), 5, '*', '1', '2', '3', '#'})
				},
				resp: successResponse(MessageVoiceOriginateUSSDNoWait),
			}}}}
			client := &Client{transport: transport, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}

			results, err := client.VoiceOriginateUSSDNoWait(ctx, tt.data)
			if err != nil {
				t.Fatalf("VoiceOriginateUSSDNoWait() error = %v", err)
			}
			if transport.service != ServiceVoice || transport.clientID != 7 || transport.message != MessageVoiceOriginateUSSDNoWait {
				t.Fatalf("subscription = service 0x%02X client %d message 0x%04X", transport.service, transport.clientID, transport.message)
			}

			transport.emit(Indication{TLVs: tlv.TLVs{
				tlv.Bytes(0x10, binary.LittleEndian.AppendUint16(nil, 42)),
				tlv.Bytes(0x11, binary.LittleEndian.AppendUint16(nil, 0x1234)),
				tlv.Bytes(0x12, []byte{byte(VoiceUSSDEncodingASCII), 2, 'o', 'k'}),
				tlv.Bytes(0x13, []byte{byte(VoiceAlphaEncodingGSM), 2, 'h', 'i'}),
				tlv.Bytes(0x14, []byte{2, 'A', 0, 'B', 0}),
				tlv.Bytes(0x15, []byte{2, 0, 'E', 0, 'R', 0}),
				tlv.Bytes(0x16, binary.LittleEndian.AppendUint16(nil, 500)),
			}})

			select {
			case got := <-results:
				if got.Err != nil {
					t.Fatalf("result error = %v", got.Err)
				}
				if !got.ProtocolErrorKnown || got.ProtocolError != QMIError(42) || !got.FailureCauseKnown || got.FailureCause != 0x1234 {
					t.Fatalf("protocol result = %+v", got)
				}
				if !got.DataKnown || string(got.Data.Data) != "ok" || !got.AlphaKnown || string(got.Alpha.Data) != "hi" {
					t.Fatalf("text result = %+v", got)
				}
				if len(got.UTF16) != 2 || got.UTF16[0] != 'A' || len(got.FailureCauseText) != 2 || got.FailureCauseText[0] != 'E' {
					t.Fatalf("UTF-16 result = %+v", got)
				}
				if !got.SIPErrorCodeKnown || got.SIPErrorCode != 500 {
					t.Fatalf("SIP result = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for USSD no-wait result")
			}
		})
	}
}

func TestDecodeVoiceUSSDNoWaitResultRejectsMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{name: "error", tlvs: tlv.TLVs{tlv.Bytes(0x10, []byte{1})}},
		{name: "failure cause", tlvs: tlv.TLVs{tlv.Bytes(0x11, []byte{1})}},
		{name: "USSD data", tlvs: tlv.TLVs{tlv.Bytes(0x12, []byte{1, 2, 'x'})}},
		{name: "alpha", tlvs: tlv.TLVs{tlv.Bytes(0x13, []byte{1, 2, 'x'})}},
		{name: "UTF-16", tlvs: tlv.TLVs{tlv.Bytes(0x14, []byte{2, 'A', 0})}},
		{name: "failure text", tlvs: tlv.TLVs{tlv.Bytes(0x15, []byte{2, 0, 'E', 0})}},
		{name: "SIP error", tlvs: tlv.TLVs{tlv.Bytes(0x16, []byte{1})}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := new(VoiceUSSDNoWaitResult).UnmarshalTLVs(tt.tlvs); err == nil {
				t.Fatal("UnmarshalTLVs() error = nil, want non-nil")
			}
		})
	}
}

func TestDecodeVoiceUSSDResult(t *testing.T) {
	tests := []struct {
		name string
		tlvs tlv.TLVs
	}{
		{
			name: "all libqmi fields",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, binary.LittleEndian.AppendUint16(nil, 0x1234)),
				tlv.Bytes(0x11, []byte{byte(VoiceAlphaEncodingGSM), 2, 'h', 'i'}),
				tlv.Bytes(0x12, []byte{byte(VoiceUSSDEncodingASCII), 2, 'o', 'k'}),
				tlv.Bytes(0x13, []byte{byte(VoiceCallControlResultSupplementaryService)}),
				tlv.Bytes(0x14, []byte{7}),
				tlv.Bytes(0x15, []byte{byte(VoiceSupplementaryServiceUSSD)}),
				tlv.Bytes(0x16, []byte{2, 'A', 0, 'B', 0}),
				tlv.Bytes(0x18, binary.LittleEndian.AppendUint16(nil, 500)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got VoiceUSSDResult
			err := got.UnmarshalTLVs(tt.tlvs)
			if err != nil {
				t.Fatalf("VoiceUSSDResult.UnmarshalTLVs() error = %v", err)
			}
			if !got.FailureCauseKnown || got.FailureCause != 0x1234 || !got.DataKnown || string(got.Data.Data) != "ok" {
				t.Fatalf("network result = %+v", got)
			}
			if !got.AlphaKnown || string(got.Alpha.Data) != "hi" || !got.CallControlResultKnown || got.CallControlResult != VoiceCallControlResultSupplementaryService {
				t.Fatalf("call-control result = %+v", got)
			}
			if !got.CallIDKnown || got.CallID != 7 || !got.CallControlSupplementaryServiceKnown || got.CallControlSupplementaryService != VoiceSupplementaryServiceUSSD {
				t.Fatalf("supplementary result = %+v", got)
			}
			if len(got.UTF16) != 2 || got.UTF16[0] != 'A' || !got.SIPErrorCodeKnown || got.SIPErrorCode != 500 {
				t.Fatalf("extended result = %+v", got)
			}
		})
	}
}

func TestVoiceUSSDDecodersRejectMalformedValues(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "invalid USSD encoding",
			fn: func() error {
				var data VoiceUSSDData
				return data.UnmarshalBinary([]byte{0xFF, 1, 'x'})
			},
		},
		{
			name: "USSD trailing data",
			fn: func() error {
				var data VoiceUSSDData
				return data.UnmarshalBinary([]byte{byte(VoiceUSSDEncodingASCII), 1, 'x', 'y'})
			},
		},
		{
			name: "alpha trailing data",
			fn: func() error {
				return new(VoiceAlphaIdentifier).UnmarshalBinary([]byte{byte(VoiceAlphaEncodingGSM), 1, 'x', 'y'})
			},
		},
		{
			name: "failure cause trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x10, []byte{1, 0, 2})})
			},
		},
		{
			name: "call-control result trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x13, []byte{1, 2})})
			},
		},
		{
			name: "call ID trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x14, []byte{1, 2})})
			},
		},
		{
			name: "supplementary service trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x15, []byte{1, 2})})
			},
		},
		{
			name: "UTF-16 trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x16, []byte{1, 'A', 0, 0})})
			},
		},
		{
			name: "SIP error trailing data",
			fn: func() error {
				var result VoiceUSSDResult
				return result.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x18, []byte{1, 0, 2})})
			},
		},
		{
			name: "indication action trailing data",
			fn: func() error {
				var event VoiceUSSDEvent
				return event.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x01, []byte{1, 2})})
			},
		},
		{
			name: "indication SIP error trailing data",
			fn: func() error {
				var event VoiceUSSDEvent
				return event.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, []byte{1}),
					tlv.Bytes(0x13, []byte{1, 0, 2}),
				})
			},
		},
		{
			name: "uint16 array trailing data",
			fn: func() error {
				_, err := decodeVoiceUint16Array([]byte{1, 0, 'A', 0, 0}, true)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("decode() error = nil, want non-nil")
			}
		})
	}
}

type voiceUSSDNoWaitTransport struct {
	fakeTransport
	service     ServiceType
	clientID    uint8
	message     MessageID
	indications chan Indication
}

func (t *voiceUSSDNoWaitTransport) Indications(ctx context.Context, service ServiceType, clientID uint8, message MessageID) (<-chan Indication, error) {
	t.service = service
	t.clientID = clientID
	t.message = message
	t.indications = make(chan Indication, 1)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *voiceUSSDNoWaitTransport) emit(indication Indication) {
	t.indications <- indication
}
