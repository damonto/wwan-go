package stk

import (
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/damonto/uicc-go/usim/tlv"
)

type Command interface {
	MarshalBinary() ([]byte, error)
	CommandDetails() CommandDetails
	DeviceIdentities() DeviceIdentities
	RawTLVs() tlv.Items
	RawBytes() []byte
}

type CommandFrame struct {
	Details CommandDetails
	Devices DeviceIdentities
	TLVs    tlv.Items
	Raw     []byte
}

func (c CommandFrame) CommandDetails() CommandDetails     { return c.Details }
func (c CommandFrame) DeviceIdentities() DeviceIdentities { return c.Devices }
func (c CommandFrame) RawTLVs() tlv.Items                 { return tlv.CloneItems(c.TLVs) }
func (c CommandFrame) RawBytes() []byte                   { return slices.Clone(c.Raw) }

func (c CommandFrame) MarshalBinary() ([]byte, error) {
	if len(c.Raw) > 0 {
		return slices.Clone(c.Raw), nil
	}
	data, err := c.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("building proactive command frame: %w", err)
	}
	return tlv.WrapBER(TagProactiveCommand, data)
}

func (c *CommandFrame) UnmarshalBinary(data []byte) error {
	frame, err := unmarshalCommandFrame(data)
	*c = frame
	return err
}

func unmarshalCommandFrame(data []byte) (CommandFrame, error) {
	tag, body, err := tlv.UnwrapBER(data)
	if err != nil {
		return CommandFrame{}, fmt.Errorf("parsing proactive command: %w", err)
	}
	if tag != TagProactiveCommand {
		return CommandFrame{}, fmt.Errorf("parsing proactive command: BER tag 0x%02X, want 0x%02X", tag, TagProactiveCommand)
	}

	items, tlvErr := commandTLVs(body)
	item, ok := items.Find(tlvCommandDetails)
	if !ok {
		if tlvErr != nil {
			return CommandFrame{}, fmt.Errorf("parsing proactive command TLVs: %w", tlvErr)
		}
		return CommandFrame{}, errors.New("parsing proactive command: command details missing")
	}
	if err := requireLen("command details", item.Value, 3); err != nil {
		return CommandFrame{}, err
	}

	devices := DeviceIdentities{Source: DeviceUICC, Destination: DeviceTerminal}
	if item, ok := items.Find(tlvDeviceIDs); ok && len(item.Value) >= 2 {
		devices = DeviceIdentities{Source: DeviceID(item.Value[0]), Destination: DeviceID(item.Value[1])}
	}

	frame := CommandFrame{
		Details: CommandDetails{
			Number:    item.Value[0],
			Type:      CommandType(item.Value[1]),
			Qualifier: item.Value[2],
		},
		Devices: devices,
		TLVs:    tlv.CloneItems(items),
		Raw:     slices.Clone(data),
	}
	if tlvErr != nil {
		return frame, commandFrameTLVError{Err: tlvErr}
	}
	return frame, nil
}

type commandFrameTLVError struct {
	Err error
}

func (e commandFrameTLVError) Error() string {
	return fmt.Sprintf("parsing proactive command TLVs: %v", e.Err)
}

func (e commandFrameTLVError) Unwrap() error {
	return e.Err
}

func commandTLVs(data []byte) (tlv.Items, error) {
	items := make(tlv.Items, 0, len(data)/2)
	for len(data) > 0 {
		item, consumed, err := tlv.Consume(data)
		if err != nil {
			return items, err
		}
		items = append(items, item)
		data = data[consumed:]
	}
	return items, nil
}

func (c CommandFrame) WriteTo(w io.Writer) (int64, error) {
	data, err := c.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func (c *CommandFrame) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return int64(len(data)), err
	}
	return int64(len(data)), c.UnmarshalBinary(data)
}

func (c CommandFrame) Valid() error {
	if err := requireTLV(c.TLVs, tlvDeviceIDs, "device identities", 2); err != nil {
		return err
	}

	switch c.Details.Type {
	case CommandDisplayText, CommandGetInkey:
		return requireTLV(c.TLVs, tlvTextString, "text string", 1)
	case CommandGetInput:
		if err := requireTLV(c.TLVs, tlvTextString, "text string", 1); err != nil {
			return err
		}
		return requireTLV(c.TLVs, tlvResponseLength, "response length", 2)
	case CommandSetupMenu:
		items := c.TLVs.All(tlvItem)
		for _, item := range items {
			if len(item.Value) == 0 {
				return errors.New("parsing proactive command: item is truncated")
			}
		}
	case CommandSelectItem:
		items := c.TLVs.All(tlvItem)
		if len(items) == 0 {
			return errors.New("parsing proactive command: item missing")
		}
		for _, item := range items {
			if len(item.Value) == 0 {
				return errors.New("parsing proactive command: item is truncated")
			}
		}
	case CommandSetupEventList:
		return requireTLV(c.TLVs, tlvEventList, "event list", 0)
	case CommandOpenChannel:
		return requireTLV(c.TLVs, tlvBufferSize, "buffer size", 2)
	case CommandReceiveData:
		return requireTLV(c.TLVs, tlvChannelDataLen, "channel data length", 1)
	case CommandSendData:
		return requireTLV(c.TLVs, tlvChannelData, "channel data", 0)
	}
	return nil
}

func (c CommandFrame) Command() (Command, error) {
	if err := c.Valid(); err != nil {
		return MalformedCommand{CommandFrame: c, Err: err}, nil
	}

	switch c.Details.Type {
	case CommandDisplayText:
		var cmd DisplayTextCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandGetInkey:
		var cmd GetInkeyCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandGetInput:
		var cmd GetInputCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandSetupMenu:
		var cmd SetupMenuCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandSelectItem:
		var cmd SelectItemCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandSetupEventList:
		var cmd SetupEventListCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandOpenChannel:
		var cmd OpenChannelCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandCloseChannel:
		var cmd CloseChannelCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandReceiveData:
		var cmd ReceiveDataCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandSendData:
		var cmd SendDataCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	case CommandGetChannelStatus:
		var cmd GetChannelStatusCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	default:
		var cmd SimpleCommand
		if err := cmd.UnmarshalFrame(c); err != nil {
			return nil, err
		}
		return cmd, nil
	}
}

type DisplayTextCommand struct {
	CommandFrame
	Text              Text
	HighPriority      bool
	UserClear         bool
	ImmediateResponse bool
	Duration          *Duration
	Icon              *Icon
}

type GetInkeyCommand struct {
	CommandFrame
	Text                   Text
	Alphabet               bool
	YesNo                  bool
	UCS2                   bool
	Packed                 bool
	ImmediateDigitResponse bool
	HelpAvailable          bool
	Duration               *Duration
}

type GetInputCommand struct {
	CommandFrame
	Text          Text
	DefaultText   *Text
	Length        ResponseLength
	Alphabet      bool
	UCS2          bool
	Packed        bool
	HideInput     bool
	HelpAvailable bool
}

type MenuCommand struct {
	CommandFrame
	Title         *Text
	Items         []Item
	DefaultItem   byte
	HelpAvailable bool
}

type SelectItemCommand struct{ MenuCommand }
type SetupMenuCommand struct{ MenuCommand }

type SetupEventListCommand struct {
	CommandFrame
	Events []Event
}

type SimpleCommand struct {
	CommandFrame
	Text          *Text
	Alpha         *Text
	Address       []byte
	Subaddress    []byte
	SMSTPDU       []byte
	URL           string
	Language      string
	Duration      *Duration
	EventList     []Event
	Bearer        []byte
	NetworkAccess []byte
	CAPDU         []byte
	ATCommand     []byte
}

type GenericCommand struct {
	CommandFrame
}

// MalformedCommand keeps enough command metadata to send a terminal response
// when the proactive command data itself cannot be understood.
type MalformedCommand struct {
	CommandFrame
	Err error
}

type ProactiveCommand struct {
	Command Command
}

func (c ProactiveCommand) MarshalBinary() ([]byte, error) {
	if c.Command == nil {
		return nil, errors.New("building proactive command: command is nil")
	}
	return c.Command.MarshalBinary()
}

func (c *ProactiveCommand) UnmarshalBinary(data []byte) error {
	frame, err := unmarshalCommandFrame(data)
	if err != nil {
		var tlvErr commandFrameTLVError
		if errors.As(err, &tlvErr) {
			c.Command = MalformedCommand{CommandFrame: frame, Err: err}
			return nil
		}
		return err
	}
	command, err := frame.Command()
	if err != nil {
		return err
	}
	c.Command = command
	return nil
}

func (c ProactiveCommand) WriteTo(w io.Writer) (int64, error) {
	data, err := c.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func (c *ProactiveCommand) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return int64(len(data)), err
	}
	return int64(len(data)), c.UnmarshalBinary(data)
}

func requireTLV(tlvs tlv.Items, tag byte, name string, minLen int) error {
	item, ok := tlvs.Find(tag)
	if !ok {
		return fmt.Errorf("parsing proactive command: %s missing", name)
	}
	if len(item.Value) < minLen {
		return fmt.Errorf("parsing proactive command: %s length %d, want at least %d", name, len(item.Value), minLen)
	}
	return nil
}

func (cmd *DisplayTextCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = DisplayTextCommand{
		CommandFrame: frame,
		HighPriority: frame.Details.Qualifier&0x01 != 0,
		UserClear:    frame.Details.Qualifier&0x80 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvTextString); ok {
		_ = cmd.Text.UnmarshalBinary(item.Value)
	}
	if _, ok := frame.TLVs.Find(tlvImmediateResp); ok {
		cmd.ImmediateResponse = true
	}
	cmd.Duration = toDuration(frame.TLVs)
	cmd.Icon = toIcon(frame.TLVs)
	return nil
}

func (cmd *GetInkeyCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = GetInkeyCommand{
		CommandFrame:           frame,
		Alphabet:               frame.Details.Qualifier&0x01 != 0,
		UCS2:                   frame.Details.Qualifier&0x02 != 0,
		YesNo:                  frame.Details.Qualifier&0x04 != 0,
		ImmediateDigitResponse: frame.Details.Qualifier&0x08 != 0,
		HelpAvailable:          frame.Details.Qualifier&0x80 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvTextString); ok {
		_ = cmd.Text.UnmarshalBinary(item.Value)
	}
	cmd.Duration = toDuration(frame.TLVs)
	return nil
}

func (cmd *GetInputCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = GetInputCommand{
		CommandFrame:  frame,
		Alphabet:      frame.Details.Qualifier&0x01 != 0,
		UCS2:          frame.Details.Qualifier&0x02 != 0,
		HideInput:     frame.Details.Qualifier&0x04 != 0,
		Packed:        frame.Details.Qualifier&0x08 != 0,
		HelpAvailable: frame.Details.Qualifier&0x80 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvTextString); ok {
		_ = cmd.Text.UnmarshalBinary(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvDefaultText); ok {
		var text Text
		_ = text.UnmarshalBinary(item.Value)
		cmd.DefaultText = &text
	}
	if item, ok := frame.TLVs.Find(tlvResponseLength); ok && len(item.Value) >= 2 {
		cmd.Length = ResponseLength{Min: item.Value[0], Max: item.Value[1]}
	}
	return nil
}

func (cmd *MenuCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = MenuCommand{
		CommandFrame:  frame,
		HelpAvailable: frame.Details.Qualifier&0x80 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var title Text
		_ = title.UnmarshalText(item.Value)
		cmd.Title = &title
	}
	if item, ok := frame.TLVs.Find(tlvDefaultText); ok {
		var title Text
		_ = title.UnmarshalBinary(item.Value)
		cmd.Title = &title
	}
	if item, ok := frame.TLVs.Find(tlvItemID); ok && len(item.Value) > 0 {
		cmd.DefaultItem = item.Value[0]
	}
	for _, item := range frame.TLVs.All(tlvItem) {
		if len(item.Value) == 0 {
			continue
		}
		var text Text
		_ = text.UnmarshalText(item.Value[1:])
		cmd.Items = append(cmd.Items, Item{
			Identifier: item.Value[0],
			Text:       text,
		})
	}
	return nil
}

func (cmd *SetupMenuCommand) UnmarshalFrame(frame CommandFrame) error {
	var menu MenuCommand
	if err := menu.UnmarshalFrame(frame); err != nil {
		return err
	}
	*cmd = SetupMenuCommand{MenuCommand: menu}
	return nil
}

func (cmd *SelectItemCommand) UnmarshalFrame(frame CommandFrame) error {
	var menu MenuCommand
	if err := menu.UnmarshalFrame(frame); err != nil {
		return err
	}
	*cmd = SelectItemCommand{MenuCommand: menu}
	return nil
}

func (cmd *SetupEventListCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = SetupEventListCommand{CommandFrame: frame}
	if item, ok := frame.TLVs.Find(tlvEventList); ok {
		cmd.Events = make([]Event, 0, len(item.Value))
		for _, event := range item.Value {
			cmd.Events = append(cmd.Events, Event(event))
		}
	}
	return nil
}

func (cmd *OpenChannelCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = OpenChannelCommand{
		CommandFrame:          frame,
		Immediate:             frame.Details.Qualifier&0x01 != 0,
		AutomaticReconnection: frame.Details.Qualifier&0x02 != 0,
		Background:            frame.Details.Qualifier&0x04 != 0,
		DNSServerRequest:      frame.Details.Qualifier&0x08 != 0,
		LaunchParameters:      frame.Details.Qualifier&0x01 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	cmd.Icon = toIcon(frame.TLVs)
	if item, ok := frame.TLVs.Find(tlvBearerDesc); ok {
		var desc BearerDescription
		if err := desc.UnmarshalBinary(item.Value); err == nil {
			cmd.BearerDescription = &desc
		}
	}
	if item, ok := frame.TLVs.Find(tlvBufferSize); ok && len(item.Value) >= 2 {
		cmd.BufferSize = uint16(item.Value[0])<<8 | uint16(item.Value[1])
	}
	if item, ok := frame.TLVs.Find(tlvNetworkAccess); ok {
		cmd.NetworkAccessName = string(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvTransportLevel); ok {
		var level TransportLevel
		if err := level.UnmarshalBinary(item.Value); err == nil {
			cmd.TransportLevel = &level
		}
	}
	addresses := frame.TLVs.All(tlvOtherAddress)
	cmd.OtherAddresses = make([]OtherAddress, 0, len(addresses))
	for _, item := range addresses {
		var address OtherAddress
		_ = address.UnmarshalBinary(item.Value)
		cmd.OtherAddresses = append(cmd.OtherAddresses, address)
	}
	if len(cmd.OtherAddresses) > 0 {
		if cmd.TransportLevel == nil {
			cmd.LocalAddress = &cmd.OtherAddresses[0]
		} else {
			cmd.DestinationAddress = &cmd.OtherAddresses[len(cmd.OtherAddresses)-1]
			if len(cmd.OtherAddresses) > 1 {
				cmd.LocalAddress = &cmd.OtherAddresses[0]
			}
		}
	}
	if item, ok := frame.TLVs.Find(tlvRemoteEntity); ok {
		var address RemoteEntityAddress
		if err := address.UnmarshalBinary(item.Value); err == nil {
			cmd.RemoteEntityAddress = &address
		}
	}
	texts := frame.TLVs.All(tlvTextString)
	if len(texts) > 0 {
		var login Text
		_ = login.UnmarshalBinary(texts[0].Value)
		cmd.Login = &login
	}
	if len(texts) > 1 {
		var password Text
		_ = password.UnmarshalBinary(texts[1].Value)
		cmd.Password = &password
	}
	return nil
}

func (cmd *CloseChannelCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = CloseChannelCommand{
		CommandFrame:        frame,
		ChannelID:           channelIdentifier(frame.Devices.Destination),
		ReuseNetworkAccess:  frame.Details.Qualifier&0x01 != 0,
		TCPListenAfterClose: frame.Details.Qualifier&0x01 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	cmd.Icon = toIcon(frame.TLVs)
	return nil
}

func (cmd *ReceiveDataCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = ReceiveDataCommand{
		CommandFrame: frame,
		ChannelID:    channelIdentifier(frame.Devices.Destination),
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	cmd.Icon = toIcon(frame.TLVs)
	if item, ok := frame.TLVs.Find(tlvChannelDataLen); ok && len(item.Value) > 0 {
		cmd.Length = item.Value[0]
	}
	return nil
}

func (cmd *SendDataCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = SendDataCommand{
		CommandFrame:    frame,
		ChannelID:       channelIdentifier(frame.Devices.Destination),
		SendImmediately: frame.Details.Qualifier&0x01 != 0,
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	cmd.Icon = toIcon(frame.TLVs)
	if item, ok := frame.TLVs.Find(tlvChannelData); ok {
		cmd.Data = slices.Clone(item.Value)
	}
	return nil
}

func (cmd *GetChannelStatusCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = GetChannelStatusCommand{CommandFrame: frame}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	cmd.Icon = toIcon(frame.TLVs)
	return nil
}

func channelIdentifier(device DeviceID) byte {
	if device >= DeviceChannel1 && device <= DeviceChannel7 {
		return byte(device - DeviceChannel)
	}
	return 0
}

func (cmd *SimpleCommand) UnmarshalFrame(frame CommandFrame) error {
	*cmd = SimpleCommand{CommandFrame: frame}
	if item, ok := frame.TLVs.Find(tlvTextString); ok {
		var text Text
		_ = text.UnmarshalBinary(item.Value)
		cmd.Text = &text
	}
	if item, ok := frame.TLVs.Find(tlvAlphaID); ok {
		var alpha Text
		_ = alpha.UnmarshalText(item.Value)
		cmd.Alpha = &alpha
	}
	if item, ok := frame.TLVs.Find(tlvAddress); ok {
		cmd.Address = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvSubaddress); ok {
		cmd.Subaddress = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvSMSTPDU); ok {
		cmd.SMSTPDU = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvURL); ok {
		cmd.URL = string(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvLanguage); ok {
		cmd.Language = string(item.Value)
	}
	cmd.Duration = toDuration(frame.TLVs)
	if item, ok := frame.TLVs.Find(tlvEventList); ok {
		for _, event := range item.Value {
			cmd.EventList = append(cmd.EventList, Event(event))
		}
	}
	if item, ok := frame.TLVs.Find(tlvBearer); ok {
		cmd.Bearer = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvNetworkAccess); ok {
		cmd.NetworkAccess = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvCAPDU); ok {
		cmd.CAPDU = slices.Clone(item.Value)
	}
	if item, ok := frame.TLVs.Find(tlvATCommand); ok {
		cmd.ATCommand = slices.Clone(item.Value)
	}
	return nil
}

func toDuration(tlvs tlv.Items) *Duration {
	item, ok := tlvs.Find(tlvDuration)
	if !ok || len(item.Value) < 2 {
		return nil
	}
	return &Duration{Unit: item.Value[0], Interval: item.Value[1]}
}

func toIcon(tlvs tlv.Items) *Icon {
	item, ok := tlvs.Find(tlvIconID)
	if !ok || len(item.Value) < 2 {
		return nil
	}
	return &Icon{SelfExplanatory: item.Value[0] != 0, Record: item.Value[1]}
}
