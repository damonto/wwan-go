package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/modem/sms"
)

func TestListMessagesIsolatesMalformedPDUs(t *testing.T) {
	tests := []struct {
		name       string
		secondPDU  []byte
		wantSecond string
		wantErrors int
		readStatus mbimproto.Status
	}{
		{name: "undefined extension", secondPDU: messagingTestPDU([]byte("code \x1b\x00 hash")), wantSecond: "code @ hash"},
		{name: "truncated stored PDU", secondPDU: []byte{0}, wantErrors: 1},
		{name: "modem error remains fatal", readStatus: mbimproto.StatusFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var records []mbimproto.SMSPDURecord
			for i := range 6 {
				pdu := messagingTestPDU([]byte(fmt.Sprintf("message %d", i+1)))
				if i == 1 {
					pdu = tt.secondPDU
				}
				records = append(records, mbimproto.SMSPDURecord{MessageIndex: uint32(i + 1), MessageStatus: mbimproto.SMSStatusNew, PDU: pdu})
			}
			backend, _ := newMessagingTestBackend(t, records, tt.readStatus)
			messages, err := backend.ListMessages(t.Context())
			if tt.readStatus != mbimproto.StatusNone {
				if !errors.Is(err, tt.readStatus) {
					t.Fatalf("ListMessages() error = %v, want %v", err, tt.readStatus)
				}
				if _, ok := errors.AsType[*MessageListError](err); ok {
					t.Fatal("modem error was reported as a recoverable partial list")
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
	tests := []struct {
		name      string
		flash     bool
		firstPDU  []byte
		wantText  string
		wantError bool
	}{
		{name: "stored malformed PDU", firstPDU: []byte{0}, wantError: true},
		{name: "flash malformed PDU", flash: true, firstPDU: []byte{0}, wantError: true},
		{name: "stored undefined extension", firstPDU: messagingTestPDU([]byte("code \x1b\x00 hash")), wantText: "code @ hash"},
		{name: "flash undefined extension", flash: true, firstPDU: messagingTestPDU([]byte("code \x1b\x00 hash")), wantText: "code @ hash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := []mbimproto.SMSPDURecord{
				{MessageIndex: 1, MessageStatus: mbimproto.SMSStatusNew, PDU: tt.firstPDU},
				{MessageIndex: 2, MessageStatus: mbimproto.SMSStatusNew, PDU: messagingTestPDU([]byte("next message"))},
			}
			backend, modem := newMessagingTestBackend(t, records, mbimproto.StatusNone)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			stream, err := backend.WatchMessages(ctx)
			if err != nil {
				t.Fatalf("WatchMessages() error = %v", err)
			}
			for i, record := range records {
				cid := uint32(mbimproto.CIDSMSMessageStoreStatus)
				data := binary.LittleEndian.AppendUint32(nil, uint32(mbimproto.SMSStatusFlagNewMessage))
				data = binary.LittleEndian.AppendUint32(data, record.MessageIndex)
				if tt.flash {
					cid = mbimproto.CIDSMSRead
					// MBIM 1.0, table 10-86: flash messages have no store index.
					record.MessageIndex = 0
					data = messagingReadPayload([]mbimproto.SMSPDURecord{record})
				}
				if err := modem.write(messagingIndication(cid, data)); err != nil {
					t.Fatalf("emit indication: %v", err)
				}
				select {
				case result, ok := <-stream:
					if !ok {
						t.Fatal("WatchMessages() closed before the next message")
					}
					if i == 0 && tt.wantError {
						decodeErr, ok := errors.AsType[*MessageDecodeError](result.Err)
						if !ok || !bytes.Equal(decodeErr.PDU, record.PDU) {
							t.Fatalf("first result error = %v, want original malformed PDU", result.Err)
						}
						wantRef := MessageRef{Storage: MessageStorageDevice, ID: 1}
						if tt.flash {
							wantRef = MessageRef{}
						}
						if decodeErr.Ref != wantRef {
							t.Errorf("decode error reference = %+v, want %+v", decodeErr.Ref, wantRef)
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
					if tt.flash && (result.Value.ID != 0 || result.Value.Storage != MessageStorageUnknown || len(result.Value.Refs) != 0) {
						t.Errorf("flash message has stored-message identity: %+v", result.Value)
					}
				case <-time.After(time.Second):
					t.Fatal("WatchMessages() did not deliver the message")
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

type messagingTestModem struct {
	conn       net.Conn
	records    []mbimproto.SMSPDURecord
	readStatus mbimproto.Status
	writeMu    sync.Mutex
	wg         sync.WaitGroup
	err        error
}

type messagingTestDialer struct{ conn net.Conn }

func (d messagingTestDialer) Dial(context.Context) (mbimproto.Conn, error) { return d.conn, nil }

func newMessagingTestBackend(t *testing.T, records []mbimproto.SMSPDURecord, status mbimproto.Status) (*Backend, *messagingTestModem) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	modem := &messagingTestModem{conn: serverConn, records: records, readStatus: status}
	modem.wg.Add(1)
	go func() {
		defer modem.wg.Done()
		modem.err = modem.serve()
		// Closing the pipe also releases a client waiting after a test-server error.
		_ = serverConn.Close()
	}()
	t.Cleanup(func() {
		if err := serverConn.Close(); err != nil {
			t.Errorf("close test server: %v", err)
		}
		modem.wg.Wait()
		if modem.err != nil {
			t.Errorf("serve MBIM: %v", modem.err)
		}
	})
	client, err := mbimproto.Open(t.Context(), mbimproto.WithDialer(messagingTestDialer{conn: clientConn}))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return New(client, "/dev/test"), modem
}

func (m *messagingTestModem) serve() error {
	for {
		header := make([]byte, 12)
		if _, err := io.ReadFull(m.conn, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		length := int(binary.LittleEndian.Uint32(header[4:8]))
		if length < len(header) || length > 4096 {
			return fmt.Errorf("unexpected MBIM request length %d", length)
		}
		request := append(header, make([]byte, length-len(header))...)
		if _, err := io.ReadFull(m.conn, request[12:]); err != nil {
			return err
		}
		kind := mbimproto.MessageType(binary.LittleEndian.Uint32(request))
		if kind == mbimproto.MessageTypeOpen || kind == mbimproto.MessageTypeClose {
			response := append(slices.Clone(header), make([]byte, 4)...)
			binary.LittleEndian.PutUint32(response, uint32(kind)|0x80000000)
			binary.LittleEndian.PutUint32(response[4:8], uint32(len(response)))
			if err := m.write(response); err != nil {
				return err
			}
			if kind == mbimproto.MessageTypeClose {
				return nil
			}
			continue
		}
		if kind != mbimproto.MessageTypeCommand || len(request) < 48 {
			return errors.New("unexpected MBIM command frame")
		}
		var service [16]byte
		copy(service[:], request[20:36])
		cid := binary.LittleEndian.Uint32(request[36:40])
		status := mbimproto.StatusNone
		var data []byte
		switch {
		case service == mbimproto.ServiceBasicConnect && cid == mbimproto.CIDDeviceServices:
			data = make([]byte, 8)
		case service == mbimproto.ServiceBasicConnect && cid == mbimproto.CIDDeviceServiceSubscribeList:
			data = request[48:]
		case service == mbimproto.ServiceSMS && cid == mbimproto.CIDSMSRead:
			if len(request) < 60 {
				return errors.New("truncated SMS read request")
			}
			status = m.readStatus
			records := m.records
			if mbimproto.SMSReadFlag(binary.LittleEndian.Uint32(request[52:56])) == mbimproto.SMSReadFlagIndex {
				id := binary.LittleEndian.Uint32(request[56:60])
				records = nil
				for _, record := range m.records {
					if record.MessageIndex == id {
						records = append(records, record)
					}
				}
			}
			data = messagingReadPayload(records)
		default:
			status = mbimproto.StatusNoDeviceSupport
		}
		if err := m.write(messagingResponse(request, status, data)); err != nil {
			return err
		}
	}
}

func (m *messagingTestModem) write(data []byte) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	_, err := m.conn.Write(data)
	return err
}

func messagingResponse(request []byte, status mbimproto.Status, data []byte) []byte {
	response := make([]byte, 48)
	copy(response, request[:40])
	binary.LittleEndian.PutUint32(response, uint32(mbimproto.MessageTypeCommandDone))
	binary.LittleEndian.PutUint32(response[4:8], uint32(len(response)+len(data)))
	binary.LittleEndian.PutUint32(response[40:44], uint32(status))
	binary.LittleEndian.PutUint32(response[44:48], uint32(len(data)))
	return append(response, data...)
}

func messagingIndication(cid uint32, data []byte) []byte {
	frame := make([]byte, 44)
	binary.LittleEndian.PutUint32(frame, uint32(mbimproto.MessageTypeIndicateStatus))
	binary.LittleEndian.PutUint32(frame[4:8], uint32(len(frame)+len(data)))
	binary.LittleEndian.PutUint32(frame[12:16], 1)
	copy(frame[20:36], mbimproto.ServiceSMS[:])
	binary.LittleEndian.PutUint32(frame[36:40], cid)
	binary.LittleEndian.PutUint32(frame[40:44], uint32(len(data)))
	return append(frame, data...)
}

func messagingReadPayload(records []mbimproto.SMSPDURecord) []byte {
	data := make([]byte, 8+8*len(records))
	binary.LittleEndian.PutUint32(data, uint32(mbimproto.SMSFormatPDU))
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(records)))
	for i, record := range records {
		data = append(data, make([]byte, (4-len(data)%4)%4)...)
		offset := len(data)
		value := make([]byte, 16)
		binary.LittleEndian.PutUint32(value, record.MessageIndex)
		binary.LittleEndian.PutUint32(value[4:8], uint32(record.MessageStatus))
		binary.LittleEndian.PutUint32(value[8:12], 16)
		binary.LittleEndian.PutUint32(value[12:16], uint32(len(record.PDU)))
		value = append(value, record.PDU...)
		data = append(data, value...)
		binary.LittleEndian.PutUint32(data[8+i*8:], uint32(offset))
		binary.LittleEndian.PutUint32(data[12+i*8:], uint32(len(value)))
	}
	return append(data, make([]byte, (4-len(data)%4)%4)...)
}
