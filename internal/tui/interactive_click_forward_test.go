// interactive_click_forward_test.go covers R202 (#67): a click of the left,
// middle or right button over the interactive preview reaches a pane program
// that tracks the mouse as a press plus a release at the press cell, and
// reaches nothing else.
package tui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clickButtonMsgs is the press and release a terminal delivers for one click
// of button at the pane cell (col, row), Shift as given.
func clickButtonMsgs(t *testing.T, m Model, button tea.MouseButton, col, row int, shift bool) (tea.MouseMsg, tea.MouseMsg) {
	t.Helper()
	at := wheelAtPreviewCell(t, m, col, row, true, shift)
	down := tea.MouseMsg{X: at.X, Y: at.Y, Action: tea.MouseActionPress, Button: button, Shift: shift}
	up := down
	up.Action = tea.MouseActionRelease
	return down, up
}

func clickAt(t *testing.T, m Model, button tea.MouseButton, col, row int, shift bool) Model {
	t.Helper()
	down, up := clickButtonMsgs(t, m, button, col, row, shift)
	m = updateModel(t, m, down)
	return updateModel(t, m, up)
}

// TestInteractiveClickForwardsEachButtonAsPressAndRelease: with the program
// tracking the mouse in SGR encoding, a click of each button is one verified
// send of press (M) then release (m) at the press cell with that button's code.
func TestInteractiveClickForwardsEachButtonAsPressAndRelease(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "clickfwd", true)
	for _, tc := range []struct {
		name     string
		button   tea.MouseButton
		col, row int
		want     string
	}{
		{"left", tea.MouseButtonLeft, 2, 1, "^[[<0;3;2M^[[<0;3;2m"},
		{"middle", tea.MouseButtonMiddle, 5, 3, "^[[<1;6;4M^[[<1;6;4m"},
		{"right", tea.MouseButtonRight, 9, 6, "^[[<2;10;7M^[[<2;10;7m"},
	} {
		before := m.interactiveDispatcher.Verifications()
		m = clickAt(t, m, tc.button, tc.col, tc.row, false)
		if v := m.interactiveDispatcher.Verifications(); v != before+1 {
			t.Fatalf("%s click: Verifications() = %d, want %d (one verified send)", tc.name, v, before+1)
		}
		waitForPaneJoined(t, socket, target, tc.want)
		if m.interactiveSelecting || m.selectionCopyNote != "" {
			t.Fatalf("%s click left selection state behind", tc.name)
		}
	}
}

// TestInteractiveClickForwardsInX10Encoding: without 1006 the click is the
// X10 report, and its release carries button code 3.
func TestInteractiveClickForwardsInX10Encoding(t *testing.T) {
	script := `printf '\033[?1000h'; stty -icanon -echo; echo ready; exec cat -v`
	m, socket, target := wheelFixtureModelScript(t, "clickx10", script, "10")
	// col 2, row 1 is 1-based (3, 2): bytes 32+b, 35, 34.
	m = clickAt(t, m, tea.MouseButtonRight, 2, 1, false)
	waitForPaneJoined(t, socket, target, "^[[M\"#\"^[[M##\"")
	_ = clickAt(t, m, tea.MouseButtonLeft, 2, 1, false)
	waitForPaneJoined(t, socket, target, "^[[M #\"^[[M##\"")
}

// TestInteractiveClickSendsNothingToNonTrackingProgram: a program that has
// not turned mouse reporting on receives nothing from a click of any button.
func TestInteractiveClickSendsNothingToNonTrackingProgram(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "clickoff", false)
	before := m.interactiveDispatcher.Verifications()
	for _, b := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonMiddle, tea.MouseButtonRight} {
		m = clickAt(t, m, b, 3, 2, false)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("clicks reached the dispatcher of a non-tracking program (Verifications %d -> %d)", before, v)
	}
	if strings.Contains(tmuxCapturePane(t, socket, target), "^[[") {
		t.Fatalf("a report reached a non-tracking pane")
	}
}

// TestInteractiveClickNotForwardedWithShiftOrScrolledBack: Shift held, or a
// grid scrolled back from live, forwards nothing, for every button.
func TestInteractiveClickNotForwardedWithShiftOrScrolledBack(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "clickshift", true)
	before := m.interactiveDispatcher.Verifications()
	for _, b := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonMiddle, tea.MouseButtonRight} {
		m = clickAt(t, m, b, 3, 2, true)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("Shift+click reached the dispatcher (Verifications %d -> %d)", before, v)
	}

	m = updateModel(t, m, wheelAtPreviewCell(t, m, 4, 2, true, true))
	if m.interactiveScrollOffset() == 0 {
		t.Fatalf("test setup: Shift+wheel did not scroll the grid back")
	}
	for _, b := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonMiddle, tea.MouseButtonRight} {
		m = clickAt(t, m, b, 3, 2, false)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a click on a scrolled-back grid reached the dispatcher (Verifications %d -> %d)", before, v)
	}
	if strings.Contains(tmuxCapturePane(t, socket, target), "^[[<") {
		t.Fatalf("a report reached the pane although every click was Shift or scrolled back")
	}
}

// TestInteractiveDragForwardsNothingToProgram: a left drag selects; the
// program sees neither the press nor the release.
func TestInteractiveDragForwardsNothingToProgram(t *testing.T) {
	m, _, _ := wheelFixtureModel(t, "clickdrag", true)
	before := m.interactiveDispatcher.Verifications()
	down, up := clickButtonMsgs(t, m, tea.MouseButtonLeft, 2, 1, false)
	drag := down
	drag.Action = tea.MouseActionMotion
	drag.X += 5
	m = updateModel(t, m, down)
	m = updateModel(t, m, drag)
	_ = updateModel(t, m, up)
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a drag reached the dispatcher (Verifications %d -> %d)", before, v)
	}
}

// waitForPaneJoined is waitForPaneContains over the capture with wrapped
// lines joined, so a report that wraps at the pane's edge is still found whole.
func waitForPaneJoined(t *testing.T, socket, target, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-J", "-t", target).Output()
		if err == nil && strings.Contains(string(out), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane never showed %q; last capture (err %v):\n%s", want, err, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
