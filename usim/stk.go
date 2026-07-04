package usim

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/uicc-go/usim/stk"
)

type STKSession struct {
	Ref     uint32
	Command stk.Command
}

type STKCallback[T stk.Command] func(context.Context, T) (stk.TerminalResponse, error)

type STKCallbacks struct {
	DisplayText       STKCallback[stk.DisplayTextCommand]
	GetInkey          STKCallback[stk.GetInkeyCommand]
	GetInput          STKCallback[stk.GetInputCommand]
	SetupMenu         STKCallback[stk.SetupMenuCommand]
	SelectItem        STKCallback[stk.SelectItemCommand]
	SetupEventList    STKCallback[stk.SetupEventListCommand]
	MoreTime          STKCallback[stk.SimpleCommand]
	Refresh           STKCallback[stk.SimpleCommand]
	PlayTone          STKCallback[stk.SimpleCommand]
	SendSMS           STKCallback[stk.SimpleCommand]
	SendSS            STKCallback[stk.SimpleCommand]
	SendUSSD          STKCallback[stk.SimpleCommand]
	SendDTMF          STKCallback[stk.SimpleCommand]
	SetupCall         STKCallback[stk.SimpleCommand]
	LaunchBrowser     STKCallback[stk.SimpleCommand]
	ProvideLocalInfo  STKCallback[stk.SimpleCommand]
	SetupIdleModeText STKCallback[stk.SimpleCommand]
	LanguageNotify    STKCallback[stk.SimpleCommand]
	Activate          STKCallback[stk.SimpleCommand]
	Generic           STKCallback[stk.GenericCommand]
}

type stkTransport interface {
	Commands(ctx context.Context, profile stk.Profile) (<-chan STKSession, error)
	Respond(ctx context.Context, session STKSession, response stk.TerminalResponse) error
	Envelope(ctx context.Context, envelope []byte) (stk.EnvelopeResponse, error)
}

type STK struct {
	transport stkTransport
	bip       *stk.BIP
}

func newSTK(transport stkTransport) (*STK, error) {
	if transport == nil {
		return nil, errors.New("creating USIM STK: transport is nil")
	}
	s := &STK{transport: transport}
	s.bip = &stk.BIP{
		SendEnvelope: func(ctx context.Context, envelope stk.Envelope) error {
			_, err := s.SendEnvelope(ctx, envelope)
			return err
		},
	}
	s.bip.SendDataAvailable = func(ctx context.Context, status stk.ChannelStatus, _ []byte, available byte, _ uint16) error {
		return s.bip.SendEnvelope(ctx, stk.DataAvailable(status, available))
	}
	s.bip.SendChannelStatus = func(ctx context.Context, status stk.ChannelStatus) error {
		return s.bip.SendEnvelope(ctx, stk.ChannelStatusEvent(status, nil, nil))
	}
	return s, nil
}

func (u *Card) STK() (*STK, error) {
	if u == nil || u.tx == nil {
		return nil, errors.New("creating USIM STK: card is nil")
	}
	transport, ok := u.tx.(stkTransport)
	if !ok {
		return nil, errors.New("creating USIM STK: reader does not support STK")
	}
	return newSTK(transport)
}

func (s *STK) Run(ctx context.Context, callbacks STKCallbacks) error {
	profile := ProfileFromCallbacks(callbacks)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		_ = s.Close()
	}()

	commands, err := s.transport.Commands(runCtx, profile)
	if err != nil {
		return fmt.Errorf("starting STK command loop: %w", err)
	}
	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case session, ok := <-commands:
			if !ok {
				return nil
			}
			if err := s.Handle(runCtx, session, callbacks); err != nil {
				return err
			}
		}
	}
}

func (s *STK) Handle(ctx context.Context, session STKSession, callbacks STKCallbacks) error {
	if session.Command == nil {
		return errors.New("handling STK command: command is nil")
	}
	response, err := s.dispatch(ctx, callbacks, session.Command)
	if err != nil {
		fallback := stk.Result(stk.ResultTerminalUnableToProcess)
		if sendErr := s.sendResponse(ctx, session, fallback); sendErr != nil {
			return errors.Join(err, sendErr)
		}
		return err
	}
	return s.sendResponse(ctx, session, response)
}

func (s *STK) SendEnvelope(ctx context.Context, envelope stk.Envelope) (stk.EnvelopeResponse, error) {
	data, err := envelope.MarshalBinary()
	if err != nil {
		return stk.EnvelopeResponse{}, err
	}
	return s.transport.Envelope(ctx, data)
}

func (s *STK) SendRawEnvelope(ctx context.Context, envelope []byte) (stk.EnvelopeResponse, error) {
	return s.transport.Envelope(ctx, envelope)
}

func (s *STK) Close() error {
	return s.bip.Close()
}

func (s *STK) sendResponse(ctx context.Context, session STKSession, response stk.TerminalResponse) error {
	return s.transport.Respond(ctx, session, response)
}

func (s *STK) dispatch(ctx context.Context, callbacks STKCallbacks, cmd stk.Command) (stk.TerminalResponse, error) {
	switch command := cmd.(type) {
	case stk.MalformedCommand:
		return stk.Result(stk.ResultCommandDataNotUnderstood), nil
	case stk.DisplayTextCommand:
		if callbacks.DisplayText != nil {
			return callbacks.DisplayText(ctx, command)
		}
	case stk.GetInkeyCommand:
		if callbacks.GetInkey != nil {
			return callbacks.GetInkey(ctx, command)
		}
	case stk.GetInputCommand:
		if callbacks.GetInput != nil {
			return callbacks.GetInput(ctx, command)
		}
	case stk.SetupMenuCommand:
		if callbacks.SetupMenu != nil {
			return callbacks.SetupMenu(ctx, command)
		}
	case stk.SelectItemCommand:
		if callbacks.SelectItem != nil {
			return callbacks.SelectItem(ctx, command)
		}
	case stk.SetupEventListCommand:
		s.bip.SetEvents(command.Events)
		if callbacks.SetupEventList != nil {
			return callbacks.SetupEventList(ctx, command)
		}
		return stk.OK(), nil
	case stk.OpenChannelCommand:
		return s.bip.OpenChannel(ctx, command)
	case stk.CloseChannelCommand:
		return s.bip.CloseChannel(ctx, command)
	case stk.SendDataCommand:
		return s.bip.SendData(ctx, command)
	case stk.ReceiveDataCommand:
		return s.bip.ReceiveData(ctx, command)
	case stk.GetChannelStatusCommand:
		return s.bip.GetChannelStatus(ctx, command)
	case stk.SimpleCommand:
		return s.dispatchSimple(ctx, callbacks, command)
	case stk.GenericCommand:
		if callbacks.Generic != nil {
			return callbacks.Generic(ctx, command)
		}
	}

	if callbacks.Generic != nil {
		return callbacks.Generic(ctx, stk.GenericCommand{CommandFrame: frameOf(cmd)})
	}
	return stk.Result(stk.ResultCommandBeyondTerminalCapabilities), nil
}

func (s *STK) dispatchSimple(ctx context.Context, callbacks STKCallbacks, cmd stk.SimpleCommand) (stk.TerminalResponse, error) {
	switch cmd.Details.Type {
	case stk.CommandMoreTime:
		if callbacks.MoreTime != nil {
			return callbacks.MoreTime(ctx, cmd)
		}
		return stk.OK(), nil
	case stk.CommandRefresh:
		if callbacks.Refresh != nil {
			return callbacks.Refresh(ctx, cmd)
		}
	case stk.CommandPlayTone:
		if callbacks.PlayTone != nil {
			return callbacks.PlayTone(ctx, cmd)
		}
	case stk.CommandSendSMS:
		if callbacks.SendSMS != nil {
			return callbacks.SendSMS(ctx, cmd)
		}
	case stk.CommandSendSS:
		if callbacks.SendSS != nil {
			return callbacks.SendSS(ctx, cmd)
		}
	case stk.CommandSendUSSD:
		if callbacks.SendUSSD != nil {
			return callbacks.SendUSSD(ctx, cmd)
		}
	case stk.CommandSendDTMF:
		if callbacks.SendDTMF != nil {
			return callbacks.SendDTMF(ctx, cmd)
		}
	case stk.CommandSetupCall:
		if callbacks.SetupCall != nil {
			return callbacks.SetupCall(ctx, cmd)
		}
	case stk.CommandLaunchBrowser:
		if callbacks.LaunchBrowser != nil {
			return callbacks.LaunchBrowser(ctx, cmd)
		}
	case stk.CommandProvideLocalInfo:
		if callbacks.ProvideLocalInfo != nil {
			return callbacks.ProvideLocalInfo(ctx, cmd)
		}
	case stk.CommandSetupIdleModeText:
		if callbacks.SetupIdleModeText != nil {
			return callbacks.SetupIdleModeText(ctx, cmd)
		}
	case stk.CommandLanguageNotify:
		if callbacks.LanguageNotify != nil {
			return callbacks.LanguageNotify(ctx, cmd)
		}
	case stk.CommandActivate:
		if callbacks.Activate != nil {
			return callbacks.Activate(ctx, cmd)
		}
	}
	if callbacks.Generic != nil {
		return callbacks.Generic(ctx, stk.GenericCommand{CommandFrame: cmd.CommandFrame})
	}
	return stk.Result(stk.ResultCommandBeyondTerminalCapabilities), nil
}

func FullSTKProfile() stk.Profile {
	return stk.NewProfile(fullSTKCapabilities()...)
}

func ProfileFromCallbacks(callbacks STKCallbacks) stk.Profile {
	capabilities := []stk.Capability{stk.CapabilityProfileDownload}
	if callbacks.DisplayText != nil {
		capabilities = append(capabilities, stk.CapabilityDisplayText)
	}
	if callbacks.GetInkey != nil {
		capabilities = append(capabilities, stk.CapabilityGetInkey)
	}
	if callbacks.GetInput != nil {
		capabilities = append(capabilities, stk.CapabilityGetInput)
	}
	if callbacks.SetupMenu != nil {
		capabilities = append(capabilities, stk.CapabilitySetupMenu, stk.CapabilityMenuSelection)
	}
	if callbacks.SelectItem != nil {
		capabilities = append(capabilities, stk.CapabilitySelectItem)
	}
	if callbacks.SetupEventList != nil {
		capabilities = append(capabilities, stk.CapabilitySetupEventList)
	}
	if callbacks.MoreTime != nil {
		capabilities = append(capabilities, stk.CapabilityMoreTime)
	}
	if callbacks.Refresh != nil {
		capabilities = append(capabilities, stk.CapabilityRefresh)
	}
	if callbacks.PlayTone != nil {
		capabilities = append(capabilities, stk.CapabilityPlayTone)
	}
	if callbacks.SendSMS != nil {
		capabilities = append(capabilities, stk.CapabilitySendSMS)
	}
	if callbacks.SendSS != nil {
		capabilities = append(capabilities, stk.CapabilitySendSS)
	}
	if callbacks.SendUSSD != nil {
		capabilities = append(capabilities, stk.CapabilitySendUSSD)
	}
	if callbacks.SendDTMF != nil {
		capabilities = append(capabilities, stk.CapabilitySendDTMF)
	}
	if callbacks.SetupCall != nil {
		capabilities = append(capabilities, stk.CapabilitySetupCall)
	}
	if callbacks.LaunchBrowser != nil {
		capabilities = append(capabilities, stk.CapabilityLaunchBrowser)
	}
	if callbacks.ProvideLocalInfo != nil {
		capabilities = append(capabilities, stk.CapabilityProvideLocalInfo)
	}
	if callbacks.SetupIdleModeText != nil {
		capabilities = append(capabilities, stk.CapabilitySetupIdleModeText)
	}
	if callbacks.LanguageNotify != nil {
		capabilities = append(capabilities, stk.CapabilityLanguageNotification)
	}
	if callbacks.Activate != nil {
		capabilities = append(capabilities, stk.CapabilityActivate)
	}
	if callbacks.Generic != nil {
		capabilities = append(capabilities, fullSTKCapabilities()...)
	}
	return stk.NewProfile(capabilities...)
}

func fullSTKCapabilities() []stk.Capability {
	return []stk.Capability{
		stk.CapabilityProfileDownload, stk.CapabilityBIP, stk.CapabilitySetupEventList,
		stk.CapabilityDisplayText, stk.CapabilityGetInkey, stk.CapabilityGetInput, stk.CapabilitySetupMenu, stk.CapabilitySelectItem,
		stk.CapabilityMoreTime, stk.CapabilityPlayTone, stk.CapabilityPollInterval, stk.CapabilityPollingOff, stk.CapabilityRefresh,
		stk.CapabilitySendSMS, stk.CapabilitySendSS, stk.CapabilitySendUSSD, stk.CapabilitySendDTMF, stk.CapabilitySetupCall,
		stk.CapabilityLaunchBrowser, stk.CapabilityProvideLocalInfo, stk.CapabilitySetupIdleModeText,
		stk.CapabilityLanguageNotification, stk.CapabilityActivate,
	}
}

func frameOf(cmd stk.Command) stk.CommandFrame {
	return stk.CommandFrame{
		Details: cmd.CommandDetails(),
		Devices: cmd.DeviceIdentities(),
		TLVs:    cmd.RawTLVs(),
		Raw:     cmd.RawBytes(),
	}
}
