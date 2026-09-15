package mbim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/sms"
)

func (*Backend) MessageStorages(context.Context) (MessageStorageInfo, error) {
	return MessageStorageInfo{Supported: []MessageStorage{MessageStorageDevice}, Default: MessageStorageDevice}, nil
}

// ListMessages returns decodable messages and a MessageListError if any stored
// PDUs are malformed. Transport errors abort the operation.
func (b *Backend) ListMessages(ctx context.Context) ([]Message, error) {
	read, err := b.client.ReadSMS(ctx, mbimproto.SMSFormatPDU, mbimproto.SMSReadFlagAll, 0)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	parts := make([]sms.Part, 0, len(read.PDURecords))
	var decodeErrors []*MessageDecodeError
	for _, record := range read.PDURecords {
		var part sms.Part
		if err := part.UnmarshalBinary(record.PDU); err != nil {
			decodeErrors = append(decodeErrors, &MessageDecodeError{
				Ref: MessageRef{Storage: MessageStorageDevice, ID: record.MessageIndex},
				PDU: slices.Clone(record.PDU), Err: err,
			})
			continue
		}
		part.Message.ID = record.MessageIndex
		part.Message.Storage = MessageStorageDevice
		part.Message.Refs = []MessageRef{{Storage: MessageStorageDevice, ID: record.MessageIndex}}
		part.Message.State = messageState(record.MessageStatus)
		parts = append(parts, part)
	}
	messages := sms.Assemble(parts)
	if len(decodeErrors) != 0 {
		return messages, &MessageListError{Errors: decodeErrors}
	}
	return messages, nil
}

func (b *Backend) ReadStoredMessage(ctx context.Context, ref MessageRef) (Message, error) {
	if ref.Storage != MessageStorageUnknown && ref.Storage != MessageStorageDevice {
		return Message{}, fmt.Errorf("reading message: storage %d is unsupported", ref.Storage)
	}
	return b.ReadMessage(ctx, ref.ID)
}

func (b *Backend) ReadMessage(ctx context.Context, id uint32) (Message, error) {
	part, err := b.readMessagePart(ctx, id)
	if err != nil {
		return Message{}, err
	}
	return sms.CloneMessage(part.Message), nil
}

func (b *Backend) readMessagePart(ctx context.Context, id uint32) (sms.Part, error) {
	read, err := b.client.ReadSMS(ctx, mbimproto.SMSFormatPDU, mbimproto.SMSReadFlagIndex, id)
	if err != nil {
		return sms.Part{}, fmt.Errorf("reading message %d: %w", id, err)
	}
	if len(read.PDURecords) != 1 {
		return sms.Part{}, fmt.Errorf("reading message %d: modem returned %d records", id, len(read.PDURecords))
	}
	record := read.PDURecords[0]
	var part sms.Part
	if err := part.UnmarshalBinary(record.PDU); err != nil {
		return sms.Part{}, &MessageDecodeError{
			Ref: MessageRef{Storage: MessageStorageDevice, ID: record.MessageIndex},
			PDU: slices.Clone(record.PDU), Err: err,
		}
	}
	part.Message.ID = record.MessageIndex
	part.Message.Storage = MessageStorageDevice
	part.Message.Refs = []MessageRef{{Storage: MessageStorageDevice, ID: record.MessageIndex}}
	part.Message.State = messageState(record.MessageStatus)
	return part, nil
}

func (b *Backend) SendMessage(ctx context.Context, cfg MessageConfig) (SendResult, error) {
	pdus, err := sms.EncodePDUs(cfg)
	if err != nil {
		return SendResult{}, err
	}
	result := SendResult{References: make([]uint32, 0, len(pdus)), Messages: make([]Message, 0, len(pdus))}
	for _, pdu := range pdus {
		sent, err := b.client.SendSMSPDU(ctx, pdu)
		if err != nil {
			return SendResult{}, fmt.Errorf("sending message part %d: %w", len(result.References)+1, err)
		}
		result.References = append(result.References, sent.MessageReference)
		var part sms.Part
		if err := part.UnmarshalBinary(pdu); err != nil {
			return SendResult{}, fmt.Errorf("decoding sent message part %d: %w", len(result.References), err)
		}
		part.Message.MessageReference = sent.MessageReference
		part.Message.State = MessageStateStoredSent
		result.Messages = append(result.Messages, sms.CloneMessage(part.Message))
	}
	return result, nil
}

func (b *Backend) StoreMessage(context.Context, MessageConfig) ([]Message, error) {
	return nil, ErrNotSupported
}

func (b *Backend) DeleteMessage(ctx context.Context, id uint32) error {
	if err := b.client.DeleteSMS(ctx, mbimproto.SMSReadFlagIndex, id); err != nil {
		return fmt.Errorf("deleting message %d: %w", id, err)
	}
	return nil
}

func (b *Backend) DeleteStoredMessage(ctx context.Context, ref MessageRef) error {
	if ref.Storage != MessageStorageUnknown && ref.Storage != MessageStorageDevice {
		return fmt.Errorf("deleting message: storage %d is unsupported", ref.Storage)
	}
	return b.DeleteMessage(ctx, ref.ID)
}

func (b *Backend) SendPDU(ctx context.Context, pdu []byte) (uint32, error) {
	result, err := b.client.SendSMSPDU(ctx, slices.Clone(pdu))
	if err != nil {
		return 0, err
	}
	return result.MessageReference, nil
}

// WatchMessages reports malformed PDUs as MessageDecodeError results and keeps
// watching. Transport and malformed notification errors are terminal.
func (b *Backend) WatchMessages(ctx context.Context) (<-chan Result[Message], error) {
	watchCtx, cancel := context.WithCancel(ctx)
	storedMessages, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceSMS, mbimproto.CIDSMSMessageStoreStatus)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching messages: %w", err)
	}
	flashMessages, err := b.client.WatchIndicationResults(watchCtx, mbimproto.ServiceSMS, mbimproto.CIDSMSRead)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watching messages: %w", err)
	}
	if err := b.ensureWatchNotifications(ctx, mbimproto.DeviceServiceSubscribeEntry{
		ServiceID: mbimproto.ServiceSMS,
		CIDs:      []uint32{mbimproto.CIDSMSRead, mbimproto.CIDSMSMessageStoreStatus},
	}); err != nil {
		cancel()
		return nil, err
	}
	out := make(chan Result[Message], 8)
	go func() {
		defer close(out)
		defer cancel()
		assembler := sms.Assembler{}
		emitPart := func(part sms.Part) bool {
			assembled, complete := assembler.Add(part)
			return !complete || sendStreamResult(watchCtx, out, Result[Message]{Value: assembled})
		}
		sendError := func(err error) {
			// The watcher terminates immediately after this best-effort error report.
			_ = sendStreamResult(watchCtx, out, Result[Message]{Err: err})
		}

		for storedMessages != nil || flashMessages != nil {
			select {
			case <-watchCtx.Done():
				return
			case indication, ok := <-storedMessages:
				if !ok {
					storedMessages = nil
					continue
				}
				if indication.Err != nil {
					sendError(fmt.Errorf("watching message store: %w", indication.Err))
					return
				}
				var status mbimproto.SMSStoreStatusInfo
				if err := status.UnmarshalBinary(indication.Value.InformationBuffer); err != nil {
					sendError(fmt.Errorf("decoding message store event: %w", err))
					return
				}
				if status.Flags&mbimproto.SMSStatusFlagNewMessage == 0 {
					continue
				}
				part, err := b.readMessagePart(watchCtx, status.MessageIndex)
				if err != nil {
					if _, ok := errors.AsType[*MessageDecodeError](err); ok {
						if !sendStreamResult(watchCtx, out, Result[Message]{Err: err}) {
							return
						}
						continue
					}
					sendError(err)
					return
				}
				if !emitPart(part) {
					return
				}
			case indication, ok := <-flashMessages:
				if !ok {
					flashMessages = nil
					continue
				}
				if indication.Err != nil {
					sendError(fmt.Errorf("watching flash messages: %w", indication.Err))
					return
				}
				var read mbimproto.SMSReadInfo
				if err := read.UnmarshalBinary(indication.Value.InformationBuffer); err != nil {
					sendError(fmt.Errorf("decoding flash message event: %w", err))
					return
				}
				parts, err := flashMessageParts(read)
				if err != nil {
					listErr, ok := errors.AsType[*MessageListError](err)
					if !ok {
						sendError(err)
						return
					}
					for _, decodeErr := range listErr.Errors {
						if !sendStreamResult(watchCtx, out, Result[Message]{Err: decodeErr}) {
							return
						}
					}
				}
				for _, part := range parts {
					if !emitPart(part) {
						return
					}
				}
			}
		}
	}()
	return out, nil
}

func flashMessageParts(read mbimproto.SMSReadInfo) ([]sms.Part, error) {
	if read.Format != mbimproto.SMSFormatPDU {
		return nil, fmt.Errorf("decoding flash messages: format %d is unsupported", read.Format)
	}
	parts := make([]sms.Part, 0, len(read.PDURecords))
	var decodeErrors []*MessageDecodeError
	for i, record := range read.PDURecords {
		var part sms.Part
		if err := part.UnmarshalBinary(record.PDU); err != nil {
			decodeErrors = append(decodeErrors, &MessageDecodeError{
				PDU: slices.Clone(record.PDU), Err: fmt.Errorf("record %d: %w", i+1, err),
			})
			continue
		}
		part.Message.State = messageState(record.MessageStatus)
		parts = append(parts, part)
	}
	if len(decodeErrors) != 0 {
		return parts, &MessageListError{Errors: decodeErrors}
	}
	return parts, nil
}

func messageState(status mbimproto.SMSStatus) MessageState {
	switch status {
	case mbimproto.SMSStatusNew:
		return MessageStateReceivedUnread
	case mbimproto.SMSStatusOld:
		return MessageStateReceivedRead
	case mbimproto.SMSStatusDraft:
		return MessageStateStoredUnsent
	case mbimproto.SMSStatusSent:
		return MessageStateStoredSent
	default:
		return MessageStateUnknown
	}
}
