package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestApplyDialogContractCoreKeys proves the ONE shared implementation
// (dialog_contract.go) performs SPEC §11.4's base vocabulary — esc cancels,
// enter submits, ↑/↓ move fields, left/right/space change a selection, and
// tab/shift+tab are unbound (reserved for completion) — directly,
// independent of any of the five dialogs that defer to it.
func TestApplyDialogContractCoreKeys(t *testing.T) {
	newContract := func(cancelled, submitted *bool, index *int, value *string) dialogContract {
		options := []string{"a", "b", "c"}
		return dialogContract{
			Fields: dialogFields{
				Count: 3,
				Index: index,
				Cycle: func(delta int) { *value = cycleOption(options, *value, delta) },
			},
			Cancel: func() { *cancelled = true },
			Submit: func() tea.Cmd { *submitted = true; return nil },
		}
	}

	t.Run("esc cancels and does not submit or cycle", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		_, handled := applyDialogContract(key("esc"), newContract(&cancelled, &submitted, &index, &value))
		if !handled || !cancelled || submitted || value != "a" {
			t.Fatalf("esc: handled=%v cancelled=%v submitted=%v value=%q", handled, cancelled, submitted, value)
		}
	})

	t.Run("enter submits and does not cancel", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		_, handled := applyDialogContract(key("enter"), newContract(&cancelled, &submitted, &index, &value))
		if !handled || cancelled || !submitted {
			t.Fatalf("enter: handled=%v cancelled=%v submitted=%v", handled, cancelled, submitted)
		}
	})

	t.Run("up and down move the focused field, wrapping", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 2, "a"
		if _, handled := applyDialogContract(key("down"), newContract(&cancelled, &submitted, &index, &value)); !handled || index != 0 {
			t.Fatalf("down from last field: handled=%v index=%d, want wrap to 0", handled, index)
		}
		if _, handled := applyDialogContract(key("up"), newContract(&cancelled, &submitted, &index, &value)); !handled || index != 2 {
			t.Fatalf("up from first field: handled=%v index=%d, want wrap to 2", handled, index)
		}
	})

	t.Run("tab and shift+tab are not contract keys (\u00a711.4: reserved for completion)", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		if _, handled := applyDialogContract(key("tab"), newContract(&cancelled, &submitted, &index, &value)); handled || index != 0 {
			t.Fatalf("tab: handled=%v index=%d, want unhandled and unchanged", handled, index)
		}
		if _, handled := applyDialogContract(key("shift+tab"), newContract(&cancelled, &submitted, &index, &value)); handled || index != 0 {
			t.Fatalf("shift+tab: handled=%v index=%d, want unhandled and unchanged", handled, index)
		}
	})

	t.Run("left and right change the selection", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "b"
		if _, handled := applyDialogContract(key("right"), newContract(&cancelled, &submitted, &index, &value)); !handled || value != "c" {
			t.Fatalf("right: handled=%v value=%q, want c", handled, value)
		}
		if _, handled := applyDialogContract(key("left"), newContract(&cancelled, &submitted, &index, &value)); !handled || value != "b" {
			t.Fatalf("left: handled=%v value=%q, want b", handled, value)
		}
	})

	t.Run("space changes the selection when there is no text field to type into", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		if _, handled := applyDialogContract(key(" "), newContract(&cancelled, &submitted, &index, &value)); !handled || value != "b" {
			t.Fatalf("space: handled=%v value=%q, want b", handled, value)
		}
	})

	t.Run("space falls through unhandled when SpaceTypesText says so", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		c := newContract(&cancelled, &submitted, &index, &value)
		c.Fields.SpaceTypesText = func() bool { return true }
		if _, handled := applyDialogContract(key(" "), c); handled || value != "a" {
			t.Fatalf("space on a text field: handled=%v value=%q, want unhandled and unchanged", handled, value)
		}
	})

	t.Run("a key outside the contract vocabulary is unhandled", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		if _, handled := applyDialogContract(key("y"), newContract(&cancelled, &submitted, &index, &value)); handled {
			t.Fatal("\"y\" is not a §11.4 contract key; applyDialogContract must leave it to the dialog")
		}
	})

	t.Run("up/down is a no-op with one or zero fields, not merely wrapped", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		c := dialogContract{Cancel: func() { cancelled = true }, Submit: func() tea.Cmd { submitted = true; return nil }}
		if _, handled := applyDialogContract(key("down"), c); handled || index != 0 {
			t.Fatalf("down with Fields.Count 0: handled=%v index=%d", handled, index)
		}
		if cancelled || submitted {
			t.Fatalf("down must not cancel or submit: cancelled=%v submitted=%v", cancelled, submitted)
		}
		_ = value
	})
}

// TestCreateModalUpDownMoveFieldsTabDoesNot proves task 025's field
// navigation switch directly on the create modal: ↑/↓ move the focused
// field (wrapping, exactly like the old tab/shift+tab did), and tab/
// shift+tab do neither -- on any field but the cwd field there is nothing
// for tab to complete, so it (and shift+tab, never bound anywhere) simply
// leaves the field where it was.
func TestCreateModalUpDownMoveFieldsTabDoesNot(t *testing.T) {
	m := New(nil, config.Settings{Socket: "test-socket"}, "")
	m.creating = true
	m.createField = 2

	updated, _ := m.Update(key("down"))
	if got := updated.(Model).createField; got != 3 {
		t.Fatalf("\"down\" moved createField to %d, want 3", got)
	}
	next := updated.(Model)
	updated, _ = next.Update(key("up"))
	if got := updated.(Model).createField; got != 2 {
		t.Fatalf("\"up\" moved createField to %d, want back to 2", got)
	}

	same := updated.(Model)
	updated, _ = same.Update(key("tab"))
	if got := updated.(Model).createField; got != 2 {
		t.Fatalf("\"tab\" on a non-path field moved createField to %d; tab is reserved for completion (\u00a711.4) and must be a no-op here", got)
	}
	updated, _ = updated.(Model).Update(key("shift+tab"))
	if got := updated.(Model).createField; got != 2 {
		t.Fatalf("\"shift+tab\" moved createField to %d; it is unbound everywhere (\u00a711.4)", got)
	}

	view := m.createView()
	for _, undisclosed := range []string{"Tab/Shift+Tab", "Tab or Shift+Tab"} {
		if strings.Contains(view, undisclosed) {
			t.Fatalf("createView names %q, which tab/shift+tab no longer do (\u00a711.4)\n%s", undisclosed, view)
		}
	}
}

// TestCreateModalTabOnPathFieldWithNoMatchDoesNotMoveFocus proves the other
// half of task 025's tab rule: on the cwd field, when there is nothing to
// complete or list (tabCompleteCreateCWD reports false -- here because the
// typed segment names no directory on disk at all), tab does NOT fall
// through to field navigation the way it used to. Focus stays on the cwd
// field and the field's own text is unchanged.
func TestCreateModalTabOnPathFieldWithNoMatchDoesNotMoveFocus(t *testing.T) {
	m := New(nil, config.Settings{Socket: "test-socket"}, "")
	m.creating = true
	m.createField = 1
	m.createCWD = "/definitely/does/not/exist/anywhere-025"
	m.createCWDPrefilled = false

	updated, _ := m.Update(key("tab"))
	got := updated.(Model)
	if got.createField != 1 {
		t.Fatalf("tab with no filesystem match moved createField to %d, want to stay on 1 (the cwd field)", got.createField)
	}
	if got.createCWD != m.createCWD {
		t.Fatalf("tab with no filesystem match changed createCWD to %q, want unchanged %q", got.createCWD, m.createCWD)
	}
}

// TestCreateModalRecentCWDCyclesOnCtrlPCtrlN proves task 025's third
// statement: recent-cwd cycling (task 009) moved off up/down onto
// Ctrl+P/Ctrl+N, and up/down on the cwd field with no candidate list open
// move fields instead (exactly like every other field), never cycling
// recents anymore.
func TestCreateModalRecentCWDCyclesOnCtrlPCtrlN(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := db.PromoteRecentCwd(ctx, "/recent/one", 5); err != nil {
		t.Fatal(err)
	}
	if err := db.PromoteRecentCwd(ctx, "/recent/two", 5); err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{Socket: "test-socket"}, "")
	m.creating = true
	m.createField = 1
	m.createCWD, m.createCWDPrefilled = "/typed/value", false

	updated, _ := m.Update(key("ctrl+p"))
	got := updated.(Model)
	if got.createCWD != "/recent/two" {
		t.Fatalf("Ctrl+P did not cycle to the most recent entry: createCWD = %q, want %q", got.createCWD, "/recent/two")
	}
	if got.createField != 1 {
		t.Fatalf("Ctrl+P moved createField to %d, want to stay on 1", got.createField)
	}

	// up/down on the cwd field, with no candidate list open, move fields
	// exactly like everywhere else -- they do NOT cycle recents anymore.
	updated, _ = got.Update(key("down"))
	got = updated.(Model)
	if got.createField != 2 {
		t.Fatalf("\"down\" on the cwd field moved createField to %d, want 2 (field navigation, not recent-cwd cycling)", got.createField)
	}
}

// TestCreateModalCandidateListOwnsUpDownWhileOpen proves task 025's fourth
// statement: while the tab-completion candidate list (task 012) is open,
// ↑/↓ still move the highlighted candidate rather than the dialog's
// focused field -- the declared per-field key set inside the list stays in
// force even though ↑/↓ became the dialog's own general navigation keys.
func TestCreateModalCandidateListOwnsUpDownWhileOpen(t *testing.T) {
	m := New(nil, config.Settings{Socket: "test-socket"}, "")
	m.creating = true
	m.createField = 1
	m.createCWDCandidates = []string{"alpha", "beta", "gamma"}
	m.createCWDCandidateIndex = 0

	updated, _ := m.Update(key("down"))
	got := updated.(Model)
	if got.createCWDCandidateIndex != 1 {
		t.Fatalf("\"down\" with the candidate list open moved createCWDCandidateIndex to %d, want 1", got.createCWDCandidateIndex)
	}
	if got.createField != 1 {
		t.Fatalf("\"down\" with the candidate list open moved createField to %d, want to stay on 1", got.createField)
	}

	updated, _ = got.Update(key("up"))
	got = updated.(Model)
	if got.createCWDCandidateIndex != 0 {
		t.Fatalf("\"up\" with the candidate list open moved createCWDCandidateIndex to %d, want back to 0", got.createCWDCandidateIndex)
	}
}
