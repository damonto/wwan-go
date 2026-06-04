package uim

const (
	FileStructureTransparent = 0x41
	FileStructureLinearFixed = 0x42
)

type FileAttributes struct {
	FileStructure byte
	FileType      byte
	RecordSize    uint16
	RecordCount   uint16
	FileSize      uint16
}

type File struct {
	Session Session
	AID     []byte
	Path    []byte
}

type TransparentRead struct {
	File   File
	Offset uint16
	Length uint16
}

type RecordRead struct {
	File   File
	Record uint16
	Length uint16
}

type AuthContext byte

const (
	AuthContext3G     AuthContext = 3
	AuthContextIMSAKA AuthContext = 11
)

type AuthenticateRequest struct {
	Session Session
	AID     []byte
	Context AuthContext
	Rand    []byte
	AUTN    []byte
}

type EnvelopeResponse struct {
	SW1  byte
	SW2  byte
	Data []byte
}

type PowerOnSIMRequest struct {
	Slot                uint8
	IgnoreHotSwapSwitch bool
}

type ChangeProvisioningSessionRequest struct {
	Session  Session
	Activate bool
	Slot     uint8
	AID      []byte
}

type OpenLogicalChannelRequest struct {
	AID []byte
}

type OpenLogicalChannelResponse struct {
	Channel uint8
}

type CloseLogicalChannelRequest struct {
	Channel uint8
}

type CloseLogicalChannelResponse struct{}

type SendAPDURequest struct {
	Command []byte
}

type SendAPDUResponse struct {
	Response []byte
}
