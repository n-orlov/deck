package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 015's (D.4, R137) copy guard for the header cursor: the
// two surfaces the PRD names -- "Footer and `?` help must say what the
// cursor can do in each position", with the detail footer called out by file
// and line -- must BOTH name `c`, `left` and `right` as the header cursor's
// own fold keys, and neither may still describe `c` the old, now-false way
// ("collapsing the selected row's own group": since task 014 `c` acts on
// whichever group the cursor rests on, and from a header that is never the
// selected row's group, because a header is not a row).
//
// The main list footer (footerLegend) is deliberately NOT asserted to carry
// these keys: SPEC §11.3's fixed set is closed against SPEC.md's own prose
// by footer_bindings_parity_test.go (TestFooterLegendGlyphSetIsClosedAgainstSpec,
// both directions) and SPEC.md is read-only to this phase, so the header
// cursor is documented in the two surfaces that can carry it. That reasoning
// is recorded next to footerLegend itself, and
// TestFooterLegendStaysClosedAgainstSpecForHeaderCursorKeys below pins the
// consequence so a later reader does not "fix" the omission and break the
// SPEC parity guards.
//
// Both assertions below fail against the tree before this task's fix. The
// detail-footer one in particular:
//
//	header_cursor_copy_test.go:NN: detail body names no "header cursor" line:
//	  "r renames · l edits launch inputs · g moves group · i or Esc closes detail"
//
// because detailBody's footer block was a single line naming only the
// dialog's own session-scoped keys.

// newHeaderCursorCopyModel builds the one-session model every assertion
// below renders, mirroring group_move_test.go's own detail-footer tests
// (a real row under a row cursor, so detailBody has a session to describe).
func newHeaderCursorCopyModel(ascii bool) Model {
	m := New(nil, config.Settings{ASCII: ascii}, "")
	m.width, m.height = 120, 40
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", Slug: "alpha"}}
	m.selected = rowCursor(0)
	m.detail = true
	return m
}

// headerCursorFooterLine returns the detail footer's header-cursor line --
// the one line of detailBody's footer block naming the cursor rather than
// the dialog's own keys -- failing loudly when there is none.
func headerCursorFooterLine(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "header cursor") {
			return line
		}
	}
	t.Fatalf("detail body names no \"header cursor\" line:\n%s", body)
	return ""
}

// TestDetailFooterNamesTheHeaderCursorFoldKeys is the detail-footer half of
// D.4: the footer names `c` and both arrows, in the Unicode legend and in
// the ASCII fallback (where the arrows are spelled "left"/"right", the
// words the task's own criteria use), attributes them to the header cursor
// rather than to the selected row, and keeps the dialog's own four keys on
// the line the existing tests read as "the footer legend".
func TestDetailFooterNamesTheHeaderCursorFoldKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ascii bool
		want  []string
	}{
		{"unicode", false, []string{"header cursor", "c folds/unfolds", "←", "→"}},
		{"ascii", true, []string{"header cursor", "c folds/unfolds", "left folds", "right unfolds"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newHeaderCursorCopyModel(tc.ascii)
			body := m.detailBody()
			line := headerCursorFooterLine(t, body)
			for _, want := range tc.want {
				if !strings.Contains(line, want) {
					t.Errorf("detail footer's header-cursor line %q does not name %q", line, want)
				}
			}
			// `c` must no longer be sold as the selected row's own group:
			// on a header there is no selected row at all.
			if strings.Contains(body, "selected row's own group") {
				t.Errorf("detail body still describes a fold key as acting on the \"selected row's own group\":\n%s", body)
			}
			// The dialog's own keys stay where every existing test looks
			// for them (group_move_test.go's detailFooterLine: the last
			// non-empty line of detailBody).
			footer := detailFooterLine(body)
			for _, want := range []string{"r renames", "l edits launch inputs", "g moves group", "i or Esc closes detail"} {
				if !strings.Contains(footer, want) {
					t.Errorf("detail footer's last line %q lost %q", footer, want)
				}
			}
		})
	}
}

// TestDetailViewRendersTheHeaderCursorFooterLine proves the new line is
// actually on screen -- inside framedDialogScrollable's wrapped, framed
// output at the 80-column dialog ceiling, not merely present in the
// unwrapped body -- so it cannot pass by being too long to survive
// wrapDialogLines.
func TestDetailViewRendersTheHeaderCursorFooterLine(t *testing.T) {
	m := newHeaderCursorCopyModel(false)
	view := m.detailView()
	for _, want := range []string{"header cursor: c folds/unfolds its group", "← folds it", "→ unfolds it"} {
		if !strings.Contains(view, want) {
			t.Errorf("rendered detail view does not contain %q (wrapped away?):\n%s", want, view)
		}
	}
}

// TestHelpNamesTheHeaderCursorFoldKeys is the `?` overlay half of D.4: the
// keymap's own `c` bullet names all three keys and the cursor they act on,
// and no longer says the fold follows the selected row.
func TestHelpNamesTheHeaderCursorFoldKeys(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		help := helpText(ascii)
		bullet := helpBulletFor(t, help, "  c / ")
		for _, want := range []string{"c", "←", "→", "header", "under the cursor"} {
			if !strings.Contains(bullet, want) {
				t.Errorf("helpText(ascii=%v)'s c bullet does not name %q:\n%s", ascii, want, bullet)
			}
		}
		if strings.Contains(bullet, "selected row's own group") {
			t.Errorf("helpText(ascii=%v)'s c bullet still describes c as collapsing the selected row's own group:\n%s", ascii, bullet)
		}
		// The mouse section's own header-collapse line must cross-reference
		// the key that really does it. It said "(like g)" -- g/G jump to the
		// first/last visible row and have never folded anything.
		if !strings.Contains(help, "toggle that group's collapse (like c)") {
			t.Errorf("helpText(ascii=%v)'s mouse section does not cross-reference c for a group-header click", ascii)
		}
	}
}

// TestFooterLegendStaysClosedAgainstSpecForHeaderCursorKeys records, as an
// executable note, why the list footer is not the third surface above: the
// three header-cursor glyphs are absent from footerLegend, and SPEC.md
// §11.3's fixed-set sentence -- read fresh, the same helper the parity test
// uses -- does not name them either, so adding any of them would fail
// TestFooterLegendGlyphSetIsClosedAgainstSpec's "footerLegend has glyph %q
// that SPEC.md §11.3's footer fixed-set sentence does not name" direction
// against a spec this phase may not edit.
func TestFooterLegendStaysClosedAgainstSpecForHeaderCursorKeys(t *testing.T) {
	present := map[string]bool{}
	for _, e := range parseFooterLegendSource(t) {
		present[e.unicodeKey] = true
	}
	spec := map[string]bool{}
	for _, g := range specFooterFixedSetGlyphs(t) {
		spec[g] = true
	}
	for _, glyph := range []string{"c", "←", "→", "←/→"} {
		if spec[glyph] {
			t.Errorf("SPEC.md §11.3's footer fixed-set sentence now names %q -- the footer may (and then must) carry it, so this test and footerLegend's doc comment need revisiting", glyph)
		}
		if present[glyph] {
			t.Errorf("footerLegend carries %q, which SPEC.md §11.3's fixed-set sentence does not name -- TestFooterLegendGlyphSetIsClosedAgainstSpec fails on it; document the header cursor in detailBody's footer and helpText instead", glyph)
		}
	}
}

// newHeaderCursorListModel builds a two-group list-mode model with the
// cursor parked on the SECOND group's header, which is the position the
// list footer has to describe: `i` (and every other session-scoped key) is
// inert there, so the detail dialog's own footer -- the surface
// TestDetailFooterNamesTheHeaderCursorFooterLine above pins -- cannot be
// opened from this position at all. Without the list footer saying it, a
// user who navigates onto a header sees no advertisement of `c`, `←` or
// `→` anywhere on screen.
func newHeaderCursorListModel(ascii bool) Model {
	m := New(nil, config.Settings{ASCII: ascii}, "")
	m.width, m.height = 120, 40
	groupWork := int64(7)
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"},
		{ID: "s2", Name: "beta", Agent: "shell", Status: "running", Slug: "beta", GroupName: "work", GroupID: &groupWork},
	}
	m.selected = headerCursor(7)
	return m
}

// TestListFooterNamesTheHeaderCursorFoldKeys is the list-mode half of D.4's
// "Footer and `?` help must say what the cursor can do in each position":
// while the cursor rests on a group header there is no selected session, so
// §11.3's status-reason slot on the footer's left is empty -- and that is
// where the header cursor's own keys belong. footerLegend's curated glyph
// set is NOT touched (see TestFooterLegendStaysClosedAgainstSpecForHeaderCursorKeys):
// this is a contextual cue in the reason slot, exactly like
// interactiveScrollCue is for interactive mode.
//
// Fails before task 015's list-footer fix with the footer rendering only the
// curated legend:
//
//	list footer on a group header does not name "c folds/unfolds": "↑↓ move · ..."
func TestListFooterNamesTheHeaderCursorFoldKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ascii bool
		want  []string
	}{
		{"unicode", false, []string{"group header", "c folds/unfolds", "← folds", "→ unfolds"}},
		{"ascii", true, []string{"group header", "c folds/unfolds", "left folds", "right unfolds"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newHeaderCursorListModel(tc.ascii)
			footer := m.footerLineContent()
			for _, want := range tc.want {
				if !strings.Contains(footer, want) {
					t.Errorf("list footer on a group header does not name %q: %q", want, footer)
				}
			}
			// The curated legend still shares the line (SPEC §11.3: the
			// reason slot never squeezes the legend out entirely).
			if !strings.Contains(footer, "?") || !strings.Contains(footer, "q") {
				t.Errorf("list footer on a group header lost the curated legend: %q", footer)
			}
		})
	}
}

// TestListFooterKeepsTheStatusReasonOnARow pins the other side of the same
// slot: with the cursor back on a session row, the footer's left is §7's
// status reason exactly as before, never the header cue.
func TestListFooterKeepsTheStatusReasonOnARow(t *testing.T) {
	m := newHeaderCursorListModel(false)
	m.sessions[0].Status = "stopped"
	m.selected = rowCursor(0)
	footer := m.footerLineContent()
	if !strings.Contains(footer, "resumable") {
		t.Errorf("list footer on a stopped row lost its status reason: %q", footer)
	}
	if strings.Contains(footer, "group header") {
		t.Errorf("list footer on a session row advertises the header cursor's own keys: %q", footer)
	}
}
