package uim

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	dmsTLVOperatingMode       = 0x01
	dmsTLVReportOperatingMode = 0x14
)

// DMSGetOperatingModeRequest encodes QMI DMS Get Operating Mode.
type DMSGetOperatingModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetOperatingModeRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageDMSGetOperatingMode,
		Timeout:       r.Timeout,
	}
}

// DMSSetOperatingModeRequest encodes QMI DMS Set Operating Mode.
type DMSSetOperatingModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mode          qcom.DMSOperatingMode
}

// Request converts the request into a QMI DMS request.
func (r DMSSetOperatingModeRequest) Request() qcom.Request {
	return qcom.Request{
		Service:       qcom.ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageDMSSetOperatingMode,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(dmsTLVOperatingMode, uint8(r.Mode)),
		},
	}
}

// DMSSetEventReportRequest encodes QMI DMS Set Event Report for operating mode.
type DMSSetEventReportRequest struct {
	ClientID            uint8
	TransactionID       uint16
	Timeout             time.Duration
	ReportOperatingMode bool
}

// Request converts the request into a QMI DMS request.
func (r DMSSetEventReportRequest) Request() qcom.Request {
	report := uint8(0)
	if r.ReportOperatingMode {
		report = 1
	}

	return qcom.Request{
		Service:       qcom.ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     qcom.MessageDMSSetEventReport,
		Timeout:       r.Timeout,
		TLVs: tlv.TLVs{
			tlv.Uint(dmsTLVReportOperatingMode, report),
		},
	}
}

// OperatingMode reads the current QMI DMS modem operating mode.
func (r *Reader) OperatingMode(ctx context.Context) (qcom.DMSOperatingMode, error) {
	var mode qcom.DMSOperatingMode
	err := r.withServiceClient(ctx, qcom.ServiceDMS, func(clientID uint8) error {
		req := DMSGetOperatingModeRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
		}.Request()
		resp, err := r.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}

		mode, err = UnmarshalDMSOperatingMode(resp.TLVs)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("querying QMI DMS operating mode: %w", err)
	}
	return mode, nil
}

// SetOperatingMode sets the QMI DMS modem operating mode.
func (r *Reader) SetOperatingMode(ctx context.Context, mode qcom.DMSOperatingMode) error {
	err := r.withServiceClient(ctx, qcom.ServiceDMS, func(clientID uint8) error {
		req := DMSSetOperatingModeRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Mode:     mode,
		}.Request()
		resp, err := r.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("setting QMI DMS operating mode: %w", err)
	}
	return nil
}

func (r *Reader) withServiceClient(ctx context.Context, service qcom.ServiceType, fn func(uint8) error) error {
	r.mu.Lock()
	transport := r.transport
	closed := r.closed || transport == nil
	r.mu.Unlock()
	if closed {
		return errReaderClosed
	}

	if boundService, ok := boundQMIService(transport); ok {
		if boundService != service {
			return fmt.Errorf("QMI transport is bound to service 0x%02X, want 0x%02X", boundService, service)
		}
		return fn(0)
	}

	clientID, err := r.allocateServiceClientID(ctx, service)
	if err != nil {
		return err
	}
	err = fn(clientID)
	releaseErr := r.releaseServiceClientID(ctx, service, clientID)
	if err != nil {
		return errors.Join(err, releaseErr)
	}
	return releaseErr
}

// UnmarshalDMSOperatingMode parses QMI DMS Get Operating Mode response TLVs.
func UnmarshalDMSOperatingMode(tlvs tlv.TLVs) (qcom.DMSOperatingMode, error) {
	value, ok := tlv.Value(tlvs, dmsTLVOperatingMode)
	if !ok {
		return 0, errors.New("parsing QMI DMS operating mode: operating mode TLV missing")
	}
	if len(value) < 1 {
		return 0, errors.New("parsing QMI DMS operating mode: operating mode TLV is truncated")
	}
	return qcom.DMSOperatingMode(value[0]), nil
}
