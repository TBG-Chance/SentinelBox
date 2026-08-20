package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValidAndSafe(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default().Validate(): %v", err)
	}
	if cfg.Network.Enabled || cfg.Suricata.Enabled {
		t.Fatal("network and Suricata must be disabled in the foundation configuration")
	}
	if cfg.Response.AutomaticRestriction || cfg.Response.AutomaticContainment {
		t.Fatal("automatic response must be disabled in the foundation configuration")
	}
}

func TestLoadUsesDefaultsAndRejectsUnknownFields(t *testing.T) {
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "sentinel.db"))
	validPath := writeConfig(t, fmt.Sprintf("version = 1\n\n[database]\npath = %q\n", databasePath))
	cfg, err := Load(validPath)
	if err != nil {
		t.Fatalf("Load(valid): %v", err)
	}
	if cfg.Database.Path != databasePath {
		t.Fatalf("database path = %q, want %q", cfg.Database.Path, databasePath)
	}
	if cfg.Server.Listen != "127.0.0.1:8080" {
		t.Fatalf("server default was not retained: %q", cfg.Server.Listen)
	}

	unknownPath := writeConfig(t, fmt.Sprintf("version = 1\nunknown = true\n\n[database]\npath = %q\n", databasePath))
	_, err = Load(unknownPath)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load(unknown) error = %v, want an unknown-field error", err)
	}
}

func TestValidateRejectsUnsafeFoundationSettings(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{
			name:    "non-loopback management",
			mutate:  func(cfg *Config) { cfg.Server.Listen = "0.0.0.0:8080" },
			message: "loopback",
		},
		{
			name:    "automatic containment",
			mutate:  func(cfg *Config) { cfg.Response.AutomaticContainment = true },
			message: "automatic response",
		},
		{
			name: "unordered thresholds",
			mutate: func(cfg *Config) {
				cfg.Risk.RestrictThreshold = cfg.Risk.WatchThreshold
			},
			message: "strictly increasing",
		},
		{
			name: "same interfaces",
			mutate: func(cfg *Config) {
				cfg.Network.Enabled = true
				cfg.Network.WANInterface = "eno1"
				cfg.Network.LANInterface = "eno1"
			},
			message: "must be different",
		},
		{
			name:    "WAN management",
			mutate:  func(cfg *Config) { cfg.Management.AllowWAN = true },
			message: "must remain false",
		},
		{
			name:    "LAN management disabled",
			mutate:  func(cfg *Config) { cfg.Management.BindLANOnly = false },
			message: "bind_lan_only",
		},
		{
			name:    "latent IPv6 forwarding",
			mutate:  func(cfg *Config) { cfg.Network.EnableIPv6Forwarding = true },
			message: "IPv6 forwarding",
		},
		{
			name:    "relative nft executable",
			mutate:  func(cfg *Config) { cfg.Network.NftBinary = "nft" },
			message: "nft_binary must be absolute",
		},
		{
			name:    "LAN network address used as gateway",
			mutate:  func(cfg *Config) { cfg.Network.LANAddress = "192.168.50.0/24" },
			message: "must use a host address",
		},
		{
			name: "DHCP range crosses gateway",
			mutate: func(cfg *Config) {
				cfg.DHCP.RangeStart = "192.168.50.1"
				cfg.DHCP.RangeEnd = "192.168.50.100"
			},
			message: "must not include the SentinelBox LAN address",
		},
		{
			name:    "invalid disabled Suricata path",
			mutate:  func(cfg *Config) { cfg.Suricata.EVEPath = "relative/eve.json" },
			message: "eve_path must be absolute",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			test.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate() error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestDurationRejectsInvalidValue(t *testing.T) {
	var duration Duration
	if err := duration.UnmarshalText([]byte("not-a-duration")); err == nil {
		t.Fatal("UnmarshalText accepted an invalid duration")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sentinelbox.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
