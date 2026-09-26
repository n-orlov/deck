package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// This file is task cure-01-03 (R142/R148, SPEC §11 and §11.9): review
// found three places where an intentional selection-follow operation left
// a wheel drift in force instead of ending it (or, for `F`, ended a drift
// for an action that changed nothing at all) -- see
// artifacts/review/reviewer-drift.log, force-header.log and
// new-selection-drift.log (all reproduced against ee7f5a5d3 through the
// reviewer_*_test.go probes those logs cite). Every assertion below is
// purely behavioural -- assertSelectionInView, assertDriftStillInForce and
// assertDriftEnded (wheel_drift_behaviour_test.go) -- never a direct read
// of m.sidebarScrollDrifted, so each test would still make a real
// assertion against a tree that carried no such field at all.

// TestWheelDriftFilterQueryEditEndsDriftThenBackgroundResortFollows is the
// filter half (reviewer_drift_test.go / reviewer-drift.log): opening the
// filter alone (a global, non-selection control) must leave a drift in
// force, but EDITING the query -- narrowing or widening the visible set --
// is an intentional selection-follow operation that ends it, so the very
// next real background reload (a status change that re-sorts the selected
// session to a new screen position, preserving its identity) follows the
// selection with the usual context margin instead of freezing the stale
// wheel offset.
func TestWheelDriftFilterQueryEditEndsDriftThenBackgroundResortFollows(t *testing.T) {
	m := drift005DriftedModel(t, 30, 24, 6)

	opened, _ := m.Update(key("/"))
	afterOpen := opened.(Model)
	assertDriftStillInForce(t, "opening the filter alone", afterOpen)

	edited, _ := afterOpen.Update(key("s"))
	afterEdit := edited.(Model)
	assertSelectionInView(t, afterEdit, "after a filter query edit")
	assertDriftEnded(t, "after a filter query edit", afterEdit)

	selected, ok := afterEdit.selectedSession()
	if !ok {
		t.Fatalf("no session selected after the filter query edit")
	}
	resorted := append([]store.Session(nil), afterEdit.baseSessions...)
	for i := range resorted {
		if resorted[i].ID == selected.ID {
			resorted[i].Status = "stopped"
		}
	}
	updated, _ := afterEdit.Update(sessionsLoaded{sessions: resorted})
	afterResort := updated.(Model)
	got, ok := afterResort.selectedSession()
	if !ok || got.ID != selected.ID {
		t.Fatalf("background re-sort changed the selected session's identity: %s -> %+v", selected.ID, got)
	}
	assertSelectionInView(t, afterResort, "after a filter query edit and a subsequent background re-sort")
}

// TestWheelDriftForceOnHeaderIsAsInertAsEnter is the force-attach half
// (reviewer_force_header_test.go / force-header.log): before this fix,
// "F" (SPEC §11.9's force-attach, `enter`'s own twin) was not in
// sessionScopedKeys, so guardSessionScopedKey's header refusal never
// applied to it -- a header cursor's own drift-clearing side effect fired
// unconditionally for `F` while `enter` (which IS in sessionScopedKeys)
// stayed refused and left the drift untouched. Both keys must behave
// identically on a header: neither moves the viewport nor ends the
// drift, and the `F` run below reuses the SAME drifted fixture `enter`
// already ran on rather than a fresh one, so nothing about the second
// Update's own starting sidebarScroll offset could quietly differ.
func TestWheelDriftForceOnHeaderIsAsInertAsEnter(t *testing.T) {
	fixture := drift005DriftedModel(t, 30, 24, 6)
	fixture.selected = headerCursor(7)
	before := fixture.sidebarScroll

	entered, _ := fixture.Update(key("enter"))
	e := entered.(Model)
	if e.sidebarScroll != before {
		t.Fatalf("enter moved the viewport on a header: %d -> %d", before, e.sidebarScroll)
	}
	assertDriftStillInForce(t, "enter on a header", e)

	forced, _ := fixture.Update(key("F"))
	f := forced.(Model)
	if f.sidebarScroll != before {
		t.Fatalf("F moved the viewport on a header: %d -> %d", before, f.sidebarScroll)
	}
	assertDriftStillInForce(t, "F on a header", f)
}

// TestWheelDriftNewSessionSelectionIntentEndsWheelDrift is the
// pending-create half (reviewer_new_selection_test.go /
// new-selection-drift.log): SPEC §11's newly-created-session visibility
// rule -- the just-created session is selected and visible -- is itself
// an intentional selection-follow operation, so fulfilling the one-shot
// pendingSelectSessionID intent ends any wheel drift in force rather than
// leaving the new row tracked off screen the way an ordinary, no-new-
// selection background reload would.
func TestWheelDriftNewSessionSelectionIntentEndsWheelDrift(t *testing.T) {
	m := drift005DriftedModel(t, 30, 24, 6)
	assertDriftStillInForce(t, "fixture before the new session arrives", m)

	created := m.baseSessions[0]
	created.ID = "cure-01-03-created"
	created.Name = "newly created"
	created.Status = "starting"
	m.pendingSelectSessionID = created.ID

	sessions := append([]store.Session(nil), m.baseSessions...)
	sessions = append(sessions, created)
	updated, _ := m.Update(sessionsLoaded{sessions: sessions})
	got := updated.(Model)

	chosen, ok := got.selectedSession()
	if !ok || chosen.ID != created.ID {
		t.Fatalf("the newly created session was not selected: %+v, ok=%v", chosen, ok)
	}
	assertSelectionInView(t, got, "the newly created session")
	assertDriftEnded(t, "the newly created session's own selection intent", got)
}
