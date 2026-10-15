package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// buildSchemaV14Fixture advances the v13 fixture one rung: migrated by
// schemaV14 and stamped schema version 14, rows kept.
func buildSchemaV14Fixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV13Fixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV14 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV14 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 14 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

// TestOutputClockMigrationUpgradesAV14Database: a schema 14 database holding
// four sessions gains sessions.last_output_at (R304), every session reads 0
// (no output seen yet), no session is recreated, and the database is stamped
// with the current version. The fixture has no such column, so the test fails
// against the v14 schema.
func TestOutputClockMigrationUpgradesAV14Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV14Fixture(t, home, path)
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`SELECT last_output_at FROM sessions`); err == nil {
		t.Fatal("the v14 fixture already has sessions.last_output_at")
	}
	_ = pre.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v14 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion != 15 {
		t.Fatalf("SchemaVersion = %d, want 15", SchemaVersion)
	}
	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	if !tableColumnSet(t, st.DB(), "sessions")["last_output_at"] {
		t.Fatal("sessions.last_output_at missing after migrating to schemaV15")
	}
	if !tableColumnSet(t, st.DB(), "bookmarks")["session_id"] {
		t.Fatal("the v14 bookmarks table did not survive the v15 migration")
	}
	rows, err := st.DB().Query(`SELECT id, last_output_at FROM sessions ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			t.Fatal(err)
		}
		if at != 0 {
			t.Fatalf("session %s last_output_at = %d after migration, want 0", id, at)
		}
		found := false
		for _, want := range ids {
			found = found || want == id
		}
		if !found {
			t.Fatalf("session %s is not one of the fixture's %v", id, ids)
		}
		seen++
	}
	if seen != 4 {
		t.Fatalf("sessions after migration = %d, want the 4 fixture rows", seen)
	}
	got, err := st.GetSession(t.Context(), ids[0])
	if err != nil || got.LastOutputAt != 0 {
		t.Fatalf("GetSession after migration: LastOutputAt = %d, %v; want 0", got.LastOutputAt, err)
	}
}
