package qcom

import (
	"context"
	"fmt"

	"github.com/damonto/wwan-go/qcom/tlv"
)

// WMSIndicationState contains the optional registration state returned by the
// modem for each general WMS indication.
type WMSIndicationState struct {
	TransportLayer        bool
	TransportLayerKnown   bool
	TransportNetwork      bool
	TransportNetworkKnown bool
	CallStatus            bool
	CallStatusKnown       bool
	ServiceReady          bool
	ServiceReadyKnown     bool
	BroadcastConfig       bool
	BroadcastConfigKnown  bool
	TransportMWI          bool
	TransportMWIKnown     bool
	SIMReady              bool
	SIMReadyKnown         bool
	SMSCAddress           bool
	SMSCAddressKnown      bool
	MemoryFull            bool
	MemoryFullKnown       bool
}

// WMSIndicationRegistration reads general indication registrations for this
// WMS control point.
func (c *Client) WMSIndicationRegistration(ctx context.Context) (WMSIndicationState, error) {
	var state WMSIndicationState
	err := c.withServiceClient(ctx, ServiceWMS, func(clientID uint8) error {
		resp, err := c.requestService(ctx, ServiceWMS, clientID, MessageWMSGetIndicationRegister, nil)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return state.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return WMSIndicationState{}, fmt.Errorf("reading QMI WMS indication registration: %w", err)
	}
	return state, nil
}

// UnmarshalTLVs parses the WMS indication-registration response.
func (s *WMSIndicationState) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*s = WMSIndicationState{}
	fields := []struct {
		kind  byte
		value *bool
		known *bool
	}{
		{0x10, &s.TransportLayer, &s.TransportLayerKnown},
		{0x11, &s.TransportNetwork, &s.TransportNetworkKnown},
		{0x12, &s.CallStatus, &s.CallStatusKnown},
		{0x13, &s.ServiceReady, &s.ServiceReadyKnown},
		{0x14, &s.BroadcastConfig, &s.BroadcastConfigKnown},
		{0x15, &s.TransportMWI, &s.TransportMWIKnown},
		{0x16, &s.SIMReady, &s.SIMReadyKnown},
		{0x17, &s.SMSCAddress, &s.SMSCAddressKnown},
		{0x18, &s.MemoryFull, &s.MemoryFullKnown},
	}
	for _, field := range fields {
		value, ok := tlv.Value(tlvs, field.kind)
		if !ok {
			continue
		}
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI WMS indication registration: TLV 0x%02X length %d, want 1", field.kind, len(value))
		}
		enabled, err := decodeWMSBool(value[0])
		if err != nil {
			return fmt.Errorf("parsing QMI WMS indication registration TLV 0x%02X: %w", field.kind, err)
		}
		*field.value = enabled
		*field.known = true
	}
	return nil
}
