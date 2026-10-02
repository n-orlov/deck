package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// This file is task 017's (R179, SPEC §11.7) contract for the create modal's
// cwd field under a caret: the ghost and tab completion exist only with the
// caret at the end of the text, right and end accept a ghost only when one is
// showing, ctrl+p and ctrl+n replace the whole text, and an offered prefill is
// edited in place. Real-tmux twins are features/create_cwd_caret.feature.

// createCWDCaretModel is a create modal on the cwd field whose text is typed
// as dir+"/uni", with one unique subdirectory "unique-directory" to complete to.
func createCWDCaretModel(t *testing.T) (Model, string) {
	t.Helper()
	parent := createEditShortDir(t)
	if err := os.Mkdir(filepath.Join(parent, "unique-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m = createEditType(t, m, filepath.Join(parent, "uni"))
	return m, parent
}

func TestCreateCWDNoGhostAndTabDoesNothingWithTheCaretNotAtTheEnd(t *testing.T) {
	m, parent := createCWDCaretModel(t)
	typed := filepath.Join(parent, "uni")
	if m.createCWDGhostSuffix() == "" {
		t.Fatal("precondition: with the caret at the end a ghost must be showing")
	}
	for _, move := range []string{"left", "home"} {
		away := createEditPress(t, m, move)
		if got := away.createCWDGhostSuffix(); got != "" {
			t.Fatalf("after %s the caret is mid-field yet the ghost is %q", move, got)
		}
		if got := createEditRow(t, away, "Working directory"); strings.Contains(got, "que-directory") {
			t.Fatalf("after %s the drawn field shows a ghost: %q", move, got)
		}
		tabbed := createEditPress(t, away, "tab")
		if tabbed.createText(createFieldCWD) != typed || tabbed.createField != createFieldCWD {
			t.Fatalf("tab with the caret mid-field after %s gave %q (focus %d), want the text unchanged on the cwd field", move, tabbed.createText(createFieldCWD), tabbed.createField)
		}
		if len(tabbed.createCWDCandidates) != 0 {
			t.Fatalf("tab with the caret mid-field opened a candidate list: %v", tabbed.createCWDCandidates)
		}
	}
}

func TestCreateCWDRightAndEndAcceptOnlyAShowingGhostOtherwiseMoveTheCaret(t *testing.T) {
	m, parent := createCWDCaretModel(t)
	typed := filepath.Join(parent, "uni")
	full := filepath.Join(parent, "unique-directory") + "/"

	// A ghost is showing at the end: right and end both accept it.
	for _, k := range []string{"right", "end"} {
		got := createEditPress(t, m, k)
		if got.createText(createFieldCWD) != full {
			t.Fatalf("%s with a ghost showing gave %q, want %q", k, got.createText(createFieldCWD), full)
		}
	}

	// No ghost (caret mid-field): right and end are caret keys and change no text.
	mid := createEditPress(t, m, "home")
	right := createEditPress(t, mid, "right")
	if right.createText(createFieldCWD) != typed || right.createEdits[createFieldCWD].Caret() != 1 {
		t.Fatalf("right mid-field gave %q caret %d, want the text unchanged and the caret at 1", right.createText(createFieldCWD), right.createEdits[createFieldCWD].Caret())
	}
	end := createEditPress(t, mid, "end")
	if end.createText(createFieldCWD) != typed || end.createEdits[createFieldCWD].Caret() != len(typed) {
		t.Fatalf("end mid-field gave %q caret %d, want the text unchanged and the caret at its end", end.createText(createFieldCWD), end.createEdits[createFieldCWD].Caret())
	}
	if end.createCWDGhostSuffix() == "" {
		t.Fatal("end moved the caret back to the end of the text, so the ghost must return")
	}

	// No ghost at all (nothing matches): right at the end is a caret key and changes nothing.
	none := createEditType(t, createEditOpen(t, t.TempDir()), "")
	none.createField = createFieldCWD
	none = createEditPress(t, none, "end")
	none = createEditType(t, none, "/no-such-directory-here")
	before := none.createText(createFieldCWD)
	for _, k := range []string{"right", "end"} {
		got := createEditPress(t, none, k)
		if got.createText(createFieldCWD) != before || got.createEdits[createFieldCWD].Caret() != len(before) {
			t.Fatalf("%s with no ghost gave %q caret %d, want the text unchanged", k, got.createText(createFieldCWD), got.createEdits[createFieldCWD].Caret())
		}
	}
}

func TestCreateCWDCtrlPCtrlNReplaceTheWholeTextAndRestoreTheSnapshotCaretIncluded(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	for _, p := range []string{"/recent/one", "/recent/two"} {
		if err := db.PromoteRecentCwd(ctx, p, 5); err != nil {
			t.Fatal(err)
		}
	}
	m := New(db, config.Settings{Socket: "test-socket"}, "")
	m.width, m.height = 120, 40
	m.creating = true
	m.createField = createFieldCWD
	m.setCreateText(createFieldCWD, "/typed/value")
	// Put the caret in the middle of the typed text: the snapshot must keep it.
	m = createEditPress(t, m, "left", "left", "left")
	if c := m.createEdits[createFieldCWD].Caret(); c != len("/typed/value")-3 {
		t.Fatalf("precondition: caret %d", c)
	}

	m = createEditPress(t, m, "ctrl+p")
	if e := m.createEdits[createFieldCWD]; e.Value() != "/recent/two" || e.Caret() != len("/recent/two") {
		t.Fatalf("ctrl+p gave %q caret %d, want the whole text replaced with the caret at its end", e.Value(), e.Caret())
	}
	m = createEditPress(t, m, "left", "ctrl+p")
	if e := m.createEdits[createFieldCWD]; e.Value() != "/recent/one" || e.Caret() != len("/recent/one") {
		t.Fatalf("second ctrl+p gave %q caret %d, want the older entry with the caret at its end", e.Value(), e.Caret())
	}
	m = createEditPress(t, m, "left", "ctrl+n")
	if e := m.createEdits[createFieldCWD]; e.Value() != "/recent/two" || e.Caret() != len("/recent/two") {
		t.Fatalf("ctrl+n gave %q caret %d, want the newer entry with the caret at its end", e.Value(), e.Caret())
	}
	m = createEditPress(t, m, "ctrl+n")
	if e := m.createEdits[createFieldCWD]; e.Value() != "/typed/value" || e.Caret() != len("/typed/value")-3 {
		t.Fatalf("ctrl+n past the newest entry gave %q caret %d, want the snapshot back with its caret at %d", e.Value(), e.Caret(), len("/typed/value")-3)
	}
}

func TestCreateCWDCycleRestoresAnOfferedPrefillSnapshot(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.PromoteRecentCwd(context.Background(), "/recent/one", 5); err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{Socket: "test-socket"}, "")
	m.width, m.height = 120, 40
	m.creating = true
	m.createField = createFieldCWD
	m.createEdits[createFieldCWD] = lineedit.NewOffered("/home/me/proj-a")
	m = createEditPress(t, m, "ctrl+p", "ctrl+n")
	if e := m.createEdits[createFieldCWD]; e.Value() != "/home/me/proj-a" || !e.Offered() {
		t.Fatalf("cycling back restored %q offered=%v, want the offered prefill", e.Value(), e.Offered())
	}
}

func TestCreateCWDLeftIntoAnOfferedPrefillAcceptsItAndEditsInPlace(t *testing.T) {
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m.createEdits[createFieldCWD] = lineedit.NewOffered("/home/me/proj-a")

	// left steps into the offered value: it is kept, accepted, caret before the
	// "a" (§11.11's offered rule, one rule for every field: left accepts and
	// moves). R179's literal "left, backspace, b" example would need left to
	// accept without moving; that clause contradicts R177's "left accepts and
	// moves" and was petitioned and adjudicated CONFIRMED (a disclosed
	// residual), so the editor keeps the one offered rule for every field.
	stepped := createEditPress(t, m, "left")
	if e := stepped.createEdits[createFieldCWD]; e.Value() != "/home/me/proj-a" || e.Offered() || e.Caret() != len("/home/me/proj-") {
		t.Fatalf("left into the offered prefill gave %q offered=%v caret %d, want it kept, accepted, caret before the last character", e.Value(), e.Offered(), e.Caret())
	}
	// Replacing the last character in place (right to its end, backspace, b)
	// edits the path rather than retyping it.
	edited := createEditPress(t, stepped, "right", "backspace", "b")
	if got := edited.createText(createFieldCWD); got != "/home/me/proj-b" {
		t.Fatalf("editing the last character of the accepted prefill gave %q, want /home/me/proj-b", got)
	}
	// backspace straight from the offered value deletes its last character, then b follows.
	direct := createEditPress(t, m, "backspace", "b")
	if got := direct.createText(createFieldCWD); got != "/home/me/proj-b" {
		t.Fatalf("backspace then b on the offered prefill gave %q, want /home/me/proj-b", got)
	}
}
