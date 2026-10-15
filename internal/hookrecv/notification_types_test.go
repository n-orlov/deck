package hookrecv

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// R308: Claude's idle_prompt notification says the agent has sat idle for a
// while. It changes no status and makes no attention transition on any row.
// R309 (hook half): only permission_prompt and elicitation_dialog are waiting.

func notificationPayload(notificationType string) []byte {
	return []byte(fmt.Sprintf(`{"hook_event_name":"Notification","session_id":"conv","notification_type":%q}`, notificationType))
}

func hookFired(t *testing.T, db *store.Store, id string) string {
	t.Helper()
	var fired *string
	if err := db.DB().QueryRow(`SELECT hook_fired FROM sessions WHERE id = ?`, id).Scan(&fired); err != nil {
		t.Fatal(err)
	}
	if fired == nil {
		return ""
	}
	return *fired
}

func TestIdlePromptChangesNothingOnIdleRunningAndWaitingRows(t *testing.T) {
	prepare := map[string]struct {
		hook   string
		status string
	}{
		"idle":    {`{"hook_event_name":"Stop","session_id":"conv","last_assistant_message":"done"}`, "idle"},
		"running": {`{"hook_event_name":"UserPromptSubmit","session_id":"conv"}`, "running"},
		"waiting": {string(notificationPayload("permission_prompt")), "waiting"},
	}
	for name, tc := range prepare {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := newHookStore(t)
			createHookSession(t, db, "row", "claude", "conv")
			if _, err := Receive(ctx, db, []byte(tc.hook), "", "", 10); err != nil {
				t.Fatal(err)
			}
			before, err := db.GetSession(ctx, "row")
			if err != nil {
				t.Fatal(err)
			}
			if before.Status != tc.status {
				t.Fatalf("setup left %q, want %q", before.Status, tc.status)
			}
			firedBefore := hookFired(t, db, "row")

			result, err := Receive(ctx, db, notificationPayload("idle_prompt"), "", "", 50)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "" {
				t.Fatalf("idle_prompt result status = %q, want none", result.Status)
			}
			after, err := db.GetSession(ctx, "row")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("idle_prompt changed the row (status, status_at, source, reason, notify_epoch, acknowledged or other):\nbefore %#v\nafter  %#v", before, after)
			}
			if got := hookFired(t, db, "row"); got != firedBefore {
				t.Fatalf("idle_prompt changed hook_fired: %q -> %q", firedBefore, got)
			}
			var events int
			if err := db.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = 'row' AND kind = 'notification' AND reason = 'idle_prompt'`).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 1 {
				t.Fatalf("idle_prompt recorded %d notification events, want 1 (recorded, never applied)", events)
			}
		})
	}
}

func TestStopThenIdlePromptLeavesTheRowIdle(t *testing.T) {
	ctx := context.Background()
	db := newHookStore(t)
	createHookSession(t, db, "row", "claude", "conv")
	if _, err := Receive(ctx, db, []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"conv"}`), "", "", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := Receive(ctx, db, []byte(`{"hook_event_name":"Stop","session_id":"conv","last_assistant_message":"done"}`), "", "", 20); err != nil {
		t.Fatal(err)
	}
	if _, err := Receive(ctx, db, notificationPayload("idle_prompt"), "", "", 30); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetSession(ctx, "row")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "idle" || row.StatusAt != 20 || row.StatusReason == "idle_prompt" {
		t.Fatalf("Stop then idle_prompt left %q/%q at %d, want idle from the Stop at 20", row.Status, row.StatusReason, row.StatusAt)
	}
}

func TestOnlyPermissionPromptAndElicitationDialogAreWaiting(t *testing.T) {
	cases := []struct {
		notificationType string
		wantWaiting      bool
	}{
		{"permission_prompt", true},
		{"elicitation_dialog", true},
		{"idle_prompt", false},
		{"auth_success", false},
		{"question", false},
		{"a_type_from_a_future_claude", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run("claude/"+tc.notificationType, func(t *testing.T) {
			ctx := context.Background()
			db := newHookStore(t)
			createHookSession(t, db, "row", "claude", "conv")
			if _, err := Receive(ctx, db, []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"conv"}`), "", "", 10); err != nil {
				t.Fatal(err)
			}
			result, err := Receive(ctx, db, notificationPayload(tc.notificationType), "", "", 20)
			if err != nil {
				t.Fatal(err)
			}
			row, err := db.GetSession(ctx, "row")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantWaiting {
				if row.Status != "waiting" || result.Status != "waiting" || row.StatusReason != tc.notificationType || row.StatusSource != "hook" {
					t.Fatalf("%s: row %q/%q/%q, result %q; want waiting from a hook", tc.notificationType, row.Status, row.StatusReason, row.StatusSource, result.Status)
				}
				return
			}
			if row.Status != "running" || result.Status != "" || row.StatusAt != 10 {
				t.Fatalf("%q: row %q at %d, result %q; want running at 10 and no status", tc.notificationType, row.Status, row.StatusAt, result.Status)
			}
		})
	}
}
