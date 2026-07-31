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

// UnmarshalTLVs parses QMI NAS serving-system TLVs.
func (s *NASServingSystem) UnmarshalTLVs(tlvs tlv.TLVs) error {
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
	if err := serving.unmarshalOptionalTLVs(tlvs); err != nil {
		return err
	}
	*s = serving
	return nil
}

func (serving *NASServingSystem) unmarshalOptionalTLVs(tlvs tlv.TLVs) error {
	if value, ok := tlv.Value(tlvs, nasTLVRoamingIndicator); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: roaming indicator TLV length %d, want 1", len(value))
		}
		serving.RoamingIndicator = NASRoamingIndicator(value[0])
		serving.RoamingIndicatorKnown = true
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
		serving.DataCapabilities = make([]NASDataCapability, count)
		for i, capability := range value[1:] {
			serving.DataCapabilities[i] = NASDataCapability(capability)
		}
		serving.DataCapabilitiesKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVCurrentPLMN); ok {
		plmn, err := parseNASPLMN(value)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS serving system current PLMN: %w", err)
		}
		serving.PLMN = plmn
		serving.PLMNKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVTimeZone); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: time zone TLV length %d, want 1", len(value))
		}
		serving.TimeZoneQuarterHours = int8(value[0])
		serving.TimeZoneKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVDaylightSaving); ok {
		if len(value) != 1 {
			return fmt.Errorf("parsing QMI NAS serving system: daylight saving TLV length %d, want 1", len(value))
		}
		serving.DaylightSavingHours = value[0]
		serving.DaylightSavingKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVLocationAreaCode); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS serving system: location area code TLV length %d, want 2", len(value))
		}
		serving.LocationAreaCode = binary.LittleEndian.Uint16(value)
		serving.LocationAreaKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVCellID); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS serving system: cell ID TLV length %d, want 4", len(value))
		}
		serving.CellID = binary.LittleEndian.Uint32(value)
		serving.CellIDKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVTrackingAreaCode); ok {
		if len(value) != 2 {
			return fmt.Errorf("parsing QMI NAS serving system: tracking area code TLV length %d, want 2", len(value))
		}
		serving.TrackingAreaCode = binary.LittleEndian.Uint16(value)
		serving.TrackingAreaKnown = true
	}
	if value, ok := tlv.Value(tlvs, nasTLVMNCIncludesPCSDigit); ok {
		if len(value) != 5 {
			return fmt.Errorf("parsing QMI NAS serving system: MNC digit TLV length %d, want 5", len(value))
		}
		mcc := binary.LittleEndian.Uint16(value[0:2])
		mnc := binary.LittleEndian.Uint16(value[2:4])
		if serving.PLMNKnown && serving.PLMN.MCC == mcc && serving.PLMN.MNC == mnc {
			serving.PLMN.MNCThreeDigits = value[4] != 0
			serving.PLMN.MNCThreeDigitsKnown = true
		}
	}
	if value, ok := tlv.Value(tlvs, nasTLVNetworkNameSource); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI NAS serving system: network name source TLV length %d, want 4", len(value))
		}
		serving.NetworkNameSource = NASNetworkNameSource(binary.LittleEndian.Uint32(value))
		serving.NetworkNameSourceKnown = true
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
