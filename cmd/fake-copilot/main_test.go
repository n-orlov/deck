package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestHelpTextNamesTheContract(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		h := newHarness(t)
		got := h.run([]string{flag}, strings.NewReader(""), nil)
		if got.code != 0 || got.stdout != helpText {
			t.Fatalf("%s = (%d, %q), want the help text and exit 0", flag, got.code, got.stdout)
		}
		if h.exists(h.home) {
			t.Fatalf("%s created %s", flag, h.home)
		}
	}
	if isHelpRequest([]string{"--help", "x"}) || isHelpRequest(nil) {
		t.Fatal("isHelpRequest must match exactly one help flag")
	}
}

// R224 item 1: the argv deck builds is accepted.
func TestAcceptsTheArgvDeckBuilds(t *testing.T) {
	cases := map[string][]string{
		"safe":  {"--session-id", testSessionID, "--plugin-dir", "/p", "--no-auto-update"},
		"edits": {"--session-id", testSessionID, "--allow-tool=write", "--plugin-dir", "/p", "--no-auto-update"},
		"yolo":  {"--session-id=" + testSessionID, "--allow-all", "--plugin-dir=/p", "--no-auto-update"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			got := h.run(args, strings.NewReader(""), nil)
			if got.code != 0 || got.stderr != "" {
				t.Fatalf("run = (%d, stderr %q), want a clean accept", got.code, got.stderr)
			}
			for _, want := range []string{"Fake copilot\n", "fake-copilot session-id: " + testSessionID + "\n", "fake-copilot session: new\n", "fake-copilot plugin-dir: /p\n"} {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("banner %q lacks %q", got.stdout, want)
				}
			}
			if strings.Contains(name, "yolo") != strings.Contains(got.stdout, "allow-all: true") {
				t.Errorf("allow-all banner line wrong for %s: %q", name, got.stdout)
			}
			if name == "edits" && !strings.Contains(got.stdout, "fake-copilot allow-tool: write\n") {
				t.Errorf("edits banner lacks allow-tool: %q", got.stdout)
			}
		})
	}
}

func TestBannerRecordsTheExactArgv(t *testing.T) {
	h := newHarness(t)
	args := []string{"--session-id", testSessionID, "--no-auto-update"}
	got := h.run(args, strings.NewReader(""), nil)
	want := `fake-copilot argv: ["--session-id","` + testSessionID + `","--no-auto-update"]` + "\n"
	if !strings.Contains(got.stdout, want) {
		t.Fatalf("banner %q lacks %q", got.stdout, want)
	}
}

// R224 item 1: every flag R216 forbids is rejected, with Copilot's wording for
// an argument its parser does not know, exit 1, and no session left behind.
func TestRejectsEveryFlagDeckNeverBuilds(t *testing.T) {
	for _, flag := range []string{"--continue", "--resume", "--connect", "--remote", "--acp", "--yolo", "--bogus"} {
		for _, form := range [][]string{{flag}, {flag + "=" + testSessionID}} {
			h := newHarness(t)
			args := append([]string{"--session-id", testSessionID}, form...)
			got := h.run(args, strings.NewReader(""), nil)
			want := "error: unexpected argument '" + form[0] + "' found\n\nFor more information, try '--help'.\n"
			if got.code != 1 || got.stderr != want || got.stdout != "" {
				t.Errorf("%v = (%d, stderr %q, stdout %q), want exit 1 with %q", args, got.code, got.stderr, got.stdout, want)
			}
			if h.exists(h.home) {
				t.Errorf("%v created a session although it was rejected", args)
			}
		}
	}
}

func TestRejectsAPositionalArgument(t *testing.T) {
	h := newHarness(t)
	got := h.run([]string{"hello"}, strings.NewReader(""), nil)
	if got.code != 1 || !strings.Contains(got.stderr, "unexpected argument 'hello' found") {
		t.Fatalf("run = (%d, %q), want a rejection", got.code, got.stderr)
	}
}

func TestAFlagMissingItsValueIsRejected(t *testing.T) {
	for _, flag := range []string{"--session-id", "--plugin-dir", "--allow-tool"} {
		h := newHarness(t)
		got := h.run([]string{flag}, strings.NewReader(""), nil)
		want := "error: a value is required for '" + flag + " <VALUE>' but none was supplied\n\nFor more information, try '--help'.\n"
		if got.code != 1 || got.stderr != want {
			t.Errorf("%s = (%d, %q), want exit 1 with %q", flag, got.code, got.stderr, want)
		}
	}
}

func TestExitCodeEnvironment(t *testing.T) {
	cases := map[string]int{"": 0, "7": 7, "125": 125}
	for value, want := range cases {
		h := newHarness(t)
		got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), map[string]string{exitCodeEnvironment: value})
		if got.code != want {
			t.Errorf("%s=%q exit = %d, want %d", exitCodeEnvironment, value, got.code, want)
		}
	}
	for _, bad := range []string{"x", "-1", "126"} {
		h := newHarness(t)
		got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), map[string]string{exitCodeEnvironment: bad})
		if got.code != 2 || !strings.Contains(got.stderr, exitCodeEnvironment) {
			t.Errorf("%s=%q = (%d, %q), want exit 2 naming the variable", exitCodeEnvironment, bad, got.code, got.stderr)
		}
	}
}

func TestCopilotRootPrefersCopilotHomeThenHome(t *testing.T) {
	env := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	if got := copilotRoot(env(map[string]string{"COPILOT_HOME": "/c", "HOME": "/h"})); got != "/c" {
		t.Errorf("root = %q, want /c", got)
	}
	if got := copilotRoot(env(map[string]string{"HOME": "/h"})); got != filepath.Join("/h", ".copilot") {
		t.Errorf("root = %q, want /h/.copilot", got)
	}
	if got := copilotRoot(env(nil)); got != "" {
		t.Errorf("root = %q, want empty", got)
	}
}

func TestLaunchWithNoHomeAtAllFails(t *testing.T) {
	h := newHarness(t)
	var stdout, stderr strings.Builder
	code := runWithIO([]string{"--session-id", testSessionID}, strings.NewReader(""), &stdout, &stderr, func(string) string { return "" }, func() (string, error) { return h.cwd, nil }, nil)
	if code != 2 || !strings.Contains(stderr.String(), "COPILOT_HOME") {
		t.Fatalf("run = (%d, %q), want exit 2 naming COPILOT_HOME", code, stderr.String())
	}
}

func TestLaunchWithAnUnwritableHomeFails(t *testing.T) {
	h := newHarness(t)
	blocker := filepath.Join(filepath.Dir(h.home), "blocker")
	write(t, blocker, "a file where a directory must go")
	got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), map[string]string{"COPILOT_HOME": blocker})
	if got.code != 2 || !strings.Contains(got.stderr, "fake-copilot:") {
		t.Fatalf("run = (%d, %q), want exit 2", got.code, got.stderr)
	}
}

func TestLaunchWithNoWorkingDirectoryFails(t *testing.T) {
	h := newHarness(t)
	var stdout, stderr strings.Builder
	code := runWithIO(nil, strings.NewReader(""), &stdout, &stderr, h.getenv(nil), func() (string, error) { return "", os.ErrNotExist }, nil)
	if code != 2 || !strings.Contains(stderr.String(), "resolve cwd") {
		t.Fatalf("run = (%d, %q), want exit 2", code, stderr.String())
	}
}

// R224 item 2, the part a process kill decides: events.jsonl never appears
// after SIGKILL, and workspace.yaml (written at launch) does.
func TestSIGKILLLeavesWorkspaceYamlButNoEvents(t *testing.T) {
	h := newHarness(t)
	cmd, _ := h.startProcess(t, "--session-id", testSessionID)
	waitFor(t, "workspace.yaml", func() bool { return h.exists(h.sessionDir(testSessionID), "workspace.yaml") })
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if h.exists(h.sessionDir(testSessionID), "events.jsonl") {
		t.Fatal("events.jsonl exists after SIGKILL before any prompt")
	}
}

// The same process, signalled cleanly, completes the transcript.
func TestTerminationSignalIsACleanShutdown(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		h := newHarness(t)
		cmd, _ := h.startProcess(t, "--session-id", testSessionID)
		waitFor(t, "workspace.yaml", func() bool { return h.exists(h.sessionDir(testSessionID), "workspace.yaml") })
		// The handler is installed before main reads its args; give the process a
		// moment to be past startup by waiting for the session directory above.
		if err := cmd.Process.Signal(sig); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("%v: process exited with %v, want a clean exit", sig, err)
		}
		got := eventTypes(t, filepath.Join(h.sessionDir(testSessionID), "events.jsonl"))
		if want := []string{"session.start", "session.model_change", "session.shutdown"}; !equal(got, want) {
			t.Errorf("%v: events = %v, want %v", sig, got, want)
		}
	}
}
