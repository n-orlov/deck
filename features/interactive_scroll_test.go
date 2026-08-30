package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerInteractiveScrollSteps backs features/interactive_scroll.feature
// (task 068, PRD II-51): the grid's own bounded scrollback, scrolled by
// Shift+PgUp/PgDn and the mouse wheel, proven against a real tmux pane and
// a real deck client rather than only internal/interactive's own
// RenderRows/offset unit tests.
func registerInteractiveScrollSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" types a (\d+)-line numbered loop labelled "([^"]+)" into the interactive pane$`, clientTypesNumberedLoopIntoInteractivePane)
	sc.Step(`^deck client "([^"]+)" types "([^"]+)" and Enter into the interactive pane$`, clientTypesTextAndEnterIntoInteractivePane)
	sc.Step(`^deck client "([^"]+)" sends shift\+pgup (\d+) times?$`, clientSendsShiftPgUpNTimes)
	sc.Step(`^deck client "([^"]+)" sends shift\+pgdown (\d+) times?$`, clientSendsShiftPgDownNTimes)
	sc.Step(`^deck client "([^"]+)" scrolls the interactive wheel up (\d+) times? over the line containing "([^"]+)"$`, clientScrollsInteractiveWheelUpOverLineContaining)
	sc.Step(`^deck client "([^"]+)" scrolls the interactive wheel down (\d+) times? over the line containing "([^"]+)"$`, clientScrollsInteractiveWheelDownOverLineContaining)

	// requirement 52 (task 069): scrolling the grid's own bounded
	// scrollback must never leak into what deck reports for the session.
	sc.Step(`^probe fixture agents for interactive-scroll are configured$`, configureAttachScrollProbeScenario)
	sc.Step(`^within several probe/repair cycles deck client "([^"]+)" row "([^"]+)" contains "([^"]+)"$`, clientRowContainsAcrossSeveralProbeCycles)
}

// clientRowContainsAcrossSeveralProbeCycles is
// features/status_probe_test.go's clientRowContainsWithinReconcile widened
// for this scenario's own timing, not a weaker assertion: it still requires
// the exact same text in the exact same row, only over a longer,
// deliberately-derived window. The probe write this scenario's own "the
// state database session ... has probe status \"error\" step just made is a
// bare probe-sourced error with no pane-exit verdict, so SPEC §7's self-heal
// (internal/service.reconcile's repairTerminalRowWithLivePane) leaves it
// alone rather than repairing it (task 902, per task 901's finding F40) --
// there is no oscillation to catch mid-cycle here, only the ordinary
// probe-then-render latency across client B's own independent ~250ms
// reconcile ticker (features/lifecycle_test.go's scenarioReconcileInterval)
// and whatever coalesced repaint follows it. The generous multi-cycle
// window is kept rather than narrowed, since nothing about this assertion
// depends on the row being caught inside a narrow slice any more.
func clientRowContainsAcrossSeveralProbeCycles(ctx context.Context, clientName, rowName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(4 * (attachScrollProbeStaleAfter + scenarioReconcileInterval))
	for {
		for _, line := range strings.Split(client.Frame(false), "\n") {
			if strings.Contains(line, rowName) && strings.Contains(line, want) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q row %q did not contain %q across several probe/repair cycles\nframe:\n%s", clientName, rowName, want, client.Frame(false))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// clientTypesNumberedLoopIntoInteractivePane types a shell for-loop that
// echoes n numbered lines sharing prefix, followed by Enter, and waits for
// the last one to reach the client's own rendered frame -- proof the whole
// path (deck's own key forwarding while m.interactive is true ->
// tmux.Dispatcher.SendLiteral -> the real pane's shell -> pipe-pane's own
// live drain -> the grid's Write -> a coalesced repaint) delivered it, not
// merely that deck accepted the keystrokes. n is chosen by each scenario to
// comfortably exceed the harness's own terminal height (30 rows,
// features/pty_driver_test.go), so scrolling back genuinely reaches
// content no longer on the live screen at all.
func clientTypesNumberedLoopIntoInteractivePane(ctx context.Context, name string, n int, prefix string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	command := fmt.Sprintf("for i in $(seq 1 %d); do echo %s_$i; done\r", n, prefix)
	if err := client.Send(command); err != nil {
		return err
	}
	lastLine := fmt.Sprintf("%s_%d", prefix, n)
	return client.WaitForFrame(ctx, false, lastLine)
}

// clientTypesTextAndEnterIntoInteractivePane types text followed by Enter
// into the currently interactive pane, the same forwarded-keystroke path
// clientTypesNumberedLoopIntoInteractivePane's own doc describes.
func clientTypesTextAndEnterIntoInteractivePane(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send(text + "\r"); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// shiftPgUpRawCSI/shiftPgDownRawCSI are the raw bytes a real xterm-class
// terminal sends for Shift+PgUp/PgDn -- see internal/tui's own
// shiftPgUpCSIString/shiftPgDownCSIString doc for why Bubble Tea's own key
// decoder (this pinned version) has no named Key type for either at all,
// which is exactly what makes this scenario worth proving end to end
// against the real, compiled binary rather than trusting a unit test of
// deck's own fmt.Stringer-based recognition alone.
const (
	shiftPgUpRawCSI   = "\x1b[5;2~"
	shiftPgDownRawCSI = "\x1b[6;2~"
)

func clientSendsShiftPgUpNTimes(ctx context.Context, name string, times int) error {
	return clientSendsRawSequenceNTimes(ctx, name, shiftPgUpRawCSI, times)
}

func clientSendsShiftPgDownNTimes(ctx context.Context, name string, times int) error {
	return clientSendsRawSequenceNTimes(ctx, name, shiftPgDownRawCSI, times)
}

// clientSendsRawSequenceNTimes sends seq repeatedly with a short pause
// between each send -- the same pacing precedent
// features/assertions_test.go's sendClientKeys documents (a back-to-back
// write burst of the same raw bytes can coalesce or drop all but the
// first one before deck's own input loop has drained the previous send).
func clientSendsRawSequenceNTimes(ctx context.Context, name, seq string, times int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	for i := 0; i < times; i++ {
		if err := client.Send(seq); err != nil {
			return fmt.Errorf("send %d/%d: %w", i+1, times, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

// clientScrollsInteractiveWheelUpOverLineContaining/...Down... locate text
// in the client's OWN current frame (locatePreviewText, features/mouse_bindings_
// test.go) -- deck's own preview panel content, while m.interactive is
// true, not a hand-computed column/row that would silently drift the
// moment layout shifts -- and send that many synthesized SGR wheel reports
// at the exact cell that text occupies.
func clientScrollsInteractiveWheelUpOverLineContaining(ctx context.Context, name string, times int, text string) error {
	return clientScrollsInteractiveWheelOverLineContaining(ctx, name, times, text, true)
}

func clientScrollsInteractiveWheelDownOverLineContaining(ctx context.Context, name string, times int, text string) error {
	return clientScrollsInteractiveWheelOverLineContaining(ctx, name, times, text, false)
}

func clientScrollsInteractiveWheelOverLineContaining(ctx context.Context, name string, times int, text string, up bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	col, row, err := locatePreviewText(client, text)
	if err != nil {
		return err
	}
	for i := 0; i < times; i++ {
		var sendErr error
		if up {
			sendErr = client.WheelUp(col, row)
		} else {
			sendErr = client.WheelDown(col, row)
		}
		if sendErr != nil {
			return fmt.Errorf("wheel notch %d/%d: %w", i+1, times, sendErr)
		}
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}
