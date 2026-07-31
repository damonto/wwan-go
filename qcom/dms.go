package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVOperatingMode       = 0x01
	dmsTLVOfflineReason       = 0x10
	dmsTLVHardwareRestricted  = 0x11
	dmsTLVReportOperatingMode = 0x14
	dmsTLVVoiceNumber         = 0x01
	dmsTLVMobileIDNumber      = 0x10
	dmsTLVIMSI                = 0x11
)

// DMSOfflineReason is a mask describing why the modem entered offline mode.
type DMSOfflineReason uint16

const (
	DMSOfflineHostImageMisconfiguration DMSOfflineReason = 1 << iota
	DMSOfflinePRIImageMisconfiguration
	DMSOfflinePRIVersionIncompatible
	DMSOfflineDeviceMemoryFull
)

// DMSGetMSISDNRequest encodes QMI DMS Get MSISDN.
type DMSGetMSISDNRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetMSISDNRequest) Request() Request {
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSGetMSISDN,
		Timeout:       r.Timeout,
	}
}

// DMSGetOperatingModeRequest encodes QMI DMS Get Operating Mode.
type DMSGetOperatingModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetOperatingModeRequest) Request() Request {
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSGetOperatingMode,
		Timeout:       r.Timeout,
	}
}

// DMSSetOperatingModeRequest encodes QMI DMS Set Operating Mode.
type DMSSetOperatingModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mode          DMSOperatingMode
}

// Request converts the request into a QMI DMS request.
func (r DMSSetOperatingModeRequest) Request() Request {
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetOperatingMode,
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
	Config              *DMSEventReportConfig
}

// Request converts the request into a QMI DMS request.
func (r DMSSetEventReportRequest) Request() Request {
	var tlvs tlv.TLVs
	if r.Config != nil {
		tlvs = dmsEventReportTLVs(*r.Config)
	} else {
		tlvs = tlv.TLVs{
			tlv.Uint(dmsTLVReportOperatingMode, boolByte(r.ReportOperatingMode)),
		}
	}

	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetEventReport,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}
}

// OperatingMode reads the current QMI DMS modem operating mode.
func (c *Client) OperatingMode(ctx context.Context) (DMSOperatingMode, error) {
	info, err := c.OperatingModeInfo(ctx)
	if err != nil {
		return 0, err
	}
	return info.Mode, nil
}

// OperatingModeInfo reads the modem operating mode and its optional offline
// and hardware-restriction details.
func (c *Client) OperatingModeInfo(ctx context.Context) (DMSGetOperatingModeResponse, error) {
	var result DMSGetOperatingModeResponse
	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		req := DMSGetOperatingModeRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}

		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return DMSGetOperatingModeResponse{}, fmt.Errorf("querying QMI DMS operating mode: %w", err)
	}
	return result, nil
}

// SetOperatingMode sets the QMI DMS modem operating mode.
func (c *Client) SetOperatingMode(ctx context.Context, mode DMSOperatingMode) error {
	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		req := DMSSetOperatingModeRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Mode:     mode,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
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

// MSISDN returns the voice number and related subscriber identifiers reported by QMI DMS.
func (c *Client) MSISDN(ctx context.Context) (DMSGetMSISDNResponse, error) {
	var result DMSGetMSISDNResponse
	err := c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		req := DMSGetMSISDNRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
		}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return DMSGetMSISDNResponse{}, fmt.Errorf("querying QMI DMS MSISDN: %w", err)
	}
	return result, nil
}

// DMSGetMSISDNResponse is the parsed QMI DMS Get MSISDN response.
type DMSGetMSISDNResponse struct {
	VoiceNumber    string
	MobileIDNumber string
	IMSI           string
}

// UnmarshalTLVs parses QMI DMS Get MSISDN response TLVs.
func (r *DMSGetMSISDNResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetMSISDNResponse{}
	voiceNumber, ok := tlv.Value(tlvs, dmsTLVVoiceNumber)
	if !ok {
		return errors.New("parsing QMI DMS MSISDN: voice number TLV missing")
	}
	r.VoiceNumber = string(voiceNumber)
	if mobileIDNumber, ok := tlv.Value(tlvs, dmsTLVMobileIDNumber); ok {
		r.MobileIDNumber = string(mobileIDNumber)
	}
	if imsi, ok := tlv.Value(tlvs, dmsTLVIMSI); ok {
		r.IMSI = string(imsi)
	}
	return nil
}

// DMSGetOperatingModeResponse is the parsed QMI DMS Get Operating Mode response.
type DMSGetOperatingModeResponse struct {
	Mode                    DMSOperatingMode
	OfflineReason           DMSOfflineReason
	OfflineReasonKnown      bool
	HardwareRestricted      bool
	HardwareRestrictedKnown bool
}

// UnmarshalTLVs parses QMI DMS Get Operating Mode response TLVs.
func (r *DMSGetOperatingModeResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetOperatingModeResponse{}
	value, ok := tlv.Value(tlvs, dmsTLVOperatingMode)
	if !ok {
		return errors.New("parsing QMI DMS operating mode: operating mode TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS operating mode: operating mode TLV length %d, want 1", len(value))
	}
	r.Mode = DMSOperatingMode(value[0])
	if value, ok := tlv.Value(tlvs, dmsTLVOfflineReason); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI DMS operating mode: offline reason TLV length %d, want 2", len(value))
		}
		r.OfflineReason = DMSOfflineReason(binary.LittleEndian.Uint16(value))
		r.OfflineReasonKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVHardwareRestricted); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DMS operating mode: hardware restriction TLV length %d, want 1", len(value))
		}
		r.HardwareRestricted = value[0] != 0
		r.HardwareRestrictedKnown = true
	}
	return nil
}
