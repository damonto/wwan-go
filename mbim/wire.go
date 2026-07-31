package mbim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"unicode/utf16"
)

const (
	accessStringMaximumSize = 200
	userNameMaximumSize     = 510
	passwordMaximumSize     = 510
)

func appendOffsetSizeElements(header []byte, elements [][]byte) []byte {
	data := slices.Clone(header)
	tableOffset := len(data)
	data = append(data, make([]byte, len(elements)*8)...)

	for i, element := range elements {
		entryOffset := tableOffset + i*8
		if len(element) != 0 {
			binary.LittleEndian.PutUint32(data[entryOffset:entryOffset+4], uint32(len(data)))
			binary.LittleEndian.PutUint32(data[entryOffset+4:entryOffset+8], uint32(len(element)))
		}
		data = append(data, element...)
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
	}
	return data
}

func offsetSizeRefs(data []byte, tableOffset, count uint32) ([]valueRef, error) {
	if tableOffset > uint32(len(data)) || count > (uint32(len(data))-tableOffset)/8 {
		return nil, errors.New("reference table is truncated")
	}
	dataStart := tableOffset + count*8

	refs := make([]valueRef, count)
	for i := range count {
		ref, err := readOffsetSizeRef(data, tableOffset+i*8)
		if err != nil {
			return nil, fmt.Errorf("reference %d: %w", i, err)
		}
		refs[i] = ref
	}
	if err := validateDataBufferRefs(data, dataStart, refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r valueRef) validateDataBuffer(base []byte, dataStart uint32) error {
	if err := r.validate(base); err != nil {
		return err
	}
	if r.size != 0 && r.offset < dataStart {
		return errors.New("value points before data buffer")
	}
	if r.offset%4 != 0 {
		return errors.New("value offset is not 4-byte aligned")
	}
	return nil
}

func validateDataBufferRefs(base []byte, dataStart uint32, refs []valueRef) error {
	if uint64(dataStart) > uint64(len(base)) {
		return errors.New("data buffer starts beyond payload")
	}

	nextOffset := uint64(dataStart)
	for i, ref := range refs {
		if err := ref.validateDataBuffer(base, dataStart); err != nil {
			return fmt.Errorf("reference %d: %w", i, err)
		}
		if ref.size == 0 {
			continue
		}
		offset := uint64(ref.offset)
		if offset != nextOffset {
			if offset < nextOffset {
				return fmt.Errorf("reference %d overlaps or precedes an earlier value", i)
			}
			return fmt.Errorf("reference %d leaves a sparse data buffer", i)
		}
		nextOffset = offset + uint64(ref.size)
		if remainder := nextOffset % 4; remainder != 0 {
			nextOffset += 4 - remainder
		}
	}
	if nextOffset > uint64(len(base)) {
		return errors.New("data buffer padding is truncated")
	}
	if nextOffset < uint64(len(base)) {
		return errors.New("data buffer has trailing data")
	}
	return nil
}

func validateContextStringRefs(base []byte, dataStart uint32, refs []valueRef) error {
	maximumSizes := [...]uint32{
		accessStringMaximumSize,
		userNameMaximumSize,
		passwordMaximumSize,
	}
	for i, maximumSize := range maximumSizes {
		if refs[i].size > maximumSize {
			return fmt.Errorf("string %d size %d exceeds %d bytes", i, refs[i].size, maximumSize)
		}
	}
	if err := validateDataBufferRefs(base, dataStart, refs); err != nil {
		return err
	}
	return validateUTF16Refs(base, refs)
}

func appendRefValue(data []byte, fieldOffset int, value []byte) []byte {
	if len(value) == 0 {
		return data
	}
	binary.LittleEndian.PutUint32(data[fieldOffset:fieldOffset+4], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[fieldOffset+4:fieldOffset+8], uint32(len(value)))
	data = append(data, value...)
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	return data
}

func padTo4Bytes(data []byte) []byte {
	return append(data, make([]byte, align4(len(data))-len(data))...)
}

type valueRef struct {
	offset uint32
	size   uint32
}

func readOffsetSizeRef(data []byte, fieldOffset uint32) (valueRef, error) {
	if fieldOffset > uint32(len(data)) || 8 > uint32(len(data))-fieldOffset {
		return valueRef{}, errors.New("reference is truncated")
	}
	return valueRef{
		offset: binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4]),
		size:   binary.LittleEndian.Uint32(data[fieldOffset+4 : fieldOffset+8]),
	}, nil
}

func readSizeOffsetRef(data []byte, fieldOffset uint32) (valueRef, error) {
	if fieldOffset > uint32(len(data)) || 8 > uint32(len(data))-fieldOffset {
		return valueRef{}, errors.New("reference is truncated")
	}
	return valueRef{
		size:   binary.LittleEndian.Uint32(data[fieldOffset : fieldOffset+4]),
		offset: binary.LittleEndian.Uint32(data[fieldOffset+4 : fieldOffset+8]),
	}, nil
}

func (r valueRef) validate(base []byte) error {
	if r.offset == 0 {
		if r.size != 0 {
			return errors.New("reference has zero offset with nonzero size")
		}
		return nil
	}
	if r.offset > uint32(len(base)) || r.size > uint32(len(base))-r.offset {
		return errors.New("value is truncated")
	}
	return nil
}

func (r valueRef) bytes(base []byte) []byte {
	if r.offset == 0 && r.size == 0 {
		return nil
	}
	return slices.Clone(base[r.offset : r.offset+r.size])
}

func byteArrayRef(base, data []byte, fieldOffset, dataStart uint32) ([]byte, error) {
	ref, err := readOffsetSizeRef(data, fieldOffset)
	if err != nil {
		return nil, err
	}
	if err := validateDataBufferRefs(base, dataStart, []valueRef{ref}); err != nil {
		return nil, err
	}
	return ref.bytes(base), nil
}

func validateUTF16Refs(base []byte, refs []valueRef) error {
	var previousEnd uint32
	var sawString bool
	for _, ref := range refs {
		if err := ref.validate(base); err != nil {
			return err
		}
		if ref.size%2 != 0 {
			return errors.New("UTF-16 string has odd byte length")
		}
		if ref.offset == 0 {
			continue
		}
		if sawString && ref.offset < previousEnd {
			return errors.New("string buffers overlap or are out of order")
		}
		previousEnd = ref.offset + ref.size
		sawString = true
	}
	return nil
}

func utf16String(data []byte, ref valueRef) (string, error) {
	if err := ref.validate(data); err != nil {
		return "", err
	}
	if ref.size == 0 {
		return "", nil
	}
	raw := data[ref.offset : ref.offset+ref.size]
	return utf16RawString(raw)
}

func utf16RawString(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", errors.New("UTF-16 string has odd byte length")
	}

	encoded := make([]uint16, len(raw)/2)
	for i := range encoded {
		encoded[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(encoded)), nil
}

func utf16Bytes(s string) []byte {
	encoded := utf16.Encode([]rune(s))
	buf := make([]byte, 0, len(encoded)*2)
	for _, v := range encoded {
		buf = binary.LittleEndian.AppendUint16(buf, v)
	}
	return buf
}
