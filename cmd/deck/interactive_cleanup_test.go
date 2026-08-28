package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// interactiveCleanupClaimRecord mirrors internal/tmux.InteractiveClaimRecord's
// JSON shape without importing an unexported detail: only WindowTarget is
// actually read here, to match a leaked temp dir back to the one window
// each of these tests cares about.
type interactiveCleanupClaimRecord struct {
	WindowTarget string `json:"window_target"`
}

// findInteractivePipeTempDir scans the real OS temp directory -- exactly
// where tmux.ArmPipePane's own os.MkdirTemp call and
// tmux.ReclaimLeakedInteractivePipes' own scan both look -- for a
// deck-interactive-pipe-* dir whose own claim.json names windowTarget.
// Mirrors features/interactive_pipe_leak_test.go's
// findDeckInteractivePipeTempDir (a different package, so duplicated
// rather than imported).
func findInteractivePipeTempDir(t *testing.T, windowTarget string) string {
	t.Helper()
	root := os.TempDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("scan %q for a deck interactive pipe temp dir: %v", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "deck-interactive-pipe-") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "claim.json"))
		if err != nil {
			continue
		}
		var record interactiveCleanupClaimRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		if record.WindowTarget == windowTarget {
			return dir
		}
	}
	return ""
}

// soleSessionWindowTarget opens the store at home and resolves the tmux
// window target (tmux.SessionName) of the one session it expects to find,
// the same way enterInteractive itself resolves it from a store row's own
// slug.
func soleSessionWindowTarget(t *testing.T, home string) string {
	t.Helper()
	db, err := store.Open(config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	rows, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListSessions = %d rows, want exactly 1", len(rows))
	}
	target, err := tmux.SessionName(rows[0].Slug)
	if err != nil {
		t.Fatalf("resolve tmux session name for slug %q: %v", rows[0].Slug, err)
	}
	return target
}

// windowOwnershipIsUnset reports whether tmux's own @deck_isize_owner
// window option (tmux.OwnershipOption) reads as unset for target, straight
// from a real `show-options`, the same "invalid option" exit-code shape
// features/tmux_option_scope_test.go's own readTmuxOptionInScope already
// relies on for an unset "@"-prefixed user option.
func windowOwnershipIsUnset(t *testing.T, socket, target string) bool {
	t.Helper()
	output, err := exec.Command("tmux", "-L", socket, "show-options", "-wv", "-t", target, tmux.OwnershipOption).CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if strings.Contains(trimmed, "invalid option") {
			return true
		}
		t.Fatalf("tmux -L %s show-options -wv -t %s %s: %v: %s", socket, target, tmux.OwnershipOption, err, trimmed)
	}
	return trimmed == ""
}

// startDeckInteractiveFixture builds binary (with extra build tags, if
// any), starts it against a fresh tmux socket/DECK_HOME, creates one shell
// session, waits for it to report "running" and enters interactive mode --
// exactly the shared setup PRD R89/task 031's SIGTERM and panic tests both
// need before they diverge on how the process actually exits.
type interactiveCleanupFixture struct {
	home, socket, windowTarget string
	before                     tmux.WindowGeometry
	terminal                   *os.File
	output                     *ptyOutput
	done                       <-chan error
	cmd                        *exec.Cmd
}

func startDeckInteractiveFixture(t *testing.T, binary, socketPrefix string, extraEnv ...string) *interactiveCleanupFixture {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	socket := socketPrefix + "-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(),
		"DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket, "DECK_RECONCILE_MS=100",
		"NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	cmd.Env = append(cmd.Env, extraEnv...)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatalf("start deck under pty: %v", err)
	}
	output := newPTYOutput()
	go io.Copy(output, terminal)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatalf("answer terminal queries: %v", err)
	}
	waitForScreen(t, output, done, "No sessions yet")
	if _, err := terminal.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "Create shell session")
	if _, err := terminal.Write([]byte("leaky\t" + cwd + "\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreenWithin(t, output, done, "running", 10*time.Second)

	windowTarget := soleSessionWindowTarget(t, home)
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	before, err := client.CaptureWindowGeometry(context.Background(), windowTarget)
	if err != nil {
		t.Fatalf("capture window geometry before entering interactive mode: %v", err)
	}

	if _, err := terminal.Write([]byte("\r")); err != nil {
		t.Fatalf("send enter to enter interactive mode: %v", err)
	}
	waitForScreenWithin(t, output, done, "Ctrl+Q", 5*time.Second)

	armed, err := client.PanePipe(context.Background(), windowTarget)
	if err != nil {
		t.Fatalf("PanePipe: %v", err)
	}
	if !armed {
		t.Fatalf("test assumption violated: pipe-pane is not armed right after entering interactive mode")
	}
	if dir := findInteractivePipeTempDir(t, windowTarget); dir == "" {
		t.Fatalf("test assumption violated: no interactive pipe temp dir on disk naming window target %q right after entering interactive mode", windowTarget)
	}
	if windowOwnershipIsUnset(t, socket, windowTarget) {
		t.Fatalf("test assumption violated: window ownership is unset right after entering interactive mode")
	}

	return &interactiveCleanupFixture{
		home: home, socket: socket, windowTarget: windowTarget, before: before,
		terminal: terminal, output: output, done: done, cmd: cmd,
	}
}

// assertInteractiveClaimTornDown is the shared assertion both the SIGTERM
// and panic tests make once deck has actually exited: no armed pipe-pane,
// no leaked temp dir, no ownership claim and the window's own geometry
// restored byte-exact -- PRD R89/task 031's own success criteria, checked
// the same way regardless of which exit route got there.
func (f *interactiveCleanupFixture) assertInteractiveClaimTornDown(t *testing.T) {
	t.Helper()
	client := tmux.Client{Socket: f.socket, Timeout: 5 * time.Second}
	armed, err := client.PanePipe(context.Background(), f.windowTarget)
	if err != nil {
		t.Fatalf("PanePipe after exit: %v", err)
	}
	if armed {
		t.Fatalf("pipe-pane is still armed for %q after deck exited, want disarmed", f.windowTarget)
	}
	if dir := findInteractivePipeTempDir(t, f.windowTarget); dir != "" {
		t.Fatalf("interactive pipe temp dir %q still on disk naming window target %q after deck exited, want removed", dir, f.windowTarget)
	}
	if !windowOwnershipIsUnset(t, f.socket, f.windowTarget) {
		t.Fatalf("window ownership is still set for %q after deck exited, want released", f.windowTarget)
	}
	after, err := client.CaptureWindowGeometry(context.Background(), f.windowTarget)
	if err != nil {
		t.Fatalf("capture window geometry after exit: %v", err)
	}
	if after != f.before {
		t.Fatalf("window geometry after exit = %+v, want byte-exact restore of %+v", after, f.before)
	}
}

// buildDeckInteractiveCleanupTestBinary builds the deck binary this
// package's own other tests already build inline (TestDeckBinary...
// ThroughPTY), factored out here because both tests below need it and one
// of them additionally needs the decktestpanic build tag.
func buildDeckInteractiveCleanupTestBinary(t *testing.T, tags ...string) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	args := []string{"build"}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, "-o", binary, ".")
	build := exec.Command("go", args...)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary (tags %v): %v\n%s", tags, err, output)
	}
	return binary
}

// TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow is PRD
// R89/task 031's SIGTERM exit route: unlike SIGKILL (task 030,
// features/interactive_pipe_leak.feature -- "the next start reclaims it"),
// SIGTERM still runs Go code in this same process before it exits, so the
// guarantee here is "the exit itself cleans it up", proven directly
// against the process that was interactive, never a later "rescuer".
//
// Bubble Tea's own SIGTERM handling turns the signal into a QuitMsg that
// Program.Run's eventLoop returns straight through without ever calling
// Update again (see cmd/deck/interactive_shutdown.go's own doc comment for
// why) -- so this specifically exercises main.go's post-Run()
// shutdownArmedInteractiveClaim call, not the panic-recovering wrapper the
// sibling test below exercises.
func TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow(t *testing.T) {
	binary := buildDeckInteractiveCleanupTestBinary(t)
	fixture := startDeckInteractiveFixture(t, binary, "deck-sigterm-interactive")

	if err := fixture.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case err := <-fixture.done:
		if err != nil {
			t.Fatalf("deck did not exit cleanly after SIGTERM: %v\noutput: %q", err, fixture.output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("deck did not exit after SIGTERM\noutput: %q", fixture.output.String())
	}
	_ = fixture.terminal.Close()

	fixture.assertInteractiveClaimTornDown(t)
}

// TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow is PRD
// R89/task 031's panic exit route. It reuses the exact same
// DECK_TEST_PANIC_KEY/decktestpanic mechanism
// features/mouse_exit_paths_test.go's TestMouseReportingDisabledOnPanic
// already relies on for requirement 36's unrelated proof (mouse reporting
// disabled on panic): testpanic_hook.go's panicOnKeyModel panics BEFORE
// ever delegating to the model it wraps, which is exactly why
// cmd/deck/interactive_shutdown.go's own recover has to sit OUTSIDE every
// other wrap (and why panicOnKeyModel now has its own Unwrap method) --
// this test is what proves that composition actually reaches the armed
// interactive claim rather than merely compiling.
func TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow(t *testing.T) {
	binary := buildDeckInteractiveCleanupTestBinary(t, "decktestpanic")
	fixture := startDeckInteractiveFixture(t, binary, "deck-panic-interactive", "DECK_TEST_PANIC_KEY=z")

	// The deliberate panic key is forwarded like any other interactive
	// keystroke (updateInteractive forwards everything except Ctrl+Q) --
	// this wrapper intercepts it one layer further out, before deck's own
	// Update ever sees it, so the pane target never actually receives a
	// literal "z".
	if _, err := fixture.terminal.Write([]byte("z")); err != nil {
		t.Fatalf("send the deliberate-panic key: %v", err)
	}
	select {
	case <-fixture.done:
		// main.go reports a panic to stderr but still exits 0 (same as
		// TestMouseReportingDisabledOnPanic's own expectation) -- the
		// process actually exiting at all, with the panic visible in its
		// own output checked below, is what this asserts, not a
		// particular exit code.
	case <-time.After(5 * time.Second):
		t.Fatalf("deck did not exit after the deliberate panic\noutput: %q", fixture.output.String())
	}
	_ = fixture.terminal.Close()
	raw := fixture.output.String()
	if !strings.Contains(raw, "Caught panic") || !strings.Contains(raw, "deliberate test panic") {
		t.Fatalf("the induced panic was not reported anywhere in deck's own output: %q", raw)
	}

	fixture.assertInteractiveClaimTornDown(t)
}
