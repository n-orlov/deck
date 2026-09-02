package tmux

import (
	"context"
	"testing"
	"time"
)

// TestRestoreWindowGeometryUnsetsAPreEntrySetWindowSize pins
// RestoreWindowGeometry's actual shape against the one case its own doc
// comment says it does not attempt: a window whose `window-size` was
// already set WINDOW-LOCALLY before CaptureWindowGeometry ever ran.
//
// SPEC.md §11.9 / PRD phase3b II-9 name a PLAIN UNSET as the exit recipe --
// "resizes back only when no client is attached, then unsets the
// window-local window-size" -- with no value-preserving restore step
// anywhere in it, and geometry.go's unsetWindowSize (called unconditionally,
// last, by RestoreWindowGeometry) is exactly that plain unset: it never
// reads geometry.WindowSizeValue at all, so there is nothing in the
// committed code that could write a captured value back. That is the
// authority this test pins.
//
// It diverges from a LITERAL reading of PRD phase3i-force-attach.md's R100,
// which asks two sequential steals to "restore the pre-first-entry size
// byte-exactly (width, height, and the window-size value including its
// set shape)" -- read as a general byte-exact-restore promise, that
// clause would require this test's pre-entry SET value to come back after
// RestoreWindowGeometry, not merely to end up unset. It does not: this
// test sets window-size window-locally BEFORE capturing (so
// WindowSizeSet==true on the captured geometry), runs RestoreWindowGeometry
// with zero attached clients, and asserts the window-local window-size
// option is UNSET afterwards -- the captured set value is deliberately not
// put back. Task 208 records this divergence (SPEC wins over a literal
// reading of R100's "set shape" wording) as its own numbered finding; this
// test is the evidence task 208 cites.
func TestRestoreWindowGeometryUnsetsAPreEntrySetWindowSize(t *testing.T) {
	socket := restoreSocket("plain-unset")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Set window-size WINDOW-LOCALLY before anything captures geometry --
	// deck's own Bootstrap never does this (only the server-global scope,
	// per task 032/034's precedent), but this test's whole point is the
	// case that precedent does not cover.
	runTmux(t, socket, "set-window-option", "-t", "s0", "window-size", "manual")

	original, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("capture original geometry: %v", err)
	}
	if !original.WindowSizeSet || original.WindowSizeValue != "manual" {
		t.Fatalf("captured geometry = set=%v value=%q, want set=true value=\"manual\" (test assumption violated: the pre-entry window-local set did not take)", original.WindowSizeSet, original.WindowSizeValue)
	}

	if attached, err := client.SessionAttachedCount(ctx, "s0"); err != nil {
		t.Fatalf("session_attached: %v", err)
	} else if attached != 0 {
		t.Fatalf("session_attached = %d before any client attaches, want 0", attached)
	}

	if err := client.RestoreWindowGeometry(ctx, "s0", original); err != nil {
		t.Fatalf("restore window geometry: %v", err)
	}

	if value, set, err := client.readBuiltinWindowOption(ctx, "s0", "window-size"); err != nil {
		t.Fatalf("read window-size after restore: %v", err)
	} else if set {
		t.Fatalf("window-size after restore = set=true value=%q, want UNSET -- RestoreWindowGeometry's plain unsetWindowSize call never reads WindowSizeValue, so a pre-entry SET value (here %q) is deliberately not restored, per SPEC.md §11.9 / PRD phase3b II-9's plain-unset recipe, not R100's literal \"including its set shape\" wording", value, original.WindowSizeValue)
	}
}
