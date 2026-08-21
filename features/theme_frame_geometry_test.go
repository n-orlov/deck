package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerThemeFrameGeometrySteps exposes requirement 49's real-client
// counterpart of features/theme_geometry_test.go (task 027): that file
// proves, entirely in-process against Model.View(), that switching a
// built-in theme changes attributes but never the frame's cell
// positions/characters. This step proves the same thing end-to-end,
// through two independent released deck client processes each started
// under its own [ui] theme selection, by comparing ScreenDriver.Frame's
// plain cell-grid text (deliberately NOT the raw escape stream --
// Frame's own doc comment: "cell-grid text, not the terminal's escape
// stream") between them. Frame carries no colour information at all, so
// an exact match here is a genuine geometry/content proof, not a
// coincidence of two clients happening to render the same colours.
func registerThemeFrameGeometrySteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" screen text matches deck client "([^"]+)" screen text$`, clientScreenTextMatchesOtherClientScreenText)
}

func clientScreenTextMatchesOtherClientScreenText(ctx context.Context, nameA, nameB string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	a, err := h.Client(nameA)
	if err != nil {
		return err
	}
	b, err := h.Client(nameB)
	if err != nil {
		return err
	}
	// Give each client's most recent render a moment to settle, matching
	// clientCapturesFrameAs' own pacing rationale, so this compares two
	// steady-state frames rather than racing a still-in-flight repaint.
	time.Sleep(50 * time.Millisecond)
	frameA := a.Frame(false)
	frameB := b.Frame(false)
	if frameA != frameB {
		return fmt.Errorf("deck client %q screen text does not match deck client %q screen text -- a theme change must alter colour only, never geometry or content:\n%q:\n%s\n%q:\n%s", nameA, nameB, nameA, frameA, nameB, frameB)
	}
	return nil
}
