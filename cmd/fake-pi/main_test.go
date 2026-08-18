package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAcceptedPiFlagsProduceDeterministicTerminalRecord(t *testing.T) {
	var first, second bytes.Buffer
	arguments := []string{"--session-id", "abc-123", "--approve", "do the thing"}
	if code, err := run(arguments, &first); err != nil || code != 0 {
		t.Fatalf("first run = (%d, %v)", code, err)
	}
	if code, err := run(arguments, &second); err != nil || code != 0 {
		t.Fatalf("second run = (%d, %v)", code, err)
	}
	if first.String() != second.String() {
		t.Fatalf("terminal output is not deterministic:\nfirst: %q\nsecond: %q", first.String(), second.String())
	}
	for _, want := range []string{
		"Fake pi",
		`fake-pi argv: ["--session-id","abc-123","--approve","do the thing"]`,
		"fake-pi session-id: abc-123",
		"fake-pi approve: true",
	} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("output %q does not contain %q", first.String(), want)
		}
	}
}

func TestRejectsUnknownFlags(t *testing.T) {
	for name, arguments := range map[string][]string{
		"unknown long flag":  {"--not-a-real-flag", "value"},
		"unknown short flag": {"-x"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run(arguments, &output); err == nil {
				t.Fatalf("run(%q) unexpectedly succeeded", arguments)
			}
		})
	}
}

func TestSessionIDMissingValueIsRejected(t *testing.T) {
	var output bytes.Buffer
	if _, err := run([]string{"--session-id"}, &output); err == nil {
		t.Fatal("run with --session-id and no value unexpectedly succeeded")
	}
}

func TestExitStatusIsControlledOnlyByFixtureEnvironment(t *testing.T) {
	var output bytes.Buffer
	t.Setenv(exitCodeEnvironment, "23")
	if code, err := run(nil, &output); err != nil || code != 23 {
		t.Fatalf("run = (%d, %v), want (23, nil)", code, err)
	}

	t.Setenv(exitCodeEnvironment, "126")
	if _, err := configuredExitCode("126"); err == nil {
		t.Fatal("out-of-range fixture exit code was accepted")
	}
}

func TestSessionIDCreatesSessionIfMissing(t *testing.T) {
	// fake-pi has no persistent session store: accepting --session-id with any
	// id is itself the "created if missing" behavior deck relies on (pi
	// creates the conversation implicitly on first use of an id).
	var output bytes.Buffer
	if code, err := run([]string{"--session-id", "brand-new-id"}, &output); err != nil || code != 0 {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	if !strings.Contains(output.String(), "fake-pi session-id: brand-new-id") {
		t.Fatalf("output %q missing session-id record", output.String())
	}
}
