package qcom

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestVoiceStatusIndicationDecoding(t *testing.T) {
	history := []byte{2, 0, 'a', 0, 'b', 0}
	tests := []struct {
		name string
		call func(*testing.T) error
	}{
		{
			name: "supplementary notification",
			call: func(t *testing.T) error {
				var got VoiceSupplementaryNotification
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, []byte{7, byte(VoiceSupplementaryIncomingECT)}),
					tlv.Bytes(0x10, binary.LittleEndian.AppendUint16(nil, 42)),
					tlv.Bytes(0x11, []byte{byte(VoiceECTCallActive), byte(VoicePresentationAllowed), 4, '1', '2', '3', '4'}),
					tlv.Bytes(0x12, binary.LittleEndian.AppendUint32(nil, uint32(VoiceSupplementaryCodeForwardBusy))),
					tlv.Bytes(0x13, history),
					tlv.Bytes(0x14, binary.LittleEndian.AppendUint64(nil, uint64(VoiceCallAttributeTX|VoiceCallAttributeRX))),
				})
				if err == nil && (got.CallID != 7 || got.Type != VoiceSupplementaryIncomingECT || !got.CUGIndexKnown || got.CUGIndex != 42 || !got.ECTNumberKnown || got.ECTNumber.Number != "1234" || !got.CodeKnown || got.Code != VoiceSupplementaryCodeForwardBusy || !got.ForwardHistoryKnown || len(got.ForwardHistory) != 2 || !got.MediaDirectionKnown || got.MediaDirection != VoiceCallAttributeTX|VoiceCallAttributeRX) {
					t.Fatalf("notification = %+v", got)
				}
				return err
			},
		},
		{
			name: "supplementary result",
			call: func(t *testing.T) error {
				var got VoiceSupplementaryResultEvent
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, []byte{byte(VoiceSupplementaryServiceInterrogate), 1}),
					tlv.Bytes(0x10, []byte{1}),
					tlv.Bytes(0x11, []byte{byte(VoiceReasonForwardNoReply)}),
					tlv.Bytes(0x12, []byte("+123")),
					tlv.Bytes(0x13, []byte{20}),
					tlv.Bytes(0x14, []byte{byte(VoiceUSSDEncodingASCII), 2, 'o', 'k'}),
					tlv.Bytes(0x15, []byte{7}),
					tlv.Bytes(0x16, []byte{byte(VoiceAlphaEncodingGSM), 2, 'h', 'i'}),
					tlv.Bytes(0x17, []byte("1234")),
					tlv.Bytes(0x18, []byte("56785678")),
					tlv.Bytes(0x19, []byte{byte(VoiceSupplementaryDataSourceNetwork)}),
					tlv.Bytes(0x1A, binary.LittleEndian.AppendUint16(nil, 42)),
					tlv.Bytes(0x1B, []byte{1, 1, 1, 4, '+', '1', '2', '3', 15}),
					tlv.Bytes(0x1C, []byte{1, 2}),
					tlv.Bytes(0x21, []byte{2, 'a', 0, 'b', 0}),
					tlv.Bytes(0x22, binary.LittleEndian.AppendUint32(nil, uint32(VoiceServiceClassVoice))),
					tlv.Bytes(0x23, []byte{2, 3, '1', '2', '3', 2, '4', '5'}),
				})
				if err == nil && (got.Service != VoiceSupplementaryServiceInterrogate || !got.ModifiedByCallControl || !got.ServiceClassKnown || got.Reason != VoiceReasonForwardNoReply || got.Number != "+123" || !got.USSDKnown || string(got.USSD.Data) != "ok" || !got.AlphaKnown || string(got.Alpha.Data) != "hi" || got.Password != "1234" || got.NewPassword != "5678" || got.DataSource != VoiceSupplementaryDataSourceNetwork || got.FailureCause != 42 || len(got.CallForwarding) != 1 || !got.CLIR.Known || len(got.USSUTF16) != 2 || !got.ExtendedClassKnown || len(got.BarredNumbers) != 2) {
					t.Fatalf("supplementary result = %+v", got)
				}
				return err
			},
		},
		{
			name: "speech codec",
			call: func(t *testing.T) error {
				var got VoiceSpeechCodecInfo
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x10, binary.LittleEndian.AppendUint32(nil, uint32(VoiceNetworkModeLTE))),
					tlv.Bytes(0x11, binary.LittleEndian.AppendUint32(nil, uint32(VoiceSpeechCodecAMRWB))),
					tlv.Bytes(0x12, binary.LittleEndian.AppendUint32(nil, 16000)),
					tlv.Bytes(0x13, []byte{7}),
				})
				if err == nil && (!got.CallIDKnown || got.CallID != 7 || !got.NetworkModeKnown || got.NetworkMode != VoiceNetworkModeLTE || !got.CodecKnown || got.Codec != VoiceSpeechCodecAMRWB || !got.SamplingRateKnown || got.SamplingRate != 16000) {
					t.Fatalf("speech codec = %+v", got)
				}
				return err
			},
		},
		{
			name: "handover",
			call: func(t *testing.T) error {
				var got VoiceHandover
				err := got.UnmarshalTLVs(tlv.TLVs{
					tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, uint32(VoiceHandoverComplete))),
					tlv.Bytes(0x10, binary.LittleEndian.AppendUint32(nil, uint32(VoiceHandoverSRVCCLTEToWCDMA))),
				})
				if err == nil && (got.State != VoiceHandoverComplete || !got.TypeKnown || got.Type != VoiceHandoverSRVCCLTEToWCDMA) {
					t.Fatalf("handover = %+v", got)
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(t); err != nil {
				t.Fatalf("decode() error = %v", err)
			}
		})
	}
}

func TestVoiceStatusDecodersRejectMalformedTLVs(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "supplementary info missing", call: func() error {
			var event VoiceSupplementaryNotification
			return event.UnmarshalTLVs(nil)
		}},
		{name: "ECT number truncated", call: func() error {
			var event VoiceSupplementaryNotification
			return event.UnmarshalTLVs(tlv.TLVs{
				tlv.Bytes(0x01, []byte{1, 1}),
				tlv.Bytes(0x11, []byte{2, 0, 4, '1'}),
			})
		}},
		{name: "supplementary result info missing", call: func() error {
			var event VoiceSupplementaryResultEvent
			return event.UnmarshalTLVs(nil)
		}},
		{name: "supplementary result alpha truncated", call: func() error {
			var event VoiceSupplementaryResultEvent
			return event.UnmarshalTLVs(tlv.TLVs{
				tlv.Bytes(0x01, []byte{1, 0}),
				tlv.Bytes(0x16, []byte{1, 3, 'a'}),
			})
		}},
		{name: "forward history truncated", call: func() error {
			var event VoiceSupplementaryNotification
			return event.UnmarshalTLVs(tlv.TLVs{
				tlv.Bytes(0x01, []byte{1, 1}),
				tlv.Bytes(0x13, []byte{2, 0, 'a', 0}),
			})
		}},
		{name: "speech codec width", call: func() error {
			var info VoiceSpeechCodecInfo
			return info.UnmarshalTLVs(tlv.TLVs{tlv.Bytes(0x11, []byte{1})})
		}},
		{name: "handover state missing", call: func() error {
			var handover VoiceHandover
			return handover.UnmarshalTLVs(nil)
		}},
		{name: "handover type width", call: func() error {
			var handover VoiceHandover
			return handover.UnmarshalTLVs(tlv.TLVs{
				tlv.Bytes(0x01, make([]byte, 4)),
				tlv.Bytes(0x10, []byte{1}),
			})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("decode() error = nil, want error")
			}
		})
	}
}

func TestVoiceStatusWatchers(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, context.Context, *Client, *voiceIndicationTransport) error
	}{
		{
			name: "supplementary",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchSupplementaryNotifications(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoiceSupplementaryNotify, TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{7, byte(VoiceSupplementaryCallHeld)}),
				}})
				select {
				case got := <-out:
					if got.CallID != 7 || got.Type != VoiceSupplementaryCallHeld {
						t.Fatalf("notification = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for supplementary notification")
				}
				return nil
			},
		},
		{
			name: "supplementary result",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchSupplementaryResults(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoiceSupplementaryResult, TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{byte(VoiceSupplementaryServiceRegister), 0}),
				}})
				select {
				case got := <-out:
					if got.Service != VoiceSupplementaryServiceRegister {
						t.Fatalf("supplementary result = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for supplementary result")
				}
				return nil
			},
		},
		{
			name: "speech codec",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchSpeechCodec(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoiceSpeechCodecInfo, TLVs: tlv.TLVs{
					tlv.Bytes(0x11, binary.LittleEndian.AppendUint32(nil, uint32(VoiceSpeechCodecEVSWB))),
				}})
				select {
				case got := <-out:
					if !got.CodecKnown || got.Codec != VoiceSpeechCodecEVSWB {
						t.Fatalf("speech codec = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for speech codec")
				}
				return nil
			},
		},
		{
			name: "handover",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchHandover(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoiceHandover, TLVs: tlv.TLVs{
					tlv.Bytes(0x01, binary.LittleEndian.AppendUint32(nil, uint32(VoiceHandoverStart))),
				}})
				select {
				case got := <-out:
					if got.State != VoiceHandoverStart {
						t.Fatalf("handover = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for handover")
				}
				return nil
			},
		},
		{
			name: "privacy",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchPrivacy(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoicePrivacy, TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{7, byte(VoicePrivacyEnhanced)}),
				}})
				select {
				case got := <-out:
					if got.CallID != 7 || got.Privacy != VoicePrivacyEnhanced {
						t.Fatalf("privacy = %+v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for privacy")
				}
				return nil
			},
		},
		{
			name: "TTY",
			run: func(t *testing.T, ctx context.Context, c *Client, transport *voiceIndicationTransport) error {
				out, err := c.VoiceWatchTTY(ctx)
				if err != nil {
					return err
				}
				transport.emit(Indication{Service: ServiceVoice, ClientID: 7, MessageID: MessageVoiceTTY, TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{byte(VoiceTTYVCO)}),
				}})
				select {
				case got := <-out:
					if got != VoiceTTYVCO {
						t.Fatalf("TTY mode = %d, want VCO", got)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for TTY mode")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := &voiceIndicationTransport{fakeTransport: fakeTransport{t: t, calls: []transportCall{
				{resp: successResponse(MessageVoiceIndicationRegister)},
				{resp: successResponse(MessageVoiceIndicationRegister)},
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceVoice: 7}}
			if err := tt.run(t, ctx, client, transport); err != nil {
				cancel()
				t.Fatalf("watch() error = %v", err)
			}
			cancel()
			transport.waitCalls(t, 2)
		})
	}
}
