package modem

import (
	"context"
	"errors"
	"testing"

	mbimproto "github.com/damonto/wwan-go/mbim"
	"github.com/damonto/wwan-go/qcom"
)

func TestModemQMIClientUsesResolvedEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		access   Access
		closed   bool
		wantErr  bool
	}{
		{name: "direct", protocol: ProtocolQMI, access: AccessDirect},
		{name: "proxy", protocol: ProtocolQMI, access: AccessProxy},
		{name: "wrong protocol", protocol: ProtocolMBIM, access: AccessDirect, wantErr: true},
		{name: "unresolved access", protocol: ProtocolQMI, access: AccessAuto, wantErr: true},
		{name: "invalid access", protocol: ProtocolQMI, access: Access(99), wantErr: true},
		{name: "closed", protocol: ProtocolQMI, access: AccessDirect, closed: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldOpen := openModemQMIClient
			t.Cleanup(func() { openModemQMIClient = oldOpen })

			wantClient := new(qcom.Client)
			called := false
			openModemQMIClient = func(_ context.Context, device string, access Access, slot uint8) (*qcom.Client, error) {
				called = true
				if device != "/dev/cdc-wdm0" || access != tt.access || slot != 2 {
					t.Errorf("QMI endpoint = (%q, %s, %d), want (/dev/cdc-wdm0, %s, 2)", device, access, slot, tt.access)
				}
				return wantClient, nil
			}

			m := newModem("/dev/cdc-wdm0", tt.protocol, tt.access, unsupportedBackend{})
			if tt.closed {
				if err := m.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
			got, err := m.QMIClient(context.Background(), 2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("QMIClient() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				if called {
					t.Fatal("QMIClient() called opener after validation error")
				}
				return
			}
			if !called || got != wantClient {
				t.Fatalf("QMIClient() = %p, called = %t; want %p, true", got, called, wantClient)
			}
		})
	}
}

func TestModemMBIMClientUsesResolvedEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		access   Access
		closed   bool
		wantErr  bool
	}{
		{name: "direct", protocol: ProtocolMBIM, access: AccessDirect},
		{name: "proxy", protocol: ProtocolMBIM, access: AccessProxy},
		{name: "wrong protocol", protocol: ProtocolQMI, access: AccessDirect, wantErr: true},
		{name: "unresolved access", protocol: ProtocolMBIM, access: AccessAuto, wantErr: true},
		{name: "invalid access", protocol: ProtocolMBIM, access: Access(99), wantErr: true},
		{name: "closed", protocol: ProtocolMBIM, access: AccessDirect, closed: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldOpen := openModemMBIMClient
			t.Cleanup(func() { openModemMBIMClient = oldOpen })

			wantClient := new(mbimproto.Client)
			called := false
			openModemMBIMClient = func(_ context.Context, device string, access Access, slot uint8) (*mbimproto.Client, error) {
				called = true
				if device != "/dev/cdc-wdm0" || access != tt.access || slot != 2 {
					t.Errorf("MBIM endpoint = (%q, %s, %d), want (/dev/cdc-wdm0, %s, 2)", device, access, slot, tt.access)
				}
				return wantClient, nil
			}

			m := newModem("/dev/cdc-wdm0", tt.protocol, tt.access, unsupportedBackend{})
			if tt.closed {
				if err := m.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
			got, err := m.MBIMClient(context.Background(), 2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MBIMClient() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				if called {
					t.Fatal("MBIMClient() called opener after validation error")
				}
				return
			}
			if !called || got != wantClient {
				t.Fatalf("MBIMClient() = %p, called = %t; want %p, true", got, called, wantClient)
			}
		})
	}
}

func TestModemProtocolClientReturnsOpenError(t *testing.T) {
	openErr := errors.New("open")
	tests := []struct {
		name     string
		protocol Protocol
		run      func(*Modem) error
	}{
		{
			name:     "QMI",
			protocol: ProtocolQMI,
			run: func(m *Modem) error {
				oldOpen := openModemQMIClient
				defer func() { openModemQMIClient = oldOpen }()
				openModemQMIClient = func(context.Context, string, Access, uint8) (*qcom.Client, error) {
					return nil, openErr
				}
				_, err := m.QMIClient(context.Background(), 1)
				return err
			},
		},
		{
			name:     "MBIM",
			protocol: ProtocolMBIM,
			run: func(m *Modem) error {
				oldOpen := openModemMBIMClient
				defer func() { openModemMBIMClient = oldOpen }()
				openModemMBIMClient = func(context.Context, string, Access, uint8) (*mbimproto.Client, error) {
					return nil, openErr
				}
				_, err := m.MBIMClient(context.Background(), 1)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModem("/dev/cdc-wdm0", tt.protocol, AccessDirect, unsupportedBackend{})
			if err := tt.run(m); !errors.Is(err, openErr) {
				t.Fatalf("client open error = %v, want %v", err, openErr)
			}
		})
	}
}
