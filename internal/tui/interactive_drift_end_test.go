// interactive_drift_end_test.go covers R149/GH #47's second half (task
// 002): SPEC §11's "a wheel drift ends at the next key that moves or acts
// on the selection" extended to interactive mode's own input path
// (updateInteractive, internal/tui/interactive.go) and to Ctrl+Q/the
// empty-sidebar click's own exitInteractive. Before this task's fix,
// neither updateInteractive nor exitInteractive ever looked at
// m.sidebarScrollDrifted at all -- a wheel drift left armed over the
// sidebar while interactive mode owned the keyboard survived every key
// forwarded to the pane and survived leaving interactive mode entirely,
// so the very next background reload (or the freshly-drawn list-mode
// frame after Ctrl+Q) could show a viewport still parked wherever the
// wheel had last left it, with the selection nowhere on screen.
//
// Every positive case below drives the SAME follow
// guardSessionScopedKey's own drift-ending branch already uses
// (followSelectionViewport, session_scoped_guard.go) -- there is no
// second follow implementation to test separately, only that
// updateInteractive/exitInteractive now call it too, at the right time
// relative to forwarding.
package tui

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// driftEndDispatchFixtureModel builds a real, entered interactive Model --
// a live grid and dispatcher backed by a genuine tmux pane, exactly like
// interactive_forward_gate_test.go's own forwardGateTestModel -- except
// the sidebar list is 30 sessions long (forwardGateTestModel's is one),
// long enough that a real sidebar-wheel notch can drift the viewport well
// off the interactive target's own row at rowCursor(targetIdx), near the
// top of that list. Only the target session has a real tmux session
// behind it; the rest exist purely to give the sidebar room to scroll.
func driftEndDispatchFixtureModel(t *testing.T, socket, slug string, targetIdx int) Model {
	t.Helper()
	m := New(nil, config.Settings{Mouse: true, Color: true}, "")
	var sessions []store.Session
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("s%02d", i)
		sessions = append(sessions, store.Session{ID: id, Name: id, Slug: id, CWD: "/work/infra", Status: "idle"})
	}
	sessions[targetIdx] = store.Session{ID: "target-" + slug, Name: slug, Slug: slug, CWD: "/work", Status: "waiting"}
	m.sessions = sessions
	m.baseSessions = append([]store.Session(nil), sessions...)
	m.width, m.height = 100, 30
	m.selected = rowCursor(targetIdx)

	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil || got.interactiveDispatcher == nil {
		t.Fatalf("enterInteractive did not enter interactive mode with a live grid and dispatcher (grid=%v dispatcher=%v)", got.interactiveGrid, got.interactiveDispatcher)
	}
	t.Cleanup(func() { got.exitInteractive() })
	return got
}

// drift149ListFixtureModel builds n plain sessions (no real tmux behind
// any of them) with m.interactive set true by hand -- for the tests below
// that never touch the dispatcher (Ctrl+Q, the empty-sidebar click, a
// background reload, a preview wheel, drag-to-copy) and so need no real
// pane at all. m.interactiveGrid is deliberately left nil: a zero-value
// *interactive.Session's pollCancel is nil, and exitInteractive's own
// teardown would call it (Session.CloseLocal) and panic -- callers that
// need scrollInteractiveByLines/beginInteractiveSelection to see a live
// grid set m.interactiveGrid themselves, and never call exitInteractive
// on that same model.
func drift149ListFixtureModel(n, targetIdx int) Model {
	var sessions []store.Session
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("s%02d", i)
		sessions = append(sessions, store.Session{ID: id, Name: id, Slug: id, CWD: "/work/infra", Status: "idle"})
	}
	m := mouseTestModel(sessions)
	m.baseSessions = append([]store.Session(nil), sessions...)
	m.width, m.height = 100, 30
	m.selected = rowCursor(targetIdx)
	m.interactive = true
	return m
}

// armSidebarWheelDriftPastSelection arms m.sidebarScrollDrifted through
// the SAME production wheel path list mode's own scrollSidebar (mouse.go)
// handles -- called directly rather than routed through Update's
// interactive-mode mouse dispatch, so this fixture proves nothing about,
// and needs nothing from, task 001/R149's own sidebar-wheel ROUTING fix --
// and scrolls all the way to the bottom of the list, off the selection's
// own row near the top, so the selection is genuinely off screen
// afterward whenever the list is longer than the sidebar's own content
// window (asserted below rather than assumed).
func armSidebarWheelDriftPastSelection(t *testing.T, m Model) Model {
	t.Helper()
	if hit := m.hitTest(10, 5); hit.panel != hitPanelSidebar {
		t.Fatalf("test setup: (10,5) does not hit the sidebar panel: %+v", hit)
	}
	m = m.scrollSidebar(wheelDown(10, 5), 999)
	if !m.sidebarScrollDrifted {
		t.Fatalf("test setup: scrollSidebar did not arm m.sidebarScrollDrifted")
	}
	layout := m.computeLayout()
	contentWidth := sidebarEntryContentWidth(layout)
	contentHeight := layout.Sidebar.Height - 2
	entries := m.sidebarEntries(contentWidth)
	if len(entries) <= contentHeight {
		t.Fatalf("test setup: the whole sidebar fits on screen (%d entries, height %d) -- the drift cannot move the selection off screen", len(entries), contentHeight)
	}
	start, _ := cursorSpan(m, entries, m.selected)
	if start == -1 {
		t.Fatalf("test setup: selected cursor %+v has no sidebar entries", m.selected)
	}
	if start >= m.sidebarScroll && start < m.sidebarScroll+contentHeight {
		t.Fatalf("test setup: the wheel drift to the bottom left the selection (entry %d) inside the visible window [%d,%d)", start, m.sidebarScroll, m.sidebarScroll+contentHeight)
	}
	return m
}

// tmuxCapturePane reads target's current screen content, byte for byte
// (tmux capture-pane -p, the same primitive internal/tmux's own
// key/literal_send tests read the far end of a dispatched send with).
func tmuxCapturePane(t *testing.T, socket, target string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		t.Fatalf("tmux capture-pane -t %s: %v", target, err)
	}
	return string(out)
}

// tmuxCursorY reads target's current #{cursor_y} -- used for Enter's own
// byte (CR), which moves the cursor down a row via the pty's ICRNL/ECHO
// translation rather than printing a visible character.
func tmuxCursorY(t *testing.T, socket, target string) int {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", target, "#{cursor_y}").Output()
	if err != nil {
		t.Fatalf("tmux display-message #{cursor_y} -t %s: %v", target, err)
	}
	y, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if convErr != nil {
		t.Fatalf("parse #{cursor_y} %q: %v", string(out), convErr)
	}
	return y
}

// waitForPaneCaptureLine0 polls target's capture-pane content (a real
// send is asynchronous from this process's point of view: tmux's own
// event loop has to read the byte back off the pty before its screen
// model reflects it) until its first line equals want, settled on that
// durable fact rather than a fixed sleep.
func waitForPaneCaptureLine0(t *testing.T, socket, target, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for {
		last = tmuxCapturePane(t, socket, target)
		line0, _, _ := strings.Cut(last, "\n")
		if line0 == want {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane capture's first line never became %q within the deadline; last capture:\n%s", want, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForCursorY polls target's #{cursor_y} until it reaches want.
func waitForCursorY(t *testing.T, socket, target string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last int
	for {
		last = tmuxCursorY(t, socket, target)
		if last == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("#{cursor_y} never reached %d within the deadline (last=%d)", want, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestUpdateInteractiveForwardedRuneKeyEndsSidebarDriftAndForwardsSameByte
// is criterion 1's rune case: from a sidebar-wheel drift, an ordinary
// printable key both ends the drift (selection back in view) AND still
// reaches the dispatcher on the very same press, with the exact byte
// interactiveLiteralPayload says an "x" keypress forwards -- proven
// against the real pane's own echoed content, not merely a nil-error
// return.
func TestUpdateInteractiveForwardedRuneKeyEndsSidebarDriftAndForwardsSameByte(t *testing.T) {
	socket := selectionTestSocket("drift149-rune")
	slug := "drift149_rune"
	m := driftEndDispatchFixtureModel(t, socket, slug, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	target := "deck_" + slug
	verificationsBefore := m.interactiveDispatcher.Verifications()

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(\"x\") returned a non-nil cmd, want nil")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf(`updateInteractive("x") left m.sidebarScrollDrifted true -- the drift did not end on the forwarded press`)
	}
	assertSelectionInView(t, got, `updateInteractive("x")`)

	if v := got.interactiveDispatcher.Verifications(); v != verificationsBefore+1 {
		t.Fatalf("dispatcher Verifications() = %d, want %d (exactly one Send on this press)", v, verificationsBefore+1)
	}
	waitForPaneCaptureLine0(t, socket, target, "x")
}

// TestUpdateInteractiveForwardedEnterEndsSidebarDriftAndForwardsSameByte is
// criterion 1's Enter case: Enter forwards through interactiveLiteralPayload
// (KeyEnter is the positive C0 byte 13, CR), which the pty echoes as a
// line advance (#{cursor_y}) rather than a printed character -- so this
// checks cursor_y moved to 1, not the screen content.
func TestUpdateInteractiveForwardedEnterEndsSidebarDriftAndForwardsSameByte(t *testing.T) {
	socket := selectionTestSocket("drift149-enter")
	slug := "drift149_enter"
	m := driftEndDispatchFixtureModel(t, socket, slug, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	target := "deck_" + slug
	verificationsBefore := m.interactiveDispatcher.Verifications()
	if y := tmuxCursorY(t, socket, target); y != 0 {
		t.Fatalf("test setup: pane cursor_y = %d, want 0 before Enter", y)
	}

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(Enter) returned a non-nil cmd, want nil")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf("updateInteractive(Enter) left m.sidebarScrollDrifted true -- the drift did not end on the forwarded press")
	}
	assertSelectionInView(t, got, "updateInteractive(Enter)")

	if v := got.interactiveDispatcher.Verifications(); v != verificationsBefore+1 {
		t.Fatalf("dispatcher Verifications() = %d, want %d (exactly one Send on this press)", v, verificationsBefore+1)
	}
	waitForCursorY(t, socket, target, 1)
}

// TestUpdateInteractiveForwardedBracketedPasteEndsSidebarDriftAndForwardsSameBytes
// is criterion 1's paste case: bubbletea decodes a bracketed paste into a
// tea.KeyMsg{Type: KeyRunes, Paste: true} carrying the whole pasted run
// (charmbracelet/bubbletea@v1.3.10/key_sequences.go), which
// interactiveLiteralPayload forwards exactly like an ordinary rune key --
// one Dispatcher call, the complete payload, never split.
func TestUpdateInteractiveForwardedBracketedPasteEndsSidebarDriftAndForwardsSameBytes(t *testing.T) {
	socket := selectionTestSocket("drift149-paste")
	slug := "drift149_paste"
	m := driftEndDispatchFixtureModel(t, socket, slug, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	target := "deck_" + slug
	verificationsBefore := m.interactiveDispatcher.Verifications()

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("ab"), Paste: true}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(paste \"ab\") returned a non-nil cmd, want nil")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf("updateInteractive(paste) left m.sidebarScrollDrifted true -- the drift did not end on the forwarded press")
	}
	assertSelectionInView(t, got, "updateInteractive(paste)")

	if v := got.interactiveDispatcher.Verifications(); v != verificationsBefore+1 {
		t.Fatalf("dispatcher Verifications() = %d, want %d (exactly one Send for the whole pasted run)", v, verificationsBefore+1)
	}
	waitForPaneCaptureLine0(t, socket, target, "ab")
}

// TestUpdateInteractiveUnrecognisedKeyWritesNoBytesAndKeepsSidebarDrift is
// criterion 2's negative case: Alt+Insert satisfies neither
// interactiveNamedKey nor interactiveLiteralPayload (the listed gap
// interactive_forward_gate_test.go's own tests already pin), so nothing
// is forwarded at all -- and because nothing was forwarded, the drift
// this key never "acted on the selection" for must survive it untouched.
func TestUpdateInteractiveUnrecognisedKeyWritesNoBytesAndKeepsSidebarDrift(t *testing.T) {
	socket := selectionTestSocket("drift149-unrecognised")
	slug := "drift149_unrec"
	m := driftEndDispatchFixtureModel(t, socket, slug, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	target := "deck_" + slug

	msg := tea.KeyMsg(tea.Key{Type: tea.KeyInsert, Alt: true})
	if _, ok := interactiveNamedKey(msg); ok {
		t.Fatalf("test assumption violated: interactiveNamedKey now forwards Alt+Insert -- pick a different key this test still proves matches neither helper")
	}
	if _, ok := interactiveLiteralPayload(msg); ok {
		t.Fatalf("test assumption violated: interactiveLiteralPayload now forwards Alt+Insert -- pick a different key this test still proves matches neither helper")
	}

	verificationsBefore := m.interactiveDispatcher.Verifications()
	before := tmuxCapturePane(t, socket, target)
	driftedScroll := m.sidebarScroll

	next, cmd := m.updateInteractive(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(Alt+Insert) returned a non-nil cmd, want nil")
	}
	if !got.sidebarScrollDrifted {
		t.Fatalf("updateInteractive(Alt+Insert) cleared m.sidebarScrollDrifted, but nothing was forwarded to act on the selection")
	}
	if got.sidebarScroll != driftedScroll {
		t.Fatalf("updateInteractive(Alt+Insert) moved sidebarScroll %d -> %d, want it unchanged", driftedScroll, got.sidebarScroll)
	}
	if v := got.interactiveDispatcher.Verifications(); v != verificationsBefore {
		t.Fatalf("dispatcher Verifications() = %d, want %d unchanged (Alt+Insert forwards nothing)", v, verificationsBefore)
	}
	if after := tmuxCapturePane(t, socket, target); after != before {
		t.Fatalf("updateInteractive(Alt+Insert) wrote bytes to the pane: before=%q after=%q", before, after)
	}
}

// TestExitInteractiveCtrlQEndsSidebarDriftWithSelectionInView is
// criterion 3's Ctrl+Q half: Ctrl+Q is intercepted ahead of the
// dispatcher-nil check inside updateInteractive itself, so this drives it
// through m.updateInteractive directly, exactly as tui.go's own Update
// dispatch does, against the list-only fixture (no real dispatcher
// needed: exitInteractive's own teardown is a no-op with no ownership/grid
// claimed).
func TestExitInteractiveCtrlQEndsSidebarDriftWithSelectionInView(t *testing.T) {
	m := drift149ListFixtureModel(30, 3)
	m = armSidebarWheelDriftPastSelection(t, m)

	// updateInteractive dispatches on msg.String() == "ctrl+q"; build the
	// message the same way the rest of this package's tests do (key
	// helper, tui_test.go) so tea's own String() decoding is exercised
	// rather than a hand-picked KeyType.
	next, _ := m.updateInteractive(key("ctrl+q"))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive(ctrl+q) returned a non-Model tea.Model")
	}
	if got.interactive {
		t.Fatalf("updateInteractive(ctrl+q) did not leave interactive mode")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf("Ctrl+Q left m.sidebarScrollDrifted true -- exitInteractive did not end the drift")
	}
	assertSelectionInView(t, got, "Ctrl+Q")
}

// TestExitInteractiveEmptySidebarClickEndsSidebarDriftWithSelectionInView
// is criterion 3's other half: a press that hit-tests to blank sidebar
// space (hitTargetNone) while interactive runs the same exitInteractive
// Ctrl+Q itself calls (tui.go's mouse branch) -- proven here directly
// against exitInteractive so it needs nothing from the mouse dispatch
// plumbing mouse_interactive_empty_sidebar_press_test.go already covers.
func TestExitInteractiveEmptySidebarClickEndsSidebarDriftWithSelectionInView(t *testing.T) {
	m := emptySidebarTestModel()
	m.selected = rowCursor(1)
	m.interactive = true

	x, y := findEmptySidebarSpace(t, m)
	if hit := m.hitTest(x, y); hit.panel != hitPanelSidebar {
		t.Fatalf("test setup: (%d,%d) is not the sidebar panel: %+v", x, y, hit)
	}
	m = m.scrollSidebar(wheelDown(x, y), 999)
	if !m.sidebarScrollDrifted {
		t.Fatalf("test setup: scrollSidebar did not arm m.sidebarScrollDrifted")
	}

	next, cmd := m.Update(press(x, y))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update(empty-sidebar press) returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("empty-sidebar press while interactive returned a non-nil cmd, want nil")
	}
	if got.interactive {
		t.Fatalf("empty-sidebar press while interactive did not leave interactive mode")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf("empty-sidebar click left m.sidebarScrollDrifted true -- exitInteractive did not end the drift")
	}
	assertSelectionInView(t, got, "empty-sidebar click")
}

// TestInteractivePreviewWheelKeepsSidebarDrift is criterion 4's wheel
// case: a wheel notch over the PREVIEW while interactive scrolls the
// interactive scrollback (scrollInteractiveByLines), never the sidebar,
// and must leave a sidebar-wheel drift already in force completely alone
// -- it names no selection at all.
func TestInteractivePreviewWheelKeepsSidebarDrift(t *testing.T) {
	m := drift149ListFixtureModel(30, 3)
	m.interactiveGrid = &interactive.Session{}
	m = armSidebarWheelDriftPastSelection(t, m)
	driftedScroll := m.sidebarScroll

	layout := m.computeLayout()
	previewX := layout.Sidebar.Width + 3
	if hit := m.hitTest(previewX, 5); hit.panel != hitPanelPreview {
		t.Fatalf("test setup: (%d,5) is not the preview panel: %+v", previewX, hit)
	}

	next, _ := m.Update(tea.MouseMsg{X: previewX, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update(preview wheel) returned a non-Model tea.Model")
	}
	if !got.sidebarScrollDrifted {
		t.Fatalf("a wheel over the preview while interactive cleared m.sidebarScrollDrifted")
	}
	if got.sidebarScroll != driftedScroll {
		t.Fatalf("a wheel over the preview while interactive moved sidebarScroll %d -> %d", driftedScroll, got.sidebarScroll)
	}
}

// TestInteractiveDragToCopyKeepsSidebarDrift is criterion 4's
// drag-to-copy case: a press/motion/release sequence inside the preview's
// own content box (beginInteractiveSelection/updateInteractiveSelection/
// commitInteractiveSelection) selects and copies text, never touching the
// sidebar selection or its drift. commitInteractiveSelection reads the
// grid's real scrollback length (AbsoluteRow), so this needs the SAME
// live, tmux-backed *interactive.Session driftEndDispatchFixtureModel
// already builds for the forwarding tests above, not a zero-value
// &interactive.Session{} (whose nil emulator panics once a release
// actually tries to commit).
func TestInteractiveDragToCopyKeepsSidebarDrift(t *testing.T) {
	// TestDragOverInteractivePreviewCopiesSelectedTextToTheNamedTmuxBuffer's
	// own discipline: swap the OSC 52 clipboard write to io.Discard so this
	// test's own drag-to-copy commit does not write an OSC 52 escape
	// sequence into the test runner's real terminal.
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	defer func() { oscClipboardWriter = previous }()

	socket := selectionTestSocket("drift149-drag")
	slug := "drift149_drag"
	m := driftEndDispatchFixtureModel(t, socket, slug, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	driftedScroll := m.sidebarScroll

	layout := m.computeLayout()
	previewX := layout.Sidebar.Width + 3
	if hit := m.hitTest(previewX, 3); hit.panel != hitPanelPreview {
		t.Fatalf("test setup: (%d,3) is not the preview panel: %+v", previewX, hit)
	}

	pressed, cmd := m.Update(press(previewX, 3))
	if cmd != nil {
		t.Fatalf("Update(preview press) returned a non-nil cmd, want nil")
	}
	afterPress, ok := pressed.(Model)
	if !ok {
		t.Fatalf("Update(preview press) returned a non-Model tea.Model")
	}
	if !afterPress.interactiveSelecting {
		t.Fatalf("test setup: the preview press did not start a drag-to-copy selection")
	}

	moved, _ := afterPress.Update(tea.MouseMsg{X: previewX + 2, Y: 3, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	afterMotion, ok := moved.(Model)
	if !ok {
		t.Fatalf("Update(preview motion) returned a non-Model tea.Model")
	}

	released, _ := afterMotion.Update(release(previewX+2, 3))
	got, ok := released.(Model)
	if !ok {
		t.Fatalf("Update(preview release) returned a non-Model tea.Model")
	}
	if !got.sidebarScrollDrifted {
		t.Fatalf("a drag-to-copy over the preview while interactive cleared m.sidebarScrollDrifted")
	}
	if got.sidebarScroll != driftedScroll {
		t.Fatalf("a drag-to-copy over the preview while interactive moved sidebarScroll %d -> %d", driftedScroll, got.sidebarScroll)
	}
}

// TestInteractiveReloadTicksKeepSidebarDriftOneAtATime is criterion 4's
// background-reload case, run one tick at a time (rather than three in a
// row) so a regression in only the SECOND or THIRD tick cannot hide
// behind the first one's own pass: R142 already proved this for list
// mode; this re-proves it with m.interactive true, the dimension this
// task's own fix touches.
func TestInteractiveReloadTicksKeepSidebarDriftOneAtATime(t *testing.T) {
	m := drift149ListFixtureModel(30, 3)
	m = armSidebarWheelDriftPastSelection(t, m)
	driftedScroll := m.sidebarScroll

	for i := 1; i <= 3; i++ {
		m = reloadSameSessions(m)
		if !m.sidebarScrollDrifted {
			t.Fatalf("reload tick %d cleared m.sidebarScrollDrifted while interactive", i)
		}
		if m.sidebarScroll != driftedScroll {
			t.Fatalf("reload tick %d moved sidebarScroll %d -> %d while interactive", i, driftedScroll, m.sidebarScroll)
		}
		if !m.interactive {
			t.Fatalf("reload tick %d left interactive mode", i)
		}
	}
}
