package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

type WakeType uint32

const (
	WakeTypeCIDResponse   WakeType = 0
	WakeTypeCIDIndication WakeType = 1
	WakeTypePacket        WakeType = 2
)

const mbimWakeReasonResponseTimeout = 10 * time.Second

type WakeReasonRequest struct {
	TransactionID uint32
	Response      *WakeReasonInfo
}

func (r *WakeReasonRequest) Request() *Request {
	r.Response = new(WakeReasonInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimWakeReasonResponseTimeout,
		Command: command(
			ServiceMsBasicConnectExtensions,
			CIDMsWakeReason,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type WakeReasonInfo struct {
	Type      WakeType
	SessionID SessionID
	TLV       TLV
	Command   *WakeCommand
	Packet    *WakePacket
}

func (i WakeReasonInfo) MarshalBinary() ([]byte, error) {
	var tlv TLV
	switch i.Type {
	case WakeTypeCIDResponse, WakeTypeCIDIndication:
		if i.Command == nil || i.Packet != nil {
			return nil, errors.New("encoding MBIM wake reason: CID wake requires only a wake command")
		}
		data, err := i.Command.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding MBIM wake command: %w", err)
		}
		tlv = TLV{Type: TLVTypeWakeCommand, Data: data}
	case WakeTypePacket:
		if i.Packet == nil || i.Command != nil {
			return nil, errors.New("encoding MBIM wake reason: packet wake requires only a wake packet")
		}
		data, err := i.Packet.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding MBIM wake packet: %w", err)
		}
		tlv = TLV{Type: TLVTypeWakePacket, Data: data}
	default:
		return nil, fmt.Errorf("encoding MBIM wake reason: wake type %d is reserved", i.Type)
	}
	tlvData, err := tlv.MarshalBinary()
	if err != nil {
		return nil, err
	}
	data := binary.LittleEndian.AppendUint32(nil, uint32(i.Type))
	data = binary.LittleEndian.AppendUint32(data, uint32(i.SessionID))
	return append(data, tlvData...), nil
}

func (i *WakeReasonInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("parsing MBIM wake reason: payload is truncated")
	}
	var tlv TLV
	if err := tlv.UnmarshalBinary(data[8:]); err != nil {
		return fmt.Errorf("parsing MBIM wake reason TLV: %w", err)
	}

	wakeType := WakeType(binary.LittleEndian.Uint32(data[:4]))
	result := WakeReasonInfo{
		Type:      wakeType,
		SessionID: SessionID(binary.LittleEndian.Uint32(data[4:8])),
		TLV:       tlv,
	}
	switch wakeType {
	case WakeTypeCIDResponse, WakeTypeCIDIndication:
		if tlv.Type != TLVTypeWakeCommand {
			return fmt.Errorf("parsing MBIM wake reason: TLV type is %d, want %d", tlv.Type, TLVTypeWakeCommand)
		}
		result.Command = new(WakeCommand)
		if err := result.Command.UnmarshalBinary(tlv.Data); err != nil {
			return fmt.Errorf("parsing MBIM wake command: %w", err)
		}
	case WakeTypePacket:
		if tlv.Type != TLVTypeWakePacket {
			return fmt.Errorf("parsing MBIM wake reason: TLV type is %d, want %d", tlv.Type, TLVTypeWakePacket)
		}
		result.Packet = new(WakePacket)
		if err := result.Packet.UnmarshalBinary(tlv.Data); err != nil {
			return fmt.Errorf("parsing MBIM wake packet: %w", err)
		}
	default:
		return fmt.Errorf("parsing MBIM wake reason: wake type %d is reserved", wakeType)
	}
	*i = result
	return nil
}

type WakeCommand struct {
	ServiceID [16]byte
	CID       uint32
	Payload   []byte
}

func (c WakeCommand) MarshalBinary() ([]byte, error) {
	if uint64(len(c.Payload)) > uint64(^uint32(0)) {
		return nil, errors.New("wake command payload exceeds UINT32 length")
	}
	if err := validateWakeCommandPayload(c.ServiceID, c.CID, c.Payload); err != nil {
		return nil, err
	}
	data := make([]byte, 28)
	copy(data[:16], c.ServiceID[:])
	binary.LittleEndian.PutUint32(data[16:20], c.CID)
	if len(c.Payload) > 0 {
		binary.LittleEndian.PutUint32(data[20:24], 28)
		binary.LittleEndian.PutUint32(data[24:28], uint32(len(c.Payload)))
		data = append(data, c.Payload...)
	}
	return padTo4Bytes(data), nil
}

func (c *WakeCommand) UnmarshalBinary(data []byte) error {
	if len(data) < 28 {
		return errors.New("wake command is truncated")
	}
	ref, err := readOffsetSizeRef(data, 20)
	if err != nil {
		return fmt.Errorf("reading payload reference: %w", err)
	}
	if err := validateDataBufferRefs(data, 28, []valueRef{ref}); err != nil {
		return fmt.Errorf("reading payload: %w", err)
	}
	payload := ref.bytes(data)
	var serviceID [16]byte
	copy(serviceID[:], data[:16])
	cid := binary.LittleEndian.Uint32(data[16:20])
	if err := validateWakeCommandPayload(serviceID, cid, payload); err != nil {
		return err
	}
	*c = WakeCommand{
		ServiceID: serviceID,
		CID:       cid,
		Payload:   payload,
	}
	return nil
}

func validateWakeCommandPayload(serviceID [16]byte, cid uint32, payload []byte) error {
	if serviceID != ServiceBasicConnect || cid != CIDConnect {
		return nil
	}
	if len(payload) != 4 {
		return fmt.Errorf("wake command connect payload length is %d, want 4", len(payload))
	}
	if value := binary.LittleEndian.Uint32(payload); value > 1 {
		return fmt.Errorf("wake command connect payload value is %d, want 0 or 1", value)
	}
	return nil
}

type WakePacket struct {
	FilterID           uint32
	OriginalPacketSize uint32
	SavedPacket        []byte
}

func (p WakePacket) MarshalBinary() ([]byte, error) {
	if uint64(len(p.SavedPacket)) > uint64(^uint32(0)) {
		return nil, errors.New("saved wake packet exceeds UINT32 length")
	}
	if uint32(len(p.SavedPacket)) > p.OriginalPacketSize {
		return nil, fmt.Errorf("saved wake packet length %d exceeds original packet size %d", len(p.SavedPacket), p.OriginalPacketSize)
	}
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], p.FilterID)
	binary.LittleEndian.PutUint32(data[4:8], p.OriginalPacketSize)
	if len(p.SavedPacket) > 0 {
		binary.LittleEndian.PutUint32(data[8:12], 16)
		binary.LittleEndian.PutUint32(data[12:16], uint32(len(p.SavedPacket)))
		data = append(data, p.SavedPacket...)
	}
	return padTo4Bytes(data), nil
}

func (p *WakePacket) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("wake packet is truncated")
	}
	ref, err := readOffsetSizeRef(data, 8)
	if err != nil {
		return fmt.Errorf("reading saved packet reference: %w", err)
	}
	if err := validateDataBufferRefs(data, 16, []valueRef{ref}); err != nil {
		return fmt.Errorf("reading saved packet: %w", err)
	}
	originalPacketSize := binary.LittleEndian.Uint32(data[4:8])
	if ref.size > originalPacketSize {
		return fmt.Errorf("saved packet size %d exceeds original packet size %d", ref.size, originalPacketSize)
	}
	*p = WakePacket{
		FilterID:           binary.LittleEndian.Uint32(data[0:4]),
		OriginalPacketSize: originalPacketSize,
		SavedPacket:        ref.bytes(data),
	}
	return nil
}

func (c *Client) WakeReason(ctx context.Context) (WakeReasonInfo, error) {
	request := WakeReasonRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return WakeReasonInfo{}, fmt.Errorf("reading MBIM wake reason: %w", err)
	}
	return *request.Response, nil
}
