package features

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type fakeAgentScenario struct {
	binary       string
	successName  string
	failureName  string
	expectedArgv string
}

func registerFakeAgentFeatureSteps(sc *godog.ScenarioContext) {
	scenario := &fakeAgentScenario{
		successName:  "deck_fake-agent-success",
		failureName:  "deck_fake-agent-failure",
		expectedArgv: `fake-claude argv: ["--session-id","123e4567-e89b-12d3-a456-426614174000","--resume","123e4567-e89b-12d3-a456-426614174001","--permission-mode","acceptEdits","write feature coverage"]`,
	}
	sc.Step(`^the repository-built fake Claude fixture is ready$`, scenario.buildFixture)
	sc.Step(`^the fake Claude fixture is launched successfully as a private tmux session command$`, scenario.launchSuccess)
	sc.Step(`^its success pane shows the deterministic banner and exact accepted argv$`, scenario.successOutput)
	sc.Step(`^the successful fake Claude session exits with status 0$`, scenario.successExited)
	sc.Step(`^the fake Claude fixture is launched with controlled failure status ([0-9]+)$`, scenario.launchFailure)
	sc.Step(`^its failure pane shows the deterministic banner and exact accepted argv$`, scenario.failureOutput)
	sc.Step(`^the failed fake Claude session remains with status ([0-9]+)$`, scenario.failureStatus)
}

func (s *fakeAgentScenario) buildFixture(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	s.binary = filepath.Join(h.Home, "fake-claude")
	build := exec.CommandContext(ctx, "go", "build", "-o", s.binary, "./cmd/fake-claude")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build repository fake Claude fixture: %w\n%s", err, output)
	}
	return nil
}

func (s *fakeAgentScenario) launchSuccess(ctx context.Context) error {
	return s.launch(ctx, s.successName, 0)
}

func (s *fakeAgentScenario) launchFailure(ctx context.Context, status int) error {
	return s.launch(ctx, s.failureName, status)
}

func (s *fakeAgentScenario) launch(ctx context.Context, session string, status int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if s.binary == "" {
		return fmt.Errorf("fake Claude fixture was not built")
	}
	// This mirrors deck's private-server contract, but launches the fixture as
	// the pane command through the public tmux CLI. No product internals or
	// fake-only deck control path participates in the observation.
	//
	// Deck's real SPEC §3.2 contract only retains panes that exit with a
	// non-zero status ("remain-on-exit failed"); a clean-exit pane is torn
	// down by tmux immediately. This fixture instead pins "remain-on-exit on"
	// so that a clean-exit success pane also survives long enough for a
	// black-box capture-pane/list-panes read, however slow the CI host is.
	// That is a fixture-local departure from deck's contract, made solely so
	// this test can observe pane text and exit status deterministically
	// without racing a fixed-duration capture window; it says nothing about
	// deck's own remain-on-exit behavior, which is exercised elsewhere.
	if _, err := tmuxOutput(ctx, h,
		"start-server", ";",
		"set-option", "-s", "exit-empty", "off", ";",
		"set-option", "-g", "remain-on-exit", "on", ";",
		"set-option", "-g", "window-size", "latest", ";",
		"set-window-option", "-g", "aggressive-resize", "on",
	); err != nil {
		return fmt.Errorf("bootstrap private tmux server: %w", err)
	}
	args := []string{"new-session", "-d", "-s", session, "-c", h.Home, "--", "env",
		"FAKE_CLAUDE_EXIT_CODE=" + fmt.Sprint(status),
		s.binary,
		"--session-id", "123e4567-e89b-12d3-a456-426614174000",
		"--resume", "123e4567-e89b-12d3-a456-426614174001",
		"--permission-mode", "acceptEdits", "write feature coverage"}
	if _, err := tmuxOutput(ctx, h, args...); err != nil {
		return fmt.Errorf("launch fake Claude as pane command: %w", err)
	}
	return nil
}

func (s *fakeAgentScenario) successOutput(ctx context.Context) error {
	return s.outputContains(ctx, s.successName)
}

func (s *fakeAgentScenario) failureOutput(ctx context.Context) error {
	return s.outputContains(ctx, s.failureName)
}

func (s *fakeAgentScenario) outputContains(ctx context.Context, session string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	// The scenario asserts pane content immediately after launching the
	// fixture, with no guarantee the fixture process has run far enough to
	// have written its banner and argv record yet (it may not even have
	// been scheduled). Poll capture-pane's full scrollback (-S -, not only
	// the currently visible screen) until the expected text appears or a
	// deadline passes, instead of trusting a single immediate read; without
	// a hold delaying the fixture's exit, tmux's own "Pane is dead" status
	// line can also be appended to an already-full screen and scroll the
	// banner's first line out of view, but the scrollback still has it.
	deadline := time.Now().Add(3 * time.Second)
	var text string
	for {
		output, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", session)
		if err != nil {
			return fmt.Errorf("capture fake Claude pane %q: %w", session, err)
		}
		text = string(output)
		if outputHasFakeClaudeContent(text, s.expectedArgv) {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, want := range []string{"Fake Claude Code", "fake-claude permission-mode: acceptEdits"} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("fake Claude pane %q does not contain %q:\n%s", session, want, text)
		}
	}
	return fmt.Errorf("fake Claude pane %q does not contain exact accepted argv %q:\n%s", session, s.expectedArgv, text)
}

// outputHasFakeClaudeContent reports whether text contains both the
// deterministic banner/permission-mode lines and the exact accepted argv
// record. capture-pane preserves terminal soft-wraps, so only the newlines
// are stripped before matching the argv, recovering the exact record emitted
// by the fixture without relying on a product-internal channel or a
// width-specific pane layout.
func outputHasFakeClaudeContent(text, expectedArgv string) bool {
	for _, want := range []string{"Fake Claude Code", "fake-claude permission-mode: acceptEdits"} {
		if !strings.Contains(text, want) {
			return false
		}
	}
	return strings.Contains(strings.ReplaceAll(text, "\n", ""), expectedArgv)
}

func (s *fakeAgentScenario) successExited(ctx context.Context) error {
	return s.waitForPaneDeadStatus(ctx, s.successName, 0)
}

func (s *fakeAgentScenario) failureStatus(ctx context.Context, want int) error {
	return s.waitForPaneDeadStatus(ctx, s.failureName, want)
}

// waitForPaneDeadStatus polls the pane's own public tmux-reported state
// (#{pane_dead} and #{pane_dead_status}) until the fixture process has
// exited with the wanted status, rather than trusting a fixed-duration
// sleep to have outrun the fixture. Because the harness pins fixture-local
// remain-on-exit to "on" (see launch), the pane is retained regardless of
// whether the fixture's exit was clean or controlled, so this same poll
// serves both the success and failure cases.
func (s *fakeAgentScenario) waitForPaneDeadStatus(ctx context.Context, session string, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		output, err := tmuxOutput(ctx, h, "list-panes", "-t", session, "-F", "#{pane_dead}|#{pane_dead_status}")
		if err == nil && strings.TrimSpace(string(output)) == fmt.Sprintf("1|%d", want) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("fake Claude session %q did not reach exit status %d", session, want)
}
