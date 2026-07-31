package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	pdcLoadFrameMax  = 32768
	pdcLoadChunkSize = 0x400
)

// PDCConfigLimits contains storage use for one configuration type.
type PDCConfigLimits struct {
	Maximum      uint64
	MaximumKnown bool
	Current      uint64
	CurrentKnown bool
}

// PDCConfigLoad contains a complete configuration to upload. Storage is
// omitted when nil so the modem can use its default storage policy.
type PDCConfigLoad struct {
	Config  PDCConfig
	Data    []byte
	Storage *PDCStorage
}

// PDCConfigDeactivation selects the active configuration store to deactivate.
type PDCConfigDeactivation struct {
	Type         PDCConfigurationType
	Subscription *uint32
}

// PDCResetRequest encodes QMI PDC Reset.
type PDCResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r PDCResetRequest) Request() Request {
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCReset,
		Timeout:       r.Timeout,
	}
}

// PDCDeleteConfigRequest encodes QMI PDC Delete Config.
type PDCDeleteConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDCConfig
	Token         *uint32
}

// Request validates and converts the configuration deletion into QMI TLVs.
func (r PDCDeleteConfigRequest) Request() (Request, error) {
	if err := validatePDCConfigurationType(r.Config.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration deletion: %w", err)
	}
	if len(r.Config.ID) > pdcConfigIDMax {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration deletion: configuration ID length %d exceeds %d", len(r.Config.ID), pdcConfigIDMax)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(r.Config.Type))}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	if len(r.Config.ID) != 0 {
		id := append([]byte{byte(len(r.Config.ID))}, r.Config.ID...)
		tlvs = append(tlvs, tlv.Bytes(0x11, id))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCDeleteConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCLoadConfigRequest encodes one QMI PDC Load Config frame.
type PDCLoadConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        PDCConfig
	TotalSize     uint32
	Chunk         []byte
	Token         *uint32
	Storage       *PDCStorage
}

// Request validates and converts one configuration frame into QMI TLVs.
func (r PDCLoadConfigRequest) Request() (Request, error) {
	value, err := encodePDCLoadFrame(r.Config, r.TotalSize, r.Chunk)
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration frame: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if r.Token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *r.Token))
	}
	if r.Storage != nil {
		if err := validatePDCStorage(*r.Storage); err != nil {
			return Request{}, fmt.Errorf("encoding QMI PDC configuration frame: %w", err)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint32(*r.Storage)))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCLoadConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCGetConfigLimitsRequest encodes QMI PDC Get Config Limits.
type PDCGetConfigLimitsRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          PDCConfigurationType
	Token         *uint32
}

// Request validates and converts the storage-limits query into QMI TLVs.
func (r PDCGetConfigLimitsRequest) Request() (Request, error) {
	tlvs, err := pdcTypeAndTokenTLVs(r.Type, r.Token)
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration limits query: %w", err)
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCGetConfigLimits,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCGetDefaultConfigInfoRequest encodes QMI PDC Get Default Config Info.
type PDCGetDefaultConfigInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          PDCConfigurationType
	Token         *uint32
}

// Request validates and converts the default-configuration query into QMI TLVs.
func (r PDCGetDefaultConfigInfoRequest) Request() (Request, error) {
	tlvs, err := pdcTypeAndTokenTLVs(r.Type, r.Token)
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC default configuration query: %w", err)
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCGetDefaultConfigInfo,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCDeactivateConfigRequest encodes QMI PDC Deactivate Config.
type PDCDeactivateConfigRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          PDCConfigurationType
	Token         *uint32
	Subscription  *uint32
}

// Request validates and converts the deactivation into QMI TLVs.
func (r PDCDeactivateConfigRequest) Request() (Request, error) {
	tlvs, err := pdcTypeAndTokenTLVs(r.Type, r.Token)
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI PDC configuration deactivation: %w", err)
	}
	if r.Subscription != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *r.Subscription))
	}
	return Request{
		Service:       ServicePDC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessagePDCDeactivateConfig,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// PDCLoadConfigResponse contains immediate frame-reset feedback.
type PDCLoadConfigResponse struct {
	FrameReset      bool
	FrameResetKnown bool
}

// UnmarshalTLVs parses the optional immediate frame-reset flag.
func (r *PDCLoadConfigResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = PDCLoadConfigResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	reset, err := decodePDCBool(value)
	if err != nil {
		return err
	}
	r.FrameReset, r.FrameResetKnown = reset, true
	return nil
}

// PDCDeleteConfigIndication is a Delete Config completion.
type PDCDeleteConfigIndication = PDCOperationIndication

// PDCDeactivateConfigIndication is a Deactivate Config completion.
type PDCDeactivateConfigIndication = PDCOperationIndication

// PDCLoadConfigIndication is one asynchronous load-frame result.
type PDCLoadConfigIndication struct {
	Result          PDCIndicationResult
	Received        uint32
	ReceivedKnown   bool
	Remaining       uint32
	RemainingKnown  bool
	FrameReset      bool
	FrameResetKnown bool
}

// UnmarshalTLVs parses one load-frame completion.
func (i *PDCLoadConfigIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCLoadConfigIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Received, i.ReceivedKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Remaining, i.RemainingKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		parsed, err := decodePDCBool(value)
		if err != nil {
			return err
		}
		i.FrameReset, i.FrameResetKnown = parsed, true
	}
	return nil
}

// PDCGetConfigLimitsIndication is the asynchronous storage-limits result.
type PDCGetConfigLimitsIndication struct {
	Result PDCIndicationResult
	Limits PDCConfigLimits
}

// UnmarshalTLVs parses configuration storage limits.
func (i *PDCGetConfigLimitsIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCGetConfigLimitsIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodePDCUint64(value)
		if err != nil {
			return err
		}
		i.Limits.Maximum, i.Limits.MaximumKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		parsed, err := decodePDCUint64(value)
		if err != nil {
			return err
		}
		i.Limits.Current, i.Limits.CurrentKnown = parsed, true
	}
	return nil
}

// PDCGetDefaultConfigInfoIndication is the asynchronous default-config result.
type PDCGetDefaultConfigInfoIndication struct {
	Result PDCIndicationResult
	Info   PDCConfigInfo
}

// UnmarshalTLVs parses default configuration metadata.
func (i *PDCGetDefaultConfigInfoIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*i = PDCGetDefaultConfigInfoIndication{}
	if err := i.Result.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.Version, i.Info.VersionKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		parsed, err := decodePDCUint32(value)
		if err != nil {
			return err
		}
		i.Info.Size, i.Info.SizeKnown = parsed, true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		parsed, err := decodePDCBytes(value, pdcConfigDescriptionMax)
		if err != nil {
			return err
		}
		i.Info.Description, i.Info.DescriptionKnown = string(parsed), true
	}
	return nil
}

// PDCReset resets this control point's PDC service state.
func (c *Client) PDCReset(ctx context.Context) error {
	c.pdcMu.Lock()
	defer c.pdcMu.Unlock()
	err := c.withServiceClient(ctx, ServicePDC, func(clientID uint8) error {
		req := PDCResetRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("resetting QMI PDC control point: %w", err)
	}
	return nil
}

// PDCDeleteConfiguration deletes one stored configuration.
func (c *Client) PDCDeleteConfiguration(ctx context.Context, config PDCConfig) error {
	if err := validatePDCRequiredConfig(config); err != nil {
		return fmt.Errorf("deleting QMI PDC configuration: %w", err)
	}
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCDeleteConfig, func(clientID uint8, token uint32) (Request, error) {
		return (PDCDeleteConfigRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   config,
			Token:    &token,
		}).Request()
	})
	if err != nil {
		return fmt.Errorf("deleting QMI PDC configuration: %w", err)
	}
	var indication PDCDeleteConfigIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return fmt.Errorf("deleting QMI PDC configuration: %w", err)
	}
	if err := indication.Result.Err(); err != nil {
		return fmt.Errorf("deleting QMI PDC configuration: %w", err)
	}
	return nil
}

// PDCLoadConfiguration uploads one complete configuration in bounded frames.
func (c *Client) PDCLoadConfiguration(ctx context.Context, load PDCConfigLoad) error {
	if err := c.pdcLoadConfiguration(ctx, load); err != nil {
		return fmt.Errorf("loading QMI PDC configuration: %w", err)
	}
	return nil
}

func (c *Client) pdcLoadConfiguration(ctx context.Context, load PDCConfigLoad) error {
	if err := validatePDCRequiredConfig(load.Config); err != nil {
		return err
	}
	if len(load.Data) == 0 {
		return errors.New("configuration data is empty")
	}
	if uint64(len(load.Data)) > uint64(^uint32(0)) {
		return fmt.Errorf("configuration data length %d exceeds uint32", len(load.Data))
	}
	if load.Storage != nil {
		if err := validatePDCStorage(*load.Storage); err != nil {
			return err
		}
	}

	c.pdcMu.Lock()
	defer c.pdcMu.Unlock()
	transport, err := c.indicationTransport()
	if err != nil {
		return err
	}
	clientID, err := c.serviceClientID(ctx, ServicePDC)
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	indications, err := transport.Indications(waitCtx, ServicePDC, clientID, MessagePDCLoadConfig)
	if err != nil {
		return fmt.Errorf("subscribing QMI PDC load result: %w", err)
	}

	total := uint32(len(load.Data))
	for offset := 0; offset < len(load.Data); {
		end := min(offset+pdcLoadChunkSize, len(load.Data))
		token := c.nextPDCTokenLocked()
		req, err := (PDCLoadConfigRequest{
			ClientID:  clientID,
			Timeout:   DefaultRequestTimeout,
			Config:    load.Config,
			TotalSize: total,
			Chunk:     load.Data[offset:end],
			Token:     &token,
			Storage:   load.Storage,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var frameResponse PDCLoadConfigResponse
		if err := frameResponse.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		if frameResponse.FrameReset {
			return errors.New("modem reset accumulated configuration frame data")
		}

		indicationTLVs, err := waitPDCIndication(ctx, indications, token)
		if err != nil {
			return err
		}
		var indication PDCLoadConfigIndication
		if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
			return err
		}
		if err := indication.Result.Err(); err != nil {
			return err
		}
		if indication.FrameReset {
			return errors.New("modem reset accumulated configuration frame data")
		}
		if !indication.RemainingKnown {
			return errors.New("remaining configuration size is missing")
		}
		expectedRemaining := uint32(len(load.Data) - end)
		if indication.Remaining != expectedRemaining {
			return fmt.Errorf("remaining configuration size %d, want %d", indication.Remaining, expectedRemaining)
		}
		if indication.ReceivedKnown && indication.Received != uint32(end) {
			return fmt.Errorf("received configuration size %d, want %d", indication.Received, end)
		}
		offset = end
	}
	return nil
}

// PDCConfigurationLimits returns storage use for one configuration type.
func (c *Client) PDCConfigurationLimits(ctx context.Context, configType PDCConfigurationType) (PDCConfigLimits, error) {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCGetConfigLimits, func(clientID uint8, token uint32) (Request, error) {
		return (PDCGetConfigLimitsRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Type:     configType,
			Token:    &token,
		}).Request()
	})
	if err != nil {
		return PDCConfigLimits{}, fmt.Errorf("reading QMI PDC configuration limits: %w", err)
	}
	var indication PDCGetConfigLimitsIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return PDCConfigLimits{}, fmt.Errorf("reading QMI PDC configuration limits: %w", err)
	}
	if err := indication.Result.Err(); err != nil {
		return PDCConfigLimits{}, fmt.Errorf("reading QMI PDC configuration limits: %w", err)
	}
	return indication.Limits, nil
}

// PDCDefaultConfigurationInfo returns metadata for the embedded default.
func (c *Client) PDCDefaultConfigurationInfo(ctx context.Context, configType PDCConfigurationType) (PDCConfigInfo, error) {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCGetDefaultConfigInfo, func(clientID uint8, token uint32) (Request, error) {
		return (PDCGetDefaultConfigInfoRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Type:     configType,
			Token:    &token,
		}).Request()
	})
	if err != nil {
		return PDCConfigInfo{}, fmt.Errorf("reading QMI PDC default configuration info: %w", err)
	}
	var indication PDCGetDefaultConfigInfoIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return PDCConfigInfo{}, fmt.Errorf("reading QMI PDC default configuration info: %w", err)
	}
	if err := indication.Result.Err(); err != nil {
		return PDCConfigInfo{}, fmt.Errorf("reading QMI PDC default configuration info: %w", err)
	}
	return indication.Info, nil
}

// PDCDeactivateConfiguration deactivates the active configuration for a store.
func (c *Client) PDCDeactivateConfiguration(ctx context.Context, deactivation PDCConfigDeactivation) error {
	indicationTLVs, err := c.pdcSingleIndication(ctx, MessagePDCDeactivateConfig, func(clientID uint8, token uint32) (Request, error) {
		return (PDCDeactivateConfigRequest{
			ClientID:     clientID,
			Timeout:      DefaultRequestTimeout,
			Type:         deactivation.Type,
			Token:        &token,
			Subscription: deactivation.Subscription,
		}).Request()
	})
	if err != nil {
		return fmt.Errorf("deactivating QMI PDC configuration: %w", err)
	}
	var indication PDCDeactivateConfigIndication
	if err := indication.UnmarshalTLVs(indicationTLVs); err != nil {
		return fmt.Errorf("deactivating QMI PDC configuration: %w", err)
	}
	if err := indication.Result.Err(); err != nil {
		return fmt.Errorf("deactivating QMI PDC configuration: %w", err)
	}
	return nil
}

func pdcTypeAndTokenTLVs(configType PDCConfigurationType, token *uint32) (tlv.TLVs, error) {
	if err := validatePDCConfigurationType(configType); err != nil {
		return nil, err
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint32(configType))}
	if token != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, *token))
	}
	return tlvs, nil
}

func encodePDCLoadFrame(config PDCConfig, totalSize uint32, chunk []byte) ([]byte, error) {
	if err := validatePDCRequiredConfig(config); err != nil {
		return nil, err
	}
	if totalSize == 0 {
		return nil, errors.New("total configuration size is zero")
	}
	if len(chunk) == 0 {
		return nil, errors.New("configuration frame is empty")
	}
	if len(chunk) > pdcLoadFrameMax {
		return nil, fmt.Errorf("configuration frame length %d exceeds %d", len(chunk), pdcLoadFrameMax)
	}
	if uint64(len(chunk)) > uint64(totalSize) {
		return nil, fmt.Errorf("configuration frame length %d exceeds total size %d", len(chunk), totalSize)
	}
	value, err := config.MarshalBinary()
	if err != nil {
		return nil, err
	}
	value = binary.LittleEndian.AppendUint32(value, totalSize)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(chunk)))
	value = append(value, chunk...)
	return value, nil
}

func validatePDCRequiredConfig(config PDCConfig) error {
	if err := validatePDCConfigurationType(config.Type); err != nil {
		return err
	}
	if len(config.ID) == 0 {
		return errors.New("configuration ID is empty")
	}
	if len(config.ID) > pdcConfigIDMax {
		return fmt.Errorf("configuration ID length %d exceeds %d", len(config.ID), pdcConfigIDMax)
	}
	return nil
}

func validatePDCStorage(storage PDCStorage) error {
	if storage > PDCStorageRemote {
		return fmt.Errorf("configuration storage %d is out of range", storage)
	}
	return nil
}

func decodePDCBool(value []byte) (bool, error) {
	if len(value) != 1 {
		return false, fmt.Errorf("parsing QMI PDC boolean: TLV length %d, want 1", len(value))
	}
	if value[0] > 1 {
		return false, fmt.Errorf("parsing QMI PDC boolean: value %d, want 0 or 1", value[0])
	}
	return value[0] == 1, nil
}

func decodePDCUint64(value []byte) (uint64, error) {
	if len(value) != 8 {
		return 0, fmt.Errorf("parsing QMI PDC uint64: TLV length %d, want 8", len(value))
	}
	return binary.LittleEndian.Uint64(value), nil
}
