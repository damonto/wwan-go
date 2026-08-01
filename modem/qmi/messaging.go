package qmi

import (
	"context"
	"fmt"

	"github.com/damonto/wwan-go/modem/sms"
	"github.com/damonto/wwan-go/qcom"
)

const qmiDeviceStorageBit = uint32(1 << 31)

func (*Backend) MessageStorages(context.Context) (MessageStorageInfo, error) {
	return MessageStorageInfo{
		Supported: []MessageStorage{MessageStorageDevice, MessageStorageSIM},
		Default:   MessageStorageDevice,
	}, nil
}

func (b *Backend) ListMessages(ctx context.Context) ([]Message, error) {
	mode := qcom.WMSMessageModeGW
	var parts []sms.Part
	for _, storage := range []qcom.WMSStorage{qcom.WMSStorageUIM, qcom.WMSStorageNV} {
		listed, err := b.client.WMSListMessages(ctx, qcom.WMSListRequest{Storage: storage, MessageMode: &mode})
		if err != nil {
			return nil, fmt.Errorf("listing QMI messages: %w", err)
		}
		for _, entry := range listed {
			raw, err := b.client.WMSReadRaw(ctx, qcom.WMSReadRequest{Reference: entry.Reference, MessageMode: &mode})
			if err != nil {
				return nil, fmt.Errorf("reading QMI message %d: %w", entry.Reference.Index, err)
			}
			if raw.Format != qcom.WMSMessageFormatGWPointToPoint {
				continue
			}
			var part sms.Part
			if err := part.UnmarshalBinary(raw.Data); err != nil {
				return nil, fmt.Errorf("decoding QMI message %d: %w", entry.Reference.Index, err)
			}
			part.Message.ID = qmiMessageID(entry.Reference)
			part.Message.Storage = qmiMessageStorage(entry.Reference.Storage)
			part.Message.Refs = []MessageRef{qmiStoredMessageRef(entry.Reference)}
			part.Message.State = qmiMessageState(entry.Tag)
			parts = append(parts, part)
		}
	}
	return sms.Assemble(parts), nil
}

func (b *Backend) ReadStoredMessage(ctx context.Context, ref MessageRef) (Message, error) {
	reference, err := qmiStoredReference(ref)
	if err != nil {
		return Message{}, err
	}
	return b.readStoredMessage(ctx, reference)
}

func (b *Backend) ReadMessage(ctx context.Context, id uint32) (Message, error) {
	return b.readStoredMessage(ctx, qmiMessageReference(id))
}

func (b *Backend) readStoredMessage(ctx context.Context, reference qcom.WMSMessageReference) (Message, error) {
	mode := qcom.WMSMessageModeGW
	raw, err := b.client.WMSReadRaw(ctx, qcom.WMSReadRequest{Reference: reference, MessageMode: &mode})
	if err != nil {
		return Message{}, fmt.Errorf("reading QMI message %d: %w", reference.Index, err)
	}
	if raw.Format != qcom.WMSMessageFormatGWPointToPoint {
		return Message{}, ErrNotSupported
	}
	var part sms.Part
	if err := part.UnmarshalBinary(raw.Data); err != nil {
		return Message{}, fmt.Errorf("decoding QMI message %d: %w", reference.Index, err)
	}
	part.Message.ID = qmiMessageID(reference)
	part.Message.Storage = qmiMessageStorage(reference.Storage)
	part.Message.Refs = []MessageRef{qmiStoredMessageRef(reference)}
	part.Message.State = qmiMessageState(raw.Tag)
	return sms.CloneMessage(part.Message), nil
}

func (b *Backend) SendMessage(ctx context.Context, cfg MessageConfig) (SendResult, error) {
	pdus, err := sms.EncodePDUs(cfg)
	if err != nil {
		return SendResult{}, err
	}
	result := SendResult{References: make([]uint32, 0, len(pdus)), Messages: make([]Message, 0, len(pdus))}
	for _, pdu := range pdus {
		sent, err := b.client.WMSSendRaw(ctx, qcom.WMSMessageFormatGWPointToPoint, pdu, qcom.WMSSendOptions{})
		if err != nil {
			return SendResult{}, fmt.Errorf("sending QMI message part %d: %w", len(result.References)+1, err)
		}
		reference := uint32(sent.MessageID)
		result.References = append(result.References, reference)
		var part sms.Part
		if err := part.UnmarshalBinary(pdu); err != nil {
			return SendResult{}, fmt.Errorf("decoding sent QMI message part %d: %w", len(result.References), err)
		}
		part.Message.MessageReference = reference
		part.Message.State = MessageStateStoredSent
		result.Messages = append(result.Messages, sms.CloneMessage(part.Message))
	}
	return result, nil
}

func (b *Backend) StoreMessage(ctx context.Context, cfg MessageConfig) ([]Message, error) {
	pdus, err := sms.EncodePDUs(cfg)
	if err != nil {
		return nil, err
	}
	tag := qcom.WMSTagMONotSent
	storage, err := qmiStorage(cfg.Storage)
	if err != nil {
		return nil, err
	}
	result := make([]Message, 0, len(pdus))
	for _, pdu := range pdus {
		reference, err := b.client.WMSWriteRaw(ctx, qcom.WMSWriteRequest{Storage: storage, Format: qcom.WMSMessageFormatGWPointToPoint, Data: pdu, Tag: &tag})
		if err != nil {
			return nil, fmt.Errorf("storing QMI message part %d: %w", len(result)+1, err)
		}
		var part sms.Part
		if err := part.UnmarshalBinary(pdu); err != nil {
			return nil, fmt.Errorf("decoding stored QMI message part %d: %w", len(result)+1, err)
		}
		part.Message.ID = qmiMessageID(reference)
		part.Message.Storage = qmiMessageStorage(reference.Storage)
		part.Message.Refs = []MessageRef{qmiStoredMessageRef(reference)}
		part.Message.State = MessageStateStoredUnsent
		result = append(result, sms.CloneMessage(part.Message))
	}
	return result, nil
}

func (b *Backend) DeleteMessage(ctx context.Context, id uint32) error {
	return b.deleteStoredMessage(ctx, qmiMessageReference(id))
}

func (b *Backend) DeleteStoredMessage(ctx context.Context, ref MessageRef) error {
	reference, err := qmiStoredReference(ref)
	if err != nil {
		return err
	}
	return b.deleteStoredMessage(ctx, reference)
}

func (b *Backend) deleteStoredMessage(ctx context.Context, reference qcom.WMSMessageReference) error {
	mode := qcom.WMSMessageModeGW
	if err := b.client.WMSDelete(ctx, qcom.WMSDeleteRequest{Storage: reference.Storage, Index: &reference.Index, MessageMode: &mode}); err != nil {
		return fmt.Errorf("deleting QMI message %d: %w", reference.Index, err)
	}
	return nil
}

func (b *Backend) SendPDU(ctx context.Context, pdu []byte) (uint32, error) {
	result, err := b.client.WMSSendRaw(ctx, qcom.WMSMessageFormatGWPointToPoint, pdu, qcom.WMSSendOptions{})
	if err != nil {
		return 0, fmt.Errorf("sending QMI PDU: %w", err)
	}
	return uint32(result.MessageID), nil
}

func (b *Backend) WatchMessages(ctx context.Context) (<-chan Result[Message], error) {
	incoming, err := b.client.WMSWatchIncoming(ctx)
	if err != nil {
		return nil, fmt.Errorf("watching QMI messages: %w", err)
	}
	out := make(chan Result[Message], 8)
	go func() {
		defer close(out)
		assembler := sms.Assembler{}
		for raw := range incoming {
			if raw.ReadError != nil {
				sendStreamResult(ctx, out, Result[Message]{Err: raw.ReadError})
				return
			}
			if raw.Format != qcom.WMSMessageFormatGWPointToPoint {
				continue
			}
			var part sms.Part
			if err := part.UnmarshalBinary(raw.Data); err != nil {
				if raw.AckIndicator == qcom.WMSAckRequired {
					_ = b.client.WMSAcknowledge(ctx, qcom.WMSAckRequest{TransactionID: raw.TransactionID, Protocol: qcom.WMSMessageProtocolWCDMA, Success: false})
				}
				sendStreamResult(ctx, out, Result[Message]{Err: err})
				return
			}
			if raw.Stored {
				part.Message.ID = qmiMessageID(raw.Reference)
				part.Message.Storage = qmiMessageStorage(raw.Reference.Storage)
				part.Message.Refs = []MessageRef{qmiStoredMessageRef(raw.Reference)}
			}
			part.Message.State = qmiMessageState(raw.Tag)
			if raw.AckIndicator == qcom.WMSAckRequired {
				if err := b.client.WMSAcknowledge(ctx, qcom.WMSAckRequest{TransactionID: raw.TransactionID, Protocol: qcom.WMSMessageProtocolWCDMA, Success: true}); err != nil {
					sendStreamResult(ctx, out, Result[Message]{Err: err})
					return
				}
			}
			message, complete := assembler.Add(part)
			if complete && !sendStreamResult(ctx, out, Result[Message]{Value: message}) {
				return
			}
		}
	}()
	return out, nil
}

func qmiStoredMessageRef(reference qcom.WMSMessageReference) MessageRef {
	return MessageRef{Storage: qmiMessageStorage(reference.Storage), ID: reference.Index}
}

func qmiStoredReference(ref MessageRef) (qcom.WMSMessageReference, error) {
	storage, err := qmiStorage(ref.Storage)
	if err != nil {
		return qcom.WMSMessageReference{}, err
	}
	return qcom.WMSMessageReference{Storage: storage, Index: ref.ID}, nil
}

func qmiStorage(storage MessageStorage) (qcom.WMSStorage, error) {
	switch storage {
	case MessageStorageUnknown, MessageStorageDevice:
		return qcom.WMSStorageNV, nil
	case MessageStorageSIM:
		return qcom.WMSStorageUIM, nil
	default:
		return 0, fmt.Errorf("using QMI message storage: storage %d is invalid", storage)
	}
}

func qmiMessageID(reference qcom.WMSMessageReference) uint32 {
	if reference.Storage == qcom.WMSStorageNV {
		return reference.Index | qmiDeviceStorageBit
	}
	return reference.Index
}

func qmiMessageReference(id uint32) qcom.WMSMessageReference {
	if id&qmiDeviceStorageBit != 0 {
		return qcom.WMSMessageReference{Storage: qcom.WMSStorageNV, Index: id &^ qmiDeviceStorageBit}
	}
	return qcom.WMSMessageReference{Storage: qcom.WMSStorageUIM, Index: id}
}

func qmiMessageStorage(storage qcom.WMSStorage) MessageStorage {
	if storage == qcom.WMSStorageUIM {
		return MessageStorageSIM
	}
	return MessageStorageDevice
}

func qmiMessageState(tag qcom.WMSTag) MessageState {
	switch tag {
	case qcom.WMSTagMTRead:
		return MessageStateReceivedRead
	case qcom.WMSTagMTNotRead:
		return MessageStateReceivedUnread
	case qcom.WMSTagMOSent:
		return MessageStateStoredSent
	case qcom.WMSTagMONotSent:
		return MessageStateStoredUnsent
	default:
		return MessageStateUnknown
	}
}
