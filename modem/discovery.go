package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultSysRoot = "/sys"
	defaultDevRoot = "/dev"
)

type discoveryConfig struct {
	sysRoot     string
	devRoot     string
	requireNode bool
}

func discover(ctx context.Context, cfg discoveryConfig) ([]Device, error) {
	classes := []string{"wwan", "usbmisc"}
	devices := make(map[string]Device)
	for _, class := range classes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		classPath := filepath.Join(cfg.sysRoot, "class", class)
		entries, err := os.ReadDir(classPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("discovering modem class %s: %w", class, err)
		}
		for _, entry := range entries {
			entryPath := filepath.Join(classPath, entry.Name())
			if hasQCOMSoCDriver(entryPath) && kernelProtocol(entryPath) == ProtocolQMI {
				continue
			}
			device, ok, err := inspectDevice(cfg, class, entry.Name())
			if err != nil {
				return nil, err
			}
			if ok {
				key := physicalDeviceKey(device)
				devices[key] = mergeDevice(devices[key], device)
			}
		}
	}
	if err := attachTTYPorts(ctx, cfg, devices); err != nil {
		return nil, err
	}
	qcomDevice, ok, err := discoverQCOMSoC(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if ok {
		devices[physicalDeviceKey(qcomDevice)] = qcomDevice
	}

	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		device = applyStaticCompatibility(device)
		result = append(result, cloneDevice(device))
	}
	sort.Slice(result, func(i, j int) bool {
		return physicalDeviceKey(result[i]) < physicalDeviceKey(result[j])
	})
	return result, nil
}

func inspectDevice(cfg discoveryConfig, subsystem, name string) (Device, bool, error) {
	classPath := filepath.Join(cfg.sysRoot, "class", subsystem)
	entryPath := filepath.Join(classPath, name)
	protocol := kernelProtocol(entryPath)
	if protocol == ProtocolUnknown {
		return Device{}, false, nil
	}

	devicePath := filepath.Join(cfg.devRoot, name)
	if ok, err := validCharacterDevice(devicePath, cfg.requireNode); err != nil {
		return Device{}, false, fmt.Errorf("inspecting modem node %s: %w", devicePath, err)
	} else if !ok {
		return Device{}, false, nil
	}

	physicalPath := physicalDevicePath(filepath.Join(entryPath, "device"))
	driver := deviceDriver(filepath.Join(entryPath, "device"))
	identity := usbIdentity(physicalPath)
	bus := BusUnknown
	if identity != (USBIdentity{}) {
		bus = BusUSB
	}
	interfaces, err := networkInterfaces(entryPath)
	if err != nil {
		return Device{}, false, fmt.Errorf("inspecting modem %s network interfaces: %w", name, err)
	}
	return Device{
		PhysicalPath: physicalPath,
		Bus:          bus,
		USB:          identity,
		Ports: devicePorts(controlPortConfig{
			sysRoot:    cfg.sysRoot,
			name:       name,
			path:       devicePath,
			sysPath:    entryPath,
			subsystem:  subsystem,
			driver:     driver,
			protocol:   protocol,
			interfaces: interfaces,
		}),
	}, true, nil
}

type controlPortConfig struct {
	sysRoot    string
	name       string
	path       string
	sysPath    string
	subsystem  string
	driver     string
	protocol   Protocol
	interfaces []string
}

func devicePorts(cfg controlPortConfig) []Port {
	portType := PortUnknown
	switch cfg.protocol {
	case ProtocolQMI:
		portType = PortQMI
	case ProtocolMBIM:
		portType = PortMBIM
	}
	controlUSB := usbInterface(filepath.Join(cfg.sysPath, "device"))
	ports := []Port{{
		Type:      portType,
		Name:      cfg.name,
		Path:      cfg.path,
		SysPath:   cfg.sysPath,
		Subsystem: cfg.subsystem,
		Driver:    cfg.driver,
		USB:       controlUSB,
	}}
	for _, interfaceName := range cfg.interfaces {
		port := networkPort(cfg.sysRoot, interfaceName, cfg.driver, cfg.path, controlUSB)
		if cfg.protocol == ProtocolQMI && cfg.driver == "qmi_wwan" && controlUSB.Valid {
			port.QMIEndpoint = QMIEndpoint{Type: QMIEndpointHSUSB, InterfaceNumber: uint32(controlUSB.Number)}
		}
		ports = append(ports, port)
	}
	return ports
}

func attachTTYPorts(ctx context.Context, cfg discoveryConfig, devices map[string]Device) error {
	classPath := filepath.Join(cfg.sysRoot, "class", "tty")
	entries, err := os.ReadDir(classPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("discovering modem class tty: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		path := filepath.Join(cfg.devRoot, name)
		if ok, err := validCharacterDevice(path, cfg.requireNode); err != nil {
			return fmt.Errorf("inspecting modem tty node %s: %w", path, err)
		} else if !ok {
			continue
		}
		entryPath := filepath.Join(classPath, name)
		physicalPath := physicalDevicePath(filepath.Join(entryPath, "device"))
		if physicalPath == "" {
			continue
		}
		key := physicalPath
		device, ok := devices[key]
		if !ok {
			continue
		}
		port := Port{
			Type:      genericTTYPortType(name),
			Name:      name,
			Path:      path,
			SysPath:   entryPath,
			Subsystem: "tty",
			Driver:    deviceDriver(filepath.Join(entryPath, "device")),
			USB:       usbInterface(filepath.Join(entryPath, "device")),
		}
		device.Ports = append(device.Ports, port)
		devices[key] = normalizeDevice(device)
	}
	return nil
}

func genericTTYPortType(name string) PortType {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "ttyusb") || strings.HasPrefix(lower, "ttyacm") || strings.Contains(lower, "at") {
		return PortAT
	}
	return PortUnknown
}

func physicalDeviceKey(device Device) string {
	if device.PhysicalPath != "" {
		return device.PhysicalPath
	}
	for _, port := range device.Ports {
		if port.SysPath != "" {
			return port.SysPath
		}
		if port.Path != "" {
			return port.Path
		}
	}
	return ""
}

// physicalDevicePath folds the separate USB interfaces of one modem into the
// USB device which owns them. Platform devices without USB identity retain
// their resolved control-port topology.
func physicalDevicePath(deviceLink string) string {
	path, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return ""
	}
	fallback := path
	for {
		if regularFile(filepath.Join(path, "idVendor")) && regularFile(filepath.Join(path, "idProduct")) {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fallback
		}
		path = parent
	}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func validCharacterDevice(path string, required bool) (bool, error) {
	if !required {
		return true, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice != 0, nil
}

func usbIdentity(physicalPath string) USBIdentity {
	if physicalPath == "" {
		return USBIdentity{}
	}
	vendorID, vendorOK := readHexUint16(filepath.Join(physicalPath, "idVendor"))
	productID, productOK := readHexUint16(filepath.Join(physicalPath, "idProduct"))
	if !vendorOK || !productOK {
		return USBIdentity{}
	}
	return USBIdentity{VendorID: vendorID, ProductID: productID}
}

func usbInterface(deviceLink string) USBInterface {
	path, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return USBInterface{}
	}
	for {
		number, ok := readHexUint8(filepath.Join(path, "bInterfaceNumber"))
		if ok {
			class, _ := readHexUint8(filepath.Join(path, "bInterfaceClass"))
			subclass, _ := readHexUint8(filepath.Join(path, "bInterfaceSubClass"))
			protocol, _ := readHexUint8(filepath.Join(path, "bInterfaceProtocol"))
			return USBInterface{
				Valid:    true,
				Number:   number,
				Class:    class,
				Subclass: subclass,
				Protocol: protocol,
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			return USBInterface{}
		}
		path = parent
	}
}

func readHexUint8(path string) (uint8, bool) {
	value, ok := readUint(path, 16, 8)
	return uint8(value), ok
}

func readHexUint16(path string) (uint16, bool) {
	value, ok := readUint(path, 16, 16)
	return uint16(value), ok
}

func readDecimalInt(path string) (int, bool) {
	value, ok := readUint(path, 10, 31)
	return int(value), ok
}

func readUint(path string, base, bits int) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), base, bits)
	return value, err == nil
}

func deviceDriver(deviceLink string) string {
	path, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return ""
	}
	for {
		if driver := symlinkBase(filepath.Join(path, "driver")); driver != "" {
			return driver
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func deviceHasDriver(deviceLink, driver string) bool {
	path, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return false
	}
	for {
		if symlinkBase(filepath.Join(path, "driver")) == driver {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

func networkPort(sysRoot, name, fallbackDriver, controlPath string, fallbackUSB USBInterface) Port {
	port := Port{
		Type:        PortNetwork,
		Name:        name,
		Subsystem:   "net",
		Driver:      fallbackDriver,
		USB:         fallbackUSB,
		ControlPath: controlPath,
	}
	sysPath := filepath.Join(sysRoot, "class", "net", name)
	if _, err := os.Lstat(sysPath); err != nil {
		return port
	}
	port.SysPath = sysPath
	if driver := deviceDriver(filepath.Join(sysPath, "device")); driver != "" {
		port.Driver = driver
	}
	if usb := usbInterface(filepath.Join(sysPath, "device")); usb.Valid {
		port.USB = usb
	}
	return port
}

func mergeDevice(current, next Device) Device {
	if len(current.Ports) == 0 {
		return normalizeDevice(next)
	}
	if current.PhysicalPath == "" {
		current.PhysicalPath = next.PhysicalPath
	}
	if current.Bus == BusUnknown {
		current.Bus = next.Bus
	}
	if current.USB == (USBIdentity{}) {
		current.USB = next.USB
	}
	current.Ports = append(current.Ports, next.Ports...)
	return normalizeDevice(current)
}

func normalizeDevice(device Device) Device {
	sort.Slice(device.Ports, func(i, j int) bool {
		if device.Ports[i].Type != device.Ports[j].Type {
			return device.Ports[i].Type < device.Ports[j].Type
		}
		if device.Ports[i].Role != device.Ports[j].Role {
			return device.Ports[i].Role < device.Ports[j].Role
		}
		if device.Ports[i].Path != device.Ports[j].Path {
			return device.Ports[i].Path < device.Ports[j].Path
		}
		if device.Ports[i].Name != device.Ports[j].Name {
			return device.Ports[i].Name < device.Ports[j].Name
		}
		if device.Ports[i].ControlPath != device.Ports[j].ControlPath {
			return device.Ports[i].ControlPath < device.Ports[j].ControlPath
		}
		if device.Ports[i].Driver != device.Ports[j].Driver {
			return device.Ports[i].Driver < device.Ports[j].Driver
		}
		return device.Ports[i].SysPath < device.Ports[j].SysPath
	})
	device.Ports = slices.CompactFunc(device.Ports, func(a, b Port) bool {
		return a == b
	})
	return device
}

func kernelProtocol(entryPath string) Protocol {
	if value, err := os.ReadFile(filepath.Join(entryPath, "type")); err == nil {
		switch strings.ToUpper(strings.TrimSpace(string(value))) {
		case "QMI":
			return ProtocolQMI
		case "MBIM":
			return ProtocolMBIM
		}
	}
	if protocol := qcomRPMsgProtocol(entryPath); protocol != ProtocolUnknown {
		return protocol
	}
	switch deviceDriver(filepath.Join(entryPath, "device")) {
	case "qmi_wwan":
		return ProtocolQMI
	case "cdc_mbim":
		return ProtocolMBIM
	}
	return ProtocolUnknown
}

func symlinkBase(path string) string {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func networkInterfaces(entryPath string) ([]string, error) {
	paths := []string{
		filepath.Join(entryPath, "device", "net"),
		filepath.Join(entryPath, "device", "device", "net"),
	}
	seen := make(map[string]struct{})
	for _, path := range paths {
		entries, err := os.ReadDir(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			seen[entry.Name()] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func cloneDevice(device Device) Device {
	device.Ports = slices.Clone(device.Ports)
	return device
}

func devicesByKey(devices []Device) map[string]Device {
	result := make(map[string]Device, len(devices))
	for _, device := range devices {
		result[physicalDeviceKey(device)] = cloneDevice(device)
	}
	return result
}

func sameDevice(a, b Device) bool {
	return a.PhysicalPath == b.PhysicalPath && a.Bus == b.Bus && a.USB == b.USB && slices.Equal(a.Ports, b.Ports)
}
