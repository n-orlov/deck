package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/n-orlov/deck/internal/theme"
)

// navigateToRowByName is the single top-anchored navigation helper behind
// selectSessionByNameThenSend, selectRowByName and clientOpensDetailForSession
// (steer 3e-002; collapses three near-duplicate copies of this search, each
// of which had its own ad hoc settle strategy -- a fixed time.Sleep in two
// of the three, and a race in the third before task 324/7ebafce patched it
// alone).
//
// Requirement 52 (task 301) auto-selects whichever session was most
// recently created, which can leave the cursor anywhere relative to want's
// row -- and, while any session in the fixture is still "starting"/
// "running", the ATTENTION order itself can still be shifting between
// attempts (a session's urgency changes as it settles), so a plain
// jump-to-top-then-walk-down-once is not enough: the walk from a previous
// attempt can be invalidated by a reorder before the marker is ever seen.
// Each attempt therefore resets to the top ("g" -- SPEC.md's quoted
// "`g`/`G` top/bottom", not a line number, since a SPEC edit can and did
// move that line) and walks exactly `attempt` rows down before checking,
// so every attempt performs a full, independent, top-anchored search --
// one that keeps succeeding once the fixture's ordering finally stops
// changing, however many attempts that takes, rather than depending on a
// single walk surviving unchanged across the whole search.
//
// There IS a bound "go to top" key ("g"); a prior version of this doc
// comment (on the now-deleted standalone selectRowByName) claimed
// otherwise and made two callers rewind with a fixed count of up-arrows
// instead -- both now use this helper's own "g" reset like the other
// caller always did.
func navigateToRowByName(ctx context.Context, client *ScreenDriver, want string) error {
	// frameHasSelectedRowNamed skips the row's status glyph and pin marker
	// (SPEC §11 line 1: gutter, status glyph, pin marker, name).
	selected := func() bool { return frameHasSelectedRowNamed(client.Frame(false), want) }
	// When want's row is already the selected one, send nothing: a "g" that
	// cannot move the selection is a no-op that any unrelated repaint (a
	// badge or order refresh still in flight) is taken for the "g"'s own,
	// and the caller's action key then follows within milliseconds and is
	// coalesced with it into one ignored "gr" KeyMsg. That is how
	// filter.feature:97 went flaky again in the ci.yml dispatch run
	// 37844873731 (de263c8a72e), U, g and r 28 ms apart.
	if selected() {
		return nil
	}
	for attempt := 0; attempt < 50; attempt++ {
		if err := sendNavKeySettled(ctx, client, "g"); err != nil {
			return err
		}
		if selected() {
			return nil
		}
		for step := 0; step < attempt; step++ {
			if err := sendNavKeySettled(ctx, client, "\x1b[B"); err != nil { // down arrow
				return err
			}
			if selected() {
				return nil
			}
		}
	}
	return fmt.Errorf("never selected session %q (no selected sidebar row names it):\n%s", want, client.Frame(false))
}

// navKeySettleWindow bounds how long sendNavKeySettled will wait for a
// keystroke's own repaint before concluding the keystroke was a genuine
// no-op (already at the top, or already at the bottommost row -- arrows
// never wrap past the first/last row, group.go's nextVisibleSelection/
// prevVisibleSelection). It is a ceiling on an event-driven wait, not a
// fixed delay every call pays: a real repaint is observed via
// ScreenDriver.WaitForFrameGone/WaitForFrame's d.updated channel and
// returns as soon as it happens, usually in well under a millisecond.
var navKeySettleWindow = 300 * time.Millisecond

// sendNavKeySettled sends key, then waits for the sidebar's currently
// selected ("> "-prefixed) row line to actually change before returning --
// an observable consequence of the keystroke having been read and
// repainted by deck's own key-handling/render loop, not a guess at how
// long that takes.
//
// Send() only writes to the pty; it never waits for that loop, so checking
// the frame immediately after Send races it: a "match" seen right after
// sending a burst of keys can reflect fewer keystrokes than were actually
// sent, so the still-in-flight remainder lands after a caller's own
// follow-up action (a resume/restart/detail keypress, or another client's
// concurrent race keypress) fires -- moving the selection a moment after
// that action was aimed at it. This was task 301's regression, patched ad
// hoc for one of these three call sites alone in 7ebafce; this helper
// fixes it for all three by construction.
//
// This polls the same d.updated channel WaitForFrame/WaitForFrameGone poll
// (features/pty_driver_test.go), comparing selectedSidebarLine's own
// scoped snippet across the wait rather than calling either of those two
// methods directly with a substring: a caller with no row currently
// selected in the visible frame (e.g. the selection scrolled off-screen
// after a layout-mode cycle) has no target substring to wait for or
// against, and a first version of this helper substituted "wait for ANY
// '> ' anywhere in the whole frame" for that case -- which is unsound and
// was caught by this task's own testing (features/mouse.feature's
// @requirement-34-wheel-scrolls-without-selecting scenario went flaky
// under it): earlier create-dialog renders leave "> Name: ..."/
// "> Working directory: ..." field markers on rows the next, shorter
// render never overwrites, and an unscoped "> " search can match that
// stale leftover before deck has even processed the keystroke. Comparing
// the selected row's identity (selectedSidebarLine: the sidebar cell with
// its status glyph, pin marker and badge run stripped) to its own prior
// value has no such false-positive surface, and no repaint of the
// still-selected row's own status can change it either (task 012, M2).
func sendNavKeySettled(ctx context.Context, client *ScreenDriver, key string) error {
	before := selectedSidebarLine(client.Frame(false))
	if err := client.Send(key); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, navKeySettleWindow)
	defer cancel()
	for {
		if selectedSidebarLine(client.Frame(false)) != before {
			return nil
		}
		select {
		case <-client.done:
			return fmt.Errorf("deck exited while waiting for the sidebar selection to settle after %q: %w\nframe:\n%s", key, client.processError(), client.Frame(false))
		case <-client.updated:
		case <-waitCtx.Done():
			// The keystroke did not move the selection at all -- a
			// genuine no-op (top/bottom edge, "g" already at the top, or
			// no row selected yet in this view). The caller's own marker
			// check decides what to do next; it is not this helper's job
			// to distinguish "no-op" from "failed" here.
			return nil
		}
	}
}

// selectSessionByNameThenSend selects the sidebar row whose name is want,
// then sends key.
func selectSessionByNameThenSend(ctx context.Context, client *ScreenDriver, want, key string) error {
	if err := navigateToRowByName(ctx, client, want); err != nil {
		return err
	}
	return client.Send(key)
}

// selectRowByName moves client's selection to the row whose name is want,
// matching the same "> name" marker clientPressesResumeOnNamedSession
// relies on. It is factored out so the launch-lease race step (task 027)
// can position every racing client on the same row BEFORE firing `r`
// concurrently, since the positioning itself must stay sequential (each
// keystroke is a real PTY write) while only the final `r` needs to land
// within the race window.
func selectRowByName(ctx context.Context, client *ScreenDriver, want string) error {
	return navigateToRowByName(ctx, client, want)
}

// clientOpensDetailForSession selects the named row and then presses "i"
// (internal/tui's detail-toggle key) instead of a resume/restart key.
//
// It waits for the detail view's own header ("<name> detail", from
// internal/tui's detailView) to actually render before returning, rather
// than returning as soon as the "i" byte is written. Without that wait, a
// caller that immediately sends another key (as clientExitsCleanly's "q"
// does when no intervening assertion forces a genuinely fresh render, e.g.
// features/crash.feature's SIGKILL scenario when "crash final line" is
// already visible in the preview pane before "i" is even sent) can write
// that key before deck's input loop has drained the "i" byte. bubbletea's
// own reader coalesces same-buffer printable runes into a single KeyMsg
// (key.go's detectOneMsg, "longest sequence of runes"), and deck's per-key
// switch in internal/tui's Update silently ignores a msg whose String() is
// "iq" -- matching neither the "i" nor the "q" case -- so tea.Quit never
// fires and the client hangs at teardown (task 014's diagnosis; see
// docs/reports/phase2b2-findings.md). Waiting for the real post-"i" render
// here guarantees deck's reader has already drained and processed the "i"
// byte by the time this step returns, so a following "q" cannot land in
// the same read.
func clientOpensDetailForSession(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := navigateToRowByName(ctx, client, want); err != nil {
		return err
	}
	if err := client.Send("i"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WaitForFrame(wait, false, want+" detail"); err != nil {
		return fmt.Errorf("deck client %q detail view for %q never rendered after \"i\": %w", clientName, want, err)
	}
	return nil
}

// selectedSidebarLine returns the frame's currently selected ("> "-
// prefixed) sidebar row's text, or "" if no row is selected in the
// current frame. The SEARCH for the "> " marker is scoped to the first
// few columns of each line (the sidebar is always the frame's leftmost
// panel) rather than searching the whole line, because the preview pane
// -- to the right of the seam -- can legitimately contain "> " as
// ordinary captured pane content (a shell continuation prompt, a quoted
// diff hunk, an agent's own reply) that has nothing to do with sidebar
// selection; scanning the whole frame for that substring would
// false-positive on it (the same class of bug task 321's gotcha
// documents for previewTitle() embedding a session name on the sidebar's
// own top-border row -- scope narrowly).
//
// R169 (task 002) cropped the returned text to the sidebar's own column,
// so a preview-only repaint on the selected row's screen line could not
// change it. That was not enough, because the sidebar-cropped line still
// carried the selected row's own STATUS: its leading status glyph and its
// trailing badge run (unseen marker, quality word, status word, archived
// badge -- sidebarRowLines, internal/tui/tui.go). sendNavKeySettled below
// compares this return value before and after a keystroke to decide the
// keystroke has been acknowledged, so a same-row status repaint -- the
// still-selected row going starting -> stopped, or waiting losing its
// unseen marker, a moment after the keystroke was written but before deck
// read it -- falsely acknowledged a navigation key that had not landed.
// navigateToRowByName then still saw its target selected, returned, and
// its caller's follow-up key (R, i, Enter) raced ahead; deck processed the
// queued "g" first and the follow-up key landed on the group header (task
// 012, inventory mechanism M2: dialogs.feature:214's pin dialog never
// opening, status_attach.feature:20's interactive preview never entering).
//
// So the return value is now the selected row's IDENTITY, not its text:
// the sidebar cell (sidebarCell, between the line's first two borders --
// the sidebar's own column in every layout) after the "> " gutter, with
// the status glyph and pin marker stripped (stripSidebarRowLead) and the
// trailing badge run stripped (stripSidebarBadgeRun). What remains is the
// row's name -- or the visible prefix of it a narrow sidebar leaves, which
// padTrunc cuts at a fixed column no status change can move -- and only an
// actual selection change (a different row under the marker, or no marker
// at all: a selected group header renders in reverse video, not with "> ")
// can change it. A line with no recognisable cell (no two borders) falls
// back to the whole line through the same two strips.
func selectedSidebarLine(frame string) string {
	const scanWidth = 6
	for _, line := range strings.Split(frame, "\n") {
		head := line
		if len(head) > scanWidth {
			head = head[:scanWidth]
		}
		if !strings.Contains(head, "> ") {
			continue
		}
		text := line
		if cell, ok := sidebarCell(line); ok {
			text = cell
		}
		text = strings.TrimLeft(text, " ")
		if rest, ok := strings.CutPrefix(text, "> "); ok {
			text = rest
		} else if i := strings.Index(text, "> "); i >= 0 {
			text = text[i+len("> "):]
		}
		return "> " + stripSidebarBadgeRun(stripSidebarRowLead(text))
	}
	return ""
}

// stripSidebarBadgeRun removes a session row's trailing line-1 badge run
// (sidebarRowLines: an optional unseen glyph, an optional quality word, the
// status word, an optional archived badge) from text, the row's own
// "<name> <badge run>" once its gutter, status glyph and pin marker are
// gone, and returns the name with its fields joined by single spaces. A
// trailing field ending in the row's width ellipsis ("…" or "...") is
// stripped too when its visible prefix is a prefix of a badge token: that
// is a badge cut short by the sidebar's width, and which badge it was (and
// so how much of it shows) depends on the status. A name that itself ends
// in a badge-shaped word loses that word as well; the result is only ever
// compared with itself across a repaint, so that costs nothing but a
// slower acknowledgement for two adjacent rows whose names then collide.
func stripSidebarBadgeRun(text string) string {
	tokens := []string{"\u25cf", "!", "live", "sampled", "\u25a3", "[archived]"}
	for _, st := range theme.StatusTokens {
		tokens = append(tokens, string(st))
	}
	isBadge := func(field string) bool {
		for _, marker := range []string{"\u2026", "..."} {
			if prefix, ok := strings.CutSuffix(field, marker); ok {
				for _, tok := range tokens {
					if strings.HasPrefix(tok, prefix) {
						return true
					}
				}
				return false
			}
		}
		for _, tok := range tokens {
			if field == tok {
				return true
			}
		}
		return false
	}
	fields := strings.Fields(text)
	for len(fields) > 1 && isBadge(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}
