package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const wmsRouteMax = 10

// WMSMessageType identifies the message type matched by an incoming route.
type WMSMessageType uint8

const WMSMessageTypePointToPoint WMSMessageType = 0

// WMSMessageClass identifies an SMS message class.
type WMSMessageClass uint8

const (
	WMSMessageClass0 WMSMessageClass = iota
	WMSMessageClass1
	WMSMessageClass2
	WMSMessageClass3
	WMSMessageClassNone
	WMSMessageClassCDMA
)

// WMSReceiptAction selects how the modem handles a matching incoming SMS.
type WMSReceiptAction uint8

const (
	WMSReceiptDiscard WMSReceiptAction = iota
	WMSReceiptStoreAndNotify
	WMSReceiptTransferOnly
	WMSReceiptTransferAndAck
)

// WMSStatusReportTransfer selects where SMS status reports are delivered.
type WMSStatusReportTransfer uint8

const (
	WMSStatusReportToSIM WMSStatusReportTransfer = iota
	WMSStatusReportToClient
)

// WMSRoute describes one incoming SMS routing rule.
type WMSRoute struct {
	MessageType  WMSMessageType
	MessageClass WMSMessageClass
	Storage      WMSStorage
	Action       WMSReceiptAction
}

// MarshalBinary encodes one incoming SMS routing rule.
func (route WMSRoute) MarshalBinary() ([]byte, error) {
	return []byte{
		byte(route.MessageType),
		byte(route.MessageClass),
		byte(route.Storage),
		byte(route.Action),
	}, nil
}

// UnmarshalBinary decodes one incoming SMS routing rule.
func (route *WMSRoute) UnmarshalBinary(value []byte) error {
	if len(value) != 4 {
		return fmt.Errorf("WMS route length %d, want 4", len(value))
	}
	*route = WMSRoute{
		MessageType:  WMSMessageType(value[0]),
		MessageClass: WMSMessageClass(value[1]),
		Storage:      WMSStorage(value[2]),
		Action:       WMSReceiptAction(value[3]),
	}
	return nil
}

type wmsRouteList []WMSRoute

func (routes wmsRouteList) MarshalBinary() ([]byte, error) {
	if len(routes) > wmsRouteMax {
		return nil, fmt.Errorf("route count %d exceeds %d", len(routes), wmsRouteMax)
	}
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(routes)))
	for i, route := range routes {
		encoded, err := route.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("route %d: %w", i, err)
		}
		value = append(value, encoded...)
	}
	return value, nil
}

func (routes *wmsRouteList) UnmarshalBinary(value []byte) error {
	if len(value) < 2 {
		return errors.New("route count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > wmsRouteMax {
		return fmt.Errorf("route count %d exceeds %d", count, wmsRouteMax)
	}
	value = value[2:]
	if len(value) != count*4 {
		return fmt.Errorf("route list length %d, want %d", len(value), count*4)
	}
	decoded := make(wmsRouteList, count)
	for i := range count {
		if err := decoded[i].UnmarshalBinary(value[i*4 : (i+1)*4]); err != nil {
			return fmt.Errorf("route %d: %w", i, err)
		}
	}
	*routes = decoded
	return nil
}

// WMSRoutes contains the modem's incoming SMS routing configuration.
type WMSRoutes struct {
	Routes               []WMSRoute
	StatusReportTransfer WMSStatusReportTransfer
	StatusReportKnown    bool
}

// WMSSendFromStoreRequest selects a stored message for transmission.
type WMSSendFromStoreRequest struct {
	Reference   WMSMessageReference
	MessageMode WMSMessageMode
	SMSOnIMS    *bool
}

// WMSSetRoutes replaces the incoming SMS routing table.
func (c *Client) WMSSetRoutes(ctx context.Context, routes []WMSRoute, statusReport *WMSStatusReportTransfer) error {
	config := WMSRoutes{Routes: routes}
	if statusReport != nil {
		config.StatusReportTransfer = *statusReport
		config.StatusReportKnown = true
	}
	tlvs, err := config.MarshalTLVs()
	if err != nil {
		return err
	}
	if err := c.wmsResultRequest(ctx, MessageWMSSetRoutes, tlvs); err != nil {
		return fmt.Errorf("setting QMI WMS routes: %w", err)
	}
	return nil
}

// WMSGetRoutes reads the incoming SMS routing table.
func (c *Client) WMSGetRoutes(ctx context.Context) (WMSRoutes, error) {
	var routes WMSRoutes
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetRoutes, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return routes.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSRoutes{}, fmt.Errorf("reading QMI WMS routes: %w", err)
	}
	return routes, nil
}

// WMSSendFromStore sends an SMS already present in UIM or NV storage.
func (c *Client) WMSSendFromStore(ctx context.Context, req WMSSendFromStoreRequest) (WMSSendResult, error) {
	value, err := req.Reference.MarshalBinary()
	if err != nil {
		return WMSSendResult{}, fmt.Errorf("sending QMI WMS message from store: %w", err)
	}
	value = append(value, byte(req.MessageMode))
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if req.SMSOnIMS != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, boolByte(*req.SMSOnIMS)))
	}

	var parsed wmsSendFromStoreResponse
	err = c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSSendFromMemoryStore, tlvs)
		if err != nil {
			return err
		}
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		if !parsed.Result.MessageIDKnown {
			return errors.New("parsing QMI WMS send-from-store response: message ID TLV missing")
		}
		return nil
	})
	if err != nil {
		return parsed.Result, fmt.Errorf("sending QMI WMS message from store: %w", err)
	}
	return parsed.Result, nil
}

// MarshalTLVs encodes the WMS incoming-message routing configuration.
func (r WMSRoutes) MarshalTLVs() (tlv.TLVs, error) {
	value, err := wmsRouteList(r.Routes).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding QMI WMS routes: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if r.StatusReportKnown {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(r.StatusReportTransfer)))
	}
	return tlvs, nil
}

// UnmarshalTLVs parses the WMS incoming-message routing configuration.
func (r *WMSRoutes) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WMSRoutes{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS routes: route list TLV missing")
	}
	var routes wmsRouteList
	if err := routes.UnmarshalBinary(value); err != nil {
		return fmt.Errorf("parsing QMI WMS routes: %w", err)
	}
	r.Routes = routes
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS routes: status report TLV length %d, want 1", len(value))
		}
		r.StatusReportTransfer = WMSStatusReportTransfer(value[0])
		r.StatusReportKnown = true
	}
	return nil
}

func (r *WMSSendResult) unmarshalSendFromStoreTLVs(tlvs tlv.TLVs) error {
	var result WMSSendResult
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS send-from-store response: message ID TLV length %d, want 2", len(value))
		}
		result.MessageID = binary.LittleEndian.Uint16(value)
		result.MessageIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS send-from-store response: CDMA cause TLV length %d, want 2", len(value))
		}
		result.CauseCode = binary.LittleEndian.Uint16(value)
		result.CauseCodeKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS send-from-store response: CDMA error class TLV length %d, want 1", len(value))
		}
		result.ErrorClass = value[0]
		result.ErrorClassKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if err := result.GSMCause.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WMS send-from-store response: %w", err)
		}
		result.GSMCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS send-from-store response: delivery failure TLV length %d, want 1", len(value))
		}
		result.DeliveryFailure = WMSDeliveryFailureType(value[0])
		result.DeliveryFailureKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x15); ok {
		if err := result.RejectCause.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI WMS send-from-store response: %w", err)
		}
		result.RejectCauseKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x16); ok {
		if err := decodeWMSDeliveryCause(&result, value); err != nil {
			return fmt.Errorf("parsing QMI WMS send-from-store response: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x17); ok {
		if err := decodeWMSCallControl(&result, value); err != nil {
			return fmt.Errorf("parsing QMI WMS send-from-store response: %w", err)
		}
	}
	if value, ok := tlv.Value(tlvs, 0x18); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI WMS send-from-store response: IMS reject cause TLV length %d, want 2", len(value))
		}
		result.IMSRejectCause = binary.LittleEndian.Uint16(value)
		result.IMSRejectCauseKnown = true
	}
	*r = result
	return nil
}

type wmsSendFromStoreResponse struct {
	Result WMSSendResult
}

func (r *wmsSendFromStoreResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var result WMSSendResult
	if err := result.unmarshalSendFromStoreTLVs(tlvs); err != nil {
		return err
	}
	*r = wmsSendFromStoreResponse{Result: result}
	return nil
}
