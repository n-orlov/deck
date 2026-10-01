package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestWaitForSettledSessionRowSettlesOnAnEllipsisTruncatedBadge is task
// 008's own regression guard (no_leak_scan.feature:17, inventory mechanism
// M1's long-name instance). Task 006 fixed waitForSettledSessionRow's
// first M1 defect by accepting either status WORD ("starting" or
// "running") via frameSidebarRowContains, which parses the sidebar row's
// own trailing badge-run text. That still races a session name long
// enough to fill the sidebar's own content width on its own: SPEC §11.3
// lays the row out as glyph, name, then the badge run, in that fixed
// order, and internal/tui/tui.go's padTrunc truncates the row's TRAILING
// content when it overflows -- so a 24-character name (no_leak_scan's own
// control value "leak-scan-control-9c2f1a") leaves no columns at all for
// "starting" (8) or "running" (7) once the glyph, the name and their
// separating spaces are laid down, and the badge run renders as a bare
// "..." with the status word gone entirely. frameSidebarRowContains then
// can never see either word for that row, no matter how long it waits:
// unlike task 006's own defect this is not a transient overwrite, it
// reproduces on every single real run (confirmed 3/3,
// /run/ralphd/artifacts/008/no_leak_scan-head-before-fix.log, ~46s each
// against the live binary before this fix).
//
// This test reproduces the shape directly against a synthetic
// ScreenDriver, no real deck binary involved: the scripted settled frame
// paints the row with its real status GLYPH ("~", running) but an
// ellipsis badge run exactly like the live failure ("... " in place of
// "running"), so frameSidebarRowContains(frame, name, "starting") and
// frameSidebarRowContains(frame, name, "running") are BOTH false against
// it -- proving the word is genuinely unreadable here, not merely slow to
// appear -- while waitForSettledSessionRow (fed the identical scripted
// stream task 006's own regression test uses) still settles, because it
// now reads the row's own leading glyph instead.
func TestWaitForSettledSessionRowSettlesOnAnEllipsisTruncatedBadge(t *testing.T) {
	const (
		cols = int(terminalColumns)
		rows = int(terminalRows)
		name = "leak-scan-control-9c2f1a"
	)
	d := &ScreenDriver{
		screen:  vt.NewEmulator(cols, rows),
		done:    make(chan struct{}),
		updated: make(chan struct{}, 1),
	}
	go d.drainScreenInput()
	t.Cleanup(func() { closeInputPipe(d.screen) })

	frameBeforeSettle := strings.Join([]string{
		"│ Create shell session              │ $                                                     │",
		"│ Name: " + name + "         │                                                       │",
		"│ Agent: shell (left/right cycles)  │                                                       │",
	}, "\r\n")
	// frameAfterSettle's sidebar cell is exactly the shape the live
	// defect's own raw capture showed: the real "~" running glyph, the
	// full (untruncated) name, then a bare "..." where the status word
	// used to be -- the row's own badge run, ellipsised away.
	frameAfterSettle := strings.Join([]string{
		"│ ▾ default  (1)                    │ $                                                     │",
		fmt.Sprintf("│ > ~ %s ... │                                                       │", name),
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

	time.Sleep(75 * time.Millisecond)
	select {
	case r := <-done:
		t.Fatalf("waitForSettledSessionRow returned (err=%v) before the settled frame was ever painted -- it must actually wait", r.err)
	default:
	}

	write(frameAfterSettle)

	// The fixture's own control: the settled frame's badge run must be
	// genuinely unreadable by the OLD word-based matcher, or this test
	// would not be exercising the defect it names.
	settled := d.Frame(false)
	if frameSidebarRowContains(settled, name, "starting") || frameSidebarRowContains(settled, name, "running") {
		t.Fatal("scripted settled frame's status word is readable by frameSidebarRowContains -- fixture does not reproduce the ellipsis-truncated badge this test means to prove")
	}
	if glyph, ok := frameSidebarRowGlyph(settled, name); !ok || !startingOrRunningRowGlyphs[glyph] {
		t.Fatalf("frameSidebarRowGlyph(settled, %q) = (%q, %v), want the running glyph -- fixture is wrong", name, glyph, ok)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("waitForSettledSessionRow returned %v, want nil: the row's own leading status glyph settled even though its trailing badge word was ellipsis-truncated away", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForSettledSessionRow never returned after the settled (ellipsis-truncated) frame was painted -- it is still relying on the unreadable status word")
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("waitForSettledSessionRow returned after %s, suspiciously fast -- want it to have actually blocked until the second write landed", elapsed)
	}
}
