package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerMouseBindingSteps backs features/mouse.feature (task 029,
// requirements 33-37, 41). Rather than a hand-computed column/row (which
// would silently drift the moment grouping, elision or a mode change
// shifts where a row actually lands — exactly the failure mode task 028's
// own hitTest guards against product-side), these steps locate the target
// text in the client's own current frame and click through the exact
// cell that text occupies.
func registerMouseBindingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" clicks on the row containing "([^"]+)"$`, clientClicksOnRowContaining)
	sc.Step(`^deck client "([^"]+)" double-clicks on the row containing "([^"]+)"$`, clientDoubleClicksOnRowContaining)
	sc.Step(`^deck client "([^"]+)" detaches$`, clientDetaches)
}

// locateText finds text's first occurrence WITHIN THE SIDEBAR PANEL of
// client's current frame, returning the 1-based column/row an SGR mouse
// report must name to land on that exact cell (matching
// ScreenDriver.Click/DoubleClick/Drag's own 1-based convention,
// features/mouse_synthesis_test.go). Every existing caller
// (clientClicksOnRowContaining/clientDoubleClicksOnRowContaining) means a
// sidebar row -- a session name or a workspace-group header -- never the
// preview. A frame-wide, row-order-first search used to be safe because a
// session's name only ever appeared once, in its own sidebar row; task
// 313/R54 broke that assumption on purpose (SPEC's own safeguard: "the
// preview's top border therefore carries the target session's name as
// text", clientPreviewTopBorderContains's doc comment), so once a name is
// the interactive target it also appears in the preview's top border,
// which sorts before the sidebar's own row in a plain row-order scan and
// silently steals the click (root-caused in
// docs/reports/phase3e-401-r54-noop-discriminator/README.md §3).
// Restricting the search to sidebarRegion's bounds removes the ambiguity
// at the source instead of merely reducing it.
func locateText(client *ScreenDriver, text string) (col, row int, err error) {
	frame := client.Frame(false)
	lines := strings.Split(frame, "\n")
	rowStart, rowEnd, colEnd, regionErr := sidebarRegion(frame)
	if regionErr != nil {
		return 0, 0, fmt.Errorf("locate %q: %w", text, regionErr)
	}
	for i := rowStart; i <= rowEnd && i < len(lines); i++ {
		line := lines[i]
		search := line
		if colEnd >= 0 {
			runes := []rune(line)
			if colEnd < len(runes) {
				search = string(runes[:colEnd])
			}
		}
		if idx := strings.Index(search, text); idx >= 0 {
			return idx + 1, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("no sidebar row of the frame contains %q:\n%s", text, frame)
}

// sidebarRegion returns the row bounds (0-based, inclusive) and column
// bound (0-based, exclusive; -1 meaning "whole line") within which
// locateText must search to be guaranteed to land inside the panel
// internal/tui/mouse.go's hitTest itself calls hitPanelSidebar, reusing
// layout_modes_test.go's own shape detection (detectLayoutMode/seamColumn)
// rather than importing internal/tui, exactly as previewTopBorderText does
// for the mirror-image problem on the preview side.
func sidebarRegion(frame string) (rowStart, rowEnd, colEnd int, err error) {
	lines := strings.Split(frame, "\n")
	mode, err := detectLayoutMode(frame)
	if err != nil {
		return 0, 0, 0, err
	}
	topIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "\u256d") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 {
		return 0, 0, 0, fmt.Errorf("no panel top border found in frame:\n%s", frame)
	}
	if mode == "stacked" {
		// Stacked mode draws the sidebar and preview as two independent,
		// fully-bordered boxes; the sidebar's own bottom border is the
		// first bordered line found below its top border, and the
		// preview's box (whatever it contains) starts only after that.
		bottomIdx := len(lines) - 1
		for i := topIdx + 1; i < len(lines); i++ {
			trimmed := strings.TrimRight(lines[i], " ")
			if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "\u256d") {
				bottomIdx = i
				break
			}
		}
		return topIdx, bottomIdx, -1, nil
	}
	// side-by-side/collapsed: sidebar and preview share one border per row,
	// so the sidebar's own column range is bounded by the shared seam, and
	// its row range is the whole box (shared top border down to the shared
	// bottom border).
	col, err := seamColumn(frame)
	if err != nil {
		return 0, 0, 0, err
	}
	bottomIdx := topIdx
	for i := len(lines) - 1; i > topIdx; i-- {
		trimmed := strings.TrimRight(lines[i], " ")
		if strings.HasPrefix(trimmed, "+") {
			bottomIdx = i
			break
		}
	}
	return topIdx, bottomIdx, col, nil
}

func clientClicksOnRowContaining(ctx context.Context, clientName, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	return client.Click(col, row)
}

func clientDoubleClicksOnRowContaining(ctx context.Context, clientName, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	return client.DoubleClick(col, row)
}

// clientDetaches sends tmux's detach binding (Ctrl-b d), mirroring
// features/assertions_test.go's clientAttachesAndDetaches/
// clientAttachesToSelectedAgentAndDetaches, but as its own step so a
// scenario that attached via a mouse gesture (rather than `\r`) can detach
// without re-deriving that pair of bytes itself.
func clientDetaches(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	if err := client.Send("\x02d"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}
