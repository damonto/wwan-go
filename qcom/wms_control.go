package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WMSModifyTagRequest changes the state tag on a stored message.
type WMSModifyTagRequest struct {
	Reference   WMSMessageReference
	Tag         WMSTag
	MessageMode *WMSMessageMode
}

// WMSStoreCapacityRequest selects the SMS store whose capacity is queried.
type WMSStoreCapacityRequest struct {
	Storage     WMSStorage
	MessageMode *WMSMessageMode
}

// WMSStoreCapacityInfo contains the maximum and optional free slot counts.
type WMSStoreCapacityInfo struct {
	Maximum   uint32
	Free      uint32
	FreeKnown bool
}

// WMSStoreCapacityResponse is the parsed Get Store Max Size response.
type WMSStoreCapacityResponse struct {
	Capacity WMSStoreCapacityInfo
}

// UnmarshalTLVs parses the mandatory maximum and optional free slot counts.
func (r *WMSStoreCapacityResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = WMSStoreCapacityResponse{}
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("parsing QMI WMS store capacity: maximum size TLV missing")
	}
	if len(value) != 4 {
		return fmt.Errorf("parsing QMI WMS store capacity: maximum size TLV length %d, want 4", len(value))
	}
	r.Capacity.Maximum = binary.LittleEndian.Uint32(value)
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI WMS store capacity: free slots TLV length %d, want 4", len(value))
		}
		r.Capacity.Free = binary.LittleEndian.Uint32(value)
		r.Capacity.FreeKnown = true
	}
	return nil
}

// WMSDomainPreference selects the circuit-switched or packet-switched domain
// used for 3GPP SMS.
type WMSDomainPreference uint8

const (
	WMSDomainPreferenceCS WMSDomainPreference = iota
	WMSDomainPreferencePS
	WMSDomainPreferenceCSOnly
	WMSDomainPreferencePSOnly
)

// WMSSubscription identifies the subscription bound to a WMS control point.
type WMSSubscription uint8

const (
	WMSSubscriptionPrimary WMSSubscription = iota
	WMSSubscriptionSecondary
	WMSSubscriptionTertiary
)

// WMSReset resets state owned by this WMS client.
func (c *Client) WMSReset(ctx context.Context) error {
	if err := c.wmsResultRequest(ctx, MessageWMSReset, nil); err != nil {
		return fmt.Errorf("resetting QMI WMS service: %w", err)
	}
	return nil
}

// WMSModifyTag changes the read or delivery tag on a stored SMS.
func (c *Client) WMSModifyTag(ctx context.Context, req WMSModifyTagRequest) error {
	value, err := req.Reference.MarshalBinary()
	if err != nil {
		return fmt.Errorf("modifying QMI WMS message tag: %w", err)
	}
	value = append(value, byte(req.Tag))
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if req.MessageMode != nil {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.MessageMode)))
	}
	if err := c.wmsResultRequest(ctx, MessageWMSModifyTag, tlvs); err != nil {
		return fmt.Errorf("modifying QMI WMS message tag: %w", err)
	}
	return nil
}

// WMSCurrentMessageProtocol reports the protocol selected for this WMS client.
func (c *Client) WMSCurrentMessageProtocol(ctx context.Context) (WMSMessageProtocol, error) {
	var protocol WMSMessageProtocol
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetMessageProtocol, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, 0x01)
		if !ok {
			return errors.New("parsing QMI WMS message protocol: protocol TLV missing")
		}
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS message protocol: TLV length %d, want 1", len(value))
		}
		protocol = WMSMessageProtocol(value[0])
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("reading QMI WMS message protocol: %w", err)
	}
	return protocol, nil
}

// WMSStoreCapacity reads the maximum and available slots in an SMS store.
func (c *Client) WMSStoreCapacity(ctx context.Context, req WMSStoreCapacityRequest) (WMSStoreCapacityInfo, error) {
	if req.Storage != WMSStorageUIM && req.Storage != WMSStorageNV {
		return WMSStoreCapacityInfo{}, fmt.Errorf("reading QMI WMS store capacity: storage %d is out of range", req.Storage)
	}
	tlvs := tlv.TLVs{tlv.Uint(0x01, uint8(req.Storage))}
	if req.MessageMode != nil {
		if err := validateWMSMessageMode(*req.MessageMode); err != nil {
			return WMSStoreCapacityInfo{}, fmt.Errorf("reading QMI WMS store capacity: %w", err)
		}
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(*req.MessageMode)))
	}

	var parsed WMSStoreCapacityResponse
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetStoreMaxSize, tlvs)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return parsed.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSStoreCapacityInfo{}, fmt.Errorf("reading QMI WMS store capacity: %w", err)
	}
	return parsed.Capacity, nil
}

// WMSSetMemoryAvailable reports whether the primary client can accept another
// SMS.
func (c *Client) WMSSetMemoryAvailable(ctx context.Context, available bool) error {
	if err := c.wmsResultRequest(ctx, MessageWMSSetMemoryStatus, tlv.TLVs{tlv.Uint(0x01, boolByte(available))}); err != nil {
		return fmt.Errorf("setting QMI WMS memory status: %w", err)
	}
	return nil
}

// WMSMemoryAvailable reads the memory status reported by this client.
func (c *Client) WMSMemoryAvailable(ctx context.Context) (bool, error) {
	available, err := c.wmsBoolResponse(ctx, MessageWMSGetMemoryStatus, 0x10)
	if err != nil {
		return false, fmt.Errorf("reading QMI WMS memory status: %w", err)
	}
	return available, nil
}

// WMSDomainPreference reads the deprecated 3GPP SMS domain preference.
func (c *Client) WMSDomainPreference(ctx context.Context) (WMSDomainPreference, error) {
	value, err := c.wmsSingleByteResponse(ctx, MessageWMSGetDomainPreference, 0x01)
	if err != nil {
		return 0, fmt.Errorf("reading QMI WMS domain preference: %w", err)
	}
	preference := WMSDomainPreference(value)
	if err := validateWMSDomainPreference(preference); err != nil {
		return 0, fmt.Errorf("reading QMI WMS domain preference: %w", err)
	}
	return preference, nil
}

// WMSSetDomainPreference sets the deprecated 3GPP SMS domain preference.
func (c *Client) WMSSetDomainPreference(ctx context.Context, preference WMSDomainPreference) error {
	if err := validateWMSDomainPreference(preference); err != nil {
		return fmt.Errorf("setting QMI WMS domain preference: %w", err)
	}
	if err := c.wmsResultRequest(ctx, MessageWMSSetDomainPreference, tlv.TLVs{tlv.Uint(0x01, uint8(preference))}); err != nil {
		return fmt.Errorf("setting QMI WMS domain preference: %w", err)
	}
	return nil
}

// WMSSetPrimaryClient changes whether this control point is the WMS primary
// client.
func (c *Client) WMSSetPrimaryClient(ctx context.Context, primary bool) error {
	if err := c.wmsResultRequest(ctx, MessageWMSSetPrimaryClient, tlv.TLVs{tlv.Uint(0x01, boolByte(primary))}); err != nil {
		return fmt.Errorf("setting QMI WMS primary client: %w", err)
	}
	return nil
}

// WMSPrimaryClient reports whether this control point is the WMS primary
// client.
func (c *Client) WMSPrimaryClient(ctx context.Context) (bool, error) {
	primary, err := c.wmsBoolResponse(ctx, MessageWMSGetPrimaryClient, 0x10)
	if err != nil {
		return false, fmt.Errorf("reading QMI WMS primary client: %w", err)
	}
	return primary, nil
}

// WMSBindSubscription binds this WMS control point to a subscription.
func (c *Client) WMSBindSubscription(ctx context.Context, subscription WMSSubscription) error {
	if err := validateWMSSubscription(subscription); err != nil {
		return fmt.Errorf("binding QMI WMS subscription: %w", err)
	}
	if err := c.wmsResultRequest(ctx, MessageWMSBindSubscription, tlv.TLVs{tlv.Uint(0x01, uint8(subscription))}); err != nil {
		return fmt.Errorf("binding QMI WMS subscription: %w", err)
	}
	return nil
}

// WMSBoundSubscription reads the subscription bound to this WMS control
// point.
func (c *Client) WMSBoundSubscription(ctx context.Context) (WMSSubscription, error) {
	value, err := c.wmsSingleByteResponse(ctx, MessageWMSGetSubscription, 0x10)
	if err != nil {
		return 0, fmt.Errorf("reading QMI WMS bound subscription: %w", err)
	}
	subscription := WMSSubscription(value)
	if err := validateWMSSubscription(subscription); err != nil {
		return 0, fmt.Errorf("reading QMI WMS bound subscription: %w", err)
	}
	return subscription, nil
}

func (c *Client) wmsResultRequest(ctx context.Context, id MessageID, tlvs tlv.TLVs) error {
	return c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, id, tlvs)
		if err != nil {
			return err
		}
		return resultOK(resp)
	})
}

func (c *Client) wmsSingleByteResponse(ctx context.Context, id MessageID, kind byte) (byte, error) {
	var result byte
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, id, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		value, ok := tlv.Value(resp.TLVs, kind)
		if !ok {
			return fmt.Errorf("response TLV 0x%02X missing", kind)
		}
		if len(value) != 1 {
			return fmt.Errorf("response TLV 0x%02X length %d, want 1", kind, len(value))
		}
		result = value[0]
		return nil
	})
	return result, err
}

func (c *Client) wmsBoolResponse(ctx context.Context, id MessageID, kind byte) (bool, error) {
	value, err := c.wmsSingleByteResponse(ctx, id, kind)
	if err != nil {
		return false, err
	}
	return decodeWMSBool(value)
}

func validateWMSDomainPreference(preference WMSDomainPreference) error {
	if preference > WMSDomainPreferencePSOnly {
		return fmt.Errorf("domain preference %d is out of range", preference)
	}
	return nil
}

func validateWMSSubscription(subscription WMSSubscription) error {
	if subscription > WMSSubscriptionTertiary {
		return fmt.Errorf("subscription %d is out of range", subscription)
	}
	return nil
}
