package qcom

import (
	"bytes"
	"context"
	"encoding"
	"strings"
	"testing"

	"github.com/damonto/wwan-go/qcom/tlv"
)

var (
	_ encoding.BinaryMarshaler   = OpenLogicalChannelRequest{}
	_ encoding.BinaryUnmarshaler = (*OpenLogicalChannelRequest)(nil)
	_ encoding.BinaryUnmarshaler = (*OpenLogicalChannelResponse)(nil)
	_ encoding.BinaryMarshaler   = CloseLogicalChannelRequest{}
	_ encoding.BinaryUnmarshaler = (*CloseLogicalChannelRequest)(nil)
	_ encoding.BinaryUnmarshaler = (*CloseLogicalChannelResponse)(nil)
	_ encoding.BinaryMarshaler   = SendAPDURequest{}
	_ encoding.BinaryUnmarshaler = (*SendAPDURequest)(nil)
	_ encoding.BinaryUnmarshaler = (*SendAPDUResponse)(nil)
)

func TestClientLogicalChannelPrimitives(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	command := []byte{0x00, 0xA4, 0x04, 0x00, byte(len(aid))}
	command = append(command, aid...)

	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.ClientID != 7 || req.MessageID != MessageOpenLogicalChannel {
						t.Fatalf("request = service %#x client %d message 0x%04X, want open logical channel", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, append([]byte{byte(len(aid))}, aid...))
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				},
				resp: successResponse(MessageOpenLogicalChannel, tlv.Bytes(0x10, []byte{0x03})),
			},
			{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.ClientID != 7 || req.MessageID != MessageSendAPDU {
						t.Fatalf("request = service %#x client %d message 0x%04X, want send APDU", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{0x03})
					assertTLV(t, req.TLVs, 0x02, encodeLengthPrefixed(command))
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				},
				resp: successResponse(MessageSendAPDU, tlv.Bytes(0x10, encodeLengthPrefixed([]byte{0x90, 0x00}))),
			},
			{
				check: func(req Request) {
					if req.Service != ServiceUIM || req.ClientID != 7 || req.MessageID != MessageCloseLogicalChannel {
						t.Fatalf("request = service %#x client %d message 0x%04X, want close logical channel", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
					assertTLV(t, req.TLVs, 0x11, []byte{0x03})
				},
				resp: successResponse(MessageCloseLogicalChannel),
			},
		},
	}
	reader := &Client{
		transport: transport,
		slot:      2,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	channel, err := reader.OpenLogicalChannel(context.Background(), aid)
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if channel != 3 {
		t.Fatalf("OpenLogicalChannel() = %d, want 3", channel)
	}

	got, err := reader.SendAPDU(context.Background(), channel, command)
	if err != nil {
		t.Fatalf("SendAPDU() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0x90, 0x00}) {
		t.Fatalf("SendAPDU() = % X, want 90 00", got)
	}

	if err := reader.CloseLogicalChannel(context.Background(), channel); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}
	if transport.idx != len(transport.calls) {
		t.Fatalf("Do() calls = %d, want %d", transport.idx, len(transport.calls))
	}
}

func TestOpenLogicalChannelOmitsEmptyAID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "open master file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t, calls: []transportCall{{
				check: func(req Request) {
					if _, ok := tlv.Value(req.TLVs, 0x10); ok {
						t.Fatal("OpenLogicalChannelWithConfig() includes an empty AID TLV")
					}
					assertTLV(t, req.TLVs, 0x01, []byte{1})
				},
				resp: successResponse(MessageOpenLogicalChannel, tlv.Bytes(0x10, []byte{1})),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			response, err := client.OpenLogicalChannelWithConfig(context.Background(), UIMOpenLogicalChannelConfig{})
			if err != nil {
				t.Fatalf("OpenLogicalChannelWithConfig() error = %v", err)
			}
			if response.Channel != 1 {
				t.Fatalf("OpenLogicalChannelWithConfig().Channel = %d, want 1", response.Channel)
			}
		})
	}
}

func TestUIMLogicalChannelOptions(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "select response and procedure bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fci := UIMFileControlFCI
			procedureBytes := UIMAPDUSkipProcedureBytes
			transport := &fakeTransport{t: t, calls: []transportCall{
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x11, []byte{byte(UIMFileControlFCI)})
					},
					resp: successResponse(
						MessageOpenLogicalChannel,
						tlv.Bytes(0x10, []byte{3}),
						tlv.Bytes(0x11, []byte{0x90, 0x00}),
						tlv.Bytes(0x12, []byte{2, 0x62, 0x00}),
					),
				},
				{
					check: func(req Request) {
						assertTLV(t, req.TLVs, 0x11, []byte{byte(UIMAPDUSkipProcedureBytes)})
					},
					resp: successResponse(MessageSendAPDU, tlv.Bytes(0x10, encodeLengthPrefixed([]byte{0x90, 0x00}))),
				},
			}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			opened, err := client.OpenLogicalChannelWithConfig(context.Background(), UIMOpenLogicalChannelConfig{
				AID:                    []byte{0xA0, 0x00},
				FileControlInformation: &fci,
			})
			if err != nil {
				t.Fatalf("OpenLogicalChannelWithConfig() error = %v", err)
			}
			if opened.Channel != 3 || !bytes.Equal(opened.SelectResponse, []byte{0x62, 0x00}) {
				t.Fatalf("OpenLogicalChannelWithConfig() = %+v", opened)
			}

			response, err := client.SendAPDUWithOptions(context.Background(), UIMAPDURequest{
				Channel:        opened.Channel,
				Command:        []byte{0x00, 0xA4},
				ProcedureBytes: &procedureBytes,
			})
			if err != nil {
				t.Fatalf("SendAPDUWithOptions() error = %v", err)
			}
			if !bytes.Equal(response, []byte{0x90, 0x00}) {
				t.Fatalf("SendAPDUWithOptions() = % X", response)
			}
		})
	}
}

func TestOpenLogicalChannelSelectResponse(t *testing.T) {
	longResponse := bytes.Repeat([]byte{0x62}, maxLogicalChannelSelectResponseLength)
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		want    []byte
		wantErr string
	}{
		{
			name: "long select response",
			tlvs: tlv.TLVs{tlv.Bytes(0x13, encodeLengthPrefixed(longResponse))},
			want: longResponse,
		},
		{
			name: "short response takes precedence",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x12, []byte{2, 0x90, 0x00}),
				tlv.Bytes(0x13, encodeLengthPrefixed([]byte{0x62, 0x00})),
			},
			want: []byte{0x90, 0x00},
		},
		{
			name:    "long select response too large",
			tlvs:    tlv.TLVs{tlv.Bytes(0x13, encodeLengthPrefixed(bytes.Repeat([]byte{0x62}, maxLogicalChannelSelectResponseLength+1)))},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseTLVs := append(tlv.TLVs{tlv.Bytes(0x10, []byte{3})}, tt.tlvs...)
			transport := &fakeTransport{t: t, calls: []transportCall{{
				resp: successResponse(MessageOpenLogicalChannel, responseTLVs...),
			}}}
			client := &Client{transport: transport, slot: 1, clientIDs: map[ServiceType]uint8{ServiceUIM: 7}}

			got, err := client.OpenLogicalChannelWithConfig(context.Background(), UIMOpenLogicalChannelConfig{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("OpenLogicalChannelWithConfig() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenLogicalChannelWithConfig() error = %v", err)
			}
			if !bytes.Equal(got.SelectResponse, tt.want) {
				t.Fatalf("SelectResponse = % X, want % X", got.SelectResponse, tt.want)
			}
		})
	}
}

func TestSendAPDURejectsLongResponse(t *testing.T) {
	reader := &Client{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					resp: errorResponse(
						MessageSendAPDU,
						QMIErrorInsufficientResources,
						tlv.Bytes(0x11, []byte{0x04, 0x00, 0x01, 0x00, 0x00, 0x00}),
					),
				},
			},
		},
		slot:      1,
		clientIDs: map[ServiceType]uint8{ServiceUIM: 7},
	}

	_, err := reader.SendAPDU(context.Background(), 1, []byte{0x00, 0xA4, 0x00, 0x00})
	if err == nil || !strings.Contains(err.Error(), "long response is not supported") {
		t.Fatalf("SendAPDU() error = %v, want long response error", err)
	}
}

func TestOpenLogicalChannelRequestMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		req     OpenLogicalChannelRequest
		want    []byte
		wantErr string
	}{
		{
			name: "aid",
			req:  OpenLogicalChannelRequest{AID: []byte{0xA0, 0x00}},
			want: []byte{0x02, 0xA0, 0x00},
		},
		{
			name:    "aid too long",
			req:     OpenLogicalChannelRequest{AID: bytes.Repeat([]byte{0x01}, maxLogicalChannelAIDLength+1)},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestOpenLogicalChannelRequestUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    OpenLogicalChannelRequest
		wantErr string
	}{
		{
			name: "aid",
			data: []byte{0x02, 0xA0, 0x00},
			want: OpenLogicalChannelRequest{AID: []byte{0xA0, 0x00}},
		},
		{
			name:    "missing length",
			data:    nil,
			wantErr: "truncated",
		},
		{
			name:    "truncated aid",
			data:    []byte{0x02, 0xA0},
			wantErr: "value length",
		},
		{
			name:    "aid too long",
			data:    append([]byte{maxLogicalChannelAIDLength + 1}, bytes.Repeat([]byte{0xA0}, maxLogicalChannelAIDLength+1)...),
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OpenLogicalChannelRequest
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got.AID, tt.want.AID) {
				t.Fatalf("UnmarshalBinary() = % X, want % X", got.AID, tt.want.AID)
			}
			if len(tt.data) > 1 && len(got.AID) > 0 {
				tt.data[1] ^= 0xff
				if bytes.Equal(got.AID, tt.data[1:]) {
					t.Fatal("UnmarshalBinary() kept AID backing array")
				}
			}
		})
	}
}

func TestLogicalChannelResponsesUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{
			name: "open channel response",
			run: func() error {
				var resp OpenLogicalChannelResponse
				if err := resp.UnmarshalBinary([]byte{0x03}); err != nil {
					return err
				}
				if resp.Channel != 3 {
					t.Fatalf("Channel = %d, want 3", resp.Channel)
				}
				return nil
			},
		},
		{
			name: "open channel missing",
			run: func() error {
				var resp OpenLogicalChannelResponse
				return resp.UnmarshalBinary(nil)
			},
			wantErr: "want 1",
		},
		{
			name: "open channel trailing data",
			run: func() error {
				var resp OpenLogicalChannelResponse
				return resp.UnmarshalBinary([]byte{3, 4})
			},
			wantErr: "want 1",
		},
		{
			name: "close channel response",
			run: func() error {
				var resp CloseLogicalChannelResponse
				return resp.UnmarshalBinary(nil)
			},
		},
		{
			name: "close channel unexpected data",
			run: func() error {
				var resp CloseLogicalChannelResponse
				return resp.UnmarshalBinary([]byte{0x00})
			},
			wantErr: "want 0",
		},
		{
			name: "send APDU response",
			run: func() error {
				var resp SendAPDUResponse
				if err := resp.UnmarshalBinary(encodeLengthPrefixed([]byte{0x90, 0x00})); err != nil {
					return err
				}
				if !bytes.Equal(resp.Response, []byte{0x90, 0x00}) {
					t.Fatalf("Response = % X, want 90 00", resp.Response)
				}
				return nil
			},
		},
		{
			name: "send APDU response truncated",
			run: func() error {
				var resp SendAPDUResponse
				return resp.UnmarshalBinary([]byte{0x02, 0x00, 0x90})
			},
			wantErr: "value length",
		},
		{
			name: "send APDU response trailing data",
			run: func() error {
				var resp SendAPDUResponse
				return resp.UnmarshalBinary([]byte{1, 0, 0x90, 0x00})
			},
			wantErr: "value length",
		},
		{
			name: "send APDU response too long",
			run: func() error {
				var resp SendAPDUResponse
				return resp.UnmarshalBinary(encodeLengthPrefixed(bytes.Repeat([]byte{0x90}, maxAPDUDataLength+1)))
			},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
		})
	}
}

func TestCloseLogicalChannelRequestBinary(t *testing.T) {
	got, err := (CloseLogicalChannelRequest{Channel: 3}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0x03}) {
		t.Fatalf("MarshalBinary() = % X, want 03", got)
	}

	var req CloseLogicalChannelRequest
	if err := req.UnmarshalBinary(got); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if req.Channel != 3 {
		t.Fatalf("Channel = %d, want 3", req.Channel)
	}
}

func TestSendAPDURequestBinary(t *testing.T) {
	tests := []struct {
		name    string
		req     SendAPDURequest
		want    []byte
		wantErr string
	}{
		{
			name: "command",
			req:  SendAPDURequest{Command: []byte{0x00, 0xA4}},
			want: []byte{0x02, 0x00, 0x00, 0xA4},
		},
		{
			name: "maximum command",
			req:  SendAPDURequest{Command: bytes.Repeat([]byte{0x00}, maxAPDUDataLength)},
			want: encodeLengthPrefixed(bytes.Repeat([]byte{0x00}, maxAPDUDataLength)),
		},
		{
			name:    "command too long",
			req:     SendAPDURequest{Command: bytes.Repeat([]byte{0x00}, maxAPDUDataLength+1)},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}

			var req SendAPDURequest
			if err := req.UnmarshalBinary(got); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if !bytes.Equal(req.Command, tt.req.Command) {
				t.Fatalf("UnmarshalBinary() = % X, want % X", req.Command, tt.req.Command)
			}
		})
	}
}

func TestSendAPDURequestUnmarshalRejectsLongCommand(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "above normal APDU block limit",
			data: encodeLengthPrefixed(bytes.Repeat([]byte{0x00}, maxAPDUDataLength+1)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req SendAPDURequest
			if err := req.UnmarshalBinary(tt.data); err == nil || !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("UnmarshalBinary() error = %v, want APDU length error", err)
			}
		})
	}
}
