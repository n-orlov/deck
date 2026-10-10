package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	sc.Step(`^the scenario's profile "([^"]+)" config\.toml selects theme "([^"]*)"$`, scenarioProfileConfigSelectsTheme)
	sc.Step(`^deck client "([^"]+)" is started on profile "([^"]+)" with colour enabled and a shortened config poll interval$`, startNamedProfileClientWithShortConfigPoll)
}

// scenarioProfileConfigSelectsTheme writes the named profile's own
// config.toml, which lives at <DECK_HOME>/profiles/<profile>/config.toml
// (SPEC §3.4), and selects the theme in it. The directory is created the
// way a real profile's is, so the typo guard has nothing to ask.
func scenarioProfileConfigSelectsTheme(ctx context.Context, profile, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "profiles", profile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create profile %q directory: %w", profile, err)
	}
	content := fmt.Sprintf("[ui]\ntheme = %q\n", name)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write profile %q config.toml selecting theme %q: %w", profile, name, err)
	}
	return nil
}

// startNamedProfileClientWithShortConfigPoll starts one more real deck
// process of an existing named profile on the scenario's shared DECK_HOME
// and private tmux socket, with DECK_CONFIG_POLL_MS as the only shortening.
func startNamedProfileClientWithShortConfigPoll(ctx context.Context, name, profile string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=", "DECK_PROFILE="+profile, fmt.Sprintf("DECK_CONFIG_POLL_MS=%d", scenarioConfigPollInterval.Milliseconds()))
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
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
