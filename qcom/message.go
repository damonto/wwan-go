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
	uimAIDMaxLength  = 32
	uimPathMaxLength = 10
)

const (
	qmiTLVResult     = 0x02
	qmiTLVCardResult = 0x10

	envelopeCommandSMSPP = 9
	// QMI CAT raw envelope buffers are capped by the modem CAT service IDL.
	catRawEnvelopeMaxLength      = 258
	catTerminalResponseMaxLength = 255
)

type tlvUnmarshaler interface {
	UnmarshalTLVs(tlv.TLVs) error
}

type tlvUnmarshalerPointer[T any] interface {
	*T
	tlvUnmarshaler
}

func unmarshalTLVStream[T any, P tlvUnmarshalerPointer[T]](ctx context.Context, input <-chan tlv.TLVs) <-chan T {
	output := make(chan T, 8)
	go func() {
		defer close(output)
		for values := range input {
			var value T
			if err := P(&value).UnmarshalTLVs(values); err != nil {
				return
			}
			select {
			case output <- value:
			case <-ctx.Done():
				return
			}
		}
	}()
	return output
}

func resultOK(resp Response) error {
	return ResultError(resp.TLVs)
}

func cardResultOK(resp Response) error {
	if err := ResultError(resp.TLVs); err != nil {
		return err
	}
	return cardError(resp.TLVs)
}

type qmiLength8Bytes []byte

func (value qmiLength8Bytes) MarshalBinary() ([]byte, error) {
	if len(value) > 0xff {
		return nil, fmt.Errorf("value length %d exceeds 255", len(value))
	}
	data := make([]byte, 1, 1+len(value))
	data[0] = byte(len(value))
	return append(data, value...), nil
}

func (value *qmiLength8Bytes) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return errors.New("length prefix is truncated")
	}
	length := int(data[0])
	if len(data) != 1+length {
		return fmt.Errorf("value length %d, want %d", len(data), 1+length)
	}
	*value = slices.Clone(data[1:])
	return nil
}

type qmiLength16Bytes []byte

func (value qmiLength16Bytes) MarshalBinary() ([]byte, error) {
	if len(value) > 0xffff {
		return nil, fmt.Errorf("value length %d exceeds 65535", len(value))
	}
	data := binary.LittleEndian.AppendUint16(nil, uint16(len(value)))
	return append(data, value...), nil
}

func (value *qmiLength16Bytes) UnmarshalBinary(data []byte) error {
	if len(data) < 2 {
		return errors.New("length prefix is truncated")
	}
	length := int(binary.LittleEndian.Uint16(data[:2]))
	if len(data) != 2+length {
		return fmt.Errorf("value length %d, want %d", len(data), 2+length)
	}
	*value = slices.Clone(data[2:])
	return nil
}

func putSessionValue(session Session, aid []byte) ([]byte, error) {
	if err := validateUIMAIDLength(aid); err != nil {
		return nil, err
	}

	value := make([]byte, 0, 2+len(aid))
	value = append(value, byte(session), byte(len(aid)))
	value = append(value, aid...)
	return value, nil
}

func putFileValue(path []byte) ([]byte, error) {
	fileID, filePath, err := splitFilePath(path)
	if err != nil {
		return nil, err
	}
	if len(filePath) > uimPathMaxLength {
		return nil, fmt.Errorf("encoding SIM path %X: QMI path length %d exceeds %d", path, len(filePath), uimPathMaxLength)
	}

	value := binary.LittleEndian.AppendUint16(nil, fileID)
	value = append(value, byte(len(filePath)))
	value = append(value, filePath...)
	return value, nil
}

func validateUIMAIDLength(aid []byte) error {
	if len(aid) > uimAIDMaxLength {
		return fmt.Errorf("AID length %d exceeds %d", len(aid), uimAIDMaxLength)
	}
	return nil
}

func splitFilePath(path []byte) (uint16, []byte, error) {
	if len(path) < 2 || len(path)%2 != 0 {
		return 0, nil, fmt.Errorf("encoding SIM path %X: path length must be an even number of bytes", path)
	}

	fileID := binary.BigEndian.Uint16(path[len(path)-2:])
	filePath := make([]byte, 0, len(path)-2)
	for i := 0; i < len(path)-2; i += 2 {
		filePath = append(filePath, path[i+1], path[i])
	}
	return fileID, filePath, nil
}

func joinBytes(parts ...[]byte) []byte {
	total := 0
	for _, part := range parts {
		total += len(part)
	}

	buf := make([]byte, 0, total)
	for _, part := range parts {
		buf = append(buf, part...)
	}
	return buf
}

func cardError(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, qmiTLVCardResult)
	if !ok {
		return nil
	}
	if len(value) != 2 {
		return fmt.Errorf("parsing QMI card result: TLV length %d, want 2", len(value))
	}

	statusWord := uint16(value[0])<<8 | uint16(value[1])
	if statusWord == 0x9000 {
		return nil
	}
	return cardStatusError(statusWord)
}

type cardStatusError uint16

func (e cardStatusError) Error() string {
	return fmt.Sprintf("unexpected status word 0x%04X", uint16(e))
}

// ServiceVersion identifies one QMI service version advertised by the device.
type ServiceVersion struct {
	Service ServiceType
	Major   uint16
	Minor   uint16
}

type serviceVersion = ServiceVersion

// ServiceVersionList contains the QMI services advertised by the device.
type ServiceVersionList []ServiceVersion

// UnmarshalTLVs parses a QMI control Get Version Info response.
func (versions *ServiceVersionList) UnmarshalTLVs(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, 0x01)
	if !ok {
		return errors.New("reading QMI service versions: service list TLV missing")
	}
	if len(value) == 0 {
		return errors.New("reading QMI service versions: service count is missing")
	}
	count := int(value[0])
	value = value[1:]
	if len(value) < count*5 {
		return errors.New("reading QMI service versions: service list is truncated")
	}

	decoded := make(ServiceVersionList, 0, count)
	for i := range count {
		offset := i * 5
		decoded = append(decoded, serviceVersion{
			Service: ServiceType(value[offset]),
			Major:   binary.LittleEndian.Uint16(value[offset+1 : offset+3]),
			Minor:   binary.LittleEndian.Uint16(value[offset+3 : offset+5]),
		})
	}
	*versions = decoded
	return nil
}
