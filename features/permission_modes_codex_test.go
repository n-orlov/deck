package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerPermissionModesCodexSteps registers the two keystroke steps
// features/permission_modes.feature's codex degradation scenario needs and
// no other feature file has (task 025, R127): opening the create modal on
// one agent with an explicitly chosen profile WITHOUT submitting, and then
// cycling the modal's Agent field onto another agent while that choice is
// still showing. Together they are the only reachable route through the
// released TUI to a permission profile the selected adapter does not
// declare -- every other surface narrows the offer to the adapter's own
// declared set first (internal/tui.createProfileOptionsFor,
// service.SetPermissionProfile), which is exactly why this scenario has to
// come in through the create modal rather than through `P`.
func registerPermissionModesCodexSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the create modal on agent "([^"]+)" for session "([^"]+)" with permission profile "([^"]+)"$`, clientOpensCreateModalWithProfileUnsubmitted)
	sc.Step(`^deck client "([^"]+)" cycles the create modal's Agent field to "([^"]+)"$`, clientCyclesCreateModalAgentField)
}

// clientOpensCreateModalWithProfileUnsubmitted drives exactly the keystrokes
// clientCreatesAgentSessionWithProfile drives (positionCreateModalOnProfileField:
// open, name, cwd, Agent, cycle Permission profile) and then stops, leaving
// the modal open with the cursor still on the Permission profile field. It
// deliberately does NOT press Enter: this scenario's whole point is what the
// modal does between choosing a profile and submitting.
func clientOpensCreateModalWithProfileUnsubmitted(ctx context.Context, clientName, kind, name, profile string) error {
	_, _, err := positionCreateModalOnProfileField(ctx, clientName, kind, name, profile)
	return err
}

// clientCyclesCreateModalAgentField moves up from the Permission profile
// field onto Agent (↑/↓ move between fields, task 025) and cycles it right
// until it reads want, then returns with the cursor left on Agent. It reads
// the Agent row through dewrapCreateModalAgentRow for the same reason
// ensureCreateModalAgent does: the dialog box's own word-wrap can split that
// row's "(left/right cycles: ...)" hint at any column.
//
// Unlike ensureCreateModalAgent this never returns to the Name field and
// never re-types anything: the modal's other fields are already filled by
// clientOpensCreateModalWithProfileUnsubmitted, and the Permission profile
// field's value is precisely what this step is about to disturb.
func clientCyclesCreateModalAgentField(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b[A"); err != nil { // Permission profile -> Agent
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if err := client.WaitForFrame(ctx, false, "> Agent:"); err != nil {
		return fmt.Errorf("move onto the create modal's Agent field: %w", err)
	}
	marker := "Agent: " + want + " (left/right cycles"
	matchesMarker := func(frame string) bool {
		return strings.Contains(dewrapCreateModalAgentRow(frame), marker)
	}
	for attempt := 0; attempt <= len(createAgentOptionsOrder); attempt++ {
		if matchesMarker(client.Frame(false)) {
			break
		}
		if err := client.Send("\x1b[C"); err != nil { // right arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := client.WaitForFrameFunc(ctx, false, matchesMarker); err != nil {
		return fmt.Errorf("cycle the create modal's Agent field to %q: %w", want, err)
	}
	return nil
}
