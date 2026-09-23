package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 019's own test obligation, the same shape
// bulk_delete_theme_test.go (018) already carries for the bulk `dd`
// confirm: the single-session archive (`A`) and delete (`dd`) confirms
// must render in SPEC.md:1355's §11.6 tokens at render time, with the
// archive confirm's own "confirming kills the live agent first" warning
// in `badge_warn` (the task's own extension of the base mapping) and every
// validation/failure note in `error`.

// task019ArchiveModel opens the `A` confirm on a live (non-stopped) row at
// 80x24 with colour enabled, so the live-agent sentence renders and every
// token this task adds can be read off a real grid.
func task019ArchiveModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "idle", CWD: "/repo/alpha", ConversationID: "conv-1"}}
	m.selected = rowCursor(0)
	m.archiveConfirming = true
	return m
}

// task019DeleteModel opens the single-session `dd` confirm (no marks) at
// 80x24 with colour enabled.
func task019DeleteModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", CWD: "/repo/alpha", ConversationID: "conv-1"}}
	m.selected = rowCursor(0)
	m.deleteConfirming = true
	m.deletePurgeValue = "keep"
	return m
}

// TestArchiveConfirmStyledBodyMatchesPlainBodyOnceStripped proves the
// styled body task 019 adds is the same dialog as archiveConfirmBody --
// what the existing substring tests in archive_confirm_test.go assert
// against -- with only colour added: once every escape sequence is
// stripped back out of BOTH sides (archiveConfirmBody() already carries a
// detailField-applied escape on its Conversation/Working directory lines
// whenever Color is enabled, task 022, well before this task's own
// styling), styledArchiveConfirmBody is byte-identical to
// wrapDialogLines(archiveConfirmBody()) line for line.
func TestArchiveConfirmStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"live row", func(m *Model) {}},
		{"stopped row", func(m *Model) { m.sessions[0].Status = "stopped" }},
		{"with a failure note", func(m *Model) { m.archiveNote = "Cannot archive alpha: boom" }},
	} {
		m := task019ArchiveModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.archiveConfirmBody())
		styled := strings.Split(m.styledArchiveConfirmBody(), "\n")
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

// TestDeleteConfirmStyledBodyMatchesPlainBodyOnceStripped is the same proof
// for the single-session delete confirm, covering the archived-row note and
// every purge branch (delete_purge_test.go's own two cases).
func TestDeleteConfirmStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"keep chosen", func(m *Model) {}},
		{"archived row", func(m *Model) { m.sessions[0].ArchivedAt = 1 }},
		{"purge chosen, no transcript", func(m *Model) { m.deletePurgeValue = "purge" }},
		{"purge chosen, path resolved", func(m *Model) {
			m.deletePurgeValue = "purge"
			m.deletePurgeOK = true
			m.deletePurgePath = "/home/user/.claude/projects/x/conv-1.jsonl"
		}},
		{"with a failure note", func(m *Model) { m.deleteNote = "Cannot delete alpha: boom" }},
	} {
		m := task019DeleteModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.deleteConfirmBody())
		styled := strings.Split(m.styledDeleteConfirmBody(), "\n")
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

// TestArchiveConfirmTokensMatchSpec proves SPEC.md:1355's token mapping for
// the archive confirm, plus task 019's own `badge_warn` extension for the
// live-agent warning, read per-cell off a real vt.Emulator grid: the title
// in `title`, the "confirming kills the live agent" sentence in
// `badge_warn`, the ordinary explanatory prose in `dimmed`, the footer
// legend's keys in `key` with the surrounding prose in `hint`.
func TestArchiveConfirmTokensMatchSpec(t *testing.T) {
	m := task019ArchiveModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	warnHex := tokenHex(t, m, theme.BadgeWarn)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)

	view := m.archiveConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Archive alpha")
	titleCol := findCol(t, term, titleRow, "Archive")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	// "confirming kills" (not the full sentence): at dialogWidth's inner
	// budget (60 columns at 80x24) this sentence itself word-wraps, so
	// only a prefix guaranteed to land on the sentence's own first
	// physical line is safe to look up as one row.
	warnRow := findRowContaining(t, term, "confirming kills")
	warnCol := findCol(t, term, warnRow, "confirming")
	if fg, ok := cellFgHex(t, term, warnCol, warnRow); !ok || fg != warnHex {
		t.Fatalf("live-agent warning foreground = %q ok=%v, want badge_warn token %s", fg, ok, warnHex)
	}

	dimRow := findRowContaining(t, term, "Archiving keeps the record")
	dimCol := findCol(t, term, dimRow, "Archiving")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	legendRow := findRowContaining(t, term, "Enter archives")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}
	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
	}
	proseCol := findCol(t, term, legendRow, "archives")
	if fg, ok := cellFgHex(t, term, proseCol, legendRow); !ok || fg != hintHex {
		t.Fatalf("legend prose foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
}

// TestArchiveConfirmOnAStoppedRowHasNoWarning proves the badge_warn line
// is absent -- not merely uncoloured -- when the row is already stopped,
// matching archive_confirm_test.go's own
// TestArchiveConfirmOnAStoppedRowPromisesNoKill.
func TestArchiveConfirmOnAStoppedRowHasNoWarning(t *testing.T) {
	m := task019ArchiveModel(t)
	m.sessions[0].Status = "stopped"
	view := stripANSI(m.archiveConfirmView())
	if strings.Contains(view, "kills the live agent") {
		t.Fatalf("a stopped row's styled archive confirm still threatens a kill:\n%s", view)
	}
}

// TestArchiveConfirmNoteIsError proves a failed submit's note renders in
// SPEC.md:1355's `error` token, matching TestBulkDeleteConfirmNoteIsError.
func TestArchiveConfirmNoteIsError(t *testing.T) {
	m := task019ArchiveModel(t)
	m.archiveNote = "Cannot archive alpha: boom"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.archiveConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot archive alpha")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("archiveNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}

// TestDeleteConfirmTokensMatchSpec proves SPEC.md:1355's token mapping for
// the single-session delete confirm: the title in `title`, the
// explanatory survives-text in `dimmed`, the footer legend's keys in
// `key` with the surrounding prose in `hint`.
func TestDeleteConfirmTokensMatchSpec(t *testing.T) {
	m := task019DeleteModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)

	view := m.deleteConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Delete alpha")
	titleCol := findCol(t, term, titleRow, "Delete")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	dimRow := findRowContaining(t, term, "This kills the live pane")
	dimCol := findCol(t, term, dimRow, "This")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	legendRow := findRowContaining(t, term, "Enter deletes")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}
	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
	}
	proseCol := findCol(t, term, legendRow, "deletes")
	if fg, ok := cellFgHex(t, term, proseCol, legendRow); !ok || fg != hintHex {
		t.Fatalf("legend prose foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
}

// TestDeleteConfirmNoteIsError mirrors TestArchiveConfirmNoteIsError/
// TestBulkDeleteConfirmNoteIsError for the single-session delete confirm.
func TestDeleteConfirmNoteIsError(t *testing.T) {
	m := task019DeleteModel(t)
	m.deleteNote = "Cannot delete alpha: boom"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.deleteConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot delete alpha")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("deleteNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}
