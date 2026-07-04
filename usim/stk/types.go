package stk

import (
	"encoding/binary"
	"fmt"
	"slices"
	"unicode/utf16"
)

const (
	TagProactiveCommand = 0xD0
	TagSMSPPDownload    = 0xD1
	TagMenuSelection    = 0xD3
	TagEventDownload    = 0xD6
)

const (
	tlvCommandDetails = 0x01
	tlvDeviceIDs      = 0x02
	tlvResult         = 0x03
	tlvDuration       = 0x04
	tlvAlphaID        = 0x05
	tlvAddress        = 0x06
	tlvSubaddress     = 0x08
	tlvSMSTPDU        = 0x0B
	tlvTextString     = 0x0D
	tlvTone           = 0x0E
	tlvItem           = 0x0F
	tlvItemID         = 0x10
	tlvResponseLength = 0x11
	tlvFileList       = 0x12
	tlvHelpRequest    = 0x15
	tlvDefaultText    = 0x17
	tlvEventList      = 0x19
	tlvCause          = 0x1A
	tlvLocationStatus = 0x1B
	tlvTransactionID  = 0x1C
	tlvIconID         = 0x1E
	tlvItemIconList   = 0x1F
	tlvCardReaderID   = 0x20
	tlvCAPDU          = 0x22
	tlvRAPDU          = 0x23
	tlvTimerID        = 0x24
	tlvTimerValue     = 0x25
	tlvDateTimeZone   = 0x26
	tlvATCommand      = 0x28
	tlvATResponse     = 0x29
	tlvImmediateResp  = 0x2B
	tlvLanguage       = 0x2D
	tlvBrowserID      = 0x30
	tlvURL            = 0x31
	tlvBearer         = 0x32
	tlvBearerDesc     = 0x35
	tlvChannelData    = 0x36
	tlvChannelDataLen = 0x37
	tlvChannelStatus  = 0x38
	tlvBufferSize     = 0x39
	tlvTransportLevel = 0x3C
	tlvOtherAddress   = 0x3E
	tlvRemoteEntity   = 0x46
	tlvNetworkAccess  = 0x47
	tlvTextAttribute  = 0x50
	tlvFrameID        = 0x68
)

type CommandType byte

const (
	CommandRefresh            CommandType = 0x01
	CommandMoreTime           CommandType = 0x02
	CommandPollInterval       CommandType = 0x03
	CommandPollingOff         CommandType = 0x04
	CommandSetupEventList     CommandType = 0x05
	CommandSetupCall          CommandType = 0x10
	CommandSendSS             CommandType = 0x11
	CommandSendUSSD           CommandType = 0x12
	CommandSendSMS            CommandType = 0x13
	CommandSendDTMF           CommandType = 0x14
	CommandLaunchBrowser      CommandType = 0x15
	CommandPlayTone           CommandType = 0x20
	CommandDisplayText        CommandType = 0x21
	CommandGetInkey           CommandType = 0x22
	CommandGetInput           CommandType = 0x23
	CommandSelectItem         CommandType = 0x24
	CommandSetupMenu          CommandType = 0x25
	CommandProvideLocalInfo   CommandType = 0x26
	CommandTimerManagement    CommandType = 0x27
	CommandSetupIdleModeText  CommandType = 0x28
	CommandPerformCardAPDU    CommandType = 0x30
	CommandPowerOnCard        CommandType = 0x31
	CommandPowerOffCard       CommandType = 0x32
	CommandGetReaderStatus    CommandType = 0x33
	CommandRunATCommand       CommandType = 0x34
	CommandLanguageNotify     CommandType = 0x35
	CommandOpenChannel        CommandType = 0x40
	CommandCloseChannel       CommandType = 0x41
	CommandReceiveData        CommandType = 0x42
	CommandSendData           CommandType = 0x43
	CommandGetChannelStatus   CommandType = 0x44
	CommandServiceSearch      CommandType = 0x45
	CommandGetServiceInfo     CommandType = 0x46
	CommandDeclareService     CommandType = 0x47
	CommandFrames             CommandType = 0x50
	CommandGetFramesStatus    CommandType = 0x51
	CommandRetrieveMultimedia CommandType = 0x60
	CommandSubmitMultimedia   CommandType = 0x61
	CommandDisplayMultimedia  CommandType = 0x62
	CommandActivate           CommandType = 0x70
	CommandContactlessState   CommandType = 0x71
	CommandCommandContainer   CommandType = 0x72
)

type DeviceID byte

const (
	DeviceKeypad   DeviceID = 0x01
	DeviceDisplay  DeviceID = 0x02
	DeviceEarpiece DeviceID = 0x03
	DeviceUICC     DeviceID = 0x81
	DeviceTerminal DeviceID = 0x82
	DeviceNetwork  DeviceID = 0x83
	DeviceChannel  DeviceID = 0x20
	DeviceChannel1 DeviceID = 0x21
	DeviceChannel2 DeviceID = 0x22
	DeviceChannel3 DeviceID = 0x23
	DeviceChannel4 DeviceID = 0x24
	DeviceChannel5 DeviceID = 0x25
	DeviceChannel6 DeviceID = 0x26
	DeviceChannel7 DeviceID = 0x27
)

type ResultCode byte

const (
	ResultCommandPerformed                  ResultCode = 0x00
	ResultPartialComprehension              ResultCode = 0x01
	ResultMissingInformation                ResultCode = 0x02
	ResultRefreshAdditionalFiles            ResultCode = 0x03
	ResultIconNotDisplayed                  ResultCode = 0x04
	ResultModifiedByCallControl             ResultCode = 0x05
	ResultLimitedService                    ResultCode = 0x06
	ResultPerformedWithModification         ResultCode = 0x07
	ResultUserTermination                   ResultCode = 0x10
	ResultBackwardMove                      ResultCode = 0x11
	ResultNoResponseFromUser                ResultCode = 0x12
	ResultHelpInformationRequired           ResultCode = 0x13
	ResultTerminalUnableToProcess           ResultCode = 0x20
	ResultNetworkUnableToProcess            ResultCode = 0x21
	ResultUserDidNotAccept                  ResultCode = 0x22
	ResultUserClearedDownCall               ResultCode = 0x23
	ResultTimerStateConflict                ResultCode = 0x24
	ResultTemporaryCallControlProblem       ResultCode = 0x25
	ResultLaunchBrowserError                ResultCode = 0x26
	ResultCommandBeyondTerminalCapabilities ResultCode = 0x30
	ResultCommandTypeNotUnderstood          ResultCode = 0x31
	ResultCommandDataNotUnderstood          ResultCode = 0x32
	ResultCommandNumberNotKnown             ResultCode = 0x33
	ResultSSReturnError                     ResultCode = 0x34
	ResultSMSRPError                        ResultCode = 0x35
	ResultRequiredValuesMissing             ResultCode = 0x36
	ResultUSSDReturnError                   ResultCode = 0x37
	ResultMultipleCardCommandsError         ResultCode = 0x38
	ResultPermanentCallControlProblem       ResultCode = 0x39
	ResultBearerIndependentProtocolError    ResultCode = 0x3A
	ResultAccessTechnologyUnableToProcess   ResultCode = 0x3B
	ResultFramesError                       ResultCode = 0x3C
	ResultMultimediaError                   ResultCode = 0x3D
)

type Event byte

const (
	EventMTCall                 Event = 0x00
	EventCallConnected          Event = 0x01
	EventCallDisconnected       Event = 0x02
	EventLocationStatus         Event = 0x03
	EventUserActivity           Event = 0x04
	EventIdleScreenAvailable    Event = 0x05
	EventCardReaderStatus       Event = 0x06
	EventLanguageSelection      Event = 0x07
	EventBrowserTermination     Event = 0x08
	EventDataAvailable          Event = 0x09
	EventChannelStatus          Event = 0x0A
	EventAccessTechnologyChange Event = 0x0B
	EventDisplayParameters      Event = 0x0C
	EventLocalConnection        Event = 0x0D
	EventHCIConnectivity        Event = 0x13
	EventNetworkSearchMode      Event = 0x14
	EventBrowsingStatus         Event = 0x15
	EventFramesInformation      Event = 0x16
)

type CommandDetails struct {
	Number    byte
	Type      CommandType
	Qualifier byte
}

type DeviceIdentities struct {
	Source      DeviceID
	Destination DeviceID
}

type Text struct {
	DCS    byte
	Raw    []byte
	String string
}

func (text Text) MarshalBinary() ([]byte, error) {
	raw := slices.Clone(text.Raw)
	dcs := text.DCS
	if len(raw) == 0 && text.String != "" {
		if dcs == 0 {
			dcs = 0x04
		}
		switch dcs {
		case 0x00:
			raw, _ = gsm7Text(text.String).MarshalText()
		case 0x08:
			runes := utf16.Encode([]rune(text.String))
			raw = make([]byte, 0, len(runes)*2)
			for _, r := range runes {
				raw = binary.BigEndian.AppendUint16(raw, r)
			}
		default:
			raw = []byte(text.String)
		}
	}
	if len(raw) == 0 && text.String == "" && dcs == 0 {
		return nil, nil
	}

	out := make([]byte, 0, 1+len(raw))
	out = append(out, dcs)
	out = append(out, raw...)
	return out, nil
}

func (text *Text) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		*text = Text{}
		return nil
	}
	text.unmarshal(data[0], data[1:])
	return nil
}

func (text Text) MarshalText() ([]byte, error) {
	if text.String != "" {
		return []byte(text.String), nil
	}
	var value Text
	if err := value.UnmarshalBinary(append([]byte{text.DCS}, text.Raw...)); err != nil {
		return nil, err
	}
	return []byte(value.String), nil
}

func (text *Text) UnmarshalText(data []byte) error {
	text.unmarshal(0x04, data)
	return nil
}

func (text *Text) unmarshal(dcs byte, raw []byte) {
	*text = Text{
		DCS:    dcs,
		Raw:    slices.Clone(raw),
		String: decodeTextString(dcs, raw),
	}
}

type Duration struct {
	Unit     byte
	Interval byte
}

type Icon struct {
	SelfExplanatory bool
	Record          byte
}

type Item struct {
	Identifier byte
	Text       Text
}

type ResponseLength struct {
	Min byte
	Max byte
}

type EnvelopeResponse struct {
	SW1  byte
	SW2  byte
	Data []byte
}

func (r EnvelopeResponse) OK() bool {
	return r.SW1 == 0x90 && r.SW2 == 0x00
}

func (r EnvelopeResponse) HasMore() bool {
	return r.SW1 == 0x91 || r.SW1 == 0x61
}

func decodeTextString(dcs byte, data []byte) string {
	switch dcs {
	case 0x08:
		if len(data)%2 != 0 {
			return string(data)
		}
		runes := make([]uint16, 0, len(data)/2)
		for len(data) > 0 {
			runes = append(runes, binary.BigEndian.Uint16(data[:2]))
			data = data[2:]
		}
		return string(utf16.Decode(runes))
	case 0x00:
		var text gsm7Text
		_ = text.UnmarshalText(data)
		return string(text)
	default:
		return string(data)
	}
}

type gsm7Text string

func (text gsm7Text) MarshalText() ([]byte, error) {
	value := string(text)
	if value == "" {
		return nil, nil
	}

	septets := make([]byte, 0, len(value))
	for _, r := range value {
		septets = append(septets, gsm7Septets(r)...)
	}

	out := make([]byte, (len(septets)*7+7)/8)
	bit := 0
	for _, septet := range septets {
		for i := range 7 {
			if septet&(1<<i) != 0 {
				out[(bit+i)/8] |= 1 << ((bit + i) % 8)
			}
		}
		bit += 7
	}
	return out, nil
}

func (text *gsm7Text) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		*text = ""
		return nil
	}

	septets := make([]byte, 0, len(data))
	carry := byte(0)
	carryBits := 0
	for _, b := range data {
		septet := ((b << carryBits) & 0x7f) | carry
		septets = append(septets, septet)
		carry = b >> (7 - carryBits)
		carryBits++
		if carryBits == 7 {
			septets = append(septets, carry&0x7f)
			carry = 0
			carryBits = 0
		}
	}

	out := make([]rune, 0, len(septets))
	for i := 0; i < len(septets); i++ {
		if septets[i] == 0x1B {
			if i+1 >= len(septets) {
				out = append(out, '?')
				continue
			}
			i++
			out = append(out, gsm7ExtensionChar(septets[i]))
			continue
		}
		out = append(out, gsm7Char(septets[i]))
	}
	*text = gsm7Text(string(out))
	return nil
}

func gsm7Char(v byte) rune {
	switch v {
	case 0x00:
		return '@'
	case 0x01:
		return '£'
	case 0x02:
		return '$'
	case 0x03:
		return '¥'
	case 0x04:
		return 'è'
	case 0x05:
		return 'é'
	case 0x06:
		return 'ù'
	case 0x07:
		return 'ì'
	case 0x08:
		return 'ò'
	case 0x09:
		return 'Ç'
	case 0x0a:
		return '\n'
	case 0x0b:
		return 'Ø'
	case 0x0c:
		return 'ø'
	case 0x0d:
		return '\r'
	case 0x0e:
		return 'Å'
	case 0x0f:
		return 'å'
	case 0x10:
		return 'Δ'
	case 0x11:
		return '_'
	case 0x12:
		return 'Φ'
	case 0x13:
		return 'Γ'
	case 0x14:
		return 'Λ'
	case 0x15:
		return 'Ω'
	case 0x16:
		return 'Π'
	case 0x17:
		return 'Ψ'
	case 0x18:
		return 'Σ'
	case 0x19:
		return 'Θ'
	case 0x1a:
		return 'Ξ'
	case 0x1c:
		return 'Æ'
	case 0x1d:
		return 'æ'
	case 0x1e:
		return 'ß'
	case 0x1f:
		return 'É'
	case 0x24:
		return '¤'
	case 0x40:
		return '¡'
	case 0x5b:
		return 'Ä'
	case 0x5c:
		return 'Ö'
	case 0x5d:
		return 'Ñ'
	case 0x5e:
		return 'Ü'
	case 0x5f:
		return '§'
	case 0x60:
		return '¿'
	case 0x7b:
		return 'ä'
	case 0x7c:
		return 'ö'
	case 0x7d:
		return 'ñ'
	case 0x7e:
		return 'ü'
	case 0x7f:
		return 'à'
	default:
		if (v >= 0x20 && v <= 0x3f) || (v >= 0x41 && v <= 0x5a) || (v >= 0x61 && v <= 0x7a) {
			return rune(v)
		}
		return '?'
	}
}

func gsm7ExtensionChar(v byte) rune {
	switch v {
	case 0x0a:
		return '\f'
	case 0x14:
		return '^'
	case 0x28:
		return '{'
	case 0x29:
		return '}'
	case 0x2f:
		return '\\'
	case 0x3c:
		return '['
	case 0x3d:
		return '~'
	case 0x3e:
		return ']'
	case 0x40:
		return '|'
	case 0x65:
		return '€'
	default:
		return '?'
	}
}

func gsm7Septets(r rune) []byte {
	switch r {
	case '@':
		return []byte{0x00}
	case '£':
		return []byte{0x01}
	case '$':
		return []byte{0x02}
	case '¥':
		return []byte{0x03}
	case 'è':
		return []byte{0x04}
	case 'é':
		return []byte{0x05}
	case 'ù':
		return []byte{0x06}
	case 'ì':
		return []byte{0x07}
	case 'ò':
		return []byte{0x08}
	case 'Ç':
		return []byte{0x09}
	case '\n':
		return []byte{0x0a}
	case 'Ø':
		return []byte{0x0b}
	case 'ø':
		return []byte{0x0c}
	case '\r':
		return []byte{0x0d}
	case 'Å':
		return []byte{0x0e}
	case 'å':
		return []byte{0x0f}
	case 'Δ':
		return []byte{0x10}
	case '_':
		return []byte{0x11}
	case 'Φ':
		return []byte{0x12}
	case 'Γ':
		return []byte{0x13}
	case 'Λ':
		return []byte{0x14}
	case 'Ω':
		return []byte{0x15}
	case 'Π':
		return []byte{0x16}
	case 'Ψ':
		return []byte{0x17}
	case 'Σ':
		return []byte{0x18}
	case 'Θ':
		return []byte{0x19}
	case 'Ξ':
		return []byte{0x1a}
	case 'Æ':
		return []byte{0x1c}
	case 'æ':
		return []byte{0x1d}
	case 'ß':
		return []byte{0x1e}
	case 'É':
		return []byte{0x1f}
	case '¤':
		return []byte{0x24}
	case '¡':
		return []byte{0x40}
	case 'Ä':
		return []byte{0x5b}
	case 'Ö':
		return []byte{0x5c}
	case 'Ñ':
		return []byte{0x5d}
	case 'Ü':
		return []byte{0x5e}
	case '§':
		return []byte{0x5f}
	case '¿':
		return []byte{0x60}
	case 'ä':
		return []byte{0x7b}
	case 'ö':
		return []byte{0x7c}
	case 'ñ':
		return []byte{0x7d}
	case 'ü':
		return []byte{0x7e}
	case 'à':
		return []byte{0x7f}
	case '\f':
		return []byte{0x1b, 0x0a}
	case '^':
		return []byte{0x1b, 0x14}
	case '{':
		return []byte{0x1b, 0x28}
	case '}':
		return []byte{0x1b, 0x29}
	case '\\':
		return []byte{0x1b, 0x2f}
	case '[':
		return []byte{0x1b, 0x3c}
	case '~':
		return []byte{0x1b, 0x3d}
	case ']':
		return []byte{0x1b, 0x3e}
	case '|':
		return []byte{0x1b, 0x40}
	case '€':
		return []byte{0x1b, 0x65}
	default:
		if (r >= 0x20 && r <= 0x3f) || (r >= 0x41 && r <= 0x5a) || (r >= 0x61 && r <= 0x7a) {
			return []byte{byte(r)}
		}
		return []byte{'?'}
	}
}

func requireLen(name string, value []byte, n int) error {
	if len(value) < n {
		return fmt.Errorf("parsing %s: value length %d, want at least %d", name, len(value), n)
	}
	return nil
}
