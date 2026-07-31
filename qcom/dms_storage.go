package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsUserDataMax = 512
	dmsERIFileMax  = 1024
)

// DMSReadUserDataRequest encodes the legacy Read User Data message.
type DMSReadUserDataRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSReadUserDataRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSReadUserData)
}

// DMSWriteUserDataRequest encodes the legacy Write User Data message.
type DMSWriteUserDataRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Data          []byte
}

// Request validates and converts the persistent payload into a QMI request.
func (r DMSWriteUserDataRequest) Request() (Request, error) {
	value, err := encodeDMSPersistentData(r.Data, dmsUserDataMax)
	if err != nil {
		return Request{}, fmt.Errorf("encoding QMI DMS user data: %w", err)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSWriteUserData,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(0x01, value)},
	}, nil
}

// DMSReadERIFileRequest encodes Read ERI File.
type DMSReadERIFileRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the query into a QMI request.
func (r DMSReadERIFileRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSReadERIFile)
}

// DMSPersistentDataResponse contains a mandatory length-prefixed payload.
type DMSPersistentDataResponse struct {
	Data []byte
	max  int
}

// UnmarshalTLVs parses a Read User Data or Read ERI File response.
func (r *DMSPersistentDataResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	max := r.max
	r.Data = nil
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("data TLV missing")
	}
	data, err := decodeDMSPersistentData(value, max)
	if err != nil {
		return err
	}
	r.Data = data
	return nil
}

// DMSUserData reads the legacy persistent user-data blob.
func (c *Client) DMSUserData(ctx context.Context) ([]byte, error) {
	result := DMSPersistentDataResponse{max: dmsUserDataMax}
	if err := c.dmsRead(ctx, MessageDMSReadUserData, &result); err != nil {
		return nil, fmt.Errorf("reading QMI DMS user data: %w", err)
	}
	return result.Data, nil
}

// DMSWriteUserData replaces the legacy persistent user-data blob.
func (c *Client) DMSWriteUserData(ctx context.Context, data []byte) error {
	req, err := (DMSWriteUserDataRequest{
		Timeout: DefaultRequestTimeout,
		Data:    data,
	}).Request()
	if err != nil {
		return fmt.Errorf("writing QMI DMS user data: %w", err)
	}
	if err := c.dmsReadRequest(ctx, req, nil); err != nil {
		return fmt.Errorf("writing QMI DMS user data: %w", err)
	}
	return nil
}

// DMSERIFile reads the modem's Extended Roaming Indicator file.
func (c *Client) DMSERIFile(ctx context.Context) ([]byte, error) {
	result := DMSPersistentDataResponse{max: dmsERIFileMax}
	if err := c.dmsRead(ctx, MessageDMSReadERIFile, &result); err != nil {
		return nil, fmt.Errorf("reading QMI DMS ERI file: %w", err)
	}
	return result.Data, nil
}

func encodeDMSPersistentData(data []byte, max int) ([]byte, error) {
	if len(data) > max {
		return nil, fmt.Errorf("data length %d exceeds maximum %d", len(data), max)
	}
	value := binary.LittleEndian.AppendUint16(nil, uint16(len(data)))
	return append(value, data...), nil
}

func decodeDMSPersistentData(value []byte, max int) ([]byte, error) {
	if len(value) < 2 {
		return nil, errors.New("data length is missing")
	}
	length := int(binary.LittleEndian.Uint16(value))
	if length > max {
		return nil, fmt.Errorf("data length %d exceeds maximum %d", length, max)
	}
	if len(value) != 2+length {
		return nil, fmt.Errorf("TLV length %d, want %d", len(value), 2+length)
	}
	if length == 0 {
		return []byte{}, nil
	}
	return append([]byte(nil), value[2:]...), nil
}
