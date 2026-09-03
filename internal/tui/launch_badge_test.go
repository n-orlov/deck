package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestListShowsLaunchDirtyBadgeAloneBothOrNeither proves the `launch↻`
// sidebar badge (task 025, SPEC §6.2/R108) tracks session.LaunchDirty
// exactly, and that a row carrying both env_dirty and launch_dirty renders
// both badges on its second line without either being lost to width -- the
// same row/line shape TestListShowsEnvDirtyBadgeOnlyWhenSet already pins
// for env↻ alone.
func TestListShowsLaunchDirtyBadgeAloneBothOrNeither(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "launch-only-session", Agent: "claude", Status: "running", PermissionProfile: "safe", LaunchDirty: true},
		{Name: "both-dirty-session", Agent: "claude", Status: "running", PermissionProfile: "safe", EnvDirty: true, LaunchDirty: true},
		{Name: "clean-session", Agent: "claude", Status: "running", PermissionProfile: "safe"},
	}
	view := model.View()
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

	launchOnly, found := rowSecondLine("launch-only-session")
	if !found {
		t.Fatalf("launch-only row not found at all:\n%s", view)
	}
	if !strings.Contains(launchOnly, "launch\u21bb") {
		t.Fatalf("launch-only row's second line missing the launch-dirty badge:\n%s", view)
	}
	if strings.Contains(launchOnly, "env\u21bb") {
		t.Fatalf("launch-only row unexpectedly shows the env-dirty badge:\n%s", view)
	}

	both, found := rowSecondLine("both-dirty-session")
	if !found {
		t.Fatalf("both-dirty row not found at all:\n%s", view)
	}
	if !strings.Contains(both, "env\u21bb") {
		t.Fatalf("both-dirty row's second line missing the env-dirty badge:\n%s", view)
	}
	if !strings.Contains(both, "launch\u21bb") {
		t.Fatalf("both-dirty row's second line missing the launch-dirty badge:\n%s", view)
	}

	clean, found := rowSecondLine("clean-session")
	if !found {
		t.Fatalf("clean row not found at all:\n%s", view)
	}
	if strings.Contains(clean, "env\u21bb") || strings.Contains(clean, "launch\u21bb") {
		t.Fatalf("clean row unexpectedly shows a dirty badge:\n%s", view)
	}
}
