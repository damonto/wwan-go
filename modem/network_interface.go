package modem

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/modem/contract"
)

func selectNetworkPort(ctx context.Context, controlPort, requested string) (Port, error) {
	devices, err := Discover(ctx)
	if err != nil {
		return Port{}, fmt.Errorf("selecting modem network interface: %w", err)
	}
	return selectNetworkPortFromDevices(devices, controlPort, requested)
}

func selectNetworkPortFromDevices(devices []Device, controlPort, requested string) (Port, error) {
	ports := networkPortsForControlPort(devices, controlPort)
	if requested != "" {
		if len(ports) == 0 {
			return Port{Type: PortNetwork, Name: requested, Subsystem: "net"}, nil
		}
		for _, port := range ports {
			if port.Name == requested {
				return port, nil
			}
		}
		return Port{}, fmt.Errorf("selecting modem network interface: %s is not associated with %s", requested, controlPort)
	}
	if len(ports) == 0 {
		return Port{}, fmt.Errorf("selecting modem network interface: found 0 associated interfaces, want at least one")
	}
	return ports[0], nil
}

func networkInterfacesForControlPort(devices []Device, controlPort string) []string {
	ports := networkPortsForControlPort(devices, controlPort)
	interfaces := make([]string, len(ports))
	for i, port := range ports {
		interfaces[i] = port.Name
	}
	return interfaces
}

func networkPortsForControlPort(devices []Device, controlPort string) []Port {
	for _, candidate := range devices {
		controlIndex := slices.IndexFunc(candidate.Ports, func(port Port) bool {
			return port.Path == controlPort && (port.Type == PortQMI || port.Type == PortMBIM)
		})
		if controlIndex < 0 {
			continue
		}
		control := candidate.Ports[controlIndex]
		exact := make([]Port, 0)
		platform := make([]Port, 0)
		for _, port := range candidate.Ports {
			if port.Type != PortNetwork {
				continue
			}
			if port.ControlPath == controlPort {
				exact = append(exact, port)
				continue
			}
			if candidate.Bus == BusPlatform && control.Type == PortQMI && isQCOMSoCNetworkPort(port) {
				platform = append(platform, port)
			}
		}
		ports := exact
		if len(ports) == 0 {
			ports = platform
		}
		slices.SortFunc(ports, func(left, right Port) int {
			if order := cmp.Compare(left.QMIEndpoint.InterfaceNumber, right.QMIEndpoint.InterfaceNumber); order != 0 {
				return order
			}
			return cmp.Compare(left.Name, right.Name)
		})
		return ports
	}
	return nil
}

func isQCOMSoCNetworkPort(port Port) bool {
	switch port.Driver {
	case bamDMUXDriverName, ipaDriverName:
		return true
	default:
		return false
	}
}

func cloneNetworkConfig(config NetworkConfig) NetworkConfig {
	return contract.CloneNetworkConfig(config)
}
