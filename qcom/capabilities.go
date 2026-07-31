package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// SupportedMessages is the QMI message-ID bit mask returned by a service.
type SupportedMessages struct {
	mask []byte
}

// Supports reports whether the service advertises id.
func (s SupportedMessages) Supports(id MessageID) bool {
	index := int(id) / 8
	return index < len(s.mask) && s.mask[index]&(1<<uint(id%8)) != 0
}

// Mask returns a copy of the raw QMI message-ID bit mask.
func (s SupportedMessages) Mask() []byte {
	return slices.Clone(s.mask)
}

// ServiceSupportedMessages queries the message IDs implemented by a modem
// service. This is useful when the same application must support several modem
// firmware generations.
func (c *Client) ServiceSupportedMessages(ctx context.Context, service ServiceType) (SupportedMessages, error) {
	switch service {
	case ServiceDMS, ServiceNAS, ServiceUIM, ServiceWDA, ServiceWDS, ServiceWMS, ServiceVoice:
	default:
		return SupportedMessages{}, fmt.Errorf("reading QMI supported messages: service 0x%02X does not define this message", service)
	}
	var supported SupportedMessages
	err := c.withServiceClient(ctx, service, func(clientID uint8) error {
		resp, err := c.requestService(ctx, service, clientID, MessageGetSupportedMessages, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x10)
		if !ok {
			return errors.New("parsing QMI supported messages: message mask TLV missing")
		}
		mask, err := decodeQMILength16Bytes(value)
		if err != nil {
			return fmt.Errorf("parsing QMI supported messages: %w", err)
		}
		supported.mask = mask
		return nil
	})
	if err != nil {
		return SupportedMessages{}, fmt.Errorf("reading QMI service 0x%02X supported messages: %w", service, err)
	}
	return supported, nil
}

func decodeQMILength8Bytes(value []byte) ([]byte, error) {
	if len(value) < 1 {
		return nil, errors.New("length is truncated")
	}
	length := int(value[0])
	if len(value) != 1+length {
		return nil, fmt.Errorf("value length %d, want %d", len(value), 1+length)
	}
	return slices.Clone(value[1 : 1+length]), nil
}

func decodeQMILength16Bytes(value []byte) ([]byte, error) {
	if len(value) < 2 {
		return nil, errors.New("length is truncated")
	}
	length := int(binary.LittleEndian.Uint16(value[:2]))
	if len(value) != 2+length {
		return nil, fmt.Errorf("value length %d, want %d", len(value), 2+length)
	}
	return slices.Clone(value[2 : 2+length]), nil
}
