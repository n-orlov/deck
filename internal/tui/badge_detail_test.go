package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestListShowsProfileBadgeForAgentsAndNoneForShell proves the list badges a
// permission profile for agent rows (SPEC §5: "needs a badge visible in the
// list") and shows none for a shell row, which has no notion of a permission
// profile at all.
func TestListShowsProfileBadgeForAgentsAndNoneForShell(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "yolo-claude", Agent: "claude", Status: "starting", PermissionProfile: "yolo"},
		{Name: "plain-shell", Agent: "shell", Status: "running", PermissionProfile: "safe"},
	}
	view := model.View()
	if !strings.Contains(view, "[yolo]") {
		t.Fatalf("list view missing yolo badge:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "plain-shell") && strings.Contains(line, "[") {
			t.Fatalf("shell row rendered a profile badge:\n%s", line)
		}
	}
}

// TestDetailViewShowsProfileAndDegradation proves the detail pane states the
// resolved profile and, when the adapter degraded it, an explicit
// degradation sentence (SPEC §5: pi + plan -> safe).
func TestDetailViewShowsProfileAndDegradation(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{
			Name: "pi-plan", Agent: "pi", Status: "starting",
			PermissionProfile:       "safe",
			PermissionProfileReason: `pi does not support permission profile "plan"; falling back to safe`,
		},
	}
	model.selected = 0
	model.detail = true
	view := model.View()
	if !strings.Contains(view, "Permission profile: safe") {
		t.Fatalf("detail view missing resolved profile:\n%s", view)
	}
	if !strings.Contains(view, "degraded") || !strings.Contains(view, "falling back to safe") {
		t.Fatalf("detail view missing explicit degradation sentence:\n%s", view)
	}
}

// TestDetailViewOmitsProfileForShell proves a shell row's detail states the
// profile notion does not apply, rather than showing a meaningless default.
func TestDetailViewOmitsProfileForShell(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "plain-shell", Agent: "shell", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0
	model.detail = true
	view := model.View()
	if !strings.Contains(view, "n/a") {
		t.Fatalf("shell detail view did not state the profile notion does not apply:\n%s", view)
	}
	if strings.Contains(view, "Permission profile: safe") {
		t.Fatalf("shell detail view showed a meaningless profile value:\n%s", view)
	}
}

// TestDetailKeyTogglesAndEscCloses proves i opens and closes the detail
// pane, and Esc also closes it without disturbing the list.
func TestDetailKeyTogglesAndEscCloses(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "s", Agent: "shell", Status: "running"}}
	updated, _ := model.Update(key("i"))
	model = updated.(Model)
	if !model.detail {
		t.Fatal("i did not open detail")
	}
	updated, _ = model.Update(key("i"))
	model = updated.(Model)
	if model.detail {
		t.Fatal("i did not close detail")
	}
	model.detail = true
	updated, _ = model.Update(key("esc"))
	if updated.(Model).detail {
		t.Fatal("Esc did not close detail")
	}
}
