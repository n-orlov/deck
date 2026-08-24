package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// sidebarHierarchyTestModel mirrors mainViewColorTestModel (main_view_
// theme_test.go) but gives its sessions the fields steer 006's three token
// swaps actually read: a hook-sourced (quality "live") status so the
// quality badge renders at all, and a real CreatedAt so "created ..." has
// something to colour on line 2.
func sidebarHierarchyTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 160, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", StatusSource: "hook", CWD: "/repo/alpha", CreatedAt: 1000},
		{ID: "s2", Name: "beta", Agent: "shell", Status: "starting", CWD: "/repo/beta", CreatedAt: 1000},
	}
	m.selected = 0
	return m
}

// TestSidebarNameRendersInTitleToken proves steer 006 item 1: a session's
// own name (not a row that is merely `starting`, see the dedicated
// override test below) renders in the `title` token instead of `text`.
func TestSidebarNameRendersInTitleToken(t *testing.T) {
	m := sidebarHierarchyTestModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	textHex := tokenHex(t, m, theme.Text)
	if titleHex == textHex {
		t.Skip("this theme's title and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "> alpha")
	col := findCol(t, term, row, "alpha")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("session name has no foreground colour")
	}
	if fg != titleHex {
		t.Fatalf("session name foreground = %s, want title token %s", fg, titleHex)
	}
}

// TestStartingRowNameStaysDimmedNotTitle proves the SPEC requirement
// 35/task 021 override still wins over the new theme.Title default: a
// `starting` row's name must render in `dimmed`, not `title`, distinct
// from a bare "name is title" assertion that would still pass if this
// override were silently deleted (it would just make every row, starting
// or not, read as title).
func TestStartingRowNameStaysDimmedNotTitle(t *testing.T) {
	m := sidebarHierarchyTestModel(t)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	titleHex := tokenHex(t, m, theme.Title)
	if dimmedHex == titleHex {
		t.Skip("this theme's dimmed and title tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "beta")
	col := findCol(t, term, row, "beta")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("starting row's name has no foreground colour")
	}
	if fg == titleHex {
		t.Fatalf("starting row's name foreground = %s (the title token) -- the starting-row dimmed override must win over the new title default", fg)
	}
	if fg != dimmedHex {
		t.Fatalf("starting row's name foreground = %s, want dimmed token %s", fg, dimmedHex)
	}
}

// TestSidebarCreatedLineRendersDimmed proves steer 006 item 2: line 2's
// "created <relative>" text renders in `dimmed` instead of `text`.
func TestSidebarCreatedLineRendersDimmed(t *testing.T) {
	m := sidebarHierarchyTestModel(t)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	textHex := tokenHex(t, m, theme.Text)
	if dimmedHex == textHex {
		t.Skip("this theme's dimmed and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "created")
	col := findCol(t, term, row, "created")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("created line has no foreground colour")
	}
	if fg != dimmedHex {
		t.Fatalf("created line foreground = %s, want dimmed token %s", fg, dimmedHex)
	}
}

// TestSidebarQualityBadgeRendersDimmed proves steer 006 item 3: the
// status-source quality badge (`live`/`sampled`) renders in `dimmed`
// instead of `badge`.
func TestSidebarQualityBadgeRendersDimmed(t *testing.T) {
	m := sidebarHierarchyTestModel(t)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	badgeHex := tokenHex(t, m, theme.Badge)
	if dimmedHex == badgeHex {
		t.Skip("this theme's dimmed and badge tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, "> alpha")
	col := findCol(t, term, row, "live")
	fg, ok := cellFgHex(t, term, col, row)
	if !ok {
		t.Fatalf("quality badge has no foreground colour")
	}
	if fg != dimmedHex {
		t.Fatalf("quality badge foreground = %s, want dimmed token %s", fg, dimmedHex)
	}
}
