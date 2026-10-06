// interactive_button_forward_test.go covers R202 (#67): over the interactive
// preview every mouse event except an ON-mode left selection drag reaches a
// pane program that tracks the mouse, in the encoding it asked for, with the
// button's identity, its press/motion/release lifecycle and its coordinates.
package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// anyMotionScript turns on any-event tracking (1003) with SGR encoding.
const anyMotionScript = `printf '\033[?1003h\033[?1006h'; stty -icanon -echo; i=0; while [ $i -lt 100 ]; do echo L$i; i=$((i+1)); done; echo ready; exec cat -v`

// x10ButtonScript turns on button-event tracking (1002) without SGR.
const x10ButtonScript = `printf '\033[?1002h'; stty -icanon -echo; echo ready; exec cat -v`

// buttonMsg is a mouse event of the given action and button at pane cell
// (col, row).
func buttonMsg(t *testing.T, m Model, button tea.MouseButton, action tea.MouseAction, col, row int, shift bool) tea.MouseMsg {
	t.Helper()
	at := wheelAtPreviewCell(t, m, col, row, true, shift)
	return tea.MouseMsg{X: at.X, Y: at.Y, Action: action, Button: button, Shift: shift}
}

func TestEncodeMouseReportNoButtonMotionAndExtraButtons(t *testing.T) {
	sgr := interactive.MouseAny | interactive.MouseSGR
	x10 := interactive.MouseAny
	for _, tc := range []struct {
		name   string
		modes  interactive.MouseMode
		button int
		kind   mouseReportKind
		want   string
		ok     bool
	}{
		{"sgr hover", sgr, mouseButtonCodeNone, mouseReportMotion, "\x1b[<35;4;3M", true},
		{"x10 hover", x10, mouseButtonCodeNone, mouseReportMotion, "\x1b[MC$#", true},
		{"sgr wheel left", sgr, mouseButtonCodeWheelLeft, mouseReportPress, "\x1b[<66;4;3M", true},
		{"sgr wheel right", sgr, mouseButtonCodeWheelRight, mouseReportPress, "\x1b[<67;4;3M", true},
		{"x10 wheel right", x10, mouseButtonCodeWheelRight, mouseReportPress, "\x1b[Mc$#", true},
		{"sgr backward press", sgr, mouseButtonCodeExtraBase, mouseReportPress, "\x1b[<128;4;3M", true},
		{"sgr forward release", sgr, mouseButtonCodeExtraBase + 1, mouseReportRelease, "\x1b[<129;4;3m", true},
		{"sgr button 11 motion", sgr, mouseButtonCodeExtraBase + 3, mouseReportMotion, "\x1b[<163;4;3M", true},
		{"x10 has no extra button", x10, mouseButtonCodeExtraBase, mouseReportPress, "", false},
		{"sgr middle motion", sgr, mouseButtonCodeMiddle, mouseReportMotion, "\x1b[<33;4;3M", true},
		{"x10 right motion", x10, mouseButtonCodeRight, mouseReportMotion, "\x1b[MB$#", true},
	} {
		got, ok := encodeMouseReport(tc.modes, tc.button, tc.kind, 3, 2)
		if ok != tc.ok || string(got) != tc.want {
			t.Errorf("%s: encodeMouseReport = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestForwardedButtonCodeCoversEveryForwardedButton(t *testing.T) {
	want := map[tea.MouseButton]int{
		tea.MouseButtonLeft: 0, tea.MouseButtonMiddle: 1, tea.MouseButtonRight: 2,
		tea.MouseButtonWheelLeft: 66, tea.MouseButtonWheelRight: 67,
		tea.MouseButtonBackward: 128, tea.MouseButtonForward: 129,
		tea.MouseButton10: 130, tea.MouseButton11: 131,
	}
	for b, code := range want {
		if got, ok := forwardedButtonCode(b); !ok || got != code {
			t.Errorf("forwardedButtonCode(%v) = %d, %v; want %d", b, got, ok, code)
		}
	}
	for _, b := range []tea.MouseButton{tea.MouseButtonNone, tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
		if _, ok := forwardedButtonCode(b); ok {
			t.Errorf("forwardedButtonCode(%v) ok; the vertical wheel and no button are not press/release buttons", b)
		}
	}
}

// TestInteractiveMiddleRightDragForwardsFullLifecycleInBothModes: a middle or
// right drag reaches a button-tracking program as press, motion (code + 32)
// and release with that button's code and the pointer's cell, whether
// select_on_drag is ON or OFF, and highlights and copies nothing.
func TestInteractiveMiddleRightDragForwardsFullLifecycleInBothModes(t *testing.T) {
	for _, selectOnDrag := range []bool{true, false} {
		for _, tc := range []struct {
			button tea.MouseButton
			want   string
		}{
			{tea.MouseButtonMiddle, "^[[<1;3;2M^[[<33;8;2M^[[<1;8;2m"},
			{tea.MouseButtonRight, "^[[<2;3;2M^[[<34;8;2M^[[<2;8;2m"},
			{tea.MouseButtonBackward, "^[[<128;3;2M^[[<160;8;2M^[[<128;8;2m"},
		} {
			name := fmt.Sprintf("mr%d", int(tc.button))
			if selectOnDrag {
				name += "on"
			}
			m, socket, target := dragFixture(t, name, dragButtonScript, interactive.MouseButton|interactive.MouseSGR, selectOnDrag)
			press := buttonMsg(t, m, tc.button, tea.MouseActionPress, 2, 1, false)
			motion := buttonMsg(t, m, tc.button, tea.MouseActionMotion, 7, 1, false)
			release := buttonMsg(t, m, tc.button, tea.MouseActionRelease, 7, 1, false)
			m = updateModel(t, m, press)
			m = updateModel(t, m, motion)
			if m.interactiveSelecting || len(previewHighlight(t, m)) != 0 {
				t.Fatalf("%v drag (select_on_drag %v) began a selection or highlighted", tc.button, selectOnDrag)
			}
			m = updateModel(t, m, release)
			waitForPaneJoined(t, socket, target, tc.want)
			if m.interactiveForwardedButtons != 0 || m.selectionCopyNote != "" {
				t.Fatalf("%v drag left forwarding or copy state behind", tc.button)
			}
			if got, err := selectionBufferText(socket); err == nil {
				t.Fatalf("%v drag copied to the tmux buffer: %q", tc.button, got)
			}
		}
	}
}

// TestInteractiveButtonMotionOnlyToProgramsThatAskForIt: plain 1000 wants
// press and release of a middle drag, no motion.
func TestInteractiveButtonMotionOnlyToProgramsThatAskForIt(t *testing.T) {
	m, socket, target := dragFixture(t, "mmotion1000", wheelPaneScript(true), interactive.MouseNormal|interactive.MouseSGR, true)
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionPress, 2, 1, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionMotion, 7, 1, false))
	_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionRelease, 7, 1, false))
	waitForPaneJoined(t, socket, target, "^[[<1;3;2M^[[<1;8;2m")
}

// TestInteractiveHoverForwardedOnlyToAnyEventPrograms: motion with no button
// is code 35 (3 + 32) for a 1003 program in either setting, and nothing for a
// program in 1002 or 1000, a plain shell, or with Shift held.
func TestInteractiveHoverForwardedOnlyToAnyEventPrograms(t *testing.T) {
	for _, selectOnDrag := range []bool{true, false} {
		m, socket, target := dragFixture(t, "hover1003"+map[bool]string{true: "on", false: "off"}[selectOnDrag], anyMotionScript, interactive.MouseAny|interactive.MouseSGR, selectOnDrag)
		before := m.interactiveDispatcher.Verifications()
		_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionMotion, 4, 2, true))
		if v := m.interactiveDispatcher.Verifications(); v != before {
			t.Fatalf("Shift hover reached the program")
		}
		_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionMotion, 4, 2, false))
		waitForPaneJoined(t, socket, target, "^[[<35;5;3M")
	}
	for name, tc := range map[string]struct {
		script string
		modes  interactive.MouseMode
	}{
		"hover1002":  {dragButtonScript, interactive.MouseButton | interactive.MouseSGR},
		"hover1000":  {wheelPaneScript(true), interactive.MouseNormal | interactive.MouseSGR},
		"hovershell": {wheelPaneScript(false), 0},
	} {
		m, socket, target := dragFixture(t, name, tc.script, tc.modes, true)
		before := m.interactiveDispatcher.Verifications()
		_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionMotion, 4, 2, false))
		if v := m.interactiveDispatcher.Verifications(); v != before {
			t.Fatalf("%s: hover reached a program that did not ask for it", name)
		}
		if strings.Contains(tmuxCapturePane(t, socket, target), "^[[<") {
			t.Fatalf("%s: a report reached the pane", name)
		}
	}
}

// TestInteractiveHoverOutsidePreviewForwardsNothing: motion over the sidebar
// is deck's, not the program's.
func TestInteractiveHoverOutsidePreviewForwardsNothing(t *testing.T) {
	m, _, _ := dragFixture(t, "hoveroutside", anyMotionScript, interactive.MouseAny|interactive.MouseSGR, true)
	before := m.interactiveDispatcher.Verifications()
	_ = updateModel(t, m, tea.MouseMsg{X: 1, Y: 2, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("hover over the sidebar reached the program")
	}
}

// TestInteractiveHorizontalWheelForwardsToTrackingProgram: a sideways notch
// is button 66 or 67 for a tracking program and nothing for a plain shell.
func TestInteractiveHorizontalWheelForwardsToTrackingProgram(t *testing.T) {
	m, socket, target := wheelFixtureModel(t, "hwheel", true)
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonWheelLeft, tea.MouseActionPress, 3, 2, false))
	_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonWheelRight, tea.MouseActionPress, 6, 4, false))
	waitForPaneJoined(t, socket, target, "^[[<66;4;3M^[[<67;7;5M")

	plain, psocket, ptarget := wheelFixtureModel(t, "hwheelplain", false)
	before := plain.interactiveDispatcher.Verifications()
	_ = updateModel(t, plain, buttonMsg(t, plain, tea.MouseButtonWheelLeft, tea.MouseActionPress, 3, 2, false))
	if v := plain.interactiveDispatcher.Verifications(); v != before || strings.Contains(tmuxCapturePane(t, psocket, ptarget), "^[[<") {
		t.Fatalf("a sideways notch reached a non-tracking program")
	}
}

// TestInteractiveOtherButtonControlsRetained: Shift on the press, a
// non-tracking program and a scrolled-back grid each forward nothing of a
// middle/right gesture, and a Shift press leaves no release to strand.
func TestInteractiveOtherButtonControlsRetained(t *testing.T) {
	m, socket, target := dragFixture(t, "ctrlmr", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, true)
	before := m.interactiveDispatcher.Verifications()
	for _, b := range []tea.MouseButton{tea.MouseButtonMiddle, tea.MouseButtonRight} {
		m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionPress, 2, 1, true))
		m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionMotion, 5, 1, false))
		m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionRelease, 5, 1, false))
	}
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a Shift press leaked a motion or release to the program")
	}
	m = updateModel(t, m, wheelAtPreviewCell(t, m, 4, 2, true, true))
	if m.interactiveScrollOffset() == 0 {
		t.Fatalf("test setup: Shift+wheel did not scroll back")
	}
	before = m.interactiveDispatcher.Verifications()
	for _, b := range []tea.MouseButton{tea.MouseButtonMiddle, tea.MouseButtonRight} {
		m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionPress, 2, 1, false))
		m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionRelease, 2, 1, false))
	}
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionMotion, 2, 1, false))
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a scrolled-back grid forwarded mouse events")
	}
	if strings.Contains(tmuxCapturePane(t, socket, target), "^[[<") {
		t.Fatalf("a report reached the pane")
	}

	plain, _, _ := dragFixture(t, "ctrlplain", wheelPaneScript(false), 0, true)
	pb := plain.interactiveDispatcher.Verifications()
	plain = updateModel(t, plain, buttonMsg(t, plain, tea.MouseButtonRight, tea.MouseActionPress, 2, 1, false))
	plain = updateModel(t, plain, buttonMsg(t, plain, tea.MouseButtonRight, tea.MouseActionMotion, 5, 1, false))
	_ = updateModel(t, plain, buttonMsg(t, plain, tea.MouseButtonRight, tea.MouseActionRelease, 5, 1, false))
	if v := plain.interactiveDispatcher.Verifications(); v != pb {
		t.Fatalf("a non-tracking program received a right drag")
	}
}

// TestInteractiveOverlappingButtonsEachKeepTheirLifecycle: a right press
// while the middle button is still held does not lose the middle release.
func TestInteractiveOverlappingButtonsEachKeepTheirLifecycle(t *testing.T) {
	m, socket, target := dragFixture(t, "overlap", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, true)
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionPress, 1, 1, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionPress, 2, 1, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionRelease, 3, 1, false))
	_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionRelease, 4, 1, false))
	waitForPaneJoined(t, socket, target, "^[[<1;2;2M^[[<2;3;2M^[[<1;4;2m^[[<2;5;2m")
}

// TestInteractiveX10ReleaseEndsTheHeldButton: an X10 release names no button
// (bubbletea reports none); it ends the right gesture as release code 3, and
// ends an ON-mode left selection by committing the copy.
func TestInteractiveX10ReleaseEndsTheHeldButton(t *testing.T) {
	m, socket, target := dragFixture(t, "x10rel", x10ButtonScript, interactive.MouseButton, true)
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionPress, 2, 1, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionMotion, 5, 1, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionRelease, 5, 1, false))
	// col 2,row 1 -> bytes 35,34; right press 32+2, motion 32+34, release 32+3.
	waitForPaneJoined(t, socket, target, "^[[M\"#\"^[[MB&\"^[[M#&\"")
	if m.interactiveForwardedButtons != 0 {
		t.Fatalf("the X10 release did not end the forwarded gesture")
	}

	sel, ssocket, _ := dragFixture(t, "x10sel", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, true)
	sel = updateModel(t, sel, buttonMsg(t, sel, tea.MouseButtonLeft, tea.MouseActionPress, 0, 1, false))
	sel = updateModel(t, sel, buttonMsg(t, sel, tea.MouseButtonLeft, tea.MouseActionMotion, 3, 1, false))
	sel = updateModel(t, sel, buttonMsg(t, sel, tea.MouseButtonNone, tea.MouseActionRelease, 3, 1, false))
	if sel.interactiveSelecting || sel.selectionCopyNote == "" {
		t.Fatalf("an X10 release did not commit the left selection (note %q)", sel.selectionCopyNote)
	}
	if got, err := selectionBufferText(ssocket); err != nil || strings.TrimSpace(got) == "" {
		t.Fatalf("nothing copied by the X10 release (err %v, %q)", err, got)
	}
}

// TestInteractiveForwardedGestureStartedOutsidePreviewIsNotForwarded: a
// middle press on the sidebar arms nothing, so a later motion and release
// over the preview are not sent.
func TestInteractiveForwardedGestureStartedOutsidePreviewIsNotForwarded(t *testing.T) {
	m, _, _ := dragFixture(t, "startout", dragButtonScript, interactive.MouseButton|interactive.MouseSGR, true)
	before := m.interactiveDispatcher.Verifications()
	m = updateModel(t, m, tea.MouseMsg{X: 1, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonMiddle})
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionMotion, 4, 1, false))
	_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionRelease, 4, 1, false))
	if v := m.interactiveDispatcher.Verifications(); v != before {
		t.Fatalf("a gesture started on the sidebar reached the program")
	}
}
