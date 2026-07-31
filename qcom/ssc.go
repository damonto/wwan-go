package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// SSCReportType selects the SSC indication size class.
type SSCReportType uint8

const (
	SSCReportSmall SSCReportType = iota
	SSCReportLarge
)

// SSCControlRequest sends an opaque request to Snapdragon Sensor Core.
type SSCControlRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Data          []byte
	ReportType    *SSCReportType
}

// Request validates and converts the control data into a QMI request.
func (r SSCControlRequest) Request() (Request, error) {
	const maximumDataLength = MaxQRTRServiceTLVLength - 9
	if len(r.Data) > maximumDataLength {
		return Request{}, fmt.Errorf("SSC control data length %d exceeds maximum %d", len(r.Data), maximumDataLength)
	}
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(r.Data)))
	value = append(value, r.Data...)
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if r.ReportType != nil {
		if *r.ReportType > SSCReportLarge {
			return Request{}, fmt.Errorf("SSC report type %d is out of range", *r.ReportType)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*r.ReportType)))
	}
	return Request{
		Service:       ServiceSSC,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageSSCControl,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// SSCControlResponse contains identifiers returned for an SSC request.
type SSCControlResponse struct {
	ClientID      uint64
	ClientIDKnown bool
	Response      uint32
	ResponseKnown bool
}

// UnmarshalTLVs parses SSC Control response TLVs.
func (r *SSCControlResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = SSCControlResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 8 {
			return fmt.Errorf("parsing QMI SSC control response: client ID TLV length %d, want 8", len(value))
		}
		r.ClientID = binary.LittleEndian.Uint64(value)
		r.ClientIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI SSC control response: response TLV length %d, want 4", len(value))
		}
		r.Response = binary.LittleEndian.Uint32(value)
		r.ResponseKnown = true
	}
	return nil
}

// SSCReport is one opaque sensor-core report. Err reports malformed wire data.
type SSCReport struct {
	ClientID uint64
	Data     []byte
	Err      error
}

// UnmarshalTLVs parses an SSC report indication.
func (r *SSCReport) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = SSCReport{}
	clientID, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI SSC report: client ID TLV missing")
	}
	if len(clientID) != 8 {
		return fmt.Errorf("parsing QMI SSC report: client ID TLV length %d, want 8", len(clientID))
	}
	data, ok := tlv.Value(tlvs, 0x02)
	if !ok {
		return errors.New("parsing QMI SSC report: data TLV missing")
	}
	decoded, err := decodeLengthPrefixedBytes(data)
	if err != nil {
		return fmt.Errorf("parsing QMI SSC report data: %w", err)
	}
	r.ClientID = binary.LittleEndian.Uint64(clientID)
	r.Data = slices.Clone(decoded)
	return nil
}

// SSCControl sends opaque control data to Snapdragon Sensor Core.
func (c *Client) SSCControl(ctx context.Context, data []byte, reportType *SSCReportType) (SSCControlResponse, error) {
	req, err := (SSCControlRequest{Timeout: DefaultRequestTimeout, Data: data, ReportType: reportType}).Request()
	if err != nil {
		return SSCControlResponse{}, fmt.Errorf("sending QMI SSC control request: %w", err)
	}
	var result SSCControlResponse
	err = c.withServiceClient(ctx, ServiceSSC, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceSSC, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return SSCControlResponse{}, fmt.Errorf("sending QMI SSC control request: %w", err)
	}
	return result, nil
}

// WatchSSCReports subscribes to one SSC report size class.
func (c *Client) WatchSSCReports(ctx context.Context, reportType SSCReportType) (<-chan SSCReport, error) {
	if reportType > SSCReportLarge {
		return nil, fmt.Errorf("watching QMI SSC reports: report type %d is out of range", reportType)
	}
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI SSC reports: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServiceSSC)
	if err != nil {
		return nil, fmt.Errorf("watching QMI SSC reports: %w", err)
	}
	messageID := MessageSSCReportSmall
	if reportType == SSCReportLarge {
		messageID = MessageSSCReportLarge
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceSSC, clientID, messageID)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI SSC reports: subscribe: %w", err)
	}

	out := make(chan SSCReport)
	go func() {
		defer close(out)
		defer cancel()
		for {
			select {
			case indication, ok := <-indications:
				if !ok {
					return
				}
				var report SSCReport
				report.Err = report.UnmarshalTLVs(indication.TLVs)
				select {
				case out <- report:
				case <-watchCtx.Done():
					return
				}
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}
