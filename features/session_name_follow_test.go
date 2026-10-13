package features

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestSessionLookupFollowsARowDeckRenamedAfterTheStepSawIt covers the lookup
// behind every "fake <agent> session <name> ..." step. Deck adopts a
// harness-written title on its own tick, so a step can name a row by the name
// it had a moment earlier; the dispatch lane (-race, slow deck) failed
// harness_names.feature:32 and :78 with `observe session ... slug: sql: no
// rows` for exactly that. A name the harness has seen follows the row id to
// its current name in one statement, and a row that carries the name now wins
// over a stale id.
func TestSessionLookupFollowsARowDeckRenamedAfterTheStepSawIt(t *testing.T) {
	h := &ScenarioHarness{Home: t.TempDir()}
	db, err := sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, name TEXT, slug TEXT, conversation_id TEXT)`,
		`INSERT INTO sessions VALUES ('id-1', 'derived-name', 'derived-slug', 'conv-1')`,
		`INSERT INTO sessions VALUES ('id-2', 'other', 'other-slug', 'conv-2')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	h.trackSessionNames = true
	h.snapshotSessionNames()

	// Deck adopts the generated title between two steps.
	if _, err := db.Exec(`UPDATE sessions SET name = 'Generated title' WHERE id = 'id-1'`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"derived-name", "Generated title"} {
		if slug, err := sessionSlugByName(h, name); err != nil || slug != "derived-slug" {
			t.Fatalf("sessionSlugByName(%q) = %q, %v; want derived-slug, nil", name, slug, err)
		}
		if id, err := sessionIDByName(h, name); err != nil || id != "id-1" {
			t.Fatalf("sessionIDByName(%q) = %q, %v; want id-1, nil", name, id, err)
		}
		if conv, err := sessionConversationID(h, name); err != nil || conv != "conv-1" {
			t.Fatalf("sessionConversationID(%q) = %q, %v; want conv-1, nil", name, conv, err)
		}
	}

	// A new row that takes the old name now owns it: the stale id must not win.
	if _, err := db.Exec(`INSERT INTO sessions VALUES ('id-3', 'derived-name', 'new-slug', 'conv-3')`); err != nil {
		t.Fatal(err)
	}
	if slug, err := sessionSlugByName(h, "derived-name"); err != nil || slug != "new-slug" {
		t.Fatalf("sessionSlugByName(derived-name) = %q, %v; want the row that carries the name now (new-slug)", slug, err)
	}

	// A name nobody ever had is still an error, not a guess.
	if _, err := sessionSlugByName(h, "never existed"); err == nil {
		t.Fatal("sessionSlugByName(never existed) succeeded, want no rows")
	}
}
