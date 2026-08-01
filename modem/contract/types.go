package contract

import (
	"fmt"
	"net/netip"
	"time"
)

// Result is a streamed value or the terminal transport/decoding error.
// Context cancellation closes a stream without an error result.
type Result[T any] struct {
	Value T
	Err   error
}

type Protocol uint8

const (
	ProtocolUnknown Protocol = iota
	ProtocolQMI
	ProtocolMBIM
)

func (p Protocol) String() string {
	switch p {
	case ProtocolQMI:
		return "QMI"
	case ProtocolMBIM:
		return "MBIM"
	default:
		return "unknown"
	}
}

type Access uint8

const (
	AccessAuto Access = iota
	AccessProxy
	AccessDirect
)

func (a Access) String() string {
	switch a {
	case AccessProxy:
		return "proxy"
	case AccessDirect:
		return "direct"
	default:
		return "auto"
	}
}

type Technology uint64

const (
	TechnologyGSM Technology = 1 << iota
	TechnologyUMTS
	TechnologyLTE
	TechnologyLTECatM
	TechnologyLTENB
	TechnologyNR5GNSA
	TechnologyNR5GSA
)

const TechnologyAny = TechnologyGSM | TechnologyUMTS | TechnologyLTE |
	TechnologyLTECatM | TechnologyLTENB | TechnologyNR5GNSA | TechnologyNR5GSA

type Feature uint64

const (
	FeatureMultiSIM Feature = 1 << iota
	FeatureProfileManagement
	FeatureSMS
	FeatureUSSD
	FeatureSignalThresholds
	FeatureCellInfo
	FeatureSAR
	FeatureFirmwareUpdate
	FeatureFacilityLocks
	FeatureInitialEPSBearer
)

type IPFamily uint8

const (
	IPFamilyUnknown IPFamily = 0
	IPFamilyIPv4    IPFamily = 1 << iota
	IPFamilyIPv6
)

const IPFamilyIPv4v6 = IPFamilyIPv4 | IPFamilyIPv6

type Authentication uint8

const (
	AuthenticationNone Authentication = 0
	AuthenticationPAP  Authentication = 1 << iota
	AuthenticationCHAP
	AuthenticationMSCHAPv2
)

type APNType uint64

const (
	APNTypeDefault APNType = 1 << iota
	APNTypeIMS
	APNTypeMMS
	APNTypeTethering
	APNTypeSUPL
	APNTypeEmergency
)

const APNTypeAny = APNTypeDefault | APNTypeIMS | APNTypeMMS | APNTypeTethering | APNTypeSUPL | APNTypeEmergency

type PowerState uint8

const (
	PowerStateUnknown PowerState = iota
	PowerStateOff
	PowerStateLow
	PowerStateOn
)

type SIMState uint8

const (
	SIMStateUnknown SIMState = iota
	SIMStateAbsent
	SIMStateLocked
	SIMStateReady
	SIMStateFailure
)

type RegistrationState uint8

const (
	RegistrationUnknown RegistrationState = iota
	RegistrationIdle
	RegistrationSearching
	RegistrationHome
	RegistrationRoaming
	RegistrationDenied
)

type PacketServiceState uint8

const (
	PacketServiceUnknown PacketServiceState = iota
	PacketServiceAttaching
	PacketServiceAttached
	PacketServiceDetaching
	PacketServiceDetached
)

// Facility identifies the 3GPP personalization locks shared by QMI and MBIM.
type Facility uint8

const (
	FacilityNetwork Facility = iota + 1
	FacilityNetworkSubset
	FacilityServiceProvider
	FacilityCorporate
)

type FacilityLock struct {
	Facility       Facility
	Enabled        bool
	Blocked        bool
	VerifyRetries  uint32
	UnblockRetries uint32
}

type MessageState uint8

const (
	MessageStateUnknown MessageState = iota
	MessageStateReceivedUnread
	MessageStateReceivedRead
	MessageStateStoredUnsent
	MessageStateStoredSent
)

type MessageStorage uint8

const (
	MessageStorageUnknown MessageStorage = iota
	MessageStorageDevice
	MessageStorageSIM
)

type USSDState uint8

const (
	USSDStateUnknown USSDState = iota
	USSDStateIdle
	USSDStateActive
	USSDStateUserResponse
	USSDStateNetworkResponse
	USSDStateTerminated
)

// PortType identifies the role of one modem port.
type PortType uint8

const (
	PortUnknown PortType = iota
	PortQMI
	PortMBIM
	PortAT
	PortNetwork
)

// Port describes one endpoint exposed by a physical modem.
type Port struct {
	Type    PortType
	Name    string
	Path    string
	SysPath string
	Driver  string
	// ControlPath identifies the QMI or MBIM device node owning a network port.
	ControlPath string
}

// Protocol returns the control protocol carried by the port.
func (p Port) Protocol() Protocol {
	switch p.Type {
	case PortQMI:
		return ProtocolQMI
	case PortMBIM:
		return ProtocolMBIM
	default:
		return ProtocolUnknown
	}
}

// Device groups all ports exposed by one physical modem.
type Device struct {
	PhysicalPath string
	Ports        []Port
}

type DeviceEventType uint8

const (
	DevicePresent DeviceEventType = iota
	DeviceAdded
	DeviceRemoved
	DeviceChanged
)

type DeviceEvent struct {
	Type   DeviceEventType
	Device Device
}

type Info struct {
	Manufacturer     string
	Model            string
	Revision         string
	HardwareRevision string
	EquipmentID      string
	DeviceID         string
	OwnNumbers       []string
}

type Capabilities struct {
	SupportedTechnologies Technology
	CurrentTechnologies   Technology
	SupportedIPFamilies   IPFamily
	Features              Feature
	MaxBearers            uint32
	MaxActiveBearers      uint32
	MaxSIMSlots           uint8
}

type Mode struct {
	Allowed   Technology
	Preferred Technology
}

type Band struct {
	Technology Technology
	Number     uint16
}

type Status struct {
	Power         PowerState
	SIM           SIMState
	Registration  RegistrationState
	PacketService PacketServiceState
	Technology    Technology
	OperatorID    string
	OperatorName  string
	SignalQuality uint8
	OwnBearers    int
}

type SIMInfo struct {
	State        SIMState
	Slot         uint8
	ICCID        string
	IMSI         string
	EID          string
	OperatorID   string
	OperatorName string
	GID1         string
	SPN          string
	ATR          []byte
	OwnNumbers   []string
	PINRetries   uint8
	PUKRetries   uint8
}

type SIMSlot struct {
	Index  uint8
	Active bool
	State  SIMState
	ICCID  string
	EID    string
}

type PreferredNetwork struct {
	OperatorID string
	Technology Technology
}

type RegisterConfig struct {
	OperatorID string
	Technology Technology
}

type NetworkStatus struct {
	Registration          RegistrationState
	PacketService         PacketServiceState
	Technology            Technology
	Available             Technology
	OperatorID            string
	OperatorName          string
	RoamingText           string
	LocationAreaCode      uint32
	TrackingAreaCode      uint32
	CellID                uint64
	UplinkBitsPerSecond   uint64
	DownlinkBitsPerSecond uint64
}

type Operator struct {
	ID         string
	Name       string
	Technology Technology
	Available  bool
	Current    bool
	Forbidden  bool
}

type Profile struct {
	ID             int32
	APN            string
	IPFamily       IPFamily
	Authentication Authentication
	Username       string
	Password       string
	APNType        APNType
	Enabled        bool
}

func (p Profile) String() string {
	return fmt.Sprintf("Profile{ID:%d APN:%q IPFamily:%d Authentication:%d APNType:%d Enabled:%t}",
		p.ID, p.APN, p.IPFamily, p.Authentication, p.APNType, p.Enabled)
}

type ProfileConfig struct {
	APN            string
	IPFamily       IPFamily
	Authentication Authentication
	Username       string
	Password       string
	APNType        APNType
	Enabled        bool
}

func (c ProfileConfig) String() string {
	return fmt.Sprintf("ProfileConfig{APN:%q IPFamily:%d Authentication:%d APNType:%d Enabled:%t}",
		c.APN, c.IPFamily, c.Authentication, c.APNType, c.Enabled)
}

type ProfileUpdate struct {
	ID             int32
	APN            *string
	IPFamily       *IPFamily
	Authentication *Authentication
	Username       *string
	Password       *string
	APNType        *APNType
	Enabled        *bool
}

func (u ProfileUpdate) String() string {
	return fmt.Sprintf("ProfileUpdate{ID:%d APNSet:%t IPFamilySet:%t AuthenticationSet:%t UsernameSet:%t PasswordSet:%t APNTypeSet:%t EnabledSet:%t}",
		u.ID, u.APN != nil, u.IPFamily != nil, u.Authentication != nil,
		u.Username != nil, u.Password != nil, u.APNType != nil, u.Enabled != nil)
}

// InitialEPSConfig contains the APN settings used for LTE/5G initial attach.
type InitialEPSConfig struct {
	ProfileID      int32
	APN            string
	IPFamily       IPFamily
	Authentication Authentication
	Username       string
	Password       string
}

func (c InitialEPSConfig) String() string {
	return fmt.Sprintf("InitialEPSConfig{ProfileID:%d APN:%q IPFamily:%d Authentication:%d}",
		c.ProfileID, c.APN, c.IPFamily, c.Authentication)
}

type SignalValue struct {
	DB    float64
	Known bool
}

type RadioSignal struct {
	Technology Technology
	RSSI       SignalValue
	RSCP       SignalValue
	ECIO       SignalValue
	RSRQ       SignalValue
	RSRP       SignalValue
	SNR        SignalValue
}

type Signal struct {
	Quality uint8
	Radios  []RadioSignal
}

type SignalThresholds struct {
	Interval           time.Duration
	RSSIChangeDB       uint32
	ErrorRateThreshold bool
}

type CellInfo struct {
	Serving          bool
	Technology       Technology
	OperatorID       string
	LocationAreaCode uint32
	TrackingAreaCode uint32
	CellID           uint64
	PhysicalCellID   uint32
	ARFCN            uint32
	Signal           RadioSignal
}

type SARState struct {
	Enabled    bool
	PowerLevel uint32
}

type FirmwareUpdateMethod uint8

const (
	FirmwareUpdateUnknown FirmwareUpdateMethod = iota
	FirmwareUpdateFastboot
	FirmwareUpdateQDL
	FirmwareUpdateQDU
)

type FirmwareUpdateInfo struct {
	Methods   []FirmwareUpdateMethod
	Version   string
	DeviceIDs []string
	Ports     []string
}

type ConnectConfig struct {
	PIN            string
	OperatorID     string
	ProfileID      int32
	APN            string
	Authentication Authentication
	Username       string
	Password       string
	IPFamily       IPFamily
	Interface      string
	SessionID      *uint32
}

func (c ConnectConfig) String() string {
	sessionID := "auto"
	if c.SessionID != nil {
		sessionID = fmt.Sprint(*c.SessionID)
	}
	return fmt.Sprintf("ConnectConfig{OperatorID:%q ProfileID:%d APN:%q Authentication:%d IPFamily:%d Interface:%q SessionID:%s}",
		c.OperatorID, c.ProfileID, c.APN, c.Authentication, c.IPFamily, c.Interface, sessionID)
}

type NetworkConfig struct {
	Interface string
	Addresses []netip.Prefix
	Gateways  []netip.Addr
	DNS       []netip.Addr
	MTU       uint32
}

type BearerInfo struct {
	ID        uint64
	Connected bool
	ProfileID int32
	APN       string
	Network   NetworkConfig
}

type BearerStats struct {
	RXBytes   uint64
	TXBytes   uint64
	RXPackets uint64
	TXPackets uint64
	Duration  time.Duration
}

type BearerEvent struct {
	Info BearerInfo
}

type MessageRef struct {
	Storage MessageStorage
	ID      uint32
}

type MessageStorageInfo struct {
	Supported []MessageStorage
	Default   MessageStorage
}

type Message struct {
	ID               uint32
	Refs             []MessageRef
	State            MessageState
	Storage          MessageStorage
	Number           string
	SMSC             string
	Text             string
	Data             []byte
	Timestamp        time.Time
	DeliveryReport   bool
	MessageReference uint32
	PDU              []byte
	PDUs             [][]byte
}

type MessageConfig struct {
	Number         string
	SMSC           string
	Text           string
	Data           []byte
	DeliveryReport bool
	Storage        MessageStorage
}

type SendResult struct {
	References []uint32
	Messages   []Message
}

type USSDMessage struct {
	State USSDState
	Text  string
	DCS   uint32
	Data  []byte
}
