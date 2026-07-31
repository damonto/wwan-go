package modem

import "context"

type unsupportedBackend struct{}

func (unsupportedBackend) Close() error { return nil }

func (unsupportedBackend) Info(context.Context) (Info, error) {
	return Info{}, ErrNotSupported
}

func (unsupportedBackend) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{}, ErrNotSupported
}

func (unsupportedBackend) SetCapabilities(context.Context, Technology) error {
	return ErrNotSupported
}

func (unsupportedBackend) Modes(context.Context) ([]Mode, Mode, error) {
	return nil, Mode{}, ErrNotSupported
}

func (unsupportedBackend) SetModes(context.Context, Mode) error { return ErrNotSupported }

func (unsupportedBackend) SupportedBands(context.Context) ([]Band, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) Bands(context.Context) ([]Band, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SetBands(context.Context, []Band) error { return ErrNotSupported }

func (unsupportedBackend) Status(context.Context) (Status, error) {
	return Status{}, ErrNotSupported
}

func (unsupportedBackend) PowerState(context.Context) (PowerState, error) {
	return PowerStateUnknown, ErrNotSupported
}

func (unsupportedBackend) SetPowerState(context.Context, PowerState) error {
	return ErrNotSupported
}

func (unsupportedBackend) Reset(context.Context) error { return ErrNotSupported }

func (unsupportedBackend) WatchStatus(context.Context) (<-chan Result[Status], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SIMInfo(context.Context) (SIMInfo, error) {
	return SIMInfo{}, ErrNotSupported
}

func (unsupportedBackend) SIMSlots(context.Context) ([]SIMSlot, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SetPrimarySIMSlot(context.Context, uint8) error {
	return ErrNotSupported
}

func (unsupportedBackend) SendPIN(context.Context, string) error { return ErrNotSupported }

func (unsupportedBackend) SendPUK(context.Context, string, string) error {
	return ErrNotSupported
}

func (unsupportedBackend) EnablePIN(context.Context, string, bool) error {
	return ErrNotSupported
}

func (unsupportedBackend) ChangePIN(context.Context, string, string) error {
	return ErrNotSupported
}

func (unsupportedBackend) PreferredNetworks(context.Context) ([]PreferredNetwork, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SetPreferredNetworks(context.Context, []PreferredNetwork) error {
	return ErrNotSupported
}

func (unsupportedBackend) WatchSIM(context.Context) (<-chan Result[SIMInfo], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) NetworkStatus(context.Context) (NetworkStatus, error) {
	return NetworkStatus{}, ErrNotSupported
}

func (unsupportedBackend) Register(context.Context, RegisterConfig) error {
	return ErrNotSupported
}

func (unsupportedBackend) ScanNetworks(context.Context) ([]Operator, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SetPacketServiceState(context.Context, PacketServiceState) error {
	return ErrNotSupported
}

func (unsupportedBackend) FacilityLocks(context.Context) ([]FacilityLock, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SetFacilityLock(context.Context, Facility, bool, string) error {
	return ErrNotSupported
}

func (unsupportedBackend) UnblockFacilityLock(context.Context, Facility, string) error {
	return ErrNotSupported
}

func (unsupportedBackend) InitialEPSBearer(context.Context) (InitialEPSConfig, error) {
	return InitialEPSConfig{}, ErrNotSupported
}

func (unsupportedBackend) InitialEPSSettings(context.Context) (InitialEPSConfig, error) {
	return InitialEPSConfig{}, ErrNotSupported
}

func (unsupportedBackend) SetInitialEPSSettings(context.Context, InitialEPSConfig) (InitialEPSConfig, error) {
	return InitialEPSConfig{}, ErrNotSupported
}

func (unsupportedBackend) CellInfo(context.Context) ([]CellInfo, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) Signal(context.Context) (Signal, error) {
	return Signal{}, ErrNotSupported
}

func (unsupportedBackend) SetSignalThresholds(context.Context, SignalThresholds) error {
	return ErrNotSupported
}

func (unsupportedBackend) WatchNetwork(context.Context) (<-chan Result[NetworkStatus], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) WatchSignal(context.Context) (<-chan Result[Signal], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) Profiles(context.Context) ([]Profile, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) CreateProfile(context.Context, ProfileConfig) (Profile, error) {
	return Profile{}, ErrNotSupported
}

func (unsupportedBackend) UpdateProfile(context.Context, ProfileUpdate) (Profile, error) {
	return Profile{}, ErrNotSupported
}

func (unsupportedBackend) DeleteProfile(context.Context, int32) error { return ErrNotSupported }

func (unsupportedBackend) WatchProfiles(context.Context) (<-chan Result[[]Profile], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) Connect(context.Context, ConnectConfig) (sessionBackend, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) ListMessages(context.Context) ([]Message, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) MessageStorages(context.Context) (MessageStorageInfo, error) {
	return MessageStorageInfo{}, ErrNotSupported
}

func (unsupportedBackend) ReadStoredMessage(context.Context, MessageRef) (Message, error) {
	return Message{}, ErrNotSupported
}

func (unsupportedBackend) ReadMessage(context.Context, uint32) (Message, error) {
	return Message{}, ErrNotSupported
}

func (unsupportedBackend) SendMessage(context.Context, MessageConfig) (SendResult, error) {
	return SendResult{}, ErrNotSupported
}

func (unsupportedBackend) StoreMessage(context.Context, MessageConfig) ([]Message, error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) DeleteMessage(context.Context, uint32) error { return ErrNotSupported }

func (unsupportedBackend) DeleteStoredMessage(context.Context, MessageRef) error {
	return ErrNotSupported
}

func (unsupportedBackend) SendPDU(context.Context, []byte) (uint32, error) {
	return 0, ErrNotSupported
}

func (unsupportedBackend) WatchMessages(context.Context) (<-chan Result[Message], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) InitiateUSSD(context.Context, string) (USSDMessage, error) {
	return USSDMessage{}, ErrNotSupported
}

func (unsupportedBackend) RespondUSSD(context.Context, string) (USSDMessage, error) {
	return USSDMessage{}, ErrNotSupported
}

func (unsupportedBackend) CancelUSSD(context.Context) error { return ErrNotSupported }

func (unsupportedBackend) WatchUSSD(context.Context) (<-chan Result[USSDMessage], error) {
	return nil, ErrNotSupported
}

func (unsupportedBackend) SAR(context.Context) (SARState, error) {
	return SARState{}, ErrNotSupported
}

func (unsupportedBackend) SetSAR(context.Context, SARState) error { return ErrNotSupported }

func (unsupportedBackend) FirmwareUpdateInfo(context.Context) (FirmwareUpdateInfo, error) {
	return FirmwareUpdateInfo{}, ErrNotSupported
}
