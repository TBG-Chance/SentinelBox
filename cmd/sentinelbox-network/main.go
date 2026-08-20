// sentinelbox-network prepares and safely manages the Milestone 2 routed
// networking baseline. It is an operator tool, not part of the service loop.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/config"
	"github.com/TBG-Chance/SentinelBox/internal/network/baseline"
	"github.com/TBG-Chance/SentinelBox/internal/network/nftables"
	"github.com/TBG-Chance/SentinelBox/internal/network/topology"
)

const defaultOperationTimeout = 15 * time.Second

type dependencies struct {
	interfaces topology.Provider
	nftRunner  nftables.Runner
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, dependencies{
		interfaces: topology.SystemProvider{},
		nftRunner:  nftables.ExecRunner{},
	}))
}

func run(args []string, stdout, stderr io.Writer, deps dependencies) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	var err error
	switch args[0] {
	case "render":
		err = runRender(args[1:], stdout, stderr)
	case "validate":
		err = runValidate(args[1:], stdout, stderr, deps)
	case "check":
		err = runCheck(args[1:], stdout, stderr, deps)
	case "apply":
		err = runApply(args[1:], stdout, stderr, deps)
	case "remove-owned":
		err = runRemoveOwned(args[1:], stdout, stderr, deps)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "sentinelbox-network: %v\n", err)
		return 1
	}
	return 0
}

func runRender(args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("render", stderr)
	configPath := flags.String("config", "", "absolute path to SentinelBox TOML configuration")
	outputPath := flags.String("output", "", "absolute staging output directory")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	bundle, err := baseline.Generate(cfg)
	if err != nil {
		return err
	}
	if err := bundle.Write(*outputPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "staged %d routed-network files under %s\n", len(bundle), *outputPath)
	return nil
}

func runValidate(args []string, stdout, stderr io.Writer, deps dependencies) error {
	flags := newFlagSet("validate", stderr)
	configPath := flags.String("config", "", "absolute path to SentinelBox TOML configuration")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	report, err := topology.Validate(cfg.Network, deps.interfaces)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "validated WAN %s and LAN %s as distinct active interfaces\n", report.WAN.Name, report.LAN.Name)
	return nil
}

func runCheck(args []string, stdout, stderr io.Writer, deps dependencies) error {
	flags := newFlagSet("check", stderr)
	configPath := flags.String("config", "", "absolute path to SentinelBox TOML configuration")
	timeout := flags.Duration("timeout", defaultOperationTimeout, "maximum nft operation duration")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *timeout <= 0 || *timeout > time.Minute {
		return fmt.Errorf("timeout must be positive and no more than one minute")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if _, err := topology.Validate(cfg.Network, deps.interfaces); err != nil {
		return err
	}
	if err := requireFirewallPrivileges(); err != nil {
		return err
	}
	client, err := nftables.New(cfg.Network.NftBinary, deps.nftRunner)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := client.CheckBaseline(ctx, baseline.Ruleset(cfg.Network, cfg.DHCP.Enabled)); err != nil {
		return err
	}
	fprintln(stdout, "nftables accepted the SentinelBox replacement batch without applying it")
	return nil
}

func runApply(args []string, stdout, stderr io.Writer, deps dependencies) error {
	flags := newFlagSet("apply", stderr)
	configPath := flags.String("config", "", "absolute path to SentinelBox TOML configuration")
	confirmed := flags.Bool("confirm-routed-network-change", false, "confirm an intentional live firewall change")
	timeout := flags.Duration("timeout", defaultOperationTimeout, "maximum nft operation duration")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if !*confirmed {
		return fmt.Errorf("apply requires --confirm-routed-network-change")
	}
	if *timeout <= 0 || *timeout > time.Minute {
		return fmt.Errorf("timeout must be positive and no more than one minute")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if _, err := topology.Validate(cfg.Network, deps.interfaces); err != nil {
		return err
	}
	if err := requireFirewallPrivileges(); err != nil {
		return err
	}
	client, err := nftables.New(cfg.Network.NftBinary, deps.nftRunner)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := client.ApplyBaseline(ctx, baseline.Ruleset(cfg.Network, cfg.DHCP.Enabled)); err != nil {
		return err
	}
	fprintln(stdout, "applied and verified the SentinelBox-owned nftables tables")
	return nil
}

func runRemoveOwned(args []string, stdout, stderr io.Writer, deps dependencies) error {
	flags := newFlagSet("remove-owned", stderr)
	configPath := flags.String("config", "", "absolute path to SentinelBox TOML configuration")
	confirmed := flags.Bool("confirm-remove-owned-tables", false, "confirm removal of SentinelBox-owned tables")
	timeout := flags.Duration("timeout", defaultOperationTimeout, "maximum nft operation duration")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if !*confirmed {
		return fmt.Errorf("remove-owned requires --confirm-remove-owned-tables")
	}
	if *timeout <= 0 || *timeout > time.Minute {
		return fmt.Errorf("timeout must be positive and no more than one minute")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if err := requireFirewallPrivileges(); err != nil {
		return err
	}
	client, err := nftables.New(cfg.Network.NftBinary, deps.nftRunner)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := client.RemoveOwned(ctx); err != nil {
		return err
	}
	fprintln(stdout, "removed SentinelBox-owned nftables tables; unrelated tables were preserved")
	return nil
}

func loadConfig(path string) (config.Config, error) {
	if strings.TrimSpace(path) == "" {
		return config.Config{}, fmt.Errorf("--config is required")
	}
	return config.Load(path)
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return nil
}

func fprintln(writer io.Writer, value string) {
	_, _ = fmt.Fprintln(writer, value)
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `Usage: sentinelbox-network <command> [options]

Commands:
  render        Stage reviewed networkd, dnsmasq, sysctl, and nftables files
  validate      Confirm explicit WAN/LAN interface roles on this host
  check         Ask nft to dry-run the SentinelBox-owned replacement batch
  apply         Apply and verify only the SentinelBox-owned nftables tables
  remove-owned  Remove only the SentinelBox-owned nftables tables

Run a command with -h for its options. Live changes always require an explicit
confirmation flag. This utility never flushes the entire nftables ruleset.`)
}
