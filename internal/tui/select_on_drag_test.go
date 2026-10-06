// select_on_drag_test.go covers R202c (#67): [ui] select_on_drag decides
// whether a left drag over the interactive preview selects and copies (ON)
// or is forwarded to a pane program that tracks the mouse (OFF).
package tui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// dragButtonScript turns on button-event tracking (1002) with SGR encoding
// (1006), then echoes what the pty delivers, as wheelPaneScript does.
const dragButtonScript = `printf '\033[?1002h\033[?1006h'; stty -icanon -echo; i=0; while [ $i -lt 100 ]; do echo L$i; i=$((i+1)); done; echo ready; exec cat -v`

// dragFixture enters interactive mode on a pane running script and waits
// until the grid has seen the program's mouse modes.
func dragFixture(t *testing.T, name, script string, want interactive.MouseMode, selectOnDrag bool) (Model, string, string) {
	t.Helper()
	m, socket, target := wheelFixtureModelScript(t, name, script, "")
	m.settings.SelectOnDrag = selectOnDrag
	deadline := time.Now().Add(5 * time.Second)
	for m.interactiveGrid.Grid().MouseModes()&want != want {
		if time.Now().After(deadline) {
			t.Fatalf("grid never saw mouse modes %v (have %v)", want, m.interactiveGrid.Grid().MouseModes())
		}
		time.Sleep(20 * time.Millisecond)
	}
	return m, socket, target
}

// dragMsgs is a left press at pane cell (col, 1), a motion to (col2, 1)
// and the release there.
func dragMsgs(t *testing.T, m Model, col, col2 int, shift bool) (press, motion, release tea.MouseMsg) {
	t.Helper()
	const row = 1
	press, _ = clickButtonMsgs(t, m, tea.MouseButtonLeft, col, row, shift)
	end, _ := clickButtonMsgs(t, m, tea.MouseButtonLeft, col2, row, shift)
	motion, release = end, end
	motion.Action = tea.MouseActionMotion
	release.Action = tea.MouseActionRelease
	return press, motion, release
}

func selectionBufferText(socket string) (string, error) {
	out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput()
	return string(out), err
}

func previewHighlight(t *testing.T, m Model) map[int]map[int]string {
	t.Helper()
	return highlightedPreviewCells(t, m, m.View(), tokenHex(t, m, theme.Selection))
}

// TestSelectOnDragOnSelectsHighlightsAndCopiesAndProgramGetsNothing: with the
// setting ON a drag over a mouse-tracking program highlights while in
// progress, copies on release, and sends the program no byte of it.
func TestSelectOnDragOnSelectsHighlightsAndCopiesAndProgramGetsNothing(t *testing.T) {
	m, socket, target := dragFixture(t, "dragon", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, true)
	before := m.interactiveDispatcher.Verifications()
	press, motion, release := dragMsgs(t, m, 0, 3, false)

	m = updateModel(t, m, press)
	m = updateModel(t, m, motion)
	if len(previewHighlight(t, m)) == 0 {
		t.Fatalf("a drag with select_on_drag ON highlighted nothing")
	}
	m = updateModel(t, m, release)
	if len(previewHighlight(t, m)) != 0 {
		t.Fatalf("the highlight survived the release")
	}
	if m.selectionCopyNote == "" || m.attachError != "" {
		t.Fatalf("release did not copy (note %q, error %q)", m.selectionCopyNote, m.attachError)
	}
	if got, err := selectionBufferText(socket); err != nil || strings.TrimSpace(got) == "" {
		t.Fatalf("tmux selection buffer empty after the copy (err %v, %q)", err, got)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("the drag reached the dispatcher (Verifications %d -> %d)", before, v)
	}
	if strings.Contains(tmuxCapturePane(t, socket, target), "^[[<") {
		t.Fatalf("a report reached the pane from a drag that selects")
	}
}

// TestSelectOnDragOffForwardsDragAsReportsAndSelectsNothing: with the setting
// OFF the same drag reaches a button-tracking program as press, motion
// (button code + 32) and release reports, and nothing is highlighted or
// copied.
func TestSelectOnDragOffForwardsDragAsReportsAndSelectsNothing(t *testing.T) {
	m, socket, target := dragFixture(t, "dragoff", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, false)
	press, motion, release := dragMsgs(t, m, 2, 7, false)

	m = updateModel(t, m, press)
	m = updateModel(t, m, motion)
	if len(previewHighlight(t, m)) != 0 || m.interactiveSelecting {
		t.Fatalf("a forwarded drag highlighted or began a selection")
	}
	waitForPaneJoined(t, socket, target, "^[[<0;3;2M^[[<32;8;2M")
	m = updateModel(t, m, release)
	waitForPaneJoined(t, socket, target, "^[[<0;3;2M^[[<32;8;2M^[[<0;8;2m")
	if m.selectionCopyNote != "" || m.interactiveForwardedButtons != 0 {
		t.Fatalf("release left copy/forwarding state behind (note %q, forwarding %v)", m.selectionCopyNote, m.interactiveForwardedButtons)
	}
	if got, err := selectionBufferText(socket); err == nil {
		t.Fatalf("a forwarded drag copied to the tmux selection buffer: %q", got)
	}
}

// TestSelectOnDragOffMotionOnlyToProgramsThatAskForIt: plain 1000 tracking
// wants press and release but no motion.
func TestSelectOnDragOffMotionOnlyToProgramsThatAskForIt(t *testing.T) {
	m, socket, target := dragFixture(t, "dragoff1000", wheelPaneScript(true), interactive.MouseNormal|interactive.MouseSGR, false)
	press, motion, release := dragMsgs(t, m, 2, 7, false)
	m = updateModel(t, m, press)
	m = updateModel(t, m, motion)
	_ = updateModel(t, m, release)
	waitForPaneJoined(t, socket, target, "^[[<0;3;2M^[[<0;8;2m")
}

// TestSelectOnDragOffShiftDragStillSelects: Shift on the press keeps the
// selection route, as it keeps the wheel and clicks off the program.
func TestSelectOnDragOffShiftDragStillSelects(t *testing.T) {
	m, socket, _ := dragFixture(t, "dragoffshift", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, false)
	before := m.interactiveDispatcher.Verifications()
	press, motion, release := dragMsgs(t, m, 0, 3, true)
	m = updateModel(t, m, press)
	m = updateModel(t, m, motion)
	if len(previewHighlight(t, m)) == 0 {
		t.Fatalf("a Shift drag with select_on_drag OFF did not highlight")
	}
	m = updateModel(t, m, release)
	if got, err := selectionBufferText(socket); err != nil || strings.TrimSpace(got) == "" {
		t.Fatalf("a Shift drag did not copy (err %v, %q)", err, got)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a Shift drag reached the dispatcher")
	}
}

// TestSelectOnDragOffNonTrackingProgramGetsNothing: OFF never selects, and a
// program that does not track the mouse receives nothing.
func TestSelectOnDragOffNonTrackingProgramGetsNothing(t *testing.T) {
	m, socket, target := dragFixture(t, "dragoffplain", wheelPaneScript(false), 0, false)
	before := m.interactiveDispatcher.Verifications()
	press, motion, release := dragMsgs(t, m, 2, 7, false)
	m = updateModel(t, m, press)
	m = updateModel(t, m, motion)
	if len(previewHighlight(t, m)) != 0 {
		t.Fatalf("OFF highlighted a drag over a non-tracking program")
	}
	m = updateModel(t, m, release)
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a drag reached a non-tracking program")
	}
	if _, err := selectionBufferText(socket); err == nil {
		t.Fatalf("OFF copied a drag")
	}
	if strings.Contains(tmuxCapturePane(t, socket, target), "^[[") {
		t.Fatalf("a report reached a non-tracking pane")
	}
}
