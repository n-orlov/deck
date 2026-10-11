package hookrecv

import (
	"context"
	"fmt"
	"testing"
)

// TestAdoptTriggersFireFromTheFourHookPaths: Stop, UserPromptSubmit, Copilot's
// agentStop and session_info_changed hand the row to Origin.Adopt; no other
// hook, and no hook of a replaced launch, does.
func TestAdoptTriggersFireFromTheFourHookPaths(t *testing.T) {
	for _, tc := range []struct {
		name, agent, event, payload, generation string
		want                                    bool
	}{
		{name: "Stop", agent: "claude", payload: `{"hook_event_name":"Stop","session_id":"c-1"}`, want: true},
		{name: "UserPromptSubmit", agent: "claude", payload: `{"hook_event_name":"UserPromptSubmit","session_id":"c-1"}`, want: true},
		{name: "agentStop", agent: "copilot", event: "agentStop", payload: `{"sessionId":"c-1"}`, want: true},
		{name: "session_info_changed", agent: "pi", payload: `{"hook_event_name":"session_info_changed","session_id":"c-1","name":"x"}`, want: true},
		{name: "SessionStart", agent: "claude", payload: `{"hook_event_name":"SessionStart","session_id":"c-1","source":"startup"}`},
		{name: "Notification", agent: "claude", payload: `{"hook_event_name":"Notification","session_id":"c-1"}`},
		{name: "copilot userPromptSubmitted", agent: "copilot", event: "userPromptSubmitted", payload: `{"sessionId":"c-1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			createHookSession(t, db, "row-1", tc.agent, "c-1")
			var adopted []string
			origin := Origin{SessionID: "row-1", Event: tc.event, Adopt: func(_ context.Context, id string, _ AdoptHint) { adopted = append(adopted, id) }}
			if _, err := ReceiveFrom(context.Background(), db, []byte(tc.payload), origin, 100); err != nil {
				t.Fatal(err)
			}
			if got := len(adopted) == 1 && adopted[0] == "row-1"; got != tc.want || len(adopted) > 1 {
				t.Fatalf("Adopt calls = %v, want a call for row-1: %v", adopted, tc.want)
			}
		})
	}
}

// TestAdoptSkipsSupersededAndOrphanHooks: a hook of a launch deck replaced, and
// one that resolves to no row, trigger nothing.
func TestAdoptSkipsSupersededAndOrphanHooks(t *testing.T) {
	db := newHookStore(t)
	createHookSession(t, db, "row-1", "claude", "c-1")
	if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = '4242@boot#gen-2', launch_lease_until = 0 WHERE id = 'row-1'`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	adopt := func(context.Context, string, AdoptHint) { calls++ }
	stop := func(conversation string) []byte {
		return []byte(fmt.Sprintf(`{"hook_event_name":"Stop","session_id":%q}`, conversation))
	}
	if _, err := ReceiveFrom(context.Background(), db, stop("c-1"), Origin{LaunchGeneration: "gen-1", Adopt: adopt}, 100); err != nil {
		t.Fatal(err)
	}
	_, _ = ReceiveFrom(context.Background(), db, stop("nobody"), Origin{Adopt: adopt}, 101)
	if calls != 0 {
		t.Fatalf("Adopt called %d times for a superseded and an orphan hook, want 0", calls)
	}
}

// TestSessionTitleIsOptionalAndHintsTheAdoption: a payload without
// session_title or transcript_path behaves exactly as before (same stored
// result, an empty hint); one with them hands both to Adopt.
func TestSessionTitleIsOptionalAndHintsTheAdoption(t *testing.T) {
	const base = `"hook_event_name":"Stop","session_id":"c-1"`
	run := func(payload string) (Result, AdoptHint, string) {
		db := newHookStore(t)
		createHookSession(t, db, "row-1", "claude", "c-1")
		var hint AdoptHint
		origin := Origin{SessionID: "row-1", Adopt: func(_ context.Context, _ string, h AdoptHint) { hint = h }}
		result, err := ReceiveFrom(context.Background(), db, []byte(payload), origin, 100)
		if err != nil {
			t.Fatal(err)
		}
		var status string
		if err := db.DB().QueryRow(`SELECT status FROM sessions WHERE id = 'row-1'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return result, hint, status
	}
	without, hintWithout, statusWithout := run(`{` + base + `}`)
	with, hintWith, statusWith := run(`{` + base + `,"transcript_path":"/t/c-1.jsonl","session_title":"my title"}`)
	if hintWithout != (AdoptHint{}) {
		t.Fatalf("hint without the optional fields = %#v, want zero", hintWithout)
	}
	if hintWith != (AdoptHint{TranscriptPath: "/t/c-1.jsonl", SessionTitle: "my title"}) {
		t.Fatalf("hint with the fields = %#v", hintWith)
	}
	if without != with || statusWithout != statusWith || statusWith != "idle" {
		t.Fatalf("session_title changed the hook's own write: %#v/%s vs %#v/%s", without, statusWithout, with, statusWith)
	}
}
