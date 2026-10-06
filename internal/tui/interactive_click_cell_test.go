// interactive_click_cell_test.go covers R202 (#67), SPEC §11.8: a click of
// the left, middle or right button (or an additional button) is forwarded as
// a press and a release at the PRESS cell, even when the terminal reports the
// release a few cells away with no motion in between; a genuine drag still
// reports its real motion and release cells.
package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// clickCellMode is one mouse-tracking mode and encoding a pane can ask for.
type clickCellMode struct {
	name   string
	script string
	modes  interactive.MouseMode
	sgr    bool
	motion bool // the program asked for held-button motion (1002/1003)
}

func clickCellModes() []clickCellMode {
	var out []clickCellMode
	for _, mode := range []struct {
		n      string
		set    string
		flag   interactive.MouseMode
		motion bool
	}{
		{"mode1000", "1000", interactive.MouseNormal, false},
		{"mode1002", "1002", interactive.MouseButton, true},
		{"mode1003", "1003", interactive.MouseAny, true},
	} {
		for _, sgr := range []bool{true, false} {
			set, flags, tag := mode.set, mode.flag, "x10"
			if sgr {
				set += `h\033[?1006`
				flags |= interactive.MouseSGR
				tag = "sgr"
			}
			out = append(out, clickCellMode{
				name:   mode.n + tag,
				script: `printf '\033[?` + set + `h'; stty -icanon -echo; echo ready; exec cat -v`,
				modes:  flags,
				sgr:    sgr,
				motion: mode.motion,
			})
		}
	}
	return out
}

// catV renders report bytes the way `cat -v` shows them in the pane.
func catV(report []byte, ok bool) string {
	if !ok {
		return ""
	}
	return strings.ReplaceAll(string(report), "\x1b", "^[")
}

// wantReports is the pane text of the given (code, kind, col, row) reports.
func wantReports(modes interactive.MouseMode, steps ...[4]int) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(catV(encodeMouseReport(modes, s[0], mouseReportKind(s[1]), s[2], s[3])))
	}
	return b.String()
}

// TestInteractiveClickReleasesAtPressCellForEveryButtonAndEncoding: the
// release of a no-motion click is reported at the press cell although the
// terminal named a different one, for left, middle and right, every
// tracking mode, both encodings and both select_on_drag settings.
func TestInteractiveClickReleasesAtPressCellForEveryButtonAndEncoding(t *testing.T) {
	buttons := []struct {
		name string
		b    tea.MouseButton
		code int
	}{
		{"left", tea.MouseButtonLeft, mouseButtonCodeLeft},
		{"middle", tea.MouseButtonMiddle, mouseButtonCodeMiddle},
		{"right", tea.MouseButtonRight, mouseButtonCodeRight},
	}
	for _, cm := range clickCellModes() {
		for _, selectOnDrag := range []bool{true, false} {
			for _, btn := range buttons {
				name := fmt.Sprintf("%s/%s/sod%v", cm.name, btn.name, selectOnDrag)
				t.Run(name, func(t *testing.T) {
					m, socket, target := dragFixture(t, strings.NewReplacer("/", "", "sod", "s").Replace(name), cm.script, cm.modes, selectOnDrag)
					m = updateModel(t, m, buttonMsg(t, m, btn.b, tea.MouseActionPress, 3, 2, false))
					m = updateModel(t, m, buttonMsg(t, m, btn.b, tea.MouseActionRelease, 6, 2, false))
					want := wantReports(cm.modes, [4]int{btn.code, int(mouseReportPress), 3, 2}, [4]int{btn.code, int(mouseReportRelease), 3, 2})
					waitForPaneJoined(t, socket, target, want)
					if m.interactiveForwardedButtons != 0 {
						t.Fatalf("held-button mask %b left after the release", m.interactiveForwardedButtons)
					}
				})
			}
		}
	}
}

// TestInteractiveDragKeepsRealMotionAndReleaseCells: a drag that reports
// motion is not a click. The release is at the pointer's cell; the motion is
// sent only to programs that asked for it. Left joins the forwarded drags
// with select_on_drag off (ON is the selection).
func TestInteractiveDragKeepsRealMotionAndReleaseCells(t *testing.T) {
	buttons := []struct {
		name string
		b    tea.MouseButton
		code int
		on   bool
	}{
		{"left", tea.MouseButtonLeft, mouseButtonCodeLeft, false},
		{"middle", tea.MouseButtonMiddle, mouseButtonCodeMiddle, true},
		{"right", tea.MouseButtonRight, mouseButtonCodeRight, true},
	}
	for _, cm := range clickCellModes() {
		for _, btn := range buttons {
			for _, selectOnDrag := range []bool{true, false} {
				if btn.b == tea.MouseButtonLeft && selectOnDrag {
					continue
				}
				name := fmt.Sprintf("%s/%s/sod%v", cm.name, btn.name, selectOnDrag)
				t.Run(name, func(t *testing.T) {
					m, socket, target := dragFixture(t, "d"+strings.NewReplacer("/", "", "sod", "s").Replace(name), cm.script, cm.modes, selectOnDrag)
					m = updateModel(t, m, buttonMsg(t, m, btn.b, tea.MouseActionPress, 3, 2, false))
					m = updateModel(t, m, buttonMsg(t, m, btn.b, tea.MouseActionMotion, 6, 2, false))
					_ = updateModel(t, m, buttonMsg(t, m, btn.b, tea.MouseActionRelease, 6, 2, false))
					steps := [][4]int{{btn.code, int(mouseReportPress), 3, 2}}
					if cm.motion {
						steps = append(steps, [4]int{btn.code, int(mouseReportMotion), 6, 2})
					}
					steps = append(steps, [4]int{btn.code, int(mouseReportRelease), 6, 2})
					waitForPaneJoined(t, socket, target, wantReports(cm.modes, steps...))
				})
			}
		}
	}
}

// TestInteractiveClickAfterDragForgetsTheDragAndOverlapsKeepTheirOwnCells:
// the press cell and the moved flag belong to one gesture. A click after a
// drag of the same button is a click again, and two buttons held at once each
// release at their own press cell, whichever order they come up in.
func TestInteractiveClickAfterDragForgetsTheDragAndOverlapsKeepTheirOwnCells(t *testing.T) {
	modes := interactive.MouseButton | interactive.MouseSGR
	script := `printf '\033[?1002h\033[?1006h'; stty -icanon -echo; echo ready; exec cat -v`
	for _, selectOnDrag := range []bool{true, false} {
		m, socket, target := dragFixture(t, fmt.Sprintf("seq%v", selectOnDrag), script, modes, selectOnDrag)
		for _, b := range []tea.MouseButton{tea.MouseButtonMiddle, tea.MouseButtonRight} {
			m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionPress, 1, 1, false))
			m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionMotion, 5, 1, false))
			m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionRelease, 5, 1, false))
			m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionPress, 8, 4, false))
			m = updateModel(t, m, buttonMsg(t, m, b, tea.MouseActionRelease, 11, 4, false))
			code := mouseButtonCodeMiddle
			if b == tea.MouseButtonRight {
				code = mouseButtonCodeRight
			}
			waitForPaneJoined(t, socket, target, wantReports(modes,
				[4]int{code, int(mouseReportPress), 1, 1}, [4]int{code, int(mouseReportMotion), 5, 1}, [4]int{code, int(mouseReportRelease), 5, 1},
				[4]int{code, int(mouseReportPress), 8, 4}, [4]int{code, int(mouseReportRelease), 8, 4}))
		}

		// Overlap: middle down at (2,1), right down at (6,3); both come up
		// elsewhere, right first.
		m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionPress, 2, 1, false))
		m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionPress, 6, 3, false))
		m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonRight, tea.MouseActionRelease, 9, 5, false))
		_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonMiddle, tea.MouseActionRelease, 10, 6, false))
		waitForPaneJoined(t, socket, target, wantReports(modes,
			[4]int{mouseButtonCodeMiddle, int(mouseReportPress), 2, 1}, [4]int{mouseButtonCodeRight, int(mouseReportPress), 6, 3},
			[4]int{mouseButtonCodeRight, int(mouseReportRelease), 6, 3}, [4]int{mouseButtonCodeMiddle, int(mouseReportRelease), 2, 1}))
	}
}

// TestInteractiveAdditionalButtonClickReleasesAtPressCell: the rule is the
// router's, not the three main buttons': an SGR click of button 8 is reported
// at its press cell too.
func TestInteractiveAdditionalButtonClickReleasesAtPressCell(t *testing.T) {
	modes := interactive.MouseNormal | interactive.MouseSGR
	m, socket, target := dragFixture(t, "extraclick", wheelPaneScript(true), modes, true)
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonBackward, tea.MouseActionPress, 4, 3, false))
	_ = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonBackward, tea.MouseActionRelease, 9, 3, false))
	waitForPaneJoined(t, socket, target, wantReports(modes,
		[4]int{mouseButtonCodeExtraBase, int(mouseReportPress), 4, 3}, [4]int{mouseButtonCodeExtraBase, int(mouseReportRelease), 4, 3}))
}
