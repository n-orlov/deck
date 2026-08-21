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
	// pi's "starting", "idle" and "waiting" have no rule and no fixture —
	// see testdata/probes/pi-PROVENANCE.md (SPEC requirement 38).
	{"pi", "running.txt", "running", "working indicator"},
	{"pi", "error.txt", "error", "agent error"},
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
