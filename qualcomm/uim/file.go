package uim

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/damonto/uicc-go/qualcomm"
	"github.com/damonto/uicc-go/qualcomm/tlv"
)

func (r *Reader) transparentResponse(
	ctx context.Context,
	file File,
	offset uint16,
	length uint16,
) (qualcomm.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qualcomm.Response{}, err
	}

	info := joinBytes(
		binary.LittleEndian.AppendUint16(nil, offset),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	resp, err := r.request(ctx, qualcomm.QMIServiceUIM, r.clientID, qualcomm.QMIUIMReadTransparent, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, info),
	})
	if err != nil {
		return qualcomm.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qualcomm.Response{}, err
	}
	return resp, nil
}

func (r *Reader) recordResponse(
	ctx context.Context,
	file File,
	record uint16,
	length uint16,
) (qualcomm.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qualcomm.Response{}, err
	}

	recordValue := joinBytes(
		binary.LittleEndian.AppendUint16(nil, record),
		binary.LittleEndian.AppendUint16(nil, length),
	)
	resp, err := r.request(ctx, qualcomm.QMIServiceUIM, r.clientID, qualcomm.QMIUIMReadRecord, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
		tlv.Bytes(0x03, recordValue),
	})
	if err != nil {
		return qualcomm.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qualcomm.Response{}, err
	}
	return resp, nil
}

func (r *Reader) fileAttributesResponse(
	ctx context.Context,
	file File,
) (qualcomm.Response, error) {
	fileValue, err := putFileValue(file.Path)
	if err != nil {
		return qualcomm.Response{}, err
	}

	resp, err := r.request(ctx, qualcomm.QMIServiceUIM, r.clientID, qualcomm.QMIUIMGetFileAttributes, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(file.Session, file.AID)),
		tlv.Bytes(0x02, fileValue),
	})
	if err != nil {
		return qualcomm.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qualcomm.Response{}, err
	}
	return resp, nil
}

func (r *Reader) authenticateResponse(
	ctx context.Context,
	req AuthenticateRequest,
) (qualcomm.Response, error) {
	value, err := req.MarshalBinary()
	if err != nil {
		return qualcomm.Response{}, err
	}

	resp, err := r.request(ctx, qualcomm.QMIServiceUIM, r.clientID, qualcomm.QMIUIMAuthenticate, tlv.TLVs{
		tlv.Bytes(0x01, putSessionValue(req.Session, req.AID)),
		tlv.Bytes(0x02, value),
	})
	if err != nil {
		return qualcomm.Response{}, err
	}
	if err := cardResultOK(resp); err != nil {
		return qualcomm.Response{}, err
	}
	return resp, nil
}

func decodeReaderFileAttributes(resp qualcomm.Response) (FileAttributes, error) {
	value, ok := tlv.Value(resp.TLVs, 0x11)
	if !ok {
		return FileAttributes{}, errors.New("reading file attributes: attributes TLV missing")
	}

	attrs, err := decodeFileAttributes(value)
	if err != nil {
		return FileAttributes{}, err
	}

	return FileAttributes{
		FileSize:      attrs.FileSize,
		RecordSize:    attrs.RecordSize,
		RecordCount:   attrs.RecordCount,
		FileType:      fileTypeToSIMFileType(attrs.FileType),
		FileStructure: fileTypeToSIMFileStructure(attrs.FileType),
	}, nil
}

func fileTypeToSIMFileStructure(fileType byte) byte {
	switch fileType {
	case 0:
		return FileStructureTransparent
	case 2:
		return FileStructureLinearFixed
	default:
		return 0
	}
}

func fileTypeToSIMFileType(fileType byte) byte {
	switch fileType {
	case 0, 1, 2:
		return 0x21
	case 3, 4:
		return 0x38
	default:
		return fileType
	}
}
