package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func testGetwd(t *testing.T) func() (string, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	return os.Getwd
}

func testGetenv(home string) func(string) string {
	return func(key string) string {
		if key == "HOME" {
			return home
		}
		return ""
	}
}

const firstUUID = "123e4567-e89b-12d3-a456-426614174000"
const secondUUID = "123e4567-e89b-12d3-a456-426614174001"

func TestAcceptedClaudeFlagsProduceDeterministicTerminalRecord(t *testing.T) {
	var first, second bytes.Buffer
	arguments := []string{"--session-id", firstUUID, "--resume", secondUUID, "--permission-mode", "acceptEdits", "write tests"}
	getwd := testGetwd(t)
	if code, err := run(arguments, &first, testGetenv(t.TempDir()), getwd); err != nil || code != 0 {
		t.Fatalf("first run = (%d, %v)", code, err)
	}
	if code, err := run(arguments, &second, testGetenv(t.TempDir()), getwd); err != nil || code != 0 {
		t.Fatalf("second run = (%d, %v)", code, err)
	}
	if first.String() != second.String() {
		t.Fatalf("terminal output is not deterministic:\nfirst: %q\nsecond: %q", first.String(), second.String())
	}
	for _, want := range []string{"Fake Claude Code", `fake-claude argv: ["--session-id","` + firstUUID, "fake-claude permission-mode: acceptEdits"} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("output %q does not contain %q", first.String(), want)
		}
	}
}

func TestRejectsInvalidUUIDsUnknownFlagsAndModes(t *testing.T) {
	for name, arguments := range map[string][]string{
		"malformed session ID": {"--session-id", "not-a-uuid"},
		"malformed resume":     {"--resume", "not-a-uuid"},
		"unknown flag":         {"--not-a-real-flag", "value"},
		"unknown mode":         {"--permission-mode", "unsafe"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run(arguments, &output, testGetenv(t.TempDir()), testGetwd(t)); err == nil {
				t.Fatalf("run(%q) unexpectedly succeeded", arguments)
			}
		})
	}
}

func TestExitCodeIsControlledOnlyByFixtureEnvironment(t *testing.T) {
	var output bytes.Buffer
	getenv := func(key string) string {
		if key == exitCodeEnvironment {
			return "23"
		}
		return ""
	}
	if code, err := run(nil, &output, getenv, testGetwd(t)); err != nil || code != 23 {
		t.Fatalf("run = (%d, %v), want (23, nil)", code, err)
	}
	if _, err := configuredExitCode("126"); err == nil {
		t.Fatal("out-of-range fixture exit code was accepted")
	}
}

func TestPaneCommandsFireEveryInjectedHookWithControllablePayload(t *testing.T) {
	directory := t.TempDir()
	record := filepath.Join(directory, "hook-record.jsonl")
	hook := filepath.Join(directory, "injected-hook")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s|' \"$DECK_SESSION_ID\" >> %q\ncat >> %q\n", record, record)
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook recorder: %v", err)
	}

	events := []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "StopFailure", "SessionEnd"}
	hooks := make(map[string]any, len(events))
	var input strings.Builder
	for index, event := range events {
		hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook}}}}
		command := map[string]any{
			"command": "hook",
			"event":   event,
			"payload": map[string]any{"session_id": firstUUID, "controlled": index, "kind": event},
		}
		encoded, err := json.Marshal(command)
		if err != nil {
			t.Fatalf("encode command: %v", err)
		}
		input.Write(encoded)
		input.WriteByte('\n')
	}
	settings, err := json.Marshal(map[string]any{"hooks": hooks})
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}

	t.Setenv("DECK_SESSION_ID", "injected-deck-session")
	getenv := func(key string) string {
		switch key {
		case commandsEnvironment:
			return "1"
		case "HOME":
			return directory
		default:
			return ""
		}
	}
	var stdout, stderr bytes.Buffer
	code, err := runWithIO([]string{"--settings", string(settings)}, strings.NewReader(input.String()), &stdout, &stderr, getenv, testGetwd(t))
	if err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
	}

	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read hook record: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(lines) != len(events) {
		t.Fatalf("recorded %d hook calls, want %d: %q", len(lines), len(events), recorded)
	}
	for index, line := range lines {
		const inherited = "injected-deck-session|"
		if !strings.HasPrefix(line, inherited) {
			t.Fatalf("hook %d did not inherit deck's pane environment: %q", index, line)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, inherited)), &payload); err != nil {
			t.Fatalf("decode payload %d: %v", index, err)
		}
		if payload["hook_event_name"] != events[index] || payload["kind"] != events[index] || payload["controlled"] != float64(index) {
			t.Fatalf("payload %d = %#v", index, payload)
		}
		if !strings.Contains(stdout.String(), "fake-claude hook fired: "+events[index]) {
			t.Fatalf("stdout does not acknowledge %s: %q", events[index], stdout.String())
		}
	}
}

func TestPaneHookCommandRequiresInjectedSettingsRatherThanCallingDeckDirectly(t *testing.T) {
	input := `{"command":"hook","event":"SessionStart","payload":{"session_id":"controlled"}}` + "\n"
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	_, err := runWithIO(nil, strings.NewReader(input), &stdout, &stderr, getenv, testGetwd(t))
	if err == nil || !strings.Contains(err.Error(), `was not injected in --settings`) {
		t.Fatalf("runWithIO error = %v, want missing injected hook", err)
	}
}

func TestPaneFixtureCommandRendersNamedFileByteForByte(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "claude"), 0o755); err != nil {
		t.Fatalf("create corpus directory: %v", err)
	}
	want := []byte("first line\r\n\x1b[35mwaiting\x1b[0m") // Deliberately no trailing newline.
	if err := os.WriteFile(filepath.Join(directory, "claude", "waiting.txt"), want, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	input := strings.NewReader(`{"command":"fixture","name":"claude/waiting.txt"}` + "\n")
	getenv := func(key string) string {
		switch key {
		case commandsEnvironment:
			return "1"
		case fixtureDirectoryEnvironment:
			return directory
		}
		return ""
	}
	var output bytes.Buffer
	if code, err := runWithIO(nil, input, &output, io.Discard, getenv, testGetwd(t)); err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v)", code, err)
	}
	got := bytes.TrimPrefix(output.Bytes(), []byte("Fake Claude Code\nfake-claude argv: null\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("rendered bytes = %q, want %q (complete output %q)", got, want, output.Bytes())
	}
}

func TestPaneFixtureCommandRejectsNamesOutsideCorpus(t *testing.T) {
	var output bytes.Buffer
	err := runCommands(strings.NewReader(`{"command":"fixture","name":"../secret"}`+"\n"), &output, io.Discard, "", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "invalid fixture name") {
		t.Fatalf("runCommands error = %v, want invalid fixture name", err)
	}
}

func TestPaneResumeCommandFiresSessionEndThenSessionStartWithNewConversation(t *testing.T) {
	directory := t.TempDir()
	record := filepath.Join(directory, "hook-record.jsonl")
	hook := filepath.Join(directory, "injected-hook")
	script := fmt.Sprintf("#!/bin/sh\ncat >> %q\n", record)
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook recorder: %v", err)
	}

	hooks := map[string]any{
		"SessionEnd":   []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook}}}},
		"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook}}}},
	}
	settings, err := json.Marshal(map[string]any{"hooks": hooks})
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}

	command := map[string]any{"command": "resume", "old_session_id": firstUUID}
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}

	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	code, err := runWithIO([]string{"--settings", string(settings)}, bytes.NewReader(append(encoded, '\n')), &stdout, &stderr, getenv, testGetwd(t))
	if err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
	}

	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read hook record: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(lines) != 2 {
		t.Fatalf("recorded %d hook calls, want 2: %q", len(lines), recorded)
	}

	var end, start map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &end); err != nil {
		t.Fatalf("decode SessionEnd payload: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &start); err != nil {
		t.Fatalf("decode SessionStart payload: %v", err)
	}

	if end["hook_event_name"] != "SessionEnd" || end["reason"] != "resume" || end["session_id"] != firstUUID {
		t.Fatalf("SessionEnd payload = %#v", end)
	}
	if start["hook_event_name"] != "SessionStart" || start["reason"] != "resume" {
		t.Fatalf("SessionStart payload = %#v", start)
	}
	newID, ok := start["session_id"].(string)
	if !ok || newID == "" || newID == firstUUID {
		t.Fatalf("SessionStart session_id = %#v, want a fresh non-empty id", start["session_id"])
	}

	wantAnnouncement := fmt.Sprintf("fake-claude resume: %s -> %s", firstUUID, newID)
	if !strings.Contains(stdout.String(), wantAnnouncement) {
		t.Fatalf("stdout %q does not contain %q", stdout.String(), wantAnnouncement)
	}
}

func TestPaneResumeCommandAcceptsAnExplicitNewConversationID(t *testing.T) {
	directory := t.TempDir()
	record := filepath.Join(directory, "hook-record.jsonl")
	hook := filepath.Join(directory, "injected-hook")
	script := fmt.Sprintf("#!/bin/sh\ncat >> %q\n", record)
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook recorder: %v", err)
	}

	hooks := map[string]any{
		"SessionEnd":   []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook}}}},
		"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook}}}},
	}
	settings, err := json.Marshal(map[string]any{"hooks": hooks})
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}

	command := map[string]any{"command": "resume", "old_session_id": firstUUID, "new_session_id": secondUUID}
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}

	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	if code, err := runWithIO([]string{"--settings", string(settings)}, bytes.NewReader(append(encoded, '\n')), &stdout, &stderr, getenv, testGetwd(t)); err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
	}

	wantAnnouncement := fmt.Sprintf("fake-claude resume: %s -> %s", firstUUID, secondUUID)
	if !strings.Contains(stdout.String(), wantAnnouncement) {
		t.Fatalf("stdout %q does not contain %q", stdout.String(), wantAnnouncement)
	}

	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read hook record: %v", err)
	}
	if !strings.Contains(string(recorded), `"session_id":"`+secondUUID+`"`) {
		t.Fatalf("hook record %q does not carry the explicit new conversation id", recorded)
	}
}

func TestPaneResumeCommandRequiresOldConversationID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runCommands(strings.NewReader(`{"command":"resume","new_session_id":"`+secondUUID+`"}`+"\n"), &stdout, &stderr, "", "")
	if err == nil || !strings.Contains(err.Error(), "old_session_id") {
		t.Fatalf("runCommands error = %v, want missing old_session_id", err)
	}
}

func TestResumeReplaysOnlyItsOwnConversationsLastMessage(t *testing.T) {
	home := t.TempDir()
	getenv := testGetenv(home)
	getwd := testGetwd(t) // one shared cwd for both conversations

	var out bytes.Buffer
	if _, err := run([]string{"--session-id", firstUUID, "hello from alpha"}, &out, getenv, getwd); err != nil {
		t.Fatalf("launch alpha: %v", err)
	}
	out.Reset()
	if _, err := run([]string{"--session-id", secondUUID, "hello from beta"}, &out, getenv, getwd); err != nil {
		t.Fatalf("launch beta: %v", err)
	}

	out.Reset()
	if _, err := run([]string{"--resume", firstUUID}, &out, getenv, getwd); err != nil {
		t.Fatalf("resume alpha: %v", err)
	}
	if !strings.Contains(out.String(), "fake-claude replay: hello from alpha") {
		t.Fatalf("resume alpha output %q does not replay alpha's own message", out.String())
	}
	if strings.Contains(out.String(), "beta") {
		t.Fatalf("resume alpha output %q leaked beta's message", out.String())
	}

	out.Reset()
	if _, err := run([]string{"--resume", secondUUID}, &out, getenv, getwd); err != nil {
		t.Fatalf("resume beta: %v", err)
	}
	if !strings.Contains(out.String(), "fake-claude replay: hello from beta") {
		t.Fatalf("resume beta output %q does not replay beta's own message", out.String())
	}
	if strings.Contains(out.String(), "alpha") {
		t.Fatalf("resume beta output %q leaked alpha's message", out.String())
	}

	cwd, err := getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	if _, err := os.Stat(filepath.Join(home, ".claude", "projects", project, firstUUID+".jsonl")); err != nil {
		t.Fatalf("expected transcript file for alpha at the real Claude path: %v", err)
	}
}

// lockedBuffer is a concurrency-safe io.Writer/observer for tests that drive
// runRepaintFixture/watchAndRepaint from one goroutine while asserting on the
// written bytes from another -- same idiom as internal/tmux/tmux_test.go's
// own lockedBuffer.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// waitUntil polls fn until it returns true or the deadline passes, returning
// whether it ever became true. Used below to assert a repaint eventually
// happened (or, with a short deadline, to give one a real chance to happen
// before asserting it did not).
func waitUntil(deadline time.Duration, fn func() bool) bool {
	end := time.Now().Add(deadline)
	for {
		if fn() {
			return true
		}
		if time.Now().After(end) {
			return fn()
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRepaintModesProduceDistinguishingObservables drives all three
// repaint behaviours requirement 1 (II-1) asks for and asserts the
// distinguishing observable for each: whether and when a "repaint #N" line
// appears in the fixture's own output after a SIGWINCH and after a byte
// read from stdin (standing in for a forwarded keystroke). It registers its
// own SIGWINCH channel and calls watchAndRepaint directly (rather than
// runRepaintFixture, which registers internally) specifically so the
// registration is guaranteed complete, synchronously, before this test
// sends the signal -- see watchAndRepaint's own doc comment.
func TestRepaintModesProduceDistinguishingObservables(t *testing.T) {
	sendSIGWINCH := func(t *testing.T) {
		t.Helper()
		if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
			t.Fatalf("send SIGWINCH to self: %v", err)
		}
	}

	t.Run("sigwinch mode repaints immediately on SIGWINCH, not on a keystroke", func(t *testing.T) {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGWINCH)
		t.Cleanup(func() { signal.Stop(signals) })

		var output lockedBuffer
		stdinReader, stdinWriter := io.Pipe()
		result := make(chan error, 1)
		go func() { result <- watchAndRepaint(repaintModeSigwinch, stdinReader, &output, signals) }()
		t.Cleanup(func() { stdinWriter.Close() })

		sendSIGWINCH(t)
		if !waitUntil(time.Second, func() bool { return strings.Contains(output.String(), "repaint #1") }) {
			t.Fatalf("sigwinch mode never repainted after SIGWINCH; output = %q", output.String())
		}

		// A keystroke alone must not repaint again in this mode.
		if _, err := stdinWriter.Write([]byte("x")); err != nil {
			t.Fatalf("write keystroke: %v", err)
		}
		if waitUntil(150*time.Millisecond, func() bool { return strings.Contains(output.String(), "repaint #2") }) {
			t.Fatalf("sigwinch mode repainted again on a bare keystroke; output = %q", output.String())
		}
	})

	t.Run("keystroke mode ignores SIGWINCH until the next byte read from stdin", func(t *testing.T) {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGWINCH)
		t.Cleanup(func() { signal.Stop(signals) })

		var output lockedBuffer
		stdinReader, stdinWriter := io.Pipe()
		result := make(chan error, 1)
		go func() { result <- watchAndRepaint(repaintModeKeystroke, stdinReader, &output, signals) }()
		t.Cleanup(func() { stdinWriter.Close() })

		// A keystroke with no preceding SIGWINCH must not repaint.
		if _, err := stdinWriter.Write([]byte("a")); err != nil {
			t.Fatalf("write keystroke: %v", err)
		}
		if waitUntil(150*time.Millisecond, func() bool { return output.String() != "" }) {
			t.Fatalf("keystroke mode repainted on a keystroke with no preceding SIGWINCH; output = %q", output.String())
		}

		sendSIGWINCH(t)
		// The SIGWINCH alone must not repaint yet.
		if waitUntil(150*time.Millisecond, func() bool { return output.String() != "" }) {
			t.Fatalf("keystroke mode repainted on SIGWINCH alone; output = %q", output.String())
		}

		if _, err := stdinWriter.Write([]byte("b")); err != nil {
			t.Fatalf("write keystroke: %v", err)
		}
		if !waitUntil(time.Second, func() bool { return strings.Contains(output.String(), "repaint #1") }) {
			t.Fatalf("keystroke mode never repainted on the keystroke following SIGWINCH; output = %q", output.String())
		}
	})

	t.Run("never mode never repaints, whatever arrives", func(t *testing.T) {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGWINCH)
		t.Cleanup(func() { signal.Stop(signals) })

		var output lockedBuffer
		stdinReader, stdinWriter := io.Pipe()
		result := make(chan error, 1)
		go func() { result <- watchAndRepaint(repaintModeNever, stdinReader, &output, signals) }()
		t.Cleanup(func() { stdinWriter.Close() })

		sendSIGWINCH(t)
		if _, err := stdinWriter.Write([]byte("c")); err != nil {
			t.Fatalf("write keystroke: %v", err)
		}
		if waitUntil(300*time.Millisecond, func() bool { return output.String() != "" }) {
			t.Fatalf("never mode repainted; output = %q", output.String())
		}
	})
}

// TestRepaintModeEnvironmentSelectsTheDedicatedFixtureAndRejectsUnknownValues
// proves the env-var dispatch runWithIO does, and that an invalid mode value
// is rejected rather than silently defaulting to one of the three.
func TestRepaintModeEnvironmentSelectsTheDedicatedFixtureAndRejectsUnknownValues(t *testing.T) {
	var output bytes.Buffer
	getenv := func(key string) string {
		if key == repaintModeEnvironment {
			return "not-a-real-mode"
		}
		return ""
	}
	if _, err := run(nil, &output, getenv, testGetwd(t)); err == nil {
		t.Fatal("invalid repaint mode was unexpectedly accepted")
	}

	// A recognised mode dispatches to the dedicated fixture: it prints no
	// Claude banner, and it does not hang the test -- stdin is already at
	// EOF (run()'s own empty reader), so it returns immediately.
	var neverOutput bytes.Buffer
	neverGetenv := func(key string) string {
		if key == repaintModeEnvironment {
			return repaintModeNever
		}
		return ""
	}
	if code, err := run(nil, &neverOutput, neverGetenv, testGetwd(t)); err != nil || code != 0 {
		t.Fatalf("run(never repaint mode) = (%d, %v), want (0, nil)", code, err)
	}
	if strings.Contains(neverOutput.String(), "Fake Claude Code") {
		t.Fatalf("repaint mode printed the normal Claude banner: %q", neverOutput.String())
	}
}
