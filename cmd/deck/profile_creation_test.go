package main

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	ctx, cancelCtx := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(ctx, binary, "work")
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		cancelCtx()
		t.Fatal(err)
	}
	out := newPTYOutput()
	go io.Copy(out, terminal)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
		cancelCtx()
	}()
	waitForScreen(t, out, doneCh, `deck: no profile "work" yet (known: default). Create it? [y/N]`)
	return terminal, out, doneCh, func() {
		cancelCtx()
		terminal.Close()
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
