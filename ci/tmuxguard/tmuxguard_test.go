// Package tmuxguard pins ci/tmux-guard.sh (task cure-01-01, R158): the shim
// ci/run.sh installs as `tmux` ahead of the real binary must refuse every
// invocation that would reach an operator "deck"/"deck-*" server BEFORE
// anything is executed, with no environment variable able to exempt one,
// and must pass every private-socket invocation through unchanged.
//
// The "real" tmux here is a recording stub, so this test never starts or
// contacts any tmux server at all: a refused call is proven refused by the
// stub never having been executed.
package tmuxguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func guardScript(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "tmux-guard.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("guard script: %v", err)
	}
	return path
}

// runGuard runs the guard with a recording stub standing in for the real
// tmux and returns the guard's exit code plus the argv the stub received
// (nil when the stub never ran).
func runGuard(t *testing.T, env []string, args ...string) (int, []string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	dir := t.TempDir()
	record := filepath.Join(dir, "argv")
	stub := filepath.Join(dir, "real-tmux")
	// The record path is passed through the environment, never spliced
	// into the script text: t.TempDir embeds the subtest name, and names
	// like "$TMUX ..." would otherwise be expanded by the stub's shell.
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > \"$GUARD_TEST_RECORD\"\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, append([]string{guardScript(t)}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "DECK_TEST_REAL_TMUX=" + stub, "GUARD_TEST_RECORD=" + record}, env...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run guard %q: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	data, readErr := os.ReadFile(record)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
		return code, nil
	}
	received := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(data) == 0 {
		received = []string{}
	}
	if code == 97 {
		t.Logf("guard output: %s", out)
	}
	return code, received
}

func TestGuardRefusesOperatorSocketsBeforeExecuting(t *testing.T) {
	// The old exemption variable (and anything like it) must not matter.
	exempt := []string{"DECK_TEST_ALLOW_NAMESPACED_SOCKET=1"}
	cases := []struct {
		name string
		env  []string
		args []string
	}{
		{"-L deck", nil, []string{"-L", "deck", "new-session", "-d"}},
		{"-L deck-a", nil, []string{"-L", "deck-a", "list-sessions"}},
		{"-L deck with the retired exemption set", exempt, []string{"-L", "deck", "new-session", "-d"}},
		{"-L deck-work with the retired exemption set", exempt, []string{"-L", "deck-work", "kill-server"}},
		{"attached -Ldeck", nil, []string{"-Ldeck", "new-session", "-d"}},
		{"attached -Ldeck-x", nil, []string{"-Ldeck-x", "has-session"}},
		{"bundled -uL deck", nil, []string{"-uL", "deck", "new-session", "-d"}},
		{"bundled attached -2Ldeck-b", nil, []string{"-2Ldeck-b", "ls"}},
		{"flags before -L", nil, []string{"-2", "-f", "/dev/null", "-L", "deck-foo", "ls"}},
		{"-L=deck", nil, []string{"-L=deck", "ls"}},
		{"-S operator socket path", nil, []string{"-S", "/tmp/tmux-1000/deck", "ls"}},
		{"attached -S deck-* path", nil, []string{"-S/tmp/tmux-1000/deck-work", "ls"}},
		{"$TMUX names an operator server", []string{"TMUX=/tmp/tmux-1000/deck,4242,0"}, []string{"display-message", "-p", "x"}},
		{"$TMUX names an operator profile server", []string{"TMUX=/tmp/tmux-1000/deck-work,4242,0"}, []string{"list-sessions"}},
		{"private -L first, operator -L later", nil, []string{"-L", "priv-x", "-L", "deck", "ls"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, received := runGuard(t, tc.env, tc.args...)
			if code != 97 {
				t.Fatalf("guard exit = %d, want 97 (refused)", code)
			}
			if received != nil {
				t.Fatalf("refused invocation still reached the real tmux with argv %q", received)
			}
		})
	}
}

func TestGuardPassesPrivateSocketsThroughUnchanged(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		args []string
	}{
		{"private -L", nil, []string{"-L", "priv-cmd-1", "new-session", "-d", "-s", "x"}},
		{"harness scenario socket", nil, []string{"-L", "deck_test_12_3", "ls"}},
		{"alias of a derived name", nil, []string{"-L", "deck_test_12_3-alias-deck-a", "ls"}},
		{"name merely containing deck", nil, []string{"-L", "notdeck", "ls"}},
		{"deck-* only as a session name after the command word", nil, []string{"-L", "priv", "new-session", "-d", "-s", "deck-foo", "-L"}},
		{"version probe", nil, []string{"-V"}},
		{"private -S path", nil, []string{"-S", "/tmp/x/priv-deck", "ls"}},
		{"$TMUX names a private server", []string{"TMUX=/tmp/tmux-1000/priv-1,99,0"}, []string{"display-message", "-p", "x"}},
		{"explicit private -L beats an operator $TMUX", []string{"TMUX=/tmp/tmux-1000/deck,99,0"}, []string{"-L", "priv-2", "ls"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, received := runGuard(t, tc.env, tc.args...)
			if code != 0 {
				t.Fatalf("guard exit = %d, want 0 (passed through)", code)
			}
			if strings.Join(received, "\x00") != strings.Join(tc.args, "\x00") {
				t.Fatalf("real tmux received %q, want %q unchanged", received, tc.args)
			}
		})
	}
}
