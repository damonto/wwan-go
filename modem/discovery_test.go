//go:build linux

package modem

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDiscoverFixture(t *testing.T) {
	root := t.TempDir()
	sysRoot := filepath.Join(root, "sys")
	devRoot := filepath.Join(root, "dev")
	if err := os.MkdirAll(devRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	addDiscoveryFixture(t, sysRoot, "wwan", "wwan0qmi", "QMI", "qmi_wwan", []string{"wwan0"})
	addDiscoveryFixture(t, sysRoot, "usbmisc", "cdc-wdm1", "MBIM", "cdc_mbim", []string{"wwan1", "wwan1.1"})

	devices, err := discover(context.Background(), sysRoot, devRoot, false)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}
	tests := []struct {
		name       string
		index      int
		protocol   Protocol
		driver     string
		interfaces []string
	}{
		{name: "MBIM", index: 0, protocol: ProtocolMBIM, driver: "cdc_mbim", interfaces: []string{"wwan1", "wwan1.1"}},
		{name: "QMI", index: 1, protocol: ProtocolQMI, driver: "qmi_wwan", interfaces: []string{"wwan0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := devices[tt.index]
			if device.Protocol != tt.protocol || device.Driver != tt.driver || !reflect.DeepEqual(device.NetworkInterfaces, tt.interfaces) {
				t.Errorf("device = %+v, want protocol %s driver %s interfaces %v", device, tt.protocol, tt.driver, tt.interfaces)
			}
		})
	}
}

func TestDiscoverAggregatesUSBControlAndATPorts(t *testing.T) {
	root := t.TempDir()
	sysRoot := filepath.Join(root, "sys")
	devRoot := filepath.Join(root, "dev")
	usbDevice := filepath.Join(sysRoot, "devices", "pci0000:00", "usb1", "1-2")
	controlInterface := filepath.Join(usbDevice, "1-2:1.4")
	atInterface := filepath.Join(usbDevice, "1-2:1.2")
	for _, path := range []string{controlInterface, atInterface, filepath.Join(sysRoot, "class", "usbmisc"), filepath.Join(sysRoot, "class", "tty")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range map[string]string{"idVendor": "2c7c\n", "idProduct": "0800\n"} {
		if err := os.WriteFile(filepath.Join(usbDevice, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	controlEntry := filepath.Join(sysRoot, "class", "usbmisc", "cdc-wdm0")
	if err := os.MkdirAll(controlEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controlEntry, "type"), []byte("QMI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(controlInterface, filepath.Join(controlEntry, "device")); err != nil {
		t.Fatal(err)
	}
	atEntry := filepath.Join(sysRoot, "class", "tty", "ttyUSB2")
	if err := os.MkdirAll(atEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(atInterface, filepath.Join(atEntry, "device")); err != nil {
		t.Fatal(err)
	}

	devices, err := discover(context.Background(), sysRoot, devRoot, false)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}
	device := devices[0]
	if device.PhysicalPath != usbDevice {
		t.Fatalf("PhysicalPath = %q, want %q", device.PhysicalPath, usbDevice)
	}
	wantPorts := []Port{
		{Type: PortQMI, Name: "cdc-wdm0", Path: filepath.Join(devRoot, "cdc-wdm0"), SysPath: controlEntry},
		{Type: PortAT, Name: "ttyUSB2", Path: filepath.Join(devRoot, "ttyUSB2"), SysPath: atEntry},
	}
	if !reflect.DeepEqual(device.Ports, wantPorts) {
		t.Fatalf("Ports = %#v, want %#v", device.Ports, wantPorts)
	}
}

func TestDiffDevices(t *testing.T) {
	current := devicesByPath([]Device{
		{Path: "/dev/a", Protocol: ProtocolQMI},
		{Path: "/dev/b", Protocol: ProtocolMBIM},
	})
	next := devicesByPath([]Device{
		{Path: "/dev/b", Protocol: ProtocolMBIM, NetworkInterfaces: []string{"wwan0"}},
		{Path: "/dev/c", Protocol: ProtocolQMI},
	})
	want := []DeviceEvent{
		{Type: DeviceRemoved, Device: Device{Path: "/dev/a", Protocol: ProtocolQMI}},
		{Type: DeviceChanged, Device: Device{Path: "/dev/b", Protocol: ProtocolMBIM, NetworkInterfaces: []string{"wwan0"}}},
		{Type: DeviceAdded, Device: Device{Path: "/dev/c", Protocol: ProtocolQMI}},
	}
	if got := diffDevices(current, next); !reflect.DeepEqual(got, want) {
		t.Errorf("diffDevices() = %#v, want %#v", got, want)
	}
}

func TestReconcileDeviceEventsPreservesSamePathReconnect(t *testing.T) {
	tests := []struct {
		name    string
		current Device
		removal kernelUevent
	}{
		{
			name: "usbmisc control node",
			current: Device{
				Path:         "/dev/cdc-wdm0",
				PhysicalPath: "/sys/devices/modem-1",
				Protocol:     ProtocolQMI,
				Ports: []Port{
					{Type: PortQMI, Name: "cdc-wdm0", Path: "/dev/cdc-wdm0", SysPath: "/sys/class/usbmisc/cdc-wdm0"},
				},
			},
			removal: kernelUevent{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := devicesByPath([]Device{tt.current})
			next, got := reconcileDeviceEvents(current, []kernelUevent{tt.removal}, []Device{tt.current})
			want := []DeviceEvent{
				{Type: DeviceRemoved, Device: tt.current},
				{Type: DeviceAdded, Device: tt.current},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("reconcileDeviceEvents() events = %#v, want %#v", got, want)
			}
			if !reflect.DeepEqual(next, devicesByPath([]Device{tt.current})) {
				t.Fatalf("reconcileDeviceEvents() current = %#v, want final device", next)
			}
		})
	}
}

func TestDeviceUeventQueueCoalescesNoiseAndRetainsRemoval(t *testing.T) {
	tests := []struct {
		name         string
		noiseCount   int
		removalCount int
	}{
		{name: "slow consumer during event storm", noiseCount: 10_000, removalCount: 10_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := newDeviceUeventQueue()
			done := make(chan struct{})
			go func() {
				defer close(done)
				for range tt.noiseCount {
					queue.push(kernelUevent{action: "change", subsystem: "net", devName: "wwan0"})
				}
				for range tt.removalCount {
					queue.push(kernelUevent{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm1"})
				}
			}()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("uevent producer blocked behind a slow consumer")
			}
			batch := queue.take()
			if !batch.rescan {
				t.Fatal("rescan = false, want true")
			}
			want := []kernelUevent{{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm1"}}
			if !reflect.DeepEqual(batch.removals, want) {
				t.Fatalf("removals = %#v, want %#v", batch.removals, want)
			}
		})
	}
}

func TestModemUevent(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "wwan", data: "add@/devices/x\x00SUBSYSTEM=wwan\x00", want: true},
		{name: "usbmisc", data: "change@/devices/x\x00SUBSYSTEM=usbmisc\x00", want: true},
		{name: "net", data: "add@/devices/x\x00SUBSYSTEM=net\x00", want: true},
		{name: "tty", data: "add@/devices/x\x00SUBSYSTEM=tty\x00", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modemUevent([]byte(tt.data)); got != tt.want {
				t.Errorf("modemUevent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func addDiscoveryFixture(t *testing.T, sysRoot, class, name, protocol, driver string, interfaces []string) {
	t.Helper()
	entry := filepath.Join(sysRoot, "class", class, name)
	if err := os.MkdirAll(filepath.Join(entry, "device", "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "type"), []byte(protocol+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	driverTarget := filepath.Join(sysRoot, "bus", "usb", "drivers", driver)
	if err := os.MkdirAll(driverTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driverTarget, filepath.Join(entry, "device", "driver")); err != nil {
		t.Fatal(err)
	}
	for _, interfaceName := range interfaces {
		if err := os.Mkdir(filepath.Join(entry, "device", "net", interfaceName), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
