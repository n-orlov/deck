package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// buildSchemaV8PopulatedFixture is buildSchemaV7PopulatedFixture advanced one
// rung: the same four rows, migrated by schemaV8 and stamped schema version 8.
func buildSchemaV8PopulatedFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV7PopulatedFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV8 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV8 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 8 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

// TestSchemaV9MigratesPopulatedV8Database is R204c's migration proof: a
// schema 8 database with four real-looking rows migrates, via schemaV9's one
// ALTER TABLE statement, to schema 9 with sessions.hook_executable present,
// every row reading back "no executable recorded" and every other column
// byte-identical to what it held before.
func TestSchemaV9MigratesPopulatedV8Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV8PopulatedFixture(t, home, path)
	columnsV8 := append(append([]string{}, sessionColumnsPreV8...), "pinned_at")

	before, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot := snapshotSessionColumns(t, before, columnsV8)
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v8 populated fixture for migration: %v", err)
	}
	defer st.Close()

	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	if SchemaVersion != 9 {
		t.Fatalf("SchemaVersion = %d, want 9", SchemaVersion)
	}
	var columns int
	if err := st.DB().QueryRow(`SELECT count(*) FROM pragma_table_info('sessions') WHERE name = 'hook_executable'`).Scan(&columns); err != nil || columns != 1 {
		t.Fatalf("sessions.hook_executable columns = %d, %v; want 1 after migrating to schemaV9", columns, err)
	}

	after := snapshotSessionColumns(t, st.DB(), columnsV8)
	for _, id := range ids {
		for i, column := range columnsV8 {
			if beforeSnapshot[id][i] != after[id][i] {
				t.Fatalf("row %s column %s changed across migration: before %+v, after %+v", id, column, beforeSnapshot[id][i], after[id][i])
			}
		}
		var recorded sql.NullString
		if err := st.DB().QueryRow(`SELECT hook_executable FROM sessions WHERE id = ?`, id).Scan(&recorded); err != nil || recorded.Valid {
			t.Fatalf("row %s hook_executable = %+v, %v; want NULL", id, recorded, err)
		}
	}

	sessions, err := st.ListSessionsIncludingArchived(context.Background())
	if err != nil || len(sessions) != 3 {
		t.Fatalf("list after migration: %d sessions, %v; want 3", len(sessions), err)
	}
	for _, s := range sessions {
		if s.HookExecutable != "" {
			t.Fatalf("session %s HookExecutable = %q, want empty", s.Name, s.HookExecutable)
		}
	}
}

// TestSchemaV9ReopenKeepsARecordedHookExecutable: a reopen of a schema 9
// database is a no-op migration, and a recorded executable survives it.
func TestSchemaV9ReopenKeepsARecordedHookExecutable(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV8PopulatedFixture(t, home, path)

	first, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetHookExecutable(context.Background(), ids[0], "/opt/deck/bin/deck"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.GetSession(context.Background(), ids[0])
	if err != nil || got.HookExecutable != "/opt/deck/bin/deck" {
		t.Fatalf("HookExecutable after reopen = %q, %v; want the recorded path", got.HookExecutable, err)
	}
}

// TestSetHookExecutableIsAPlainColumnWrite: recording a binding changes only
// that column: no status change, no event.
func TestSetHookExecutableIsAPlainColumnWrite(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	session, err := st.CreateSession(context.Background(), CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000d4", Name: "delta", CWD: "/x", Agent: "claude", CapturedPath: "/bin",
		StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var eventsBefore int
	if err := st.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if err := st.SetHookExecutable(context.Background(), session.ID, "/x/deck"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(context.Background(), session.ID)
	if err != nil || got.HookExecutable != "/x/deck" || got.Status != session.Status || got.StatusReason != session.StatusReason {
		t.Fatalf("after SetHookExecutable: %+v, %v", got, err)
	}
	var eventsAfter int
	if err := st.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsAfter); err != nil || eventsAfter != eventsBefore {
		t.Fatalf("events %d -> %d, %v; want no event", eventsBefore, eventsAfter, err)
	}
	if err := st.SetHookExecutable(context.Background(), "absent", "/x/deck"); err != nil {
		t.Fatalf("an absent id must not be an error: %v", err)
	}
}
