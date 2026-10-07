package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

const (
	testSessionID = "11111111-2222-4333-8444-555555555555"
	// runMainEnv makes the test binary behave as the fake itself, so a test can
	// signal a real process (a SIGKILL cannot be delivered to a function call).
	runMainEnv = "FAKE_COPILOT_TEST_RUN_MAIN"
)

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

type outcome struct {
	code           int
	stdout, stderr string
}

// harness is one temporary COPILOT_HOME, working directory and capture file.
type harness struct {
	t       *testing.T
	home    string
	cwd     string
	capture string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, home: filepath.Join(dir, "copilot-home"), cwd: filepath.Join(dir, "work"), capture: filepath.Join(dir, "captured.txt")}
	if err := os.MkdirAll(h.cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) getenv(extra map[string]string) func(string) string {
	return func(key string) string {
		if value, ok := extra[key]; ok {
			return value
		}
		if key == "COPILOT_HOME" {
			return h.home
		}
		return ""
	}
}

func (h *harness) run(args []string, stdin io.Reader, env map[string]string) outcome {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWithIO(args, stdin, &stdout, &stderr, h.getenv(env), func() (string, error) { return h.cwd, nil }, nil)
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (h *harness) sessionDir(id string) string { return filepath.Join(h.home, "session-state", id) }

func (h *harness) exists(parts ...string) bool {
	_, err := os.Stat(filepath.Join(parts...))
	return err == nil
}

// installPlugin writes deck's real plugin directory and a DECK_EXE that records
// every `_hook` call (its DECK_HOOK_EVENT and stdin) to the capture file.
func (h *harness) installPlugin(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(h.home), "plugin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "plugin.json"), agent.CopilotPluginManifest)
	write(t, filepath.Join(dir, "hooks.json"), agent.CopilotHooksConfig())
	exe := filepath.Join(filepath.Dir(h.home), "deck-exe")
	write(t, exe, "#!/bin/sh\n[ \"$1\" = _hook ] || exit 3\nbody=$(cat)\nprintf '%s\\t%s\\n' \"$DECK_HOOK_EVENT\" \"$body\" >> \"$HOOK_CAPTURE\"\n")
	if err := os.Chmod(exe, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DECK_EXE", exe)
	t.Setenv("HOOK_CAPTURE", h.capture)
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// capturedHook is one `_hook` call a plugin entry made.
type capturedHook struct {
	event   string
	payload map[string]any
}

func (h *harness) captured(t *testing.T) []capturedHook {
	t.Helper()
	data, err := os.ReadFile(h.capture)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []capturedHook
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		name, body, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("capture line %q has no tab", line)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("capture line %q is not a JSON payload: %v", line, err)
		}
		calls = append(calls, capturedHook{event: name, payload: payload})
	}
	return calls
}

func (h *harness) capturedEvents(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, call := range h.captured(t) {
		names = append(names, call.event)
	}
	return names
}

func eventTypes(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("events.jsonl line %q: %v", line, err)
		}
		types = append(types, record.Type)
	}
	return types
}

func equal(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// startProcess runs this test binary as the fake, in commands mode, with a
// stdin the test keeps open.
func (h *harness) startProcess(t *testing.T, args ...string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...) //nolint:gosec // G204: re-executes this test binary as the fake
	cmd.Dir = h.cwd
	cmd.Env = append(os.Environ(), runMainEnv+"=1", "COPILOT_HOME="+h.home, commandsEnvironment+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(syscall.SIGKILL); _ = cmd.Wait() })
	return cmd, stdin
}
