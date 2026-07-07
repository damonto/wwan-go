package uim

import (
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

// NASGetSysInfoRequest encodes QMI NAS Get System Info.
type NASGetSysInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI NAS request.
func (r NASGetSysInfoRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageNASGetSysInfo,
		Timeout:       r.Timeout,
	}
}

// NASGetSysInfoResponse is the parsed NAS Get System Info response.
type NASGetSysInfoResponse struct {
	SysInfo qcom.NASSysInfo
}

// UnmarshalTLVs parses the NAS Get System Info response fields used by IMS.
func (r *NASGetSysInfoResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = NASGetSysInfoResponse{}
	value, ok := tlv.Value(tlvs, 0x29)
	if !ok || len(value) == 0 {
		return nil
	}
	r.SysInfo = qcom.NASSysInfo{
		VoPSKnown:     true,
		VoPSSupported: value[0] == 1,
	}
	return nil
}
