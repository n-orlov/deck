package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// R319: Y's write (AcknowledgeSession) runs asynchronously, after the key was
// pressed on one status episode. It names that episode (status_at +
// status_seq), so an acknowledgement that lands after the row moved on --
// to a prompt sharing the millisecond, or one whose stepped-back clock
// repeated the old stamp -- leaves the newer prompt unseen.

// acknowledgeCurrent is Y pressed on row "s"'s current episode.
func acknowledgeCurrent(ctx context.Context, db *Store) error {
	got, err := db.GetSession(ctx, "s")
	if err != nil {
		return err
	}
	return db.AcknowledgeSession(ctx, "s", got.Episode())
}

func TestAcknowledgeNamesTheEpisodeNotTheStamp(t *testing.T) {
	db := episodeStore(t)
	ctx := context.Background()
	episodeUpdate(t, db, "waiting", 3010)
	pressed := episodeRow(t, db).Episode()
	episodeUpdate(t, db, "error", 3010)
	before := episodeRow(t, db)
	if before.StatusAt != 3010 || before.Status != "error" {
		t.Fatalf("setup: want error at 3010, got %+v", before)
	}
	if err := db.AcknowledgeSession(ctx, "s", pressed); err != nil {
		t.Fatalf("a stale acknowledgement is not an error, got %v", err)
	}
	after := episodeRow(t, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("Y pressed on the waiting episode wrote the newer error episode:\nbefore=%+v\nafter =%+v", before, after)
	}
	if after.StatusSeen() {
		t.Fatalf("the newer error sharing the stamp was hidden: seen_status_at=%d status_at=%d", after.SeenStatusAt, after.StatusAt)
	}
	// Y on the error itself still clears it.
	if err := db.AcknowledgeSession(ctx, "s", after.Episode()); err != nil {
		t.Fatal(err)
	}
	if got := episodeRow(t, db); !got.StatusSeen() {
		t.Fatalf("Y on the current error did not set it seen: %+v", got)
	}
}

func TestAcknowledgeCannotSeeAPromptWhoseStampAStepBackRepeated(t *testing.T) {
	for _, next := range []string{"waiting", "error"} {
		t.Run(next, func(t *testing.T) {
			db := episodeStore(t)
			ctx := context.Background()
			episodeUpdate(t, db, "error", 5000)
			pressed := episodeRow(t, db).Episode()
			episodeUpdate(t, db, "running", 4000) // the clock steps back
			episodeUpdate(t, db, next, 5000)      // and reaches the old stamp again
			before := episodeRow(t, db)
			if before.StatusAt != pressed.At || before.Status != next {
				t.Fatalf("setup: want %s at %d, got %+v", next, pressed.At, before)
			}
			if err := db.AcknowledgeSession(ctx, "s", pressed); err != nil {
				t.Fatal(err)
			}
			if got := episodeRow(t, db); got.StatusSeen() || !reflect.DeepEqual(got, before) {
				t.Fatalf("Y on the earlier error marked a later %s with the same stamp seen:\nbefore=%+v\nafter =%+v", next, before, got)
			}
		})
	}
}

func TestAcknowledgeStillClearsItsPromptAfterARepeat(t *testing.T) {
	db := episodeStore(t)
	episodeUpdate(t, db, "waiting", 3010)
	pressed := episodeRow(t, db).Episode()
	// A repeat of the same prompt is inert (R303): same episode, Y still applies.
	episodeUpdate(t, db, "waiting", 4000)
	if err := db.AcknowledgeSession(context.Background(), "s", pressed); err != nil {
		t.Fatal(err)
	}
	if got := episodeRow(t, db); !got.StatusSeen() || got.StatusAt != 3010 {
		t.Fatalf("Y did not clear its own prompt after a repeat: %+v", got)
	}
}

func TestAcknowledgeIsInertOnASeenRowAndMovesNothing(t *testing.T) {
	db := episodeStore(t)
	ctx := context.Background()
	episodeUpdate(t, db, "waiting", 3010)
	before := episodeRow(t, db)
	if err := acknowledgeCurrent(ctx, db); err != nil {
		t.Fatal(err)
	}
	seen := episodeRow(t, db)
	want := before
	want.SeenStatusAt = before.StatusAt
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("Y changed more than seen_status_at:\nwant=%+v\ngot =%+v", want, seen)
	}
	if err := acknowledgeCurrent(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := episodeRow(t, db); !reflect.DeepEqual(got, seen) {
		t.Fatalf("a second Y rewrote a seen row:\nbefore=%+v\nafter =%+v", seen, got)
	}
	err := db.AcknowledgeSession(ctx, "missing", seen.Episode())
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Y on a missing row = %v, want a not-found error", err)
	}
}
