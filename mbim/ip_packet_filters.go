package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type PacketFilter struct {
	Filter []byte
	Mask   []byte
	ID     uint32
}

type IPPacketFiltersInfo struct {
	MBIMExVersion uint16
	SessionID     SessionID
	Filters       []PacketFilter
}

type IPPacketFiltersRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	SessionID     SessionID
	Response      *IPPacketFiltersInfo
}

func (r *IPPacketFiltersRequest) Request() *Request {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.SessionID))
	r.Response = &IPPacketFiltersInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDIPPacketFilters, CommandTypeQuery, data),
		Response:      r.Response,
	}
}

type IPPacketFiltersSetRequest struct {
	TransactionID uint32
	MBIMExVersion uint16
	Filters       IPPacketFiltersInfo
	Response      *IPPacketFiltersInfo
}

func (r *IPPacketFiltersSetRequest) Request() *Request {
	filters := r.Filters
	filters.MBIMExVersion = r.MBIMExVersion
	data, err := filters.MarshalBinary()
	r.Response = &IPPacketFiltersInfo{MBIMExVersion: r.MBIMExVersion}
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       commandWithError(ServiceBasicConnect, CIDIPPacketFilters, CommandTypeSet, data, err),
		Response:      r.Response,
	}
}

func (r IPPacketFiltersInfo) MarshalBinary() ([]byte, error) {
	if err := validateIPPacketFilters(r.MBIMExVersion, r.Filters); err != nil {
		return nil, fmt.Errorf("encoding MBIM IP packet filters: %w", err)
	}
	return r.marshalBinary(r.MBIMExVersion), nil
}

func validateIPPacketFilters(version uint16, filters []PacketFilter) error {
	filterIDs := make(map[uint32]int, len(filters))
	for i, filter := range filters {
		if len(filter.Filter) != len(filter.Mask) {
			return fmt.Errorf("filter %d mask length %d, want %d", i, len(filter.Mask), len(filter.Filter))
		}
		if version < mbimExVersion30 {
			continue
		}
		if previous, ok := filterIDs[filter.ID]; ok {
			return fmt.Errorf("filter %d ID %d duplicates filter %d", i, filter.ID, previous)
		}
		filterIDs[filter.ID] = i
	}
	return nil
}

func (r IPPacketFiltersInfo) marshalBinary(version uint16) []byte {
	elements := make([][]byte, len(r.Filters))
	for i, filter := range r.Filters {
		fixedSize := 12
		if version >= mbimExVersion30 {
			fixedSize = 16
		}
		data := make([]byte, fixedSize)
		binary.LittleEndian.PutUint32(data[0:4], uint32(len(filter.Filter)))
		if fixedSize == 16 {
			binary.LittleEndian.PutUint32(data[12:16], filter.ID)
		}
		data = appendRefValue(data, 4, filter.Filter)
		if len(filter.Mask) != 0 {
			binary.LittleEndian.PutUint32(data[8:12], uint32(len(data)))
			data = append(data, filter.Mask...)
			for len(data)%4 != 0 {
				data = append(data, 0)
			}
		}
		elements[i] = data
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(r.SessionID))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

func (r *IPPacketFiltersInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return errors.New("parsing MBIM IP packet filters: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[4:8])
	refs, err := offsetSizeRefs(data, 8, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM IP packet filters: %w", err)
	}
	filters := make([]PacketFilter, count)
	fixedSize := 12
	if r.MBIMExVersion >= mbimExVersion30 {
		fixedSize = 16
	}
	for i, ref := range refs {
		record := data[ref.offset : ref.offset+ref.size]
		if len(record) < fixedSize {
			return fmt.Errorf("parsing MBIM IP packet filter %d: payload is truncated", i)
		}
		filterSize := binary.LittleEndian.Uint32(record[0:4])
		filterOffset := binary.LittleEndian.Uint32(record[4:8])
		maskOffset := binary.LittleEndian.Uint32(record[8:12])
		filterRef := valueRef{offset: filterOffset, size: filterSize}
		maskRef := valueRef{offset: maskOffset, size: filterSize}
		if err := validateDataBufferRefs(record, uint32(fixedSize), []valueRef{filterRef, maskRef}); err != nil {
			return fmt.Errorf("parsing MBIM IP packet filter %d data buffer: %w", i, err)
		}
		filters[i] = PacketFilter{
			Filter: filterRef.bytes(record),
			Mask:   maskRef.bytes(record),
		}
		if fixedSize == 16 {
			filters[i].ID = binary.LittleEndian.Uint32(record[12:16])
		}
	}
	version := r.MBIMExVersion
	if err := validateIPPacketFilters(version, filters); err != nil {
		return fmt.Errorf("parsing MBIM IP packet filters: %w", err)
	}
	*r = IPPacketFiltersInfo{
		MBIMExVersion: version,
		SessionID:     SessionID(binary.LittleEndian.Uint32(data[0:4])),
		Filters:       filters,
	}
	return nil
}

func (c *Client) IPPacketFilters(ctx context.Context, sessionID SessionID) (IPPacketFiltersInfo, error) {
	request := IPPacketFiltersRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		SessionID:     sessionID,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return IPPacketFiltersInfo{}, fmt.Errorf("reading MBIM IP packet filters: %w", err)
	}
	return cloneIPPacketFilters(*request.Response), nil
}

func (c *Client) SetIPPacketFilters(ctx context.Context, filters IPPacketFiltersInfo) (IPPacketFiltersInfo, error) {
	if err := validateIPPacketFilters(c.mbimExVersion, filters.Filters); err != nil {
		return IPPacketFiltersInfo{}, fmt.Errorf("setting MBIM IP packet filters: %w", err)
	}
	filters.MBIMExVersion = c.mbimExVersion
	request := IPPacketFiltersSetRequest{
		TransactionID: c.nextTransactionID(),
		MBIMExVersion: c.mbimExVersion,
		Filters:       cloneIPPacketFilters(filters),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return IPPacketFiltersInfo{}, fmt.Errorf("setting MBIM IP packet filters: %w", err)
	}
	return cloneIPPacketFilters(*request.Response), nil
}

func cloneIPPacketFilters(info IPPacketFiltersInfo) IPPacketFiltersInfo {
	out := info
	out.Filters = slices.Clone(info.Filters)
	for i := range out.Filters {
		out.Filters[i].Filter = slices.Clone(out.Filters[i].Filter)
		out.Filters[i].Mask = slices.Clone(out.Filters[i].Mask)
	}
	return out
}
