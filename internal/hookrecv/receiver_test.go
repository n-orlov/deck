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

// TestReceiveHookAppliesOverAnyStaleSourceExceptStopped proves requirement 45:
// there is no per-event AllowedFrom list left to defeat §7's
// `user-terminal > hook > probe > tmux` precedence. A hook (the second-highest
// precedence source) lands regardless of the current status VALUE and
// regardless of which lower-or-equal-precedence source (tmux, probe, or a
// plain prior "user" write) produced it -- precedence is decided by source,
// not by an enumerated predecessor status. The one status value a hook can
// never move a row away from is "stopped": §7's transition table gives it no
// return edge except the explicit `r` resume, which does not go through
// Receive.
func TestReceiveHookAppliesOverAnyStaleSourceExceptStopped(t *testing.T) {
	events := []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "StopFailure", "SessionEnd"}
	statuses := []string{"starting", "running", "waiting", "idle", "error", "stopped"}
	sources := []string{"user", "tmux", "probe", "hook"}
	for _, event := range events {
		mapping := Mappings[event]
		for _, current := range statuses {
			for _, currentSource := range sources {
				t.Run(event+"/from_"+current+"/via_"+currentSource, func(t *testing.T) {
					db := newHookStore(t)
					id := event + "-" + current + "-" + currentSource
					createHookSession(t, db, id, "claude", id)
					if _, err := db.DB().Exec(`UPDATE sessions SET status = ?, status_reason = 'before', status_source = ?, status_at = 7, notify_epoch = 3, acknowledged = 1, last_message = 'before message' WHERE id = ?`, current, currentSource, id); err != nil {
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
					if current == "stopped" {
						if got.Status != current || got.StatusReason != "before" || got.StatusSource != currentSource || got.StatusAt != 7 || got.NotifyEpoch != 3 || !got.Acknowledged || got.LastMessage != "before message" {
							t.Fatalf("hook resurrected a stopped row: %#v", got)
						}
					} else if got.Status != mapping.Status || got.StatusSource != "hook" || got.StatusAt != 20 {
						t.Fatalf("hook did not apply over stale %s verdict: %#v", currentSource, got)
					}
					var events int
					if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ? AND payload = ?`, id, mapping.Kind, string(raw)).Scan(&events); err != nil || events != 1 {
						t.Fatalf("event audit count = %d, %v; want 1", events, err)
					}
				})
			}
		}
	}
}

// TestReceiveHookOverridesStaleTmuxLaunchFailure is the concrete scenario
// requirement 45 names: a row parked at `error` by a tmux-sourced launch
// failure (task 004) is not stuck there once the agent's own hooks start
// arriving -- the hook outranks the stale tmux verdict, regardless of `error`
// not being that event's predecessor in some enumerated list.
func TestReceiveHookOverridesStaleTmuxLaunchFailure(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		extra      string
		wantStatus string
	}{
		{name: "error to idle via Stop", event: "Stop", extra: `,"last_assistant_message":"done"`, wantStatus: "idle"},
		{name: "error to waiting via Notification", event: "Notification", extra: `,"notification_type":"permission_prompt"`, wantStatus: "waiting"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			id := "launch-failed-" + tc.event
			createHookSession(t, db, id, "claude", id)
			if _, err := db.DB().Exec(`UPDATE sessions SET status = 'error', status_reason = 'duplicate session: deck_x', status_source = 'tmux', status_at = 5 WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q%s}`, tc.event, id, tc.extra))
			if _, err := Receive(context.Background(), db, raw, "", 50); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetSession(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus || got.StatusSource != "hook" || got.StatusAt != 50 {
				t.Fatalf("status = %#v, want %s from hook at 50", got, tc.wantStatus)
			}
		})
	}
}

// TestReceiveDoesNotResurrectAUserKilledRow proves the other half of
// requirement 45: killed_by_user (§7's `user-terminal`) outranks every hook,
// including one that arrives milliseconds later.
func TestReceiveDoesNotResurrectAUserKilledRow(t *testing.T) {
	db := newHookStore(t)
	id := "user-killed"
	createHookSession(t, db, id, "claude", id)
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: id, Status: "stopped", Source: "user", At: 10, KilledByUser: true,
	}); err != nil {
		t.Fatal(err)
	}
	raw := []byte(fmt.Sprintf(`{"hook_event_name":"SessionStart","session_id":%q,"source":"resume"}`, id))
	if _, err := Receive(context.Background(), db, raw, "", 20); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || got.StatusSource != "user" || got.StatusAt != 10 {
		t.Fatalf("a late hook resurrected a user-killed row: %#v", got)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'session_start'`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("hook event audit count = %d, %v; want 1", events, err)
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

// TestReceiveSessionStartFollowsLiveConversation covers requirement 44: an
// in-session resume (task 001) leaves a row "running" under its OLD
// conversation id; the immediately-following SessionStart for the NEW
// conversation must be resolved via the injected row identity (the new id
// matches no row's stored conversation_id) and must move the row's
// conversation_id to the new value, durably -- proven by reopening the state
// database from its on-disk path rather than trusting the in-memory handle
// the write went through.
func TestReceiveSessionStartFollowsLiveConversation(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")
	db, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	createHookSession(t, db, "resumed-row", "claude", "old-conversation")
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 5 WHERE id = ?`, "resumed-row"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	raw := []byte(`{"hook_event_name":"SessionStart","session_id":"new-conversation","source":"resume"}`)
	result, err := Receive(ctx, db, raw, "resumed-row", 30)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "resumed-row" {
		t.Fatalf("resolved session = %q, want resumed-row", result.SessionID)
	}
	_ = db.Close()

	// Reopen against the same on-disk path: an in-memory-only write would not
	// survive this round trip.
	reopened, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	row, err := reopened.GetSession(ctx, "resumed-row")
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != "new-conversation" {
		t.Fatalf("conversation_id = %q, want new-conversation", row.ConversationID)
	}
	// The status transition itself (SessionStart's AllowedFrom is "starting"
	// only) is correctly a no-op from "running" -- the row was never stopped.
	if row.Status != "running" {
		t.Fatalf("status = %q, want running (unaffected)", row.Status)
	}
	var kind, reason, payload string
	if err := reopened.DB().QueryRow(`SELECT kind, reason, payload FROM events WHERE session_id = ? AND kind = 'set_conversation_id' ORDER BY at DESC LIMIT 1`, "resumed-row").Scan(&kind, &reason, &payload); err != nil {
		t.Fatal(err)
	}
	if reason != "hook" || payload != "new-conversation" {
		t.Fatalf("conversation change event = kind:%q reason:%q payload:%q", kind, reason, payload)
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
