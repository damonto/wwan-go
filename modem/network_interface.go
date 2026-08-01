package modem

import (
	"context"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/modem/contract"
)

func selectNetworkInterface(ctx context.Context, controlPort, requested string) (string, error) {
	devices, err := Discover(ctx)
	if err != nil {
		return "", fmt.Errorf("selecting modem network interface: %w", err)
	}
	interfaces := networkInterfacesForControlPort(devices, controlPort)
	if requested != "" {
		if len(interfaces) != 0 && !slices.Contains(interfaces, requested) {
			return "", fmt.Errorf("selecting modem network interface: %s is not associated with %s", requested, controlPort)
		}
		return requested, nil
	}
	if len(interfaces) != 1 {
		return "", fmt.Errorf("selecting modem network interface: found %d associated interfaces, want exactly one", len(interfaces))
	}
	return interfaces[0], nil
}

func networkInterfacesForControlPort(devices []Device, controlPort string) []string {
	for _, candidate := range devices {
		if !slices.ContainsFunc(candidate.Ports, func(port Port) bool {
			return port.Path == controlPort && (port.Type == PortQMI || port.Type == PortMBIM)
		}) {
			continue
		}
		interfaces := make([]string, 0)
		for _, port := range candidate.Ports {
			if port.Type == PortNetwork && port.ControlPath == controlPort {
				interfaces = append(interfaces, port.Name)
			}
		}
		return interfaces
	}
	return nil
}

func cloneNetworkConfig(config NetworkConfig) NetworkConfig {
	return contract.CloneNetworkConfig(config)
}
