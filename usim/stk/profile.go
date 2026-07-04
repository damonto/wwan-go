package stk

import "slices"

type Capability byte

const (
	CapabilityProfileDownload Capability = iota
	CapabilitySMSPPDownload
	CapabilityMenuSelection
	CapabilityDisplayText
	CapabilityGetInkey
	CapabilityGetInput
	CapabilityMoreTime
	CapabilityPlayTone
	CapabilityPollInterval
	CapabilityPollingOff
	CapabilityRefresh
	CapabilitySelectItem
	CapabilitySendSMS
	CapabilitySendSS
	CapabilitySendUSSD
	CapabilitySetupCall
	CapabilitySetupMenu
	CapabilityProvideLocalInfo
	CapabilitySetupEventList
	CapabilitySetupIdleModeText
	CapabilityLanguageNotification
	CapabilityLaunchBrowser
	CapabilitySendDTMF
	CapabilityBIP
	CapabilityActivate
)

type Profile struct {
	Data         []byte
	Events       uint32
	FullFunction uint32
	Commands     []CommandType
}

func NewProfile(capabilities ...Capability) Profile {
	profile := Profile{Data: make([]byte, 20)}
	for _, capability := range capabilities {
		profile.Enable(capability)
	}
	return profile.trim()
}

func (p *Profile) Enable(capability Capability) {
	if len(p.Data) < 20 {
		p.Data = append(p.Data, make([]byte, 20-len(p.Data))...)
	}

	switch capability {
	case CapabilityProfileDownload:
		p.set(1, 1)
	case CapabilitySMSPPDownload:
		p.set(1, 2)
	case CapabilityMenuSelection:
		p.set(1, 4)
	case CapabilityDisplayText:
		p.set(3, 1)
		p.addCommands(CommandDisplayText)
		p.Events |= qmiEventDisplayText
	case CapabilityGetInkey:
		p.set(3, 2)
		p.addCommands(CommandGetInkey)
		p.Events |= qmiEventGetInkey
	case CapabilityGetInput:
		p.set(3, 3)
		p.addCommands(CommandGetInput)
		p.Events |= qmiEventGetInput
	case CapabilityMoreTime:
		p.set(3, 4)
		p.addCommands(CommandMoreTime)
	case CapabilityPlayTone:
		p.set(3, 5)
		p.addCommands(CommandPlayTone)
		p.Events |= qmiEventPlayTone
	case CapabilityPollInterval:
		p.set(3, 6)
		p.addCommands(CommandPollInterval)
	case CapabilityPollingOff:
		p.set(3, 7)
		p.addCommands(CommandPollingOff)
	case CapabilityRefresh:
		p.set(3, 8)
		p.addCommands(CommandRefresh)
		p.Events |= qmiEventRefreshAlpha
	case CapabilitySelectItem:
		p.set(4, 1)
		p.addCommands(CommandSelectItem)
		p.Events |= qmiEventSelectItem
	case CapabilitySendSMS:
		p.set(4, 2)
		p.addCommands(CommandSendSMS)
		p.Events |= qmiEventSendSMS
		p.FullFunction |= 1 << 0
	case CapabilitySendSS:
		p.set(4, 3)
		p.addCommands(CommandSendSS)
		p.Events |= qmiEventSendSS
		p.FullFunction |= 1 << 3
	case CapabilitySendUSSD:
		p.set(4, 4)
		p.addCommands(CommandSendUSSD)
		p.Events |= qmiEventSendUSSD
		p.FullFunction |= 1 << 4
	case CapabilitySetupCall:
		p.set(4, 5)
		p.addCommands(CommandSetupCall)
		p.Events |= qmiEventSetupCall
		p.FullFunction |= 1 << 1
	case CapabilitySetupMenu:
		p.set(4, 6)
		p.addCommands(CommandSetupMenu)
		p.Events |= qmiEventSetupMenu
	case CapabilityProvideLocalInfo:
		p.set(4, 7)
		p.addCommands(CommandProvideLocalInfo)
		p.Events |= qmiEventProvideLocalInfo
	case CapabilitySetupEventList:
		p.set(4, 8)
		p.addCommands(CommandSetupEventList)
		p.Events |= qmiEventSetupEventUserActivity | qmiEventSetupEventIdleScreen | qmiEventSetupEventLanguage | qmiEventSetupEventBrowser | qmiEventSetupEventHCI
	case CapabilitySetupIdleModeText:
		p.set(5, 1)
		p.addCommands(CommandSetupIdleModeText)
		p.Events |= qmiEventIdleModeText
	case CapabilityLanguageNotification:
		p.set(5, 4)
		p.addCommands(CommandLanguageNotify)
		p.Events |= qmiEventLanguageNotification
	case CapabilityLaunchBrowser:
		p.set(7, 4)
		p.addCommands(CommandLaunchBrowser)
		p.Events |= qmiEventLaunchBrowser
	case CapabilitySendDTMF:
		p.set(7, 8)
		p.addCommands(CommandSendDTMF)
		p.Events |= qmiEventSendDTMF
		p.FullFunction |= 1 << 2
	case CapabilityBIP:
		p.set(6, 3)
		p.set(6, 4)
		p.set(12, 1)
		p.set(12, 2)
		p.set(12, 3)
		p.set(12, 4)
		p.set(12, 5)
		p.Data[12] = 0x07
		p.addCommands(CommandOpenChannel, CommandCloseChannel, CommandReceiveData, CommandSendData, CommandGetChannelStatus)
		p.Events |= qmiEventBIP
	case CapabilityActivate:
		p.set(17, 1)
		p.addCommands(CommandActivate)
		p.Events |= qmiEventActivate
	}
}

func (p Profile) Bytes() []byte {
	return slices.Clone(p.Data)
}

func (p Profile) QMIEventMask() uint32 {
	return p.Events
}

func (p Profile) QMIFullFunctionMask() uint32 {
	return p.FullFunction
}

func (p Profile) ProactiveCommandTypes() []CommandType {
	if len(p.Commands) == 0 {
		return p.commandTypesFromProfile()
	}
	return slices.Clone(p.Commands)
}

func (p *Profile) set(byteNumber, bitNumber int) {
	if byteNumber <= 0 || bitNumber <= 0 || bitNumber > 8 {
		return
	}
	for len(p.Data) < byteNumber {
		p.Data = append(p.Data, 0)
	}
	p.Data[byteNumber-1] |= 1 << (bitNumber - 1)
}

func (p *Profile) addCommands(commands ...CommandType) {
	for _, command := range commands {
		p.addCommand(command)
	}
}

func (p *Profile) addCommand(command CommandType) {
	if slices.Contains(p.Commands, command) {
		return
	}
	p.Commands = append(p.Commands, command)
}

func (p Profile) commandTypesFromProfile() []CommandType {
	var commands []CommandType
	add := func(command CommandType) {
		if !slices.Contains(commands, command) {
			commands = append(commands, command)
		}
	}

	if p.enabled(3, 1) {
		add(CommandDisplayText)
	}
	if p.enabled(3, 2) {
		add(CommandGetInkey)
	}
	if p.enabled(3, 3) {
		add(CommandGetInput)
	}
	if p.enabled(3, 4) {
		add(CommandMoreTime)
	}
	if p.enabled(3, 5) {
		add(CommandPlayTone)
	}
	if p.enabled(3, 6) {
		add(CommandPollInterval)
	}
	if p.enabled(3, 7) {
		add(CommandPollingOff)
	}
	if p.enabled(3, 8) {
		add(CommandRefresh)
	}
	if p.enabled(4, 1) {
		add(CommandSelectItem)
	}
	if p.enabled(4, 2) {
		add(CommandSendSMS)
	}
	if p.enabled(4, 3) {
		add(CommandSendSS)
	}
	if p.enabled(4, 4) {
		add(CommandSendUSSD)
	}
	if p.enabled(4, 5) {
		add(CommandSetupCall)
	}
	if p.enabled(4, 6) {
		add(CommandSetupMenu)
	}
	if p.enabled(4, 7) {
		add(CommandProvideLocalInfo)
	}
	if p.enabled(4, 8) {
		add(CommandSetupEventList)
	}
	if p.enabled(5, 1) {
		add(CommandSetupIdleModeText)
	}
	if p.enabled(5, 4) {
		add(CommandLanguageNotify)
	}
	if p.enabled(7, 4) {
		add(CommandLaunchBrowser)
	}
	if p.enabled(7, 8) {
		add(CommandSendDTMF)
	}
	if p.enabled(12, 1) {
		add(CommandOpenChannel)
	}
	if p.enabled(12, 2) {
		add(CommandCloseChannel)
	}
	if p.enabled(12, 3) {
		add(CommandReceiveData)
	}
	if p.enabled(12, 4) {
		add(CommandSendData)
	}
	if p.enabled(12, 5) {
		add(CommandGetChannelStatus)
	}
	if p.enabled(17, 1) {
		add(CommandActivate)
	}
	return commands
}

func (p Profile) enabled(byteNumber, bitNumber int) bool {
	if byteNumber <= 0 || bitNumber <= 0 || bitNumber > 8 || len(p.Data) < byteNumber {
		return false
	}
	return p.Data[byteNumber-1]&(1<<(bitNumber-1)) != 0
}

func (p Profile) trim() Profile {
	end := len(p.Data)
	for end > 0 && p.Data[end-1] == 0 {
		end--
	}
	if end == 0 {
		end = 1
	}
	p.Data = slices.Clone(p.Data[:end])
	p.Commands = slices.Clone(p.Commands)
	return p
}

const (
	qmiEventDisplayText            uint32 = 1 << 0
	qmiEventGetInkey               uint32 = 1 << 1
	qmiEventGetInput               uint32 = 1 << 2
	qmiEventSetupMenu              uint32 = 1 << 3
	qmiEventSelectItem             uint32 = 1 << 4
	qmiEventSendSMS                uint32 = 1 << 5
	qmiEventSetupEventUserActivity uint32 = 1 << 6
	qmiEventSetupEventIdleScreen   uint32 = 1 << 7
	qmiEventSetupEventLanguage     uint32 = 1 << 8
	qmiEventIdleModeText           uint32 = 1 << 9
	qmiEventLanguageNotification   uint32 = 1 << 10
	qmiEventRefreshAlpha           uint32 = 1 << 11
	qmiEventPlayTone               uint32 = 1 << 13
	qmiEventSetupCall              uint32 = 1 << 14
	qmiEventSendDTMF               uint32 = 1 << 15
	qmiEventLaunchBrowser          uint32 = 1 << 16
	qmiEventSendSS                 uint32 = 1 << 17
	qmiEventSendUSSD               uint32 = 1 << 18
	qmiEventProvideLocalInfo       uint32 = 1 << 19
	qmiEventBIP                    uint32 = 1 << 20
	qmiEventSetupEventBrowser      uint32 = 1 << 21
	qmiEventActivate               uint32 = 1 << 24
	qmiEventSetupEventHCI          uint32 = 1 << 25
)
