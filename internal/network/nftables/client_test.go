package nftables

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type response struct {
	output string
	err    error
}

type invocation struct {
	stdin string
	args  []string
}

type fakeRunner struct {
	responses   []response
	invocations []invocation
}

func (r *fakeRunner) Run(_ context.Context, stdin []byte, _ string, args ...string) ([]byte, error) {
	r.invocations = append(r.invocations, invocation{stdin: string(stdin), args: append([]string(nil), args...)})
	if len(r.responses) == 0 {
		return nil, errors.New("unexpected invocation")
	}
	next := r.responses[0]
	r.responses = r.responses[1:]
	return []byte(next.output), next.err
}

func TestCheckBaselineUsesDryRunAndPreservesOtherTables(t *testing.T) {
	runner := &fakeRunner{responses: []response{
		{output: rulesetJSON(true, false, false)},
		{},
	}}
	client := newTestClient(t, runner)
	if err := client.CheckBaseline(context.Background(), "table inet sentinelbox_filter {}\ntable ip sentinelbox_nat {}"); err != nil {
		t.Fatalf("CheckBaseline(): %v", err)
	}
	if got := strings.Join(runner.invocations[1].args, " "); got != "--check --file -" {
		t.Fatalf("check args = %q", got)
	}
	batch := runner.invocations[1].stdin
	if !strings.Contains(batch, "delete table inet sentinelbox_filter") {
		t.Fatalf("replacement batch does not delete the existing owned table: %s", batch)
	}
	if strings.Contains(batch, "delete table inet customer_firewall") || strings.Contains(batch, "flush ruleset") {
		t.Fatalf("replacement batch touches non-owned state: %s", batch)
	}
}

func TestApplyBaselineChecksAppliesAndVerifies(t *testing.T) {
	runner := &fakeRunner{responses: []response{
		{output: rulesetJSON(false, false, false)},
		{},
		{},
		{output: rulesetJSON(true, true, true)},
	}}
	client := newTestClient(t, runner)
	if err := client.ApplyBaseline(context.Background(), "table inet sentinelbox_filter {}\ntable ip sentinelbox_nat {}"); err != nil {
		t.Fatalf("ApplyBaseline(): %v", err)
	}
	if len(runner.invocations) != 4 {
		t.Fatalf("invocations = %d, want 4", len(runner.invocations))
	}
	if got := strings.Join(runner.invocations[2].args, " "); got != "--file -" {
		t.Fatalf("apply args = %q", got)
	}
}

func TestApplyBaselineRemovesOwnedTablesAfterVerificationFailure(t *testing.T) {
	runner := &fakeRunner{responses: []response{
		{output: rulesetJSON(false, false, false)},
		{},
		{},
		{output: rulesetJSON(true, false, false)},
		{output: rulesetJSON(true, true, false)},
		{},
		{},
	}}
	client := newTestClient(t, runner)
	err := client.ApplyBaseline(context.Background(), "table inet sentinelbox_filter {}\ntable ip sentinelbox_nat {}")
	if err == nil || !strings.Contains(err.Error(), "owned tables were removed") {
		t.Fatalf("ApplyBaseline() error = %v", err)
	}
	cleanupBatch := runner.invocations[len(runner.invocations)-1].stdin
	if !strings.Contains(cleanupBatch, "delete table inet sentinelbox_filter") || !strings.Contains(cleanupBatch, "delete table ip sentinelbox_nat") {
		t.Fatalf("cleanup batch = %s", cleanupBatch)
	}
}

func TestRemoveOwnedIgnoresUnrelatedTables(t *testing.T) {
	runner := &fakeRunner{responses: []response{
		{output: rulesetJSON(true, true, false)},
		{},
		{},
	}}
	client := newTestClient(t, runner)
	if err := client.RemoveOwned(context.Background()); err != nil {
		t.Fatalf("RemoveOwned(): %v", err)
	}
	batch := runner.invocations[2].stdin
	if strings.Contains(batch, "customer_firewall") || strings.Contains(batch, "flush") {
		t.Fatalf("cleanup batch touched unrelated state: %s", batch)
	}
}

func TestNewRejectsShellStyleExecutableLookup(t *testing.T) {
	if _, err := New("nft", &fakeRunner{}); err == nil {
		t.Fatal("New() accepted a non-absolute executable")
	}
}

func newTestClient(t *testing.T, runner Runner) *Client {
	t.Helper()
	client, err := New("/usr/sbin/nft", runner)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func rulesetJSON(filter, nat, sets bool) string {
	objects := []string{`{"metainfo":{"json_schema_version":1}}`, `{"table":{"family":"inet","name":"customer_firewall"}}`}
	if filter {
		objects = append(objects, `{"table":{"family":"inet","name":"sentinelbox_filter"}}`)
	}
	if nat {
		objects = append(objects, `{"table":{"family":"ip","name":"sentinelbox_nat"}}`)
	}
	if sets {
		for _, name := range []string{"sbox_watch", "sbox_restricted", "sbox_contained", "sbox_trusted"} {
			objects = append(objects, `{"set":{"family":"inet","table":"sentinelbox_filter","name":"`+name+`"}}`)
		}
		objects = append(objects,
			`{"chain":{"family":"inet","table":"sentinelbox_filter","name":"input","type":"filter","hook":"input","policy":"drop"}}`,
			`{"chain":{"family":"inet","table":"sentinelbox_filter","name":"forward","type":"filter","hook":"forward","policy":"drop"}}`,
			`{"chain":{"family":"inet","table":"sentinelbox_filter","name":"output","type":"filter","hook":"output","policy":"accept"}}`,
			`{"chain":{"family":"ip","table":"sentinelbox_nat","name":"postrouting","type":"nat","hook":"postrouting","policy":"accept"}}`,
		)
	}
	return `{"nftables":[` + strings.Join(objects, ",") + `]}`
}
