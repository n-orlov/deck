package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/racebuild"
)

// scenarioConfigPollInterval is the shortened config/theme poll interval the
// reload scenarios hand their clients through DECK_CONFIG_POLL_MS, the
// test-only override of SPEC §13.1 that production never sets (the default
// stays 30 s; no bound is raised anywhere).
const scenarioConfigPollInterval = 200 * time.Millisecond

func registerConfigReloadSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with colour enabled and a shortened config poll interval$`, startNamedClientWithShortConfigPoll)
	sc.Step(`^within one shortened config poll interval deck client "([^"]+)" text "([^"]+)" has foreground "(#[0-9a-fA-F]{6})"$`, textHasForegroundWithinConfigPoll)
}

func startNamedClientWithShortConfigPoll(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=", fmt.Sprintf("DECK_CONFIG_POLL_MS=%d", scenarioConfigPollInterval.Milliseconds()))
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// textHasForegroundWithinConfigPoll retries the per-cell colour assertion
// until one shortened poll interval (plus the same render allowance the
// reconcile-interval steps get) has passed.
func textHasForegroundWithinConfigPoll(ctx context.Context, name, text, want string) error {
	budget := scenarioConfigPollInterval + 250*time.Millisecond
	if racebuild.Enabled {
		budget = scenarioConfigPollInterval + 20*time.Second
	}
	deadline := time.Now().Add(budget)
	for {
		err := textHasForeground(ctx, name, text, want)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("within %s: %w", budget, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
