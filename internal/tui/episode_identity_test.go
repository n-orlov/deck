package tui

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// R319/R321/R322: a status_at stamp is not an identity. A cached engagement
// decision, an in-flight seen write, the sort hold and a dwell in progress
// each belong to the status episode (status_at + status_seq) they observed,
// whether the next episode shares the millisecond or the clock stepped back.

// restamp moves the target through running into a fresh waiting episode whose
// stamp is the given one, then reloads the model.
func restamp(t *testing.T, m Model, db *store.Store, id string, runningAt, waitingAt int64) Model {
	t.Helper()
	ctx := context.Background()
	for _, u := range []store.StatusUpdateInput{
		{SessionID: id, Status: "running", Reason: "episode test", Source: "hook", At: runningAt},
		{SessionID: id, Status: "waiting", Reason: "episode test", Source: "hook", At: waitingAt},
	} {
		if err := db.UpdateSessionStatus(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	return reloadInert(t, m)
}

func TestLateEngagementWriteCannotSeeANewerPromptSharingItsStamp(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runningGap int64 // running's stamp relative to the shared stamp
	}{
		{"same millisecond", 0},
		{"clock stepped back between them", -500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, "row-waiting", false)
			old := snapshotDurable(t, rig.db, "row-waiting").Session
			late := rig.m.engage(engageKey) // issued, not yet run: older async work
			if late == nil {
				t.Fatal("the first engagement issued no write")
			}
			m := restamp(t, rig.m, rig.db, "row-waiting", old.StatusAt+tc.runningGap, old.StatusAt)
			now := snapshotDurable(t, rig.db, "row-waiting").Session
			if now.StatusAt != old.StatusAt || now.Episode() == old.Episode() || now.StatusSeen() {
				t.Fatalf("fixture: want a new unseen episode on the old stamp, old=%+v now=%+v", old, now)
			}
			// The older write lands late: it must not see the new prompt.
			if msg := late(); msg.(sessionEngaged).err != nil {
				t.Fatal(msg)
			}
			if snapshotDurable(t, rig.db, "row-waiting").Session.StatusSeen() {
				t.Fatal("an engagement with the older episode marked the newer prompt seen")
			}
			// The cached decision for the old episode must not suppress engagement
			// with the new one, even though status_at is equal.
			cmd := m.engage(engageKey)
			if cmd == nil {
				t.Fatal("the old episode's cached engagement suppressed engagement with the newer prompt")
			}
			m = settleEngagement(t, m, cmd)
			if got := snapshotDurable(t, rig.db, "row-waiting").Session; !got.StatusSeen() || got.SeenStatusAt != got.StatusAt {
				t.Fatalf("engaging the new episode did not set seen: %+v", got)
			}
			if again := m.engage(engageKey); again != nil {
				t.Fatal("a second engagement with the same episode issued another write")
			}
		})
	}
}

func TestDwellOfAnEarlierPromptNeverSeesANewPromptOnTheSameStamp(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runningGap int64
	}{
		{"same millisecond", 0},
		{"clock stepped back", -700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDwellRig(t)
			r.capture(dwellPromptPane) // the first prompt is on screen at the view's start
			old := snapshotDurable(t, r.db, dwellTarget).Session
			r.advance(seenDwell - 100*time.Millisecond)
			r.m = restamp(t, r.m, r.db, dwellTarget, old.StatusAt+tc.runningGap, old.StatusAt)
			r.capture(dwellPromptPane)
			r.advance(100 * time.Millisecond)
			r.tick()
			if r.seen() || r.writes != 0 {
				t.Fatalf("seen=%v writes=%d: the dwell of the earlier prompt saw a new one sharing its stamp", r.seen(), r.writes)
			}
		})
	}
}

func TestEngagementHoldBelongsToItsEpisode(t *testing.T) {
	held := map[string]store.StatusEpisode{"a": {At: 50, Seq: 1}}
	same := store.Session{ID: "a", Status: "waiting", StatusAt: 50, StatusSeq: 1, SeenStatusAt: 50}
	next := store.Session{ID: "a", Status: "waiting", StatusAt: 50, StatusSeq: 3, SeenStatusAt: 50}
	if got, want := attentionSortRank(same, held), attentionRank("waiting"); got != want {
		t.Fatalf("the engaged episode lost its hold: rank %d, want %d", got, want)
	}
	if got, want := attentionSortRank(next, held), sessionAttentionRank(next); got != want {
		t.Fatalf("a later episode on the same stamp inherited the hold: rank %d, want %d", got, want)
	}
}
