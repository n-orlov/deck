package features

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cucumber/godog"
)

type fakeAgentDriftScenario struct {
	realHelp string
	fakeHelp string
}

func registerFakeAgentDriftSteps(sc *godog.ScenarioContext) {
	scenario := &fakeAgentDriftScenario{}

	sc.Step(`^the installed Claude CLI is available$`, scenario.installedClaudeIsAvailable)
	sc.Step(`^I read the installed Claude CLI help$`, scenario.readInstalledHelp)
	sc.Step(`^I read the repository-built fake Claude help$`, scenario.readBuiltFakeHelp)
	sc.Step(`^both help texts document the UUID-valued "--session-id" flag$`, func() error {
		return scenario.bothDocumentUUIDFlag("--session-id")
	})
	sc.Step(`^both help texts document the UUID-valued "--resume" flag$`, func() error {
		return scenario.bothDocumentUUIDFlag("--resume")
	})
	sc.Step(`^both help texts document the "--permission-mode" flag$`, func() error {
		return scenario.bothDocumentFlag("Claude", "--permission-mode")
	})
	sc.Step(`^the fake Claude permission modes equal the installed Claude permission modes$`, scenario.permissionModesMatch)

	registerFakeCodexDriftSteps(sc)
}

func (s *fakeAgentDriftScenario) installedClaudeIsAvailable() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("installed Claude CLI is required for @real-agents: %w", err)
	}
	return nil
}

func (s *fakeAgentDriftScenario) readInstalledHelp() error {
	output, err := exec.Command("claude", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read installed Claude CLI help: %w\n%s", err, output)
	}
	s.realHelp = string(output)
	return nil
}

func (s *fakeAgentDriftScenario) readBuiltFakeHelp() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	temporaryDirectory, err := os.MkdirTemp("", "deck-fake-claude-drift-")
	if err != nil {
		return fmt.Errorf("create temporary fake Claude build directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	binary := filepath.Join(temporaryDirectory, "fake-claude")
	build := exec.Command("go", "build", "-o", binary, "./cmd/fake-claude")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build repository fake Claude: %w\n%s", err, output)
	}
	output, err := exec.Command(binary, "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read repository-built fake Claude help: %w\n%s", err, output)
	}
	s.fakeHelp = string(output)
	return nil
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate repository root containing go.mod")
		}
		directory = parent
	}
}

func (s *fakeAgentDriftScenario) bothDocumentUUIDFlag(flag string) error {
	for name, help := range map[string]string{"installed Claude": s.realHelp, "fake Claude": s.fakeHelp} {
		if !uuidFlagPattern(flag).MatchString(help) {
			return fmt.Errorf("%s help does not document %s with a UUID value", name, flag)
		}
	}
	return nil
}

func (s *fakeAgentDriftScenario) bothDocumentFlag(agentName, flag string) error {
	for name, help := range map[string]string{"installed " + agentName: s.realHelp, "fake " + agentName: s.fakeHelp} {
		if !strings.Contains(help, flag) {
			return fmt.Errorf("%s help does not document %s", name, flag)
		}
	}
	return nil
}

func (s *fakeAgentDriftScenario) permissionModesMatch() error {
	fakeModes, err := documentedPermissionModes(s.fakeHelp)
	if err != nil {
		return fmt.Errorf("parse fake Claude permission modes: %w", err)
	}
	realModes, err := documentedPermissionModes(s.realHelp)
	if err != nil {
		return fmt.Errorf("parse installed Claude permission modes: %w", err)
	}
	if strings.Join(fakeModes, ",") != strings.Join(realModes, ",") {
		return fmt.Errorf("fake Claude permission modes %q do not match installed Claude modes %q", fakeModes, realModes)
	}
	return nil
}

func uuidFlagPattern(flag string) *regexp.Regexp {
	valueDescription := `(?i:uuid)`
	if flag == "--resume" {
		// Current Claude documents this as a conversation "session ID" while
		// --session-id documents that the identifier itself is a UUID.
		valueDescription = `(?i:uuid|session ID)`
	}
	return regexp.MustCompile(regexp.QuoteMeta(flag) + `(?:\s|=|<|\[)[^\n]*` + valueDescription)
}

var permissionModeDeclarationPattern = regexp.MustCompile(`(?im)^\s*--permission-mode\b[^\n]*(?:\n {8,}[^\n]*)*`)
var permissionModeChoicesPattern = regexp.MustCompile(`(?is)\b(?:one of|choices?)\s*:?\s*\(?(.+?)\)?\s*\.?$`)
var permissionModeNamePattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*`)

// documentedPermissionModes reads only the --permission-mode declaration,
// rather than searching all help prose. Thus an added or removed documented
// mode is a drift failure instead of being silently ignored.
func documentedPermissionModes(help string) ([]string, error) {
	declaration := permissionModeDeclarationPattern.FindString(help)
	match := permissionModeChoicesPattern.FindStringSubmatch(declaration)
	if len(match) != 2 {
		return nil, errors.New("--permission-mode declaration does not enumerate its modes")
	}
	modes := permissionModeNamePattern.FindAllString(match[1], -1)
	if len(modes) == 0 {
		return nil, errors.New("--permission-mode declaration has no modes")
	}
	sort.Strings(modes)
	return modes, nil
}

// registerFakeCodexDriftSteps extends fake_agent_drift.feature's existing
// idiom -- Given the installed CLI is available, read both helps, then
// assert facts about them -- to codex (R126; task 022), rather than
// inventing a second one. Only the flag content differs: codex-cli 0.154.0
// documents `-a`/`--ask-for-approval` and `-s`/`--sandbox` as clap enumerated
// options ("[possible values: …]") instead of Claude's `--permission-mode`
// ("one of"/"choices" prose), and has no `--session-id` at all (SPEC §5,
// §8.2; docs/reports/codex-cli-0.154.0-spike.md Q2c/Q2d confirms the exact
// "[possible values: on-request, never]" / "[possible values: read-only,
// workspace-write, danger-full-access]" declarations this drift check keys
// on). It shares fakeAgentDriftScenario and its build/read helpers with the
// Claude scenario, giving each scenario its own instance (godog calls
// initializeScenario, and so this registration function, fresh before every
// scenario).
func registerFakeCodexDriftSteps(sc *godog.ScenarioContext) {
	scenario := &fakeAgentDriftScenario{}

	sc.Step(`^the installed Codex CLI is available$`, scenario.installedCodexIsAvailable)
	sc.Step(`^I read the installed Codex CLI help$`, scenario.readInstalledCodexHelp)
	sc.Step(`^I read the repository-built fake Codex help$`, scenario.readBuiltFakeCodexHelp)
	sc.Step(`^both help texts document the "([^"]+)" option$`, func(option string) error {
		return scenario.bothDocumentFlag("Codex", option)
	})
	sc.Step(`^the fake and installed Codex "([^"]+)" values are equal$`, scenario.codexOptionValuesMatch)
	sc.Step(`^the installed Codex help does not document a "([^"]+)" flag$`, scenario.installedHelpDoesNotDocumentFlag)
}

func (s *fakeAgentDriftScenario) installedCodexIsAvailable() error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("installed Codex CLI is required for @real-agents: %w", err)
	}
	return nil
}

func (s *fakeAgentDriftScenario) readInstalledCodexHelp() error {
	output, err := exec.Command("codex", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read installed Codex CLI help: %w\n%s", err, output)
	}
	s.realHelp = string(output)
	return nil
}

// readBuiltFakeCodexHelp is readBuiltFakeHelp's codex counterpart: same
// temp-dir build-then-run, from ./cmd/fake-codex instead of ./cmd/fake-claude.
func (s *fakeAgentDriftScenario) readBuiltFakeCodexHelp() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	temporaryDirectory, err := os.MkdirTemp("", "deck-fake-codex-drift-")
	if err != nil {
		return fmt.Errorf("create temporary fake Codex build directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	binary := filepath.Join(temporaryDirectory, "fake-codex")
	build := exec.Command("go", "build", "-o", binary, "./cmd/fake-codex")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build repository fake Codex: %w\n%s", err, output)
	}
	output, err := exec.Command(binary, "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read repository-built fake Codex help: %w\n%s", err, output)
	}
	s.fakeHelp = string(output)
	return nil
}

// codexOptionValuesMatch parses option's enumerated value list out of both
// help texts (via documentedFlagValues) and compares them, mirroring
// permissionModesMatch's shape for codex's `-a`/`-s` options.
func (s *fakeAgentDriftScenario) codexOptionValuesMatch(option string) error {
	fakeValues, err := documentedFlagValues(s.fakeHelp, option)
	if err != nil {
		return fmt.Errorf("parse fake Codex %s values: %w", option, err)
	}
	realValues, err := documentedFlagValues(s.realHelp, option)
	if err != nil {
		return fmt.Errorf("parse installed Codex %s values: %w", option, err)
	}
	if strings.Join(fakeValues, ",") != strings.Join(realValues, ",") {
		return fmt.Errorf("fake Codex %s values %q do not match installed Codex %s values %q", option, fakeValues, option, realValues)
	}
	return nil
}

// installedHelpDoesNotDocumentFlag guards the other direction of R126's
// contract: codex-cli 0.154.0 has no --session-id flag at all (deck must
// never pass one -- SPEC §5), so if the installed CLI ever grows one this
// fails loudly instead of the fixture silently going stale by omission.
func (s *fakeAgentDriftScenario) installedHelpDoesNotDocumentFlag(flag string) error {
	if strings.Contains(s.realHelp, flag) {
		return fmt.Errorf("installed Codex help documents %q, which codex-cli 0.154.0 does not have", flag)
	}
	return nil
}

// flagDeclarationPattern is permissionModeDeclarationPattern generalized to
// any flag: the line(s) containing flag, plus any indented continuation.
func flagDeclarationPattern(flag string) *regexp.Regexp {
	return regexp.MustCompile(`(?im)^[^\n]*` + regexp.QuoteMeta(flag) + `\b[^\n]*(?:\n {8,}[^\n]*)*`)
}

// flagValuesPattern is permissionModeChoicesPattern generalized to also
// recognise clap's "[possible values: …]" phrasing (codex-cli), alongside
// the "one of"/"choices" prose Claude's own help already used.
var flagValuesPattern = regexp.MustCompile(`(?is)\b(?:one of|choices?|possible values)\s*:?\s*\(?\[?(.+?)\]?\)?\s*\.?$`)

// documentedFlagValues is documentedPermissionModes generalized to any
// enumerated-value flag, reading only that flag's own declaration rather
// than searching all help prose, so an added or removed documented value is
// a drift failure instead of being silently ignored.
func documentedFlagValues(help, flag string) ([]string, error) {
	declaration := flagDeclarationPattern(flag).FindString(help)
	if declaration == "" {
		return nil, fmt.Errorf("%s declaration not found", flag)
	}
	match := flagValuesPattern.FindStringSubmatch(declaration)
	if len(match) != 2 {
		return nil, fmt.Errorf("%s declaration does not enumerate its values", flag)
	}
	values := permissionModeNamePattern.FindAllString(match[1], -1)
	if len(values) == 0 {
		return nil, fmt.Errorf("%s declaration has no values", flag)
	}
	sort.Strings(values)
	return values, nil
}
