package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestProbeMissStreakMigrationUpgradesAV15Database: a schema 15 database gains
// sessions.probe_miss_streak (R315), every session reads 0, and the database is
// stamped with the current version. The fixture lacks the column, so the test
// fails against the v15 schema.
func TestProbeMissStreakMigrationUpgradesAV15Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV14Fixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range schemaV15 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 15 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`SELECT probe_miss_streak FROM sessions`); err == nil {
		t.Fatal("the v15 fixture already has sessions.probe_miss_streak")
	}
	_ = fixture.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v15 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 16 {
		t.Fatalf("SchemaVersion = %d, want at least 16", SchemaVersion)
	}
	if !tableColumnSet(t, st.DB(), "sessions")["probe_miss_streak"] {
		t.Fatal("sessions.probe_miss_streak missing after migrating to schemaV16")
	}
	got, err := st.GetSession(t.Context(), ids[0])
	if err != nil || got.ProbeMissStreak != 0 {
		t.Fatalf("GetSession after migration: streak = %d, %v; want 0", got.ProbeMissStreak, err)
	}
}
