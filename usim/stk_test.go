package usim

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	stkpkg "github.com/damonto/uicc-go/usim/stk"
	"github.com/damonto/uicc-go/usim/tlv"
)

func TestSTKHandle(t *testing.T) {
	raw := proactive(t,
		tlv.NewComprehension(0x01, []byte{0x01, byte(stkpkg.CommandDisplayText), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stkpkg.DeviceUICC), byte(stkpkg.DeviceDisplay)}),
		tlv.NewComprehension(0x0D, []byte{0x04, 'H', 'i'}),
	)
	var cmd stkpkg.ProactiveCommand
	if err := cmd.UnmarshalBinary(raw); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	var malformedCmd stkpkg.ProactiveCommand
	err := malformedCmd.UnmarshalBinary(proactive(t,
		tlv.NewComprehension(0x01, []byte{0x02, byte(stkpkg.CommandDisplayText), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stkpkg.DeviceUICC), byte(stkpkg.DeviceDisplay)}),
	))
	if err != nil {
		t.Fatalf("UnmarshalBinary() malformed error = %v", err)
	}
	var bipStatusCmd stkpkg.ProactiveCommand
	err = bipStatusCmd.UnmarshalBinary(proactive(t,
		tlv.NewComprehension(0x01, []byte{0x03, byte(stkpkg.CommandGetChannelStatus), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stkpkg.DeviceUICC), byte(stkpkg.DeviceTerminal)}),
	))
	if err != nil {
		t.Fatalf("UnmarshalBinary() BIP status error = %v", err)
	}
	var setupEventCmd stkpkg.ProactiveCommand
	err = setupEventCmd.UnmarshalBinary(proactive(t,
		tlv.NewComprehension(0x01, []byte{0x04, byte(stkpkg.CommandSetupEventList), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stkpkg.DeviceUICC), byte(stkpkg.DeviceTerminal)}),
		tlv.NewComprehension(0x19, []byte{byte(stkpkg.EventDataAvailable), byte(stkpkg.EventChannelStatus)}),
	))
	if err != nil {
		t.Fatalf("UnmarshalBinary() setup event list error = %v", err)
	}

	tests := []struct {
		name      string
		callbacks STKCallbacks
		command   stkpkg.Command
		want      stkpkg.ResultCode
		wantErr   bool
	}{
		{
			name: "callback",
			callbacks: STKCallbacks{DisplayText: func(context.Context, stkpkg.DisplayTextCommand) (stkpkg.TerminalResponse, error) {
				return stkpkg.OK(), nil
			}},
			want: stkpkg.ResultCommandPerformed,
		},
		{
			name:    "built-in BIP",
			command: bipStatusCmd.Command,
			want:    stkpkg.ResultCommandPerformed,
		},
		{
			name:    "built-in setup event list",
			command: setupEventCmd.Command,
			want:    stkpkg.ResultCommandPerformed,
		},
		{
			name: "missing callback",
			want: stkpkg.ResultCommandBeyondTerminalCapabilities,
		},
		{
			name: "callback error sends unable",
			callbacks: STKCallbacks{DisplayText: func(context.Context, stkpkg.DisplayTextCommand) (stkpkg.TerminalResponse, error) {
				return stkpkg.TerminalResponse{}, errors.New("screen busy")
			}},
			want:    stkpkg.ResultTerminalUnableToProcess,
			wantErr: true,
		},
		{
			name: "malformed command sends data not understood",
			callbacks: STKCallbacks{DisplayText: func(context.Context, stkpkg.DisplayTextCommand) (stkpkg.TerminalResponse, error) {
				return stkpkg.OK(), nil
			}},
			command: malformedCmd.Command,
			want:    stkpkg.ResultCommandDataNotUnderstood,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeSTKTransport{}
			stk, err := newSTK(transport)
			if err != nil {
				t.Fatalf("newSTK() error = %v", err)
			}
			command := tt.command
			if command == nil {
				command = cmd.Command
			}
			err = stk.Handle(context.Background(), STKSession{Command: command}, tt.callbacks)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Handle() error = nil, want non-nil")
				}
			} else if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(transport.responses) != 1 {
				t.Fatalf("responses = %d, want 1", len(transport.responses))
			}
			result := transport.responses[0][len(transport.responses[0])-1]
			if stkpkg.ResultCode(result) != tt.want {
				t.Fatalf("result = 0x%02X, want 0x%02X", result, tt.want)
			}
		})
	}
}

func TestProfileFromCallbacksDoesNotAdvertiseBuiltInBIP(t *testing.T) {
	profile := ProfileFromCallbacks(STKCallbacks{})
	commands := profile.ProactiveCommandTypes()

	tests := []struct {
		name string
		cmd  stkpkg.CommandType
	}{
		{"open channel", stkpkg.CommandOpenChannel},
		{"close channel", stkpkg.CommandCloseChannel},
		{"receive data", stkpkg.CommandReceiveData},
		{"send data", stkpkg.CommandSendData},
		{"get channel status", stkpkg.CommandGetChannelStatus},
		{"setup event list", stkpkg.CommandSetupEventList},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if slices.Contains(commands, tt.cmd) {
				t.Fatalf("ProactiveCommandTypes() = %v, did not want %v", commands, tt.cmd)
			}
		})
	}
}

func TestFullSTKProfileIncludesInteractiveCommands(t *testing.T) {
	commands := FullSTKProfile().ProactiveCommandTypes()

	tests := []struct {
		name string
		cmd  stkpkg.CommandType
	}{
		{"setup menu", stkpkg.CommandSetupMenu},
		{"select item", stkpkg.CommandSelectItem},
		{"display text", stkpkg.CommandDisplayText},
		{"get input", stkpkg.CommandGetInput},
		{"get inkey", stkpkg.CommandGetInkey},
		{"send ussd", stkpkg.CommandSendUSSD},
		{"setup call", stkpkg.CommandSetupCall},
		{"open channel", stkpkg.CommandOpenChannel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Contains(commands, tt.cmd) {
				t.Fatalf("FullSTKProfile().ProactiveCommandTypes() = %v, want %v", commands, tt.cmd)
			}
		})
	}
}

func TestSTKUsesEnvelopeForBuiltInBIPEvents(t *testing.T) {
	transport := &fakeSTKTransport{}
	stk, err := newSTK(transport)
	if err != nil {
		t.Fatalf("newSTK() error = %v", err)
	}

	status := stkpkg.NewChannelStatus(2, true, stkpkg.ChannelStatusNoInfo)
	if err := stk.bip.SendDataAvailable(context.Background(), status, []byte("pong"), 4, 1); err != nil {
		t.Fatalf("SendDataAvailable() error = %v", err)
	}
	if len(transport.envelopes) != 1 || transport.envelopes[0][0] != stkpkg.TagEventDownload {
		t.Fatalf("DataAvailable envelope = % X, want event download", transport.envelopes)
	}

	if err := stk.bip.SendChannelStatus(context.Background(), status); err != nil {
		t.Fatalf("SendChannelStatus() error = %v", err)
	}
	if len(transport.envelopes) != 2 || transport.envelopes[1][0] != stkpkg.TagEventDownload {
		t.Fatalf("ChannelStatus envelope = % X, want event download", transport.envelopes)
	}
}

func TestSTKRunCancelsTransportContextOnHandleError(t *testing.T) {
	var cmd stkpkg.ProactiveCommand
	err := cmd.UnmarshalBinary(proactive(t,
		tlv.NewComprehension(0x01, []byte{0x01, byte(stkpkg.CommandDisplayText), 0x00}),
		tlv.NewComprehension(0x02, []byte{byte(stkpkg.DeviceUICC), byte(stkpkg.DeviceDisplay)}),
		tlv.NewComprehension(0x0D, []byte{0x04, 'H', 'i'}),
	))
	if err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}

	transport := &cancelAwareSTKTransport{
		command:  STKSession{Command: cmd.Command},
		canceled: make(chan struct{}),
	}
	stk, err := newSTK(transport)
	if err != nil {
		t.Fatalf("newSTK() error = %v", err)
	}

	callbacks := STKCallbacks{
		DisplayText: func(context.Context, stkpkg.DisplayTextCommand) (stkpkg.TerminalResponse, error) {
			return stkpkg.TerminalResponse{}, errors.New("screen busy")
		},
	}

	if err := stk.Run(context.Background(), callbacks); err == nil {
		t.Fatal("Run() error = nil, want callback error")
	}
	select {
	case <-transport.canceled:
	case <-time.After(time.Second):
		t.Fatal("transport command context was not canceled")
	}
}

func TestQCOMEventConfirmation(t *testing.T) {
	yes := true
	no := false
	tests := []struct {
		name     string
		command  stkpkg.Command
		response stkpkg.TerminalResponse
		wantOK   bool
		wantUser *bool
		wantIcon *bool
	}{
		{
			name:     "open channel success confirms user",
			command:  stkpkg.OpenChannelCommand{},
			response: stkpkg.OK(),
			wantOK:   true,
			wantUser: &yes,
			wantIcon: &no,
		},
		{
			name:     "open channel failure rejects user",
			command:  stkpkg.OpenChannelCommand{},
			response: stkpkg.Result(stkpkg.ResultBearerIndependentProtocolError),
			wantOK:   true,
			wantUser: &no,
			wantIcon: &no,
		},
		{
			name:     "close channel confirms icon only",
			command:  stkpkg.CloseChannelCommand{},
			response: stkpkg.OK(),
			wantOK:   true,
			wantIcon: &no,
		},
		{
			name: "refresh confirms icon only",
			command: stkpkg.SimpleCommand{CommandFrame: stkpkg.CommandFrame{
				Details: stkpkg.CommandDetails{Type: stkpkg.CommandRefresh},
			}},
			response: stkpkg.OK(),
			wantOK:   true,
			wantIcon: &no,
		},
		{
			name:     "display text uses terminal response",
			command:  stkpkg.DisplayTextCommand{},
			response: stkpkg.OK(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := qcomEventConfirmation(tt.command, tt.response)
			if ok != tt.wantOK {
				t.Fatalf("qcomEventConfirmation() ok = %v, want %v", ok, tt.wantOK)
			}
			assertOptionalBool(t, "UserConfirmed", got.UserConfirmed, tt.wantUser)
			assertOptionalBool(t, "IconDisplayed", got.IconDisplayed, tt.wantIcon)
		})
	}
}

type fakeSTKTransport struct {
	responses [][]byte
	envelopes [][]byte
}

func (t *fakeSTKTransport) Commands(context.Context, stkpkg.Profile) (<-chan STKSession, error) {
	ch := make(chan STKSession)
	close(ch)
	return ch, nil
}

func (t *fakeSTKTransport) Respond(_ context.Context, session STKSession, response stkpkg.TerminalResponse) error {
	data, err := response.MarshalFor(session.Command)
	if err != nil {
		return err
	}
	t.responses = append(t.responses, data)
	return nil
}

func (t *fakeSTKTransport) Envelope(_ context.Context, envelope []byte) (stkpkg.EnvelopeResponse, error) {
	t.envelopes = append(t.envelopes, slices.Clone(envelope))
	return stkpkg.EnvelopeResponse{SW1: 0x90, SW2: 0x00}, nil
}

type cancelAwareSTKTransport struct {
	command   STKSession
	canceled  chan struct{}
	responses [][]byte
}

func (t *cancelAwareSTKTransport) Commands(ctx context.Context, _ stkpkg.Profile) (<-chan STKSession, error) {
	ch := make(chan STKSession, 1)
	ch <- t.command
	go func() {
		<-ctx.Done()
		close(t.canceled)
	}()
	return ch, nil
}

func (t *cancelAwareSTKTransport) Respond(_ context.Context, session STKSession, response stkpkg.TerminalResponse) error {
	data, err := response.MarshalFor(session.Command)
	if err != nil {
		return err
	}
	t.responses = append(t.responses, data)
	return nil
}

func (t *cancelAwareSTKTransport) Envelope(context.Context, []byte) (stkpkg.EnvelopeResponse, error) {
	return stkpkg.EnvelopeResponse{SW1: 0x90, SW2: 0x00}, nil
}

func assertOptionalBool(t *testing.T, name string, got, want *bool) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s = %v, want %v", name, *got, *want)
	}
}

func proactive(t *testing.T, tlvs ...tlv.Item) []byte {
	t.Helper()
	body, err := tlv.Items(tlvs).MarshalBinary()
	if err != nil {
		t.Fatalf("tlv.Items.MarshalBinary() error = %v", err)
	}
	raw, err := tlv.WrapBER(stkpkg.TagProactiveCommand, body)
	if err != nil {
		t.Fatalf("tlv.WrapBER() error = %v", err)
	}
	return raw
}
