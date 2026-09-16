package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// matrixSidebarStatusFg renders one sidebar row for session through the
// REAL production render path (Model.sidebarRowLines -> colorToken ->
// theme.Theme.Color, exactly what a running client paints) into a
// vt.Emulator, then reads the ACTUAL painted foreground colour off the
// cell carrying wantText, the same per-cell extraction
// settings_color_cues_test.go's cellFgHex/findCol use for every other
// colour-token proof in this package -- never grepping the raw escape
// bytes and never comparing matrix.toml's authored strings directly.
func matrixSidebarStatusFg(t *testing.T, m Model, session store.Session, wantText string) string {
	t.Helper()
	lines, _, _ := m.sidebarRowLines(0, session, false)
	line := lines[0]
	term := renderSettingsToEmulator(t, line, 200, 1)
	col := findCol(t, term, 0, wantText)
	hex, ok := cellFgHex(t, term, col, 0)
	if !ok {
		t.Fatalf("text %q in rendered row %q has no foreground colour", wantText, line)
	}
	return hex
}

// TestMatrixStatusTokensRenderAsSevenDistinctColours is task 317's proof
// for R56: under the built-in `matrix` theme, each of §7's seven status
// tokens -- the six real session.Status words plus the archived FLAG's own
// token (theme.StatusTokens' full list, task 014's statusToken mapping) --
// paints as its own, pairwise-distinct colour on a real sidebar row, read
// per-cell off a vt.Emulator grid rather than compared as authored TOML
// strings. This is deliberately an "as painted" check, not a re-statement
// of matrix.toml: it goes through Model.sidebarRowLines exactly as a
// running client would, so a bug anywhere between the theme file and the
// terminal (wrong token wired to the wrong word, a stale default sneaking
// in, colorToken losing a token) would be caught here even if the TOML
// file itself were perfectly authored.
//
// The six ordinary statuses are read off the status WORD itself (the text
// segment task 014/theme_color.go's statusToken colours); archived is not
// a session.Status value (SPEC requirement 27 -- archived_at is a flag,
// never a status) so it is read off the ▣ badge glyph a row gains when
// ArchivedAt != 0, which is the only place theme.Archived is ever painted.
func TestMatrixStatusTokensRenderAsSevenDistinctColours(t *testing.T) {
	th, ok := theme.Builtin("matrix")
	if !ok {
		t.Fatal("built-in theme matrix is not registered")
	}
	m := Model{settings: config.Settings{Color: true, Theme: th}}

	got := make(map[string]string, len(theme.StatusTokens))

	// Session names are deliberately UNRELATED to their status word ("pane"
	// + a number, never "sess-<status>") -- findCol locates the FIRST
	// column matching wantText, so a name that happened to contain the
	// status word as a substring (e.g. "sess-running" containing
	// "running") would silently read that occurrence's colour (the row's
	// nameTok, theme.Title) instead of the actual status word's, which is
	// exactly the false-negative this test caught on its first run.
	statuses := []string{"starting", "running", "waiting", "idle", "error", "stopped"}
	for i, status := range statuses {
		session := store.Session{ID: status, Name: fmt.Sprintf("pane%d", i), Status: status, Acknowledged: true}
		got[status] = matrixSidebarStatusFg(t, m, session, status)
	}

	// archived: session.Status is deliberately an ordinary status
	// ("running") to prove the badge's OWN token is what is read, not the
	// status word's -- ArchivedAt != 0 is what triggers the ▣ badge.
	archivedGlyph := m.glyph("\u25a3", "[archived]")
	archivedSession := store.Session{ID: "archived", Name: "pane99", Status: "running", Acknowledged: true, ArchivedAt: 1000}
	got["archived"] = matrixSidebarStatusFg(t, m, archivedSession, archivedGlyph)

	if len(got) != len(theme.StatusTokens) {
		t.Fatalf("collected %d status colours, want %d (one per theme.StatusTokens entry)", len(got), len(theme.StatusTokens))
	}

	// Pairwise distinctness, reported exhaustively rather than
	// short-circuiting on the first collision, so a failure names every
	// colliding pair at once.
	byColour := make(map[string][]string, len(got))
	for status, hex := range got {
		byColour[hex] = append(byColour[hex], status)
	}
	var collisions []string
	for hex, statuses := range byColour {
		if len(statuses) > 1 {
			collisions = append(collisions, hex+": "+strings.Join(statuses, ", "))
		}
	}
	if len(collisions) > 0 {
		t.Fatalf("matrix theme: %d of the seven status tokens render as the SAME painted colour (not pairwise-distinct): %s", len(collisions), strings.Join(collisions, " | "))
	}
	if len(byColour) != 7 {
		t.Fatalf("matrix theme: got %d distinct painted colours across the seven status tokens, want 7 -- %v", len(byColour), got)
	}
}
