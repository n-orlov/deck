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
		wantStatus  string
		wantReason  string
		wantMessage string
	}{
		{name: "session start fresh", event: "SessionStart", extra: `,"source":"startup"`, wantStatus: "running", wantReason: "startup"},
		{name: "session start resumed", event: "SessionStart", extra: `,"source":"resume"`, wantStatus: "running", wantReason: "resume"},
		{name: "session start compacted", event: "SessionStart", extra: `,"source":"compact"`, wantStatus: "running", wantReason: "compact"},
		{name: "user prompt", event: "UserPromptSubmit", wantStatus: "running"},
		{name: "permission notification", event: "Notification", extra: `,"notification_type":"permission_prompt"`, wantStatus: "waiting", wantReason: "permission_prompt"},
		{name: "question notification", event: "Notification", extra: `,"notification_type":"question"`, wantStatus: "waiting", wantReason: "question"},
		{name: "needs input notification", event: "Notification", extra: `,"notification_type":"needs_input"`, wantStatus: "waiting", wantReason: "needs_input"},
		{name: "idle notification", event: "Notification", extra: `,"notification_type":"idle_prompt"`, wantStatus: "waiting", wantReason: "idle_prompt"},
		{name: "stop", event: "Stop", extra: `,"last_assistant_message":"done ✓"`, wantStatus: "idle", wantMessage: "done ✓"},
		{name: "API failure", event: "StopFailure", extra: `,"error_type":"api_error"`, wantStatus: "error", wantReason: "api_error"},
		{name: "turn failure", event: "StopFailure", extra: `,"error_type":"turn_error"`, wantStatus: "error", wantReason: "turn_error"},
		{name: "session end", event: "SessionEnd", extra: `,"reason":"logout"`, wantStatus: "stopped", wantReason: "logout"},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			id := fmt.Sprintf("row-%d", index)
			conversationID := fmt.Sprintf("conversation-%d", index)
			createHookSession(t, db, id, "claude", conversationID)
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
