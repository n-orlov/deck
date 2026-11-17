package tmux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// externalResizeAfterRead returns a tmux binary that performs another
// process's `resize-window -t s0 -x 100 -y 27` right after the nth geometry
// read (the `display-message` that asks for #{window_width} #{window_height})
// returns, and records that it did. The read itself is the real one, so the
// fit sees the window as it was BEFORE the external resize, which is the
// interleaving the fit's check-then-mutate used to lose: the resize lands
// after the check and before the fit's next mutation. The shim counts calls
// itself, so the interleaving is identical on every run.
func externalResizeAfterRead(t *testing.T, socket string, nth int) (binary, ranMarker string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "tmux")
	ranMarker = filepath.Join(dir, "external-resizes")
	counter := filepath.Join(dir, "reads")
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
*display-message*'#{window_width} #{window_height}'*)
	tmux "$@" || exit $?
	n=$(cat '%[3]s' 2>/dev/null || echo 0)
	n=$((n+1))
	echo $n > '%[3]s'
	if [ "$n" = '%[4]d' ]; then
		tmux -L '%[1]s' resize-window -t s0 -x 100 -y 27 || exit 1
		echo external >> '%[2]s'
	fi
	exit 0 ;;
esac
exec tmux "$@"
`, socket, ranMarker, counter, nth)
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, ranMarker
}

// requireExternalWindowKept fails unless the other process's resize ran
// exactly once and the window and its `window-size` option are exactly as
// that process left them (100x27, manual).
func requireExternalWindowKept(t *testing.T, client Client, ranMarker string) {
	t.Helper()
	ctx := context.Background()
	if got, err := os.ReadFile(ranMarker); err != nil || strings.Count(string(got), "external") != 1 {
		t.Fatalf("test setup: the other process's resize ran %q times (%v), want exactly once", got, err)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != 100 || h != 27 {
		t.Fatalf("window after the fit = %dx%d (%v), want the other process's 100x27", w, h, err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the fit = set=%v %q (%v), want the other process's manual pin left in place", set, value, err)
	}
}

func newExternalResizeSession(t *testing.T, name string, nth int) (Client, string) {
	t.Helper()
	socket := geometrySocket(name)
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	t.Cleanup(cleanup)
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	binary, ranMarker := externalResizeAfterRead(t, socket, nth)
	return Client{Socket: socket, Binary: binary, Timeout: 5 * time.Second}, ranMarker
}

// TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsBetweenItsReadAndItsResize
// is SPEC §11's "passive fit never undoes a resize another process made" for
// the gap between the fit's geometry read and its resize-window: the read saw
// 80x24, another process then resized the window to 100x27, and the fit must
// not issue its resize-window over that. It fails with the fit's resize a
// separate tmux call after the check (the window ends at the fit's 45x15).
func TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsBetweenItsReadAndItsResize(t *testing.T) {
	client, ran := newExternalResizeSession(t, "casresize", 1)
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest", false}, 45, 15, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit resized between its read and its resize = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requireExternalWindowKept(t, client, ran)
}

// TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsBeforeItsUnpin covers
// the fit's last mutation: the pane already is the wanted size, so the fit
// reads and finishes without a resize, and another process resizes (and pins)
// the window after that read and before the closing unpin. The unpin must not
// run over that pin. It fails with an unconditional unpin (the option ends
// unset).
func TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsBeforeItsUnpin(t *testing.T) {
	client, ran := newExternalResizeSession(t, "casunpin", 1)
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest", false}, 80, 24, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit whose unpin raced a resize = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requireExternalWindowKept(t, client, ran)
}

// TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsAfterItsOwnResizeBeforeItsUnpin
// is the same rule one step further along: the fit's own resize-window landed
// (80x24 to 45x15), the fit's second read confirmed the pane, and the other
// process resized the window after that read, before the unpin. The fit's
// resize count stays 1, and neither the unpin nor anything else touches the
// other process's window.
func TestFitWindowToPaneUnlessResizedKeepsAResizeThatLandsAfterItsOwnResizeBeforeItsUnpin(t *testing.T) {
	client, ran := newExternalResizeSession(t, "caslate", 2)
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest", false}, 45, 15, func() bool { return true })
	if err != nil || resizes != 1 || stop != FitResizedElsewhere {
		t.Fatalf("fit whose unpin raced a resize after its own = (%d resizes, stop %d, %v), want (1, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requireExternalWindowKept(t, client, ran)
}

// TestUnpinWindowSizeIfAtOnlyUnpinsAWindowThatIsStillAtTheGivenSize covers the
// compare-and-set directly, the way the attached-client stand-down uses it:
// a window at another size keeps its pin, one at the size loses it.
func TestUnpinWindowSizeIfAtOnlyUnpinsAWindowThatIsStillAtTheGivenSize(t *testing.T) {
	socket := geometrySocket("unpinifat")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	runTmux(t, socket, "resize-window", "-t", "s0", "-x", "100", "-y", "27")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	unpinned, err := client.UnpinWindowSizeIfAt(ctx, "s0", WindowView{80, 24, "latest", false})
	if err != nil || unpinned {
		t.Fatalf("unpin of a window believed 80x24 that is 100x27 = (%v, %v), want (false, nil)", unpinned, err)
	}
	if _, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set {
		t.Fatalf("window-size after the refused unpin: set=%v (%v), want the pin still set", set, err)
	}
	unpinned, err = client.UnpinWindowSizeIfAt(ctx, "s0", WindowView{100, 27, "manual", true})
	if err != nil || !unpinned {
		t.Fatalf("unpin of a window at its own size = (%v, %v), want (true, nil)", unpinned, err)
	}
	if _, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || set {
		t.Fatalf("window-size after the unpin: set=%v (%v), want unset", set, err)
	}
}

// TestResizeWindowIfAtIssuesNothingForAStaleSize: the resize half of the
// compare-and-set, on its own.
func TestResizeWindowIfAtIssuesNothingForAStaleSize(t *testing.T) {
	socket := geometrySocket("resizeifat")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	applied, err := client.resizeWindowIfAt(ctx, "s0", WindowView{81, 24, "latest", false}, fitWindowSize{50, 20})
	if err != nil || applied {
		t.Fatalf("resize from a stale 81x24 = (%v, %v), want (false, nil)", applied, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 80 || h != 24 {
		t.Fatalf("window after the stale resize = %dx%d, want untouched 80x24", w, h)
	}
	applied, err = client.resizeWindowIfAt(ctx, "s0", WindowView{80, 24, "latest", false}, fitWindowSize{50, 20})
	if err != nil || !applied {
		t.Fatalf("resize from the true 80x24 = (%v, %v), want (true, nil)", applied, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 50 || h != 20 {
		t.Fatalf("window after the resize = %dx%d, want 50x20", w, h)
	}
}
