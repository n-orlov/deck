package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// probeGoldens are captured terminal fixtures shared with pane-driven tests.
// The expectation is deliberately independent of probeRules: changing a rule
// cannot silently rewrite the verdict asserted for the captured bytes.
var probeGoldens = []struct {
	kind, file, status, reason string
}{
	{"claude", "starting.txt", "starting", "startup"},
	{"claude", "running.txt", "running", "working indicator"},
	{"claude", "waiting.txt", "waiting", "permission prompt"},
	{"claude", "idle.txt", "idle", "idle prompt"},
	{"claude", "error.txt", "error", "api error"},
	// pi's "starting" and "waiting" have no rule and no fixture — see
	// testdata/probes/pi-PROVENANCE.md (SPEC requirement 38).
	{"pi", "running.txt", "running", "working indicator"},
	{"pi", "error.txt", "error", "agent error"},
	{"pi", "idle.txt", "idle", "status footer, no working/error indicator"},
	// A real capture taken mid-way through a 25s tool call: proves the idle
	// rule's ordering (task 037 / operator steer) is sufficient on its own —
	// "Working..." is still on screen throughout a long tool call, so this
	// still resolves to running, not idle.
	{"pi", "sleep-midrun.txt", "running", "working indicator"},
}

func TestProbeGoldenPaneCorpus(t *testing.T) {
	adapters := map[string]Adapter{"claude": NewClaude(), "pi": NewPi()}
	var tested []string
	for _, golden := range probeGoldens {
		golden := golden
		t.Run(golden.kind+"/"+golden.file, func(t *testing.T) {
			path := filepath.Join("testdata", "probes", golden.kind, golden.file)
			pane, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read captured pane fixture %s: %v", path, err)
			}
			status, reason := adapters[golden.kind].Probe(string(pane))
			if status != golden.status || reason != golden.reason {
				t.Fatalf("Probe(captured %s) = (%q, %q), want (%q, %q)", path, status, reason, golden.status, golden.reason)
			}
		})
		tested = append(tested, golden.kind+"/"+golden.file)
	}

	// Every corpus file must have an explicit expectation. This turns an
	// upstream prompt update into a reviewed golden change, not dead test data.
	var present []string
	for kind := range adapters {
		entries, err := os.ReadDir(filepath.Join("testdata", "probes", kind))
		if err != nil {
			t.Fatalf("read %s corpus: %v", kind, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				present = append(present, kind+"/"+entry.Name())
			}
		}
	}
	sort.Strings(tested)
	sort.Strings(present)
	if !reflect.DeepEqual(tested, present) {
		t.Fatalf("golden expectations = %v, corpus files = %v", tested, present)
	}
}

func TestProbeDeclinesUnknownTextAndShellIsIneligible(t *testing.T) {
	for _, adapter := range []Adapter{NewClaude(), NewPi(), NewShell()} {
		status, reason := adapter.Probe("ordinary pane output with no agent verdict")
		if status != "" || reason != "" {
			t.Errorf("%s unknown pane verdict = (%q, %q), want no verdict", adapter.Kind(), status, reason)
		}
	}

	// Even agent-looking text is not meaningful for a plain shell.
	status, reason := NewShell().Probe("API Error: text printed by a shell script")
	if status != "" || reason != "" {
		t.Fatalf("shell Probe = (%q, %q), want explicit ineligibility", status, reason)
	}
}

// TestPiIdleRuleStaysLastAmongPiRules proves — not merely asserts by
// inspection — that the pi idle rule (keyed on the durable status-footer
// markers) is ordered after both the pi error and pi running rules, so it
// can only fire on the *absence* of those verdicts, never ahead of them
// (§7: idle must not be inferred from pane liveness alone). Confirmed by
// hand that swapping the idle rule to the front of the pi block makes this
// test fail (task 037 / operator steer 003-pi-idle-rule.md).
func TestPiIdleRuleStaysLastAmongPiRules(t *testing.T) {
	var errorIdx, runningIdx, idleIdx = -1, -1, -1
	for i, rule := range probeRules {
		if rule.kind != "pi" {
			continue
		}
		switch rule.status {
		case "error":
			errorIdx = i
		case "running":
			runningIdx = i
		case "idle":
			idleIdx = i
		}
	}
	if errorIdx == -1 || runningIdx == -1 || idleIdx == -1 {
		t.Fatalf("expected pi error, running and idle rules all present: error=%d running=%d idle=%d", errorIdx, runningIdx, idleIdx)
	}
	if idleIdx < errorIdx {
		t.Fatalf("pi idle rule (index %d) must be ordered after the pi error rule (index %d)", idleIdx, errorIdx)
	}
	if idleIdx < runningIdx {
		t.Fatalf("pi idle rule (index %d) must be ordered after the pi running rule (index %d)", idleIdx, runningIdx)
	}
}

// TestPiIdleRuleDoesNotChangeExistingVerdicts pins that adding the idle rule
// left the previously-established pi/running and pi/error verdicts exactly
// as they were (task 037's requirement not to disturb 007's refit).
func TestPiIdleRuleDoesNotChangeExistingVerdicts(t *testing.T) {
	pi := NewPi()
	for _, golden := range []struct{ file, status, reason string }{
		{"running.txt", "running", "working indicator"},
		{"error.txt", "error", "agent error"},
	} {
		pane, err := os.ReadFile(filepath.Join("testdata", "probes", "pi", golden.file))
		if err != nil {
			t.Fatalf("read %s: %v", golden.file, err)
		}
		status, reason := pi.Probe(string(pane))
		if status != golden.status || reason != golden.reason {
			t.Fatalf("Probe(%s) = (%q, %q), want (%q, %q) unchanged by the idle rule", golden.file, status, reason, golden.status, golden.reason)
		}
	}
}

func TestProbeRuleTableHasOneRulePerGolden(t *testing.T) {
	got := make(map[string]bool)
	for _, rule := range probeRules {
		key := rule.kind + "/" + rule.status
		if got[key] {
			t.Fatalf("duplicate probe rule for %s", key)
		}
		got[key] = true
		if len(rule.contains) == 0 || rule.reason == "" {
			t.Fatalf("incomplete probe rule for %s: %#v", key, rule)
		}
	}
	for _, golden := range probeGoldens {
		key := golden.kind + "/" + golden.status
		if !got[key] {
			t.Errorf("golden %s has no table rule", key)
		}
	}
}
