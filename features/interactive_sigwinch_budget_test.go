package features

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// registerInteractiveSigwinchBudgetSteps wires PRD phase3b II-11's
// scenario: a full enter/exit cycle -- CaptureWindowGeometry then
// FitWindowToPane to enter, RestoreWindowGeometry to exit, the exact same
// internal/tmux primitives task 034/035 shipped, run against a fake agent
// occupying a bare tmux session on this scenario's own private socket --
// must cost exactly two SIGWINCH, both attached and detached. It reuses
// task 027's own count step (registerFakeAgentSizeSteps's "the fake ...
// agent received exactly N SIGWINCH signals") unchanged: that step only
// ever reads the fixture's own $DECK_HOME/log counter file and does not
// care how the fixture's pty came to exist, so running the fixture as a
// tmux pane's command instead of directly under StartFakeAgentWithSize's
// bare pty is a legitimate substitution, not a new counting mechanism.
func registerInteractiveSigwinchBudgetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fake "([^"]+)" agent occupies a bare tmux session "([^"]+)" at (\d+)x(\d+)$`, aFakeAgentOccupiesABareTmuxSessionAt)
	sc.Step(`^deck enters interactive mode on tmux session "([^"]+)" fitting to (\d+)x(\d+)$`, deckEntersInteractiveModeFittingTo)
	sc.Step(`^deck exits interactive mode on tmux session "([^"]+)"$`, deckExitsInteractiveModeOnTmuxSession)
	sc.Step(`^a real tmux client attaches to session "([^"]+)" at (\d+)x(\d+)$`, aRealTmuxClientAttachesToSessionAt)
	sc.Step(`^the real tmux client attached to session "([^"]+)" detaches$`, theRealTmuxClientAttachedToSessionDetaches)
}

// aFakeAgentOccupiesABareTmuxSessionAt creates a bare, single-pane tmux
// session directly on this scenario's own private socket (the same
// bare-session shape interactive_geometry_test.go's
// tmuxSessionIsABareSplitWindow already uses -- there is no deck-level
// consumer of FitWindowToPane/RestoreWindowGeometry yet, task 061 wires
// Enter itself) whose pane command IS the named fake-agent fixture
// binary, with DECK_HOME and its own commands-reading env var passed
// through `new-session -e`, so it keeps running (and therefore keeps
// observing every SIGWINCH tmux's own resize-window/window-size-latest
// machinery delivers to it) for the rest of the scenario.
func aFakeAgentOccupiesABareTmuxSessionAt(ctx context.Context, kind, session string, cols, rows int) error {
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
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := []string{
		"-L", h.Socket, "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows),
		"-e", "DECK_HOME=" + h.Home,
		"-e", spec.commandsEnv,
		"--", binary,
	}
	if output, err := exec.CommandContext(commandCtx, "tmux", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s new-session (fake %s agent) -s %s: %w: %s", h.Socket, kind, session, err, output)
	}
	return nil
}

// deckEntersInteractiveModeFittingTo is PRD II-7+II-8's entry sequence,
// run directly against target's tmux session: capture the window's own
// geometry BEFORE doing anything else (stashed under session, for the
// matching exit step), then fit the window -- never the pane -- to
// width/height. This is the exact CaptureWindowGeometry/FitWindowToPane
// pair task 034 shipped, not a re-implementation.
func deckEntersInteractiveModeFittingTo(ctx context.Context, session string, width, height int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	geometry, err := client.CaptureWindowGeometry(ctx, session)
	if err != nil {
		return fmt.Errorf("capture window geometry for session %q: %w", session, err)
	}
	if h.sigwinchCycleGeometries == nil {
		h.sigwinchCycleGeometries = make(map[string]tmux.WindowGeometry)
	}
	h.sigwinchCycleGeometries[session] = geometry
	if _, err := client.FitWindowToPane(ctx, session, session, width, height); err != nil {
		return fmt.Errorf("fit window to pane for session %q at %dx%d: %w", session, width, height, err)
	}
	return nil
}

// deckExitsInteractiveModeOnTmuxSession is PRD II-9's exit sequence,
// restoring whatever the matching deckEntersInteractiveModeFittingTo step
// captured for session -- the exact RestoreWindowGeometry task 035
// shipped, in the load-bearing order that function itself owns.
func deckExitsInteractiveModeOnTmuxSession(ctx context.Context, session string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	geometry, ok := h.sigwinchCycleGeometries[session]
	if !ok {
		return fmt.Errorf("no captured entry geometry for tmux session %q; the enters-interactive-mode step must run first", session)
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	if err := client.RestoreWindowGeometry(ctx, session, geometry); err != nil {
		return fmt.Errorf("restore window geometry for session %q: %w", session, err)
	}
	return nil
}

// rawAttachedTmuxClient is one real `tmux attach-session` client attached
// directly on a scenario's own private socket, through a real pty --
// exactly internal/tmux/restore_test.go's own attachThroughPTY technique,
// reproduced here (not exported from that package, and this is a
// different package) because the features package's own attach steps
// (ScenarioHarness.StartClient et al.) start the released deck BINARY,
// not a bare `tmux attach-session` against a session deck itself never
// created.
type rawAttachedTmuxClient struct {
	terminal *os.File
	cmd      *exec.Cmd
	cancel   context.CancelFunc
}

// aRealTmuxClientAttachesToSessionAt attaches a second, real tmux client
// to session at cols x rows and waits for #{session_attached} to observe
// it, so the SIGWINCH-budget scenario's "attached throughout" half has an
// actual live client PRD II-9's exit gate (#{session_attached}==0) can
// see -- not a simulated one.
func aRealTmuxClientAttachesToSessionAt(ctx context.Context, session string, cols, rows uint16) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	attachCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(attachCtx, "tmux", "-L", h.Socket, "attach-session", "-t", session)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		cancel()
		return fmt.Errorf("attach real tmux client to session %q at %dx%d: %w", session, cols, rows, err)
	}
	// Drain the attached client's own output so it never blocks on a full
	// pty buffer; this scenario asserts on the fake agent's own SIGWINCH
	// counter, never on this client's screen content.
	go func() { _, _ = io.CopyBuffer(io.Discard, terminal, make([]byte, 4096)) }()
	if h.rawAttachedTmuxClients == nil {
		h.rawAttachedTmuxClients = make(map[string]*rawAttachedTmuxClient)
	}
	h.rawAttachedTmuxClients[session] = &rawAttachedTmuxClient{terminal: terminal, cmd: cmd, cancel: cancel}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	return waitForFeatureSessionAttachedCount(ctx, client, session, 1)
}

// theRealTmuxClientAttachedToSessionDetaches sends the tmux detach chord
// and waits for the attach-session process to exit cleanly, so no scenario
// leaves a raw tmux client running past its own steps (KillTMuxServer in
// ScenarioHarness.Close would reap it either way, but an explicit,
// deterministic detach here is what lets a later step in the SAME
// scenario observe #{session_attached} return to 0 if it ever needs to).
func theRealTmuxClientAttachedToSessionDetaches(ctx context.Context, session string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, ok := h.rawAttachedTmuxClients[session]
	if !ok {
		return fmt.Errorf("no real tmux client attached to session %q", session)
	}
	delete(h.rawAttachedTmuxClients, session)
	if _, err := client.terminal.Write([]byte("\x02d")); err != nil {
		client.cancel()
		_ = client.terminal.Close()
		return fmt.Errorf("send detach chord to session %q: %w", session, err)
	}
	done := make(chan error, 1)
	go func() { done <- client.cmd.Wait() }()
	select {
	case waitErr := <-done:
		client.cancel()
		_ = client.terminal.Close()
		if waitErr != nil {
			return fmt.Errorf("tmux attach-session to %q did not exit cleanly after detach: %w", session, waitErr)
		}
		return nil
	case <-time.After(3 * time.Second):
		client.cancel()
		_ = client.terminal.Close()
		return fmt.Errorf("tmux attach-session to %q did not exit within the deadline after detach", session)
	}
}

// waitForFeatureSessionAttachedCount polls target's #{session_attached}
// until it reads want or a deadline expires, the exact shape
// internal/tmux/restore_test.go's own waitForSessionAttachedCount uses,
// reproduced here for the same not-exported-from-that-package reason as
// rawAttachedTmuxClient above.
func waitForFeatureSessionAttachedCount(ctx context.Context, client tmux.Client, target string, want int) error {
	deadline := time.Now().Add(3 * time.Second)
	var last int
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = client.SessionAttachedCount(ctx, target)
		if lastErr == nil && last == want {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("session_attached on %q did not reach %d within the deadline (last=%d err=%v)", target, want, last, lastErr)
}
