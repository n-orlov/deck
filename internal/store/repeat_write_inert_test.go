package store

import (
	"context"
	"path/filepath"
	"testing"
)

// R303: a status write that leaves the status unchanged is inert. These six
// tests each seed a row in a status with a recognisable status_at,
// status_source, notify_epoch, hook_fired and seen state, repeat the status
// through one writer, and require every one of those five to read back
// exactly as seeded (the reason may update; last_probe_at is not asserted
// because it may advance).

const (
	repeatSeedAt     = 1000
	repeatSeedEpoch  = 7
	repeatSeedFired  = `[["waiting","permission_prompt"]]`
	repeatWriteLater = 5000
)

// repeatSeed is the five values a repeat must leave alone, read through the
// public Session plus the raw hook_fired column the Session type omits.
type repeatSeed struct {
	statusAt, notifyEpoch int64
	source                string
	acknowledged          bool
	hookFired             string
}

func openRepeatStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedRepeatRow creates a row, moves it to status as source (a real
// transition at repeatSeedAt) and then pins notify_epoch, hook_fired and the
// seen state to values a spurious reset would visibly change.
func seedRepeatRow(t *testing.T, db *Store, status, source, reason string, acknowledged bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.CreateSession(ctx, CreateSessionInput{
		ID: "row", Name: "row", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	in := StatusUpdateInput{SessionID: "row", Status: status, Reason: reason, Source: source, At: repeatSeedAt}
	if source == "probe" {
		in.StaleAfter = 1
	}
	if err := db.UpdateSessionStatus(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE sessions SET notify_epoch = ?, hook_fired = ?, acknowledged = ? WHERE id = 'row'`,
		repeatSeedEpoch, repeatSeedFired, boolInt(acknowledged)); err != nil {
		t.Fatal(err)
	}
}

func readRepeatRow(t *testing.T, db *Store) (Session, repeatSeed) {
	t.Helper()
	ctx := context.Background()
	row, err := db.GetSession(ctx, "row")
	if err != nil {
		t.Fatal(err)
	}
	var fired string
	if err := db.db.QueryRowContext(ctx, `SELECT COALESCE(hook_fired, '') FROM sessions WHERE id = 'row'`).Scan(&fired); err != nil {
		t.Fatal(err)
	}
	return row, repeatSeed{row.StatusAt, row.NotifyEpoch, row.StatusSource, row.Acknowledged, fired}
}

// requireInert fails unless the five protected values equal their seed.
func requireInert(t *testing.T, db *Store, wantSource string, wantAcknowledged bool) Session {
	t.Helper()
	row, got := readRepeatRow(t, db)
	want := repeatSeed{repeatSeedAt, repeatSeedEpoch, wantSource, wantAcknowledged, repeatSeedFired}
	if got != want {
		t.Fatalf("a repeat write moved protected state: status_at/notify_epoch/source/acknowledged/hook_fired = %+v, want %+v", got, want)
	}
	return row
}

func TestProbeRepeatingWaitingIsInert(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "waiting", "probe", "idle_prompt", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "waiting", Reason: "idle_prompt",
		Source: "probe", At: repeatWriteLater, StaleAfter: 1, EventKind: "probe"}); err != nil {
		t.Fatal(err)
	}
	// The seed's acknowledged=1 stands for a row cleared with Y: a waiting
	// repeat must not mark it unseen again.
	requireInert(t, db, "probe", true)
}

func TestProbeRepeatingRunningIsInert(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "running", "probe", "working", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "working",
		Source: "probe", At: repeatWriteLater, StaleAfter: 1, EventKind: "probe"}); err != nil {
		t.Fatal(err)
	}
	requireInert(t, db, "probe", true)
}

func TestProbeRepeatingErrorIsInert(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "error", "probe", "api_error", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "error", Reason: "api_error",
		Source: "probe", At: repeatWriteLater, StaleAfter: 1, EventKind: "probe"}); err != nil {
		t.Fatal(err)
	}
	requireInert(t, db, "probe", true)
}

func TestHookRepeatingSameStatusAndReasonIsInert(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "waiting", "hook", "permission_prompt", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "waiting", Reason: "permission_prompt",
		Source: "hook", At: repeatWriteLater, EventKind: "notification"}); err != nil {
		t.Fatal(err)
	}
	row := requireInert(t, db, "hook", true)
	if row.StatusReason != "permission_prompt" {
		t.Fatalf("reason = %q, want permission_prompt", row.StatusReason)
	}
}

// TestHookRepeatingSameStatusWithDifferentReasonUpdatesOnlyTheReason is the
// "same status, different reason" case: the reason updates, status_at does
// not, and the other four protected values stay.
func TestHookRepeatingSameStatusWithDifferentReasonUpdatesOnlyTheReason(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "waiting", "hook", "permission_prompt", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "waiting", Reason: "question",
		Source: "hook", At: repeatWriteLater, EventKind: "notification"}); err != nil {
		t.Fatal(err)
	}
	row := requireInert(t, db, "hook", true)
	if row.StatusReason != "question" {
		t.Fatalf("reason = %q, want question (a repeat still updates the reason)", row.StatusReason)
	}
}

func TestUserSourcedRepeatIsInert(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "running", "user", "attached", false)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "running", Reason: "attached",
		Source: "user", At: repeatWriteLater, EventKind: "attached"}); err != nil {
		t.Fatal(err)
	}
	// The unseen state (acknowledged=0) is also left as it was.
	requireInert(t, db, "user", false)
}

// A repeat that is a real transition target elsewhere still behaves as before:
// the first waiting after running dates the row and opens an unseen episode.
func TestTransitionStillDatesTheRowAndMarksItUnseen(t *testing.T) {
	db := openRepeatStore(t)
	ctx := context.Background()
	seedRepeatRow(t, db, "running", "hook", "working", true)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "row", Status: "waiting", Reason: "permission_prompt",
		Source: "hook", At: repeatWriteLater, EventKind: "notification"}); err != nil {
		t.Fatal(err)
	}
	row, _ := readRepeatRow(t, db)
	if row.StatusAt != repeatWriteLater || row.Acknowledged {
		t.Fatalf("transition to waiting: status_at=%d acknowledged=%v, want %d/false", row.StatusAt, row.Acknowledged, repeatWriteLater)
	}
}
