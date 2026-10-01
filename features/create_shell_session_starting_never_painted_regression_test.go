package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestWaitForSettledSessionRowNeverRequiresStarting is task 006's own
// regression guard (inventory mechanism M1) for the defect
// clientCreatesShellSession.WaitForFrame(ctx, false, "starting") used to
// have: SPEC §7's shell-only fast-forward rule can promote a shell row from
// "starting" straight to "running" before anything ever samples the
// intermediate frame, so insisting on the literal word "starting" raced
// that promotion and timed out the full default deadline even though the
// row settled perfectly correctly.
//
// This test feeds waitForSettledSessionRow a SCRIPTED frame sequence over a
// synthetic ScreenDriver -- no real deck binary, no real tmux -- that never
// paints "starting" at all: the first frame is the create modal's own
// screen (names neither the session nor either status word), and the
// second jumps straight to the row showing "running". That is exactly the
// shape SPEC §7's fast-forward licenses and the old code could never
// observe. waitForSettledSessionRow (features/assertions_test.go) does not
// exist before task 006 -- this file, copied unmodified into an export of
// a192accf7d, fails to even compile there, which is this regression's own
// proof that the unfixed tree cannot pass it (see
// /run/ralphd/artifacts/006/unfixed-build.log).
//
// This same M1 mechanism is also the inventoried root cause of
// preview.feature:304 ("a stopped session's preview names its own state
// instead of showing stale bytes", task 010): its own first step is
// `deck client "A" creates shell session "retiring"`, i.e. the very same
// clientCreatesShellSession this test guards, calling the very same
// waitForSettledSessionRow. 006's fix already covers it -- task 010 does
// not need a second product change, only its own re-verification that
// 006's fix actually clears preview.feature:304's failure (36792029158),
// which this file, copied unmodified into the same a192accf7d export,
// cannot even compile to prove -- see
// /run/ralphd/artifacts/010/unfixed-build.log -- and
// /run/ralphd/artifacts/010/preview-20x-{normal,race}.log for
// preview.feature:304 itself, 20/20 both ways at HEAD.
func TestWaitForSettledSessionRowNeverRequiresStarting(t *testing.T) {
	const (
		cols = int(terminalColumns)
		rows = int(terminalRows)
		name = "fast-forwarded"
	)
	d := &ScreenDriver{
		screen:  vt.NewEmulator(cols, rows),
		done:    make(chan struct{}),
		updated: make(chan struct{}, 1),
	}
	go d.drainScreenInput()

	// frameBeforeSettle is the create modal still open: no session row, no
	// "starting", no "running" anywhere on screen.
	frameBeforeSettle := strings.Join([]string{
		"│ Create shell session              │ $                                                     │",
		"│ Name: " + name + "                     │                                                       │",
		"│ Agent: shell (left/right cycles)  │                                                       │",
	}, "\r\n")
	// frameAfterSettle is the main view immediately after the modal closed
	// -- the row's VERY FIRST paint already shows "running", exactly the
	// race this task fixes: "starting" is never painted at any point in
	// this scripted sequence, not even transiently. The leading "~" is the
	// row's own real status glyph (task 008 moved waitForSettledSessionRow's
	// predicate onto it; sidebarRowLeadGlyphs/frameSidebarRowGlyph,
	// features/status_probe_test.go) -- carried here too so this fixture
	// stays a faithful rendering of a real settled row, not just enough to
	// satisfy whichever matcher happens to read it.
	frameAfterSettle := strings.Join([]string{
		"│ ▾ default  (1)                    │ $                                                     │",
		fmt.Sprintf("│ > ~ %s running              │                                                       │", name),
		"│   just now                        │                                                       │",
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

	type result struct {
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		done <- result{err: waitForSettledSessionRow(context.Background(), d, name)}
	}()

	// Let waitForSettledSessionRow observe the still-open modal frame
	// before the scripted sequence jumps to the settled one, so a passing
	// result below cannot be explained by it having returned instantly,
	// before the second write even happened.
	time.Sleep(75 * time.Millisecond)
	select {
	case r := <-done:
		t.Fatalf("waitForSettledSessionRow returned (err=%v) before the settled frame was ever painted -- it must actually wait", r.err)
	default:
	}
	if frameSidebarRowContains(d.Frame(false), name, "starting") || frameSidebarRowContains(d.Frame(false), name, "running") {
		t.Fatal("scripted pre-settle frame already shows a status for the row -- fixture is wrong, not what this test means to prove")
	}

	write(frameAfterSettle)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("waitForSettledSessionRow returned %v, want nil: SPEC §7's shell fast-forward may promote starting->running without ever painting \"starting\", and this scripted sequence never does", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForSettledSessionRow never returned after the settled frame was painted")
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("waitForSettledSessionRow returned after %s, suspiciously fast -- want it to have actually blocked until the second write landed", elapsed)
	}

	// The whole point: this regression's own scripted stream never once
	// contained the literal word "starting", yet the wait still settled.
	if strings.Contains(written.String(), "starting") {
		t.Fatal(`regression test's own scripted sequence painted "starting" somewhere -- it must never, to prove the settle predicate does not depend on it`)
	}
}
