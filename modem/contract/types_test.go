package contract

import "testing"

func TestPortProtocol(t *testing.T) {
	tests := []struct {
		name string
		port Port
		want Protocol
	}{
		{name: "QMI", port: Port{Type: PortQMI}, want: ProtocolQMI},
		{name: "MBIM", port: Port{Type: PortMBIM}, want: ProtocolMBIM},
		{name: "unknown", port: Port{Type: PortUnknown}, want: ProtocolUnknown},
		{name: "AT", port: Port{Type: PortAT}, want: ProtocolUnknown},
		{name: "network", port: Port{Type: PortNetwork}, want: ProtocolUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.port.Protocol(); got != tt.want {
				t.Errorf("Port.Protocol() = %s, want %s", got, tt.want)
			}
		})
	}
}
