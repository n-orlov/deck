package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/config"
)

// registerConfigFileSteps exposes requirement 5's contract: a scenario can
// write a named config.toml into its own scenario root before a client
// starts, and later assert the file's PARSED content -- through
// internal/config's own parser, never a substring grep on the raw bytes --
// after the run. The scenario root doubles as DECK_HOME for every client the
// harness starts (see ScenarioHarness.Environment), which is where
// internal/config resolves config.toml when DECK_HOME is set (the same
// resolution a real installation performs for $XDG_CONFIG_HOME/deck when
// DECK_HOME is unset), so this is the one location a scenario's clients and
// a scenario's file-content assertions agree on.
func registerConfigFileSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario's config\.toml is written with:$`, writeScenarioConfigTOML)
	sc.Step(`^the scenario's config\.toml parses with allow_yolo (true|false)$`, assertConfigAllowYolo)
	sc.Step(`^the scenario's config\.toml does not parse with allow_yolo (true|false)$`, assertConfigNotAllowYolo)
	sc.Step(`^the scenario's config\.toml parses with stale_after "([^"]+)"$`, assertConfigStaleAfter)
	sc.Step(`^the scenario's config\.toml does not parse with stale_after "([^"]+)"$`, assertConfigNotStaleAfter)
	sc.Step(`^the scenario's config\.toml parses with mouse (true|false)$`, assertConfigMouse)
	sc.Step(`^the scenario's config\.toml does not parse with mouse (true|false)$`, assertConfigNotMouse)
	sc.Step(`^the scenario's config\.toml parses with env "([^"]*)" set to "([^"]*)"$`, assertConfigEnv)
	sc.Step(`^the scenario's config\.toml does not parse with env "([^"]*)" set to "([^"]*)"$`, assertConfigNotEnv)
}

// writeScenarioConfigTOML writes doc's raw content verbatim (no templating,
// no quoting help) to the scenario's config.toml, before any client that is
// meant to observe it has started. A trailing newline is added only if the
// docstring did not already end in one, so a scenario's TOML never has its
// last line accidentally joined with EOF.
func writeScenarioConfigTOML(ctx context.Context, doc *godog.DocString) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	content := doc.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	path := filepath.Join(h.Home, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml: %w", err)
	}
	return nil
}

// parseScenarioConfig re-reads and parses the scenario's config.toml through
// internal/config.LoadFrom exactly as a real deck process would, pointed at
// the scenario's own DECK_HOME and nothing else -- never a text search over
// the file's bytes. It is deliberately callable at any point in a scenario,
// including after a client has run and possibly rewritten the file itself
// (the "assert ... after the run" half of requirement 5).
func parseScenarioConfig(h *ScenarioHarness) (config.Settings, error) {
	getenv := func(key string) string {
		if key == "DECK_HOME" {
			return h.Home
		}
		return ""
	}
	settings, err := config.LoadFrom(getenv, os.UserHomeDir)
	if err != nil {
		return config.Settings{}, fmt.Errorf("parse scenario config.toml: %w", err)
	}
	return settings, nil
}

func assertConfigAllowYolo(ctx context.Context, wantRaw string) error {
	return checkConfigAllowYolo(ctx, wantRaw, true)
}

func assertConfigNotAllowYolo(ctx context.Context, wantRaw string) error {
	return checkConfigAllowYolo(ctx, wantRaw, false)
}

// checkConfigAllowYolo backs both the positive and the negative allow_yolo
// step. want is what the step names; positive asks for equality with the
// parsed value, negative -- requirement 5's proof that the assertion is
// discriminating, in the same "does not have"/"does not fit" shape as tasks
// 001 and 002 -- asks for inequality.
func checkConfigAllowYolo(ctx context.Context, wantRaw string, positive bool) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, err := strconv.ParseBool(wantRaw)
	if err != nil {
		return fmt.Errorf("parse expected allow_yolo %q: %w", wantRaw, err)
	}
	settings, err := parseScenarioConfig(h)
	if err != nil {
		return err
	}
	matches := settings.AllowYolo == want
	if positive && !matches {
		return fmt.Errorf("scenario config.toml parses with allow_yolo=%v, want %v", settings.AllowYolo, want)
	}
	if !positive && matches {
		return fmt.Errorf("scenario config.toml parses with allow_yolo=%v, want it not to match %v", settings.AllowYolo, want)
	}
	return nil
}

func assertConfigStaleAfter(ctx context.Context, wantRaw string) error {
	return checkConfigStaleAfter(ctx, wantRaw, true)
}

func assertConfigNotStaleAfter(ctx context.Context, wantRaw string) error {
	return checkConfigStaleAfter(ctx, wantRaw, false)
}

func checkConfigStaleAfter(ctx context.Context, wantRaw string, positive bool) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, err := time.ParseDuration(wantRaw)
	if err != nil {
		return fmt.Errorf("parse expected stale_after %q: %w", wantRaw, err)
	}
	settings, err := parseScenarioConfig(h)
	if err != nil {
		return err
	}
	matches := settings.StaleAfter == want
	if positive && !matches {
		return fmt.Errorf("scenario config.toml parses with stale_after=%s, want %s", settings.StaleAfter, want)
	}
	if !positive && matches {
		return fmt.Errorf("scenario config.toml parses with stale_after=%s, want it not to match %s", settings.StaleAfter, want)
	}
	return nil
}

func assertConfigMouse(ctx context.Context, wantRaw string) error {
	return checkConfigMouse(ctx, wantRaw, true)
}

func assertConfigNotMouse(ctx context.Context, wantRaw string) error {
	return checkConfigMouse(ctx, wantRaw, false)
}

func checkConfigMouse(ctx context.Context, wantRaw string, positive bool) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, err := strconv.ParseBool(wantRaw)
	if err != nil {
		return fmt.Errorf("parse expected mouse %q: %w", wantRaw, err)
	}
	settings, err := parseScenarioConfig(h)
	if err != nil {
		return err
	}
	matches := settings.Mouse == want
	if positive && !matches {
		return fmt.Errorf("scenario config.toml parses with mouse=%v, want %v", settings.Mouse, want)
	}
	if !positive && matches {
		return fmt.Errorf("scenario config.toml parses with mouse=%v, want it not to match %v", settings.Mouse, want)
	}
	return nil
}

func assertConfigEnv(ctx context.Context, key, want string) error {
	return checkConfigEnv(ctx, key, want, true)
}

func assertConfigNotEnv(ctx context.Context, key, want string) error {
	return checkConfigEnv(ctx, key, want, false)
}

func checkConfigEnv(ctx context.Context, key, want string, positive bool) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	settings, err := parseScenarioConfig(h)
	if err != nil {
		return err
	}
	got, present := settings.Env[key]
	matches := present && got == want
	if positive && !matches {
		return fmt.Errorf("scenario config.toml [env] %q = %q (present=%v), want %q", key, got, present, want)
	}
	if !positive && matches {
		return fmt.Errorf("scenario config.toml [env] %q = %q, want it not to match %q", key, got, want)
	}
	return nil
}
