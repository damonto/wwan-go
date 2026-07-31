package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dpmPortMax     = 32
	dpmPortNameMax = 32
)

// DPMControlPort maps a QMI control-port name to its default data endpoint.
type DPMControlPort struct {
	Name            string
	DefaultEndpoint DataEndpoint
}

// DPMHardwareDataPort describes a hardware-accelerated data endpoint.
type DPMHardwareDataPort struct {
	Endpoint     DataEndpoint
	ConsumerPipe uint32
	ProducerPipe uint32
}

// DPMSoftwareDataPort maps a software data endpoint to its port name.
type DPMSoftwareDataPort struct {
	Endpoint DataEndpoint
	Name     string
}

// DPMDataPortBufferInfo describes the modem-facing FIFO sizes of a data port.
type DPMDataPortBufferInfo struct {
	Endpoint           DataEndpoint
	UplinkFIFOSize     uint32
	DownlinkFIFOSize   uint32
	DownlinkBufferSize uint32
}

// DPMOpenPortConfig selects the control and data ports to open.
type DPMOpenPortConfig struct {
	ControlPorts      []DPMControlPort
	HardwareDataPorts []DPMHardwareDataPort
	SoftwareDataPorts []DPMSoftwareDataPort
	DataPortBuffers   []DPMDataPortBufferInfo
}

// DPMClosePortConfig selects the control and data ports to close.
type DPMClosePortConfig struct {
	ControlPortNames []string
	DataPorts        []DataEndpoint
}

// DPMCapabilities reports optional data-port mapper features.
type DPMCapabilities struct {
	ShimSupported      bool
	ShimSupportedKnown bool
	ReverseIPSync      bool
	ReverseIPSyncKnown bool
	CLATSupported      bool
	CLATSupportedKnown bool
}

// DPMOpenPortRequest encodes QMI DPM Open Port.
type DPMOpenPortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        DPMOpenPortConfig
}

// Request converts the port mappings into QMI DPM TLVs.
func (r DPMOpenPortRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDPM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDPMOpenPort,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DPMClosePortRequest encodes QMI DPM Close Port.
type DPMClosePortRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        DPMClosePortConfig
}

// Request converts the selected ports into QMI DPM TLVs.
func (r DPMClosePortRequest) Request() (Request, error) {
	tlvs, err := r.Config.MarshalTLVs()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDPM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDPMClosePort,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DPMGetCapabilitiesRequest encodes Get Supported Capabilities.
type DPMGetCapabilitiesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DPMGetCapabilitiesRequest) Request() Request {
	return Request{
		Service:       ServiceDPM,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDPMGetCapabilities,
		Timeout:       r.Timeout,
	}
}

// DPMGetCapabilitiesResponse contains parsed optional feature flags.
type DPMGetCapabilitiesResponse struct {
	Capabilities DPMCapabilities
}

// UnmarshalTLVs parses Get Supported Capabilities response TLVs.
func (r *DPMGetCapabilitiesResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DPMGetCapabilitiesResponse{}
	fields := []struct {
		typ   uint8
		value *bool
		known *bool
	}{
		{0x10, &r.Capabilities.ShimSupported, &r.Capabilities.ShimSupportedKnown},
		{0x11, &r.Capabilities.ReverseIPSync, &r.Capabilities.ReverseIPSyncKnown},
		{0x12, &r.Capabilities.CLATSupported, &r.Capabilities.CLATSupportedKnown},
	}
	for _, field := range fields {
		value, ok := tlv.Value(tlvs, field.typ)
		if !ok {
			continue
		}
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI DPM capabilities: TLV 0x%02X length %d, want 1", field.typ, len(value))
		}
		if value[0] > 1 {
			return fmt.Errorf("parsing QMI DPM capabilities: TLV 0x%02X value %d is not boolean", field.typ, value[0])
		}
		*field.value = value[0] == 1
		*field.known = true
	}
	return nil
}

// DPMOpenPort opens the requested modem control and data ports.
func (c *Client) DPMOpenPort(ctx context.Context, config DPMOpenPortConfig) error {
	err := c.withServiceClient(ctx, ServiceDPM, func(clientID uint8) error {
		req, err := (DPMOpenPortRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   config,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("opening QMI DPM ports: %w", err)
	}
	return nil
}

// DPMClosePort closes the requested modem control and data ports.
func (c *Client) DPMClosePort(ctx context.Context, config DPMClosePortConfig) error {
	err := c.withServiceClient(ctx, ServiceDPM, func(clientID uint8) error {
		req, err := (DPMClosePortRequest{
			ClientID: clientID,
			Timeout:  DefaultRequestTimeout,
			Config:   config,
		}).Request()
		if err != nil {
			return err
		}
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
	if err != nil {
		return fmt.Errorf("closing QMI DPM ports: %w", err)
	}
	return nil
}

// DPMCapabilities returns features supported by the data-port mapper.
func (c *Client) DPMCapabilities(ctx context.Context) (DPMCapabilities, error) {
	var capabilities DPMCapabilities
	err := c.withServiceClient(ctx, ServiceDPM, func(clientID uint8) error {
		req := DPMGetCapabilitiesRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, nil, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed DPMGetCapabilitiesResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		capabilities = parsed.Capabilities
		return nil
	})
	if err != nil {
		return DPMCapabilities{}, fmt.Errorf("querying QMI DPM capabilities: %w", err)
	}
	return capabilities, nil
}

// MarshalTLVs encodes DPM open-port configuration.
func (config DPMOpenPortConfig) MarshalTLVs() (tlv.TLVs, error) {
	if len(config.ControlPorts) == 0 && len(config.HardwareDataPorts) == 0 &&
		len(config.SoftwareDataPorts) == 0 && len(config.DataPortBuffers) == 0 {
		return nil, errors.New("encoding QMI DPM open port: no ports configured")
	}

	var tlvs tlv.TLVs
	if len(config.ControlPorts) > 0 {
		value, err := encodeDPMList(len(config.ControlPorts), func(dst []byte, index int) ([]byte, error) {
			name, err := dpmPortName(config.ControlPorts[index].Name)
			if err != nil {
				return nil, err
			}
			dst = append(dst, byte(len(name)))
			dst = append(dst, name...)
			return config.ControlPorts[index].DefaultEndpoint.AppendBinary(dst)
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM control ports: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if len(config.HardwareDataPorts) > 0 {
		value, err := encodeDPMList(len(config.HardwareDataPorts), func(dst []byte, index int) ([]byte, error) {
			port := config.HardwareDataPorts[index]
			dst, err := port.Endpoint.AppendBinary(dst)
			if err != nil {
				return nil, err
			}
			dst = binary.LittleEndian.AppendUint32(dst, port.ConsumerPipe)
			return binary.LittleEndian.AppendUint32(dst, port.ProducerPipe), nil
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM hardware data ports: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}
	if len(config.SoftwareDataPorts) > 0 {
		value, err := encodeDPMList(len(config.SoftwareDataPorts), func(dst []byte, index int) ([]byte, error) {
			port := config.SoftwareDataPorts[index]
			dst, err := port.Endpoint.AppendBinary(dst)
			if err != nil {
				return nil, err
			}
			name, err := dpmPortName(port.Name)
			if err != nil {
				return nil, err
			}
			dst = append(dst, byte(len(name)))
			return append(dst, name...), nil
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM software data ports: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x12, value))
	}
	if len(config.DataPortBuffers) > 0 {
		value, err := encodeDPMList(len(config.DataPortBuffers), func(dst []byte, index int) ([]byte, error) {
			info := config.DataPortBuffers[index]
			dst, err := info.Endpoint.AppendBinary(dst)
			if err != nil {
				return nil, err
			}
			dst = binary.LittleEndian.AppendUint32(dst, info.UplinkFIFOSize)
			dst = binary.LittleEndian.AppendUint32(dst, info.DownlinkFIFOSize)
			return binary.LittleEndian.AppendUint32(dst, info.DownlinkBufferSize), nil
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM data port buffers: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x13, value))
	}
	return tlvs, nil
}

// MarshalTLVs encodes DPM close-port configuration.
func (config DPMClosePortConfig) MarshalTLVs() (tlv.TLVs, error) {
	if len(config.ControlPortNames) == 0 && len(config.DataPorts) == 0 {
		return nil, errors.New("encoding QMI DPM close port: no ports configured")
	}
	var tlvs tlv.TLVs
	if len(config.ControlPortNames) > 0 {
		value, err := encodeDPMList(len(config.ControlPortNames), func(dst []byte, index int) ([]byte, error) {
			name, err := dpmPortName(config.ControlPortNames[index])
			if err != nil {
				return nil, err
			}
			dst = append(dst, byte(len(name)))
			return append(dst, name...), nil
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM control ports: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, value))
	}
	if len(config.DataPorts) > 0 {
		value, err := encodeDPMList(len(config.DataPorts), func(dst []byte, index int) ([]byte, error) {
			return config.DataPorts[index].AppendBinary(dst)
		})
		if err != nil {
			return nil, fmt.Errorf("encoding QMI DPM data ports: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x11, value))
	}
	return tlvs, nil
}

func encodeDPMList(count int, appendItem func([]byte, int) ([]byte, error)) ([]byte, error) {
	if count > dpmPortMax {
		return nil, fmt.Errorf("port count %d exceeds %d", count, dpmPortMax)
	}
	value := make([]byte, 1, 1+count*8)
	value[0] = byte(count)
	for index := range count {
		var err error
		value, err = appendItem(value, index)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func dpmPortName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("port name is empty")
	}
	if len(name) > dpmPortNameMax {
		return "", fmt.Errorf("port name length %d exceeds %d", len(name), dpmPortNameMax)
	}
	return name, nil
}
