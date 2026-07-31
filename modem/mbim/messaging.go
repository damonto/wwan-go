package mbim

import (
	"context"
	"fmt"
	"slices"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/sms"
)

func (*Backend) MessageStorages(context.Context) (MessageStorageInfo, error) {
	return MessageStorageInfo{Supported: []MessageStorage{MessageStorageDevice}, Default: MessageStorageDevice}, nil
}

func (b *Backend) ListMessages(ctx context.Context) ([]Message, error) {
	read, err := b.client.ReadSMS(ctx, mbimproto.SMSFormatPDU, mbimproto.SMSReadFlagAll, 0)
	if err != nil {
		return nil, fmt.Errorf("listing MBIM messages: %w", err)
	}
	parts := make([]sms.Part, 0, len(read.PDURecords))
	for _, record := range read.PDURecords {
		part, err := sms.DecodePDU(record.PDU)
		if err != nil {
			return nil, fmt.Errorf("decoding MBIM message %d: %w", record.MessageIndex, err)
		}
		part.Message.ID = record.MessageIndex
		part.Message.Storage = MessageStorageDevice
		part.Message.Refs = []MessageRef{{Storage: MessageStorageDevice, ID: record.MessageIndex}}
		part.Message.State = mbimMessageState(record.MessageStatus)
		parts = append(parts, part)
	}
	return sms.Assemble(parts), nil
}

func (b *Backend) ReadStoredMessage(ctx context.Context, ref MessageRef) (Message, error) {
	if ref.Storage != MessageStorageUnknown && ref.Storage != MessageStorageDevice {
		return Message{}, fmt.Errorf("reading MBIM message: storage %d is unsupported", ref.Storage)
	}
	return b.ReadMessage(ctx, ref.ID)
}

func (b *Backend) ReadMessage(ctx context.Context, id uint32) (Message, error) {
	read, err := b.client.ReadSMS(ctx, mbimproto.SMSFormatPDU, mbimproto.SMSReadFlagIndex, id)
	if err != nil {
		return Message{}, fmt.Errorf("reading MBIM message %d: %w", id, err)
	}
	if len(read.PDURecords) != 1 {
		return Message{}, fmt.Errorf("reading MBIM message %d: modem returned %d records", id, len(read.PDURecords))
	}
	record := read.PDURecords[0]
	part, err := sms.DecodePDU(record.PDU)
	if err != nil {
		return Message{}, fmt.Errorf("decoding MBIM message %d: %w", id, err)
	}
	part.Message.ID = record.MessageIndex
	part.Message.Storage = MessageStorageDevice
	part.Message.Refs = []MessageRef{{Storage: MessageStorageDevice, ID: record.MessageIndex}}
	part.Message.State = mbimMessageState(record.MessageStatus)
	return sms.CloneMessage(part.Message), nil
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
			return SendResult{}, fmt.Errorf("sending MBIM message part %d: %w", len(result.References)+1, err)
		}
		result.References = append(result.References, sent.MessageReference)
		part, _ := sms.DecodePDU(pdu)
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
		return fmt.Errorf("deleting MBIM message %d: %w", id, err)
	}
	return nil
}

func (b *Backend) DeleteStoredMessage(ctx context.Context, ref MessageRef) error {
	if ref.Storage != MessageStorageUnknown && ref.Storage != MessageStorageDevice {
		return fmt.Errorf("deleting MBIM message: storage %d is unsupported", ref.Storage)
	}
	return b.DeleteMessage(ctx, ref.ID)
}

func (b *Backend) SendPDU(ctx context.Context, pdu []byte) (uint32, error) {
	result, err := b.client.SendSMSPDU(ctx, slices.Clone(pdu))
	if err != nil {
		return 0, fmt.Errorf("sending MBIM PDU: %w", err)
	}
	return result.MessageReference, nil
}

func (b *Backend) WatchMessages(ctx context.Context) (<-chan Result[Message], error) {
	indications, err := b.client.WatchIndicationResults(ctx, mbimproto.ServiceSMS, mbimproto.CIDSMSMessageStoreStatus)
	if err != nil {
		return nil, fmt.Errorf("watching MBIM messages: %w", err)
	}
	out := make(chan Result[Message], 8)
	go func() {
		defer close(out)
		assembler := sms.Assembler{}
		for indication := range indications {
			if indication.Err != nil {
				sendStreamResult(ctx, out, Result[Message]{Err: indication.Err})
				return
			}
			var status mbimproto.SMSStoreStatusInfo
			if err := status.UnmarshalBinary(indication.Value.InformationBuffer); err != nil {
				sendStreamResult(ctx, out, Result[Message]{Err: fmt.Errorf("decoding MBIM message event: %w", err)})
				return
			}
			if status.Flags&mbimproto.SMSStatusFlagNewMessage == 0 {
				continue
			}
			message, err := b.ReadMessage(ctx, status.MessageIndex)
			if err != nil {
				sendStreamResult(ctx, out, Result[Message]{Err: err})
				return
			}
			part, err := sms.DecodePDU(message.PDU)
			if err != nil {
				sendStreamResult(ctx, out, Result[Message]{Err: err})
				return
			}
			part.Message = message
			assembled, complete := assembler.Add(part)
			if complete && !sendStreamResult(ctx, out, Result[Message]{Value: assembled}) {
				return
			}
		}
	}()
	return out, nil
}

func mbimMessageState(status mbimproto.SMSStatus) MessageState {
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
