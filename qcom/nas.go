package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const nasTLVServingSystem = 0x01

// Response and indication IDs are paired explicitly because their offsets vary
// between fields, even within the same NAS message.
func nasTLVType(messageType MessageType, response, indication byte) byte {
	if messageType == MessageTypeIndication {
		return indication
	}
	return response
}

// NASGetServingSystemRequest encodes QMI NAS Get Serving System.
type NASGetServingSystemRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI NAS request.
func (r NASGetServingSystemRequest) Request() Request {
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASGetServingSystem,
		Timeout:       r.Timeout,
	}
}

// NASServingSystem reads the current QMI NAS serving-system fields.
func (c *Client) NASServingSystem(ctx context.Context) (NASServingSystem, error) {
	var serving NASServingSystem
	err := c.withServiceClient(ctx, ServiceNAS, func(clientID uint8) error {
		req := NASGetServingSystemRequest{ClientID: clientID, Timeout: DefaultRequestTimeout}.Request()
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed NASGetServingSystemResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		serving = parsed.ServingSystem
		return nil
	})
	if err != nil {
		return NASServingSystem{}, fmt.Errorf("querying QMI NAS serving system: %w", err)
	}
	return serving, nil
}

// NASGetServingSystemResponse is the parsed NAS Get Serving System response.
type NASGetServingSystemResponse struct {
	ServingSystem NASServingSystem
}

// UnmarshalTLVs parses the mandatory serving-system aggregate.
func (r *NASGetServingSystemResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	var serving NASServingSystem
	if err := serving.UnmarshalTLVs(tlvs); err != nil {
		return err
	}
	*r = NASGetServingSystemResponse{ServingSystem: serving}
	return nil
}

// UnmarshalTLVs parses QMI NAS Get Serving System response TLVs.
func (s *NASServingSystem) UnmarshalTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, MessageTypeResponse)
}

// UnmarshalIndicationTLVs parses QMI NAS Serving System indication TLVs.
func (s *NASServingSystem) UnmarshalIndicationTLVs(tlvs tlv.TLVs) error {
	return s.unmarshalTLVs(tlvs, MessageTypeIndication)
}

func (s *NASServingSystem) unmarshalTLVs(tlvs tlv.TLVs, messageType MessageType) error {
	value, ok := tlv.Value(tlvs, nasTLVServingSystem)
	if !ok {
		return errors.New("parsing QMI NAS serving system: serving system TLV missing")
	}
	if len(value) < 5 {
		return errors.New("parsing QMI NAS serving system: serving system TLV is truncated")
	}
	count := int(value[4])
	want := 5 + count
	if len(value) != want {
		return fmt.Errorf("parsing QMI NAS serving system: serving system TLV length %d, want %d", len(value), want)
	}
	serving := NASServingSystem{
		RegistrationState: NASRegistrationState(value[0]),
		CSAttachState:     NASAttachState(value[1]),
		PSAttachState:     NASAttachState(value[2]),
		SelectedNetwork:   NASSelectedNetwork(value[3]),
		RadioInterfaces:   make([]NASRadioInterface, count),
	}
	for i, radio := range value[5 : 5+count] {
		serving.RadioInterfaces[i] = NASRadioInterface(radio)
	}
	if err := serving.unmarshalOptionalTLVs(tlvs, messageType); err != nil {
		return err
	}
	*s = serving
	return nil
}

func (s *NASServingSystem) unmarshalOptionalTLVs(tlvs tlv.TLVs, messageType MessageType) error {
	if value, ok := tlv.Value(tlvs, nasTLVRoamingIndicator); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: roaming indicator TLV length %d, want 1", len(value))
		}
		s.RoamingIndicator = NASRoamingIndicator(value[0])
		s.RoamingIndicatorKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVDataCapabilities); ok {
		if len(value) < 1 {
			return errors.New("parsing QMI NAS serving system: data capabilities count is truncated")
		}
		count := int(value[0])
		if count > nasMaxDataCapabilities {
			return fmt.Errorf("parsing QMI NAS serving system: data capabilities count %d exceeds maximum %d", count, nasMaxDataCapabilities)
		}
		if len(value) != 1+count {
			return fmt.Errorf("parsing QMI NAS serving system: data capabilities TLV length %d, want %d", len(value), 1+count)
		}
		s.DataCapabilities = make([]NASDataCapability, count)
		for i, capability := range value[1:] {
			s.DataCapabilities[i] = NASDataCapability(capability)
		}
		s.DataCapabilitiesKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVCurrentPLMN); ok {
		plmn, err := parseNASPLMN(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS serving system current PLMN: %w", err)
		}
		s.PLMN = plmn
		s.PLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVTimeZone); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: time zone TLV length %d, want 1", len(value))
		}
		s.TimeZoneQuarterHours = int8(value[0])
		s.TimeZoneKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVDaylightSaving); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: daylight saving TLV length %d, want 1", len(value))
		}
		s.DaylightSavingHours = value[0]
		s.DaylightSavingKnown = true
	}
	// Qualcomm nas_serving_system_ind_msg_data_v01 adds time and no-change
	// fields, so these response and indication IDs differ.
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVLocationAreaCode, 0x1D)); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS serving system: location area code TLV length %d, want 2", len(value))
		}
		s.LocationAreaCode = binary.LittleEndian.Uint16(value)
		s.LocationAreaKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVCellID, 0x1E)); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS serving system: cell ID TLV length %d, want 4", len(value))
		}
		s.CellID = binary.LittleEndian.Uint32(value)
		s.CellIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVTrackingAreaCode, 0x25)); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS serving system: tracking area code TLV length %d, want 2", len(value))
		}
		s.TrackingAreaCode = binary.LittleEndian.Uint16(value)
		s.TrackingAreaKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVMNCIncludesPCSDigit, 0x29)); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI NAS serving system: MNC digit TLV length %d, want 5", len(value))
		}
		mcc := binary.LittleEndian.Uint16(value[0:2])
		mnc := binary.LittleEndian.Uint16(value[2:4])
		if s.PLMNKnown && s.PLMN.MCC == mcc && s.PLMN.MNC == mnc {
			s.PLMN.MNCThreeDigits = value[4] != 0
			s.PLMN.MNCThreeDigitsKnown = true
		}
	}
	if value, ok := tlv.Value(tlvs, nasTLVType(messageType, nasTLVNetworkNameSource, 0x2B)); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS serving system: network name source TLV length %d, want 4", len(value))
		}
		s.NetworkNameSource = NASNetworkNameSource(binary.LittleEndian.Uint32(value))
		s.NetworkNameSourceKnown = true
	}
	return nil
}

// NASGetSysInfoRequest encodes QMI NAS Get System Info.
type NASGetSysInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI NAS request.
func (r NASGetSysInfoRequest) Request() Request {
	return Request{
		Service:       ServiceNAS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageNASGetSysInfo,
		Timeout:       r.Timeout,
	}
}

// NASGetSysInfoResponse is the parsed NAS Get System Info response.
type NASGetSysInfoResponse struct {
	SysInfo NASSysInfo
}
