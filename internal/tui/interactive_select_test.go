package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// selectionTestSocket/newBareSelectionSession mirror
// internal/interactive/grid_test.go's own newBareInteractiveSession: a
// bare tmux session with no deck session/pane record behind it at all,
// just enough to exercise interactive.Start's real pipe-pane path and
// tmux.Client.SetSelectionBuffer's real `load-buffer` against a genuine
// tmux server, rather than a hand-built fixture (this package's own
// task 216/steer 017 item 3 needs the whole selection round trip proven
// against the real primitive it commits to, not merely the geometry).
func selectionTestSocket(name string) string {
	return fmt.Sprintf("deck-tui-select-%s-%d-%d", name, time.Now().UnixNano(), time.Now().UnixNano()%997)
}

func newBareSelectionSession(t *testing.T, socket, session string, width, height int) {
	t.Helper()
	args := []string{"-L", socket, "new-session", "-d", "-s", session, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start bare tmux session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
}

// TestPreviewCellAtSideBySideMapsContentBox proves previewCellAt resolves
// exactly the content box previewContentSize/previewCellAtSideBySide
// both describe -- border, padding and the sidebar/seam all refused,
// only the interior content cells accepted, at the exact column/row
// hitTestSideBySide itself would call hitPanelPreview.
func TestPreviewCellAtSideBySideMapsContentBox(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	if _, _, ok := m.previewCellAt(sw, 5); ok {
		t.Fatalf("the seam column (x=%d) resolved inside the preview content box", sw)
	}
	if _, _, ok := m.previewCellAt(sw+1, 5); ok {
		t.Fatalf("the preview's own left border (x=%d) resolved inside its content box", sw+1)
	}
	col, row, ok := m.previewCellAt(sw+2, 1)
	if !ok {
		t.Fatalf("previewCellAt(%d, 1) refused, want the content box's own (0,0)", sw+2)
	}
	if col != 0 || row != 0 {
		t.Fatalf("previewCellAt(%d, 1) = (%d, %d), want (0, 0)", sw+2, col, row)
	}
	contentWidth, contentHeight := m.previewContentSize()
	col, row, ok = m.previewCellAt(sw+2+contentWidth-1, contentHeight)
	if !ok || col != contentWidth-1 || row != contentHeight-1 {
		t.Fatalf("previewCellAt at the content box's own bottom-right corner = (%d, %d, %v), want (%d, %d, true)", col, row, ok, contentWidth-1, contentHeight-1)
	}
	if _, _, ok := m.previewCellAt(sw+2+contentWidth, contentHeight); ok {
		t.Fatalf("one column past the content box's own right edge still resolved inside it")
	}
}

// TestPreviewCellAtStackedMapsContentBox is the same proof against the
// stacked (below-80-column) layout, whose preview panel is a second,
// fully-bordered box below the sidebar's own rather than sharing one
// border with a seam.
func TestPreviewCellAtStackedMapsContentBox(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 60, 40
	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("test assumption violated: 60x40 did not select the stacked layout (got %v)", layout.Effective)
	}
	lh := layout.Sidebar.Height

	col, row, ok := m.previewCellAt(2, lh+1)
	if !ok || col != 0 || row != 0 {
		t.Fatalf("previewCellAt(2, %d) = (%d, %d, %v), want (0, 0, true)", lh+1, col, row, ok)
	}
	if _, _, ok := m.previewCellAt(0, lh+1); ok {
		t.Fatalf("the stacked preview's own left border resolved inside its content box")
	}
	if _, _, ok := m.previewCellAt(2, lh); ok {
		t.Fatalf("the stacked preview's own top border resolved inside its content box")
	}
}

// TestPreviewClampToContentClampsOffPanelCoordinates proves the drag-
// continuation half never refuses: unlike previewCellAt, any coordinate
// -- however far off the panel -- resolves to the nearest still-in-bounds
// content cell.
func TestPreviewClampToContentClampsOffPanelCoordinates(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	contentWidth, contentHeight := m.previewContentSize()

	if col, row := m.previewClampToContent(-100, -100); col != 0 || row != 0 {
		t.Fatalf("previewClampToContent(-100,-100) = (%d, %d), want (0, 0)", col, row)
	}
	if col, row := m.previewClampToContent(100000, 100000); col != contentWidth-1 || row != contentHeight-1 {
		t.Fatalf("previewClampToContent(way off bottom-right) = (%d, %d), want (%d, %d)", col, row, contentWidth-1, contentHeight-1)
	}
}

// TestBeginInteractiveSelectionScope proves beginInteractiveSelection's
// own stated scope: it refuses (ok=false, m unchanged) unless m.interactive
// is true AND m.interactiveGrid is set AND the press resolved inside the
// preview's own content box, and only then starts a selection.
func TestBeginInteractiveSelectionScope(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	layout := m.computeLayout()
	previewX := layout.Sidebar.Width + 3

	if _, ok := m.beginInteractiveSelection(previewX, 5); ok {
		t.Fatalf("beginInteractiveSelection succeeded while m.interactive is false")
	}

	m.interactive = true
	if _, ok := m.beginInteractiveSelection(previewX, 5); ok {
		t.Fatalf("beginInteractiveSelection succeeded with a nil interactiveGrid")
	}

	m.interactiveGrid = &interactive.Session{}
	updated, ok := m.beginInteractiveSelection(previewX, 5)
	if !ok {
		t.Fatalf("beginInteractiveSelection refused a press inside the preview's own content box")
	}
	if !updated.interactiveSelecting {
		t.Fatalf("beginInteractiveSelection did not set interactiveSelecting")
	}
	if updated.interactiveSelectDragged {
		t.Fatalf("beginInteractiveSelection alone (no motion yet) set interactiveSelectDragged")
	}

	if _, ok := m.beginInteractiveSelection(layout.Sidebar.Width, 5); ok {
		t.Fatalf("beginInteractiveSelection succeeded on the seam column")
	}
}

// TestClickOverInteractivePreviewWithoutMotionCommitsNothing proves the
// stated exception's own boundary (SPEC §11.8: "a click over the preview
// does nothing; a drag selects"): a press immediately followed by a
// release, with no intervening motion event, must not write anything
// into deck's own selection buffer at all.
func TestClickOverInteractivePreviewWithoutMotionCommitsNothing(t *testing.T) {
	socket := selectionTestSocket("click")
	newBareSelectionSession(t, socket, "clicktarget", 80, 24)
	client := tmux.Client{Socket: socket}

	sess, err := interactive.Start(context.Background(), client, "clicktarget", 80, 24, func(context.Context) ([]byte, error) {
		return []byte("HELLO WORLD"), nil
	})
	if err != nil {
		t.Fatalf("interactive.Start: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	m.interactive = true
	m.interactiveGrid = sess
	m.tmuxClient = client
	layout := m.computeLayout()
	previewX := layout.Sidebar.Width + 3

	updated, _ := m.Update(press(previewX, 3))
	afterPress := updated.(Model)
	if !afterPress.interactiveSelecting {
		t.Fatalf("press over the preview's content box did not start a selection")
	}
	updated, _ = afterPress.Update(release(previewX, 3))
	afterRelease := updated.(Model)
	if afterRelease.interactiveSelecting {
		t.Fatalf("release did not end the selection")
	}

	out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput()
	if err == nil {
		t.Fatalf("show-buffer succeeded after a plain click (no drag): %q -- a click must commit nothing", string(out))
	}
}

// TestDragOverInteractivePreviewCopiesSelectedTextToTheNamedTmuxBuffer is
// the full round trip end to end against a real tmux server: a real
// interactive.Session fed real pane content, a press inside its content
// box, a motion elsewhere on the SAME row (a genuine drag), and a
// release -- proving the exact text between those two cells lands in
// deck's own named tmux buffer (tmux.SelectionBufferName), the load-
// bearing half SPEC §11.8 describes.
func TestDragOverInteractivePreviewCopiesSelectedTextToTheNamedTmuxBuffer(t *testing.T) {
	// The OSC 52 half writes straight to oscClipboardWriter (os.Stdout in
	// production); swapping it to io.Discard here keeps this test's own
	// escape sequence out of the test runner's real terminal -- the OSC 52
	// byte shape itself is proven separately, below.
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	defer func() { oscClipboardWriter = previous }()

	socket := selectionTestSocket("drag")
	newBareSelectionSession(t, socket, "dragtarget", 80, 24)
	client := tmux.Client{Socket: socket}

	sess, err := interactive.Start(context.Background(), client, "dragtarget", 80, 24, func(context.Context) ([]byte, error) {
		return []byte("HELLO WORLD"), nil
	})
	if err != nil {
		t.Fatalf("interactive.Start: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	m.interactive = true
	m.interactiveGrid = sess
	m.tmuxClient = client
	layout := m.computeLayout()
	baseX := layout.Sidebar.Width + 2 // content col 0

	// "HELLO" occupies content columns 0-4 on the grid's own first row.
	pressX, pressY := baseX+0, 1
	releaseX := baseX + 4

	updated, _ := m.Update(press(pressX, pressY))
	afterPress := updated.(Model)
	updated, _ = afterPress.Update(motion(releaseX, pressY))
	afterMotion := updated.(Model)
	if !afterMotion.interactiveSelectDragged {
		t.Fatalf("a motion event after the press did not mark the selection dragged")
	}
	updated, _ = afterMotion.Update(release(releaseX, pressY))
	afterRelease := updated.(Model)
	if afterRelease.interactiveSelecting {
		t.Fatalf("release did not end the selection")
	}
	if afterRelease.attachError != "" {
		t.Fatalf("commit reported an error: %q", afterRelease.attachError)
	}

	out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput()
	if err != nil {
		t.Fatalf("show-buffer -b %s: %v: %s", tmux.SelectionBufferName, err, out)
	}
	if got := strings.TrimRight(string(out), "\n"); got != "HELLO" {
		t.Fatalf("tmux selection buffer = %q, want %q", got, "HELLO")
	}
}

// TestWriteOSCClipboardBestEffortEmitsTheStandardEscapeSequence proves
// the OSC 52 half's own byte shape: `ESC ] 52 ; c ; <base64> BEL`, the
// exact sequence go-osc52's own doc names, with the ORIGINAL text
// recoverable by base64-decoding the payload segment.
func TestWriteOSCClipboardBestEffortEmitsTheStandardEscapeSequence(t *testing.T) {
	var buf bytes.Buffer
	previous := oscClipboardWriter
	oscClipboardWriter = &buf
	defer func() { oscClipboardWriter = previous }()

	writeOSCClipboardBestEffort("copy me")

	got := buf.String()
	const prefix, suffix = "\x1b]52;c;", "\x07"
	if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) {
		t.Fatalf("OSC 52 write %q does not have the expected ESC]52;c;...BEL envelope", got)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(got, prefix), suffix)
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload %q is not valid base64: %v", payload, err)
	}
	if string(decoded) != "copy me" {
		t.Fatalf("decoded OSC 52 payload = %q, want %q", decoded, "copy me")
	}
}

var _ tea.Model = Model{}
