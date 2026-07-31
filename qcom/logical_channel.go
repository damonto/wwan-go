package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	maxLogicalChannelAIDLength            = uimAIDMaxLength
	maxLogicalChannelSelectResponseLength = 1024
	maxAPDUDataLength                     = 1024
)

// UIMFileControlInformation selects the data returned by an application select.
type UIMFileControlInformation uint8

const (
	UIMFileControlNoData UIMFileControlInformation = iota
	UIMFileControlFCP
	UIMFileControlFCI
	UIMFileControlFCIWithInterfaces
	UIMFileControlFMD
)

// UIMOpenLogicalChannelConfig selects an application and optional select response format.
type UIMOpenLogicalChannelConfig struct {
	AID                    []byte
	FileControlInformation *UIMFileControlInformation
}

// UIMAPDUProcedureBytes controls whether intermediate procedure bytes are returned.
type UIMAPDUProcedureBytes uint8

const (
	UIMAPDUReturnProcedureBytes UIMAPDUProcedureBytes = iota
	UIMAPDUSkipProcedureBytes
)

// UIMAPDURequest contains the channel, command, and optional procedure-byte policy.
type UIMAPDURequest struct {
	Channel        uint8
	Command        []byte
	ProcedureBytes *UIMAPDUProcedureBytes
}

func (c *Client) OpenLogicalChannel(ctx context.Context, aid []byte) (uint8, error) {
	response, err := c.OpenLogicalChannelWithConfig(ctx, UIMOpenLogicalChannelConfig{AID: aid})
	if err != nil {
		return 0, err
	}
	return response.Channel, nil
}

// OpenLogicalChannelWithConfig opens an application channel and returns its select response.
func (c *Client) OpenLogicalChannelWithConfig(ctx context.Context, config UIMOpenLogicalChannelConfig) (OpenLogicalChannelResponse, error) {
	tlvs := make(tlv.TLVs, 0, 3)
	if len(config.AID) > 0 {
		request := OpenLogicalChannelRequest{AID: slices.Clone(config.AID)}
		value, err := request.MarshalBinary()
		if err != nil {
			return OpenLogicalChannelResponse{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	tlvs = append(tlvs, tlv.Uint(0x01, c.slot))
	if config.FileControlInformation != nil {
		if *config.FileControlInformation > UIMFileControlFMD {
			return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: file-control information %d is out of range", *config.FileControlInformation)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*config.FileControlInformation)))
	}
	resp, err := c.request(ctx, MessageOpenLogicalChannel, tlvs)
	if err != nil {
		return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}
	if err := cardErrorAt(resp.TLVs, 0x11); err != nil {
		return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return OpenLogicalChannelResponse{}, errors.New("opening QMI UIM logical channel: channel TLV missing")
	}

	var response OpenLogicalChannelResponse
	if err := response.UnmarshalBinary(value); err != nil {
		return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}
	if value, ok := tlv.Value(resp.TLVs, 0x12); ok {
		response.SelectResponse, err = decodeQMILength8Bytes(value)
		if err != nil {
			return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: select response: %w", err)
		}
	} else if value, ok := tlv.Value(resp.TLVs, 0x13); ok {
		response.SelectResponse, err = decodeLengthPrefixedBytes(value)
		if err != nil {
			return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: long select response: %w", err)
		}
		if len(response.SelectResponse) > maxLogicalChannelSelectResponseLength {
			return OpenLogicalChannelResponse{}, fmt.Errorf("opening QMI UIM logical channel: long select response length %d exceeds %d", len(response.SelectResponse), maxLogicalChannelSelectResponseLength)
		}
	}
	return response, nil
}

func (c *Client) CloseLogicalChannel(ctx context.Context, channel uint8) error {
	request := CloseLogicalChannelRequest{Channel: channel}
	value, err := request.MarshalBinary()
	if err != nil {
		return err
	}

	resp, err := c.request(ctx, MessageCloseLogicalChannel, tlv.TLVs{
		tlv.Uint(0x01, c.slot),
		tlv.Bytes(0x11, value),
	})
	if err != nil {
		return fmt.Errorf("closing QMI UIM logical channel: %w", err)
	}
	if err := cardResultOK(resp); err != nil {
		return fmt.Errorf("closing QMI UIM logical channel: %w", err)
	}
	return nil
}

func (c *Client) SendAPDU(ctx context.Context, channel uint8, command []byte) ([]byte, error) {
	return c.SendAPDUWithOptions(ctx, UIMAPDURequest{Channel: channel, Command: command})
}

// SendAPDUWithOptions sends an APDU with an optional procedure-byte policy.
func (c *Client) SendAPDUWithOptions(ctx context.Context, req UIMAPDURequest) ([]byte, error) {
	request := SendAPDURequest{Command: slices.Clone(req.Command)}
	value, err := request.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tlvs := tlv.TLVs{
		tlv.Uint(0x10, req.Channel),
		tlv.Bytes(0x02, value),
		tlv.Uint(0x01, c.slot),
	}
	if req.ProcedureBytes != nil {
		if *req.ProcedureBytes > UIMAPDUSkipProcedureBytes {
			return nil, fmt.Errorf("sending QMI UIM APDU: procedure-byte policy %d is out of range", *req.ProcedureBytes)
		}
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(*req.ProcedureBytes)))
	}
	resp, err := c.request(ctx, MessageSendAPDU, tlvs)
	if err != nil {
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}
	if err := resultOK(resp); err != nil {
		if errors.Is(err, QMIErrorInsufficientResources) {
			if _, ok := tlv.Value(resp.TLVs, 0x11); ok {
				return nil, errors.New("sending QMI UIM APDU: long response is not supported")
			}
		}
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		if _, ok := tlv.Value(resp.TLVs, 0x11); ok {
			return nil, errors.New("sending QMI UIM APDU: long response is not supported")
		}
		if err := cardError(resp.TLVs); err != nil {
			return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
		}
		return nil, errors.New("sending QMI UIM APDU: response TLV missing")
	}

	var response SendAPDUResponse
	if err := response.UnmarshalBinary(value); err != nil {
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}
	return response.Response, nil
}

func (r OpenLogicalChannelRequest) MarshalBinary() ([]byte, error) {
	if err := validateUIMAIDLength(r.AID); err != nil {
		return nil, fmt.Errorf("marshaling QMI UIM open logical channel request: %w", err)
	}

	data := make([]byte, 0, 1+len(r.AID))
	data = append(data, byte(len(r.AID)))
	data = append(data, r.AID...)
	return data, nil
}

func (r *OpenLogicalChannelRequest) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return errors.New("unmarshaling QMI UIM open logical channel request: AID length is missing")
	}

	length := int(data[0])
	if len(data) != 1+length {
		return fmt.Errorf("unmarshaling QMI UIM open logical channel request: AID length %d does not match actual length %d", length, len(data)-1)
	}
	if length > maxLogicalChannelAIDLength {
		return fmt.Errorf("unmarshaling QMI UIM open logical channel request: AID length %d exceeds %d", length, maxLogicalChannelAIDLength)
	}
	r.AID = slices.Clone(data[1:])
	return nil
}

func (r *OpenLogicalChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return fmt.Errorf("unmarshaling QMI UIM open logical channel response: length %d, want 1", len(data))
	}
	r.Channel = data[0]
	return nil
}

func (r CloseLogicalChannelRequest) MarshalBinary() ([]byte, error) {
	return []byte{r.Channel}, nil
}

func (r *CloseLogicalChannelRequest) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return fmt.Errorf("unmarshaling QMI UIM close logical channel request: length %d, want 1", len(data))
	}
	r.Channel = data[0]
	return nil
}

func (r *CloseLogicalChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("unmarshaling QMI UIM close logical channel response: length %d, want 0", len(data))
	}
	return nil
}

func (r SendAPDURequest) MarshalBinary() ([]byte, error) {
	if len(r.Command) > maxAPDUDataLength {
		return nil, fmt.Errorf("marshaling QMI UIM APDU request: command length %d exceeds %d", len(r.Command), maxAPDUDataLength)
	}

	data := binary.LittleEndian.AppendUint16(nil, uint16(len(r.Command)))
	data = append(data, r.Command...)
	return data, nil
}

func (r *SendAPDURequest) UnmarshalBinary(data []byte) error {
	command, err := decodeLengthPrefixedBytes(data)
	if err != nil {
		return fmt.Errorf("unmarshaling QMI UIM APDU request: %w", err)
	}
	if len(command) > maxAPDUDataLength {
		return fmt.Errorf("unmarshaling QMI UIM APDU request: command length %d exceeds %d", len(command), maxAPDUDataLength)
	}
	r.Command = command
	return nil
}

func (r *SendAPDUResponse) UnmarshalBinary(data []byte) error {
	response, err := decodeLengthPrefixedBytes(data)
	if err != nil {
		return fmt.Errorf("unmarshaling QMI UIM APDU response: %w", err)
	}
	if len(response) > maxAPDUDataLength {
		return fmt.Errorf("unmarshaling QMI UIM APDU response: response length %d exceeds %d", len(response), maxAPDUDataLength)
	}
	r.Response = response
	return nil
}
