package hookrecv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

func TestReceiveMappingTable(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		extra       string
		current     string
		wantStatus  string
		wantReason  string
		wantMessage string
	}{
		{name: "session start fresh", event: "SessionStart", extra: `,"source":"startup"`, current: "starting", wantStatus: "running", wantReason: "startup"},
		{name: "session start resumed", event: "SessionStart", extra: `,"source":"resume"`, current: "starting", wantStatus: "running", wantReason: "resume"},
		{name: "session start compacted", event: "SessionStart", extra: `,"source":"compact"`, current: "starting", wantStatus: "running", wantReason: "compact"},
		{name: "user prompt", event: "UserPromptSubmit", current: "idle", wantStatus: "running"},
		{name: "permission notification", event: "Notification", extra: `,"notification_type":"permission_prompt"`, current: "running", wantStatus: "waiting", wantReason: "permission_prompt"},
		{name: "question notification", event: "Notification", extra: `,"notification_type":"question"`, current: "running", wantStatus: "waiting", wantReason: "question"},
		{name: "needs input notification", event: "Notification", extra: `,"notification_type":"needs_input"`, current: "running", wantStatus: "waiting", wantReason: "needs_input"},
		{name: "idle notification", event: "Notification", extra: `,"notification_type":"idle_prompt"`, current: "running", wantStatus: "waiting", wantReason: "idle_prompt"},
		{name: "stop", event: "Stop", extra: `,"last_assistant_message":"done ✓"`, current: "running", wantStatus: "idle", wantMessage: "done ✓"},
		{name: "API failure", event: "StopFailure", extra: `,"error_type":"api_error"`, current: "running", wantStatus: "error", wantReason: "api_error"},
		{name: "turn failure", event: "StopFailure", extra: `,"error_type":"turn_error"`, current: "running", wantStatus: "error", wantReason: "turn_error"},
		{name: "session end", event: "SessionEnd", extra: `,"reason":"logout"`, current: "waiting", wantStatus: "stopped", wantReason: "logout"},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			id := fmt.Sprintf("row-%d", index)
			conversationID := fmt.Sprintf("conversation-%d", index)
			createHookSession(t, db, id, "claude", conversationID)
			if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_source = 'hook', status_at = 2 WHERE id = ?`, tc.current, id); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q%s}`, tc.event, conversationID, tc.extra))

			result, err := Receive(context.Background(), db, raw, "wrong-fallback", int64(100+index))
			if err != nil {
				t.Fatal(err)
			}
			if result.SessionID != id || result.Status != tc.wantStatus || result.Reason != tc.wantReason {
				t.Fatalf("result = %#v", result)
			}
			row, err := db.GetSession(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != tc.wantStatus || row.StatusReason != tc.wantReason || row.StatusSource != "hook" || row.LastMessage != tc.wantMessage {
				t.Fatalf("persisted row = %#v", row)
			}
			var kind, reason, payload string
			if err := db.DB().QueryRow(`SELECT kind, reason, payload FROM events WHERE session_id = ? ORDER BY at DESC LIMIT 1`, id).Scan(&kind, &reason, &payload); err != nil {
				t.Fatal(err)
			}
			if kind != Mappings[tc.event].Kind || reason != tc.wantReason || payload != string(raw) {
				t.Fatalf("event = kind:%q reason:%q payload:%q", kind, reason, payload)
			}
		})
	}
}

func TestReceiveEnforcesEveryHookTransitionAndStillAuditsRejectedEvents(t *testing.T) {
	statuses := []string{"starting", "running", "waiting", "idle", "error", "stopped"}
	expected := map[string][]string{
		"SessionStart":     {"starting"},
		"UserPromptSubmit": {"idle", "error"},
		"Notification":     {"running"},
		"Stop":             {"running"},
		"StopFailure":      {"running"},
		"SessionEnd":       {"starting", "running", "waiting", "idle", "error", "stopped"},
	}
	for event, allowed := range expected {
		mapping, ok := Mappings[event]
		if !ok || fmt.Sprint(mapping.AllowedFrom) != fmt.Sprint(allowed) {
			t.Fatalf("%s AllowedFrom = %v; want %v", event, mapping.AllowedFrom, allowed)
		}
		for _, current := range statuses {
			t.Run(event+"/from_"+current, func(t *testing.T) {
				db := newHookStore(t)
				id := event + "-" + current
				createHookSession(t, db, id, "claude", id)
				if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_reason = 'before', status_source = 'user', status_at = 7, notify_epoch = 3, acknowledged = 1, last_message = 'before message' WHERE id = ?`, current, id); err != nil {
					t.Fatal(err)
				}
				raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q,"source":"resume","notification_type":"question","error_type":"api_error","reason":"logout","last_assistant_message":"after message"}`, event, id))
				if _, err := Receive(context.Background(), db, raw, "", 20); err != nil {
					t.Fatal(err)
				}
				got, err := db.GetSession(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				allowedTransition := stringIn(allowed, current)
				if allowedTransition {
					if got.Status != mapping.Status || got.StatusSource != "hook" || got.StatusAt != 20 {
						t.Fatalf("allowed transition persisted %#v", got)
					}
				} else if got.Status != current || got.StatusReason != "before" || got.StatusSource != "user" || got.StatusAt != 7 || got.NotifyEpoch != 3 || !got.Acknowledged || got.LastMessage != "before message" {
					t.Fatalf("rejected transition changed metadata: %#v", got)
				}
				var events int
				if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ? AND payload = ?`, id, mapping.Kind, string(raw)).Scan(&events); err != nil || events != 1 {
					t.Fatalf("event audit count = %d, %v; want 1", events, err)
				}
			})
		}
	}
}

// TestReceiveSessionEndReasonTaxonomy proves requirement 43: an in-session
// SessionEnd (Claude's own /resume or /clear, same pane, same tmux session,
// new conversation) is recorded as an event but does not stop a live row,
// while a real end reason still does.
func TestReceiveSessionEndReasonTaxonomy(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		current    string
		wantStatus string // status left on the row after the hook
	}{
		{name: "resume is not an end", reason: "resume", current: "running", wantStatus: "running"},
		{name: "clear is not an end", reason: "clear", current: "waiting", wantStatus: "waiting"},
		{name: "logout is a real end", reason: "logout", current: "running", wantStatus: "stopped"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			id := "row-" + tc.name
			createHookSession(t, db, id, "claude", id)
			if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_source = 'hook', status_at = 5 WHERE id = ?`, tc.current, id); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"hook_event_name":"SessionEnd","session_id":%q,"reason":%q}`, id, tc.reason))

			if _, err := Receive(context.Background(), db, raw, "", 200); err != nil {
				t.Fatal(err)
			}

			row, err := db.GetSession(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", row.Status, tc.wantStatus)
			}

			// The event is recorded either way -- this is the audit trail that
			// distinguishes "deck decided this wasn't an end" from "deck never
			// heard about it".
			var kind, reason string
			if err := db.DB().QueryRow(`SELECT kind, reason FROM events WHERE session_id = ? ORDER BY at DESC LIMIT 1`, id).Scan(&kind, &reason); err != nil {
				t.Fatal(err)
			}
			if kind != "session_end" || reason != tc.reason {
				t.Fatalf("event = kind:%q reason:%q", kind, reason)
			}
		})
	}
}

func TestReceiveDoesNotReviveCleanStopOrProcessCrash(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		event      string
		paneStatus any
	}{
		{name: "clean stopped row", status: "stopped", event: "SessionStart"},
		{name: "process crash row", status: "error", event: "UserPromptSubmit", paneStatus: 137},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			createHookSession(t, db, tc.name, "claude", tc.name)
			if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_reason = 'terminal', status_source = 'tmux', status_at = 9, pane_exit_status = ?, crash_tail = 'fatal', notify_epoch = 4, acknowledged = 0, last_message = 'last' WHERE id = ?`, tc.status, tc.paneStatus, tc.name); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q,"source":"resume"}`, tc.event, tc.name))
			if _, err := Receive(context.Background(), db, raw, "", 30); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetSession(context.Background(), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.status || got.StatusReason != "terminal" || got.StatusSource != "tmux" || got.StatusAt != 9 || got.CrashTail != "fatal" || got.NotifyEpoch != 4 || got.Acknowledged || got.LastMessage != "last" {
				t.Fatalf("terminal metadata changed: %#v", got)
			}
			if tc.paneStatus != nil && (got.PaneExitStatus == nil || *got.PaneExitStatus != 137) {
				t.Fatalf("pane exit status changed: %#v", got.PaneExitStatus)
			}
			var events int
			if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, tc.name).Scan(&events); err != nil || events != 1 {
				t.Fatalf("rejected hook events = %d, %v; want 1", events, err)
			}
		})
	}
}

func stringIn(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestReceiveResolutionPrefersConversationThenUsesInjectedIdentity(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "conversation-row", "claude", "reported-conversation")
	createHookSession(t, db, "injected-row", "claude", "different-conversation")
	ctx := context.Background()

	if result, err := Receive(ctx, db, []byte(`{"hook_event_name":"SessionStart","session_id":"reported-conversation","source":"resume"}`), "injected-row", 20); err != nil {
		t.Fatal(err)
	} else if result.SessionID != "conversation-row" {
		t.Fatalf("conversation resolution selected %q", result.SessionID)
	}
	injected, err := db.GetSession(ctx, "injected-row")
	if err != nil {
		t.Fatal(err)
	}
	if injected.Status != "starting" {
		t.Fatalf("fallback row changed despite conversation match: %#v", injected)
	}

	if result, err := Receive(ctx, db, []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"unknown-upstream-id"}`), "injected-row", 21); err != nil {
		t.Fatal(err)
	} else if result.SessionID != "injected-row" {
		t.Fatalf("injected resolution selected %q", result.SessionID)
	}
}

func TestReceivePreservesUnresolvedPayloadAsOrphan(t *testing.T) {
	db := newHookStore(t)
	raw := []byte("{\n  \"hook_event_name\": \"Notification\", \"notification_type\": \"question\", \"session_id\": \"gone\"\n}")
	result, err := Receive(context.Background(), db, raw, "also-gone", 42)
	if !errors.Is(err, ErrUnresolved) || !result.Orphan {
		t.Fatalf("Receive error/result = %v, %#v", err, result)
	}
	var sessionID sql.NullString
	var kind, reason, payload string
	if err := db.DB().QueryRow(`SELECT session_id, kind, reason, payload FROM events`).Scan(&sessionID, &kind, &reason, &payload); err != nil {
		t.Fatal(err)
	}
	if sessionID.Valid || kind != "notification" || reason != "question" || payload != string(raw) {
		t.Fatalf("orphan = session:%#v kind:%q reason:%q payload:%q", sessionID, kind, reason, payload)
	}
}

func TestReceiveRejectsShellTarget(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "shell-row", "shell", "")
	_, err := Receive(context.Background(), db, []byte(`{"hook_event_name":"SessionStart"}`), "shell-row", 50)
	if err == nil {
		t.Fatal("shell hook unexpectedly accepted")
	}
	var count int
	if scanErr := db.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&count); scanErr != nil || count != 0 {
		t.Fatalf("shell hook events = %d, %v; want none", count, scanErr)
	}
}

func newHookStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createHookSession(t *testing.T, db *store.Store, id, agent, conversationID string) {
	t.Helper()
	_, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: id, CWD: "/work", Agent: agent, CapturedPath: "/bin",
		Status: "starting", StatusSource: "user", StatusAt: 1, CreatedAt: 1,
		ConversationID: conversationID,
	})
	if err != nil {
		t.Fatal(err)
	}
}
