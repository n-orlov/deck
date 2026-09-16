package hookrecv

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// The payload constants below are copied verbatim from
// docs/reports/codex-cli-0.154.0-spike.md, Q3c ("Observed payloads (real
// session, `-a on-request -s workspace-write`)"). The doc elides repeated
// UUID prefixes with "..." for readability; every elision below is expanded
// to a full id so the JSON parses, and every field deck actually interprets
// (hook_event_name, session_id, source, tool_name, reason,
// last_assistant_message) is untouched from the doc's own text. These
// prove requirement 123 against codex's real shapes, not a paraphrase of
// them -- and, by using Mappings/Receive with no codex-specific branch,
// that the single receiver and single mapping table already cover codex.

const codexSessionStartPayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"SessionStart","model":"fake-model","permission_mode":"default","source":"startup"}`

const codexUserPromptSubmitPayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","turn_id":"01a09616-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"UserPromptSubmit","model":"fake-model","permission_mode":"default","prompt":"please write the marker file"}`

// PermissionRequest - shell command case (SPEC/spike: Bash).
const codexPermissionRequestBashPayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","turn_id":"01a09616-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"PermissionRequest","model":"fake-model","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"touch $HOME/codex-spike-escalation-test","description":"Write a marker file outside the workspace"}}`

// PermissionRequest - file-write case (apply_patch). The doc's own session
// id for this event is elided past "01a0961b-"; the remainder is filled in
// here for JSON validity only -- the field the mapping reads, tool_name, is
// verbatim.
const codexPermissionRequestApplyPatchPayload = `{"session_id":"01a0961b-150d-7252-959c-d72a289dae41","turn_id":"01a0961b-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a0961b-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"PermissionRequest","model":"fake-model","permission_mode":"default","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: hello.txt\n@@\n+added by codex spike\n*** End Patch"}}`

const codexStopPayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","turn_id":"01a09616-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"Stop","model":"fake-model","permission_mode":"default","stop_hook_active":false,"last_assistant_message":"All done. The directory listing is above."}`

// Stop with a null last_assistant_message -- Q3c: "Verified null when the
// turn produced no assistant text".
const codexStopNullMessagePayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","turn_id":"01a09616-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"Stop","model":"fake-model","permission_mode":"default","stop_hook_active":false,"last_assistant_message":null}`

const codexSessionEndPayload = `{"session_id":"01a09616-150d-7252-959c-d72a289dae41","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"SessionEnd","reason":"other"}`

const codexConversationID = "01a09616-150d-7252-959c-d72a289dae41"

// TestReceiveCodexFiveEvents covers all five events codex actually
// instruments (SessionStart, UserPromptSubmit, PermissionRequest, Stop,
// SessionEnd -- see internal/agent's codexHookEvents) plus both
// PermissionRequest shapes the spike observed, driving Receive with the
// codex payload text above through the exact same Mappings table and
// Receive function every Claude test above uses -- no codex branch
// anywhere in the receiver.
func TestReceiveCodexFiveEvents(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus string
		wantReason string
	}{
		{name: "SessionStart", payload: codexSessionStartPayload, wantStatus: "running", wantReason: "startup"},
		{name: "UserPromptSubmit", payload: codexUserPromptSubmitPayload, wantStatus: "running", wantReason: ""},
		{name: "PermissionRequest Bash", payload: codexPermissionRequestBashPayload, wantStatus: "waiting", wantReason: "Bash"},
		{name: "PermissionRequest apply_patch", payload: codexPermissionRequestApplyPatchPayload, wantStatus: "waiting", wantReason: "apply_patch"},
		{name: "Stop", payload: codexStopPayload, wantStatus: "idle", wantReason: ""},
		{name: "SessionEnd", payload: codexSessionEndPayload, wantStatus: "stopped", wantReason: "other"},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			id := fmt.Sprintf("codex-row-%d", index)
			var p payload
			if err := json.Unmarshal([]byte(tc.payload), &p); err != nil {
				t.Fatal(err)
			}
			createHookSession(t, db, id, "codex", p.ConversationID)
			if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 2 WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}

			result, err := Receive(context.Background(), db, []byte(tc.payload), "wrong-fallback", "", int64(100+index))
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
			if row.Status != tc.wantStatus || row.StatusReason != tc.wantReason || row.StatusSource != "hook" {
				t.Fatalf("persisted row = %#v", row)
			}
		})
	}
}

// TestReceiveCodexStopWithNullLastAssistantMessage proves a null
// last_assistant_message (Q3c: observed when the turn produced no
// assistant text) decodes cleanly to an empty Go string rather than
// erroring or crashing the decode -- and, per store.UpdateSessionStatus's
// own pre-existing "empty message means not provided" rule (an empty
// string never overwrites a prior last_message), the row's previous
// message is left exactly as it was rather than being blanked.
func TestReceiveCodexStopWithNullLastAssistantMessage(t *testing.T) {
	db := newHookStore(t)
	const id = "codex-null-message"
	createHookSession(t, db, id, "codex", codexConversationID)
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 2, last_message = 'stale' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	result, err := Receive(context.Background(), db, []byte(codexStopNullMessagePayload), "wrong-fallback", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "idle" {
		t.Fatalf("result = %#v", result)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "idle" || row.LastMessage != "stale" {
		t.Fatalf("row = %#v, want idle with the prior last message left alone", row)
	}
}

// TestReceiveCodexSessionEndOtherStopsTheRow proves a codex SessionEnd
// whose reason is "other" (the only reason value Q3c ever observed --
// fired on both /quit and double Ctrl+C) is a real end: unlike Claude's
// "resume"/"clear" in-session taxonomy, codex's "other" is not carved out,
// so the row stops.
func TestReceiveCodexSessionEndOtherStopsTheRow(t *testing.T) {
	db := newHookStore(t)
	const id = "codex-session-end"
	createHookSession(t, db, id, "codex", codexConversationID)
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 2 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := Receive(context.Background(), db, []byte(codexSessionEndPayload), "wrong-fallback", "", 200); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "stopped" || row.StatusReason != "other" || row.StatusSource != "hook" {
		t.Fatalf("row = %#v, want stopped/other/hook", row)
	}
}

// TestReceiveCodexSessionStartAdoptsTheRowsConversationID proves
// requirement 44 against a codex payload: a SessionStart resolved through
// the injected row identity (its own session_id matches no row yet) moves
// that row's stored conversation id to the one codex just reported.
func TestReceiveCodexSessionStartAdoptsTheRowsConversationID(t *testing.T) {
	db := newHookStore(t)
	const id = "codex-adopt-row"
	createHookSession(t, db, id, "codex", "pre-launch-placeholder")
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'starting', status_source = 'user', status_at = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	result, err := Receive(context.Background(), db, []byte(codexSessionStartPayload), id, "", 30)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != id {
		t.Fatalf("resolved session = %q, want %q", result.SessionID, id)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != codexConversationID {
		t.Fatalf("conversation_id = %q, want %q", row.ConversationID, codexConversationID)
	}
	if row.Status != "running" || row.StatusSource != "hook" {
		t.Fatalf("row = %#v, want running/hook", row)
	}
}

// TestReceiveCodexSupersededSessionStartDoesNotMoveTheConversationID proves
// requirement 74/44's interaction against a codex payload: a SessionStart
// carrying a launch generation that is not the row's current one comes
// from a pane deck already replaced, so it must not move the row's
// conversation id onto the replaced pane's conversation.
func TestReceiveCodexSupersededSessionStartDoesNotMoveTheConversationID(t *testing.T) {
	db := newHookStore(t)
	const id = "codex-superseded-row"
	createHookSession(t, db, id, "codex", "current-codex-conversation")
	if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = '4242@boot#gen-current', status = 'running', status_source = 'hook' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	result, err := Receive(context.Background(), db, []byte(codexSessionStartPayload), id, "gen-killed", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Superseded {
		t.Fatalf("result.Superseded = false, want true: %#v", result)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != "current-codex-conversation" {
		t.Fatalf("superseded SessionStart moved the row's conversation id to %q", row.ConversationID)
	}
	if row.Status != "running" {
		t.Fatalf("superseded SessionStart moved the row's status to %q", row.Status)
	}
}

// TestReceiveCodexSecondSessionStartWithTheSameIDIsANoOpThatLogsNoChange
// proves PRD R125's own idempotency requirement: once a row has adopted a
// conversation id from its first SessionStart, a second SessionStart
// carrying that exact same id (a resumed session's own SessionStart fires
// again, source: "resume") must not re-run SetConversationID -- the row
// already carries that id, so there is nothing to move it to, and no
// set_conversation_id event should be recorded a second time.
func TestReceiveCodexSecondSessionStartWithTheSameIDIsANoOpThatLogsNoChange(t *testing.T) {
	db := newHookStore(t)
	const id = "codex-repeat-row"
	createHookSession(t, db, id, "codex", "")
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'starting', status_source = 'user', status_at = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	// First SessionStart adopts the id (requirement 44).
	if _, err := Receive(context.Background(), db, []byte(codexSessionStartPayload), id, "", 30); err != nil {
		t.Fatalf("first SessionStart: %v", err)
	}
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConversationID != codexConversationID {
		t.Fatalf("conversation_id after first SessionStart = %q, want %q", row.ConversationID, codexConversationID)
	}

	// Second SessionStart, same payload/id, later timestamp -- a resumed
	// session's own SessionStart, or a duplicate delivery.
	result, err := Receive(context.Background(), db, []byte(codexSessionStartPayload), id, "", 60)
	if err != nil {
		t.Fatalf("second SessionStart: %v", err)
	}
	if result.SessionID != id {
		t.Fatalf("second SessionStart resolved session = %q, want %q", result.SessionID, id)
	}

	row2, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row2.ConversationID != codexConversationID {
		t.Fatalf("conversation_id changed on a second, identical SessionStart: %q", row2.ConversationID)
	}

	var setConversationIDEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'set_conversation_id'`, id).Scan(&setConversationIDEvents); err != nil {
		t.Fatal(err)
	}
	if setConversationIDEvents != 1 {
		t.Fatalf("set_conversation_id events = %d, want exactly 1 (the first adoption only, second is a logged no-op)", setConversationIDEvents)
	}
}
