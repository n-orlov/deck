package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// fakeAgentSpec names a fake-agent fixture's package, the env var that keeps
// it reading pane input (so a scenario has time to resize it before it
// exits), the file requirement 4's recorder appends "COLSxROWS" lines to
// under DECK_HOME, and the env var that switches it into requirement 5's
// render-then-fall-silent mode. All fixtures implement identical recording
// and silent-fixture contracts; this table is what lets one set of steps
// exercise both.
type fakeAgentSpec struct {
	packagePath      string
	commandsEnv      string
	sizesLog         string
	silentFixtureEnv string
}

var fakeAgentSpecs = map[string]fakeAgentSpec{
	"claude": {packagePath: "./cmd/fake-claude", commandsEnv: "FAKE_CLAUDE_COMMANDS=1", sizesLog: "fake-claude-sizes.log", silentFixtureEnv: "FAKE_CLAUDE_FIXTURE"},
	"pi":     {packagePath: "./cmd/fake-pi", commandsEnv: "FAKE_PI_COMMANDS=1", sizesLog: "fake-pi-sizes.log", silentFixtureEnv: "FAKE_PI_FIXTURE"},
}

func registerFakeAgentSizeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fake "([^"]+)" agent is started recording sizes at (\d+)x(\d+)$`, aFakeAgentIsStartedRecordingSizesAt)
	sc.Step(`^the fake "([^"]+)" agent terminal is resized to (\d+)x(\d+)$`, theFakeAgentTerminalIsResizedTo)
	sc.Step(`^the fake "([^"]+)" agent recorded sizes are "([^"]*)"$`, theFakeAgentRecordedSizesAre)
	sc.Step(`^the fake "([^"]+)" agent is stopped$`, theFakeAgentIsStopped)
	sc.Step(`^a fake "([^"]+)" agent renders the "([^"]+)" preview fixture and falls silent at (\d+)x(\d+)$`, aFakeAgentRendersThePreviewFixtureAndFallsSilentAt)
	sc.Step(`^the fake "([^"]+)" agent's rendered pane is byte-identical across two consecutive captures$`, theFakeAgentsRenderedPaneIsByteIdenticalAcrossTwoConsecutiveCaptures)
}

// previewFixtureDirectory returns the checked-in directory of requirement
// 5's deterministic preview fixtures (internal/agent/testdata/preview),
// read directly rather than copied -- renderFixture only ever reads them,
// it never writes, so there is nothing for a scenario to isolate.
func previewFixtureDirectory() (string, error) {
	root, err := repositoryRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "internal", "agent", "testdata", "preview"), nil
}

// aFakeAgentRendersThePreviewFixtureAndFallsSilentAt starts the named fixture
// binary with its requirement-5 silent-fixture env var set to fixture, so it
// renders that file from internal/agent/testdata/preview exactly once and
// then produces no further output for the rest of the scenario.
func aFakeAgentRendersThePreviewFixtureAndFallsSilentAt(ctx context.Context, kind, fixture string, cols, rows uint16) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	spec, ok := fakeAgentSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown fake agent kind %q", kind)
	}
	binary, err := buildFakeAgentBinary(ctx, h, kind, spec.packagePath)
	if err != nil {
		return err
	}
	fixtureDir, err := previewFixtureDirectory()
	if err != nil {
		return err
	}
	driver, err := h.StartFakeAgentWithSize(ctx, binary, cols, rows,
		"FAKE_AGENT_FIXTURE_DIR="+fixtureDir,
		spec.silentFixtureEnv+"="+fixture,
	)
	if err != nil {
		return err
	}
	if h.fakeAgents == nil {
		h.fakeAgents = make(map[string]*ScreenDriver)
	}
	h.fakeAgents[kind] = driver
	return nil
}

// theFakeAgentsRenderedPaneIsByteIdenticalAcrossTwoConsecutiveCaptures reads
// the fixture's own pty-backed screen twice, with a short pause in between
// to give a misbehaving fixture time to emit something, and asserts the two
// captures are byte-identical -- requirement 5's byte-stable preview pane,
// proven from the fixture's own output rather than assumed.
func theFakeAgentsRenderedPaneIsByteIdenticalAcrossTwoConsecutiveCaptures(ctx context.Context, kind string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	driver, err := fakeAgentDriver(h, kind)
	if err != nil {
		return err
	}
	// The fixture's single render is asynchronous with this step, so the
	// first capture must wait for something to have arrived at all --
	// otherwise an empty "before it rendered" frame gets compared against a
	// populated "after it rendered" one and the assertion fails for a race,
	// not for the thing it means to test.
	first, err := waitForNonEmptyFrame(driver)
	if err != nil {
		return fmt.Errorf("fake %q agent never rendered a frame: %w", kind, err)
	}
	time.Sleep(150 * time.Millisecond)
	second := driver.Frame(true)
	if first != second {
		return fmt.Errorf("fake %q agent pane changed between captures:\n--- first ---\n%s\n--- second ---\n%s", kind, first, second)
	}
	return nil
}

func waitForNonEmptyFrame(driver *ScreenDriver) (string, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		frame := driver.Frame(true)
		if strings.TrimSpace(frame) != "" {
			return frame, nil
		}
		if time.Now().After(deadline) {
			return frame, errors.New("timed out waiting for a non-empty frame")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// aFakeAgentIsStartedRecordingSizesAt builds the named fixture (once per
// scenario invocation) and starts it directly under a real PTY at the given
// geometry, with its commands-reading mode enabled so it keeps running (and
// therefore keeps observing SIGWINCH) until the scenario explicitly stops it.
func aFakeAgentIsStartedRecordingSizesAt(ctx context.Context, kind string, cols, rows uint16) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	spec, ok := fakeAgentSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown fake agent kind %q", kind)
	}
	binary, err := buildFakeAgentBinary(ctx, h, kind, spec.packagePath)
	if err != nil {
		return err
	}
	driver, err := h.StartFakeAgentWithSize(ctx, binary, cols, rows, spec.commandsEnv)
	if err != nil {
		return err
	}
	if h.fakeAgents == nil {
		h.fakeAgents = make(map[string]*ScreenDriver)
	}
	h.fakeAgents[kind] = driver
	return nil
}

// buildFakeAgentBinary compiles the named fixture package into this
// scenario's DECK_HOME, mirroring the pattern already used by
// fakeClaudeOnPATHForFutureClients and fakeAgentScenario.buildFixture.
func buildFakeAgentBinary(ctx context.Context, h *ScenarioHarness, kind, packagePath string) (string, error) {
	root, err := repositoryRoot()
	if err != nil {
		return "", err
	}
	binary := filepath.Join(h.Home, "fake-"+kind+"-size-fixture")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, packagePath)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build fake %q agent fixture: %w\n%s", kind, err, output)
	}
	return binary, nil
}

func fakeAgentDriver(h *ScenarioHarness, kind string) (*ScreenDriver, error) {
	driver, ok := h.fakeAgents[kind]
	if !ok {
		return nil, fmt.Errorf("fake %q agent has not been started", kind)
	}
	return driver, nil
}

// theFakeAgentTerminalIsResizedTo resizes the fixture's own pty via
// TIOCSWINSZ, exactly as ScreenDriver.Resize does for a deck client
// (requirement 1's driver capability, reused here for requirement 4's own
// coverage): the kernel, not the harness, is what delivers SIGWINCH to the
// fixture's foreground process group.
func theFakeAgentTerminalIsResizedTo(ctx context.Context, kind string, cols, rows uint16) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	driver, err := fakeAgentDriver(h, kind)
	if err != nil {
		return err
	}
	return driver.Resize(cols, rows)
}

// theFakeAgentRecordedSizesAre reads the fixture's own size log (never the
// harness's or deck's bookkeeping) and compares its "COLSxROWS" lines,
// comma-joined, against want. It polls briefly because SIGWINCH delivery and
// the fixture's own file write are asynchronous with the resize step.
func theFakeAgentRecordedSizesAre(ctx context.Context, kind, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	spec, ok := fakeAgentSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown fake agent kind %q", kind)
	}
	path := filepath.Join(h.Home, "log", spec.sizesLog)
	got, err := waitForRecordedSizes(path, want)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("fake %q agent recorded sizes = %q, want %q", kind, got, want)
	}
	return nil
}

func waitForRecordedSizes(path, want string) (string, error) {
	deadline := time.Now().Add(2 * time.Second)
	var last string
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			last = strings.Join(strings.Fields(string(data)), ",")
			if last == want {
				return last, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("read fake agent sizes log %q: %w", path, err)
		}
		if time.Now().After(deadline) {
			return last, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// theFakeAgentIsStopped sends the fixture's commands-reading loop a
// canonical-mode EOF (Ctrl-D) so it exits on its own, then waits for that
// exit -- never a forced kill, so teardown never reports a "hung" fixture.
func theFakeAgentIsStopped(ctx context.Context, kind string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	driver, err := fakeAgentDriver(h, kind)
	if err != nil {
		return err
	}
	if err := driver.Send("\x04"); err != nil {
		return fmt.Errorf("send EOF to fake %q agent: %w", kind, err)
	}
	return driver.Stop(2 * time.Second)
}
