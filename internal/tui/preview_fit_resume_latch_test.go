package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// R117: a relaunch that actually created a pane must clear the passive-fit
// latch, since previewFitSessionID's "already settled" claim about the OLD
// pane no longer holds for whatever pane the relaunch just created. A
// no-op resume/restart outcome (already running, starting elsewhere, not
// leasable) created no pane, so it must leave the latch exactly as it was.
//
// Every test below drives Model.Update directly on previewFitModel (from
// preview_fit_overlap_test.go): s1 selected, previewFitSessionID pre-set to
// "s1" to simulate a fit already settled for it, then a resume/restart/
// batch-undo outcome delivered, then a previewTick to observe whether a
// fit is (re-)issued with no selection change in between.

func resumeLatchModel(t *testing.T) Model {
	t.Helper()
	m := previewFitModel(t)
	m.previewFitSessionID = "s1"
	return m
}

func TestSessionResumedThatCreatedAPaneClearsTheLatch(t *testing.T) {
	m := resumeLatchModel(t)

	updated, _ := m.Update(sessionResumed{
		session: store.Session{ID: "s1", Slug: "one", Name: "one", Status: "running"},
		outcome: service.ResumeStarted,
	})
	m = updated.(Model)
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q after a sessionResumed that created a pane, want cleared", m.previewFitSessionID)
	}
	if m.previewFitInFlight != "" {
		t.Fatalf("previewFitInFlight = %q after sessionResumed, want untouched (empty)", m.previewFitInFlight)
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("previewTick after a pane-creating resume returned %d commands, want 2 (the reschedule plus a fit) with no selection change", len(got))
	}
	if m.previewFitInFlight != "s1" {
		t.Fatalf("previewFitInFlight = %q, want %q -- the relaunch's fit is the one now outstanding", m.previewFitInFlight, "s1")
	}
}

func TestSessionRestartedThatCreatedAPaneClearsTheLatch(t *testing.T) {
	m := resumeLatchModel(t)

	updated, _ := m.Update(sessionRestarted{
		session: store.Session{ID: "s1", Slug: "one", Name: "one", Status: "running"},
		outcome: service.ResumeStarted,
	})
	m = updated.(Model)
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q after a sessionRestarted that created a pane, want cleared", m.previewFitSessionID)
	}
	if m.previewFitInFlight != "" {
		t.Fatalf("previewFitInFlight = %q after sessionRestarted, want untouched (empty)", m.previewFitInFlight)
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("previewTick after a pane-creating restart returned %d commands, want 2 (the reschedule plus a fit) with no selection change", len(got))
	}
}

// noopResumeOutcomes covers every sessionResumed/sessionRestarted outcome
// that created no pane: the latch must survive every one of them
// untouched, and no fit must be issued on the following tick.
var noopResumeOutcomes = []service.ResumeOutcome{
	service.ResumeStartingElsewhere,
	service.ResumeAlreadyRunning,
	service.ResumeNotLeasable,
}

func TestSessionResumedNoopOutcomesLeaveTheLatchSetAndIssueNoFit(t *testing.T) {
	for _, outcome := range noopResumeOutcomes {
		outcome := outcome
		t.Run(fmt.Sprintf("outcome=%d", outcome), func(t *testing.T) {
			m := resumeLatchModel(t)

			updated, _ := m.Update(sessionResumed{
				session: store.Session{ID: "s1", Slug: "one", Name: "one", Status: "running"},
				outcome: outcome,
			})
			m = updated.(Model)
			if m.previewFitSessionID != "s1" {
				t.Fatalf("previewFitSessionID = %q after a no-op sessionResumed(%d), want left set at %q", m.previewFitSessionID, outcome, "s1")
			}
			if m.previewFitInFlight != "" {
				t.Fatalf("previewFitInFlight = %q after sessionResumed, want untouched (empty)", m.previewFitInFlight)
			}

			updated, cmd := m.Update(previewTick(time.Now()))
			m = updated.(Model)
			if got := previewTickCmds(t, cmd); len(got) != 1 {
				t.Fatalf("previewTick after a no-op resume(%d) with no selection change returned %d commands, want 1 (the reschedule alone, no fit)", outcome, len(got))
			}
		})
	}
}

func TestSessionRestartedNoopOutcomesLeaveTheLatchSetAndIssueNoFit(t *testing.T) {
	for _, outcome := range noopResumeOutcomes {
		outcome := outcome
		t.Run(fmt.Sprintf("outcome=%d", outcome), func(t *testing.T) {
			m := resumeLatchModel(t)

			updated, _ := m.Update(sessionRestarted{
				session: store.Session{ID: "s1", Slug: "one", Name: "one", Status: "running"},
				outcome: outcome,
			})
			m = updated.(Model)
			if m.previewFitSessionID != "s1" {
				t.Fatalf("previewFitSessionID = %q after a no-op sessionRestarted(%d), want left set at %q", m.previewFitSessionID, outcome, "s1")
			}
			if m.previewFitInFlight != "" {
				t.Fatalf("previewFitInFlight = %q after sessionRestarted, want untouched (empty)", m.previewFitInFlight)
			}

			updated, cmd := m.Update(previewTick(time.Now()))
			m = updated.(Model)
			if got := previewTickCmds(t, cmd); len(got) != 1 {
				t.Fatalf("previewTick after a no-op restart(%d) with no selection change returned %d commands, want 1 (the reschedule alone, no fit)", outcome, len(got))
			}
		})
	}
}

// TestSessionsBulkResumedClearsTheLatchOnlyForItsOwnPaneCreatingEntry
// covers the batch `u` undo (R112): the latch is cleared only when it
// names a session this batch actually relaunched a pane for (outcome
// ResumeStarted, no error), and left alone for every other entry.
func TestSessionsBulkResumedClearsTheLatchOnlyForItsOwnPaneCreatingEntry(t *testing.T) {
	m := resumeLatchModel(t)

	updated, _ := m.Update(sessionsBulkResumed{
		sessionIDs: []string{"s2", "s1"},
		outcomes:   []service.ResumeOutcome{service.ResumeAlreadyRunning, service.ResumeStarted},
		errs:       []error{nil, nil},
	})
	m = updated.(Model)
	if m.previewFitSessionID != "" {
		t.Fatalf("previewFitSessionID = %q after a batch resume that included s1's own pane-creating outcome, want cleared", m.previewFitSessionID)
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 2 {
		t.Fatalf("previewTick after the batch resume returned %d commands, want 2 (the reschedule plus a fit) with no selection change", len(got))
	}
}

func TestSessionsBulkResumedLeavesTheLatchWhenItNamesNoneOfIts(t *testing.T) {
	m := resumeLatchModel(t)

	updated, _ := m.Update(sessionsBulkResumed{
		sessionIDs: []string{"s2"},
		outcomes:   []service.ResumeOutcome{service.ResumeStarted},
		errs:       []error{nil},
	})
	m = updated.(Model)
	if m.previewFitSessionID != "s1" {
		t.Fatalf("previewFitSessionID = %q after a batch resume that never named s1, want left set at %q", m.previewFitSessionID, "s1")
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if got := previewTickCmds(t, cmd); len(got) != 1 {
		t.Fatalf("previewTick after a batch resume that never named s1 returned %d commands, want 1 (the reschedule alone, no fit)", len(got))
	}
}
