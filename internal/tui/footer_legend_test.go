package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestFooterKeyLegendReflectsEligibility is task 013's own test (PRD R80):
// footerKeyLegend must show only the keys the current selection (or, for
// the one action that switches to it, the marked set) actually accepts,
// and it must both ADD and DROP keys correctly -- a test that only checks
// the keys that should be there would pass on a footer that still shows
// `x` on a stopped row (the exact defect R80 names), so every case below
// asserts both presence and absence.
func TestFooterKeyLegendReflectsEligibility(t *testing.T) {
	newModel := func(sessions []store.Session, selected int, marked map[string]bool) Model {
		m := New(nil, config.Settings{}, "")
		m.sessions = sessions
		m.selected = selected
		m.marked = marked
		return m
	}

	t.Run("live row: kill and restart eligible, resume is not", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "live", Agent: "shell", Status: "running"}}, 0, nil)
		legend := m.footerKeyLegend()
		for _, want := range []string{"Y acknowledge", "x kill", "R relaunch"} {
			if !strings.Contains(legend, want) {
				t.Errorf("live row: legend missing %q: %q", want, legend)
			}
		}
		if strings.Contains(legend, "r resume") {
			t.Errorf("live row: legend must not offer r resume: %q", legend)
		}
	})

	t.Run("stopped row: resume eligible, kill and restart are not", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "stopped", Agent: "shell", Status: "stopped"}}, 0, nil)
		legend := m.footerKeyLegend()
		if !strings.Contains(legend, "r resume") {
			t.Errorf("stopped row: legend missing r resume: %q", legend)
		}
		for _, unwanted := range []string{"x kill", "R relaunch"} {
			if strings.Contains(legend, unwanted) {
				t.Errorf("stopped row: legend must not offer %q: %q", unwanted, legend)
			}
		}
	})

	t.Run("archived row: eligibility follows status, not the archive flag", func(t *testing.T) {
		// canKill/canResume/canRestart (task 012) key off Status alone; an
		// archived row's own status is still whatever it was when it was
		// archived (store.Session's own invariant -- see
		// internal/store/archive_test.go), so a stopped-and-archived row
		// behaves exactly like the plain stopped case above: this pins
		// that the footer does not invent a second, archive-aware rule.
		m := newModel([]store.Session{{ID: "s1", Name: "archived", Agent: "shell", Status: "stopped", ArchivedAt: 100}}, 0, nil)
		legend := m.footerKeyLegend()
		if !strings.Contains(legend, "r resume") {
			t.Errorf("archived+stopped row: legend missing r resume: %q", legend)
		}
		if strings.Contains(legend, "x kill") {
			t.Errorf("archived+stopped row: legend must not offer x kill: %q", legend)
		}
	})

	t.Run("attention-pending row (waiting): kill and restart eligible, resume is not", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "waiting", Agent: "claude", Status: "waiting"}}, 0, nil)
		legend := m.footerKeyLegend()
		for _, want := range []string{"x kill", "R relaunch"} {
			if !strings.Contains(legend, want) {
				t.Errorf("waiting row: legend missing %q: %q", want, legend)
			}
		}
		if strings.Contains(legend, "r resume") {
			t.Errorf("waiting row: legend must not offer r resume: %q", legend)
		}
	})

	t.Run("marked set: x asks the marked rows, not the selected one", func(t *testing.T) {
		sessions := []store.Session{
			{ID: "s1", Name: "marked-stopped-1", Agent: "shell", Status: "stopped"},
			{ID: "s2", Name: "marked-stopped-2", Agent: "shell", Status: "stopped"},
			{ID: "s3", Name: "unmarked-live", Agent: "shell", Status: "running"},
		}
		// Every MARKED row is stopped (canKill false for each), even though
		// the SELECTED row (unmarked, index 2) is live -- x must ask the
		// marked set here, exactly as x's own batch handler does, so it
		// must disappear despite the selected row being killable.
		m := newModel(sessions, 2, map[string]bool{"s1": true, "s2": true})
		legend := m.footerKeyLegend()
		if strings.Contains(legend, "x kill") {
			t.Errorf("all-stopped marked set: legend must not offer x kill: %q", legend)
		}

		// r/R never switch to the marked set (their own key handlers act
		// on the selected row regardless of any mark), so the selected
		// live row still drives them: r absent, R present.
		if strings.Contains(legend, "r resume") {
			t.Errorf("marked set with a live selected row: legend must not offer r resume: %q", legend)
		}
		if !strings.Contains(legend, "R relaunch") {
			t.Errorf("marked set with a live selected row: legend missing R relaunch: %q", legend)
		}

		// Mark a live row too: now at least one marked row is killable, so
		// x must come back.
		m.marked["s3"] = true
		legend = m.footerKeyLegend()
		if !strings.Contains(legend, "x kill") {
			t.Errorf("marked set with one killable row: legend missing x kill: %q", legend)
		}
	})

	t.Run("empty list: every predicated per-row key disappears", func(t *testing.T) {
		m := newModel(nil, 0, nil)
		legend := m.footerKeyLegend()
		for _, unwanted := range []string{"Y acknowledge", "x kill", "r resume", "R relaunch"} {
			if strings.Contains(legend, unwanted) {
				t.Errorf("empty list: legend must not offer %q: %q", unwanted, legend)
			}
		}
		// Global commands that never act on a row stay, exactly as before.
		for _, want := range []string{"n new", "? help", "q quit"} {
			if !strings.Contains(legend, want) {
				t.Errorf("empty list: legend missing global key %q: %q", want, legend)
			}
		}
	})
}

// TestFooterLineSharesLineWithLongStatusReason is R80's other half: the
// footer's key legend and the selected row's status reason (§11.3) are
// joined into exactly one line, by plain concatenation with no
// width-aware truncation (footerLine only ever truncates the separate
// below-minimum notice) -- so the join must not drop or split either half
// regardless of how long the reason is, at deck's 80-column minimum.
func TestFooterLineSharesLineWithLongStatusReason(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "shell", Status: "stopped"}}
	m.selected = 0

	line := m.footerLine()
	if strings.Contains(line, "\n") {
		t.Fatalf("footer line must stay exactly one line, got %q", line)
	}
	if !strings.Contains(line, "resumable") {
		t.Fatalf("footer line dropped the selected row's status reason: %q", line)
	}
	if !strings.Contains(line, "r resume") {
		t.Fatalf("footer line dropped the resume key beside the reason: %q", line)
	}

	// The join formula is `reason + "    " + keys`, unconditionally. Swap
	// in a same-shape reason as long as SPEC §11.3's own example --
	// `pane failed after the stale frame`, launch_lease.feature's
	// non-leasable-error reason text -- and confirm both halves still
	// survive intact.
	longReason := "pane failed after the stale frame"
	assembled := longReason + "    " + m.footerKeyLegend()
	if strings.Contains(assembled, "\n") {
		t.Fatalf("long-reason join must stay one line, got %q", assembled)
	}
	if !strings.Contains(assembled, longReason) {
		t.Fatalf("long-reason join lost the reason text: %q", assembled)
	}
	if !strings.Contains(assembled, "r resume") {
		t.Fatalf("long-reason join lost the resume key: %q", assembled)
	}
}
