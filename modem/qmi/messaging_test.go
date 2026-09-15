package qmi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/modem/sms"
	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

func TestListMessagesIsolatesMalformedPDUs(t *testing.T) {
	tests := []struct {
		name       string
		secondPDU  []byte
		wantSecond string
		wantErrors int
		readErr    error
	}{
		{name: "undefined extension", secondPDU: messagingTestPDU([]byte("code \x1b\x00 hash")), wantSecond: "code @ hash"},
		{name: "double escape", secondPDU: messagingTestPDU([]byte("code\x1b\x1bhash")), wantSecond: "code hash"},
		{name: "truncated stored PDU", secondPDU: []byte{0}, wantErrors: 1},
		{name: "transport error remains fatal", readErr: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &messagingTestTransport{readErr: tt.readErr}
			for i := range 6 {
				pdu := messagingTestPDU([]byte(fmt.Sprintf("message %d", i+1)))
				if i == 1 {
					pdu = tt.secondPDU
				}
				transport.stored = append(transport.stored, messagingStoredPDU{
					ref: qcom.WMSMessageReference{Storage: qcom.WMSStorageNV, Index: uint32(i + 1)},
					pdu: pdu,
				})
			}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			})

			messages, err := New(client, "/dev/test").ListMessages(t.Context())
			if tt.readErr != nil {
				if !errors.Is(err, tt.readErr) {
					t.Fatalf("ListMessages() error = %v, want %v", err, tt.readErr)
				}
				if _, ok := errors.AsType[*MessageListError](err); ok {
					t.Fatal("transport error was reported as a recoverable partial list")
				}
				return
			}
			if tt.wantErrors == 0 && err != nil {
				t.Fatalf("ListMessages() error = %v", err)
			}
			if tt.wantErrors != 0 {
				listErr, ok := errors.AsType[*MessageListError](err)
				if !ok || len(listErr.Errors) != tt.wantErrors {
					t.Fatalf("ListMessages() error = %v, want %d decoding errors", err, tt.wantErrors)
				}
				decodeErr := listErr.Errors[0]
				wantRef := MessageRef{Storage: MessageStorageDevice, ID: 2}
				if decodeErr.Ref != wantRef || !bytes.Equal(decodeErr.PDU, tt.secondPDU) {
					t.Errorf("decode error = %+v, want ref %+v and PDU %x", decodeErr, wantRef, tt.secondPDU)
				}
			}
			if len(messages) != 6-tt.wantErrors {
				t.Fatalf("ListMessages() returned %d messages, want %d", len(messages), 6-tt.wantErrors)
			}
			if messages[len(messages)-1].Text != "message 6" {
				t.Error("ListMessages() did not reach the last stored message")
			}
			if tt.wantErrors == 0 && messages[1].Text != tt.wantSecond {
				t.Errorf("second message = %q, want %q", messages[1].Text, tt.wantSecond)
			}
		})
	}
}

func TestWatchMessagesContinuesAfterMalformedPDU(t *testing.T) {
	imsEnabled, imsDisabled := true, false
	tests := []struct {
		name         string
		firstTPDU    []byte
		ackIndicator qcom.WMSACKIndicator
		smsOnIMS     *bool
		wantText     string
		wantError    bool
	}{
		{
			name: "transfer route carries a TPDU without an SMSC prefix",
			firstTPDU: []byte{
				0x04, 4, 0x91, 0x21, 0x43, 0, 0,
				0x62, 0x90, 0x51, 0x41, 0, 0, 0,
				5, 0xe8, 0x32, 0x9b, 0xfd, 0x06,
			},
			wantText: "hello",
		},
		{name: "undefined extension is acknowledged", firstTPDU: messagingTestPDU([]byte("code \x1b\x00 hash"))[1:], wantText: "code @ hash"},
		{name: "malformed TPDU is isolated", firstTPDU: []byte{0}, wantError: true},
		{name: "transfer route without ACK", firstTPDU: messagingTestPDU([]byte("hello"))[1:], ackIndicator: qcom.WMSACKNotRequired, wantText: "hello"},
		{name: "ACK preserves IMS route", firstTPDU: messagingTestPDU([]byte("hello"))[1:], smsOnIMS: &imsEnabled, wantText: "hello"},
		{name: "ACK preserves circuit switched route", firstTPDU: messagingTestPDU([]byte("hello"))[1:], smsOnIMS: &imsDisabled, wantText: "hello"},
		{name: "negative ACK preserves IMS route", firstTPDU: []byte{0}, smsOnIMS: &imsEnabled, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &messagingTestTransport{}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			})
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			stream, err := New(client, "/dev/test").WatchMessages(ctx)
			if err != nil {
				t.Fatalf("WatchMessages() error = %v", err)
			}
			for i, pdu := range [][]byte{tt.firstTPDU, messagingTestPDU([]byte("next message"))[1:]} {
				payload := binary.LittleEndian.AppendUint32([]byte{byte(tt.ackIndicator)}, uint32(i+1))
				payload = append(payload, byte(qcom.WMSMessageFormatGWPointToPoint))
				payload = binary.LittleEndian.AppendUint16(payload, uint16(len(pdu)))
				payload = append(payload, pdu...)
				tlvs := tlv.TLVs{tlv.Bytes(0x11, payload)}
				if tt.smsOnIMS != nil {
					var value byte
					if *tt.smsOnIMS {
						value = 1
					}
					tlvs = append(tlvs, tlv.Uint(0x16, value))
				}
				transport.emit(qcom.Indication{
					Service: qcom.ServiceWMS, MessageID: qcom.MessageWMSEventReport,
					TLVs: tlvs,
				})
				select {
				case result, ok := <-stream:
					if !ok {
						t.Fatal("WatchMessages() closed before the next message")
					}
					if i == 0 && tt.wantError {
						decodeErr, ok := errors.AsType[*MessageDecodeError](result.Err)
						if !ok || !bytes.Equal(decodeErr.PDU, pdu) {
							t.Fatalf("first result error = %v, want original malformed PDU", result.Err)
						}
						continue
					}
					want := tt.wantText
					if i == 1 {
						want = "next message"
					}
					if result.Err != nil || result.Value.Text != want {
						t.Fatalf("result = %+v, want text %q", result, want)
					}
					if result.Value.Number != "+1234" || result.Value.SMSC != "" || result.Value.Storage != MessageStorageUnknown || len(result.Value.Refs) != 0 {
						t.Errorf("transfer-route message metadata = %+v", result.Value)
					}
					if !bytes.Equal(result.Value.PDU, append([]byte{0}, pdu...)) {
						t.Errorf("PDU = %x, want a zero SMSC prefix followed by %x", result.Value.PDU, pdu)
					}
				case <-time.After(time.Second):
					t.Fatal("WatchMessages() did not deliver the message")
				}
			}
			wantACKs := []bool{!tt.wantError, true}
			if tt.ackIndicator == qcom.WMSACKNotRequired {
				wantACKs = nil
			}
			if got, want := transport.ackResults(), wantACKs; !slices.Equal(got, want) {
				t.Errorf("ACK success values = %v, want %v", got, want)
			}
			for i, ack := range transport.acknowledgements() {
				if ack.TransactionID != uint32(i+1) || ack.Protocol != qcom.WMSMessageProtocolWCDMA {
					t.Errorf("ACK context = %+v, want transaction %d and WCDMA", ack, i+1)
				}
				if (ack.SMSOnIMS == nil) != (tt.smsOnIMS == nil) || ack.SMSOnIMS != nil && *ack.SMSOnIMS != *tt.smsOnIMS {
					t.Errorf("ACK IMS route = %v, want %v", ack.SMSOnIMS, tt.smsOnIMS)
				}
			}
			cancel()
			select {
			case _, ok := <-stream:
				if ok {
					t.Error("WatchMessages() returned an unexpected result after cancellation")
				}
			case <-time.After(time.Second):
				t.Fatal("WatchMessages() did not close after cancellation")
			}
		})
	}
}

func messagingTestPDU(septets []byte) []byte {
	packed, _ := sms.PackSeptets(septets, nil)
	pdu := []byte{0, 0, 4, 0x91, 0x21, 0x43, 0, 0, 0x62, 0x90, 0x51, 0x41, 0, 0, 0, byte(len(septets))}
	return append(pdu, packed...)
}

func TestWatchMessagesAcknowledgesOnlyTransferRoutes(t *testing.T) {
	tests := []struct {
		name     string
		kind     byte
		payload  []byte
		wantACKs int
	}{
		{
			name:     "stored message",
			kind:     0x10,
			payload:  []byte{byte(qcom.WMSStorageNV), 1, 0, 0, 0},
			wantACKs: 0,
		},
		{
			name:     "transfer route",
			kind:     0x11,
			payload:  []byte{byte(qcom.WMSACKRequired), 1, 0, 0, 0, byte(qcom.WMSMessageFormatGWPointToPoint), 1, 0, 0},
			wantACKs: 1,
		},
		{
			name:     "transfer route without ACK",
			kind:     0x11,
			payload:  []byte{byte(qcom.WMSACKNotRequired), 1, 0, 0, 0, byte(qcom.WMSMessageFormatGWPointToPoint), 1, 0, 0},
			wantACKs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &messagingTestTransport{}
			client, err := qcom.NewClient(transport)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			defer client.Close()

			backend := New(client, "/dev/test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := backend.WatchMessages(ctx)
			if err != nil {
				t.Fatalf("WatchMessages() error = %v", err)
			}

			transport.emit(qcom.Indication{
				Service:   qcom.ServiceWMS,
				MessageID: qcom.MessageWMSEventReport,
				TLVs:      tlv.TLVs{tlv.Bytes(tt.kind, tt.payload)},
			})

			select {
			case result := <-stream:
				if result.Err == nil {
					t.Fatal("WatchMessages() returned nil error for malformed test PDU")
				}
			case <-time.After(time.Second):
				t.Fatal("WatchMessages() did not report malformed test PDU")
			}

			if got := transport.ackCount(); got != tt.wantACKs {
				t.Fatalf("WMS ACK count = %d, want %d", got, tt.wantACKs)
			}
		})
	}
}

type messagingTestTransport struct {
	mu          sync.Mutex
	indications chan qcom.Indication
	acks        int
	ackRequests []qcom.WMSACKRequest
	stored      []messagingStoredPDU
	readErr     error
}

type messagingStoredPDU struct {
	ref qcom.WMSMessageReference
	pdu []byte
}

func (t *messagingTestTransport) QMIService() qcom.ServiceType {
	return qcom.ServiceWMS
}

func (t *messagingTestTransport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	if err := ctx.Err(); err != nil {
		return qcom.Response{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if req.MessageID == qcom.MessageWMSSendACK {
		t.acks++
		value, ok := tlv.Value(req.TLVs, 0x01)
		if !ok || len(value) != 6 {
			return qcom.Response{}, errors.New("invalid ACK info")
		}
		ack := qcom.WMSACKRequest{
			TransactionID: binary.LittleEndian.Uint32(value[:4]),
			Protocol:      qcom.WMSMessageProtocol(value[4]),
			Success:       value[5] != 0,
		}
		if value, ok := tlv.Value(req.TLVs, 0x12); ok {
			if len(value) != 1 {
				return qcom.Response{}, errors.New("invalid ACK IMS route")
			}
			onIMS := value[0] != 0
			ack.SMSOnIMS = &onIMS
		}
		t.ackRequests = append(t.ackRequests, ack)
	}

	response := qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlv.TLVs{tlv.Bytes(0x02, []byte{0, 0, 0, 0})},
	}
	if req.MessageID == qcom.MessageWMSListMessages {
		value, ok := tlv.Value(req.TLVs, 0x01)
		if !ok || len(value) != 1 {
			return qcom.Response{}, errors.New("invalid message storage")
		}
		listed := make([]byte, 4)
		for _, message := range t.stored {
			if message.ref.Storage == qcom.WMSStorage(value[0]) {
				listed = binary.LittleEndian.AppendUint32(listed, message.ref.Index)
				listed = append(listed, byte(qcom.WMSTagMTNotRead))
			}
		}
		binary.LittleEndian.PutUint32(listed, uint32((len(listed)-4)/5))
		response.TLVs = append(response.TLVs, tlv.Bytes(0x01, listed))
	}
	if req.MessageID == qcom.MessageWMSRawRead {
		if t.readErr != nil {
			return qcom.Response{}, t.readErr
		}
		pdu := []byte{0}
		if t.stored != nil {
			value, ok := tlv.Value(req.TLVs, 0x01)
			if !ok {
				return qcom.Response{}, errors.New("missing message reference")
			}
			var ref qcom.WMSMessageReference
			if err := ref.UnmarshalBinary(value); err != nil {
				return qcom.Response{}, err
			}
			index := slices.IndexFunc(t.stored, func(message messagingStoredPDU) bool { return message.ref == ref })
			if index < 0 {
				return qcom.Response{}, errors.New("stored message not found")
			}
			pdu = t.stored[index].pdu
		}
		value, err := (qcom.WMSRawMessage{Tag: qcom.WMSTagMTNotRead, Format: qcom.WMSMessageFormatGWPointToPoint, Data: pdu}).MarshalBinary()
		if err != nil {
			return qcom.Response{}, err
		}
		response.TLVs = append(response.TLVs, tlv.Bytes(0x01, value))
	}
	return response, nil
}

func (t *messagingTestTransport) Indications(ctx context.Context, _ qcom.ServiceType, _ uint8, _ qcom.MessageID) (<-chan qcom.Indication, error) {
	t.mu.Lock()
	t.indications = make(chan qcom.Indication, 2)
	indications := t.indications
	t.mu.Unlock()
	go func() {
		<-ctx.Done()
		close(indications)
	}()
	return indications, nil
}

func (t *messagingTestTransport) emit(indication qcom.Indication) {
	t.mu.Lock()
	indications := t.indications
	t.mu.Unlock()
	indications <- indication
}

func (t *messagingTestTransport) ackCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.acks
}

func (t *messagingTestTransport) ackResults() []bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	results := make([]bool, len(t.ackRequests))
	for i, ack := range t.ackRequests {
		results[i] = ack.Success
	}
	return results
}

func (t *messagingTestTransport) acknowledgements() []qcom.WMSACKRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.ackRequests)
}

func (t *messagingTestTransport) Close() error { return nil }
