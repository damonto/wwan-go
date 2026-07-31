package mbim

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestClientReportsFragmentFaultAndRecovers(t *testing.T) {
	tests := []struct {
		name       string
		frameIndex int
		wantStatus ProtocolError
	}{
		{name: "timeout", frameIndex: 0, wantStatus: ProtocolErrorTimeoutFragment},
		{name: "out of sequence", frameIndex: 1, wantStatus: ProtocolErrorFragmentOutOfSequence},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			fragmented := mbimIndication(ServiceBasicConnect, CIDSignalState, bytes.Repeat([]byte{0xab}, 96))
			binary.LittleEndian.PutUint32(fragmented[8:12], 37)
			frames, err := (fragmentedMessage{data: fragmented, maxFrameSize: 64}).Frames()
			if err != nil {
				t.Fatalf("Frames() error = %v", err)
			}
			if len(frames) < 2 {
				t.Fatalf("Frames() count = %d, want at least 2", len(frames))
			}

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if _, err := serverConn.Write(frames[tt.frameIndex]); err != nil {
					errCh <- err
					return
				}
				hostError, err := readFrame(serverConn)
				if err != nil {
					errCh <- err
					return
				}
				if err := checkHostErrorFrame(hostError, 37, tt.wantStatus); err != nil {
					errCh <- err
					return
				}
				_, err = serverConn.Write(mbimIndication(ServiceBasicConnect, CIDRadioState, []byte{0x7f}))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			got, err := client.NextIndication(ctx, ServiceBasicConnect, CIDRadioState)
			if err != nil {
				t.Fatalf("NextIndication() error = %v", err)
			}
			if !bytes.Equal(got.InformationBuffer, []byte{0x7f}) {
				t.Fatalf("InformationBuffer = %X, want 7F", got.InformationBuffer)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestTransmitReportsFragmentFault(t *testing.T) {
	tests := []struct {
		name       string
		frameIndex int
		wantStatus ProtocolError
	}{
		{name: "timeout", frameIndex: 0, wantStatus: ProtocolErrorTimeoutFragment},
		{name: "out of sequence", frameIndex: 1, wantStatus: ProtocolErrorFragmentOutOfSequence},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			response := mbimCommandDone(1, ServiceBasicConnect, CIDSubscriberReadyStatus, bytes.Repeat([]byte{0xef}, 96))
			frames, err := (fragmentedMessage{data: response, maxFrameSize: 64}).Frames()
			if err != nil {
				t.Fatalf("Frames() error = %v", err)
			}

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if err := expectMBIMCommandWithService(
					serverConn,
					1,
					ServiceBasicConnect,
					CIDSubscriberReadyStatus,
					CommandTypeQuery,
					nil,
				); err != nil {
					errCh <- err
					return
				}
				if _, err := serverConn.Write(frames[tt.frameIndex]); err != nil {
					errCh <- err
					return
				}
				hostError, err := readFrame(serverConn)
				if err != nil {
					errCh <- err
					return
				}
				errCh <- checkHostErrorFrame(hostError, 1, tt.wantStatus)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			request := (&SubscriberReadyStatusRequest{TransactionID: 1}).Request()
			err = request.Transmit(ctx, clientConn)
			if !errors.Is(err, tt.wantStatus) {
				t.Fatalf("Transmit() error = %v, want %v", err, tt.wantStatus)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientAllowsUnrelatedFrameBetweenFragments(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })

	payload := bytes.Repeat([]byte{0xcd}, 96)
	fragmented := mbimIndication(ServiceBasicConnect, CIDSignalState, payload)
	frames, err := (fragmentedMessage{data: fragmented, maxFrameSize: 64}).Frames()
	if err != nil {
		t.Fatalf("Frames() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		defer serverConn.Close()
		if _, err := serverConn.Write(frames[0]); err != nil {
			errCh <- err
			return
		}
		if _, err := serverConn.Write(mbimIndication(ServiceBasicConnect, CIDRadioState, []byte{1})); err != nil {
			errCh <- err
			return
		}
		for _, frame := range frames[1:] {
			if _, err := serverConn.Write(frame); err != nil {
				errCh <- err
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := &Client{conn: clientConn}
	radio, err := client.NextIndication(ctx, ServiceBasicConnect, CIDRadioState)
	if err != nil {
		t.Fatalf("NextIndication(radio) error = %v", err)
	}
	if !bytes.Equal(radio.InformationBuffer, []byte{1}) {
		t.Fatalf("radio payload = %X, want 01", radio.InformationBuffer)
	}
	signal, err := client.NextIndication(ctx, ServiceBasicConnect, CIDSignalState)
	if err != nil {
		t.Fatalf("NextIndication(signal) error = %v", err)
	}
	if !bytes.Equal(signal.InformationBuffer, payload) {
		t.Fatalf("signal payload length = %d, want %d", len(signal.InformationBuffer), len(payload))
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func checkHostErrorFrame(frame []byte, transactionID uint32, status ProtocolError) error {
	if len(frame) != 16 {
		return fmt.Errorf("host error length = %d, want 16", len(frame))
	}
	if got := MessageType(binary.LittleEndian.Uint32(frame[0:4])); got != MessageTypeHostError {
		return fmt.Errorf("message type = %#x, want %#x", got, MessageTypeHostError)
	}
	if got := binary.LittleEndian.Uint32(frame[8:12]); got != transactionID {
		return fmt.Errorf("transaction ID = %d, want %d", got, transactionID)
	}
	if got := ProtocolError(binary.LittleEndian.Uint32(frame[12:16])); got != status {
		return fmt.Errorf("host error status = %d, want %d", got, status)
	}
	return nil
}
