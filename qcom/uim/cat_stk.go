package uim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	catCleanupTimeout = 5 * time.Second
)

const (
	catSetEventReportRawTLV          = 0x10
	catSetEventReportSlotTLV         = 0x12
	catSetEventReportFullFunctionTLV = 0x13

	catSetEventReportRawErrorTLV          = 0x10
	catSetEventReportFullFunctionErrorTLV = 0x12
)

type CAT struct {
	reader *Reader
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

func NewCAT(reader *Reader) *CAT {
	return &CAT{reader: reader}
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
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return CATConfiguration{}, err
	}
	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATGetConfiguration, nil)
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
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return err
	}
	if err := c.setConfiguration(ctx, service, clientID, config); err != nil {
		return fmt.Errorf("setting QMI CAT configuration: %w", err)
	}
	return nil
}

func (c *CAT) Commands(ctx context.Context, eventMask, fullFunctionMask uint32) (<-chan CATCommand, error) {
	transport, err := c.reader.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI CAT commands: %w", err)
	}
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI CAT commands: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, service, clientID, qcom.MessageCATEventReport)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI CAT commands: %w", err)
	}
	if err := c.setEventReport(ctx, service, clientID, eventMask, fullFunctionMask); err != nil {
		cancel()
		c.releaseCATClient(service, clientID)
		return nil, fmt.Errorf("watching QMI CAT commands: %w", err)
	}

	out := make(chan CATCommand, 8)
	go func() {
		defer c.releaseCATClient(service, clientID)
		defer cancel()
		defer close(out)

		for ind := range indications {
			session, err := decodeCATCommand(ind.TLVs)
			if err != nil {
				continue
			}
			select {
			case out <- session:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *CAT) TerminalResponse(ctx context.Context, ref uint32, response []byte) error {
	if len(response) > catTerminalResponseMaxLength {
		return fmt.Errorf("sending QMI CAT terminal response: response length %d exceeds QMI CAT terminal response limit %d", len(response), catTerminalResponseMaxLength)
	}

	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return err
	}
	value := binary.LittleEndian.AppendUint32(nil, ref)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(response)))
	value = append(value, response...)

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATSendTerminalResponse, tlv.TLVs{
		tlv.Bytes(0x01, value),
		tlv.Uint(0x10, c.reader.slot),
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
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return err
	}
	tlvs := tlv.TLVs{tlv.Uint(0x12, c.reader.slot)}
	if confirmation.UserConfirmed != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*confirmation.UserConfirmed)))
	}
	if confirmation.IconDisplayed != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, boolByte(*confirmation.IconDisplayed)))
	}

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATEventConfirmation, tlvs)
	if err != nil {
		return fmt.Errorf("sending QMI CAT event confirmation: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("sending QMI CAT event confirmation: %w", err)
	}
	return nil
}

func (c *CAT) Envelope(ctx context.Context, envelope []byte, envType uint16) (EnvelopeResponse, error) {
	resp, err := c.reader.sendCATEnvelope(ctx, envelope, envType)
	if err != nil {
		return EnvelopeResponse{}, err
	}
	return resp, nil
}

func (c *CAT) TerminalProfile(ctx context.Context) ([]byte, error) {
	service, clientID, err := c.reader.catClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATGetTerminalProfile, tlv.TLVs{
		tlv.Uint(0x10, c.reader.slot),
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
	if len(value) < 1+length {
		return nil, errors.New("reading QMI CAT terminal profile: profile data is truncated")
	}
	return slices.Clone(value[1 : 1+length]), nil
}

func (c *CAT) setEventReport(ctx context.Context, service qcom.ServiceType, clientID uint8, mask, full uint32) error {
	tlvs := tlv.TLVs{
		tlv.Uint(catSetEventReportRawTLV, mask),
		tlv.Uint(catSetEventReportSlotTLV, uint8(1<<(c.reader.slot-1))),
	}
	if full != 0 {
		tlvs = append(tlvs, tlv.Uint(catSetEventReportFullFunctionTLV, full))
	}

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATSetEventReport, tlvs)
	if err != nil {
		return err
	}
	if err := resultOK(resp); err != nil {
		return err
	}
	return eventReportRegistrationOK(resp.TLVs, mask, full)
}

func (c *CAT) setConfiguration(ctx context.Context, service qcom.ServiceType, clientID uint8, config CATConfiguration) error {
	if len(config.CustomProfile) > 0xff {
		return fmt.Errorf("terminal profile length %d exceeds 255", len(config.CustomProfile))
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(config.Mode))}
	if len(config.CustomProfile) > 0 {
		custom := append([]byte{byte(len(config.CustomProfile))}, config.CustomProfile...)
		tlvs = append(tlvs, tlv.Bytes(0x10, custom))
	}

	resp, err := c.reader.requestService(ctx, service, clientID, qcom.MessageCATSetConfiguration, tlvs)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (c *CAT) releaseCATClient(service qcom.ServiceType, clientID uint8) {
	ctx, cancel := context.WithTimeout(context.Background(), catCleanupTimeout)
	defer cancel()
	_ = c.reader.releaseCATClient(ctx, service, clientID)
}

func decodeCATCommand(tlvs tlv.TLVs) (CATCommand, error) {
	for _, item := range tlvs {
		if !isRawCATCommandTLV(item.Type) {
			continue
		}
		var command CATCommand
		if err := command.UnmarshalBinary(item.Value); err != nil {
			return CATCommand{}, fmt.Errorf("parsing QMI CAT indication: %w", err)
		}
		return command, nil
	}
	return CATCommand{}, errors.New("parsing QMI CAT indication: raw command TLV missing")
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

func eventReportRegistrationOK(tlvs tlv.TLVs, raw, full uint32) error {
	checks := []struct {
		name      string
		tag       byte
		requested uint32
	}{
		{name: "raw", tag: catSetEventReportRawErrorTLV, requested: raw},
		{name: "full-function", tag: catSetEventReportFullFunctionErrorTLV, requested: full},
	}

	for _, check := range checks {
		failed, ok, err := eventReportErrorMask(tlvs, check.tag)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if failed &= check.requested; failed != 0 {
			return fmt.Errorf("registering QMI CAT events: %s mask 0x%08X rejected", check.name, failed)
		}
	}
	return nil
}

func eventReportErrorMask(tlvs tlv.TLVs, tag byte) (uint32, bool, error) {
	value, ok := tlv.Value(tlvs, tag)
	if !ok {
		return 0, false, nil
	}
	if len(value) < 4 {
		return 0, false, fmt.Errorf("parsing QMI CAT event registration status: TLV 0x%02X length %d, want at least 4", tag, len(value))
	}
	return binary.LittleEndian.Uint32(value), true, nil
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func (r *Reader) sendCATEnvelope(ctx context.Context, envelope []byte, envType uint16) (EnvelopeResponse, error) {
	if len(envelope) < 2 {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: envelope length %d is too short", len(envelope))
	}
	if len(envelope) > catRawEnvelopeMaxLength {
		return EnvelopeResponse{}, fmt.Errorf("running QMI CAT envelope: envelope length %d exceeds QMI CAT raw envelope limit %d", len(envelope), catRawEnvelopeMaxLength)
	}

	service, clientID, err := r.catClient(ctx)
	if err != nil {
		return EnvelopeResponse{}, err
	}

	value := binary.LittleEndian.AppendUint16(nil, envType)
	value = binary.LittleEndian.AppendUint16(value, uint16(len(envelope)))
	value = append(value, envelope...)
	resp, err := r.requestService(ctx, service, clientID, qcom.MessageCATSendEnvelope, tlv.TLVs{
		tlv.Bytes(0x01, value),
		tlv.Uint(0x10, r.slot),
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
