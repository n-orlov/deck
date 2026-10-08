package store

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

// buildSchemaV10PopulatedFixture advances the v9 fixture one rung: migrated by
// schemaV10 and stamped schema version 10, with one event per row.
func buildSchemaV10PopulatedFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV9PopulatedFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV10 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV10 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 10 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		if _, err := fixture.Exec(`INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, ?, 'stop', 'r', '{}')`, id, 10+i); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

// TestSchemaV11AddsHookResultColumnsToExistingEvents: a schema 10 database
// migrates with its events untouched and the five hook_* columns NULL ("no
// hook ran"), and equals a freshly created database.
func TestSchemaV11AddsHookResultColumnsToExistingEvents(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	buildSchemaV10PopulatedFixture(t, home, path)

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v10 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 11 {
		t.Fatalf("SchemaVersion = %d, want at least 11", SchemaVersion)
	}
	rows, err := st.DB().Query(`SELECT kind, reason, payload, hook_kind, hook_exit, hook_timed_out, hook_output, hook_error FROM events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var kind, reason, payload string
		var hk, ho, he sql.NullString
		var exit, timedOut sql.NullInt64
		if err := rows.Scan(&kind, &reason, &payload, &hk, &exit, &timedOut, &ho, &he); err != nil {
			t.Fatal(err)
		}
		if kind != "stop" || reason != "r" || payload != "{}" || hk.Valid || exit.Valid || timedOut.Valid || ho.Valid || he.Valid {
			t.Fatalf("event after migration = %q %q %q hook=%v %v %v %v %v", kind, reason, payload, hk, exit, timedOut, ho, he)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("events after migration = %d, want 4", n)
	}

	freshHome := t.TempDir()
	fresh, err := OpenPath(freshHome, filepath.Join(freshHome, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	freshObjects, _ := schemaShape(t, fresh.DB())
	migratedObjects, _ := schemaShape(t, st.DB())
	if !reflect.DeepEqual(freshObjects, migratedObjects) {
		t.Fatalf("schema objects differ:\nfresh    %q\nmigrated %q", freshObjects, migratedObjects)
	}
}
