package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	pdcConfigIDMax          = 124
	pdcConfigListMax        = 25
	pdcConfigDescriptionMax = 255
	pdcConfigPathMax        = 255
	pdcConfigHeaderMax      = 255
	pdcListEmptyTimeout     = 2 * time.Second
)

type pdcIndicationRegistration uint8

const (
	pdcIndicationConfigChange pdcIndicationRegistration = iota
	pdcIndicationClientRefresh
)

// PDCConfigurationType identifies a persistent modem configuration store.
type PDCConfigurationType uint32

const (
	PDCConfigurationPlatform PDCConfigurationType = 0x00
	PDCConfigurationSoftware PDCConfigurationType = 0x01
	PDCConfigurationDatabase PDCConfigurationType = 0x10
)

// PDCActivationType selects a normal activation or a refresh-only operation.
type PDCActivationType uint32

const (
	PDCActivationRegular PDCActivationType = iota
	PDCActivationRefreshOnly
)

// PDCStorage identifies where a configuration is stored.
type PDCStorage uint32

const (
	PDCStorageLocal PDCStorage = iota
	PDCStorageRemote
)

// PDCRefreshEventType identifies one modem configuration refresh stage.
type PDCRefreshEventType uint32

const (
	PDCRefreshStart PDCRefreshEventType = iota
	PDCRefreshComplete
	PDCRefreshClient
)

// PDCConfig identifies one persistent modem configuration.
type PDCConfig struct {
	Type PDCConfigurationType
	ID   []byte
}

// PDCIndicationConfig updates PDC indication registration. ConfigChange is
// always sent because QMI PDC requires that setting in every register request.
type PDCIndicationConfig struct {
	ConfigChange  bool
	ClientRefresh *bool
}

// PDCRegisterRequest encodes QMI PDC Indication Register.
type PDCRegisterRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDCIndicationConfig
}

// Request converts the indication settings into a QMI request.
func (r PDCRegisterRequest) Request() Request {
	tlvs := tlv.TLVs{tlv.Uint(0x10, boolByte(r.Config.ConfigChange))}
	if r.Config.ClientRefresh != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*r.Config.ClientRefresh)))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCRegister,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// PDCGetSelectedConfigRequest encodes QMI PDC Get Selected Config.
type PDCGetSelectedConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          PDCConfigurationType
	Token         *uint32
	Subscription  *uint32
	Slot          *uint32
}

// Request validates and converts the selected-configuration query.
func (r PDCGetSelectedConfigRequest) Request() (Request, error) {
	if err := validatePDCConfigurationType(r.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC selected configuration query: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Type))}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	if r.Subscription != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *r.Subscription))
	}
	if r.Slot != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, *r.Slot))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCGetSelectedConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCSetSelectedConfigRequest encodes QMI PDC Set Selected Config.
type PDCSetSelectedConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDCConfig
	Token         *uint32
	Subscription  *uint32
	Slot          *uint32
}

// Request validates and converts the new selected configuration.
func (r PDCSetSelectedConfigRequest) Request() (Request, error) {
	config, err := r.Config.MarshalBinary()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC selected configuration: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, config)}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	if r.Subscription != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *r.Subscription))
	}
	if r.Slot != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, *r.Slot))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCSetSelectedConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCListConfigsRequest encodes QMI PDC List Configs.
type PDCListConfigsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Token         *uint32
	Type          PDCConfigurationType
	MultiSupport  *bool
}

// Request validates and converts the configuration-list query.
func (r PDCListConfigsRequest) Request() (Request, error) {
	if err := validatePDCConfigurationType(r.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration list: %w", err)
	}
	var tlvs tlv.TLVs
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	tlvs = append(tlvs, tlv.Uint(0x11, uint32(r.Type)))
	if r.MultiSupport != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, boolByte(*r.MultiSupport)))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCListConfigs,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCActivateConfigRequest encodes QMI PDC Activate Config.
type PDCActivateConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          PDCConfigurationType
	Token         *uint32
	Activation    *PDCActivationType
	Subscription  *uint32
	Slot          *uint32
}

// Request validates and converts the activation request.
func (r PDCActivateConfigRequest) Request() (Request, error) {
	if err := validatePDCConfigurationType(r.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration activation: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Type))}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	if r.Activation != nil {
		if *r.Activation > PDCActivationRefreshOnly {
			return Request{}, fmt.Errorf("encoding QMI PDC configuration activation: activation type %d is out of range", *r.Activation)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint32(*r.Activation)))
	}
	if r.Subscription != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, *r.Subscription))
	}
	if r.Slot != nil {
		tlvs = append(tlvs, tlv.Uint(0x13, *r.Slot))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCActivateConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCGetConfigInfoRequest encodes QMI PDC Get Config Info.
type PDCGetConfigInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDCConfig
	Token         *uint32
}

// Request validates and converts the configuration-information query.
func (r PDCGetConfigInfoRequest) Request() (Request, error) {
	config, err := r.Config.MarshalBinary()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration info query: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, config)}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCGetConfigInfo,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCIndicationResult is the common asynchronous PDC completion status.
type PDCIndicationResult struct {
	Error      QMIError
	Token      uint32
	TokenKnown bool
}

// Err returns the modem-reported operation error.
func (r PDCIndicationResult) Err() error {
	if r.Error == QMIErrorNone {
		return nil
	}
	return r.Error
}

// PDCOperationIndication is the common Set Selected and Activate indication.
type PDCOperationIndication struct {
	Result PDCIndicationResult
}

// PDCSetSelectedConfigIndication is a Set Selected Config completion.
type PDCSetSelectedConfigIndication = PDCOperationIndication

// PDCActivateConfigIndication is an Activate Config completion.
type PDCActivateConfigIndication = PDCOperationIndication

// UnmarshalTLVs parses the common PDC operation result and token.
func (i *PDCOperationIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCOperationIndication{}
	return i.Result.UnmarshalTLVs(tlvs)
}

// PDCSelectedConfig contains active and pending configuration IDs.
type PDCSelectedConfig struct {
	Active       []byte
	ActiveKnown  bool
	Pending      []byte
	PendingKnown bool
}

// PDCGetSelectedConfigIndication is the asynchronous selected-config result.
type PDCGetSelectedConfigIndication struct {
	Result   PDCIndicationResult
	Selected PDCSelectedConfig
}

// UnmarshalTLVs parses selected configuration IDs and completion metadata.
func (i *PDCGetSelectedConfigIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCGetSelectedConfigIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		var id qmiLength8Bytes
		if err := id.UnmarshalBinary(value); err != nil {
			return err
		}
		if len(id) > pdcConfigIDMax {
			return fmt.Errorf("ID length %d exceeds %d", len(id), pdcConfigIDMax)
		}
		i.Selected.Active = id
		i.Selected.ActiveKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		var id qmiLength8Bytes
		if err := id.UnmarshalBinary(value); err != nil {
			return err
		}
		if len(id) > pdcConfigIDMax {
			return fmt.Errorf("ID length %d exceeds %d", len(id), pdcConfigIDMax)
		}
		i.Selected.Pending = id
		i.Selected.PendingKnown = true
	}
	return nil
}

// PDCListConfigsIndication is one configuration-list result frame.
type PDCListConfigsIndication struct {
	Result             PDCIndicationResult
	Configs            []PDCConfig
	ConfigsKnown       bool
	MoreAvailable      bool
	MoreAvailableKnown bool
}

// UnmarshalTLVs parses a PDC configuration-list indication.
func (i *PDCListConfigsIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCListConfigsIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		configs, err := decodePDCConfigList(value)
		if err != nil {
			return err
		}
		i.Configs = configs
		i.ConfigsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI PDC configuration list: more-available TLV length %d, want 1", len(value))
		}
		i.MoreAvailable = value[0] != 0
		i.MoreAvailableKnown = true
	}
	return nil
}

// PDCConfigInfo contains metadata returned for one persistent configuration.
type PDCConfigInfo struct {
	Size             uint32
	SizeKnown        bool
	Description      string
	DescriptionKnown bool
	Version          uint32
	VersionKnown     bool
	Storage          PDCStorage
	StorageKnown     bool
	Path             string
	PathKnown        bool
	BaseVersion      uint32
	BaseVersionKnown bool
	Header           []byte
	HeaderKnown      bool
}

// PDCGetConfigInfoIndication is the asynchronous configuration-info result.
type PDCGetConfigInfoIndication struct {
	Result PDCIndicationResult
	Info   PDCConfigInfo
}

// UnmarshalTLVs parses configuration metadata and completion information.
func (i *PDCGetConfigInfoIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCGetConfigInfoIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.Size, i.Info.SizeKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		var parsed qmiLength8Bytes
		if err := parsed.UnmarshalBinary(value); err != nil {
			return err
		}
		if len(parsed) > pdcConfigDescriptionMax {
			return fmt.Errorf("value length %d exceeds %d", len(parsed), pdcConfigDescriptionMax)
		}
		i.Info.Description, i.Info.DescriptionKnown = string(parsed), true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.Version, i.Info.VersionKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.Storage, i.Info.StorageKnown = PDCStorage(parsed), true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		var path pdcUTF16
		if err := path.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI PDC configuration path: %w", err)
		}
		i.Info.Path, i.Info.PathKnown = path.String(), true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.BaseVersion, i.Info.BaseVersionKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		var parsed qmiLength8Bytes
		if err := parsed.UnmarshalBinary(value); err != nil {
			return err
		}
		if len(parsed) > pdcConfigHeaderMax {
			return fmt.Errorf("value length %d exceeds %d", len(parsed), pdcConfigHeaderMax)
		}
		i.Info.Header, i.Info.HeaderKnown = parsed, true
	}
	return nil
}

// PDCRefreshEvent describes one PDC refresh indication.
type PDCRefreshEvent struct {
	Type              PDCRefreshEventType
	Subscription      uint32
	SubscriptionKnown bool
	Slot              uint32
	SlotKnown         bool
}

// PDCRefreshIndication is a parsed PDC Refresh indication.
type PDCRefreshIndication struct {
	Event PDCRefreshEvent
}

// UnmarshalTLVs parses a PDC refresh event.
func (i *PDCRefreshIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCRefreshIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDC refresh: event TLV missing")
	}
	event, err := decodePDCUint32(value)
	if err != nil {
		return err
	}
	i.Event.Type = PDCRefreshEventType(event)
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Event.Subscription, i.Event.SubscriptionKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Event.Slot, i.Event.SlotKnown = parsed, true
	}
	return nil
}

// PDCConfigChangeIndication reports a newly selected configuration.
type PDCConfigChangeIndication struct {
	Config PDCConfig
}

// UnmarshalTLVs parses a PDC Config Change indication.
func (i *PDCConfigChangeIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCConfigChangeIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDC configuration change: configuration TLV missing")
	}
	var config PDCConfig
	if err := config.UnmarshalBinary(value); err != nil {
		return fmt.Errorf("parsing QMI PDC configuration change: %w", err)
	}
	i.Config = config
	return nil
}

// PDCSelectionQuery selects the subscription and slot whose active and
// pending configuration IDs should be queried.
type PDCSelectionQuery struct {
	Type         PDCConfigurationType
	Subscription *uint32
	Slot         *uint32
}

// PDCConfigSelection identifies a configuration to make pending.
type PDCConfigSelection struct {
	Config       PDCConfig
	Subscription *uint32
	Slot         *uint32
}

// PDCConfigActivation describes a pending-configuration activation.
type PDCConfigActivation struct {
	Type         PDCConfigurationType
	Activation   *PDCActivationType
	Subscription *uint32
	Slot         *uint32
}

// PDCRegister updates selected PDC indication settings.
func (c *Client) PDCRegister(ctx context.Context, config PDCIndicationConfig) error {
	c.pdcMu.Lock()
	defer c.pdcMu.Unlock()

	err := c.withServiceClient(ctx, ServicePDC, func(clientID uint8) error {
		req := PDCRegisterRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   config,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("registering QMI PDC indications: %w", err)
	}
	return nil
}

// PDCSelectedConfiguration returns active and pending configuration IDs.
func (c *Client) PDCSelectedConfiguration(ctx context.Context, query PDCSelectionQuery) (PDCSelectedConfig, error) {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCGetSelectedConfig, func(clientID uint8, token uint32) (Request, error) {
		return (PDCGetSelectedConfigRequest{
			ClientID:     clientID,
			Timeout:      DefaultRequestTimeout,
			Type:         query.Type,
			Token:        &token,
			Subscription: query.Subscription,
			Slot:         query.Slot,
		}).Request()
	})
	if err != nil {
		return PDCSelectedConfig{}, fmt.Errorf("reading QMI PDC selected configuration: %w", err)
	}
	var indication PDCGetSelectedConfigIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return PDCSelectedConfig{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return PDCSelectedConfig{}, fmt.Errorf("reading QMI PDC selected configuration: %w", err)
	}
	return indication.Selected, nil
}

// PDCSelectConfiguration makes one stored configuration pending.
func (c *Client) PDCSelectConfiguration(ctx context.Context, selection PDCConfigSelection) error {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCSetSelectedConfig, func(clientID uint8, token uint32) (Request, error) {
		return (PDCSetSelectedConfigRequest{
			ClientID:     clientID,
			Timeout:      DefaultRequestTimeout,
			Config:       selection.Config,
			Token:        &token,
			Subscription: selection.Subscription,
			Slot:         selection.Slot,
		}).Request()
	})
	if err != nil {
		return fmt.Errorf("selecting QMI PDC configuration: %w", err)
	}
	var indication PDCSetSelectedConfigIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return err
	}
	if err := indication.Result.Err(); err != nil {
		return fmt.Errorf("selecting QMI PDC configuration: %w", err)
	}
	return nil
}

// PDCActivateConfiguration activates the pending configuration for a store.
// A regular activation may reboot the modem before it emits its completion
// indication; callers should treat transport removal according to device
// lifecycle policy.
func (c *Client) PDCActivateConfiguration(ctx context.Context, activation PDCConfigActivation) error {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCActivateConfig, func(clientID uint8, token uint32) (Request, error) {
		return (PDCActivateConfigRequest{
			ClientID:     clientID,
			Timeout:      DefaultRequestTimeout,
			Type:         activation.Type,
			Token:        &token,
			Activation:   activation.Activation,
			Subscription: activation.Subscription,
			Slot:         activation.Slot,
		}).Request()
	})
	if err != nil {
		return fmt.Errorf("activating QMI PDC configuration: %w", err)
	}
	var indication PDCActivateConfigIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return err
	}
	if err := indication.Result.Err(); err != nil {
		return fmt.Errorf("activating QMI PDC configuration: %w", err)
	}
	return nil
}

// PDCConfigurationInfo returns metadata for one stored configuration.
func (c *Client) PDCConfigurationInfo(ctx context.Context, config PDCConfig) (PDCConfigInfo, error) {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCGetConfigInfo, func(clientID uint8, token uint32) (Request, error) {
		return (PDCGetConfigInfoRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   config,
			Token:    &token,
		}).Request()
	})
	if err != nil {
		return PDCConfigInfo{}, fmt.Errorf("reading QMI PDC configuration info: %w", err)
	}
	var indication PDCGetConfigInfoIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return PDCConfigInfo{}, err
	}
	if err := indication.Result.Err(); err != nil {
		return PDCConfigInfo{}, fmt.Errorf("reading QMI PDC configuration info: %w", err)
	}
	return indication.Info, nil
}

// PDCConfigurations lists stored configurations of the requested type.
func (c *Client) PDCConfigurations(ctx context.Context, configType PDCConfigurationType) ([]PDCConfig, error) {
	c.pdcMu.Lock()
	defer c.pdcMu.Unlock()

	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("listing QMI PDC configurations: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServicePDC)
	if err != nil {
		return nil, fmt.Errorf("listing QMI PDC configurations: %w", err)
	}
	token := c.nextPDCTokenLocked()
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	indications, err := transport.Indications(waitCtx, ServicePDC, clientID, MessagePDCListConfigs)
	if err != nil {
		return nil, fmt.Errorf("listing QMI PDC configurations: subscribe result: %w", err)
	}
	multiSupport := true
	req, err := (PDCListConfigsRequest{
		ClientID:     clientID,
		Timeout:      DefaultRequestTimeout,
		Token:        &token,
		Type:         configType,
		MultiSupport: &multiSupport,
	}).Request()
	if err != nil {
		return nil, err
	}
	resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return nil, fmt.Errorf("listing QMI PDC configurations: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return nil, fmt.Errorf("listing QMI PDC configurations: %w", err)
	}

	var configs []PDCConfig
	emptyTimer := time.NewTimer(pdcListEmptyTimeout)
	defer emptyTimer.Stop()
	emptyTimeout := emptyTimer.C
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("listing QMI PDC configurations: %w", ctx.Err())
		case <-emptyTimeout:
			return configs, nil
		case indication, ok := <-indications:
			if !ok {
				return nil, errors.New("listing QMI PDC configurations: indication stream closed")
			}
			var parsed PDCListConfigsIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return nil, err
			}
			if parsed.Result.TokenKnown && parsed.Result.Token != token {
				continue
			}
			if emptyTimeout != nil {
				emptyTimer.Stop()
				emptyTimeout = nil
			}
			if err := parsed.Result.Err(); err != nil {
				return nil, fmt.Errorf("listing QMI PDC configurations: %w", err)
			}
			configs = append(configs, parsed.Configs...)
			if !parsed.MoreAvailableKnown || !parsed.MoreAvailable {
				return configs, nil
			}
		}
	}
}

// PDCWatchRefresh subscribes to modem configuration refresh events.
func (c *Client) PDCWatchRefresh(ctx context.Context) (<-chan PDCRefreshEvent, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDC)
	if err != nil {
		return nil, fmt.Errorf("watching QMI PDC refresh events: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServicePDC, clientID, MessagePDCRefresh)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDC refresh events: %w", err)
	}
	if err := c.acquirePDCIndication(ctx, pdcIndicationClientRefresh); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDC refresh events: %w", err)
	}

	out := make(chan PDCRefreshEvent, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releasePDCIndication(pdcIndicationClientRefresh)
		for indication := range indications {
			var parsed PDCRefreshIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Event:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

// PDCWatchConfigChanges subscribes to selected-configuration changes.
func (c *Client) PDCWatchConfigChanges(ctx context.Context) (<-chan PDCConfig, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDC)
	if err != nil {
		return nil, fmt.Errorf("watching QMI PDC configuration changes: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServicePDC, clientID, MessagePDCConfigChange)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDC configuration changes: %w", err)
	}
	if err := c.acquirePDCIndication(ctx, pdcIndicationConfigChange); err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI PDC configuration changes: %w", err)
	}

	out := make(chan PDCConfig, 8)
	go func() {
		defer close(out)
		defer cancel()
		defer c.releasePDCIndication(pdcIndicationConfigChange)
		for indication := range indications {
			var parsed PDCConfigChangeIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Config:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) pdcSingleIndication(
	ctx context.Context,
	message MessageID,
	build func(uint8, uint32) (Request, error),
) (tlv.TLVs, error) {
	c.pdcMu.Lock()
	defer c.pdcMu.Unlock()

	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDC)
	if err != nil {
		return nil, err
	}
	token := c.nextPDCTokenLocked()
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	indications, err := transport.Indications(waitCtx, ServicePDC, clientID, message)
	if err != nil {
		return nil, fmt.Errorf("subscribing QMI PDC result: %w", err)
	}
	req, err := build(clientID, token)
	if err != nil {
		return nil, err
	}
	resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
	if err != nil {
		return nil, err
	}
	if err := resultOK(resp); err != nil {
		return nil, err
	}
	return waitPDCIndication(ctx, indications, token)
}

func waitPDCIndication(ctx context.Context, indications <-chan Indication, token uint32) (tlv.TLVs, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case indication, ok := <-indications:
			if !ok {
				return nil, errors.New("QMI PDC indication stream closed")
			}
			gotToken, known, err := decodePDCIndicationToken(indication.TLVs)
			if err != nil {
				return nil, err
			}
			if known && gotToken != token {
				continue
			}
			return indication.TLVs, nil
		}
	}
}

func (c *Client) nextPDCTokenLocked() uint32 {
	c.pdcToken++
	if c.pdcToken == 0 {
		c.pdcToken++
	}
	return c.pdcToken
}

func (c *Client) acquirePDCIndication(ctx context.Context, registration pdcIndicationRegistration) error {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.pdcIndicationRefs == nil {
		c.pdcIndicationRefs = make(map[pdcIndicationRegistration]int)
	}
	if c.pdcIndicationRefs[registration] > 0 {
		c.pdcIndicationRefs[registration]++
		return nil
	}
	c.pdcIndicationRefs[registration] = 1
	if err := c.syncPDCIndications(ctx); err != nil {
		delete(c.pdcIndicationRefs, registration)
		return err
	}
	return nil
}

func (c *Client) releasePDCIndication(registration pdcIndicationRegistration) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorCleanupTimeout)
	defer cancel()

	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	count := c.pdcIndicationRefs[registration]
	if count == 0 {
		return
	}
	if count > 1 {
		c.pdcIndicationRefs[registration]--
		return
	}
	delete(c.pdcIndicationRefs, registration)
	_ = c.syncPDCIndications(ctx)
}

func (c *Client) syncPDCIndications(ctx context.Context) error {
	configChange := c.pdcIndicationRefs[pdcIndicationConfigChange] > 0
	clientRefresh := c.pdcIndicationRefs[pdcIndicationClientRefresh] > 0
	return c.PDCRegister(ctx, PDCIndicationConfig{
		ConfigChange:  configChange,
		ClientRefresh: &clientRefresh,
	})
}

func validatePDCConfigurationType(configType PDCConfigurationType) error {
	switch configType {
	case PDCConfigurationPlatform, PDCConfigurationSoftware, PDCConfigurationDatabase:
		return nil
	default:
		return fmt.Errorf("configuration type 0x%X is unsupported", uint32(configType))
	}
}

// MarshalBinary encodes a QMI PDC configuration reference.
func (config PDCConfig) MarshalBinary() ([]byte, error) {
	if err := validatePDCConfigurationType(config.Type); err != nil {
		return nil, err
	}
	if len(config.ID) > pdcConfigIDMax {
		return nil, fmt.Errorf("configuration ID length %d exceeds %d", len(config.ID), pdcConfigIDMax)
	}
	value := binary.LittleEndian.AppendUint32(nil, uint32(config.Type))
	value = append(value, byte(len(config.ID)))
	value = append(value, config.ID...)
	return value, nil
}

// UnmarshalBinary decodes a QMI PDC configuration reference.
func (config *PDCConfig) UnmarshalBinary(value []byte) error {
	if len(value) < 5 {
		return errors.New("configuration type or ID length is truncated")
	}
	configType := PDCConfigurationType(binary.LittleEndian.Uint32(value[:4]))
	if err := validatePDCConfigurationType(configType); err != nil {
		return err
	}
	length := int(value[4])
	if length > pdcConfigIDMax {
		return fmt.Errorf("configuration ID length %d exceeds %d", length, pdcConfigIDMax)
	}
	if len(value) != 5+length {
		return fmt.Errorf("configuration value length %d, want %d", len(value), 5+length)
	}
	*config = PDCConfig{Type: configType, ID: slices.Clone(value[5:])}
	return nil
}

func decodePDCConfigList(value []byte) ([]PDCConfig, error) {
	if len(value) == 0 {
		return nil, errors.New("parsing QMI PDC configuration list: count is missing")
	}
	count := int(value[0])
	if count > pdcConfigListMax {
		return nil, fmt.Errorf("parsing QMI PDC configuration list: count %d exceeds %d", count, pdcConfigListMax)
	}
	configs := make([]PDCConfig, count)
	offset := 1
	for index := range count {
		if len(value)-offset < 5 {
			return nil, fmt.Errorf("parsing QMI PDC configuration list item %d: header is truncated", index)
		}
		length := int(value[offset+4])
		end := offset + 5 + length
		if end > len(value) {
			return nil, fmt.Errorf("parsing QMI PDC configuration list item %d: ID is truncated", index)
		}
		var config PDCConfig
		if err := config.UnmarshalBinary(value[offset:end]); err != nil {
			return nil, fmt.Errorf("parsing QMI PDC configuration list item %d: %w", index, err)
		}
		configs[index] = config
		offset = end
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI PDC configuration list: %d trailing bytes", len(value)-offset)
	}
	return configs, nil
}

// UnmarshalTLVs parses the common QMI PDC indication result and token.
func (result *PDCIndicationResult) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*result = PDCIndicationResult{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI PDC indication: result TLV missing")
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI PDC indication: result TLV length %d, want 2", len(value))
	}
	result.Error = QMIError(binary.LittleEndian.Uint16(value))
	token, known, err := decodePDCIndicationToken(tlvs)
	if err != nil {
		return err
	}
	result.Token, result.TokenKnown = token, known
	return nil
}

func decodePDCIndicationToken(tlvs tlv.TLVs) (uint32, bool, error) {
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return 0, false, nil
	}
	if len(value) != 4 {
		return 0, false, fmt.Errorf("parsing QMI PDC indication: token TLV length %d, want 4", len(value))
	}
	return binary.LittleEndian.Uint32(value), true, nil
}

func decodePDCUint32(value []byte) (uint32, error) {
	if len(value) != 4 {
		return 0, fmt.Errorf("uint32 TLV length %d, want 4", len(value))
	}
	return binary.LittleEndian.Uint32(value), nil
}

type pdcUTF16 string

func (text pdcUTF16) String() string {
	return string(text)
}

func (text pdcUTF16) MarshalBinary() ([]byte, error) {
	if !utf8.ValidString(string(text)) {
		return nil, errors.New("UTF-16 value is not valid UTF-8")
	}
	for _, r := range text {
		if r == 0 {
			return nil, errors.New("UTF-16 value contains a NUL character")
		}
	}
	codeUnits := utf16.Encode([]rune(text))
	if len(codeUnits) > pdcConfigPathMax {
		return nil, fmt.Errorf("UTF-16 value length %d exceeds %d", len(codeUnits), pdcConfigPathMax)
	}
	value := make([]byte, 0, len(codeUnits)*2)
	for _, codeUnit := range codeUnits {
		value = binary.LittleEndian.AppendUint16(value, codeUnit)
	}
	return value, nil
}

func (text *pdcUTF16) UnmarshalBinary(value []byte) error {
	if len(value)%2 != 0 {
		return fmt.Errorf("UTF-16 value has odd byte length %d", len(value))
	}
	count := len(value) / 2
	if count > pdcConfigPathMax {
		return fmt.Errorf("UTF-16 value length %d exceeds %d", count, pdcConfigPathMax)
	}
	codeUnits := make([]uint16, count)
	for index := range count {
		codeUnits[index] = binary.LittleEndian.Uint16(value[index*2 : index*2+2])
	}
	if len(codeUnits) > 0 && codeUnits[len(codeUnits)-1] == 0 {
		codeUnits = codeUnits[:len(codeUnits)-1]
	}
	for index := 0; index < len(codeUnits); index++ {
		codeUnit := codeUnits[index]
		if codeUnit == 0 {
			return errors.New("UTF-16 value contains an embedded NUL character")
		}
		switch {
		case codeUnit >= 0xd800 && codeUnit <= 0xdbff:
			if index+1 >= len(codeUnits) || codeUnits[index+1] < 0xdc00 || codeUnits[index+1] > 0xdfff {
				return fmt.Errorf("UTF-16 value contains an unpaired high surrogate at code unit %d", index)
			}
			index++
		case codeUnit >= 0xdc00 && codeUnit <= 0xdfff:
			return fmt.Errorf("UTF-16 value contains an unpaired low surrogate at code unit %d", index)
		}
	}
	*text = pdcUTF16(string(utf16.Decode(codeUnits)))
	return nil
}

func (text pdcUTF16) MarshalText() ([]byte, error) {
	if _, err := text.MarshalBinary(); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func (text *pdcUTF16) UnmarshalText(value []byte) error {
	if !utf8.Valid(value) {
		return errors.New("decoding UTF-16 text: value is not valid UTF-8")
	}
	decoded := pdcUTF16(string(value))
	if _, err := decoded.MarshalBinary(); err != nil {
		return err
	}
	*text = decoded
	return nil
}
