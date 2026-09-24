package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestInteractiveFoldOwnGroupKeepsPreviewTargetName proves R138/SPEC
// §11.6 (SPEC.md:1706-1710): folding the group that contains the session
// interactive mode currently owns the keyboard for must not erase that
// session's name from the preview panel's own border. Before this fix,
// previewTitle read the target off m.selectedSession() (the CURSOR), and
// setGroupCollapsed deliberately re-targets a row cursor onto its own
// group's header the moment that group folds (its own doc: "a row
// cursor whose session belongs to the group being folded... lands on
// that same group's own header") -- so the very act SPEC requires to
// stay legal (folding the interactive session's group) silently blanked
// the name. This clicks the SESSION'S OWN header (group "alpha", which
// s1 itself belongs to), not some other, unrelated group's header, since
// only folding the OWNING group can hit the defect at all.
func TestInteractiveFoldOwnGroupKeepsPreviewTargetName(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A tmux binary that only logs its argv and refuses: any invocation
	// at all (geometry, ownership, fit, ...) is what "no geometry calls"
	// must rule out, so a call that returned success would hide the very
	// thing this test is checking for.
	calls := filepath.Join(home, "tmux.calls")
	binary := filepath.Join(home, "tmux")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+calls+"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}

	group, err := db.CreateGroup(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	windowTarget, err := tmux.SessionName("s1")
	if err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{Mouse: true}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s1", Name: "s1", Slug: "s1", Status: "running", GroupName: group.Name, GroupID: &group.ID}}
	m.interactive = true
	m.selected = rowCursor(0)
	m.interactiveWindowTarget = windowTarget
	m.interactiveGeometry = tmux.WindowGeometry{Width: 61, Height: 27}
	m.tmuxClient = tmux.Client{Socket: "review", Binary: binary}

	if before := m.previewTitle(); !strings.Contains(before, "s1") {
		t.Fatalf("test setup: preview title %q does not name the target session before the fold", before)
	}

	x, y := findHeader(t, m, "alpha")
	next, cmd := m.Update(press(x, y))
	got := next.(Model)

	if !got.selected.IsHeader() {
		t.Fatalf("test setup: header press left cursor %+v, want it re-targeted onto alpha's own header (setGroupCollapsed's documented behaviour) -- otherwise this never exercises the defect", got.selected)
	}
	if !got.isGroupCollapsed(group.ID) {
		t.Fatalf("header press did not collapse alpha")
	}

	title := got.previewTitle()
	if !strings.Contains(title, "s1") {
		t.Fatalf("folding the interactive session's own group dropped its name from the preview border: title=%q cursor=%+v", title, got.selected)
	}

	if !got.interactive {
		t.Fatalf("header fold on the interactive session's own group left interactive mode, want it to stay entered")
	}
	if got.interactiveWindowTarget != windowTarget {
		t.Fatalf("header fold changed interactiveWindowTarget: got %q, want unchanged %q", got.interactiveWindowTarget, windowTarget)
	}
	if got.interactiveGeometry != m.interactiveGeometry {
		t.Fatalf("header fold changed interactiveGeometry: got %+v, want unchanged %+v", got.interactiveGeometry, m.interactiveGeometry)
	}
	if got.interactiveGrid != m.interactiveGrid || got.interactiveDispatcher != m.interactiveDispatcher {
		t.Fatalf("header fold touched the transport (grid=%v dispatcher=%v), want both unchanged", got.interactiveGrid, got.interactiveDispatcher)
	}

	if cmd == nil {
		t.Fatal("header press on a group header returned a nil cmd, want persistCollapsedGroups' command")
	}
	if msg, ok := cmd().(uiStatePersisted); !ok || msg.err != nil {
		t.Fatalf("header press returned cmd = %+v, want a successful uiStatePersisted", msg)
	}

	if data, err := os.ReadFile(calls); err == nil {
		t.Fatalf("header fold invoked tmux (geometry must be untouched): %s", data)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
