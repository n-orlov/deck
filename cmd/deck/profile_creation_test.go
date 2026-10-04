package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestRunRefusesUnknownProfileWhenStdinIsNotATerminal pins SPEC §3.4's
// "if stdin is not a terminal deck refuses" rule (R153): a strings.Reader
// stdin is never an *os.File, so confirmAndCreateProfile's own isFile
// check can never see it as a terminal -- exactly the shape run()'s real
// non-interactive callers (a script, a CI job, `deck work < /dev/null`)
// present. The exact prompt text is still printed (SPEC: "refuses with the
// same message"), exit is 1 (never 2, never 0), and nothing is created --
// launch order stops at confirm, never reaching config.LoadFromProfile's
// own directory-touching callers below it.
func TestRunRefusesUnknownProfileWhenStdinIsNotATerminal(t *testing.T) {
	env := tempDeckEnv(t)
	code, _, stderr := runWithEnv(t, env, "work")
	if code != 1 {
		t.Fatalf("run(deck work) with non-TTY stdin, unknown profile: exit = %d, want 1 (stderr=%q)", code, stderr)
	}
	want := `deck: no profile "work" yet (known: default). Create it? [y/N]`
	if !strings.Contains(stderr, want) {
		t.Fatalf("run(deck work) stderr = %q, want it to contain %q", stderr, want)
	}
	assertEmptyDirs(t, env)
}

// buildDeckProfileCreationTestBinary builds the deck binary this file's
// own pty-driven tests exec, the same way cmd/deck's other pty tests do
// (see buildDeckInteractiveCleanupTestBinary/TestDeckBinaryShellCreate...
// ThroughPTY): a real terminal is the only way to exercise
// confirmAndCreateProfile's term.IsTerminal branch honestly, since an
// in-process *os.File stdin would need the real controlling terminal.
func buildDeckProfileCreationTestBinary(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary: %v\n%s", err, output)
	}
	return binary
}

// startProfileCreationPTY starts the deck binary against an unknown named
// profile ("work") over a real pty, waits for SPEC §3.4's exact creation
// prompt to appear, and returns the running process for the caller to
// answer.
func startProfileCreationPTY(t *testing.T, binary, home string) (terminal *os.File, output *ptyOutput, done <-chan error, cancel func()) {
	t.Helper()
	terminal, output, done, cancel = launchProfileCreationPTY(t, binary, home)
	waitForScreen(t, output, done, `deck: no profile "work" yet (known: default). Create it? [y/N]`)
	return terminal, output, done, cancel
}

// launchProfileCreationPTY starts the deck binary against an unknown named
// profile ("work") over a real pty and returns immediately, without
// waiting for the confirmation prompt -- unlike startProfileCreationPTY,
// so a caller that needs two such processes racing the SAME unknown
// profile (R153's two-pending-confirmations regression) can get both
// running concurrently before either one's own ~5s terminal-query delay
// has elapsed, rather than paying that delay twice in series and starving
// the first process's own context timeout.
func launchProfileCreationPTY(t *testing.T, binary, home string) (terminal *os.File, output *ptyOutput, done <-chan error, cancel func()) {
	t.Helper()
	ctx, cancelCtx := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(ctx, binary, "work")
	// A private DECK_TMUX_SOCKET (task cure-01-01, R158): without it an
	// accepted "work" profile derives socket "deck-work" and the TUI it
	// opens queries that server -- the operator's own namespace, which
	// ci/tmux-guard.sh refuses. DECK_TMUX_SOCKET wins outright over the
	// derivation (SPEC §3.4), and nothing here asserts the socket name;
	// the derivation itself is pinned in internal/config and by the
	// features/ profile scenarios.
	sum := fnv.New32a()
	sum.Write([]byte(home))
	socket := fmt.Sprintf("priv-profile-creation-%d-%08x", os.Getpid(), sum.Sum32())
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket, "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		cancelCtx()
		t.Fatal(err)
	}
	out := newPTYOutput()
	go pumpTerminal(out, terminal)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
		cancelCtx()
	}()
	return terminal, out, doneCh, func() {
		cancelCtx()
		terminal.Close()
		// Never leave the private server behind, should one have started.
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	}
}

// waitForProcessExit blocks until done reports the process has exited (or
// fails the test after timeout), returning the process's exit code.
func waitForProcessExit(t *testing.T, done <-chan error, timeout time.Duration) int {
	t.Helper()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		t.Fatalf("wait for deck to exit: %v", err)
	case <-time.After(timeout):
		t.Fatal("deck did not exit in time")
	}
	return -1
}

// assertNoProfileDir fails the test if home/profiles/<name> exists at all
// (SPEC: "anything else exits 1 and creates nothing").
func assertNoProfileDir(t *testing.T, home, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, "profiles", name)); !os.IsNotExist(err) {
		t.Fatalf("profiles/%s exists (err=%v), want it never created", name, err)
	}
}

// TestDeckBinaryProfileCreationDeclinedThroughPTY pins SPEC §3.4: on a real
// terminal, "only y creates it; anything else exits 1 and creates nothing".
// "n" is the plain decline; "Y", "yes", " y" and an empty line are the
// near-misses a case-folding or prefix/trim-happy check would wrongly
// accept, so each must also exit 1 with no profiles/work directory.
func TestDeckBinaryProfileCreationDeclinedThroughPTY(t *testing.T) {
	binary := buildDeckProfileCreationTestBinary(t)
	for _, tc := range []struct{ name, answer string }{
		{"n", "n\n"},
		{"uppercase Y", "Y\n"},
		{"yes", "yes\n"},
		{"leading space y", " y\n"},
		{"empty line", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Parallel: each subtest spends ~5s before the prompt in
			// bubbletea's init-time background-colour query, which this
			// bare pty never answers; the subtests share nothing but the
			// read-only binary.
			t.Parallel()
			home := t.TempDir()
			terminal, output, done, cancel := startProfileCreationPTY(t, binary, home)
			defer cancel()
			if _, err := terminal.Write([]byte(tc.answer)); err != nil {
				t.Fatal(err)
			}
			code := waitForProcessExit(t, done, 5*time.Second)
			if code != 1 {
				t.Fatalf("deck work, answered %q, exit = %d, want 1\noutput: %q", tc.answer, code, output.String())
			}
			assertNoProfileDir(t, home, "work")
		})
	}
}

// TestDeckBinaryProfileCreationAcceptedThroughPTY pins SPEC §3.4's "y"
// path: creation makes the profile's config, data and log directories and
// copies the default profile's config.toml once, byte-for-byte, and the
// copy is independent from then on -- editing one profile's file never
// touches the other's bytes.
func TestDeckBinaryProfileCreationAcceptedThroughPTY(t *testing.T) {
	binary := buildDeckProfileCreationTestBinary(t)
	home := t.TempDir()
	defaultConfig := "# deck default profile marker eR2q9\nascii = true\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(defaultConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	terminal, output, done, cancel := startProfileCreationPTY(t, binary, home)
	defer cancel()
	if _, err := terminal.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}

	workConfig := filepath.Join(home, "profiles", "work", "config.toml")
	workData := filepath.Join(home, "profiles", "work")
	workLog := filepath.Join(home, "profiles", "work", "log")
	waitForPath(t, workLog, 5*time.Second)
	if info, err := os.Stat(workData); err != nil || !info.IsDir() {
		t.Fatalf("profiles/work data dir = %v, %v", info, err)
	}
	if info, err := os.Stat(workLog); err != nil || !info.IsDir() {
		t.Fatalf("profiles/work log dir = %v, %v", info, err)
	}
	got := waitForFileContent(t, workConfig, []byte(defaultConfig), 5*time.Second)
	if !bytes.Equal(got, []byte(defaultConfig)) {
		t.Fatalf("copied config = %q, want byte-identical %q", got, defaultConfig)
	}

	// Now the process is past creation and marching toward the real TUI
	// startup (tmux, terminal queries) -- this test only needs the
	// filesystem result, so it stops the process here rather than driving
	// the rest of that dance.
	cancel()
	<-done
	_ = output

	// Independence: editing the named profile's own copy must never touch
	// default's original bytes, and vice versa.
	if err := os.WriteFile(workConfig, []byte("# edited only in work\nascii = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaultBytes, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatalf("read default config after editing work's copy: %v", err)
	}
	if !bytes.Equal(defaultBytes, []byte(defaultConfig)) {
		t.Fatalf("default config after editing work's copy = %q, want unchanged %q", defaultBytes, defaultConfig)
	}
}

// TestDeckBinaryProfileCreationWithNoDefaultConfigCreatesNoConfigFile pins
// SPEC §3.4: "copies the default profile's config.toml once, if it
// exists" -- when default has no config.toml, the new profile gets none
// either (not an empty file, not an error), even though its data and log
// directories are still created together.
func TestDeckBinaryProfileCreationWithNoDefaultConfigCreatesNoConfigFile(t *testing.T) {
	binary := buildDeckProfileCreationTestBinary(t)
	home := t.TempDir()
	// Deliberately no config.toml written under home: default has none.
	terminal, output, done, cancel := startProfileCreationPTY(t, binary, home)
	defer cancel()
	if _, err := terminal.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	workData := filepath.Join(home, "profiles", "work")
	waitForPath(t, filepath.Join(workData, "log"), 5*time.Second)
	cancel()
	<-done
	_ = output
	if _, err := os.Stat(filepath.Join(workData, "log")); err != nil {
		t.Fatalf("profiles/work/log missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workData, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("profiles/work/config.toml stat = %v, want os.IsNotExist", err)
	}
}

// TestReviewTwoPendingConfirmationsDoNotOverwriteProfileConfig pins R153:
// two real deck processes racing the same not-yet-known profile can both
// reach SPEC §3.4's confirm prompt before either creates anything (both
// saw config.ProfileExists == false, or one's directory only appeared
// while the other was already waiting on its own prompt). Answering the
// first process's prompt "y" creates profiles/work and its config.toml
// copy; something -- the profile's own owner, in this scenario -- then
// edits that file. Answering the SECOND, already-pending process's prompt
// "y" must not re-copy the default config over those edited bytes: R153's
// bug was a plain os.ReadFile+os.WriteFile in config.CreateProfile, which
// silently clobbered the first owner's edit every time a second launch's
// confirmation lost the race. The fix makes the copy a race-safe
// create-if-absent (os.OpenFile with O_CREATE|O_EXCL): a second, later
// caller for the same profile finds the file already there and leaves it
// exactly as found, no matter how its own confirmation happened to be
// pending when the first one won.
func TestReviewTwoPendingConfirmationsDoNotOverwriteProfileConfig(t *testing.T) {
	binary := buildDeckProfileCreationTestBinary(t)
	home := t.TempDir()
	defaultConfig := "# default\n[ui]\nascii = true\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(defaultConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	// Both processes reach the confirm prompt for the same unknown profile
	// before either has created anything -- the "two pending confirmations"
	// this regression is named for. Launched together, before waiting on
	// either one's own ~5s terminal-query delay, so they are genuinely
	// racing rather than serialized (and so neither starves the other's own
	// 10s process context while this goroutine waits on the first).
	terminal1, output1, done1, cancel1 := launchProfileCreationPTY(t, binary, home)
	defer cancel1()
	terminal2, output2, done2, cancel2 := launchProfileCreationPTY(t, binary, home)
	defer cancel2()
	prompt := `deck: no profile "work" yet (known: default). Create it? [y/N]`
	waitForScreen(t, output1, done1, prompt)
	waitForScreen(t, output2, done2, prompt)

	workConfig := filepath.Join(home, "profiles", "work", "config.toml")

	// The first owner's "y" creates the profile and its default-copied
	// config; that owner then edits it -- e.g. a settings save between
	// launch and this profile's next start.
	if _, err := terminal1.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	waitForFileContent(t, workConfig, []byte(defaultConfig), 5*time.Second)
	editedConfig := []byte("# edited by profile work's own owner\n[ui]\nascii = false\n")
	if err := os.WriteFile(workConfig, editedConfig, 0o600); err != nil {
		t.Fatal(err)
	}

	// The second, already-pending confirmation's "y" must not re-copy the
	// default config over the edit above.
	if _, err := terminal2.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	assertFileContentStaysStable(t, workConfig, editedConfig, 1*time.Second)

	cancel1()
	cancel2()
	<-done1
	<-done2
	_, _ = output1, output2
}

// assertFileContentStaysStable polls path for window, failing the test the
// moment its bytes stop matching want -- catching a re-copy landing at any
// point inside the window, not only a snapshot taken once at the end.
func assertFileContentStaysStable(t *testing.T, path string, want []byte, window time.Duration) {
	t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s during settle window: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("content of %s changed during settle window: got %q, want it to stay %q", path, got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitForPath polls until path exists (any type) or fails the test after
// timeout.
func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s to exist", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitForFileContent polls path until it exists and its bytes equal want,
// or fails the test after timeout -- CreateProfile's directory-then-copy
// ordering means a bare existence check can observe the file microseconds
// before its content write lands.
func waitForFileContent(t *testing.T, path string, want []byte, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []byte
	var lastErr error
	for {
		last, lastErr = os.ReadFile(path)
		if lastErr == nil && bytes.Equal(last, want) {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s to contain %q (last read: %q, %v)", path, want, last, lastErr)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
