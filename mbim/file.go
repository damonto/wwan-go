package mbim

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

func (r *Reader) FileAttributes(ctx context.Context, file FileRef) (FileAttributes, error) {
	if len(file.Path) == 0 {
		return FileAttributes{}, errors.New("reading MBIM file attributes: path is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return FileAttributes{}, errors.New("reading MBIM file attributes: reader is closed")
	}

	request := FileStatusRequest{
		TransactionID: r.nextTransactionID(),
		ApplicationID: slices.Clone(file.AID),
		FilePath:      filePath(file),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return FileAttributes{}, fmt.Errorf("reading MBIM file attributes %X: %w", file.Path, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return FileAttributes{}, fmt.Errorf("reading MBIM file attributes %X: %w", file.Path, err)
	}
	return fileStatusAttributes(request.Response), nil
}

func (r *Reader) ReadTransparent(ctx context.Context, req TransparentRead) ([]byte, error) {
	if len(req.File.Path) == 0 {
		return nil, errors.New("reading MBIM transparent file: path is empty")
	}

	length := req.Length
	if length == 0 {
		attrs, err := r.FileAttributes(ctx, req.File)
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

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("reading MBIM transparent file: reader is closed")
	}

	request := ReadBinaryRequest{
		TransactionID: r.nextTransactionID(),
		ApplicationID: slices.Clone(req.File.AID),
		FilePath:      filePath(req.File),
		Offset:        uint32(req.Offset),
		Size:          uint32(length),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("reading MBIM transparent file %X: %w", req.File.Path, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return nil, fmt.Errorf("reading MBIM transparent file %X: %w", req.File.Path, err)
	}
	return slices.Clone(request.Response.Data), nil
}

func (r *Reader) ReadRecord(ctx context.Context, req RecordRead) ([]byte, error) {
	if len(req.File.Path) == 0 {
		return nil, errors.New("reading MBIM record file: path is empty")
	}
	if req.Record == 0 {
		return nil, errors.New("reading MBIM record file: record number is zero")
	}

	attrs, err := r.FileAttributes(ctx, req.File)
	if err != nil {
		return nil, err
	}
	if attrs.FileStructure != FileStructureLinearFixed {
		return nil, errors.New("reading MBIM record file: unexpected file structure")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("reading MBIM record file: reader is closed")
	}

	request := ReadRecordRequest{
		TransactionID: r.nextTransactionID(),
		ApplicationID: slices.Clone(req.File.AID),
		FilePath:      filePath(req.File),
		Record:        uint32(req.Record),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("reading MBIM record file %X record %d: %w", req.File.Path, req.Record, err)
	}
	if err := cardStatusError(request.Response.StatusWord1, request.Response.StatusWord2); err != nil {
		return nil, fmt.Errorf("reading MBIM record file %X record %d: %w", req.File.Path, req.Record, err)
	}
	return slices.Clone(request.Response.Data), nil
}

func filePath(file FileRef) []byte {
	if len(file.AID) != 0 || hasPrefix(file.Path, masterFilePath) {
		return slices.Clone(file.Path)
	}
	return append(slices.Clone(masterFilePath), file.Path...)
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
