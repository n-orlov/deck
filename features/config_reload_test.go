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
	sc.Step(`^the scenario's profile "([^"]+)" config\.toml gets an unrelated edit$`, scenarioProfileConfigGetsUnrelatedEdit)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" keeps foreground "(#[0-9a-fA-F]{6})" for (\d+) further shortened config poll intervals$`, textKeepsForegroundForConfigPolls)
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

// scenarioProfileConfigGetsUnrelatedEdit appends a comment to the named
// profile's config.toml: the file's mtime and size move, so that profile's
// reload poll re-reads it, while no setting changes.
func scenarioProfileConfigGetsUnrelatedEdit(ctx context.Context, profile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "profiles", profile, "config.toml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open profile %q config.toml for an unrelated edit: %w", profile, err)
	}
	if _, err := f.WriteString("# unrelated edit\n"); err != nil {
		_ = f.Close()
		return fmt.Errorf("append to profile %q config.toml: %w", profile, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close profile %q config.toml: %w", profile, err)
	}
	return nil
}

// textKeepsForegroundForConfigPolls asserts the colour holds at every check
// across n further poll intervals, so a client whose poll reloaded the wrong
// profile cannot slip through between two samples.
func textKeepsForegroundForConfigPolls(ctx context.Context, name, text, want string, polls int) error {
	if polls < 1 {
		return fmt.Errorf("at least one further poll interval is required, got %d", polls)
	}
	window := time.Duration(polls) * scenarioConfigPollInterval
	deadline := time.Now().Add(window)
	for {
		if err := textHasForeground(ctx, name, text, want); err != nil {
			return fmt.Errorf("within %d further poll intervals (%s): %w", polls, window, err)
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
