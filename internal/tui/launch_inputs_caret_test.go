package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 012's (R178, SPEC §11.4/§11.11) contract for the
// launch-inputs editor's three text fields: they are edited by the shared
// line editor, so on a focused text field left, right and space are the
// editor's (the dialog contract's Cycle is for the login_shell selection
// only) and the opening value is an offered value (a printable key replaces
// it; a caret key accepts it and edits in place). The tests drive only
// Update, the store row and the exported seams, so they also compile against
// the pre-phase tree, where they fail.

const launchInputsCaretSessionID = "00000000-0000-4000-8000-000000000012"

// launchInputsSeededModel opens the editor on a session whose three text
// inputs hold pre/post/args, wired to the real store mutator, and returns the
// store so the test can read back exactly what a submit wrote.
func launchInputsSeededModel(t *testing.T, pre, post, args string) (Model, *store.Store) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: launchInputsCaretSessionID, Name: "launch-012", CWD: "/work/launch-012", Agent: "shell", CapturedPath: "/bin",
		StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	var launchArgs []string
	if args != "" {
		launchArgs = []string{args}
	}
	if err := db.SetLaunchInputs(ctx, launchInputsCaretSessionID, pre, post, launchArgs, false, "user", 5); err != nil {
		t.Fatal(err)
	}
	session, err := db.GetSession(ctx, launchInputsCaretSessionID)
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "").WithLaunchInputsSetter(
		func(ctx context.Context, sessionID, preLaunch, postDestroy string, launchArgs []string, loginShell bool) (store.Session, error) {
			if err := db.SetLaunchInputs(ctx, sessionID, preLaunch, postDestroy, launchArgs, loginShell, "user", 10); err != nil {
				return store.Session{}, err
			}
			return db.GetSession(ctx, sessionID)
		},
	)
	m.width, m.height = 100, 40
	m.sessions = []store.Session{session}
	m.selected = rowCursor(0)
	for _, k := range []string{"i", "l"} {
		got, _ := m.Update(key(k))
		m = got.(Model)
	}
	if !m.launchInputsEditing {
		t.Fatal("\"l\" inside detail did not open the launch-inputs editor")
	}
	return m, db
}

func sendKeys(m Model, keys ...string) Model {
	for _, k := range keys {
		got, _ := m.Update(key(k))
		m = got.(Model)
	}
	return m
}

// submitLaunchInputsRow presses Enter, runs the resulting command and reads
// the row the store now holds.
func submitLaunchInputsRow(t *testing.T, m Model, db *store.Store) store.Session {
	t.Helper()
	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd == nil {
		t.Fatalf("enter dispatched no command (note %q)", m.launchInputsNote)
	}
	m.Update(cmd())
	row, err := db.GetSession(context.Background(), launchInputsCaretSessionID)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

// TestLaunchInputsLeftMovesTheCaretOnATextField is R178's launch-inputs case:
// left on a focused text field reaches the editor, so a character typed next
// lands inside the value rather than at its end.
func TestLaunchInputsLeftMovesTheCaretOnATextField(t *testing.T) {
	m, db := launchInputsSeededModel(t, "abc", "", "")
	m = sendKeys(m, "left", "X")
	if row := submitLaunchInputsRow(t, m, db); row.PreLaunch != "abXc" {
		t.Fatalf("PreLaunch = %q, want %q (left accepted the offer and moved the caret before the c)", row.PreLaunch, "abXc")
	}
}

// TestLaunchInputsRightMovesTheCaretOnATextField: right is the editor's too.
func TestLaunchInputsRightMovesTheCaretOnATextField(t *testing.T) {
	m, db := launchInputsSeededModel(t, "abc", "", "")
	m = sendKeys(m, "left", "left", "left", "right", "X")
	if row := submitLaunchInputsRow(t, m, db); row.PreLaunch != "aXbc" {
		t.Fatalf("PreLaunch = %q, want %q", row.PreLaunch, "aXbc")
	}
}

// TestLaunchInputsOpeningValueIsOfferedAndReplacedByAPrintableKey rewrites the
// old append-to-prefill expectation to the §11.11 offered rule: the first
// printable key replaces the stored value, it is not appended to it.
func TestLaunchInputsOpeningValueIsOfferedAndReplacedByAPrintableKey(t *testing.T) {
	m, db := launchInputsSeededModel(t, "echo old", "echo gone", "")
	m = sendKeys(m, "n", "e", "w")
	m = sendKeys(m, "down", "backspace") // backspace accepts the offer and edits it in place
	row := submitLaunchInputsRow(t, m, db)
	if row.PreLaunch != "new" {
		t.Fatalf("PreLaunch = %q, want %q (a printable key replaces the offered value)", row.PreLaunch, "new")
	}
	if row.PostDestroy != "echo gon" {
		t.Fatalf("PostDestroy = %q, want %q (backspace accepts the offer and deletes one rune)", row.PostDestroy, "echo gon")
	}
}

// TestLaunchInputsSpaceOnATextFieldIsTypedAndOnLoginShellToggles: space is the
// editor's on a text field and the selection's on login_shell.
func TestLaunchInputsSpaceOnATextFieldIsTypedAndOnLoginShellToggles(t *testing.T) {
	m, db := launchInputsSeededModel(t, "", "", "")
	m = sendKeys(m, "a", " ", "b")
	if m.launchInputsLoginShell {
		t.Fatal("space on a text field toggled login_shell")
	}
	m = sendKeys(m, "down", "down", "down")
	if m.launchInputsField != launchInputsFieldLoginShell {
		t.Fatalf("field = %d, want login_shell", m.launchInputsField)
	}
	m = sendKeys(m, " ")
	if !m.launchInputsLoginShell {
		t.Fatal("space on login_shell did not toggle it")
	}
	m = sendKeys(m, "left")
	if m.launchInputsLoginShell {
		t.Fatal("left on login_shell did not toggle it back")
	}
	m = sendKeys(m, "right")
	if row := submitLaunchInputsRow(t, m, db); row.PreLaunch != "a b" || !row.LoginShell {
		t.Fatalf("row = pre %q login_shell %v, want %q and true", row.PreLaunch, row.LoginShell, "a b")
	}
}
