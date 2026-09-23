package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSidebarRowHidesSafeBadgeButKeepsNonSafeBadgeAtMinimumWidth pins
// SPEC.md:1339's rule that line 2's permission badge is shown only for a
// non-`safe` profile -- a `safe` row renders no badge there at all, while
// `plan`/`edits`/`yolo` still render theirs as the line's last segment,
// after the bare age. Run at SidebarWidthFloor (the tightest content
// budget the row can ever render at) so the skip isn't something that
// only happens to hold at a wider width.
func TestSidebarRowHidesSafeBadgeButKeepsNonSafeBadgeAtMinimumWidth(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, profile := range []string{"plan", "edits", "yolo"} {
		m := New(nil, config.Settings{}, "")
		m.width, m.height = 100, 30
		m.sidebarWidth = SidebarWidthFloor
		m.sessions = []store.Session{
			{ID: "s1", Name: "aa", Agent: "claude", Status: "running", PermissionProfile: "safe", CreatedAt: now},
			{ID: "s2", Name: "bb", Agent: "claude", Status: "running", PermissionProfile: profile, CreatedAt: now},
		}
		m.selected = rowCursor(-1)
		view := m.View()
		lines := strings.Split(view, "\n")

		rowSecondLine := func(name string) (string, bool) {
			for i, line := range lines {
				if strings.Contains(line, name) {
					if i+1 < len(lines) {
						return lines[i+1], true
					}
					return "", true
				}
			}
			return "", false
		}

		safeLine2, found := rowSecondLine("aa")
		if !found {
			t.Fatalf("[profile=%s] safe row not found at all:\n%s", profile, view)
		}
		if strings.Contains(safeLine2, "[safe]") {
			t.Fatalf("[profile=%s] safe row's second line unexpectedly shows a permission badge:\n%q", profile, safeLine2)
		}

		nonSafeLine2, found := rowSecondLine("bb")
		if !found {
			t.Fatalf("[profile=%s] non-safe row not found at all:\n%s", profile, view)
		}
		want := "[" + profile + "]"
		if !strings.Contains(nonSafeLine2, want) {
			t.Fatalf("[profile=%s] non-safe row's second line missing its permission badge %q:\n%q", profile, want, nonSafeLine2)
		}
		fields := strings.Split(nonSafeLine2, "\u2502")
		if len(fields) < 2 {
			t.Fatalf("[profile=%s] non-safe row's second line has no sidebar border to scope the check to:\n%q", profile, nonSafeLine2)
		}
		sidebarContent := strings.TrimRight(fields[1], " ")
		if !strings.HasSuffix(sidebarContent, want) {
			t.Fatalf("[profile=%s] non-safe row's permission badge %q is not the line's last segment:\n%q", profile, want, sidebarContent)
		}
	}
}
