package mbim

type MessageType uint32

const (
	MessageTypeOpen      MessageType = 0x00000001
	MessageTypeClose     MessageType = 0x00000002
	MessageTypeCommand   MessageType = 0x00000003
	MessageTypeHostError MessageType = 0x00000004

	MessageTypeOpenDone       MessageType = 0x80000001
	MessageTypeCloseDone      MessageType = 0x80000002
	MessageTypeCommandDone    MessageType = 0x80000003
	MessageTypeFunctionError  MessageType = 0x80000004
	MessageTypeIndicateStatus MessageType = 0x80000007
)

type CommandType uint32

const (
	CommandTypeQuery CommandType = iota
	CommandTypeSet
)
