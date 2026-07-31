package sms

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"github.com/damonto/wwan-go/modem/contract"
)

type Message = contract.Message
type MessageConfig = contract.MessageConfig

const (
	smsMaxUserDataOctets = 140
	smsGSM7SingleSeptets = 160
	smsGSM7PartSeptets   = 153
	smsUCS2SingleUnits   = 70
	smsUCS2PartUnits     = 67
	smsBinaryPartOctets  = 134
)

var smsConcatReference atomic.Uint32

type smsAlphabet uint8

const (
	smsAlphabetGSM7 smsAlphabet = iota
	smsAlphabetBinary
	smsAlphabetUCS2
)

type Part struct {
	Message   Message
	Reference uint16
	Total     uint8
	Index     uint8
}

func EncodePDUs(cfg MessageConfig) ([][]byte, error) {
	if cfg.Text != "" {
		if _, ok := EncodeGSM7(cfg.Text); ok {
			return encodeGSM7PDUs(cfg)
		}
		return encodeUCS2PDUs(cfg)
	}
	return encodeBinaryPDUs(cfg)
}

func encodeGSM7PDUs(cfg MessageConfig) ([][]byte, error) {
	septets, _ := EncodeGSM7(cfg.Text)
	if len(septets) <= smsGSM7SingleSeptets {
		return [][]byte{buildSubmitPDU(cfg, smsAlphabetGSM7, septets, nil)}, nil
	}
	chunks, err := splitGSM7(cfg.Text, smsGSM7PartSeptets)
	if err != nil {
		return nil, err
	}
	return buildMultipartPDUs(cfg, smsAlphabetGSM7, chunks), nil
}

func encodeUCS2PDUs(cfg MessageConfig) ([][]byte, error) {
	units := utf16.Encode([]rune(cfg.Text))
	if len(units) <= smsUCS2SingleUnits {
		return [][]byte{buildSubmitPDU(cfg, smsAlphabetUCS2, UCS2Bytes(units), nil)}, nil
	}
	unitChunks := splitUTF16(cfg.Text, smsUCS2PartUnits)
	chunks := make([][]byte, len(unitChunks))
	for i, chunk := range unitChunks {
		chunks[i] = UCS2Bytes(chunk)
	}
	return buildMultipartPDUs(cfg, smsAlphabetUCS2, chunks), nil
}

func encodeBinaryPDUs(cfg MessageConfig) ([][]byte, error) {
	if len(cfg.Data) <= smsMaxUserDataOctets {
		return [][]byte{buildSubmitPDU(cfg, smsAlphabetBinary, slices.Clone(cfg.Data), nil)}, nil
	}
	chunks := make([][]byte, 0, (len(cfg.Data)+smsBinaryPartOctets-1)/smsBinaryPartOctets)
	for data := cfg.Data; len(data) > 0; {
		length := min(len(data), smsBinaryPartOctets)
		chunks = append(chunks, slices.Clone(data[:length]))
		data = data[length:]
	}
	return buildMultipartPDUs(cfg, smsAlphabetBinary, chunks), nil
}

func buildMultipartPDUs(cfg MessageConfig, alphabet smsAlphabet, chunks [][]byte) [][]byte {
	ref := uint8(smsConcatReference.Add(1))
	if ref == 0 {
		ref = uint8(smsConcatReference.Add(1))
	}
	result := make([][]byte, len(chunks))
	for i, chunk := range chunks {
		header := []byte{5, 0, 3, ref, byte(len(chunks)), byte(i + 1)}
		result[i] = buildSubmitPDU(cfg, alphabet, chunk, header)
	}
	return result
}

func buildSubmitPDU(cfg MessageConfig, alphabet smsAlphabet, data, header []byte) []byte {
	pdu := encodeSMSC(cfg.SMSC)
	first := byte(0x01)
	if cfg.DeliveryReport {
		first |= 0x20
	}
	if len(header) != 0 {
		first |= 0x40
	}
	pdu = append(pdu, first, 0)
	pdu = appendAddress(pdu, cfg.Number)
	pdu = append(pdu, 0)
	switch alphabet {
	case smsAlphabetBinary:
		pdu = append(pdu, 0x04)
	case smsAlphabetUCS2:
		pdu = append(pdu, 0x08)
	default:
		pdu = append(pdu, 0)
	}
	if alphabet == smsAlphabetGSM7 {
		packed, headerSeptets := PackSeptets(data, header)
		pdu = append(pdu, byte(headerSeptets+len(data)))
		return append(pdu, packed...)
	}
	userData := append(slices.Clone(header), data...)
	pdu = append(pdu, byte(len(userData)))
	return append(pdu, userData...)
}

func DecodePDU(pdu []byte) (Part, error) {
	if len(pdu) < 2 {
		return Part{}, errors.New("decoding SMS PDU: PDU is truncated")
	}
	offset := 0
	smscLength := int(pdu[offset])
	offset++
	if smscLength > len(pdu)-offset {
		return Part{}, errors.New("decoding SMS PDU: SMSC address is truncated")
	}
	smsc := ""
	if smscLength > 0 {
		if smscLength < 1 {
			return Part{}, errors.New("decoding SMS PDU: SMSC type is missing")
		}
		smsc = decodeBCDAddress(pdu[offset], pdu[offset+1:offset+smscLength], (smscLength-1)*2)
	}
	offset += smscLength
	if offset >= len(pdu) {
		return Part{}, errors.New("decoding SMS PDU: TPDU is missing")
	}
	first := pdu[offset]
	offset++
	part := Part{Message: Message{SMSC: smsc, PDU: slices.Clone(pdu), PDUs: [][]byte{slices.Clone(pdu)}}}
	switch first & 0x03 {
	case 0:
		return decodeDeliverPDU(pdu, offset, first, part)
	case 1:
		return decodeSubmitPDU(pdu, offset, first, part)
	case 2:
		return decodeStatusReportPDU(pdu, offset, part)
	default:
		return Part{}, fmt.Errorf("decoding SMS PDU: message type %#x is reserved", first&0x03)
	}
}

func decodeDeliverPDU(pdu []byte, offset int, first byte, part Part) (Part, error) {
	number, next, err := readAddress(pdu, offset)
	if err != nil {
		return Part{}, err
	}
	part.Message.Number = number
	offset = next
	if len(pdu)-offset < 10 {
		return Part{}, errors.New("decoding SMS-DELIVER: fixed fields are truncated")
	}
	offset++
	dcs := pdu[offset]
	offset++
	part.Message.Timestamp = decodeSMSTimestamp(pdu[offset : offset+7])
	offset += 7
	return decodeUserData(pdu, offset, first&0x40 != 0, dcs, part)
}

func decodeSubmitPDU(pdu []byte, offset int, first byte, part Part) (Part, error) {
	if offset >= len(pdu) {
		return Part{}, errors.New("decoding SMS-SUBMIT: message reference is missing")
	}
	part.Message.MessageReference = uint32(pdu[offset])
	offset++
	number, next, err := readAddress(pdu, offset)
	if err != nil {
		return Part{}, err
	}
	part.Message.Number = number
	offset = next
	if len(pdu)-offset < 2 {
		return Part{}, errors.New("decoding SMS-SUBMIT: PID or DCS is missing")
	}
	offset++
	dcs := pdu[offset]
	offset++
	switch (first >> 3) & 0x03 {
	case 1:
		offset += 7
	case 2:
		offset++
	case 3:
		offset += 7
	}
	if offset > len(pdu) {
		return Part{}, errors.New("decoding SMS-SUBMIT: validity period is truncated")
	}
	part.Message.DeliveryReport = first&0x20 != 0
	return decodeUserData(pdu, offset, first&0x40 != 0, dcs, part)
}

func decodeStatusReportPDU(pdu []byte, offset int, part Part) (Part, error) {
	if offset >= len(pdu) {
		return Part{}, errors.New("decoding SMS-STATUS-REPORT: message reference is missing")
	}
	part.Message.MessageReference = uint32(pdu[offset])
	part.Message.DeliveryReport = true
	offset++
	number, next, err := readAddress(pdu, offset)
	if err != nil {
		return Part{}, err
	}
	part.Message.Number = number
	offset = next
	if len(pdu)-offset < 15 {
		return Part{}, errors.New("decoding SMS-STATUS-REPORT: timestamps or status are truncated")
	}
	part.Message.Timestamp = decodeSMSTimestamp(pdu[offset : offset+7])
	return part, nil
}

func decodeUserData(pdu []byte, offset int, hasHeader bool, dcs byte, part Part) (Part, error) {
	if offset >= len(pdu) {
		return Part{}, errors.New("decoding SMS PDU: user data length is missing")
	}
	length := int(pdu[offset])
	offset++
	alphabet := alphabetFromDCS(dcs)
	if alphabet == smsAlphabetGSM7 {
		octets := (length*7 + 7) / 8
		if octets > len(pdu)-offset {
			return Part{}, errors.New("decoding SMS PDU: GSM7 user data is truncated")
		}
		data := pdu[offset : offset+octets]
		headerSeptets := 0
		if hasHeader {
			var err error
			headerSeptets, err = parseUserDataHeader(data, &part)
			if err != nil {
				return Part{}, err
			}
		}
		if headerSeptets > length {
			return Part{}, errors.New("decoding SMS PDU: UDH exceeds GSM7 user data length")
		}
		septets := UnpackSeptets(data, headerSeptets*7, length-headerSeptets)
		text, err := DecodeGSM7(septets)
		if err != nil {
			return Part{}, err
		}
		part.Message.Text = text
		return part, nil
	}
	if length > len(pdu)-offset {
		return Part{}, errors.New("decoding SMS PDU: user data is truncated")
	}
	data := pdu[offset : offset+length]
	headerLength := 0
	if hasHeader {
		var err error
		headerLength, err = parseUserDataHeaderOctets(data, &part)
		if err != nil {
			return Part{}, err
		}
	}
	payload := data[headerLength:]
	if alphabet == smsAlphabetBinary {
		part.Message.Data = slices.Clone(payload)
		return part, nil
	}
	if len(payload)%2 != 0 {
		return Part{}, errors.New("decoding SMS PDU: UCS-2 user data has odd length")
	}
	units := make([]uint16, len(payload)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(payload[i*2:])
	}
	part.Message.Text = string(utf16.Decode(units))
	return part, nil
}

func parseUserDataHeader(data []byte, part *Part) (int, error) {
	headerLength, err := parseUserDataHeaderOctets(data, part)
	if err != nil {
		return 0, err
	}
	return (headerLength*8 + 6) / 7, nil
}

func parseUserDataHeaderOctets(data []byte, part *Part) (int, error) {
	if len(data) == 0 {
		return 0, errors.New("decoding SMS PDU: UDH length is missing")
	}
	headerLength := int(data[0]) + 1
	if headerLength > len(data) {
		return 0, errors.New("decoding SMS PDU: UDH is truncated")
	}
	for offset := 1; offset < headerLength; {
		if headerLength-offset < 2 {
			return 0, errors.New("decoding SMS PDU: UDH information element is truncated")
		}
		kind, length := data[offset], int(data[offset+1])
		offset += 2
		if length > headerLength-offset {
			return 0, errors.New("decoding SMS PDU: UDH information element data is truncated")
		}
		value := data[offset : offset+length]
		switch {
		case kind == 0 && length == 3:
			part.Reference, part.Total, part.Index = uint16(value[0]), value[1], value[2]
		case kind == 8 && length == 4:
			part.Reference, part.Total, part.Index = binary.BigEndian.Uint16(value[:2]), value[2], value[3]
		}
		offset += length
	}
	if part.Total != 0 && (part.Index == 0 || part.Index > part.Total) {
		return 0, fmt.Errorf("decoding SMS PDU: multipart index %d is outside 1..%d", part.Index, part.Total)
	}
	return headerLength, nil
}

func alphabetFromDCS(dcs byte) smsAlphabet {
	switch dcs & 0x0c {
	case 0x04:
		return smsAlphabetBinary
	case 0x08:
		return smsAlphabetUCS2
	default:
		return smsAlphabetGSM7
	}
}

func appendAddress(dst []byte, number string) []byte {
	digits, international := normalizeNumber(number)
	dst = append(dst, byte(len(digits)))
	typeOfAddress := byte(0x81)
	if international {
		typeOfAddress = 0x91
	}
	dst = append(dst, typeOfAddress)
	return append(dst, encodeBCDDigits(digits)...)
}

func readAddress(pdu []byte, offset int) (string, int, error) {
	if len(pdu)-offset < 2 {
		return "", offset, errors.New("decoding SMS PDU: address is truncated")
	}
	digitCount := int(pdu[offset])
	typeOfAddress := pdu[offset+1]
	offset += 2
	if typeOfAddress&0x70 == 0x50 {
		// 3GPP TS 23.040 expresses an alphanumeric address length in
		// useful semi-octets, not in GSM 7-bit characters.
		octets := (digitCount + 1) / 2
		if octets > len(pdu)-offset {
			return "", offset, errors.New("decoding SMS PDU: alphanumeric address is truncated")
		}
		septets := digitCount * 4 / 7
		text, err := DecodeGSM7(UnpackSeptets(pdu[offset:offset+octets], 0, septets))
		if err != nil {
			return "", offset, fmt.Errorf("decoding SMS PDU address: %w", err)
		}
		return text, offset + octets, nil
	}
	octets := (digitCount + 1) / 2
	if octets > len(pdu)-offset {
		return "", offset, errors.New("decoding SMS PDU: address digits are truncated")
	}
	address := decodeBCDAddress(typeOfAddress, pdu[offset:offset+octets], digitCount)
	return address, offset + octets, nil
}

func encodeSMSC(number string) []byte {
	if number == "" {
		return []byte{0}
	}
	digits, international := normalizeNumber(number)
	typeOfAddress := byte(0x81)
	if international {
		typeOfAddress = 0x91
	}
	address := append([]byte{typeOfAddress}, encodeBCDDigits(digits)...)
	return append([]byte{byte(len(address))}, address...)
}

func normalizeNumber(number string) (string, bool) {
	international := strings.HasPrefix(number, "+")
	return strings.TrimPrefix(number, "+"), international
}

func encodeBCDDigits(digits string) []byte {
	result := make([]byte, (len(digits)+1)/2)
	for i := range result {
		low := bcdNibble(digits[i*2])
		high := byte(0x0f)
		if i*2+1 < len(digits) {
			high = bcdNibble(digits[i*2+1])
		}
		result[i] = low | high<<4
	}
	return result
}

func bcdNibble(value byte) byte {
	if value >= '0' && value <= '9' {
		return value - '0'
	}
	switch value {
	case '*':
		return 0x0a
	case '#':
		return 0x0b
	case 'a', 'A':
		return 0x0c
	case 'b', 'B':
		return 0x0d
	case 'c', 'C':
		return 0x0e
	default:
		return 0x0f
	}
}

func decodeBCDAddress(typeOfAddress byte, data []byte, digitCount int) string {
	var result strings.Builder
	if typeOfAddress&0x70 == 0x10 {
		result.WriteByte('+')
	}
	for i := 0; i < digitCount; i++ {
		nibble := data[i/2] >> (4 * (i % 2)) & 0x0f
		if nibble <= 9 {
			result.WriteByte('0' + nibble)
		}
	}
	return result.String()
}

func PackSeptets(septets, header []byte) ([]byte, int) {
	headerSeptets := (len(header)*8 + 6) / 7
	startBit := headerSeptets * 7
	result := make([]byte, (startBit+len(septets)*7+7)/8)
	copy(result, header)
	for i, septet := range septets {
		putBits(result, startBit+i*7, septet&0x7f, 7)
	}
	return result, headerSeptets
}

func UnpackSeptets(data []byte, startBit, count int) []byte {
	result := make([]byte, count)
	for i := range count {
		result[i] = getBits(data, startBit+i*7, 7)
	}
	return result
}

func putBits(dst []byte, offset int, value byte, count int) {
	for bit := range count {
		if value&(1<<bit) != 0 {
			dst[(offset+bit)/8] |= 1 << ((offset + bit) % 8)
		}
	}
}

func getBits(src []byte, offset, count int) byte {
	var value byte
	for bit := range count {
		if src[(offset+bit)/8]&(1<<((offset+bit)%8)) != 0 {
			value |= 1 << bit
		}
	}
	return value
}

func splitGSM7(text string, capacity int) ([][]byte, error) {
	var chunks [][]byte
	current := make([]byte, 0, capacity)
	for _, r := range text {
		encoded, ok := encodeGSM7Rune(r)
		if !ok {
			return nil, fmt.Errorf("encoding GSM7: character %q is not representable", r)
		}
		if len(current)+len(encoded) > capacity {
			chunks = append(chunks, current)
			current = make([]byte, 0, capacity)
		}
		current = append(current, encoded...)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks, nil
}

func splitUTF16(text string, capacity int) [][]uint16 {
	var chunks [][]uint16
	current := make([]uint16, 0, capacity)
	for _, r := range text {
		units := utf16.Encode([]rune{r})
		if len(current)+len(units) > capacity {
			chunks = append(chunks, current)
			current = make([]uint16, 0, capacity)
		}
		current = append(current, units...)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func UCS2Bytes(units []uint16) []byte {
	result := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.BigEndian.PutUint16(result[i*2:], unit)
	}
	return result
}

func decodeSMSTimestamp(value []byte) time.Time {
	if len(value) != 7 {
		return time.Time{}
	}
	fields := [7]int{}
	for i, octet := range value {
		fields[i] = int(octet&0x0f)*10 + int(octet>>4&0x07)
	}
	year := 2000 + fields[0]
	if fields[0] >= 90 {
		year = 1900 + fields[0]
	}
	offset := fields[6] * 15 * 60
	if value[6]&0x08 != 0 {
		offset = -offset
	}
	return time.Date(year, time.Month(fields[1]), fields[2], fields[3], fields[4], fields[5], 0, time.FixedZone("SMS", offset))
}

func EncodeGSM7(text string) ([]byte, bool) {
	result := make([]byte, 0, len(text))
	for _, r := range text {
		encoded, ok := encodeGSM7Rune(r)
		if !ok {
			return nil, false
		}
		result = append(result, encoded...)
	}
	return result, true
}

func encodeGSM7Rune(r rune) ([]byte, bool) {
	if value, ok := gsm7DefaultEncode[r]; ok {
		return []byte{value}, true
	}
	if value, ok := gsm7ExtensionEncode[r]; ok {
		return []byte{0x1b, value}, true
	}
	return nil, false
}

func DecodeGSM7(septets []byte) (string, error) {
	var result strings.Builder
	for i := 0; i < len(septets); i++ {
		value := septets[i]
		if value == 0x1b {
			i++
			if i >= len(septets) {
				return "", errors.New("decoding GSM7: trailing escape septet")
			}
			r, ok := gsm7ExtensionDecode[septets[i]]
			if !ok {
				return "", fmt.Errorf("decoding GSM7: unknown extension %#x", septets[i])
			}
			result.WriteRune(r)
			continue
		}
		result.WriteRune(gsm7DefaultDecode[value&0x7f])
	}
	return result.String(), nil
}

var gsm7DefaultDecode = [128]rune{
	'@', '£', '$', '¥', 'è', 'é', 'ù', 'ì', 'ò', 'Ç', '\n', 'Ø', 'ø', '\r', 'Å', 'å',
	'Δ', '_', 'Φ', 'Γ', 'Λ', 'Ω', 'Π', 'Ψ', 'Σ', 'Θ', 'Ξ', 0, 'Æ', 'æ', 'ß', 'É',
	' ', '!', '"', '#', '¤', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ':', ';', '<', '=', '>', '?',
	'¡', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', 'Ä', 'Ö', 'Ñ', 'Ü', '§',
	'¿', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', 'ä', 'ö', 'ñ', 'ü', 'à',
}

var gsm7DefaultEncode = func() map[rune]byte {
	result := make(map[rune]byte, len(gsm7DefaultDecode))
	for value, r := range gsm7DefaultDecode {
		if r != 0 {
			result[r] = byte(value)
		}
	}
	return result
}()

var gsm7ExtensionDecode = map[byte]rune{
	0x0a: '\f', 0x14: '^', 0x28: '{', 0x29: '}', 0x2f: '\\',
	0x3c: '[', 0x3d: '~', 0x3e: ']', 0x40: '|', 0x65: '€',
}

var gsm7ExtensionEncode = func() map[rune]byte {
	result := make(map[rune]byte, len(gsm7ExtensionDecode))
	for value, r := range gsm7ExtensionDecode {
		result[r] = value
	}
	return result
}()

type Assembler struct {
	parts map[smsAssemblyKey]map[uint8]Part
}

type smsAssemblyKey struct {
	number string
	ref    uint16
	total  uint8
}

func (a *Assembler) Add(part Part) (Message, bool) {
	if part.Total <= 1 {
		return CloneMessage(part.Message), true
	}
	if a.parts == nil {
		a.parts = make(map[smsAssemblyKey]map[uint8]Part)
	}
	key := smsAssemblyKey{number: part.Message.Number, ref: part.Reference, total: part.Total}
	parts := a.parts[key]
	if parts == nil {
		parts = make(map[uint8]Part, part.Total)
		a.parts[key] = parts
	}
	parts[part.Index] = part
	if len(parts) != int(part.Total) {
		return Message{}, false
	}
	result := CloneMessage(parts[1].Message)
	result.Text = ""
	result.Data = nil
	result.PDU = nil
	result.PDUs = make([][]byte, 0, part.Total)
	result.Refs = nil
	for index := uint8(1); index <= part.Total; index++ {
		current, ok := parts[index]
		if !ok {
			return Message{}, false
		}
		result.Text += current.Message.Text
		result.Data = append(result.Data, current.Message.Data...)
		result.PDUs = append(result.PDUs, slices.Clone(current.Message.PDU))
		result.Refs = append(result.Refs, current.Message.Refs...)
	}
	result.PDU = slices.Clone(result.PDUs[0])
	delete(a.parts, key)
	return result, true
}

// Assemble combines complete multipart groups and returns single-part messages
// unchanged. Incomplete groups are omitted.
func Assemble(parts []Part) []Message {
	assembler := Assembler{}
	result := make([]Message, 0, len(parts))
	for _, part := range parts {
		message, complete := assembler.Add(part)
		if complete {
			result = append(result, message)
		}
	}
	return slices.Clip(result)
}

func CloneMessage(message Message) Message {
	message.Refs = slices.Clone(message.Refs)
	message.Data = slices.Clone(message.Data)
	message.PDU = slices.Clone(message.PDU)
	message.PDUs = make([][]byte, len(message.PDUs))
	for i, pdu := range message.PDUs {
		message.PDUs[i] = slices.Clone(pdu)
	}
	return message
}

// DecodeUCS2 decodes a big-endian UCS-2 payload.
func DecodeUCS2(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("UCS-2 payload has odd byte length")
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(units)), nil
}
