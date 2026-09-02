package tui

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestLostAttachDialogSwallowsListKeysAndReleasesOnEnter is task 117's
// proof that the lost-attach dialog (task 116) really does intercept
// every key ahead of the list's own handling -- not just the two keys
// lost_attach_test.go already covers (an arbitrary "q" and \u21b5) -- by
// driving the exact keys SPEC \u00a711.9 worries about: `j` (selection
// movement), the `d d` delete chord and `x` (kill), each of which would
// otherwise mutate m.selected, open a confirm dialog or fire a real
// service call against a live, kill/delete-eligible row. It uses a real
// store.Store (not a stub) so "the store row is unchanged" is a genuine
// read-back, not an assertion about an in-memory struct nothing ever
// touches.
func TestLostAttachDialogSwallowsListKeysAndReleasesOnEnter(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "alpha", Name: "alpha", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "beta", Name: "beta", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}

	var killCalled, deleteCalled bool
	m := New(db, config.Settings{}, "")
	m.width, m.height = 80, 24
	updated, _ := m.Update(m.loadSessions())
	m = updated.(Model)
	if len(m.sessions) != 2 {
		t.Fatalf("want 2 loaded sessions, got %d", len(m.sessions))
	}
	m.selected = 0
	m.kill = func(context.Context, store.Session) error {
		killCalled = true
		return nil
	}
	m.deleteSvc = func(context.Context, store.Session) error {
		deleteCalled = true
		return nil
	}
	// alpha is running: canKill and canDelete both accept it, so if `x`
	// or `d d` ever reached the ordinary list handling below the dialog,
	// something here would fire.
	if !canKill(m.sessions[0]) || !canDelete(m.sessions[0]) {
		t.Fatal("fixture row must be eligible for both x and dd, or their non-firing proves nothing")
	}

	m.lostAttach = true
	m.lostAttachSession = "alpha"

	wantRow, err := db.GetSession(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	wantSelected := m.selected
	wantFrame := m.mainView()

	for _, k := range []string{"j", "d", "d", "x"} {
		got, cmd := m.Update(key(k))
		m = got.(Model)
		if cmd != nil {
			t.Fatalf("key %q while lost-attach dialog is up returned a non-nil cmd", k)
		}
		if !m.lostAttach {
			t.Fatalf("key %q closed the lost-attach dialog; only enter should", k)
		}
	}

	if m.selected != wantSelected {
		t.Fatalf("selection index moved: got %d, want %d", m.selected, wantSelected)
	}
	if m.deleteConfirming {
		t.Fatal("d d opened the delete-confirm dialog through the lost-attach dialog")
	}
	if killCalled {
		t.Fatal("x fired the kill service call through the lost-attach dialog")
	}
	if deleteCalled {
		t.Fatal("d d fired the delete service call through the lost-attach dialog")
	}
	if got := m.mainView(); got != wantFrame {
		t.Fatalf("session list frame changed while the lost-attach dialog swallowed keys:\ngot:\n%s\nwant:\n%s", got, wantFrame)
	}
	gotRow, err := db.GetSession(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRow, wantRow) {
		t.Fatalf("store row changed while the lost-attach dialog swallowed keys: got %#v, want %#v", gotRow, wantRow)
	}

	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd != nil {
		t.Fatal("enter on the lost-attach dialog returned a non-nil cmd")
	}
	if m.lostAttach {
		t.Fatal("enter did not dismiss the lost-attach dialog")
	}

	// The list must be interactive again: an ordinary `j` now actually
	// moves the selection, proving it is the list handling the key again
	// and not some other overlay still swallowing it.
	got, _ = m.Update(key("j"))
	m = got.(Model)
	if m.selected == wantSelected {
		t.Fatal("j did not move the selection after the lost-attach dialog closed; the list is not interactive again")
	}
	if killCalled || deleteCalled {
		t.Fatal("a service call fired that the swallow assertions above should have caught")
	}
}
