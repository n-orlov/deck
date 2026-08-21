package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// mainViewColorTestModel builds a colour-enabled Model with a couple of
// sessions loaded on a generous frame (the default 80x24 sidebar is only
// 33 content columns wide, tight enough to risk truncating a name before
// an assertion below can reach it), the default theme active, so
// title/border_focus/border/selection/key/hint all resolve to the real,
// distinct hex values this file's tests read per cell (SPEC requirement
// 33: colour only from theme tokens).
func mainViewColorTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 120, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", CWD: "/repo/alpha"},
		{ID: "s2", Name: "beta", Agent: "shell", Status: "running", CWD: "/repo/beta"},
	}
	m.selected = 0
	return m
}

// TestSidebarTitleRendersInTitleToken proves SPEC requirement 35: the
// sidebar's own border title ("deck — sessions") renders in the `title`
// token, read per cell off a real vt.Emulator grid.
func TestSidebarTitleRendersInTitleToken(t *testing.T) {
	m := mainViewColorTestModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	textHex := tokenHex(t, m, theme.Text)
	if titleHex == textHex {
		t.Skip("this theme's title and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "deck")
	col := findCol(t, term, row, "deck")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("sidebar title has no foreground colour")
	}
	if fg != titleHex {
		t.Fatalf("sidebar title foreground = %s, want title token %s", fg, titleHex)
	}
}

// TestSidebarBorderIsFocusPreviewBorderIsUnfocused proves SPEC requirements
// 19/42: in the main view's side-by-side frame, the sidebar (the only
// focusable region) draws its border in `border_focus`, and the preview
// (never focusable there) draws its own in plain `border` -- never the
// same colour, and never swapped.
func TestSidebarBorderIsFocusPreviewBorderIsUnfocused(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		t.Fatalf("120x30 frame computed as stacked, want side-by-side (test assumes a shared seam)")
	}
	sw := layout.Sidebar.Width

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	leftFg, ok := cellFgHex(t, term, 0, 0)
	if !ok {
		t.Fatalf("sidebar's top-left corner has no foreground colour")
	}
	if leftFg != focusHex {
		t.Fatalf("sidebar border corner = %s, want border_focus token %s", leftFg, focusHex)
	}
	seamFg, ok := cellFgHex(t, term, sw, 0)
	if !ok {
		t.Fatalf("preview's seam corner has no foreground colour")
	}
	if seamFg != borderHex {
		t.Fatalf("preview border seam = %s, want border token %s", seamFg, borderHex)
	}
}

// TestSelectedSidebarRowUsesSelectionBackground proves SPEC requirement 42
// for the main session list: the currently-selected row's own text
// carries a `selection` BACKGROUND, and an unselected row carries none.
func TestSelectedSidebarRowUsesSelectionBackground(t *testing.T) {
	m := mainViewColorTestModel(t)
	selHex := tokenHex(t, m, theme.Selection)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "> alpha")
	col := findCol(t, term, row, "alpha")
	bg, ok := cellBgHex(t, term, col, row)
	if !ok {
		t.Fatalf("selected row %q has no background colour", "alpha")
	}
	if bg != selHex {
		t.Fatalf("selected row background = %s, want selection token %s", bg, selHex)
	}

	otherRow := findRowContaining(t, term, "  beta")
	otherCol := findCol(t, term, otherRow, "beta")
	if otherBg, ok := cellBgHex(t, term, otherCol, otherRow); ok && otherBg == selHex {
		t.Fatalf("unselected row %q also carries the selection background %s", "beta", otherBg)
	}
}

// TestGroupHeaderRendersInGroupToken proves SPEC requirement 35: a
// workspace header line in the sidebar renders in the `group` token.
func TestGroupHeaderRendersInGroupToken(t *testing.T) {
	m := mainViewColorTestModel(t)
	groupHex := tokenHex(t, m, theme.Group)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "> alpha")
	// alpha's workspace header ("alpha", the basename of /repo/alpha) is
	// on the row directly above its own session row (task 014's grouping
	// preserves each session's relative position; alpha is m.sessions[0]).
	headerRow := row - 1
	col := findCol(t, term, headerRow, "alpha")
	fg, ok := cellFgHex(t, term, col, headerRow)
	if !ok {
		t.Fatalf("group header has no foreground colour")
	}
	if fg != groupHex {
		t.Fatalf("group header foreground = %s, want group token %s", fg, groupHex)
	}
}

// TestFooterKeyAndHintTokens proves SPEC requirement 35: the footer's key
// legend renders each key glyph in `key` and each hint word after it in
// `hint`, distinctly.
func TestFooterKeyAndHintTokens(t *testing.T) {
	m := mainViewColorTestModel(t)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)
	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "acknowledge")

	keyCol := findCol(t, term, row, "Y")
	keyFg, ok := cellFgHex(t, term, keyCol, row)
	if !ok {
		t.Fatalf("footer key %q has no foreground colour", "Y")
	}
	if keyFg != keyHex {
		t.Fatalf("footer key foreground = %s, want key token %s", keyFg, keyHex)
	}

	hintCol := findCol(t, term, row, "acknowledge")
	hintFg, ok := cellFgHex(t, term, hintCol, row)
	if !ok {
		t.Fatalf("footer hint %q has no foreground colour", "acknowledge")
	}
	if hintFg != hintHex {
		t.Fatalf("footer hint foreground = %s, want hint token %s", hintFg, hintHex)
	}
}
