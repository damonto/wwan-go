package qcom

import (
	"context"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasTLVRegisterSystemSelection = 0x10
	nasTLVRegisterOperatorName    = 0x11
	nasTLVRegisterServingSystem   = 0x13
	nasTLVRegisterNetworkTime     = 0x17
	nasTLVRegisterSystemInfo      = 0x18
	nasTLVRegisterSignalInfo      = 0x19
	nasTLVRegisterErrorRate       = 0x1A
	nasTLVRegisterCurrentPLMNName = 0x1E
	nasTLVRegisterRFBandInfo      = 0x20
	nasTLVRegisterNetworkReject   = 0x21
	nasTLVRegisterEDRXChange      = 0x35
)

type nasIndicationRegistration uint8

const (
	nasIndicationSystemSelection nasIndicationRegistration = iota
	nasIndicationOperatorName
	nasIndicationServingSystem
	nasIndicationNetworkTime
	nasIndicationSystemInfo
	nasIndicationSignalInfo
	nasIndicationErrorRate
	nasIndicationCurrentPLMNName
	nasIndicationRFBandInfo
	nasIndicationNetworkReject
	nasIndicationEDRXChange
)

// NASNetworkRejectIndicationConfig controls reject indications and whether the
// modem suppresses the related system-information indication.
type NASNetworkRejectIndicationConfig struct {
	Enabled            bool
	SuppressSystemInfo bool
}

// NASIndicationConfig selects common NAS indications for one control point.
// Nil fields are omitted, allowing one registration to change only one event.
type NASIndicationConfig struct {
	SystemSelection *bool
	OperatorName    *bool
	ServingSystem   *bool
	NetworkTime     *bool
	SystemInfo      *bool
	SignalInfo      *bool
	ErrorRate       *bool
	CurrentPLMNName *bool
	RFBandInfo      *bool
	NetworkReject   *NASNetworkRejectIndicationConfig
	EDRXChange      *bool
}

// NASIndicationRegisterRequest encodes NAS Indication Register.
type NASIndicationRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        NASIndicationConfig
}

// Request converts the request into a QMI NAS request.
func (r NASIndicationRegisterRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config.SystemSelection != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterSystemSelection, boolByte(*r.Config.SystemSelection)))
	}
	if r.Config.OperatorName != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterOperatorName, boolByte(*r.Config.OperatorName)))
	}
	if r.Config.ServingSystem != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterServingSystem, boolByte(*r.Config.ServingSystem)))
	}
	if r.Config.NetworkTime != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterNetworkTime, boolByte(*r.Config.NetworkTime)))
	}
	if r.Config.SystemInfo != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterSystemInfo, boolByte(*r.Config.SystemInfo)))
	}
	if r.Config.SignalInfo != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterSignalInfo, boolByte(*r.Config.SignalInfo)))
	}
	if r.Config.ErrorRate != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterErrorRate, boolByte(*r.Config.ErrorRate)))
	}
	if r.Config.CurrentPLMNName != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterCurrentPLMNName, boolByte(*r.Config.CurrentPLMNName)))
	}
	if r.Config.RFBandInfo != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterRFBandInfo, boolByte(*r.Config.RFBandInfo)))
	}
	if r.Config.NetworkReject != nil {
		tlvs = append(tlvs, tlv.Bytes(nasTLVRegisterNetworkReject, []byte{
			boolByte(r.Config.NetworkReject.Enabled),
			boolByte(r.Config.NetworkReject.SuppressSystemInfo),
		}))
	}
	if r.Config.EDRXChange != nil {
		tlvs = append(tlvs, tlv.Uint(nasTLVRegisterEDRXChange, boolByte(*r.Config.EDRXChange)))
	}
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASIndicationRegister,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// NASSetIndicationRegistration changes common NAS indication subscriptions.
func (c *Client) NASSetIndicationRegistration(ctx context.Context, config NASIndicationConfig) error {
	req := NASIndicationRegisterRequest{Timeout: DefaultRequestTimeout, Config: config}.Request()
	if err := c.nasReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("setting QMI NAS indication registration: %w", err)
	}
	return nil
}

// NASWatchServingSystem subscribes to registration and serving-cell changes.
func (c *Client) NASWatchServingSystem(ctx context.Context) (<-chan NASServingSystem, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASGetServingSystem, nasIndicationServingSystem)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS serving system: %w", err)
	}
	out := make(chan NASServingSystem, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASGetServingSystemResponse
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed.ServingSystem:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchOperatorName subscribes to legacy SIM and network operator-name data.
func (c *Client) NASWatchOperatorName(ctx context.Context) (<-chan NASOperatorNameData, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASOperatorName, nasIndicationOperatorName)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS operator-name data: %w", err)
	}
	out := make(chan NASOperatorNameData, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASOperatorNameData
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchSignalInfo subscribes to per-RAT signal measurement changes.
func (c *Client) NASWatchSignalInfo(ctx context.Context) (<-chan NASSignalInfo, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASSignalInfo, nasIndicationSignalInfo)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS signal information: %w", err)
	}
	out := make(chan NASSignalInfo, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASSignalInfo
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchErrorRate subscribes to RAT-specific legacy error-rate changes.
func (c *Client) NASWatchErrorRate(ctx context.Context) (<-chan NASErrorRateMeasurement, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASSetEventReport, nasIndicationErrorRate)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS error rate: %w", err)
	}
	out := make(chan NASErrorRateMeasurement, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var event NASEventReport
			if err := event.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			if !event.ErrorRateKnown {
				continue
			}
			select {
			case out <- event.ErrorRate:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchSystemInfo subscribes to service, serving-cell, and voice/SMS domain changes.
func (c *Client) NASWatchSystemInfo(ctx context.Context) (<-chan NASSysInfo, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASSysInfo, nasIndicationSystemInfo)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS system information: %w", err)
	}
	out := make(chan NASSysInfo, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASSysInfo
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchNetworkTime subscribes to network-provided civil-time changes.
func (c *Client) NASWatchNetworkTime(ctx context.Context) (<-chan NASNetworkTimeUpdate, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASNetworkTime, nasIndicationNetworkTime)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS network time: %w", err)
	}
	out := make(chan NASNetworkTimeUpdate, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASNetworkTimeUpdate
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchSystemSelection subscribes to modem selection-policy changes.
func (c *Client) NASWatchSystemSelection(ctx context.Context) (<-chan NASSystemSelectionPreference, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASGetSystemSelectionPreference, nasIndicationSystemSelection)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS system selection: %w", err)
	}
	out := make(chan NASSystemSelectionPreference, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASSystemSelectionPreference
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchCurrentPLMNName subscribes to current operator-name changes.
func (c *Client) NASWatchCurrentPLMNName(ctx context.Context) (<-chan NASCurrentPLMNName, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASCurrentPLMNName, nasIndicationCurrentPLMNName)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS current PLMN name: %w", err)
	}
	out := make(chan NASCurrentPLMNName, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASCurrentPLMNName
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchRFBandInfo subscribes to current radio-band changes.
func (c *Client) NASWatchRFBandInfo(ctx context.Context) (<-chan NASRFBandInfo, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASRFBandInfo, nasIndicationRFBandInfo)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS RF band information: %w", err)
	}
	out := make(chan NASRFBandInfo, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASRFBandInfo
			if err := parsed.UnmarshalIndicationTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchEDRXParameters subscribes to negotiated eDRX parameter changes.
func (c *Client) NASWatchEDRXParameters(ctx context.Context) (<-chan NASEDRXParameters, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASEDRXChangeInfo, nasIndicationEDRXChange)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS eDRX parameters: %w", err)
	}
	out := make(chan NASEDRXParameters, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASEDRXParameters
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// NASWatchNetworkReject subscribes to network registration rejection details.
func (c *Client) NASWatchNetworkReject(ctx context.Context) (<-chan NASNetworkReject, error) {
	raw, err := c.watchNASTLVs(ctx, MessageNASNetworkReject, nasIndicationNetworkReject)
	if err != nil {
		return nil, fmt.Errorf("watching QMI NAS network rejections: %w", err)
	}
	out := make(chan NASNetworkReject, 8)
	go func() {
		defer close(out)
		for tlvs := range raw {
			var parsed NASNetworkReject
			if err := parsed.UnmarshalTLVs(tlvs); err != nil {
				return
			}
			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) watchNASTLVs(
	ctx context.Context,
	id MessageID,
	registration nasIndicationRegistration,
) (<-chan tlv.TLVs, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceNAS)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceNAS, clientID, id)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := c.acquireNASIndication(ctx, registration); err != nil {
		cancel()
		return nil, err
	}
	out := make(chan tlv.TLVs, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releaseNASIndication(registration)
		for indication := range indications {
			select {
			case out <- indication.TLVs:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) acquireNASIndication(ctx context.Context, registration nasIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.nasIndicationRefs == nil {
		c.nasIndicationRefs = make(map[nasIndicationRegistration]int)
	}
	if c.nasIndicationRefs[registration] > 0 {
		c.nasIndicationRefs[registration]++
		return nil
	}
	c.nasIndicationRefs[registration] = 1
	if err := c.setNASIndication(ctx, registration, true); err != nil {
		delete(c.nasIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releaseNASIndication(registration nasIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.nasIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.nasIndicationRefs[registration]--
		return
	}
	delete(c.nasIndicationRefs, registration)
	_ = c.setNASIndication(ctx, registration, false)
}

func (c *Client) setNASIndication(ctx context.Context, registration nasIndicationRegistration, enabled bool) error {
	config := NASIndicationConfig{}
	switch registration {
	case nasIndicationSystemSelection:
		config.SystemSelection = &enabled
	case nasIndicationOperatorName:
		config.OperatorName = &enabled
	case nasIndicationServingSystem:
		config.ServingSystem = &enabled
	case nasIndicationNetworkTime:
		config.NetworkTime = &enabled
	case nasIndicationSystemInfo:
		config.SystemInfo = &enabled
	case nasIndicationSignalInfo:
		config.SignalInfo = &enabled
	case nasIndicationErrorRate:
		config.ErrorRate = &enabled
	case nasIndicationCurrentPLMNName:
		config.CurrentPLMNName = &enabled
	case nasIndicationRFBandInfo:
		config.RFBandInfo = &enabled
	case nasIndicationNetworkReject:
		config.NetworkReject = &NASNetworkRejectIndicationConfig{Enabled: enabled}
	case nasIndicationEDRXChange:
		config.EDRXChange = &enabled
	default:
		return fmt.Errorf("setting QMI NAS indication: registration %d is out of range", registration)
	}
	return c.NASSetIndicationRegistration(ctx, config)
}
