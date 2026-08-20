package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestPreviewBodyLinesNoLivePaneStates covers SPEC requirement 26 (task
// 020): an `error` row shows the durable crash tail headed by copy stating
// it is the last output before the exit and not live; `stopped`,
// `archived` and a `starting` row with no pane yet each render a one-line
// placeholder naming the state, and none of the four ever leaks the
// generic "no live preview captured for this row yet" copy that would
// have applied to every non-live status before this task.
func TestPreviewBodyLinesNoLivePaneStates(t *testing.T) {
	const width, height = 30, 6

	cases := []struct {
		name        string
		session     store.Session
		wantContain string // substring the rendered body must contain
	}{
		{
			name: "error shows crash tail headed as last output before exit",
			session: store.Session{
				ID:        "s-error",
				Status:    "error",
				CrashTail: "panic: boom\ngoroutine 1 [running]:\nmain.main()",
			},
			wantContain: "not live",
		},
		{
			name:        "stopped shows a one-line placeholder naming stopped",
			session:     store.Session{ID: "s-stopped", Status: "stopped"},
			wantContain: "stopped",
		},
		{
			name:        "archived shows a one-line placeholder naming archived",
			session:     store.Session{ID: "s-archived", Status: "archived"},
			wantContain: "archived",
		},
		{
			name:        "starting with no pane yet shows a one-line placeholder naming starting",
			session:     store.Session{ID: "s-starting", Status: "starting"},
			wantContain: "starting",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{}, "")
			m.sessions = []store.Session{tc.session}
			m.selected = 0
			// Never captured for this session id: previewLive stays false
			// so the placeholder branch, never the live crop, is exercised.

			lines := m.previewBodyLines(width, height)
			if len(lines) != height {
				t.Fatalf("len(lines) = %d, want %d", len(lines), height)
			}
			for _, l := range lines {
				if got := stringWidth(l); got > width {
					t.Fatalf("line %q has display width %d, exceeds panel width %d", l, got, width)
				}
			}
			joined := strings.ToLower(strings.Join(lines, " "))
			if !strings.Contains(joined, strings.ToLower(tc.wantContain)) {
				t.Fatalf("body %q does not mention %q", joined, tc.wantContain)
			}
			if strings.Contains(joined, "no live preview captured for this row yet") {
				t.Fatalf("body %q leaked the old generic no-live-preview copy", joined)
			}
		})
	}
}

// TestCrashTailPreviewLinesAnchorsToNewestLines proves the crash tail is
// anchored to its newest (last) lines when the stored tail has more lines
// than the panel has room for, mirroring requirement 23's own bottom
// anchoring, and that the header always survives even under a tight
// height budget.
func TestCrashTailPreviewLinesAnchorsToNewestLines(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	tail := "line1\nline2\nline3\nline4\nline5"
	const width, height = 20, 3

	lines := m.crashTailPreviewLines(tail, width, height)
	joined := strings.Join(lines, " ")
	if !strings.Contains(strings.ToLower(joined), "not live") {
		t.Fatalf("crash tail body %q missing not-live header", joined)
	}
	if !strings.Contains(joined, "line5") {
		t.Fatalf("crash tail body %q does not keep the newest line", joined)
	}
	if strings.Contains(joined, "line1") {
		t.Fatalf("crash tail body %q kept the oldest line instead of anchoring to the newest", joined)
	}
}

// TestCrashTailPreviewLinesEmptyTail proves an error row with no stored
// crash tail still gets the not-live header rather than silently showing
// nothing, so it is never mistaken for a live but blank pane.
func TestCrashTailPreviewLinesEmptyTail(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	lines := m.crashTailPreviewLines("", 30, 4)
	joined := strings.ToLower(strings.Join(lines, " "))
	if !strings.Contains(joined, "not live") {
		t.Fatalf("body %q missing not-live header", joined)
	}
	if !strings.Contains(joined, "no crash output") {
		t.Fatalf("body %q missing no-crash-output copy", joined)
	}
}
