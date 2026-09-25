package tui

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 005 (A.5, R142, GH #40, SPEC §11: "a wheel drift ...
// ends at the next key that moves or acts on the selection"). Task 004
// (r142_wheel_drift_test.go) proved a drift SURVIVES a background reload,
// re-sort or re-group; this file proves the other half of the same rule --
// a drift ENDS, with the selected row brought back into view (one row of
// context, the same margin followSelectionViewport already gives every
// ordinary selection move), on the very keypress that acts, for every key
// SPEC/the PRD name as session- or header-scoped plus every plain
// navigation key -- and does NOT end for the keys that name no selection
// (`?`, `q`, `<`, `>`, settings, filter toggles and the other global
// bindings; TestDriftPreservingKeysLeaveTheDriftInPlace and
// TestDriftPreservingGlobalKeysLeaveTheDriftInPlace).
//
// Before this task's fix (HEAD 5dfbdfc), guardSessionScopedKey only ever
// answered the header question; nothing anywhere cleared
// m.sidebarScrollDrifted or re-followed the viewport on any of these keys,
// so every one of driftEndingKeyFollowTestCases below failed against that
// tree (see artifacts/probes/005-drift-ending-keys-fail-5dfbdfc.log).

// drift005FixtureModel builds n sessions in one real group, tall enough
// that a wheel notch actually moves sidebarScroll, with every
// session-scoped handler's own service function wired to a real,
// non-nil stub (mirroring sessionScopedKeyGuardFixture) so a key that
// reaches its own handler body runs it instead of degrading on a nil
// dependency check -- this test cares whether the VIEWPORT reacted, not
// whether each handler's own downstream action fully succeeds.
func drift005FixtureModel(n, height int) Model {
	grpID := int64(7)
	var sessions []store.Session
	for i := 0; i < n; i++ {
		sessions = append(sessions, store.Session{
			ID:                fmt.Sprintf("s%02d", i),
			Name:              fmt.Sprintf("s%02d", i),
			CWD:               "/work/grp",
			GroupName:         "grp",
			GroupID:           &grpID,
			Status:            "running",
			Agent:             "claude",
			PermissionProfile: "default",
			ResumeState:       "auto",
		})
	}
	m := New(nil, config.Settings{Mouse: true}, "")
	m.sessions = sessions
	m.baseSessions = sessions
	m.width, m.height = 80, height
	m.selected = rowCursor(0)
	m.selectedByUser = true
	m.kill = func(context.Context, store.Session) error { return nil }
	m.acknowledge = func(context.Context, string) error { return nil }
	m.resume = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		return store.Session{}, service.ResumeOutcome(0), nil
	}
	m.restart = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		return store.Session{}, service.ResumeOutcome(0), nil
	}
	m.profileSwitch = func(context.Context, string, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.resumeMode = func(context.Context, string, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.archiveSvc = func(context.Context, store.Session) error { return nil }
	m.unarchiveSvc = func(context.Context, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.attach = func(context.Context, string) (*exec.Cmd, error) {
		return exec.Command("true"), nil
	}
	return m
}

// drift005DriftedModel returns drift005FixtureModel(n, height) after the
// selection (row 0, pinned at the very top) has been wheel-drifted away:
// enough "wheel down" notches to leave sidebarScroll well away from 0
// while the selection itself never moves, exactly the setup R142/GH #40's
// drift rule describes.
func drift005DriftedModel(t *testing.T, n, height, notches int) Model {
	t.Helper()
	m := drift005FixtureModel(n, height)
	for i := 0; i < notches; i++ {
		next, _ := m.Update(wheelDown(10, 5))
		m = next.(Model)
	}
	if m.sidebarScroll == 0 {
		t.Fatalf("fixture: %d wheel-down notches left sidebarScroll at 0", notches)
	}
	return m
}

// assertDriftEndedWithContext is the shared assertion every case in
// TestDriftEndingKeysBringSelectionBackIntoView and
// TestWheelDriftDDRaisesConfirmWithTargetVisible make, all of it through
// observable behaviour (no internal drift field, so it runs to a real
// assertion on a tree with no drift state too): the selected cursor's own
// entry span is fully inside the sidebar's visible window
// (assertSelectionInView), the resulting sidebarScroll is EXACTLY what a
// fresh followSelectionViewport call would produce right now -- i.e. it
// already carries the one-row context margin followSelectionViewport
// gives every ordinary selection move, not merely "somewhere that happens
// to include the row" -- and the drift is really over, not merely paused:
// the next background reload must follow a selection that has gone off
// screen back into view.
func assertDriftEndedWithContext(t *testing.T, label string, m Model) {
	t.Helper()
	assertSelectionInView(t, m, label)
	before := m.sidebarScroll
	m.followSelectionViewport()
	if m.sidebarScroll != before {
		t.Fatalf("%s: sidebarScroll = %d is not what followSelectionViewport's own one-row-context margin computes (%d) -- the row is visible, but without the required context", label, before, m.sidebarScroll)
	}
	m.sidebarScroll = before
	assertDriftEnded(t, label, m)
}

// driftEndingKeyFollowTestCases is task 005's own table: every
// sessionScopedKeys entry (bar "detail:g", a synthetic key with no real
// tea.KeyMsg -- see TestDriftEndingDetailGKey below), the header fold keys
// (c/left/right), `F` (force-attach, SPEC §11's list names it beside `↵`),
// and every plain navigation key.
func driftEndingKeyFollowTestCases() []string {
	var keys []string
	for k := range sessionScopedKeys {
		if k == "detail:g" {
			continue
		}
		keys = append(keys, k)
	}
	keys = append(keys, "F", "c", "left", "right")
	keys = append(keys, "up", "down", "k", "j", "pgup", "pgdown", "g", "G", " ")
	return keys
}

// TestDriftEndingKeysBringSelectionBackIntoView is success criterion 1:
// from a drift, every one of driftEndingKeyFollowTestCases brings the
// selected row into view with one row of context and clears the drift, on
// the very keypress that acts.
func TestDriftEndingKeysBringSelectionBackIntoView(t *testing.T) {
	for _, k := range driftEndingKeyFollowTestCases() {
		t.Run(fmt.Sprintf("key=%q", k), func(t *testing.T) {
			m := drift005DriftedModel(t, 30, 24, 6)
			updated, _ := m.Update(key(k))
			out, ok := updated.(Model)
			if !ok {
				t.Fatalf("Update(%q) returned %T, not tui.Model", k, updated)
			}
			assertDriftEndedWithContext(t, fmt.Sprintf("key %q", k), out)
		})
	}
}

// TestDriftEndingDetailGKey covers the one entry
// driftEndingKeyFollowTestCases can't drive through Update:
// sessionScopedKeys' synthetic "detail:g" (rename.go's `i`-detail move-
// group picker), which guardSessionScopedKey answers identically to every
// real key above -- it is called directly, with the `i` detail dialog
// open, exactly as rename.go's own case "g" calls it.
func TestDriftEndingDetailGKey(t *testing.T) {
	m := drift005DriftedModel(t, 30, 24, 6)
	m.detail = true
	if m.guardSessionScopedKey("detail:g") {
		t.Fatalf(`guardSessionScopedKey("detail:g") refused with a selected session in force`)
	}
	assertDriftEndedWithContext(t, `guardSessionScopedKey("detail:g")`, m)
}

// TestWheelDriftDDRaisesConfirmWithTargetVisible is success criterion 3:
// the dd chord's FIRST `d` (from a drift) already brings the target row
// into view and clears the drift -- acting on that same press, before the
// SECOND `d` ever opens the confirm dialog -- and the confirm opens with
// the same row still in view.
func TestWheelDriftDDRaisesConfirmWithTargetVisible(t *testing.T) {
	m := drift005DriftedModel(t, 30, 24, 6)
	targetID := m.sessions[0].ID

	firstUpdated, cmd := m.Update(key("d"))
	if cmd != nil {
		t.Fatalf("the first d of dd returned a non-nil tea.Cmd")
	}
	first, ok := firstUpdated.(Model)
	if !ok {
		t.Fatalf("Update(\"d\") returned %T, not tui.Model", firstUpdated)
	}
	if !first.pendingDelete {
		t.Fatalf("the first d of dd did not raise m.pendingDelete")
	}
	// Criterion 3's "acting on the same press": the FIRST d already ends
	// the drift and brings the target row into view, not merely the
	// second one that opens the confirm.
	assertDriftEndedWithContext(t, "first d of dd", first)

	secondUpdated, _ := first.Update(key("d"))
	second, ok := secondUpdated.(Model)
	if !ok {
		t.Fatalf("Update(\"d\") (second) returned %T, not tui.Model", secondUpdated)
	}
	if !second.deleteConfirming {
		t.Fatalf("the second d of dd did not open the delete confirm")
	}
	if second.pendingDelete {
		t.Fatalf("the second d of dd left m.pendingDelete set")
	}
	idx, ok := second.selected.SessionIndex()
	if !ok || idx < 0 || idx >= len(second.sessions) || second.sessions[idx].ID != targetID {
		t.Fatalf("the confirm opened for the wrong session: cursor %+v, want id %q", second.selected, targetID)
	}
	assertDriftEndedWithContext(t, "second d of dd (confirm open)", second)
}

// assertDriftLeftInPlace runs one key from a fresh drift and fails unless
// the wheel's own sidebarScroll survives it AND the drift itself does:
// the next background reload must still keep the wheel's offset.
func assertDriftLeftInPlace(t *testing.T, k string) {
	t.Helper()
	m := drift005DriftedModel(t, 30, 24, 6)
	driftedScroll := m.sidebarScroll
	updated, _ := m.Update(key(k))
	out, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update(%q) returned %T, not tui.Model", k, updated)
	}
	if out.sidebarScroll != driftedScroll {
		t.Fatalf("key %q moved sidebarScroll from %d to %d while claiming to leave the drift in place", k, driftedScroll, out.sidebarScroll)
	}
	// SPEC §11 says a key that names no selection leaves the drift alone:
	// a background reload arriving right after the key must still keep
	// the wheel's offset, not snap it back onto the selection.
	after := reloadSameSessions(out)
	if after.sidebarScroll != driftedScroll {
		t.Fatalf("key %q, then a background reload: sidebarScroll moved %d -> %d -- the drift did not survive the key", k, driftedScroll, after.sidebarScroll)
	}
	assertDriftStillInForce(t, fmt.Sprintf("key %q, then a reload", k), after)
}

// TestDriftPreservingKeysLeaveTheDriftInPlace is success criterion 4: the
// keys SPEC's drift rule names outright as naming no selection -- `?`
// (help), `q` (quit), `<`/`>` (sidebar WIDTH, never scroll or selection).
func TestDriftPreservingKeysLeaveTheDriftInPlace(t *testing.T) {
	for _, k := range []string{"?", "q", "<", ">"} {
		t.Run(fmt.Sprintf("key=%q", k), func(t *testing.T) {
			assertDriftLeftInPlace(t, k)
		})
	}
}

// TestDriftPreservingGlobalKeysLeaveTheDriftInPlace covers the rest of
// SPEC §11's "keys that name no selection (... settings) leave the drift
// alone" and the PRD R142's "settings, filter toggles": `,` (settings),
// `/` (the filter field), `t` (theme picker), plus the other global
// bindings that act on no selected row -- `n` (create), `E` (event log),
// `|` (layout mode), `u` (undo the last kill, whatever is selected),
// `esc` (clears marks/held filter) and `ctrl+c`. The first attempt at this
// task ended the drift on every one of these (an exclusion list that only
// spared ?/q/ctrl+c/</>), which review caught for `,`, `/` and `t`.
func TestDriftPreservingGlobalKeysLeaveTheDriftInPlace(t *testing.T) {
	for _, k := range []string{",", "/", "t", "n", "E", "|", "u", "esc", "ctrl+c"} {
		t.Run(fmt.Sprintf("key=%q", k), func(t *testing.T) {
			assertDriftLeftInPlace(t, k)
		})
	}
}
