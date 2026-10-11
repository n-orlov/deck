package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// buildSchemaV11PopulatedFixture advances the v10 fixture one rung: migrated
// by schemaV11 and stamped schema version 11, rows kept.
func buildSchemaV11PopulatedFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV10PopulatedFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV11 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV11 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 11 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

// TestNameSourceMigrationDefaultsEveryExistingRowToUser: a schema 11 database
// holding rows migrates so each row reads name_source 'user' (a name nothing
// may overwrite) and NULL in pending_harness_name, its set-at timestamp and
// the adoption override. The fixture has no name_source column at all, so the
// test fails against the v11 schema.
func TestNameSourceMigrationDefaultsEveryExistingRowToUser(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	buildSchemaV11PopulatedFixture(t, home, path)
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`SELECT name_source FROM sessions`); err == nil {
		t.Fatal("the v11 fixture already has sessions.name_source")
	}
	_ = pre.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v11 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion < 12 {
		t.Fatalf("SchemaVersion = %d, want at least 12", SchemaVersion)
	}
	rows, err := st.DB().Query(`SELECT name_source, pending_harness_name, pending_harness_name_at, harness_name_adopt FROM sessions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var source string
		var pending sql.NullString
		var pendingAt, adopt sql.NullInt64
		if err := rows.Scan(&source, &pending, &pendingAt, &adopt); err != nil {
			t.Fatal(err)
		}
		if source != "user" || pending.Valid || pendingAt.Valid || adopt.Valid {
			t.Fatalf("migrated row = %q %v %v %v, want user and three NULLs", source, pending, pendingAt, adopt)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("sessions after migration = %d, want 4", n)
	}
}

// TestNameSourceColumnEnforcesItsValues: name_source accepts exactly
// user/derived/harness and the adoption override only 0, 1 or NULL.
func TestNameSourceColumnEnforcesItsValues(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := mustCreateRenameSession(t, st, "alpha")
	for _, ok := range []string{"user", "derived", "harness"} {
		if _, err := st.DB().Exec(`UPDATE sessions SET name_source = ? WHERE id = ?`, ok, s); err != nil {
			t.Fatalf("name_source %q refused: %v", ok, err)
		}
	}
	if _, err := st.DB().Exec(`UPDATE sessions SET name_source = 'bogus' WHERE id = ?`, s); err == nil {
		t.Fatal("name_source 'bogus' accepted")
	}
	if _, err := st.DB().Exec(`UPDATE sessions SET harness_name_adopt = 2 WHERE id = ?`, s); err == nil {
		t.Fatal("harness_name_adopt 2 accepted")
	}
	for _, v := range []any{0, 1, nil} {
		if _, err := st.DB().Exec(`UPDATE sessions SET harness_name_adopt = ? WHERE id = ?`, v, s); err != nil {
			t.Fatalf("harness_name_adopt %v refused: %v", v, err)
		}
	}
}

// TestNameSourceFollowsTheRenameSource: RenameSession writes name_source
// beside the name and still records the renamed event with its source.
func TestNameSourceFollowsTheRenameSource(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id := mustCreateRenameSession(t, st, "alpha")
	for i, c := range []struct{ source, want string }{
		{"harness", "harness"}, {"derived", "derived"}, {"user", "user"}, {"", "user"}, {"tui", "user"},
	} {
		name := "name-" + strings.Repeat("x", i+1)
		if err := st.RenameSession(context.Background(), id, name, c.source, int64(100+i)); err != nil {
			t.Fatal(err)
		}
		var got, reason, payload string
		if err := st.DB().QueryRow(`SELECT name_source FROM sessions WHERE id = ?`, id).Scan(&got); err != nil || got != c.want {
			t.Fatalf("source %q: name_source = %q, %v; want %q", c.source, got, err, c.want)
		}
		if err := st.DB().QueryRow(`SELECT reason, payload FROM events WHERE session_id = ? AND kind = 'renamed' ORDER BY seq DESC LIMIT 1`, id).Scan(&reason, &payload); err != nil {
			t.Fatal(err)
		}
		wantReason := c.source
		if wantReason == "" {
			wantReason = "user"
		}
		if reason != wantReason || payload != name {
			t.Fatalf("source %q: renamed event = %q %q, want %q %q", c.source, reason, payload, wantReason, name)
		}
	}
}

func mustCreateRenameSession(t *testing.T, st *Store, name string) string {
	t.Helper()
	const id = "00000000-0000-4000-8000-0000000000c3"
	if _, err := st.CreateSession(context.Background(), CreateSessionInput{
		ID: id, Name: name, CWD: "/x", Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
