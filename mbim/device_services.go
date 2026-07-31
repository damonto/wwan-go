package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type DeviceServicesRequest struct {
	TransactionID uint32
	Response      *DeviceServicesResponse
}

func (r *DeviceServicesRequest) Request() *Request {
	r.Response = new(DeviceServicesResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command: command(
			ServiceBasicConnect,
			CIDDeviceServices,
			CommandTypeQuery,
			nil,
		),
		Response: r.Response,
	}
}

type DeviceServicesResponse struct {
	MaxDSSSessions uint32
	Services       []DeviceService
}

func (r DeviceServicesResponse) SupportsCID(serviceID [16]byte, cid uint32) bool {
	return slices.ContainsFunc(r.Services, func(service DeviceService) bool {
		return service.ServiceID == serviceID && slices.Contains(service.CIDs, cid)
	})
}

func (r *DeviceServicesResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return errors.New("parsing MBIM device services: payload is truncated")
	}
	serviceCount := binary.LittleEndian.Uint32(data[:4])
	maxDSSSessions := binary.LittleEndian.Uint32(data[4:8])
	if maxDSSSessions > 256 {
		return fmt.Errorf("parsing MBIM device services: maximum DSS sessions %d exceeds 256", maxDSSSessions)
	}
	refs, err := offsetSizeRefs(data, 8, serviceCount)
	if err != nil {
		return fmt.Errorf("parsing MBIM device services: %w", err)
	}

	services := make([]DeviceService, serviceCount)
	for i, ref := range refs {
		if err := services[i].UnmarshalBinary(ref.bytes(data)); err != nil {
			return fmt.Errorf("parsing MBIM device service %d: %w", i, err)
		}
	}
	*r = DeviceServicesResponse{MaxDSSSessions: maxDSSSessions, Services: services}
	return nil
}

type DeviceService struct {
	ServiceID       [16]byte
	DSSPayload      uint32
	MaxDSSInstances uint32
	CIDs            []uint32
}

func (s *DeviceService) UnmarshalBinary(data []byte) error {
	if len(data) < 28 {
		return errors.New("device service is truncated")
	}
	cidCount := binary.LittleEndian.Uint32(data[24:28])
	wantLength := uint64(28) + uint64(cidCount)*4
	if uint64(len(data)) != wantLength {
		return fmt.Errorf("device service length %d, want %d for %d CIDs", len(data), wantLength, cidCount)
	}

	var serviceID [16]byte
	copy(serviceID[:], data[:16])
	cids := make([]uint32, cidCount)
	for i := range cidCount {
		offset := 28 + i*4
		cids[i] = binary.LittleEndian.Uint32(data[offset : offset+4])
	}
	*s = DeviceService{
		ServiceID:       serviceID,
		DSSPayload:      binary.LittleEndian.Uint32(data[16:20]),
		MaxDSSInstances: binary.LittleEndian.Uint32(data[20:24]),
		CIDs:            cids,
	}
	return nil
}

func (c *Client) DeviceServices(ctx context.Context) (DeviceServicesResponse, error) {
	request := DeviceServicesRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return DeviceServicesResponse{}, fmt.Errorf("reading MBIM device services: %w", err)
	}
	response := *request.Response
	response.Services = make([]DeviceService, len(request.Response.Services))
	for i, service := range request.Response.Services {
		response.Services[i] = service
		response.Services[i].CIDs = slices.Clone(service.CIDs)
	}
	return response, nil
}
