package qmi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/damonto/wwan-go/modem/sms"
	"github.com/damonto/wwan-go/qcom"
	wwansim "github.com/damonto/wwan-go/sim"
	simcard "github.com/damonto/wwan-go/sim/card"
	"github.com/damonto/wwan-go/sim/simfile"
)

type Backend struct {
	client           *qcom.Client
	device           string
	metadataMu       sync.RWMutex
	metadataLoadOnce sync.Once
	metadataLoad     chan struct{}
	metadataKey      string
	metadata         SIMInfo
	enrichDelay      time.Duration
	ipaMu            sync.Mutex
	ipaReady         map[string]struct{}
	newRMNetLink     func(context.Context, string, uint32) (*rmnetLink, error)
}

var qmiSIMICCIDFile = qcom.File{
	Session: qcom.SessionPrimaryGWProvisioning,
	Path:    []byte{0x3F, 0x00, 0x2F, 0xE2},
}

const (
	qmiSIMICCIDFileSize     = 10
	qmiSIMICCIDTimeout      = 2 * time.Second
	qmiSIMEnrichmentTimeout = 5 * time.Second
)

func New(client *qcom.Client, device string) *Backend {
	return &Backend{client: client, device: device, enrichDelay: defaultSIMEnrichmentDelay}
}

func (b *Backend) Close() error { return b.client.Close() }

func (b *Backend) Info(ctx context.Context) (Info, error) {
	manufacturer, err := b.client.Manufacturer(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading QMI manufacturer: %w", err)
	}
	model, err := b.client.ModelID(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading QMI model: %w", err)
	}
	revision, err := b.client.RevisionInfo(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading QMI revision: %w", err)
	}
	hardware, err := b.client.HardwareRevision(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading QMI hardware revision: %w", err)
	}
	serials, err := b.client.SerialNumbers(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading QMI serial numbers: %w", err)
	}
	result := Info{
		Manufacturer:     manufacturer,
		Model:            model,
		Revision:         revision.Revision,
		HardwareRevision: hardware,
		EquipmentID:      serials.IMEI,
		DeviceID:         serials.ESN,
	}
	if numbers, err := b.client.MSISDN(ctx); err == nil && numbers.VoiceNumber != "" {
		result.OwnNumbers = []string{numbers.VoiceNumber}
	}
	return result, nil
}

func (b *Backend) Capabilities(ctx context.Context) (Capabilities, error) {
	caps, err := b.client.DeviceCapabilities(ctx)
	if err != nil {
		return Capabilities{}, fmt.Errorf("reading QMI device capabilities: %w", err)
	}
	versions, err := b.client.ServiceVersions(ctx)
	if err != nil {
		return Capabilities{}, fmt.Errorf("reading QMI service versions: %w", err)
	}
	technologies := technologyFromDMSRadios(caps.RadioInterfaces)
	result := Capabilities{
		SupportedTechnologies: technologies,
		CurrentTechnologies:   technologies,
		SupportedIPFamilies:   IPFamilyIPv4v6,
		Features:              featuresFromVersions(versions),
		MaxBearers:            uint32(caps.MaxActiveDataSubscriptions),
		MaxActiveBearers:      uint32(caps.MaxActiveDataSubscriptions),
		MaxSIMSlots:           1,
	}
	if result.Features&FeatureFacilityLocks != 0 {
		if _, lockErr := b.client.DMSGetCKStatus(ctx, qcom.DMSUIMFacilityNetwork); isUnsupported(lockErr) {
			result.Features &^= FeatureFacilityLocks
		}
	}
	if result.Features&FeatureSAR != 0 {
		if _, sarErr := b.client.SARRFState(ctx); isSARUnsupported(sarErr) {
			result.Features &^= FeatureSAR
		}
	}
	if hasService(versions, qcom.ServiceUIM) {
		if slots, slotErr := b.SIMSlots(ctx); slotErr == nil && len(slots) > 0 {
			result.MaxSIMSlots = uint8(min(len(slots), math.MaxUint8))
			if len(slots) > 1 {
				result.Features |= FeatureMultiSIM
			}
		}
	}
	return result, nil
}

func featuresFromVersions(versions []qcom.ServiceVersion) Feature {
	var features Feature
	for _, version := range versions {
		switch version.Service {
		case qcom.ServiceDMS:
			features |= FeatureFirmwareUpdate | FeatureFacilityLocks
		case qcom.ServiceNAS:
			features |= FeatureSignalThresholds | FeatureCellInfo
		case qcom.ServiceWDS:
			features |= FeatureProfileManagement | FeatureInitialEPSBearer
		case qcom.ServiceWMS:
			features |= FeatureSMS
		case qcom.ServiceVoice:
			features |= FeatureUSSD
		case qcom.ServiceSAR:
			features |= FeatureSAR
		}
	}
	return features
}

func hasService(versions []qcom.ServiceVersion, service qcom.ServiceType) bool {
	return slices.ContainsFunc(versions, func(version qcom.ServiceVersion) bool {
		return version.Service == service
	})
}

func technologyFromDMSRadios(radios []qcom.DMSRadioInterface) Technology {
	var result Technology
	for _, radio := range radios {
		switch radio {
		case qcom.DMSRadioInterfaceGSM:
			result |= TechnologyGSM
		case qcom.DMSRadioInterfaceUMTS:
			result |= TechnologyUMTS
		case qcom.DMSRadioInterfaceLTE:
			result |= TechnologyLTE
		case qcom.DMSRadioInterfaceNR5G:
			result |= TechnologyNR5GNSA | TechnologyNR5GSA
		}
	}
	return result
}

func (b *Backend) SetPowerState(ctx context.Context, state PowerState) error {
	var mode qcom.DMSOperatingMode
	switch state {
	case PowerStateOff:
		mode = qcom.DMSOperatingModeOffline
	case PowerStateLow:
		mode = qcom.DMSOperatingModeLowPower
	case PowerStateOn:
		mode = qcom.DMSOperatingModeOnline
	default:
		return fmt.Errorf("setting QMI power state: state %d is invalid", state)
	}
	if err := b.client.SetOperatingMode(ctx, mode); err != nil {
		return fmt.Errorf("setting QMI power state: %w", err)
	}
	return nil
}

func (b *Backend) PowerState(ctx context.Context) (PowerState, error) {
	mode, err := b.client.OperatingMode(ctx)
	if err != nil {
		return PowerStateUnknown, fmt.Errorf("reading QMI power state: %w", err)
	}
	return powerState(mode), nil
}

func (b *Backend) Reset(ctx context.Context) error {
	b.ipaMu.Lock()
	defer b.ipaMu.Unlock()
	if err := b.client.DMSReset(ctx); err != nil {
		return fmt.Errorf("resetting QMI modem: %w", err)
	}
	clear(b.ipaReady)
	return nil
}

func (b *Backend) Modes(ctx context.Context) ([]Mode, Mode, error) {
	caps, err := b.Capabilities(ctx)
	if err != nil {
		return nil, Mode{}, err
	}
	preference, err := b.client.SystemSelectionPreference(ctx)
	if err != nil {
		return nil, Mode{}, fmt.Errorf("reading QMI mode preference: %w", err)
	}
	current := Mode{Allowed: caps.CurrentTechnologies}
	if preference.ModePreferenceKnown {
		current.Allowed = technologyFromModePreference(preference.ModePreference)
	}
	return []Mode{{Allowed: caps.SupportedTechnologies}}, current, nil
}

func (b *Backend) SetModes(ctx context.Context, mode Mode) error {
	if mode.Allowed == 0 || mode.Allowed&^TechnologyAny != 0 {
		return fmt.Errorf("setting QMI modes: allowed technologies %#x are invalid", mode.Allowed)
	}
	if mode.Preferred&^mode.Allowed != 0 {
		return errors.New("setting QMI modes: preferred technologies are not a subset of allowed technologies")
	}
	preference := modePreferenceFromTechnology(mode.Allowed)
	if err := b.client.SetSystemSelectionPreference(ctx, qcom.NASSystemSelectionConfig{ModePreference: &preference}); err != nil {
		return fmt.Errorf("setting QMI modes: %w", err)
	}
	return nil
}

func (b *Backend) SetCapabilities(ctx context.Context, technologies Technology) error {
	if technologies == 0 || technologies&^TechnologyAny != 0 {
		return fmt.Errorf("setting QMI capabilities: technologies %#x are invalid", technologies)
	}
	preference := modePreferenceFromTechnology(technologies)
	duration := qcom.NASChangePermanent
	if err := b.client.SetSystemSelectionPreference(ctx, qcom.NASSystemSelectionConfig{
		ModePreference: &preference,
		ChangeDuration: &duration,
	}); err != nil {
		return fmt.Errorf("setting QMI capabilities: %w", err)
	}
	if err := b.Reset(ctx); err != nil {
		return fmt.Errorf("setting QMI capabilities: %w", err)
	}
	return nil
}

func (b *Backend) Status(ctx context.Context) (Status, error) {
	mode, err := b.client.OperatingMode(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading QMI operating mode: %w", err)
	}
	card, err := b.client.CardStatus(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading QMI card status: %w", err)
	}
	network, err := b.NetworkStatus(ctx)
	if err != nil {
		return Status{}, err
	}
	signal, err := b.Signal(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{
		Power:         powerState(mode),
		SIM:           simStateFromCardStatus(card),
		Registration:  network.Registration,
		PacketService: network.PacketService,
		Technology:    network.Technology,
		OperatorID:    network.OperatorID,
		OperatorName:  network.OperatorName,
		SignalQuality: signal.Quality,
	}, nil
}

func (b *Backend) SIMInfo(ctx context.Context) (SIMInfo, error) {
	result, err := b.simInfo(ctx)
	if err != nil {
		return SIMInfo{}, err
	}
	if result.State == SIMStateAbsent {
		return result, nil
	}
	enrichmentCtx, cancel := context.WithTimeout(ctx, qmiSIMEnrichmentTimeout)
	defer cancel()
	if result.ICCID == "" {
		if value, readErr := b.client.ICCID(enrichmentCtx); readErr == nil {
			result.ICCID = value
		}
	}
	if result.State != SIMStateReady || result.ICCID == "" {
		return result, nil
	}
	if value, readErr := b.client.IMSI(enrichmentCtx); readErr == nil {
		result.IMSI = value
	}
	if value, readErr := b.client.MSISDN(enrichmentCtx); readErr == nil && value.VoiceNumber != "" {
		result.OwnNumbers = []string{value.VoiceNumber}
	}
	applySIMMetadata(&result, b.simMetadata(enrichmentCtx, result.ICCID))
	return result, nil
}

func (b *Backend) simInfo(ctx context.Context) (SIMInfo, error) {
	status, err := b.client.CardStatus(ctx)
	if err != nil {
		return SIMInfo{}, fmt.Errorf("reading QMI card status: %w", err)
	}
	return b.simInfoFromCardStatus(ctx, status), nil
}

func (b *Backend) simInfoFromCardStatus(ctx context.Context, status qcom.CardStatus) SIMInfo {
	result := basicSIMInfo(status, b.client.Slot())
	if !simIdentityReady(status) {
		return result
	}
	iccid, err := b.readSIMICCID(ctx)
	if err != nil {
		return result
	}
	result.ICCID = iccid
	if metadata, ok := b.cachedSIMMetadata(iccid); ok {
		applySIMMetadata(&result, metadata)
	}
	return result
}

func simIdentityReady(status qcom.CardStatus) bool {
	if len(status.Cards) == 0 || status.Cards[0].State != qcom.CardStatePresent {
		return false
	}
	for _, app := range status.Cards[0].Applications {
		if (app.Type == qcom.ApplicationTypeSIM || app.Type == qcom.ApplicationTypeUSIM) && app.State == qcom.ApplicationStateReady {
			return true
		}
	}
	return false
}

func basicSIMInfo(status qcom.CardStatus, slot uint8) SIMInfo {
	result := SIMInfo{Slot: slot, State: simStateFromCardStatus(status)}
	if len(status.Cards) == 0 {
		return result
	}
	card := status.Cards[0]
	result.PINRetries = card.UPINRetries
	result.PUKRetries = card.UPUKRetries
	for _, app := range card.Applications {
		if app.Type != qcom.ApplicationTypeSIM && app.Type != qcom.ApplicationTypeUSIM {
			continue
		}
		result.PINRetries = app.PIN1Retries
		result.PUKRetries = app.PUK1Retries
		break
	}
	return result
}

func (b *Backend) readSIMICCID(ctx context.Context) (string, error) {
	readCtx, cancel := context.WithTimeout(ctx, qmiSIMICCIDTimeout)
	defer cancel()
	raw, err := b.client.ReadTransparent(readCtx, qcom.TransparentRead{
		File:   qmiSIMICCIDFile,
		Length: qmiSIMICCIDFileSize,
	})
	if err != nil {
		return "", fmt.Errorf("reading QMI UIM EF_ICCID: %w", err)
	}
	var iccid simfile.ICCID
	if err := iccid.UnmarshalBinary(raw); err != nil {
		return "", fmt.Errorf("decoding QMI UIM EF_ICCID: %w", err)
	}
	return iccid.String(), nil
}

func (b *Backend) SIMSlots(ctx context.Context) ([]SIMSlot, error) {
	status, err := b.client.SlotStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading QMI SIM slots: %w", err)
	}
	slots := make([]SIMSlot, len(status.Slots))
	for i, slot := range status.Slots {
		slots[i] = SIMSlot{
			Index:  uint8(i + 1),
			Active: slot.PhysicalSlotStatus == qcom.SlotStateActive,
			State:  simStateFromPhysicalCardState(slot.PhysicalCardStatus),
			ICCID:  decodeSlotICCID(slot.ICCID),
		}
	}
	return slots, nil
}

type nonClosingSIMReader struct{ simcard.Reader }

func (nonClosingSIMReader) Close() error { return nil }

func (b *Backend) simMetadata(ctx context.Context, iccid string) SIMInfo {
	if metadata, ok := b.cachedSIMMetadata(iccid); ok {
		return metadata
	}
	if err := b.lockMetadata(ctx); err != nil {
		return SIMInfo{}
	}
	defer b.unlockMetadata()
	if metadata, ok := b.cachedSIMMetadata(iccid); ok {
		return metadata
	}

	metadata := SIMInfo{}
	if atr, err := b.client.ATR(ctx); err == nil {
		metadata.ATR = atr
	}
	reader, err := wwansim.NewQCOM(b.client)
	if err == nil {
		card, cardErr := wwansim.New(ctx, nonClosingSIMReader{Reader: reader}, nil)
		if cardErr == nil {
			metadata.ICCID = card.ICCID()
			metadata.IMSI = card.IMSI()
			metadata.OperatorID = card.MCC() + card.MNC()
			metadata.OperatorName = card.SPN()
			metadata.GID1 = card.GID1()
			metadata.SPN = card.SPN()
			_ = card.Close()
		}
	}
	if metadata.ICCID == "" || metadata.ICCID != iccid {
		return SIMInfo{}
	}
	b.metadataMu.Lock()
	b.metadataKey = iccid
	b.metadata = metadata
	b.metadataMu.Unlock()
	return metadata
}

func (b *Backend) lockMetadata(ctx context.Context) error {
	b.metadataLoadOnce.Do(func() {
		b.metadataLoad = make(chan struct{}, 1)
		b.metadataLoad <- struct{}{}
	})
	select {
	case <-b.metadataLoad:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Backend) unlockMetadata() {
	b.metadataLoad <- struct{}{}
}

func (b *Backend) cachedSIMMetadata(iccid string) (SIMInfo, bool) {
	if iccid == "" {
		return SIMInfo{}, false
	}
	b.metadataMu.RLock()
	defer b.metadataMu.RUnlock()
	if b.metadataKey != iccid {
		return SIMInfo{}, false
	}
	return b.metadata, true
}

func applySIMMetadata(result *SIMInfo, metadata SIMInfo) {
	mergeSIMIdentity(result, metadata)
	result.OperatorID = metadata.OperatorID
	result.OperatorName = metadata.OperatorName
	result.GID1 = metadata.GID1
	result.SPN = metadata.SPN
	result.ATR = slices.Clone(metadata.ATR)
}

func mergeSIMIdentity(result *SIMInfo, metadata SIMInfo) {
	if result.ICCID == "" {
		result.ICCID = metadata.ICCID
	}
	if result.IMSI == "" {
		result.IMSI = metadata.IMSI
	}
}

func decodeSlotICCID(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	if slices.IndexFunc(value, func(b byte) bool { return b < '0' || b > '9' }) == -1 {
		return string(value)
	}
	var iccid simfile.ICCID
	if err := iccid.UnmarshalBinary(value); err == nil {
		return iccid.String()
	}
	return ""
}

func (b *Backend) SetPrimarySIMSlot(ctx context.Context, slot uint8) error {
	if err := b.client.SwitchSlot(ctx, 1, uint32(slot)); err != nil {
		return fmt.Errorf("setting QMI primary SIM slot: %w", err)
	}
	return nil
}

func (b *Backend) SendPIN(ctx context.Context, pin string) error {
	_, err := b.client.VerifyPIN(ctx, qcom.PINVerifyRequest{Session: qcom.SessionPrimaryGWProvisioning, ID: qcom.PINIDPIN1, PIN: pin})
	if err != nil {
		return fmt.Errorf("sending QMI PIN: %w", err)
	}
	return nil
}

func (b *Backend) SendPUK(ctx context.Context, puk, newPIN string) error {
	_, err := b.client.UnblockPIN(ctx, qcom.PINUnblockRequest{Session: qcom.SessionPrimaryGWProvisioning, ID: qcom.PINIDPIN1, PUK: puk, NewPIN: newPIN})
	if err != nil {
		return fmt.Errorf("sending QMI PUK: %w", err)
	}
	return nil
}

func (b *Backend) EnablePIN(ctx context.Context, pin string, enabled bool) error {
	_, err := b.client.SetPINProtection(ctx, qcom.PINProtectionRequest{Session: qcom.SessionPrimaryGWProvisioning, ID: qcom.PINIDPIN1, PIN: pin, Enable: enabled})
	if err != nil {
		return fmt.Errorf("setting QMI PIN protection: %w", err)
	}
	return nil
}

func (b *Backend) ChangePIN(ctx context.Context, oldPIN, newPIN string) error {
	_, err := b.client.ChangePIN(ctx, qcom.PINChangeRequest{Session: qcom.SessionPrimaryGWProvisioning, ID: qcom.PINIDPIN1, OldPIN: oldPIN, NewPIN: newPIN})
	if err != nil {
		return fmt.Errorf("changing QMI PIN: %w", err)
	}
	return nil
}

func (b *Backend) PreferredNetworks(ctx context.Context) ([]PreferredNetwork, error) {
	result, err := b.client.PreferredNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading QMI preferred networks: %w", err)
	}
	networks := make([]PreferredNetwork, len(result.Networks))
	for i, network := range result.Networks {
		networks[i] = PreferredNetwork{OperatorID: network.PLMN.String(), Technology: technologyFromPLMNAccess(network.AccessTechnology)}
	}
	return networks, nil
}

func (b *Backend) SetPreferredNetworks(ctx context.Context, networks []PreferredNetwork) error {
	values := make([]qcom.NASPreferredNetwork, len(networks))
	for i, network := range networks {
		var plmn qcom.NASPLMN
		if err := plmn.UnmarshalText([]byte(network.OperatorID)); err != nil {
			return fmt.Errorf("setting QMI preferred network %d: %w", i, err)
		}
		values[i] = qcom.NASPreferredNetwork{PLMN: plmn, AccessTechnology: plmnAccessFromTechnology(network.Technology)}
	}
	clear := true
	if err := b.client.SetPreferredNetworks(ctx, qcom.NASPreferredNetworksConfig{Networks: values, ClearPrevious: &clear}); err != nil {
		return fmt.Errorf("setting QMI preferred networks: %w", err)
	}
	return nil
}

func (b *Backend) NetworkStatus(ctx context.Context) (NetworkStatus, error) {
	serving, err := b.client.NASServingSystem(ctx)
	if err != nil {
		return NetworkStatus{}, fmt.Errorf("reading QMI serving system: %w", err)
	}
	return networkStatusFromServing(serving), nil
}

func networkStatusFromServing(serving qcom.NASServingSystem) NetworkStatus {
	result := NetworkStatus{
		Registration:     registrationStateFromServingSystem(serving),
		PacketService:    packetServiceStateFromAttachState(serving.PSAttachState),
		Technology:       technologyFromNASRadios(serving.RadioInterfaces),
		Available:        technologyFromNASRadios(serving.RadioInterfaces),
		LocationAreaCode: uint32(serving.LocationAreaCode),
		TrackingAreaCode: uint32(serving.TrackingAreaCode),
		CellID:           uint64(serving.CellID),
	}
	if serving.PLMNKnown {
		result.OperatorID = serving.PLMN.String()
		result.OperatorName = decodeNetworkDescription(serving.PLMN.Description)
	}
	if serving.RoamingIndicatorKnown && serving.RoamingIndicator == qcom.NASRoamingIndicatorRoaming {
		result.RoamingText = "roaming"
	}
	return result
}

func decodeNetworkDescription(value string) string {
	raw := []byte(value)
	if decoded, ok := printableString(raw); ok {
		return decoded
	}

	// Some Qualcomm firmware returns this nominally textual field as packed
	// GSM 7-bit. Keep the same best-effort fallback used by libqmi.
	septets := sms.UnpackSeptets(raw, 0, len(raw)*8/7)
	for len(septets) > 0 && septets[len(septets)-1] == 0 {
		septets = septets[:len(septets)-1]
	}
	var gsm7 sms.GSM7
	if err := gsm7.UnmarshalBinary(septets); err == nil {
		if decoded, ok := printableString([]byte(gsm7.String())); ok {
			return decoded
		}
	}

	for len(raw) >= 2 && raw[len(raw)-2] == 0 && raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-2]
	}
	var ucs2 sms.UCS2LE
	if err := ucs2.UnmarshalBinary(raw); err == nil {
		if decoded, ok := printableString([]byte(ucs2.String())); ok {
			return decoded
		}
	}
	return ""
}

func printableString(value []byte) (string, bool) {
	value = bytes.TrimRight(value, "\x00")
	if !utf8.Valid(value) {
		return "", false
	}
	for _, r := range string(value) {
		if r == '\r' || r == '\n' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			return "", false
		}
	}
	return string(value), true
}

func (b *Backend) Register(ctx context.Context, cfg RegisterConfig) error {
	registration := qcom.NASNetworkRegistration{Action: qcom.NASRegisterAutomatically}
	if cfg.OperatorID != "" {
		var plmn qcom.NASPLMN
		if err := plmn.UnmarshalText([]byte(cfg.OperatorID)); err != nil {
			return fmt.Errorf("registering QMI network: %w", err)
		}
		radio := nasRadioFromTechnology(cfg.Technology)
		registration.Action = qcom.NASRegisterManually
		registration.Manual = &qcom.NASManualRegistration{PLMN: plmn, RadioInterface: radio}
	}
	if err := b.client.RegisterNetwork(ctx, registration); err != nil {
		return fmt.Errorf("registering QMI network: %w", err)
	}
	return nil
}

func (b *Backend) ScanNetworks(ctx context.Context) ([]Operator, error) {
	scan, err := b.client.NetworkScan(ctx, qcom.NASNetworkScanConfig{})
	if err != nil {
		return nil, fmt.Errorf("scanning QMI networks: %w", err)
	}
	operators := make([]Operator, len(scan.Networks))
	for i, network := range scan.Networks {
		operators[i] = Operator{
			ID:         network.PLMN.String(),
			Name:       decodeNetworkDescription(network.PLMN.Description),
			Technology: technologyFromNASRadios(network.RadioInterfaces),
			Available:  network.Status.InUse() == qcom.NASNetworkInUseAvailable,
			Current:    network.Status.InUse() == qcom.NASNetworkInUseCurrent,
			Forbidden:  network.Status.Forbidden() == qcom.NASNetworkForbidden,
		}
	}
	return operators, nil
}

func (b *Backend) SetPacketServiceState(ctx context.Context, state PacketServiceState) error {
	action := qcom.NASPSAttach
	if state == PacketServiceDetached {
		action = qcom.NASPSDetach
	}
	if err := b.client.NASAttachDetach(ctx, action); err != nil {
		return fmt.Errorf("setting QMI packet service: %w", err)
	}
	return nil
}

func (b *Backend) FacilityLocks(ctx context.Context) ([]FacilityLock, error) {
	facilities := []Facility{FacilityNetwork, FacilityNetworkSubset, FacilityServiceProvider, FacilityCorporate}
	result := make([]FacilityLock, 0, len(facilities))
	for _, facility := range facilities {
		status, err := b.client.DMSGetCKStatus(ctx, uimFacility(facility))
		if err != nil {
			if isUnsupported(err) {
				return nil, fmt.Errorf("reading QMI facility %d: %w: %w", facility, ErrNotSupported, err)
			}
			return nil, fmt.Errorf("reading QMI facility %d: %w", facility, err)
		}
		result = append(result, FacilityLock{
			Facility:       facility,
			Enabled:        status.FacilityState != qcom.DMSFacilityDeactivated,
			Blocked:        status.FacilityState == qcom.DMSFacilityBlocked,
			VerifyRetries:  uint32(status.VerifyRetries),
			UnblockRetries: uint32(status.UnblockRetries),
		})
	}
	return result, nil
}

func (b *Backend) SetFacilityLock(ctx context.Context, facility Facility, enabled bool, key string) error {
	state := qcom.DMSFacilityDeactivated
	if enabled {
		state = qcom.DMSFacilityActivated
	}
	_, err := b.client.DMSSetCKProtection(ctx, qcom.DMSCKProtectionRequest{
		Facility: uimFacility(facility),
		State:    state,
		Key:      key,
	})
	if err != nil {
		return fmt.Errorf("setting QMI facility lock: %w", err)
	}
	return nil
}

func (b *Backend) UnblockFacilityLock(ctx context.Context, facility Facility, key string) error {
	_, err := b.client.DMSUnblockCK(ctx, qcom.DMSCKUnblockRequest{Facility: uimFacility(facility), Key: key})
	if err != nil {
		return fmt.Errorf("unblocking QMI facility lock: %w", err)
	}
	return nil
}

func (b *Backend) InitialEPSBearer(ctx context.Context) (InitialEPSConfig, error) {
	parameters, err := b.client.WDSLTEAttachParameters(ctx)
	if errors.Is(err, qcom.QMIErrorInformationUnavailable) {
		return InitialEPSConfig{}, nil
	}
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading QMI initial EPS bearer: %w", err)
	}
	result := InitialEPSConfig{}
	if parameters.APNKnown {
		result.APN = parameters.APN
	}
	if parameters.IPSupportKnown {
		result.IPFamily = ipFamilyFromAttachSupport(parameters.IPSupport)
	}
	if list, listErr := b.client.WDSLTEAttachPDNList(ctx); listErr == nil && list.CurrentKnown && len(list.Current) > 0 {
		result.ProfileID = int32(list.Current[0])
	}
	return result, nil
}

func (b *Backend) InitialEPSSettings(ctx context.Context) (InitialEPSConfig, error) {
	list, err := b.client.WDSLTEAttachPDNList(ctx)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading QMI initial EPS settings: %w", err)
	}
	if !list.CurrentKnown || len(list.Current) == 0 {
		return b.InitialEPSBearer(ctx)
	}
	if list.Current[0] > math.MaxUint8 {
		return InitialEPSConfig{}, fmt.Errorf("reading QMI initial EPS settings: profile ID %d exceeds 255", list.Current[0])
	}
	settings, err := b.client.WDSProfileSettings(ctx, qcom.WDSProfileID{Type: qcom.WDSProfileType3GPP, Index: uint8(list.Current[0])})
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading QMI initial EPS profile %d: %w", list.Current[0], err)
	}
	return initialEPSFromProfile(profileFromSettings(settings)), nil
}

func (b *Backend) SetInitialEPSSettings(ctx context.Context, cfg InitialEPSConfig) (InitialEPSConfig, error) {
	list, err := b.client.WDSLTEAttachPDNList(ctx)
	if err != nil {
		return InitialEPSConfig{}, fmt.Errorf("reading QMI initial EPS profile list: %w", err)
	}
	profileID := cfg.ProfileID
	profileKnown := profileID != 0
	created := false
	if !profileKnown && list.CurrentKnown && len(list.Current) > 0 {
		profileID = int32(list.Current[0])
	}
	if profileID > math.MaxUint8 {
		return InitialEPSConfig{}, fmt.Errorf("setting QMI initial EPS settings: profile ID %d exceeds 255", profileID)
	}

	if profileKnown {
		apn, family, auth := cfg.APN, cfg.IPFamily, cfg.Authentication
		username, password, enabled := cfg.Username, cfg.Password, true
		_, err = b.UpdateProfile(ctx, ProfileUpdate{
			ID:             profileID,
			APN:            &apn,
			IPFamily:       &family,
			Authentication: &auth,
			Username:       &username,
			Password:       &password,
			Enabled:        &enabled,
		})
		if err != nil {
			return InitialEPSConfig{}, fmt.Errorf("setting QMI initial EPS profile: %w", err)
		}
	} else {
		profile, createErr := b.CreateProfile(ctx, ProfileConfig{
			APN:            cfg.APN,
			IPFamily:       cfg.IPFamily,
			Authentication: cfg.Authentication,
			Username:       cfg.Username,
			Password:       cfg.Password,
			Enabled:        true,
		})
		if createErr != nil {
			return InitialEPSConfig{}, fmt.Errorf("creating QMI initial EPS profile: %w", createErr)
		}
		profileID = profile.ID
		created = true
		profileKnown = true
	}

	profiles := []uint16{uint16(profileID)}
	if list.CurrentKnown {
		for _, current := range list.Current {
			if current != uint16(profileID) {
				profiles = append(profiles, current)
			}
		}
	}
	action := qcom.WDSAttachPDNListNoAction
	if err := b.client.WDSSetLTEAttachPDNList(ctx, profiles, &action); err != nil {
		setErr := fmt.Errorf("setting QMI initial EPS profile list: %w", err)
		if created {
			setErr = errors.Join(setErr, b.DeleteProfile(ctx, profileID))
		}
		return InitialEPSConfig{}, setErr
	}
	return b.InitialEPSSettings(ctx)
}

func uimFacility(facility Facility) qcom.DMSUIMFacility {
	switch facility {
	case FacilityNetworkSubset:
		return qcom.DMSUIMFacilityNetworkSubset
	case FacilityServiceProvider:
		return qcom.DMSUIMFacilityServiceProvider
	case FacilityCorporate:
		return qcom.DMSUIMFacilityCorporate
	default:
		return qcom.DMSUIMFacilityNetwork
	}
}

func ipFamilyFromAttachSupport(value qcom.WDSIPSupportType) IPFamily {
	switch value {
	case qcom.WDSIPSupportIPv4:
		return IPFamilyIPv4
	case qcom.WDSIPSupportIPv6:
		return IPFamilyIPv6
	case qcom.WDSIPSupportIPv4v6:
		return IPFamilyIPv4v6
	default:
		return IPFamilyUnknown
	}
}

func (b *Backend) Signal(ctx context.Context) (Signal, error) {
	info, err := b.client.SignalInfo(ctx)
	if isSignalUnavailable(err) {
		return Signal{}, nil
	}
	if err != nil {
		return Signal{}, fmt.Errorf("reading QMI signal: %w", err)
	}
	return signalFromInfo(info), nil
}

func signalFromInfo(info qcom.NASSignalInfo) Signal {
	result := Signal{}
	if info.GSMKnown {
		result.Radios = append(result.Radios, RadioSignal{Technology: TechnologyGSM, RSSI: knownSignal(float64(info.GSM))})
	}
	if info.WCDMAKnown {
		radio := RadioSignal{Technology: TechnologyUMTS, RSSI: knownSignal(float64(info.WCDMA.RSSI)), ECIO: knownSignal(float64(info.WCDMA.ECIO) / 2)}
		if info.UMTSRSCPKnown {
			radio.RSCP = knownSignal(float64(info.UMTSRSCP))
		}
		result.Radios = append(result.Radios, radio)
	}
	if info.LTEKnown {
		result.Radios = append(result.Radios, RadioSignal{Technology: TechnologyLTE, RSSI: knownSignal(float64(info.LTE.RSSI)), RSRQ: knownSignal(float64(info.LTE.RSRQ)), RSRP: knownSignal(float64(info.LTE.RSRP)), SNR: knownSignal(float64(info.LTE.SNR) / 10)})
	}
	if info.NR5GKnown {
		radio := RadioSignal{Technology: TechnologyNR5GNSA | TechnologyNR5GSA, RSRP: knownSignal(float64(info.NR5G.RSRP)), SNR: knownSignal(float64(info.NR5G.SNR) / 10)}
		if info.NR5GRSRQKnown {
			radio.RSRQ = knownSignal(float64(info.NR5GRSRQ))
		}
		result.Radios = append(result.Radios, radio)
	}
	result.Quality = signalQuality(result.Radios)
	return result
}

func (b *Backend) SetSignalThresholds(ctx context.Context, thresholds SignalThresholds) error {
	if thresholds.ErrorRateThreshold {
		return fmt.Errorf("setting QMI signal thresholds: %w", ErrNotSupported)
	}
	lteReport, nr5gReport, err := signalReportConfigs(thresholds.Interval)
	if err != nil {
		return fmt.Errorf("setting QMI signal thresholds: %w", err)
	}
	deltaDB := thresholds.RSSIChangeDB
	if deltaDB == 0 {
		deltaDB = 5
	}
	if deltaDB > math.MaxUint16/10 {
		return fmt.Errorf("setting QMI signal thresholds: RSSI delta %d dB is too large", deltaDB)
	}
	delta := uint16(deltaDB * 10)
	config := qcom.NASSignalThresholdConfig2{
		LTERSSI:    qcom.NASScaledSignalConfig{Delta: &delta},
		NR5GRSRP:   qcom.NASScaledSignalConfig{Delta: &delta},
		WCDMARSCP:  qcom.NASScaledSignalConfig{Delta: &delta},
		LTEReport:  lteReport,
		NR5GReport: nr5gReport,
	}
	if err := b.client.ConfigureSignalInfo2(ctx, config); err == nil {
		return nil
	} else if !isSignalV2Unsupported(err) {
		return fmt.Errorf("setting QMI signal thresholds: %w", err)
	}

	legacy := make([]int8, 0, 16)
	for value := int32(-110); value <= -30 && len(legacy) < cap(legacy); value += int32(deltaDB) {
		legacy = append(legacy, int8(value))
	}
	if err := b.client.ConfigureSignalInfo(ctx, qcom.NASSignalThresholdConfig{RSSI: legacy, LTEReport: lteReport}); err != nil {
		return fmt.Errorf("setting QMI signal thresholds: %w", err)
	}
	return nil
}

func signalReportConfigs(interval time.Duration) (*qcom.NASLTESignalReportConfig, *qcom.NASNR5GSignalReportConfig, error) {
	if interval == 0 {
		return nil, nil, nil
	}
	if interval < 0 {
		return nil, nil, errors.New("signal interval is negative")
	}
	if interval > 5*time.Second {
		return nil, nil, fmt.Errorf("signal interval %s exceeds the QMI LTE limit of 5s", interval)
	}

	seconds := uint8((interval + time.Second - 1) / time.Second)
	lte := &qcom.NASLTESignalReportConfig{Rate: qcom.NASLTESignalReportRate(seconds)}
	nr5g := &qcom.NASNR5GSignalReportConfig{Rate: qcom.NASNR5GSignalReportRate(seconds)}
	return lte, nr5g, nil
}

func isSignalV2Unsupported(err error) bool {
	var protocolErr qcom.QMIError
	if !errors.As(err, &protocolErr) {
		return false
	}
	return protocolErr == qcom.QMIErrorInvalidQmiCommand || protocolErr == qcom.QMIErrorNotSupported
}

func (b *Backend) Profiles(ctx context.Context) ([]Profile, error) {
	entries, err := b.client.WDSProfiles(ctx, qcom.WDSProfileType3GPP)
	if err != nil {
		return nil, fmt.Errorf("reading QMI profiles: %w", err)
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		settings, err := b.client.WDSProfileSettings(ctx, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("reading QMI profile %d: %w", entry.ID.Index, err)
		}
		profiles = append(profiles, profileFromSettings(settings))
	}
	return profiles, nil
}

func (b *Backend) CreateProfile(ctx context.Context, cfg ProfileConfig) (Profile, error) {
	profileCfg := qcom.WDSProfileConfig{Type: qcom.WDSProfileType3GPP, APN: cfg.APN, PDPType: pdpTypeFromIPFamily(cfg.IPFamily), Username: cfg.Username, Password: cfg.Password, Authentication: authenticationMask(cfg.Authentication)}
	disabled := !cfg.Enabled
	profileCfg.APNDisabled = &disabled
	apnType := apnTypeMask(cfg.APNType)
	profileCfg.APNType = &apnType
	id, err := b.client.WDSCreateProfileWithConfig(ctx, profileCfg)
	if err != nil {
		return Profile{}, fmt.Errorf("creating QMI profile: %w", err)
	}
	settings, err := b.client.WDSProfileSettings(ctx, id)
	if err != nil {
		return Profile{}, fmt.Errorf("reading created QMI profile: %w", err)
	}
	return profileFromSettings(settings), nil
}

func (b *Backend) UpdateProfile(ctx context.Context, update ProfileUpdate) (Profile, error) {
	id := qcom.WDSProfileID{Type: qcom.WDSProfileType3GPP, Index: uint8(update.ID)}
	value := qcom.WDSProfileUpdate{APN: update.APN, Username: update.Username, Password: update.Password}
	if update.IPFamily != nil {
		pdp := pdpTypeFromIPFamily(*update.IPFamily)
		value.PDPType = &pdp
	}
	if update.Authentication != nil {
		auth := authenticationMask(*update.Authentication)
		value.Authentication = &auth
	}
	if update.APNType != nil {
		apnType := apnTypeMask(*update.APNType)
		value.APNType = &apnType
	}
	if update.Enabled != nil {
		disabled := !*update.Enabled
		value.APNDisabled = &disabled
	}
	if err := b.client.WDSUpdateProfile(ctx, id, value); err != nil {
		return Profile{}, fmt.Errorf("updating QMI profile: %w", err)
	}
	settings, err := b.client.WDSProfileSettings(ctx, id)
	if err != nil {
		return Profile{}, fmt.Errorf("reading updated QMI profile: %w", err)
	}
	return profileFromSettings(settings), nil
}

func (b *Backend) DeleteProfile(ctx context.Context, id int32) error {
	if id > 255 {
		return fmt.Errorf("deleting QMI profile: profile ID %d exceeds 255", id)
	}
	if err := b.client.WDSDeleteProfile(ctx, uint8(id)); err != nil {
		return fmt.Errorf("deleting QMI profile: %w", err)
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
		if radio.Technology&network.Technology != 0 {
			cell.Signal = radio
			break
		}
	}
	location, err := b.client.CellLocationInfo(ctx)
	if err == nil {
		applyCellLocation(&cell, location)
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []CellInfo{cell}, nil
}

func applyCellLocation(cell *CellInfo, location qcom.NASCellLocationInfo) {
	if cell == nil {
		return
	}
	if location.LTEIntraEARFCNKnown {
		cell.ARFCN = location.LTEIntraEARFCN
		return
	}
	// Older Qualcomm firmware reports the serving EARFCN in the standard
	// LTE intrafrequency TLV but omits the newer standalone EARFCN TLV.
	if location.LTEIntraKnown {
		cell.ARFCN = uint32(location.LTEIntra.EARFCN)
	}
}

func (b *Backend) SAR(ctx context.Context) (SARState, error) {
	state, err := b.client.SARRFState(ctx)
	if err != nil {
		if isSARUnsupported(err) {
			return SARState{}, fmt.Errorf("reading QMI SAR state: %w: %w", ErrNotSupported, err)
		}
		return SARState{}, fmt.Errorf("reading QMI SAR state: %w", err)
	}
	return SARState{Enabled: true, PowerLevel: uint32(state)}, nil
}

func isUnsupported(err error) bool {
	return errors.Is(err, qcom.QMIErrorNotSupported) ||
		errors.Is(err, qcom.QMIErrorDeviceUnsupported) ||
		errors.Is(err, qcom.QMIErrorInvalidQmiCommand)
}

func isSARUnsupported(err error) bool {
	return isUnsupported(err) || errors.Is(err, qcom.QMIErrorNoMemory)
}

func isSignalUnavailable(err error) bool {
	return errors.Is(err, qcom.QMIErrorInformationUnavailable) ||
		errors.Is(err, qcom.QMIErrorNoRadio) ||
		errors.Is(err, qcom.QMIErrorNoNetworkFound)
}

func (b *Backend) SetSAR(ctx context.Context, state SARState) error {
	if !state.Enabled {
		return errors.New("setting QMI SAR state: disabling SAR is not supported")
	}
	if err := b.client.SetSARRFState(ctx, qcom.SARRFState(state.PowerLevel)); err != nil {
		return fmt.Errorf("setting QMI SAR state: %w", err)
	}
	return nil
}

func (b *Backend) FirmwareUpdateInfo(ctx context.Context) (FirmwareUpdateInfo, error) {
	revision, err := b.client.RevisionInfo(ctx)
	if err != nil {
		return FirmwareUpdateInfo{}, fmt.Errorf("reading QMI firmware revision: %w", err)
	}
	return FirmwareUpdateInfo{Methods: []FirmwareUpdateMethod{FirmwareUpdateQDL}, Version: revision.Revision, Ports: []string{b.device}}, nil
}

func powerState(mode qcom.DMSOperatingMode) PowerState {
	switch mode {
	case qcom.DMSOperatingModeOnline:
		return PowerStateOn
	case qcom.DMSOperatingModeLowPower,
		qcom.DMSOperatingModePersistentLowPower,
		qcom.DMSOperatingModeModeOnlyLowPower:
		return PowerStateLow
	case qcom.DMSOperatingModeOffline, qcom.DMSOperatingModeShuttingDown:
		return PowerStateOff
	default:
		return PowerStateUnknown
	}
}

func technologyFromModePreference(preference qcom.NASModePreference) Technology {
	var result Technology
	if preference&qcom.NASModePreferenceGSM != 0 {
		result |= TechnologyGSM
	}
	if preference&qcom.NASModePreferenceUMTS != 0 {
		result |= TechnologyUMTS
	}
	if preference&qcom.NASModePreferenceLTE != 0 {
		result |= TechnologyLTE
	}
	if preference&qcom.NASModePreferenceNR5G != 0 {
		result |= TechnologyNR5GNSA | TechnologyNR5GSA
	}
	return result
}

func modePreferenceFromTechnology(technology Technology) qcom.NASModePreference {
	var result qcom.NASModePreference
	if technology&TechnologyGSM != 0 {
		result |= qcom.NASModePreferenceGSM
	}
	if technology&TechnologyUMTS != 0 {
		result |= qcom.NASModePreferenceUMTS
	}
	if technology&(TechnologyLTE|TechnologyLTECatM|TechnologyLTENB) != 0 {
		result |= qcom.NASModePreferenceLTE
	}
	if technology&(TechnologyNR5GNSA|TechnologyNR5GSA) != 0 {
		result |= qcom.NASModePreferenceNR5G
	}
	return result
}

func technologyFromNASRadios(radios []qcom.NASRadioInterface) Technology {
	var result Technology
	for _, radio := range radios {
		switch radio {
		case qcom.NASRadioInterfaceGSM:
			result |= TechnologyGSM
		case qcom.NASRadioInterfaceUMTS:
			result |= TechnologyUMTS
		case qcom.NASRadioInterfaceLTE:
			result |= TechnologyLTE
		case qcom.NASRadioInterfaceLTEM1:
			result |= TechnologyLTECatM
		case qcom.NASRadioInterfaceLTENB1:
			result |= TechnologyLTENB
		case qcom.NASRadioInterfaceNR5G:
			result |= TechnologyNR5GNSA | TechnologyNR5GSA
		}
	}
	return result
}

func simStateFromCardStatus(status qcom.CardStatus) SIMState {
	if len(status.Cards) == 0 || status.Cards[0].State == qcom.CardStateAbsent {
		return SIMStateAbsent
	}
	card := status.Cards[0]
	if card.State == qcom.CardStateError {
		return SIMStateFailure
	}
	for _, app := range card.Applications {
		switch app.State {
		case qcom.ApplicationStateReady:
			return SIMStateReady
		case qcom.ApplicationStatePIN1OrUPINRequired, qcom.ApplicationStatePUK1OrUPINRequired, qcom.ApplicationStateCheckPersonalization, qcom.ApplicationStatePIN1Blocked:
			return SIMStateLocked
		}
	}
	return SIMStateUnknown
}

func simStateFromPhysicalCardState(state qcom.PhysicalCardState) SIMState {
	switch state {
	case qcom.PhysicalCardStateAbsent:
		return SIMStateAbsent
	case qcom.PhysicalCardStatePresent:
		return SIMStateReady
	default:
		return SIMStateUnknown
	}
}

func registrationStateFromServingSystem(serving qcom.NASServingSystem) RegistrationState {
	switch serving.RegistrationState {
	case qcom.NASRegistrationNotRegistered:
		return RegistrationIdle
	case qcom.NASRegistrationSearching:
		return RegistrationSearching
	case qcom.NASRegistrationDenied:
		return RegistrationDenied
	case qcom.NASRegistrationRegistered:
		if serving.RoamingIndicatorKnown && serving.RoamingIndicator == qcom.NASRoamingIndicatorRoaming {
			return RegistrationRoaming
		}
		return RegistrationHome
	default:
		return RegistrationUnknown
	}
}

func packetServiceStateFromAttachState(state qcom.NASAttachState) PacketServiceState {
	switch state {
	case qcom.NASAttachAttached:
		return PacketServiceAttached
	case qcom.NASAttachDetached:
		return PacketServiceDetached
	default:
		return PacketServiceUnknown
	}
}

func technologyFromPLMNAccess(access qcom.NASPLMNAccessTechnology) Technology {
	var result Technology
	if access&(qcom.NASPLMNAccessGSM|qcom.NASPLMNAccessGSMCompact) != 0 {
		result |= TechnologyGSM
	}
	if access&qcom.NASPLMNAccessUTRAN != 0 {
		result |= TechnologyUMTS
	}
	if access&qcom.NASPLMNAccessEUTRAN != 0 {
		result |= TechnologyLTE
	}
	if access&qcom.NASPLMNAccessNGRAN != 0 {
		result |= TechnologyNR5GSA | TechnologyNR5GNSA
	}
	return result
}

func plmnAccessFromTechnology(technology Technology) qcom.NASPLMNAccessTechnology {
	if technology == 0 || technology == TechnologyAny {
		return qcom.NASPLMNAccessAll
	}
	var result qcom.NASPLMNAccessTechnology
	if technology&TechnologyGSM != 0 {
		result |= qcom.NASPLMNAccessGSM
	}
	if technology&TechnologyUMTS != 0 {
		result |= qcom.NASPLMNAccessUTRAN
	}
	if technology&(TechnologyLTE|TechnologyLTECatM|TechnologyLTENB) != 0 {
		result |= qcom.NASPLMNAccessEUTRAN
	}
	if technology&(TechnologyNR5GNSA|TechnologyNR5GSA) != 0 {
		result |= qcom.NASPLMNAccessNGRAN
	}
	return result
}

func nasRadioFromTechnology(technology Technology) qcom.NASRadioInterface {
	switch {
	case technology&TechnologyNR5GSA != 0:
		return qcom.NASRadioInterfaceNR5G
	case technology&TechnologyLTE != 0:
		return qcom.NASRadioInterfaceLTE
	case technology&TechnologyUMTS != 0:
		return qcom.NASRadioInterfaceUMTS
	case technology&TechnologyGSM != 0:
		return qcom.NASRadioInterfaceGSM
	default:
		return qcom.NASRadioInterfaceNoService
	}
}

func pdpTypeFromIPFamily(family IPFamily) qcom.WDSPDPType {
	switch family {
	case IPFamilyIPv6:
		return qcom.WDSPDPTypeIPv6
	case IPFamilyIPv4v6:
		return qcom.WDSPDPTypeIPv4v6
	default:
		return qcom.WDSPDPTypeIPv4
	}
}

func ipFamilyFromPDPType(pdp qcom.WDSPDPType) IPFamily {
	switch pdp {
	case qcom.WDSPDPTypeIPv6:
		return IPFamilyIPv6
	case qcom.WDSPDPTypeIPv4v6:
		return IPFamilyIPv4v6
	case qcom.WDSPDPTypeIPv4:
		return IPFamilyIPv4
	default:
		return IPFamilyUnknown
	}
}

func authenticationMask(authentication Authentication) qcom.WDSAuthenticationMask {
	var result qcom.WDSAuthenticationMask
	if authentication&AuthenticationPAP != 0 {
		result |= qcom.WDSAuthenticationPAP
	}
	if authentication&(AuthenticationCHAP|AuthenticationMSCHAPv2) != 0 {
		result |= qcom.WDSAuthenticationCHAP
	}
	return result
}

func authenticationFromMask(mask qcom.WDSAuthenticationMask) Authentication {
	var result Authentication
	if mask&qcom.WDSAuthenticationPAP != 0 {
		result |= AuthenticationPAP
	}
	if mask&qcom.WDSAuthenticationCHAP != 0 {
		result |= AuthenticationCHAP
	}
	return result
}

func profileFromSettings(settings qcom.WDSProfileSettings) Profile {
	return Profile{
		ID:             int32(settings.ID.Index),
		APN:            settings.APN,
		IPFamily:       ipFamilyFromPDPType(settings.PDPType),
		Authentication: authenticationFromMask(settings.Authentication),
		Username:       settings.Username,
		Password:       settings.Password,
		APNType:        apnTypeFromMask(settings.APNType),
		Enabled:        !settings.APNDisabled,
	}
}

func apnTypeMask(value APNType) qcom.WDSAPNTypeMask {
	var result qcom.WDSAPNTypeMask
	if value&APNTypeDefault != 0 {
		result |= qcom.WDSAPNTypeDefault
	}
	if value&APNTypeIMS != 0 {
		result |= qcom.WDSAPNTypeIMS
	}
	if value&APNTypeMMS != 0 {
		result |= qcom.WDSAPNTypeMMS
	}
	if value&APNTypeTethering != 0 {
		result |= qcom.WDSAPNTypeDUN
	}
	if value&APNTypeSUPL != 0 {
		result |= qcom.WDSAPNTypeSUPL
	}
	if value&APNTypeEmergency != 0 {
		result |= qcom.WDSAPNTypeEmergency
	}
	return result
}

func apnTypeFromMask(value qcom.WDSAPNTypeMask) APNType {
	var result APNType
	if value&qcom.WDSAPNTypeDefault != 0 {
		result |= APNTypeDefault
	}
	if value&qcom.WDSAPNTypeIMS != 0 {
		result |= APNTypeIMS
	}
	if value&qcom.WDSAPNTypeMMS != 0 {
		result |= APNTypeMMS
	}
	if value&qcom.WDSAPNTypeDUN != 0 {
		result |= APNTypeTethering
	}
	if value&qcom.WDSAPNTypeSUPL != 0 {
		result |= APNTypeSUPL
	}
	if value&qcom.WDSAPNTypeEmergency != 0 {
		result |= APNTypeEmergency
	}
	return result
}

func initialEPSFromProfile(profile Profile) InitialEPSConfig {
	return InitialEPSConfig{
		ProfileID:      profile.ID,
		APN:            profile.APN,
		IPFamily:       profile.IPFamily,
		Authentication: profile.Authentication,
		Username:       profile.Username,
		Password:       profile.Password,
	}
}

func knownSignal(db float64) SignalValue { return SignalValue{DB: db, Known: true} }

func signalQuality(radios []RadioSignal) uint8 {
	best := -113.0
	known := false
	for _, radio := range radios {
		for _, value := range []SignalValue{radio.RSSI, radio.RSRP, radio.RSCP} {
			if value.Known && (!known || value.DB > best) {
				best = value.DB
				known = true
			}
		}
	}
	if !known {
		return 0
	}
	quality := int((best + 113) * 100 / 62)
	return uint8(min(max(quality, 0), 100))
}
