package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TBG-Chance/SentinelBox/internal/network/topology"
)

type cliInterfaceProvider struct{}

func (cliInterfaceProvider) Interfaces() ([]topology.Interface, error) {
	return []topology.Interface{
		{Index: 2, Name: "eno1", Flags: net.FlagUp, HardwareAddress: net.HardwareAddr{2, 0, 0, 0, 0, 1}},
		{Index: 3, Name: "enx001122334455", Flags: net.FlagUp, HardwareAddress: net.HardwareAddr{2, 0, 0, 0, 0, 2}},
	}, nil
}

func TestRunRenderStagesBaseline(t *testing.T) {
	configPath := writeRoutedConfig(t)
	outputPath := filepath.Join(t.TempDir(), "staging")
	var stdout, stderr bytes.Buffer
	code := run([]string{"render", "--config", configPath, "--output", outputPath}, &stdout, &stderr, dependencies{})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outputPath, "etc", "dnsmasq.d", "sentinelbox.conf")); err != nil {
		t.Fatalf("staged dnsmasq file: %v", err)
	}
	if !strings.Contains(stdout.String(), "staged 5") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunValidateUsesExplicitRoles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--config", writeRoutedConfig(t)}, &stdout, &stderr, dependencies{interfaces: cliInterfaceProvider{}})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "WAN eno1 and LAN enx001122334455") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunApplyRequiresExplicitConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"apply"}, &stdout, &stderr, dependencies{})
	if code != 1 || !strings.Contains(stderr.String(), "--confirm-routed-network-change") {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
}

func writeRoutedConfig(t *testing.T) string {
	t.Helper()
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "sentinel.db"))
	contents := fmt.Sprintf(`version = 1

[database]
path = %q

[network]
enabled = true
wan_interface = "eno1"
lan_interface = "enx001122334455"

[dhcp]
enabled = true
`, databasePath)
	path := filepath.Join(t.TempDir(), "sentinelbox.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
