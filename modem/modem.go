package modem

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

const closeTimeout = 5 * time.Second

const connectPollInterval = 250 * time.Millisecond

// Modem is one opened QMI or MBIM control node.
type Modem struct {
	mu        sync.RWMutex
	backend   backend
	device    string
	protocol  Protocol
	access    Access
	closed    bool
	done      chan struct{}
	bearers   map[uint64]*Bearer
	nextID    uint64
	closeOnce sync.Once
	closeErr  error
}

func newModem(device string, protocol Protocol, access Access, b backend) *Modem {
	return &Modem{
		backend:  b,
		device:   device,
		protocol: protocol,
		access:   access,
		done:     make(chan struct{}),
		bearers:  make(map[uint64]*Bearer),
	}
}

func (m *Modem) Device() string { return m.device }

func (m *Modem) Protocol() Protocol { return m.protocol }

func (m *Modem) Access() Access { return m.access }

func (m *Modem) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		close(m.done)
		bearers := make([]*Bearer, 0, len(m.bearers))
		for _, bearer := range m.bearers {
			bearers = append(bearers, bearer)
		}
		m.bearers = nil
		b := m.backend
		m.backend = nil
		m.mu.Unlock()

		var errs []error
		for _, bearer := range bearers {
			errs = append(errs, bearer.closeOwned())
		}
		if b != nil {
			errs = append(errs, b.Close())
		}
		m.closeErr = errors.Join(errs...)
	})
	return m.closeErr
}

func (m *Modem) currentBackend() (backend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.backend == nil {
		return nil, ErrClosed
	}
	return m.backend, nil
}

func (m *Modem) Info(ctx context.Context) (Info, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Info{}, err
	}
	return b.Info(ctx)
}

func (m *Modem) Capabilities(ctx context.Context) (Capabilities, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Capabilities{}, err
	}
	return b.Capabilities(ctx)
}

func (m *Modem) SetCapabilities(ctx context.Context, technologies Technology) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetCapabilities(ctx, technologies)
}

func (m *Modem) Modes(ctx context.Context) ([]Mode, Mode, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, Mode{}, err
	}
	return b.Modes(ctx)
}

func (m *Modem) SetModes(ctx context.Context, mode Mode) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetModes(ctx, mode)
}

func (m *Modem) SupportedBands(ctx context.Context) ([]Band, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.SupportedBands(ctx)
}

func (m *Modem) Bands(ctx context.Context) ([]Band, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.Bands(ctx)
}

// SetBands selects radio bands. An empty slice restores all supported bands.
func (m *Modem) SetBands(ctx context.Context, bands []Band) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetBands(ctx, slices.Clone(bands))
}

func (m *Modem) Status(ctx context.Context) (Status, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Status{}, err
	}
	status, err := b.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	m.mu.RLock()
	status.OwnBearers = len(m.bearers)
	m.mu.RUnlock()
	return status, nil
}

func (m *Modem) SetPowerState(ctx context.Context, state PowerState) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetPowerState(ctx, state)
}

func (m *Modem) Reset(ctx context.Context) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.Reset(ctx)
}

func (m *Modem) WatchStatus(ctx context.Context) (<-chan Result[Status], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchStatus)
}

func (m *Modem) SIMInfo(ctx context.Context) (SIMInfo, error) {
	b, err := m.currentBackend()
	if err != nil {
		return SIMInfo{}, err
	}
	return b.SIMInfo(ctx)
}

func (m *Modem) SIMSlots(ctx context.Context) ([]SIMSlot, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.SIMSlots(ctx)
}

func (m *Modem) SetPrimarySIMSlot(ctx context.Context, slot uint8) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	if slot == 0 {
		return errors.New("setting primary SIM slot: slot is zero")
	}
	return b.SetPrimarySIMSlot(ctx, slot)
}

func (m *Modem) SendPIN(ctx context.Context, pin string) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SendPIN(ctx, pin)
}

func (m *Modem) SendPUK(ctx context.Context, puk, newPIN string) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SendPUK(ctx, puk, newPIN)
}

func (m *Modem) EnablePIN(ctx context.Context, pin string, enabled bool) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.EnablePIN(ctx, pin, enabled)
}

func (m *Modem) ChangePIN(ctx context.Context, oldPIN, newPIN string) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.ChangePIN(ctx, oldPIN, newPIN)
}

func (m *Modem) PreferredNetworks(ctx context.Context) ([]PreferredNetwork, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.PreferredNetworks(ctx)
}

func (m *Modem) SetPreferredNetworks(ctx context.Context, networks []PreferredNetwork) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetPreferredNetworks(ctx, networks)
}

func (m *Modem) WatchSIM(ctx context.Context) (<-chan Result[SIMInfo], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchSIM)
}

func (m *Modem) NetworkStatus(ctx context.Context) (NetworkStatus, error) {
	b, err := m.currentBackend()
	if err != nil {
		return NetworkStatus{}, err
	}
	return b.NetworkStatus(ctx)
}

func (m *Modem) Register(ctx context.Context, cfg RegisterConfig) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.Register(ctx, cfg)
}

func (m *Modem) ScanNetworks(ctx context.Context) ([]Operator, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.ScanNetworks(ctx)
}

func (m *Modem) SetPacketServiceState(ctx context.Context, state PacketServiceState) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	if state != PacketServiceAttached && state != PacketServiceDetached {
		return fmt.Errorf("setting packet service: state %d is not attach or detach", state)
	}
	return b.SetPacketServiceState(ctx, state)
}

func (m *Modem) FacilityLocks(ctx context.Context) ([]FacilityLock, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.FacilityLocks(ctx)
}

func (m *Modem) SetFacilityLock(ctx context.Context, facility Facility, enabled bool, key string) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	if err := validateFacilityKey(facility, key); err != nil {
		return err
	}
	return b.SetFacilityLock(ctx, facility, enabled, key)
}

func (m *Modem) UnblockFacilityLock(ctx context.Context, facility Facility, key string) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	if err := validateFacilityKey(facility, key); err != nil {
		return err
	}
	return b.UnblockFacilityLock(ctx, facility, key)
}

func validateFacilityKey(facility Facility, key string) error {
	if facility < FacilityNetwork || facility > FacilityCorporate {
		return fmt.Errorf("setting facility lock: facility %d is invalid", facility)
	}
	if len(key) == 0 || len(key) > 8 {
		return fmt.Errorf("setting facility lock: key length %d is outside 1..8", len(key))
	}
	for i := range len(key) {
		if key[i] == 0 || key[i] > 0x7f {
			return errors.New("setting facility lock: key must contain non-NUL ASCII characters")
		}
	}
	return nil
}

func (m *Modem) InitialEPSBearer(ctx context.Context) (InitialEPSConfig, error) {
	b, err := m.currentBackend()
	if err != nil {
		return InitialEPSConfig{}, err
	}
	return b.InitialEPSBearer(ctx)
}

func (m *Modem) InitialEPSSettings(ctx context.Context) (InitialEPSConfig, error) {
	b, err := m.currentBackend()
	if err != nil {
		return InitialEPSConfig{}, err
	}
	return b.InitialEPSSettings(ctx)
}

func (m *Modem) SetInitialEPSSettings(ctx context.Context, cfg InitialEPSConfig) (InitialEPSConfig, error) {
	b, err := m.currentBackend()
	if err != nil {
		return InitialEPSConfig{}, err
	}
	if cfg.ProfileID < 0 {
		return InitialEPSConfig{}, errors.New("setting initial EPS settings: profile ID is negative")
	}
	if cfg.IPFamily != 0 && cfg.IPFamily&^IPFamilyIPv4v6 != 0 {
		return InitialEPSConfig{}, fmt.Errorf("setting initial EPS settings: IP family %#x is invalid", cfg.IPFamily)
	}
	return b.SetInitialEPSSettings(ctx, cfg)
}

func (m *Modem) CellInfo(ctx context.Context) ([]CellInfo, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.CellInfo(ctx)
}

func (m *Modem) Signal(ctx context.Context) (Signal, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Signal{}, err
	}
	return b.Signal(ctx)
}

func (m *Modem) SetSignalThresholds(ctx context.Context, thresholds SignalThresholds) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetSignalThresholds(ctx, thresholds)
}

func (m *Modem) WatchNetwork(ctx context.Context) (<-chan Result[NetworkStatus], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchNetwork)
}

func (m *Modem) WatchSignal(ctx context.Context) (<-chan Result[Signal], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchSignal)
}

func (m *Modem) Profiles(ctx context.Context) ([]Profile, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.Profiles(ctx)
}

func (m *Modem) CreateProfile(ctx context.Context, cfg ProfileConfig) (Profile, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Profile{}, err
	}
	if cfg.APNType&^APNTypeAny != 0 {
		return Profile{}, fmt.Errorf("creating profile: APN type %#x is invalid", cfg.APNType)
	}
	return b.CreateProfile(ctx, cfg)
}

func (m *Modem) UpdateProfile(ctx context.Context, update ProfileUpdate) (Profile, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Profile{}, err
	}
	if update.ID < 0 {
		return Profile{}, errors.New("updating profile: profile ID is negative")
	}
	if update.APNType != nil && *update.APNType&^APNTypeAny != 0 {
		return Profile{}, fmt.Errorf("updating profile: APN type %#x is invalid", *update.APNType)
	}
	return b.UpdateProfile(ctx, update)
}

func (m *Modem) DeleteProfile(ctx context.Context, id int32) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	if id < 0 {
		return errors.New("deleting profile: profile ID is negative")
	}
	return b.DeleteProfile(ctx, id)
}

func (m *Modem) WatchProfiles(ctx context.Context) (<-chan Result[[]Profile], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchProfiles)
}

func (m *Modem) Connect(ctx context.Context, cfg ConnectConfig) (*Bearer, error) {
	if err := validateConnectConfig(cfg); err != nil {
		return nil, err
	}
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	interfaceName, err := selectNetworkInterface(ctx, m.device, cfg.Interface)
	if err != nil {
		return nil, err
	}
	cfg.Interface = interfaceName
	if cfg.PIN != "" {
		info, err := b.SIMInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("connecting modem: reading SIM: %w", err)
		}
		if info.State == SIMStateLocked {
			if err := b.SendPIN(ctx, cfg.PIN); err != nil {
				return nil, fmt.Errorf("connecting modem: unlocking SIM: %w", err)
			}
			if _, err := pollUntil(ctx, connectPollInterval, b.SIMInfo, func(info SIMInfo) bool {
				return info.State == SIMStateReady
			}); err != nil {
				return nil, fmt.Errorf("connecting modem: waiting for SIM readiness: %w", err)
			}
		}
	}
	power, err := b.PowerState(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting modem: reading power state: %w", err)
	}
	if power != PowerStateOn {
		if err := b.SetPowerState(ctx, PowerStateOn); err != nil {
			return nil, fmt.Errorf("connecting modem: powering modem: %w", err)
		}
		if _, err := pollUntil(ctx, connectPollInterval, b.PowerState, func(state PowerState) bool {
			return state == PowerStateOn
		}); err != nil {
			return nil, fmt.Errorf("connecting modem: waiting for modem power: %w", err)
		}
	}
	network, err := b.NetworkStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting modem: reading network: %w", err)
	}
	registered := network.Registration == RegistrationHome || network.Registration == RegistrationRoaming
	wrongOperator := cfg.OperatorID != "" && network.OperatorID != cfg.OperatorID
	if !registered || wrongOperator {
		if err := b.Register(ctx, RegisterConfig{OperatorID: cfg.OperatorID}); err != nil {
			return nil, fmt.Errorf("connecting modem: registering network: %w", err)
		}
		network, err = pollUntil(ctx, connectPollInterval, b.NetworkStatus, func(status NetworkStatus) bool {
			registered := status.Registration == RegistrationHome || status.Registration == RegistrationRoaming
			return registered && (cfg.OperatorID == "" || status.OperatorID == cfg.OperatorID)
		})
		if err != nil {
			return nil, fmt.Errorf("connecting modem: waiting for network registration: %w", err)
		}
	}
	if network.PacketService != PacketServiceAttached {
		if err := b.SetPacketServiceState(ctx, PacketServiceAttached); err != nil {
			return nil, fmt.Errorf("connecting modem: attaching packet service: %w", err)
		}
		if _, err := pollUntil(ctx, connectPollInterval, b.NetworkStatus, func(status NetworkStatus) bool {
			return status.PacketService == PacketServiceAttached
		}); err != nil {
			return nil, fmt.Errorf("connecting modem: waiting for packet service attachment: %w", err)
		}
	}

	session, err := b.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = session.Close()
		return nil, ErrClosed
	}
	m.nextID++
	bearer := &Bearer{id: m.nextID, modem: m, session: session, done: make(chan struct{})}
	m.bearers[bearer.id] = bearer
	m.mu.Unlock()
	return bearer, nil
}

func pollUntil[T any](ctx context.Context, interval time.Duration, query func(context.Context) (T, error), ready func(T) bool) (T, error) {
	var zero T
	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, err := query(ctx)
		if err != nil {
			return value, err
		}
		if err := ctx.Err(); err != nil {
			return value, err
		}
		if ready(value) {
			return value, nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return value, ctx.Err()
		}
	}
}

func validateConnectConfig(cfg ConnectConfig) error {
	if cfg.ProfileID < 0 {
		return errors.New("connecting modem: profile ID is negative")
	}
	if cfg.ProfileID != 0 && (cfg.APN != "" || cfg.Username != "" || cfg.Password != "" || cfg.Authentication != 0) {
		return errors.New("connecting modem: profile ID and inline APN settings are mutually exclusive")
	}
	if cfg.IPFamily != 0 && cfg.IPFamily&^IPFamilyIPv4v6 != 0 {
		return fmt.Errorf("connecting modem: IP family %#x is invalid", cfg.IPFamily)
	}
	return nil
}

func (m *Modem) DisconnectAll(ctx context.Context) error {
	m.mu.RLock()
	bearers := make([]*Bearer, 0, len(m.bearers))
	for _, bearer := range m.bearers {
		bearers = append(bearers, bearer)
	}
	m.mu.RUnlock()
	var errs []error
	for _, bearer := range bearers {
		errs = append(errs, bearer.Disconnect(ctx))
	}
	return errors.Join(errs...)
}

func (m *Modem) Bearers() []*Bearer {
	m.mu.RLock()
	bearers := make([]*Bearer, 0, len(m.bearers))
	for _, bearer := range m.bearers {
		bearers = append(bearers, bearer)
	}
	m.mu.RUnlock()
	slices.SortFunc(bearers, func(a, b *Bearer) int {
		return cmp.Compare(a.Info().ID, b.Info().ID)
	})
	return bearers
}

func (m *Modem) Bearer(id uint64) (*Bearer, bool) {
	m.mu.RLock()
	bearer, ok := m.bearers[id]
	m.mu.RUnlock()
	return bearer, ok
}

func (m *Modem) removeBearer(id uint64) {
	m.mu.Lock()
	if m.bearers != nil {
		delete(m.bearers, id)
	}
	m.mu.Unlock()
}

func (m *Modem) ListMessages(ctx context.Context) ([]Message, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return b.ListMessages(ctx)
}

func (m *Modem) MessageStorages(ctx context.Context) (MessageStorageInfo, error) {
	b, err := m.currentBackend()
	if err != nil {
		return MessageStorageInfo{}, err
	}
	return b.MessageStorages(ctx)
}

func (m *Modem) ReadMessage(ctx context.Context, id uint32) (Message, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Message{}, err
	}
	return b.ReadMessage(ctx, id)
}

func (m *Modem) ReadStoredMessage(ctx context.Context, ref MessageRef) (Message, error) {
	b, err := m.currentBackend()
	if err != nil {
		return Message{}, err
	}
	return b.ReadStoredMessage(ctx, ref)
}

func (m *Modem) SendMessage(ctx context.Context, cfg MessageConfig) (SendResult, error) {
	b, err := m.currentBackend()
	if err != nil {
		return SendResult{}, err
	}
	if err := validateMessageConfig(cfg); err != nil {
		return SendResult{}, err
	}
	return b.SendMessage(ctx, cfg)
}

func (m *Modem) StoreMessage(ctx context.Context, cfg MessageConfig) ([]Message, error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	if err := validateMessageConfig(cfg); err != nil {
		return nil, err
	}
	return b.StoreMessage(ctx, cfg)
}

func validateMessageConfig(cfg MessageConfig) error {
	if cfg.Number == "" {
		return errors.New("preparing SMS: destination number is empty")
	}
	if cfg.Text != "" && len(cfg.Data) != 0 {
		return errors.New("preparing SMS: text and binary data are mutually exclusive")
	}
	if cfg.Text == "" && len(cfg.Data) == 0 {
		return errors.New("preparing SMS: text and binary data are empty")
	}
	return nil
}

func (m *Modem) DeleteMessage(ctx context.Context, id uint32) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.DeleteMessage(ctx, id)
}

func (m *Modem) DeleteStoredMessage(ctx context.Context, ref MessageRef) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.DeleteStoredMessage(ctx, ref)
}

func (m *Modem) DeleteMessages(ctx context.Context, refs []MessageRef) error {
	var errs []error
	for _, ref := range refs {
		if err := m.DeleteStoredMessage(ctx, ref); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Modem) SendPDU(ctx context.Context, pdu []byte) (uint32, error) {
	b, err := m.currentBackend()
	if err != nil {
		return 0, err
	}
	if len(pdu) == 0 {
		return 0, errors.New("sending SMS PDU: PDU is empty")
	}
	return b.SendPDU(ctx, pdu)
}

func (m *Modem) WatchMessages(ctx context.Context) (<-chan Result[Message], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchMessages)
}

func (m *Modem) InitiateUSSD(ctx context.Context, text string) (USSDMessage, error) {
	b, err := m.currentBackend()
	if err != nil {
		return USSDMessage{}, err
	}
	if text == "" {
		return USSDMessage{}, errors.New("initiating USSD: text is empty")
	}
	return b.InitiateUSSD(ctx, text)
}

func (m *Modem) RespondUSSD(ctx context.Context, text string) (USSDMessage, error) {
	b, err := m.currentBackend()
	if err != nil {
		return USSDMessage{}, err
	}
	if text == "" {
		return USSDMessage{}, errors.New("responding to USSD: text is empty")
	}
	return b.RespondUSSD(ctx, text)
}

func (m *Modem) CancelUSSD(ctx context.Context) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.CancelUSSD(ctx)
}

func (m *Modem) WatchUSSD(ctx context.Context) (<-chan Result[USSDMessage], error) {
	b, err := m.currentBackend()
	if err != nil {
		return nil, err
	}
	return startModemStream(ctx, m.done, b.WatchUSSD)
}

func (m *Modem) SAR(ctx context.Context) (SARState, error) {
	b, err := m.currentBackend()
	if err != nil {
		return SARState{}, err
	}
	return b.SAR(ctx)
}

func (m *Modem) SetSAR(ctx context.Context, state SARState) error {
	b, err := m.currentBackend()
	if err != nil {
		return err
	}
	return b.SetSAR(ctx, state)
}

func (m *Modem) FirmwareUpdateInfo(ctx context.Context) (FirmwareUpdateInfo, error) {
	b, err := m.currentBackend()
	if err != nil {
		return FirmwareUpdateInfo{}, err
	}
	return b.FirmwareUpdateInfo(ctx)
}

func startModemStream[T any](
	ctx context.Context,
	modemDone <-chan struct{},
	start func(context.Context) (<-chan Result[T], error),
) (<-chan Result[T], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	stream, err := start(watchCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan Result[T], 16)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-modemDone:
				return
			case result, ok := <-stream:
				if !ok {
					return
				}
				select {
				case out <- result:
				case <-ctx.Done():
					return
				case <-modemDone:
					return
				}
			}
		}
	}()
	return out, nil
}

// Bearer owns one modem-side packet data session.
type Bearer struct {
	mu       sync.Mutex
	id       uint64
	modem    *Modem
	session  sessionBackend
	closed   bool
	done     chan struct{}
	doneOnce sync.Once
}

func (b *Bearer) Info() BearerInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session == nil {
		return BearerInfo{ID: b.id}
	}
	info := b.session.Info()
	info.ID = b.id
	return info
}

func (b *Bearer) Stats(ctx context.Context) (BearerStats, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.session == nil {
		return BearerStats{}, ErrClosed
	}
	return b.session.Stats(ctx)
}

func (b *Bearer) Watch(ctx context.Context) (<-chan Result[BearerEvent], error) {
	b.mu.Lock()
	if b.closed || b.session == nil {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	watchCtx, cancel := context.WithCancel(ctx)
	stream, err := b.session.Watch(watchCtx)
	done := b.done
	b.mu.Unlock()
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan Result[BearerEvent], 16)
	go func() {
		defer cancel()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case result, ok := <-stream:
				if !ok {
					return
				}
				result.Value.Info.ID = b.id
				select {
				case out <- result:
				case <-ctx.Done():
					return
				case <-done:
					return
				}
			}
		}
	}()
	return out, nil
}

func (b *Bearer) Disconnect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.session == nil {
		return nil
	}
	if err := b.session.Disconnect(ctx); err != nil {
		return err
	}
	b.closed = true
	b.closeDone()
	b.modem.removeBearer(b.id)
	return nil
}

func (b *Bearer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	disconnectErr := b.Disconnect(ctx)
	if disconnectErr == nil {
		return nil
	}
	return errors.Join(disconnectErr, b.closeOwned())
}

func (b *Bearer) closeOwned() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.session == nil {
		return nil
	}
	err := b.session.Close()
	b.closed = true
	b.closeDone()
	if b.modem != nil {
		b.modem.removeBearer(b.id)
	}
	return err
}

func (b *Bearer) closeDone() {
	b.doneOnce.Do(func() {
		if b.done != nil {
			close(b.done)
		}
	})
}
