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
			origin := Origin{SessionID: "row-1", Event: tc.event, Adopt: func(_ context.Context, id string) { adopted = append(adopted, id) }}
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
	adopt := func(context.Context, string) { calls++ }
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
