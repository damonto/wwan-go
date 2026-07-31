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

const dsdAPNMaxLength = 100

// DSDAPNType identifies one standardized APN use.
type DSDAPNType uint32

const (
	DSDAPNTypeDefault DSDAPNType = iota
	DSDAPNTypeIMS
	DSDAPNTypeMMS
	DSDAPNTypeDUN
	DSDAPNTypeSUPL
	DSDAPNTypeHIPRI
	DSDAPNTypeFOTA
	DSDAPNTypeCBS
	DSDAPNTypeIA
	DSDAPNTypeEmergency
)

// DSDAPNTypePreference is a mask of standardized APN uses.
type DSDAPNTypePreference uint64

const (
	DSDAPNTypePreferenceDefault DSDAPNTypePreference = 1 << iota
	DSDAPNTypePreferenceIMS
	DSDAPNTypePreferenceMMS
	DSDAPNTypePreferenceDUN
	DSDAPNTypePreferenceSUPL
	DSDAPNTypePreferenceHIPRI
	DSDAPNTypePreferenceFOTA
	DSDAPNTypePreferenceCBS
	DSDAPNTypePreferenceIA
	DSDAPNTypePreferenceEmergency
)

const dsdAPNTypePreferenceAll = DSDAPNTypePreferenceDefault |
	DSDAPNTypePreferenceIMS |
	DSDAPNTypePreferenceMMS |
	DSDAPNTypePreferenceDUN |
	DSDAPNTypePreferenceSUPL |
	DSDAPNTypePreferenceHIPRI |
	DSDAPNTypePreferenceFOTA |
	DSDAPNTypePreferenceCBS |
	DSDAPNTypePreferenceIA |
	DSDAPNTypePreferenceEmergency

// DSDAPNInfo contains the APN mapped to a standardized APN type.
type DSDAPNInfo struct {
	Name      string
	NameKnown bool
}

// DSDAPNTypeConfig assigns standardized uses to an APN. PreferredTypes is
// omitted when nil so callers do not overwrite a modem-specific preference.
type DSDAPNTypeConfig struct {
	Name           string
	Types          DSDAPNTypePreference
	PreferredTypes *DSDAPNTypePreference
}

// DSDGetAPNInfoRequest encodes QMI DSD Get APN Info.
type DSDGetAPNInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Type          DSDAPNType
}

// Request validates and converts the APN lookup into a QMI request.
func (r DSDGetAPNInfoRequest) Request() (Request, error) {
	if err := validateDSDAPNType(r.Type); err != nil {
		return Request{}, fmt.Errorf("encoding QMI DSD APN lookup: %w", err)
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDGetAPNInfo,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint32(r.Type))},
	}, nil
}

// DSDSetAPNTypeRequest encodes QMI DSD Set APN Type.
type DSDSetAPNTypeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Config        DSDAPNTypeConfig
}

// Request validates and converts the APN type assignment into QMI TLVs.
func (r DSDSetAPNTypeRequest) Request() (Request, error) {
	value, err := r.Config.MarshalBinary()
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI DSD APN types: %w", err)
	}
	tlvs := tlv.TLVs{tlv.Bytes(0x01, value)}
	if r.Config.PreferredTypes != nil {
		if err := validateDSDAPNTypePreference(*r.Config.PreferredTypes); err != nil {
			return Request{}, fmt.Errorf("encoding QMI DSD preferred APN types: %w", err)
		}
		tlvs = append(tlvs, tlv.Bytes(0x10, binary.LittleEndian.AppendUint64(nil, uint64(*r.Config.PreferredTypes))))
	}
	return Request{
		Service:       ServiceDSD,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDSDSetAPNType,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DSDGetAPNInfoResponse is the parsed APN lookup response.
type DSDGetAPNInfoResponse struct {
	Info DSDAPNInfo
}

// UnmarshalTLVs parses the optional APN name.
func (r *DSDGetAPNInfoResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DSDGetAPNInfoResponse{}
	value, ok := tlv.Value(tlvs, 0x10)
	if !ok {
		return nil
	}
	name := string(value)
	if err := validateDSDAPNName(name); err != nil {
		return fmt.Errorf("parsing QMI DSD APN info: %w", err)
	}
	r.Info = DSDAPNInfo{Name: name, NameKnown: true}
	return nil
}

// DSDAPNInfo reads the APN assigned to a standardized APN type.
func (c *Client) DSDAPNInfo(ctx context.Context, apnType DSDAPNType) (DSDAPNInfo, error) {
	req, err := (DSDGetAPNInfoRequest{Timeout: DefaultRequestTimeout, Type: apnType}).Request()
	if err != nil {
		return DSDAPNInfo{}, fmt.Errorf("reading QMI DSD APN info: %w", err)
	}

	var info DSDAPNInfo
	err = c.withServiceClient(ctx, ServiceDSD, func(clientID uint8) error {
		req.ClientID = clientID
		resp, err := c.requestServiceWithTimeout(ctx, req.Service, req.ClientID, req.MessageID, req.TLVs, req.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		var parsed DSDGetAPNInfoResponse
		if err := parsed.UnmarshalTLVs(resp.TLVs); err != nil {
			return err
		}
		info = parsed.Info
		return nil
	})
	if err != nil {
		return DSDAPNInfo{}, fmt.Errorf("reading QMI DSD APN info: %w", err)
	}
	return info, nil
}

// DSDSetAPNType assigns standardized APN uses to an APN.
func (c *Client) DSDSetAPNType(ctx context.Context, config DSDAPNTypeConfig) error {
	req, err := (DSDSetAPNTypeRequest{Timeout: DefaultRequestTimeout, Config: config}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DSD APN types: %w", err)
	}
	if err := c.dsdResultRequest(ctx, req); err != nil {
		return fmt.Errorf("setting QMI DSD APN types: %w", err)
	}
	return nil
}

// MarshalBinary encodes the QMI DSD APN type aggregate.
func (config DSDAPNTypeConfig) MarshalBinary() ([]byte, error) {
	if err := validateDSDAPNName(config.Name); err != nil {
		return nil, err
	}
	if err := validateDSDAPNTypePreference(config.Types); err != nil {
		return nil, err
	}
	value := make([]byte, 1, 1+len(config.Name)+8)
	value[0] = byte(len(config.Name))
	value = append(value, config.Name...)
	value = binary.LittleEndian.AppendUint64(value, uint64(config.Types))
	return value, nil
}

func validateDSDAPNType(apnType DSDAPNType) error {
	if apnType > DSDAPNTypeEmergency {
		return fmt.Errorf("APN type %d is out of range", apnType)
	}
	return nil
}

func validateDSDAPNTypePreference(preference DSDAPNTypePreference) error {
	if preference&^dsdAPNTypePreferenceAll != 0 {
		return fmt.Errorf("APN type mask 0x%X contains unsupported bits", uint64(preference))
	}
	return nil
}

func validateDSDAPNName(name string) error {
	if strings.IndexByte(name, 0) >= 0 {
		return errors.New("APN name contains a NUL byte")
	}
	if len(name) > dsdAPNMaxLength {
		return fmt.Errorf("APN name length %d exceeds maximum %d", len(name), dsdAPNMaxLength)
	}
	return nil
}
