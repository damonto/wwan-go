package qcom

import (
	"errors"

	"github.com/damonto/uicc-go/qcom/tlv"
)

type InternalOpenRequest struct {
	TransactionID uint16
	DevicePath    []byte
}

func (r InternalOpenRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageInternalProxyOpen,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, r.DevicePath),
		},
	}
}

type AllocateClientIDRequest struct {
	TransactionID uint16
	ServiceType   ServiceType
}

func (r AllocateClientIDRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageAllocateClientID,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Uint(0x01, r.ServiceType),
		},
	}
}

type AllocateClientIDResponse struct {
	ClientID uint8
}

func (r *AllocateClientIDResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = AllocateClientIDResponse{}
	if value, ok := tlvs.Find(0x01); ok && len(value.Value) >= 2 {
		r.ClientID = value.Value[1]
		return nil
	}
	return errors.New("parsing QMI allocate client ID response: client ID TLV missing")
}

type ReleaseClientIDRequest struct {
	ClientID      uint8
	TransactionID uint16
	ServiceType   ServiceType
}

func (r ReleaseClientIDRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageReleaseClientID,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte{byte(r.ServiceType), r.ClientID}),
		},
	}
}

type ReleaseClientIDResponse struct {
	ClientID uint8
}

func (r *ReleaseClientIDResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = ReleaseClientIDResponse{}
	if value, ok := tlvs.Find(0x01); ok && len(value.Value) >= 2 {
		r.ClientID = value.Value[1]
		return nil
	}
	return errors.New("parsing QMI release client ID response: client ID TLV missing")
}
