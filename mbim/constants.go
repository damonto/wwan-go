package mbim

const (
	CIDDeviceCaps            = 0x00000001
	CIDRadioState            = 0x00000003
	CIDSubscriberReadyStatus = 0x00000002
	CIDRegisterState         = 0x00000009
	CIDPacketService         = 0x0000000A
	CIDConnect               = 0x0000000C
	CIDProvisionedContexts   = 0x0000000D
	CIDIPConfiguration       = 0x0000000F
	CIDDeviceServices        = 0x00000010

	CIDAuthAKA = 0x00000001

	CIDSTKPAC              = 0x00000001
	CIDSTKTerminalResponse = 0x00000002
	CIDSTKEnvelope         = 0x00000003

	CIDUiccATR                = 0x00000001
	CIDUiccOpenChannel        = 0x00000002
	CIDUiccCloseChannel       = 0x00000003
	CIDUiccAPDU               = 0x00000004
	CIDUiccTerminalCapability = 0x00000005
	CIDUiccReset              = 0x00000006
	CIDUiccApplicationList    = 0x00000007
	CIDUiccFileStatus         = 0x00000008
	CIDUiccReadBinary         = 0x00000009
	CIDUiccReadRecord         = 0x0000000A

	CIDProxyControlConfiguration = 0x00000001
	CIDDeviceSlotMappings        = 0x00000007
	CIDVersion                   = 0x0000000F
)

const uiccChannelGroupDefault = 1

const (
	mbimVersion10        uint16 = 0x0100
	mbimExVersion10      uint16 = 0x0100
	mbimExVersion30      uint16 = 0x0300
	mbimExVersion40      uint16 = 0x0400
	hostMBIMExVersion           = mbimExVersion40
	activeSubscriberSlot        = 0xFFFFFFFF
)

const (
	defaultMaxControlTransfer = 4096
	maxFrameLength            = 2 * 1024 * 1024
)
