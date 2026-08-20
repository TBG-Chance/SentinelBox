package baseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

func TestGenerateRoutedBaseline(t *testing.T) {
	cfg := routedConfig()
	bundle, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if len(bundle) != 5 {
		t.Fatalf("len(bundle) = %d, want 5", len(bundle))
	}

	files := make(map[string]string, len(bundle))
	for _, file := range bundle {
		files[file.Path] = string(file.Contents)
	}
	assertContains(t, files["etc/systemd/network/10-sentinelbox-wan.network"], "DHCP=ipv4")
	assertContains(t, files["etc/systemd/network/20-sentinelbox-lan.network"], "Address=192.168.50.1/24")
	assertContains(t, files["etc/dnsmasq.d/sentinelbox.conf"], "dhcp-range=192.168.50.100,192.168.50.199,255.255.255.0,12h")
	assertContains(t, files["etc/sysctl.d/90-sentinelbox-forwarding.conf"], "net.ipv6.conf.all.forwarding=0")

	rules := files["etc/nftables.d/sentinelbox.nft"]
	for _, expected := range []string{
		"table inet sentinelbox_filter",
		"table ip sentinelbox_nat",
		"policy drop",
		"iifname \"enxusb\" oifname \"eno1\" ip saddr 192.168.50.0/24 accept",
		"oifname \"eno1\" ip saddr 192.168.50.0/24 masquerade",
	} {
		assertContains(t, rules, expected)
	}
	for _, forbidden := range []string{"flush ruleset", "dport 8080", "policy accept;\n\t\tiifname \"eno1\""} {
		if strings.Contains(rules, forbidden) {
			t.Fatalf("ruleset contains forbidden text %q", forbidden)
		}
	}
}

func TestGenerateWithoutDHCPDoesNotExposeServices(t *testing.T) {
	cfg := routedConfig()
	cfg.DHCP.Enabled = false
	bundle, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if len(bundle) != 4 {
		t.Fatalf("len(bundle) = %d, want 4", len(bundle))
	}
	for _, file := range bundle {
		if file.Path == "etc/dnsmasq.d/sentinelbox.conf" {
			t.Fatal("dnsmasq configuration generated while DHCP is disabled")
		}
		if file.Path == "etc/nftables.d/sentinelbox.nft" && strings.Contains(string(file.Contents), "dport 53") {
			t.Fatal("DNS firewall allowance generated while DHCP is disabled")
		}
	}
}

func TestBundleWriteStagesFiles(t *testing.T) {
	bundle, err := Generate(routedConfig())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "staging")
	if err := bundle.Write(root); err != nil {
		t.Fatalf("Write(): %v", err)
	}
	if err := bundle.Write(root); err != nil {
		t.Fatalf("second Write() must be safe and repeatable: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "etc", "nftables.d", "sentinelbox.nft"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(contents), "sentinelbox_filter")
}

func TestBundleWriteRejectsEscapingPath(t *testing.T) {
	bundle := Bundle{{Path: "../escape", Mode: 0o600, Contents: []byte("unsafe")}}
	if err := bundle.Write(filepath.Join(t.TempDir(), "staging")); err == nil {
		t.Fatal("Write() accepted an escaping path")
	}
}

func routedConfig() config.Config {
	cfg := config.Default()
	cfg.Network.Enabled = true
	cfg.Network.WANInterface = "eno1"
	cfg.Network.LANInterface = "enxusb"
	cfg.DHCP.Enabled = true
	return cfg
}

func assertContains(t *testing.T, value, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("value does not contain %q:\n%s", substring, value)
	}
}
