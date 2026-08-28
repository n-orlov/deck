package features

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCoalescedTwoKeystrokesWithNoDelayStillDispatchBoth is task 118's own
// required black-box proof: it writes two real keystrokes into the PTY back
// to back with NO sleep between the two driver.Send calls, deliberately
// skipping the pacing sendClientKeys otherwise adds after every send (that
// 25ms pad is precisely what normally prevents the two bytes from landing in
// the same read -- see sendClientKeys's own comment in assertions_test.go).
// The harness itself is not made slower or more forgiving anywhere else;
// this test alone accepts the raw, unpaced timing to exercise the exact
// coalescing bubbletea's PTY reader can produce.
//
// The pair used is the "dd" delete chord: pressing it once shows only the
// pending-delete indicator ("press d again to confirm"); pressing it twice
// opens the confirm dialog itself, whose body contains wording ("kills the
// live pane", "survives, untouched") the indicator never does. Before task
// 118's fix, a coalesced KeyMsg{Runes: "dd"} matched no case in Update's
// switch and the whole event was dropped silently -- neither indicator nor
// dialog would appear, and a slower, separately-delivered "d","d" (the
// normal sendClientKeys path) would keep working, masking the defect in
// every scenario that already paces its keystrokes. Sending the two bytes
// with no gap at all is what actually exercises the coalescing path.
func TestCoalescedTwoKeystrokesWithNoDelayStillDispatchBoth(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	cwd := filepath.Join(home, "coalesced-keymsg-cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("create working directory: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_coalesced_keymsg_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(3 * time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}

	// Create one real shell session -- dd's first half (the pending
	// indicator) is a no-op with zero sessions, so the chord needs a real
	// row to act on.
	if err := driver.Send("n"); err != nil {
		t.Fatal(err)
	}
	// Title-independent (F19) equivalent-row wait: waits on the Agent row,
	// which createFieldRows renders unconditionally, rather than on the
	// "Create shell session" title, which only appears while shell is the
	// pre-selected agent -- true here regardless, since this is the very
	// first modal this fresh-DECK_HOME test opens, but the wait target no
	// longer depends on that being so.
	if err := driver.WaitForFrame(ctx, false, "Agent: "); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("coalesced-dd"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(75 * time.Millisecond)
	if err := driver.Send("\x1b[B" + cwd + "\r"); err != nil {
		t.Fatal(err)
	}
	// "starting" (not the session's own name) is the unambiguous wait target:
	// the create modal's own Name field still shows "coalesced-dd" the whole
	// time it is being typed, well before Enter is even sent, so waiting on
	// the name text itself would race ahead of the still-open modal actually
	// processing Enter -- exactly what the very first version of this test
	// did, letting the very next Send land inside the still-focused cwd field
	// instead of the main view (a test-harness bug, not a product one; see
	// clientCreatesShellSession in assertions_test.go, which already waits on
	// "starting" for the same reason).
	if err := driver.WaitForFrame(ctx, false, "starting"); err != nil {
		t.Fatal(err)
	}

	// Sanity check (proves a single "d" alone only shows the pending
	// indicator, never the confirm dialog) before the real, unpaced pair.
	if err := driver.Send("d"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "press d again to confirm"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("\x1b"); err != nil { // clear the pending indicator
		t.Fatal(err)
	}
	if err := driver.WaitForFrameGone(ctx, false, "press d again to confirm"); err != nil {
		t.Fatal(err)
	}

	// The real proof: two keystrokes, no sleep, no sendClientKeys pacing.
	if err := driver.Send("d"); err != nil {
		t.Fatalf("send first d: %v", err)
	}
	if err := driver.Send("d"); err != nil {
		t.Fatalf("send second d with no delay: %v", err)
	}

	if err := driver.WaitForFrame(ctx, false, "kills the live pane"); err != nil {
		t.Fatalf("coalesced \"dd\" (no delay) did not open the delete confirm dialog: %v\nraw: %s", err, driver.sentLog())
	}
	if err := driver.WaitForFrame(ctx, false, "survives, untouched"); err != nil {
		t.Fatalf("coalesced \"dd\" (no delay) confirm dialog is missing its body text: %v", err)
	}

	if err := driver.Send("\x1b"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "coalesced-dd"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("q"); err != nil {
		t.Fatal(err)
	}
}
