package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// R324: sessions.last_seen_at and the unseen predicate. A row is unseen iff
// last_output_at > last_seen_at, or its status is waiting/error and not seen
// (R319); a running row never shows the output half.

func TestSessionUnseenPredicate(t *testing.T) {
	cases := []struct {
		name string
		s    Session
		want bool
	}{
		{"output after a view sets it", Session{Status: "idle", LastOutputAt: 200, LastSeenAt: 100}, true},
		{"a view clears it", Session{Status: "idle", LastOutputAt: 200, LastSeenAt: 200}, false},
		{"never printed is not unseen", Session{Status: "idle"}, false},
		{"attention-unseen waiting sets it", Session{Status: "waiting", StatusAt: 50}, true},
		{"attention-unseen error sets it", Session{Status: "error", StatusAt: 50}, true},
		{"seen waiting is not unseen", Session{Status: "waiting", StatusAt: 50, SeenStatusAt: 50}, false},
		{"seen error is not unseen", Session{Status: "error", StatusAt: 50, SeenStatusAt: 50}, false},
		{"seen waiting with unviewed output is unseen", Session{Status: "waiting", StatusAt: 50, SeenStatusAt: 50, LastOutputAt: 9}, true},
		{"a running row shows no output-unseen marker", Session{Status: "running", LastOutputAt: 200}, false},
		{"a stopped row with unviewed output is unseen", Session{Status: "stopped", LastOutputAt: 200}, true},
		{"a zero row is seen", Session{}, false},
	}
	for _, tc := range cases {
		if got := tc.s.Unseen(); got != tc.want {
			t.Errorf("%s: Unseen() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func lastSeenStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	db, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateSession(context.Background(), CreateSessionInput{
		ID: "s", Name: "s", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

// MarkViewed writes only on a view change or a bucket change: a repeat view of
// the same output writes nothing, and it touches no status field.
func TestMarkViewedWritesOnlyOnAChange(t *testing.T) {
	ctx := context.Background()
	db := lastSeenStore(t)
	get := func() Session {
		t.Helper()
		s, err := db.GetSession(ctx, "s")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if wrote, err := db.MarkViewed(ctx, "s"); err != nil || wrote {
		t.Fatalf("viewing a row that never printed wrote=%v err=%v, want no write", wrote, err)
	}
	if err := db.RecordOutput(ctx, "s", 5000); err != nil {
		t.Fatal(err)
	}
	if !get().Unseen() {
		t.Fatal("output after a view did not set the marker")
	}
	if wrote, err := db.MarkViewed(ctx, "s"); err != nil || !wrote {
		t.Fatalf("view wrote=%v err=%v, want a write", wrote, err)
	}
	if got := get(); got.Unseen() || got.LastSeenAt != 5000 {
		t.Fatalf("after a view: unseen=%v last_seen_at=%d, want cleared at 5000", got.Unseen(), got.LastSeenAt)
	}
	for i := 0; i < 5; i++ { // ticks while the row stays viewed
		if wrote, err := db.MarkViewed(ctx, "s"); err != nil || wrote {
			t.Fatalf("tick %d wrote=%v err=%v: a tick with no bucket change must not write", i, wrote, err)
		}
	}
	if err := db.RecordOutput(ctx, "s", 16000); err != nil {
		t.Fatal(err)
	}
	if !get().Unseen() {
		t.Fatal("output in a later bucket did not set the marker again")
	}
	if wrote, err := db.MarkViewed(ctx, "s"); err != nil || !wrote {
		t.Fatalf("a bucket change under a view wrote=%v err=%v, want a write", wrote, err)
	}
	after := get()
	if after.Unseen() || after.Status != "idle" || after.StatusAt != 100 || after.SeenStatusAt != 100 {
		t.Fatalf("a view changed more than last_seen_at: %+v", after)
	}
	if _, err := db.MarkViewed(ctx, ""); err == nil {
		t.Fatal("MarkViewed accepted an empty id")
	}
}

// An attention row stays unseen after a view of its output until its status is
// seen (R319): MarkViewed does not acknowledge a prompt.
func TestMarkViewedDoesNotSeeAnAttentionStatus(t *testing.T) {
	ctx := context.Background()
	db := lastSeenStore(t)
	if err := db.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "s", Status: "waiting", Reason: "permission_prompt", Source: "hook", At: 200}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MarkViewed(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Unseen() {
		t.Fatal("a view of the output saw an unseen waiting status")
	}
	if _, err := db.MarkStatusSeen(ctx, "s", 200); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetSession(ctx, "s"); got.Unseen() {
		t.Fatal("marking the status seen left the marker")
	}
}

// schemaV19 backfills last_seen_at = last_output_at so an upgrade does not
// light every existing session.
func TestLastSeenAtMigrationBackfillsFromLastOutputAt(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV14Fixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rung := range [][]string{schemaV15, schemaV16, schemaV17, schemaV18} {
		for _, statement := range rung {
			if _, err := fixture.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 18 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`UPDATE sessions SET last_output_at = 7000 WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if tableColumnSet(t, fixture, "sessions")["last_seen_at"] {
		t.Fatal("the v18 fixture already has last_seen_at")
	}
	_ = fixture.Close()
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v18 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 19 || !tableColumnSet(t, st.DB(), "sessions")["last_seen_at"] {
		t.Fatalf("schema %d: last_seen_at missing after migrating", SchemaVersion)
	}
	for i, id := range ids {
		got, err := st.GetSession(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if i == 0 {
			want = 7000
		}
		if got.LastSeenAt != want || got.LastOutputAt != want || got.Unseen() && got.Status != "waiting" && got.Status != "error" {
			t.Fatalf("session %s: last_seen_at=%d last_output_at=%d unseen=%v, want both %d and no output marker", id, got.LastSeenAt, got.LastOutputAt, got.Unseen(), want)
		}
	}
}
