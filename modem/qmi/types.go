package qmi

import "github.com/damonto/wwan-go/modem/contract"

var ErrNotSupported = contract.ErrNotSupported

type sessionBackend = contract.Session

type Result[T any] = contract.Result[T]

type Protocol = contract.Protocol

const (
	ProtocolUnknown = contract.ProtocolUnknown
	ProtocolQMI     = contract.ProtocolQMI
	ProtocolMBIM    = contract.ProtocolMBIM
)

type Access = contract.Access

const (
	AccessAuto   = contract.AccessAuto
	AccessProxy  = contract.AccessProxy
	AccessDirect = contract.AccessDirect
)

type Technology = contract.Technology

const (
	TechnologyGSM     = contract.TechnologyGSM
	TechnologyUMTS    = contract.TechnologyUMTS
	TechnologyLTE     = contract.TechnologyLTE
	TechnologyLTECatM = contract.TechnologyLTECatM
	TechnologyLTENB   = contract.TechnologyLTENB
	TechnologyNR5GNSA = contract.TechnologyNR5GNSA
	TechnologyNR5GSA  = contract.TechnologyNR5GSA
	TechnologyAny     = contract.TechnologyAny
)

type Feature = contract.Feature

const (
	FeatureMultiSIM          = contract.FeatureMultiSIM
	FeatureProfileManagement = contract.FeatureProfileManagement
	FeatureSMS               = contract.FeatureSMS
	FeatureUSSD              = contract.FeatureUSSD
	FeatureSignalThresholds  = contract.FeatureSignalThresholds
	FeatureCellInfo          = contract.FeatureCellInfo
	FeatureSAR               = contract.FeatureSAR
	FeatureFirmwareUpdate    = contract.FeatureFirmwareUpdate
	FeatureFacilityLocks     = contract.FeatureFacilityLocks
	FeatureInitialEPSBearer  = contract.FeatureInitialEPSBearer
)

type IPFamily = contract.IPFamily

const (
	IPFamilyUnknown = contract.IPFamilyUnknown
	IPFamilyIPv4    = contract.IPFamilyIPv4
	IPFamilyIPv6    = contract.IPFamilyIPv6
	IPFamilyIPv4v6  = contract.IPFamilyIPv4v6
)

type Authentication = contract.Authentication

const (
	AuthenticationNone     = contract.AuthenticationNone
	AuthenticationPAP      = contract.AuthenticationPAP
	AuthenticationCHAP     = contract.AuthenticationCHAP
	AuthenticationMSCHAPv2 = contract.AuthenticationMSCHAPv2
)

type APNType = contract.APNType

const (
	APNTypeDefault   = contract.APNTypeDefault
	APNTypeIMS       = contract.APNTypeIMS
	APNTypeMMS       = contract.APNTypeMMS
	APNTypeTethering = contract.APNTypeTethering
	APNTypeSUPL      = contract.APNTypeSUPL
	APNTypeEmergency = contract.APNTypeEmergency
	APNTypeAny       = contract.APNTypeAny
)

type PowerState = contract.PowerState

const (
	PowerStateUnknown = contract.PowerStateUnknown
	PowerStateOff     = contract.PowerStateOff
	PowerStateLow     = contract.PowerStateLow
	PowerStateOn      = contract.PowerStateOn
)

type SIMState = contract.SIMState

const (
	SIMStateUnknown = contract.SIMStateUnknown
	SIMStateAbsent  = contract.SIMStateAbsent
	SIMStateLocked  = contract.SIMStateLocked
	SIMStateReady   = contract.SIMStateReady
	SIMStateFailure = contract.SIMStateFailure
)

type RegistrationState = contract.RegistrationState

const (
	RegistrationUnknown   = contract.RegistrationUnknown
	RegistrationIdle      = contract.RegistrationIdle
	RegistrationSearching = contract.RegistrationSearching
	RegistrationHome      = contract.RegistrationHome
	RegistrationRoaming   = contract.RegistrationRoaming
	RegistrationDenied    = contract.RegistrationDenied
)

type PacketServiceState = contract.PacketServiceState

const (
	PacketServiceUnknown   = contract.PacketServiceUnknown
	PacketServiceAttaching = contract.PacketServiceAttaching
	PacketServiceAttached  = contract.PacketServiceAttached
	PacketServiceDetaching = contract.PacketServiceDetaching
	PacketServiceDetached  = contract.PacketServiceDetached
)

type Facility = contract.Facility

const (
	FacilityNetwork         = contract.FacilityNetwork
	FacilityNetworkSubset   = contract.FacilityNetworkSubset
	FacilityServiceProvider = contract.FacilityServiceProvider
	FacilityCorporate       = contract.FacilityCorporate
)

type MessageState = contract.MessageState

const (
	MessageStateUnknown        = contract.MessageStateUnknown
	MessageStateReceivedUnread = contract.MessageStateReceivedUnread
	MessageStateReceivedRead   = contract.MessageStateReceivedRead
	MessageStateStoredUnsent   = contract.MessageStateStoredUnsent
	MessageStateStoredSent     = contract.MessageStateStoredSent
)

type MessageStorage = contract.MessageStorage

const (
	MessageStorageUnknown = contract.MessageStorageUnknown
	MessageStorageDevice  = contract.MessageStorageDevice
	MessageStorageSIM     = contract.MessageStorageSIM
)

type USSDState = contract.USSDState

const (
	USSDStateUnknown         = contract.USSDStateUnknown
	USSDStateIdle            = contract.USSDStateIdle
	USSDStateActive          = contract.USSDStateActive
	USSDStateUserResponse    = contract.USSDStateUserResponse
	USSDStateNetworkResponse = contract.USSDStateNetworkResponse
	USSDStateTerminated      = contract.USSDStateTerminated
)

type DeviceEventType = contract.DeviceEventType

const (
	DevicePresent = contract.DevicePresent
	DeviceAdded   = contract.DeviceAdded
	DeviceRemoved = contract.DeviceRemoved
	DeviceChanged = contract.DeviceChanged
)

type FirmwareUpdateMethod = contract.FirmwareUpdateMethod

const (
	FirmwareUpdateUnknown  = contract.FirmwareUpdateUnknown
	FirmwareUpdateFastboot = contract.FirmwareUpdateFastboot
	FirmwareUpdateQDL      = contract.FirmwareUpdateQDL
	FirmwareUpdateQDU      = contract.FirmwareUpdateQDU
)

type Device = contract.Device
type Port = contract.Port
type PortType = contract.PortType

const (
	PortUnknown = contract.PortUnknown
	PortQMI     = contract.PortQMI
	PortMBIM    = contract.PortMBIM
	PortAT      = contract.PortAT
	PortNetwork = contract.PortNetwork
)

type DeviceEvent = contract.DeviceEvent
type Info = contract.Info
type Capabilities = contract.Capabilities
type Mode = contract.Mode
type Band = contract.Band
type Status = contract.Status
type SIMInfo = contract.SIMInfo
type SIMSlot = contract.SIMSlot
type PreferredNetwork = contract.PreferredNetwork
type RegisterConfig = contract.RegisterConfig
type NetworkStatus = contract.NetworkStatus
type Operator = contract.Operator
type FacilityLock = contract.FacilityLock
type Profile = contract.Profile
type ProfileConfig = contract.ProfileConfig
type ProfileUpdate = contract.ProfileUpdate
type InitialEPSConfig = contract.InitialEPSConfig
type SignalValue = contract.SignalValue
type RadioSignal = contract.RadioSignal
type Signal = contract.Signal
type SignalThresholds = contract.SignalThresholds
type CellInfo = contract.CellInfo
type SARState = contract.SARState
type FirmwareUpdateInfo = contract.FirmwareUpdateInfo
type ConnectConfig = contract.ConnectConfig
type NetworkConfig = contract.NetworkConfig
type BearerInfo = contract.BearerInfo
type BearerStats = contract.BearerStats
type BearerEvent = contract.BearerEvent
type Message = contract.Message
type MessageRef = contract.MessageRef
type MessageStorageInfo = contract.MessageStorageInfo
type MessageConfig = contract.MessageConfig
type SendResult = contract.SendResult
type USSDMessage = contract.USSDMessage
