package tui

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 024's own unit evidence for the launch-inputs editor's
// contract (023/023-follow-up already wired a setter through the model --
// see launch_inputs_wiring_test.go -- this file exercises the DIALOG
// itself: typing, field navigation and in-dialog validation), each test
// wired to a REAL store.Store so "through the task 020 mutator" is
// store.Store.SetLaunchInputs itself, never a stand-in.

// newLaunchInputsTestStore opens a real, on-disk store.Store (mirroring
// dialog_contract_test.go's own store.OpenPath pattern) and seeds one
// session row with every launch input at its zero value, so a test can
// prove exactly what changed.
func newLaunchInputsTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000024"
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: "launch-024", CWD: "/work/launch-024", Agent: "shell", CapturedPath: "/bin",
		StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	return db, id
}

// launchInputsTestModel opens the editor (via the same "l" inside detail
// path detailView itself uses -- rename.go's case "l") on the store's one
// seeded session, wired to that store's OWN SetLaunchInputs (task 020)
// through WithLaunchInputsSetter exactly as cmd/deck/main.go wires it in
// the shipped binary.
func launchInputsTestModel(t *testing.T, db *store.Store, id string) Model {
	t.Helper()
	ctx := context.Background()
	session, err := db.GetSession(ctx, id)
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
	m.sessions = []store.Session{session}
	m.selected = 0
	got, _ := m.Update(key("i"))
	m = got.(Model)
	got, _ = m.Update(key("l"))
	m = got.(Model)
	if !m.launchInputsEditing {
		t.Fatal("\"l\" inside detail did not open the launch-inputs editor")
	}
	return m
}

// typeInto sends msg through Update for every rune of s, one keystroke at a
// time, mirroring how a live PTY would deliver typed text.
func typeInto(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		got, _ := m.Update(key(string(r)))
		m = got.(Model)
	}
	return m
}

// TestLaunchInputsEditorSubmitWritesExactlyWhatWasTypedThroughTheStoreMutator
// proves the editor's central contract: typing into all four fields (three
// free-text fields plus the login_shell toggle) and pressing Enter writes
// exactly those four values into the row, through store.Store.SetLaunchInputs
// (task 020) -- not a stand-in double -- and marks launch_dirty exactly as
// that mutator's own store-level test already pins.
func TestLaunchInputsEditorSubmitWritesExactlyWhatWasTypedThroughTheStoreMutator(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := launchInputsTestModel(t, db, id)

	// Field 0: Pre-launch command.
	m = typeInto(t, m, "echo pre-024")
	got, _ := m.Update(key("down"))
	m = got.(Model)
	// Field 1: Post-destroy command.
	m = typeInto(t, m, "echo post-024")
	got, _ = m.Update(key("down"))
	m = got.(Model)
	// Field 2: Launch args (JSON array text).
	m = typeInto(t, m, `["--flag","v024"]`)
	got, _ = m.Update(key("down"))
	m = got.(Model)
	// Field 3: Login shell -- space toggles the boolean, never types text.
	got, _ = m.Update(key(" "))
	m = got.(Model)
	if !m.launchInputsLoginShell {
		t.Fatal("space on the login_shell field did not toggle it on")
	}

	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd == nil {
		t.Fatal("enter did not dispatch a command")
	}
	msg := cmd()
	got, _ = m.Update(msg)
	m = got.(Model)
	if m.launchInputsEditing {
		t.Fatal("a successful submit left the editor open")
	}
	if m.launchInputsNote != "" {
		t.Fatalf("a successful submit left a note %q, want none", m.launchInputsNote)
	}

	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.PreLaunch != "echo pre-024" {
		t.Fatalf("PreLaunch = %q, want %q", row.PreLaunch, "echo pre-024")
	}
	if row.PostDestroy != "echo post-024" {
		t.Fatalf("PostDestroy = %q, want %q", row.PostDestroy, "echo post-024")
	}
	if want := []string{"--flag", "v024"}; !reflect.DeepEqual(row.LaunchArgs, want) {
		t.Fatalf("LaunchArgs = %#v, want %#v", row.LaunchArgs, want)
	}
	if !row.LoginShell {
		t.Fatal("LoginShell = false, want true")
	}
	if !row.LaunchDirty {
		t.Fatal("launch_dirty = false after a submit, want true")
	}
}

// TestLaunchInputsEditorEscWritesNothingAtAll proves esc discards every
// typed field without ever reaching the store mutator: the row stays at
// its pre-edit zero value and launch_dirty is never set.
func TestLaunchInputsEditorEscWritesNothingAtAll(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := launchInputsTestModel(t, db, id)

	m = typeInto(t, m, "echo pre-esc")
	got, _ := m.Update(key("down"))
	m = got.(Model)
	m = typeInto(t, m, "echo post-esc")
	got, _ = m.Update(key("down"))
	m = got.(Model)
	m = typeInto(t, m, `["esc"]`)
	got, _ = m.Update(key("down"))
	m = got.(Model)
	got, _ = m.Update(key(" "))
	m = got.(Model)
	if !m.launchInputsLoginShell {
		t.Fatal("space did not toggle login_shell before esc")
	}

	got, cmd := m.Update(key("esc"))
	m = got.(Model)
	if cmd != nil {
		t.Fatal("esc dispatched a command; it must change nothing")
	}
	if m.launchInputsEditing {
		t.Fatal("esc did not close the launch-inputs editor")
	}
	if !m.detail {
		t.Fatal("esc on the launch-inputs editor closed detail underneath it too")
	}

	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.PreLaunch != "" || row.PostDestroy != "" || len(row.LaunchArgs) != 0 || row.LoginShell {
		t.Fatalf("esc reached the store: %+v, want every launch input still at its zero value", row)
	}
	if row.LaunchDirty {
		t.Fatal("esc set launch_dirty; it must never write anything")
	}
}

// TestLaunchInputsEditorUpDownMoveBetweenFourFieldsTabDoesNot proves ↑/↓
// cycle m.launchInputsField through all four fields (wrapping both ways)
// while tab -- reserved package-wide for completion, SPEC §11.4 -- neither
// moves the field nor types anything.
func TestLaunchInputsEditorUpDownMoveBetweenFourFieldsTabDoesNot(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := launchInputsTestModel(t, db, id)

	if m.launchInputsField != launchInputsFieldPreLaunch {
		t.Fatalf("opening field = %d, want %d (pre_launch)", m.launchInputsField, launchInputsFieldPreLaunch)
	}

	wantDown := []int{
		launchInputsFieldPostDestroy,
		launchInputsFieldLaunchArgs,
		launchInputsFieldLoginShell,
		launchInputsFieldPreLaunch, // wraps
	}
	for i, want := range wantDown {
		got, _ := m.Update(key("down"))
		m = got.(Model)
		if m.launchInputsField != want {
			t.Fatalf("down #%d moved to field %d, want %d", i+1, m.launchInputsField, want)
		}
	}

	wantUp := []int{
		launchInputsFieldLoginShell, // wraps backward
		launchInputsFieldLaunchArgs,
		launchInputsFieldPostDestroy,
		launchInputsFieldPreLaunch,
	}
	for i, want := range wantUp {
		got, _ := m.Update(key("up"))
		m = got.(Model)
		if m.launchInputsField != want {
			t.Fatalf("up #%d moved to field %d, want %d", i+1, m.launchInputsField, want)
		}
	}

	before := m.launchInputsField
	got, _ := m.Update(key("tab"))
	m = got.(Model)
	if m.launchInputsField != before {
		t.Fatalf("tab moved the field from %d to %d; tab must never move between fields", before, m.launchInputsField)
	}
	if m.launchInputsPreLaunch != "" {
		t.Fatalf("tab typed into pre_launch: %q, want unchanged", m.launchInputsPreLaunch)
	}
}

// TestLaunchInputsEditorInvalidLaunchArgsReportsInDialogAndRetainsTyped
// proves the launch_args field's in-dialog validation (SPEC §11.4): a
// malformed JSON array is rejected before the store mutator is ever
// consulted, the dialog stays open, and the exact text the user typed is
// kept in the field for correction rather than reverted.
func TestLaunchInputsEditorInvalidLaunchArgsReportsInDialogAndRetainsTyped(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := launchInputsTestModel(t, db, id)

	got, _ := m.Update(key("down")) // pre_launch -> post_destroy
	m = got.(Model)
	got, _ = m.Update(key("down")) // post_destroy -> launch_args
	m = got.(Model)
	if m.launchInputsField != launchInputsFieldLaunchArgs {
		t.Fatalf("field = %d, want %d (launch_args)", m.launchInputsField, launchInputsFieldLaunchArgs)
	}

	const invalid = "not-json"
	m = typeInto(t, m, invalid)

	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd != nil {
		t.Fatal("enter with an invalid launch_args value dispatched a command; validation must happen before the setter is ever consulted")
	}
	if m.launchInputsNote == "" {
		t.Fatal("an invalid launch_args value produced no in-dialog note")
	}
	if !m.launchInputsEditing {
		t.Fatal("an invalid submit closed the editor; it must stay open for correction")
	}
	if m.launchInputsLaunchArgs != invalid {
		t.Fatalf("launchInputsLaunchArgs = %q, want the typed value %q retained", m.launchInputsLaunchArgs, invalid)
	}

	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.LaunchDirty || row.PreLaunch != "" || row.PostDestroy != "" || len(row.LaunchArgs) != 0 {
		t.Fatalf("an invalid submit reached the store: %+v", row)
	}
}
