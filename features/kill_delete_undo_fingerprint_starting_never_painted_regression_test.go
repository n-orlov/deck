package features

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestFingerprintShellCreateNeverRequiresStarting is task 025's own
// regression guard for the defect inventory.md §8 attributes to
// clientCreatesShellSessionWithScratchCWDLabelled
// (features/kill_delete_undo_fingerprint_test.go, requirement 29): that
// helper is a sibling of the shared clientCreatesShellSession step task
// 006 (930d2517a6) converged onto waitForSettledSessionRow, but it was
// never converged itself and, at a192accf7d, still ends its own create
// flow with the literal `client.WaitForFrame(ctx, false, "starting")`. SPEC
// §7's shell-only fast-forward rule can promote a shell row from
// "starting" straight to "running" before anything ever samples the
// intermediate frame, so insisting on the literal word "starting" races
// that promotion and burns the full default deadline even though the row
// settled perfectly correctly -- observed at kill_delete_undo.feature:176
// and :288, dispatch run 36908150282 ("after scenario hook failed: timed
// out waiting for frame \"starting\": context deadline exceeded").
//
// This test drives clientCreatesShellSessionWithScratchCWDLabelled
// ITSELF -- not a stand-in -- over a synthetic ScreenDriver fed a SCRIPTED
// frame sequence that never paints "starting" at all: the first frame is
// the create modal already positioned on the shell agent (so
// ensureCreateModalAgent's own marker matches on its very first read and
// never has to cycle anything or wait further), and the second jumps
// straight to the sidebar row showing "running" -- exactly the shape SPEC
// §7's fast-forward licenses and a literal WaitForFrame(ctx, false,
// "starting") can never observe. The harness fields this test pokes
// directly (ScenarioHarness.namedClients, .namedDirectories,
// scenarioHarnessKey) and every helper it calls through
// (assertionHarness, h.Client, namedDirectory, ensureCreateModalAgent,
// ScreenDriver.Send/WaitForFrame, defaultWaitDeadline) already exist at
// a192accf7d with this exact shape, so this file, copied unmodified into
// a git-archive export of that commit, compiles there unchanged and fails
// on the real timeout deadline rather than a compile error -- see
// /run/ralphd/artifacts/025/unfixed-fail.log. It passes at HEAD, where
// commit e0fd0e6bd6 (task 018) converged the helper onto
// waitForSettledSessionRow, exactly the way
// TestWaitForSettledSessionRowNeverRequiresStarting (task 006) already
// proves that function itself never needs "starting" painted.
func TestFingerprintShellCreateNeverRequiresStarting(t *testing.T) {
	// Shrink the deadline clientCreatesShellSessionWithScratchCWDLabelled's
	// final wait inherits (withDefaultWaitDeadline, features/pty_driver_test.go)
	// so the unfixed tree's real timeout failure above lands in seconds, the
	// same technique TestWaitForFrameAppliesADefaultDeadlineWhenTheCallersContextHasNone
	// already uses -- defaultWaitDeadline is a var for exactly this purpose and
	// exists unchanged at a192accf7d.
	original := defaultWaitDeadline
	defaultWaitDeadline = 2 * time.Second
	defer func() { defaultWaitDeadline = original }()

	const (
		cols        = int(terminalColumns)
		rows        = int(terminalRows)
		clientName  = "A"
		sessionName = "fp-fast-forwarded"
		label       = "fp-scratch"
		dir         = "/scratch/fp-scratch"
	)

	d := &ScreenDriver{
		screen:  vt.NewEmulator(cols, rows),
		done:    make(chan struct{}),
		updated: make(chan struct{}, 1),
	}
	go d.drainScreenInput()
	t.Cleanup(func() { closeInputPipe(d.screen) })

	// clientCreatesShellSessionWithScratchCWDLabelled calls client.Send
	// three times before its final wait; Send writes to d.terminal (a real
	// *os.File backed by the deck process's own pty master in production,
	// nil in a bare synthetic driver). Wire it to the write end of a pipe
	// whose read end this test never drains -- the bytes themselves do not
	// matter here, only that the write neither panics a nil *os.File nor
	// blocks.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	d.terminal = w

	h := &ScenarioHarness{
		namedClients:     map[string]*ScreenDriver{clientName: d},
		namedDirectories: map[string]string{label: dir},
	}
	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)

	// frameBeforeSettle is the create modal, already reading "Agent: shell
	// (left/right cycles" -- ensureCreateModalAgent's marker matches on
	// this very first frame, so it returns immediately without cycling the
	// field or issuing any further wait. No session row, no "starting", no
	// "running" anywhere on screen.
	frameBeforeSettle := strings.Join([]string{
		"│ Create shell session                               │ $                                                     │",
		"│ > Name:                                             │                                                       │",
		"│   Agent: shell (left/right cycles: shell/claude/pi) │                                                       │",
	}, "\r\n")
	// frameAfterSettle is the main view immediately after the modal closed
	// -- the row's VERY FIRST paint already shows "running" (leading "~"
	// glyph, frameSidebarRowGlyph/startingOrRunningRowGlyphs): "starting"
	// is never painted at any point in this scripted sequence, not even
	// transiently.
	frameAfterSettle := strings.Join([]string{
		"│ ▾ default  (1)                                      │ $                                                     │",
		fmt.Sprintf("│ > ~ %s running                       │                                                       │", sessionName),
		"│   just now                                          │                                                       │",
	}, "\r\n")

	var written strings.Builder
	write := func(frame string) {
		raw := "\x1b[H\x1b[2J" + frame
		written.WriteString(raw)
		d.mu.Lock()
		_, _ = d.screen.Write([]byte(raw))
		d.mu.Unlock()
		select {
		case d.updated <- struct{}{}:
		default:
		}
	}

	write(frameBeforeSettle)

	type result struct{ err error }
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		done <- result{err: clientCreatesShellSessionWithScratchCWDLabelled(ctx, clientName, sessionName, label)}
	}()

	// Let the helper send its keys, settle through ensureCreateModalAgent
	// and reach its final wait against the still-open modal frame before
	// the scripted sequence jumps to the settled one, so a passing result
	// below cannot be explained by it having returned instantly, before the
	// second write even happened.
	time.Sleep(150 * time.Millisecond)
	select {
	case r := <-done:
		t.Fatalf("clientCreatesShellSessionWithScratchCWDLabelled returned (err=%v) before the settled frame was ever painted -- it must actually wait", r.err)
	default:
	}

	write(frameAfterSettle)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("clientCreatesShellSessionWithScratchCWDLabelled returned %v, want nil: SPEC §7's shell fast-forward may promote starting->running without ever painting \"starting\", and this scripted sequence never does (kill_delete_undo.feature:176/:288, dispatch run 36908150282)", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clientCreatesShellSessionWithScratchCWDLabelled never returned after the settled frame was painted")
	}
	if elapsed := time.Since(start); elapsed < 140*time.Millisecond {
		t.Fatalf("clientCreatesShellSessionWithScratchCWDLabelled returned after %s, suspiciously fast -- want it to have actually blocked until the second write landed", elapsed)
	}

	// The whole point: this regression's own scripted stream never once
	// contained the literal word "starting", yet the create flow still
	// settled.
	if strings.Contains(written.String(), "starting") {
		t.Fatal(`regression test's own scripted sequence painted "starting" somewhere -- it must never, to prove the settle wait does not depend on it`)
	}
}
