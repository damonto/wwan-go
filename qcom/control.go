package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// CTLQoSHeader controls whether data packets include a QMI QoS flow header.
type CTLQoSHeader uint8

const (
	CTLQoSHeaderAbsent CTLQoSHeader = iota
	CTLQoSHeaderPresent
)

// CTLLinkProtocol identifies a link-layer format supported by the host.
type CTLLinkProtocol uint16

const (
	CTLLinkProtocolEthernet CTLLinkProtocol = 1 << iota
	CTLLinkProtocolRawIP
)

// CTLDataFormatConfig describes the legacy CTL data format negotiation.
// A zero LinkProtocols value omits the optional protocol mask.
type CTLDataFormatConfig struct {
	QoSHeader     CTLQoSHeader
	LinkProtocols CTLLinkProtocol
}

// CTLSetInstanceID assigns the host instance and returns its QMI link ID.
func (c *Client) CTLSetInstanceID(ctx context.Context, instanceID uint8) (uint16, error) {
	resp, err := c.ctlRequest(ctx, MessageCTLSetInstanceID, tlv.TLVs{
		tlv.Uint(0x01, instanceID),
	})
	if err != nil {
		return 0, fmt.Errorf("setting QMI CTL instance ID: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x01)
	if !ok {
		return 0, errors.New("setting QMI CTL instance ID: link ID TLV missing")
	}
	if len(value) != 2 {
		return 0, fmt.Errorf("setting QMI CTL instance ID: link ID TLV length %d, want 2", len(value))
	}
	return binary.LittleEndian.Uint16(value), nil
}

// CTLSetDataFormat negotiates the legacy QMUX link protocol.
func (c *Client) CTLSetDataFormat(ctx context.Context, config CTLDataFormatConfig) (CTLLinkProtocol, error) {
	if config.QoSHeader > CTLQoSHeaderPresent {
		return 0, fmt.Errorf("setting QMI CTL data format: QoS header value %d is out of range", config.QoSHeader)
	}
	const supportedProtocols = CTLLinkProtocolEthernet | CTLLinkProtocolRawIP
	if unsupported := config.LinkProtocols &^ supportedProtocols; unsupported != 0 {
		return 0, fmt.Errorf("setting QMI CTL data format: link protocol mask 0x%X contains unsupported bits", config.LinkProtocols)
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(config.QoSHeader))}
	if config.LinkProtocols != 0 {
		tlvs = append(tlvs, tlv.Uint(0x10, uint16(config.LinkProtocols)))
	}
	resp, err := c.ctlRequest(ctx, MessageCTLSetDataFormat, tlvs)
	if err != nil {
		return 0, fmt.Errorf("setting QMI CTL data format: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		return 0, errors.New("setting QMI CTL data format: selected protocol TLV missing")
	}
	if len(value) != 2 {
		return 0, fmt.Errorf("setting QMI CTL data format: selected protocol TLV length %d, want 2", len(value))
	}
	return CTLLinkProtocol(binary.LittleEndian.Uint16(value)), nil
}

// CTLSync synchronizes the QMI control point with the modem.
func (c *Client) CTLSync(ctx context.Context) error {
	if _, err := c.ctlRequest(ctx, MessageCTLSync, nil); err != nil {
		return fmt.Errorf("synchronizing QMI CTL service: %w", err)
	}
	return nil
}

// CTLWatchSync subscribes to modem-initiated CTL synchronization events.
func (c *Client) CTLWatchSync(ctx context.Context) (<-chan struct{}, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI CTL synchronization: %w", err)
	}
	indications, err := transport.Indications(ctx, ServiceControl, 0, MessageCTLSync)
	if err != nil {
		return nil, fmt.Errorf("watching QMI CTL synchronization: %w", err)
	}

	out := make(chan struct{}, 8)
	go func() {
		defer close(out)
		for range indications {
			select {
			case out <- struct{}{}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) ctlRequest(ctx context.Context, id MessageID, tlvs tlv.TLVs) (Response, error) {
	resp, err := c.requestService(ctx, ServiceControl, 0, id, tlvs)
	if err != nil {
		return Response{}, err
	}
	if err := resultOK(resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

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
			tlv.Uint(0x01, uint8(r.ServiceType)),
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
