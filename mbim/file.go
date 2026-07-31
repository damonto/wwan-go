package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

const (
	uiccFileApplicationIDMaximumSize = 16
	uiccFilePathMaximumSize          = 8
	uiccFileResponseMaximumSize      = 32768
	uiccRecordNumberMaximum          = 256
)

type FileStructure byte

const (
	FileStructureTransparent FileStructure = 0x41
	FileStructureLinearFixed FileStructure = 0x42
)

type FileType byte

const (
	FileTypeWorkingEF FileType = 0x21
	FileTypeDFOrADF   FileType = 0x38
)

type Application struct {
	AID   []byte
	Label string
}

type FileRef struct {
	AID  []byte
	Path []byte
}

type FileAttributes struct {
	FileStructure FileStructure
	FileType      FileType
	RecordSize    uint16
	RecordCount   uint16
	FileSize      uint16
}

type TransparentRead struct {
	File     FileRef
	Offset   uint16
	Length   uint16
	LocalPIN string
	Data     []byte
}

type RecordRead struct {
	File     FileRef
	Record   uint16
	LocalPIN string
	Data     []byte
}

var (
	masterFilePath               = []byte{0x3F, 0x00}
	applicationDedicatedFilePath = []byte{0x7F, 0xFF}
)

func (c *Client) FileAttributes(ctx context.Context, file FileRef) (FileAttributes, error) {
	file, err := normalizeUICCFile(file)
	if err != nil {
		return FileAttributes{}, fmt.Errorf("reading MBIM file attributes: %w", err)
	}

	request := FileStatusRequest{
		TransactionID: c.nextTransactionID(),
		ApplicationID: file.AID,
		FilePath:      file.Path,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return FileAttributes{}, fmt.Errorf("reading MBIM file attributes %X: %w", file.Path, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return FileAttributes{}, fmt.Errorf("reading MBIM file attributes %X: %w", file.Path, err)
	}
	return fileStatusAttributes(request.Response), nil
}

func (c *Client) ReadTransparent(ctx context.Context, req TransparentRead) ([]byte, error) {
	file, err := normalizeUICCFile(req.File)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM transparent file: %w", err)
	}
	req.File = file

	length := req.Length
	if length == 0 {
		attrs, err := c.FileAttributes(ctx, file)
		if err != nil {
			return nil, err
		}
		if attrs.FileStructure != FileStructureTransparent {
			return nil, errors.New("reading MBIM transparent file: unexpected file structure")
		}
		if req.Offset > attrs.FileSize {
			return nil, errors.New("reading MBIM transparent file: offset exceeds file size")
		}
		length = attrs.FileSize - req.Offset
	}
	if length == 0 {
		return nil, nil
	}
	if uint32(length) > uiccFileResponseMaximumSize {
		return nil, fmt.Errorf(
			"reading MBIM transparent file: length %d exceeds %d bytes",
			length,
			uiccFileResponseMaximumSize,
		)
	}

	request := ReadBinaryRequest{
		TransactionID: c.nextTransactionID(),
		ApplicationID: file.AID,
		FilePath:      file.Path,
		Offset:        uint32(req.Offset),
		Size:          uint32(length),
		LocalPIN:      req.LocalPIN,
		Data:          slices.Clone(req.Data),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM transparent file %X: %w", req.File.Path, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return nil, fmt.Errorf("reading MBIM transparent file %X: %w", req.File.Path, err)
	}
	return slices.Clone(request.Response.Data), nil
}

func (c *Client) ReadRecord(ctx context.Context, req RecordRead) ([]byte, error) {
	file, err := normalizeUICCFile(req.File)
	if err != nil {
		return nil, fmt.Errorf("reading MBIM record file: %w", err)
	}
	if req.Record == 0 {
		return nil, errors.New("reading MBIM record file: record number is zero")
	}
	if uint32(req.Record) > uiccRecordNumberMaximum {
		return nil, fmt.Errorf(
			"reading MBIM record file: record number %d exceeds %d",
			req.Record,
			uiccRecordNumberMaximum,
		)
	}

	attrs, err := c.FileAttributes(ctx, file)
	if err != nil {
		return nil, err
	}
	if attrs.FileStructure != FileStructureLinearFixed {
		return nil, errors.New("reading MBIM record file: unexpected file structure")
	}

	request := ReadRecordRequest{
		TransactionID: c.nextTransactionID(),
		ApplicationID: file.AID,
		FilePath:      file.Path,
		Record:        uint32(req.Record),
		LocalPIN:      req.LocalPIN,
		Data:          slices.Clone(req.Data),
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return nil, fmt.Errorf("reading MBIM record file %X record %d: %w", req.File.Path, req.Record, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return nil, fmt.Errorf("reading MBIM record file %X record %d: %w", req.File.Path, req.Record, err)
	}
	return slices.Clone(request.Response.Data), nil
}

func filePath(file FileRef) []byte {
	if hasPrefix(file.Path, masterFilePath) || hasPrefix(file.Path, applicationDedicatedFilePath) {
		return slices.Clone(file.Path)
	}
	prefix := masterFilePath
	if len(file.AID) != 0 {
		prefix = applicationDedicatedFilePath
	}
	return append(slices.Clone(prefix), file.Path...)
}

func normalizeUICCFile(file FileRef) (FileRef, error) {
	if len(file.Path) == 0 {
		return FileRef{}, errors.New("path is empty")
	}
	if len(file.AID) > uiccFileApplicationIDMaximumSize {
		return FileRef{}, fmt.Errorf(
			"application ID length %d exceeds %d bytes",
			len(file.AID),
			uiccFileApplicationIDMaximumSize,
		)
	}

	path := filePath(file)
	if len(path) > uiccFilePathMaximumSize {
		return FileRef{}, fmt.Errorf("path length %d exceeds %d bytes", len(path), uiccFilePathMaximumSize)
	}
	if len(path)%2 != 0 {
		return FileRef{}, fmt.Errorf("path length %d is not a sequence of 16-bit file IDs", len(path))
	}
	if hasPrefix(path, applicationDedicatedFilePath) && len(file.AID) == 0 {
		return FileRef{}, errors.New("ADF-relative path requires an application ID")
	}
	if !hasPrefix(path, masterFilePath) && !hasPrefix(path, applicationDedicatedFilePath) {
		return FileRef{}, errors.New("path must start with file ID 3F00 or 7FFF")
	}
	return FileRef{AID: slices.Clone(file.AID), Path: path}, nil
}

func fileStatusAttributes(status *FileStatusResponse) FileAttributes {
	attrs := FileAttributes{
		FileStructure: fileStructure(status.FileStructure),
		FileType:      fileType(status.FileType),
		RecordSize:    uint16(status.FileItemSize),
		RecordCount:   uint16(status.FileItemCount),
	}
	if status.FileStructure == UiccFileStructureTransparent {
		attrs.FileSize = uint16(status.FileItemSize)
	} else {
		attrs.FileSize = uint16(status.FileItemCount * status.FileItemSize)
	}
	return attrs
}

func fileStructure(structure UiccFileStructure) FileStructure {
	switch structure {
	case UiccFileStructureTransparent:
		return FileStructureTransparent
	case UiccFileStructureLinear, UiccFileStructureCyclic:
		return FileStructureLinearFixed
	default:
		return 0
	}
}

func fileType(fileType UiccFileType) FileType {
	switch fileType {
	case UiccFileTypeWorkingEF, UiccFileTypeInternalEF:
		return FileTypeWorkingEF
	case UiccFileTypeDFOrADF:
		return FileTypeDFOrADF
	default:
		return FileType(fileType)
	}
}

func hasPrefix(data, prefix []byte) bool {
	return len(data) >= len(prefix) && slices.Equal(data[:len(prefix)], prefix)
}

type FileStatusRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Response      *FileStatusResponse
}

func (r *FileStatusRequest) Request() *Request {
	data := refHeader(20, r.ApplicationID, r.FilePath)
	data = appendRefs(data, 20, r.ApplicationID, r.FilePath)

	r.Response = new(FileStatusResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccFileStatus,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type FileStatusResponse struct {
	Version                   uint32
	StatusWord1               uint32
	StatusWord2               uint32
	FileAccessibility         UiccFileAccessibility
	FileType                  UiccFileType
	FileStructure             UiccFileStructure
	FileItemCount             uint32
	FileItemSize              uint32
	AccessConditionRead       PinType
	AccessConditionUpdate     PinType
	AccessConditionActivate   PinType
	AccessConditionDeactivate PinType
}

func (r *FileStatusResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 48 {
		return fmt.Errorf("parsing MBIM file status: payload length is %d, want 48", len(data))
	}
	version := binary.LittleEndian.Uint32(data[:4])
	if version != 1 {
		return fmt.Errorf("parsing MBIM file status: version is %d, want 1", version)
	}
	fileAccessibility := UiccFileAccessibility(binary.LittleEndian.Uint32(data[12:16]))
	if fileAccessibility > UiccFileAccessibilityShareable {
		return fmt.Errorf("parsing MBIM file status: file accessibility is %d, want 0..%d", fileAccessibility, UiccFileAccessibilityShareable)
	}
	fileType := UiccFileType(binary.LittleEndian.Uint32(data[16:20]))
	if fileType > UiccFileTypeDFOrADF {
		return fmt.Errorf("parsing MBIM file status: file type is %d, want 0..%d", fileType, UiccFileTypeDFOrADF)
	}
	fileStructure := UiccFileStructure(binary.LittleEndian.Uint32(data[20:24]))
	if fileStructure > UiccFileStructureBERTLV {
		return fmt.Errorf("parsing MBIM file status: file structure is %d, want 0..%d", fileStructure, UiccFileStructureBERTLV)
	}
	fileItemCount := binary.LittleEndian.Uint32(data[24:28])
	if (fileStructure == UiccFileStructureTransparent || fileStructure == UiccFileStructureBERTLV) && fileItemCount != 1 {
		return fmt.Errorf("parsing MBIM file status: item count is %d, want 1 for file structure %d", fileItemCount, fileStructure)
	}
	accessConditions := [...]PinType{
		PinType(binary.LittleEndian.Uint32(data[32:36])),
		PinType(binary.LittleEndian.Uint32(data[36:40])),
		PinType(binary.LittleEndian.Uint32(data[40:44])),
		PinType(binary.LittleEndian.Uint32(data[44:48])),
	}
	for i, condition := range accessConditions {
		if condition > PinTypeADM {
			return fmt.Errorf("parsing MBIM file status: access condition %d is %d, want 0..%d", i, condition, PinTypeADM)
		}
	}

	*r = FileStatusResponse{
		Version:                   version,
		StatusWord1:               binary.LittleEndian.Uint32(data[4:8]),
		StatusWord2:               binary.LittleEndian.Uint32(data[8:12]),
		FileAccessibility:         fileAccessibility,
		FileType:                  fileType,
		FileStructure:             fileStructure,
		FileItemCount:             fileItemCount,
		FileItemSize:              binary.LittleEndian.Uint32(data[28:32]),
		AccessConditionRead:       accessConditions[0],
		AccessConditionUpdate:     accessConditions[1],
		AccessConditionActivate:   accessConditions[2],
		AccessConditionDeactivate: accessConditions[3],
	}
	return nil
}

type ReadBinaryRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Offset        uint32
	Size          uint32
	LocalPIN      string
	Data          []byte
	Response      *ReadBinaryResponse
}

func (r *ReadBinaryRequest) Request() *Request {
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	binary.LittleEndian.PutUint32(data[20:24], r.Offset)
	binary.LittleEndian.PutUint32(data[24:28], r.Size)
	data = appendRefValue(data, 4, r.ApplicationID)
	data = appendRefValue(data, 12, r.FilePath)
	data = appendRefValue(data, 28, utf16Bytes(r.LocalPIN))
	data = appendRefValue(data, 36, r.Data)

	r.Response = new(ReadBinaryResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReadBinary,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ReadBinaryResponse struct {
	Version     uint32
	StatusWord1 uint32
	StatusWord2 uint32
	Data        []byte
}

func (r *ReadBinaryResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM read binary: payload is truncated")
	}
	version := binary.LittleEndian.Uint32(data[:4])
	if version != 1 {
		return fmt.Errorf("parsing MBIM read binary: version is %d, want 1", version)
	}
	value, err := byteArrayRef(data, data, 12, 20)
	if err != nil {
		return fmt.Errorf("parsing MBIM read binary data: %w", err)
	}
	if len(value) > uiccFileResponseMaximumSize {
		return fmt.Errorf("parsing MBIM read binary: response size %d exceeds %d bytes", len(value), uiccFileResponseMaximumSize)
	}
	*r = ReadBinaryResponse{
		Version:     version,
		StatusWord1: binary.LittleEndian.Uint32(data[4:8]),
		StatusWord2: binary.LittleEndian.Uint32(data[8:12]),
		Data:        value,
	}
	return nil
}

type ReadRecordRequest struct {
	TransactionID uint32
	ApplicationID []byte
	FilePath      []byte
	Record        uint32
	LocalPIN      string
	Data          []byte
	Response      *ReadRecordResponse
}

func (r *ReadRecordRequest) Request() *Request {
	data := make([]byte, 40)
	binary.LittleEndian.PutUint32(data[0:4], 1)
	binary.LittleEndian.PutUint32(data[20:24], r.Record)
	data = appendRefValue(data, 4, r.ApplicationID)
	data = appendRefValue(data, 12, r.FilePath)
	data = appendRefValue(data, 24, utf16Bytes(r.LocalPIN))
	data = appendRefValue(data, 32, r.Data)

	r.Response = new(ReadRecordResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Command: command(
			ServiceMsUiccLowLevelAccess,
			CIDUiccReadRecord,
			CommandTypeQuery,
			data,
		),
		Response: r.Response,
	}
}

type ReadRecordResponse struct {
	Version     uint32
	StatusWord1 uint32
	StatusWord2 uint32
	Data        []byte
}

func (r *ReadRecordResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 20 {
		return errors.New("parsing MBIM read record: payload is truncated")
	}
	version := binary.LittleEndian.Uint32(data[:4])
	if version != 1 {
		return fmt.Errorf("parsing MBIM read record: version is %d, want 1", version)
	}
	value, err := byteArrayRef(data, data, 12, 20)
	if err != nil {
		return fmt.Errorf("parsing MBIM read record data: %w", err)
	}
	if len(value) > uiccFileResponseMaximumSize {
		return fmt.Errorf("parsing MBIM read record: response size %d exceeds %d bytes", len(value), uiccFileResponseMaximumSize)
	}
	*r = ReadRecordResponse{
		Version:     version,
		StatusWord1: binary.LittleEndian.Uint32(data[4:8]),
		StatusWord2: binary.LittleEndian.Uint32(data[8:12]),
		Data:        value,
	}
	return nil
}

func refHeader(baseOffset int, refs ...[]byte) []byte {
	data := binary.LittleEndian.AppendUint32(nil, 1)
	offset := baseOffset
	for _, ref := range refs {
		data = binary.LittleEndian.AppendUint32(data, uint32(offset))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(ref)))
		offset = align4(offset + len(ref))
	}
	return data
}

func appendRefs(data []byte, baseOffset int, refs ...[]byte) []byte {
	for _, ref := range refs {
		data = append(data, ref...)
		for (baseOffset+len(ref))%4 != 0 {
			data = append(data, 0)
			baseOffset++
		}
		baseOffset += len(ref)
	}
	return data
}
