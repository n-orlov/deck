package features

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

// registerSettingsSteps backs features/settings.feature (task 020,
// requirement 48): the `,` full-screen settings takeover. Every other step
// the scenarios need already exists (sendClientKeys drives the takeover's
// raw keymap exactly as an interactive user would, clientScreenContains/
// clientScreenDoesNotContain read its rendered labels/values, and
// registerConfigFileSteps/scenarioConfigTOMLIsCapturedAs already prove the
// atomic writer's PARSED output and the discard-prompt's "leaves the file
// unchanged" guarantee) -- this file adds only the one assertion missing
// for requirement 24's "driving every key the takeover binds leaves the
// session set untouched" proof: a durable count of exactly how many
// sessions exist, so a settings-only session that neither creates nor
// deletes a row has something stronger than "the two sessions I already
// know the names of are still there" to stand on.
func registerSettingsSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the state database has exactly (\d+) sessions?$`, stateDatabaseHasExactlySessionCount)
}

func stateDatabaseHasExactlySessionCount(ctx context.Context, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&got); err != nil {
		return fmt.Errorf("count sessions: %w", err)
	}
	if got != want {
		return fmt.Errorf("state database has %d sessions, want exactly %d", got, want)
	}
	return nil
}
