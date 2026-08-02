package modem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	qcomSoCDriverName = "qcom-q6v5-mss"
	bamDMUXDriverName = "bam-dmux"
	ipaDriverName     = "ipa"
	bamDMUXSIOPort0   = 0x0e04
)

func hasQCOMSoCDriver(entryPath string) bool {
	// udev DRIVERS matching walks the full parent chain. RPMSG endpoints often
	// have their own nearer driver, while qcom-q6v5-mss owns an ancestor.
	return deviceHasDriver(filepath.Join(entryPath, "device"), qcomSoCDriverName)
}

func discoverQCOMSoC(ctx context.Context, cfg discoveryConfig) (Device, bool, error) {
	ports, physicalPath, err := qcomSoCControlPorts(ctx, cfg)
	if err != nil {
		return Device{}, false, err
	}
	if !hasQMIControlPort(ports) {
		return Device{}, false, nil
	}

	device := normalizeDevice(Device{
		PhysicalPath: physicalPath,
		Bus:          BusPlatform,
		Ports:        ports,
	})
	controlPath := firstQMIControlPath(device.Ports)
	networkPorts, err := qcomSoCNetworkPorts(ctx, cfg, controlPath)
	if err != nil {
		return Device{}, false, err
	}
	device.Ports = append(device.Ports, networkPorts...)
	return normalizeDevice(device), true, nil
}

func qcomSoCControlPorts(ctx context.Context, cfg discoveryConfig) ([]Port, string, error) {
	var ports []Port
	physicalPath := ""
	for _, subsystem := range []string{"wwan", "rpmsg"} {
		classPath := filepath.Join(cfg.sysRoot, "class", subsystem)
		entries, err := os.ReadDir(classPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("discovering Qualcomm SoC class %s: %w", subsystem, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			entryPath := filepath.Join(classPath, entry.Name())
			if !hasQCOMSoCDriver(entryPath) {
				continue
			}
			port, ok := qcomSoCPort(cfg, subsystem, entry.Name(), entryPath)
			if !ok {
				continue
			}
			if valid, err := validCharacterDevice(port.Path, cfg.requireNode); err != nil {
				return nil, "", fmt.Errorf("inspecting Qualcomm SoC node %s: %w", port.Path, err)
			} else if !valid {
				continue
			}
			ports = append(ports, port)
			if physicalPath == "" && port.Type == PortQMI {
				physicalPath = driverDevicePath(filepath.Join(entryPath, "device"), qcomSoCDriverName)
			}
		}
	}
	return ports, physicalPath, nil
}

func qcomSoCPort(cfg discoveryConfig, subsystem, name, entryPath string) (Port, bool) {
	port := Port{
		Name:      name,
		Path:      filepath.Join(cfg.devRoot, name),
		SysPath:   entryPath,
		Subsystem: subsystem,
		Driver:    deviceDriver(filepath.Join(entryPath, "device")),
	}
	switch subsystem {
	case "wwan":
		if kernelProtocol(entryPath) != ProtocolQMI {
			return Port{}, false
		}
		port.Type = PortQMI
		return port, true
	case "rpmsg":
		service := qcomRPMsgService(entryPath)
		if !strings.HasPrefix(service, "DATA") || strings.HasPrefix(service, "DATA40") {
			return Port{}, false
		}
		if strings.HasSuffix(service, "_CNTL") {
			port.Type = PortQMI
			return port, true
		}
		port.Type = PortAT
		port.Role = PortRoleSecondary
		return port, true
	default:
		return Port{}, false
	}
}

func qcomRPMsgService(entryPath string) string {
	for _, path := range []string{
		filepath.Join(entryPath, "name"),
		filepath.Join(entryPath, "device", "name"),
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func qcomRPMsgProtocol(entryPath string) Protocol {
	if filepath.Base(filepath.Dir(entryPath)) != "rpmsg" {
		return ProtocolUnknown
	}
	if !hasQCOMSoCDriver(entryPath) {
		return ProtocolUnknown
	}
	service := qcomRPMsgService(entryPath)
	if !strings.HasPrefix(service, "DATA") || strings.HasPrefix(service, "DATA40") {
		return ProtocolUnknown
	}
	if strings.HasSuffix(service, "_CNTL") {
		return ProtocolQMI
	}
	return ProtocolUnknown
}

func qcomSoCNetworkPorts(ctx context.Context, cfg discoveryConfig, controlPath string) ([]Port, error) {
	classPath := filepath.Join(cfg.sysRoot, "class", "net")
	entries, err := os.ReadDir(classPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("discovering Qualcomm SoC network ports: %w", err)
	}
	var ports []Port
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sysPath := filepath.Join(classPath, entry.Name())
		driver := deviceDriver(filepath.Join(sysPath, "device"))
		if driver != bamDMUXDriverName && driver != ipaDriverName {
			continue
		}
		endpoint, err := qcomSoCEndpoint(sysPath, driver)
		if err != nil {
			// One malformed BAM-DMUX endpoint must not hide the modem or other
			// valid data ports. Without a mapping this port is not connectable.
			continue
		}
		ports = append(ports, Port{
			Type:        PortNetwork,
			Name:        entry.Name(),
			SysPath:     sysPath,
			Subsystem:   "net",
			Driver:      driver,
			QMIEndpoint: endpoint,
			ControlPath: controlPath,
		})
	}
	return ports, nil
}

func qcomSoCEndpoint(sysPath, driver string) (QMIEndpoint, error) {
	switch driver {
	case ipaDriverName:
		return QMIEndpoint{Type: QMIEndpointEmbedded, InterfaceNumber: 1}, nil
	case bamDMUXDriverName:
		portNumber, ok := qcomSoCDevPort(sysPath)
		if !ok {
			return QMIEndpoint{}, errors.New("BAM-DMUX dev_port is missing or invalid")
		}
		if portNumber < 0 || portNumber >= 8 {
			return QMIEndpoint{}, fmt.Errorf("BAM-DMUX dev_port %d is outside 0..7", portNumber)
		}
		return QMIEndpoint{
			Type:            QMIEndpointBAMDMUX,
			InterfaceNumber: uint32(portNumber),
			SIOPort:         uint16(bamDMUXSIOPort0 + portNumber),
		}, nil
	default:
		return QMIEndpoint{}, fmt.Errorf("driver %q is unsupported", driver)
	}
}

func qcomSoCDevPort(sysPath string) (int, bool) {
	for _, path := range []string{
		filepath.Join(sysPath, "dev_port"),
		filepath.Join(sysPath, "device", "dev_port"),
	} {
		if value, ok := readDecimalInt(path); ok {
			return value, true
		}
	}
	return 0, false
}

func driverDevicePath(deviceLink, driver string) string {
	path, err := filepath.EvalSymlinks(deviceLink)
	if err != nil {
		return ""
	}
	fallback := path
	for {
		if symlinkBase(filepath.Join(path, "driver")) == driver {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fallback
		}
		path = parent
	}
}

func hasQMIControlPort(ports []Port) bool {
	for _, port := range ports {
		if port.Type == PortQMI {
			return true
		}
	}
	return false
}

func firstQMIControlPath(ports []Port) string {
	for _, port := range ports {
		if port.Type == PortQMI {
			return port.Path
		}
	}
	return ""
}
