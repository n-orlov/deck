package features

import (
	"context"

	"github.com/cucumber/godog"
)

// registerFilterSteps backs features/filter.feature (task 123, I-10, SPEC
// requirement 33): opening the `/` list filter, typing an incremental
// query into it, and clearing it with Esc.
func registerFilterSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the list filter$`, clientOpensListFilter)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the filter field$`, clientTypesIntoFilterField)
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
// already-open filter field (SPEC.md:318 "incrementally"): the typed text
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
// focus: SPEC.md:318's "esc clearing" -- the query is discarded, not
// merely the field's focus, so filterStatusLine goes back to reporting
// nothing at all and "Filter:" leaves the screen entirely.
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
