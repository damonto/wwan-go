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
	maxRecordContentLength      = 255
	maxTransparentContentLength = 4096
)

// UIMSecurityAttributeLogic describes how a file's access conditions combine.
type UIMSecurityAttributeLogic uint8

const (
	UIMSecurityAlways UIMSecurityAttributeLogic = iota
	UIMSecurityNever
	UIMSecurityAnd
	UIMSecurityOr
	UIMSecuritySingle
)

// UIMSecurityAttribute is a mask of credentials required for file access.
type UIMSecurityAttribute uint16

const (
	UIMSecurityPIN1 UIMSecurityAttribute = 1 << iota
	UIMSecurityPIN2
	UIMSecurityUPIN
	UIMSecurityADM
)

// UIMFileSecurity contains one file-operation access rule.
type UIMFileSecurity struct {
	Logic      UIMSecurityAttributeLogic
	Attributes UIMSecurityAttribute
}

// UIMTransparentReadResult contains transparent file bytes and encryption state.
type UIMTransparentReadResult struct {
	Data           []byte
	Encrypted      bool
	EncryptedKnown bool
}

type RawFileAttributes struct {
	FileSize           uint16
	FileID             uint16
	FileType           QMIFileType
	RecordSize         uint16
	RecordCount        uint16
	ReadSecurity       UIMFileSecurity
	WriteSecurity      UIMFileSecurity
	IncreaseSecurity   UIMFileSecurity
	DeactivateSecurity UIMFileSecurity
	ActivateSecurity   UIMFileSecurity
	Raw                []byte
}

// MarshalBinary encodes raw QMI UIM file attributes.
func (r RawFileAttributes) MarshalBinary() ([]byte, error) {
	if len(r.Raw) > 0xffff {
		return nil, fmt.Errorf("file attributes raw length %d exceeds 65535", len(r.Raw))
	}
	value := binary.LittleEndian.AppendUint16(nil, r.FileSize)
	value = binary.LittleEndian.AppendUint16(value, r.FileID)
	value = append(value, byte(r.FileType))
	value = binary.LittleEndian.AppendUint16(value, r.RecordSize)
	value = binary.LittleEndian.AppendUint16(value, r.RecordCount)
	for _, security := range []UIMFileSecurity{
		r.ReadSecurity,
		r.WriteSecurity,
		r.IncreaseSecurity,
		r.DeactivateSecurity,
		r.ActivateSecurity,
	} {
		encoded, err := security.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding file attributes security: %w", err)
		}
		value = append(value, encoded...)
	}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(r.Raw)))
	return append(value, r.Raw...), nil
}

// UnmarshalBinary decodes raw QMI UIM file attributes.
func (r *RawFileAttributes) UnmarshalBinary(data []byte) error {
	if len(data) < 26 {
		return errors.New("reading file attributes: attributes payload is truncated")
	}

	parsed := RawFileAttributes{
		FileSize:    binary.LittleEndian.Uint16(data[:2]),
		FileID:      binary.LittleEndian.Uint16(data[2:4]),
		FileType:    QMIFileType(data[4]),
		RecordSize:  binary.LittleEndian.Uint16(data[5:7]),
		RecordCount: binary.LittleEndian.Uint16(data[7:9]),
	}
	security := []struct {
		value []byte
		dst   *UIMFileSecurity
	}{
		{data[9:12], &parsed.ReadSecurity},
		{data[12:15], &parsed.WriteSecurity},
		{data[15:18], &parsed.IncreaseSecurity},
		{data[18:21], &parsed.DeactivateSecurity},
		{data[21:24], &parsed.ActivateSecurity},
	}
	for _, field := range security {
		if err := field.dst.UnmarshalBinary(field.value); err != nil {
			return fmt.Errorf("reading file attributes security: %w", err)
		}
	}

	rawLength := int(binary.LittleEndian.Uint16(data[24:26]))
	if len(data) != 26+rawLength {
		return fmt.Errorf("reading file attributes: payload length %d, want %d", len(data), 26+rawLength)
	}

	parsed.Raw = slices.Clone(data[26 : 26+rawLength])
	*r = parsed
	return nil
}

func (security UIMFileSecurity) MarshalBinary() ([]byte, error) {
	value := []byte{byte(security.Logic)}
	return binary.LittleEndian.AppendUint16(value, uint16(security.Attributes)), nil
}

func (security *UIMFileSecurity) UnmarshalBinary(data []byte) error {
	if len(data) != 3 {
		return fmt.Errorf("file security length %d, want 3", len(data))
	}
	*security = UIMFileSecurity{
		Logic:      UIMSecurityAttributeLogic(data[0]),
		Attributes: UIMSecurityAttribute(binary.LittleEndian.Uint16(data[1:])),
	}
	return nil
}

func (c *Client) FileAttributes(ctx context.Context, file File) (FileAttributes, error) {
	return c.GetFileAttributes(ctx, file)
}

func (c *Client) GetFileAttributes(ctx context.Context, file File) (FileAttributes, error) {
	response, err := c.fileAttributesResponse(ctx, file)
	if err != nil {
		return FileAttributes{}, err
	}
	var attributes FileAttributes
	if err := attributes.UnmarshalTLVs(response.TLVs); err != nil {
		return FileAttributes{}, err
	}
	return attributes, nil
}

func (c *Client) ReadTransparent(ctx context.Context, req TransparentRead) ([]byte, error) {
	result, err := c.ReadTransparentResult(ctx, req)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// ReadTransparentResult reads a transparent file and reports whether the modem encrypted it.
func (c *Client) ReadTransparentResult(ctx context.Context, req TransparentRead) (UIMTransparentReadResult, error) {
	length := req.Length
	if length == 0 {
		attrs, err := c.FileAttributes(ctx, req.File)
		if err != nil {
			return UIMTransparentReadResult{}, err
		}
		if attrs.FileStructure != FileStructureTransparent {
			return UIMTransparentReadResult{}, errors.New("reading transparent file: unexpected file structure")
		}
		if req.Offset > attrs.FileSize {
			return UIMTransparentReadResult{}, errors.New("reading transparent file: offset exceeds file size")
		}
		length = attrs.FileSize - req.Offset
	}

	response, err := c.transparentResponse(ctx, req.File, req.Offset, length, req.EncryptData)
	if err != nil {
		return UIMTransparentReadResult{}, err
	}

	value, ok := tlv.Value(response.TLVs, 0x11)
	if !ok {
		return UIMTransparentReadResult{}, errors.New("reading transparent file: read result TLV missing")
	}
	var data qmiLength16Bytes
	if err := data.UnmarshalBinary(value); err != nil {
		return UIMTransparentReadResult{}, fmt.Errorf("reading transparent file: %w", err)
	}
	result := UIMTransparentReadResult{Data: data}
	if value, ok := tlv.Value(response.TLVs, 0x13); ok {
		if len(value) != 1 {
			return UIMTransparentReadResult{}, fmt.Errorf("reading transparent file: encrypted-data TLV length %d, want 1", len(value))
		}
		result.Encrypted = value[0] != 0
		result.EncryptedKnown = true
	}
	return result, nil
}

func (c *Client) ReadRecord(ctx context.Context, req RecordRead) ([]byte, error) {
	req.LastRecord = 0
	records, err := c.ReadRecords(ctx, req)
	if err != nil {
		return nil, err
	}
	return records[0], nil
}

// ReadRecords reads a contiguous range from a linear-fixed or cyclic file.
func (c *Client) ReadRecords(ctx context.Context, req RecordRead) ([][]byte, error) {
	if req.Record == 0 {
		return nil, errors.New("reading record file: record number is zero")
	}
	if req.LastRecord != 0 && req.LastRecord < req.Record {
		return nil, errors.New("reading record file: last record precedes first record")
	}

	length := req.Length
	if length == 0 {
		attrs, err := c.FileAttributes(ctx, req.File)
		if err != nil {
			return nil, err
		}
		if attrs.FileStructure != FileStructureLinearFixed {
			return nil, errors.New("reading record file: unexpected file structure")
		}
		length = attrs.RecordSize
	}

	response, err := c.recordResponse(ctx, req.File, req.Record, length, req.LastRecord)
	if err != nil {
		return nil, err
	}

	value, ok := tlv.Value(response.TLVs, 0x11)
	if !ok {
		return nil, errors.New("reading record file: read result TLV missing")
	}
	var first qmiLength16Bytes
	if err := first.UnmarshalBinary(value); err != nil {
		return nil, fmt.Errorf("reading record file: %w", err)
	}
	records := [][]byte{first}
	additionalValue, ok := tlv.Value(response.TLVs, 0x12)
	if !ok {
		if req.LastRecord != 0 && req.LastRecord != req.Record {
			return nil, errors.New("reading record file: additional read result TLV missing")
		}
		return records, nil
	}
	var additional qmiLength16Bytes
	if err := additional.UnmarshalBinary(additionalValue); err != nil {
		return nil, fmt.Errorf("reading record file: additional records: %w", err)
	}
	if len(first) == 0 || len(additional)%len(first) != 0 {
		return nil, errors.New("reading record file: additional data does not contain whole records")
	}
	for len(additional) > 0 {
		records = append(records, slices.Clone(additional[:len(first)]))
		additional = additional[len(first):]
	}
	if req.LastRecord != 0 && len(records) != int(req.LastRecord-req.Record)+1 {
		return nil, fmt.Errorf("reading record file: record count %d, want %d", len(records), int(req.LastRecord-req.Record)+1)
	}
	return records, nil
}

func (c *Client) WriteRecord(ctx context.Context, req RecordWrite) error {
	if req.Record == 0 {
		return errors.New("writing record file: record number is zero")
	}
	if len(req.Data) > maxRecordContentLength {
		return fmt.Errorf("writing record file: content length %d exceeds QMI UIM limit %d", len(req.Data), maxRecordContentLength)
	}

	fileValue, err := putFileValue(req.File.Path)
	if err != nil {
		return fmt.Errorf("writing record file: %w", err)
	}
	sessionValue, err := putSessionValue(req.File.Session, req.File.AID)
	if err != nil {
		return fmt.Errorf("writing record file: %w", err)
	}

	recordValue := binary.LittleEndian.AppendUint16(nil, req.Record)
	recordValue = binary.LittleEndian.AppendUint16(recordValue, uint16(len(req.Data)))
	recordValue = append(recordValue, req.Data...)

	resp, err := c.request(ctx, MessageWriteRecord, tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, recordValue),
	})
	if err != nil {
		return fmt.Errorf("writing record file: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("writing record file: %w", err)
	}
	if _, ok := tlv.Value(resp.TLVs, 0x11); ok {
		return errors.New("writing record file: response indication is not supported")
	}
	if err := cardError(resp.TLVs); err != nil {
		return fmt.Errorf("writing record file: %w", err)
	}
	return nil
}

// WriteTransparent writes bytes at an offset in a transparent elementary file.
func (c *Client) WriteTransparent(ctx context.Context, req TransparentWrite) error {
	if len(req.Data) == 0 {
		return errors.New("writing transparent file: content is empty")
	}
	if len(req.Data) > maxTransparentContentLength {
		return fmt.Errorf("writing transparent file: content length %d exceeds QMI UIM limit %d", len(req.Data), maxTransparentContentLength)
	}
	fileValue, err := putFileValue(req.File.Path)
	if err != nil {
		return fmt.Errorf("writing transparent file: %w", err)
	}
	sessionValue, err := putSessionValue(req.File.Session, req.File.AID)
	if err != nil {
		return fmt.Errorf("writing transparent file: %w", err)
	}

	writeValue := binary.LittleEndian.AppendUint16(nil, req.Offset)
	writeValue = binary.LittleEndian.AppendUint16(writeValue, uint16(len(req.Data)))
	writeValue = append(writeValue, req.Data...)
	resp, err := c.request(ctx, MessageWriteTransparent, tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, writeValue),
	})
	if err != nil {
		return fmt.Errorf("writing transparent file: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("writing transparent file: %w", err)
	}
	if _, ok := tlv.Value(resp.TLVs, 0x11); ok {
		return errors.New("writing transparent file: response indication is not supported")
	}
	if err := cardError(resp.TLVs); err != nil {
		return fmt.Errorf("writing transparent file: %w", err)
	}
	return nil
}

func (c *Client) transparentResponse(
	ctx context.Context,
	file File,
	offset uint16,
	length uint16,
	encryptData bool,
) (Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return Response{}, err
	}
	sessionValue, err := putSessionValue(file.Session, file.AID)
	if err != nil {
		return Response{}, err
	}

	info := joinBytes(
		binary.LittleEndian.AppendUint16(nil, offset),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, info),
	}
	if encryptData {
		tlvs = append(tlvs, tlv.Uint(0x11, uint8(1)))
	}
	resp, err := c.request(ctx, MessageReadTransparent, tlvs)
	if err != nil {
		return Response{}, err
	}
	if err := ResultError(resp.TLVs); err != nil {
		if errors.Is(err, QMIErrorInsufficientResources) {
			if _, ok := tlv.Value(resp.TLVs, 0x15); ok {
				return Response{}, errors.New("reading transparent file: long response is not supported")
			}
		}
		return Response{}, err
	}
	if _, ok := tlv.Value(resp.TLVs, 0x12); ok {
		return Response{}, errors.New("reading transparent file: response indication is not supported")
	}
	if err := cardError(resp.TLVs); err != nil {
		return Response{}, err
	}
	return resp, nil
}

func (c *Client) recordResponse(
	ctx context.Context,
	file File,
	record uint16,
	length uint16,
	lastRecord uint16,
) (Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return Response{}, err
	}
	sessionValue, err := putSessionValue(file.Session, file.AID)
	if err != nil {
		return Response{}, err
	}

	recordValue := joinBytes(
		binary.LittleEndian.AppendUint16(nil, record),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, recordValue),
	}
	if lastRecord != 0 {
		tlvs = append(tlvs, tlv.Uint(0x10, lastRecord))
	}
	resp, err := c.request(ctx, MessageReadRecord, tlvs)
	if err != nil {
		return Response{}, err
	}
	if err := ResultError(resp.TLVs); err != nil {
		return Response{}, err
	}
	if _, ok := tlv.Value(resp.TLVs, 0x13); ok {
		return Response{}, errors.New("reading record file: response indication is not supported")
	}
	if err := cardError(resp.TLVs); err != nil {
		return Response{}, err
	}
	return resp, nil
}

func (c *Client) fileAttributesResponse(
	ctx context.Context,
	file File,
) (Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return Response{}, err
	}
	sessionValue, err := putSessionValue(file.Session, file.AID)
	if err != nil {
		return Response{}, err
	}

	resp, err := c.request(ctx, MessageGetFileAttributes, tlv.TLVs{
		tlv.Bytes(0x01, sessionValue),
		tlv.Bytes(0x02, fileValue),
	})
	if err != nil {
		return Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return Response{}, err
	}
	if _, ok := tlv.Value(resp.TLVs, 0x12); ok {
		return Response{}, errors.New("reading file attributes: response indication is not supported")
	}
	return resp, nil
}

// UnmarshalTLVs parses a QMI UIM file-attributes response.
func (a *FileAttributes) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x11)
	if !ok {
		return errors.New("reading file attributes: attributes TLV missing")
	}

	var attrs RawFileAttributes
	if err := attrs.UnmarshalBinary(value); err != nil {
		return err
	}

	*a = FileAttributes{
		FileSize:           attrs.FileSize,
		FileID:             attrs.FileID,
		RecordSize:         attrs.RecordSize,
		RecordCount:        attrs.RecordCount,
		FileType:           fileTypeToSIMFileType(attrs.FileType),
		FileStructure:      fileTypeToSIMFileStructure(attrs.FileType),
		ReadSecurity:       attrs.ReadSecurity,
		WriteSecurity:      attrs.WriteSecurity,
		IncreaseSecurity:   attrs.IncreaseSecurity,
		DeactivateSecurity: attrs.DeactivateSecurity,
		ActivateSecurity:   attrs.ActivateSecurity,
		Raw:                slices.Clone(attrs.Raw),
	}
	return nil
}

func fileTypeToSIMFileStructure(fileType QMIFileType) FileStructure {
	switch fileType {
	case QMIFileTypeTransparent:
		return FileStructureTransparent
	case QMIFileTypeLinearFixed:
		return FileStructureLinearFixed
	default:
		return 0
	}
}

func fileTypeToSIMFileType(fileType QMIFileType) FileType {
	switch fileType {
	case QMIFileTypeTransparent, QMIFileTypeCyclic, QMIFileTypeLinearFixed:
		return FileTypeWorkingEF
	case QMIFileTypeDedicated, QMIFileTypeMaster:
		return FileTypeDFOrADF
	default:
		return FileType(fileType)
	}
}
