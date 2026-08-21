package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
)

// TestApplyDialogContractCoreKeys proves the ONE shared implementation
// (dialog_contract.go) performs SPEC §11.4's base vocabulary — esc cancels,
// enter submits, tab/shift+tab move fields, left/right/space change a
// selection — directly, independent of any of the five dialogs that defer
// to it.
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

	t.Run("tab and shift+tab move the focused field, wrapping", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 2, "a"
		if _, handled := applyDialogContract(key("tab"), newContract(&cancelled, &submitted, &index, &value)); !handled || index != 0 {
			t.Fatalf("tab from last field: handled=%v index=%d, want wrap to 0", handled, index)
		}
		if _, handled := applyDialogContract(key("shift+tab"), newContract(&cancelled, &submitted, &index, &value)); !handled || index != 2 {
			t.Fatalf("shift+tab from first field: handled=%v index=%d, want wrap to 2", handled, index)
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

	t.Run("tab is a no-op with one or zero fields, not merely wrapped", func(t *testing.T) {
		var cancelled, submitted bool
		index, value := 0, "a"
		c := dialogContract{Cancel: func() { cancelled = true }, Submit: func() tea.Cmd { submitted = true; return nil }}
		if _, handled := applyDialogContract(key("tab"), c); handled || index != 0 {
			t.Fatalf("tab with Fields.Count 0: handled=%v index=%d", handled, index)
		}
		if cancelled || submitted {
			t.Fatalf("tab must not cancel or submit: cancelled=%v submitted=%v", cancelled, submitted)
		}
		_ = value
	})
}

// TestCreateModalDownUpDoNotMoveFields proves the create modal's field
// navigation binds ONLY tab/shift+tab (SPEC §11.4), not the undisclosed
// "down"/"up" aliases the pre-shared-contract switch used to accept even
// though createView's own footer never named them — a dialog must not bind
// a key the contract does not give it and the dialog does not name on
// screen.
func TestCreateModalDownUpDoNotMoveFields(t *testing.T) {
	m := New(nil, config.Settings{Socket: "test-socket"}, "")
	m.creating = true
	m.createField = 2

	updated, _ := m.Update(key("down"))
	if got := updated.(Model).createField; got != 2 {
		t.Fatalf("\"down\" moved createField to %d; it is not a named create-dialog key", got)
	}
	updated, _ = m.Update(key("up"))
	if got := updated.(Model).createField; got != 2 {
		t.Fatalf("\"up\" moved createField to %d; it is not a named create-dialog key", got)
	}

	view := m.createView()
	for _, undisclosed := range []string{"Up/Down", "Up or Down"} {
		if strings.Contains(view, undisclosed) {
			t.Fatalf("createView names %q, which would legitimise the removed alias:\n%s", undisclosed, view)
		}
	}
}
