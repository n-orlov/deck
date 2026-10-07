// copilot_mouse_test.go covers R221 item 4 end to end: a pane running the
// way copilot 1.0.93 does (alternate screen, mouse 1003 + 1006, focus
// reporting 1004, cursor hidden) is forwarded wheel notches and hover like
// any other mouse-tracking full-screen app, and deck never writes the
// focus-in / focus-out reports (`CSI I`, `CSI O`) into it.
package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// copilotModesScript is the mode run copilot prints at startup (see
// internal/agent/testdata/probes/copilot/stream.raw) and then echoes the
// pty's input byte for byte in caret notation.
const copilotModesScript = `printf '\033[?1049h\033[?2004h\033[?1004h\033[?1003h\033[?1006h\033[?25l'; stty -icanon -echo; echo ready; exec cat -v`

func TestCopilotPaneGetsWheelAndHoverAndNoFocusReports(t *testing.T) {
	m, socket, target := dragFixture(t, "copilot", copilotModesScript, interactive.MouseAny|interactive.MouseSGR, false)
	if got := m.interactiveGrid.Grid().MouseModes(); got != interactive.MouseAny|interactive.MouseSGR {
		t.Fatalf("grid mouse modes = %b, want exactly 1003+1006", got)
	}

	// Window focus changes arrive at the model as bubbletea focus messages;
	// they must not become CSI I / CSI O in the pane, however the pane's
	// mode 1004 is set.
	m = updateModel(t, m, tea.FocusMsg{})
	m = updateModel(t, m, tea.BlurMsg{})
	m = updateModel(t, m, tea.FocusMsg{})

	m = updateModel(t, m, wheelAtPreviewCell(t, m, 2, 1, true, false))
	m = updateModel(t, m, buttonMsg(t, m, tea.MouseButtonNone, tea.MouseActionMotion, 4, 2, false))
	m = updateModel(t, m, tea.BlurMsg{})
	_ = updateModel(t, m, wheelAtPreviewCell(t, m, 7, 4, false, false))
	// Forwarded through the same routing rule as any tracking app.
	for _, report := range []string{"^[[<64;3;2M", "^[[<35;5;3M", "^[[<65;8;5M"} {
		waitForPaneJoined(t, socket, target, report)
	}

	out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-J", "-t", target).Output()
	if err != nil {
		t.Fatalf("capture-pane: %v", err)
	}
	for _, report := range []string{"^[[I", "^[[O"} {
		if strings.Contains(string(out), report) {
			t.Errorf("deck wrote a focus report (%s) into the pane:\n%s", report, out)
		}
	}
}

// TestTUINeverWritesFocusReports: no non-test source of this package builds
// `CSI I` / `CSI O`, names the ansi package's Focus / Blur reports, or turns
// bubbletea's own focus reporting on (which is what would make tea.FocusMsg
// worth answering).
func TestTUINeverWritesFocusReports(t *testing.T) {
	fset := token.NewFileSet()
	var files []*ast.File
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		files = append(files, f)
	}
	var hits []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if v, err := strconv.Unquote(x.Value); x.Kind == token.STRING && err == nil &&
					(strings.Contains(v, "\x1b[I") || strings.Contains(v, "\x1b[O") || strings.Contains(v, "\x9bI") || strings.Contains(v, "\x9bO")) {
					hits = append(hits, fset.Position(x.Pos()).String()+": literal "+x.Value)
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "ansi" && (x.Sel.Name == "Focus" || x.Sel.Name == "Blur") {
					hits = append(hits, fset.Position(x.Pos()).String()+": ansi."+x.Sel.Name)
				}
				if strings.Contains(x.Sel.Name, "ReportFocus") {
					hits = append(hits, fset.Position(x.Pos()).String()+": "+x.Sel.Name)
				}
			}
			return true
		})
	}
	if len(hits) != 0 {
		t.Fatalf("package tui writes or requests focus reports:\n%s", strings.Join(hits, "\n"))
	}
}
