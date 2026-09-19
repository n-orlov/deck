package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// buildSchemaV6Fixture creates a fresh, standalone schemaV1-V6 database
// file at path (schema_version = 6), seeds it with three sessions that
// each carry a distinct sessions.workspace value, and closes it -- the
// pre-R128 shape schemaV7 (task 008) must migrate away from. Returns the
// three session ids in insertion order.
func buildSchemaV6Fixture(t *testing.T, home, path string) [3]string {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ladder := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6} {
		for _, statement := range ladder {
			if _, err := fixture.Exec(statement); err != nil {
				t.Fatalf("build schemaV6 fixture: %v", err)
			}
		}
	}
	if _, err := fixture.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', 6)`); err != nil {
		t.Fatal(err)
	}
	ids := [3]string{
		"v6-fixture-session-a", "v6-fixture-session-b", "v6-fixture-session-c",
	}
	legacyColumnValues := [3]string{"team-a", "team-b", "team-c"}
	names := [3]string{"alpha", "bravo", "charlie"}
	for i, id := range ids {
		if _, err := fixture.Exec(`INSERT INTO sessions (
			id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at, workspace
		) VALUES (?, ?, ?, ?, 'shell', '/bin/sh', 'stopped', 'test', ?, ?, ?)`,
			id, names[i], names[i], "/work/"+names[i], int64(i+1), int64(i+1), legacyColumnValues[i]); err != nil {
			t.Fatalf("seed fixture session %d: %v", i, err)
		}
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// tableColumnSet reads the current column names of table via PRAGMA
// table_info, so a test can assert a column's presence or absence
// without depending on ordinal position.
func tableColumnSet(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return cols
}

// TestSchemaV7MigratesV6LegacyColumnRowsToNullGroupID is task 008's (R128,
// part 1) required migration proof: a schemaV6 fixture holding three
// sessions with three DISTINCT sessions.workspace values migrates, in one
// commit's single schemaV7 step, to three rows whose group_id reads back
// NULL (never seeded from the old workspace value -- the standing-rules
// prohibition on "seeding groups from old workspace values"), an empty
// groups table (schemaV7 creates it; nothing populates it), and the same
// three sessions still listable by name after the file is reopened.
func TestSchemaV7MigratesV6LegacyColumnRowsToNullGroupID(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV6Fixture(t, home, path)

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v6 fixture for migration: %v", err)
	}
	defer st.Close()

	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}

	cols := tableColumnSet(t, st.DB(), "sessions")
	if cols["workspace"] {
		t.Fatal("sessions.workspace still present after migrating to schemaV7")
	}
	if !cols["group_id"] {
		t.Fatal("sessions.group_id missing after migrating to schemaV7")
	}

	for _, id := range ids {
		var groupID sql.NullInt64
		if err := st.DB().QueryRow(`SELECT group_id FROM sessions WHERE id = ?`, id).Scan(&groupID); err != nil {
			t.Fatalf("read migrated row %s group_id: %v", id, err)
		}
		if groupID.Valid {
			t.Fatalf("row %s group_id = %v, want NULL (never seeded from the old workspace value)", id, groupID.Int64)
		}
	}

	var groupsCount int
	if err := st.DB().QueryRow(`SELECT count(*) FROM groups`).Scan(&groupsCount); err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if groupsCount != 0 {
		t.Fatalf("groups table has %d rows after migration, want 0 (empty -- nothing seeds it from the old workspace values)", groupsCount)
	}

	sessions, err := st.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions after migration: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("len(sessions) after migration = %d, want 3", len(sessions))
	}
	gotNames := map[string]bool{}
	for _, s := range sessions {
		gotNames[s.Name] = true
		if s.GroupID != nil {
			t.Fatalf("session %s GroupID = %v, want nil", s.Name, *s.GroupID)
		}
		if s.GroupName != "" {
			t.Fatalf("session %s GroupName = %q, want empty", s.Name, s.GroupName)
		}
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !gotNames[want] {
			t.Fatalf("migrated sessions = %v, missing %q", gotNames, want)
		}
	}
}

// TestSchemaV7MigrationIsAtomicOnMidMigrationFailure is task 008's (R128,
// part 1) required atomicity proof: migrate() runs the whole schema bump
// -- schemaV7's three statements plus the meta.schema_version write --
// inside ONE transaction (store.go's migrate), so a failure on the LAST
// of three schemaV7 statements, after the first two already landed
// against the live *sql.DB handle inside that same uncommitted
// transaction, must roll back all of it: no groups table, no group_id
// column, sessions.workspace still present, meta.schema_version still 6,
// and the original three rows still present and readable both directly
// and (once the real ladder is restored) through a subsequent, successful
// migration of the very same file.
func TestSchemaV7MigrationIsAtomicOnMidMigrationFailure(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	buildSchemaV6Fixture(t, home, path)

	original := schemaV7
	schemaV7 = []string{
		original[0],                    // CREATE TABLE groups: succeeds inside the transaction
		original[1],                    // ALTER TABLE sessions ADD COLUMN group_id: succeeds inside the transaction
		"THIS IS NOT VALID SQL AT ALL", // fails, forcing migrate()'s deferred tx.Rollback()
	}
	defer func() { schemaV7 = original }()

	if _, err := OpenPath(home, path); err == nil {
		t.Fatal("OpenPath with a broken schemaV7 unexpectedly succeeded")
	}

	// Inspect the file with a brand-new *sql.DB handle, entirely outside
	// this package's own Store/migrate path, so this assertion cannot be
	// fooled by in-memory state the failed OpenPath call above might have
	// left behind.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}

	var version int
	if err := raw.QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("post-failure schema version = %d, %v; want 6 (the failed migration must not have been partially recorded)", version, err)
	}

	var groupsTables int
	if err := raw.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'groups'`).Scan(&groupsTables); err != nil {
		t.Fatal(err)
	}
	if groupsTables != 0 {
		t.Fatal("groups table exists after a rolled-back migration, want it entirely absent")
	}

	cols := tableColumnSet(t, raw, "sessions")
	if !cols["workspace"] {
		t.Fatal("sessions.workspace column gone after a rolled-back migration, want it still present")
	}
	if cols["group_id"] {
		t.Fatal("sessions.group_id column present after a rolled-back migration, want it entirely absent")
	}

	var sessionCount int
	if err := raw.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount != 3 {
		t.Fatalf("post-failure session count = %d, %v; want 3 (the schemaV6 database is intact)", sessionCount, err)
	}
	var legacyColumnNonNullCount int
	if err := raw.QueryRow(`SELECT count(*) FROM sessions WHERE workspace IS NOT NULL`).Scan(&legacyColumnNonNullCount); err != nil || legacyColumnNonNullCount != 3 {
		t.Fatalf("post-failure rows with a legacy column value = %d, %v; want 3 (readable, unchanged)", legacyColumnNonNullCount, err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// The real ladder, restored by the deferred func above running before
	// this point would be wrong -- restore it explicitly here so the
	// following OpenPath exercises the genuine schemaV7, proving the
	// failed attempt left nothing behind that would trip up a later,
	// correct migration of this exact file.
	schemaV7 = original
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("re-open after restoring the real schemaV7: %v", err)
	}
	defer st.Close()
	sessions, err := st.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions after the successful migration: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("len(sessions) after the successful migration = %d, want 3", len(sessions))
	}
}
