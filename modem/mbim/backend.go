package mbim

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/contract"
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
	metadata            simFileMetadata
}

type simFileMetadata struct {
	OperatorID   string
	OperatorName string
	GID1         string
	SPN          string
}

type simATRReader interface {
	QueryUICCATR(context.Context) ([]byte, error)
}

type simPINReader interface {
	PIN(context.Context) (mbimproto.PINInfo, error)
}

const unknownPINAttempts = 0x00ffffff

func New(client *mbimproto.Client, device string) *Backend {
	return &Backend{client: client, device: device}
}

func (b *Backend) Close() error { return b.client.Close() }

func (b *Backend) Info(ctx context.Context) (Info, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return Info{}, err
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
		return Capabilities{}, err
	}
	services, err := b.client.DeviceServices(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	features := featuresFromServices(services)
	pduSMS := mbimproto.SMSCapsPDUReceive | mbimproto.SMSCapsPDUSend
	if caps.SMSCaps&pduSMS != pduSMS {
		features &^= FeatureSMS
	}
	result := Capabilities{
		SupportedTechnologies: technologyFromDataClass(caps.DataClass),
		CurrentTechnologies:   technologyFromDataClass(caps.DataClass),
		SupportedIPFamilies:   IPFamilyIPv4v6,
		Features:              features,
		MaxBearers:            caps.MaxSessions,
		MaxActiveBearers:      caps.MaxSessions,
		MaxSIMSlots:           1,
	}
	if features&FeatureMultiSIM != 0 {
		system, systemErr := b.client.SystemCapabilities(ctx)
		if systemErr != nil {
			return Capabilities{}, systemErr
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

func featuresFromServices(services mbimproto.DeviceServicesResponse) Feature {
	supports := services.SupportsCID
	var features Feature
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDProvisionedContexts) {
		features |= FeatureProfileManagement
	}
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDSignalState) {
		features |= FeatureSignalThresholds
	}
	if supports(mbimproto.ServiceBasicConnect, mbimproto.CIDPIN) &&
		supports(mbimproto.ServiceBasicConnect, mbimproto.CIDPINList) {
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
	if supports(mbimproto.ServiceMSSAR, mbimproto.CIDMSSARConfig) {
		features |= FeatureSAR
	}
	if supports(mbimproto.ServiceMSFirmwareID, mbimproto.CIDMSFirmwareIDGet) {
		features |= FeatureFirmwareUpdate
	}
	if supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDMSBaseStationsInfo) {
		features |= FeatureCellInfo
	}
	if supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDMSLTEAttachConfiguration) &&
		supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDMSLTEAttachInfo) {
		features |= FeatureInitialEPSBearer
	}
	if supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDMSSystemCapabilities) &&
		supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDDeviceSlotMappings) &&
		supports(mbimproto.ServiceMSBasicConnectExtensions, mbimproto.CIDMSSlotInfoStatus) {
		features |= FeatureMultiSIM
	}
	return features
}

func technologyFromDataClass(class mbimproto.DataClass) Technology {
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
		return fmt.Errorf("setting power state: state %d is invalid", state)
	}
	if _, err := b.client.SetRadioState(ctx, radio); err != nil {
		return err
	}
	return nil
}

func (b *Backend) PowerState(ctx context.Context) (PowerState, error) {
	radio, err := b.client.RadioState(ctx)
	if err != nil {
		return PowerStateUnknown, err
	}
	return powerState(radio), nil
}

func (b *Backend) Reset(ctx context.Context) error {
	if err := b.client.ResetDevice(ctx); err != nil {
		return err
	}
	return nil
}

func (b *Backend) SetCapabilities(context.Context, Technology) error {
	return ErrNotSupported
}

func (b *Backend) Status(ctx context.Context) (Status, error) {
	radio, err := b.client.RadioState(ctx)
	if err != nil {
		return Status{}, err
	}
	ready, err := b.client.SubscriberReadyStatus(ctx)
	if err != nil {
		return Status{}, err
	}
	sim, _, _ := resolveSubscriberSIMState(ctx, ready.ReadyState, b.client)
	registration, err := b.client.RegistrationState(ctx)
	if err != nil {
		return Status{}, err
	}
	packet, err := b.client.PacketService(ctx)
	if err != nil {
		return Status{}, err
	}
	signalState, err := b.client.SignalState(ctx)
	if err != nil {
		return Status{}, err
	}
	network := networkStatus(registration, packet)
	signal := signalFromState(signalState)
	return Status{
		Power:         powerState(radio),
		SIM:           sim,
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
		return SIMInfo{}, err
	}
	result := simInfoFromSubscriber(ready)
	state, pin, pinValid := resolveSubscriberSIMState(ctx, ready.ReadyState, b.client)
	result.State = state
	if pinValid {
		applyPINRetries(&result, pin)
		result.PINStatus = simPINStatus(pin)
		if pin.Type == mbimproto.PINTypeNone && state == SIMStateLocked {
			result.PINStatus.Lock = contract.SIMLockUnknown
		}
	}
	result.ATR = readSIMATR(ctx, ready.ReadyState, b.client)
	if result.State == SIMStateAbsent {
		return result, nil
	}
	if result.State != SIMStateReady || result.ICCID == "" {
		return result, nil
	}
	metadata := b.simMetadata(ctx, result.ICCID)
	result.OperatorID = metadata.OperatorID
	result.OperatorName = metadata.OperatorName
	result.GID1 = metadata.GID1
	result.SPN = metadata.SPN
	return result, nil
}

func readSIMATR(ctx context.Context, state mbimproto.SubscriberReadyState, reader simATRReader) []byte {
	switch state {
	case mbimproto.SubscriberReadyStateInitialized,
		mbimproto.SubscriberReadyStateNotActivated,
		mbimproto.SubscriberReadyStateDeviceLocked,
		mbimproto.SubscriberReadyStateNoESIMProfile:
	default:
		return nil
	}
	atr, err := reader.QueryUICCATR(ctx)
	if err != nil {
		return nil
	}
	return slices.Clone(atr)
}

func simInfoFromSubscriber(ready mbimproto.SubscriberReadyStatusResponse) SIMInfo {
	result := SIMInfo{
		State:      simState(ready.ReadyState),
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

func applyPINRetries(info *SIMInfo, pin mbimproto.PINInfo) {
	// MBIM 1.0 specifies 0x00ffffff for an unknown value, while common
	// implementations also use an all-bits-set uint32.
	if pin.RemainingAttempts == unknownPINAttempts || pin.RemainingAttempts == math.MaxUint32 {
		return
	}

	retries := uint8(min(pin.RemainingAttempts, math.MaxUint8))
	switch pin.Type {
	case mbimproto.PINTypePIN1:
		info.PINRetries = retries
	case mbimproto.PINTypePUK1:
		info.PUKRetries = retries
	}
}

func (b *Backend) querySIMState(ctx context.Context) (SIMState, error) {
	ready, err := b.client.SubscriberReadyStatus(ctx)
	if err != nil {
		return SIMStateUnknown, err
	}
	state, _, _ := resolveSubscriberSIMState(ctx, ready.ReadyState, b.client)
	return state, nil
}

func resolveSubscriberSIMState(
	ctx context.Context,
	readyState mbimproto.SubscriberReadyState,
	reader simPINReader,
) (SIMState, mbimproto.PINInfo, bool) {
	state := simState(readyState)
	if readyState != mbimproto.SubscriberReadyStateDeviceLocked &&
		readyState != mbimproto.SubscriberReadyStateInitialized {
		return state, mbimproto.PINInfo{}, false
	}

	pin, err := reader.PIN(ctx)
	if errors.Is(err, mbimproto.StatusPINRequired) {
		// Some devices return PIN_REQUIRED instead of a successful PIN1 response.
		return SIMStateLocked, pin, true
	}
	if err != nil {
		return state, pin, false
	}
	if pin.State != mbimproto.PINStateLocked {
		return state, pin, true
	}
	if pin.Type == mbimproto.PINTypePIN1 || pin.Type == mbimproto.PINTypePUK1 {
		return SIMStateLocked, pin, true
	}
	// The public contract cannot describe PIN2 or personalization locks. Do not
	// misreport those as a SIM PIN lock.
	return state, pin, true
}

type nonClosingSIMReader struct{ simcard.Reader }

func (nonClosingSIMReader) Close() error { return nil }

func (b *Backend) simMetadata(ctx context.Context, iccid string) simFileMetadata {
	b.metadataMu.Lock()
	defer b.metadataMu.Unlock()
	if iccid != "" && b.metadataKey == iccid {
		return b.metadata
	}
	metadata := simFileMetadata{}
	reader, err := wwansim.NewMBIM(b.client)
	if err != nil {
		return metadata
	}
	card, err := wwansim.New(ctx, nonClosingSIMReader{Reader: reader}, nil)
	if err != nil {
		return metadata
	}
	metadata.OperatorID = card.MCC() + card.MNC()
	metadata.OperatorName = card.SPN()
	metadata.GID1 = card.GID1()
	metadata.SPN = card.SPN()
	_ = card.Close() // Metadata has been read; cleanup cannot change the result.
	b.metadataKey = iccid
	b.metadata = metadata
	return metadata
}

func (b *Backend) SIMSlots(ctx context.Context) ([]SIMSlot, error) {
	capabilities, err := b.client.SystemCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	mappings, err := b.client.DeviceSlotMappings(ctx)
	if err != nil {
		return nil, err
	}
	active := uint32(math.MaxUint32)
	if len(mappings) > 0 {
		active = mappings[0].Slot
	}
	slots := make([]SIMSlot, capabilities.Slots)
	for i := range capabilities.Slots {
		status, err := b.client.SlotInfoStatus(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("reading SIM slot %d: %w", i+1, err)
		}
		slots[i] = SIMSlot{Index: uint8(i + 1), Active: i == active, State: slotSIMState(status.State)}
	}
	if sim, simErr := b.SIMInfo(ctx); simErr == nil {
		populateActiveSIMSlot(slots, sim)
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return slots, nil
}

func populateActiveSIMSlot(slots []SIMSlot, sim SIMInfo) {
	for i := range slots {
		if !slots[i].Active {
			continue
		}
		slots[i].ICCID = sim.ICCID
		slots[i].EID = sim.EID
		slots[i].ATR = slices.Clone(sim.ATR)
		return
	}
}

func (b *Backend) SetPrimarySIMSlot(ctx context.Context, slot uint8) error {
	if _, err := b.client.SetDeviceSlotMappings(ctx, []mbimproto.SlotMapping{{Slot: uint32(slot - 1)}}); err != nil {
		return err
	}
	return nil
}

func (b *Backend) SendPIN(ctx context.Context, pin string) error {
	result, err := b.client.SetPIN(ctx, mbimproto.PINTypePIN1, mbimproto.PINOperationEnter, pin, "")
	return pinOperationError(result, err)
}

func (b *Backend) SendPUK(ctx context.Context, puk, newPIN string) error {
	result, err := b.client.SetPIN(ctx, mbimproto.PINTypePUK1, mbimproto.PINOperationEnter, puk, newPIN)
	return pinOperationError(result, err)
}

func (b *Backend) EnablePIN(ctx context.Context, pin string, enabled bool) error {
	operation := mbimproto.PINOperationDisable
	if enabled {
		operation = mbimproto.PINOperationEnable
	}
	result, err := b.client.SetPIN(ctx, mbimproto.PINTypePIN1, operation, pin, "")
	return pinOperationError(result, err)
}

func (b *Backend) ChangePIN(ctx context.Context, oldPIN, newPIN string) error {
	result, err := b.client.SetPIN(ctx, mbimproto.PINTypePIN1, mbimproto.PINOperationChange, oldPIN, newPIN)
	return pinOperationError(result, err)
}

func (b *Backend) PreferredNetworks(ctx context.Context) ([]PreferredNetwork, error) {
	providers, err := b.client.PreferredProviders(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PreferredNetwork, len(providers))
	for i, provider := range providers {
		result[i] = PreferredNetwork{OperatorID: provider.ID, Technology: technologyFromCellularClass(provider.CellularClass)}
	}
	return result, nil
}

func (b *Backend) SetPreferredNetworks(ctx context.Context, networks []PreferredNetwork) error {
	providers := make([]mbimproto.Provider, len(networks))
	for i, network := range networks {
		providers[i] = mbimproto.Provider{ID: network.OperatorID, State: mbimproto.ProviderStatePreferred, CellularClass: cellularClass(network.Technology), RSSI: 99, ErrorRate: 99}
	}
	if _, err := b.client.SetPreferredProviders(ctx, providers); err != nil {
		return err
	}
	return nil
}

func (b *Backend) NetworkStatus(ctx context.Context) (NetworkStatus, error) {
	registration, err := b.client.RegistrationState(ctx)
	if err != nil {
		return NetworkStatus{}, err
	}
	packet, err := b.client.PacketService(ctx)
	if err != nil {
		return NetworkStatus{}, err
	}
	result := networkStatus(registration, packet)
	if location, locationErr := b.client.LocationInfoStatus(ctx); locationErr == nil {
		result.LocationAreaCode = location.LocationAreaCode
		result.TrackingAreaCode = location.TrackingAreaCode
		result.CellID = uint64(location.CellID)
	}
	return result, nil
}

func networkStatus(registration mbimproto.RegistrationStateInfo, packet mbimproto.PacketServiceInfo) NetworkStatus {
	result := NetworkStatus{}
	applyRegistration(&result, registration)
	applyPacketService(&result, packet)
	return result
}

func applyRegistration(result *NetworkStatus, registration mbimproto.RegistrationStateInfo) {
	result.Registration = registrationState(registration.RegisterState)
	result.Available = technologyFromDataClass(mbimproto.DataClass(registration.AvailableDataClasses))
	result.OperatorID = registration.ProviderID
	result.OperatorName = registration.ProviderName
	result.RoamingText = registration.RoamingText
}

func applyPacketService(result *NetworkStatus, packet mbimproto.PacketServiceInfo) {
	result.PacketService = PacketServiceState(packet.PacketServiceState)
	result.Technology = technologyFromDataClass(packet.CurrentDataClass)
	result.UplinkBitsPerSecond = packet.UplinkSpeed
	result.DownlinkBitsPerSecond = packet.DownlinkSpeed
}

func (b *Backend) Register(ctx context.Context, cfg RegisterConfig) error {
	action := mbimproto.RegisterActionAutomatic
	if cfg.OperatorID != "" {
		action = mbimproto.RegisterActionManual
	}
	if _, err := b.client.SetRegistrationState(
		ctx,
		cfg.OperatorID,
		action,
		dataClass(b.client.Version().MBIMExVersion, cfg.Technology),
	); err != nil {
		return err
	}
	return nil
}

func (b *Backend) ScanNetworks(ctx context.Context) ([]Operator, error) {
	providers, err := b.client.VisibleProviders(ctx, mbimproto.VisibleProvidersActionFullScan)
	if err != nil {
		return nil, err
	}
	result := make([]Operator, len(providers))
	for i, provider := range providers {
		result[i] = Operator{
			ID:         provider.ID,
			Name:       provider.Name,
			Technology: technologyFromCellularClass(provider.CellularClass),
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
		return err
	}
	return nil
}

func (b *Backend) FacilityLocks(ctx context.Context) ([]FacilityLock, error) {
	list, err := b.client.PINList(ctx)
	if err != nil {
		return nil, err
	}
	active, activeErr := b.client.PIN(ctx)
	result := []FacilityLock{
		facilityLock(FacilityNetwork, list.Network),
		facilityLock(FacilityNetworkSubset, list.NetworkSubset),
		facilityLock(FacilityServiceProvider, list.ServiceProvider),
		facilityLock(FacilityCorporate, list.Corporate),
	}
	if activeErr == nil {
		for i := range result {
			facility, blocked := facilityFromPINType(active.Type)
			if result[i].Facility != facility {
				continue
			}
			result[i].Enabled = result[i].Enabled || active.State == mbimproto.PINStateLocked
			result[i].Blocked = blocked && active.State == mbimproto.PINStateLocked
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
	operation := mbimproto.PINOperationDisable
	if enabled {
		operation = mbimproto.PINOperationEnable
	}
	if _, err := b.client.SetPIN(ctx, pinType(facility), operation, key, ""); err != nil {
		return err
	}
	return nil
}

func (b *Backend) UnblockFacilityLock(ctx context.Context, facility Facility, key string) error {
	if _, err := b.client.SetPIN(ctx, pukType(facility), mbimproto.PINOperationEnter, key, ""); err != nil {
		return err
	}
	return nil
}

func (b *Backend) InitialEPSBearer(ctx context.Context) (InitialEPSConfig, error) {
	info, err := b.client.LTEAttachInfo(ctx)
	if err != nil {
		return InitialEPSConfig{}, err
	}
	return InitialEPSConfig{
		APN:            info.AccessString,
		IPFamily:       ipFamily(info.IPType),
		Authentication: authentication(info.AuthProtocol),
		Username:       info.UserName,
		Password:       info.Password,
	}, nil
}

func (b *Backend) InitialEPSSettings(ctx context.Context) (InitialEPSConfig, error) {
	configurations, err := b.client.LTEAttachConfigurations(ctx)
	if err != nil {
		return InitialEPSConfig{}, err
	}
	for _, current := range configurations {
		if current.Roaming == mbimproto.LTEAttachRoamingControlHome {
			return initialEPSConfig(current), nil
		}
	}
	return InitialEPSConfig{}, fmt.Errorf("reading initial EPS settings: home configuration: %w", ErrNotSupported)
}

func (b *Backend) SetInitialEPSSettings(ctx context.Context, cfg InitialEPSConfig) (InitialEPSConfig, error) {
	configurations, err := b.client.LTEAttachConfigurations(ctx)
	if err != nil {
		return InitialEPSConfig{}, err
	}
	home := mbimproto.LTEAttachConfiguration{
		IPType:       contextIPType(cfg.IPFamily),
		Roaming:      mbimproto.LTEAttachRoamingControlHome,
		Source:       mbimproto.ContextSourceUser,
		AccessString: cfg.APN,
		UserName:     cfg.Username,
		Password:     cfg.Password,
		AuthProtocol: authProtocol(cfg.Authentication),
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
		return InitialEPSConfig{}, err
	}
	for _, current := range updated {
		if current.Roaming == mbimproto.LTEAttachRoamingControlHome {
			return initialEPSConfig(current), nil
		}
	}
	return InitialEPSConfig{}, errors.New("setting initial EPS settings: home configuration missing from response")
}

func facilityLock(facility Facility, desc mbimproto.PINDesc) FacilityLock {
	return FacilityLock{Facility: facility, Enabled: desc.Mode == mbimproto.PINModeEnabled}
}

func pinType(facility Facility) mbimproto.PINType {
	switch facility {
	case FacilityNetworkSubset:
		return mbimproto.PINTypeNetworkSubset
	case FacilityServiceProvider:
		return mbimproto.PINTypeServiceProvider
	case FacilityCorporate:
		return mbimproto.PINTypeCorporate
	default:
		return mbimproto.PINTypeNetwork
	}
}

func pukType(facility Facility) mbimproto.PINType {
	switch facility {
	case FacilityNetworkSubset:
		return mbimproto.PINTypeNetworkSubsetPUK
	case FacilityServiceProvider:
		return mbimproto.PINTypeServiceProviderPUK
	case FacilityCorporate:
		return mbimproto.PINTypeCorporatePUK
	default:
		return mbimproto.PINTypeNetworkPUK
	}
}

func facilityFromPINType(pin mbimproto.PINType) (Facility, bool) {
	switch pin {
	case mbimproto.PINTypeNetwork, mbimproto.PINTypeNetworkPUK:
		return FacilityNetwork, pin == mbimproto.PINTypeNetworkPUK
	case mbimproto.PINTypeNetworkSubset, mbimproto.PINTypeNetworkSubsetPUK:
		return FacilityNetworkSubset, pin == mbimproto.PINTypeNetworkSubsetPUK
	case mbimproto.PINTypeServiceProvider, mbimproto.PINTypeServiceProviderPUK:
		return FacilityServiceProvider, pin == mbimproto.PINTypeServiceProviderPUK
	case mbimproto.PINTypeCorporate, mbimproto.PINTypeCorporatePUK:
		return FacilityCorporate, pin == mbimproto.PINTypeCorporatePUK
	default:
		return 0, false
	}
}

func initialEPSConfig(value mbimproto.LTEAttachConfiguration) InitialEPSConfig {
	return InitialEPSConfig{
		APN:            value.AccessString,
		IPFamily:       ipFamily(value.IPType),
		Authentication: authentication(value.AuthProtocol),
		Username:       value.UserName,
		Password:       value.Password,
	}
}

func (b *Backend) Signal(ctx context.Context) (Signal, error) {
	state, err := b.client.SignalState(ctx)
	if err != nil {
		return Signal{}, err
	}
	return signalFromState(state), nil
}

func signalFromState(state mbimproto.SignalStateInfo) Signal {
	result := Signal{}
	if state.RSSI <= 31 {
		rssi := -113 + float64(state.RSSI)*2
		result.Quality = uint8(math.Round(float64(state.RSSI) * 100 / 31))
		result.Radios = append(result.Radios, RadioSignal{RSSI: knownSignal(rssi)})
	}
	for _, value := range state.RsrpSnr {
		radio := RadioSignal{Technology: technologyFromDataClass(value.SystemType)}
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
		return err
	}
	return nil
}

func (b *Backend) Profiles(ctx context.Context) ([]Profile, error) {
	contexts, err := b.client.ProvisionedContextsV2(ctx)
	if err == nil {
		result := make([]Profile, len(contexts))
		for i, value := range contexts {
			result[i] = profileFromV2(value)
		}
		return result, nil
	}
	legacy, legacyErr := b.client.ProvisionedContexts(ctx)
	if legacyErr != nil {
		return nil, fmt.Errorf("reading profiles: %w", errors.Join(err, legacyErr))
	}
	result := make([]Profile, len(legacy))
	for i, value := range legacy {
		result[i] = profileFromV1(value)
	}
	return result, nil
}

func (b *Backend) CreateProfile(ctx context.Context, cfg ProfileConfig) (Profile, error) {
	value := profileV2Config(math.MaxUint32, cfg)
	contexts, v2Err := b.client.SetProvisionedContextV2(ctx, mbimproto.ContextOperationDefault, value)
	if v2Err == nil {
		for _, current := range contexts {
			if current.AccessString == cfg.APN && current.UserName == cfg.Username {
				return profileFromV2(current), nil
			}
		}
		return Profile{}, errors.New("creating profile: modem did not return the created V2 profile")
	}

	legacy := mbimproto.ProvisionedContext{ContextID: math.MaxUint32, ContextType: contextType(cfg.APNType), AccessString: cfg.APN, UserName: cfg.Username, Password: cfg.Password, AuthProtocol: authProtocol(cfg.Authentication)}
	contextsV1, err := b.client.SetProvisionedContext(ctx, legacy, "")
	if err != nil {
		return Profile{}, fmt.Errorf("creating profile: %w", errors.Join(v2Err, err))
	}
	for _, current := range contextsV1 {
		if current.AccessString == cfg.APN && current.UserName == cfg.Username {
			return profileFromV1(current), nil
		}
	}
	return Profile{}, errors.New("creating profile: modem did not return the created profile")
}

func (b *Backend) UpdateProfile(ctx context.Context, update ProfileUpdate) (Profile, error) {
	contextsV2, v2Err := b.client.ProvisionedContextsV2(ctx)
	if v2Err == nil {
		for _, current := range contextsV2 {
			if current.ContextID != uint32(update.ID) {
				continue
			}
			applyProfileV2Update(&current, update)
			updated, err := b.client.SetProvisionedContextV2(ctx, mbimproto.ContextOperationDefault, current)
			if err != nil {
				return Profile{}, err
			}
			for _, result := range updated {
				if result.ContextID == uint32(update.ID) {
					return profileFromV2(result), nil
				}
			}
			return Profile{}, fmt.Errorf("updating V2 profile: profile %d missing from response", update.ID)
		}
		return Profile{}, fmt.Errorf("updating V2 profile: profile %d not found", update.ID)
	}

	contexts, err := b.client.ProvisionedContexts(ctx)
	if err != nil {
		return Profile{}, fmt.Errorf("reading profiles: %w", errors.Join(v2Err, err))
	}
	if update.IPFamily != nil || update.Enabled != nil {
		return Profile{}, fmt.Errorf("updating legacy profile IP family or state: %w", ErrNotSupported)
	}
	var value *mbimproto.ProvisionedContext
	for i := range contexts {
		if contexts[i].ContextID == uint32(update.ID) {
			value = &contexts[i]
			break
		}
	}
	if value == nil {
		return Profile{}, fmt.Errorf("updating profile: profile %d not found", update.ID)
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
		value.AuthProtocol = authProtocol(*update.Authentication)
	}
	if update.APNType != nil {
		value.ContextType = contextType(*update.APNType)
	}
	updated, err := b.client.SetProvisionedContext(ctx, *value, "")
	if err != nil {
		return Profile{}, err
	}
	for _, current := range updated {
		if current.ContextID == uint32(update.ID) {
			return profileFromV1(current), nil
		}
	}
	return Profile{}, fmt.Errorf("updating profile: profile %d missing from response", update.ID)
}

func profileV2Config(id uint32, cfg ProfileConfig) mbimproto.ProvisionedContextV2 {
	return mbimproto.ProvisionedContextV2{
		ContextID:    id,
		ContextType:  contextType(cfg.APNType),
		IPType:       contextIPType(cfg.IPFamily),
		State:        contextState(cfg.Enabled),
		Roaming:      mbimproto.ContextRoamingControlAllowAll,
		MediaType:    mbimproto.ContextMediaTypeCellularOnly,
		Source:       mbimproto.ContextSourceUser,
		AccessString: cfg.APN,
		UserName:     cfg.Username,
		Password:     cfg.Password,
		AuthProtocol: authProtocol(cfg.Authentication),
	}
}

func applyProfileV2Update(value *mbimproto.ProvisionedContextV2, update ProfileUpdate) {
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
		value.IPType = contextIPType(*update.IPFamily)
	}
	if update.Authentication != nil {
		value.AuthProtocol = authProtocol(*update.Authentication)
	}
	if update.Enabled != nil {
		value.State = contextState(*update.Enabled)
	}
	if update.APNType != nil {
		value.ContextType = contextType(*update.APNType)
	}
}

func contextState(enabled bool) mbimproto.ContextState {
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
			return fmt.Errorf("deleting profile: %w", errors.Join(err, legacyErr))
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
		applyLTEServingCell(&cell, stations.LTEServingCell)
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []CellInfo{cell}, nil
}

func applyLTEServingCell(cell *CellInfo, serving *mbimproto.LTEServingCell) {
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
		return SARState{}, err
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
		return err
	}
	return nil
}

func (b *Backend) FirmwareUpdateInfo(ctx context.Context) (FirmwareUpdateInfo, error) {
	caps, err := b.client.DeviceCaps(ctx)
	if err != nil {
		return FirmwareUpdateInfo{}, err
	}
	result := FirmwareUpdateInfo{Methods: []FirmwareUpdateMethod{FirmwareUpdateQDU}, Version: caps.FirmwareInfo, Ports: []string{b.device}}
	if id, idErr := b.client.FirmwareID(ctx); idErr == nil {
		result.DeviceIDs = []string{fmt.Sprintf("%x", id)}
	}
	return result, nil
}

func powerState(state mbimproto.RadioStateInfo) PowerState {
	if state.HwRadioState == mbimproto.RadioSwitchStateOff || state.SwRadioState == mbimproto.RadioSwitchStateOff {
		return PowerStateLow
	}
	return PowerStateOn
}

func simState(state mbimproto.SubscriberReadyState) SIMState {
	switch state {
	case mbimproto.SubscriberReadyStateInitialized:
		return SIMStateReady
	case mbimproto.SubscriberReadyStateSIMNotInserted, mbimproto.SubscriberReadyStateNoESIMProfile:
		return SIMStateAbsent
	case mbimproto.SubscriberReadyStateDeviceLocked:
		// CID_SUBSCRIBER_READY_STATUS does not identify which lock is active.
		// CID_PIN is the source of truth for SIM PIN and PUK locks.
		return SIMStateUnknown
	case mbimproto.SubscriberReadyStateBadSIM, mbimproto.SubscriberReadyStateFailure:
		return SIMStateFailure
	default:
		return SIMStateUnknown
	}
}

func slotSIMState(state mbimproto.UICCSlotState) SIMState {
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

func registrationState(state mbimproto.RegisterState) RegistrationState {
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

func dataClass(mbimExVersion uint16, technology Technology) mbimproto.DataClass {
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
	if technology&modeNR5G != 0 {
		if mbimExVersion >= mbimExVersion30 {
			result |= mbimproto.DataClass5G
		} else {
			if technology&TechnologyNR5GNSA != 0 {
				result |= mbimproto.DataClass5GNSA
			}
			if technology&TechnologyNR5GSA != 0 {
				result |= mbimproto.DataClass5GSA
			}
		}
	}
	return result
}

func technologyFromCellularClass(class mbimproto.CellularClass) Technology {
	if class&mbimproto.CellularClassGSM != 0 {
		return TechnologyGSM | TechnologyUMTS | TechnologyLTE | TechnologyNR5GNSA | TechnologyNR5GSA
	}
	return 0
}

func cellularClass(technology Technology) mbimproto.CellularClass {
	if technology&TechnologyAny != 0 {
		return mbimproto.CellularClassGSM
	}
	return mbimproto.CellularClassNone
}

func authProtocol(authentication Authentication) mbimproto.AuthProtocol {
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

func authentication(protocol mbimproto.AuthProtocol) Authentication {
	switch protocol {
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

func ipFamily(typ mbimproto.ContextIPType) IPFamily {
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

func profileFromV1(value mbimproto.ProvisionedContext) Profile {
	return Profile{ID: int32(value.ContextID), APN: value.AccessString, Authentication: authentication(value.AuthProtocol), Username: value.UserName, Password: value.Password, APNType: apnType(value.ContextType), Enabled: true}
}

func profileFromV2(value mbimproto.ProvisionedContextV2) Profile {
	return Profile{ID: int32(value.ContextID), APN: value.AccessString, IPFamily: ipFamily(value.IPType), Authentication: authentication(value.AuthProtocol), Username: value.UserName, Password: value.Password, APNType: apnType(value.ContextType), Enabled: value.State == mbimproto.ContextStateEnabled}
}

func contextType(value APNType) mbimproto.ContextType {
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

func apnType(value mbimproto.ContextType) APNType {
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
