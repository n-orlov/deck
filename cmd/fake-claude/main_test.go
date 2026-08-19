package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
