// Package topology validates the physical interface roles used by routed mode.
package topology

import (
	"errors"
	"fmt"
	"net"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

// Interface is the subset of net.Interface needed for safety validation.
type Interface struct {
	Index           int
	Name            string
	Flags           net.Flags
	HardwareAddress net.HardwareAddr
}

// Provider makes interface discovery replaceable in tests.
type Provider interface {
	Interfaces() ([]Interface, error)
}

// SystemProvider discovers interfaces from the running operating system.
type SystemProvider struct{}

func (SystemProvider) Interfaces() ([]Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	result := make([]Interface, 0, len(interfaces))
	for _, networkInterface := range interfaces {
		result = append(result, Interface{
			Index:           networkInterface.Index,
			Name:            networkInterface.Name,
			Flags:           networkInterface.Flags,
			HardwareAddress: networkInterface.HardwareAddr,
		})
	}
	return result, nil
}

// Report records the exact interfaces accepted for each routed role.
type Report struct {
	WAN Interface
	LAN Interface
}

// Validate confirms that configured roles refer to distinct, active physical
// interfaces. It intentionally does not guess roles from enumeration order.
func Validate(networkConfig config.NetworkConfig, provider Provider) (Report, error) {
	if !networkConfig.Enabled {
		return Report{}, fmt.Errorf("network.enabled must be true for routed topology validation")
	}
	if provider == nil {
		return Report{}, fmt.Errorf("interface provider is required")
	}

	interfaces, err := provider.Interfaces()
	if err != nil {
		return Report{}, fmt.Errorf("enumerate network interfaces: %w", err)
	}

	byName := make(map[string]Interface, len(interfaces))
	for _, networkInterface := range interfaces {
		byName[networkInterface.Name] = networkInterface
	}

	wan, wanFound := byName[networkConfig.WANInterface]
	lan, lanFound := byName[networkConfig.LANInterface]
	var failures []error
	if !wanFound {
		failures = append(failures, fmt.Errorf("WAN interface %q was not found", networkConfig.WANInterface))
	} else if err := validateRole("WAN", wan); err != nil {
		failures = append(failures, err)
	}
	if !lanFound {
		failures = append(failures, fmt.Errorf("LAN interface %q was not found", networkConfig.LANInterface))
	} else if err := validateRole("LAN", lan); err != nil {
		failures = append(failures, err)
	}
	if wanFound && lanFound && wan.Index == lan.Index {
		failures = append(failures, fmt.Errorf("WAN and LAN roles resolve to the same interface index %d", wan.Index))
	}
	if len(failures) > 0 {
		return Report{}, fmt.Errorf("invalid routed topology: %w", errors.Join(failures...))
	}

	return Report{WAN: wan, LAN: lan}, nil
}

func validateRole(role string, networkInterface Interface) error {
	var failures []error
	if networkInterface.Flags&net.FlagUp == 0 {
		failures = append(failures, fmt.Errorf("%s interface %q is not administratively up", role, networkInterface.Name))
	}
	if networkInterface.Flags&net.FlagLoopback != 0 {
		failures = append(failures, fmt.Errorf("%s interface %q is loopback", role, networkInterface.Name))
	}
	if len(networkInterface.HardwareAddress) == 0 {
		failures = append(failures, fmt.Errorf("%s interface %q has no hardware address", role, networkInterface.Name))
	}
	return errors.Join(failures...)
}
