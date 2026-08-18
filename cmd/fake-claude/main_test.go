package main

import (
	"bytes"
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
