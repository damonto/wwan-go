package uim

import (
	"bytes"
	"context"
	"encoding"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestCATCommandBinary(t *testing.T) {
	var _ encoding.BinaryMarshaler = CATCommand{}
	var _ encoding.BinaryUnmarshaler = (*CATCommand)(nil)
	var _ io.WriterTo = CATCommand{}
	var _ io.ReaderFrom = (*CATCommand)(nil)

	tests := []struct {
		name    string
		cmd     CATCommand
		want    []byte
		badData []byte
	}{
		{
			name: "raw command",
			cmd:  CATCommand{Ref: 0x01020304, Data: []byte{0xD0, 0x00}},
			want: []byte{0x04, 0x03, 0x02, 0x01, 0x02, 0x00, 0xD0, 0x00},
		},
		{
			name:    "trailing bytes",
			badData: []byte{0x04, 0x03, 0x02, 0x01, 0x00, 0x00, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.badData != nil {
				var decoded CATCommand
				if err := decoded.UnmarshalBinary(tt.badData); err == nil {
					t.Fatal("UnmarshalBinary() error = nil, want non-nil")
				}
				return
			}

			got, err := tt.cmd.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}

			var decoded CATCommand
			if err := decoded.UnmarshalBinary(got); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if decoded.Ref != tt.cmd.Ref || !bytes.Equal(decoded.Data, tt.cmd.Data) {
				t.Fatalf("UnmarshalBinary() = %+v, want %+v", decoded, tt.cmd)
			}
		})
	}
}

func TestCATTerminalResponse(t *testing.T) {
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageCATSendTerminalResponse {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATSendTerminalResponse)
					}
					value, ok := tlv.Value(req.TLVs, 0x01)
					if !ok {
						t.Fatal("terminal response TLV missing")
					}
					want := binary.LittleEndian.AppendUint32(nil, 0xAABBCCDD)
					want = binary.LittleEndian.AppendUint16(want, 3)
					want = append(want, 0x81, 0x01, 0x00)
					if !bytes.Equal(value, want) {
						t.Fatalf("terminal response TLV = % X, want % X", value, want)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{0x01})
				},
				resp: successResponse(qcom.MessageCATSendTerminalResponse),
			}},
		},
		slot:        1,
		catService:  qcom.ServiceCAT2,
		catClientID: 7,
	}

	if err := NewCAT(reader).TerminalResponse(context.Background(), 0xAABBCCDD, []byte{0x81, 0x01, 0x00}); err != nil {
		t.Fatalf("TerminalResponse() error = %v", err)
	}
}

func TestCATTerminalResponseRejectsOversize(t *testing.T) {
	reader := &Reader{
		transport:   &fakeTransport{t: t},
		slot:        1,
		catService:  qcom.ServiceCAT2,
		catClientID: 7,
	}

	if err := NewCAT(reader).TerminalResponse(context.Background(), 1, bytes.Repeat([]byte{0xAA}, catTerminalResponseMaxLength+1)); err == nil {
		t.Fatal("TerminalResponse() error = nil, want non-nil")
	}
}

func TestCATEventConfirmation(t *testing.T) {
	yes := true
	no := false
	tests := []struct {
		name         string
		confirmation CATEventConfirmation
		wantTLVs     map[byte][]byte
	}{
		{
			name: "user and icon confirmation",
			confirmation: CATEventConfirmation{
				UserConfirmed: &yes,
				IconDisplayed: &no,
			},
			wantTLVs: map[byte][]byte{
				0x10: {0x01},
				0x11: {0x00},
				0x12: {0x01},
			},
		},
		{
			name: "icon confirmation only",
			confirmation: CATEventConfirmation{
				IconDisplayed: &no,
			},
			wantTLVs: map[byte][]byte{
				0x11: {0x00},
				0x12: {0x01},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageCATEventConfirmation {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATEventConfirmation)
					}
					for tag, want := range tt.wantTLVs {
						assertTLV(t, req.TLVs, tag, want)
					}
				},
				resp: successResponse(qcom.MessageCATEventConfirmation),
			}}}
			reader := &Reader{
				transport:   transport,
				slot:        1,
				catService:  qcom.ServiceCAT2,
				catClientID: 7,
			}

			if err := NewCAT(reader).EventConfirmation(context.Background(), tt.confirmation); err != nil {
				t.Fatalf("EventConfirmation() error = %v", err)
			}
			if got := transport.callCount(); got != 1 {
				t.Fatalf("Do() calls = %d, want 1", got)
			}
		})
	}
}

func TestCATConfiguration(t *testing.T) {
	tests := []struct {
		name string
		resp qcom.Response
		want CATConfiguration
	}{
		{
			name: "mode only",
			resp: successResponse(qcom.MessageCATGetConfiguration,
				tlv.Bytes(0x10, []byte{byte(CATConfigGobi)}),
			),
			want: CATConfiguration{Mode: CATConfigGobi},
		},
		{
			name: "custom profile",
			resp: successResponse(qcom.MessageCATGetConfiguration,
				tlv.Bytes(0x10, []byte{byte(CATConfigCustomRaw)}),
				tlv.Bytes(0x11, []byte{0x02, 0x11, 0x22}),
			),
			want: CATConfiguration{Mode: CATConfigCustomRaw, CustomProfile: []byte{0x11, 0x22}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				transport: &fakeTransport{
					t: t,
					calls: []transportCall{{
						check: func(req qcom.Request) {
							if req.MessageID != qcom.MessageCATGetConfiguration {
								t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATGetConfiguration)
							}
						},
						resp: tt.resp,
					}},
				},
				slot:        1,
				catService:  qcom.ServiceCAT2,
				catClientID: 7,
			}

			got, err := NewCAT(reader).Configuration(context.Background())
			if err != nil {
				t.Fatalf("Configuration() error = %v", err)
			}
			if got.Mode != tt.want.Mode || !bytes.Equal(got.CustomProfile, tt.want.CustomProfile) {
				t.Fatalf("Configuration() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCATSetConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config CATConfiguration
		want   []byte
	}{
		{
			name:   "mode only",
			config: CATConfiguration{Mode: CATConfigGobi},
		},
		{
			name:   "custom profile",
			config: CATConfiguration{Mode: CATConfigCustomRaw, CustomProfile: []byte{0x11, 0x22}},
			want:   []byte{0x02, 0x11, 0x22},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				transport: &fakeTransport{
					t: t,
					calls: []transportCall{{
						check: func(req qcom.Request) {
							if req.MessageID != qcom.MessageCATSetConfiguration {
								t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATSetConfiguration)
							}
							assertTLV(t, req.TLVs, 0x01, []byte{byte(tt.config.Mode)})
							value, ok := tlv.Value(req.TLVs, 0x10)
							if !bytes.Equal(value, tt.want) || ok != (tt.want != nil) {
								t.Fatalf("custom profile TLV = % X, present %v; want % X", value, ok, tt.want)
							}
						},
						resp: successResponse(qcom.MessageCATSetConfiguration),
					}},
				},
				slot:        1,
				catService:  qcom.ServiceCAT2,
				catClientID: 7,
			}

			if err := NewCAT(reader).SetConfiguration(context.Background(), tt.config); err != nil {
				t.Fatalf("SetConfiguration() error = %v", err)
			}
		})
	}
}

func TestCATEnvelope(t *testing.T) {
	envelope := []byte{0xD3, 0x07, 0x82, 0x02, 0x01, 0x81, 0x90, 0x01, 0x02}
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageCATSendEnvelope {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATSendEnvelope)
					}
					value := binary.LittleEndian.AppendUint16(nil, 0x01)
					value = binary.LittleEndian.AppendUint16(value, uint16(len(envelope)))
					value = append(value, envelope...)
					assertTLV(t, req.TLVs, 0x01, value)
					assertTLV(t, req.TLVs, 0x10, []byte{0x01})
				},
				resp: successResponse(qcom.MessageCATSendEnvelope, tlv.Bytes(0x10, []byte{0x90, 0x00, 0x01, 0xAA})),
			}},
		},
		slot:        1,
		catService:  qcom.ServiceCAT2,
		catClientID: 7,
	}

	got, err := NewCAT(reader).Envelope(context.Background(), envelope, 0x01)
	if err != nil {
		t.Fatalf("Envelope() error = %v", err)
	}
	if got.SW1 != 0x90 || got.SW2 != 0x00 || !bytes.Equal(got.Data, []byte{0xAA}) {
		t.Fatalf("Envelope() = %+v, want 9000 AA", got)
	}
}

func TestCATTerminalProfile(t *testing.T) {
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageCATGetTerminalProfile {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATGetTerminalProfile)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{0x02})
				},
				resp: successResponse(qcom.MessageCATGetTerminalProfile, tlv.Bytes(0x10, []byte{0x02, 0xAA, 0x55})),
			}},
		},
		slot:        2,
		catService:  qcom.ServiceCAT2,
		catClientID: 7,
	}

	got, err := NewCAT(reader).TerminalProfile(context.Background())
	if err != nil {
		t.Fatalf("TerminalProfile() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0xAA, 0x55}) {
		t.Fatalf("TerminalProfile() = % X, want AA 55", got)
	}
}

func TestCATCommandsDecodeRawIndication(t *testing.T) {
	raw := []byte{0xD0, 0x0E, 0x81, 0x03, 0x01, 0x21, 0x00, 0x82, 0x02, 0x81, 0x02, 0x8D, 0x03, 0x04, 'H', 'i'}
	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageCATSetEventReport {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATSetEventReport)
						}
						assertTLV(t, req.TLVs, 0x10, []byte{0x01, 0x00, 0x00, 0x00})
						assertTLV(t, req.TLVs, 0x12, []byte{0x01})
					},
					resp: successResponse(qcom.MessageCATSetEventReport),
				},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageReleaseClientID {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageReleaseClientID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceCAT2), 7})
					},
					resp: successResponse(qcom.MessageReleaseClientID),
				},
			},
		},
	}
	reader := &Reader{
		transport:   transport,
		slot:        1,
		catService:  qcom.ServiceCAT2,
		catClientID: 7,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands, err := NewCAT(reader).Commands(ctx, 1, 0)
	if err != nil {
		t.Fatalf("Commands() error = %v", err)
	}

	value := binary.LittleEndian.AppendUint32(nil, 0x01020304)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(raw)))
	value = append(value, raw...)
	transport.emit(qcom.Indication{TLVs: tlv.TLVs{tlv.Bytes(0x10, value)}})

	select {
	case got := <-commands:
		if got.Ref != 0x01020304 {
			t.Fatalf("ref = 0x%08X, want 0x01020304", got.Ref)
		}
		if !bytes.Equal(got.Data, raw) {
			t.Fatalf("command data = % X, want % X", got.Data, raw)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command")
	}
	cancel()
	transport.waitCalls(t, 2)
}

func TestCATCommandsRejectsRegistrationErrorMask(t *testing.T) {
	tests := []struct {
		name    string
		raw     uint32
		full    uint32
		respTLV tlv.TLV
	}{
		{
			name:    "raw",
			raw:     0x01,
			respTLV: tlv.Uint(0x10, uint32(0x01)),
		},
		{
			name:    "full function",
			raw:     0x20,
			full:    0x01,
			respTLV: tlv.Uint(0x12, uint32(0x01)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				transport: &fakeIndicationTransport{
					fakeTransport: fakeTransport{
						t: t,
						calls: []transportCall{{
							check: func(req qcom.Request) {
								if req.MessageID != qcom.MessageCATSetEventReport {
									t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageCATSetEventReport)
								}
							},
							resp: successResponse(qcom.MessageCATSetEventReport, tt.respTLV),
						}, {
							check: func(req qcom.Request) {
								if req.MessageID != qcom.MessageReleaseClientID {
									t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageReleaseClientID)
								}
							},
							resp: successResponse(qcom.MessageReleaseClientID),
						}},
					},
				},
				slot:        1,
				catService:  qcom.ServiceCAT2,
				catClientID: 7,
			}

			_, err := NewCAT(reader).Commands(context.Background(), tt.raw, tt.full)
			if err == nil {
				t.Fatal("Commands() error = nil, want non-nil")
			}
		})
	}
}

func TestDecodeCATCommandIgnoresGobiSetupEventListIndication(t *testing.T) {
	_, err := decodeCATCommand(tlv.TLVs{
		tlv.Uint(0x16, uint32(0x0F)),
	})
	if err == nil {
		t.Fatal("decodeCATCommand() error = nil, want non-nil")
	}
}
