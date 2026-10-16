package store

import (
	"context"
	"testing"
)

// R319/R321/R322: a status_at stamp is a wall-clock millisecond, so it is not
// an identity: two attention episodes can share one and a stepped clock can
// repeat an old one. A seen write names the episode (status_at + status_seq)
// it observed, and an older write cannot mark a newer episode seen.

func episodeStore(t *testing.T) *Store {
	t.Helper()
	return lastSeenStore(t)
}

func episodeUpdate(t *testing.T, db *Store, status string, at int64) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), StatusUpdateInput{
		SessionID: "s", Status: status, Reason: "episode-test", Source: "hook", At: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func episodeRow(t *testing.T, db *Store) Session {
	t.Helper()
	got, err := db.GetSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMarkStatusSeenNamesTheEpisodeNotTheStamp(t *testing.T) {
	cases := []struct {
		name string
		// steps move the row from the observed episode (waiting@3010) to
		// another attention episode; the stale write must then do nothing.
		steps func(t *testing.T, db *Store)
	}{
		{"waiting again in the same millisecond (the review's probe)", func(t *testing.T, db *Store) {
			episodeUpdate(t, db, "running", 3010)
			episodeUpdate(t, db, "waiting", 3010)
		}},
		{"waiting again after the clock stepped back and forward onto the stamp", func(t *testing.T, db *Store) {
			episodeUpdate(t, db, "running", 2000)
			episodeUpdate(t, db, "waiting", 3010)
		}},
		{"error in the same millisecond (no notify_epoch change between them)", func(t *testing.T, db *Store) {
			episodeUpdate(t, db, "error", 3010)
		}},
		{"waiting -> error -> waiting, all on one stamp", func(t *testing.T, db *Store) {
			episodeUpdate(t, db, "error", 3010)
			episodeUpdate(t, db, "waiting", 3010)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := episodeStore(t)
			episodeUpdate(t, db, "waiting", 3010)
			old := episodeRow(t, db).Episode()
			tc.steps(t, db)
			before := episodeRow(t, db)
			if before.Status == "running" || before.StatusSeen() {
				t.Fatalf("fixture: want a fresh unseen attention episode, got %+v", before)
			}
			if before.StatusAt != 3010 {
				t.Fatalf("fixture: the new episode must reuse the stamp 3010, got %d", before.StatusAt)
			}
			changed, err := db.MarkStatusSeen(context.Background(), "s", old)
			if err != nil || changed {
				t.Fatalf("a late write for the older episode = (%v, %v), want (false, nil)", changed, err)
			}
			after := episodeRow(t, db)
			if after.StatusSeen() || after.SeenStatusAt != before.SeenStatusAt {
				t.Fatalf("the older episode's write marked the newer prompt seen: %+v", after)
			}
			// The newer episode's own write still lands.
			changed, err = db.MarkStatusSeen(context.Background(), "s", before.Episode())
			if err != nil || !changed {
				t.Fatalf("the current episode's write = (%v, %v), want (true, nil)", changed, err)
			}
			if got := episodeRow(t, db); !got.StatusSeen() || got.SeenStatusAt != got.StatusAt {
				t.Fatalf("seen_status_at == status_at must hold after the write: %+v", got)
			}
		})
	}
}

func TestStatusEpisodeAdvancesOnTransitionsOnly(t *testing.T) {
	db := episodeStore(t)
	episodeUpdate(t, db, "waiting", 3010)
	first := episodeRow(t, db)
	// A repeat of the current status (R303) opens no episode and is inert,
	// even one dated later or earlier than the row's stamp.
	for _, at := range []int64{3010, 3500, 100} {
		episodeUpdate(t, db, "waiting", at)
		if got := episodeRow(t, db); got.Episode() != first.Episode() || got.SeenStatusAt != first.SeenStatusAt {
			t.Fatalf("a repeat at %d moved the episode: %+v -> %+v", at, first, got)
		}
	}
	episodeUpdate(t, db, "running", 3010)
	if got := episodeRow(t, db); got.StatusSeq <= first.StatusSeq {
		t.Fatalf("a transition did not advance status_seq: %d -> %d", first.StatusSeq, got.StatusSeq)
	}
}

func TestMarkStatusSeenIsInertOnASeenRowAndMovesNothing(t *testing.T) {
	db := episodeStore(t)
	episodeUpdate(t, db, "waiting", 3010)
	ep := episodeRow(t, db).Episode()
	before := episodeRow(t, db)
	if changed, err := db.MarkStatusSeen(context.Background(), "s", ep); err != nil || !changed {
		t.Fatalf("first write = (%v, %v), want (true, nil)", changed, err)
	}
	seen := episodeRow(t, db)
	if seen.Status != before.Status || seen.StatusAt != before.StatusAt || seen.StatusSeq != before.StatusSeq ||
		seen.NotifyEpoch != before.NotifyEpoch || seen.StatusSource != before.StatusSource || seen.LastEventAt != before.LastEventAt {
		t.Fatalf("the seen write changed more than seen_status_at:\nbefore=%+v\nafter =%+v", before, seen)
	}
	if changed, err := db.MarkStatusSeen(context.Background(), "s", ep); err != nil || changed {
		t.Fatalf("second write = (%v, %v), want (false, nil)", changed, err)
	}
	// A repeated waiting hook on a seen row keeps it seen and unmoved.
	episodeUpdate(t, db, "waiting", 4000)
	if got := episodeRow(t, db); !got.StatusSeen() || got.StatusAt != 3010 || got.StatusSeq != seen.StatusSeq {
		t.Fatalf("a repeat hook disturbed a seen episode: %+v", got)
	}
}

// A new row starts at status_seq 0; each transition counts one.

func TestStatusSeqStartsAtZeroAndCountsTransitions(t *testing.T) {
	db := episodeStore(t)
	if got := episodeRow(t, db); got.StatusSeq != 0 {
		t.Fatalf("a fresh row's status_seq = %d, want 0", got.StatusSeq)
	}
	episodeUpdate(t, db, "waiting", 150)
	if got := episodeRow(t, db); got.StatusSeq != 1 {
		t.Fatalf("status_seq after one transition = %d, want 1", got.StatusSeq)
	}
}
