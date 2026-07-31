package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
)

type PacketStatisticsInfo struct {
	InOctets    uint64
	InPackets   uint64
	InErrors    uint32
	InDiscards  uint32
	OutOctets   uint64
	OutPackets  uint64
	OutErrors   uint32
	OutDiscards uint32
}

type PacketStatisticsRequest struct {
	TransactionID uint32
	Response      *PacketStatisticsInfo
}

func (r *PacketStatisticsRequest) Request() *Request {
	r.Response = new(PacketStatisticsInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDPacketStatistics, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *PacketStatisticsInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 48 {
		return fmt.Errorf("parsing MBIM packet statistics: payload length is %d, want 48", len(data))
	}
	*r = PacketStatisticsInfo{
		InDiscards:  binary.LittleEndian.Uint32(data[0:4]),
		InErrors:    binary.LittleEndian.Uint32(data[4:8]),
		InOctets:    binary.LittleEndian.Uint64(data[8:16]),
		InPackets:   binary.LittleEndian.Uint64(data[16:24]),
		OutOctets:   binary.LittleEndian.Uint64(data[24:32]),
		OutPackets:  binary.LittleEndian.Uint64(data[32:40]),
		OutErrors:   binary.LittleEndian.Uint32(data[40:44]),
		OutDiscards: binary.LittleEndian.Uint32(data[44:48]),
	}
	return nil
}

func (c *Client) PacketStatistics(ctx context.Context) (PacketStatisticsInfo, error) {
	request := PacketStatisticsRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return PacketStatisticsInfo{}, fmt.Errorf("reading MBIM packet statistics: %w", err)
	}
	return *request.Response, nil
}
