package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readClaudeGolden(t *testing.T, name string) string {
	t.Helper()
	pane, err := os.ReadFile(filepath.Join("testdata", "probes", "claude", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(pane)
}

// TestClaudeWaitingShapesProbeWaiting pins R312: each dialog shape whose marker
// repo evidence establishes probes `waiting` with its own reason.
func TestClaudeWaitingShapesProbeWaiting(t *testing.T) {
	shapes := []struct{ file, reason, marker string }{
		{"waiting.txt", "permission prompt", "Do you want to proceed?"},
		{"waiting-edit.txt", "file edit confirmation", "Do you want to make this edit"},
		{"waiting-create.txt", "file create confirmation", "Do you want to create"},
		{"waiting-trust.txt", "folder trust", "Do you trust the files in this folder"},
	}
	for _, shape := range shapes {
		t.Run(shape.file, func(t *testing.T) {
			pane := readClaudeGolden(t, shape.file)
			if !strings.Contains(pane, shape.marker) {
				t.Fatalf("golden %s lacks marker %q", shape.file, shape.marker)
			}
			status, reason := NewClaude().Probe(pane)
			if status != "waiting" || reason != shape.reason {
				t.Fatalf("Probe(%s) = (%q, %q), want (waiting, %q)", shape.file, status, reason, shape.reason)
			}
		})
	}
}

// TestClaudeNonWaitingGoldensDoNotYieldWaiting: the idle, running, starting and
// error goldens are never waiting. idle.txt was replaced by the idle-* goldens
// in R310 (claude-PROVENANCE.md), so those stand for it; every golden in the
// directory that is not a waiting shape is swept as well.
func TestClaudeNonWaitingGoldensDoNotYieldWaiting(t *testing.T) {
	want := map[string]string{
		"idle-scrolled.txt":  "idle",
		"idle-draft.txt":     "idle",
		"idle-altscreen.txt": "idle",
		"running.txt":        "running",
		"starting.txt":       "starting",
		"error.txt":          "error",
	}
	for name, status := range want {
		got, _ := NewClaude().Probe(readClaudeGolden(t, name))
		if got != status || got == "waiting" {
			t.Errorf("Probe(%s) = %q, want %q and never waiting", name, got, status)
		}
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "probes", "claude"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "waiting") {
			continue
		}
		if got, _ := NewClaude().Probe(readClaudeGolden(t, entry.Name())); got == "waiting" {
			t.Errorf("non-waiting golden %s probes waiting", entry.Name())
		}
	}
}

// TestClaudeAnsweredFileDialogsAreHistory: like the permission dialog (R311), a
// file-edit, file-create or trust dialog above newer output and a live composer
// is history, not waiting.
func TestClaudeAnsweredFileDialogsAreHistory(t *testing.T) {
	for _, dialog := range []string{
		"Do you want to make this edit to a.go?",
		"Do you want to create a.go?",
		"Do you trust the files in this folder?",
	} {
		pane := "● Update(a.go)\n\n " + dialog + "\n ❯ 1. Yes\n   2. No\n\n● Edited a.go\n  ⎿  done\n\n● All set.\n\n" + claudeComposer
		if status, reason := NewClaude().Probe(pane); status != "idle" {
			t.Errorf("answered %q: Probe = (%q, %q), want idle", dialog, status, reason)
		}
	}
}
