package tui

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestHighlightRangeSGRReassertsBackgroundAcrossContentSGRs is GH #18's
// unit-level regression, straight at highlightRangeSGR: a captured pane
// row carries its OWN SGR sequences (a coloured token's foreground
// selection and the \x1b[0m reset that ends it), and before the fix the
// reset -- passed through untouched, like every content escape -- also
// wiped the selection background for the rest of the row, so the
// highlight visibly died at the first coloured character and stayed dead
// until the next row opened a fresh span. The fixture row here is exactly
// that shape (fixture SGR bytes standing in for a tmux capture's, the
// no_literal_color_test.go exemption), the open/close sequences are the
// production pair (backgroundSGR(theme.Selection)/selectionCloseSGR), and
// the result is read per-cell off a real vt.Emulator: every cell of the
// span must carry the selection background -- INCLUDING the cells after
// the content's reset -- while the coloured cells keep the content's own
// foreground, and the cells past the span's end must carry no background
// at all.
func TestHighlightRangeSGRReassertsBackgroundAcrossContentSGRs(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 12, 1
	openSeq, ok := m.backgroundSGR(theme.Selection)
	if !ok {
		t.Fatalf("backgroundSGR(theme.Selection) unavailable with Color enabled")
	}
	selectionHex := tokenHex(t, m, theme.Selection)

	const contentFgHex = "#ff5050"
	contentFg := "\x1b[38;2;255;80;80m"
	line := "ab" + contentFg + "cd" + "\x1b[0m" + "efgh"

	// Span columns 0-5: "abcdef" -- the content's reset sits between
	// columns 3 and 4, INSIDE the span.
	out := highlightRangeSGR(line, 0, 5, openSeq, selectionCloseSGR)
	term := renderSettingsToEmulator(t, out, m.width, m.height)

	for col := 0; col <= 5; col++ {
		bg, hasBg := cellBgHex(t, term, col, 0)
		if !hasBg || bg != selectionHex {
			t.Errorf("cell %d: background = %q (present=%v), want the selection token's %s -- the pane content's own SGR between columns 3 and 4 must not strip the selection background from the rest of the span", col, bg, hasBg, selectionHex)
		}
	}
	for _, col := range []int{2, 3} {
		fg, hasFg := cellFgHex(t, term, col, 0)
		if !hasFg || fg != contentFgHex {
			t.Errorf("cell %d: foreground = %q (present=%v), want the content's own %s -- the selection highlight changes only the background", col, fg, hasFg, contentFgHex)
		}
	}
	for _, col := range []int{6, 7} {
		if bg, hasBg := cellBgHex(t, term, col, 0); hasBg {
			t.Errorf("cell %d: background = %q, want none -- the span closed at column 5 and the re-asserted background must not leak past it", col, bg)
		}
	}
}

// TestInProgressSelectionHighlightSurvivesColoredPaneContent is GH #18's
// render-path regression, asserted the way
// TestInProgressSelectionHighlightsExactlyTheCellsTheCopyReturns asserts
// its wrapped-run agreement: through Model.View() ->
// highlightInProgressSelection -> a real vt.Emulator, never against
// highlightRangeSGR alone. The pane's seeded row holds a coloured token
// mid-line (its own truecolor foreground plus the reset ending it --
// exactly what a real agent's syntax-highlighted output puts in a
// captured row), the drag spans across that token, and the highlighted
// cell set must STILL equal SelectedText's run: before the fix the
// token's reset killed the selection background mid-row, so the
// highlighted set came out truncated at the first coloured character.
// The coloured cells themselves must keep the content's foreground while
// carrying the selection background -- a highlight that repainted the
// foreground too would pass a background-only check while destroying the
// very syntax colouring §11.8's background-only design preserves.
func TestInProgressSelectionHighlightSurvivesColoredPaneContent(t *testing.T) {
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	defer func() { oscClipboardWriter = previous }()

	m := New(nil, config.Settings{Mouse: true, Color: true}, "")
	m.sessions = []store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}}
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	m.width, m.height = 100, 30
	contentWidth, contentHeight := m.previewContentSize()

	const gridWidth = 40
	if contentWidth <= gridWidth || contentHeight < 3 {
		t.Fatalf("test assumption violated: preview content box is %dx%d, want wider than %d and at least 3 rows tall", contentWidth, contentHeight, gridWidth)
	}

	socket := selectionTestSocket("colorhighlight")
	newQuietSelectionPane(t, socket, "colortarget", gridWidth, contentHeight)
	client := tmux.Client{Socket: socket}

	// One 26-column row: a truecolor token at columns 4-6, reset, then
	// plain text -- the reset lands strictly inside the drag below.
	const contentFgHex = "#ff5050"
	seedRow := "pre \x1b[38;2;255;80;80mHOT\x1b[0mtail plus more"
	sess, err := interactive.Start(context.Background(), client, "colortarget", gridWidth, contentHeight, func(context.Context) ([]byte, error) {
		return []byte(seedRow), nil
	})
	if err != nil {
		t.Fatalf("interactive.Start: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	m.interactive = true
	m.interactiveGrid = sess
	m.tmuxClient = client

	selectionHex := tokenHex(t, m, theme.Selection)
	if selectionHex == contentFgHex {
		t.Fatalf("test assumption violated: this theme's selection token is the fixture's own %s, so the assertions below would be ambiguous", contentFgHex)
	}

	// Anchor before the coloured token, release well after its reset: the
	// span 1-15 contains the token (columns 4-6) and the reset after it.
	const anchorCol, curCol, row = 1, 15, 0
	baseX := m.computeLayout().Sidebar.Width + 2
	pressX, pressY := baseX+anchorCol, row+1
	dragX, dragY := baseX+curCol, row+1
	if col, r, ok := m.previewCellAt(pressX, pressY); !ok || col != anchorCol || r != row {
		t.Fatalf("previewCellAt(%d,%d) = (%d,%d,%v), want (%d,%d,true)", pressX, pressY, col, r, ok, anchorCol, row)
	}
	if col, r, ok := m.previewCellAt(dragX, dragY); !ok || col != curCol || r != row {
		t.Fatalf("previewCellAt(%d,%d) = (%d,%d,%v), want (%d,%d,true)", dragX, dragY, col, r, ok, curCol, row)
	}

	updated, _ := m.Update(press(pressX, pressY))
	afterPress := updated.(Model)
	updated, _ = afterPress.Update(motion(dragX, dragY))
	afterMotion := updated.(Model)
	if !afterMotion.interactiveSelecting || !afterMotion.interactiveSelectDragged {
		t.Fatalf("press+motion inside the preview did not leave a drag in progress (selecting=%v dragged=%v)", afterMotion.interactiveSelecting, afterMotion.interactiveSelectDragged)
	}

	offset := afterMotion.interactiveScrollOffset()
	absRow := sess.AbsoluteRow(offset, contentHeight, row)
	want := sess.SelectedText(anchorCol, absRow, curCol, absRow)
	if !strings.Contains(want, "HOT") {
		t.Fatalf("test assumption violated: SelectedText = %q, want it to contain the coloured token", want)
	}

	view := afterMotion.View()
	cells := highlightedPreviewCells(t, afterMotion, view, selectionHex)
	got, spans := highlightedCellsAsText(t, cells)
	if got != want {
		t.Fatalf("highlighted cells = %q\nSelectedText     = %q\nthe pane's own SGR reset inside the span must not truncate the highlight (GH #18)", got, want)
	}
	if len(spans) != 1 || spans[0] != [3]int{row, anchorCol, curCol} {
		t.Fatalf("highlight spans = %v, want exactly {%d,%d,%d}", spans, row, anchorCol, curCol)
	}

	// The coloured cells keep their own foreground under the highlight.
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	for i, col := 0, baseX+4; i < 3; i, col = i+1, col+1 {
		fg, hasFg := cellFgHex(t, term, col, pressY)
		if !hasFg || fg != contentFgHex {
			t.Errorf("coloured cell %d (screen col %d): foreground = %q (present=%v), want the content's own %s under the selection background", i, col, fg, hasFg, contentFgHex)
		}
	}

	updated, _ = afterMotion.Update(release(dragX, dragY))
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
	if copied := strings.TrimRight(string(out), "\n"); copied != want {
		t.Fatalf("tmux buffer received %q, want %q -- the copy must be unaffected by the highlight's SGR re-assertion", copied, want)
	}
}
