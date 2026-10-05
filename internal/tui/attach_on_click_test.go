package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// attachOnClickModel is the squeezed-preview fixture the click-enters
// test (mouse_test.go) uses: an attempted entry fails the 7-row floor
// from pure arithmetic, so entryRefusal is the observable proof an entry
// was attempted at all, and prepareAttach counts any recorded attachment.
func attachOnClickModel(t *testing.T, attachOnClick bool, attachments *[]string) Model {
	t.Helper()
	m := mouseTestModel([]store.Session{
		{ID: "s1", Name: "one", Agent: "shell", Status: "running", Slug: "one"},
		{ID: "s2", Name: "two", Agent: "shell", Status: "running", Slug: "two"},
	})
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-62"})
	m.width, m.height = 80, 9
	m.settings.AttachOnClick = attachOnClick
	m.prepareAttach = func(_ context.Context, id string) error {
		*attachments = append(*attachments, id)
		return nil
	}
	m.selected = rowCursor(-1)
	return m
}

// assertSelectOnly checks the whole "only selects" contract: the row is
// selected, the list keeps focus, and no entry was attempted (no refusal,
// no ownership claim or fit command, no attachment recorded).
func assertSelectOnly(t *testing.T, m Model, cmd tea.Cmd, attachments []string, want int) {
	t.Helper()
	if m.selected != rowCursor(want) {
		t.Fatalf("selected = %v, want row %d", m.selected, want)
	}
	if m.interactive {
		t.Fatalf("entered interactive mode with attach_on_click off")
	}
	if m.entryRefusal.active || m.entryRefusal.reason != "" {
		t.Fatalf("entryRefusal = %+v: an entry was attempted with attach_on_click off", m.entryRefusal)
	}
	if cmd != nil {
		t.Fatalf("a command was returned (ownership claim or fit) with attach_on_click off")
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments recorded = %v with attach_on_click off, want none", attachments)
	}
}

// TestAttachOnClickOnSingleClickEnters: the default (on) is today's
// behaviour -- a click attempts entry.
func TestAttachOnClickOnSingleClickEnters(t *testing.T) {
	var attachments []string
	m := attachOnClickModel(t, true, &attachments)
	x, y := findRow(t, m, 0)
	updated, _ := m.Update(press(x, y))
	got := updated.(Model)
	if got.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("entryRefusal = %+v, want the click to have attempted entry", got.entryRefusal)
	}
}

// TestAttachOnClickOffSingleClickOnlySelects: off, one click selects and
// nothing else.
func TestAttachOnClickOffSingleClickOnlySelects(t *testing.T) {
	var attachments []string
	m := attachOnClickModel(t, false, &attachments)
	x, y := findRow(t, m, 1)
	updated, cmd := m.Update(press(x, y))
	assertSelectOnly(t, updated.(Model), cmd, attachments, 1)
}

// TestAttachOnClickOffDoubleClickOnlySelects: off, a double-click (two
// presses on the same row, each with its release) selects and nothing else:
// no entry, claim, fit or attachment, and the list keeps focus.
func TestAttachOnClickOffDoubleClickOnlySelects(t *testing.T) {
	var attachments []string
	m := attachOnClickModel(t, false, &attachments)
	x, y := findRow(t, m, 0)
	var cmd tea.Cmd
	var next tea.Model = m
	for _, msg := range []tea.MouseMsg{press(x, y), release(x, y), press(x, y), release(x, y)} {
		next, cmd = next.(Model).Update(msg)
		assertSelectOnly(t, next.(Model), cmd, attachments, 0)
	}
}

// TestAttachOnClickOffEnterStillEnters: after a select-only click, ↵ still
// attempts entry, with the same refusal ladder as ever.
func TestAttachOnClickOffEnterStillEnters(t *testing.T) {
	var attachments []string
	m := attachOnClickModel(t, false, &attachments)
	x, y := findRow(t, m, 0)
	updated, _ := m.Update(press(x, y))
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.entryRefusal.kind != entryRefusalRowFloor || !strings.Contains(got.entryRefusal.reason, "7-row floor") {
		t.Fatalf("entryRefusal = %+v, want ↵ to attempt entry after a select-only click", got.entryRefusal)
	}
}

// TestAttachOnClickOffPreviewClickStillEnters: a click on the passive
// preview is ↵ by definition, so it still attempts entry with the setting
// off.
func TestAttachOnClickOffPreviewClickStillEnters(t *testing.T) {
	var attachments []string
	m := attachOnClickModel(t, false, &attachments)
	m.selected = rowCursor(0)
	layout := m.computeLayout()
	updated, _ := m.Update(press(layout.Preview.X+1, layout.Preview.Y+1))
	if got := updated.(Model); got.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("preview click with attach_on_click off: entryRefusal = %+v, want entry attempted", got.entryRefusal)
	}
}
