package main

import (
	"bytes"
	"strings"
	"testing"
)

const firstUUID = "123e4567-e89b-12d3-a456-426614174000"
const secondUUID = "123e4567-e89b-12d3-a456-426614174001"

func TestAcceptedClaudeFlagsProduceDeterministicTerminalRecord(t *testing.T) {
	var first, second bytes.Buffer
	arguments := []string{"--session-id", firstUUID, "--resume", secondUUID, "--permission-mode", "acceptEdits", "write tests"}
	getenv := func(string) string { return "" }
	if code, err := run(arguments, &first, getenv); err != nil || code != 0 {
		t.Fatalf("first run = (%d, %v)", code, err)
	}
	if code, err := run(arguments, &second, getenv); err != nil || code != 0 {
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
			if _, err := run(arguments, &output, func(string) string { return "" }); err == nil {
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
	if code, err := run(nil, &output, getenv); err != nil || code != 23 {
		t.Fatalf("run = (%d, %v), want (23, nil)", code, err)
	}
	if _, err := configuredExitCode("126"); err == nil {
		t.Fatal("out-of-range fixture exit code was accepted")
	}
}
