package hookrecv

import (
	"context"
	"fmt"
	"testing"
)

// TestSupersededLaunchGenerationDropsTheWholeHookClass is R74 leg 2 (issue
// #11). A hook whose DECK_LAUNCH_GENERATION is not the row's current one comes
// from a pane deck has already replaced, so its verdict is about a process that
// is gone: it must never reach the row. The point of the matrix is that this
// holds for the whole class, not just for the SessionEnd->stopped case that
// wrecked a live row in the field -- Stop->idle and Notification->waiting are
// equally false statements about the same dead pane.
func TestSupersededLaunchGenerationDropsTheWholeHookClass(t *testing.T) {
	events := []struct {
		event      string
		extra      string
		wantStatus string
	}{
		{event: "SessionEnd", extra: `,"reason":"other"`, wantStatus: "stopped"},
		{event: "Stop", extra: `,"last_assistant_message":"from the dead pane"`, wantStatus: "idle"},
		{event: "Notification", extra: `,"notification_type":"question"`, wantStatus: "waiting"},
		{event: "SessionStart", extra: `,"source":"resume"`, wantStatus: "running"},
		{event: "UserPromptSubmit", wantStatus: "running"},
		{event: "StopFailure", extra: `,"error_type":"api_error"`, wantStatus: "error"},
	}
	tokens := []struct {
		name      string
		rowToken  string
		hookToken string
		wantApply bool
	}{
		{name: "superseded token", rowToken: "gen-current", hookToken: "gen-killed", wantApply: false},
		{name: "current token", rowToken: "gen-current", hookToken: "gen-current", wantApply: true},
		// Deliberate: the row names a leased launch, and such a launch always
		// exports its token, so a tokenless hook is from the row's older,
		// pre-lease pane.
		{name: "hook carries no token", rowToken: "gen-current", hookToken: "", wantApply: false},
		// Deliberate: nothing to discriminate, so pre-R74 behaviour stands.
		{name: "row carries no token", rowToken: "", hookToken: "", wantApply: true},
		{name: "row carries no token, hook does", rowToken: "", hookToken: "gen-orphan", wantApply: true},
	}
	for _, ev := range events {
		for _, tk := range tokens {
			t.Run(ev.event+"/"+tk.name, func(t *testing.T) {
				db := newHookStore(t)
				id := ev.event + "-" + tk.name
				conversationID := "conversation-" + id
				createHookSession(t, db, id, "claude", conversationID)
				// The live pane's launch is running: this is the row a late
				// hook from the replaced pane must not be allowed to move.
				if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_reason = 'before', status_source = 'hook', status_at = 7, last_message = 'before message' WHERE id = ?`, id); err != nil {
					t.Fatal(err)
				}
				owner := "4242@boot"
				if tk.rowToken != "" {
					owner += "#" + tk.rowToken
				}
				if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = ?, launch_lease_until = 0 WHERE id = ?`, owner, id); err != nil {
					t.Fatal(err)
				}

				raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q%s}`, ev.event, conversationID, ev.extra))
				result, err := Receive(context.Background(), db, raw, id, tk.hookToken, 20)
				if err != nil {
					t.Fatalf("receive: %v", err)
				}
				row, err := db.GetSession(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				if tk.wantApply {
					if row.Status != ev.wantStatus || row.StatusSource != "hook" || row.StatusAt != 20 {
						t.Fatalf("hook carrying the current token did not apply: %#v", row)
					}
				} else if row.Status != "running" || row.StatusReason != "before" || row.StatusAt != 7 || row.LastMessage != "before message" {
					t.Fatalf("hook from a superseded launch reached the row: %#v", row)
				}
				if result.Superseded == tk.wantApply {
					t.Fatalf("result.Superseded = %v with row token %q and hook token %q", result.Superseded, tk.rowToken, tk.hookToken)
				}
				// Dropped is not the same as unheard: the payload is still on
				// the record either way, which is what makes a stale pane's
				// hook diagnosable after the fact.
				var events int
				if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND payload = ?`, id, string(raw)).Scan(&events); err != nil || events != 1 {
					t.Fatalf("recorded events = %d, %v; want 1", events, err)
				}
			})
		}
	}
}

// TestSupersededSessionStartDoesNotMoveTheConversationID keeps requirement
// 44's identity move on the same side of the R74 rule as the status write: a
// replaced pane's SessionStart names the conversation that pane was running,
// and adopting it would point the row at a conversation that is over.
func TestSupersededSessionStartDoesNotMoveTheConversationID(t *testing.T) {
	db := newHookStore(t)
	const id = "identity-row"
	createHookSession(t, db, id, "claude", "current-conversation")
	if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = '4242@boot#gen-current', status = 'running' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"hook_event_name":"SessionStart","session_id":"replaced-conversation","source":"resume"}`)
	if _, err := Receive(context.Background(), db, raw, id, "gen-killed", 30); err != nil {
		t.Fatalf("receive: %v", err)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != "current-conversation" {
		t.Fatalf("superseded SessionStart moved the row's conversation id to %q", row.ConversationID)
	}
	if row.Status != "running" {
		t.Fatalf("superseded SessionStart moved the row's status to %q", row.Status)
	}
}
