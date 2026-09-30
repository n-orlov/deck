package features

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// The two frames below are trimmed, byte-accurate excerpts of the sidebar
// box CI run 36774969847 actually rendered around agent_session.feature:33
// (review-findings.md's B4; artifacts under
// review2/api/run-36774969847-raw/features-rerun-1.log), captured moments
// apart while client A's own "presses R on session ... restart claude"
// step was in flight:
//
//   - rowSelectedFrame is the confirmed-good state right after the session
//     was created: the row itself carries the "> " marker and the preview
//     pane shows the row's own status ("Session is starting; no pane
//     yet.").
//   - headerSelectedFrame is the LAST frame the run captured before "fake-
//     claude resume:" timed out 20.25s later: the group header now carries
//     the reverse-video selection (raw bytes \x1b[7m...\x1b[27m, exactly as
//     CI wrote them) and the preview pane has fallen back to the generic
//     "Select or create a session to preview it here." -- both proving
//     selection moved off the row onto its header, not merely that the "> "
//     glyph failed to redraw. The row itself is still fully rendered and
//     still counted in the header's own "(1)", so this is not a case of the
//     row scrolling off-screen or the group folding.
const rowSelectedFrame = "+ deck - sessions -----------------+---------------------------------------------------------------+\r\n" +
	"| socket: deck_test_150123_2       | Session is starting; no pane yet.                             |\r\n" +
	"| v default  (1)                   |                                                               |\r\n" +
	"| > . restart claude starting      |                                                               |\r\n" +
	"|   just now                       |                                                               |\r\n"

const headerSelectedFrame = "+ deck - sessions -----------------+---------------------------------------------------------------+\r\n" +
	"| socket: deck_test_150123_2       | Select or create a session to preview it here.                |\r\n" +
	"| \x1b[7mv default  (1)\x1b[27m                   |                                                               |\r\n" +
	"|   . restart claude starting      |                                                               |\r\n" +
	"|   just now                       |                                                               |\r\n"

// TestFrameHasSelectedRowNamedOnCapturedHeaderInterleavingFrame (B4,
// cure-01-01-2) pins the exact captured evidence behind B4's rejection: on
// rowSelectedFrame, frameHasSelectedRowNamed must recognise "restart
// claude" as selected -- the healthy state navigateToRowByName's own
// pre-send check relies on, and the one selectSessionByNameThenSend's
// caller (clientPressesRestartOnNamedSession) had already confirmed true
// on CI run 36774969847 right before sending "R". On headerSelectedFrame
// -- CI's own record of what the frame had become by the time the wait
// gave up -- it must not, proving deck really did stop treating "restart
// claude" as selected, rather than merely failing to redraw its marker.
func TestFrameHasSelectedRowNamedOnCapturedHeaderInterleavingFrame(t *testing.T) {
	cases := []struct {
		name  string
		frame string
		want  bool
	}{
		{"row_selected_before_the_interleaving", rowSelectedFrame, true},
		{"header_selected_after_the_interleaving", headerSelectedFrame, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			screen := vt.NewEmulator(100, 6)
			screen.Write([]byte(tc.frame))
			driver := &ScreenDriver{screen: screen}
			got := frameHasSelectedRowNamed(driver.Frame(false), "restart claude")
			if got != tc.want {
				t.Fatalf("frameHasSelectedRowNamed(%q, %q) = %v, want %v\nframe:\n%s", tc.frame, "restart claude", got, tc.want, driver.Frame(false))
			}
		})
	}
}

// fakeKeySendDriver builds a *ScreenDriver whose "pty" is an os.Pipe
// instead of a real deck process, and whose frame content is driven
// entirely by the supplied dispatch function rather than by any actual
// key-handling loop. This is the minimum surface
// selectSessionByNameThenSend/navigateToRowByName/sendNavKeySettled need
// (Send + Frame + the done/updated channels WaitForFrame*/sendNavKeySettled
// poll) to exercise the real retry path deterministically -- scripting
// exactly when the CI-observed header-selected interleaving appears,
// rather than depending on the real, CI-only-reproducible timing race ever
// actually firing under this sandbox's own docker sibling (tried
// repeatedly while investigating this task, under -race and under
// artificial CPU/core contention via `docker run --cpus`, and never
// reproduced -- see this iteration's own notes).
func fakeKeySendDriver(t *testing.T, initial string, dispatch func(sent string) string) *ScreenDriver {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	d := &ScreenDriver{
		terminal: w,
		screen:   vt.NewEmulator(100, 6),
		updated:  make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	d.mu.Lock()
	_, _ = d.screen.Write([]byte(initial))
	d.mu.Unlock()
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := r.Read(buf)
			if err != nil {
				return
			}
			frame := dispatch(string(buf[:n]))
			d.mu.Lock()
			_, _ = d.screen.Write([]byte(frame))
			d.mu.Unlock()
			select {
			case d.updated <- struct{}{}:
			default:
			}
		}
	}()
	return d
}

// TestClientScreenContainsWithinReconcileIntervalRetriesPendingKeySend (B4,
// cure-01-01-2) proves clientScreenContainsWithinReconcileInterval's B4
// retry (features/assertions_test.go) actually recovers from the exact
// captured failure shape above end to end through the real step
// functions: clientPressesRestartOnNamedSession records a pending "R" on
// "restart claude" against a script that answers the FIRST "R" by
// clobbering the selection onto headerSelectedFrame -- exactly what B4
// observed -- so "fake-claude resume:" never appears from that keystroke.
// The very next clientScreenContainsWithinReconcileInterval("fake-claude
// resume:") must notice the wait failed, redo the select-then-send (now
// answered with rowSelectedFrame, then with the awaited "fake-claude
// resume:" text once the row is genuinely reselected and "R" resent), and
// return nil -- not merely a longer wait on the SAME dead keystroke.
func TestClientScreenContainsWithinReconcileIntervalRetriesPendingKeySend(t *testing.T) {
	firstRSent := false
	driver := fakeKeySendDriver(t, rowSelectedFrame, func(sent string) string {
		switch sent {
		case "R":
			if !firstRSent {
				firstRSent = true
				return headerSelectedFrame
			}
			return rowSelectedFrame + "fake-claude resume: restart claude\r\n"
		default: // "g" and any down-arrow bytes navigateToRowByName sends
			return rowSelectedFrame
		}
	})

	h := &ScenarioHarness{Home: t.TempDir(), namedClients: map[string]*ScreenDriver{"A": driver}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, scenarioHarnessKey{}, h)

	if err := clientPressesRestartOnNamedSession(ctx, "A", "restart claude"); err != nil {
		t.Fatalf("clientPressesRestartOnNamedSession: %v", err)
	}
	if _, ok := h.pendingKeySends["A"]; !ok {
		t.Fatalf("clientPressesRestartOnNamedSession did not record a pending key send")
	}

	if err := clientScreenContainsWithinReconcileInterval(ctx, "A", "fake-claude resume:"); err != nil {
		t.Fatalf("clientScreenContainsWithinReconcileInterval did not recover from the scripted B4 interleaving via retry: %v\nframe:\n%s", err, driver.Frame(false))
	}
	if _, ok := h.pendingKeySends["A"]; ok {
		t.Fatalf("pending key send must be consumed (one-shot) once the retry has run")
	}
}

// TestClientScreenContainsWithinReconcileIntervalControlNoPendingSend is
// TestClientScreenContainsWithinReconcileIntervalRetriesPendingKeySend's
// control: with no pendingKeySend recorded at all (the overwhelming
// majority of "within one configured reconcile interval" call sites, none
// of which follow a resume/restart keypress), a timeout must still be
// returned unchanged, on the very same deadline, rather than retried --
// there is nothing this mechanism could safely redo.
func TestClientScreenContainsWithinReconcileIntervalControlNoPendingSend(t *testing.T) {
	driver := fakeKeySendDriver(t, rowSelectedFrame, func(string) string { return rowSelectedFrame })
	h := &ScenarioHarness{Home: t.TempDir(), namedClients: map[string]*ScreenDriver{"A": driver}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, scenarioHarnessKey{}, h)

	err := clientScreenContainsWithinReconcileInterval(ctx, "A", "never appears")
	if err == nil {
		t.Fatal("expected a timeout error with nothing pending to retry")
	}
	t.Logf("control failed as expected with no pending send: %v", err)
}
