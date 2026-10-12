package store

import (
	"context"
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
