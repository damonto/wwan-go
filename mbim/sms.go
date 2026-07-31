package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

type SMSStorageState uint32

const (
	SMSStorageStateNotInitialized SMSStorageState = 0
	SMSStorageStateInitialized    SMSStorageState = 1
)

type SMSFormat uint32

const (
	SMSFormatPDU  SMSFormat = 0
	SMSFormatCDMA SMSFormat = 1
)

type SMSReadFlag uint32

const (
	SMSReadFlagAll   SMSReadFlag = 0
	SMSReadFlagIndex SMSReadFlag = 1
	SMSReadFlagNew   SMSReadFlag = 2
	SMSReadFlagOld   SMSReadFlag = 3
	SMSReadFlagSent  SMSReadFlag = 4
	SMSReadFlagDraft SMSReadFlag = 5
)

type SMSStatus uint32

const (
	SMSStatusNew   SMSStatus = 0
	SMSStatusOld   SMSStatus = 1
	SMSStatusDraft SMSStatus = 2
	SMSStatusSent  SMSStatus = 3
)

type SMSStatusFlags uint32

const (
	SMSStatusFlagNone       SMSStatusFlags = 0
	SMSStatusFlagStoreFull  SMSStatusFlags = 1 << 0
	SMSStatusFlagNewMessage SMSStatusFlags = 1 << 1
)

type SMSCDMALanguage uint32

const (
	SMSCDMALanguageUnknown SMSCDMALanguage = iota
	SMSCDMALanguageEnglish
	SMSCDMALanguageFrench
	SMSCDMALanguageSpanish
	SMSCDMALanguageJapanese
	SMSCDMALanguageKorean
	SMSCDMALanguageChinese
	SMSCDMALanguageHebrew
)

type SMSCDMAEncoding uint32

const (
	SMSCDMAEncodingOctet SMSCDMAEncoding = iota
	SMSCDMAEncodingEPM
	SMSCDMAEncoding7BitASCII
	SMSCDMAEncodingIA5
	SMSCDMAEncodingUnicode
	SMSCDMAEncodingShiftJIS
	SMSCDMAEncodingKorean
	SMSCDMAEncodingLatinHebrew
	SMSCDMAEncodingLatin
	SMSCDMAEncodingGSM7Bit
)

type SMSPDURecord struct {
	MessageIndex  uint32
	MessageStatus SMSStatus
	PDU           []byte
}

type SMSCDMARecord struct {
	MessageIndex     uint32
	MessageStatus    SMSStatus
	Address          string
	Timestamp        string
	Encoding         SMSCDMAEncoding
	Language         SMSCDMALanguage
	EncodedMessage   []byte
	SizeInCharacters uint32
}

type SMSReadInfo struct {
	Format      SMSFormat
	PDURecords  []SMSPDURecord
	CDMARecords []SMSCDMARecord
}

type SMSConfigurationInfo struct {
	Format               SMSFormat
	StorageState         SMSStorageState
	MaxMessages          uint32
	CdmaShortMessageSize uint32
	SCAddress            string
}

type SMSSendPDU struct {
	PDU []byte
}

type SMSSendCDMA struct {
	Encoding         SMSCDMAEncoding
	Language         SMSCDMALanguage
	Address          string
	EncodedMessage   []byte
	SizeInCharacters uint32
}

type SMSSendInfo struct {
	MessageReference uint32
}

type SMSStoreStatusInfo struct {
	Flags        SMSStatusFlags
	MessageIndex uint32
}

const smsCDMATimestampLength = 20

func validSMSFormat(format SMSFormat) bool {
	return format <= SMSFormatCDMA
}

func validSMSReadFlag(flag SMSReadFlag) bool {
	return flag <= SMSReadFlagDraft
}

func validSMSStatus(status SMSStatus) bool {
	return status <= SMSStatusSent
}

func validSMSCDMAEncoding(encoding SMSCDMAEncoding) bool {
	return encoding <= SMSCDMAEncodingGSM7Bit
}

func validSMSCDMALanguage(language SMSCDMALanguage) bool {
	return language <= SMSCDMALanguageHebrew
}

func validateSMSFilter(flag SMSReadFlag, messageIndex uint32) error {
	if !validSMSReadFlag(flag) {
		return fmt.Errorf("flag %d is outside 0..%d", flag, SMSReadFlagDraft)
	}
	if flag == SMSReadFlagIndex {
		if messageIndex == 0 {
			return errors.New("message index must be non-zero when the index flag is used")
		}
		return nil
	}
	if messageIndex != 0 {
		return fmt.Errorf("message index must be zero when flag %d is used", flag)
	}
	return nil
}

type SMSConfigurationRequest struct {
	TransactionID uint32
	Response      *SMSConfigurationInfo
}

func (r *SMSConfigurationRequest) Request() *Request {
	r.Response = new(SMSConfigurationInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSConfiguration, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

type SMSConfigurationSetRequest struct {
	TransactionID uint32
	Format        SMSFormat
	SCAddress     string
	Response      *SMSConfigurationInfo
}

func (r *SMSConfigurationSetRequest) Request() *Request {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], uint32(r.Format))
	data = appendRefValue(data, 4, utf16Bytes(r.SCAddress))
	r.Response = new(SMSConfigurationInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSConfiguration, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func (r *SMSConfigurationInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 24 {
		return errors.New("parsing MBIM SMS configuration: payload is truncated")
	}
	storageState := SMSStorageState(binary.LittleEndian.Uint32(data[0:4]))
	if storageState > SMSStorageStateInitialized {
		return fmt.Errorf("parsing MBIM SMS configuration: storage state %d is outside 0..%d", storageState, SMSStorageStateInitialized)
	}
	if storageState == SMSStorageStateNotInitialized {
		*r = SMSConfigurationInfo{StorageState: storageState}
		return nil
	}
	format := SMSFormat(binary.LittleEndian.Uint32(data[4:8]))
	if !validSMSFormat(format) {
		return fmt.Errorf("parsing MBIM SMS configuration: format %d is outside 0..%d", format, SMSFormatCDMA)
	}
	addressRef, err := readOffsetSizeRef(data, 16)
	if err != nil {
		return fmt.Errorf("parsing MBIM SMS service center address: %w", err)
	}
	if err := validateDataBufferRefs(data, 24, []valueRef{addressRef}); err != nil {
		return fmt.Errorf("parsing MBIM SMS service center address: %w", err)
	}
	if addressRef.size > 40 {
		return fmt.Errorf("parsing MBIM SMS service center address: size %d exceeds 40 bytes", addressRef.size)
	}
	if err := validateUTF16Refs(data, []valueRef{addressRef}); err != nil {
		return fmt.Errorf("parsing MBIM SMS service center address: %w", err)
	}
	address, err := utf16String(data, addressRef)
	if err != nil {
		return fmt.Errorf("parsing MBIM SMS service center address: %w", err)
	}
	*r = SMSConfigurationInfo{
		StorageState:         storageState,
		Format:               format,
		MaxMessages:          binary.LittleEndian.Uint32(data[8:12]),
		CdmaShortMessageSize: binary.LittleEndian.Uint32(data[12:16]),
		SCAddress:            address,
	}
	return nil
}

type SMSReadRequest struct {
	TransactionID uint32
	Format        SMSFormat
	Flag          SMSReadFlag
	MessageIndex  uint32
	Response      *SMSReadInfo
}

func (r *SMSReadRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Format))
	data = binary.LittleEndian.AppendUint32(data, uint32(r.Flag))
	data = binary.LittleEndian.AppendUint32(data, r.MessageIndex)
	r.Response = new(SMSReadInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSRead, CommandTypeQuery, data),
		Response:      r.Response,
	}
}

func (r *SMSReadInfo) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return errors.New("parsing MBIM SMS messages: payload is truncated")
	}
	format := SMSFormat(binary.LittleEndian.Uint32(data[0:4]))
	count := binary.LittleEndian.Uint32(data[4:8])
	refs, err := offsetSizeRefs(data, 8, count)
	if err != nil {
		return fmt.Errorf("parsing MBIM SMS messages: %w", err)
	}

	*r = SMSReadInfo{Format: format}
	switch format {
	case SMSFormatPDU:
		r.PDURecords = make([]SMSPDURecord, count)
		for i, ref := range refs {
			if err := r.PDURecords[i].UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
				return fmt.Errorf("parsing MBIM SMS PDU record %d: %w", i, err)
			}
		}
	case SMSFormatCDMA:
		r.CDMARecords = make([]SMSCDMARecord, count)
		for i, ref := range refs {
			if err := r.CDMARecords[i].UnmarshalBinary(data[ref.offset : ref.offset+ref.size]); err != nil {
				return fmt.Errorf("parsing MBIM SMS CDMA record %d: %w", i, err)
			}
		}
	default:
		return fmt.Errorf("parsing MBIM SMS messages: unsupported format %d", format)
	}
	return nil
}

func (r *SMSPDURecord) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
		return errors.New("payload is truncated")
	}
	pduRef, err := readOffsetSizeRef(data, 8)
	if err != nil {
		return err
	}
	if err := validateDataBufferRefs(data, 16, []valueRef{pduRef}); err != nil {
		return err
	}
	if pduRef.size > 255 {
		return fmt.Errorf("PDU size %d exceeds 255 bytes", pduRef.size)
	}
	status := SMSStatus(binary.LittleEndian.Uint32(data[4:8]))
	if !validSMSStatus(status) {
		return fmt.Errorf("message status %d is outside 0..%d", status, SMSStatusSent)
	}
	*r = SMSPDURecord{
		MessageIndex:  binary.LittleEndian.Uint32(data[0:4]),
		MessageStatus: status,
		PDU:           pduRef.bytes(data),
	}
	return nil
}

func (r *SMSCDMARecord) UnmarshalBinary(data []byte) error {
	if len(data) < 44 {
		return errors.New("payload is truncated")
	}
	status := SMSStatus(binary.LittleEndian.Uint32(data[4:8]))
	if !validSMSStatus(status) {
		return fmt.Errorf("message status %d is outside 0..%d", status, SMSStatusSent)
	}
	encoding := SMSCDMAEncoding(binary.LittleEndian.Uint32(data[24:28]))
	if !validSMSCDMAEncoding(encoding) {
		return fmt.Errorf("encoding %d is outside 0..%d", encoding, SMSCDMAEncodingGSM7Bit)
	}
	language := SMSCDMALanguage(binary.LittleEndian.Uint32(data[28:32]))
	if !validSMSCDMALanguage(language) {
		return fmt.Errorf("language %d is outside 0..%d", language, SMSCDMALanguageHebrew)
	}
	addressRef, err := readOffsetSizeRef(data, 8)
	if err != nil {
		return fmt.Errorf("address: %w", err)
	}
	timestampRef, err := readOffsetSizeRef(data, 16)
	if err != nil {
		return fmt.Errorf("timestamp: %w", err)
	}
	messageRef, err := readOffsetSizeRef(data, 32)
	if err != nil {
		return fmt.Errorf("encoded message: %w", err)
	}
	refs := []struct {
		name        string
		ref         valueRef
		maximumSize uint32
	}{
		{name: "address", ref: addressRef, maximumSize: 40},
		{name: "timestamp", ref: timestampRef, maximumSize: 21},
		{name: "encoded message", ref: messageRef, maximumSize: 160},
	}
	for _, value := range refs {
		if value.ref.size > value.maximumSize {
			return fmt.Errorf("%s: size %d exceeds %d bytes", value.name, value.ref.size, value.maximumSize)
		}
	}
	dataRefs := []valueRef{addressRef, timestampRef, messageRef}
	if err := validateDataBufferRefs(data, 44, dataRefs); err != nil {
		return fmt.Errorf("data buffer: %w", err)
	}
	if addressRef.size%2 != 0 {
		return errors.New("address: UTF-16 string has odd byte length")
	}
	address, err := utf16String(data, addressRef)
	if err != nil {
		return fmt.Errorf("address: %w", err)
	}
	timestamp, err := parseSMSCDMATimestamp(timestampRef.bytes(data))
	if err != nil {
		return fmt.Errorf("timestamp: %w", err)
	}
	*r = SMSCDMARecord{
		MessageIndex:     binary.LittleEndian.Uint32(data[0:4]),
		MessageStatus:    status,
		Address:          address,
		Timestamp:        timestamp,
		Encoding:         encoding,
		Language:         language,
		EncodedMessage:   messageRef.bytes(data),
		SizeInCharacters: binary.LittleEndian.Uint32(data[40:44]),
	}
	return nil
}

func parseSMSCDMATimestamp(timestamp []byte) (string, error) {
	if len(timestamp) == 0 {
		return "", nil
	}
	if len(timestamp) == smsCDMATimestampLength+1 {
		// The table permits 21 bytes although the documented representation is
		// 20 bytes. Treat the extra byte only as a C-style terminator.
		if timestamp[smsCDMATimestampLength] != 0 {
			return "", errors.New("21-byte value is not NUL-terminated")
		}
		timestamp = timestamp[:smsCDMATimestampLength]
	}
	if len(timestamp) != smsCDMATimestampLength {
		return "", fmt.Errorf("length is %d, want %d bytes", len(timestamp), smsCDMATimestampLength)
	}
	if timestamp[2] != '/' || timestamp[5] != '/' || timestamp[8] != ',' ||
		timestamp[11] != ':' || timestamp[14] != ':' {
		return "", errors.New("format is not YY/MM/DD,HH:mm:SS±ZZ")
	}
	if timestamp[17] != '+' && timestamp[17] != '-' {
		return "", errors.New("time zone sign is not '+' or '-'")
	}

	fields := []struct {
		name    string
		offset  int
		minimum int
		maximum int
	}{
		{name: "year", offset: 0, minimum: 0, maximum: 99},
		{name: "month", offset: 3, minimum: 1, maximum: 12},
		{name: "day", offset: 6, minimum: 1, maximum: 31},
		{name: "hour", offset: 9, minimum: 0, maximum: 23},
		{name: "minute", offset: 12, minimum: 0, maximum: 59},
		{name: "second", offset: 15, minimum: 0, maximum: 59},
	}
	for _, field := range fields {
		value, ok := decimalPair(timestamp[field.offset : field.offset+2])
		if !ok {
			return "", fmt.Errorf("%s contains a non-decimal digit", field.name)
		}
		if value < field.minimum || value > field.maximum {
			return "", fmt.Errorf("%s %d is outside %d..%d", field.name, value, field.minimum, field.maximum)
		}
	}
	zone, ok := decimalPair(timestamp[18:20])
	if !ok {
		return "", errors.New("time zone contains a non-decimal digit")
	}
	if zone > 13 {
		return "", fmt.Errorf("time zone %d exceeds 13 hours", zone)
	}
	if timestamp[17] == '-' && zone > 12 {
		return "", fmt.Errorf("negative time zone %d exceeds 12 hours", zone)
	}
	return string(timestamp), nil
}

func decimalPair(value []byte) (int, bool) {
	if len(value) != 2 || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return 0, false
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), true
}

type SMSSendRequest struct {
	TransactionID uint32
	Format        SMSFormat
	PDU           SMSSendPDU
	CDMA          SMSSendCDMA
	Response      *SMSSendInfo
}

func (r *SMSSendRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Format))
	if r.Format == SMSFormatCDMA {
		data = append(data, marshalSMSSendCDMA(r.CDMA)...)
	} else {
		data = append(data, marshalSMSSendPDU(r.PDU)...)
	}
	r.Response = new(SMSSendInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDLongResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSSend, CommandTypeSet, data),
		Response:      r.Response,
	}
}

func marshalSMSSendPDU(message SMSSendPDU) []byte {
	data := make([]byte, 8)
	return appendRefValue(data, 0, message.PDU)
}

func marshalSMSSendCDMA(message SMSSendCDMA) []byte {
	data := make([]byte, 28)
	binary.LittleEndian.PutUint32(data[0:4], uint32(message.Encoding))
	binary.LittleEndian.PutUint32(data[4:8], uint32(message.Language))
	binary.LittleEndian.PutUint32(data[24:28], message.SizeInCharacters)
	data = appendRefValue(data, 8, utf16Bytes(message.Address))
	data = appendRefValue(data, 16, message.EncodedMessage)
	return data
}

func (r *SMSSendInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("parsing MBIM SMS send response: payload length is %d, want 4", len(data))
	}
	messageReference := binary.LittleEndian.Uint32(data[0:4])
	if messageReference > 0xffff {
		return fmt.Errorf("parsing MBIM SMS send response: message reference %d exceeds 65535", messageReference)
	}
	r.MessageReference = messageReference
	return nil
}

type SMSDeleteRequest struct {
	TransactionID uint32
	Flag          SMSReadFlag
	MessageIndex  uint32
	Response      *emptyResponse
}

func (r *SMSDeleteRequest) Request() *Request {
	data := binary.LittleEndian.AppendUint32(nil, uint32(r.Flag))
	data = binary.LittleEndian.AppendUint32(data, r.MessageIndex)
	r.Response = new(emptyResponse)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSDelete, CommandTypeSet, data),
		Response:      r.Response,
	}
}

type SMSStoreStatusRequest struct {
	TransactionID uint32
	Response      *SMSStoreStatusInfo
}

func (r *SMSStoreStatusRequest) Request() *Request {
	r.Response = new(SMSStoreStatusInfo)
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: r.TransactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceSMS, CIDSMSMessageStoreStatus, CommandTypeQuery, nil),
		Response:      r.Response,
	}
}

func (r *SMSStoreStatusInfo) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("parsing MBIM SMS store status: payload length is %d, want 8", len(data))
	}
	flags := SMSStatusFlags(binary.LittleEndian.Uint32(data[0:4]))
	knownFlags := SMSStatusFlagStoreFull | SMSStatusFlagNewMessage
	if flags&^knownFlags != 0 {
		return fmt.Errorf("parsing MBIM SMS store status: flags %#x contain reserved bits", flags)
	}
	*r = SMSStoreStatusInfo{
		Flags:        flags,
		MessageIndex: binary.LittleEndian.Uint32(data[4:8]),
	}
	return nil
}

type emptyResponse struct{}

func (*emptyResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("parsing empty MBIM response: length %d, want 0", len(data))
	}
	return nil
}

func (c *Client) SMSConfiguration(ctx context.Context) (SMSConfigurationInfo, error) {
	request := SMSConfigurationRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSConfigurationInfo{}, fmt.Errorf("reading MBIM SMS configuration: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SetSMSConfiguration(ctx context.Context, format SMSFormat, scAddress string) (SMSConfigurationInfo, error) {
	if !validSMSFormat(format) {
		return SMSConfigurationInfo{}, fmt.Errorf("setting MBIM SMS configuration: format %d is outside 0..%d", format, SMSFormatCDMA)
	}
	if size := len(utf16Bytes(scAddress)); size > 40 {
		return SMSConfigurationInfo{}, fmt.Errorf("setting MBIM SMS configuration: service center address length %d exceeds 40 bytes", size)
	}
	request := SMSConfigurationSetRequest{
		TransactionID: c.nextTransactionID(),
		Format:        format,
		SCAddress:     scAddress,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSConfigurationInfo{}, fmt.Errorf("setting MBIM SMS configuration: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) ReadSMS(ctx context.Context, format SMSFormat, flag SMSReadFlag, messageIndex uint32) (SMSReadInfo, error) {
	if !validSMSFormat(format) {
		return SMSReadInfo{}, fmt.Errorf("reading MBIM SMS messages: format %d is outside 0..%d", format, SMSFormatCDMA)
	}
	if err := validateSMSFilter(flag, messageIndex); err != nil {
		return SMSReadInfo{}, fmt.Errorf("reading MBIM SMS messages: %w", err)
	}
	request := SMSReadRequest{
		TransactionID: c.nextTransactionID(),
		Format:        format,
		Flag:          flag,
		MessageIndex:  messageIndex,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSReadInfo{}, fmt.Errorf("reading MBIM SMS messages: %w", err)
	}
	return cloneSMSReadInfo(*request.Response), nil
}

func (c *Client) SendSMSPDU(ctx context.Context, pdu []byte) (SMSSendInfo, error) {
	if len(pdu) > 255 {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM SMS PDU: length %d exceeds 255 bytes", len(pdu))
	}
	request := SMSSendRequest{
		TransactionID: c.nextTransactionID(),
		Format:        SMSFormatPDU,
		PDU:           SMSSendPDU{PDU: slices.Clone(pdu)},
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM SMS PDU: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) SendSMSCDMA(ctx context.Context, message SMSSendCDMA) (SMSSendInfo, error) {
	if !validSMSCDMAEncoding(message.Encoding) {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM CDMA SMS: encoding %d is outside 0..%d", message.Encoding, SMSCDMAEncodingGSM7Bit)
	}
	if !validSMSCDMALanguage(message.Language) {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM CDMA SMS: language %d is outside 0..%d", message.Language, SMSCDMALanguageHebrew)
	}
	if size := len(utf16Bytes(message.Address)); size > 40 {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM CDMA SMS: address length %d exceeds 40 bytes", size)
	}
	if len(message.EncodedMessage) > 160 {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM CDMA SMS: message length %d exceeds 160 bytes", len(message.EncodedMessage))
	}
	message.EncodedMessage = slices.Clone(message.EncodedMessage)
	request := SMSSendRequest{
		TransactionID: c.nextTransactionID(),
		Format:        SMSFormatCDMA,
		CDMA:          message,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSSendInfo{}, fmt.Errorf("sending MBIM CDMA SMS: %w", err)
	}
	return *request.Response, nil
}

func (c *Client) DeleteSMS(ctx context.Context, flag SMSReadFlag, messageIndex uint32) error {
	if err := validateSMSFilter(flag, messageIndex); err != nil {
		return fmt.Errorf("deleting MBIM SMS messages: %w", err)
	}
	request := SMSDeleteRequest{
		TransactionID: c.nextTransactionID(),
		Flag:          flag,
		MessageIndex:  messageIndex,
	}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return fmt.Errorf("deleting MBIM SMS messages: %w", err)
	}
	return nil
}

func (c *Client) SMSStoreStatus(ctx context.Context) (SMSStoreStatusInfo, error) {
	request := SMSStoreStatusRequest{TransactionID: c.nextTransactionID()}
	if err := c.transmit(ctx, request.Request()); err != nil {
		return SMSStoreStatusInfo{}, fmt.Errorf("reading MBIM SMS store status: %w", err)
	}
	return *request.Response, nil
}

func cloneSMSReadInfo(info SMSReadInfo) SMSReadInfo {
	out := info
	out.PDURecords = slices.Clone(info.PDURecords)
	for i := range out.PDURecords {
		out.PDURecords[i].PDU = slices.Clone(out.PDURecords[i].PDU)
	}
	out.CDMARecords = slices.Clone(info.CDMARecords)
	for i := range out.CDMARecords {
		out.CDMARecords[i].EncodedMessage = slices.Clone(out.CDMARecords[i].EncodedMessage)
	}
	return out
}
