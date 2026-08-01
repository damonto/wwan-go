package qrtr

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/damonto/wwan-go/qcom"
)

type fakeDialer struct {
	mu       sync.Mutex
	conn     packetConn
	err      error
	service  qcom.ServiceType
	services []qcom.ServiceType
}

func (d *fakeDialer) Dial(_ context.Context, service qcom.ServiceType) (packetConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.service = service
	d.services = append(d.services, service)
	return d.conn, d.err
}

func (d *fakeDialer) dialedServices() []qcom.ServiceType {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.services)
}

type fakeConn struct{}

func (fakeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (fakeConn) Close() error                     { return nil }
func (fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestOpenUsesDialer(t *testing.T) {
	tests := []struct {
		name    string
		service qcom.ServiceType
		dialer  *fakeDialer
		wantErr bool
	}{
		{"default service", 0, &fakeDialer{conn: fakeConn{}}, false},
		{"custom service", qcom.ServiceCAT, &fakeDialer{conn: fakeConn{}}, false},
		{"dial error", 0, &fakeDialer{err: errors.New("boom")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{WithDialer(tt.dialer)}
			if tt.service != 0 {
				opts = append(opts, WithService(tt.service))
			}
			got, err := Open(context.Background(), opts...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Open() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if got == nil {
				t.Fatal("Open() = nil, want transport")
			}
			wantService := tt.service
			if wantService == 0 {
				wantService = qcom.ServiceUIM
			}
			if tt.dialer.service != wantService {
				t.Fatalf("service = %d, want %d", tt.dialer.service, wantService)
			}
		})
	}
}

func TestOpenRejectsNilDialer(t *testing.T) {
	if _, err := Open(context.Background(), WithDialer(nil)); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}

func TestTransportLazilyOpensMultipleServices(t *testing.T) {
	tests := []struct {
		name     string
		services []qcom.ServiceType
		want     []qcom.ServiceType
	}{
		{
			name:     "legacy and extended services",
			services: []qcom.ServiceType{qcom.ServiceWDS, qcom.ServiceNAS, qcom.ServiceWDS, 0x0302},
			want:     []qcom.ServiceType{qcom.ServiceUIM, qcom.ServiceWDS, qcom.ServiceNAS, 0x0302},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer := &fakeDialer{conn: fakeConn{}}
			transport, err := Open(context.Background(), WithDialer(dialer))
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer transport.Close()

			for _, service := range tt.services {
				clientID, err := transport.ClientID(context.Background(), service)
				if err != nil {
					t.Fatalf("ClientID(0x%X) error = %v", service, err)
				}
				if clientID != 0 {
					t.Fatalf("ClientID(0x%X) = %d, want 0", service, clientID)
				}
			}

			if got := dialer.dialedServices(); !slices.Equal(got, tt.want) {
				t.Fatalf("dialed services = %v, want %v", got, tt.want)
			}
		})
	}
}
