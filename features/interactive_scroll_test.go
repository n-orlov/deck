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

	// issue #29: the two page-stepping steps every scenario that must reach
	// a marker in the grid's scrollback now uses instead of a FIXED page
	// count. The fixed-count steps above stay registered and unchanged --
	// they are the raw primitive these two are built out of, and the only
	// honest way to express "exactly N pages" for a future scenario whose
	// point IS the page arithmetic -- but a scenario that wants to reach
	// particular CONTENT can no longer name a number, because the entry
	// seed now pulls the pane's own tmux history into the grid
	// (internal/interactive.CaptureSeedWithHistory) and how many pages sit
	// between the live bottom and that content depends on the pane's past.
	sc.Step(`^deck client "([^"]+)" scrolls back with shift\+pgup until the screen contains "([^"]+)"$`, clientScrollsBackWithShiftPgUpUntilScreenContains)
	sc.Step(`^deck client "([^"]+)" scrolls forward with shift\+pgdown by the same number of pages$`, clientScrollsForwardWithShiftPgDownBySameNumberOfPages)

	// issue #29's own premise: output a pane produced BEFORE deck entered
	// interactive mode at all, which no scenario could previously state.
	sc.Step(`^(\d+) numbered lines labelled "([^"]+)" are printed into session "([^"]+)"'s pane before deck enters interactive mode$`, sessionPanePrintsNumberedLinesBeforeInteractiveEntry)
	sc.Step(`^session "([^"]+)"'s pane holds "([^"]+)" in its tmux history but not on its visible screen$`, sessionPaneHoldsLineInTMuxHistoryButNotOnScreen)

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

// shiftPageScrollSearchMaxPages bounds the "scroll back until the screen
// contains ..." search below. A bound is needed only so that a genuine
// regression fails as a step error with a frame attached instead of pressing
// Shift+PgUp forever, so it is deliberately far larger than any distance a
// scenario in this package can legitimately need: at the harness's own
// 30-row terminal (features/pty_driver_test.go's terminalRows) one page is
// roughly twenty rows of interactive body, so 64 pages reaches something
// like 1200 lines back, while every marker any scenario here searches for
// is within the ~60 lines that scenario printed itself. Exhausting the
// grid's whole ScrollbackMaxLines bound of 2000 lines would take 100 pages
// at that height (286 at interactiveMinInnerRows' 7-row floor), so this is
// deliberately not a "reach anything the grid can hold" bound -- it is a
// termination guard for a search that should end in single digits.
// Crucially the bound is on the
// SEARCH, not on where the content is expected to be: the number of pages
// actually pressed is whatever it takes, which is what makes these
// scenarios indifferent to how much pre-entry tmux history the entry seed
// pulled in (issue #29).
const shiftPageScrollSearchMaxPages = 64

// shiftPageScrollMaxPagesToMarker is the assertion the search itself makes
// once it HAS found the marker, and it is the one thing the fixed
// "sends shift+pgup 3 times" steps this search replaced were still proving
// that nothing else in the repo proves: that Shift+PgUp steps a whole PAGE.
//
// Making the distance a search was right -- with the entry seed now
// prepending the pane's own tmux history (issue #29), how many pages sit
// between the live bottom and a marker is a property of the pane's past, not
// of the scenario -- but a search alone is blind to GRANULARITY. Measured:
// degrade internal/tui's scrollInteractiveByPage to `height := 1`, one LINE
// per keypress, and without this bound every scenario in this file still
// PASSED. The search simply pressed Shift+PgUp 33-35 times instead of 2 and
// found its marker anyway, well inside the 64-page termination guard above.
// Nothing else in internal/tui referenced scrollInteractiveByPage,
// shiftPageScrollDir or the raw "\x1b[5;2~" bytes at all, so that degrade was
// green repo-wide. The COUNT is what tells a page from a line, so the count
// is asserted.
//
// The bound is what these scenarios actually need on a green run plus honest
// headroom, both measured on this harness rather than reasoned about: all four
// Shift+PgUp scenarios here need exactly 2 pages green (markers "Do you want
// to proceed?", "INTERACTIVE_SCROLL_LINE_1 ", "PRE_ENTRY_LINE_1 " and
// "SNAP_SCROLL_LINE_1 ") and 33-35 presses under the one-line degrade. That
// is the expected shape: every marker is the first of the ~60 numbered lines
// the scenario printed itself (or, for the claude fixture, a prompt a couple
// of screenfuls up), so it sits ~35 rows above the live bottom, and at the
// harness's 30-row terminal (features/pty_driver_test.go's terminalRows) one
// page of interactive body is most of that.
//
// 8 is therefore 4x the green count. That much headroom is deliberate: the
// preview box can lose rows to a taller footer or a notice line, and even at
// the 7-row interactiveMinInnerRows floor a ~35-row distance is 5 pages. It
// still separates cleanly from both wrong step sizes, since a LINE step needs
// ~35 presses and a wheel-notch step (interactiveWheelStepLines = 3) ~12.
const shiftPageScrollMaxPagesToMarker = 8

// shiftPageRepaintWindow is how long one page step is given to reach the
// client's rendered frame before the search moves on to the next page, and
// shiftPageTearSettle is the extra pause taken once the frame has changed
// at all, before the marker is looked for in it.
//
// Both exist because a page step is observed through a real pty: deck's own
// repaint for the new offset is coalesced, and a frame read the instant the
// grid changes can be a partially-written one. Waiting for "the frame
// differs from the pre-keystroke frame" (via WaitForFrameFunc, so the wait
// is driven by real screen updates rather than a fixed sleep) and only then
// pausing briefly and re-reading is what keeps the search from stepping
// PAST the page the marker is on -- the one failure mode that would make
// these scenarios flaky rather than merely slow. A page that renders
// identically to its predecessor (two identical fixture screens, say) never
// trips that predicate at all, which is why the wait's own timeout is not
// treated as an error.
const (
	shiftPageRepaintWindow = 300 * time.Millisecond
	shiftPageTearSettle    = 50 * time.Millisecond
)

// clientScrollsBackWithShiftPgUpUntilScreenContains presses Shift+PgUp one
// WHOLE page at a time (the exact step internal/tui's
// scrollInteractiveByPage takes, so consecutive pages tile the grid's
// rows without gaps and no row can be scrolled past unseen) until want is
// on the client's rendered frame, and records how many pages that took so
// the matching Shift+PgDn step can undo exactly this much.
//
// It refuses to pass vacuously in two directions, one at each end:
//
//   - want must NOT already be on the frame when the step starts. That
//     refusal preserves the first half of what the fixed
//     "sends shift+pgup 3 times" + "screen contains ..." pair used to prove
//     -- content that is NOT on the live screen becomes visible again purely
//     because Shift+PgUp was pressed -- while dropping the one part of that
//     pair which was never really about the product: the number 3, measured
//     against an entry seed whose scrollback started EMPTY. Since issue #29
//     the seed prepends the pane's own tmux history, so three pages back is
//     wherever this harness's shell banner happens to put it.
//   - the number of pages it took must be SMALL
//     (shiftPageScrollMaxPagesToMarker). That is the second half of what the
//     fixed count proved, and the half a bare search would have thrown away:
//     that a press moves a whole PAGE rather than one line or one wheel
//     notch. See that constant's doc for the measured degrade this exists to
//     catch.
func clientScrollsBackWithShiftPgUpUntilScreenContains(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if frame := client.Frame(false); strings.Contains(frame, want) {
		return fmt.Errorf("client %q already shows %q before any Shift+PgUp, so scrolling back to it would prove nothing:\n%s", name, want, frame)
	}
	for pages := 1; pages <= shiftPageScrollSearchMaxPages; pages++ {
		before := client.Frame(false)
		if err := client.Send(shiftPgUpRawCSI); err != nil {
			return fmt.Errorf("send Shift+PgUp page %d: %w", pages, err)
		}
		// The same pacing every repeated-keystroke step in this package
		// needs (clientSendsRawSequenceNTimes' own doc): a burst of the
		// identical raw bytes can coalesce before deck's input loop has
		// drained the previous one, which here would silently scroll one
		// page where two were pressed.
		time.Sleep(25 * time.Millisecond)

		wait, cancel := context.WithTimeout(ctx, shiftPageRepaintWindow)
		frame, waitErr := client.WaitForFrameFunc(wait, false, func(frame string) bool {
			return strings.Contains(frame, want) || frame != before
		})
		cancel()
		// waitErr is only ever this window's own timeout, which is a
		// legitimate outcome (an identically-rendered page), so it is not
		// an error here -- but the SCENARIO's ctx being done is, and that
		// is the one case the timeout could be hiding.
		if waitErr != nil && ctx.Err() != nil {
			return fmt.Errorf("scrolling back to %q on client %q: %w", want, name, ctx.Err())
		}
		if !strings.Contains(frame, want) {
			time.Sleep(shiftPageTearSettle)
			frame = client.Frame(false)
		}
		if strings.Contains(frame, want) {
			if h.interactiveScrollPagesBack == nil {
				h.interactiveScrollPagesBack = make(map[string]int)
			}
			h.interactiveScrollPagesBack[name] = pages
			// The granularity assertion (shiftPageScrollMaxPagesToMarker's
			// own doc): finding the marker proves Shift+PgUp scrolls, and
			// only the COUNT proves it scrolls by a PAGE.
			if pages > shiftPageScrollMaxPagesToMarker {
				return fmt.Errorf("client %q needed %d Shift+PgUp presses to bring %q onto the frame, more than the %d a WHOLE-PAGE step can possibly need for a marker this scenario printed itself ~60 lines above the live bottom (a green run here takes 2) -- Shift+PgUp has stopped stepping a page: a one-LINE step needs ~35 presses and a wheel-notch-sized step ~12, and BOTH still find the marker eventually, which is why this count and not the search's success is what holds internal/tui's scrollInteractiveByPage to previewContentSize's content height:\n%s", name, pages, want, shiftPageScrollMaxPagesToMarker, client.Frame(false))
			}
			return nil
		}
	}
	return fmt.Errorf("client %q never showed %q within %d Shift+PgUp pages of its interactive scrollback:\n%s", name, want, shiftPageScrollSearchMaxPages, client.Frame(false))
}

// clientScrollsForwardWithShiftPgDownBySameNumberOfPages presses Shift+PgDn
// exactly as many times as the preceding Shift+PgUp search pressed
// Shift+PgUp, which returns the offset to the live bottom exactly:
// scrollInteractiveByPage adds and subtracts the same page height
// (previewContentSize's own content height, unchanged here since nothing
// resizes the terminal mid-scenario) and scrollInteractiveByLines clamps at
// 0, so an equal count can only land ON the live view, never short of it.
//
// It is deliberately a mirror of the recorded count rather than its own
// "scroll until the live bottom is visible" search: the assertions that
// follow it in the feature file are what must prove the view came back
// (the scrolled-back content gone, the live content present), and a search
// that hunted for those same strings itself would make those Then steps
// tautological.
func clientScrollsForwardWithShiftPgDownBySameNumberOfPages(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	pages, ok := h.interactiveScrollPagesBack[name]
	if !ok {
		return fmt.Errorf("client %q has not scrolled back with Shift+PgUp, so there is no page count to mirror", name)
	}
	return clientSendsRawSequenceNTimes(ctx, name, shiftPgDownRawCSI, pages)
}

// sessionPanePrintsNumberedLinesBeforeInteractiveEntry is issue #29's own
// premise made into a step: n numbered lines are printed into a session's
// REAL tmux pane while deck is not in interactive mode at all, so the
// output exists only in tmux's own history and pane screen and has never
// been written through any deck grid.
//
// It drives the pane directly with tmux send-keys on the scenario's private
// socket (features/env_editor_test.go's liveShellEchoesEnvKeyAs is the
// precedent), never through a deck client's keyboard, precisely because
// deck must not be involved in producing this output -- that is the whole
// point of the scenario it backs. It then waits for the pane's OWN
// capture-pane to show the last line, so the step cannot return before the
// shell has really finished printing; a deck frame could not be used for
// that wait, because deck renders another session's pane content only as a
// cropped passive preview and this output deliberately overflows it.
func sessionPanePrintsNumberedLinesBeforeInteractiveEntry(ctx context.Context, n int, prefix, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	target, err := sessionPaneTarget(h, session)
	if err != nil {
		return err
	}
	command := fmt.Sprintf("for i in $(seq 1 %d); do echo %s_$i; done", n, prefix)
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", "--", command); err != nil {
		return fmt.Errorf("send the numbered loop to session %q's live pane: %w", session, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("send Enter to session %q's live pane: %w", session, err)
	}
	last := fmt.Sprintf("%s_%d", prefix, n)
	deadline := time.Now().Add(5 * time.Second)
	var capture []byte
	for {
		capture, err = tmuxOutput(ctx, h, "capture-pane", "-p", "-t", target)
		if err == nil && paneCaptureHasExactLine(capture, last) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q's live pane never printed %q; last capture:\n%s", session, last, string(capture))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// sessionPaneHoldsLineInTMuxHistoryButNotOnScreen is the non-vacuity
// control for issue #29's scenario, asserted against tmux itself rather
// than against deck: the line must be in the pane's SCROLLED-OFF history
// and must NOT be on its visible screen. Without it, "Shift+PgUp shows the
// line" could pass on a build that only ever seeds the visible screen,
// merely because the pane happened to be short enough for the line to
// still be on it.
//
// The history range asked for here is the same one the product asks for --
// internal/interactive.ScrollbackMaxLines, 2000 lines, spelled as a literal
// because this package deliberately observes only the released binary,
// tmux, SQLite and the filesystem (features/assertions_test.go's own
// registerBlackBoxAssertionSteps doc). tmux clamps a -S beyond the real
// history rather than failing, so the value is an upper bound, not an
// assumption about how much history exists.
func sessionPaneHoldsLineInTMuxHistoryButNotOnScreen(ctx context.Context, session, line string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	target, err := sessionPaneTarget(h, session)
	if err != nil {
		return err
	}
	visible, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-t", target)
	if err != nil {
		return fmt.Errorf("capture session %q's visible pane screen: %w", session, err)
	}
	if paneCaptureHasExactLine(visible, line) {
		return fmt.Errorf("session %q's pane still shows %q on its VISIBLE screen, so nothing about it needs scrollback at all:\n%s", session, line, string(visible))
	}
	history, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-2000", "-t", target)
	if err != nil {
		return fmt.Errorf("capture session %q's pane history: %w", session, err)
	}
	if !paneCaptureHasExactLine(history, line) {
		return fmt.Errorf("session %q's pane does not hold %q in its tmux history either, so the line was never printed:\n%s", session, line, string(history))
	}
	return nil
}

// sessionPaneTarget resolves a scenario's display name for a session to the
// tmux target its single pane lives in, the same "deck_" + slug shape every
// other step in this package that talks to a live pane uses.
func sessionPaneTarget(h *ScenarioHarness, session string) (string, error) {
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return "", err
	}
	return "deck_" + slug, nil
}

// paneCaptureHasExactLine reports whether a capture-pane -p body contains a
// row whose whole content is want. It is an exact, whole-row match rather
// than a substring one so that a marker like "PRE_ENTRY_LINE_1" cannot be
// satisfied by "PRE_ENTRY_LINE_10", nor by the shell's own echo of the loop
// command that printed it. Trailing spaces are trimmed because whether they
// are present at all is a capture-pane flag (-N) rather than a property of
// the pane.
func paneCaptureHasExactLine(capture []byte, want string) bool {
	for _, line := range strings.Split(string(capture), "\n") {
		if strings.TrimRight(line, " ") == want {
			return true
		}
	}
	return false
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
