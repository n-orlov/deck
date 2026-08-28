package tui

import (
	"context"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// newQuietSelectionPane starts a bare tmux session whose ONLY process is a
// long sleep, so the pane emits no bytes at all for the whole test: unlike
// newBareSelectionSession's default shell (whose prompt can reach the grid
// through the armed pipe), a silent pane means the grid's content is
// exactly the seed this test writes into it and nothing else, which is
// what lets the highlighted-cell set and SelectedText's own return value
// be compared as SETS rather than approximately.
func newQuietSelectionPane(t *testing.T, socket, session string, width, height int) {
	t.Helper()
	args := []string{
		"-L", socket, "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height),
		"sleep", "600",
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start quiet tmux session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
}

// highlightedPreviewCells reads the drag-to-copy highlight straight off a
// rendered frame, per CELL: it walks every terminal cell of view, resolves
// which of them belong to the interactive preview's own content box
// through m.previewCellAt (the SAME hit-test the drag itself uses, so a
// sidebar or border cell that happens to carry the selection colour can
// never be mistaken for a highlighted preview cell), and keeps those whose
// BACKGROUND is theme.Selection's own colour. The result is keyed by the
// preview's own (viewRow, col) -- interactive.Session's own view-relative
// row space -- so it can be compared against SelectedText's return value
// without ever parsing an escape sequence or comparing a golden frame:
// what is asserted is which cells came out highlighted on a real emulator
// grid, exactly as a user's terminal would show them.
func highlightedPreviewCells(t *testing.T, m Model, view, selectionHex string) map[int]map[int]string {
	t.Helper()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	cells := map[int]map[int]string{}
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			col, row, ok := m.previewCellAt(x, y)
			if !ok {
				continue
			}
			bg, hasBg := cellBgHex(t, term, x, y)
			if !hasBg || bg != selectionHex {
				continue
			}
			cell := term.CellAt(x, y)
			if cell == nil {
				continue
			}
			if cells[row] == nil {
				cells[row] = map[int]string{}
			}
			cells[row][col] = cell.Content
		}
	}
	return cells
}

// highlightedCellsAsText joins a highlightedPreviewCells map into the same
// shape SelectedText returns -- rows in ascending view order, each row's
// own highlighted columns in ascending order with trailing spaces trimmed
// (SelectedText's own documented trim), rows joined by "\n" -- and fails
// if any row's highlighted columns are not one contiguous run, since a
// gapped highlight would not be a selection at all. It also returns each
// row's own (viewRow, first, last) span so a caller can assert the LINEAR
// shape (a fully highlighted middle row) directly.
func highlightedCellsAsText(t *testing.T, cells map[int]map[int]string) (string, [][3]int) {
	t.Helper()
	rows := make([]int, 0, len(cells))
	for row := range cells {
		rows = append(rows, row)
	}
	sort.Ints(rows)
	var lines []string
	var spans [][3]int
	for _, row := range rows {
		cols := make([]int, 0, len(cells[row]))
		for col := range cells[row] {
			cols = append(cols, col)
		}
		sort.Ints(cols)
		for i := 1; i < len(cols); i++ {
			if cols[i] != cols[i-1]+1 {
				t.Fatalf("view row %d's highlighted columns are not contiguous: %v", row, cols)
			}
		}
		var b strings.Builder
		for _, col := range cols {
			b.WriteString(cells[row][col])
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
		spans = append(spans, [3]int{row, cols[0], cols[len(cols)-1]})
	}
	return strings.Join(lines, "\n"), spans
}

// TestInProgressSelectionHighlightsExactlyTheCellsTheCopyReturns is task
// 206's agreement property (steer 018 / SPEC §11.8: "the selected cells
// are marked with the selection token ... it covers exactly the run
// SelectedText would return"), asserted through the REAL render path --
// Model.View() -> previewBodyLines -> interactiveBodyLines ->
// highlightInProgressSelection/highlightRangeSGR -> a real vt.Emulator --
// rather than against SelectionHighlightRange alone: a no-op or broken
// renderer highlights no cells and fails here, which is exactly what a
// range-only test cannot detect.
//
// The selected run is a GENUINELY WRAPPED one: a single 100-character
// write with no CR and no LF anywhere in it, which the emulator itself
// wraps over three 40-column rows, so the multi-row shape under test is
// the one a real agent's long output produces, not three separate lines
// stitched together by newlines. The drag runs from a high column on the
// first wrapped row to a low column on the third, so a RECTANGULAR
// highlight (columns clipped to the narrower endpoint) is distinguishable
// from the linear one SPEC §11.8 requires -- asserted explicitly on the
// middle row, which a linear selection covers in full.
//
// Finally the release commits the copy, and the text tmux's own named
// selection buffer received is compared against the very same highlighted
// cells: the highlight the user saw and the text the user got are proven
// to be one set, not two independently plausible ones.
func TestInProgressSelectionHighlightsExactlyTheCellsTheCopyReturns(t *testing.T) {
	// The OSC 52 half writes to oscClipboardWriter (os.Stdout in
	// production); discard it here so this test's escape sequence never
	// reaches the test runner's own terminal. Nothing else about the copy
	// path is altered.
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	defer func() { oscClipboardWriter = previous }()

	m := New(nil, config.Settings{Mouse: true, Color: true}, "")
	m.sessions = []store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}}
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	m.width, m.height = 100, 30
	contentWidth, contentHeight := m.previewContentSize()

	// A grid NARROWER than the panel's own content box, so the wrap under
	// test is the grid's own and no panel cropping is involved, and
	// exactly as TALL as the content box, so RenderRows needs no blank
	// top padding and view row i is grid row i.
	const gridWidth = 40
	if contentWidth <= gridWidth || contentHeight < 5 {
		t.Fatalf("test assumption violated: preview content box is %dx%d, want wider than %d and at least 5 rows tall", contentWidth, contentHeight, gridWidth)
	}

	socket := selectionTestSocket("wraphighlight")
	newQuietSelectionPane(t, socket, "wraptarget", gridWidth, contentHeight)
	client := tmux.Client{Socket: socket}

	// 100 characters, no CR, no LF: rows 0 and 1 are full 40-column
	// wrapped continuations of one logical line, row 2 holds its last 20.
	run := strings.Repeat("abcdefghij", 10)
	sess, err := interactive.Start(context.Background(), client, "wraptarget", gridWidth, contentHeight, func(context.Context) ([]byte, error) {
		return []byte(run), nil
	})
	if err != nil {
		t.Fatalf("interactive.Start: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	m.interactive = true
	m.interactiveGrid = sess
	m.tmuxClient = client

	selectionHex := tokenHex(t, m, theme.Selection)
	textHex := tokenHex(t, m, theme.Text)
	if selectionHex == textHex {
		t.Fatalf("test assumption violated: this theme's selection and text tokens share %s, so a background match would be ambiguous", selectionHex)
	}

	// Anchor high on the first wrapped row, release low on the third: a
	// rectangular selection would clip the middle row to columns 5-30.
	const anchorCol, anchorRow = 30, 0
	const curCol, curRow = 5, 2
	baseX := m.computeLayout().Sidebar.Width + 2
	pressX, pressY := baseX+anchorCol, anchorRow+1
	dragX, dragY := baseX+curCol, curRow+1
	if col, row, ok := m.previewCellAt(pressX, pressY); !ok || col != anchorCol || row != anchorRow {
		t.Fatalf("previewCellAt(%d,%d) = (%d,%d,%v), want (%d,%d,true)", pressX, pressY, col, row, ok, anchorCol, anchorRow)
	}
	if col, row, ok := m.previewCellAt(dragX, dragY); !ok || col != curCol || row != curRow {
		t.Fatalf("previewCellAt(%d,%d) = (%d,%d,%v), want (%d,%d,true)", dragX, dragY, col, row, ok, curCol, curRow)
	}

	updated, _ := m.Update(press(pressX, pressY))
	afterPress := updated.(Model)
	updated, _ = afterPress.Update(motion(dragX, dragY))
	afterMotion := updated.(Model)
	if !afterMotion.interactiveSelecting || !afterMotion.interactiveSelectDragged {
		t.Fatalf("press+motion inside the preview did not leave a drag in progress (selecting=%v dragged=%v)", afterMotion.interactiveSelecting, afterMotion.interactiveSelectDragged)
	}

	// What the copy WOULD return for this very anchor/current pair, via
	// the same AbsoluteRow conversion commitInteractiveSelection uses.
	offset := afterMotion.interactiveScrollOffset
	fromRow := sess.AbsoluteRow(offset, contentHeight, anchorRow)
	toRow := sess.AbsoluteRow(offset, contentHeight, curRow)
	want := sess.SelectedText(anchorCol, fromRow, curCol, toRow)
	if want == "" {
		t.Fatalf("test assumption violated: SelectedText returned nothing for the wrapped run")
	}

	cells := highlightedPreviewCells(t, afterMotion, afterMotion.View(), selectionHex)
	got, spans := highlightedCellsAsText(t, cells)
	if got != want {
		t.Fatalf("highlighted cells = %q\nSelectedText     = %q\nthe marked set and the copied set must be the same set", got, want)
	}
	if len(spans) != 3 {
		t.Fatalf("highlight covers %d view rows (%v), want the 3 rows the wrapped run occupies", len(spans), spans)
	}
	if spans[0] != [3]int{anchorRow, anchorCol, gridWidth - 1} {
		t.Fatalf("first selected row's span = %v, want the anchor's own tail {%d,%d,%d}", spans[0], anchorRow, anchorCol, gridWidth-1)
	}
	if spans[1] != [3]int{1, 0, gridWidth - 1} {
		t.Fatalf("middle row's span = %v, want the FULL row {1,0,%d}: a linear selection covers every column of a row strictly between the endpoints, a rectangular one would clip it to columns %d-%d", spans[1], gridWidth-1, curCol, anchorCol)
	}
	if spans[2] != [3]int{curRow, 0, curCol} {
		t.Fatalf("last selected row's span = %v, want the release's own head {%d,0,%d}", spans[2], curRow, curCol)
	}

	// The release commits the copy and the marking clears with it.
	updated, _ = afterMotion.Update(release(dragX, dragY))
	afterRelease := updated.(Model)
	if afterRelease.interactiveSelecting {
		t.Fatalf("release did not end the selection")
	}
	if afterRelease.attachError != "" {
		t.Fatalf("commit reported an error: %q", afterRelease.attachError)
	}
	if left := highlightedPreviewCells(t, afterRelease, afterRelease.View(), selectionHex); len(left) != 0 {
		t.Fatalf("%d view rows still carry the selection highlight after the release committed the copy, want none: %v", len(left), left)
	}

	out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput()
	if err != nil {
		t.Fatalf("show-buffer -b %s: %v: %s", tmux.SelectionBufferName, err, out)
	}
	if copied := strings.TrimRight(string(out), "\n"); copied != got {
		t.Fatalf("tmux buffer received %q, highlighted cells were %q -- the user copied something other than what was marked", copied, got)
	}
}
