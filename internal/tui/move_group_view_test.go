package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// moveGroupViewModel is a Model with one grouped session selected and the
// picker's candidate groups loaded, ready to render the `g` picker body
// without a store: styledMoveGroupBody only reads model fields.
func moveGroupViewModel(color bool, groups []store.Group) Model {
	gid := int64(1)
	m := New(nil, config.Settings{Color: color}, "")
	m.width, m.height = 80, 30
	m.sessions = []store.Session{{ID: "s1", Name: "alpha-one", Agent: "shell", Status: "stopped", GroupID: &gid, GroupName: "alpha"}}
	m.selected = rowCursor(0)
	m.moveGroupOptions = groups
	m.moveGroupValue = 2
	return m
}

func TestStyledMoveGroupBodyNamesSessionCurrentAndNewGroupAndOptions(t *testing.T) {
	groups := []store.Group{{ID: 1, Name: "alpha"}, {ID: 2, Name: "bravo"}}
	m := moveGroupViewModel(false, groups)
	body := m.styledMoveGroupBody()
	lines := strings.Split(body, "\n")
	want := []string{
		"Move alpha-one to a different group",
		"",
		"Current group: alpha",
		"New group: bravo (left/right cycles: alpha, bravo, default)",
		"",
		"Left/Right cycles · Enter confirms · Esc cancels",
	}
	for i, w := range want {
		if i >= len(lines) || strings.TrimSpace(lines[i]) != w {
			t.Fatalf("styled body line %d = %q, want %q\nfull body:\n%s", i, lines[i], w, body)
		}
	}
	if len(lines) != len(want) {
		t.Fatalf("a body with no note has %d lines, want %d:\n%s", len(lines), len(want), body)
	}
}

func TestStyledMoveGroupBodyAppendsTheNoteInErrorTokenOnly(t *testing.T) {
	m := moveGroupViewModel(true, []store.Group{{ID: 1, Name: "alpha"}, {ID: 2, Name: "bravo"}})
	m.moveGroupNote = "moving a session's group is unavailable"
	body := m.styledMoveGroupBody()
	if !strings.Contains(body, m.colorToken(theme.Error, m.moveGroupNote)) {
		t.Fatalf("note is not rendered in the error token:\n%q", body)
	}
	if got := stripANSI(body); !strings.HasSuffix(got, "\n\n"+m.moveGroupNote) {
		t.Fatalf("note should follow the footer after one blank line, got:\n%s", got)
	}

	noNote := stripANSI(moveGroupViewModel(true, []store.Group{{ID: 1, Name: "alpha"}}).styledMoveGroupBody())
	if !strings.HasSuffix(noNote, "Esc cancels") {
		t.Fatalf("a model with no note must end at the footer, got:\n%s", noNote)
	}
}

func TestStyledMoveGroupBodyColoursTitleAndFooterKeysAndHints(t *testing.T) {
	m := moveGroupViewModel(true, []store.Group{{ID: 1, Name: "alpha"}, {ID: 2, Name: "bravo"}})
	body := m.styledMoveGroupBody()
	for _, c := range []struct {
		tok  theme.Token
		text string
	}{
		{theme.Title, "Move alpha-one to a different group"},
		{theme.Key, "Enter"},
		{theme.Key, "Esc"},
		{theme.Hint, "confirms"},
		{theme.Hint, "cancels"},
	} {
		if !strings.Contains(body, m.colorToken(c.tok, c.text)) {
			t.Errorf("%q is not rendered in token %v:\n%q", c.text, c.tok, body)
		}
	}
	if strings.Contains(body, m.colorToken(theme.Hint, "Enter")) {
		t.Error("the Enter key was rendered as a hint, not a key")
	}
}

func TestStyledMoveGroupBodyWrapsLongCycleListWithoutLosingAnyGroup(t *testing.T) {
	var groups []store.Group
	for i, n := range []string{"payments-platform", "growth-experiments", "infrastructure-core", "data-science-lab", "mobile-clients"} {
		groups = append(groups, store.Group{ID: int64(i + 1), Name: n})
	}
	m := moveGroupViewModel(true, groups)
	m.width = 44
	plain := stripANSI(m.styledMoveGroupBody())
	flat := strings.Join(strings.Fields(plain), " ")
	for _, g := range groups {
		if !strings.Contains(flat, g.Name) {
			t.Errorf("wrapped picker lost group %q:\n%s", g.Name, plain)
		}
	}
	if !strings.Contains(flat, "default") {
		t.Errorf("wrapped picker lost the default group:\n%s", plain)
	}
	if !strings.Contains(plain, "Current group: alpha") && !strings.Contains(plain, "Current group:") {
		t.Errorf("current-group label missing:\n%s", plain)
	}
}

func TestMoveGroupViewFramesTheStyledBody(t *testing.T) {
	m := moveGroupViewModel(false, []store.Group{{ID: 1, Name: "alpha"}, {ID: 2, Name: "bravo"}})
	view := m.moveGroupView()
	for _, w := range []string{"Move alpha-one to a different group", "Current group: alpha", "New group: bravo", "Enter confirms"} {
		if !strings.Contains(view, w) {
			t.Errorf("framed picker lacks %q:\n%s", w, view)
		}
	}
}
