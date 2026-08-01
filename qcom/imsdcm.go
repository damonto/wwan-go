package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const imsDCMMaxAPNLength = 100

// IMSDCMAPNType identifies the purpose of an IMS DCM packet-data connection.
type IMSDCMAPNType uint32

const (
	IMSDCMAPNIMS IMSDCMAPNType = iota
	IMSDCMAPNInternet
	IMSDCMAPNEmergency
	IMSDCMAPNRCS
	IMSDCMAPNUT
	IMSDCMAPNWLAN
)

// IMSDCMRAT identifies the access technology for an IMS DCM connection.
type IMSDCMRAT uint32

const (
	IMSDCMRATEHRPD IMSDCMRAT = iota
	IMSDCMRATLTE
	IMSDCMRATEPC
	IMSDCMRATWLAN
)

// IMSDCMIPFamily identifies the requested packet-data address family.
type IMSDCMIPFamily uint32

const (
	IMSDCMIPv4 IMSDCMIPFamily = iota
	IMSDCMIPv6
)

// IMSDCMInstance identifies an IMS stack instance. Qualcomm's IDL defines
// -1, 0, 1, and 2; this intentionally differs from libqmi's uncertain enum.
type IMSDCMInstance int32

const (
	IMSDCMInstanceNone   IMSDCMInstance = -1
	IMSDCMInstanceGlobal IMSDCMInstance = iota - 1
	IMSDCMInstance1
	IMSDCMInstance2
)

// IMSDCMConnection contains the mandatory PDP activation parameters.
type IMSDCMConnection struct {
	APN           string
	APNType       IMSDCMAPNType
	RAT           IMSDCMRAT
	IPFamily      IMSDCMIPFamily
	WDSProfileNum uint32
}

// IMSDCMPDPActivateRequest encodes IMS DCM PDP Activate Request.
type IMSDCMPDPActivateRequest struct {
	ClientID       uint8
	TransactionID  uint16
	Timeout        time.Duration
	Connection     IMSDCMConnection
	SequenceNumber *uint32
	SubscriptionID *uint32
	SlotID         *uint32
	Instance       *IMSDCMInstance
}

// Request validates and converts PDP activation parameters into a QMI request.
func (r IMSDCMPDPActivateRequest) Request() (Request, error) {
	connection, err := r.Connection.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, connection)}
	for _, field := range []struct {
		typ   uint8
		value *uint32
	}{
		{0x10, r.SequenceNumber},
		{0x11, r.SubscriptionID},
		{0x12, r.SlotID},
	} {
		if field.value != nil {
			tlvs = append(tlvs, tlv.Uint(field.typ, *field.value))
		}
	}
	if r.Instance != nil {
		value, err := r.Instance.MarshalBinary()
		if err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(0x13, value))
	}
	return Request{
		Service:       ServiceIMSDCM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSDCMPDPActivate,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// MarshalBinary encodes the mandatory IMS DCM connection aggregate.
func (c IMSDCMConnection) MarshalBinary() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	value := make([]byte, 0, 1+len(c.APN)+16)
	value = append(value, byte(len(c.APN)))
	value = append(value, c.APN...)
	value = binary.LittleEndian.AppendUint32(value, uint32(c.APNType))
	value = binary.LittleEndian.AppendUint32(value, uint32(c.RAT))
	value = binary.LittleEndian.AppendUint32(value, uint32(c.IPFamily))
	value = binary.LittleEndian.AppendUint32(value, c.WDSProfileNum)
	return value, nil
}

// UnmarshalBinary decodes the mandatory IMS DCM connection aggregate.
func (c *IMSDCMConnection) UnmarshalBinary(value []byte) error {
	if len(value) < 17 {
		return fmt.Errorf("IMS DCM connection length %d is shorter than 17", len(value))
	}
	apnLength := int(value[0])
	if len(value) != 1+apnLength+16 {
		return fmt.Errorf("IMS DCM connection length %d, want %d", len(value), 1+apnLength+16)
	}
	parsed := IMSDCMConnection{
		APN:           string(value[1 : 1+apnLength]),
		APNType:       IMSDCMAPNType(binary.LittleEndian.Uint32(value[1+apnLength:])),
		RAT:           IMSDCMRAT(binary.LittleEndian.Uint32(value[5+apnLength:])),
		IPFamily:      IMSDCMIPFamily(binary.LittleEndian.Uint32(value[9+apnLength:])),
		WDSProfileNum: binary.LittleEndian.Uint32(value[13+apnLength:]),
	}
	if err := parsed.validate(); err != nil {
		return err
	}
	*c = parsed
	return nil
}

func (c IMSDCMConnection) validate() error {
	if c.APN == "" {
		return errors.New("IMS DCM APN is required")
	}
	if strings.IndexByte(c.APN, 0) >= 0 {
		return errors.New("IMS DCM APN contains a NUL byte")
	}
	if len(c.APN) > imsDCMMaxAPNLength {
		return fmt.Errorf("IMS DCM APN length %d exceeds maximum %d", len(c.APN), imsDCMMaxAPNLength)
	}
	if c.APNType > IMSDCMAPNWLAN {
		return fmt.Errorf("IMS DCM APN type %d is out of range", c.APNType)
	}
	if c.RAT > IMSDCMRATWLAN {
		return fmt.Errorf("IMS DCM RAT %d is out of range", c.RAT)
	}
	if c.IPFamily > IMSDCMIPv6 {
		return fmt.Errorf("IMS DCM IP family %d is out of range", c.IPFamily)
	}
	return nil
}

// IMSDCMPDPActivateResponse contains fields from the immediate activation response.
type IMSDCMPDPActivateResponse struct {
	PDPID               uint8
	PDPIDKnown          bool
	SequenceNumber      uint32
	SequenceNumberKnown bool
	Instance            IMSDCMInstance
	InstanceKnown       bool
}

// UnmarshalTLVs parses IMS DCM PDP Activate response TLVs.
func (r *IMSDCMPDPActivateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSDCMPDPActivateResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI IMS DCM activation response: PDP ID TLV length %d, want 1", len(value))
		}
		r.PDPID = value[0]
		r.PDPIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI IMS DCM activation response: sequence TLV length %d, want 4", len(value))
		}
		r.SequenceNumber = binary.LittleEndian.Uint32(value)
		r.SequenceNumberKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if err := r.Instance.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI IMS DCM activation response instance: %w", err)
		}
		r.InstanceKnown = true
	}
	return nil
}

// IMSDCMPDPActivation is the asynchronous result of a PDP activation request.
type IMSDCMPDPActivation struct {
	PDPID               uint8
	SequenceNumber      uint32
	SequenceNumberKnown bool
	Address             netip.Addr
	AddressKnown        bool
	Instance            IMSDCMInstance
	InstanceKnown       bool
	ProtocolError       QMIError
	ProtocolErrorKnown  bool
	Err                 error
}

// UnmarshalTLVs parses an IMS DCM PDP Activate indication.
func (r *IMSDCMPDPActivation) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSDCMPDPActivation{}
	result, ok := tlv.Value(tlvs, qmiTLVResult)
	if !ok {
		return errors.New("parsing QMI IMS DCM activation: result TLV missing")
	}
	if len(result) != 4 {
		return fmt.Errorf("parsing QMI IMS DCM activation: result TLV length %d, want 4", len(result))
	}
	if QMIResult(binary.LittleEndian.Uint16(result[:2])) != QMIResultSuccess {
		r.ProtocolError = QMIError(binary.LittleEndian.Uint16(result[2:]))
		r.ProtocolErrorKnown = true
	}
	pdpID, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI IMS DCM activation: PDP ID TLV missing")
	}
	if len(pdpID) != 1 {
		return fmt.Errorf("parsing QMI IMS DCM activation: PDP ID TLV length %d, want 1", len(pdpID))
	}
	r.PDPID = pdpID[0]
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI IMS DCM activation: sequence TLV length %d, want 4", len(value))
		}
		r.SequenceNumber = binary.LittleEndian.Uint32(value)
		r.SequenceNumberKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		address, err := decodeIMSDCMAddress(value)
		if err != nil {
			return err
		}
		r.Address = address
		r.AddressKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		if err := r.Instance.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI IMS DCM activation instance: %w", err)
		}
		r.InstanceKnown = true
	}
	return nil
}

// IMSDCMPDPDeactivateRequest encodes IMS DCM PDP Deactivate Request.
type IMSDCMPDPDeactivateRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	PDPID         uint8
	Instance      *IMSDCMInstance
}

// Request validates and converts the deactivation into a QMI request.
func (r IMSDCMPDPDeactivateRequest) Request() (Request, error) {
	tlvs := tlv.TLVs{tlv.Uint(0x01, r.PDPID)}
	if r.Instance != nil {
		value, err := r.Instance.MarshalBinary()
		if err != nil {
			return Request{}, err
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	return Request{
		Service:       ServiceIMSDCM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageIMSDCMPDPDeactivate,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// IMSDCMPDPDeactivateResponse contains the deactivated PDP and IMS instance.
type IMSDCMPDPDeactivateResponse struct {
	PDPID         uint8
	PDPIDKnown    bool
	Instance      IMSDCMInstance
	InstanceKnown bool
}

// UnmarshalTLVs parses IMS DCM PDP Deactivate response TLVs.
func (r *IMSDCMPDPDeactivateResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = IMSDCMPDPDeactivateResponse{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI IMS DCM deactivation response: PDP ID TLV length %d, want 1", len(value))
		}
		r.PDPID = value[0]
		r.PDPIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		if err := r.Instance.UnmarshalBinary(value); err != nil {
			return fmt.Errorf("parsing QMI IMS DCM deactivation response instance: %w", err)
		}
		r.InstanceKnown = true
	}
	return nil
}

// IMSDCMPDPActivate requests an IMS packet-data connection.
func (c *Client) IMSDCMPDPActivate(ctx context.Context, request IMSDCMPDPActivateRequest) (IMSDCMPDPActivateResponse, error) {
	if request.Timeout <= 0 {
		request.Timeout = DefaultRequestTimeout
	}
	req, err := request.Request()
	if err != nil {
		return IMSDCMPDPActivateResponse{}, fmt.Errorf("activating QMI IMS DCM PDP: %w", err)
	}
	var result IMSDCMPDPActivateResponse
	if err := c.imsDCMRequest(ctx, req, &result); err != nil {
		return IMSDCMPDPActivateResponse{}, fmt.Errorf("activating QMI IMS DCM PDP: %w", err)
	}
	return result, nil
}

// IMSDCMPDPDeactivate requests teardown of an IMS packet-data connection.
func (c *Client) IMSDCMPDPDeactivate(ctx context.Context, pdpID uint8, instance *IMSDCMInstance) (IMSDCMPDPDeactivateResponse, error) {
	req, err := (IMSDCMPDPDeactivateRequest{
		Timeout:  DefaultRequestTimeout,
		PDPID:    pdpID,
		Instance: instance,
	}).Request()
	if err != nil {
		return IMSDCMPDPDeactivateResponse{}, fmt.Errorf("deactivating QMI IMS DCM PDP: %w", err)
	}
	var result IMSDCMPDPDeactivateResponse
	if err := c.imsDCMRequest(ctx, req, &result); err != nil {
		return IMSDCMPDPDeactivateResponse{}, fmt.Errorf("deactivating QMI IMS DCM PDP: %w", err)
	}
	return result, nil
}

// WatchIMSDCMPDPActivations subscribes to asynchronous PDP activation results.
func (c *Client) WatchIMSDCMPDPActivations(ctx context.Context) (<-chan IMSDCMPDPActivation, error) {
	transport, err := c.indicationTransport()
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMS DCM PDP activations: %w", err)
	}
	clientID, err := c.serviceClientID(ctx, ServiceIMSDCM)
	if err != nil {
		return nil, fmt.Errorf("watching QMI IMS DCM PDP activations: %w", err)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	indications, err := transport.Indications(watchCtx, ServiceIMSDCM, clientID, MessageIMSDCMPDPActivate)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching QMI IMS DCM PDP activations: subscribe: %w", err)
	}
	out := make(chan IMSDCMPDPActivation)
	go func() {
		defer close(out)
		defer cancel()
		for {
			select {
			case indication, ok := <-indications:
				if !ok {
					return
				}
				var activation IMSDCMPDPActivation
				activation.Err = activation.UnmarshalTLVs(indication.TLVs)
				select {
				case out <- activation:
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

func (c *Client) imsDCMRequest(ctx context.Context, req Request, dst tlvUnmarshaler) error {
	return c.withServiceClient(ctx, ServiceIMSDCM, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceIMSDCM, clientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return dst.UnmarshalTLVs(resp.TLVs)
	})
}

func (instance IMSDCMInstance) validate() error {
	if instance < IMSDCMInstanceNone || instance > IMSDCMInstance2 {
		return fmt.Errorf("IMS DCM instance %d is out of range", instance)
	}
	return nil
}

func (instance IMSDCMInstance) MarshalBinary() ([]byte, error) {
	if err := instance.validate(); err != nil {
		return nil, err
	}
	return binary.LittleEndian.AppendUint32(nil, uint32(instance)), nil
}

func (instance *IMSDCMInstance) UnmarshalBinary(value []byte) error {
	if len(value) != 4 {
		return fmt.Errorf("instance length %d, want 4", len(value))
	}
	decoded := IMSDCMInstance(int32(binary.LittleEndian.Uint32(value)))
	if err := decoded.validate(); err != nil {
		return err
	}
	*instance = decoded
	return nil
}

func decodeIMSDCMAddress(value []byte) (netip.Addr, error) {
	if len(value) < 5 {
		return netip.Addr{}, errors.New("parsing QMI IMS DCM activation: address TLV is truncated")
	}
	family := IMSDCMIPFamily(binary.LittleEndian.Uint32(value[:4]))
	length := int(value[4])
	if len(value) != 5+length {
		return netip.Addr{}, fmt.Errorf("parsing QMI IMS DCM activation: address TLV length %d, want %d", len(value), 5+length)
	}
	address, err := netip.ParseAddr(string(value[5:]))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parsing QMI IMS DCM activation address: %w", err)
	}
	if family == IMSDCMIPv4 && !address.Is4() || family == IMSDCMIPv6 && !address.Is6() {
		return netip.Addr{}, fmt.Errorf("parsing QMI IMS DCM activation: address %s does not match family %d", address, family)
	}
	if family > IMSDCMIPv6 {
		return netip.Addr{}, fmt.Errorf("parsing QMI IMS DCM activation: IP family %d is out of range", family)
	}
	return address, nil
}
