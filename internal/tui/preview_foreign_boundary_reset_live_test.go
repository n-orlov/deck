package tui

import (
	"context"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestForeignBoundaryResetClosesRealTmuxCaptureUnderNoColor is task
// cure-03-01-2/R118's companion proof against a REAL tmux CapturePreview
// (not a synthetic byte string): a full-width row that opens truecolour
// fg/bg and never closes it, captured through the actual tmux client, must
// not leak into deck's own right-border column in Model.View() -- in
// EITHER layout, and with the colour-enabled control proving the same
// scenario already worked before this task (so a NO_COLOR-only red here
// is a genuine colour-disabled-boundary gap, not a fixture defect).
func TestForeignBoundaryResetClosesRealTmuxCaptureUnderNoColor(t *testing.T) {
	socket := selectionTestSocket("foreign-boundary-no-color")
	const slug = "foreign-boundary-no-color"
	target, _ := tmux.SessionName(slug)
	newQuietSelectionPane(t, socket, target, 80, 24)
	cmd := exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", target, "printf '\033[38;2;17;34;51m\033[48;2;68;85;102m'; i=0; while [ $i -lt 80 ]; do printf X; i=$((i+1)); done; sleep 60")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("respawn: %v %s", err, out)
	}
	client := tmux.Client{Socket: socket}
	var capture tmux.PreviewCapture
	deadline := time.Now().Add(3 * time.Second)
	for {
		var err error
		capture, err = client.CapturePreview(context.Background(), slug)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(capture.Bytes), "X") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pane never produced marker")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("real capture prefix: %q", string(capture.Bytes[:min(len(capture.Bytes), 160)]))
	for _, stacked := range []bool{false, true} {
		for _, color := range []bool{false, true} {
			name := "side-by-side"
			if stacked {
				name = "stacked"
			}
			if color {
				name += "/color-control"
			} else {
				name += "/NO_COLOR"
			}
			t.Run(name, func(t *testing.T) {
				m := New(nil, config.Settings{Color: color, ASCII: true, Theme: theme.Builtins()[0]}, "")
				m.width, m.height = 120, 80
				if stacked {
					m.layoutMode = LayoutStacked
				}
				m.sessions = []store.Session{{ID: "s", Name: "foreign", Slug: slug, Agent: "shell", Status: "running"}}
				m.selected = 0
				m.previewLive = true
				m.previewSessionID = "s"
				m.previewBytes = capture.Bytes
				m.previewPaneWidth = capture.Width
				m.previewPaneHeight = capture.Height
				view := m.View()
				if !strings.Contains(view, "XXXX") {
					t.Fatal("fixture error: colored capture row was cropped out")
				}
				term := renderSettingsToEmulator(t, view, m.width, m.height)
				for row := 1; row < m.height-1; row++ {
					if bg, ok := cellBgHex(t, term, m.width-1, row); ok && bg == "#445566" {
						t.Fatalf("real capture leaked background %s into deck border at (%d,%d) color=%v", bg, m.width-1, row, color)
					}
					if fg, ok := cellFgHex(t, term, m.width-1, row); ok && fg == "#112233" {
						t.Fatalf("real capture leaked foreground %s into deck border at (%d,%d) color=%v", fg, m.width-1, row, color)
					}
				}
			})
		}
	}
}
