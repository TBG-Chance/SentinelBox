// Package nftables applies and verifies only SentinelBox-owned nftables tables.
package nftables

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/TBG-Chance/SentinelBox/internal/network/baseline"
)

const maxCommandOutput = 8 << 20

// Runner isolates process execution so safety behavior can be verified without
// requiring root or modifying the test host firewall.
type Runner interface {
	Run(ctx context.Context, stdin []byte, executable string, args ...string) ([]byte, error)
}

// ExecRunner invokes nft directly. It never uses a shell.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin []byte, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Stdin = bytes.NewReader(stdin)
	output := &boundedBuffer{remaining: maxCommandOutput}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if output.truncated {
		output.buffer.WriteString("\n[command output truncated]")
	}
	return output.buffer.Bytes(), err
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	remaining int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
		b.truncated = true
	}
	if len(value) > 0 {
		_, _ = b.buffer.Write(value)
		b.remaining -= len(value)
	}
	return originalLength, nil
}

// Client serializes check/apply/verify operations for one appliance.
type Client struct {
	executable string
	runner     Runner
	mu         sync.Mutex
}

func New(executable string, runner Runner) (*Client, error) {
	if !filepath.IsAbs(executable) && !strings.HasPrefix(executable, "/") {
		return nil, fmt.Errorf("nft executable path must be absolute")
	}
	if filepath.Base(executable) != "nft" {
		return nil, fmt.Errorf("nft executable path must name nft")
	}
	if runner == nil {
		return nil, fmt.Errorf("nft runner is required")
	}
	return &Client{executable: executable, runner: runner}, nil
}

// CheckBaseline asks nft to validate the full replacement batch without
// changing kernel state.
func (c *Client) CheckBaseline(ctx context.Context, ruleset string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, err := c.inspect(ctx)
	if err != nil {
		return err
	}
	return c.check(ctx, replacementBatch(state, ruleset))
}

// ApplyBaseline checks, atomically applies, then verifies the SentinelBox
// tables. Verification failure triggers best-effort removal of owned tables.
func (c *Client) ApplyBaseline(ctx context.Context, ruleset string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.inspect(ctx)
	if err != nil {
		return err
	}
	batch := replacementBatch(state, ruleset)
	if err := c.check(ctx, batch); err != nil {
		return err
	}
	if err := c.runFile(ctx, batch); err != nil {
		return fmt.Errorf("apply SentinelBox nftables baseline: %w", err)
	}
	if err := c.verify(ctx); err != nil {
		cleanupErr := c.removeOwnedLocked(ctx)
		if cleanupErr != nil {
			return fmt.Errorf("verify applied nftables baseline: %w; cleanup also failed: %v", err, cleanupErr)
		}
		return fmt.Errorf("verify applied nftables baseline: %w; owned tables were removed", err)
	}
	return nil
}

// RemoveOwned removes only the two tables named by the baseline package.
func (c *Client) RemoveOwned(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeOwnedLocked(ctx)
}

func (c *Client) removeOwnedLocked(ctx context.Context) error {
	state, err := c.inspect(ctx)
	if err != nil {
		return err
	}
	batch := deletionBatch(state)
	if batch == "" {
		return nil
	}
	if err := c.check(ctx, batch); err != nil {
		return err
	}
	if err := c.runFile(ctx, batch); err != nil {
		return fmt.Errorf("remove SentinelBox-owned nftables tables: %w", err)
	}
	return nil
}

func (c *Client) verify(ctx context.Context) error {
	state, err := c.inspect(ctx)
	if err != nil {
		return err
	}
	var failures []error
	if !state.hasTable(baseline.FilterFamily, baseline.FilterTable) {
		failures = append(failures, fmt.Errorf("missing %s table %s", baseline.FilterFamily, baseline.FilterTable))
	}
	if !state.hasTable(baseline.NATFamily, baseline.NATTable) {
		failures = append(failures, fmt.Errorf("missing %s table %s", baseline.NATFamily, baseline.NATTable))
	}
	for _, setName := range []string{"sbox_watch", "sbox_restricted", "sbox_contained", "sbox_trusted"} {
		if !state.hasSet(baseline.FilterFamily, baseline.FilterTable, setName) {
			failures = append(failures, fmt.Errorf("missing set %s", setName))
		}
	}
	for _, expected := range []struct {
		family string
		table  string
		name   string
		type_  string
		hook   string
		policy string
	}{
		{baseline.FilterFamily, baseline.FilterTable, "input", "filter", "input", "drop"},
		{baseline.FilterFamily, baseline.FilterTable, "forward", "filter", "forward", "drop"},
		{baseline.FilterFamily, baseline.FilterTable, "output", "filter", "output", "accept"},
		{baseline.NATFamily, baseline.NATTable, "postrouting", "nat", "postrouting", "accept"},
	} {
		chain, ok := state.chain(expected.family, expected.table, expected.name)
		if !ok {
			failures = append(failures, fmt.Errorf("missing chain %s", expected.name))
			continue
		}
		if chain.Type != expected.type_ || chain.Hook != expected.hook || chain.Policy != expected.policy {
			failures = append(failures, fmt.Errorf("chain %s has type=%q hook=%q policy=%q", expected.name, chain.Type, chain.Hook, chain.Policy))
		}
	}
	return errors.Join(failures...)
}

func (c *Client) check(ctx context.Context, batch string) error {
	output, err := c.runner.Run(ctx, []byte(batch), c.executable, "--check", "--file", "-")
	if err != nil {
		return commandError("check SentinelBox nftables batch", output, err)
	}
	return nil
}

func (c *Client) runFile(ctx context.Context, batch string) error {
	output, err := c.runner.Run(ctx, []byte(batch), c.executable, "--file", "-")
	if err != nil {
		return commandError("load SentinelBox nftables batch", output, err)
	}
	return nil
}

type nftState struct {
	tables map[string]struct{}
	sets   map[string]struct{}
	chains map[string]chainState
}

type chainState struct {
	Type   string
	Hook   string
	Policy string
}

func (c *Client) inspect(ctx context.Context) (nftState, error) {
	output, err := c.runner.Run(ctx, nil, c.executable, "--json", "list", "ruleset")
	if err != nil {
		return nftState{}, commandError("inspect nftables ruleset", output, err)
	}

	var document struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nftState{}, fmt.Errorf("decode nftables JSON: %w", err)
	}
	state := nftState{
		tables: make(map[string]struct{}),
		sets:   make(map[string]struct{}),
		chains: make(map[string]chainState),
	}
	for _, object := range document.Nftables {
		if rawTable, ok := object["table"]; ok {
			var table struct {
				Family string `json:"family"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(rawTable, &table); err != nil {
				return nftState{}, fmt.Errorf("decode nftables table: %w", err)
			}
			state.tables[tableKey(table.Family, table.Name)] = struct{}{}
		}
		if rawSet, ok := object["set"]; ok {
			var set struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(rawSet, &set); err != nil {
				return nftState{}, fmt.Errorf("decode nftables set: %w", err)
			}
			state.sets[setKey(set.Family, set.Table, set.Name)] = struct{}{}
		}
		if rawChain, ok := object["chain"]; ok {
			var chain struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
				Type   string `json:"type"`
				Hook   string `json:"hook"`
				Policy string `json:"policy"`
			}
			if err := json.Unmarshal(rawChain, &chain); err != nil {
				return nftState{}, fmt.Errorf("decode nftables chain: %w", err)
			}
			state.chains[chainKey(chain.Family, chain.Table, chain.Name)] = chainState{
				Type: chain.Type, Hook: chain.Hook, Policy: chain.Policy,
			}
		}
	}
	return state, nil
}

func (s nftState) hasTable(family, table string) bool {
	_, ok := s.tables[tableKey(family, table)]
	return ok
}

func (s nftState) hasSet(family, table, set string) bool {
	_, ok := s.sets[setKey(family, table, set)]
	return ok
}

func (s nftState) chain(family, table, chain string) (chainState, bool) {
	value, ok := s.chains[chainKey(family, table, chain)]
	return value, ok
}

func replacementBatch(state nftState, ruleset string) string {
	return deletionBatch(state) + strings.TrimSpace(ruleset) + "\n"
}

func deletionBatch(state nftState) string {
	var batch strings.Builder
	if state.hasTable(baseline.FilterFamily, baseline.FilterTable) {
		fmt.Fprintf(&batch, "delete table %s %s\n", baseline.FilterFamily, baseline.FilterTable)
	}
	if state.hasTable(baseline.NATFamily, baseline.NATTable) {
		fmt.Fprintf(&batch, "delete table %s %s\n", baseline.NATFamily, baseline.NATTable)
	}
	return batch.String()
}

func tableKey(family, table string) string {
	return family + "\x00" + table
}

func setKey(family, table, set string) string {
	return family + "\x00" + table + "\x00" + set
}

func chainKey(family, table, chain string) string {
	return family + "\x00" + table + "\x00" + chain
}

func commandError(operation string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s: %w: %s", operation, err, detail)
}
