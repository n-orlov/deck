package interactive

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// R221 items 3-4 over the R219a capture of copilot 1.0.93: the alt-screen
// enter/exit, scroll regions, the hide/show-cursor storm and the mouse and
// focus modes the stream sets all land in the grid as the stream's own final
// state, never an intermediate one.

// copilotExit is the shutdown run the capture ends with: every mouse and
// focus mode reset, then the alternate screen left.
const copilotExit = "\x1b[?1006l\x1b[?1003l"

// splitCopilotStream cuts the capture where copilot starts shutting down:
// head is everything it drew and set while running (alternate screen on,
// mouse 1003+1006, focus 1004), tail is the shutdown and the summary it
// prints on the main screen.
func splitCopilotStream(t *testing.T) (head, tail []byte) {
	t.Helper()
	raw := readCopilotStream(t)
	i := bytes.Index(raw, []byte(copilotExit))
	if i < 0 {
		t.Fatalf("the capture no longer carries the shutdown run %q", copilotExit)
	}
	return raw[:i], raw[i:]
}

// streamGrid returns a drained grid at the capture's own size (120x40, copilot-PROVENANCE.md) that is
// retired when the test ends.
func streamGrid(t *testing.T) *Grid {
	t.Helper()
	s := &Session{}
	g := s.newDrainedGrid(120, 40)
	t.Cleanup(func() {
		retireGrid(g)
		s.replyDrains.Wait()
	})
	return g
}

// screenRows is the grid's visible rows as plain text, right-trimmed.
func screenRows(g *Grid) []string {
	rows := strings.Split(g.Render(), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(ansiEscapeRe.ReplaceAllString(r, ""), " ")
	}
	return rows
}

func joinedRows(g *Grid) string { return strings.Join(screenRows(g), "\n") }

func writeAll(t *testing.T, g *Grid, chunks ...[]byte) {
	t.Helper()
	for _, c := range chunks {
		if _, err := g.Write(c); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

// TestCopilotAltScreenEnterExitLeavesTheMainScreenSummary: while copilot
// runs its frame is on the alternate screen; once it leaves, the grid shows
// the main screen again -- what was there before copilot started plus the
// summary copilot printed on exit -- and none of the alternate screen's
// frame.
func TestCopilotAltScreenEnterExitLeavesTheMainScreenSummary(t *testing.T) {
	head, tail := splitCopilotStream(t)
	const before = "user@host:~/work$ copilot"

	g := streamGrid(t)
	writeAll(t, g, []byte(before+"\r\n"), head)
	if !g.IsAltScreen() {
		t.Fatalf("the capture's \\e[?1049h did not put the grid on the alternate screen")
	}
	if rows := joinedRows(g); !strings.Contains(rows, "[Current]") || strings.Contains(rows, before) {
		t.Fatalf("while running, the grid must show copilot's frame and not the main screen:\n%s", rows)
	}

	writeAll(t, g, tail)
	if g.IsAltScreen() {
		t.Fatalf("the capture's \\e[?1049l left the grid on the alternate screen")
	}
	rows := joinedRows(g)
	for _, want := range []string{before, "Resume     copilot --resume=5113b778-c479-4319-8cd6-f8c7b4604ca5", "Duration   16s"} {
		if !strings.Contains(rows, want) {
			t.Errorf("after exit the main screen lacks %q:\n%s", want, rows)
		}
	}
	for _, gone := range []string{"[Current]", "Check for mistakes"} {
		if strings.Contains(rows, gone) {
			t.Errorf("after exit the alternate screen's %q is still on the grid:\n%s", gone, rows)
		}
	}
}

// TestCopilotScrollRegionKeepsHeaderAndFooterOnTheAltScreen: copilot's
// header and footer stay put while its transcript scrolls inside a
// `CSI t;b r` region; resetting the region and leaving the alternate screen
// then restores the main screen untouched. The capture's own bytes carry no
// region, so the region run is spliced between its running state and its
// shutdown.
func TestCopilotScrollRegionKeepsHeaderAndFooterOnTheAltScreen(t *testing.T) {
	head, tail := splitCopilotStream(t)
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("transcript %02d", i))
	}
	region := []byte("\x1b[H\x1b[2JHEADER\x1b[40;1HFOOTER\x1b[2;39r\x1b[2;1H" + strings.Join(lines, "\r\n"))
	reset := []byte("\x1b[r\x1b[40;1H")

	g := streamGrid(t)
	writeAll(t, g, []byte("main screen line\r\n"), head, region)
	rows := screenRows(g)
	if rows[0] != "HEADER" || rows[39] != "FOOTER" {
		t.Fatalf("header/footer scrolled with the region: first=%q last=%q", rows[0], rows[39])
	}
	for r := 2; r <= 39; r++ { // 38 region rows hold the last 38 of 60 lines
		if want := lines[r+20]; rows[r-1] != want {
			t.Errorf("region row %d = %q, want %q", r, rows[r-1], want)
		}
	}

	writeAll(t, g, reset, tail)
	final := joinedRows(g)
	if g.IsAltScreen() || !strings.HasPrefix(final, "main screen line") {
		t.Fatalf("leaving the alternate screen did not restore the main screen (alt=%v):\n%s", g.IsAltScreen(), final)
	}
	for _, gone := range []string{"HEADER", "FOOTER", "transcript"} {
		if strings.Contains(final, gone) {
			t.Errorf("after exit the alternate screen's %q is still on the grid:\n%s", gone, final)
		}
	}
	if !strings.Contains(final, "Resume     copilot --resume=") {
		t.Errorf("after exit the summary is missing:\n%s", final)
	}

	// A region set and reset byte by byte reaches the same grid.
	one := streamGrid(t)
	whole := append(append(append([]byte("main screen line\r\n"), head...), region...), append(reset, tail...)...)
	for i := range whole {
		writeAll(t, one, whole[i:i+1])
	}
	if got := joinedRows(one); got != final {
		t.Errorf("byte-by-byte write differs from the one-shot write:\n%s\n--- want ---\n%s", got, final)
	}
}

// toggleEnds returns the offset just past every `CSI ? 25 l/h` in p with the
// state it set.
func toggleEnds(p []byte) (ends []int, visible []bool) {
	for i := 0; i+6 <= len(p); i++ {
		if bytes.HasPrefix(p[i:], []byte("\x1b[?25")) && (p[i+5] == 'l' || p[i+5] == 'h') {
			ends = append(ends, i+6)
			visible = append(visible, p[i+5] == 'h')
		}
	}
	return ends, visible
}

// TestCopilotCursorStormEndsOnTheStreamsFinalState: the capture hides and
// shows the cursor around its drawing; after each toggle the grid's bit --
// and the bit a rendered snapshot reports -- is the one that toggle set, and
// at the end of the stream the cursor is visible.
func TestCopilotCursorStormEndsOnTheStreamsFinalState(t *testing.T) {
	raw := readCopilotStream(t)
	ends, visible := toggleEnds(raw)
	if len(ends) < 3 || visible[len(visible)-1] != true || visible[0] != false {
		t.Fatalf("the capture no longer hides then finally shows the cursor: %v", visible)
	}
	g := streamGrid(t)
	pos := 0
	for i, end := range ends {
		writeAll(t, g, raw[pos:end])
		pos = end
		if got := g.CursorVisible(); got != visible[i] {
			t.Fatalf("after toggle %d (visible=%v) the grid says visible=%v", i, visible[i], got)
		}
	}
	writeAll(t, g, raw[pos:])
	if !g.CursorVisible() {
		t.Errorf("at the end of the stream the cursor is hidden, the stream's last toggle is ?25h")
	}
}

// TestCopilotCursorStormInterleavedWithMovesEndsOnTheLastToggle: hundreds of
// `hide, move, draw, show` rounds -- the storm copilot emits around every
// move -- end on whichever state the stream's last toggle set, whether the
// stream stops mid-round (hidden) or at a round's end (shown), and the
// snapshot a renderer would draw the cursor from agrees.
func TestCopilotCursorStormInterleavedWithMovesEndsOnTheLastToggle(t *testing.T) {
	var storm []byte
	for i := 0; i < 300; i++ {
		storm = append(storm, fmt.Sprintf("\x1b[?25l\x1b[%d;%dH%c\x1b[?25h", i%24+1, i%70+1, 'a'+i%26)...)
	}
	for name, c := range map[string]struct {
		stream []byte
		want   bool
	}{
		"ends shown":               {storm, true},
		"ends hidden mid-round":    {append(append([]byte(nil), storm...), "\x1b[?25l\x1b[3;3Hx"...), false},
		"ends hidden after a hide": {append(append([]byte(nil), storm...), "\x1b[?25l"...), false},
	} {
		s := &Session{}
		s.grid = s.newDrainedGrid(80, 24)
		_, _ = s.grid.Write(c.stream)
		if got := s.grid.CursorVisible(); got != c.want {
			t.Errorf("%s: CursorVisible = %v, want %v", name, got, c.want)
		}
		if snap := s.RenderSnapshot(0, 24); snap.CursorVisible != c.want {
			t.Errorf("%s: snapshot CursorVisible = %v, want %v", name, snap.CursorVisible, c.want)
		}
		retireGrid(s.grid)
		s.replyDrains.Wait()
	}
}

// TestCopilotMouseAndFocusModesAreReportedWhileRunning: the 1003 (any
// motion), 1006 (SGR) and 1004 (focus) modes copilot sets are in the grid's
// reported modes while it runs -- 1003 and 1006 as mouse modes, which is
// what lets the wheel and click forwarding treat it as any mouse-tracking
// full-screen app -- and every one is off again after its shutdown run.
func TestCopilotMouseAndFocusModesAreReportedWhileRunning(t *testing.T) {
	head, tail := splitCopilotStream(t)
	g := streamGrid(t)
	if g.FocusReporting() || g.MouseModes().Any() {
		t.Fatalf("a fresh grid reports modes: mouse=%b focus=%v", g.MouseModes(), g.FocusReporting())
	}
	writeAll(t, g, head)
	if got, want := g.MouseModes(), MouseAny|MouseSGR; got != want {
		t.Errorf("mouse modes while running = %b, want 1003+1006 only (%b)", got, want)
	}
	if !g.FocusReporting() {
		t.Errorf("focus reporting (1004) set by the stream is not reported")
	}
	writeAll(t, g, tail)
	if g.MouseModes().Any() || g.FocusReporting() {
		t.Errorf("after the shutdown run: mouse=%b focus=%v, want everything off", g.MouseModes(), g.FocusReporting())
	}
}

// TestFocusReportingIsNotAMouseMode: mode 1004 alone must not make the pane
// look mouse-tracking, or a wheel notch would be forwarded to a program that
// never asked for the mouse.
func TestFocusReportingIsNotAMouseMode(t *testing.T) {
	g := streamGrid(t)
	writeAll(t, g, []byte("\x1b[?1004h"))
	if !g.FocusReporting() || g.MouseModes().Any() {
		t.Fatalf("after ?1004h: focus=%v mouse=%b, want focus on and no mouse mode", g.FocusReporting(), g.MouseModes())
	}
	writeAll(t, g, []byte("\x1b[?1004l"))
	if g.FocusReporting() {
		t.Fatalf("after ?1004l focus reporting is still on")
	}
}

// TestInteractiveNeverWritesFocusReportsToThePane: the grid reports mode
// 1004 but deck never answers it. No non-test source in this package builds
// the focus-in / focus-out reports (`CSI I`, `CSI O`), calls the emulator's
// Focus or Blur (which write them to the reply pipe), or names the ansi
// package's Focus / Blur constants.
func TestInteractiveNeverWritesFocusReportsToThePane(t *testing.T) {
	if hits := focusReportUses(t, ".", false); len(hits) != 0 {
		t.Fatalf("package interactive writes focus reports:\n%s", strings.Join(hits, "\n"))
	}
	// The scan is only worth anything if it sees what it looks for.
	probe := t.TempDir()
	src := "package p\nimport \"github.com/charmbracelet/x/ansi\"\nvar a = \"\\x1b[I\"\nvar b = ansi.Blur\nfunc f(e interface{ Focus() }) { e.Focus() }\n"
	if err := os.WriteFile(filepath.Join(probe, "p.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := focusReportUses(t, probe, false); len(hits) != 3 {
		t.Fatalf("the scan found %d of the 3 planted uses: %v", len(hits), hits)
	}
}

// focusReportUses lists every use, in the non-test Go files of dir, of a
// string literal carrying CSI I or CSI O, an ansi.Focus/ansi.Blur reference
// or a call of a Focus/Blur method. allowMethods skips the method calls
// (for a package whose own widgets legitimately have Focus methods).
func focusReportUses(t *testing.T, dir string, allowMethods bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		files = append(files, f)
	}
	var hits []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			where := fset.Position(n.Pos()).String()
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind != token.STRING {
					return true
				}
				if v, err := strconv.Unquote(x.Value); err == nil && (strings.Contains(v, "\x1b[I") || strings.Contains(v, "\x1b[O") || strings.Contains(v, "\x9bI") || strings.Contains(v, "\x9bO")) {
					hits = append(hits, where+": literal "+x.Value)
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "ansi" && (x.Sel.Name == "Focus" || x.Sel.Name == "Blur") {
					hits = append(hits, where+": ansi."+x.Sel.Name)
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && !allowMethods && len(x.Args) == 0 && (sel.Sel.Name == "Focus" || sel.Sel.Name == "Blur") {
					hits = append(hits, where+": call of ."+sel.Sel.Name+"()")
				}
			}
			return true
		})
	}
	return hits
}
