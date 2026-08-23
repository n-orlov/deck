package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	sigwinchCountLog string
	silentFixtureEnv string
}

var fakeAgentSpecs = map[string]fakeAgentSpec{
	"claude": {packagePath: "./cmd/fake-claude", commandsEnv: "FAKE_CLAUDE_COMMANDS=1", sizesLog: "fake-claude-sizes.log", sigwinchCountLog: "fake-claude-sigwinch-count", silentFixtureEnv: "FAKE_CLAUDE_FIXTURE"},
	"pi":     {packagePath: "./cmd/fake-pi", commandsEnv: "FAKE_PI_COMMANDS=1", sizesLog: "fake-pi-sizes.log", sigwinchCountLog: "fake-pi-sigwinch-count", silentFixtureEnv: "FAKE_PI_FIXTURE"},
}

func registerFakeAgentSizeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fake "([^"]+)" agent is started recording sizes at (\d+)x(\d+)$`, aFakeAgentIsStartedRecordingSizesAt)
	sc.Step(`^the fake "([^"]+)" agent terminal is resized to (\d+)x(\d+)$`, theFakeAgentTerminalIsResizedTo)
	sc.Step(`^the fake "([^"]+)" agent recorded sizes are "([^"]*)"$`, theFakeAgentRecordedSizesAre)
	sc.Step(`^the fake "([^"]+)" agent is stopped$`, theFakeAgentIsStopped)
	sc.Step(`^a fake "([^"]+)" agent renders the "([^"]+)" preview fixture and falls silent at (\d+)x(\d+)$`, aFakeAgentRendersThePreviewFixtureAndFallsSilentAt)
	sc.Step(`^the fake "([^"]+)" agent's rendered pane is byte-identical across two consecutive captures$`, theFakeAgentsRenderedPaneIsByteIdenticalAcrossTwoConsecutiveCaptures)
	sc.Step(`^the fake "([^"]+)" agent received exactly (\d+) SIGWINCH signals?$`, theFakeAgentReceivedExactlySigwinchSignals)
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
	info, err := os.Stat(filepath.Join(fixtureDir, fixture))
	if err != nil {
		return fmt.Errorf("stat preview fixture %q: %w", fixture, err)
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
	if h.fakeAgentFixtureBytes == nil {
		h.fakeAgentFixtureBytes = make(map[string]int)
	}
	h.fakeAgentFixtureBytes[kind] = int(info.Size())
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
	// The fixture's single render is one Write() on the fixture side, but this
	// harness's own pty reader (ScreenDriver.read, features/pty_driver_test.go)
	// pulls it off the pty in a loop of up to-4096-byte Reads, so a fixture
	// bigger than that (oversized.txt is 4880 bytes) legitimately arrives
	// across two Reads with an unpredictable gap between them, especially
	// under host CPU contention. "Frame is non-empty" is true after the FIRST
	// of those Reads is processed, well before the fixture has finished
	// rendering; comparing that partial frame against a later, complete one
	// fails on a genuine harness-side race, not on any product misbehaviour
	// (task 114). Waiting for the accumulated raw byte count to reach the
	// fixture's own known, exact on-disk size is a deterministic proxy for
	// "the whole write has arrived" -- no guessed sleep, no flakiness budget.
	minBytes := h.fakeAgentFixtureBytes[kind]
	first, err := waitForFixtureFullyRendered(driver, minBytes)
	if err != nil {
		return fmt.Errorf("fake %q agent never fully rendered its fixture: %w", kind, err)
	}
	time.Sleep(150 * time.Millisecond)
	second := driver.Frame(true)
	if first != second {
		return fmt.Errorf("fake %q agent pane changed between captures:\n--- first ---\n%s\n--- second ---\n%s", kind, first, second)
	}
	return nil
}

// waitForFixtureFullyRendered polls until the driver's accumulated raw byte
// count reaches minBytes (the fixture's own on-disk size, stat'd before it
// was rendered) AND the resulting frame is non-empty, then returns that
// frame. If minBytes is 0 (unknown -- defensive only, every caller today
// always sets it) it falls back to the old "merely non-empty" check.
func waitForFixtureFullyRendered(driver *ScreenDriver, minBytes int) (string, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		frame := driver.Frame(true)
		nonEmpty := strings.TrimSpace(frame) != ""
		if nonEmpty && (minBytes == 0 || len(driver.Raw()) >= minBytes) {
			return frame, nil
		}
		if time.Now().After(deadline) {
			return frame, fmt.Errorf("timed out waiting for a fully rendered frame (raw bytes seen = %d, want >= %d)", len(driver.Raw()), minBytes)
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

// theFakeAgentReceivedExactlySigwinchSignals reads the fixture's own
// dedicated SIGWINCH counter (requirement 11 / II-2) -- a bare decimal
// integer the fixture overwrites on every SIGWINCH it observes, never a
// derivation of the sizes log above -- and asserts it equals want exactly.
// It is deliberately an exact-equality check, not "at least want": the whole
// point of a count assertion is to catch an off-by-one, and a step that only
// ever asserted a lower bound could not.
func theFakeAgentReceivedExactlySigwinchSignals(ctx context.Context, kind string, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	spec, ok := fakeAgentSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown fake agent kind %q", kind)
	}
	path := filepath.Join(h.Home, "log", spec.sigwinchCountLog)
	got, err := waitForSigwinchCount(path, want)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("fake %q agent received %d SIGWINCH signals, want exactly %d", kind, got, want)
	}
	return nil
}

// readSigwinchCount reads path's bare decimal integer, treating a missing
// file (no SIGWINCH observed yet) as 0, exactly like
// features/input_count_test.go's readInputCount treats its own counter file.
func readSigwinchCount(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read sigwinch count %q: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, nil
	}
	total, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("sigwinch count %q is not a bare integer: %q", path, trimmed)
	}
	return total, nil
}

// waitForSigwinchCount polls path until it reads want or a 2-second deadline
// passes, returning whatever the last read was either way so the caller can
// report the exact mismatch. It never returns early just because the count
// reached or passed want-as-a-floor: SIGWINCH delivery and the fixture's own
// file write are asynchronous with whatever step sent the signal, but once
// the count is read as strictly greater than want the assertion is already
// wrong and there is nothing more to wait for.
func waitForSigwinchCount(path string, want int) (int, error) {
	deadline := time.Now().Add(2 * time.Second)
	var last int
	for {
		total, err := readSigwinchCount(path)
		if err != nil {
			return 0, err
		}
		last = total
		if total == want || total > want {
			return total, nil
		}
		if time.Now().After(deadline) {
			return last, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
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
