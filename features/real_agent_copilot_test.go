package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

// R223: the @real-agents copilot scenarios in real_agent_copilot.feature run
// against an installed GitHub Copilot CLI with a temporary COPILOT_HOME, so no
// operator state (~/.copilot) is read or written. Like every other
// @real-agents step they are registered in every run (the feature is parsed
// strictly) and the default tag filter keeps them from starting a real CLI.
//
// Upstream behaviour is asserted without aliases or coercion: an incompatible
// Copilot upgrade is a visible conformance failure. The one deliberate
// exception is the Given step, which SKIPS (never fails) when the CLI is
// absent or cannot answer a prompt, and which states the reason without ever
// printing a credential.

const (
	realCopilotProbePrompt = "Reply with OK only."
	// realCopilotLoginProbeTimeout bounds the one non-interactive prompt that
	// decides whether the CLI is logged in. It is a probe of the host, not a
	// wait inside a scenario.
	realCopilotLoginProbeTimeout = 120 * time.Second
	realCopilotPaneTimeout       = 150 * time.Second
	realCopilotHookTimeout       = 150 * time.Second
)

// copilotFixtureNeed is one key substring of a probe fixture: any one of the
// alternatives satisfies it. Each must appear in the recorded fixture
// (internal/agent/testdata/probes/copilot, see its PROVENANCE) and in a live
// pane of the installed CLI, which is the drift this scenario exists to catch.
type copilotFixtureNeed []string

// realCopilotFixtureKeys are the six probe fixtures R219 fits its rules to and
// the substrings of each that the pane probe keys on, in R219's precedence
// order. They mirror internal/agent's copilotProbeRules, which this package
// never imports; a unit test pins them to the fixture files.
var realCopilotFixtureKeys = map[string][]copilotFixtureNeed{
	"trust":      {{"Confirm folder trust"}, {"Do you trust the files in this folder?"}},
	"permission": {{"Do you want to "}, {"↑/↓ to navigate · enter to select · esc to cancel"}},
	"question":   {{"Copilot needs information."}},
	"working":    {{"Working"}, {"esc interrupt", "esc edit prompt"}},
	"error":      {{"✗ "}},
	"idle":       {{"· / commands", "@ files · # issues"}},
}

// realCopilotFixtureFiles names each fixture's file under the probes dir.
var realCopilotFixtureFiles = map[string]string{
	"trust": "trust.txt", "permission": "permission.txt", "question": "question.txt",
	"working": "working.txt", "error": "error.txt", "idle": "idle.txt",
}

// copilotPaneSatisfies reports whether pane carries every need of the fixture.
func copilotPaneSatisfies(pane string, needs []copilotFixtureNeed) bool {
	for _, need := range needs {
		found := false
		for _, alternative := range need {
			if strings.Contains(pane, alternative) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// realCopilotSkipReason returns "" when the scenario may run, else the reason
// it is skipped. lookPath resolves the binary and answersPrompt runs the
// logged-in probe against the resolved path; neither the probe's output nor
// any environment value is ever part of the reason.
func realCopilotSkipReason(lookPath func(string) (string, error), answersPrompt func(path string) error) string {
	path, err := lookPath("copilot")
	if err != nil {
		return "skipping real Copilot scenario: no installed copilot CLI on PATH"
	}
	if err := answersPrompt(path); err != nil {
		return "skipping real Copilot scenario: the installed copilot CLI could not answer a prompt, so it is not logged in " +
			"(run `copilot login`, or export COPILOT_GITHUB_TOKEN, GH_TOKEN or GITHUB_TOKEN, before opting in)"
	}
	return ""
}

// copilotAnswersProbePrompt runs one non-interactive prompt with a temporary
// COPILOT_HOME and reports whether the CLI exited 0. Output is discarded.
func copilotAnswersProbePrompt(path string) error {
	home, err := os.MkdirTemp("", "deck-real-copilot-probe-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(home) }()
	ctx, cancel := context.WithTimeout(context.Background(), realCopilotLoginProbeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, "-p", realCopilotProbePrompt, "-s", "--no-auto-update")
	command.Dir = home
	command.Env = append(os.Environ(), "COPILOT_HOME="+home)
	return command.Run()
}

// realCopilotTrustGrace is how long a trust answer is given to repaint before
// a still-visible trust frame is taken to be an unanswered prompt.
const realCopilotTrustGrace = 3 * time.Second

type realCopilotScenario struct {
	home string
	// trustAnswered records, per session name, when the folder-trust prompt was
	// last answered, so the explicit answer step and the readiness wait share
	// one answer instead of each sending its own.
	trustAnswered map[string]time.Time
	// capture and keys replace the tmux pane when set (unit tests only).
	capture func(ctx context.Context, name string) (string, error)
	keys    func(ctx context.Context, name string, args ...string) error
}

func registerRealAgentCopilotSteps(sc *godog.ScenarioContext) {
	scenario := &realCopilotScenario{}
	sc.Step(`^the installed Copilot CLI resolves on PATH and answers a prompt, or this scenario is skipped with a stated reason$`, scenario.installedAndLoggedInOrSkip)
	sc.Step(`^the real Copilot CLI is isolated in a temporary COPILOT_HOME$`, scenario.isolateCopilotHome)
	sc.Step(`^the real Copilot home holds a session directory named by session "([^"]+)"'s conversation id$`, scenario.homeHoldsSessionDirectory)
	sc.Step(`^the real Copilot home holds exactly ([0-9]+) session director(?:y|ies)$`, scenario.homeHoldsSessionDirectories)
	sc.Step(`^session "([^"]+)"'s real Copilot pane shows the "([a-z]+)" probe fixture's key substrings$`, scenario.paneShowsFixture)
	sc.Step(`^session "([^"]+)" answers the real Copilot folder-trust prompt by remembering the folder$`, scenario.answerTrust)
	sc.Step(`^session "([^"]+)" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears$`, scenario.reachIdle)
	sc.Step(`^session "([^"]+)" sends the line "([^"]*)" to its real Copilot pane$`, scenario.sendLine)
	sc.Step(`^session "([^"]+)" cancels the real Copilot dialog with Escape$`, scenario.sendEscape)
	sc.Step(`^session "([^"]+)" receives the real Copilot "([A-Za-z]+)" hook$`, scenario.hookDelivered)
	sc.Step(`^the real Copilot process of session "([^"]+)" is killed with SIGKILL$`, scenario.killProcessGroup)
	sc.Step(`^the audit log's first and most recent launch argv for session "([^"]+)" are identical$`, auditFirstAndLatestLaunchArgvIdentical)
}

func (s *realCopilotScenario) installedAndLoggedInOrSkip(ctx context.Context) error {
	if reason := realCopilotSkipReason(exec.LookPath, copilotAnswersProbePrompt); reason != "" {
		godog.Log(ctx, reason)
		return godog.ErrSkip
	}
	return nil
}

// isolateCopilotHome gives every deck client a COPILOT_HOME under the
// scenario's own DECK_HOME (removed with it). The CLI writes its trusted
// folders and session state there, never under ~/.copilot.
func (s *realCopilotScenario) isolateCopilotHome(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	s.home = filepath.Join(h.Home, "real-copilot-home")
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return fmt.Errorf("create temporary COPILOT_HOME: %w", err)
	}
	h.clientEnv = append(h.clientEnv, "COPILOT_HOME="+s.home)
	return nil
}

func (s *realCopilotScenario) sessionStateDir() (string, error) {
	if s.home == "" {
		return "", errors.New("the real Copilot CLI was not isolated in a temporary COPILOT_HOME")
	}
	return filepath.Join(s.home, "session-state"), nil
}

func (s *realCopilotScenario) homeHoldsSessionDirectory(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("session %q has no conversation id", name)
	}
	root, err := s.sessionStateDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, id)
	return pollUntil(realCopilotPaneTimeout, func() error {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("real copilot launched with --session-id %s created no session directory: %w", id, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		return nil
	})
}

func (s *realCopilotScenario) homeHoldsSessionDirectories(count int) error {
	root, err := s.sessionStateDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read %s: %w", root, err)
	}
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() && copilotSessionDirName.MatchString(entry.Name()) {
			directories = append(directories, entry.Name())
		}
	}
	if len(directories) != count {
		return fmt.Errorf("real Copilot home holds %d session directories %v, want %d: a relaunch must resume the one session, never start another", len(directories), directories, count)
	}
	return nil
}

var copilotSessionDirName = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// pollUntil retries check every 100 ms until it returns nil or timeout passes,
// returning the last error.
func pollUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := check()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *realCopilotScenario) pane(ctx context.Context, name string) (string, string, error) {
	if s.capture != nil {
		pane, err := s.capture(ctx, name)
		return "", pane, err
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return "", "", err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", "", err
	}
	target := "deck_" + slug
	output, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-t", target)
	return target, string(output), err
}

func (s *realCopilotScenario) paneShowsFixture(ctx context.Context, name, fixture string) error {
	needs, ok := realCopilotFixtureKeys[fixture]
	if !ok {
		return fmt.Errorf("unknown probe fixture %q", fixture)
	}
	return s.waitForPane(ctx, name, fmt.Sprintf("the %q probe fixture's key substrings %q", fixture, needs), func(pane string) bool {
		return copilotPaneSatisfies(pane, needs)
	})
}

// waitForPane polls the live pane of session name until accept holds, and
// otherwise fails naming what was expected and the last screen seen.
func (s *realCopilotScenario) waitForPane(ctx context.Context, name, want string, accept func(pane string) bool) error {
	var last string
	var lastErr error
	err := pollUntil(realCopilotPaneTimeout, func() error {
		_, pane, err := s.pane(ctx, name)
		last, lastErr = pane, err
		if err == nil && accept(pane) {
			return nil
		}
		return errors.New("not yet")
	})
	if err != nil {
		detail := "none"
		if lastErr != nil {
			detail = lastErr.Error()
		}
		return fmt.Errorf("session %q's real Copilot pane never showed %s (last capture error: %s):\n%s", name, want, detail, last)
	}
	return nil
}

func (s *realCopilotScenario) sendKeys(ctx context.Context, name string, args ...string) error {
	if s.keys != nil {
		return s.keys(ctx, name, args...)
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	target, _, err := s.pane(ctx, name)
	if err != nil {
		return err
	}
	_, err = tmuxOutput(ctx, h, append([]string{"send-keys", "-t", target}, args...)...)
	return err
}

func (s *realCopilotScenario) answerTrust(ctx context.Context, name string) error {
	// Option 2 is "Yes, and remember this folder for future sessions": the CLI
	// records it in the temporary COPILOT_HOME, so later launches do not ask.
	if err := s.sendKeys(ctx, name, "Down", "Enter"); err != nil {
		return err
	}
	if s.trustAnswered == nil {
		s.trustAnswered = map[string]time.Time{}
	}
	s.trustAnswered[name] = time.Now()
	return nil
}

// trustNeedsAnswer reports whether a visible trust frame is a prompt nobody has
// answered: no answer was sent for the session, or the last one is older than
// the grace. A frame still on screen within the grace is the answer repainting.
func (s *realCopilotScenario) trustNeedsAnswer(name string) bool {
	answered, ok := s.trustAnswered[name]
	return !ok || time.Since(answered) > realCopilotTrustGrace
}

func (s *realCopilotScenario) reachIdle(ctx context.Context, name string) error {
	idle := realCopilotFixtureKeys["idle"]
	trust := realCopilotFixtureKeys["trust"]
	return s.waitForPane(ctx, name, fmt.Sprintf("the idle footer %q", idle), func(pane string) bool {
		if copilotPaneSatisfies(pane, trust) {
			if s.trustNeedsAnswer(name) {
				_ = s.answerTrust(ctx, name)
			}
			return false
		}
		return copilotPaneSatisfies(pane, idle) && !strings.Contains(pane, "Working")
	})
}

func (s *realCopilotScenario) sendLine(ctx context.Context, name, line string) error {
	if err := s.sendKeys(ctx, name, "-l", line); err != nil {
		return err
	}
	// A slash command opens a completion list; Enter sent in the same
	// instant is swallowed by it.
	time.Sleep(500 * time.Millisecond)
	return s.sendKeys(ctx, name, "Enter")
}

func (s *realCopilotScenario) sendEscape(ctx context.Context, name string) error {
	return s.sendKeys(ctx, name, "Escape")
}

// copilotHookEventKinds maps the Copilot hook names a scenario speaks to the
// event kind deck's receiver writes for them (SPEC §8.4).
var copilotHookEventKinds = map[string]string{
	"userPromptSubmitted": "user_prompt_submitted",
	"sessionStart":        "session_start",
	"agentStop":           "stop",
	"sessionEnd":          "session_end",
}

func (s *realCopilotScenario) hookDelivered(ctx context.Context, name, hook string) error {
	kind, ok := copilotHookEventKinds[hook]
	if !ok {
		return fmt.Errorf("unknown Copilot hook %q", hook)
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	return pollUntil(realCopilotHookTimeout, func() error {
		counts, err := copilotEventCounts(ctx, h, sessionID, []string{kind})
		if err != nil {
			return err
		}
		if counts[kind] == 0 {
			return fmt.Errorf("real Copilot never delivered its %s hook to deck for session %q (no %q event)", hook, name, kind)
		}
		return nil
	})
}

// killProcessGroup SIGKILLs the pane's whole process group. The pane pid is
// read from tmux for this one session and verified to lead its own group, so a
// launcher that forks the real binary (as the npm loader does) dies with it.
func (s *realCopilotScenario) killProcessGroup(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "display-message", "-p", "-t", "deck_"+slug, "#{pane_pid}")
	if err != nil {
		return err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid); err != nil || pid <= 1 {
		return fmt.Errorf("parse pane pid for session %q from %q", name, strings.TrimSpace(string(output)))
	}
	target := pid
	if group, err := syscall.Getpgid(pid); err == nil && group == pid {
		target = -pid
	}
	if err := syscall.Kill(target, syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL pane process %d of session %q: %w", pid, name, err)
	}
	return nil
}

func auditFirstAndLatestLaunchArgvIdentical(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	var records [][]string
	if err := pollUntil(5*time.Second, func() error {
		var err error
		records, err = launchArgvRecordsForSession(h, name)
		if err == nil && len(records) < 2 {
			err = fmt.Errorf("audit log has %d launch records for session %q, want a relaunch", len(records), name)
		}
		return err
	}); err != nil {
		return err
	}
	first, latest := records[0], records[len(records)-1]
	if strings.Join(first, "\x00") != strings.Join(latest, "\x00") {
		return fmt.Errorf("session %q relaunched with argv %q, want exactly its first launch's %q (copilot --session-id creates and resumes)", name, latest, first)
	}
	return nil
}
