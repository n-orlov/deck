package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestThemeFallbackNoticeShownOnFirstPaint proves SPEC requirement 28: an
// unknown or unparseable [ui] theme name falls back to the default theme
// AND the very first painted frame states that the named theme could not
// be loaded -- it never silently renders the default as though the
// configured theme had applied. config.LoadFrom/theme.Resolve computed
// this reason once (Settings.ThemeReason); this pins the copy the Model
// actually shows for it, at the very first View() call with no prior
// Update at all.
func TestThemeFallbackNoticeShownOnFirstPaint(t *testing.T) {
	reason := `theme "nonexistent-theme" not found; using default theme "dark"`
	model := New(nil, config.Settings{ThemeReason: reason}, "")
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	view := model.View()
	if !strings.Contains(view, reason) {
		t.Fatalf("first-paint view missing theme fallback reason %q:\n%s", reason, view)
	}
}

// TestThemeFallbackNoticeAbsentWhenThemeResolvedCleanly proves the notice
// costs nothing and says nothing false when there is nothing to report:
// ThemeReason is "" whenever no [ui] theme was configured, or the
// configured name resolved cleanly (theme.Resolve's own contract), and in
// that case the frame carries no theme-fallback text at all.
func TestThemeFallbackNoticeAbsentWhenThemeResolvedCleanly(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	view := model.View()
	if strings.Contains(view, "using default theme") {
		t.Fatalf("view wrongly shows a theme fallback notice with no ThemeReason set:\n%s", view)
	}
}

// TestThemeFallbackNoticeStaysWithinFrameBudget proves the notice's rows
// are reserved the same way attachError/resumeNote/startupBanner already
// are (task 005/requirement 37's convention): at a range of terminal
// sizes, including ones narrow enough that the reason text must wrap onto
// several lines, Model.View() never renders more lines than the terminal's
// actual height. (The footer's own un-truncated key legend can separately
// exceed a mid-range width regardless of this notice -- a pre-existing,
// out-of-scope defect TestBelowMinimumFrameStaysWithinBudget already
// documents -- so this test scopes itself to the row-count budget the
// theme notice is actually responsible for.)
func TestThemeFallbackNoticeStaysWithinFrameBudget(t *testing.T) {
	reason := `theme "some-unresolvable-theme-name-that-is-long" not found; using default theme "dark"`
	sizes := [][2]int{{80, 24}, {70, 24}, {60, 20}, {100, 30}, {120, 24}}
	for _, size := range sizes {
		width, height := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
			model := New(nil, config.Settings{ThemeReason: reason}, "")
			model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}
			model.width, model.height = width, height

			view := model.View()
			lines := strings.Split(view, "\n")
			if len(lines) > height {
				t.Fatalf("view has %d lines, exceeding height %d:\n%s", len(lines), height, view)
			}
		})
	}
}
