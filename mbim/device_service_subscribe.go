package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type DeviceServiceSubscribeList struct {
	Entries []DeviceServiceSubscribeEntry
}

type DeviceServiceSubscribeEntry struct {
	ServiceID [16]byte
	CIDs      []uint32
}

func (e DeviceServiceSubscribeEntry) MarshalBinary() ([]byte, error) {
	return e.marshalBinary(), nil
}

func (e DeviceServiceSubscribeEntry) marshalBinary() []byte {
	data := make([]byte, 0, 20+len(e.CIDs)*4)
	data = append(data, e.ServiceID[:]...)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(e.CIDs)))
	for _, cid := range e.CIDs {
		data = binary.LittleEndian.AppendUint32(data, cid)
	}
	return data
}

func (e *DeviceServiceSubscribeEntry) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM device service subscription: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[16:20])
	wantLength := uint64(20) + uint64(count)*4
	if uint64(len(data)) != wantLength {
		return fmt.Errorf("parsing MBIM device service subscription: payload length %d, want %d for %d CIDs", len(data), wantLength, count)
	}
	var serviceID [16]byte
	copy(serviceID[:], data[0:16])
	cids := make([]uint32, count)
	for i := range count {
		offset := 20 + i*4
		cids[i] = binary.LittleEndian.Uint32(data[offset : offset+4])
	}
	*e = DeviceServiceSubscribeEntry{ServiceID: serviceID, CIDs: cids}
	return nil
}

func (l DeviceServiceSubscribeList) MarshalBinary() ([]byte, error) {
	return l.marshalBinary(), nil
}

func (l DeviceServiceSubscribeList) marshalBinary() []byte {
	elements := make([][]byte, len(l.Entries))
	for i, entry := range l.Entries {
		elements[i] = entry.marshalBinary()
	}
	header := binary.LittleEndian.AppendUint32(nil, uint32(len(elements)))
	return appendOffsetSizeElements(header, elements)
}

func (l *DeviceServiceSubscribeList) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM device service subscriptions: payload is truncated")
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	refs, err := offsetSizeRefs(data, 4, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM device service subscriptions: %w", err)
	}
	entries := make([]DeviceServiceSubscribeEntry, count)
	for i, ref := range refs {
		if err := entries[i].UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
			return fmt.Errorf("parsing MBIM device service subscription %d: %w", i, err)
		}
	}
	l.Entries = entries
	return nil
}

type DeviceServiceSubscribeListRequest struct {
	TransactionID uint32
	List          DeviceServiceSubscribeList
	Response      *DeviceServiceSubscribeList
}

func (r *DeviceServiceSubscribeListRequest) Request() *Request {
	data, err := r.List.MarshalBinary()
	r.Response = new(DeviceServiceSubscribeList)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       commandWithError(ServiceBasicConnect, CIDDeviceServiceSubscribeList, CommandTypeSet, data, err),
		Response:      r.Response,
	}
}

func (c *Client) SetDeviceServiceSubscribeList(ctx context.Context, list DeviceServiceSubscribeList) (DeviceServiceSubscribeList, error) {
	request := DeviceServiceSubscribeListRequest{TransactionID: c.nextTransactionID(), List: list}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return DeviceServiceSubscribeList{}, fmt.Errorf("setting MBIM device service subscriptions: %w", err)
	}
	return cloneDeviceServiceSubscribeList(*request.Response), nil
}

func cloneDeviceServiceSubscribeList(list DeviceServiceSubscribeList) DeviceServiceSubscribeList {
	out := DeviceServiceSubscribeList{Entries: slices.Clone(list.Entries)}
	for i := range out.Entries {
		out.Entries[i].CIDs = slices.Clone(out.Entries[i].CIDs)
	}
	return out
}
