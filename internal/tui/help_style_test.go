package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// helpStyleTestModel opens the `?` overlay with colour on, at a frame tall
// enough (400 rows, matching TestEmptyAndHelpViewsAreDiscoverable's own
// workaround for task 078's height-bounding) that nothing is scrolled off
// before these tests can read it back.
func helpStyleTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.help = true
	m.width, m.height = 100, 400
	return m
}

// wantHelpStyleHeaders is this test's OWN literal copy of the headers
// task 082 requires styled, kept independent of help_style.go's
// helpStyleHeaders so a production edit that silently drops one (e.g. the
// key-removal typo demonstrated red then reverted for this test, see the
// commit message) is still caught here even though iterating the
// production map itself would just silently iterate zero times over the
// missing entry and pass vacuously.
var wantHelpStyleHeaders = []string{
	"Keys",
	"Create dialog fields",
	"Settings takeover (opened with ,)",
	"Theme picker (opened with t)",
	"Runtime controls",
	"Mouse (every binding duplicates a key above; nothing here is mouse-only)",
}

// TestHelpStyleHeaderSetMatchesProduction guards against the two ways the
// two lists above could silently drift apart: production listing a header
// this test never checks, or this test expecting one production dropped.
func TestHelpStyleHeaderSetMatchesProduction(t *testing.T) {
	if len(wantHelpStyleHeaders) != len(helpStyleHeaders) {
		t.Fatalf("wantHelpStyleHeaders has %d entries, helpStyleHeaders has %d -- they must name the same set", len(wantHelpStyleHeaders), len(helpStyleHeaders))
	}
	for _, h := range wantHelpStyleHeaders {
		if !helpStyleHeaders[h] {
			t.Errorf("wantHelpStyleHeaders names %q, which helpStyleHeaders does not contain", h)
		}
	}
}

// TestHelpStyleHeadersRenderInTitleToken proves every header task 082
// names (wantHelpStyleHeaders, not the production map -- see its own doc
// comment for why) actually renders in theme.Title once color is on --
// read per-cell off a real vt.Emulator grid, not grepped from raw escape
// bytes, exactly like sidebar_hierarchy_test.go's token assertions.
func TestHelpStyleHeadersRenderInTitleToken(t *testing.T) {
	m := helpStyleTestModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	textHex := tokenHex(t, m, theme.Text)
	if titleHex == textHex {
		t.Skip("this theme's title and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	for _, header := range wantHelpStyleHeaders {
		row := findRowContaining(t, term, header)
		col := findCol(t, term, row, header)
		fg, ok := cellFgHex(t, term, col, row)
		if !ok {
			t.Errorf("header %q has no foreground colour", header)
			continue
		}
		if fg != titleHex {
			t.Errorf("header %q foreground = %s, want title token %s", header, fg, titleHex)
		}
	}
}

// TestHelpKeysEntryKeycapRendersInKeyToken proves a "Keys" section entry's
// leading keycap phrase renders in theme.Key while the rest of that same
// line's prose renders in theme.Text -- both single-token entries (e.g.
// "dd", "q") and the multi-token alternation form ("↑/↓ or j/k").
func TestHelpKeysEntryKeycapRendersInKeyToken(t *testing.T) {
	m := helpStyleTestModel(t)
	keyHex := tokenHex(t, m, theme.Key)
	textHex := tokenHex(t, m, theme.Text)
	if keyHex == textHex {
		t.Skip("this theme's key and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	cases := []struct {
		keycap string
		prose  string
	}{
		{"dd", "delete"},
		{"PgUp/PgDn", "page"},
	}
	for _, c := range cases {
		row := findRowContaining(t, term, c.keycap)
		keyCol := findCol(t, term, row, c.keycap)
		fg, ok := cellFgHex(t, term, keyCol, row)
		if !ok {
			t.Errorf("keycap %q has no foreground colour", c.keycap)
			continue
		}
		if fg != keyHex {
			t.Errorf("keycap %q foreground = %s, want key token %s", c.keycap, fg, keyHex)
		}

		proseCol := findCol(t, term, row, c.prose)
		fg, ok = cellFgHex(t, term, proseCol, row)
		if !ok {
			t.Errorf("entry %q's prose has no foreground colour", c.keycap)
			continue
		}
		if fg != textHex {
			t.Errorf("entry %q's prose foreground = %s, want text token %s", c.keycap, fg, textHex)
		}
	}

	// The multi-token alternation form: the whole "↑/↓ or j/k" phrase is
	// one keycap span, not just its first token.
	row := findRowContaining(t, term, "select a session")
	for _, tok := range []string{"↑/↓", "or", "j/k"} {
		col := findCol(t, term, row, tok)
		fg, ok := cellFgHex(t, term, col, row)
		if !ok {
			t.Fatalf("alternation token %q has no foreground colour", tok)
		}
		if fg != keyHex {
			t.Errorf("alternation token %q foreground = %s, want key token %s", tok, fg, keyHex)
		}
	}
}

// TestHelpKeysEntryContinuationRendersInDimmedToken proves a "Keys"
// section entry's wrapped continuation line (the ↵ entry has several)
// renders in theme.Dimmed, distinct from that entry's own first-line
// prose in theme.Text.
func TestHelpKeysEntryContinuationRendersInDimmedToken(t *testing.T) {
	m := helpStyleTestModel(t)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	textHex := tokenHex(t, m, theme.Text)
	if dimmedHex == textHex {
		t.Skip("this theme's dimmed and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "forward to its live pane")
	col := findCol(t, term, row, "forward")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("continuation line has no foreground colour")
	}
	if fg != dimmedHex {
		t.Fatalf("continuation line foreground = %s, want dimmed token %s", fg, dimmedHex)
	}
}

// TestHelpNonKeysSectionBodyStaysUnstyled confirms the deliberate scope
// choice recorded in help_style.go's doc comment: outside the "Keys"
// section, only the header line is coloured -- the body text (e.g.
// "Settings takeover"'s own key list) has no foreground colour of its
// own, i.e. it renders exactly as an uncoloured cell (no theme.Key/Text/
// Dimmed applied) even though it visually resembles a keycap+prose entry.
func TestHelpNonKeysSectionBodyStaysUnstyled(t *testing.T) {
	m := helpStyleTestModel(t)
	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "switch focus between the category list")
	col := findCol(t, term, row, "Tab")
	if _, ok := cellFgHex(t, term, col, row); ok {
		t.Fatalf("Settings takeover entry's leading token has a foreground colour; body styling outside \"Keys\" should not be applied")
	}
}

// TestHelpKeysEntriesAllGetKeycapStyling is the non-vacuousness guard for
// splitLeadingKeyPhrase/helpKeycapTokens: every real "Keys" section entry
// (helpText's own content, read live so this cannot drift into a stale
// fixture) must resolve a non-empty leading keycap phrase. It is not
// enough for the tests above to pass on the handful of entries they name;
// this walks every entry the same way helpKeysSectionEntries (help_
// keymap_parity_test.go) does, so a future entry whose leading token is
// missing from helpKeycapTokens is caught here, not silently rendered
// unstyled.
func TestHelpKeysEntriesAllGetKeycapStyling(t *testing.T) {
	lines := strings.Split(helpText(false), "\n")
	start := -1
	for i, l := range lines {
		if l == "Keys" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("could not find the \"Keys\" section header in helpText")
	}
	checked := 0
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if !strings.HasPrefix(l, "  ") {
			break
		}
		if strings.HasPrefix(l, "   ") {
			continue
		}
		checked++
		body := strings.TrimPrefix(l, "  ")
		phrase, rest := splitLeadingKeyPhrase(body)
		if phrase == "" {
			t.Errorf("entry %q has no recognised leading keycap phrase -- helpKeycapTokens needs a new entry", l)
			continue
		}
		if phrase+rest != body {
			t.Errorf("entry %q: phrase %q + rest %q != body %q", l, phrase, rest, body)
		}
	}
	if checked < 20 {
		t.Fatalf("only checked %d \"Keys\" section entries, expected 25+ -- extraction is broken, not the source", checked)
	}
}

// TestHelpStyleDegradesToPlainTextWithColorOff proves the NO_COLOR/
// DECK_COLOR=0 degradation path stated explicitly (task 082's own
// successCriteria): with m.settings.Color false, styledHelpText produces
// byte-for-byte the same text as bare helpText -- no escape sequence
// anywhere -- because every colorToken call it makes is a no-op.
func TestHelpStyleDegradesToPlainTextWithColorOff(t *testing.T) {
	m := New(nil, config.Settings{}, "") // Color zero-value is false
	styled := m.styledHelpText()
	plain := helpText(m.settings.ASCII)
	if styled != plain {
		t.Fatalf("styledHelpText with Color=false must equal bare helpText byte-for-byte; got a diff (styled len=%d, plain len=%d)", len(styled), len(plain))
	}
	if strings.Contains(styled, "\x1b[") {
		t.Fatalf("styledHelpText with Color=false must contain no escape sequence at all")
	}
}

// TestHelpOverlayWidthStaysWithinFrameBudgetAt80ColumnsWithColorOn re-runs
// TestHelpOverlayWidthStaysWithinFrameBudgetAt80Columns's own assertion
// (help_keymap_parity_test.go) but with colour ON, confirming -- not
// merely assuming -- that panel.go's stringWidth/ansiEscapeLen skip the
// new escape sequences task 082 adds rather than spending columns on
// them.
func TestHelpOverlayWidthStaysWithinFrameBudgetAt80ColumnsWithColorOn(t *testing.T) {
	model := New(nil, config.Settings{Color: true}, "")
	model.width, model.height = 80, 24
	model.help = true
	view := model.View()
	for i, line := range strings.Split(view, "\n") {
		if w := stringWidth(line); w > 80 {
			t.Errorf("help view line %d is %d columns wide, exceeding the 80-column budget: %q", i, w, line)
		}
	}
}
