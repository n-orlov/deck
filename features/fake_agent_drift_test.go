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
		return scenario.bothDocumentFlag("--permission-mode")
	})
	sc.Step(`^the fake Claude permission modes equal the installed Claude permission modes$`, scenario.permissionModesMatch)
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

func (s *fakeAgentDriftScenario) bothDocumentFlag(flag string) error {
	for name, help := range map[string]string{"installed Claude": s.realHelp, "fake Claude": s.fakeHelp} {
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
