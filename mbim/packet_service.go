package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type PacketServiceRequest struct {
	TransactionID uint32
	Response      *PacketServiceInfo
}

func (r *PacketServiceRequest) Request() *Request {
	r.Response = new(PacketServiceInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDPacketService,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type PacketServiceSetRequest struct {
	TransactionID uint32
	Action        PacketServiceAction
	Response      *PacketServiceInfo
}

func (r *PacketServiceSetRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Action))

	r.Response = new(PacketServiceInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDPacketService,
			CommandTypeSet,
			data,
		),
		Response: r.Response,
	}
}

func (r *PacketServiceInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 28 {
		return errors.New("parsing MBIM packet service: payload is truncated")
	}
	r.NwError = binary.LittleEndian.Uint32(data[:4])
	r.PacketServiceState = PacketServiceState(binary.LittleEndian.Uint32(data[4:8]))
	r.HighestAvailableDataClass = binary.LittleEndian.Uint32(data[8:12])
	r.UplinkSpeed = binary.LittleEndian.Uint64(data[12:20])
	r.DownlinkSpeed = binary.LittleEndian.Uint64(data[20:28])
	return nil
}

func (r *Reader) PacketService(ctx context.Context) (PacketServiceInfo, error) {
	request := PacketServiceRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return PacketServiceInfo{}, fmt.Errorf("reading MBIM packet service: %w", err)
	}
	return *request.Response, nil
}

func (r *Reader) SetPacketService(ctx context.Context, action PacketServiceAction) (PacketServiceInfo, error) {
	request := PacketServiceSetRequest{
		TransactionID: r.nextTransactionID(),
		Action:        action,
	}
	if err := r.transmit(ctx, request.Request()); err != nil {
		return PacketServiceInfo{}, fmt.Errorf("setting MBIM packet service: %w", err)
	}
	return *request.Response, nil
}
