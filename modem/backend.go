package modem

import (
	"context"

	"github.com/damonto/wwan-go/modem/contract"
)

type coreBackend interface {
	Close() error
	Info(context.Context) (Info, error)
	Capabilities(context.Context) (Capabilities, error)
	SetCapabilities(context.Context, Technology) error
	Modes(context.Context) ([]Mode, Mode, error)
	SetModes(context.Context, Mode) error
	SupportedBands(context.Context) ([]Band, error)
	Bands(context.Context) ([]Band, error)
	SetBands(context.Context, []Band) error
	Status(context.Context) (Status, error)
	PowerState(context.Context) (PowerState, error)
	SetPowerState(context.Context, PowerState) error
	Reset(context.Context) error
	WatchStatus(context.Context) (<-chan Result[Status], error)
}

type simBackend interface {
	SIMInfo(context.Context) (SIMInfo, error)
	SIMSlots(context.Context) ([]SIMSlot, error)
	SetPrimarySIMSlot(context.Context, uint8) error
	SendPIN(context.Context, string) error
	SendPUK(context.Context, string, string) error
	EnablePIN(context.Context, string, bool) error
	ChangePIN(context.Context, string, string) error
	PreferredNetworks(context.Context) ([]PreferredNetwork, error)
	SetPreferredNetworks(context.Context, []PreferredNetwork) error
	WatchSIM(context.Context) (<-chan Result[SIMInfo], error)
}

type networkBackend interface {
	NetworkStatus(context.Context) (NetworkStatus, error)
	Register(context.Context, RegisterConfig) error
	ScanNetworks(context.Context) ([]Operator, error)
	SetPacketServiceState(context.Context, PacketServiceState) error
	FacilityLocks(context.Context) ([]FacilityLock, error)
	SetFacilityLock(context.Context, Facility, bool, string) error
	UnblockFacilityLock(context.Context, Facility, string) error
	InitialEPSBearer(context.Context) (InitialEPSConfig, error)
	InitialEPSSettings(context.Context) (InitialEPSConfig, error)
	SetInitialEPSSettings(context.Context, InitialEPSConfig) (InitialEPSConfig, error)
	CellInfo(context.Context) ([]CellInfo, error)
	Signal(context.Context) (Signal, error)
	SetSignalThresholds(context.Context, SignalThresholds) error
	WatchNetwork(context.Context) (<-chan Result[NetworkStatus], error)
	WatchSignal(context.Context) (<-chan Result[Signal], error)
}

type profileBackend interface {
	Profiles(context.Context) ([]Profile, error)
	CreateProfile(context.Context, ProfileConfig) (Profile, error)
	UpdateProfile(context.Context, ProfileUpdate) (Profile, error)
	DeleteProfile(context.Context, int32) error
	WatchProfiles(context.Context) (<-chan Result[[]Profile], error)
}

type bearerBackend interface {
	Connect(context.Context, ConnectConfig) (sessionBackend, error)
}

type sessionBackend = contract.Session

type messagingBackend interface {
	MessageStorages(context.Context) (MessageStorageInfo, error)
	ListMessages(context.Context) ([]Message, error)
	ReadMessage(context.Context, uint32) (Message, error)
	ReadStoredMessage(context.Context, MessageRef) (Message, error)
	SendMessage(context.Context, MessageConfig) (SendResult, error)
	StoreMessage(context.Context, MessageConfig) ([]Message, error)
	DeleteMessage(context.Context, uint32) error
	DeleteStoredMessage(context.Context, MessageRef) error
	SendPDU(context.Context, []byte) (uint32, error)
	WatchMessages(context.Context) (<-chan Result[Message], error)
}

type ussdBackend interface {
	InitiateUSSD(context.Context, string) (USSDMessage, error)
	RespondUSSD(context.Context, string) (USSDMessage, error)
	CancelUSSD(context.Context) error
	WatchUSSD(context.Context) (<-chan Result[USSDMessage], error)
}

type sarBackend interface {
	SAR(context.Context) (SARState, error)
	SetSAR(context.Context, SARState) error
	FirmwareUpdateInfo(context.Context) (FirmwareUpdateInfo, error)
}

type backend interface {
	coreBackend
	simBackend
	networkBackend
	profileBackend
	bearerBackend
	messagingBackend
	ussdBackend
	sarBackend
}
