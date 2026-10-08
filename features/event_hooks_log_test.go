package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// eventHookLogLines returns the structured log's "event_hook" lines (SPEC
// §13.1) for the session named name, oldest first.
func eventHookLogLines(ctx context.Context, name string) ([]map[string]json.RawMessage, error) {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return nil, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var id string
	if err := db.QueryRowContext(ctx, "SELECT id FROM sessions WHERE name = ?", name).Scan(&id); err != nil {
		return nil, fmt.Errorf("observe session %q: %w", name, err)
	}
	if _, err := os.Stat(filepath.Join(h.Home, "log", "deck.jsonl")); err != nil {
		return nil, err
	}
	records, err := readAudit(h)
	if err != nil {
		return nil, err
	}
	var lines []map[string]json.RawMessage
	for _, record := range records {
		if string(record["event"]) == `"event_hook"` && string(record["session_id"]) == fmt.Sprintf("%q", id) {
			lines = append(lines, record)
		}
	}
	return lines, nil
}

// eventHookLogOutcome checks one line against "exit status N", "timed out"
// or "detached" (no exit status and no duration: deck never waited).
func eventHookLogOutcome(line map[string]json.RawMessage, kind, outcome string) error {
	if string(line["kind"]) != fmt.Sprintf("%q", kind) {
		return fmt.Errorf("kind = %s, want %q", line["kind"], kind)
	}
	var exit int
	switch outcome {
	case "detached":
		if string(line["detached"]) != "true" || line["exit_code"] != nil || line["script_duration_ms"] != nil {
			return fmt.Errorf("line %v is not a detached invocation without exit status", line)
		}
		return nil
	case "timed out":
		if string(line["timed_out"]) != "true" {
			return fmt.Errorf("timed_out = %s, want true", line["timed_out"])
		}
	default:
		if _, err := fmt.Sscanf(outcome, "exit status %d", &exit); err != nil || string(line["exit_code"]) != fmt.Sprint(exit) {
			return fmt.Errorf("exit_code = %s, want %s", line["exit_code"], outcome)
		}
	}
	if line["script_duration_ms"] == nil {
		return fmt.Errorf("line %v carries no script_duration_ms", line)
	}
	return nil
}

// structuredLogRecordsEventHook polls (the TUI's own events dispatch
// asynchronously) until the log holds exactly want event_hook lines for the
// session and the last one matches kind and outcome.
func structuredLogRecordsEventHook(ctx context.Context, want int, name, kind, outcome string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		lines, err := eventHookLogLines(ctx, name)
		if err == nil && len(lines) != want {
			err = fmt.Errorf("%d event_hook lines, want %d", len(lines), want)
		}
		if err == nil {
			err = eventHookLogOutcome(lines[len(lines)-1], kind, outcome)
		}
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("structured log for session %q: %w", name, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
