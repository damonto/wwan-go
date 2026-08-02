//go:build linux

package modem

import (
	"context"
	"fmt"
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
	if err := os.MkdirAll(filepath.Join(sysRoot, "class", "usbmisc", "cdc-wdm9", "device"), 0o755); err != nil {
		t.Fatal(err)
	}

	devices, err := discover(context.Background(), discoveryConfig{sysRoot: sysRoot, devRoot: devRoot})
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}
	tests := []struct {
		name  string
		index int
		ports []Port
	}{
		{
			name:  "MBIM",
			index: 0,
			ports: []Port{
				{Type: PortMBIM, Name: "cdc-wdm1", Path: filepath.Join(devRoot, "cdc-wdm1"), SysPath: filepath.Join(sysRoot, "class", "usbmisc", "cdc-wdm1"), Subsystem: "usbmisc", Driver: "cdc_mbim"},
				{Type: PortNetwork, Name: "wwan1", Subsystem: "net", Driver: "cdc_mbim", ControlPath: filepath.Join(devRoot, "cdc-wdm1")},
				{Type: PortNetwork, Name: "wwan1.1", Subsystem: "net", Driver: "cdc_mbim", ControlPath: filepath.Join(devRoot, "cdc-wdm1")},
			},
		},
		{
			name:  "QMI",
			index: 1,
			ports: []Port{
				{Type: PortQMI, Name: "wwan0qmi", Path: filepath.Join(devRoot, "wwan0qmi"), SysPath: filepath.Join(sysRoot, "class", "wwan", "wwan0qmi"), Subsystem: "wwan", Driver: "qmi_wwan"},
				{Type: PortNetwork, Name: "wwan0", Subsystem: "net", Driver: "qmi_wwan", ControlPath: filepath.Join(devRoot, "wwan0qmi")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := devices[tt.index]
			if !reflect.DeepEqual(device.Ports, tt.ports) {
				t.Errorf("device ports = %#v, want %#v", device.Ports, tt.ports)
			}
		})
	}
}

func TestDiscoverAggregatesUSBPortsWithoutSelectingOne(t *testing.T) {
	root := t.TempDir()
	sysRoot := filepath.Join(root, "sys")
	devRoot := filepath.Join(root, "dev")
	usbDevice := filepath.Join(sysRoot, "devices", "pci0000:00", "usb1", "1-2")
	qmiInterface := filepath.Join(usbDevice, "1-2:1.4")
	mbimInterface := filepath.Join(usbDevice, "1-2:1.5")
	atInterface := filepath.Join(usbDevice, "1-2:1.2")
	for _, path := range []string{
		filepath.Join(qmiInterface, "net", "wwan0"),
		filepath.Join(mbimInterface, "net", "wwan1"),
		atInterface,
		filepath.Join(sysRoot, "class", "wwan"),
		filepath.Join(sysRoot, "class", "usbmisc"),
		filepath.Join(sysRoot, "class", "tty"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range map[string]string{"idVendor": "2c7c\n", "idProduct": "0306\n"} {
		if err := os.WriteFile(filepath.Join(usbDevice, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	addUSBInterfaceMetadata(t, qmiInterface, 4, 0xff, 0xff, 0xff)
	addUSBInterfaceMetadata(t, mbimInterface, 5, 0x02, 0x0e, 0x00)
	addUSBInterfaceMetadata(t, atInterface, 2, 0xff, 0xff, 0xff)
	addDriverLink(t, sysRoot, qmiInterface, "qmi_wwan")
	addDriverLink(t, sysRoot, mbimInterface, "cdc_mbim")
	addDriverLink(t, sysRoot, atInterface, "option")
	qmiEntry := filepath.Join(sysRoot, "class", "usbmisc", "cdc-wdm0")
	if err := os.MkdirAll(qmiEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qmiEntry, "type"), []byte("QMI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(qmiInterface, filepath.Join(qmiEntry, "device")); err != nil {
		t.Fatal(err)
	}
	mbimEntry := filepath.Join(sysRoot, "class", "wwan", "wwan0mbim0")
	if err := os.MkdirAll(mbimEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mbimEntry, "type"), []byte("MBIM\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(mbimInterface, filepath.Join(mbimEntry, "device")); err != nil {
		t.Fatal(err)
	}
	atEntry := filepath.Join(sysRoot, "class", "tty", "ttyUSB2")
	if err := os.MkdirAll(atEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(atInterface, filepath.Join(atEntry, "device")); err != nil {
		t.Fatal(err)
	}

	devices, err := discover(context.Background(), discoveryConfig{sysRoot: sysRoot, devRoot: devRoot})
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
	if device.Bus != BusUSB || device.USB != (USBIdentity{VendorID: 0x2c7c, ProductID: 0x0306}) {
		t.Fatalf("device identity = %d/%#v, want USB 2c7c:0306", device.Bus, device.USB)
	}
	qmiUSB := USBInterface{Valid: true, Number: 4, Class: 0xff, Subclass: 0xff, Protocol: 0xff}
	mbimUSB := USBInterface{Valid: true, Number: 5, Class: 0x02, Subclass: 0x0e}
	atUSB := USBInterface{Valid: true, Number: 2, Class: 0xff, Subclass: 0xff, Protocol: 0xff}
	wantPorts := []Port{
		{Type: PortQMI, Name: "cdc-wdm0", Path: filepath.Join(devRoot, "cdc-wdm0"), SysPath: qmiEntry, Subsystem: "usbmisc", Driver: "qmi_wwan", USB: qmiUSB},
		{Type: PortMBIM, Name: "wwan0mbim0", Path: filepath.Join(devRoot, "wwan0mbim0"), SysPath: mbimEntry, Subsystem: "wwan", Driver: "cdc_mbim", USB: mbimUSB},
		{Type: PortAT, Role: PortRolePrimary, Name: "ttyUSB2", Path: filepath.Join(devRoot, "ttyUSB2"), SysPath: atEntry, Subsystem: "tty", Driver: "option", USB: atUSB},
		{Type: PortNetwork, Name: "wwan0", Subsystem: "net", Driver: "qmi_wwan", USB: qmiUSB, QMIEndpoint: QMIEndpoint{Type: QMIEndpointHSUSB, InterfaceNumber: 4}, ControlPath: filepath.Join(devRoot, "cdc-wdm0")},
		{Type: PortNetwork, Name: "wwan1", Subsystem: "net", Driver: "cdc_mbim", USB: mbimUSB, ControlPath: filepath.Join(devRoot, "wwan0mbim0")},
	}
	if !reflect.DeepEqual(device.Ports, wantPorts) {
		t.Fatalf("Ports = %#v, want %#v", device.Ports, wantPorts)
	}
}

func TestDiscoverPreservesKernelProtocolOnQCOMAncestor(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		portName string
		want     PortType
	}{
		{name: "QMI uses SoC branch", protocol: "QMI", portName: "wwan0qmi0", want: PortQMI},
		{name: "MBIM stays generic", protocol: "MBIM", portName: "wwan0mbim0", want: PortMBIM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sysRoot := filepath.Join(root, "sys")
			devRoot := filepath.Join(root, "dev")
			modemPath := filepath.Join(sysRoot, "devices", "platform", "soc", "modem")
			portPath := filepath.Join(modemPath, "wwan", tt.portName)
			entryPath := filepath.Join(sysRoot, "class", "wwan", tt.portName)
			for _, path := range []string{portPath, entryPath} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			addPlatformDriverLink(t, sysRoot, modemPath, qcomSoCDriverName)
			if err := os.WriteFile(filepath.Join(entryPath, "type"), []byte(tt.protocol+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(portPath, filepath.Join(entryPath, "device")); err != nil {
				t.Fatal(err)
			}

			devices, err := discover(context.Background(), discoveryConfig{sysRoot: sysRoot, devRoot: devRoot})
			if err != nil {
				t.Fatalf("discover() error = %v", err)
			}
			if len(devices) != 1 || len(devices[0].Ports) != 1 || devices[0].Ports[0].Type != tt.want {
				t.Fatalf("devices = %#v, want one kernel-confirmed port type %d", devices, tt.want)
			}
		})
	}
}

func TestKernelProtocolUsesMetadataOnly(t *testing.T) {
	tests := []struct {
		name      string
		typeValue string
		driver    string
		entryName string
		want      Protocol
	}{
		{name: "WWAN QMI type", typeValue: "QMI\n", entryName: "port0", want: ProtocolQMI},
		{name: "WWAN MBIM type", typeValue: "MBIM\n", entryName: "port0", want: ProtocolMBIM},
		{name: "WWAN type takes priority", typeValue: "QMI\n", driver: "cdc_mbim", entryName: "port0", want: ProtocolQMI},
		{name: "QMI driver", driver: "qmi_wwan", entryName: "cdc-wdm0", want: ProtocolQMI},
		{name: "MBIM driver", driver: "cdc_mbim", entryName: "cdc-wdm0", want: ProtocolMBIM},
		{name: "misleading QMI name", entryName: "definitely-qmi", want: ProtocolUnknown},
		{name: "misleading MBIM name", entryName: "definitely-mbim", want: ProtocolUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			entry := filepath.Join(root, tt.entryName)
			if err := os.MkdirAll(filepath.Join(entry, "device"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.typeValue != "" {
				if err := os.WriteFile(filepath.Join(entry, "type"), []byte(tt.typeValue), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.driver != "" {
				driverTarget := filepath.Join(root, "drivers", tt.driver)
				if err := os.MkdirAll(driverTarget, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(driverTarget, filepath.Join(entry, "device", "driver")); err != nil {
					t.Fatal(err)
				}
			}

			if got := kernelProtocol(entry); got != tt.want {
				t.Errorf("kernelProtocol() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGenericTTYPortType(t *testing.T) {
	tests := []struct {
		name string
		port string
		want PortType
	}{
		{name: "USB serial", port: "ttyUSB0", want: PortAT},
		{name: "CDC ACM", port: "ttyACM0", want: PortAT},
		{name: "explicit AT name", port: "modem-at", want: PortAT},
		{name: "unrecognized platform tty", port: "ttyHS0", want: PortUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := genericTTYPortType(tt.port); got != tt.want {
				t.Errorf("genericTTYPortType(%q) = %d, want %d", tt.port, got, tt.want)
			}
		})
	}
}

func TestDiffDevices(t *testing.T) {
	current := devicesByKey([]Device{
		{PhysicalPath: "/sys/a", Ports: []Port{{Type: PortQMI, Path: "/dev/a"}}},
		{PhysicalPath: "/sys/b", Ports: []Port{{Type: PortMBIM, Path: "/dev/b"}}},
	})
	next := devicesByKey([]Device{
		{PhysicalPath: "/sys/b", Ports: []Port{{Type: PortMBIM, Path: "/dev/b"}, {Type: PortNetwork, Name: "wwan0"}}},
		{PhysicalPath: "/sys/c", Ports: []Port{{Type: PortQMI, Path: "/dev/c"}}},
	})
	want := []DeviceEvent{
		{Type: DeviceRemoved, Device: Device{PhysicalPath: "/sys/a", Ports: []Port{{Type: PortQMI, Path: "/dev/a"}}}},
		{Type: DeviceChanged, Device: Device{PhysicalPath: "/sys/b", Ports: []Port{{Type: PortMBIM, Path: "/dev/b"}, {Type: PortNetwork, Name: "wwan0"}}}},
		{Type: DeviceAdded, Device: Device{PhysicalPath: "/sys/c", Ports: []Port{{Type: PortQMI, Path: "/dev/c"}}}},
	}
	if got := diffDevices(current, next, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("diffDevices() = %#v, want %#v", got, want)
	}
}

func TestReconcileDeviceEventsUsesPhysicalDeviceSemantics(t *testing.T) {
	qmiPort := Port{Type: PortQMI, Name: "cdc-wdm0", Path: "/dev/cdc-wdm0", SysPath: "/sys/class/usbmisc/cdc-wdm0"}
	mbimPort := Port{Type: PortMBIM, Name: "wwan0mbim0", Path: "/dev/wwan0mbim0", SysPath: "/sys/class/wwan/wwan0mbim0"}
	tests := []struct {
		name     string
		current  Device
		removals []kernelUevent
		next     []Device
		want     []DeviceEvent
	}{
		{
			name:     "same control port reconnects",
			current:  Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}},
			removals: []kernelUevent{{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm0"}},
			next:     []Device{{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}}},
			want: []DeviceEvent{
				{Type: DeviceRemoved, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}}},
				{Type: DeviceAdded, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}}},
			},
		},
		{
			name:     "one of multiple control ports disappears",
			current:  Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort, mbimPort}},
			removals: []kernelUevent{{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm0"}},
			next:     []Device{{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{mbimPort}}},
			want:     []DeviceEvent{{Type: DeviceChanged, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{mbimPort}}}},
		},
		{
			name:     "last control port disappears",
			current:  Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}},
			removals: []kernelUevent{{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm0"}},
			want:     []DeviceEvent{{Type: DeviceRemoved, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort}}}},
		},
		{
			name:    "multiple control ports reconnect once",
			current: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort, mbimPort}},
			removals: []kernelUevent{
				{action: "remove", subsystem: "usbmisc", devName: "cdc-wdm0"},
				{action: "remove", subsystem: "wwan", devName: "wwan0mbim0"},
			},
			next: []Device{{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort, mbimPort}}},
			want: []DeviceEvent{
				{Type: DeviceRemoved, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort, mbimPort}}},
				{Type: DeviceAdded, Device: Device{PhysicalPath: "/sys/devices/modem-1", Ports: []Port{qmiPort, mbimPort}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := devicesByKey([]Device{tt.current})
			next, got := reconcileDeviceEvents(current, tt.removals, tt.next)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("reconcileDeviceEvents() events = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(next, devicesByKey(tt.next)) {
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
		{name: "rpmsg", data: "add@/devices/x\x00SUBSYSTEM=rpmsg\x00", want: true},
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

func addUSBInterfaceMetadata(t *testing.T, path string, number, class, subclass, protocol uint8) {
	t.Helper()
	values := map[string]uint8{
		"bInterfaceNumber":   number,
		"bInterfaceClass":    class,
		"bInterfaceSubClass": subclass,
		"bInterfaceProtocol": protocol,
	}
	for name, value := range values {
		if err := os.WriteFile(filepath.Join(path, name), fmt.Appendf(nil, "%02x\n", value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func addDriverLink(t *testing.T, sysRoot, devicePath, driver string) {
	t.Helper()
	driverTarget := filepath.Join(sysRoot, "bus", "usb", "drivers", driver)
	if err := os.MkdirAll(driverTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driverTarget, filepath.Join(devicePath, "driver")); err != nil {
		t.Fatal(err)
	}
}
