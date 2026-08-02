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

func TestNetworkPortsForControlPortUsesQCOMPlatformAssociation(t *testing.T) {
	network := Port{
		Type:        PortNetwork,
		Name:        "rmnet0",
		Driver:      bamDMUXDriverName,
		ControlPath: "/dev/rpmsg0",
		QMIEndpoint: QMIEndpoint{Type: QMIEndpointBAMDMUX, InterfaceNumber: 3, SIOPort: 0x0e07},
	}
	devices := []Device{{
		Bus: BusPlatform,
		Ports: []Port{
			{Type: PortQMI, Path: "/dev/rpmsg0"},
			{Type: PortQMI, Path: "/dev/rpmsg1"},
			network,
		},
	}}
	tests := []struct {
		name        string
		controlPort string
		want        []Port
	}{
		{name: "exact control", controlPort: "/dev/rpmsg0", want: []Port{network}},
		{name: "second control on same device", controlPort: "/dev/rpmsg1", want: []Port{network}},
		{name: "unknown control", controlPort: "/dev/rpmsg2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkPortsForControlPort(devices, tt.controlPort); !slices.Equal(got, tt.want) {
				t.Errorf("networkPortsForControlPort() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSelectNetworkPortFromDevicesPreservesEndpoint(t *testing.T) {
	endpoint := QMIEndpoint{Type: QMIEndpointBAMDMUX, InterfaceNumber: 3, SIOPort: 0x0e07}
	devices := []Device{{
		Ports: []Port{
			{Type: PortQMI, Path: "/dev/rpmsg0"},
			{Type: PortNetwork, Name: "rmnet0", ControlPath: "/dev/rpmsg0", QMIEndpoint: endpoint},
		},
	}}
	tests := []struct {
		name      string
		devices   []Device
		control   string
		requested string
		want      Port
		wantErr   bool
	}{
		{name: "automatic", devices: devices, control: "/dev/rpmsg0", want: devices[0].Ports[1]},
		{name: "requested", devices: devices, control: "/dev/rpmsg0", requested: "rmnet0", want: devices[0].Ports[1]},
		{name: "wrong association", devices: devices, control: "/dev/rpmsg0", requested: "rmnet1", wantErr: true},
		{name: "caller supplied generic interface", control: "/dev/cdc-wdm0", requested: "wwan0", want: Port{Type: PortNetwork, Name: "wwan0", Subsystem: "net"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectNetworkPortFromDevices(tt.devices, tt.control, tt.requested)
			if (err != nil) != tt.wantErr {
				t.Fatalf("selectNetworkPortFromDevices() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("selectNetworkPortFromDevices() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSelectNetworkPortFromDevicesChoosesLowestEndpoint(t *testing.T) {
	tests := []struct {
		name  string
		ports []Port
		want  string
	}{
		{
			name: "endpoint number wins",
			ports: []Port{
				{Type: PortNetwork, Name: "wwan1", ControlPath: "/dev/wwan0qmi0", QMIEndpoint: QMIEndpoint{Type: QMIEndpointBAMDMUX, InterfaceNumber: 1, SIOPort: 0x0e05}},
				{Type: PortNetwork, Name: "wwan0", ControlPath: "/dev/wwan0qmi0", QMIEndpoint: QMIEndpoint{Type: QMIEndpointBAMDMUX, InterfaceNumber: 0, SIOPort: 0x0e04}},
			},
			want: "wwan0",
		},
		{
			name: "name breaks endpoint tie",
			ports: []Port{
				{Type: PortNetwork, Name: "wwan1", ControlPath: "/dev/wwan0qmi0"},
				{Type: PortNetwork, Name: "wwan0", ControlPath: "/dev/wwan0qmi0"},
			},
			want: "wwan0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := []Device{{Ports: append([]Port{{Type: PortQMI, Path: "/dev/wwan0qmi0"}}, tt.ports...)}}
			got, err := selectNetworkPortFromDevices(devices, "/dev/wwan0qmi0", "")
			if err != nil {
				t.Fatalf("selectNetworkPortFromDevices() error = %v", err)
			}
			if got.Name != tt.want {
				t.Fatalf("selected port = %#v, want %s", got, tt.want)
			}
		})
	}
}
