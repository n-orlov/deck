package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerPinPersistenceSteps backs features/pin_persistence.feature (task
// 012, requirements R159/R160): pressing `p` on a sidebar row, and reading
// back whether a named row currently carries the `✦`/`*` pin marker
// (task 006's sidebarStatusGlyph lead, SPEC §11's fixed line-1 order --
// gutter, status glyph, pin marker, name).
func registerPinPersistenceSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" presses p on session "([^"]+)"$`, clientPressesPOnSession)
	sc.Step(`^deck client "([^"]+)" row "([^"]+)" shows the pin marker$`, clientRowShowsThePinMarker)
	sc.Step(`^deck client "([^"]+)" row "([^"]+)" does not show the pin marker$`, clientRowDoesNotShowThePinMarker)
}

// clientPressesPOnSession selects the sidebar row named want (by repeated
// down-arrows from the top, selectSessionByNameThenSend's own convention --
// features/navigation_settle_test.go) and sends the top-level `p` key,
// which task 010 bound to toggle that single row's pin.
func clientPressesPOnSession(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	return selectSessionByNameThenSend(ctx, client, want, "p")
}

// clientRowShowsThePinMarker polls (like clientScreenContainsBefore) for
// the named row to render with the pin marker immediately before its name,
// and clientRowDoesNotShowThePinMarker polls for the opposite -- both
// bounded at 5s since the marker is a pure render of already-loaded
// session state, never something reconciliation produces.
func clientRowShowsThePinMarker(ctx context.Context, clientName, name string) error {
	return waitForRowPinMarker(ctx, clientName, name, true)
}

func clientRowDoesNotShowThePinMarker(ctx context.Context, clientName, name string) error {
	return waitForRowPinMarker(ctx, clientName, name, false)
}

func waitForRowPinMarker(ctx context.Context, clientName, name string, want bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	frame, err := client.WaitForFrameFunc(wait, false, func(frame string) bool {
		return frameSidebarRowHasPinMarker(frame, name) == want
	})
	if err != nil {
		if want {
			return fmt.Errorf("client %q row %q never rendered the pin marker within 5s: %w\nframe:\n%s", clientName, name, err, client.Frame(false))
		}
		return fmt.Errorf("client %q row %q still rendered the pin marker within 5s: %w\nframe:\n%s", clientName, name, err, client.Frame(false))
	}
	_ = frame
	return nil
}

// frameSidebarRowHasPinMarker reports whether frame's sidebar holds a
// session row named name (selected or not) whose line-1 lead carries the
// pin marker -- `✦ ` (or ASCII `* `) sitting between the row's status
// glyph and its name (stripSidebarRowLead's own fixed order, task 006,
// features/status_probe_test.go). It scopes to the sidebar cell exactly
// like frameHasSelectedRowNamed does, so a "✦ name" substring incidentally
// captured in the preview pane's own content can never false-positive.
func frameSidebarRowHasPinMarker(frame, name string) bool {
	if name == "" {
		return false
	}
	for _, line := range strings.Split(frame, "\n") {
		cell, ok := sidebarCell(line)
		if !ok {
			continue
		}
		text := strings.TrimLeft(cell, " ")
		text = strings.TrimPrefix(text, "> ")
		for _, g := range sidebarRowLeadGlyphs {
			rest, ok := strings.CutPrefix(text, g+" ")
			if !ok {
				continue
			}
			for _, pin := range []string{"\u2726 ", "* "} {
				if after, ok := strings.CutPrefix(rest, pin); ok && strings.HasPrefix(after, name) {
					return true
				}
			}
			break
		}
	}
	return false
}
