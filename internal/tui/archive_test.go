package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestListShowsArchivedGlyphFromArchivedAtFlagNotFromStatus proves SPEC
// requirement 27's rendering contract (task 111): the ▣ glyph is driven
// exclusively by session.ArchivedAt, never by session.Status -- there is
// no "archived" status among the six SPEC enumerates (see
// internal/tui/attention.go's own doc on that), so a row with
// Status == "archived" but ArchivedAt == 0 must NOT show the glyph, while
// a row with ArchivedAt set (whatever its real status, "stopped" here as
// requirement 27 requires in practice) must show it.
func TestListShowsArchivedGlyphFromArchivedAtFlagNotFromStatus(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "flag-not-set", Agent: "shell", Status: "archived", ArchivedAt: 0},
		{Name: "flag-is-set", Agent: "shell", Status: "stopped", ArchivedAt: 12345},
	}
	view := model.View()
	lines := strings.Split(view, "\n")

	foundUnflagged, foundFlagged := false, false
	for _, line := range lines {
		if strings.Contains(line, "flag-not-set") {
			foundUnflagged = true
			if strings.Contains(line, "\u25a3") {
				t.Fatalf("row with ArchivedAt=0 unexpectedly shows the archived glyph (status alone must never drive it):\n%s", view)
			}
		}
		if strings.Contains(line, "flag-is-set") {
			foundFlagged = true
			if !strings.Contains(line, "\u25a3") {
				t.Fatalf("row with ArchivedAt set is missing the archived glyph:\n%s", view)
			}
		}
	}
	if !foundUnflagged {
		t.Fatalf("unflagged row not found on screen at all:\n%s", view)
	}
	if !foundFlagged {
		t.Fatalf("flagged row not found on screen at all:\n%s", view)
	}
}

// TestListShowsArchivedGlyphInASCIIModeAsBracketedWord proves DECK_ASCII's
// fallback for the ▣ glyph (SPEC §11.3's EAW rule): a terminal that cannot
// render the optional glyph still gets an unambiguous textual marker.
func TestListShowsArchivedGlyphInASCIIModeAsBracketedWord(t *testing.T) {
	model := New(nil, config.Settings{ASCII: true}, "")
	model.width = 100
	model.sessions = []store.Session{
		{Name: "old", Agent: "shell", Status: "stopped", ArchivedAt: 999},
	}
	view := model.View()
	if !strings.Contains(view, "[archived]") {
		t.Fatalf("ASCII-mode view missing the [archived] fallback marker:\n%s", view)
	}
	if strings.Contains(view, "\u25a3") {
		t.Fatalf("ASCII mode unexpectedly rendered the Unicode glyph:\n%s", view)
	}
}
