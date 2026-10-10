package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	sc.Step(`^the scenario runs on the real-install XDG layout with DECK_HOME unset and its config\.toml selects theme "([^"]*)"$`, scenarioRunsOnRealInstallXDGLayout)
	sc.Step(`^the scenario's clients do not pin DECK_ASCII$`, scenarioClientsDoNotPinASCII)
	sc.Step(`^within one shortened config poll interval deck client "([^"]+)" screen contains "([^"]+)"$`, screenContainsWithinConfigPoll)
	sc.Step(`^within one shortened config poll interval deck client "([^"]+)" screen does not contain "([^"]+)"$`, screenDoesNotContainWithinConfigPoll)
	sc.Step(`^within one shortened config poll interval deck client "([^"]+)" raw output enabled SGR mouse reporting$`, rawOutputEnabledMouseReportingWithinConfigPoll)
	sc.Step(`^within one shortened config poll interval deck client "([^"]+)" screen shows sessions in this order:$`, sessionsInOrderWithinConfigPoll)
	sc.Step(`^deck client "([^"]+)" is started on profile "([^"]+)" with colour enabled and a shortened config poll interval$`, startNamedProfileClientWithShortConfigPoll)
}

// scenarioRunsOnRealInstallXDGLayout moves every client started afterwards
// onto the layout a real install uses: DECK_HOME unset, HOME a temp home, and
// XDG_CONFIG_HOME / XDG_DATA_HOME / XDG_STATE_HOME pointing into the scenario
// root, so config.toml resolves to $XDG_CONFIG_HOME/deck/config.toml (SPEC
// §3.4) instead of the DECK_HOME shortcut every other scenario uses. The
// default profile's config.toml is written there selecting the theme.
func scenarioRunsOnRealInstallXDGLayout(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	root := filepath.Join(h.Home, "xdg-install")
	configHome := filepath.Join(root, "config")
	dir := filepath.Join(configHome, "deck")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create XDG config directory: %w", err)
	}
	content := fmt.Sprintf("[ui]\ntheme = %q\n", name)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write XDG config.toml selecting theme %q: %w", name, err)
	}
	h.clientEnv = append(h.clientEnv,
		"DECK_HOME=", "HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_STATE_HOME="+filepath.Join(root, "state"))
	return nil
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
	return client.WaitForFrame(ctx, false, h.sessionsTitle())
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
	return client.WaitForFrame(ctx, false, h.sessionsTitle())
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

// configPollBudget is one shortened poll interval plus the render allowance
// the other poll steps get; nothing here raises a bound.
func configPollBudget() time.Duration {
	if racebuild.Enabled {
		return scenarioConfigPollInterval + 20*time.Second
	}
	return scenarioConfigPollInterval + 250*time.Millisecond
}

// withinConfigPoll retries check until it passes or one shortened poll
// interval (plus the render allowance) has passed.
func withinConfigPoll(ctx context.Context, check func() error) error {
	budget := configPollBudget()
	deadline := time.Now().Add(budget)
	for {
		err := check()
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

// scenarioClientsDoNotPinASCII empties DECK_ASCII (the harness pins it to 1
// for every client) so [ui] ascii in the file is what the clients run with.
func scenarioClientsDoNotPinASCII(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_ASCII=")
	return nil
}

func screenContainsWithinConfigPoll(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return withinConfigPoll(ctx, func() error {
		frame := client.Frame(false)
		if !strings.Contains(frame, want) {
			return fmt.Errorf("deck client %q screen does not contain %q:\n%s", name, want, frame)
		}
		return nil
	})
}

func screenDoesNotContainWithinConfigPoll(ctx context.Context, name, unwanted string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return withinConfigPoll(ctx, func() error {
		frame := client.Frame(false)
		if strings.Contains(frame, unwanted) {
			return fmt.Errorf("deck client %q screen still contains %q:\n%s", name, unwanted, frame)
		}
		return nil
	})
}

func rawOutputEnabledMouseReportingWithinConfigPoll(ctx context.Context, name string) error {
	return withinConfigPoll(ctx, func() error { return clientRawOutputEnabledMouseReporting(ctx, name) })
}

func sessionsInOrderWithinConfigPoll(ctx context.Context, name string, table *godog.Table) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return withinConfigPoll(ctx, func() error { return sessionsRenderInOrder(client, table) })
}

// sessionsTitle is the main view's title as the clients of this scenario
// draw it: the harness pins DECK_ASCII=1, which draws a hyphen, and a
// scenario that releases the pin (scenarioClientsDoNotPinASCII) with
// [ui] ascii off gets the em dash instead.
func (h *ScenarioHarness) sessionsTitle() string {
	for _, e := range h.clientEnv {
		if e == "DECK_ASCII=" {
			return "deck \u2014 sessions"
		}
	}
	return "deck - sessions"
}
