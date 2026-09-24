package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is cure-01-05 (review findings F6, R136/SPEC §11 "the
// viewport follows the selection", R139's settingsApplyLiveFields ->
// resortSessionsLive live-save path, and R137 "selection never names a
// hidden row or a filtered-out header"). Each test below is adapted from
// the review's own probe (artifacts/review64/probes.log,
// review_phase4c_test.go) that failed against the pre-cure tree; the
// review's own failure text for each is quoted in its doc comment. cure
// commit message names the exact `git apply -R` fail-before run.

// TestCure0105DefaultGroupFirstLiveSaveFollowsSelection is the review's
// TestReviewDefaultFirstFollowsSelectedCursorViaSettings: against the
// pre-cure tree this failed with "sidebarScroll = 44 leaves selection
// span [2,3] outside window [44,65)" (row) and "... span [1,1] outside
// window [44,65)" (header) -- settingsApplyLiveFields flipped
// m.settings.DefaultGroupFirst (which reorders every group bucket
// groupSessions() produces) without ever re-clamping m.sidebarScroll to
// the selection's new, drastically different rendered position.
func TestCure0105DefaultGroupFirstLiveSaveFollowsSelection(t *testing.T) {
	for _, kind := range []string{"row", "header"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := settingsLiveApplyTestModel(t)
			base := viewportFollowTestModel(30, 24)
			m.width, m.height = 80, 24
			m.sessions = append(base.sessions, store.Session{ID: "d", Name: "d", Status: "idle"})
			m.baseSessions = m.sessions
			if kind == "row" {
				m.setSelection(rowCursor(30))
			} else {
				m.setSelection(headerCursor(0))
			}
			m.settingsEdits = settingsEditsFromSettings(m.settings)
			m.settingsEdits.DefaultGroupFirst = true
			m.settingsSave()
			if !m.settings.DefaultGroupFirst {
				t.Fatal("flag not applied")
			}
			assertSelectionInView(t, m, "live default_group_first toggle on "+kind)
		})
	}
}

// TestCure0105HeaderRenameFollowsNewPosition is the review's
// TestReviewHeaderRenamePreservesCursorAndFollow: failed with "header
// moved by rename: sidebarScroll = 0 leaves selection span [61,61]
// outside window [0,21)" -- a rename that moves a header's alphabetical
// slot kept the cursor's identity (correct: same durable groupID) but
// sessionsLoaded never re-clamped the viewport to the header's new
// rendered position.
func TestCure0105HeaderRenameFollowsNewPosition(t *testing.T) {
	m := viewportFollowTestModel(30, 24)
	m.allGroups = []store.Group{{ID: 2, Name: "aaa"}}
	m.setSelection(headerCursor(2))
	renamed := append([]store.Session(nil), m.sessions...)
	next, _ := m.Update(sessionsLoaded{sessions: renamed, groups: []store.Group{{ID: 2, Name: "zzz"}}})
	got := next.(Model)
	if got.selected != headerCursor(2) {
		t.Fatalf("rename changed cursor: %+v", got.selected)
	}
	assertSelectionInView(t, got, "header moved by rename")
}

// TestCure0105HeaderOnlyLoadReachableByArrows is the review's
// TestReviewHeaderOnlyArrowNavigationAfterLoad: failed with "down after
// header-only load left invalid row cursor {kind:0 index:0 groupID:0};
// headers cannot be reached by arrows" -- a header-only sidebar (zero
// sessions, at least one group) left the fresh model's zero-value
// rowCursor(0) selected, which names no row at all, so nextVisibleSelection
// (an exact-match walk of visualOrder) found nothing to step from and
// down/j did nothing.
func TestCure0105HeaderOnlyLoadReachableByArrows(t *testing.T) {
	m := groupTestModel(nil)
	next, _ := m.Update(sessionsLoaded{groups: []store.Group{{ID: 7, Name: "empty"}}})
	m = next.(Model)
	next, _ = m.Update(key("down"))
	m = next.(Model)
	if !m.selected.IsHeader() {
		t.Fatalf("down after header-only load left invalid row cursor %+v; headers cannot be reached by arrows", m.selected)
	}
}

// TestCure0105FilteredReloadNeverSelectsAbsentHeader is the review's
// TestReviewFilteredReloadCannotLeaveCursorOnAbsentHeader: failed with
// "groupID 1 has no sidebar entry after losing its last filter match" --
// the selected header's own group lost its only filter-matching session
// on reload, which (correctly, per groupSessions' filtered-render rule)
// removes that group's bucket entirely, but sessionsLoaded had no
// preserve/normalize path for a HEADER cursor at all (only rows), so the
// stale header cursor was left naming a bucket that no longer exists.
func TestCure0105FilteredReloadNeverSelectsAbsentHeader(t *testing.T) {
	a, b := int64(1), int64(2)
	rows := []store.Session{
		{ID: "a", Name: "keep-a", GroupName: "aaa", GroupID: &a},
		{ID: "b", Name: "keep-b", GroupName: "bbb", GroupID: &b},
	}
	m := groupTestModel(rows)
	m.width, m.height = 80, 24
	m.baseSessions = rows
	m.filterQuery = "keep"
	m.selected = headerCursor(a)
	incoming := append([]store.Session(nil), rows...)
	incoming[0].Name = "gone"
	next, _ := m.Update(sessionsLoaded{sessions: incoming, groups: []store.Group{{ID: a, Name: "aaa"}, {ID: b, Name: "bbb"}}})
	got := next.(Model)
	if len(got.sessions) != 1 {
		t.Fatalf("fixture: filtered row count=%d", len(got.sessions))
	}
	assertSelectionInView(t, got, "filtered reload after selected group's last match disappears")
}

// TestCure0105RestartNeverSelectsCollapsedRow is the review's
// TestReviewRestartNeverSelectsCollapsedRow: failed with "first reload
// after restart selects hidden row {kind:0 index:0 groupID:0} in
// persisted collapsed group" -- a brand new Model's zero-value
// rowCursor(0) was never checked against the just-persisted fold before
// the FIRST reload after restart lands, so the very first frame selected
// a row its own collapsed group was hiding.
func TestCure0105RestartNeverSelectsCollapsedRow(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	group, err := db.CreateGroup(context.Background(), "folded")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "")
	m.setGroupCollapsed(group.ID, true)
	if msg := m.persistCollapsedGroups()().(uiStatePersisted); msg.err != nil {
		t.Fatal(msg.err)
	}
	fresh := New(db, config.Settings{}, "")
	next, _ := fresh.Update(sessionsLoaded{
		sessions: []store.Session{{ID: "s1", Name: "s1", GroupName: group.Name, GroupID: &group.ID}},
		groups:   []store.Group{group},
	})
	got := next.(Model)
	if !got.isStopVisible(got.selected) {
		t.Fatalf("first reload after restart selects hidden row %+v in persisted collapsed group", got.selected)
	}
}
