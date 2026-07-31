package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type ServiceActivationInfo struct {
	NwError              uint32
	VendorSpecificBuffer []byte
}

type ServiceActivationRequest struct {
	TransactionID        uint32
	VendorSpecificBuffer []byte
	Response             *ServiceActivationInfo
}

func (r *ServiceActivationRequest) Request() *Request {
	r.Response = new(ServiceActivationInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command(ServiceBasicConnect, CIDServiceActivation, CommandTypeSet, r.VendorSpecificBuffer),
		Response:      r.Response,
	}
}

func (r *ServiceActivationInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return errors.New("parsing MBIM service activation: payload is truncated")
	}
	*r = ServiceActivationInfo{
		NwError:              binary.LittleEndian.Uint32(data[0:4]),
		VendorSpecificBuffer: slices.Clone(data[4:]),
	}
	return nil
}

func (c *Client) ActivateService(ctx context.Context, vendorSpecificBuffer []byte) (ServiceActivationInfo, error) {
	request := ServiceActivationRequest{
		TransactionID:        c.nextTransactionID(),
		VendorSpecificBuffer: slices.Clone(vendorSpecificBuffer),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		response := *request.Response
		response.VendorSpecificBuffer = slices.Clone(response.VendorSpecificBuffer)
		return response, fmt.Errorf("activating MBIM service: %w", err)
	}
	response := *request.Response
	response.VendorSpecificBuffer = slices.Clone(response.VendorSpecificBuffer)
	return response, nil
}
