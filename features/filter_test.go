package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
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
	sc.Step(`^deck client "([^"]+)" filtered sidebar comes to list exactly one session row, named "([^"]+)"$`, clientFilteredSidebarListsExactlyOneSessionRowNamed)
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
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q still has archived_at=%d after U\nframe:\n%s", sessionName, archivedAt, client.Frame(false))
		}
		time.Sleep(25 * time.Millisecond)
	}
	// The store write lands before deck repaints the row it unarchived, so
	// returning on the database alone let the next step race that repaint:
	// nightly dispatch run 37327435258 (85cb0f91f7, -race) recorded
	// filter.feature:97 flaky when the row losing its "[archived]" badge
	// changed the selected sidebar line, navigateToRowByName's "g" settle
	// took that repaint for its own, and "r" followed "g" by 7 ms -- close
	// enough to be read as one "gr" KeyMsg that the keymap ignores (the same
	// coalescing clientOpensDetailForSession documents), so resume never
	// fired. Waiting here for the repaint itself -- the selected row without
	// its archived badge and the footer no longer offering U -- leaves no
	// unarchive repaint in flight for a following step to mistake.
	if _, err := client.WaitForFrameFunc(ctx, false, unarchiveRepainted); err != nil {
		return fmt.Errorf("deck client %q never repainted %q as unarchived after U: %w\nframe:\n%s", clientName, sessionName, err, client.Frame(false))
	}
	return nil
}

// unarchiveRepainted reports whether frame shows deck's own repaint after
// `U`: the selected sidebar row no longer carries the archived badge (ascii
// "[archived]", possibly truncated to "[arch...", or the unicode glyph) and
// the footer no longer offers "U unarchive" for it.
func unarchiveRepainted(frame string) bool {
	row := selectedSidebarLine(frame)
	if strings.Contains(row, "[arch") || strings.Contains(row, "\u25a3") {
		return false
	}
	return !strings.Contains(frame, "U unarchive")
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

// clientFilteredSidebarListsExactlyOneSessionRowNamed waits for a frame whose
// sidebar column (not the filter field's echo below the box, not the preview)
// lists exactly one session row, named name. It is the row-level proof that
// the list re-narrowed to the query on screen: the input echo alone names the
// query whether or not any row matches it.
func clientFilteredSidebarListsExactlyOneSessionRowNamed(ctx context.Context, clientName, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	frame, err := client.WaitForFrameFunc(ctx, false, func(frame string) bool {
		return sidebarListsOnlySessionRow(frame, name) == nil
	})
	if err != nil {
		return fmt.Errorf("deck client %q: %w (last verdict: %w):\n%s", clientName, err, sidebarListsOnlySessionRow(frame, name), frame)
	}
	return nil
}

// sidebarListsOnlySessionRow is that step's verdict on one frame, in either
// glyph mode: the sidebar column (the text between a content row's left
// border and the sidebar/preview divider, `│` or ASCII `|`) holds exactly one
// session row -- a line whose text, after the selection gutter, leads with a
// status glyph (sidebarRowLeadGlyphs) -- and that row's name is name. Group
// headers, a row's second line and anything outside the box (the filter
// field's own echo) are not session rows.
func sidebarListsOnlySessionRow(frame, name string) error {
	var names []string
	for _, line := range strings.Split(frame, "\n") {
		cells := strings.Split(line, "\u2502")
		if len(cells) < 3 {
			cells = strings.Split(line, "|")
		}
		if len(cells) < 3 {
			continue
		}
		text := strings.TrimSpace(cells[1])
		text = strings.TrimPrefix(text, "> ")
		entry := stripSidebarRowLead(text)
		if entry == text {
			continue // no status glyph: a header, line 2, or a hint
		}
		if fields := strings.Fields(entry); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	if len(names) != 1 || names[0] != name {
		return fmt.Errorf("sidebar lists session rows %q, want exactly [%q]", names, name)
	}
	return nil
}

// TestSidebarListsOnlySessionRowReadsTheRowsNotTheEcho pins that the filter
// scenario's row step fails while the list is still empty even though the
// field's echo below the box names the query, and fails when a second row is
// listed. The frames are the ASCII sidebar a real client renders in
// filter.feature's middle-edit scenario.
func TestSidebarListsOnlySessionRowReadsTheRowsNotTheEcho(t *testing.T) {
	const (
		top    = "+ deck - sessions -----------------+----------------+"
		sock   = "| socket: deck_test_1093_2         | $              |"
		group  = "| v default  (%d)                   |                |"
		sel    = "| > ~ filter-edit-alpha running    |                |"
		other  = "|   ~ filter-edit-beta running     |                |"
		when   = "|   2s ago                         |                |"
		blank  = "|                                  |                |"
		bottom = "+----------------------------------+----------------+"
		echo   = "Filter: filter-edit-alpha"
	)
	join := func(lines ...string) string { return strings.Join(lines, "\n") }
	cases := []struct {
		name  string
		frame string
		ok    bool
	}{
		{"narrowed to the edited query", join(top, sock, fmt.Sprintf(group, 1), sel, when, blank, bottom, echo), true},
		{"empty list, echo names the query", join(top, sock, fmt.Sprintf(group, 0), blank, blank, bottom, echo), false},
		{"not narrowed", join(top, sock, fmt.Sprintf(group, 2), sel, when, other, when, bottom, echo), false},
		{"only the other row", join(top, sock, fmt.Sprintf(group, 1), other, when, bottom, echo), false},
		{"unicode frame narrowed", strings.ReplaceAll(join(top, sock, fmt.Sprintf(group, 1), sel, when, bottom), "|", "\u2502"), true},
	}
	for _, tc := range cases {
		err := sidebarListsOnlySessionRow(tc.frame, "filter-edit-alpha")
		if (err == nil) != tc.ok {
			t.Errorf("%s: verdict %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}
