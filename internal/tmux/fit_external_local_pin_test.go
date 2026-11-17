package tmux

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// The local-pin rule (SPEC §11): whether the window's OWN options hold
// `window-size` is part of what the fit saw. Another process can change that
// after the fit's read while leaving the width, the height and even the
// effective value as they were -- under a global `window-size manual` a
// `resize-window` to the window's own size only turns the inherited `manual`
// into a window-local one -- and neither the fit's resize nor its closing
// unpin may overwrite that change. Each case below fails when the compare-and-
// set compares only the numbers and the effective value: the resize lands, or
// the unpin removes the other process's window-local option.

// newLocalPinSession starts s0 at 80x24 under a global window-size of global,
// with a window-local `window-size manual` when localManual, and a tmux shim
// that runs externalArgs as another process right after the fit's nth
// geometry read.
func newLocalPinSession(t *testing.T, name, global string, localManual bool, nth int, externalArgs string) (Client, string) {
	t.Helper()
	socket := geometrySocket(name)
	t.Cleanup(newBareGeometrySession(t, socket, "s0", 80, 24))
	runTmux(t, socket, "set-option", "-g", "window-size", global)
	runTmux(t, socket, "set-option", "-wu", "-t", "s0", "window-size")
	if localManual {
		runTmux(t, socket, "resize-window", "-t", "s0", "-x", "80", "-y", "24")
	}
	binary, ran := externalCommandAfterRead(t, socket, nth, externalArgs)
	return Client{Socket: socket, Binary: binary, Timeout: 5 * time.Second}, ran
}

// requireWindowAsOtherProcessLeftIt fails unless the other process's command
// ran exactly once and the window is width x height with its window-local
// `window-size` option set to wantLocal ("" = not set window-locally), and no
// probe marker is left behind.
func requireWindowAsOtherProcessLeftIt(t *testing.T, client Client, ranMarker string, width, height int, wantLocal string) {
	t.Helper()
	ctx := context.Background()
	if got, err := os.ReadFile(ranMarker); err != nil || strings.Count(string(got), "external") != 1 {
		t.Fatalf("test setup: the other process's command ran %q times (%v), want exactly once", got, err)
	}
	if w, h, err := client.WindowSize(ctx, "s0"); err != nil || w != width || h != height {
		t.Fatalf("window after the fit = %dx%d (%v), want %dx%d", w, h, err, width, height)
	}
	value, set, err := client.readBuiltinWindowOption(ctx, "s0")
	if err != nil || set != (wantLocal != "") || value != wantLocal {
		t.Fatalf("window-local window-size after the fit = set=%v %q (%v), want %q as the other process left it", set, value, err, wantLocal)
	}
	requireNoProbeMarker(t, client)
}

// requireNoProbeMarker fails if windowCAS's probe marker outlived its own
// tmux invocation.
func requireNoProbeMarker(t *testing.T, client Client) {
	t.Helper()
	options := runTmux(t, client.Socket, "show-options", "-w", "-t", "s0")
	if strings.Contains(options, windowSizeInheritedOption) {
		t.Fatalf("window options after the compare-and-set still hold the probe marker:\n%s", options)
	}
}

func requireStoodDown(t *testing.T, resizes int, stop FitStop, err error, wantResizes int) {
	t.Helper()
	if err != nil || resizes != wantResizes || stop != FitResizedElsewhere {
		t.Fatalf("fit = (%d resizes, stop %d, %v), want (%d, FitResizedElsewhere, nil)", resizes, stop, err, wantResizes)
	}
}

var inheritedManual80x24 = WindowView{Width: 80, Height: 24, Pin: "manual"}

// The finding's own interleaving: inherited manual, and another process's
// same-size resize-window makes it window-local after the fit's read.
func TestFitKeepsAnExternalResizeThatMakesAnInheritedManualPinLocalBeforeItsResize(t *testing.T) {
	client, ran := newLocalPinSession(t, "localpinresize", "manual", false, 1, "resize-window -t s0 -x 80 -y 24")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", inheritedManual80x24, 45, 15, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "manual")
}

func TestFitKeepsAnExternalResizeThatMakesAnInheritedManualPinLocalBeforeItsUnpin(t *testing.T) {
	client, ran := newLocalPinSession(t, "localpinunpin", "manual", false, 1, "resize-window -t s0 -x 80 -y 24")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", inheritedManual80x24, 80, 24, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "manual")
}

// The same transition made by a bare option write instead of a resize.
func TestFitKeepsAnExternalOptionWriteThatMakesAnInheritedManualPinLocalBeforeItsResize(t *testing.T) {
	client, ran := newLocalPinSession(t, "localpinsetresize", "manual", false, 1, "set-option -w -t s0 window-size manual")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", inheritedManual80x24, 45, 15, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "manual")
}

func TestFitKeepsAnExternalOptionWriteThatMakesAnInheritedManualPinLocalBeforeItsUnpin(t *testing.T) {
	client, ran := newLocalPinSession(t, "localpinsetunpin", "manual", false, 1, "set-option -w -t s0 window-size manual")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", inheritedManual80x24, 80, 24, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "manual")
}

// Not only manual: an explicit window-local `latest` under the global
// `latest` changes neither the size nor the effective value either.
func TestFitKeepsAnExternalExplicitLocalLatestBeforeItsResize(t *testing.T) {
	client, ran := newLocalPinSession(t, "locallatestresize", "latest", false, 1, "set-option -w -t s0 window-size latest")
	from := WindowView{Width: 80, Height: 24, Pin: "latest"}
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", from, 45, 15, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "latest")
}

func TestFitKeepsAnExternalExplicitLocalLatestBeforeItsUnpin(t *testing.T) {
	client, ran := newLocalPinSession(t, "locallatestunpin", "latest", false, 1, "set-option -w -t s0 window-size latest")
	from := WindowView{Width: 80, Height: 24, Pin: "latest"}
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", from, 80, 24, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "latest")
}

// The opposite transition: the window's own `manual` is removed by another
// process under a global `manual`, so the effective value and the size stay.
// The fit's stale resize must not write a window-local pin back.
func TestFitKeepsAnExternalUnpinThatLeavesTheInheritedManualBeforeItsResize(t *testing.T) {
	client, ran := newLocalPinSession(t, "localunpinresize", "manual", true, 1, "set-option -w -u -t s0 window-size")
	from := WindowView{Width: 80, Height: 24, Pin: "manual", PinSet: true}
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", from, 45, 15, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 0)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 80, 24, "")
}

// The same opposite transition landing after the fit's own resize: the
// closing unpin sees the window no longer holds the pin the fit wrote and
// reports the window as changed elsewhere.
func TestFitReportsAnExternalUnpinAfterItsOwnResizeAsResizedElsewhere(t *testing.T) {
	client, ran := newLocalPinSession(t, "localunpinlate", "manual", false, 2, "set-option -w -u -t s0 window-size")
	resizes, stop, err := client.FitWindowToPaneUnlessResized(context.Background(), "s0", "s0", inheritedManual80x24, 45, 15, func() bool { return true })
	requireStoodDown(t, resizes, stop, err, 1)
	requireWindowAsOtherProcessLeftIt(t, client, ran, 45, 15, "")
}

// ReadWindowView reports where the pin lives, in the same observation.
func TestReadWindowViewReportsWhetherThePinIsWindowLocal(t *testing.T) {
	socket := geometrySocket("viewpinset")
	t.Cleanup(newBareGeometrySession(t, socket, "s0", 80, 24))
	runTmux(t, socket, "set-option", "-g", "window-size", "manual")
	runTmux(t, socket, "set-option", "-wu", "-t", "s0", "window-size")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()
	if view, err := client.ReadWindowView(ctx, "s0"); err != nil || view != inheritedManual80x24 {
		t.Fatalf("view of an inherited pin = %+v (%v), want %+v", view, err, inheritedManual80x24)
	}
	runTmux(t, socket, "set-option", "-w", "-t", "s0", "window-size", "manual")
	want := WindowView{Width: 80, Height: 24, Pin: "manual", PinSet: true}
	if view, err := client.ReadWindowView(ctx, "s0"); err != nil || view != want {
		t.Fatalf("view of a window-local pin = %+v (%v), want %+v", view, err, want)
	}
}

// The compare-and-set on its own: a view that has the pin's location wrong is
// stale in both directions, and neither the refused nor the applied CAS
// leaves the probe's marker or a window-local option it did not mean to.
func TestWindowCASComparesWhereThePinLives(t *testing.T) {
	socket := geometrySocket("casparentpin")
	t.Cleanup(newBareGeometrySession(t, socket, "s0", 80, 24))
	runTmux(t, socket, "set-option", "-g", "window-size", "manual")
	runTmux(t, socket, "set-option", "-wu", "-t", "s0", "window-size")
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()
	local := WindowView{Width: 80, Height: 24, Pin: "manual", PinSet: true}

	if applied, err := client.resizeWindowIfAt(ctx, "s0", local, fitWindowSize{50, 20}); err != nil || applied {
		t.Fatalf("resize from a local-pin view of an inherited pin = (%v, %v), want (false, nil)", applied, err)
	}
	if _, set, _ := client.readBuiltinWindowOption(ctx, "s0"); set {
		t.Fatal("the refused CAS left a window-local window-size behind")
	}
	requireNoProbeMarker(t, client)

	runTmux(t, socket, "set-option", "-w", "-t", "s0", "window-size", "manual")
	if applied, err := client.UnpinWindowSizeIfAt(ctx, "s0", inheritedManual80x24); err != nil || applied {
		t.Fatalf("unpin from an inherited view of a local pin = (%v, %v), want (false, nil)", applied, err)
	}
	if value, set, _ := client.readBuiltinWindowOption(ctx, "s0"); !set || value != "manual" {
		t.Fatalf("window-local window-size after the refused unpin = set=%v %q, want manual", set, value)
	}
	requireNoProbeMarker(t, client)

	if applied, err := client.UnpinWindowSizeIfAt(ctx, "s0", local); err != nil || !applied {
		t.Fatalf("unpin from the true local-pin view = (%v, %v), want (true, nil)", applied, err)
	}
	if _, set, _ := client.readBuiltinWindowOption(ctx, "s0"); set {
		t.Fatal("window-size still set window-locally after the matching unpin")
	}
	requireNoProbeMarker(t, client)
	if applied, err := client.resizeWindowIfAt(ctx, "s0", inheritedManual80x24, fitWindowSize{50, 20}); err != nil || !applied {
		t.Fatalf("resize from the true inherited view = (%v, %v), want (true, nil)", applied, err)
	}
	if w, h, _ := client.WindowSize(ctx, "s0"); w != 50 || h != 20 {
		t.Fatalf("window after the applied resize = %dx%d, want 50x20", w, h)
	}
	requireNoProbeMarker(t, client)
}

// windowCASOutcome accepts a non-zero exit only together with the probe's own
// "already set" answer and a branch marker; anything else is an error.
func TestWindowCASOutcomeAcceptsOnlyTheProbesOwnRefusal(t *testing.T) {
	exit1 := errors.New("exit status 1")
	cases := []struct {
		name           string
		output         string
		err            error
		applied, known bool
	}{
		{"inherited, applied", windowCASApplied + "\n", nil, true, true},
		{"inherited, stale", windowCASStale + "\n", nil, false, true},
		{"window-local, applied", windowSizeAlreadySet + "\n" + windowCASApplied + "\n", exit1, true, true},
		{"window-local, stale", windowSizeAlreadySet + "\n" + windowCASStale + "\n", exit1, false, true},
		{"action rejected after the probe", windowSizeAlreadySet + "\nno such window: s0\n", exit1, false, false},
		{"failure without the probe's refusal", windowCASApplied + "\n", exit1, false, false},
		{"no marker", "", nil, false, false},
	}
	for _, tc := range cases {
		applied, known := windowCASOutcome(tc.output, tc.err)
		if applied != tc.applied || known != tc.known {
			t.Errorf("%s: windowCASOutcome = (%v, %v), want (%v, %v)", tc.name, applied, known, tc.applied, tc.known)
		}
	}
}
