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

	t.Run("live row: kill, restart, attach and interactive eligible, resume is not", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "live", Agent: "shell", Status: "running"}}, 0, nil)
		legend := m.footerKeyLegend()
		for _, want := range []string{"Y acknowledge", "x kill", "R relaunch", "↵ interactive", "a attach", "i detail"} {
			if !strings.Contains(legend, want) {
				t.Errorf("live row: legend missing %q: %q", want, legend)
			}
		}
		if strings.Contains(legend, "r resume") {
			t.Errorf("live row: legend must not offer r resume: %q", legend)
		}
		// A shell has no permission profile and no conversation id
		// (agentCapabilities), and both handlers refuse it saying so, so
		// neither key belongs here.
		for _, unwanted := range []string{"P profile", "p pin"} {
			if strings.Contains(legend, unwanted) {
				t.Errorf("live shell row: legend must not offer %q: %q", unwanted, legend)
			}
		}
	})

	t.Run("live agent row: profile and pin come back when the agent has them", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "live-agent", Agent: "claude", Status: "running", PermissionProfile: "safe"}}, 0, nil)
		legend := m.footerKeyLegend()
		for _, want := range []string{"P profile", "p pin"} {
			if !strings.Contains(legend, want) {
				t.Errorf("claude row: legend missing %q: %q", want, legend)
			}
		}
	})

	t.Run("stopped row: resume eligible, kill/restart/attach/interactive are not", func(t *testing.T) {
		m := newModel([]store.Session{{ID: "s1", Name: "stopped", Agent: "shell", Status: "stopped"}}, 0, nil)
		legend := m.footerKeyLegend()
		if !strings.Contains(legend, "r resume") {
			t.Errorf("stopped row: legend missing r resume: %q", legend)
		}
		// `↵` and `a` both refuse a stopped row with "resume it first"
		// (canReachPane, shared with enterInteractive/attachSelected), so
		// neither may be advertised here.
		for _, unwanted := range []string{"x kill", "R relaunch", "↵ interactive", "a attach"} {
			if strings.Contains(legend, unwanted) {
				t.Errorf("stopped row: legend must not offer %q: %q", unwanted, legend)
			}
		}
		// The detail dialog describes a stopped row perfectly well.
		if !strings.Contains(legend, "i detail") {
			t.Errorf("stopped row: legend missing i detail: %q", legend)
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
		for _, want := range []string{"x kill", "R relaunch", "Y acknowledge", "a attach"} {
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

	t.Run("empty list: every per-row key disappears", func(t *testing.T) {
		m := newModel(nil, 0, nil)
		legend := m.footerKeyLegend()
		// Including the keys whose handlers refuse an empty list without
		// naming a status: `↵`/`a` (no row to reach), `i` (nothing to
		// describe), `P`/`p` (no row's agent to ask about).
		for _, unwanted := range []string{"Y acknowledge", "x kill", "r resume", "R relaunch", "↵ interactive", "a attach", "i detail", "P profile", "p pin"} {
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
// footer's key legend and the selected row's status reason (§11.3) share
// exactly ONE physical line, inside the terminal's own width, at deck's
// 80-column minimum -- including for a reason as long as §11.3's own
// example, `pane failed after the stale frame`. Both halves are present,
// neither is squeezed out, and no key is cut in half: an entry is either
// listed whole or dropped, with the elision marked.
func TestFooterLineSharesLineWithLongStatusReason(t *testing.T) {
	// Every eligible entry's own visible text, so an assertion can tell a
	// dropped entry (fine) from a half-rendered one (never).
	wholeEntries := func(m Model) []string {
		var out []string
		for _, e := range footerLegend {
			if e.eligible != nil && !e.eligible(m) {
				continue
			}
			text := m.glyph(e.unicodeKey, e.asciiKey)
			if e.hint != "" {
				text += " " + e.hint
			}
			out = append(out, text)
		}
		return out
	}
	// checkShared asserts the whole contract for one model: one line, no
	// wider than the terminal, both halves visible, every legend segment
	// on it a complete entry.
	checkShared := func(t *testing.T, m Model, reasonHead string) string {
		t.Helper()
		line := m.footerLine()
		if strings.Contains(line, "\n") {
			t.Fatalf("footer must stay exactly one line, got %q", line)
		}
		width, _ := m.frameSize()
		if got := stringWidth(line); got > width {
			t.Fatalf("footer is %d cells wide at width %d, so it wraps instead of sharing one line: %q", got, width, line)
		}
		if !strings.Contains(line, reasonHead) {
			t.Fatalf("footer dropped the selected row's status reason (wanted %q): %q", reasonHead, line)
		}
		marker := m.glyph("…", "...")
		legendPart := line[strings.Index(line, "    ")+4:]
		whole := wholeEntries(m)
		for _, seg := range strings.Split(legendPart, m.glyph(" · ", " - ")) {
			if seg == marker || seg == "" {
				continue
			}
			found := false
			for _, w := range whole {
				if seg == w {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("footer segment %q is not a complete legend entry (a clipped key advertises a key that does not exist); line: %q", seg, line)
			}
		}
		if !strings.Contains(legendPart, "↑/↓") {
			t.Fatalf("footer legend lost its first entry beside the reason: %q", line)
		}
		return line
	}

	t.Run("short reason: both halves in full", func(t *testing.T) {
		m := New(nil, config.Settings{}, "")
		m.width, m.height = 200, 24
		m.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "shell", Status: "stopped"}}
		m.selected = 0
		line := checkShared(t, m, "stopped · resumable")
		// With 200 columns nothing has to give: the whole legend is there,
		// tail entries included, and nothing is elided.
		if !strings.Contains(line, "q quit") || !strings.Contains(line, "r resume") {
			t.Fatalf("at 200 columns the whole legend must survive: %q", line)
		}
		if strings.Contains(line, "…") {
			t.Fatalf("at 200 columns nothing may be elided: %q", line)
		}
		if want := m.selectedRowReason() + "    " + m.footerKeyLegend(); line != want {
			t.Fatalf("wide footer is not the plain join:\n got %q\nwant %q", line, want)
		}
	})

	t.Run("80 columns, short reason", func(t *testing.T) {
		m := New(nil, config.Settings{}, "")
		m.width, m.height = 80, 24
		m.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "shell", Status: "stopped"}}
		m.selected = 0
		line := checkShared(t, m, "stopped · resumable")
		// The reason is short enough to survive whole here; the legend is
		// the half that has to give, and it must have given something (the
		// full stopped-row legend does not fit in 80 columns).
		if !strings.Contains(line, "r resume") {
			t.Fatalf("the stopped row's own key must survive at 80 columns: %q", line)
		}
		if !strings.Contains(line, "…") {
			t.Fatalf("the legend does not fit in 80 columns, so the elision must be marked: %q", line)
		}
		t.Logf("80-col stopped-row footer (%d cells): %q", stringWidth(line), line)
	})

	t.Run("80 columns, §11.3's long reason", func(t *testing.T) {
		// `pane failed after the stale frame` is SPEC §11.3's own example
		// of a long §7 reason and launch_lease.feature's non-leasable
		// error text; it reaches the footer as the selected row's stored
		// StatusReason, the way any §7 verdict does.
		m := New(nil, config.Settings{}, "")
		m.width, m.height = 80, 24
		m.sessions = []store.Session{{
			ID: "s1", Name: "sess", Agent: "shell", Status: "error",
			StatusReason: "pane failed after the stale frame",
		}}
		m.selected = 0
		if got := m.selectedRowReason(); !strings.Contains(got, "pane failed after the stale frame") {
			t.Fatalf("the row's stored status reason must reach the footer, got %q", got)
		}
		line := checkShared(t, m, "error · pane failed")
		if !strings.Contains(line, "…") {
			t.Fatalf("at 80 columns beside this reason something must be elided, and the elision marked: %q", line)
		}
		t.Logf("80-col long-reason footer (%d cells): %q", stringWidth(line), line)

		// The reason may be elided, but it may not be reduced to nothing:
		// both halves keep a real share of the line.
		reason, keys, ok := strings.Cut(line, "    ")
		if !ok {
			t.Fatalf("footer no longer separates reason from keys: %q", line)
		}
		if stringWidth(reason) < 10 || stringWidth(keys) < 10 {
			t.Fatalf("one half squeezed the other out: reason=%q keys=%q", reason, keys)
		}

		// End to end at exactly 80x24: this is mainView's last line, so the
		// shared footer is what a real 80-column terminal shows, and it
		// still fits on one row rather than wrapping into the frame.
		viewLines := strings.Split(m.mainView(), "\n")
		footer := viewLines[len(viewLines)-1]
		if footer != line {
			t.Fatalf("80x24 frame's last line is not the shared footer:\n got %q\nwant %q", footer, line)
		}
		if got := stringWidth(footer); got > 80 {
			t.Fatalf("80x24 frame's footer is %d cells wide: %q", got, footer)
		}
	})

	t.Run("80 columns, empty list: the reason goes with the rows", func(t *testing.T) {
		m := New(nil, config.Settings{}, "")
		m.width, m.height = 80, 24
		line := m.footerLine()
		if strings.Contains(line, "\n") {
			t.Fatalf("footer must stay one line, got %q", line)
		}
		if got := stringWidth(line); got > 80 {
			t.Fatalf("empty-list footer is %d cells wide: %q", got, line)
		}
		// Only the global keys are left, and they now fit without eliding
		// anything -- so the empty list is also the case that proves the
		// width fitting is not unconditional.
		for _, want := range []string{"↑/↓", "n new", "? help", "q quit"} {
			if !strings.Contains(line, want) {
				t.Fatalf("empty-list footer missing %q: %q", want, line)
			}
		}
		if strings.Contains(line, "…") {
			t.Fatalf("the global-only legend fits in 80 columns, so nothing may be elided: %q", line)
		}
	})
}
