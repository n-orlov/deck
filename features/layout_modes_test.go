package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerLayoutModeSteps covers requirement 38 (features/layout_modes.feature):
// auto's 80-column boundary, `|`'s four-state cycle, a mid-scenario resize
// re-choosing auto's mode, a pinned mode falling back below its floors and
// returning when the terminal does, `<`/`>` clamping at both ends, and
// persistence across a restart with config.toml proven unchanged. Every step
// here observes only the released binary's own rendered frame or the
// filesystem, never an internal/tui type, so the detection below re-derives
// §11.3's chrome shape (one shared seam vs. two independent boxes, and the
// collapsed strip's own marker column) from plain text rather than importing
// the layout package.
func registerLayoutModeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" layout is "([^"]+)"$`, clientLayoutIs)
	sc.Step(`^deck client "([^"]+)" presses "([^"]+)" (\d+) times?$`, clientPressesKeyNTimes)
	sc.Step(`^deck client "([^"]+)" sidebar seam column is captured as "([^"]+)"$`, clientSeamColumnIsCapturedAs)
	sc.Step(`^deck client "([^"]+)" sidebar seam column still matches the captured "([^"]+)"$`, clientSeamColumnStillMatchesCaptured)
	sc.Step(`^deck client "([^"]+)" is restarted with terminal size (\d+)x(\d+)$`, clientIsRestartedWithSize)
	sc.Step(`^the scenario's config\.toml is captured as "([^"]+)"$`, scenarioConfigTOMLIsCapturedAs)
	sc.Step(`^the scenario's config\.toml still matches the captured "([^"]+)"$`, scenarioConfigTOMLStillMatchesCaptured)
}

// detectLayoutMode re-derives §11.2's effective mode from the plain rendered
// frame text: it scans past any leading banner lines (e.g. the
// below-minimum stacked notice) for the first panel's top border, then
// classifies by shape alone.
//
//   - No interior seam glyph on that border at all: the sidebar and preview
//     are two fully-bordered, independent boxes stacked vertically (the
//     only shape §11.3 ever draws that way) -> "stacked".
//   - A seam glyph is present, and the following content row's third
//     column (border, one padding column, then content) is the cropMarker
//     glyph ("»"/">") -> the 3-column collapsed strip -> "collapsed".
//   - A seam glyph is present and the marker is absent -> "side-by-side".
func detectLayoutMode(frame string) (string, error) {
	lines := strings.Split(frame, "\n")
	topIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "\u256d") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 || topIdx+1 >= len(lines) {
		return "", fmt.Errorf("no panel top border found in frame:\n%s", frame)
	}
	top := []rune(lines[topIdx])
	if len(top) < 2 {
		return "", fmt.Errorf("top border line too short: %q", lines[topIdx])
	}
	interior := string(top[1 : len(top)-1])
	if !strings.ContainsAny(interior, "+\u252c") {
		return "stacked", nil
	}
	row1 := []rune(lines[topIdx+1])
	if len(row1) >= 3 && (row1[2] == '>' || row1[2] == '\u00bb') {
		return "collapsed", nil
	}
	return "side-by-side", nil
}

func clientLayoutIs(ctx context.Context, name, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	// A resize or a `|` keystroke's redraw lands over several PTY writes, so
	// a single immediate Frame() snapshot can catch the emulator mid-repaint
	// (e.g. a torn frame with one row still at the old width). Poll like
	// WaitForFrame does, rather than reading only once, so this step
	// observes the settled frame the same way every other frame assertion
	// in this package does.
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var (
		frame string
		got   string
		getErr error
	)
	for {
		frame = client.Frame(false)
		got, getErr = detectLayoutMode(frame)
		if getErr == nil && got == want {
			return nil
		}
		select {
		case <-client.done:
			if getErr != nil {
				return fmt.Errorf("client %q exited before layout settled: %w\nframe:\n%s", name, getErr, frame)
			}
			return fmt.Errorf("client %q exited with layout %q, want %q\nframe:\n%s", name, got, want, frame)
		case <-client.updated:
		case <-wait.Done():
			if getErr != nil {
				return fmt.Errorf("client %q: %w (timed out waiting for layout %q)\nframe:\n%s", name, getErr, want, frame)
			}
			return fmt.Errorf("client %q layout = %q, want %q (timed out waiting for it to settle)\nframe:\n%s", name, got, want, frame)
		}
	}
}

// clientPressesKeyNTimes drives a repeated keystroke (e.g. `<`/`>`) one at a
// time with a short pause between presses, matching
// clientWidensSidebarUntilVisible's own documented pacing gotcha: a burst
// sent faster than deck's Bubble Tea input loop drains it can coalesce or
// drop all but the first byte.
func clientPressesKeyNTimes(ctx context.Context, name, key string, n int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := client.Send(key); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// seamColumn finds the column index of the shared border seam on a
// side-by-side/collapsed frame's top border ("+" or "┬" strictly between
// the two corners), which moves exactly as far as `<`/`>` move
// sidebar_width. It deliberately does not care what mode produced the
// frame; callers only use it while known to be in side-by-side/collapsed.
func seamColumn(frame string) (int, error) {
	lines := strings.Split(frame, "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty frame")
	}
	top := []rune(lines[0])
	for i := 1; i < len(top)-1; i++ {
		if top[i] == '+' || top[i] == '\u252c' {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no interior seam found in top border: %q", lines[0])
}

func clientSeamColumnIsCapturedAs(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	col, err := seamColumn(client.Frame(false))
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	if h.layoutSeamSnapshots == nil {
		h.layoutSeamSnapshots = make(map[string]int)
	}
	h.layoutSeamSnapshots[label] = col
	return nil
}

func clientSeamColumnStillMatchesCaptured(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.layoutSeamSnapshots[label]
	if !ok {
		return fmt.Errorf("no sidebar seam column was captured as %q", label)
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	got, err := seamColumn(client.Frame(false))
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("client %q sidebar seam column moved: captured %q as %d, now %d (clamping did not hold)", name, label, want, got)
	}
	return nil
}

// clientIsRestartedWithSize stops the named client (if still running; a
// scenario may have already sent "deck client exits cleanly" first, which
// leaves the harness's bookkeeping entry in place) and starts a fresh one
// under the same name, sharing this scenario's DECK_HOME and tmux socket so
// a restarted client observes exactly the durable state (SPEC §11.2's
// ui_state table, task 016) the previous one persisted.
func clientIsRestartedWithSize(ctx context.Context, name string, cols, rows int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if existing, ok := h.namedClients[name]; ok {
		select {
		case <-existing.done:
			// already exited (e.g. a prior "exits cleanly" step)
		default:
			if err := existing.Stop(5 * time.Second); err != nil {
				return fmt.Errorf("stop deck client %q before restart: %w", name, err)
			}
		}
		delete(h.namedClients, name)
	}
	client, err := h.StartNamedClientWithSize(ctx, name, uint16(cols), uint16(rows))
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

func scenarioConfigTOMLIsCapturedAs(ctx context.Context, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(h.Home, "config.toml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read scenario config.toml: %w", err)
		}
		data = nil
	}
	if h.configTOMLSnapshots == nil {
		h.configTOMLSnapshots = make(map[string]string)
	}
	h.configTOMLSnapshots[label] = string(data)
	return nil
}

func scenarioConfigTOMLStillMatchesCaptured(ctx context.Context, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.configTOMLSnapshots[label]
	if !ok {
		return fmt.Errorf("no config.toml was captured as %q", label)
	}
	data, err := os.ReadFile(filepath.Join(h.Home, "config.toml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read scenario config.toml: %w", err)
		}
	}
	got := string(data)
	if got != want {
		return fmt.Errorf("scenario config.toml changed since capture %q:\nbefore (%d bytes): %q\nafter (%d bytes): %q", label, len(want), want, len(got), got)
	}
	return nil
}
