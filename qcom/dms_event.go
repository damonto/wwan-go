package qcom

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVReportPowerState   = 0x10
	dmsTLVEventPowerState    = 0x10
	dmsTLVBatteryLevelLimits = 0x11
	dmsTLVEventPIN1Status    = 0x11
	dmsTLVReportPINState     = 0x12
	dmsTLVEventPIN2Status    = 0x12
	dmsTLVReportActivation   = 0x13
	dmsTLVEventActivation    = 0x13
	dmsTLVEventOperatingMode = 0x14
	dmsTLVReportUIMState     = 0x15
	dmsTLVEventUIMState      = 0x15
	dmsTLVReportWireless     = 0x16
	dmsTLVEventWireless      = 0x16
	dmsTLVReportPRLInit      = 0x17
	dmsTLVEventPRLInit       = 0x17
)

// DMSBatteryLevelLimits selects thresholds that trigger power-state events.
type DMSBatteryLevelLimits struct {
	Lower uint8
	Upper uint8
}

// DMSEventReportConfig updates selected DMS event-report settings. Nil fields
// are omitted so callers do not overwrite settings they do not own.
type DMSEventReportConfig struct {
	PowerState         *bool
	BatteryLevelLimits *DMSBatteryLevelLimits
	PINState           *bool
	ActivationState    *bool
	OperatingMode      *bool
	UIMState           *bool
	WirelessDisable    *bool
	PRLInit            *bool
}

// DMSPINStatus identifies the current state of a legacy DMS UIM PIN.
type DMSPINStatus uint8

const (
	DMSPINStatusNotInitialized DMSPINStatus = iota
	DMSPINStatusEnabledNotVerified
	DMSPINStatusEnabledVerified
	DMSPINStatusDisabled
	DMSPINStatusBlocked
	DMSPINStatusPermanentlyBlocked
	DMSPINStatusUnblocked
	DMSPINStatusChanged
)

// DMSPINState contains a PIN state and its remaining retry counts.
type DMSPINState struct {
	Status         DMSPINStatus
	VerifyRetries  uint8
	UnblockRetries uint8
}

// DMSActivationState identifies the CDMA service activation state.
type DMSActivationState uint16

const (
	DMSActivationNotActivated DMSActivationState = iota
	DMSActivationActivated
	DMSActivationConnecting
	DMSActivationConnected
	DMSActivationOTASPSecurityAuthenticated
	DMSActivationOTASPNAMDownloaded
	DMSActivationOTASPMDNDownloaded
	DMSActivationOTASPIMSIDownloaded
	DMSActivationOTASPPRLDownloaded
	DMSActivationOTASPSPCDownloaded
	DMSActivationOTASPSettingsCommitted
)

// DMSUIMState identifies the initialization state of the legacy DMS UIM view.
type DMSUIMState uint8

const (
	DMSUIMStateInitializationCompleted DMSUIMState = iota
	DMSUIMStateInitializationFailed
	DMSUIMStateNotPresent
	DMSUIMStateUnavailable DMSUIMState = 0xFF
)

// DMSEvent contains the state fields present in one DMS Event Report
// indication. Known fields distinguish an omitted TLV from a zero value.
type DMSEvent struct {
	PowerState            DMSPowerState
	PowerStateKnown       bool
	PIN1                  DMSPINState
	PIN1Known             bool
	PIN2                  DMSPINState
	PIN2Known             bool
	ActivationState       DMSActivationState
	ActivationStateKnown  bool
	OperatingMode         DMSOperatingMode
	OperatingModeKnown    bool
	UIMState              DMSUIMState
	UIMStateKnown         bool
	WirelessDisabled      bool
	WirelessDisabledKnown bool
	PRLInitialized        bool
	PRLInitializedKnown   bool
}

// UnmarshalTLVs parses a QMI DMS Event Report indication.
func (e *DMSEvent) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*e = DMSEvent{}
	if value, ok := tlv.Value(tlvs, dmsTLVEventPowerState); ok {
		if err := requireDMSEventLength(value, 2); err != nil {
			return fmt.Errorf("parsing QMI DMS event power state: %w", err)
		}
		e.PowerState = DMSPowerState{Status: DMSPowerStatus(value[0]), BatteryLevel: value[1]}
		e.PowerStateKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventPIN1Status); ok {
		if err := e.PIN1.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI DMS event PIN1 status: %w", err)
		}
		e.PIN1Known = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventPIN2Status); ok {
		if err := e.PIN2.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI DMS event PIN2 status: %w", err)
		}
		e.PIN2Known = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventActivation); ok {
		if err := requireDMSEventLength(value, 2); err != nil {
			return fmt.Errorf("parsing QMI DMS event activation state: %w", err)
		}
		e.ActivationState = DMSActivationState(binary.LittleEndian.Uint16(value))
		e.ActivationStateKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventOperatingMode); ok {
		if err := requireDMSEventLength(value, 1); err != nil {
			return fmt.Errorf("parsing QMI DMS event operating mode: %w", err)
		}
		e.OperatingMode = DMSOperatingMode(value[0])
		e.OperatingModeKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventUIMState); ok {
		if err := requireDMSEventLength(value, 1); err != nil {
			return fmt.Errorf("parsing QMI DMS event UIM state: %w", err)
		}
		e.UIMState = DMSUIMState(value[0])
		e.UIMStateKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventWireless); ok {
		if err := requireDMSEventLength(value, 1); err != nil {
			return fmt.Errorf("parsing QMI DMS event wireless disable state: %w", err)
		}
		e.WirelessDisabled = value[0] != 0
		e.WirelessDisabledKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVEventPRLInit); ok {
		if err := requireDMSEventLength(value, 1); err != nil {
			return fmt.Errorf("parsing QMI DMS event PRL initialization: %w", err)
		}
		e.PRLInitialized = value[0] != 0
		e.PRLInitializedKnown = true
	}
	return nil
}

// DMSSetEventReport configures DMS state-change indications.
func (c *Client) DMSSetEventReport(ctx context.Context, config DMSEventReportConfig) error {
	if limits := config.BatteryLevelLimits; limits != nil {
		if limits.Lower > 100 || limits.Upper > 100 {
			return fmt.Errorf("configuring QMI DMS event reports: battery limits %d..%d exceed 100", limits.Lower, limits.Upper)
		}
		if limits.Lower > limits.Upper {
			return fmt.Errorf("configuring QMI DMS event reports: lower battery limit %d exceeds upper limit %d", limits.Lower, limits.Upper)
		}
	}

	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		req := DMSSetEventReportRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   &config,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("configuring QMI DMS event reports: %w", err)
	}
	return nil
}

// DMSWatchEvents subscribes to common DMS power, UIM, activation, operating
// mode, hardware-radio-switch, and PRL initialization changes.
func (c *Client) DMSWatchEvents(ctx context.Context) (<-chan DMSEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceDMS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI DMS events: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceDMS, clientID, MessageDMSSetEventReport)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI DMS events: %w", err)
	}
	if err := c.acquireDMSEvents(ctx); err != nil {
		cancel()
		return nil, err
	}

	out := make(chan DMSEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseDMSEvents()
		for indication := range indications {
			var event DMSEvent
			if err := event.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- event:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func dmsEventReportTLVs(config DMSEventReportConfig) tlv.TLVs {
	var tlvs tlv.TLVs
	if config.PowerState != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportPowerState, boolByte(*config.PowerState)))
	}
	if config.BatteryLevelLimits != nil {
		tlvs = append(tlvs, tlv.Bytes(dmsTLVBatteryLevelLimits, []byte{
			config.BatteryLevelLimits.Lower,
			config.BatteryLevelLimits.Upper,
		}))
	}
	if config.PINState != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportPINState, boolByte(*config.PINState)))
	}
	if config.ActivationState != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportActivation, boolByte(*config.ActivationState)))
	}
	if config.OperatingMode != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportOperatingMode, boolByte(*config.OperatingMode)))
	}
	if config.UIMState != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportUIMState, boolByte(*config.UIMState)))
	}
	if config.WirelessDisable != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportWireless, boolByte(*config.WirelessDisable)))
	}
	if config.PRLInit != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVReportPRLInit, boolByte(*config.PRLInit)))
	}
	return tlvs
}

func (state DMSPINState) MarshalBinary() ([]byte, error) {
	return []byte{byte(state.Status), state.VerifyRetries, state.UnblockRetries}, nil
}

func (state *DMSPINState) UnmarshalBinary(value []byte) error {
	if err := requireDMSEventLength(value, 3); err != nil {
		return err
	}
	*state = DMSPINState{
		Status:         DMSPINStatus(value[0]),
		VerifyRetries:  value[1],
		UnblockRetries: value[2],
	}
	return nil
}

func requireDMSEventLength(value []byte, want int) error {
	if len(value) != want {
		return fmt.Errorf("TLV length %d, want %d", len(value), want)
	}
	return nil
}

func (c *Client) acquireDMSEvents(ctx context.Context) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.dmsEventRefs > 0 {
		c.dmsEventRefs++
		return nil
	}
	if err := c.setDMSEvents(ctx, true); err != nil {
		return err
	}
	c.dmsEventRefs = 1
	return nil
}

func (c *Client) releaseDMSEvents() {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.dmsEventRefs == 0 {
		return
	}
	if c.dmsEventRefs > 1 {
		c.dmsEventRefs--
		return
	}
	c.dmsEventRefs = 0
	_ = c.setDMSEvents(ctx, false)
}

func (c *Client) setDMSEvents(ctx context.Context, enabled bool) error {
	return c.DMSSetEventReport(ctx, DMSEventReportConfig{
		PowerState:      &enabled,
		PINState:        &enabled,
		ActivationState: &enabled,
		OperatingMode:   &enabled,
		UIMState:        &enabled,
		WirelessDisable: &enabled,
		PRLInit:         &enabled,
	})
}
