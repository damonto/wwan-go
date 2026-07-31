package modem

import (
	"context"
	"fmt"
	"slices"

	"github.com/damonto/wwan-go/modem/contract"
)

func selectNetworkInterface(ctx context.Context, device, requested string) (string, error) {
	devices, err := Discover(ctx)
	if err != nil {
		return "", fmt.Errorf("selecting modem network interface: %w", err)
	}
	var interfaces []string
	for _, candidate := range devices {
		if candidate.Path == device {
			interfaces = candidate.NetworkInterfaces
			break
		}
	}
	if requested != "" {
		if len(interfaces) != 0 && !slices.Contains(interfaces, requested) {
			return "", fmt.Errorf("selecting modem network interface: %s is not associated with %s", requested, device)
		}
		return requested, nil
	}
	if len(interfaces) != 1 {
		return "", fmt.Errorf("selecting modem network interface: found %d associated interfaces, want exactly one", len(interfaces))
	}
	return interfaces[0], nil
}

func cloneNetworkConfig(config NetworkConfig) NetworkConfig {
	return contract.CloneNetworkConfig(config)
}
