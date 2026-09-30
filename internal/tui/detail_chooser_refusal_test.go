package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R161/R162: canSwitchProfile's and canPinResume's refusal (rename.go's
// case "P"/"c") sets m.attachError and returns without ever opening the
// profile picker or the resume-mode chooser and without writing any
// session state -- but detailBody never rendered m.attachError, so a
// reader watching the still-open detail dialog never saw the refusal at
// all; the message only existed in the model field a test could inspect.
// This proves both routes (`i` then `P`, `i` then `c`) against a shell
// session, which SPEC §5/§8 name as having no permission profile and no
// conversation id to lock or restart fresh, at the 80x24 minimum frame
// size, and that the dialog stays open (no chooser opened) with no
// session-state write attempted (the noop callback is never invoked to
// mutate anything visible, and cmd is nil -- neither route ever returns a
// tea.Cmd on this refusal branch).
func TestDetailChooserRefusalIsVisibleInDetail(t *testing.T) {
	for _, tc := range []struct {
		key  string
		full string
		// visible is a prefix short enough to survive the detail box's own
		// word-wrap (the box body wraps at well under 80 columns) --
		// checked against the rendered frame, while full is checked
		// against the unwrapped m.attachError field itself.
		visible string
	}{
		{"P", "Cannot change permission profile: shell has no permission profile", "Cannot change permission profile"},
		{"c", "Cannot change resume mode: shell has no conversation id to lock or restart fresh", "Cannot change resume mode"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			noop := func(context.Context, string, string) (store.Session, error) {
				return store.Session{}, nil
			}
			m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
				nil, config.Settings{}, "", nil, nil, nil, nil, nil, noop, noop,
			)
			m.width, m.height = 80, 24
			m.sessions = []store.Session{{ID: "refusal", Name: "shell-row", Agent: "shell", Status: "running", CWD: "/tmp"}}
			m.selected = rowCursor(0)

			got, _ := m.Update(key("i"))
			m = got.(Model)
			if !m.detail {
				t.Fatal("i did not open the detail dialog")
			}

			got, cmd := m.Update(key(tc.key))
			m = got.(Model)

			if !m.detail {
				t.Fatalf("%s on an ineligible session closed the detail dialog", tc.key)
			}
			if m.profileSwitching {
				t.Fatalf("%s on an ineligible session opened the profile picker", tc.key)
			}
			if m.pinning {
				t.Fatalf("%s on an ineligible session opened the resume-mode chooser", tc.key)
			}
			if cmd != nil {
				t.Fatalf("%s on an ineligible session dispatched a tea.Cmd, want a pure no-op refusal", tc.key)
			}
			if !strings.Contains(m.attachError, tc.full) {
				t.Fatalf("m.attachError = %q, want it to contain %q", m.attachError, tc.full)
			}

			frame := stripANSI(m.View())
			if !strings.Contains(frame, tc.visible) {
				t.Fatalf("%s's refusal is not visible while detail remains open; wanted %q in:\n%s", tc.key, tc.visible, frame)
			}
			// The frame stays within the 80x24 budget the dialog was
			// asked to render at -- the refusal line must fit inside the
			// existing box, never push it past the terminal.
			for _, line := range strings.Split(frame, "\n") {
				if n := len([]rune(line)); n > 80 {
					t.Fatalf("a rendered line is %d runes wide, want <= 80:\n%q", n, line)
				}
			}
			if got := strings.Count(frame, "\n") + 1; got > 24 {
				t.Fatalf("the rendered frame is %d lines tall, want <= 24:\n%s", got, frame)
			}
			// The existing key-help footer (task 015/D.4's header-cursor
			// line and the dialog's own P/c/p/r/l/g legend) must still be
			// present -- the refusal is an addition, never a replacement.
			if !strings.Contains(frame, "header cursor: c folds/unfolds its group") {
				t.Fatalf("the header-cursor footer line is missing once a refusal is shown:\n%s", frame)
			}
			if !strings.Contains(frame, "P switches permission profile") ||
				!strings.Contains(frame, "c changes resume mode") ||
				!strings.Contains(frame, "toggles pinned") ||
				!strings.Contains(frame, "r renames") ||
				!strings.Contains(frame, "l edits launch inputs") ||
				!strings.Contains(frame, "g moves") {
				t.Fatalf("the P/c/p/r/l/g key-help legend is missing once a refusal is shown:\n%s", frame)
			}
		})
	}
}
