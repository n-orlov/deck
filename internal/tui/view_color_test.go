package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// stripANSI removes every CSI/OSC escape sequence ansiEscapeLen recognises
// from s, leaving only the visible runes — so a coloured line's border/seam
// positions can be checked by rune index without the escape bytes' zero-
// width-but-nonzero-length shifting anything. This is deliberately a
// second, independent implementation from stringWidth/truncateToWidth
// (same recognition rule, different job: strip rather than measure) so
// this test does not just re-check the production width function against
// itself.
func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += ansiEscapeLen(s, i)
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// TestColouredPreviewKeepsFullFrameBordersColumnAligned covers the review
// finding at the *whole rendered frame* level (task 002 covered
// cropPreviewBottomLeft in isolation): with colour enabled (not NO_COLOR,
// so the panel borders themselves also carry SGR escapes via
// m.borderColor) and a live preview whose captured bytes contain SGR
// colour sequences (the shape a real `tmux capture-pane -e` returns for a
// coloured pane), every line Model.View() renders for the two panels must
// still have exactly the terminal's visible width, and the seam between
// the sidebar and the preview — plus the preview's own right border — must
// land in the same column on every single row, top border, content rows
// and bottom border alike. Before task 001/002's escape-aware
// stringWidth/truncateToWidth fix, a coloured content row's escape bytes
// were spent as columns, so its rendered width fell short of contentWidth
// by the escape bytes' length and the border after it sheared out of
// column with every other row's border.
func TestColouredPreviewKeepsFullFrameBordersColumnAligned(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.sessions = []store.Session{{ID: "sess-1", Name: "coloured-session", Agent: "shell", Status: "running"}}
	m.selected = 0
	m.previewLive = true
	m.previewSessionID = "sess-1"

	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		t.Fatalf("default 80x24 frame computed as stacked, want side-by-side (test assumes a shared seam)")
	}
	sw := layout.Sidebar.Width
	contentHeight := layout.Sidebar.Height - 2
	contentWidth := layout.Preview.Width - 4
	if contentHeight <= 0 || contentWidth <= 0 {
		t.Fatalf("degenerate layout: contentHeight=%d contentWidth=%d", contentHeight, contentWidth)
	}

	rows := make([]string, contentHeight)
	for i := range rows {
		rows[i] = "\x1b[31mRED\x1b[0m row"
	}
	m.previewPaneWidth = contentWidth
	m.previewPaneHeight = contentHeight
	m.previewBytes = []byte(strings.Join(rows, "\n"))

	view := m.View()
	if !strings.Contains(view, "\x1b[31m") {
		t.Fatalf("rendered view lost the preview's SGR colour escape entirely:\n%q", view)
	}

	width, _ := m.frameSize()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatalf("no lines rendered")
	}
	// The footer (last line) carries no panel borders; every line above it
	// belongs to the two panels' frame (top border, content rows, bottom
	// border).
	frame := lines[:len(lines)-1]
	if len(frame) == 0 {
		t.Fatalf("no frame lines rendered")
	}

	bc := m.box()
	seamColumn := map[string]bool{bc.vertical: true, bc.seamTop: true, bc.seamBottom: true}
	rightColumn := map[string]bool{bc.vertical: true, bc.topRight: true, bc.bottomRight: true}

	for i, line := range frame {
		stripped := stripANSI(line)
		r := []rune(stripped)
		if len(r) != width {
			t.Fatalf("frame line %d has visible width %d, want %d (terminal width): %q", i, len(r), width, stripped)
		}
		if sw >= len(r) {
			t.Fatalf("sidebar width %d is not less than the rendered line's %d runes", sw, len(r))
		}
		if got := string(r[sw]); !seamColumn[got] {
			t.Fatalf("frame line %d column %d (the seam, first column of the preview panel) = %q, want one of the seam/vertical border glyphs %v: %q", i, sw, got, seamColumn, stripped)
		}
		if got := string(r[len(r)-1]); !rightColumn[got] {
			t.Fatalf("frame line %d's last column = %q, want the preview's right border/corner glyph %v: %q", i, got, rightColumn, stripped)
		}
	}
}
