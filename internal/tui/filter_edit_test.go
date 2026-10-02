package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func filterSend(t *testing.T, m Model, keys ...tea.KeyMsg) Model {
	t.Helper()
	for _, k := range keys {
		got, _ := m.Update(k)
		m = got.(Model)
	}
	return m
}

func filterTyped(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = filterSend(t, m, key(string(r)))
	}
	return m
}

func openedFilterModel() Model {
	m := newFilterTestModel(filterTestSessions())
	m.width, m.height = 80, 24
	got, _ := m.Update(key("/"))
	return got.(Model)
}

// TestFilterEditsTheQueryInItsMiddle is R178's contract for the `/` filter: a
// caret key moves into the query, a typed character lands at the caret, and
// the list re-narrows to the edited query.
func TestFilterEditsTheQueryInItsMiddle(t *testing.T) {
	m := openedFilterModel()
	m = filterTyped(t, m, "alpa-agent")
	if len(m.sessions) != 0 {
		t.Fatalf("setup: typo query matched %v", m.sessions)
	}
	for i := 0; i < 7; i++ {
		m = filterSend(t, m, key("left"))
	}
	if m.filterQuery != "alpa-agent" {
		t.Fatalf("caret keys changed the query: %q", m.filterQuery)
	}
	m = filterSend(t, m, key("h"))
	if m.filterQuery != "alpha-agent" {
		t.Fatalf("query = %q, want the character inserted at the caret", m.filterQuery)
	}
	if len(m.sessions) != 1 || m.sessions[0].ID != "s-alpha" {
		t.Fatalf("list did not re-narrow to the edited query: %v", m.sessions)
	}
	// backspace deletes before the caret, delete under it.
	m = filterSend(t, m, key("backspace"))
	if m.filterQuery != "alpa-agent" {
		t.Fatalf("backspace in the middle: %q", m.filterQuery)
	}
	m = filterSend(t, m, tea.KeyMsg{Type: tea.KeyDelete})
	if m.filterQuery != "alp-agent" {
		t.Fatalf("delete under the caret: %q", m.filterQuery)
	}
}

// TestFilterCaretIsDrawnAsReverseVideoNotUnderscore: the open field draws a
// reversed cell at the caret (a reversed blank at the end) and no `_`.
func TestFilterCaretIsDrawnAsReverseVideoNotUnderscore(t *testing.T) {
	m := openedFilterModel()
	m = filterTyped(t, m, "alpha")
	line := m.filterStatusLine(80)[0]
	if !strings.Contains(line, "Filter: alpha\x1b[7m \x1b[27m") {
		t.Fatalf("end-of-text caret is not a reversed blank: %q", line)
	}
	m = filterSend(t, m, key("left"), key("left"))
	line = m.filterStatusLine(80)[0]
	if !strings.Contains(line, "Filter: alp\x1b[7mh\x1b[27ma") {
		t.Fatalf("caret is not a reversed cell over h: %q", line)
	}
	if strings.Contains(line, "_") {
		t.Fatalf("filter line draws a stand-in caret: %q", line)
	}
}

// TestFilterPasteIsOneInsertion: a bracketed paste lands at the caret with its
// control characters dropped.
func TestFilterPasteIsOneInsertion(t *testing.T) {
	m := openedFilterModel()
	m = filterTyped(t, m, "-agent")
	m = filterSend(t, m, tea.KeyMsg{Type: tea.KeyHome})
	m = filterSend(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alpha\n"), Paste: true})
	if m.filterQuery != "alpha-agent" {
		t.Fatalf("paste: query = %q", m.filterQuery)
	}
}

// TestFilterReopenedKeepsHeldQueryWithCaretAtEnd: a second `/` refines the
// held query, which is the editor's text, caret at its end.
func TestFilterReopenedKeepsHeldQueryWithCaretAtEnd(t *testing.T) {
	m := openedFilterModel()
	m = filterTyped(t, m, "alpha")
	m = filterSend(t, m, key("enter"), key("/"))
	m = filterTyped(t, m, "-agent")
	if m.filterQuery != "alpha-agent" {
		t.Fatalf("reopened query = %q", m.filterQuery)
	}
}

// TestFilterHandlerHasNoOwnBackspaceOrAppendCase is R178's source scan.
func TestFilterHandlerHasNoOwnBackspaceOrAppendCase(t *testing.T) {
	src, err := os.ReadFile("filter.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (m Model) updateFilter(")
	end := strings.Index(text, "func (m Model) filterStatusLine(")
	if start < 0 || end < start {
		t.Fatal("could not locate updateFilter in filter.go")
	}
	body := text[start:end]
	for _, banned := range []string{`case "backspace"`, `"ctrl+h"`, `+= string(`, `msg.Runes`} {
		if strings.Contains(body, banned) {
			t.Fatalf("updateFilter still carries its own text editing (%s)", banned)
		}
	}
	if !strings.Contains(body, "filterEdit.Update(") {
		t.Fatal("updateFilter does not edit through the shared line editor")
	}
	if strings.Contains(text[end:], `+"_"`) {
		t.Fatal("filter render still draws an `_` caret stand-in")
	}
}
