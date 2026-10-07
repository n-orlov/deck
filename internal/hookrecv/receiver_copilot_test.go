package hookrecv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

const copilotConversationID = "7c1f2a9e-3b4d-4e5f-8a6b-0c9d8e7f6a5b"

// copilotRow creates a copilot row already in `current`, sourced from a hook.
func copilotRow(t *testing.T, db *store.Store, current string) {
	t.Helper()
	createHookSession(t, db, "row-1", "copilot", copilotConversationID)
	if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_source = 'hook', status_at = 2 WHERE id = 'row-1'`, current); err != nil {
		t.Fatal(err)
	}
}

func copilotPayload(extra string) []byte {
	return []byte(fmt.Sprintf(`{"sessionId":%q,"timestamp":1,"cwd":"/work"%s}`, copilotConversationID, extra))
}

func receiveCopilot(t *testing.T, db *store.Store, event string, raw []byte, root string) Result {
	t.Helper()
	result, err := ReceiveFrom(context.Background(), db, raw, Origin{SessionID: "row-1", Event: event, CopilotRoot: root}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func eventCount(t *testing.T, db *store.Store, where string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = 'row-1' AND ` + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestCopilotHookTable is one case per R218 table row. The rows that change
// nothing assert the row is exactly as it was and that the hook left its event.
func TestCopilotHookTable(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		extra      string
		current    string
		wantStatus string
		wantReason string
		wantKind   string
	}{
		{name: "userPromptSubmitted", event: "userPromptSubmitted", extra: `,"prompt":"hi"`, current: "idle", wantStatus: "running", wantReason: "prompt", wantKind: "user_prompt_submitted"},
		{name: "sessionStart new", event: "sessionStart", extra: `,"source":"new"`, current: "starting", wantStatus: "running", wantReason: "new", wantKind: "session_start"},
		{name: "sessionStart resume", event: "sessionStart", extra: `,"source":"resume"`, current: "starting", wantStatus: "running", wantReason: "resume", wantKind: "session_start"},
		{name: "notification permission_prompt", event: "notification", extra: `,"notification_type":"permission_prompt"`, current: "running", wantStatus: "waiting", wantReason: "permission_prompt", wantKind: "notification"},
		{name: "notification elicitation_dialog", event: "notification", extra: `,"notification_type":"elicitation_dialog"`, current: "running", wantStatus: "waiting", wantReason: "elicitation_dialog", wantKind: "notification"},
		{name: "notification other type", event: "notification", extra: `,"notification_type":"idle_prompt"`, current: "running", wantStatus: "running", wantKind: "notification"},
		{name: "notification no type", event: "notification", current: "idle", wantStatus: "idle", wantKind: "notification"},
		{name: "agentStop end_turn", event: "agentStop", extra: `,"stopReason":"end_turn"`, current: "running", wantStatus: "idle", wantReason: "end_turn", wantKind: "stop"},
		{name: "agentStop stopReason", event: "agentStop", extra: `,"stopReason":"max_tokens"`, current: "running", wantStatus: "idle", wantReason: "max_tokens", wantKind: "stop"},
		{name: "agentStop no stopReason", event: "agentStop", current: "running", wantStatus: "idle", wantReason: "end_turn", wantKind: "stop"},
		{name: "errorOccurred", event: "errorOccurred", extra: `,"errorContext":"model_call","recoverable":true`, current: "running", wantStatus: "running", wantKind: "error_occurred"},
		{name: "sessionEnd", event: "sessionEnd", extra: `,"reason":"user_exit"`, current: "running", wantStatus: "stopped", wantReason: "user_exit", wantKind: "session_end"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			copilotRow(t, db, tc.current)
			before, err := db.GetSession(context.Background(), "row-1")
			if err != nil {
				t.Fatal(err)
			}
			raw := copilotPayload(tc.extra)
			result := receiveCopilot(t, db, tc.event, raw, "")
			if result.SessionID != "row-1" || result.Kind != tc.wantKind {
				t.Fatalf("result = %#v", result)
			}
			row, err := db.GetSession(context.Background(), "row-1")
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != tc.wantStatus || row.StatusReason != tc.wantReason {
				t.Fatalf("row status/reason = %q/%q, want %q/%q", row.Status, row.StatusReason, tc.wantStatus, tc.wantReason)
			}
			if tc.wantStatus == tc.current && row.StatusReason != before.StatusReason {
				t.Fatalf("a no-change event touched the row: before %#v after %#v", before, row)
			}
			if row.ConversationID != copilotConversationID {
				t.Fatalf("conversation id moved to %q", row.ConversationID)
			}
			var kind, payload string
			if err := db.DB().QueryRow(`SELECT kind, payload FROM events WHERE session_id = 'row-1' ORDER BY seq DESC LIMIT 1`).Scan(&kind, &payload); err != nil {
				t.Fatal(err)
			}
			if kind != tc.wantKind || payload != string(raw) {
				t.Fatalf("event = %q %q", kind, payload)
			}
		})
	}
}

// A no-change event leaves the row byte for byte as it was, not merely at the
// same status.
func TestCopilotNoChangeEventsLeaveTheRowUntouched(t *testing.T) {
	for _, tc := range []struct{ event, extra string }{
		{"errorOccurred", `,"recoverable":true`},
		{"notification", `,"notification_type":"idle_prompt"`},
	} {
		t.Run(tc.event, func(t *testing.T) {
			db := newHookStore(t)
			copilotRow(t, db, "running")
			before, _ := db.GetSession(context.Background(), "row-1")
			receiveCopilot(t, db, tc.event, copilotPayload(tc.extra), "")
			after, _ := db.GetSession(context.Background(), "row-1")
			if fmt.Sprintf("%#v", before) != fmt.Sprintf("%#v", after) {
				t.Fatalf("row changed:\nbefore %#v\nafter  %#v", before, after)
			}
		})
	}
}

func TestCopilotUnknownEventIsUnsupported(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "running")
	if _, err := ReceiveFrom(context.Background(), db, copilotPayload(""), Origin{SessionID: "row-1", Event: "preToolUse"}, 100); err == nil {
		t.Fatal("an event outside the six was accepted")
	}
}

func TestCopilotMismatchedSessionIDIsNotApplied(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "running")
	raw := []byte(`{"sessionId":"11111111-2222-4333-8444-555555555555","timestamp":1,"reason":"user_exit"}`)
	// The injected deck row id resolves the row; the sessionId names another conversation.
	result := receiveCopilot(t, db, "sessionEnd", raw, "")
	if result.SessionID != "row-1" || result.Status != "" {
		t.Fatalf("result = %#v", result)
	}
	row, err := db.GetSession(context.Background(), "row-1")
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != copilotConversationID || row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("row = %#v", row)
	}
	if n := eventCount(t, db, "1 = 1"); n != 1 {
		t.Fatalf("event-log entries = %d, want exactly 1", n)
	}
	if n := eventCount(t, db, "kind = 'session_end.identity_mismatch' AND payload = '"+string(raw)+"'"); n != 1 {
		t.Fatalf("the one entry is not the identity-mismatch record")
	}
}

func TestCopilotMismatchedSessionStartDoesNotMoveTheRow(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "idle")
	raw := []byte(`{"sessionId":"11111111-2222-4333-8444-555555555555","source":"new"}`)
	receiveCopilot(t, db, "sessionStart", raw, "")
	row, _ := db.GetSession(context.Background(), "row-1")
	if row.ConversationID != copilotConversationID || row.Status != "idle" {
		t.Fatalf("a swapped conversation moved the row: %#v", row)
	}
}

func TestCopilotPayloadWithoutSessionIDUsesTheEnvRowIdentity(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "idle")
	result := receiveCopilot(t, db, "userPromptSubmitted", []byte(`{"prompt":"hi"}`), "")
	if result.SessionID != "row-1" || result.Status != "running" {
		t.Fatalf("result = %#v", result)
	}
	row, _ := db.GetSession(context.Background(), "row-1")
	if row.Status != "running" || row.StatusReason != "prompt" || row.ConversationID != copilotConversationID {
		t.Fatalf("row = %#v", row)
	}
}

func TestCopilotUnresolvedPayloadIsAnOrphan(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "idle")
	_, err := ReceiveFrom(context.Background(), db, []byte(`{"prompt":"hi"}`), Origin{Event: "userPromptSubmitted"}, 100)
	if err == nil {
		t.Fatal("no row identity and no sessionId resolved")
	}
}

func TestCopilotSupersededLaunchIsNotApplied(t *testing.T) {
	db := newHookStore(t)
	copilotRow(t, db, "running")
	if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = '4242@boot#gen-2', launch_lease_until = 0 WHERE id = 'row-1'`); err != nil {
		t.Fatal(err)
	}
	result, err := ReceiveFrom(context.Background(), db, copilotPayload(`,"reason":"user_exit"`),
		Origin{SessionID: "row-1", LaunchGeneration: "gen-1", Event: "sessionEnd"}, 100)
	if err != nil || !result.Superseded {
		t.Fatalf("result = %#v err = %v", result, err)
	}
	row, _ := db.GetSession(context.Background(), "row-1")
	if row.Status != "running" {
		t.Fatalf("a superseded copilot hook changed the row: %#v", row)
	}
}

// R218: an agentStop's transcriptPath is the row's transcript only when it is a
// regular file under the resolved copilot root.
func TestCopilotAgentStopTranscriptPath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	inside := filepath.Join(root, "session-state", copilotConversationID, "events.jsonl")
	mustWrite := func(path string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	mustWrite(inside)
	outsideFile := mustWrite(filepath.Join(outside, "events.jsonl"))
	directory := filepath.Join(root, "session-state", "a-directory")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(root, "escape.jsonl")
	if err := os.Symlink(outsideFile, escape); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path string
		want       bool
	}{
		{"regular file under the root", inside, true},
		{"outside the root", outsideFile, false},
		{"symlink out of the root", escape, false},
		{"dot-dot out of the root", filepath.Join(root, "..", filepath.Base(outside), "events.jsonl"), false},
		{"a directory", directory, false},
		{"missing file", filepath.Join(root, "session-state", "missing.jsonl"), false},
		{"relative path", "events.jsonl", false},
		{"no path", "", false},
		{"the root itself", root, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			copilotRow(t, db, "running")
			raw := copilotPayload(fmt.Sprintf(`,"transcriptPath":%q,"stopReason":"end_turn"`, tc.path))
			result := receiveCopilot(t, db, "agentStop", raw, root)
			if result.Status != "idle" {
				t.Fatalf("agentStop did not apply: %#v", result)
			}
			var recorded string
			err := db.DB().QueryRow(`SELECT payload FROM events WHERE session_id = 'row-1' AND kind = 'transcript'`).Scan(&recorded)
			switch {
			case tc.want && (err != nil || recorded != tc.path):
				t.Fatalf("transcript not recorded: %q %v", recorded, err)
			case !tc.want && err == nil:
				t.Fatalf("transcript %q recorded", recorded)
			}
		})
	}
	t.Run("no resolvable root", func(t *testing.T) {
		db := newHookStore(t)
		copilotRow(t, db, "running")
		receiveCopilot(t, db, "agentStop", copilotPayload(fmt.Sprintf(`,"transcriptPath":%q`, inside)), "")
		if n := eventCount(t, db, "kind = 'transcript'"); n != 0 {
			t.Fatal("recorded with no root")
		}
	})
	t.Run("a non-agentStop event never records", func(t *testing.T) {
		db := newHookStore(t)
		copilotRow(t, db, "running")
		receiveCopilot(t, db, "userPromptSubmitted", copilotPayload(fmt.Sprintf(`,"transcriptPath":%q`, inside)), root)
		if n := eventCount(t, db, "kind = 'transcript'"); n != 0 {
			t.Fatal("recorded on the wrong event")
		}
	})
}

func TestCopilotRootResolution(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	noHome := func() (string, error) { return "", fmt.Errorf("no home") }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := CopilotRoot(env(map[string]string{"COPILOT_HOME": "/c"}), home); got != "/c" {
		t.Fatalf("COPILOT_HOME root = %q", got)
	}
	if got := CopilotRoot(env(nil), home); got != "/home/u/.copilot" {
		t.Fatalf("default root = %q", got)
	}
	if got := CopilotRoot(env(nil), noHome); got != "" {
		t.Fatalf("unknown root = %q", got)
	}
}

// The other kinds' payloads still ignore an environment-supplied event: with no
// DECK_HOOK_EVENT the payload's own hook_event_name decides, as before.
func TestReceiveFromWithoutAnEnvEventIsReceive(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "row-1", "claude", "conv-1")
	raw := []byte(`{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"idle_prompt"}`)
	result, err := ReceiveFrom(context.Background(), db, raw, Origin{SessionID: "row-1"}, 100)
	if err != nil || result.Status != "waiting" || result.Reason != "idle_prompt" {
		t.Fatalf("result = %#v err = %v", result, err)
	}
}
