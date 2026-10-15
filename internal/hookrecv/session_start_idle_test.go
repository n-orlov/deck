package hookrecv

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// R307: SessionStart, including --resume and --continue, lands on idle and
// never on running, for the three agents whose hook table maps it. One test
// per agent; each fails when the agent's table maps SessionStart to running.
// UserPromptSubmit (userPromptSubmitted) is what enters running.

func TestSessionStartLandsOnIdleForClaude(t *testing.T) {
	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		t.Run(source, func(t *testing.T) {
			db := newHookStore(t)
			createHookSession(t, db, "claude-row", "claude", "claude-conversation")
			raw := []byte(fmt.Sprintf(`{"hook_event_name":"SessionStart","session_id":"claude-conversation","source":%q}`, source))
			result, err := Receive(context.Background(), db, raw, "", "", 10)
			if err != nil {
				t.Fatal(err)
			}
			row, err := db.GetSession(context.Background(), "claude-row")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "idle" || row.Status != "idle" || row.StatusSource != "hook" || row.StatusReason != source {
				t.Fatalf("Claude SessionStart(%s): result status %q, row %q/%q/%q; want idle from a hook", source, result.Status, row.Status, row.StatusSource, row.StatusReason)
			}
			// The prompt that follows is what enters running.
			prompt := []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"claude-conversation"}`)
			if _, err := Receive(context.Background(), db, prompt, "", "", 11); err != nil {
				t.Fatal(err)
			}
			if row, _ = db.GetSession(context.Background(), "claude-row"); row.Status != "running" {
				t.Fatalf("UserPromptSubmit after SessionStart left %q, want running", row.Status)
			}
		})
	}
}

func TestSessionStartLandsOnIdleForCodex(t *testing.T) {
	for _, source := range []string{"startup", "resume"} {
		t.Run(source, func(t *testing.T) {
			db := newHookStore(t)
			var p struct {
				ConversationID string `json:"session_id"`
			}
			if err := json.Unmarshal([]byte(codexSessionStartPayload), &p); err != nil {
				t.Fatal(err)
			}
			createHookSession(t, db, "codex-row", "codex", p.ConversationID)
			var fields map[string]any
			if err := json.Unmarshal([]byte(codexSessionStartPayload), &fields); err != nil {
				t.Fatal(err)
			}
			fields["source"] = source
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Receive(context.Background(), db, raw, "", "", 10)
			if err != nil {
				t.Fatal(err)
			}
			row, err := db.GetSession(context.Background(), "codex-row")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "idle" || row.Status != "idle" || row.StatusSource != "hook" || row.StatusReason != source {
				t.Fatalf("Codex SessionStart(%s): result status %q, row %q/%q/%q; want idle from a hook", source, result.Status, row.Status, row.StatusSource, row.StatusReason)
			}
		})
	}
}

func TestSessionStartLandsOnIdleForCopilot(t *testing.T) {
	for _, source := range []string{"new", "resume"} {
		t.Run(source, func(t *testing.T) {
			db := newHookStore(t)
			copilotRow(t, db, "starting")
			result := receiveCopilot(t, db, "sessionStart", copilotPayload(fmt.Sprintf(`,"source":%q`, source)), "")
			row, err := db.GetSession(context.Background(), "row-1")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "idle" || row.Status != "idle" || row.StatusSource != "hook" || row.StatusReason != source {
				t.Fatalf("Copilot sessionStart(%s): result status %q, row %q/%q/%q; want idle from a hook", source, result.Status, row.Status, row.StatusSource, row.StatusReason)
			}
			receiveCopilot(t, db, "userPromptSubmitted", copilotPayload(`,"prompt":"hi"`), "")
			if row, _ = db.GetSession(context.Background(), "row-1"); row.Status != "running" {
				t.Fatalf("userPromptSubmitted after sessionStart left %q, want running", row.Status)
			}
		})
	}
}

// Pi is not one of the three agents R307 names: its own session_start
// extension event keeps running (SPEC §8.3), although it shares the receiver's
// SessionStart name.
func TestPiSessionStartStaysRunning(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "pi-row", "pi", "pi-conversation")
	raw := []byte(`{"hook_event_name":"SessionStart","session_id":"pi-conversation","source":"startup"}`)
	result, err := Receive(context.Background(), db, raw, "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	row, err := db.GetSession(context.Background(), "pi-row")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "running" || row.Status != "running" {
		t.Fatalf("Pi session_start: result %q, row %q; want running", result.Status, row.Status)
	}
}

// A SessionStart from a replaced pane must not revive a process-crash row any
// more now that it lands on idle: the crash verdict (an error carrying a pane
// exit status) is terminal for it, as it was when SessionStart meant running.
func TestSessionStartDoesNotReviveAProcessCrashRow(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "crashed-row", "claude", "crashed-conversation")
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'error', status_source = 'tmux', status_at = 5, pane_exit_status = 137 WHERE id = 'crashed-row'`); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"hook_event_name":"SessionStart","session_id":"crashed-conversation","source":"resume"}`)
	if _, err := Receive(context.Background(), db, raw, "", "", 10); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetSession(context.Background(), "crashed-row")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "error" || row.StatusSource != "tmux" || row.StatusAt != 5 {
		t.Fatalf("a SessionStart revived a process-crash row: %q/%q at %d", row.Status, row.StatusSource, row.StatusAt)
	}
}
