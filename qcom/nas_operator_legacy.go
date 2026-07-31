package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	nasMaxServiceProviderName = 16
	nasMaxOperatorPLMNRecords = 255
	nasMaxOperatorPLMNNames   = 64
	nasMaxOperatorName        = 255
)

// NASServiceProviderName contains the raw EF-SPN value and display condition.
type NASServiceProviderName struct {
	DisplayCondition uint8
	Name             []byte
}

// NASOperatorPLMNRecord maps a PLMN and location-area range to a name record.
type NASOperatorPLMNRecord struct {
	MCC          string
	MNC          string
	LACStart     uint16
	LACEnd       uint16
	NameRecordID uint8
}

// NASOperatorPLMNName contains one encoded long and short operator name.
type NASOperatorPLMNName struct {
	Long  NASEncodedNetworkName
	Short NASEncodedNetworkName
}

// NASOperatorNameData contains legacy operator-name sources from the modem.
type NASOperatorNameData struct {
	ServiceProvider      NASServiceProviderName
	ServiceProviderKnown bool
	PLMNRecords          []NASOperatorPLMNRecord
	PLMNRecordsKnown     bool
	PLMNNames            []NASOperatorPLMNName
	PLMNNamesKnown       bool
	OperatorName         string
	OperatorNameKnown    bool
	NITZ                 NASOperatorPLMNName
	NITZKnown            bool
}

// OperatorNameData returns legacy SIM and network operator-name sources.
func (c *Client) OperatorNameData(ctx context.Context) (NASOperatorNameData, error) {
	var result NASOperatorNameData
	if err := c.nasRead(ctx, MessageNASGetOperatorName, &result); err != nil {
		return NASOperatorNameData{}, fmt.Errorf("querying QMI NAS operator-name data: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses Get Operator Name and Operator Name indication TLVs.
func (d *NASOperatorNameData) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*d = NASOperatorNameData{}
	if value, ok := tlv.Value(tlvs, 0x10); ok {
		if len(value) < 1 {
			return errors.New("parsing QMI NAS operator-name data: service-provider header is truncated")
		}
		if len(value)-1 > nasMaxServiceProviderName {
			return fmt.Errorf("parsing QMI NAS operator-name data: service-provider name length %d exceeds %d", len(value)-1, nasMaxServiceProviderName)
		}
		d.ServiceProvider = NASServiceProviderName{
			DisplayCondition: value[0],
			Name:             slices.Clone(value[1:]),
		}
		d.ServiceProviderKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x11); ok {
		records, err := decodeNASOperatorPLMNRecords(value)
		if err != nil {
			return err
		}
		d.PLMNRecords = records
		d.PLMNRecordsKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x12); ok {
		names, err := decodeNASOperatorPLMNNames(value)
		if err != nil {
			return err
		}
		d.PLMNNames = names
		d.PLMNNamesKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x13); ok {
		if len(value) > nasMaxOperatorName {
			return fmt.Errorf("parsing QMI NAS operator-name data: operator name length %d exceeds %d", len(value), nasMaxOperatorName)
		}
		d.OperatorName = string(value)
		d.OperatorNameKnown = true
	}
	if value, ok := tlv.Value(tlvs, 0x14); ok {
		name, offset, err := decodeNASOperatorPLMNNameAt(value, 0)
		if err != nil {
			return fmt.Errorf("parsing QMI NAS operator-name NITZ information: %w", err)
		}
		if offset != len(value) {
			return fmt.Errorf("parsing QMI NAS operator-name NITZ information: %d trailing bytes", len(value)-offset)
		}
		d.NITZ = name
		d.NITZKnown = true
	}
	return nil
}

func decodeNASOperatorPLMNRecords(value []byte) ([]NASOperatorPLMNRecord, error) {
	if len(value) < 2 {
		return nil, errors.New("parsing QMI NAS operator PLMN list: count is truncated")
	}
	count := int(binary.LittleEndian.Uint16(value[:2]))
	if count > nasMaxOperatorPLMNRecords {
		return nil, fmt.Errorf("parsing QMI NAS operator PLMN list: count %d exceeds %d", count, nasMaxOperatorPLMNRecords)
	}
	const recordLength = 11
	want := 2 + count*recordLength
	if len(value) != want {
		return nil, fmt.Errorf("parsing QMI NAS operator PLMN list: TLV length %d, want %d", len(value), want)
	}
	records := make([]NASOperatorPLMNRecord, count)
	for index := range count {
		offset := 2 + index*recordLength
		records[index] = NASOperatorPLMNRecord{
			MCC:          string(value[offset : offset+3]),
			MNC:          string(value[offset+3 : offset+6]),
			LACStart:     binary.LittleEndian.Uint16(value[offset+6 : offset+8]),
			LACEnd:       binary.LittleEndian.Uint16(value[offset+8 : offset+10]),
			NameRecordID: value[offset+10],
		}
	}
	return records, nil
}

func decodeNASOperatorPLMNNames(value []byte) ([]NASOperatorPLMNName, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI NAS operator PLMN names: count is missing")
	}
	count := int(value[0])
	if count > nasMaxOperatorPLMNNames {
		return nil, fmt.Errorf("parsing QMI NAS operator PLMN names: count %d exceeds %d", count, nasMaxOperatorPLMNNames)
	}
	names := make([]NASOperatorPLMNName, count)
	offset := 1
	for index := range count {
		name, next, err := decodeNASOperatorPLMNNameAt(value, offset)
		if err != nil {
			return nil, fmt.Errorf("parsing QMI NAS operator PLMN name %d: %w", index, err)
		}
		names[index] = name
		offset = next
	}
	if offset != len(value) {
		return nil, fmt.Errorf("parsing QMI NAS operator PLMN names: %d trailing bytes", len(value)-offset)
	}
	return names, nil
}

func decodeNASOperatorPLMNNameAt(value []byte, offset int) (NASOperatorPLMNName, int, error) {
	if len(value)-offset < 6 {
		return NASOperatorPLMNName{}, offset, errors.New("metadata or name lengths are truncated")
	}
	encoding := NASNetworkDescriptionEncoding(value[offset])
	initials := NASCountryInitials(value[offset+1])
	longSpare := NASNameSpareBits(value[offset+2])
	shortSpare := NASNameSpareBits(value[offset+3])
	offset += 4

	longLength := int(value[offset])
	offset++
	if len(value)-offset < longLength+1 {
		return NASOperatorPLMNName{}, offset, errors.New("long name is truncated")
	}
	longName := slices.Clone(value[offset : offset+longLength])
	offset += longLength
	shortLength := int(value[offset])
	offset++
	if len(value)-offset < shortLength {
		return NASOperatorPLMNName{}, offset, errors.New("short name is truncated")
	}
	shortName := slices.Clone(value[offset : offset+shortLength])
	offset += shortLength

	return NASOperatorPLMNName{
		Long: NASEncodedNetworkName{
			Encoding:        encoding,
			CountryInitials: initials,
			SpareBits:       longSpare,
			Data:            longName,
		},
		Short: NASEncodedNetworkName{
			Encoding:        encoding,
			CountryInitials: initials,
			SpareBits:       shortSpare,
			Data:            shortName,
		},
	}, offset, nil
}
