package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	defaultSysRoot = "/sys"
	defaultDevRoot = "/dev"
)

func discover(ctx context.Context, sysRoot, devRoot string, requireNode bool) ([]Device, error) {
	classes := []string{"wwan", "usbmisc"}
	devices := make(map[string]Device)
	for _, class := range classes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		classPath := filepath.Join(sysRoot, "class", class)
		entries, err := os.ReadDir(classPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("discovering modem class %s: %w", class, err)
		}
		for _, entry := range entries {
			device, ok, err := inspectDevice(classPath, devRoot, entry.Name(), requireNode)
			if err != nil {
				return nil, err
			}
			if ok {
				key := physicalDeviceKey(device)
				devices[key] = mergeDevice(devices[key], device)
			}
		}
	}
	if err := attachATPorts(ctx, sysRoot, devRoot, requireNode, devices); err != nil {
		return nil, err
	}

	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		result = append(result, cloneDevice(device))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func inspectDevice(classPath, devRoot, name string, requireNode bool) (Device, bool, error) {
	entryPath := filepath.Join(classPath, name)
	protocol := protocolHint(entryPath, name)
	if protocol == ProtocolUnknown && !controlNodeName(name) {
		return Device{}, false, nil
	}

	devicePath := filepath.Join(devRoot, name)
	if requireNode {
		info, err := os.Stat(devicePath)
		if errors.Is(err, os.ErrNotExist) {
			return Device{}, false, nil
		}
		if err != nil {
			return Device{}, false, fmt.Errorf("inspecting modem node %s: %w", devicePath, err)
		}
		if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice == 0 {
			return Device{}, false, nil
		}
	}

	physicalPath := physicalDevicePath(filepath.Join(entryPath, "device"))
	driver := symlinkBase(filepath.Join(entryPath, "device", "driver"))
	interfaces, err := networkInterfaces(entryPath)
	if err != nil {
		return Device{}, false, fmt.Errorf("inspecting modem %s network interfaces: %w", name, err)
	}
	return Device{
		Path:              devicePath,
		Protocol:          protocol,
		Driver:            driver,
		PhysicalPath:      physicalPath,
		NetworkInterfaces: interfaces,
		Ports:             devicePorts(name, devicePath, entryPath, driver, protocol, interfaces),
	}, true, nil
}

func devicePorts(name, devicePath, entryPath, driver string, protocol Protocol, interfaces []string) []Port {
	portType := PortUnknown
	switch protocol {
	case ProtocolQMI:
		portType = PortQMI
	case ProtocolMBIM:
		portType = PortMBIM
	}
	ports := []Port{{Type: portType, Name: name, Path: devicePath, SysPath: entryPath, Driver: driver}}
	for _, interfaceName := range interfaces {
		ports = append(ports, Port{Type: PortNetwork, Name: interfaceName, Driver: driver})
	}
	return ports
}

func attachATPorts(ctx context.Context, sysRoot, devRoot string, requireNode bool, devices map[string]Device) error {
	classPath := filepath.Join(sysRoot, "class", "tty")
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
		if !atPortName(name) {
			continue
		}
		path := filepath.Join(devRoot, name)
		if requireNode {
			info, err := os.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspecting modem AT node %s: %w", path, err)
			}
			if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice == 0 {
				continue
			}
		}
		entryPath := filepath.Join(classPath, name)
		physicalPath := physicalDevicePath(filepath.Join(entryPath, "device"))
		key := physicalPath
		device, ok := devices[key]
		if !ok {
			continue
		}
		port := Port{
			Type:    PortAT,
			Name:    name,
			Path:    path,
			SysPath: entryPath,
			Driver:  symlinkBase(filepath.Join(entryPath, "device", "driver")),
		}
		device.Ports = append(device.Ports, port)
		devices[key] = normalizeDevice(device)
	}
	return nil
}

func atPortName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "ttyusb") || strings.HasPrefix(lower, "ttyacm") || strings.Contains(lower, "at")
}

func physicalDeviceKey(device Device) string {
	if device.PhysicalPath != "" {
		return device.PhysicalPath
	}
	return device.Path
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

func mergeDevice(current, next Device) Device {
	if current.Path == "" {
		return normalizeDevice(next)
	}
	current.Ports = append(current.Ports, next.Ports...)
	current.NetworkInterfaces = append(current.NetworkInterfaces, next.NetworkInterfaces...)
	return normalizeDevice(current)
}

func normalizeDevice(device Device) Device {
	sort.Slice(device.Ports, func(i, j int) bool {
		if device.Ports[i].Type != device.Ports[j].Type {
			return device.Ports[i].Type < device.Ports[j].Type
		}
		return device.Ports[i].Path < device.Ports[j].Path
	})
	device.Ports = slices.CompactFunc(device.Ports, func(a, b Port) bool {
		return a.Type == b.Type && a.Name == b.Name && a.Path == b.Path
	})
	sort.Strings(device.NetworkInterfaces)
	device.NetworkInterfaces = slices.Compact(device.NetworkInterfaces)
	for _, port := range device.Ports {
		if port.Type != PortQMI && port.Type != PortMBIM {
			continue
		}
		device.Path = port.Path
		device.Driver = port.Driver
		if port.Type == PortQMI {
			device.Protocol = ProtocolQMI
		} else {
			device.Protocol = ProtocolMBIM
		}
		break
	}
	return device
}

func protocolHint(entryPath, name string) Protocol {
	if value, err := os.ReadFile(filepath.Join(entryPath, "type")); err == nil {
		switch strings.ToUpper(strings.TrimSpace(string(value))) {
		case "QMI":
			return ProtocolQMI
		case "MBIM":
			return ProtocolMBIM
		}
	}
	switch symlinkBase(filepath.Join(entryPath, "device", "driver")) {
	case "qmi_wwan":
		return ProtocolQMI
	case "cdc_mbim":
		return ProtocolMBIM
	}
	upper := strings.ToUpper(name)
	switch {
	case strings.HasSuffix(upper, "QMI"):
		return ProtocolQMI
	case strings.HasSuffix(upper, "MBIM"):
		return ProtocolMBIM
	default:
		return ProtocolUnknown
	}
}

func controlNodeName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "cdc-wdm") || strings.Contains(lower, "qmi") || strings.Contains(lower, "mbim")
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
	device.NetworkInterfaces = slices.Clone(device.NetworkInterfaces)
	device.Ports = slices.Clone(device.Ports)
	return device
}

func devicesByPath(devices []Device) map[string]Device {
	result := make(map[string]Device, len(devices))
	for _, device := range devices {
		result[physicalDeviceKey(device)] = cloneDevice(device)
	}
	return result
}

func sameDevice(a, b Device) bool {
	return a.Path == b.Path && a.Protocol == b.Protocol && a.Driver == b.Driver &&
		a.PhysicalPath == b.PhysicalPath && slices.Equal(a.NetworkInterfaces, b.NetworkInterfaces) &&
		slices.Equal(a.Ports, b.Ports)
}
