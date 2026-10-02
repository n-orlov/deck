package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerFilterSteps backs features/filter.feature (task 123, I-10, SPEC
// requirement 33): opening the `/` list filter, typing an incremental
// query into it, and clearing it with Esc -- plus R71's `U` (issue #8),
// pressed on a row the filter is the only way to reach.
func registerFilterSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the list filter$`, clientOpensListFilter)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the filter field$`, clientTypesIntoFilterField)
	sc.Step(`^deck client "([^"]+)" presses left (\d+) times in the filter field$`, clientPressesLeftInFilterField)
	sc.Step(`^deck client "([^"]+)" inserts "([^"]*)" at the caret of the filter field$`, clientInsertsAtFilterCaret)
	sc.Step(`^deck client "([^"]+)" keeps the filter in force with enter$`, clientKeepsFilterInForceWithEnter)
	sc.Step(`^deck client "([^"]+)" unarchives its selected session "([^"]+)"$`, clientUnarchivesSelectedSession)
	sc.Step(`^deck client "([^"]+)" clears the list filter with escape$`, clientClearsListFilterWithEscape)
}

// clientOpensListFilter sends `/` (SPEC.md:984, requirement 33), the only
// way updateFilter's text field gets keyboard focus. filterStatusLine's
// own "Filter: " prefix appears on screen the instant filtering opens,
// even with an empty query, so that is what this waits for.
func clientOpensListFilter(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("/"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Filter:")
}

// clientTypesIntoFilterField sends query as raw keystrokes into the
// already-open filter field (SPEC §11.10 "incrementally"): the typed text
// is echoed back verbatim inside filterStatusLine's own "Filter: <query>"
// text, so waiting for that same substring to appear on screen is a
// universal, query-content-independent readiness check -- unlike waiting
// for a session row's name, which would not apply to every step (e.g. a
// query typed one field at a time in a scenario asserting per-keystroke
// narrowing).
func clientTypesIntoFilterField(ctx context.Context, clientName, query string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send(query); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Filter: "+query)
}

// clientClearsListFilterWithEscape sends Esc while the filter field has
// focus: SPEC §11.10's "esc clears the query and returns to the unfiltered
// list" -- the query is discarded, not merely the field's focus, so
// filterStatusLine goes back to reporting nothing at all and "Filter:"
// leaves the screen entirely.
func clientClearsListFilterWithEscape(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b"); err != nil {
		return err
	}
	return client.WaitForFrameGone(ctx, false, "Filter:")
}

// clientKeepsFilterInForceWithEnter sends Enter while the filter field has
// focus: SPEC.md's "Enter keeps the filter applied and returns the keymap to
// the (now narrowed) list" -- the query and its narrowed list both stay in
// force while the text field closes, which is what makes a top-level key
// like R71's `U` reach a row only the filter can display. The status line
// switches from the live "Filter: <query>" echo to the held
// `Filter "<query>" in force (N matching)` wording, so waiting for "in
// force" is a direct observation of the field having closed with the query
// kept -- never a sleep, and never satisfiable by the open-field frame.
func clientKeepsFilterInForceWithEnter(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "in force")
}

// clientUnarchivesSelectedSession sends R71's `U` (issue #8) on whatever row
// the (filtered) list has selected and waits until archived_at is really
// back to 0 in the state database. It goes through the real keypress, the
// real narrowed list and the real service/store path -- nothing here calls
// Unarchive directly -- and the wait is bounded on that durable
// consequence, polled the way stateDatabaseSessionIsReaped polls its own,
// because the store write happens on a bubbletea command goroutine that
// races a read-once assertion.
func clientUnarchivesSelectedSession(ctx context.Context, clientName, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("U"); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		archivedAt, err := sessionArchivedAt(ctx, sessionName)
		if err != nil {
			return err
		}
		if archivedAt == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q still has archived_at=%d after U\nframe:\n%s", sessionName, archivedAt, client.Frame(false))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// clientPressesLeftInFilterField sends n real left-arrow keys to the open
// filter field, moving the shared editor's caret (SPEC §11.11). A caret move
// alone changes nothing the screen's text shows, so the next insertion and the
// frame assertion on the edited query are what observe it.
func clientPressesLeftInFilterField(ctx context.Context, clientName string, n int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := client.Send("\x1b[D"); err != nil {
			return err
		}
	}
	return nil
}

// clientInsertsAtFilterCaret types text at the filter field's caret. Unlike
// clientTypesIntoFilterField it does not wait for "Filter: "+text, because
// with the caret in the middle the typed text is not contiguous with the
// label; the scenario's own frame assertion on the edited query observes it.
func clientInsertsAtFilterCaret(ctx context.Context, clientName, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	return client.Send(text)
}
