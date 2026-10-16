package store

import (
	"context"
	"path/filepath"
	"testing"
)

// R319: a status is seen exactly when seen_status_at == status_at (equality,
// never an ordering), so neither a clock that steps back nor two stamps in the
// same millisecond can hide a later prompt.

func seenClockStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateSession(context.Background(), CreateSessionInput{
		ID: "s", Name: "s", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seenClockMove(t *testing.T, db *Store, status string, at int64) Session {
	t.Helper()
	ctx := context.Background()
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "s", Status: status, Reason: "permission_prompt", Source: "hook", At: at,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestLaterPromptAfterClockStepBackIsStillUnseen(t *testing.T) {
	db := seenClockStore(t)
	ctx := context.Background()
	seenClockMove(t, db, "waiting", 5000)
	if err := db.AcknowledgeSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetSession(ctx, "s"); !got.StatusSeen() || got.SeenStatusAt != 5000 {
		t.Fatalf("after acknowledge: seen_status_at=%d status_at=%d, want seen at 5000", got.SeenStatusAt, got.StatusAt)
	}
	seenClockMove(t, db, "running", 6000)
	// The clock steps back: the next prompt's stamp is below the seen stamp.
	got := seenClockMove(t, db, "waiting", 4000)
	if got.StatusAt != 4000 {
		t.Fatalf("status_at = %d, want 4000", got.StatusAt)
	}
	if got.StatusSeen() {
		t.Fatalf("a prompt stamped 4000 after a seen 5000 was hidden: seen_status_at=%d status_at=%d", got.SeenStatusAt, got.StatusAt)
	}
}

func TestTwoStampsInTheSameMillisecondCannotHidePrompt(t *testing.T) {
	for _, next := range []string{"waiting", "error"} {
		t.Run("seen waiting then "+next+" at the same stamp", func(t *testing.T) {
			db := seenClockStore(t)
			ctx := context.Background()
			seenClockMove(t, db, "waiting", 5000)
			if err := db.AcknowledgeSession(ctx, "s"); err != nil {
				t.Fatal(err)
			}
			// running at the seen stamp, then a new prompt at the same stamp.
			seenClockMove(t, db, "running", 5000)
			got := seenClockMove(t, db, next, 5000)
			if got.StatusAt != 5000 {
				t.Fatalf("status_at = %d, want 5000", got.StatusAt)
			}
			if got.StatusSeen() {
				t.Fatalf("a prompt in the same millisecond as a seen one was hidden: seen_status_at=%d status_at=%d", got.SeenStatusAt, got.StatusAt)
			}
		})
	}
}

func TestSeenStatusIsEqualityNotOrdering(t *testing.T) {
	cases := []struct {
		seen, at int64
		want     bool
	}{
		{100, 100, true},
		{0, 100, false},
		{200, 100, false}, // seen stamp ahead of status_at: not seen
		{100, 200, false},
	}
	for _, c := range cases {
		if got := (Session{SeenStatusAt: c.seen, StatusAt: c.at}).StatusSeen(); got != c.want {
			t.Errorf("StatusSeen(seen=%d, at=%d) = %v, want %v", c.seen, c.at, got, c.want)
		}
	}
}
