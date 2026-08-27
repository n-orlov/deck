package hookrecv

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// TestResumedRowAcceptsTheNewPanesFirstHook is issue #9's guard-2 leg, driven
// through the real hook path. store.UpdateSessionStatus drops a hook write of
// `running` while pane_exit_status is set — correctly, for a crash verdict on
// the CURRENT pane — and SessionStart and UserPromptSubmit both map to
// `running`. Because nothing cleared the column on resume, the new pane's very
// first hook was dropped for the rest of the row's life.
//
// The trap that made this hard to spot is asserted too: UpdateSessionStatus
// records the event unconditionally, so the event log looks healthy either way.
// That is why the verdict below is read off the status COLUMN. The operator's
// transcribed row is exactly this shape: `session_start` present in events at
// 07:25:10, status_at still reading the previous day.
func TestResumedRowAcceptsTheNewPanesFirstHook(t *testing.T) {
	for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
		t.Run(event, func(t *testing.T) {
			ctx := context.Background()
			db := newHookStore(t)
			id := "resumed-after-crash-" + event
			conversationID := "conversation-" + event
			createHookSession(t, db, id, "claude", conversationID)

			// The previous pane's crash, stored the way reconciliation stores it.
			exit := 137
			if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
				SessionID: id, Status: "error", Reason: "tmux pane exited with status 137",
				Source: "tmux", At: 10, EventKind: "tmux.pane_dead",
				PaneExitStatus: &exit, CrashTail: "claude: killed",
			}); err != nil {
				t.Fatalf("store the crash verdict: %v", err)
			}
			// The row is killed so it becomes leasable again: AcquireLaunchLease
			// refuses any status other than stopped, which is why `r` alone was
			// no escape from the wedge.
			if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
				SessionID: id, Status: "stopped", Reason: "killed by user",
				Source: "user", At: 11, EventKind: "killed", KilledByUser: true,
			}); err != nil {
				t.Fatalf("kill the crashed row: %v", err)
			}
			before, err := db.GetSession(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if before.Status != "stopped" || before.PaneExitStatus == nil || before.CrashTail == "" {
				t.Fatalf("fixture is not discriminating: want a stopped row still carrying the previous pane's verdict, got %#v", before)
			}

			// The resume itself, then the tmux-sourced launch.ready observation
			// service.Resume writes once the new pane exists.
			result, err := db.AcquireLaunchLease(ctx, id, store.CurrentLaunchLeaseOwner(), 30*time.Second, 12)
			if err != nil {
				t.Fatalf("acquire launch lease: %v", err)
			}
			if result.Outcome != store.LaunchLeaseAcquired {
				t.Fatalf("lease outcome = %v, want acquired", result.Outcome)
			}
			if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
				SessionID: id, Status: "starting", Source: "tmux", At: 13, EventKind: "launch.ready",
			}); err != nil {
				t.Fatalf("record launch.ready: %v", err)
			}

			raw := []byte(fmt.Sprintf(`{"hook_event_name":%q,"session_id":%q,"source":"resume"}`, event, conversationID))
			// The new pane carries the generation THIS lease minted, which is
			// what a really-resumed pane exports (issue #11, R74): passing an
			// empty token here would make the hook look like one from the
			// replaced pane and test R74's drop instead of #9's guard.
			if _, err := Receive(ctx, db, raw, "", result.LaunchGeneration, 14); err != nil {
				t.Fatalf("receive %s from the new pane: %v", event, err)
			}

			// The event log looks healthy whether or not the fix is present...
			var events int
			if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ? AND payload = ?`,
				id, Mappings[event].Kind, string(raw)).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 1 {
				t.Fatalf("%s events = %d, want 1", event, events)
			}
			// ...so the verdict is the status column.
			got, err := db.GetSession(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "running" || got.StatusSource != "hook" || got.StatusAt != 14 {
				t.Fatalf("the new pane's %s was dropped by the replaced pane's crash verdict (#9): status=%q source=%q status_at=%d",
					event, got.Status, got.StatusSource, got.StatusAt)
			}
		})
	}
}
