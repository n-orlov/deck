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
	"github.com/n-orlov/deck/internal/tmux"
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
	// fakeAgentFixtureBytes records, per agent kind, the exact on-disk byte
	// length of the requirement-5 preview fixture that kind's driver was
	// started with (task 114). The single render write on the fixture side
	// can arrive at this harness's pty reader split across more than one
	// 4096-byte Read (cmd/pty_driver_test.go's read loop), so "the pane has
	// rendered something" and "the pane has rendered the WHOLE fixture" are
	// different moments; polling the accumulated raw byte count against this
	// exact, known length is what tells the two apart deterministically
	// instead of guessing from a fixed sleep.
	fakeAgentFixtureBytes map[string]int
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

	// windowOwnershipSnapshots backs task 314's (R54) no-op discriminator:
	// a tmux window user option value (internal/tmux/ownership.go's
	// @deck_isize_owner, a fresh random tag per claim) captured under a
	// label, proving a click that must be a no-op never re-claimed the
	// window at all.
	windowOwnershipSnapshots map[string]string

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

	// codexPaneAnnouncements backs features/codex_hooks.feature's B3
	// attribution check (task 008): a step captures a Codex pane's own
	// authoritative SessionStart identity -- its session_id AND its
	// transcript_path -- straight off cmd/fake-codex's own banner lines
	// (never off the state database), keyed by the scenario's session
	// name, so a later step can compare the STORED row and the PRODUCTION
	// transcript-path resolution against this independently-captured pair
	// rather than against each other.
	codexPaneAnnouncements map[string]codexPaneAnnouncement

	// Test seams exercise teardown's leak reporting without weakening the
	// default black-box lifecycle used by feature scenarios.
	tmuxProbe  func() bool
	removeHome func(string) error

	// namedDirectories and directoryFingerprints back requirement 29's
	// working-directory fingerprint harness (task 002): the former maps a
	// scenario-chosen label to a real directory path (populated today only
	// by the scratch-directory seeding step, and by a session's own cwd in
	// later tasks), the latter maps a fingerprint label to the snapshot
	// taken of that path, so a later step can assert byte-for-byte and
	// stat-for-stat identity without repeating the path.
	namedDirectories      map[string]string
	directoryFingerprints map[string]directoryFingerprintRecord

	// livePaneEnvSnapshots backs task 021's own "an env-editor commit never
	// applies silently to an already-running pane" criterion: a step
	// captures a live pane process's /proc/<pid>/environ value for one key
	// under a label (never a hardcoded expected string, since a session's
	// real PATH varies by host), and a later step asserts the current
	// value is still byte-identical to it.
	livePaneEnvSnapshots map[string]string

	// livePanePidSnapshots backs task 023's "inject-instead never kills or
	// relaunches the pane" criterion: a step captures a live pane's own
	// #{pane_pid} under a label before an inject, and a later step asserts
	// the pid is still identical afterwards -- a restart would produce a
	// brand-new pid, since tmux exec's a fresh process into the pane.
	livePanePidSnapshots map[string]string

	// capturedSessionIDs backs task 107's reap-leaves-no-trace scenario: a
	// step captures a session's durable store id, keyed by its display name,
	// before dd/reap remove the sessions row entirely -- once the row is
	// gone, the id can no longer be looked up by name, but the scenario
	// still needs it to build the $DECK_HOME/captures/<id>/ and history
	// file paths and to match the audit JSONL's own session_id field.
	capturedSessionIDs map[string]string

	// transcriptSnapshots backs task 110's purge/no-purge scenarios
	// (requirements 25/26): a step captures an agent's own declared
	// transcript file's path and bytes under a label, right after the
	// agent has written to it and before dd's own kill/reap can touch
	// anything, so a later step can assert either that the exact same
	// bytes are still there at that exact path (no purge) or that the
	// path is gone (purge chosen).
	transcriptSnapshots map[string]transcriptSnapshot

	// lastGeometryFitResizes/lastGeometryFitErr back task 034's (II-7/II-8)
	// scenario: the number of resize-window calls internal/tmux.Client.
	// FitWindowToPane actually issued the last time deckFitsWindowPaneTo
	// ran, and any error it returned (a non-convergent fit is asserted via
	// this, not by the step itself failing immediately, so a scenario can
	// assert ON the failure shape rather than merely triggering one).
	lastGeometryFitResizes int
	lastGeometryFitErr     error

	// lastNaiveLoopConverged/lastNaiveLoopFinalHeight back task 034's
	// mandatory negative control: the PRD-named naive pane-targeting
	// alternative (resize-window with the WANTED pane size, never
	// compensating for chrome) run to its own bound, and whether it ever
	// reached the wanted size.
	lastNaiveLoopConverged   bool
	lastNaiveLoopFinalHeight int

	// sigwinchCycleGeometries backs task 036's (II-11) full enter/exit
	// SIGWINCH-budget scenario: the WindowGeometry captured by the "enters
	// interactive mode" step, keyed by tmux session, so the matching
	// "exits interactive mode" step has exactly what RestoreWindowGeometry
	// needs without threading it through the Gherkin text itself.
	sigwinchCycleGeometries map[string]tmux.WindowGeometry

	// rawAttachedTmuxClients backs the same scenario's "attached throughout"
	// half: a real `tmux attach-session` client, attached directly on this
	// scenario's own private socket (bypassing deck entirely, the same
	// black-box shape interactive_geometry_test.go and internal/tmux's own
	// restore_test.go already use), keyed by session so the matching detach
	// step can find it again.
	rawAttachedTmuxClients map[string]*rawAttachedTmuxClient

	// optionTableDumps backs task 037's (II-12) byte-exact-restore scenario:
	// every one of tmux's seven option tables for one target, captured under
	// a caller-chosen label ("before"/"entered"/"after"), keyed by
	// target+label so two labels for the same target can be diffed later
	// without re-reading tmux (a second read could itself observe a
	// different state than the one the scenario meant to freeze).
	optionTableDumps map[string]optionTableDump

	// interactiveScrollPagesBack backs features/interactive_scroll.feature's
	// own Shift+PgUp/PgDn symmetry (issue #29): how many WHOLE pages the
	// "scrolls back with shift+pgup until the screen contains ..." step
	// actually had to press for one client, keyed by client name, so the
	// matching "scrolls forward with shift+pgdown by the same number of
	// pages" step can return that client's view to the live bottom exactly.
	// It is a page COUNT rather than a hardcoded number in the Gherkin
	// because the number of pages between the live bottom and a marker now
	// depends on how much of the pane's own tmux history the entry seed
	// pulled into the grid (internal/interactive.CaptureSeedWithHistory),
	// which is a property of the pane's past, not of the scenario -- a fixed
	// count would silently stop landing on the live bottom the moment the
	// harness's own shell banner changed length.
	interactiveScrollPagesBack map[string]int
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

// StartClientInDir starts a client with the released binary's own process
// working directory pinned to dir, so a scenario can assert exactly what
// deck prefills its create modal's cwd field with when it started (task
// 008, SPEC §11.7's no-history fallback) rather than depending on whatever
// directory happens to be the test binary's own cwd.
func (h *ScenarioHarness) StartClientInDir(ctx context.Context, dir string, extraEnv ...string) (*ScreenDriver, error) {
	if h.Binary == "" {
		return nil, errors.New("scenario deck binary is required")
	}
	client, err := StartScreenDriverInDir(ctx, h.Binary, h.Environment(extraEnv...), dir, terminalColumns, terminalRows)
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

// StartNamedClientInDir is StartNamedClient with the released binary's own
// process working directory pinned to dir (task 008).
func (h *ScenarioHarness) StartNamedClientInDir(ctx context.Context, name, dir string, extraEnv ...string) (*ScreenDriver, error) {
	if name == "" {
		return nil, errors.New("client name is required")
	}
	if _, exists := h.namedClients[name]; exists {
		return nil, fmt.Errorf("deck client %q is already running", name)
	}
	client, err := h.StartClientInDir(ctx, dir, extraEnv...)
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

// scenarioBuildDeadline bounds the per-scenario `go build` in
// registerScenarioLifecycle's Before hook (steer 021 §1/task 219). Before
// this bound existed, that build ran via a bare exec.Command with no
// context and no deadline: `cmd/go` blocks indefinitely on its own
// build/module-cache locks by design, and that cache is a shared docker
// volume across every container on the host, so a stale lock or ANY
// concurrent `go` process sharing that volume converted into an unbounded
// hang here -- upstream of every ScreenDriver wait task 205 already bounded,
// and with no diagnostic at all: a real 45m-timeout run's goroutine dump
// showed a goroutine blocked 42+ minutes inside exactly this call, attributed
// by godog to whichever scenario's Before hook happened to draw the short
// straw (a red herring -- the scenario name in the panic is not where the
// bug is). Five minutes is roughly 60x the normal cost of this build
// (measured ~1-5s) and still far cheaper than the failure mode it replaces.
// A timeout here is an infra/environment fault, not a product bug -- callers
// evaluating a features run for stability must classify it as such, never as
// evidence about deck's own behaviour. This does NOT restructure the
// once-per-scenario build into once-per-suite; that trade is recorded as a
// deferred requirement in docs/reports/phase3-findings.md instead.
var scenarioBuildDeadline = 5 * time.Minute

func registerScenarioLifecycle(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, sce *godog.Scenario) (context.Context, error) {
		root, err := filepath.Abs("..")
		if err != nil {
			return ctx, fmt.Errorf("locate repository root: %w", err)
		}
		// The normal feature suite may be invoked without the focused driver
		// test, so build its own released binary once per scenario.
		binary := filepath.Join(os.TempDir(), fmt.Sprintf("deck-godog-%d-%d", os.Getpid(), scenarioSequence.Add(1)))
		buildCtx, cancel := context.WithTimeout(context.Background(), scenarioBuildDeadline)
		start := time.Now()
		// R146/task 016: when the caller (ci/suite.sh) sets GOCOVERDIR, build the
		// released binary with `-cover` so every scenario's deck process writes
		// black-box coverage counters into that directory via its inherited
		// environment (StartScreenDriverInDir's cmd.Env starts from os.Environ(),
		// which already carries GOCOVERDIR when the test process itself has it).
		// Unset, this is the exact same `go build -o binary cmd/deck` as before.
		buildArgs := []string{"build", "-o", binary}
		if os.Getenv("GOCOVERDIR") != "" {
			buildArgs = append(buildArgs, "-cover")
		}
		buildArgs = append(buildArgs, filepath.Join(root, "cmd", "deck"))
		output, buildErr := exec.CommandContext(buildCtx, "go", buildArgs...).CombinedOutput()
		elapsed := time.Since(start)
		timedOut := buildCtx.Err() != nil
		cancel()
		if timedOut {
			return ctx, fmt.Errorf("INFRA FAULT (not a product bug): per-scenario `go build` for %q (binary %s) did not finish within its %s bound; elapsed %s; partial output:\n%s", sce.Name, binary, scenarioBuildDeadline, elapsed, output)
		}
		if buildErr != nil {
			return ctx, fmt.Errorf("build deck for scenario lifecycle: %w\n%s", buildErr, output)
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
