package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/service"
)

// cure-01-07 (R145): two frame-consistency defects the nightly -race and
// stability lanes exposed as intermittent feature failures (runs
// 36234392586 and 36238432665). Each test asserts SPEC's own rule on the
// very frame the triggering update produces, never on "a later reload
// eventually fixes it".

// TestLayoutCycleKeepsTheSelectedRowInView is SPEC §11's "the viewport
// follows the selection" across `|`: every layout step that resizes the
// sidebar's content box (every mode but the row-less collapsed strip) must leave the selected row on screen in the frame
// that step renders. Before the fix, a shrink kept the old scroll offset
// and the selected bottom row fell out of view until an unrelated reload.
func TestLayoutCycleKeepsTheSelectedRowInView(t *testing.T) {
	m := drift005FixtureModel(5, 30)
	last := len(m.sessions) - 1
	m.selected = rowCursor(last)
	m.followSelectionViewport()
	want := "> " + m.sessions[last].Name
	if view := m.View(); !strings.Contains(view, want) {
		t.Fatalf("precondition: selected row %q not in view before any layout change:\n%s", want, view)
	}
	for press := 1; press <= 4; press++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("|")})
		m = updated.(Model)
		if m.computeLayout().Effective == LayoutCollapsed {
			continue // the collapsed strip renders no rows at all
		}
		if view := m.View(); !strings.Contains(view, want) {
			t.Fatalf("after `|` press %d (layout %q) the selected row %q is not in view:\n%s", press, m.layoutMode, want, view)
		}
	}
}

// TestLayoutCycleKeepsAWheelDriftInPlace: the follow above never overrides a
// live wheel drift (SPEC §11/R142) -- a layout change only re-clamps it.
func TestLayoutCycleKeepsAWheelDriftInPlace(t *testing.T) {
	m := drift005FixtureModel(5, 30)
	m.selected = rowCursor(len(m.sessions) - 1)
	m.sidebarScrollDrifted = true
	m.sidebarScroll = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("|")})
	m = updated.(Model)
	if !m.sidebarScrollDrifted {
		t.Fatalf("a `|` layout change ended the wheel drift; want it kept")
	}
	if m.sidebarScroll != 0 {
		t.Fatalf("sidebarScroll = %d after `|` under a drift at offset 0, want 0 (re-clamped, not followed)", m.sidebarScroll)
	}
}

// hasEnvBadge reports whether view renders the env_dirty badge in either
// glyph set (`env↻` in the default theme, `env*` under the ASCII fallback
// the feature harness runs with).
func hasEnvBadge(view string) bool {
	return strings.Contains(view, "env↻") || strings.Contains(view, "env*")
}

func envDirtyRestartModel() Model {
	m := drift005FixtureModel(1, 30)
	m.sessions[0].Agent = "shell"
	m.sessions[0].EnvDirty = true
	m.baseSessions = m.sessions
	return m
}

// TestRestartSuccessDropsEnvBadgeInTheFrameTheDialogCloses is SPEC §6.3's
// "restart applies the pending edit and clears env_dirty": Restart has
// already cleared env_dirty in the store before its result arrives, so the
// frame that closes the restart/inject choice dialog must not still show
// the `env*` badge from the stale in-memory row.
func TestRestartSuccessDropsEnvBadgeInTheFrameTheDialogCloses(t *testing.T) {
	m := envDirtyRestartModel()
	if view := m.View(); !hasEnvBadge(view) {
		t.Fatalf("precondition: env_dirty row does not render the env badge:\n%s", view)
	}
	m.restartChoosing = true
	restarted := m.sessions[0]
	restarted.EnvDirty = false
	restarted.Status = "starting"
	updated, _ := m.Update(sessionRestarted{session: restarted, outcome: service.ResumeStarted})
	m = updated.(Model)
	if m.restartChoosing {
		t.Fatalf("restart choice dialog still open after a successful restart")
	}
	if view := m.View(); hasEnvBadge(view) {
		t.Fatalf("frame closing the dialog after a successful restart still shows the env badge:\n%s", view)
	}
}

// TestInjectSuccessDropsEnvBadgeInTheFrameTheDialogCloses is the same rule
// for task 023's inject-instead (InjectEnv clears env_dirty in the store).
func TestInjectSuccessDropsEnvBadgeInTheFrameTheDialogCloses(t *testing.T) {
	m := envDirtyRestartModel()
	if view := m.View(); !hasEnvBadge(view) {
		t.Fatalf("precondition: env_dirty row does not render the env badge:\n%s", view)
	}
	m.restartChoosing = true
	injected := m.sessions[0]
	injected.EnvDirty = false
	updated, _ := m.Update(envInjected{session: injected, keys: []string{"PATH"}})
	m = updated.(Model)
	if m.restartChoosing {
		t.Fatalf("restart choice dialog still open after a successful inject")
	}
	if view := m.View(); hasEnvBadge(view) {
		t.Fatalf("frame closing the dialog after a successful inject still shows the env badge:\n%s", view)
	}
}
