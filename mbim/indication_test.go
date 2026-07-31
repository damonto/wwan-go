package mbim

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClientIndicationAPIs(t *testing.T) {
	tests := []struct {
		name  string
		watch bool
	}{
		{name: "next indication"},
		{name: "watch indications", watch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			payload := []byte{1, 2, 3, 4}
			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				_, err := serverConn.Write(mbimIndication(ServiceBasicConnect, CIDRadioState, payload))
				errCh <- err
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			var got Indication
			if tt.watch {
				indications, err := client.WatchIndications(ctx, ServiceBasicConnect, CIDRadioState)
				if err != nil {
					t.Fatalf("WatchIndications() error = %v", err)
				}
				select {
				case got = <-indications:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				var err error
				got, err = client.NextIndication(ctx, ServiceBasicConnect, CIDRadioState)
				if err != nil {
					t.Fatalf("NextIndication() error = %v", err)
				}
			}

			if got.ServiceID != ServiceBasicConnect || got.CommandID != CIDRadioState {
				t.Fatalf("indication = service %x CID %d", got.ServiceID, got.CommandID)
			}
			if !bytes.Equal(got.InformationBuffer, payload) {
				t.Fatalf("InformationBuffer = %x, want %x", got.InformationBuffer, payload)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientNextIndicationFiltersAndQueues(t *testing.T) {
	tests := []struct {
		name         string
		firstCID     uint32
		secondCID    uint32
		requestedCID uint32
	}{
		{
			name:         "different CID remains queued",
			firstCID:     CIDSignalState,
			secondCID:    CIDRadioState,
			requestedCID: CIDRadioState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				if _, err := serverConn.Write(mbimIndication(ServiceBasicConnect, tt.firstCID, []byte{1})); err != nil {
					errCh <- err
					return
				}
				if _, err := serverConn.Write(mbimIndication(ServiceBasicConnect, tt.secondCID, []byte{2})); err != nil {
					errCh <- err
					return
				}
			}()

			client := &Client{conn: clientConn}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			got, err := client.NextIndication(ctx, ServiceBasicConnect, tt.requestedCID)
			if err != nil {
				t.Fatalf("NextIndication(requested) error = %v", err)
			}
			if got.CommandID != tt.secondCID || !bytes.Equal(got.InformationBuffer, []byte{2}) {
				t.Fatalf("requested indication = CID %d data %x", got.CommandID, got.InformationBuffer)
			}

			queued, err := client.NextIndication(ctx, ServiceBasicConnect, tt.firstCID)
			if err != nil {
				t.Fatalf("NextIndication(queued) error = %v", err)
			}
			if queued.CommandID != tt.firstCID || !bytes.Equal(queued.InformationBuffer, []byte{1}) {
				t.Fatalf("queued indication = CID %d data %x", queued.CommandID, queued.InformationBuffer)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientWatchIndicationResultsReportsReceiverError(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "connection closes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			results, err := client.WatchIndicationResults(ctx, ServiceBasicConnect, CIDRadioState)
			if err != nil {
				t.Fatalf("WatchIndicationResults() error = %v", err)
			}
			if err := serverConn.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			select {
			case result := <-results:
				if result.Err == nil || !strings.Contains(result.Err.Error(), "receiving MBIM message") {
					t.Fatalf("result error = %v, want receiver error", result.Err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestClientNextIndicationReportsReceiverError(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "connection closes while waiting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })
			go func() { _ = serverConn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := (&Client{conn: clientConn}).NextIndication(ctx, ServiceBasicConnect, CIDRadioState)
			if err == nil || !strings.Contains(err.Error(), "receiving MBIM message") {
				t.Fatalf("NextIndication() error = %v, want receiver error", err)
			}
		})
	}
}

func TestClientWatchSTKPACResultsReportsDecodeError(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "truncated PAC", payload: []byte{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close() })

			errCh := make(chan error, 1)
			go func() {
				defer close(errCh)
				defer serverConn.Close()
				_, err := serverConn.Write(mbimIndication(ServiceSTK, CIDSTKPAC, tt.payload))
				errCh <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client := &Client{conn: clientConn}
			results, err := client.WatchSTKPACResults(ctx)
			if err != nil {
				t.Fatalf("WatchSTKPACResults() error = %v", err)
			}
			select {
			case result := <-results:
				if result.Err == nil || !strings.Contains(result.Err.Error(), "payload is truncated") {
					t.Fatalf("result error = %v, want truncated payload", result.Err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
		})
	}
}
