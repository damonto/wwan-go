package modem

import (
	"slices"
	"testing"
)

func TestNetworkInterfacesForControlPort(t *testing.T) {
	devices := []Device{
		{
			PhysicalPath: "/sys/modem0",
			Ports: []Port{
				{Type: PortQMI, Path: "/dev/cdc-wdm0"},
				{Type: PortMBIM, Path: "/dev/wwan0mbim0"},
				{Type: PortNetwork, Name: "wwan0", ControlPath: "/dev/cdc-wdm0"},
				{Type: PortNetwork, Name: "wwan1", ControlPath: "/dev/wwan0mbim0"},
				{Type: PortNetwork, Name: "orphan0"},
			},
		},
	}
	tests := []struct {
		name        string
		controlPort string
		want        []string
	}{
		{name: "QMI", controlPort: "/dev/cdc-wdm0", want: []string{"wwan0"}},
		{name: "MBIM", controlPort: "/dev/wwan0mbim0", want: []string{"wwan1"}},
		{name: "missing", controlPort: "/dev/cdc-wdm1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkInterfacesForControlPort(devices, tt.controlPort); !slices.Equal(got, tt.want) {
				t.Errorf("networkInterfacesForControlPort() = %v, want %v", got, tt.want)
			}
		})
	}
}
