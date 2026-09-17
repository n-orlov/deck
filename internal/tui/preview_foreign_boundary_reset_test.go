package tui

import (
	"github.com/n-orlov/deck/internal/config"
	"testing"
)

// TestForeignBoundaryResetClosesCapturedSGRUnderNoColor is task
// cure-03-01-2/R118's red-first proof for panel.go's canvasResetIfPainting:
// NO_COLOR disables deck's OWN paint, never the captured pane's own
// attributes. A foreign row can leave SGR open (the pane's own colour,
// never closed by the pane itself); the deck-owned pad/border immediately
// past it must not inherit it, independently of whether deck's own colour
// painting is enabled. Before this task's fix, canvasResetIfPainting gated
// its reset solely on deck's own colour state, so under NO_COLOR the
// foreign-to-deck boundary emitted no reset at all and the pane's colour
// leaked straight into deck's border/pad cells.
func TestForeignBoundaryResetClosesCapturedSGRUnderNoColor(t *testing.T) {
	m := New(nil, config.Settings{Color: false, ASCII: true}, "")
	const width = 24
	const pane = "\x1b[38;2;17;34;51m\x1b[48;2;68;85;102mX"
	for _, stacked := range []bool{false, true} {
		name := "side-by-side"
		if stacked {
			name = "stacked"
		}
		t.Run(name, func(t *testing.T) {
			rows, owners := m.cropPreviewBottomLeft([]byte(pane), 20, 3, 20, 3)
			var rendered string
			if stacked {
				rendered = m.fullBoxPreviewContentLine(width, rows[0], true, owners[0] == previewLineDeckOwned)
			} else {
				rendered = m.previewContentLine(width, rows[0], owners[0] == previewLineDeckOwned)
			}
			term := renderSettingsToEmulator(t, rendered, width, 2)
			if bg, ok := cellBgHex(t, term, 2, 0); !ok || bg != "#445566" {
				t.Fatalf("invalid capture setup: first cell bg=%q/%v", bg, ok)
			}
			if bg, ok := cellBgHex(t, term, width-1, 0); ok {
				t.Errorf("deck right border inherits captured background %q under NO_COLOR", bg)
			}
			if fg, ok := cellFgHex(t, term, width-1, 0); ok {
				t.Errorf("deck right border inherits captured foreground %q under NO_COLOR", fg)
			}
		})
	}
}
