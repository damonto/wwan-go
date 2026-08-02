package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	catCleanupTimeout           = 5 * time.Second
	catTerminalProfileMaxLength = 80
)

const (
	catSetEventReportRawTLV          = 0x10
	catSetEventReportSlotTLV         = 0x12
	catSetEventReportFullFunctionTLV = 0x13

	catSetEventReportRawErrorTLV          = 0x10
	catSetEventReportFullFunctionErrorTLV = 0x12
)

type CAT struct {
	client *Client
}

type CATConfiguration struct {
	Mode          CATConfigMode
	CustomProfile []byte
}

type CATCommand struct {
	Ref  uint32
	Data []byte
}

type CATEventConfirmation struct {
	UserConfirmed *bool
	IconDisplayed *bool
}

// CATCachedCommandID identifies a proactive command retained by the modem for
// recovery after the host STK service restarts.
type CATCachedCommandID uint32

const (
	// CATCachedCommandSetupMenu identifies a cached SET UP MENU command.
	CATCachedCommandSetupMenu CATCachedCommandID = iota + 1
	// CATCachedCommandSetupEventList identifies a cached SET UP EVENT LIST command.
	CATCachedCommandSetupEventList
	// CATCachedCommandSetupIdleModeText identifies a cached SET UP IDLE MODE TEXT command.
	CATCachedCommandSetupIdleModeText
)

func (c CATCommand) MarshalBinary() ([]byte, error) {
	if len(c.Data) > 0xffff {
		return nil, fmt.Errorf("building QMI CAT command: data length %d exceeds uint16 length field", len(c.Data))
	}
	value := binary.LittleEndian.AppendUint32(nil, c.Ref)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(c.Data)))
	return append(value, c.Data...), nil
}

func (c *CATCommand) UnmarshalBinary(data []byte) error {
	if len(data) < 6 {
		return errors.New("parsing QMI CAT command: raw command TLV is truncated")
	}
	ref := binary.LittleEndian.Uint32(data[:4])
	length := int(binary.LittleEndian.Uint16(data[4:6]))
	if len(data) < 6+length {
		return errors.New("parsing QMI CAT command: raw command data is truncated")
	}
	if len(data) != 6+length {
		return errors.New("parsing QMI CAT command: raw command data has trailing bytes")
	}
	c.Ref = ref
	c.Data = slices.Clone(data[6 : 6+length])
	return nil
}

func (c CATCommand) WriteTo(w io.Writer) (int64, error) {
	data, err := c.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func (c *CATCommand) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return int64(len(data)), err
	}
	return int64(len(data)), c.UnmarshalBinary(data)
}

func NewCAT(client *Client) *CAT {
	return &CAT{client: client}
}

func (m CATConfigMode) String() string {
	switch m {
	case CATConfigDisabled:
		return "disabled"
	case CATConfigGobi:
		return "gobi"
	case CATConfigAndroid:
		return "android"
	case CATConfigDecoded:
		return "decoded"
	case CATConfigDecodedPull:
		return "decoded-pull"
	case CATConfigCustomRaw:
		return "custom-raw"
	case CATConfigCustomDecoded:
		return "custom-decoded"
	default:
		return fmt.Sprintf("unknown-0x%02X", uint8(m))
	}
}

func (c *CAT) Configuration(ctx context.Context) (CATConfiguration, error) {
	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return CATConfiguration{}, err
	}
	resp, err := c.client.requestService(ctx, service, clientID, MessageCATGetConfiguration, nil)
	if err != nil {
		return CATConfiguration{}, fmt.Errorf("reading QMI CAT configuration: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return CATConfiguration{}, fmt.Errorf("reading QMI CAT configuration: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok || len(value) == 0 {
		return CATConfiguration{}, errors.New("reading QMI CAT configuration: mode TLV missing")
	}
	config := CATConfiguration{Mode: CATConfigMode(value[0])}
	if profile, ok := tlv.Value(resp.TLVs, 0x11); ok {
		if len(profile) == 0 {
			return CATConfiguration{}, errors.New("reading QMI CAT configuration: custom profile TLV is truncated")
		}
		length := int(profile[0])
		if len(profile) < 1+length {
			return CATConfiguration{}, errors.New("reading QMI CAT configuration: custom profile data is truncated")
		}
		config.CustomProfile = slices.Clone(profile[1 : 1+length])
	}
	return config, nil
}

func (c *CAT) SetConfiguration(ctx context.Context, config CATConfiguration) error {
	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return err
	}
	if err := c.setConfiguration(ctx, service, clientID, config); err != nil {
		return fmt.Errorf("setting QMI CAT configuration: %w", err)
	}
	return nil
}

func (c *CAT) Commands(ctx context.Context, eventMask, fullFunctionMask uint32) (<-chan CATCommand, error) {
	commands, _, err := c.commands(ctx, CATEventClaimConfig{
		RawMask:          eventMask,
		FullFunctionMask: fullFunctionMask,
	}, false)
	return commands, err
}

// ForceClaimCommands installs the indication watch before force-claiming the
// requested events, so cached commands replayed during registration are not
// lost. Releasing another CAT client is disruptive and must be explicitly
// requested by the caller.
func (c *CAT) ForceClaimCommands(ctx context.Context, config CATEventClaimConfig) (<-chan CATCommand, CATEventClaim, error) {
	return c.commands(ctx, config, true)
}

func (c *CAT) commands(ctx context.Context, config CATEventClaimConfig, forceClaim bool) (<-chan CATCommand, CATEventClaim, error) {
	transport, err := c.client.indicationTransport()
	if err != nil {
		return nil, CATEventClaim{}, fmt.Errorf("watching QMI CAT commands: %w", err)
	}
	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return nil, CATEventClaim{}, fmt.Errorf("watching QMI CAT commands: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, service, clientID, MessageCATEventReport)
	if err != nil {
		cancel()
		return nil, CATEventClaim{}, fmt.Errorf("watching QMI CAT commands: %w", err)
	}

	var claim CATEventClaim
	if forceClaim {
		claim, err = c.ForceClaimEvents(ctx, config)
	} else {
		err = c.setEventReport(ctx, service, clientID, config.RawMask, config.FullFunctionMask)
	}
	if err != nil {
		cancel()
		c.releaseCATClient(service, clientID)
		return nil, CATEventClaim{}, err
	}

	out := make(chan CATCommand, 8)
	go func() {
		defer close(out)
		defer c.releaseCATClient(service, clientID)
		defer cancel()

		for ind := range indications {
			var command CATCommand
			if err := command.UnmarshalTLVs(ind.TLVs); err != nil {
				return
			}
			select {
			case out <- command:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, claim, nil
}

func (c *CAT) TerminalResponse(ctx context.Context, ref uint32, response []byte) error {
	if len(response) > catTerminalResponseMaxLength {
		return fmt.Errorf("sending QMI CAT terminal response: response length %d exceeds QMI CAT terminal response limit %d", len(response), catTerminalResponseMaxLength)
	}

	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return err
	}
	value := binary.LittleEndian.AppendUint32(nil, ref)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(response)))
	value = append(value, response...)

	resp, err := c.client.requestService(ctx, service, clientID, MessageCATSendTerminalResponse, tlv.TLVs{
		tlv.Bytes(0x01, value),
		tlv.Uint(0x10, c.client.slot),
	})
	if err != nil {
		return fmt.Errorf("sending QMI CAT terminal response: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("sending QMI CAT terminal response: %w", err)
	}
	return nil
}

func (c *CAT) EventConfirmation(ctx context.Context, confirmation CATEventConfirmation) error {
	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return err
	}
	tlvs := tlv.TLVs{tlv.Uint(0x12, c.client.slot)}
	if confirmation.UserConfirmed != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*confirmation.UserConfirmed)))
	}
	if confirmation.IconDisplayed != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*confirmation.IconDisplayed)))
	}

	resp, err := c.client.requestService(ctx, service, clientID, MessageCATEventConfirmation, tlvs)
	if err != nil {
		return fmt.Errorf("sending QMI CAT event confirmation: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("sending QMI CAT event confirmation: %w", err)
	}
	return nil
}

func (c *CAT) Envelope(ctx context.Context, envelope []byte, envType uint16) (EnvelopeResponse, error) {
	resp, err := c.client.sendCATEnvelope(ctx, envelope, envType)
	if err != nil {
		return EnvelopeResponse{}, err
	}
	return resp, nil
}

func (c *CAT) TerminalProfile(ctx context.Context) ([]byte, error) {
	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.requestService(ctx, service, clientID, MessageCATGetTerminalProfile, tlv.TLVs{
		tlv.Uint(0x10, c.client.slot),
	})
	if err != nil {
		return nil, fmt.Errorf("reading QMI CAT terminal profile: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return nil, fmt.Errorf("reading QMI CAT terminal profile: %w", err)
	}
	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return nil, errors.New("reading QMI CAT terminal profile: profile TLV missing")
	}
	if len(value) == 0 {
		return nil, errors.New("reading QMI CAT terminal profile: profile TLV is truncated")
	}
	length := int(value[0])
	if len(value) != 1+length {
		return nil, fmt.Errorf("reading QMI CAT terminal profile: profile TLV length %d, want %d", len(value), 1+length)
	}
	return slices.Clone(value[1 : 1+length]), nil
}

// CachedProactiveCommand retrieves a recovery copy of a persistent proactive
// command. The returned command is historical state and must not receive a
// terminal response.
func (c *CAT) CachedProactiveCommand(ctx context.Context, commandID CATCachedCommandID) (CATCommand, error) {
	tag, ok := cachedCommandTLV(commandID)
	if !ok {
		return CATCommand{}, fmt.Errorf("reading cached QMI CAT command: command ID %d is unsupported", commandID)
	}

	service, clientID, err := c.client.catClient(ctx)
	if err != nil {
		return CATCommand{}, err
	}
	resp, err := c.client.requestService(ctx, service, clientID, MessageCATGetCachedProactiveCommand, tlv.TLVs{
		tlv.Uint(0x01, uint32(commandID)),
		tlv.Uint(0x10, c.client.slot),
	})
	if err != nil {
		return CATCommand{}, fmt.Errorf("reading cached QMI CAT command %d: %w", commandID, err)
	}
	if err := resultOK(resp); err != nil {
		return CATCommand{}, fmt.Errorf("reading cached QMI CAT command %d: %w", commandID, err)
	}

	value, ok := tlv.Value(resp.TLVs, tag)
	if !ok {
		return CATCommand{}, fmt.Errorf("reading cached QMI CAT command %d: command TLV missing", commandID)
	}
	var command CATCommand
	if err := command.UnmarshalBinary(value); err != nil {
		return CATCommand{}, fmt.Errorf("reading cached QMI CAT command %d: %w", commandID, err)
	}
	return command, nil
}

func cachedCommandTLV(commandID CATCachedCommandID) (byte, bool) {
	switch commandID {
	case CATCachedCommandSetupMenu:
		return 0x10, true
	case CATCachedCommandSetupEventList:
		return 0x11, true
	case CATCachedCommandSetupIdleModeText:
		return 0x12, true
	default:
		return 0, false
	}
}

func (c *CAT) setEventReport(ctx context.Context, service ServiceType, clientID uint8, mask, full uint32) error {
	tlvs := tlv.TLVs{
		tlv.Uint(catSetEventReportRawTLV, mask),
		tlv.Uint(catSetEventReportSlotTLV, uint8(1<<(c.client.slot-1))),
	}
	if full != 0 {
		tlvs = append(tlvs, tlv.Uint(catSetEventReportFullFunctionTLV, full))
	}

	resp, err := c.client.requestService(ctx, service, clientID, MessageCATSetEventReport, tlvs)
	if err != nil {
		return fmt.Errorf("registering QMI CAT events: %w", err)
	}
	if err := checkEventReportRegistration(resp.TLVs, mask, full); err != nil {
		return fmt.Errorf("registering QMI CAT events: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("registering QMI CAT events: %w", err)
	}
	return nil
}

func (c *CAT) setConfiguration(ctx context.Context, service ServiceType, clientID uint8, config CATConfiguration) error {
	if len(config.CustomProfile) > catTerminalProfileMaxLength {
		return fmt.Errorf("terminal profile length %d exceeds %d", len(config.CustomProfile), catTerminalProfileMaxLength)
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(config.Mode))}
	if len(config.CustomProfile) > 0 {
		custom := append([]byte{byte(len(config.CustomProfile))}, config.CustomProfile...)
		tlvs = append(tlvs, tlv.Bytes(0x10, custom))
	}

	resp, err := c.client.requestService(ctx, service, clientID, MessageCATSetConfiguration, tlvs)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *CAT) releaseCATClient(service ServiceType, clientID uint8) {
	ctx, cancel := context.WithTimeout(context.Background(), catCleanupTimeout)
	defer cancel()
	_ = c.client.releaseCATClient(ctx, service, clientID)
}

// UnmarshalTLVs decodes a raw command from a QMI CAT indication.
func (c *CATCommand) UnmarshalTLVs(tlvs tlv.TLVs) error {
	for _, item := range tlvs {
		if !isRawCATCommandTLV(item.Type) {
			continue
		}
		var command CATCommand
		if err := command.UnmarshalBinary(item.Value); err != nil {
			return fmt.Errorf("parsing QMI CAT indication: %w", err)
		}
		*c = command
		return nil
	}
	return errors.New("parsing QMI CAT indication: raw command TLV missing")
}

func isRawCATCommandTLV(tag byte) bool {
	switch tag {
	case 0x10, 0x11, 0x12, 0x13, 0x14, 0x17, 0x18,
		0x47, 0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,
		0x51, 0x52, 0x53, 0x54, 0x66, 0x6A:
		return true
	default:
		return false
	}
}

func checkEventReportRegistration(tlvs tlv.TLVs, raw, full uint32) error {
	checks := []struct {
		tag       byte
		requested uint32
	}{
		{tag: catSetEventReportRawErrorTLV, requested: raw},
		{tag: catSetEventReportFullFunctionErrorTLV, requested: full},
	}

	for _, check := range checks {
		failed, ok, err := eventReportErrorMask(tlvs, check.tag)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		rejected := failed & check.requested
		if rejected != 0 {
			switch check.tag {
			case catSetEventReportRawErrorTLV:
				return fmt.Errorf("raw mask 0x%08X already registered by another control point", rejected)
			case catSetEventReportFullFunctionErrorTLV:
				return fmt.Errorf("full-function mask 0x%08X was not enabled", rejected)
			}
		}
	}
	return nil
}

func eventReportErrorMask(tlvs tlv.TLVs, tag byte) (uint32, bool, error) {
	value, ok := tlv.Value(tlvs, tag)
	if !ok {
		return 0, false, nil
	}
	if len(value) != 4 {
		return 0, false, fmt.Errorf("parsing QMI CAT event registration status: TLV 0x%02X length %d, want 4", tag, len(value))
	}
	return binary.LittleEndian.Uint32(value), true, nil
}

func (c *Client) sendCATEnvelope(ctx context.Context, envelope []byte, envType uint16) (EnvelopeResponse, error) {
	if len(envelope) < 2 {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: envelope length %d is too short", len(envelope))
	}
	if len(envelope) > catRawEnvelopeMaxLength {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: envelope length %d exceeds QMI CAT raw envelope limit %d", len(envelope), catRawEnvelopeMaxLength)
	}

	service, clientID, err := c.catClient(ctx)
	if err != nil {
		return EnvelopeResponse{}, err
	}

	value := binary.LittleEndian.AppendUint16(nil, envType)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(envelope)))
	value = append(value, envelope...)
	resp, err := c.requestService(ctx, service, clientID, MessageCATSendEnvelope, tlv.TLVs{
		tlv.Bytes(0x01, value),
		tlv.Uint(0x10, c.slot),
	})
	if err != nil {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: %w", err)
	}

	result, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return EnvelopeResponse{}, errors.New("running QMI CAT envelope: raw response TLV missing")
	}
	if len(result) < 3 {
		return EnvelopeResponse{}, errors.New("running QMI CAT envelope: raw response TLV is truncated")
	}
	length := int(result[2])
	if len(result) < 3+length {
		return EnvelopeResponse{}, errors.New("running QMI CAT envelope: envelope response data is truncated")
	}
	return EnvelopeResponse{
		SW1:  result[0],
		SW2:  result[1],
		Data: slices.Clone(result[3 : 3+length]),
	}, nil
}
