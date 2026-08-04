package qmi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
	"github.com/damonto/wwan-go/qcom/tlv"
)

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
			payload:  []byte{byte(qcom.WMSAckRequired), 1, 0, 0, 0, byte(qcom.WMSMessageFormatGWPointToPoint), 1, 0, 0},
			wantACKs: 1,
		},
		{
			name:     "transfer route without ACK",
			kind:     0x11,
			payload:  []byte{byte(qcom.WMSAckNotRequired), 1, 0, 0, 0, byte(qcom.WMSMessageFormatGWPointToPoint), 1, 0, 0},
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
}

func (t *messagingTestTransport) QMIService() qcom.ServiceType {
	return qcom.ServiceWMS
}

func (t *messagingTestTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.mu.Lock()
	if req.MessageID == qcom.MessageWMSSendAck {
		t.acks++
	}
	t.mu.Unlock()

	response := qcom.Response{
		Service:       req.Service,
		ClientID:      req.ClientID,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		TLVs:          tlv.TLVs{tlv.Bytes(0x02, []byte{0, 0, 0, 0})},
	}
	if req.MessageID == qcom.MessageWMSRawRead {
		response.TLVs = append(response.TLVs, tlv.Bytes(0x01, []byte{
			byte(qcom.WMSTagMTNotRead),
			byte(qcom.WMSMessageFormatGWPointToPoint),
			1, 0, 0,
		}))
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

func (t *messagingTestTransport) Close() error { return nil }
