package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 021's own test obligation for the rename sub-dialog,
// the same shape archive_delete_confirm_theme_test.go (019) already
// carries: the dialog must render in SPEC.md:1355's §11.6 tokens at
// render time, with no change to renameBody's own plain visible text.

// task021RenameModel opens the rename sub-dialog on a live row at 80x24
// with colour enabled, so every token this task adds can be read off a
// real grid.
func task021RenameModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	m.selected = 0
	m.renaming = true
	m.renameValue = "alpha"
	m.renamePrefilled = true
	return m
}

// TestRenameStyledBodyMatchesPlainBodyOnceStripped proves the styled body
// task 021 adds is the same dialog as renameBody -- what rename_test.go's
// existing substring assertions check against -- with only colour added:
// once every escape sequence is stripped back out of BOTH sides,
// styledRenameBody is byte-identical to wrapDialogLines(renameBody())
// line for line.
func TestRenameStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"prefilled value", func(m *Model) {}},
		{"typed value", func(m *Model) { m.renameValue, m.renamePrefilled = "beta", false }},
		{"with a failure note", func(m *Model) { m.renameNote = `session name "b" already exists` }},
	} {
		m := task021RenameModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.renameBody())
		styled := strings.Split(m.styledRenameBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, plain has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got, want := stripANSI(styled[i]), stripANSI(plain[i]); got != want {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, want)
			}
		}
	}
}

// TestRenameViewCarriesSectionTeamTokens proves the rendered rename
// dialog actually carries §11.6 SGR escapes at render time (not merely
// that styledRenameBody's plain text matches, which the test above
// already covers): the title, the tmux-name explanation and the footer
// legend's keys each carry their own colour once escapes are counted
// rather than stripped.
func TestRenameViewCarriesSectionTokens(t *testing.T) {
	m := task021RenameModel(t)
	th := m.activeTheme()

	titleSGR, ok := m.sgrForToken(th, theme.Title)
	if !ok {
		t.Fatal("theme has no title token")
	}
	dimmedSGR, ok := m.sgrForToken(th, theme.Dimmed)
	if !ok {
		t.Fatal("theme has no dimmed token")
	}
	keySGR, ok := m.sgrForToken(th, theme.Key)
	if !ok {
		t.Fatal("theme has no key token")
	}

	view := m.View()
	if !strings.Contains(view, titleSGR+"Rename alpha") {
		t.Fatalf("rename dialog title is not coloured with theme.Title:\n%q", view)
	}
	if !strings.Contains(view, dimmedSGR) {
		t.Fatalf("rename dialog explanation is not coloured with theme.Dimmed:\n%q", view)
	}
	if !strings.Contains(view, keySGR+"Enter") {
		t.Fatalf("rename dialog footer's Enter is not coloured with theme.Key:\n%q", view)
	}
	if !strings.Contains(view, keySGR+"Esc") {
		t.Fatalf("rename dialog footer's Esc is not coloured with theme.Key:\n%q", view)
	}
	if !strings.Contains(stripANSI(view), "deck_alpha") {
		t.Fatalf("rename dialog lost its plain visible text once escapes are stripped:\n%s", stripANSI(view))
	}
}

// TestRenameViewFocusedFieldGetsSelectionBackground is task 105's own
// success criterion (F18, SPEC.md:1355): the rename sub-dialog's single
// field -- "New name" -- is always the focused field whenever the dialog
// is open (there is nothing else to tab to), so it must carry the same
// selection background a focused create-modal row does
// (TestCreateViewFocusedFieldGetsSelectionBackground, create_view_theme_test.go),
// read per-cell off a real vt.Emulator grid rather than grepped from raw
// escape bytes. This is the regression test for renderRenameFieldRow
// (rename.go): before that change, detailField's own doc comment said
// plainly that neither dialog it serves composes its label/value under a
// shared selection background, and reverting to it here turns this
// assertion red -- see docs/reports/phase3g-105-rename-selection/ for the
// quoted failure.
func TestRenameViewFocusedFieldGetsSelectionBackground(t *testing.T) {
	m := task021RenameModel(t)
	selectionHex := tokenHex(t, m, theme.Selection)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	nameRow := findRowContaining(t, term, "New name:")
	nameCol := findCol(t, term, nameRow, "New name:")
	if bg, ok := cellBgHex(t, term, nameCol, nameRow); !ok || bg != selectionHex {
		t.Fatalf("focused New name row label background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	valueCol := findCol(t, term, nameRow, "alpha")
	if bg, ok := cellBgHex(t, term, valueCol, nameRow); !ok || bg != selectionHex {
		t.Fatalf("focused New name row value background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	// The title line one row above must NOT carry the selection
	// background -- proof the background is scoped to the field row, not
	// leaked across the whole dialog.
	titleRow := findRowContaining(t, term, "Rename alpha")
	titleCol := findCol(t, term, titleRow, "Rename")
	if bg, ok := cellBgHex(t, term, titleCol, titleRow); ok && bg == selectionHex {
		t.Fatalf("title row carries the selection background %s -- selection must not leak past the New name field", bg)
	}
}
