package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
)

// eventHookLogLines returns the "event_hook" lines of the fixture's log.
func eventHookLogLines(t *testing.T, logger *audit.Logger) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(logger.Path())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["event"] == "event_hook" {
			lines = append(lines, record)
		}
	}
	return lines
}

func withHookAudit(t *testing.T, f *hookDispatchFixture) *audit.Logger {
	t.Helper()
	clock, err := config.NewClock("", "")
	if err != nil {
		t.Fatal(err)
	}
	logger, err := audit.New(config.Paths{LogDir: filepath.Join(t.TempDir(), "log")}, clock)
	if err != nil {
		t.Fatal(err)
	}
	f.d.Audit = logger
	return logger
}

// TestEventHookDispatchLogsEachInvocationWithExitStatusAndDuration pins SPEC
// §13.1: every spawn, attached, detached or never started, appends one
// "event_hook" line to deck.jsonl; a skipped offer appends none.
func TestEventHookDispatchLogsEachInvocationWithExitStatusAndDuration(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	logger := withHookAudit(t, &f)
	ctx := context.Background()
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "permission_prompt", At: time.Now()}
	if out := f.d.Dispatch(ctx, ev, false); !out.Spawned || out.Err != nil {
		t.Fatalf("attached dispatch = %+v", out)
	}
	if out := f.d.Dispatch(ctx, ev, false); out.Spawned {
		t.Fatalf("a deduped offer spawned: %+v", out)
	}
	lines := eventHookLogLines(t, logger)
	if len(lines) != 1 {
		t.Fatalf("event_hook lines = %v, want exactly one (the dedupe skip logs nothing)", lines)
	}
	first := lines[0]
	if first["session_id"] != "s1" || first["kind"] != "waiting" || first["exit_code"] != float64(0) {
		t.Errorf("attached line = %#v, want s1/waiting/exit_code 0", first)
	}
	if ms, ok := first["script_duration_ms"].(float64); !ok || ms <= 0 {
		t.Errorf("attached line script_duration_ms = %#v, want a positive duration", first["script_duration_ms"])
	}

	stopped := newHookDispatchFixture(t, "stopped")
	stoppedLog := withHookAudit(t, &stopped)
	if out := stopped.d.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "session_end", Reason: "logout"}, true); !out.Spawned {
		t.Fatalf("detached dispatch = %+v", out)
	}
	if lines := eventHookLogLines(t, stoppedLog); len(lines) != 1 || lines[0]["kind"] != "ended" || lines[0]["detached"] != true || lines[0]["exit_code"] != nil {
		t.Errorf("detached lines = %v, want one ended line with detached and no exit status", lines)
	}

	missing := newHookDispatchFixture(t, "waiting")
	missingLog := withHookAudit(t, &missing)
	missing.d.Policy.Command = []string{filepath.Join(missing.out, "nope")}
	if out := missing.d.Dispatch(ctx, ev, false); out.Spawned {
		t.Fatalf("missing script spawned: %+v", out)
	}
	if lines := eventHookLogLines(t, missingLog); len(lines) != 1 || lines[0]["exit_code"] != float64(-1) || lines[0]["error"] == nil {
		t.Errorf("missing-script lines = %v, want exit_code -1 and an error", lines)
	}
}
