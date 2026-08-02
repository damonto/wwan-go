//go:build linux

package modem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverQCOMSoC(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		devPort *int
		want    QMIEndpoint
		wantNet string
	}{
		{
			name:    "BAM-DMUX maps dev_port to SIO",
			driver:  bamDMUXDriverName,
			devPort: intPointer(3),
			want: QMIEndpoint{
				Type:            QMIEndpointBAMDMUX,
				InterfaceNumber: 3,
				SIOPort:         bamDMUXSIOPort0 + 3,
			},
			wantNet: "rmnet0",
		},
		{
			name:    "IPA uses embedded endpoint",
			driver:  ipaDriverName,
			want:    QMIEndpoint{Type: QMIEndpointEmbedded, InterfaceNumber: 1},
			wantNet: "rmnet_ipa0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sysRoot := filepath.Join(root, "sys")
			devRoot := filepath.Join(root, "dev")
			modemPath := filepath.Join(sysRoot, "devices", "platform", "soc", "modem")
			addPlatformDriverLink(t, sysRoot, modemPath, qcomSoCDriverName)
			addQCOMRPMsgFixture(t, sysRoot, modemPath, "rpmsg0", "DATA5_CNTL")
			addQCOMRPMsgFixture(t, sysRoot, modemPath, "rpmsg1", "DATA5")
			addQCOMRPMsgFixture(t, sysRoot, modemPath, "rpmsg2", "DATA40_CNTL")
			addQCOMNetworkFixture(t, sysRoot, modemPath, tt.wantNet, tt.driver, tt.devPort)

			devices, err := discover(context.Background(), discoveryConfig{sysRoot: sysRoot, devRoot: devRoot})
			if err != nil {
				t.Fatalf("discover() error = %v", err)
			}
			if len(devices) != 1 {
				t.Fatalf("len(devices) = %d, want 1", len(devices))
			}
			device := devices[0]
			if device.Bus != BusPlatform || device.PhysicalPath != modemPath {
				t.Fatalf("device = bus %d path %q, want platform %q", device.Bus, device.PhysicalPath, modemPath)
			}
			ports := make(map[string]Port, len(device.Ports))
			for _, port := range device.Ports {
				ports[port.Name] = port
			}
			if port := ports["rpmsg0"]; port.Type != PortQMI || port.Subsystem != "rpmsg" || port.Driver != "rpmsg_chrdev" {
				t.Errorf("rpmsg0 = %#v, want QMI control", port)
			}
			if got := kernelProtocol(ports["rpmsg0"].SysPath); got != ProtocolQMI {
				t.Errorf("kernelProtocol(rpmsg0) = %s, want QMI", got)
			}
			if err := validatePortMetadata(ports["rpmsg0"]); err != nil {
				t.Errorf("validatePortMetadata(rpmsg0) error = %v", err)
			}
			if port := ports["rpmsg1"]; port.Type != PortAT || port.Role != PortRoleSecondary {
				t.Errorf("rpmsg1 = %#v, want secondary AT", port)
			}
			if _, ok := ports["rpmsg2"]; ok {
				t.Error("DATA40 control port was not ignored")
			}
			network := ports[tt.wantNet]
			if network.Type != PortNetwork || network.Driver != tt.driver || network.QMIEndpoint != tt.want {
				t.Errorf("network port = %#v, want driver %q endpoint %#v", network, tt.driver, tt.want)
			}
			if network.ControlPath != filepath.Join(devRoot, "rpmsg0") {
				t.Errorf("network ControlPath = %q, want rpmsg0", network.ControlPath)
			}
		})
	}
}

func TestQCOMSoCEndpointRejectsInvalidBAMDMUXPort(t *testing.T) {
	tests := []struct {
		name  string
		value string
		write bool
	}{
		{name: "missing"},
		{name: "not a number", value: "x\n", write: true},
		{name: "outside range", value: "8\n", write: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysPath := t.TempDir()
			if tt.write {
				if err := os.WriteFile(filepath.Join(sysPath, "dev_port"), []byte(tt.value), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := qcomSoCEndpoint(sysPath, bamDMUXDriverName); err == nil {
				t.Fatal("qcomSoCEndpoint() error = nil, want error")
			}
		})
	}
}

func TestDiscoverQCOMSoCSkipsInvalidBAMDMUXPort(t *testing.T) {
	tests := []struct {
		name       string
		badDevPort *int
	}{
		{name: "missing dev_port"},
		{name: "out of range dev_port", badDevPort: intPointer(8)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sysRoot := filepath.Join(root, "sys")
			devRoot := filepath.Join(root, "dev")
			modemPath := filepath.Join(sysRoot, "devices", "platform", "soc", "modem")
			addPlatformDriverLink(t, sysRoot, modemPath, qcomSoCDriverName)
			addQCOMRPMsgFixture(t, sysRoot, modemPath, "rpmsg0", "DATA5_CNTL")
			addQCOMNetworkFixture(t, sysRoot, modemPath, "rmnet_bad", bamDMUXDriverName, tt.badDevPort)
			addQCOMNetworkFixture(t, sysRoot, modemPath, "rmnet_good", bamDMUXDriverName, intPointer(2))

			devices, err := discover(context.Background(), discoveryConfig{sysRoot: sysRoot, devRoot: devRoot})
			if err != nil {
				t.Fatalf("discover() error = %v", err)
			}
			if len(devices) != 1 {
				t.Fatalf("len(devices) = %d, want 1", len(devices))
			}
			ports := make(map[string]Port, len(devices[0].Ports))
			for _, port := range devices[0].Ports {
				ports[port.Name] = port
			}
			if _, ok := ports["rmnet_bad"]; ok {
				t.Error("invalid BAM-DMUX port was not skipped")
			}
			want := QMIEndpoint{
				Type:            QMIEndpointBAMDMUX,
				InterfaceNumber: 2,
				SIOPort:         bamDMUXSIOPort0 + 2,
			}
			if port := ports["rmnet_good"]; port.QMIEndpoint != want {
				t.Errorf("rmnet_good endpoint = %#v, want %#v", port.QMIEndpoint, want)
			}
			if port := ports["rpmsg0"]; port.Type != PortQMI {
				t.Errorf("rpmsg0 = %#v, want QMI control port", port)
			}
		})
	}
}

func addQCOMRPMsgFixture(t *testing.T, sysRoot, modemPath, name, service string) {
	t.Helper()
	devicePath := filepath.Join(modemPath, name)
	entryPath := filepath.Join(sysRoot, "class", "rpmsg", name)
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(entryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entryPath, "name"), []byte(service+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// RPMSG endpoints may have a nearer driver of their own. QCOM matching
	// must still find qcom-q6v5-mss on the parent chain, like udev DRIVERS.
	addPlatformDriverLink(t, sysRoot, devicePath, "rpmsg_chrdev")
	if err := os.Symlink(devicePath, filepath.Join(entryPath, "device")); err != nil {
		t.Fatal(err)
	}
}

func addQCOMNetworkFixture(t *testing.T, sysRoot, modemPath, name, driver string, devPort *int) {
	t.Helper()
	devicePath := filepath.Join(modemPath, name)
	entryPath := filepath.Join(sysRoot, "class", "net", name)
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(entryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	addPlatformDriverLink(t, sysRoot, devicePath, driver)
	if err := os.Symlink(devicePath, filepath.Join(entryPath, "device")); err != nil {
		t.Fatal(err)
	}
	if devPort != nil {
		if err := os.WriteFile(filepath.Join(entryPath, "dev_port"), fmt.Appendf(nil, "%d\n", *devPort), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func addPlatformDriverLink(t *testing.T, sysRoot, devicePath, driver string) {
	t.Helper()
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	driverTarget := filepath.Join(sysRoot, "bus", "platform", "drivers", driver)
	if err := os.MkdirAll(driverTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driverTarget, filepath.Join(devicePath, "driver")); err != nil {
		t.Fatal(err)
	}
}

func intPointer(value int) *int { return &value }
