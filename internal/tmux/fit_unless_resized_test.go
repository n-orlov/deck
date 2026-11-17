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

// TestFitWindowToPaneUnlessResizedLeavesAnotherProcessesResizeAlone covers
// passive fit's promise (SPEC §11) at the tmux layer: a fit whose window
// is no longer at the size the fit started from has been resized by
// someone else, and the fit leaves that window exactly as it found it: no
// resize-window and no unpin. A fit from the right size, and a fit
// its caller no longer wants, behave as the outcome names.
func TestFitWindowToPaneUnlessResizedLeavesAnotherProcessesResizeAlone(t *testing.T) {
	socket := geometrySocket("unlessresized")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Another process resizes the window after the fit read 80x24.
	runTmux(t, socket, "resize-window", "-t", "s0", "-x", "100", "-y", "27")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(ctx, "s0", "s0", WindowView{80, 24, "latest", false}, 45, 15, func() bool { return true })
	if err != nil || resizes != 0 || stop != FitResizedElsewhere {
		t.Fatalf("fit started from 80x24 on a window resized to 100x27 = (%d resizes, stop %d, %v), want (0, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != 100 || h != 27 {
		t.Fatalf("window after the stood-down fit = %dx%d (%v), want the other process's 100x27", w, h, err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the stood-down fit = set=%v %q (%v), want the other process's manual pin left in place", set, value, err)
	}

	// The caller no longer wants the fit: nothing is issued either.
	resizes, stop, err = client.FitWindowToPaneUnlessResized(ctx, "s0", "s0", WindowView{100, 27, "manual", true}, 45, 15, func() bool { return false })
	if err != nil || resizes != 0 || stop != FitUnwanted {
		t.Fatalf("unwanted fit = (%d resizes, stop %d, %v), want (0, FitUnwanted, nil)", resizes, stop, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 100 || h != 27 {
		t.Fatalf("window after the unwanted fit = %dx%d, want 100x27", w, h)
	}

	// From the size the window really has, the fit lands and unpins.
	resizes, stop, err = client.FitWindowToPaneUnlessResized(ctx, "s0", "s0", WindowView{100, 27, "manual", true}, 45, 15, func() bool { return true })
	if err != nil || resizes != 1 || stop != FitFinished {
		t.Fatalf("fit from the window's own size = (%d resizes, stop %d, %v), want (1, FitFinished, nil)", resizes, stop, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 45 || h != 15 {
		t.Fatalf("window after the fit = %dx%d, want 45x15", w, h)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || set {
		t.Fatalf("window-size after the fit = set=%v %q (%v), want unset", set, value, err)
	}
}

// TestFitWindowToPaneUnlessResizedKeepsAnExternalPinAfterItsOwnResize covers
// the same promise for a fit that is interrupted part-way: another process
// resizes the window after the fit's first resize-window and before the
// fit's next read. The fit must stop there and must not unpin, because the
// window-size option is now the other process's `manual`, not deck's. The
// tmux binary is a shim that performs that external resize right after the
// fit's own resize-window returns, so the interleaving is the same on every
// run.
func TestFitWindowToPaneUnlessResizedKeepsAnExternalPinAfterItsOwnResize(t *testing.T) {
	socket := geometrySocket("unlessresizedpartial")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	runTmux(t, socket, "set-option", "-g", "window-size", "latest")
	directory := t.TempDir()
	binary := filepath.Join(directory, "tmux")
	externalResizes := filepath.Join(directory, "external-resizes")
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
*resize-window*)
	tmux "$@" || exit $?
	tmux -L '%s' resize-window -t s0 -x 100 -y 27 || exit $?
	echo external >> '%s'
	exit 0 ;;
esac
exec tmux "$@"
`, socket, externalResizes)
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := Client{Socket: socket, Binary: binary, Timeout: 5 * time.Second}
	ctx := context.Background()

	resizes, stop, err := client.FitWindowToPaneUnlessResized(ctx, "s0", "s0", WindowView{80, 24, "latest", false}, 45, 15, func() bool { return true })
	if err != nil || resizes != 1 || stop != FitResizedElsewhere {
		t.Fatalf("fit interrupted after its first resize = (%d resizes, stop %d, %v), want (1, FitResizedElsewhere, nil)", resizes, stop, err)
	}
	if got, readErr := os.ReadFile(externalResizes); readErr != nil || strings.Count(string(got), "external") != 1 {
		t.Fatalf("the shim's external resize ran %q times (%v), want exactly once", got, readErr)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != 100 || h != 27 {
		t.Fatalf("window after the interrupted fit = %dx%d (%v), want the other process's 100x27", w, h, err)
	}
	if value, set, err := client.readBuiltinWindowOption(ctx, "s0"); err != nil || !set || value != "manual" {
		t.Fatalf("window-size after the interrupted fit = set=%v %q (%v), want the other process's manual pin left in place", set, value, err)
	}
}
