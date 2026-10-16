package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// R319: Y's store write is a queued command, so it can land after the row
// has moved to a newer prompt. The command carries the status episode the
// key was pressed on (status_at + status_seq); a newer prompt sharing that
// stamp, or reaching it again after a clock step back, stays unseen.

// acknowledgeCurrent is Y pressed on the row's current episode.
func acknowledgeCurrent(ctx context.Context, db *store.Store, id string) error {
	got, err := db.GetSession(ctx, id)
	if err != nil {
		return err
	}
	return db.AcknowledgeSession(ctx, id, got.Episode())
}

// queueY presses Y on the rig's unseen waiting row (status_at 110) and
// returns the not-yet-run command.
func queueY(t *testing.T) (*store.Store, func() Model) {
	t.Helper()
	m, db := ackOfferStoreModel(t)
	if m.sessions[0].StatusAt != 110 || m.sessions[0].StatusSeen() {
		t.Fatalf("setup: want an unseen waiting row at 110, got %+v", m.sessions[0])
	}
	updated, cmd := m.Update(key("Y"))
	if cmd == nil {
		t.Fatal("Y on an unseen row scheduled nothing")
	}
	m = updated.(Model)
	run := func() Model {
		next, _ := m.Update(cmd())
		next, _ = next.(Model).Update(next.(Model).loadSessions())
		return next.(Model)
	}
	return db, run
}

func ackEpisodeMove(t *testing.T, db *store.Store, status string, at int64) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: "waiting", Status: status, Reason: "episode test", Source: "hook", At: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func ackEpisodeRow(t *testing.T, db *store.Store) store.Session {
	t.Helper()
	got, err := db.GetSession(context.Background(), "waiting")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestQueuedYCannotSeeANewErrorSharingItsStamp(t *testing.T) {
	db, run := queueY(t)
	ackEpisodeMove(t, db, "error", 110)
	before := ackEpisodeRow(t, db)
	if before.Status != "error" || before.StatusAt != 110 {
		t.Fatalf("setup: want error at 110, got %+v", before)
	}
	m := run()
	if got := ackEpisodeRow(t, db); got.StatusSeen() || got.SeenStatusAt != before.SeenStatusAt {
		t.Fatalf("Y queued on waiting marked the newer error seen: seen_status_at %d -> %d (status_at %d)",
			before.SeenStatusAt, got.SeenStatusAt, got.StatusAt)
	}
	if m.attachError != "" {
		t.Errorf("a Y overtaken by a newer prompt set a message: %q", m.attachError)
	}
	if !NeedsAttention(m.sessions[0]) || m.sessions[0].StatusSeen() {
		t.Errorf("the newer error left attention: %+v", m.sessions[0])
	}
}

func TestQueuedYCannotSeeAPromptAfterAClockStepBackToItsStamp(t *testing.T) {
	for _, next := range []string{"waiting", "error"} {
		t.Run(next, func(t *testing.T) {
			db, run := queueY(t)
			ackEpisodeMove(t, db, "running", 90) // the clock steps back
			ackEpisodeMove(t, db, next, 110)     // and repeats the stamp Y saw
			before := ackEpisodeRow(t, db)
			if before.Status != next || before.StatusAt != 110 {
				t.Fatalf("setup: want %s at 110, got %+v", next, before)
			}
			run()
			if got := ackEpisodeRow(t, db); got.StatusSeen() {
				t.Fatalf("Y queued on the earlier waiting marked a later %s at the same stamp seen: %+v", next, got)
			}
		})
	}
}

func TestQueuedYDoesNotWriteARowThatLeftItsEpisode(t *testing.T) {
	db, run := queueY(t)
	ackEpisodeMove(t, db, "running", 110)
	before := ackEpisodeRow(t, db)
	run()
	if got := ackEpisodeRow(t, db); got.SeenStatusAt != before.SeenStatusAt {
		t.Fatalf("Y queued on waiting wrote the running episode: seen_status_at %d -> %d", before.SeenStatusAt, got.SeenStatusAt)
	}
}

func TestQueuedYStillClearsItsPromptAfterARepeat(t *testing.T) {
	db, run := queueY(t)
	ackEpisodeMove(t, db, "waiting", 200) // a repeat: same episode (R303)
	m := run()
	got := ackEpisodeRow(t, db)
	if !got.StatusSeen() || got.StatusAt != 110 {
		t.Fatalf("Y did not clear its own prompt after a repeat: %+v", got)
	}
	if !m.sessions[0].StatusSeen() {
		t.Errorf("the model still shows the acknowledged row unseen: %+v", m.sessions[0])
	}
}
