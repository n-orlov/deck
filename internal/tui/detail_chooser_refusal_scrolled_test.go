package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestDetailChooserRefusalVisibleWithScrollableDetail is cure-01-01-2's own
// cure for a gap TestDetailChooserRefusalIsVisibleInDetail (cure-01-02)
// never exercised: that test's session is short enough that the whole
// detail body fits inside one 80x24 page, so detailScroll never has to
// move for the refusal to already be on screen. A crashed shell with a
// full 200-line CrashTail (renderCrashTail's own durable capture, tui.go)
// pushes the body far past dialogContentBudget -- detailBody appends
// m.attachError AFTER the crash tail and BEFORE the two footer legend
// lines, so with the dialog left at its OWN opening scroll position
// (detailScroll==0, i.e. the reviewer's finding: the refusal existed only
// off-screen, at the very bottom of a body the visible top of which never
// moved) the refusal was invisible even though it was set. rename.go's
// case "P"/"c" now jumps m.detailScroll to the body's own dialogMaxScroll
// the instant the refusal fires, so the still-open frame always lands on
// the trailing page carrying both the refusal and the footer legend --
// this asserts that holds starting from EVERY reachable scroll position,
// not just 0: unscrolled, mid-scroll (a PgDown already taken before P/c is
// pressed) and already at the bottom.
func TestDetailChooserRefusalVisibleWithScrollableDetail(t *testing.T) {
	for _, tc := range []struct {
		key  string
		full string
	}{
		{"P", "Cannot change permission profile"},
		{"c", "Cannot change resume mode"},
	} {
		for _, startAtBottom := range []bool{false, true} {
			for _, pgDownsFirst := range []int{0, 1, 3} {
				t.Run(tc.key, func(t *testing.T) {
					noop := func(context.Context, string, string) (store.Session, error) {
						return store.Session{}, nil
					}
					m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
						nil, config.Settings{}, "", nil, nil, nil, nil, nil, noop, noop,
					)
					m.width, m.height = 80, 24
					exitStatus := 1
					m.sessions = []store.Session{{
						ID:             "crashed",
						Name:           "crashed-shell",
						Agent:          "shell",
						CWD:            "/tmp/project",
						Status:         "error",
						StatusReason:   "pane exited with status 1",
						StatusSource:   "tmux",
						StatusAt:       1,
						PaneExitStatus: &exitStatus,
						// A full 200-line crash tail (renderCrashTail's own
						// durable capture) is what actually pushes the body
						// past the 80x24 frame's dialogContentBudget --
						// without it the whole dialog fits on one page and
						// detailScroll can never move at all.
						CrashTail: strings.Repeat("shell diagnostic line, quite a lot of them\n", 200),
					}}
					m.selected = rowCursor(0)

					got, _ := m.Update(key("i"))
					m = got.(Model)
					if !m.detail {
						t.Fatal("i did not open the detail dialog")
					}

					if startAtBottom {
						m.detailScroll = m.dialogMaxScroll(m.detailBody())
					}
					for i := 0; i < pgDownsFirst; i++ {
						got, _ = m.Update(key("pgdown"))
						m = got.(Model)
					}
					if m.dialogMaxScroll(m.detailBody()) == 0 {
						t.Fatalf("fixture did not overflow the 80x24 frame; dialogMaxScroll == 0, this test proves nothing")
					}

					got, cmd := m.Update(key(tc.key))
					m = got.(Model)
					if !m.detail || m.profileSwitching || m.pinning || cmd != nil {
						t.Fatalf("%s on an ineligible session changed chooser state or dispatched a cmd (detail=%v profileSwitching=%v pinning=%v cmd=%v)", tc.key, m.detail, m.profileSwitching, m.pinning, cmd)
					}
					if !strings.Contains(m.attachError, tc.full) {
						t.Fatalf("m.attachError = %q, want it to contain %q", m.attachError, tc.full)
					}

					frame := stripANSI(m.View())
					if !strings.Contains(frame, tc.full) {
						t.Fatalf("%s's refusal is not visible in the immediate frame at detailScroll=%d (started at bottom=%v, %d pgdowns first); wanted %q in:\n%s", tc.key, m.detailScroll, startAtBottom, pgDownsFirst, tc.full, frame)
					}
					for _, line := range strings.Split(frame, "\n") {
						if n := len([]rune(line)); n > 80 {
							t.Fatalf("a rendered line is %d runes wide, want <= 80:\n%q", n, line)
						}
					}
					if got := strings.Count(frame, "\n") + 1; got > 24 {
						t.Fatalf("the rendered frame is %d lines tall, want <= 24:\n%s", got, frame)
					}
					if !strings.Contains(frame, "header cursor: c folds/unfolds its group") {
						t.Fatalf("the header-cursor footer line is missing once a scrolled refusal is shown:\n%s", frame)
					}
					if !strings.Contains(frame, "P switches permission profile") ||
						!strings.Contains(frame, "c changes resume mode") ||
						!strings.Contains(frame, "toggles pinned") ||
						!strings.Contains(frame, "r renames") ||
						!strings.Contains(frame, "l edits launch inputs") ||
						!strings.Contains(frame, "g moves") {
						t.Fatalf("the P/c/p/r/l/g key-help legend is missing once a scrolled refusal is shown:\n%s", frame)
					}
				})
			}
		}
	}
}
