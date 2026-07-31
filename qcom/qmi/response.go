package qmi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

var (
	errUnexpectedControlMessageType = errors.New("unexpected control QMI message type")
	errUnexpectedServiceMessageType = errors.New("unexpected service QMI message type")
)

const (
	controlMessageTypeResponse   qcom.MessageType = 0x01
	controlMessageTypeIndication qcom.MessageType = 0x02
)

type Response struct {
	QMUXHeader
	TransactionID uint16
	MessageID     qcom.MessageID
	MessageType   qcom.MessageType
	MessageLength uint16
	TLVs          tlv.TLVs
}

func (r Response) qcomResponse() qcom.Response {
	return qcom.Response{
		Service:       qcom.ServiceType(r.ServiceType),
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     r.MessageID,
		TLVs:          r.TLVs,
	}
}

func (r Response) qcomIndication() qcom.Indication {
	return qcom.Indication{
		Service:       qcom.ServiceType(r.ServiceType),
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     r.MessageID,
		TLVs:          r.TLVs,
	}
}

func (r Response) isResponse() bool {
	if qcom.ServiceType(r.QMUXHeader.ServiceType) == qcom.ServiceControl {
		return r.MessageType == controlMessageTypeResponse
	}
	return r.MessageType == qcom.MessageTypeResponse
}

func (r Response) isIndication() bool {
	if qcom.ServiceType(r.QMUXHeader.ServiceType) == qcom.ServiceControl {
		return r.MessageType == controlMessageTypeIndication
	}
	return r.MessageType == qcom.MessageTypeIndication
}

func (r *Response) UnmarshalBinary(data []byte) error {
	*r = Response{}
	if len(data) < 12 {
		return fmt.Errorf("parsing QMI message: data too short: got %d bytes", len(data))
	}

	reader := bytes.NewReader(data)
	if err := binary.Read(reader, binary.LittleEndian, &r.QMUXHeader); err != nil {
		return fmt.Errorf("parsing QMI message: read QMUX header: %w", err)
	}
	if r.QMUXHeader.IfType != qcom.QMUXIfType {
		return fmt.Errorf("parsing QMI message: unexpected QMUX marker 0x%02X", r.QMUXHeader.IfType)
	}
	if got, want := len(data), int(r.QMUXHeader.Length)+1; got != want {
		return fmt.Errorf("parsing QMI message: QMUX length mismatch: got %d bytes, header declares %d", got, want)
	}

	if qcom.ServiceType(r.QMUXHeader.ServiceType) == qcom.ServiceControl {
		var header Header[uint8]
		if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
			return fmt.Errorf("parsing QMI message: read control QMI header: %w", err)
		}
		r.MessageType = header.MessageType
		r.TransactionID = uint16(header.TransactionID)
		r.MessageID = header.MessageID
		r.MessageLength = header.MessageLength
	} else {
		var header Header[uint16]
		if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
			return fmt.Errorf("parsing QMI message: read service QMI header: %w", err)
		}
		r.MessageType = header.MessageType
		r.TransactionID = header.TransactionID
		r.MessageID = header.MessageID
		r.MessageLength = header.MessageLength
	}

	if qcom.ServiceType(r.QMUXHeader.ServiceType) == qcom.ServiceControl {
		if !r.isResponse() && !r.isIndication() {
			return fmt.Errorf("parsing QMI message: %w: 0x%02X", errUnexpectedControlMessageType, r.MessageType)
		}
	} else if r.MessageType != qcom.MessageTypeResponse && r.MessageType != qcom.MessageTypeIndication {
		return fmt.Errorf("parsing QMI message: %w: 0x%02X", errUnexpectedServiceMessageType, r.MessageType)
	}
	if got, want := reader.Len(), int(r.MessageLength); got != want {
		return fmt.Errorf("parsing QMI message: QMI TLV length mismatch: got %d bytes, header declares %d", got, want)
	}
	if r.MessageLength > 0 {
		if err := r.TLVs.UnmarshalBinary(data[len(data)-int(r.MessageLength):]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Response) ReadFrom(reader io.Reader) (int64, error) {
	frame, read, err := readFrame(reader)
	if err != nil {
		return read, err
	}
	return read, r.UnmarshalBinary(frame)
}

func ReadFrame(r io.Reader) ([]byte, error) {
	frame, _, err := readFrame(r)
	return frame, err
}

func readFrame(r io.Reader) ([]byte, int64, error) {
	header := make([]byte, 3)
	n, err := io.ReadFull(r, header)
	read := int64(n)
	if err != nil {
		return nil, read, err
	}

	length := int(binary.LittleEndian.Uint16(header[1:3])) + 1
	if length < len(header) {
		return nil, read, errors.New("reading QMI frame: invalid length")
	}

	frame := make([]byte, length)
	copy(frame, header)
	n, err = io.ReadFull(r, frame[len(header):])
	read += int64(n)
	if err != nil {
		return nil, read, err
	}
	return frame, read, nil
}
