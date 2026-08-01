package mbim

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	mbimproto "github.com/damonto/wwan-go/mbim"
	wwansim "github.com/damonto/wwan-go/sim"
	simcard "github.com/damonto/wwan-go/sim/card"
)

type Backend struct {
	client              *mbimproto.Client
	device              string
	mu                  sync.Mutex
	slots               map[uint32]struct{}
	sessionsPrepared    bool
	notificationMu      sync.Mutex
	notificationEntries []mbimproto.DeviceServiceSubscribeEntry
	metadataMu          sync.Mutex
	metadataKey         string
	metadata            SIMInfo
}

func New(client *mbimproto.Client, device string) *Backend {
	return &Backend{client: client, device: device}
}

func (b *Backend) Close() error { return b.client.Close() }

func (b *Backend) Info(ctx context.Context) (Info, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading MBIM device capabilities: %w", err)
	}
	return Info{
		Revision:         caps.FirmwareInfo,
		HardwareRevision: caps.HardwareInfo,
		EquipmentID:      caps.DeviceID,
		DeviceID:         caps.DeviceID,
	}, nil
}

func (b *Backend) Capabilities(ctx context.Context) (Capabilities, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return Capabilities{}, fmt.Errorf("reading MBIM device capabilities: %w", err)
	}
	services, err := b.client.DeviceServices(ctx)
	if err != nil {
		return Capabilities{}, fmt.Errorf("reading MBIM device services: %w", err)
	}
	features := mbimFeatures(services)
	pduSMS := mbimproto.SMSCapsPDUReceive | mbimproto.SMSCapsPDUSend
	if caps.SMSCaps&pduSMS != pduSMS {
		features &^= FeatureSMS
	}
	result := Capabilities{
		SupportedTechnologies: mbimDataClass(caps.DataClass),
		CurrentTechnologies:   mbimDataClass(caps.DataClass),
		SupportedIPFamilies:   IPFamilyIPv4v6,
		Features:              features,
		MaxBearers:            caps.MaxSessions,
		MaxActiveBearers:      caps.MaxSessions,
		MaxSIMSlots:           1,
	}
	if features&FeatureMultiSIM != 0 {
		system, systemErr := b.client.SystemCapabilities(ctx)
		if systemErr != nil {
			return Capabilities{}, fmt.Errorf("reading MBIM SIM slot capabilities: %w", systemErr)
		}
		if system.Slots > 0 {
			result.MaxSIMSlots = uint8(min(system.Slots, math.MaxUint8))
		}
		if result.MaxSIMSlots < 2 {
			result.Features &^= FeatureMultiSIM
		}
	}
	return result, nil
}

func mbimFeatures(services mbimproto.DeviceServicesResponse) Feature {
	supports := services.SupportsCID
	var features Feature
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDProvisionedContexts) {
		features |= FeatureProfileManagement
	}
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDSignalState) {
		features |= FeatureSignalThresholds
	}
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDPin) &&
		supports(mbimproto.ServiceBasicConnect, mbimproto.CIDPinList) {
		features |= FeatureFacilityLocks
	}
	if supports(mbimproto.ServiceSMS, mbimproto.CIDSMSRead) &&
		supports(mbimproto.ServiceSMS, mbimproto.CIDSMSSend) &&
		supports(mbimproto.ServiceSMS, mbimproto.CIDSMSDelete) {
		features |= FeatureSMS
	}
	if supports(mbimproto.ServiceUSSD, mbimproto.CIDUSSD) {
		features |= FeatureUSSD
	}
	if supports(mbimproto.ServiceMsSAR, mbimproto.CIDMsSARConfig) {
		features |= FeatureSAR
	}
	if supports(mbimproto.ServiceMsFirmwareID, mbimproto.CIDMsFirmwareIDGet) {
		features |= FeatureFirmwareUpdate
	}
	if supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDMsBaseStationsInfo) {
		features |= FeatureCellInfo
	}
	if supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDMsLteAttachConfiguration) &&
		supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDMsLteAttachInfo) {
		features |= FeatureInitialEPSBearer
	}
	if supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDMsSystemCapabilities) &&
		supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDDeviceSlotMappings) &&
		supports(mbimproto.ServiceMsBasicConnectExtensions, mbimproto.CIDMsSlotInfoStatus) {
		features |= FeatureMultiSIM
	}
	return features
}

func mbimDataClass(class mbimproto.DataClass) Technology {
	var result Technology
	if class&mbimproto.DataClassGPRS != 0 || class&mbimproto.DataClassEDGE != 0 {
		result |= TechnologyGSM
	}
	if class&mbimproto.DataClassUMTS != 0 || class&mbimproto.DataClassHSDPA != 0 || class&mbimproto.DataClassHSUPA != 0 {
		result |= TechnologyUMTS
	}
	if class&mbimproto.DataClassLTE != 0 {
		result |= TechnologyLTE
	}
	if class&(mbimproto.DataClass5GNSA|mbimproto.DataClass5GSA) != 0 {
		result |= TechnologyNR5GNSA | TechnologyNR5GSA
	}
	return result
}

func (b *Backend) SetPowerState(ctx context.Context, state PowerState) error {
	var radio mbimproto.RadioSwitchState
	switch state {
	case PowerStateOff, PowerStateLow:
		radio = mbimproto.RadioSwitchStateOff
	case PowerStateOn:
		radio = mbimproto.RadioSwitchStateOn
	default:
		return fmt.Errorf("setting MBIM power state: state %d is invalid", state)
	}
	if _, err := b.client.SetRadioState(ctx, radio); err != nil {
		return fmt.Errorf("setting MBIM power state: %w", err)
	}
	return nil
}

func (b *Backend) PowerState(ctx context.Context) (PowerState, error) {
	radio, err := b.client.RadioState(ctx)
	if err != nil {
		return PowerStateUnknown, fmt.Errorf("reading MBIM power state: %w", err)
	}
	return mbimPowerState(radio), nil
}

func (b *Backend) Reset(ctx context.Context) error {
	if err := b.client.ResetDevice(ctx); err != nil {
		return fmt.Errorf("resetting MBIM modem: %w", err)
	}
	return nil
}

func (b *Backend) Modes(ctx context.Context) ([]Mode, Mode, error) {
	caps, err := b.Capabilities(ctx)
	if err != nil {
		return nil, Mode{}, err
	}
	registration, err := b.client.RegistrationState(ctx)
	if err != nil {
		return nil, Mode{}, fmt.Errorf("reading MBIM mode preference: %w", err)
	}
	current := Mode{Allowed: mbimDataClass(registration.PreferredDataClasses)}
	if current.Allowed == 0 {
		current.Allowed = caps.CurrentTechnologies
	}
	return []Mode{{Allowed: caps.SupportedTechnologies}}, current, nil
}

func (b *Backend) SetModes(ctx context.Context, mode Mode) error {
	if mode.Allowed == 0 || mode.Allowed&^TechnologyAny != 0 {
		return fmt.Errorf("setting MBIM modes: allowed technologies %#x are invalid", mode.Allowed)
	}
	if mode.Preferred&^mode.Allowed != 0 {
		return errors.New("setting MBIM modes: preferred technologies are not a subset of allowed technologies")
	}
	if _, err := b.client.SetRegistrationState(ctx, "", mbimproto.RegisterActionAutomatic, technologyToMBIMDataClass(mode.Allowed)); err != nil {
		return fmt.Errorf("setting MBIM modes: %w", err)
	}
	return nil
}

func (b *Backend) SetCapabilities(context.Context, Technology) error {
	return fmt.Errorf("setting MBIM capabilities: %w", ErrNotSupported)
}

func (b *Backend) Status(ctx context.Context) (Status, error) {
	radio, err := b.client.RadioState(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading MBIM radio state: %w", err)
	}
	ready, err := b.client.SubscriberReadyStatus(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading MBIM subscriber status: %w", err)
	}
	registration, err := b.client.RegistrationState(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading MBIM registration state: %w", err)
	}
	packet, err := b.client.PacketService(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading MBIM packet service: %w", err)
	}
	signalState, err := b.client.SignalState(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading MBIM signal: %w", err)
	}
	network := networkStatusFromMBIM(registration, packet)
	signal := signalFromMBIM(signalState)
	return Status{
		Power:         mbimPowerState(radio),
		SIM:           mbimSIMState(ready.ReadyState),
		Registration:  network.Registration,
		PacketService: network.PacketService,
		Technology:    network.Technology,
		OperatorID:    network.OperatorID,
		OperatorName:  network.OperatorName,
		SignalQuality: signal.Quality,
	}, nil
}

func (b *Backend) SIMInfo(ctx context.Context) (SIMInfo, error) {
	ready, err := b.client.SubscriberReadyStatus(ctx)
	if err != nil {
		return SIMInfo{}, fmt.Errorf("reading MBIM subscriber status: %w", err)
	}
	result := simInfoFromSubscriber(ready)
	if pin, pinErr := b.client.PIN(ctx); pinErr == nil {
		result.PINRetries = uint8(min(pin.RemainingAttempts, math.MaxUint8))
		if pin.State == mbimproto.PinStateLocked {
			result.State = SIMStateLocked
		}
	}
	metadata := b.simMetadata(ctx, result.ICCID)
	result.OperatorID = metadata.OperatorID
	result.OperatorName = metadata.OperatorName
	result.GID1 = metadata.GID1
	result.SPN = metadata.SPN
	result.ATR = slices.Clone(metadata.ATR)
	return result, nil
}

func simInfoFromSubscriber(ready mbimproto.SubscriberReadyStatusResponse) SIMInfo {
	result := SIMInfo{
		State:      mbimSIMState(ready.ReadyState),
		Slot:       uint8(ready.SlotID + 1),
		ICCID:      ready.SIMICCID,
		IMSI:       ready.SubscriberID,
		OwnNumbers: append([]string(nil), ready.TelephoneNumbers...),
	}
	if result.Slot == 0 {
		result.Slot = 1
	}
	return result
}

type nonClosingSIMReader struct{ simcard.Reader }

func (nonClosingSIMReader) Close() error { return nil }

func (b *Backend) simMetadata(ctx context.Context, iccid string) SIMInfo {
	b.metadataMu.Lock()
	defer b.metadataMu.Unlock()
	if iccid != "" && b.metadataKey == iccid {
		return b.metadata
	}
	metadata := SIMInfo{}
	if atr, err := b.client.QueryUiccATR(ctx); err == nil {
		metadata.ATR = atr
	}
	reader, err := wwansim.NewMBIM(b.client)
	if err == nil {
		card, cardErr := wwansim.New(ctx, nonClosingSIMReader{Reader: reader}, nil)
		if cardErr == nil {
			metadata.OperatorID = card.MCC() + card.MNC()
			metadata.OperatorName = card.SPN()
			metadata.GID1 = card.GID1()
			metadata.SPN = card.SPN()
			_ = card.Close()
		}
	}
	b.metadataKey = iccid
	b.metadata = metadata
	return metadata
}

func (b *Backend) SIMSlots(ctx context.Context) ([]SIMSlot, error) {
	capabilities, err := b.client.SystemCapabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM SIM slot capabilities: %w", err)
	}
	mappings, err := b.client.DeviceSlotMappings(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM SIM slot mappings: %w", err)
	}
	active := uint32(math.MaxUint32)
	if len(mappings) > 0 {
		active = mappings[0].Slot
	}
	slots := make([]SIMSlot, capabilities.Slots)
	for i := range capabilities.Slots {
		status, err := b.client.SlotInfoStatus(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("reading MBIM SIM slot %d: %w", i+1, err)
		}
		slots[i] = SIMSlot{Index: uint8(i + 1), Active: i == active, State: mbimSlotSIMState(status.State)}
	}
	if sim, simErr := b.SIMInfo(ctx); simErr == nil {
		populateActiveMBIMSIMSlot(slots, sim)
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return slots, nil
}

func populateActiveMBIMSIMSlot(slots []SIMSlot, sim SIMInfo) {
	for i := range slots {
		if !slots[i].Active {
			continue
		}
		slots[i].ICCID = sim.ICCID
		slots[i].EID = sim.EID
		return
	}
}

func (b *Backend) SetPrimarySIMSlot(ctx context.Context, slot uint8) error {
	if _, err := b.client.SetDeviceSlotMappings(ctx, []mbimproto.SlotMapping{{Slot: uint32(slot - 1)}}); err != nil {
		return fmt.Errorf("setting MBIM primary SIM slot: %w", err)
	}
	return nil
}

func (b *Backend) SendPIN(ctx context.Context, pin string) error {
	if _, err := b.client.SetPIN(ctx, mbimproto.PinTypePIN1, mbimproto.PinOperationEnter, pin, ""); err != nil {
		return fmt.Errorf("sending MBIM PIN: %w", err)
	}
	return nil
}

func (b *Backend) SendPUK(ctx context.Context, puk, newPIN string) error {
	if _, err := b.client.SetPIN(ctx, mbimproto.PinTypePUK1, mbimproto.PinOperationEnter, puk, newPIN); err != nil {
		return fmt.Errorf("sending MBIM PUK: %w", err)
	}
	return nil
}

func (b *Backend) EnablePIN(ctx context.Context, pin string, enabled bool) error {
	operation := mbimproto.PinOperationDisable
	if enabled {
		operation = mbimproto.PinOperationEnable
	}
	if _, err := b.client.SetPIN(ctx, mbimproto.PinTypePIN1, operation, pin, ""); err != nil {
		return fmt.Errorf("setting MBIM PIN protection: %w", err)
	}
	return nil
}

func (b *Backend) ChangePIN(ctx context.Context, oldPIN, newPIN string) error {
	if _, err := b.client.SetPIN(ctx, mbimproto.PinTypePIN1, mbimproto.PinOperationChange, oldPIN, newPIN); err != nil {
		return fmt.Errorf("changing MBIM PIN: %w", err)
	}
	return nil
}

func (b *Backend) PreferredNetworks(ctx context.Context) ([]PreferredNetwork, error) {
	providers, err := b.client.PreferredProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM preferred networks: %w", err)
	}
	result := make([]PreferredNetwork, len(providers))
	for i, provider := range providers {
		result[i] = PreferredNetwork{OperatorID: provider.ID, Technology: mbimCellularClass(provider.CellularClass)}
	}
	return result, nil
}

func (b *Backend) SetPreferredNetworks(ctx context.Context, networks []PreferredNetwork) error {
	providers := make([]mbimproto.Provider, len(networks))
	for i, network := range networks {
		providers[i] = mbimproto.Provider{ID: network.OperatorID, State: mbimproto.ProviderStatePreferred, CellularClass: technologyToMBIMCellularClass(network.Technology), RSSI: 99, ErrorRate: 99}
	}
	if _, err := b.client.SetPreferredProviders(ctx, providers); err != nil {
		return fmt.Errorf("setting MBIM preferred networks: %w", err)
	}
	return nil
}

func (b *Backend) NetworkStatus(ctx context.Context) (NetworkStatus, error) {
	registration, err := b.client.RegistrationState(ctx)
	if err != nil {
		return NetworkStatus{}, fmt.Errorf("reading MBIM registration state: %w", err)
	}
	packet, err := b.client.PacketService(ctx)
	if err != nil {
		return NetworkStatus{}, fmt.Errorf("reading MBIM packet service: %w", err)
	}
	result := networkStatusFromMBIM(registration, packet)
	if location, locationErr := b.client.LocationInfoStatus(ctx); locationErr == nil {
		result.LocationAreaCode = location.LocationAreaCode
		result.TrackingAreaCode = location.TrackingAreaCode
		result.CellID = uint64(location.CellID)
	}
	return result, nil
}

func networkStatusFromMBIM(registration mbimproto.RegistrationStateInfo, packet mbimproto.PacketServiceInfo) NetworkStatus {
	result := NetworkStatus{}
	applyMBIMRegistration(&result, registration)
	applyMBIMPacketService(&result, packet)
	return result
}

func applyMBIMRegistration(result *NetworkStatus, registration mbimproto.RegistrationStateInfo) {
	result.Registration = mbimRegistrationState(registration.RegisterState)
	result.Available = mbimDataClass(mbimproto.DataClass(registration.AvailableDataClasses))
	result.OperatorID = registration.ProviderID
	result.OperatorName = registration.ProviderName
	result.RoamingText = registration.RoamingText
}

func applyMBIMPacketService(result *NetworkStatus, packet mbimproto.PacketServiceInfo) {
	result.PacketService = PacketServiceState(packet.PacketServiceState)
	result.Technology = mbimDataClass(packet.CurrentDataClass)
	result.UplinkBitsPerSecond = packet.UplinkSpeed
	result.DownlinkBitsPerSecond = packet.DownlinkSpeed
}

func (b *Backend) Register(ctx context.Context, cfg RegisterConfig) error {
	action := mbimproto.RegisterActionAutomatic
	if cfg.OperatorID != "" {
		action = mbimproto.RegisterActionManual
	}
	if _, err := b.client.SetRegistrationState(ctx, cfg.OperatorID, action, technologyToMBIMDataClass(cfg.Technology)); err != nil {
		return fmt.Errorf("registering MBIM network: %w", err)
	}
	return nil
}

func (b *Backend) ScanNetworks(ctx context.Context) ([]Operator, error) {
	providers, err := b.client.VisibleProviders(ctx, mbimproto.VisibleProvidersActionFullScan)
	if err != nil {
		return nil, fmt.Errorf("scanning MBIM networks: %w", err)
	}
	result := make([]Operator, len(providers))
	for i, provider := range providers {
		result[i] = Operator{
			ID:         provider.ID,
			Name:       provider.Name,
			Technology: mbimCellularClass(provider.CellularClass),
			Available:  provider.State&mbimproto.ProviderStateVisible != 0,
			Current:    provider.State&mbimproto.ProviderStateRegistered != 0,
			Forbidden:  provider.State&mbimproto.ProviderStateForbidden != 0,
		}
	}
	return result, nil
}

func (b *Backend) SetPacketServiceState(ctx context.Context, state PacketServiceState) error {
	action := mbimproto.PacketServiceActionAttach
	if state == PacketServiceDetached {
		action = mbimproto.PacketServiceActionDetach
	}
	if _, err := b.client.SetPacketService(ctx, action); err != nil {
		return fmt.Errorf("setting MBIM packet service: %w", err)
	}
	return nil
}

func (b *Backend) FacilityLocks(ctx context.Context) ([]FacilityLock, error) {
	list, err := b.client.PINList(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM facility locks: %w", err)
	}
	active, activeErr := b.client.PIN(ctx)
	result := []FacilityLock{
		mbimFacilityLock(FacilityNetwork, list.Network),
		mbimFacilityLock(FacilityNetworkSubset, list.NetworkSubset),
		mbimFacilityLock(FacilityServiceProvider, list.ServiceProvider),
		mbimFacilityLock(FacilityCorporate, list.Corporate),
	}
	if activeErr == nil {
		for i := range result {
			facility, blocked := mbimPINFacility(active.Type)
			if result[i].Facility != facility {
				continue
			}
			result[i].Enabled = result[i].Enabled || active.State == mbimproto.PinStateLocked
			result[i].Blocked = blocked && active.State == mbimproto.PinStateLocked
			if blocked {
				result[i].UnblockRetries = active.RemainingAttempts
			} else {
				result[i].VerifyRetries = active.RemainingAttempts
			}
		}
	}
	return result, nil
}

func (b *Backend) SetFacilityLock(ctx context.Context, facility Facility, enabled bool, key string) error {
	operation := mbimproto.PinOperationDisable
	if enabled {
		operation = mbimproto.PinOperationEnable
	}
	if _, err := b.client.SetPIN(ctx, facilityToMBIMPIN(facility), operation, key, ""); err != nil {
		return fmt.Errorf("setting MBIM facility lock: %w", err)
	}
	return nil
}

func (b *Backend) UnblockFacilityLock(ctx context.Context, facility Facility, key string) error {
	if _, err := b.client.SetPIN(ctx, facilityToMBIMPUK(facility), mbimproto.PinOperationEnter, key, ""); err != nil {
		return fmt.Errorf("unblocking MBIM facility lock: %w", err)
	}
	return nil
}

func (b *Backend) InitialEPSBearer(ctx context.Context) (InitialEPSConfig, error) {
	info, err := b.client.LTEAttachInfo(ctx)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading MBIM initial EPS bearer: %w", err)
	}
	return InitialEPSConfig{
		APN:            info.AccessString,
		IPFamily:       mbimIPFamily(info.IPType),
		Authentication: mbimAuthentication(info.AuthProtocol),
		Username:       info.UserName,
		Password:       info.Password,
	}, nil
}

func (b *Backend) InitialEPSSettings(ctx context.Context) (InitialEPSConfig, error) {
	configurations, err := b.client.LTEAttachConfigurations(ctx)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading MBIM initial EPS settings: %w", err)
	}
	for _, current := range configurations {
		if current.Roaming == mbimproto.LTEAttachRoamingControlHome {
			return initialEPSFromMBIM(current), nil
		}
	}
	return InitialEPSConfig{}, fmt.Errorf("reading MBIM initial EPS settings: home configuration: %w", ErrNotSupported)
}

func (b *Backend) SetInitialEPSSettings(ctx context.Context, cfg InitialEPSConfig) (InitialEPSConfig, error) {
	configurations, err := b.client.LTEAttachConfigurations(ctx)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading MBIM initial EPS settings: %w", err)
	}
	home := mbimproto.LTEAttachConfiguration{
		IPType:       ipFamilyToMBIM(cfg.IPFamily),
		Roaming:      mbimproto.LTEAttachRoamingControlHome,
		Source:       mbimproto.ContextSourceUser,
		AccessString: cfg.APN,
		UserName:     cfg.Username,
		Password:     cfg.Password,
		AuthProtocol: authenticationToMBIM(cfg.Authentication),
	}
	found := false
	for i := range configurations {
		if configurations[i].Roaming == mbimproto.LTEAttachRoamingControlHome {
			configurations[i] = home
			found = true
			break
		}
	}
	if !found {
		configurations = append(configurations, home)
	}
	updated, err := b.client.SetLTEAttachConfigurations(ctx, mbimproto.LTEAttachContextOperationDefault, configurations)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("setting MBIM initial EPS settings: %w", err)
	}
	for _, current := range updated {
		if current.Roaming == mbimproto.LTEAttachRoamingControlHome {
			return initialEPSFromMBIM(current), nil
		}
	}
	return InitialEPSConfig{}, errors.New("setting MBIM initial EPS settings: home configuration missing from response")
}

func mbimFacilityLock(facility Facility, desc mbimproto.PinDesc) FacilityLock {
	return FacilityLock{Facility: facility, Enabled: desc.Mode == mbimproto.PinModeEnabled}
}

func facilityToMBIMPIN(facility Facility) mbimproto.PinType {
	switch facility {
	case FacilityNetworkSubset:
		return mbimproto.PinTypeNetworkSubset
	case FacilityServiceProvider:
		return mbimproto.PinTypeServiceProvider
	case FacilityCorporate:
		return mbimproto.PinTypeCorporate
	default:
		return mbimproto.PinTypeNetwork
	}
}

func facilityToMBIMPUK(facility Facility) mbimproto.PinType {
	switch facility {
	case FacilityNetworkSubset:
		return mbimproto.PinTypeNetworkSubsetPUK
	case FacilityServiceProvider:
		return mbimproto.PinTypeServiceProviderPUK
	case FacilityCorporate:
		return mbimproto.PinTypeCorporatePUK
	default:
		return mbimproto.PinTypeNetworkPUK
	}
}

func mbimPINFacility(pin mbimproto.PinType) (Facility, bool) {
	switch pin {
	case mbimproto.PinTypeNetwork, mbimproto.PinTypeNetworkPUK:
		return FacilityNetwork, pin == mbimproto.PinTypeNetworkPUK
	case mbimproto.PinTypeNetworkSubset, mbimproto.PinTypeNetworkSubsetPUK:
		return FacilityNetworkSubset, pin == mbimproto.PinTypeNetworkSubsetPUK
	case mbimproto.PinTypeServiceProvider, mbimproto.PinTypeServiceProviderPUK:
		return FacilityServiceProvider, pin == mbimproto.PinTypeServiceProviderPUK
	case mbimproto.PinTypeCorporate, mbimproto.PinTypeCorporatePUK:
		return FacilityCorporate, pin == mbimproto.PinTypeCorporatePUK
	default:
		return 0, false
	}
}

func initialEPSFromMBIM(value mbimproto.LTEAttachConfiguration) InitialEPSConfig {
	return InitialEPSConfig{
		APN:            value.AccessString,
		IPFamily:       mbimIPFamily(value.IPType),
		Authentication: mbimAuthentication(value.AuthProtocol),
		Username:       value.UserName,
		Password:       value.Password,
	}
}

func (b *Backend) Signal(ctx context.Context) (Signal, error) {
	state, err := b.client.SignalState(ctx)
	if err != nil {
		return Signal{}, fmt.Errorf("reading MBIM signal: %w", err)
	}
	return signalFromMBIM(state), nil
}

func signalFromMBIM(state mbimproto.SignalStateInfo) Signal {
	result := Signal{}
	if state.RSSI <= 31 {
		rssi := -113 + float64(state.RSSI)*2
		result.Quality = uint8(math.Round(float64(state.RSSI) * 100 / 31))
		result.Radios = append(result.Radios, RadioSignal{RSSI: knownSignal(rssi)})
	}
	for _, value := range state.RsrpSnr {
		radio := RadioSignal{Technology: mbimDataClass(value.SystemType)}
		if value.RSRP <= 126 {
			radio.RSRP = knownSignal(float64(value.RSRP) - 156)
		}
		if value.SNR <= 128 {
			radio.SNR = knownSignal(float64(value.SNR)/2 - 23)
		}
		result.Radios = append(result.Radios, radio)
	}
	return result
}

func (b *Backend) SetSignalThresholds(ctx context.Context, thresholds SignalThresholds) error {
	seconds := uint32(0)
	if thresholds.Interval > 0 {
		seconds = uint32(max(1, math.Ceil(thresholds.Interval.Seconds())))
	}
	errorRateThreshold := uint32(99)
	if thresholds.ErrorRateThreshold {
		errorRateThreshold = 1
	}
	_, err := b.client.SetSignalState(ctx, mbimproto.SignalStateSet{SignalStrengthInterval: seconds, RSSIThreshold: thresholds.RSSIChangeDB, ErrorRateThreshold: errorRateThreshold})
	if err != nil {
		return fmt.Errorf("setting MBIM signal thresholds: %w", err)
	}
	return nil
}

func (b *Backend) Profiles(ctx context.Context) ([]Profile, error) {
	contexts, err := b.client.ProvisionedContextsV2(ctx)
	if err == nil {
		result := make([]Profile, len(contexts))
		for i, value := range contexts {
			result[i] = mbimProfileV2(value)
		}
		return result, nil
	}
	legacy, legacyErr := b.client.ProvisionedContexts(ctx)
	if legacyErr != nil {
		return nil, fmt.Errorf("reading MBIM profiles: %w", errors.Join(err, legacyErr))
	}
	result := make([]Profile, len(legacy))
	for i, value := range legacy {
		result[i] = mbimProfile(value)
	}
	return result, nil
}

func (b *Backend) CreateProfile(ctx context.Context, cfg ProfileConfig) (Profile, error) {
	value := mbimProfileV2Config(math.MaxUint32, cfg)
	contexts, v2Err := b.client.SetProvisionedContextV2(ctx, mbimproto.ContextOperationDefault, value)
	if v2Err == nil {
		for _, current := range contexts {
			if current.AccessString == cfg.APN && current.UserName == cfg.Username {
				return mbimProfileV2(current), nil
			}
		}
		return Profile{}, errors.New("creating MBIM profile: modem did not return the created V2 profile")
	}

	legacy := mbimproto.ProvisionedContext{ContextID: math.MaxUint32, ContextType: apnTypeToMBIM(cfg.APNType), AccessString: cfg.APN, UserName: cfg.Username, Password: cfg.Password, AuthProtocol: authenticationToMBIM(cfg.Authentication)}
	contextsV1, err := b.client.SetProvisionedContext(ctx, legacy, "")
	if err != nil {
		return Profile{}, fmt.Errorf("creating MBIM profile: %w", errors.Join(v2Err, err))
	}
	for _, current := range contextsV1 {
		if current.AccessString == cfg.APN && current.UserName == cfg.Username {
			return mbimProfile(current), nil
		}
	}
	return Profile{}, errors.New("creating MBIM profile: modem did not return the created profile")
}

func (b *Backend) UpdateProfile(ctx context.Context, update ProfileUpdate) (Profile, error) {
	contextsV2, v2Err := b.client.ProvisionedContextsV2(ctx)
	if v2Err == nil {
		for _, current := range contextsV2 {
			if current.ContextID != uint32(update.ID) {
				continue
			}
			applyMBIMProfileV2Update(&current, update)
			updated, err := b.client.SetProvisionedContextV2(ctx, mbimproto.ContextOperationDefault, current)
			if err != nil {
				return Profile{}, fmt.Errorf("updating MBIM V2 profile: %w", err)
			}
			for _, result := range updated {
				if result.ContextID == uint32(update.ID) {
					return mbimProfileV2(result), nil
				}
			}
			return Profile{}, fmt.Errorf("updating MBIM V2 profile: profile %d missing from response", update.ID)
		}
		return Profile{}, fmt.Errorf("updating MBIM V2 profile: profile %d not found", update.ID)
	}

	contexts, err := b.client.ProvisionedContexts(ctx)
	if err != nil {
		return Profile{}, fmt.Errorf("reading MBIM profiles: %w", errors.Join(v2Err, err))
	}
	if update.IPFamily != nil || update.Enabled != nil {
		return Profile{}, fmt.Errorf("updating legacy MBIM profile IP family or state: %w", ErrNotSupported)
	}
	var value *mbimproto.ProvisionedContext
	for i := range contexts {
		if contexts[i].ContextID == uint32(update.ID) {
			value = &contexts[i]
			break
		}
	}
	if value == nil {
		return Profile{}, fmt.Errorf("updating MBIM profile: profile %d not found", update.ID)
	}
	if update.APN != nil {
		value.AccessString = *update.APN
	}
	if update.Username != nil {
		value.UserName = *update.Username
	}
	if update.Password != nil {
		value.Password = *update.Password
	}
	if update.Authentication != nil {
		value.AuthProtocol = authenticationToMBIM(*update.Authentication)
	}
	if update.APNType != nil {
		value.ContextType = apnTypeToMBIM(*update.APNType)
	}
	updated, err := b.client.SetProvisionedContext(ctx, *value, "")
	if err != nil {
		return Profile{}, fmt.Errorf("updating MBIM profile: %w", err)
	}
	for _, current := range updated {
		if current.ContextID == uint32(update.ID) {
			return mbimProfile(current), nil
		}
	}
	return Profile{}, fmt.Errorf("updating MBIM profile: profile %d missing from response", update.ID)
}

func mbimProfileV2Config(id uint32, cfg ProfileConfig) mbimproto.ProvisionedContextV2 {
	return mbimproto.ProvisionedContextV2{
		ContextID:    id,
		ContextType:  apnTypeToMBIM(cfg.APNType),
		IPType:       ipFamilyToMBIM(cfg.IPFamily),
		State:        mbimContextState(cfg.Enabled),
		Roaming:      mbimproto.ContextRoamingControlAllowAll,
		MediaType:    mbimproto.ContextMediaTypeCellularOnly,
		Source:       mbimproto.ContextSourceUser,
		AccessString: cfg.APN,
		UserName:     cfg.Username,
		Password:     cfg.Password,
		AuthProtocol: authenticationToMBIM(cfg.Authentication),
	}
}

func applyMBIMProfileV2Update(value *mbimproto.ProvisionedContextV2, update ProfileUpdate) {
	if update.APN != nil {
		value.AccessString = *update.APN
	}
	if update.Username != nil {
		value.UserName = *update.Username
	}
	if update.Password != nil {
		value.Password = *update.Password
	}
	if update.IPFamily != nil {
		value.IPType = ipFamilyToMBIM(*update.IPFamily)
	}
	if update.Authentication != nil {
		value.AuthProtocol = authenticationToMBIM(*update.Authentication)
	}
	if update.Enabled != nil {
		value.State = mbimContextState(*update.Enabled)
	}
	if update.APNType != nil {
		value.ContextType = apnTypeToMBIM(*update.APNType)
	}
}

func mbimContextState(enabled bool) mbimproto.ContextState {
	if enabled {
		return mbimproto.ContextStateEnabled
	}
	return mbimproto.ContextStateDisabled
}

func (b *Backend) DeleteProfile(ctx context.Context, id int32) error {
	value := mbimproto.ProvisionedContextV2{ContextID: uint32(id), ContextType: mbimproto.ContextTypeInternet, IPType: mbimproto.ContextIPTypeDefault, State: mbimproto.ContextStateDisabled, Roaming: mbimproto.ContextRoamingControlAllowAll, MediaType: mbimproto.ContextMediaTypeCellularOnly, Source: mbimproto.ContextSourceUser}
	if _, err := b.client.SetProvisionedContextV2(ctx, mbimproto.ContextOperationDelete, value); err == nil {
		return nil
	} else {
		legacy := mbimproto.ProvisionedContext{ContextID: uint32(id), ContextType: mbimproto.ContextTypeNone}
		if _, legacyErr := b.client.SetProvisionedContext(ctx, legacy, ""); legacyErr != nil {
			return fmt.Errorf("deleting MBIM profile: %w", errors.Join(err, legacyErr))
		}
	}
	return nil
}

func (b *Backend) WatchProfiles(ctx context.Context) (<-chan Result[[]Profile], error) {
	return pollStream(ctx, b.Profiles), nil
}

func (b *Backend) CellInfo(ctx context.Context) ([]CellInfo, error) {
	network, err := b.NetworkStatus(ctx)
	if err != nil {
		return nil, err
	}
	signal, err := b.Signal(ctx)
	if err != nil {
		return nil, err
	}
	cell := CellInfo{
		Serving:          true,
		Technology:       network.Technology,
		OperatorID:       network.OperatorID,
		LocationAreaCode: network.LocationAreaCode,
		TrackingAreaCode: network.TrackingAreaCode,
		CellID:           network.CellID,
	}
	for _, radio := range signal.Radios {
		if radio.Technology == 0 || radio.Technology&network.Technology != 0 {
			cell.Signal = radio
			break
		}
	}
	stations, err := b.client.BaseStationsInfo(ctx, mbimproto.BaseStationCounts{LTE: 1})
	if err == nil {
		applyMBIMLTEServingCell(&cell, stations.LTEServingCell)
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []CellInfo{cell}, nil
}

func applyMBIMLTEServingCell(cell *CellInfo, serving *mbimproto.LTEServingCell) {
	if cell == nil || serving == nil {
		return
	}
	if serving.ProviderID != "" {
		cell.OperatorID = serving.ProviderID
	}
	if serving.CellID != math.MaxUint32 {
		cell.CellID = uint64(serving.CellID)
	}
	if serving.EARFCN != math.MaxUint32 {
		cell.ARFCN = serving.EARFCN
	}
	if serving.PhysicalCellID != math.MaxUint32 {
		cell.PhysicalCellID = serving.PhysicalCellID
	}
	if serving.TAC != math.MaxUint32 {
		cell.TrackingAreaCode = serving.TAC
	}
}

func (b *Backend) SAR(ctx context.Context) (SARState, error) {
	config, err := b.client.SARConfig(ctx)
	if err != nil {
		return SARState{}, fmt.Errorf("reading MBIM SAR state: %w", err)
	}
	result := SARState{Enabled: config.BackoffState == mbimproto.SARBackoffStateEnabled}
	if len(config.States) > 0 {
		result.PowerLevel = config.States[0].BackoffIndex
	}
	return result, nil
}

func (b *Backend) SetSAR(ctx context.Context, state SARState) error {
	backoff := mbimproto.SARBackoffStateDisabled
	if state.Enabled {
		backoff = mbimproto.SARBackoffStateEnabled
	}
	_, err := b.client.SetSARConfig(ctx, mbimproto.SARConfig{
		Mode:         mbimproto.SARControlModeOS,
		BackoffState: backoff,
		States:       []mbimproto.SARConfigState{{AntennaIndex: math.MaxUint32, BackoffIndex: state.PowerLevel}},
	})
	if err != nil {
		return fmt.Errorf("setting MBIM SAR state: %w", err)
	}
	return nil
}

func (b *Backend) FirmwareUpdateInfo(ctx context.Context) (FirmwareUpdateInfo, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return FirmwareUpdateInfo{}, fmt.Errorf("reading MBIM firmware revision: %w", err)
	}
	result := FirmwareUpdateInfo{Methods: []FirmwareUpdateMethod{FirmwareUpdateQDU}, Version: caps.FirmwareInfo, Ports: []string{b.device}}
	if id, idErr := b.client.FirmwareID(ctx); idErr == nil {
		result.DeviceIDs = []string{fmt.Sprintf("%x", id)}
	}
	return result, nil
}

func mbimPowerState(state mbimproto.RadioStateInfo) PowerState {
	if state.HwRadioState == mbimproto.RadioSwitchStateOff || state.SwRadioState == mbimproto.RadioSwitchStateOff {
		return PowerStateOff
	}
	return PowerStateOn
}

func mbimSIMState(state mbimproto.SubscriberReadyState) SIMState {
	switch state {
	case mbimproto.SubscriberReadyStateInitialized:
		return SIMStateReady
	case mbimproto.SubscriberReadyStateSIMNotInserted, mbimproto.SubscriberReadyStateNoESIMProfile:
		return SIMStateAbsent
	case mbimproto.SubscriberReadyStateDeviceLocked:
		return SIMStateLocked
	case mbimproto.SubscriberReadyStateBadSIM, mbimproto.SubscriberReadyStateFailure:
		return SIMStateFailure
	default:
		return SIMStateUnknown
	}
}

func mbimSlotSIMState(state mbimproto.UICCSlotState) SIMState {
	switch state {
	case mbimproto.UICCSlotStateOffEmpty, mbimproto.UICCSlotStateEmpty, mbimproto.UICCSlotStateActiveESIMNoProfiles:
		return SIMStateAbsent
	case mbimproto.UICCSlotStateActive, mbimproto.UICCSlotStateActiveESIM:
		return SIMStateReady
	case mbimproto.UICCSlotStateError:
		return SIMStateFailure
	default:
		return SIMStateUnknown
	}
}

func mbimRegistrationState(state mbimproto.RegisterState) RegistrationState {
	switch state {
	case mbimproto.RegisterStateDeregistered:
		return RegistrationIdle
	case mbimproto.RegisterStateSearching:
		return RegistrationSearching
	case mbimproto.RegisterStateHome:
		return RegistrationHome
	case mbimproto.RegisterStateRoaming, mbimproto.RegisterStatePartner:
		return RegistrationRoaming
	case mbimproto.RegisterStateDenied:
		return RegistrationDenied
	default:
		return RegistrationUnknown
	}
}

func technologyToMBIMDataClass(technology Technology) mbimproto.DataClass {
	var result mbimproto.DataClass
	if technology&TechnologyGSM != 0 {
		result |= mbimproto.DataClassGPRS | mbimproto.DataClassEDGE
	}
	if technology&TechnologyUMTS != 0 {
		result |= mbimproto.DataClassUMTS | mbimproto.DataClassHSDPA | mbimproto.DataClassHSUPA
	}
	if technology&(TechnologyLTE|TechnologyLTECatM|TechnologyLTENB) != 0 {
		result |= mbimproto.DataClassLTE
	}
	if technology&TechnologyNR5GNSA != 0 {
		result |= mbimproto.DataClass5GNSA
	}
	if technology&TechnologyNR5GSA != 0 {
		result |= mbimproto.DataClass5GSA
	}
	return result
}

func mbimCellularClass(class mbimproto.CellularClass) Technology {
	if class&mbimproto.CellularClassGSM != 0 {
		return TechnologyGSM | TechnologyUMTS | TechnologyLTE | TechnologyNR5GNSA | TechnologyNR5GSA
	}
	return 0
}

func technologyToMBIMCellularClass(technology Technology) mbimproto.CellularClass {
	if technology&TechnologyAny != 0 {
		return mbimproto.CellularClassGSM
	}
	return mbimproto.CellularClassNone
}

func authenticationToMBIM(authentication Authentication) mbimproto.AuthProtocol {
	switch {
	case authentication&AuthenticationMSCHAPv2 != 0:
		return mbimproto.AuthProtocolMSCHAPV2
	case authentication&AuthenticationCHAP != 0:
		return mbimproto.AuthProtocolCHAP
	case authentication&AuthenticationPAP != 0:
		return mbimproto.AuthProtocolPAP
	default:
		return mbimproto.AuthProtocolNone
	}
}

func mbimAuthentication(authentication mbimproto.AuthProtocol) Authentication {
	switch authentication {
	case mbimproto.AuthProtocolPAP:
		return AuthenticationPAP
	case mbimproto.AuthProtocolCHAP:
		return AuthenticationCHAP
	case mbimproto.AuthProtocolMSCHAPV2:
		return AuthenticationMSCHAPv2
	default:
		return AuthenticationNone
	}
}

func mbimIPFamily(typ mbimproto.ContextIPType) IPFamily {
	switch typ {
	case mbimproto.ContextIPTypeIPv4:
		return IPFamilyIPv4
	case mbimproto.ContextIPTypeIPv6:
		return IPFamilyIPv6
	case mbimproto.ContextIPTypeIPv4v6, mbimproto.ContextIPTypeIPv4AndIPv6:
		return IPFamilyIPv4v6
	default:
		return IPFamilyUnknown
	}
}

func mbimProfile(value mbimproto.ProvisionedContext) Profile {
	return Profile{ID: int32(value.ContextID), APN: value.AccessString, Authentication: mbimAuthentication(value.AuthProtocol), Username: value.UserName, Password: value.Password, APNType: mbimAPNType(value.ContextType), Enabled: true}
}

func mbimProfileV2(value mbimproto.ProvisionedContextV2) Profile {
	return Profile{ID: int32(value.ContextID), APN: value.AccessString, IPFamily: mbimIPFamily(value.IPType), Authentication: mbimAuthentication(value.AuthProtocol), Username: value.UserName, Password: value.Password, APNType: mbimAPNType(value.ContextType), Enabled: value.State == mbimproto.ContextStateEnabled}
}

func apnTypeToMBIM(value APNType) mbimproto.ContextType {
	switch {
	case value&APNTypeDefault != 0:
		return mbimproto.ContextTypeInternet
	case value&APNTypeIMS != 0:
		return mbimproto.ContextTypeIMS
	case value&APNTypeMMS != 0:
		return mbimproto.ContextTypeMMS
	case value&APNTypeTethering != 0:
		return mbimproto.ContextTypeTethering
	case value&APNTypeSUPL != 0:
		return mbimproto.ContextTypeSUPL
	case value&APNTypeEmergency != 0:
		return mbimproto.ContextTypeEmergencyCalling
	default:
		return mbimproto.ContextTypeInternet
	}
}

func mbimAPNType(value mbimproto.ContextType) APNType {
	switch value {
	case mbimproto.ContextTypeInternet:
		return APNTypeDefault
	case mbimproto.ContextTypeIMS:
		return APNTypeIMS
	case mbimproto.ContextTypeMMS:
		return APNTypeMMS
	case mbimproto.ContextTypeTethering:
		return APNTypeTethering
	case mbimproto.ContextTypeSUPL:
		return APNTypeSUPL
	case mbimproto.ContextTypeEmergencyCalling:
		return APNTypeEmergency
	default:
		return 0
	}
}
