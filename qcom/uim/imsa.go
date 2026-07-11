package uim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	imsaTLVIMSRegistered = 0x10
	imsaTLVFailureCode   = 0x11
	imsaTLVRegStatus     = 0x12
	imsaTLVVoIPService   = 0x11
	imsaTLVVoIPRAT       = 0x14
)

// IMSAGetRegistrationStatusRequest encodes QMI IMSA Get Registration Status.
type IMSAGetRegistrationStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI IMSA request.
func (r IMSAGetRegistrationStatusRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageIMSAGetRegistrationStatus,
		Timeout:       r.Timeout,
	}
}

// IMSAGetServiceStatusRequest encodes QMI IMSA Get Service Status.
type IMSAGetServiceStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI IMSA request.
func (r IMSAGetServiceStatusRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceIMSA,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageIMSAGetServiceStatus,
		Timeout:       r.Timeout,
	}
}

// IMSAStatus reads IMS registration and VoIP service state from QMI IMSA.
func (r *Reader) IMSAStatus(ctx context.Context) (qcom.IMSAStatus, error) {
	if r == nil {
		return qcom.IMSAStatus{}, errors.New("querying QMI IMSA status: reader is nil")
	}
	var status qcom.IMSAStatus
	err := r.withServiceClient(ctx, qcom.ServiceIMSA, func(clientID uint8) error {
		var err error
		status, err = r.imsaStatus(ctx, clientID)
		return err
	})
	if err != nil {
		return qcom.IMSAStatus{}, fmt.Errorf("querying QMI IMSA status: %w", err)
	}
	return status, nil
}

func (r *Reader) imsaStatus(ctx context.Context, clientID uint8) (qcom.IMSAStatus, error) {
	regReq := IMSAGetRegistrationStatusRequest{
		ClientID: clientID,
		Timeout:  DefaultRequestTimeout,
	}.Request()
	regResp, err := r.requestServiceWithTimeout(ctx, regReq.Service, regReq.ClientID, regReq.MessageID, regReq.TLVs, regReq.Timeout)
	if err != nil {
		return qcom.IMSAStatus{}, err
	}
	if err := resultOK(regResp); err != nil {
		return qcom.IMSAStatus{}, err
	}
	var regStatus IMSARegistrationStatusResponse
	if err := regStatus.UnmarshalTLVs(regResp.TLVs); err != nil {
		return qcom.IMSAStatus{}, err
	}

	serviceReq := IMSAGetServiceStatusRequest{
		ClientID: clientID,
		Timeout:  DefaultRequestTimeout,
	}.Request()
	serviceResp, err := r.requestServiceWithTimeout(ctx, serviceReq.Service, serviceReq.ClientID, serviceReq.MessageID, serviceReq.TLVs, serviceReq.Timeout)
	if err != nil {
		return qcom.IMSAStatus{}, err
	}
	if err := resultOK(serviceResp); err != nil {
		return qcom.IMSAStatus{}, err
	}
	var serviceStatus IMSAServiceStatusResponse
	if err := serviceStatus.UnmarshalTLVs(serviceResp.TLVs); err != nil {
		return qcom.IMSAStatus{}, err
	}

	regStatus.Status.VoIPServiceKnown = serviceStatus.Status.VoIPServiceKnown
	regStatus.Status.VoIPService = serviceStatus.Status.VoIPService
	regStatus.Status.VoIPRATKnown = serviceStatus.Status.VoIPRATKnown
	regStatus.Status.VoIPRAT = serviceStatus.Status.VoIPRAT
	return regStatus.Status, nil
}

// IMSARegistrationStatusResponse is the parsed QMI IMSA registration status.
type IMSARegistrationStatusResponse struct {
	Status qcom.IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA registration fields.
func (r *IMSARegistrationStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSARegistrationStatusResponse{}
	if value, ok := tlv.Value(tlvs, imsaTLVRegStatus); ok {
		if len(value) < 4 {
			return errors.New("parsing QMI IMSA registration status: status TLV is truncated")
		}
		r.Status.RegistrationKnown = true
		r.Status.Registration = qcom.IMSRegistrationStatus(binary.LittleEndian.Uint32(value[:4]))
	} else if value, ok := tlv.Value(tlvs, imsaTLVIMSRegistered); ok {
		if len(value) == 0 {
			return errors.New("parsing QMI IMSA registration status: registered TLV is truncated")
		}
		r.Status.RegistrationKnown = true
		if value[0] != 0 {
			r.Status.Registration = qcom.IMSRegistrationStatusRegistered
		} else {
			r.Status.Registration = qcom.IMSRegistrationStatusNotRegistered
		}
	}
	if value, ok := tlv.Value(tlvs, imsaTLVFailureCode); ok {
		if len(value) < 2 {
			return errors.New("parsing QMI IMSA registration status: failure code TLV is truncated")
		}
		r.Status.FailureCodeKnown = true
		r.Status.FailureCode = binary.LittleEndian.Uint16(value[:2])
	}
	return nil
}

// IMSAServiceStatusResponse is the parsed QMI IMSA service status.
type IMSAServiceStatusResponse struct {
	Status qcom.IMSAStatus
}

// UnmarshalTLVs parses QMI IMSA VoIP service fields.
func (r *IMSAServiceStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSAServiceStatusResponse{}
	if value, ok := tlv.Value(tlvs, imsaTLVVoIPService); ok {
		if len(value) < 4 {
			return errors.New("parsing QMI IMSA service status: VoIP service TLV is truncated")
		}
		r.Status.VoIPServiceKnown = true
		r.Status.VoIPService = qcom.IMSServiceStatus(binary.LittleEndian.Uint32(value[:4]))
	}
	if value, ok := tlv.Value(tlvs, imsaTLVVoIPRAT); ok {
		if len(value) < 4 {
			return errors.New("parsing QMI IMSA service status: VoIP RAT TLV is truncated")
		}
		r.Status.VoIPRATKnown = true
		r.Status.VoIPRAT = qcom.IMSServiceRAT(binary.LittleEndian.Uint32(value[:4]))
	}
	return nil
}
