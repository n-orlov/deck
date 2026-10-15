package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestLastEventAtMigrationUpgradesAV16Database: a schema 16 database gains the
// events_session_at index (R315), and Session.LastEventAt reads the newest
// event already on disk for each session, then every later one. The fixture
// lacks the index, so the test fails against the v16 schema.
func TestLastEventAtMigrationUpgradesAV16Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV14Fixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range append(append([]string{}, schemaV15...), schemaV16...) {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 16 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	var indexes int
	if err := fixture.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'events_session_at'`).Scan(&indexes); err != nil || indexes != 0 {
		t.Fatalf("the v16 fixture already has events_session_at (%d, %v)", indexes, err)
	}
	if _, err := fixture.Exec(`INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, 7000, 'note', '', ''), (?, 4000, 'note', '', '')`, ids[0], ids[0]); err != nil {
		t.Fatal(err)
	}
	var want int64
	if err := fixture.QueryRow(`SELECT MAX(at) FROM events WHERE session_id = ?`, ids[0]).Scan(&want); err != nil {
		t.Fatal(err)
	}
	_ = fixture.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v16 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 17 {
		t.Fatalf("SchemaVersion = %d, want at least 17", SchemaVersion)
	}
	if err := st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'events_session_at'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("events_session_at missing after migrating to schemaV17 (%d, %v)", indexes, err)
	}
	got, err := st.GetSession(t.Context(), ids[0])
	if err != nil || got.LastEventAt != want || want < 7000 {
		t.Fatalf("GetSession after migration: last_event_at = %d, %v; want the newest event %d", got.LastEventAt, err, want)
	}
	if err := st.RecordSessionNote(t.Context(), ids[0], "after", want+1); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetSession(t.Context(), ids[0]); got.LastEventAt != want+1 {
		t.Fatalf("last_event_at = %d after a post-migration event, want %d", got.LastEventAt, want+1)
	}
}
