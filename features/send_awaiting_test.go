package features

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startScriptDriver runs a tiny /bin/sh script under the ScreenDriver's real
// PTY and registers its teardown, so the Send* helpers are proven against a
// program whose output the test fully controls.
func startScriptDriver(t *testing.T, body string) (*ScreenDriver, context.Context) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "program.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	driver, err := StartScreenDriver(ctx, script, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The scripts end in a blocking read; kill the child so Stop reaps
		// it at once instead of waiting out its own SIGQUIT grace.
		_ = driver.cmd.Process.Kill()
		_ = driver.Stop(time.Second)
	})
	return driver, ctx
}

// SendAwaitingVisible returns once the typed text is on screen, and keeps
// the legacy pause when the text was already there or never shows.
func TestSendAwaitingVisibleWaitsForTheEchoNotAFixedPause(t *testing.T) {
	t.Run("returns with the typed text on screen", func(t *testing.T) {
		driver, ctx := startScriptDriver(t, "printf ready; cat\n")
		if err := driver.WaitForFrame(ctx, false, "ready"); err != nil {
			t.Fatal(err)
		}
		if err := driver.SendAwaitingVisible(ctx, "typed-name", "typed-name"); err != nil {
			t.Fatal(err)
		}
		if frame := driver.Frame(false); !strings.Contains(frame, "typed-name") {
			t.Fatalf("returned before the typed text was on screen:\n%s", frame)
		}
	})
	t.Run("text that never shows is paced for the fallback and does not fail", func(t *testing.T) {
		// The fallback replaces a fixed 75ms pause and must never exceed it:
		// a raised per-step bound is exactly what the runtime cure forbids.
		const replacedPause = 75 * time.Millisecond
		if typedEchoFallback != replacedPause || typedEchoLegacyPace != replacedPause {
			t.Fatalf("fallback %s / legacy pace %s, want both exactly the replaced %s pause", typedEchoFallback, typedEchoLegacyPace, replacedPause)
		}
		driver, ctx := startScriptDriver(t, "printf ready; cat >/dev/null\n")
		if err := driver.WaitForFrame(ctx, false, "ready"); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := driver.SendAwaitingVisible(ctx, "x", "never-echoed"); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed < typedEchoFallback {
			t.Fatalf("returned after %s, want at least the %s fallback (the text never showed)", elapsed, typedEchoFallback)
		}
	})
	t.Run("text already on screen keeps the legacy pause", func(t *testing.T) {
		driver, ctx := startScriptDriver(t, "printf already; cat >/dev/null\n")
		if err := driver.WaitForFrame(ctx, false, "already"); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := driver.SendAwaitingVisible(ctx, "y", "already"); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed < typedEchoLegacyPace {
			t.Fatalf("returned after %s, want at least the %s legacy pause (the echo proves nothing)", elapsed, typedEchoLegacyPace)
		}
	})
}

// SendAwaitingChange returns on the first changed frame, and runs out its
// bound when the key changes nothing.
func TestSendAwaitingChangeReturnsOnTheFirstChangedFrame(t *testing.T) {
	driver, ctx := startScriptDriver(t, "printf ready; cat\n")
	if err := driver.WaitForFrame(ctx, false, "ready"); err != nil {
		t.Fatal(err)
	}
	if err := driver.SendAwaitingChange(ctx, "z", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if frame := driver.Frame(false); !strings.Contains(frame, "readyz") {
		t.Fatalf("returned before the key changed the frame:\n%s", frame)
	}
	bound := 120 * time.Millisecond
	start := time.Now()
	if err := driver.SendAwaitingChange(ctx, "", bound); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < bound {
		t.Fatalf("a write that changes nothing returned after %s, want the full %s bound", elapsed, bound)
	}
}

// SendAwaitingInteractive follows the interactive marker in both
// directions and runs out its bound when the frame never changes mode.
func TestSendAwaitingInteractiveFollowsTheModeMarker(t *testing.T) {
	t.Run("entering waits for the marker", func(t *testing.T) {
		driver, ctx := startScriptDriver(t, "printf list; read x; printf ' Ctrl+Q to leave'; cat >/dev/null\n")
		if err := driver.WaitForFrame(ctx, false, "list"); err != nil {
			t.Fatal(err)
		}
		if err := driver.SendAwaitingInteractive(ctx, "\r", true); err != nil {
			t.Fatal(err)
		}
		if frame := driver.Frame(false); !strings.Contains(frame, interactiveFrameMarker) {
			t.Fatalf("returned before the interactive marker showed:\n%s", frame)
		}
	})
	t.Run("leaving waits for the marker to go", func(t *testing.T) {
		driver, ctx := startScriptDriver(t, "printf 'Ctrl+Q to leave'; read x; printf '\\033[2J\\033[Hback in the list'; cat >/dev/null\n")
		if err := driver.WaitForFrame(ctx, false, interactiveFrameMarker); err != nil {
			t.Fatal(err)
		}
		if err := driver.SendAwaitingInteractive(ctx, "\r", false); err != nil {
			t.Fatal(err)
		}
		if frame := driver.Frame(false); strings.Contains(frame, interactiveFrameMarker) {
			t.Fatalf("returned while the interactive marker was still on screen:\n%s", frame)
		}
	})
	t.Run("a mode that never changes is paced for the full bound", func(t *testing.T) {
		driver, ctx := startScriptDriver(t, "printf list; cat >/dev/null\n")
		if err := driver.WaitForFrame(ctx, false, "list"); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := driver.SendAwaitingInteractive(ctx, "\r", true); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed < interactiveSettleBound {
			t.Fatalf("returned after %s, want the full %s bound (the frame never showed interactive mode)", elapsed, interactiveSettleBound)
		}
	})
}

// "g" is withheld only when the selection already sits on the line right
// under the top border; every other shape still gets it.
func TestSelectionOnSidebarTopLineOnlyForTheFirstContentLine(t *testing.T) {
	top := "+----------+\n| > first  |\n|   second |\n+----------+"
	lower := "+----------+\n|   first  |\n| > second|\n+----------+"
	header := "+----------+\n| group    |\n|   first  |\n+----------+"
	for name, tc := range map[string]struct {
		frame string
		want  bool
	}{"selection on the first content line": {top, true}, "selection further down": {lower, false}, "no marker (header selected)": {header, false}, "empty frame": {"", false}} {
		t.Run(name, func(t *testing.T) {
			if got := selectionOnSidebarTopLine(tc.frame); got != tc.want {
				t.Fatalf("selectionOnSidebarTopLine = %v, want %v for\n%s", got, tc.want, tc.frame)
			}
		})
	}
}
