package qcom

import (
	"encoding/binary"
	"fmt"
	"io"
)

const dataEndpointWireLength = 8

// AppendBinary appends the QMI data_ep_id_type_v01 representation to dst.
func (e DataEndpoint) AppendBinary(dst []byte) ([]byte, error) {
	dst = binary.LittleEndian.AppendUint32(dst, uint32(e.Type))
	dst = binary.LittleEndian.AppendUint32(dst, e.InterfaceID)
	return dst, nil
}

// MarshalBinary returns the QMI data_ep_id_type_v01 representation.
func (e DataEndpoint) MarshalBinary() ([]byte, error) {
	return e.AppendBinary(nil)
}

// UnmarshalBinary decodes the QMI data_ep_id_type_v01 representation.
func (e *DataEndpoint) UnmarshalBinary(data []byte) error {
	if len(data) != dataEndpointWireLength {
		return fmt.Errorf("parsing QMI data endpoint: length %d, want %d", len(data), dataEndpointWireLength)
	}
	e.Type = DataEndpointType(binary.LittleEndian.Uint32(data[:4]))
	e.InterfaceID = binary.LittleEndian.Uint32(data[4:])
	return nil
}

// WriteTo writes the QMI data_ep_id_type_v01 representation.
func (e DataEndpoint) WriteTo(w io.Writer) (int64, error) {
	data, err := e.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

// ReadFrom reads the fixed-width QMI data_ep_id_type_v01 representation.
func (e *DataEndpoint) ReadFrom(r io.Reader) (int64, error) {
	data := make([]byte, dataEndpointWireLength)
	n, err := io.ReadFull(r, data)
	if err != nil {
		return int64(n), err
	}
	return int64(n), e.UnmarshalBinary(data)
}
