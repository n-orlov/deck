package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestSeenStatusAtMigrationBackfillsFromAcknowledged: a schema 17 database
// holding acknowledged = 1 and acknowledged = 0 rows (R318) loses
// sessions.acknowledged and gains sessions.seen_status_at, backfilled to the
// row's own status_at where it was acknowledged and 0 otherwise, so each row
// reads back the same seen/unseen answer it gave before. The fixture still
// has the acknowledged column and no seen_status_at, so the test fails against
// the v17 schema.
func TestSeenStatusAtMigrationBackfillsFromAcknowledged(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV14Fixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rung := range [][]string{schemaV15, schemaV16, schemaV17} {
		for _, statement := range rung {
			if _, err := fixture.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 17 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	type seed struct {
		status       string
		statusAt     int64
		acknowledged int
	}
	seeds := map[string]seed{
		ids[0]: {"waiting", 7000, 1},
		ids[1]: {"waiting", 8000, 0},
		ids[2]: {"error", 9000, 1},
		ids[3]: {"error", 9500, 0},
	}
	for id, s := range seeds {
		if _, err := fixture.Exec(`UPDATE sessions SET status = ?, status_at = ?, acknowledged = ? WHERE id = ?`,
			s.status, s.statusAt, s.acknowledged, id); err != nil {
			t.Fatal(err)
		}
	}
	if !tableColumnSet(t, fixture, "sessions")["acknowledged"] || tableColumnSet(t, fixture, "sessions")["seen_status_at"] {
		t.Fatal("the v17 fixture does not hold acknowledged without seen_status_at")
	}
	_ = fixture.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v17 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 18 {
		t.Fatalf("SchemaVersion = %d, want at least 18", SchemaVersion)
	}
	columns := tableColumnSet(t, st.DB(), "sessions")
	if columns["acknowledged"] || !columns["seen_status_at"] {
		t.Fatalf("after migrating to schemaV18: acknowledged=%v seen_status_at=%v, want false/true", columns["acknowledged"], columns["seen_status_at"])
	}
	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	for id, s := range seeds {
		got, err := st.GetSession(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		wantSeenAt := int64(0)
		if s.acknowledged == 1 {
			wantSeenAt = s.statusAt
		}
		if got.StatusAt != s.statusAt || got.SeenStatusAt != wantSeenAt || got.StatusSeen() != (s.acknowledged == 1) {
			t.Fatalf("session %s (%s, status_at %d, acknowledged %d): status_at=%d seen_status_at=%d seen=%v, want seen_status_at %d seen=%v",
				id, s.status, s.statusAt, s.acknowledged, got.StatusAt, got.SeenStatusAt, got.StatusSeen(), wantSeenAt, s.acknowledged == 1)
		}
	}
}
