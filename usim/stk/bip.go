package stk

import (
	"fmt"
	"slices"
)

type BearerType byte

const (
	BearerTypeDefault   BearerType = 0x03
	BearerTypeLocal     BearerType = 0x04
	BearerTypeBluetooth BearerType = 0x05
	BearerTypeIrDA      BearerType = 0x06
	BearerTypeRS232     BearerType = 0x07
	BearerTypeUSB       BearerType = 0x10
)

type BearerDescription struct {
	Type       BearerType
	Parameters []byte
}

func (d BearerDescription) MarshalBinary() ([]byte, error) {
	return d.bytes(), nil
}

func (d BearerDescription) bytes() []byte {
	return append([]byte{byte(d.Type)}, d.Parameters...)
}

func (d *BearerDescription) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("parsing bearer description: length %d, want at least 1", len(data))
	}
	d.Type = BearerType(data[0])
	d.Parameters = slices.Clone(data[1:])
	return nil
}

type AddressType byte

const (
	AddressTypeIPv4 AddressType = 0x21
	AddressTypeIPv6 AddressType = 0x57
)

type OtherAddress struct {
	Type    AddressType
	Address []byte
}

func (a OtherAddress) MarshalBinary() ([]byte, error) {
	return a.bytes(), nil
}

func (a OtherAddress) bytes() []byte {
	if len(a.Address) == 0 {
		return nil
	}
	return append([]byte{byte(a.Type)}, a.Address...)
}

func (a *OtherAddress) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		*a = OtherAddress{}
		return nil
	}
	a.Type = AddressType(data[0])
	a.Address = slices.Clone(data[1:])
	return nil
}

type TransportProtocol byte

const (
	TransportUDPClientRemote TransportProtocol = 0x01
	TransportTCPClientRemote TransportProtocol = 0x02
	TransportTCPServer       TransportProtocol = 0x03
	TransportUDPClientLocal  TransportProtocol = 0x04
	TransportTCPClientLocal  TransportProtocol = 0x05
	TransportDirect          TransportProtocol = 0x06
)

type TransportLevel struct {
	Protocol TransportProtocol
	Port     uint16
}

func (l TransportLevel) MarshalBinary() ([]byte, error) {
	return []byte{byte(l.Protocol), byte(l.Port >> 8), byte(l.Port)}, nil
}

func (l *TransportLevel) UnmarshalBinary(data []byte) error {
	if len(data) != 3 {
		return fmt.Errorf("parsing transport level: length %d, want 3", len(data))
	}
	l.Protocol = TransportProtocol(data[0])
	l.Port = uint16(data[1])<<8 | uint16(data[2])
	return nil
}

type RemoteEntityAddress struct {
	Coding  byte
	Address []byte
}

func (a RemoteEntityAddress) MarshalBinary() ([]byte, error) {
	return append([]byte{a.Coding}, a.Address...), nil
}

func (a *RemoteEntityAddress) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("parsing remote entity address: length %d, want at least 1", len(data))
	}
	a.Coding = data[0]
	a.Address = slices.Clone(data[1:])
	return nil
}

type ChannelStatus struct {
	Identifier byte
	Status     byte
	Info       byte
}

const (
	ChannelStatusNoInfo      byte = 0x00
	ChannelStatusLinkDropped byte = 0x05
)

func NewChannelStatus(identifier byte, established bool, info byte) ChannelStatus {
	status := identifier & 0x07
	if established {
		status |= 0x80
	}
	return ChannelStatus{Identifier: identifier, Status: status, Info: info}
}

func (s ChannelStatus) LinkEstablished() bool {
	return s.Status&0x80 != 0
}

func (s ChannelStatus) MarshalBinary() ([]byte, error) {
	return s.bytes(), nil
}

func (s ChannelStatus) bytes() []byte {
	status := s.Status
	if s.Identifier <= 7 {
		status = status&0xF8 | s.Identifier&0x07
	}
	return []byte{status, s.Info}
}

func (s *ChannelStatus) UnmarshalBinary(data []byte) error {
	if len(data) != 2 {
		return fmt.Errorf("parsing channel status: length %d, want 2", len(data))
	}
	s.Identifier = data[0] & 0x07
	s.Status = data[0]
	s.Info = data[1]
	return nil
}

type BIPCause byte

const (
	BIPCauseNoSpecificCause         BIPCause = 0x00
	BIPCauseNoChannelAvailable      BIPCause = 0x01
	BIPCauseChannelClosed           BIPCause = 0x02
	BIPCauseInvalidChannelID        BIPCause = 0x03
	BIPCauseBufferSizeUnavailable   BIPCause = 0x04
	BIPCauseSecurityError           BIPCause = 0x05
	BIPCauseTransportUnavailable    BIPCause = 0x06
	BIPCauseRemoteDeviceUnreachable BIPCause = 0x07
	BIPCauseServiceUnavailable      BIPCause = 0x08
	BIPCauseUnknownServiceID        BIPCause = 0x09
	BIPCausePortUnavailable         BIPCause = 0x10
	BIPCauseBadLaunchParameters     BIPCause = 0x11
	BIPCauseApplicationLaunchFailed BIPCause = 0x12
)

type OpenChannelCommand struct {
	CommandFrame
	Alpha                 *Text
	Icon                  *Icon
	BearerDescription     *BearerDescription
	BufferSize            uint16
	NetworkAccessName     string
	OtherAddresses        []OtherAddress
	LocalAddress          *OtherAddress
	DestinationAddress    *OtherAddress
	RemoteEntityAddress   *RemoteEntityAddress
	TransportLevel        *TransportLevel
	Login                 *Text
	Password              *Text
	Immediate             bool
	AutomaticReconnection bool
	Background            bool
	DNSServerRequest      bool
	LaunchParameters      bool
}

type CloseChannelCommand struct {
	CommandFrame
	ChannelID           byte
	Alpha               *Text
	Icon                *Icon
	ReuseNetworkAccess  bool
	TCPListenAfterClose bool
}

type ReceiveDataCommand struct {
	CommandFrame
	ChannelID byte
	Alpha     *Text
	Icon      *Icon
	Length    byte
}

type SendDataCommand struct {
	CommandFrame
	ChannelID       byte
	Alpha           *Text
	Icon            *Icon
	Data            []byte
	SendImmediately bool
}

type GetChannelStatusCommand struct {
	CommandFrame
	Alpha *Text
	Icon  *Icon
}

func OpenChannelOK(status ChannelStatus, bufferSize uint16, bearer ...BearerDescription) TerminalResponse {
	response := TerminalResponse{
		Result:          ResultCommandPerformed,
		ChannelStatuses: []ChannelStatus{status},
		BufferSize:      &bufferSize,
	}
	if len(bearer) > 0 {
		description := bearer[0]
		response.BearerDescription = &description
	}
	return response
}

func ReceiveDataOK(data []byte, remaining byte) TerminalResponse {
	return TerminalResponse{
		Result:         ResultCommandPerformed,
		ChannelData:    append([]byte{}, data...),
		ChannelDataLen: &remaining,
	}
}

func SendDataOK(available byte) TerminalResponse {
	return TerminalResponse{
		Result:         ResultCommandPerformed,
		ChannelDataLen: &available,
	}
}

func GetChannelStatusOK(statuses ...ChannelStatus) TerminalResponse {
	return TerminalResponse{
		Result:          ResultCommandPerformed,
		ChannelStatuses: slices.Clone(statuses),
	}
}

func BIPError(cause BIPCause) TerminalResponse {
	return Result(ResultBearerIndependentProtocolError, byte(cause))
}
