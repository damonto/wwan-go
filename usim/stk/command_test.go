package stk

import (
	"bytes"
	"encoding"
	"io"
	"testing"

	"github.com/damonto/uicc-go/usim/tlv"
)

func TestProactiveCommandUnmarshalBinary(t *testing.T) {
	var _ encoding.BinaryMarshaler = CommandFrame{}
	var _ encoding.BinaryUnmarshaler = (*CommandFrame)(nil)
	var _ io.WriterTo = CommandFrame{}
	var _ io.ReaderFrom = (*CommandFrame)(nil)
	var _ encoding.BinaryMarshaler = ProactiveCommand{}
	var _ encoding.BinaryUnmarshaler = (*ProactiveCommand)(nil)
	var _ io.WriterTo = ProactiveCommand{}
	var _ io.ReaderFrom = (*ProactiveCommand)(nil)

	tests := []struct {
		name string
		raw  []byte
		want func(t *testing.T, cmd Command)
	}{
		{
			name: "display text",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x01, byte(CommandDisplayText), 0x80}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, 'H', 'i'}),
				tlv.NewComprehension(tlvImmediateResp, nil),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(DisplayTextCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want DisplayTextCommand", cmd)
				}
				if got.Text.String != "Hi" {
					t.Fatalf("Text = %q, want Hi", got.Text.String)
				}
				if !got.UserClear || !got.ImmediateResponse {
					t.Fatalf("flags = userClear:%t immediate:%t, want true/true", got.UserClear, got.ImmediateResponse)
				}
			},
		},
		{
			name: "setup menu",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x02, byte(CommandSetupMenu), 0x80}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvAlphaID, []byte("main")),
				tlv.NewComprehension(tlvItem, []byte{0x01, 'o', 'n', 'e'}),
				tlv.NewComprehension(tlvItem, []byte{0x02, 't', 'w', 'o'}),
				tlv.NewComprehension(tlvItemID, []byte{0x02}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(SetupMenuCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want SetupMenuCommand", cmd)
				}
				if len(got.Items) != 2 || got.Items[1].Identifier != 2 || got.Items[1].Text.String != "two" {
					t.Fatalf("items = %+v, want second item two", got.Items)
				}
				if got.DefaultItem != 2 || !got.HelpAvailable {
					t.Fatalf("default/help = %d/%t, want 2/true", got.DefaultItem, got.HelpAvailable)
				}
			},
		},
		{
			name: "unknown command",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x03, 0x7f, 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(UnknownCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want UnknownCommand", cmd)
				}
				if got.Details.Type != 0x7f {
					t.Fatalf("command type = 0x%02X, want 0x7F", got.Details.Type)
				}
			},
		},
		{
			name: "open channel",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x05, byte(CommandOpenChannel), 0x0f}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvBearerDesc, []byte{byte(BearerTypeDefault)}),
				tlv.NewComprehension(tlvBufferSize, []byte{0x04, 0x00}),
				tlv.NewComprehension(tlvNetworkAccess, []byte("apn")),
				tlv.NewComprehension(tlvTransportLevel, []byte{byte(TransportTCPClientRemote), 0x00, 0x50}),
				tlv.NewComprehension(tlvOtherAddress, []byte{byte(AddressTypeIPv4), 0x01, 0x02, 0x03, 0x04}),
				tlv.NewComprehension(tlvRemoteEntity, []byte{0x00, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, 'u', 's', 'e', 'r'}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(OpenChannelCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want OpenChannelCommand", cmd)
				}
				if got.BufferSize != 1024 || got.NetworkAccessName != "apn" {
					t.Fatalf("open channel buffer/APN = %d/%q, want 1024/apn", got.BufferSize, got.NetworkAccessName)
				}
				if got.BearerDescription == nil || got.BearerDescription.Type != BearerTypeDefault {
					t.Fatalf("bearer = %+v, want default", got.BearerDescription)
				}
				if got.TransportLevel == nil || got.TransportLevel.Protocol != TransportTCPClientRemote || got.TransportLevel.Port != 80 {
					t.Fatalf("transport = %+v, want TCP remote port 80", got.TransportLevel)
				}
				if got.LocalAddress != nil {
					t.Fatalf("local address = %+v, want nil", got.LocalAddress)
				}
				if got.DestinationAddress == nil || got.DestinationAddress.Type != AddressTypeIPv4 || !bytes.Equal(got.DestinationAddress.Address, []byte{1, 2, 3, 4}) {
					t.Fatalf("destination address = %+v, want IPv4 1.2.3.4", got.DestinationAddress)
				}
				if got.RemoteEntityAddress == nil || got.RemoteEntityAddress.Coding != 0 || !bytes.Equal(got.RemoteEntityAddress.Address, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}) {
					t.Fatalf("remote entity = %+v, want IEEE address", got.RemoteEntityAddress)
				}
				if got.Login == nil || got.Login.String != "user" {
					t.Fatalf("login = %+v, want user", got.Login)
				}
				if !got.Immediate || !got.AutomaticReconnection || !got.Background || !got.DNSServerRequest {
					t.Fatalf("open channel flags = %+v, want qualifier bits set", got)
				}
			},
		},
		{
			name: "open channel local and destination address",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x09, byte(CommandOpenChannel), 0x01}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvBearerDesc, []byte{byte(BearerTypeDefault)}),
				tlv.NewComprehension(tlvBufferSize, []byte{0x04, 0x00}),
				tlv.NewComprehension(tlvTransportLevel, []byte{byte(TransportUDPClientRemote), 0x13, 0x88}),
				tlv.NewComprehension(tlvOtherAddress, nil),
				tlv.NewComprehension(tlvOtherAddress, []byte{byte(AddressTypeIPv4), 192, 0, 2, 1}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(OpenChannelCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want OpenChannelCommand", cmd)
				}
				if got.LocalAddress == nil || len(got.LocalAddress.Address) != 0 {
					t.Fatalf("local address = %+v, want dynamic local address", got.LocalAddress)
				}
				if got.DestinationAddress == nil || !bytes.Equal(got.DestinationAddress.Address, []byte{192, 0, 2, 1}) {
					t.Fatalf("destination address = %+v, want 192.0.2.1", got.DestinationAddress)
				}
			},
		},
		{
			name: "get inkey qualifier",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0A, byte(CommandGetInkey), 0x8F}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, '?'}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(GetInkeyCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want GetInkeyCommand", cmd)
				}
				if !got.Alphabet || !got.UCS2 || !got.YesNo || !got.ImmediateDigitResponse || !got.HelpAvailable || got.Packed {
					t.Fatalf("get inkey flags = %+v, want qualifier bits 1/2/3/4/8", got)
				}
			},
		},
		{
			name: "get input qualifier",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0B, byte(CommandGetInput), 0x8F}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, '?'}),
				tlv.NewComprehension(tlvResponseLength, []byte{0x01, 0x08}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(GetInputCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want GetInputCommand", cmd)
				}
				if !got.Alphabet || !got.UCS2 || !got.HideInput || !got.Packed || !got.HelpAvailable {
					t.Fatalf("get input flags = %+v, want qualifier bits 1/2/3/4/8", got)
				}
			},
		},
		{
			name: "receive data",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0C, byte(CommandReceiveData), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceChannel3)}),
				tlv.NewComprehension(tlvChannelDataLen, []byte{0x40}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(ReceiveDataCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want ReceiveDataCommand", cmd)
				}
				if got.ChannelID != 3 || got.Length != 0x40 {
					t.Fatalf("receive data channel/length = %d/%d, want 3/64", got.ChannelID, got.Length)
				}
			},
		},
		{
			name: "send data",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0D, byte(CommandSendData), 0x01}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceChannel2)}),
				tlv.NewComprehension(tlvChannelData, []byte{0x01, 0x02, 0x03}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(SendDataCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want SendDataCommand", cmd)
				}
				if got.ChannelID != 2 || !got.SendImmediately || !bytes.Equal(got.Data, []byte{1, 2, 3}) {
					t.Fatalf("send data = %+v, want channel 2 immediate data", got)
				}
			},
		},
		{
			name: "get channel status",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0E, byte(CommandGetChannelStatus), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
			),
			want: func(t *testing.T, cmd Command) {
				if _, ok := cmd.(GetChannelStatusCommand); !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want GetChannelStatusCommand", cmd)
				}
			},
		},
		{
			name: "missing required object becomes malformed command",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x04, byte(CommandDisplayText), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(MalformedCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want MalformedCommand", cmd)
				}
				if got.Details.Type != CommandDisplayText || got.Err == nil {
					t.Fatalf("malformed command = %+v, want display text with error", got)
				}
				if got.ResultCode() != ResultRequiredValuesMissing {
					t.Fatalf("ResultCode() = 0x%02X, want 0x%02X", got.ResultCode(), ResultRequiredValuesMissing)
				}
			},
		},
		{
			name: "truncated TLV after command details becomes malformed command",
			raw: []byte{
				0xD0, 0x0D,
				0x81, 0x03, 0x09, byte(CommandDisplayText), 0x00,
				0x82, 0x02, byte(DeviceUICC), byte(DeviceDisplay),
				0x8D, 0x03, 0x04, 'H',
			},
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(MalformedCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want MalformedCommand", cmd)
				}
				if got.Details.Number != 0x09 || got.Err == nil {
					t.Fatalf("malformed command = %+v, want command number 9 with error", got)
				}
				if got.ResultCode() != ResultCommandDataNotUnderstood {
					t.Fatalf("ResultCode() = 0x%02X, want 0x%02X", got.ResultCode(), ResultCommandDataNotUnderstood)
				}
			},
		},
		{
			name: "unexpected comprehension TLV becomes malformed command",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0F, byte(CommandDisplayText), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, 'H', 'i'}),
				tlv.NewComprehension(0x7e, []byte{0x01}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(MalformedCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want MalformedCommand", cmd)
				}
				if got.ResultCode() != ResultCommandDataNotUnderstood {
					t.Fatalf("ResultCode() = 0x%02X, want 0x%02X", got.ResultCode(), ResultCommandDataNotUnderstood)
				}
			},
		},
		{
			name: "missing required TLV wins over unexpected comprehension TLV",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x11, byte(CommandDisplayText), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
				tlv.NewComprehension(0x7e, []byte{0x01}),
			),
			want: func(t *testing.T, cmd Command) {
				got, ok := cmd.(MalformedCommand)
				if !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want MalformedCommand", cmd)
				}
				if got.ResultCode() != ResultRequiredValuesMissing {
					t.Fatalf("ResultCode() = 0x%02X, want 0x%02X", got.ResultCode(), ResultRequiredValuesMissing)
				}
			},
		},
		{
			name: "unexpected optional TLV marks partial comprehension",
			raw: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x10, byte(CommandDisplayText), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, 'H', 'i'}),
				tlv.New(0x7e, []byte{0x01}),
			),
			want: func(t *testing.T, cmd Command) {
				if _, ok := cmd.(DisplayTextCommand); !ok {
					t.Fatalf("UnmarshalBinary() command type = %T, want DisplayTextCommand", cmd)
				}
				if !cmd.PartialComprehension() {
					t.Fatal("PartialComprehension() = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ProactiveCommand
			if err := got.UnmarshalBinary(tt.raw); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			tt.want(t, got.Command)

			encoded, err := got.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(encoded, tt.raw) {
				t.Fatalf("MarshalBinary() = % X, want % X", encoded, tt.raw)
			}
		})
	}
}

func TestTerminalResponseMarshalFor(t *testing.T) {
	tests := []struct {
		name     string
		command  []byte
		response TerminalResponse
		want     []byte
	}{
		{
			name: "ok",
			command: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x01, byte(CommandDisplayText), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceDisplay)}),
				tlv.NewComprehension(tlvTextString, []byte{0x04, 'H', 'i'}),
			),
			response: OK(),
			want: []byte{
				0x81, 0x03, 0x01, 0x21, 0x00,
				0x82, 0x02, 0x82, 0x81,
				0x83, 0x01, 0x00,
			},
		},
		{
			name: "open channel",
			command: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x09, byte(CommandOpenChannel), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceTerminal)}),
				tlv.NewComprehension(tlvBearerDesc, []byte{byte(BearerTypeDefault)}),
				tlv.NewComprehension(tlvBufferSize, []byte{0x04, 0x00}),
			),
			response: TerminalResponse{
				Result:            ResultCommandPerformed,
				ChannelStatuses:   []ChannelStatus{NewChannelStatus(1, true, ChannelStatusNoInfo)},
				BearerDescription: &BearerDescription{Type: BearerTypeDefault},
				BufferSize:        uint16Ptr(1024),
				OtherAddresses:    []OtherAddress{{Type: AddressTypeIPv4, Address: []byte{192, 0, 2, 1}}},
			},
			want: []byte{
				0x81, 0x03, 0x09, 0x40, 0x00,
				0x82, 0x02, 0x82, 0x81,
				0x83, 0x01, 0x00,
				0xB8, 0x02, 0x81, 0x00,
				0xB5, 0x01, 0x03,
				0xB9, 0x02, 0x04, 0x00,
				0xBE, 0x05, 0x21, 0xC0, 0x00, 0x02, 0x01,
			},
		},
		{
			name: "receive data empty",
			command: proactive(t,
				tlv.NewComprehension(tlvCommandDetails, []byte{0x0a, byte(CommandReceiveData), 0x00}),
				tlv.NewComprehension(tlvDeviceIDs, []byte{byte(DeviceUICC), byte(DeviceChannel1)}),
				tlv.NewComprehension(tlvChannelDataLen, []byte{0x00}),
			),
			response: ReceiveDataOK(nil, 0),
			want: []byte{
				0x81, 0x03, 0x0A, 0x42, 0x00,
				0x82, 0x02, 0x82, 0x81,
				0x83, 0x01, 0x00,
				0xB6, 0x00,
				0xB7, 0x01, 0x00,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd ProactiveCommand
			err := cmd.UnmarshalBinary(tt.command)
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}

			got, err := tt.response.MarshalFor(cmd.Command)
			if err != nil {
				t.Fatalf("MarshalFor() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalFor() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestEnvelopeMarshalBinary(t *testing.T) {
	var _ encoding.BinaryMarshaler = Envelope{}
	var _ io.WriterTo = Envelope{}

	tests := []struct {
		name string
		env  encoding.BinaryMarshaler
		want []byte
	}{
		{
			name: "menu selection",
			env:  MenuSelection(0x02, true),
			want: []byte{0xD3, 0x09, 0x82, 0x02, 0x01, 0x81, 0x90, 0x01, 0x02, 0x95, 0x00},
		},
		{
			name: "event download",
			env:  EventDownload(EventIdleScreenAvailable, DeviceDisplay, DeviceUICC),
			want: []byte{0xD6, 0x07, 0x99, 0x01, 0x05, 0x82, 0x02, 0x02, 0x81},
		},
		{
			name: "sms pp download",
			env:  SMSPPDownload{ServiceCenterAddress: "+12345", TPDU: []byte{0x00, 0x01}},
			want: []byte{0xD1, 0x0E, 0x82, 0x02, 0x83, 0x81, 0x86, 0x04, 0x91, 0x21, 0x43, 0xF5, 0x8B, 0x02, 0x00, 0x01},
		},
		{
			name: "data available",
			env:  DataAvailable(NewChannelStatus(2, true, ChannelStatusNoInfo), 0x20),
			want: []byte{0xD6, 0x0E, 0x99, 0x01, 0x09, 0x82, 0x02, 0x82, 0x81, 0xB8, 0x02, 0x82, 0x00, 0xB7, 0x01, 0x20},
		},
		{
			name: "channel status event",
			env: ChannelStatusEvent(
				NewChannelStatus(3, false, ChannelStatusLinkDropped),
				&BearerDescription{Type: BearerTypeDefault},
				&OtherAddress{Type: AddressTypeIPv4, Address: []byte{10, 0, 0, 1}},
			),
			want: []byte{0xD6, 0x15, 0x99, 0x01, 0x0A, 0x82, 0x02, 0x82, 0x81, 0xB8, 0x02, 0x03, 0x05, 0xB5, 0x01, 0x03, 0xBE, 0x05, 0x21, 0x0A, 0x00, 0x00, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.env.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func proactive(t *testing.T, tlvs ...tlv.Item) []byte {
	t.Helper()
	body, err := tlv.Items(tlvs).MarshalBinary()
	if err != nil {
		t.Fatalf("tlv.Items.MarshalBinary() error = %v", err)
	}
	raw, err := tlv.WrapBER(TagProactiveCommand, body)
	if err != nil {
		t.Fatalf("tlv.WrapBER() error = %v", err)
	}
	return raw
}

func uint16Ptr(v uint16) *uint16 {
	return &v
}
