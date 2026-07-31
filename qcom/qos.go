package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// QOSSubscription identifies a modem subscription for the QoS service.
type QOSSubscription uint32

const (
	QOSSubscriptionPrimary QOSSubscription = 1 + iota
	QOSSubscriptionSecondary
	QOSSubscriptionTertiary
)

// QOSFlowStatus is the current state of a negotiated QoS flow.
type QOSFlowStatus uint8

const (
	QOSFlowStatusDefault QOSFlowStatus = iota
	QOSFlowStatusActivated
	QOSFlowStatusSuspended
	QOSFlowStatusGone
)

// QOSFlowEvent describes why a flow-status indication was emitted.
type QOSFlowEvent uint8

const (
	QOSFlowEventActivated QOSFlowEvent = 1 + iota
	QOSFlowEventSuspended
	QOSFlowEventGone
	QOSFlowEventModifyAccepted
	QOSFlowEventModifyRejected
	QOSFlowEventInfoCodeUpdated
)

// QOSFlowStatusUpdate is emitted for a flow owned by this QoS control point.
type QOSFlowStatusUpdate struct {
	ID          uint32
	Status      QOSFlowStatus
	Event       QOSFlowEvent
	Reason      uint8
	ReasonKnown bool
}

// QOSDataPortConfig selects a modern endpoint/mux or one legacy SIO port.
type QOSDataPortConfig struct {
	Endpoint   *DataEndpoint
	MuxID      *uint8
	LegacyPort *WDSSIOPort
}

// QOSResetRequest encodes QMI QOS Reset.
type QOSResetRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the reset into a QMI request.
func (r QOSResetRequest) Request() Request {
	return qosEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageQOSReset)
}

// QOSGetFlowStatusRequest encodes Get QoS Status.
type QOSGetFlowStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	FlowID        uint32
}

// Request converts the flow query into a QMI request.
func (r QOSGetFlowStatusRequest) Request() Request {
	return Request{
		Service:       ServiceQOS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageQOSGetStatus,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, r.FlowID)},
	}
}

// QOSGetFlowStatusResponse is the parsed flow status response.
type QOSGetFlowStatusResponse struct {
	Status QOSFlowStatus
}

// UnmarshalTLVs parses the mandatory flow status.
func (r *QOSGetFlowStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = QOSGetFlowStatusResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI QOS flow status: status TLV is missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI QOS flow status: status TLV length %d, want 1", len(value))
	}
	r.Status = QOSFlowStatus(value[0])
	return nil
}

// QOSFlowStatusIndication is the parsed per-flow status indication.
type QOSFlowStatusIndication struct {
	Update QOSFlowStatusUpdate
}

// UnmarshalTLVs parses the mandatory status aggregate and optional reason.
func (r *QOSFlowStatusIndication) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = QOSFlowStatusIndication{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI QOS flow status indication: status TLV is missing")
	}
	if len(value) != 6 {
		return fmt.Errorf("parsing QMI QOS flow status indication: status TLV length %d, want 6", len(value))
	}
	r.Update.ID = binary.LittleEndian.Uint32(value[:4])
	r.Update.Status = QOSFlowStatus(value[4])
	r.Update.Event = QOSFlowEvent(value[5])
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI QOS flow status indication: reason TLV length %d, want 1", len(value))
		}
		r.Update.Reason = value[0]
		r.Update.ReasonKnown = true
	}
	return nil
}

// QOSGetNetworkStatusRequest encodes Get QoS Network Status.
type QOSGetNetworkStatusRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r QOSGetNetworkStatusRequest) Request() Request {
	return qosEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageQOSGetNetworkStatus)
}

// QOSGetNetworkStatusResponse is the parsed network QoS support status.
type QOSGetNetworkStatusResponse struct {
	Supported bool
}

// UnmarshalTLVs parses the mandatory support flag.
func (r *QOSGetNetworkStatusResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = QOSGetNetworkStatusResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI QOS network status: support TLV is missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI QOS network status: support TLV length %d, want 1", len(value))
	}
	r.Supported = value[0] != 0
	return nil
}

// QOSBindDataPortRequest encodes Bind Data Port.
type QOSBindDataPortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        QOSDataPortConfig
}

// Request validates and converts a modern or legacy data-port binding.
func (r QOSBindDataPortRequest) Request() (Request, error) {
	modern := r.Config.Endpoint != nil || r.Config.MuxID != nil
	if !modern && r.Config.LegacyPort == nil {
		return Request{}, errors.New("encoding QMI QOS data port: no port selected")
	}
	if modern && r.Config.LegacyPort != nil {
		return Request{}, errors.New("encoding QMI QOS data port: modern and legacy ports are mutually exclusive")
	}
	var tlvs tlv.TLVs
	if r.Config.Endpoint != nil {
		value, _ := r.Config.Endpoint.MarshalBinary() // Fixed-width endpoint encoding cannot fail.
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if r.Config.MuxID != nil {
		tlvs = append(tlvs, tlv.Uint(0x11, *r.Config.MuxID))
	}
	if r.Config.LegacyPort != nil {
		tlvs = append(tlvs, tlv.Uint(0x12, uint16(*r.Config.LegacyPort)))
	}
	return Request{
		Service:       ServiceQOS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageQOSBindDataPort,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// QOSBindSubscriptionRequest encodes Bind Subscription.
type QOSBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Subscription  QOSSubscription
}

// Request validates and converts the subscription binding.
func (r QOSBindSubscriptionRequest) Request() (Request, error) {
	if err := validateQOSSubscription(r.Subscription); err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceQOS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageQOSBindSubscription,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Subscription))},
	}, nil
}

// QOSGetBindSubscriptionRequest encodes Get Bind Subscription.
type QOSGetBindSubscriptionRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r QOSGetBindSubscriptionRequest) Request() Request {
	return qosEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageQOSGetBindSubscription)
}

// QOSGetBindSubscriptionResponse is the parsed bound subscription.
type QOSGetBindSubscriptionResponse struct {
	Subscription      QOSSubscription
	SubscriptionKnown bool
}

// UnmarshalTLVs parses the optional bound subscription.
func (r *QOSGetBindSubscriptionResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = QOSGetBindSubscriptionResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI QOS bound subscription: subscription TLV length %d, want 4", len(value))
	}
	r.Subscription = QOSSubscription(binary.LittleEndian.Uint32(value))
	r.SubscriptionKnown = true
	return nil
}

// QOSReset resets this control point's QOS service state.
func (c *Client) QOSReset(ctx context.Context) error {
	req := QOSResetRequest{Timeout: DefaultRequestTimeout}.Request()
	if err := c.qosResultRequest(ctx, req); err != nil {
		return fmt.Errorf("resetting QMI QOS control point: %w", err)
	}
	return nil
}

// QOSFlowStatus reads the current status of one negotiated flow.
func (c *Client) QOSFlowStatus(ctx context.Context, flowID uint32) (QOSFlowStatus, error) {
	var status QOSFlowStatus
	err := c.withServiceClient(ctx, ServiceQOS, func(clientID uint8) error {
		req := QOSGetFlowStatusRequest{ClientID: clientID, Timeout: DefaultRequestTimeout, FlowID: flowID}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed QOSGetFlowStatusResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		status = parsed.Status
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading QMI QOS flow %d status: %w", flowID, err)
	}
	return status, nil
}

// QOSNetworkSupported reports whether the active network supports QoS.
func (c *Client) QOSNetworkSupported(ctx context.Context) (bool, error) {
	var supported bool
	err := c.withServiceClient(ctx, ServiceQOS, func(clientID uint8) error {
		req := QOSGetNetworkStatusRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed QOSGetNetworkStatusResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		supported = parsed.Supported
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("reading QMI QOS network status: %w", err)
	}
	return supported, nil
}

// QOSBindDataPort binds this QoS control point to a data channel.
func (c *Client) QOSBindDataPort(ctx context.Context, config QOSDataPortConfig) error {
	req, err := (QOSBindDataPortRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return err
	}
	if err := c.qosResultRequest(ctx, req); err != nil {
		return fmt.Errorf("binding QMI QOS data port: %w", err)
	}
	return nil
}

// QOSBindSubscription binds this QoS control point to a subscription.
func (c *Client) QOSBindSubscription(ctx context.Context, subscription QOSSubscription) error {
	req, err := (QOSBindSubscriptionRequest{Timeout: DefaultRequestTimeout, Subscription: subscription}).Request()
	if err != nil {
		return err
	}
	if err := c.qosResultRequest(ctx, req); err != nil {
		return fmt.Errorf("binding QMI QOS subscription: %w", err)
	}
	return nil
}

// QOSBoundSubscription reads the subscription bound to this QoS control point.
func (c *Client) QOSBoundSubscription(ctx context.Context) (QOSSubscription, error) {
	var subscription QOSSubscription
	err := c.withServiceClient(ctx, ServiceQOS, func(clientID uint8) error {
		req := QOSGetBindSubscriptionRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed QOSGetBindSubscriptionResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		if !parsed.SubscriptionKnown {
			return errors.New("parsing QMI QOS bound subscription: subscription TLV is missing")
		}
		subscription = parsed.Subscription
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading QMI QOS bound subscription: %w", err)
	}
	return subscription, nil
}

// QOSWatchFlowStatus subscribes to status changes for flows owned by this control point.
func (c *Client) QOSWatchFlowStatus(ctx context.Context) (<-chan QOSFlowStatusUpdate, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceQOS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI QOS flow status: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceQOS, clientID, MessageQOSStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI QOS flow status: %w", err)
	}
	out := make(chan QOSFlowStatusUpdate, 8)
	go func() {
		defer close(out)
		defer cancel()
		for indication := range indications {
			var parsed QOSFlowStatusIndication
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Update:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

// QOSWatchNetworkStatus subscribes to changes in network QoS support.
func (c *Client) QOSWatchNetworkStatus(ctx context.Context) (<-chan bool, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, err
	}
	clientID, err := c.serviceClientID(ctx, ServiceQOS)
	if err != nil {
		return nil, fmt.Errorf("watching QMI QOS network status: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceQOS, clientID, MessageQOSNetworkStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI QOS network status: %w", err)
	}
	out := make(chan bool, 8)
	go func() {
		defer close(out)
		defer cancel()
		for indication := range indications {
			var parsed QOSGetNetworkStatusResponse
			if err := parsed.UnmarshalTLVs(indication.TLVs); err != nil {
				return
			}
			select {
			case out <- parsed.Supported:
			case <-watchCtx.Done():
				return
			}
		}
	}()
	return out, nil
}

func qosEmptyRequest(clientID uint8, transactionID uint16, timeout time.Duration, message MessageID) Request {
	return Request{
		Service:       ServiceQOS,
		ClientID:      clientID,
		TransactionID: transactionID,
		MessageID:     message,
		Timeout:       timeout,
	}
}

func (c *Client) qosResultRequest(ctx context.Context, req Request) error {
	return c.withServiceClient(ctx, ServiceQOS, func(clientID uint8) error {
		req.ClientID = clientID
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

func validateQOSSubscription(subscription QOSSubscription) error {
	if subscription < QOSSubscriptionPrimary || subscription > QOSSubscriptionTertiary {
		return fmt.Errorf("QMI QOS subscription %d is out of range", subscription)
	}
	return nil
}
