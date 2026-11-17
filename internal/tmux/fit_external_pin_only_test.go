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

// externalCommandAfterRead returns a tmux binary that, right after the nth
// geometry read (the display-message asking for #{window_width}
// #{window_height}) returns, runs one more tmux command line as "another
// process" and records that it did. The read itself is the real one, so the
// fit sees the window as it was before that command.
func externalCommandAfterRead(t *testing.T, socket string, nth int, externalArgs string) (binary, ranMarker string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "tmux")
	ranMarker = filepath.Join(dir, "external")
	counter := filepath.Join(dir, "reads")
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
*display-message*'#{window_width} #{window_height}'*)
	tmux "$@" || exit $?
	n=$(cat '%[3]s' 2>/dev/null || echo 0)
	n=$((n+1))
	echo $n > '%[3]s'
	if [ "$n" = '%[4]d' ]; then
		tmux -L '%[1]s' %[5]s || exit 1
		echo external >> '%[2]s'
	fi
	exit 0 ;;
esac
exec tmux "$@"
`, socket, ranMarker, counter, nth, externalArgs)
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, ranMarker
}

// requirePinOnlyChangeKept fails unless the other process's command ran once
// and the window is still 80x24 with the manual pin that process wrote.
func requirePinOnlyChangeKept(t *testing.T, client Client, ranMarker string) {
	t.Helper()
	ctx := context.Background()
	if got, err := os.ReadFile(ranMarker); err != nil || strings.Count(string(got), "external") != 1 {
		t.Fatalf("test setup: the other process's command ran %q times (%v), want exactly once", got, err)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != 80 || h != 24 {
		t.Fatalf("window after the fit = %dx%d (%v), want the untouched 80x24", w, h, err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the fit = set=%v %q (%v), want the other process's manual pin left in place", set, value, err)
	}
}

func newPinOnlySession(t *testing.T, name, externalArgs string) (Client, string) {
	t.Helper()
	socket := geometrySocket(name)
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	t.Cleanup(cleanup)
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	binary, ran := externalCommandAfterRead(t, socket, 1, externalArgs)
	return Client{Socket: socket, Binary: binary, Timeout: 5 * time.Second}, ran
}

// The pin-only rule (SPEC §11): another process that pins the window WITHOUT
// changing its width or height after the fit read it still made an observable
// change (`window-size` latest -> manual) and the fit must not overwrite it.

func TestFitKeepsASameSizeResizeWindowBetweenItsReadAndItsResize(t *testing.T) {
	client, ran := newPinOnlySession(t, "pinonlyresize", "resize-window -t s0 -x 80 -y 24")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest"}, 45, 15, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requirePinOnlyChangeKept(t, client, ran)
}

func TestFitKeepsASameSizeResizeWindowBetweenItsReadAndItsUnpin(t *testing.T) {
	client, ran := newPinOnlySession(t, "pinonlyunpin", "resize-window -t s0 -x 80 -y 24")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest"}, 80, 24, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requirePinOnlyChangeKept(t, client, ran)
}

// Same rule, a different external command: a bare option write, no resize.
func TestFitKeepsAnExternalWindowSizeOptionWriteBetweenItsReadAndItsUnpin(t *testing.T) {
	client, ran := newPinOnlySession(t, "pinonlyset", "set-option -w -t s0 window-size manual")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest"}, 80, 24, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requirePinOnlyChangeKept(t, client, ran)
}

func TestFitKeepsAnExternalWindowSizeOptionWriteBetweenItsReadAndItsResize(t *testing.T) {
	client, ran := newPinOnlySession(t, "pinonlysetresize", "set-option -w -t s0 window-size manual")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", WindowView{80, 24, "latest"}, 45, 15, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	requirePinOnlyChangeKept(t, client, ran)
}

// A pin another process wrote between the CALLER's read (ReadWindowView) and
// the fit's first read is caught by the fit's own first comparison.
func TestFitKeepsAPinWrittenBetweenTheCallersViewAndTheFit(t *testing.T) {
	socket := geometrySocket("pinonlybefore")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()
	view, err := client.ReadWindowView(ctx, "s0")
	if err != nil || view != (WindowView{80, 24, "latest"}) {
		t.Fatalf("ReadWindowView = %+v (%v), want 80x24 latest", view, err)
	}
	runTmux(t, socket, "resize-window", "-t", "s0", "-x", "80", "-y", "24")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(ctx, "s0", "s0", view, 45, 15, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != 80 || h != 24 {
		t.Fatalf("window after the fit = %dx%d (%v), want the untouched 80x24", w, h, err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the fit = set=%v %q (%v), want the other process's manual pin left in place", set, value, err)
	}
}

// The compare-and-sets themselves: a view with a different pin is stale even
// when the numbers match; the same view applies.
func TestWindowCASTreatsADifferentPinAsStaleAtTheSameSize(t *testing.T) {
	socket := geometrySocket("casdiffpin")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	runTmux(t, socket, "resize-window", "-t", "s0", "-x", "80", "-y", "24")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	if applied, err := client.UnpinWindowSizeIfAt(ctx, "s0", WindowView{80, 24, "latest"}); err != nil || applied {
		t.Fatalf("unpin from a latest view of a manual window = (%v, %v), want (false, nil)", applied, err)
	}
	if applied, err := client.resizeWindowIfAt(ctx, "s0", WindowView{80, 24, "latest"}, fitWindowSize{50, 20}); err != nil || applied {
		t.Fatalf("resize from a latest view of a manual window = (%v, %v), want (false, nil)", applied, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 80 || h != 24 {
		t.Fatalf("window after the refused CASes = %dx%d, want 80x24", w, h)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the refused CASes = set=%v %q (%v), want manual", set, value, err)
	}
	if applied, err := client.UnpinWindowSizeIfAt(ctx, "s0", WindowView{80, 24, "manual"}); err != nil || !applied {
		t.Fatalf("unpin from the true manual view = (%v, %v), want (true, nil)", applied, err)
	}
	if _, set, _ := client.readBuiltinWindowOption(ctx, "s0"); set {
		t.Fatal("window-size still set after the matching unpin")
	}
}

// A pin word that is not a tmux option word is refused instead of being
// pasted into a format condition.
func TestWindowCASRefusesAPinThatIsNotAnOptionWord(t *testing.T) {
	client := Client{Socket: "unused", Timeout: time.Second}
	if _, err := client.UnpinWindowSizeIfAt(context.Background(), "s0", WindowView{80, 24, "a,b}"}); err == nil {
		t.Fatal("unpin with a malformed pin = nil error, want a refusal")
	}
}
