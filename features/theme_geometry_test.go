package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/theme"
)

// frameSnapshot is a full-grid capture of one settled deck frame: every
// cell's rendered character (content) and, separately, its foreground and
// background colour (as "#rrggbb", or "" when the cell has no colour set --
// terminal default). Keeping content and colour apart is the whole point of
// this file's tests (requirement 32): a theme change must move colour
// without moving a single character.
type frameSnapshot struct {
	cols, rows int
	content    [][]string
	fg         [][]string
	bg         [][]string
}

// captureFrameSnapshot reads every cell of d's current emulator grid
// directly (via CellAt, not the trimmed Frame string), so a theme's border
// colour at the frame's last column is captured even if it happens to be
// blank-adjacent.
func captureFrameSnapshot(d *ScreenDriver) frameSnapshot {
	cols, rows := d.GridSize()
	snap := frameSnapshot{
		cols:    cols,
		rows:    rows,
		content: make([][]string, rows),
		fg:      make([][]string, rows),
		bg:      make([][]string, rows),
	}
	for y := 0; y < rows; y++ {
		snap.content[y] = make([]string, cols)
		snap.fg[y] = make([]string, cols)
		snap.bg[y] = make([]string, cols)
		for x := 0; x < cols; x++ {
			cell := d.CellAt(x, y)
			if cell == nil {
				continue
			}
			snap.content[y][x] = cell.Content
			if hex, err := cellForegroundHex(cell); err == nil {
				snap.fg[y][x] = hex
			}
			if hex, err := cellBackgroundHex(cell); err == nil {
				snap.bg[y][x] = hex
			}
		}
	}
	return snap
}

// assertIdenticalGeometryAndContent fails with the first differing cell's
// position and both themes' characters there if got and want disagree on
// size or on any single cell's rendered character. Colour is deliberately
// not inspected here -- that is countAttributeDifferences' job.
func assertIdenticalGeometryAndContent(t *testing.T, gotName, wantName string, got, want frameSnapshot) {
	t.Helper()
	if got.cols != want.cols || got.rows != want.rows {
		t.Fatalf("theme %q rendered %dx%d, theme %q rendered %dx%d: geometry must not depend on theme",
			gotName, got.cols, got.rows, wantName, want.cols, want.rows)
	}
	for y := 0; y < want.rows; y++ {
		for x := 0; x < want.cols; x++ {
			if got.content[y][x] != want.content[y][x] {
				t.Fatalf("cell (%d,%d) differs by theme alone: %q (theme %q) vs %q (theme %q)",
					x, y, got.content[y][x], gotName, want.content[y][x], wantName)
			}
		}
	}
}

// countAttributeDifferences counts cells whose foreground or background
// colour differs between a and b. A non-zero count is required by every
// caller in this file: two themes that painted every cell identically would
// make the geometry assertion above vacuously true rather than a proof that
// attributes are actually theme-driven.
func countAttributeDifferences(a, b frameSnapshot) int {
	diffs := 0
	for y := 0; y < a.rows && y < b.rows; y++ {
		for x := 0; x < a.cols && x < b.cols; x++ {
			if a.fg[y][x] != b.fg[y][x] || a.bg[y][x] != b.bg[y][x] {
				diffs++
			}
		}
	}
	return diffs
}

// hasAnyColouredCell reports whether at least one cell in snap carries a
// foreground or background colour, the per-cell colour assertion
// requirement 36 asks for under DECK_ASCII=1: ASCII must swap glyphs, never
// strip colour.
func hasAnyColouredCell(snap frameSnapshot) bool {
	for y := 0; y < snap.rows; y++ {
		for x := 0; x < snap.cols; x++ {
			if snap.fg[y][x] != "" || snap.bg[y][x] != "" {
				return true
			}
		}
	}
	return false
}

// renderThemeFrame starts a fresh released deck client whose config.toml
// pins themeName, waits for the settled empty-state frame ("No sessions
// yet"), confirms it has stopped changing, and returns its full-grid
// snapshot. ascii controls DECK_ASCII; colour is always left enabled
// (NO_COLOR is not set) so a theme's real colours are what gets captured.
// socket is shared by every call in one sub-test (never run concurrently
// here) so the sidebar's own "socket: <name>" line is identical across
// themes and never itself the reason content differs.
func renderThemeFrame(t *testing.T, binary, themeName, socket string, ascii bool) frameSnapshot {
	t.Helper()
	home := t.TempDir()
	config := fmt.Sprintf("[ui]\ntheme = %q\n", themeName)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config.toml selecting theme %q: %v", themeName, err)
	}
	env := []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=" + socket,
		"DECK_ANIM=0",
		"DECK_MOUSE=0",
	}
	if ascii {
		env = append(env, "DECK_ASCII=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriverWithSize(ctx, binary, env, 80, 24)
	if err != nil {
		t.Fatalf("start deck client (theme %q, ascii=%v): %v", themeName, ascii, err)
	}
	t.Cleanup(func() {
		_ = driver.Send("q")
		_ = driver.Stop(5 * time.Second)
	})
	if err := driver.WaitForFrame(ctx, false, "No sessions yet"); err != nil {
		t.Fatalf("theme %q (ascii=%v) never rendered its empty state: %v", themeName, ascii, err)
	}
	settled := captureFrameSnapshot(driver)
	// Guard against a still-settling frame (e.g. a start-up banner that has
	// not finished painting) being mistaken for the real, final render.
	time.Sleep(100 * time.Millisecond)
	after := captureFrameSnapshot(driver)
	if fmt.Sprint(settled.content) != fmt.Sprint(after.content) {
		t.Fatalf("theme %q (ascii=%v) frame kept changing after settling", themeName, ascii)
	}
	return after
}

// TestThemeChangesAttributesButNotFrameGeometry is requirement 32: the same
// state, rendered under every built-in theme, must move to identical cell
// positions with identical characters -- only the colour painted onto those
// cells may change. This is proven both with Unicode glyphs (the harness
// default) and, separately, under DECK_ASCII=1 (requirement 36), where the
// same geometry invariant must still hold and colour must still be present
// on at least one cell -- ASCII swaps box-drawing/status glyphs for plain
// characters, it must never also silently disable colour.
func TestThemeChangesAttributesButNotFrameGeometry(t *testing.T) {
	binary := buildDeckBinary(t)
	builtins := theme.Builtins()
	if len(builtins) < 2 {
		t.Fatalf("need at least two built-in themes to prove attributes differ by theme, found %d", len(builtins))
	}

	for _, ascii := range []bool{false, true} {
		label := "unicode"
		if ascii {
			label = "ascii"
		}
		t.Run(label, func(t *testing.T) {
			socket := fmt.Sprintf("deck_theme_geom_%s_%d", label, time.Now().UnixNano())
			snapshots := make([]frameSnapshot, len(builtins))
			for i, th := range builtins {
				snapshots[i] = renderThemeFrame(t, binary, th.Name, socket, ascii)
			}

			base := snapshots[0]
			totalDiffs := 0
			for i := 1; i < len(snapshots); i++ {
				assertIdenticalGeometryAndContent(t, builtins[i].Name, builtins[0].Name, snapshots[i], base)
				diffs := countAttributeDifferences(base, snapshots[i])
				if diffs == 0 {
					t.Fatalf("themes %q and %q painted every cell identically; the attribute comparison is vacuous", builtins[0].Name, builtins[i].Name)
				}
				t.Logf("themes %q vs %q differ in colour at %d of %d cells", builtins[0].Name, builtins[i].Name, diffs, base.cols*base.rows)
				totalDiffs += diffs
			}
			if totalDiffs == 0 {
				t.Fatalf("no theme pair produced any attribute difference")
			}

			if ascii {
				for i, th := range builtins {
					if !hasAnyColouredCell(snapshots[i]) {
						t.Fatalf("theme %q rendered no coloured cell at all under DECK_ASCII=1; colour must survive ASCII mode", th.Name)
					}
				}
			}
		})
	}
}
