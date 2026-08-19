package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerHookContractSteps observes the released _hook command through only
// its SQLite and JSONL boundaries. The setup client exits before the hook is
// invoked, making the latency scenario genuinely single-writer.
func registerHookContractSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an uncontended Claude hook target "([^"]+)" exists in the scenario store$`, uncontendedClaudeHookTarget)
	sc.Step(`^the released hook receiver handles a "([^"]+)" event for "([^"]+)"$`, releasedHookHandlesEvent)
	sc.Step(`^exactly one hook store write is recorded$`, exactlyOneHookStoreWrite)
	sc.Step(`^its operation-scoped store duration is below ([0-9]+) milliseconds$`, hookStoreDurationBelow)
	sc.Step(`^the scenario store session "([^"]+)" is stopped by session end$`, sessionStoppedBySessionEnd)
	sc.Step(`^the hook audit records no liveness, probe, dispatch, or enqueue attempt$`, hookAuditHasNoSubsequentWork)
}

func uncontendedClaudeHookTarget(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	// Starting and cleanly stopping the released TUI creates the real schema;
	// the row itself is then an external fixture, as in store_feature_test.go.
	client, err := h.StartNamedClient(ctx, "schema-bootstrap")
	if err != nil {
		return fmt.Errorf("start schema bootstrap client: %w", err)
	}
	if err := client.WaitForFrame(ctx, false, "deck - sessions"); err != nil {
		return err
	}
	if err := client.Send("q"); err != nil {
		return err
	}
	if err := client.Stop(5 * time.Second); err != nil {
		return fmt.Errorf("stop schema bootstrap client: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
	if err != nil {
		return fmt.Errorf("open scenario store fixture: %w", err)
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `INSERT INTO sessions
		(id, name, slug, cwd, agent, captured_path, conversation_id,
		 status, status_source, status_at, created_at)
		VALUES (?, ?, ?, ?, 'claude', ?, ?, 'starting', 'user', 1, 1)`,
		"hook-"+name, name, "hook-"+name, h.Home, os.Getenv("PATH"), "conversation-"+name)
	if err != nil {
		return fmt.Errorf("insert Claude hook target %q: %w", name, err)
	}
	return nil
}

func releasedHookHandlesEvent(ctx context.Context, event, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"hook_event_name": event,
		"session_id":      "conversation-" + name,
	}
	switch event {
	case "Notification":
		payload["notification_type"] = "permission_prompt"
	case "SessionEnd":
		payload["reason"] = "logout"
	default:
		return fmt.Errorf("unsupported hook-contract fixture event %q", event)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, h.Binary, "_hook")
	cmd.Env = append(os.Environ(), h.Environment("DECK_SESSION_ID=hook-"+name)...)
	cmd.Stdin = strings.NewReader(string(encoded))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("released hook receiver: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func hookStoreWriteRecords(ctx context.Context) ([]map[string]json.RawMessage, error) {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return nil, err
	}
	records, err := readAudit(h)
	if err != nil {
		return nil, err
	}
	var writes []map[string]json.RawMessage
	for _, record := range records {
		var event string
		if err := json.Unmarshal(record["event"], &event); err != nil {
			return nil, fmt.Errorf("decode audit event: %w", err)
		}
		if event == "hook.store_write" {
			writes = append(writes, record)
		}
	}
	return writes, nil
}

func exactlyOneHookStoreWrite(ctx context.Context) error {
	writes, err := hookStoreWriteRecords(ctx)
	if err != nil {
		return err
	}
	if len(writes) != 1 {
		return fmt.Errorf("hook store write count = %d, want exactly 1", len(writes))
	}
	var succeeded bool
	if err := json.Unmarshal(writes[0]["succeeded"], &succeeded); err != nil || !succeeded {
		return fmt.Errorf("hook store write succeeded = %v, decode error %v", succeeded, err)
	}
	return nil
}

func hookStoreDurationBelow(ctx context.Context, limit int) error {
	writes, err := hookStoreWriteRecords(ctx)
	if err != nil {
		return err
	}
	if len(writes) != 1 {
		return fmt.Errorf("hook store write count = %d, want exactly 1", len(writes))
	}
	var duration float64
	if err := json.Unmarshal(writes[0]["store_duration_ms"], &duration); err != nil {
		return fmt.Errorf("decode operation-scoped store duration: %w", err)
	}
	if duration < 0 || duration >= float64(limit) {
		return fmt.Errorf("operation-scoped hook store duration = %.3fms, want < %dms", duration, limit)
	}
	return nil
}

func sessionStoppedBySessionEnd(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	var status, source, reason string
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT status, status_source, status_reason FROM sessions WHERE name = ?`, name).Scan(&status, &source, &reason); err != nil {
		return fmt.Errorf("read ended session: %w", err)
	}
	if status != "stopped" || source != "hook" || reason != "logout" {
		return fmt.Errorf("ended session = status %q source %q reason %q", status, source, reason)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = 'session_end'`, "hook-"+name).Scan(&eventCount); err != nil {
		return err
	}
	if eventCount != 1 {
		return fmt.Errorf("session_end event count = %d, want 1", eventCount)
	}
	return nil
}

func hookAuditHasNoSubsequentWork(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	records, err := readAudit(h)
	if err != nil {
		return err
	}
	// The bootstrap client performs no transitions, so the terminal hook must
	// leave exactly its measured store-write line. This is stronger than a
	// name-prefix check and pins the Phase 2 no-enqueue absence for Phase 5.
	if len(records) != 1 {
		var events []string
		for _, record := range records {
			events = append(events, string(record["event"]))
		}
		return fmt.Errorf("session-end hook audit events = %v, want only one store write", events)
	}
	var event string
	if err := json.Unmarshal(records[0]["event"], &event); err != nil {
		return err
	}
	if event != "hook.store_write" {
		return fmt.Errorf("session-end hook audit event = %q, want hook.store_write", event)
	}
	return nil
}
