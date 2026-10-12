package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func adoptedTestRow(t *testing.T, st *Store, id, source string) {
	t.Helper()
	if _, err := st.CreateSession(context.Background(), CreateSessionInput{
		ID: id, Name: "n-" + id, CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1, NameSource: source,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestNameAdoptedMigrationDefaultsEveryExistingRowToNotAdopted: a schema 11
// database migrates so each row reads name_adopted 0 (its name was never the
// harness's as far as deck knows).
func TestNameAdoptedMigrationDefaultsEveryExistingRowToNotAdopted(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV11PopulatedFixture(t, home, path)
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var bad int
	if err := st.DB().QueryRow(`SELECT COALESCE(SUM(name_adopted != 0), 0) FROM sessions`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("rows with name_adopted != 0: %d, %v", bad, err)
	}
	var adopted int
	if err := st.DB().QueryRow(`SELECT name_adopted FROM sessions WHERE id = ?`, ids[0]).Scan(&adopted); err != nil || adopted != 0 {
		t.Fatalf("migrated row %s name_adopted = %d, %v; want 0", ids[0], adopted, err)
	}
}

// TestNameAdoptedFollowsTheNameOrigin: every adoption (generated or explicit,
// renamed or frozen in place) marks the row adopted; a rename typed in deck
// clears the mark.
func TestNameAdoptedFollowsTheNameOrigin(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, tc := range []struct {
		name  string
		act   func(id string) error
		want  bool
		state string
	}{
		{"generated adoption", func(id string) error {
			_, err := st.AdoptSessionName(ctx, id, "Adopted A", "harness:claude", "harness", 5)
			return err
		}, true, "harness"},
		{"explicit adoption", func(id string) error {
			_, err := st.AdoptSessionName(ctx, id, "Adopted B", "harness:claude", "user", 5)
			return err
		}, true, "user"},
		{"freeze in place", func(id string) error { _, err := st.FreezeSessionName(ctx, id); return err }, true, "user"},
		{"typed rename after adoption", func(id string) error {
			if _, err := st.AdoptSessionName(ctx, id, "Adopted D", "harness:claude", "user", 5); err != nil {
				return err
			}
			return st.RenameSession(ctx, id, "typed D", "user", 6)
		}, false, "user"},
		{"fresh row", func(string) error { return nil }, false, "derived"},
	} {
		id := "r-" + strings.ReplaceAll(tc.name, " ", "-")
		adoptedTestRow(t, st, id, "derived")
		if err := tc.act(id); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, err := st.GetSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.NameAdopted != tc.want || got.NameSource != tc.state {
			t.Errorf("%s: adopted=%v source=%q, want %v %q", tc.name, got.NameAdopted, got.NameSource, tc.want, tc.state)
		}
	}
}

// buildSchemaV12AdoptionFixture is a schema 12 database whose four rows carry
// the name histories schema 12 could record: an explicit harness title adopted
// as 'user', a generated harness title, an explicit harness title frozen in
// place after a generated adoption, and a harness title later replaced by a
// name typed in deck. It returns the ids in that order.
func buildSchemaV12AdoptionFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV11PopulatedFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	statements := append([]string{}, schemaV12...)
	statements = append(statements, `UPDATE meta SET version = 12 WHERE key = 'schema_version'`)
	rows := []struct{ name, source string }{
		{"external-origin-claude $(printf EXTERNAL)", "user"},
		{"Generated pi title", "harness"},
		{"Chosen inside codex", "user"},
		{"typed in deck", "user"},
	}
	for i, row := range rows {
		statements = append(statements, fmt.Sprintf(`UPDATE sessions SET name = '%s', name_source = '%s' WHERE id = '%s'`,
			strings.ReplaceAll(row.name, "'", "''"), row.source, ids[i]))
	}
	events := []struct {
		id           int
		reason, name string
	}{
		{0, "harness:claude", rows[0].name},
		{1, "harness:pi", rows[1].name},
		{2, "harness:codex", rows[2].name},
		{3, "harness:claude", "Old harness title"},
		{3, "user", rows[3].name},
	}
	for i, ev := range events {
		statements = append(statements, fmt.Sprintf(`INSERT INTO events (session_id, at, kind, reason, payload) VALUES ('%s', %d, 'renamed', '%s', '%s')`,
			ids[ev.id], 1000+i, ev.reason, strings.ReplaceAll(ev.name, "'", "''")))
	}
	for _, statement := range statements {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schema 12 adoption fixture: %v (%s)", err, statement)
		}
	}
	return ids
}

// TestNameAdoptedMigrationKeepsSchema12HarnessOrigin: upgrading a schema 12
// database keeps every harness-adopted name marked adopted (explicit titles
// adopted or frozen as 'user' included), so none of them reaches launch or
// resume argv after the upgrade; a name typed in deck after an adoption stays
// a typed name.
func TestNameAdoptedMigrationKeepsSchema12HarnessOrigin(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV12AdoptionFixture(t, home, path)
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open schema 12 fixture: %v", err)
	}
	defer st.Close()
	for i, want := range []struct {
		adopted bool
		source  string
	}{{true, "user"}, {true, "harness"}, {true, "user"}, {false, "user"}} {
		got, err := st.GetSession(context.Background(), ids[i])
		if err != nil {
			t.Fatal(err)
		}
		if got.NameAdopted != want.adopted || got.NameSource != want.source {
			t.Errorf("row %d (%q): adopted=%v source=%q, want %v %q", i, got.Name, got.NameAdopted, got.NameSource, want.adopted, want.source)
		}
	}
}
