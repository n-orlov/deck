// interactive_wheel_forward_test.go covers R183/GH #59 end to end through
// Update on a real tmux pane: a wheel notch over the interactive preview
// reaches a pane program that tracks the mouse as the exact report for the
// cell under the pointer, and scrolls deck's grid otherwise. It uses only
// APIs that exist before the phase, so it runs unchanged against the
// pre-phase export (where every forwarding assertion fails: the notch
// scrolls the grid and nothing reaches the pane).
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// wheelPaneScript prints 100 history lines, optionally turns on mouse
// reporting (X10 press tracking 1000 and SGR encoding 1006), then echoes
// whatever the pty delivers, byte for byte, as `cat -v` caret notation
// ("ready" is printed last so a test knows the program is reading).
func wheelPaneScript(mouseOn bool) string {
	on := ""
	if mouseOn {
		on = `printf '\033[?1000h\033[?1006h'; `
	}
	return on + `stty -icanon -echo; i=0; while [ $i -lt 100 ]; do echo L$i; i=$((i+1)); done; echo ready; exec cat -v`
}

// wheelFixtureModel enters interactive mode on a real pane running
// wheelPaneScript, in a 30-session sidebar so a sidebar wheel can drift
// off the target's row.
func wheelFixtureModel(t *testing.T, name string, mouseOn bool) (Model, string, string) {
	t.Helper()
	flags := ""
	if mouseOn {
		flags = "11"
	}
	return wheelFixtureModelScript(t, name, wheelPaneScript(mouseOn), flags)
}

// wheelFixtureModelScript is wheelFixtureModel over an arbitrary pane
// script; a non-empty wantFlags is the "#{mouse_standard_flag}#{mouse_sgr_flag}"
// value the pane must report before the model enters interactive mode.
func wheelFixtureModelScript(t *testing.T, name, script, wantFlags string) (Model, string, string) {
	t.Helper()
	socket := selectionTestSocket(name)
	slug := "wheel_" + name
	target := "deck_" + slug
	args := []string{"-L", socket, "new-session", "-d", "-s", target, "-x", "80", "-y", "24", "sh", "-c", script}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start wheel pane: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(tmuxCapturePane(t, socket, target), "ready") {
		if time.Now().After(deadline) {
			t.Fatalf("wheel pane never printed ready:\n%s", tmuxCapturePane(t, socket, target))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if wantFlags != "" {
		for {
			out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", target, "#{mouse_standard_flag}#{mouse_sgr_flag}").Output()
			if err == nil && strings.TrimSpace(string(out)) == wantFlags {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("pane never reported mouse flags %q (last %q, err %v)", wantFlags, out, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	m := New(nil, config.Settings{Mouse: true, Color: true, SelectOnDrag: true}, "")
	var sessions []store.Session
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("s%02d", i)
		sessions = append(sessions, store.Session{ID: id, Name: id, Slug: id, CWD: "/work/infra", Status: "idle"})
	}
	sessions[3] = store.Session{ID: "target-" + slug, Name: slug, Slug: slug, CWD: "/work", Status: "waiting"}
	m.sessions = sessions
	m.baseSessions = append([]store.Session(nil), sessions...)
	m.width, m.height = 100, 30
	m.selected = rowCursor(3)
	m.tmuxClient = tmux.Client{Socket: socket}

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok || got.attachError != "" || !got.interactive || got.interactiveGrid == nil || got.interactiveDispatcher == nil {
		t.Fatalf("enterInteractive did not enter interactive mode (attachError=%q, refusal=%+v)", got.attachError, got.entryRefusal)
	}
	t.Cleanup(func() { got.exitInteractive() })
	return got, socket, target
}

// wheelAtPreviewCell builds a wheel notch whose pointer sits on the pane
// cell (col, row), after checking previewCellAt agrees.
func wheelAtPreviewCell(t *testing.T, m Model, col, row int, up bool, shift bool) tea.MouseMsg {
	t.Helper()
	x := m.computeLayout().Sidebar.Width + 2 + col
	y := 1 + row
	gc, gr, ok := m.previewCellAt(x, y)
	if !ok || gc != col || gr != row {
		t.Fatalf("test setup: previewCellAt(%d,%d) = (%d,%d,%v), want (%d,%d,true)", x, y, gc, gr, ok, col, row)
	}
	button := tea.MouseButtonWheelDown
	if up {
		button = tea.MouseButtonWheelUp
	}
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: button, Shift: shift}
}

// waitForPaneContains polls the pane's capture until it contains want.
func waitForPaneContains(t *testing.T, socket, target, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(tmuxCapturePane(t, socket, target), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane never showed %q; last capture:\n%s", want, tmuxCapturePane(t, socket, target))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned a non-Model tea.Model")
	}
	return got
}

// TestInteractiveWheelReachesMouseTrackingProgramAsSGRReport: with the
// program tracking the mouse (1000+1006) and the grid at live, a notch over
// the preview is forwarded as the exact SGR report for the pane cell under
// the pointer, through one verified dispatcher send, and the grid does not
// scroll. A notch up is button 64, a notch down 65.
func TestInteractiveWheelReachesMouseTrackingProgramAsSGRReport(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "sgr", true)
	if got := m.interactiveScrollOffset(); got != 0 {
		t.Fatalf("test setup: offset = %d, want 0", got)
	}
	for _, tc := range []struct {
		col, row int
		up       bool
		want     string
	}{
		{col: 2, row: 1, up: true, want: "^[[<64;3;2M"},
		{col: 7, row: 4, up: false, want: "^[[<65;8;5M"},
	} {
		before := m.interactiveDispatcher.Verifications()
		m = updateModel(t, m, wheelAtPreviewCell(t, m, tc.col, tc.row, tc.up, false))
		if v := m.interactiveDispatcher.Verifications(); v != before+1 {
			t.Fatalf("notch at (%d,%d): Verifications() = %d, want %d (one verified send)", tc.col, tc.row, v, before+1)
		}
		if off := m.interactiveScrollOffset(); off != 0 {
			t.Fatalf("notch at (%d,%d) scrolled the grid to offset %d, want it forwarded with offset 0", tc.col, tc.row, off)
		}
		waitForPaneContains(t, socket, target, tc.want)
	}
}

// TestInteractiveWheelForwardedNotchEndsSidebarDrift: a forwarded notch is
// input to the pane, so it ends a sidebar wheel drift exactly as a
// forwarded key does -- the selection is back in view, and the notch still
// reached the program.
func TestInteractiveWheelForwardedNotchEndsSidebarDrift(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "drift", true)
	m = armSidebarWheelDriftPastSelection(t, m)
	m = updateModel(t, m, wheelAtPreviewCell(t, m, 4, 2, true, false))
	if m.sidebarScrollDrifted {
		t.Fatalf("a forwarded notch left m.sidebarScrollDrifted true")
	}
	assertSelectionInView(t, m, "forwarded wheel notch")
	waitForPaneContains(t, socket, target, "^[[<64;5;3M")
}

// TestInteractiveWheelScrollsGridWhenNotForwarded: Shift+wheel scrolls the
// grid while reporting is on, a notch while scrolled back keeps scrolling
// it, and with reporting off the notch scrolls the grid -- nothing reaches
// the program in any of the three.
func TestInteractiveWheelScrollsGridWhenNotForwarded(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "shift", true)
	before := m.interactiveDispatcher.Verifications()
	m = updateModel(t, m, wheelAtPreviewCell(t, m, 4, 2, true, true))
	if off := m.interactiveScrollOffset(); off != interactiveWheelStepLines {
		t.Fatalf("Shift+wheel with reporting on: offset = %d, want %d (scrolled the grid)", off, interactiveWheelStepLines)
	}
	m = updateModel(t, m, wheelAtPreviewCell(t, m, 4, 2, true, false))
	if off := m.interactiveScrollOffset(); off != 2*interactiveWheelStepLines {
		t.Fatalf("wheel while scrolled back with reporting on: offset = %d, want %d", off, 2*interactiveWheelStepLines)
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a scrolled notch reached the dispatcher (Verifications %d -> %d)", before, v)
	}

	off, offSocket, offTarget := wheelFixtureModel(t, "off", false)
	offBefore := off.interactiveDispatcher.Verifications()
	off = updateModel(t, off, wheelAtPreviewCell(t, off, 4, 2, true, false))
	if got := off.interactiveScrollOffset(); got != interactiveWheelStepLines {
		t.Fatalf("wheel with reporting off: offset = %d, want %d", got, interactiveWheelStepLines)
	}
	if v := off.interactiveDispatcher.Verifications(); v != offBefore {
		t.Fatalf("a notch with reporting off reached the dispatcher (Verifications %d -> %d)", offBefore, v)
	}
	for _, c := range []struct{ socket, target string }{{socket, target}, {offSocket, offTarget}} {
		if strings.Contains(tmuxCapturePane(t, c.socket, c.target), "^[[<") {
			t.Fatalf("a mouse report reached %s although the notches were scrolls", c.target)
		}
	}
}
