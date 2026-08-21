package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// ScenarioHarness owns every externally visible resource used by one Gherkin
// scenario. It intentionally uses only the released binary and tmux commands:
// feature steps remain a black-box consumer of deck.
type ScenarioHarness struct {
	Home   string
	Socket string
	Binary string

	clients         []*ScreenDriver
	namedClients    map[string]*ScreenDriver
	workingDir      string
	sentinel        []byte
	databaseFixture []byte
	newerRefusal    string
	// v1FixtureSessionID, when set by v1DatabaseFixtureWithSession, is the id
	// a fixture inserted directly into a v1 sessions table before the
	// released binary opened and migrated the database (task 010), so a
	// later step can prove the row was migrated in place rather than
	// recreated.
	v1FixtureSessionID string
	// agentPATHDir, when set by fakeClaudeOnPATHForFutureClients, is prepended
	// to a real PATH for every subsequently started named client, so a real
	// coding-agent session can find its fixture-provided binary without ever
	// hiding the real tmux/go that the harness itself still needs.
	agentPATHDir string
	// agentHOMEDir, when set by fakeClaudeOnPATHForFutureClients, is passed as
	// HOME for every subsequently started named client, so a fixture's
	// per-conversation transcript (cmd/fake-claude's transcriptPath) is written
	// under a scenario-scoped directory rather than the real developer's home.
	agentHOMEDir string
	// fakeAgents holds fake-agent fixture drivers keyed by agent kind
	// ("claude"/"pi"), started directly (not through the deck binary) to prove
	// requirement 4's size-recording contract from the fixture's own
	// perspective. Tracked in clients too, so Close tears them down the same
	// way as every deck client.
	fakeAgents map[string]*ScreenDriver
	// clientEnv holds scenario-scoped runtime controls that must be present in
	// every subsequently started released client (for example a frozen probe
	// clock). Steps set it before starting any client.
	clientEnv []string

	// windowGeometrySnapshots and sizeLogSnapshots back task 022's preview.
	// feature invariant checks (requirement 21/27): a step captures a
	// private tmux window's own #{window_width}x#{window_height} (or a fake
	// agent's own SIGWINCH size-recording log content) under a label, and a
	// later step asserts the current value is byte-identical to it, proving
	// the preview capture engine never resized a pane or triggered a
	// SIGWINCH across whatever gestures ran in between.
	windowGeometrySnapshots map[string]string
	sizeLogSnapshots        map[string]string

	// layoutSeamSnapshots and configTOMLSnapshots back features/layout_modes.feature
	// (task 030, requirement 38): the first captures the shared seam's own
	// rendered column so `<`/`>` clamping can be proven idempotent past
	// each end without hardcoding an expected column number, and the
	// second captures config.toml's own bytes (or their absence) so a
	// restart across layout_mode/sidebar_width keypresses can prove the
	// file never changed.
	layoutSeamSnapshots map[string]int
	configTOMLSnapshots map[string]string

	// statusRowSnapshots backs features/attention_sort.feature's `space`
	// non-vacuity check (requirements 31/32, task 026): a step captures
	// every session row's status-related columns under a label, and a
	// later step asserts the current values are byte-identical to it,
	// proving repeated `space` presses never write to the sessions table
	// (only m.selected moves).
	statusRowSnapshots map[string]string

	// preResumeConversationIDs backs features/status_recovery.feature's
	// in-session resume scenario (requirement 43/44): a step captures a
	// session's conversation_id immediately before firing fake-claude's
	// "resume" pane command, so a later step can assert the durable row
	// moved to a genuinely different, non-empty id rather than merely
	// re-reading whatever value happens to be there now.
	preResumeConversationIDs map[string]string

	// Test seams exercise teardown's leak reporting without weakening the
	// default black-box lifecycle used by feature scenarios.
	tmuxProbe  func() bool
	removeHome func(string) error
}

var scenarioSequence atomic.Uint64

// Reconciliation includes a real tmux CLI liveness query, so retain a
// practical cadence that permits that external operation and render to finish
// within the one-cadence black-box deadline.
const scenarioReconcileInterval = 250 * time.Millisecond

type scenarioHarnessKey struct{}

func newScenarioHarness(binary string) (*ScenarioHarness, error) {
	home, err := os.MkdirTemp("", "deck-scenario-")
	if err != nil {
		return nil, fmt.Errorf("create scenario DECK_HOME: %w", err)
	}
	// tmux socket names are global to the user, unlike DECK_HOME. Include a
	// process-local sequence as well as the PID so parallel godog runs cannot
	// collide with one another.
	socket := fmt.Sprintf("deck_test_%d_%d", os.Getpid(), scenarioSequence.Add(1))
	return &ScenarioHarness{Home: home, Socket: socket, Binary: binary, namedClients: make(map[string]*ScreenDriver)}, nil
}

// Environment is supplied to every deck client in this scenario. Multiple
// clients deliberately receive the same root and socket.
func (h *ScenarioHarness) Environment(extra ...string) []string {
	env := []string{
		"DECK_HOME=" + h.Home,
		"DECK_TMUX_SOCKET=" + h.Socket,
		"DECK_ASCII=1", "DECK_ANIM=0",
		"DECK_RECONCILE_MS=" + fmt.Sprintf("%d", scenarioReconcileInterval.Milliseconds()), "DECK_PREVIEW_MS=50",
	}
	// NO_COLOR=1 is the default so every scenario that does not care about
	// colour keeps seeing plain text, but a requirement-1 scenario that does
	// care needs to unset it entirely rather than merely re-set it to "1".
	// An explicit "NO_COLOR=" sentinel (empty value) in clientEnv/extra means
	// exactly that -- config's getenv wrapper cannot distinguish "set to
	// empty" from "unset" anyway, so the sentinel is dropped rather than
	// passed through to exec, and the default is skipped.
	all := append(append([]string{}, h.clientEnv...), extra...)
	noColorOverridden := false
	filtered := make([]string, 0, len(all))
	for _, e := range all {
		if e == "NO_COLOR=" {
			noColorOverridden = true
			continue
		}
		if strings.HasPrefix(e, "NO_COLOR=") {
			noColorOverridden = true
		}
		filtered = append(filtered, e)
	}
	if !noColorOverridden {
		env = append(env, "NO_COLOR=1")
	}
	return append(env, filtered...)
}

// StartClient starts another independent PTY client while retaining ownership
// for teardown. This is the only supported way feature steps create clients.
func (h *ScenarioHarness) StartClient(ctx context.Context, extraEnv ...string) (*ScreenDriver, error) {
	return h.StartClientWithSize(ctx, terminalColumns, terminalRows, extraEnv...)
}

// StartClientWithSize is StartClient with an explicit initial geometry, for
// scenarios that start at a size other than the harness default and
// optionally resize mid-scenario (requirement 1).
func (h *ScenarioHarness) StartClientWithSize(ctx context.Context, cols, rows uint16, extraEnv ...string) (*ScreenDriver, error) {
	if h.Binary == "" {
		return nil, errors.New("scenario deck binary is required")
	}
	if h.agentPATHDir != "" && !hasPATHOverride(extraEnv) {
		extraEnv = append(extraEnv, "PATH="+h.agentPATHDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if h.agentHOMEDir != "" && !hasHOMEOverride(extraEnv) {
		extraEnv = append(extraEnv, "HOME="+h.agentHOMEDir)
	}
	client, err := StartScreenDriverWithSize(ctx, h.Binary, h.Environment(extraEnv...), cols, rows)
	if err != nil {
		return nil, err
	}
	h.clients = append(h.clients, client)
	return client, nil
}

// hasPATHOverride reports whether extraEnv already sets PATH itself (e.g.
// startClientWithoutTMux/startClientWithTMuxVersion deliberately replace
// PATH entirely), so agentPATHDir must not also append a second PATH entry.
func hasPATHOverride(extraEnv []string) bool {
	for _, entry := range extraEnv {
		if strings.HasPrefix(entry, "PATH=") {
			return true
		}
	}
	return false
}

// hasHOMEOverride mirrors hasPATHOverride for HOME, so a step that already
// passed its own HOME= entry is never overridden by agentHOMEDir.
func hasHOMEOverride(extraEnv []string) bool {
	for _, entry := range extraEnv {
		if strings.HasPrefix(entry, "HOME=") {
			return true
		}
	}
	return false
}

// StartNamedClient gives Gherkin steps a stable external-client handle. Names
// are harness bookkeeping only; no data is passed into the deck binary.
func (h *ScenarioHarness) StartNamedClient(ctx context.Context, name string, extraEnv ...string) (*ScreenDriver, error) {
	return h.StartNamedClientWithSize(ctx, name, terminalColumns, terminalRows, extraEnv...)
}

// StartNamedClientWithSize is StartNamedClient with an explicit initial
// geometry (requirement 1).
func (h *ScenarioHarness) StartNamedClientWithSize(ctx context.Context, name string, cols, rows uint16, extraEnv ...string) (*ScreenDriver, error) {
	if name == "" {
		return nil, errors.New("client name is required")
	}
	if _, exists := h.namedClients[name]; exists {
		return nil, fmt.Errorf("deck client %q is already running", name)
	}
	client, err := h.StartClientWithSize(ctx, cols, rows, extraEnv...)
	if err != nil {
		return nil, err
	}
	h.namedClients[name] = client
	return client, nil
}

func (h *ScenarioHarness) Client(name string) (*ScreenDriver, error) {
	client, ok := h.namedClients[name]
	if !ok {
		return nil, fmt.Errorf("deck client %q has not been started", name)
	}
	return client, nil
}

// StartFakeAgentWithSize starts a bare fake-agent fixture binary directly
// (not deck itself) under this scenario's DECK_HOME, tracked for the same
// teardown as every deck client. Requirement 4's size-recording contract is
// the fixture's own experience, not deck's, so this deliberately bypasses
// h.Binary and every deck-specific launch plumbing.
func (h *ScenarioHarness) StartFakeAgentWithSize(ctx context.Context, binary string, cols, rows uint16, extraEnv ...string) (*ScreenDriver, error) {
	driver, err := StartScreenDriverWithSize(ctx, binary, h.Environment(extraEnv...), cols, rows)
	if err != nil {
		return nil, err
	}
	h.clients = append(h.clients, driver)
	return driver, nil
}

// KillTMuxServer is the black-box reboot primitive: it removes all live panes
// but leaves the durable DECK_HOME state untouched.
func (h *ScenarioHarness) KillTMuxServer(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "tmux", "-L", h.Socket, "kill-server")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "no server running") && !strings.Contains(string(output), "No such file") {
		return fmt.Errorf("kill private tmux server %q: %w: %s", h.Socket, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Close is deliberately strict. It cleans every resource it owns, then checks
// that no client, responding socket, or scenario directory survived cleanup.
func (h *ScenarioHarness) Close() error {
	var problems []error
	for _, client := range h.clients {
		if err := client.Stop(time.Second); err != nil {
			problems = append(problems, fmt.Errorf("surviving deck client: %w", err))
		}
		select {
		case <-client.done:
		default:
			problems = append(problems, errors.New("surviving deck child after termination"))
		}
	}

	killCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := h.KillTMuxServer(killCtx); err != nil {
		problems = append(problems, err)
	}
	cancel()
	probe := h.tmuxResponds
	if h.tmuxProbe != nil {
		probe = h.tmuxProbe
	}
	if probe() {
		problems = append(problems, fmt.Errorf("private tmux socket %q still responds after teardown", h.Socket))
	}

	removeHome := os.RemoveAll
	if h.removeHome != nil {
		removeHome = h.removeHome
	}
	if err := removeHome(h.Home); err != nil {
		problems = append(problems, fmt.Errorf("remove scenario DECK_HOME %q: %w", h.Home, err))
	}
	if _, err := os.Lstat(h.Home); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			problems = append(problems, fmt.Errorf("scenario DECK_HOME %q survived teardown", h.Home))
		} else {
			problems = append(problems, fmt.Errorf("inspect scenario DECK_HOME %q: %w", h.Home, err))
		}
	}
	return errors.Join(problems...)
}

func (h *ScenarioHarness) tmuxResponds() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// list-sessions returns success only for a responding server which owns at
	// least one session. It does not create a server when none exists.
	return exec.CommandContext(ctx, "tmux", "-L", h.Socket, "list-sessions").Run() == nil
}

func scenarioHarness(ctx context.Context) (*ScenarioHarness, error) {
	harness, ok := ctx.Value(scenarioHarnessKey{}).(*ScenarioHarness)
	if !ok || harness == nil {
		return nil, errors.New("scenario harness was not initialized")
	}
	return harness, nil
}

func registerScenarioLifecycle(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		root, err := filepath.Abs("..")
		if err != nil {
			return ctx, fmt.Errorf("locate repository root: %w", err)
		}
		// The normal feature suite may be invoked without the focused driver
		// test, so build its own released binary once per scenario.
		binary := filepath.Join(os.TempDir(), fmt.Sprintf("deck-godog-%d-%d", os.Getpid(), scenarioSequence.Add(1)))
		if output, err := exec.Command("go", "build", "-o", binary, filepath.Join(root, "cmd", "deck")).CombinedOutput(); err != nil {
			return ctx, fmt.Errorf("build deck for scenario lifecycle: %w\n%s", err, output)
		}
		harness, err := newScenarioHarness(binary)
		if err != nil {
			_ = os.Remove(binary)
			return ctx, err
		}
		return context.WithValue(ctx, scenarioHarnessKey{}, harness), nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		harness, err := scenarioHarness(ctx)
		if err != nil {
			return ctx, err
		}
		closeErr := harness.Close()
		_ = os.Remove(harness.Binary)
		if scenarioErr != nil && closeErr != nil {
			return ctx, errors.Join(scenarioErr, closeErr)
		}
		return ctx, closeErr
	})
}

func TestScenarioHarnessTeardownReportsLeaks(t *testing.T) {
	t.Run("surviving child", func(t *testing.T) {
		harness, err := newScenarioHarness(buildDeckBinary(t))
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command("sleep", "30")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		driver := &ScreenDriver{cmd: child, done: make(chan struct{})}
		go func() { _ = child.Wait(); close(driver.done) }()
		harness.clients = append(harness.clients, driver)
		err = harness.Close()
		if err == nil || !strings.Contains(err.Error(), "surviving deck client") {
			t.Fatalf("Close() error = %v, want surviving-client diagnostic", err)
		}
		_ = os.Remove(harness.Binary)
	})

	t.Run("responding socket", func(t *testing.T) {
		harness, err := newScenarioHarness(buildDeckBinary(t))
		if err != nil {
			t.Fatal(err)
		}
		harness.tmuxProbe = func() bool { return true }
		err = harness.Close()
		if err == nil || !strings.Contains(err.Error(), "still responds") {
			t.Fatalf("Close() error = %v, want responding-socket diagnostic", err)
		}
		_ = os.Remove(harness.Binary)
	})

	t.Run("surviving root", func(t *testing.T) {
		harness, err := newScenarioHarness(buildDeckBinary(t))
		if err != nil {
			t.Fatal(err)
		}
		harness.removeHome = func(string) error { return nil }
		err = harness.Close()
		if err == nil || !strings.Contains(err.Error(), "survived teardown") {
			t.Fatalf("Close() error = %v, want surviving-root diagnostic", err)
		}
		if cleanupErr := os.RemoveAll(harness.Home); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
		_ = os.Remove(harness.Binary)
	})
}

func TestScenarioHarnessSharesIsolationAndCleansUp(t *testing.T) {
	binary := buildDeckBinary(t)
	harness, err := newScenarioHarness(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := harness.StartClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	// Let the first process initialize the empty database before a second
	// client opens the same durable root; the scenario is still multi-client,
	// just not racing first-use migration.
	second, err := harness.StartClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []*ScreenDriver{second, first} {
		if err := client.WaitForFrame(ctx, false, "No sessions"); err != nil {
			t.Fatal(err)
		}
		if err := client.Send("q"); err != nil {
			t.Fatal(err)
		}
	}
	// The exposed reboot primitive is harmless before a server exists and is
	// what future @reboot steps use after creating live sessions.
	if err := harness.KillTMuxServer(ctx); err != nil {
		t.Fatal(err)
	}
	if err := harness.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(harness.Home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scenario root remains after clean teardown: %v", err)
	}
}
