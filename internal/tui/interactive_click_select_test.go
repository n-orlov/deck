package tui

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// clickSelectModel is an interactive model over a real grid, with colour on
// so a selection highlight would be visible if one were drawn.
func clickSelectModel(t *testing.T, name string) (Model, string, int) {
	t.Helper()
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	t.Cleanup(func() { oscClipboardWriter = previous })

	socket := selectionTestSocket(name)
	newBareSelectionSession(t, socket, name+"target")
	client := tmux.Client{Socket: socket}
	sess, err := interactive.Start(context.Background(), client, name+"target", 80, 24, func(context.Context) ([]byte, error) {
		return []byte("HELLO WORLD"), nil
	})
	if err != nil {
		t.Fatalf("interactive.Start: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	m := New(nil, config.Settings{Mouse: true, Color: true}, "")
	m.sessions = []store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}}
	m.width, m.height = 100, 30
	m.interactive = true
	m.interactiveGrid = sess
	m.tmuxClient = client
	return m, socket, m.computeLayout().Sidebar.Width + 2
}

func blankPreviewRows(m Model) []string {
	_, h := m.previewContentSize()
	lines := make([]string, h)
	for i := range lines {
		lines[i] = "HELLO WORLD"
	}
	return lines
}

// TestPressAloneDrawsNoSelectionHighlight: a press without motion is a
// click, not a selection, so nothing is marked; the first motion starts the
// marking (SPEC §11.8).
func TestPressAloneDrawsNoSelectionHighlight(t *testing.T) {
	m, _, baseX := clickSelectModel(t, "pressonly")
	lines := blankPreviewRows(m)

	updated, _ := m.Update(press(baseX+2, 1))
	afterPress := updated.(Model)
	if got := afterPress.highlightInProgressSelection(lines, len(lines)); strings.Join(got, "\n") != strings.Join(lines, "\n") {
		t.Fatalf("a press alone drew a highlight: %q", got[0])
	}

	updated, _ = afterPress.Update(motion(baseX+6, 1))
	afterMotion := updated.(Model)
	got := afterMotion.highlightInProgressSelection(lines, len(lines))
	if got[0] == lines[0] {
		t.Fatalf("a drag drew no highlight (control for the press-only case): %q", got[0])
	}
}

// TestNoMotionClickMarksAndCopiesNothing: press then release in place
// leaves no selection state, no highlight, no copy note or error, and no
// tmux buffer.
func TestNoMotionClickMarksAndCopiesNothing(t *testing.T) {
	m, socket, baseX := clickSelectModel(t, "clickmarks")
	lines := blankPreviewRows(m)

	updated, _ := m.Update(press(baseX+2, 1))
	held := updated.(Model)
	if got := held.highlightInProgressSelection(lines, len(lines)); strings.Join(got, "\n") != strings.Join(lines, "\n") {
		t.Fatalf("a held press drew a highlight: %q", got[0])
	}
	updated, _ = held.Update(release(baseX+2, 1))
	after := updated.(Model)
	if after.interactiveSelecting || after.interactiveSelectDragged {
		t.Fatalf("a click left a selection in progress (selecting=%v dragged=%v)", after.interactiveSelecting, after.interactiveSelectDragged)
	}
	if got := after.highlightInProgressSelection(lines, len(lines)); strings.Join(got, "\n") != strings.Join(lines, "\n") {
		t.Fatalf("a click left a highlight: %q", got[0])
	}
	if after.selectionCopyNote != "" || after.attachError != "" {
		t.Fatalf("a click reported a copy (note=%q err=%q)", after.selectionCopyNote, after.attachError)
	}
	if out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput(); err == nil {
		t.Fatalf("a click copied %q", out)
	}
}

// TestEncodeMouseReportButtonsAndActions: every button, in SGR (release
// `m`, the button's own code) and X10 (release is button code 3), for press,
// release and motion.
func TestEncodeMouseReportButtonsAndActions(t *testing.T) {
	sgr := interactive.MouseNormal | interactive.MouseSGR
	x10 := interactive.MouseNormal
	buttons := []struct {
		name string
		code byte
	}{{"left", 0}, {"middle", 1}, {"right", 2}}
	esc := "\x1b"
	sgrCases := []struct {
		kind mouseReportKind
		sfx  string
		add  string
	}{
		{mouseReportPress, "M", ""},
		{mouseReportRelease, "m", ""},
		{mouseReportMotion, "M", "+32"},
	}
	for _, b := range buttons {
		for _, c := range sgrCases {
			code := int(b.code)
			if c.add != "" {
				code += 32
			}
			want := esc + "[<" + strconv.Itoa(code) + ";11;6" + c.sfx
			got, ok := encodeMouseReport(sgr, int(b.code), c.kind, 10, 5)
			if !ok || string(got) != want {
				t.Errorf("sgr %s kind %d: got %q (ok=%v), want %q", b.name, c.kind, got, ok, want)
			}
		}
		x10Want := map[mouseReportKind]byte{
			mouseReportPress:   32 + b.code,
			mouseReportRelease: 32 + x10ReleaseButton,
			mouseReportMotion:  32 + 32 + b.code,
		}
		for kind, code := range x10Want {
			want := []byte{0x1b, '[', 'M', code, 32 + 11, 32 + 6}
			got, ok := encodeMouseReport(x10, int(b.code), kind, 10, 5)
			if !ok || !bytes.Equal(got, want) {
				t.Errorf("x10 %s kind %d: got %q (ok=%v), want %q", b.name, kind, got, ok, want)
			}
		}
	}
}

// TestEncodeMouseReportDropsWhatItCannotCarry: X10 past 223, 1005 and 1015
// and negative cells report nothing for any button.
func TestEncodeMouseReportDropsWhatItCannotCarry(t *testing.T) {
	for _, kind := range []mouseReportKind{mouseReportPress, mouseReportRelease, mouseReportMotion} {
		for _, b := range []int{0, 1, 2} {
			cases := []struct {
				name     string
				modes    interactive.MouseMode
				col, row int
			}{
				{"x10 col past 223", interactive.MouseNormal, 223, 0},
				{"x10 row past 223", interactive.MouseNormal, 0, 223},
				{"1005", interactive.MouseNormal | interactive.MouseUTF8, 0, 0},
				{"1015", interactive.MouseNormal | interactive.MouseURXVT, 0, 0},
				{"negative", interactive.MouseNormal | interactive.MouseSGR, -1, 0},
			}
			for _, c := range cases {
				if got, ok := encodeMouseReport(c.modes, b, kind, c.col, c.row); ok || len(got) != 0 {
					t.Errorf("%s button %d kind %d: got %q (ok=%v), want dropped", c.name, b, kind, got, ok)
				}
			}
		}
	}
}
