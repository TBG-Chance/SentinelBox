package topology

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

type fakeProvider struct {
	interfaces []Interface
	err        error
}

func (p fakeProvider) Interfaces() ([]Interface, error) {
	return p.interfaces, p.err
}

func TestValidateAcceptsExplicitPhysicalRoles(t *testing.T) {
	cfg := routedNetworkConfig()
	provider := fakeProvider{interfaces: []Interface{
		{Index: 2, Name: "eno1", Flags: net.FlagUp, HardwareAddress: mustMAC(t, "02:00:00:00:00:01")},
		{Index: 7, Name: "enxusb", Flags: net.FlagUp, HardwareAddress: mustMAC(t, "02:00:00:00:00:02")},
	}}

	report, err := Validate(cfg, provider)
	if err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if report.WAN.Name != "eno1" || report.LAN.Name != "enxusb" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidateRejectsUnsafeTopology(t *testing.T) {
	tests := []struct {
		name      string
		provider  fakeProvider
		wantError string
	}{
		{
			name: "missing LAN",
			provider: fakeProvider{interfaces: []Interface{
				{Index: 2, Name: "eno1", Flags: net.FlagUp, HardwareAddress: mustMAC(t, "02:00:00:00:00:01")},
			}},
			wantError: "LAN interface",
		},
		{
			name: "down WAN",
			provider: fakeProvider{interfaces: []Interface{
				{Index: 2, Name: "eno1", HardwareAddress: mustMAC(t, "02:00:00:00:00:01")},
				{Index: 7, Name: "enxusb", Flags: net.FlagUp, HardwareAddress: mustMAC(t, "02:00:00:00:00:02")},
			}},
			wantError: "not administratively up",
		},
		{
			name:      "provider failure",
			provider:  fakeProvider{err: errors.New("discovery failed")},
			wantError: "enumerate network interfaces",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(routedNetworkConfig(), test.provider)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestValidateRejectsDisabledNetwork(t *testing.T) {
	cfg := routedNetworkConfig()
	cfg.Enabled = false
	_, err := Validate(cfg, fakeProvider{})
	if err == nil || !strings.Contains(err.Error(), "network.enabled") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func routedNetworkConfig() config.NetworkConfig {
	cfg := config.Default().Network
	cfg.Enabled = true
	cfg.WANInterface = "eno1"
	cfg.LANInterface = "enxusb"
	return cfg
}

func mustMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	address, err := net.ParseMAC(value)
	if err != nil {
		t.Fatal(err)
	}
	return address
}
