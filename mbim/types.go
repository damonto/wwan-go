package mbim

import "net"

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

type SubscriberReadyState uint32

const (
	SubscriberReadyStateNotInitialized SubscriberReadyState = iota
	SubscriberReadyStateInitialized
	SubscriberReadyStateSIMNotInserted
	SubscriberReadyStateBadSIM
	SubscriberReadyStateFailure
	SubscriberReadyStateNotActivated
	SubscriberReadyStateDeviceLocked
	SubscriberReadyStateNoESIMProfile
)

type ReadyInfo uint32

const (
	ReadyInfoNone            ReadyInfo = 0
	ReadyInfoProtectUniqueID ReadyInfo = 1 << 0
)

type SubscriberReadyStatusFlags uint32

const (
	SubscriberReadyStatusFlagNone                 SubscriberReadyStatusFlags = 0
	SubscriberReadyStatusFlagESIM                 SubscriberReadyStatusFlags = 1 << 0
	SubscriberReadyStatusFlagSIMRemovabilityKnown SubscriberReadyStatusFlags = 1 << 1
	SubscriberReadyStatusFlagSIMRemovable         SubscriberReadyStatusFlags = 1 << 2
	SubscriberReadyStatusFlagSIMSlotActive        SubscriberReadyStatusFlags = 1 << 3
)

type CommandType uint32

const (
	CommandTypeQuery CommandType = iota
	CommandTypeSet
)

type RadioSwitchState uint32

const (
	RadioSwitchStateOff RadioSwitchState = iota
	RadioSwitchStateOn
)

type RadioStateInfo struct {
	HwRadioState RadioSwitchState
	SwRadioState RadioSwitchState
}

type PacketServiceAction uint32

const (
	PacketServiceActionAttach PacketServiceAction = iota
	PacketServiceActionDetach
)

type PacketServiceState uint32

const (
	PacketServiceStateUnknown PacketServiceState = iota
	PacketServiceStateAttaching
	PacketServiceStateAttached
	PacketServiceStateDetaching
	PacketServiceStateDetached
)

type PacketServiceInfo struct {
	NwError                   uint32
	PacketServiceState        PacketServiceState
	HighestAvailableDataClass uint32
	UplinkSpeed               uint64
	DownlinkSpeed             uint64
}

type RegisterState uint32

const (
	RegisterStateUnknown RegisterState = iota
	RegisterStateDeregistered
	RegisterStateSearching
	RegisterStateHome
	RegisterStateRoaming
	RegisterStatePartner
	RegisterStateDenied
)

type RegisterMode uint32

const (
	RegisterModeUnknown RegisterMode = iota
	RegisterModeAutomatic
	RegisterModeManual
)

type RegistrationFlags uint32

const (
	RegistrationFlagManualSelectionNotAvailable RegistrationFlags = 1 << iota
	RegistrationFlagPacketServiceAutomaticAttach
)

type RegistrationStateInfo struct {
	NwError              uint32
	RegisterState        RegisterState
	RegisterMode         RegisterMode
	AvailableDataClasses uint32
	CurrentCellularClass uint32
	ProviderID           string
	ProviderName         string
	RoamingText          string
	RegistrationFlags    RegistrationFlags
}

type ActivationCommand uint32

const (
	ActivationCommandDeactivate ActivationCommand = iota
	ActivationCommandActivate
)

type ActivationOption uint32

const (
	ActivationOptionDefault ActivationOption = iota
	ActivationOptionPerNonDefaultURSPRules
	ActivationOptionPerDefaultURSPRule
	ActivationOptionPerURSPRules
)

type Compression uint32

const (
	CompressionNone Compression = iota
	CompressionEnable
)

type AuthProtocol uint32

const (
	AuthProtocolNone AuthProtocol = iota
	AuthProtocolPAP
	AuthProtocolCHAP
	AuthProtocolMSCHAPV2
)

type ContextIPType uint32

const (
	ContextIPTypeDefault ContextIPType = iota
	ContextIPTypeIPv4
	ContextIPTypeIPv6
	ContextIPTypeIPv4v6
	ContextIPTypeIPv4AndIPv6
)

type ActivationState uint32

const (
	ActivationStateUnknown ActivationState = iota
	ActivationStateActivated
	ActivationStateActivating
	ActivationStateDeactivated
	ActivationStateDeactivating
)

type VoiceCallState uint32

const (
	VoiceCallStateNone VoiceCallState = iota
	VoiceCallStateInProgress
	VoiceCallStateHangUp
)

type AccessMediaType uint32

const (
	AccessMediaTypeNone AccessMediaType = iota
	AccessMediaType3GPP
	AccessMediaType3GPPPreferred
)

type ContextType [16]byte

var (
	ContextTypeNone     = ContextType{0xB4, 0x3F, 0x75, 0x8C, 0xA5, 0x60, 0x4B, 0x46, 0xB3, 0x5E, 0xC5, 0x86, 0x96, 0x41, 0xFB, 0x54}
	ContextTypeInternet = ContextType{0x7E, 0x5E, 0x2A, 0x7E, 0x4E, 0x6F, 0x72, 0x72, 0x73, 0x6B, 0x65, 0x6E, 0x7E, 0x5E, 0x2A, 0x7E}
	ContextTypeIMS      = ContextType{0x21, 0x61, 0x0D, 0x01, 0x30, 0x74, 0x4B, 0xCE, 0x94, 0x25, 0xB5, 0x3A, 0x07, 0xD6, 0x97, 0xD6}
)

type ProvisionedContext struct {
	ContextID    uint32
	ContextType  ContextType
	AccessString string
	UserName     string
	Password     string
	Compression  Compression
	AuthProtocol AuthProtocol
}

type TLVType uint16

const (
	TLVTypeWCharString TLVType = 10
	TLVTypePCO         TLVType = 13
)

type IPConfigurationAvailable uint32

const (
	IPConfigurationAvailableAddress IPConfigurationAvailable = 1 << iota
	IPConfigurationAvailableGateway
	IPConfigurationAvailableDNSServer
	IPConfigurationAvailableMTU
)

type IPAddress struct {
	IP           net.IP
	PrefixLength uint32
}

type IPConfigurationInfo struct {
	SessionID                  uint32
	IPv4ConfigurationAvailable IPConfigurationAvailable
	IPv6ConfigurationAvailable IPConfigurationAvailable
	IPv4Addresses              []IPAddress
	IPv6Addresses              []IPAddress
	IPv4MTU                    uint32
	IPv6MTU                    uint32
}

type STKPACProfile byte

const (
	STKPACNotHandledByFunctionCannotBeHandledByHost STKPACProfile = iota
	STKPACNotHandledByFunctionMayBeHandledByHost
	STKPACHandledByFunctionOnlyTransparentToHost
	STKPACHandledByFunctionNotificationToHostPossible
	STKPACHandledByFunctionNotificationsToHostEnabled
	STKPACHandledByFunctionCanBeOverriddenByHost
	STKPACHandledByHostFunctionNotAbleToHandle
	STKPACHandledByHostFunctionAbleToHandle
)

type STKPACType uint32

const (
	STKPACTypeProactiveCommand STKPACType = iota
	STKPACTypeNotification
)

type UiccApplicationType uint32

const (
	UiccApplicationTypeUnknown UiccApplicationType = iota
	UiccApplicationTypeMF
	UiccApplicationTypeMFSIM
	UiccApplicationTypeMFRUIM
	UiccApplicationTypeUSIM
	UiccApplicationTypeCSIM
	UiccApplicationTypeISIM
)

type UiccSecureMessaging uint32

const (
	UiccSecureMessagingNone UiccSecureMessaging = iota
	UiccSecureMessagingNoHeaderAuth
)

type UiccClassByteType uint32

const (
	UiccClassByteTypeInterIndustry UiccClassByteType = iota
	UiccClassByteTypeExtended
)

type UiccPassThroughAction uint32

const (
	UiccPassThroughActionDisable UiccPassThroughAction = iota
	UiccPassThroughActionEnable
)

type UiccPassThroughStatus uint32

const (
	UiccPassThroughStatusDisabled UiccPassThroughStatus = iota
	UiccPassThroughStatusEnabled
)

type UiccFileAccessibility uint32

const (
	UiccFileAccessibilityUnknown UiccFileAccessibility = iota
	UiccFileAccessibilityNotShareable
	UiccFileAccessibilityShareable
)

type UiccFileType uint32

const (
	UiccFileTypeUnknown UiccFileType = iota
	UiccFileTypeWorkingEF
	UiccFileTypeInternalEF
	UiccFileTypeDFOrADF
)

type UiccFileStructure uint32

const (
	UiccFileStructureUnknown UiccFileStructure = iota
	UiccFileStructureTransparent
	UiccFileStructureCyclic
	UiccFileStructureLinear
	UiccFileStructureBERTLV
)

type PinType uint32

const (
	PinTypeUnknown PinType = iota
	PinTypeCustom
	PinTypePIN1
	PinTypePIN2
	PinTypeDeviceSIM
	PinTypeDeviceFirstSIM
	PinTypeNetwork
	PinTypeNetworkSubset
	PinTypeServiceProvider
	PinTypeCorporate
	PinTypeSubsidy
	PinTypePUK1
	PinTypePUK2
	PinTypeDeviceFirstSIMPUK
	PinTypeNetworkPUK
	PinTypeNetworkSubsetPUK
	PinTypeServiceProviderPUK
	PinTypeCorporatePUK
	PinTypeNEV
	PinTypeADM
)

type FileStructure byte

const (
	FileStructureTransparent FileStructure = 0x41
	FileStructureLinearFixed FileStructure = 0x42
)

type FileType byte

const (
	FileTypeWorkingEF FileType = 0x21
	FileTypeDFOrADF   FileType = 0x38
)

type Application struct {
	AID   []byte
	Label string
}

type FileRef struct {
	AID  []byte
	Path []byte
}

type FileAttributes struct {
	FileStructure FileStructure
	FileType      FileType
	RecordSize    uint16
	RecordCount   uint16
	FileSize      uint16
}

type TransparentRead struct {
	File   FileRef
	Offset uint16
	Length uint16
}

type RecordRead struct {
	File   FileRef
	Record uint16
}
