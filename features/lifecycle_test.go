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
	// clientEnv holds scenario-scoped runtime controls that must be present in
	// every subsequently started released client (for example a frozen probe
	// clock). Steps set it before starting any client.
	clientEnv []string

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
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
		"DECK_RECONCILE_MS=" + fmt.Sprintf("%d", scenarioReconcileInterval.Milliseconds()), "DECK_PREVIEW_MS=50",
	}
	env = append(env, h.clientEnv...)
	return append(env, extra...)
}

// StartClient starts another independent PTY client while retaining ownership
// for teardown. This is the only supported way feature steps create clients.
func (h *ScenarioHarness) StartClient(ctx context.Context, extraEnv ...string) (*ScreenDriver, error) {
	if h.Binary == "" {
		return nil, errors.New("scenario deck binary is required")
	}
	if h.agentPATHDir != "" && !hasPATHOverride(extraEnv) {
		extraEnv = append(extraEnv, "PATH="+h.agentPATHDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if h.agentHOMEDir != "" && !hasHOMEOverride(extraEnv) {
		extraEnv = append(extraEnv, "HOME="+h.agentHOMEDir)
	}
	client, err := StartScreenDriver(ctx, h.Binary, h.Environment(extraEnv...))
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
	if name == "" {
		return nil, errors.New("client name is required")
	}
	if _, exists := h.namedClients[name]; exists {
		return nil, fmt.Errorf("deck client %q is already running", name)
	}
	client, err := h.StartClient(ctx, extraEnv...)
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
