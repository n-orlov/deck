package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
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

func TestAcceptedPiFlagsProduceDeterministicTerminalRecord(t *testing.T) {
	var first, second bytes.Buffer
	arguments := []string{"--session-id", "abc-123", "--approve", "do the thing"}
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
			if _, err := run(arguments, &output, testGetenv(t.TempDir()), testGetwd(t)); err == nil {
				t.Fatalf("run(%q) unexpectedly succeeded", arguments)
			}
		})
	}
}

func TestSessionIDMissingValueIsRejected(t *testing.T) {
	var output bytes.Buffer
	if _, err := run([]string{"--session-id"}, &output, testGetenv(t.TempDir()), testGetwd(t)); err == nil {
		t.Fatal("run with --session-id and no value unexpectedly succeeded")
	}
}

func TestExitStatusIsControlledOnlyByFixtureEnvironment(t *testing.T) {
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
	if code, err := run([]string{"--session-id", "brand-new-id"}, &output, testGetenv(t.TempDir()), testGetwd(t)); err != nil || code != 0 {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	if !strings.Contains(output.String(), "fake-pi session-id: brand-new-id") {
		t.Fatalf("output %q missing session-id record", output.String())
	}
}

func TestPaneFixtureCommandRendersNamedFileByteForByte(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "pi"), 0o755); err != nil {
		t.Fatalf("create corpus directory: %v", err)
	}
	want := []byte("⠹ working\r\n\x1b[2mctrl-c to stop\x1b[0m") // Deliberately no trailing newline.
	if err := os.WriteFile(filepath.Join(directory, "pi", "running.txt"), want, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	input := strings.NewReader(`{"command":"fixture","name":"pi/running.txt"}` + "\n")
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
	if code, err := runWithIO(nil, input, &output, getenv, testGetwd(t)); err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v)", code, err)
	}
	got := bytes.TrimPrefix(output.Bytes(), []byte("Fake pi\nfake-pi argv: null\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("rendered bytes = %q, want %q (complete output %q)", got, want, output.Bytes())
	}
}

// TestRenderedFixturesProbeToTheRealPiVerdict proves task 008's derivation
// claim directly, rather than by inspection: cmd/fake-pi has no fixture data
// of its own — FAKE_AGENT_FIXTURE_DIR is pointed at the exact directory
// internal/agent/probe_test.go's probeGoldens table already pins
// (internal/agent/testdata/probes/pi/*.txt, captured against a real pi
// binary per testdata/probes/pi-PROVENANCE.md). This test renders each real
// capture through the fake-pi binary's own "fixture" pane command exactly as
// a live scenario would, then feeds the rendered pane bytes to the real
// internal/agent.NewPi().Probe implementation (task 007's refit rules) and
// asserts the verdict matches probeGoldens — i.e. a fake-pi pane probes to
// the same verdict a real pi pane would, and (since every session starts at
// "starting") a session sampling this rendered pane leaves "starting" for
// that matched, real-capture-backed status.
func TestRenderedFixturesProbeToTheRealPiVerdict(t *testing.T) {
	corpus := filepath.Join("..", "..", "internal", "agent", "testdata", "probes")
	if _, err := os.Stat(filepath.Join(corpus, "pi")); err != nil {
		t.Fatalf("locate shared pi probe corpus: %v", err)
	}

	cases := []struct {
		fixture, wantStatus, wantReason string
	}{
		{"pi/running.txt", "running", "working indicator"},
		{"pi/error.txt", "error", "agent error"},
	}
	pi := agent.NewPi()
	for _, testCase := range cases {
		t.Run(testCase.fixture, func(t *testing.T) {
			input := strings.NewReader(`{"command":"fixture","name":"` + testCase.fixture + `"}` + "\n")
			getenv := func(key string) string {
				switch key {
				case commandsEnvironment:
					return "1"
				case fixtureDirectoryEnvironment:
					return corpus
				}
				return ""
			}
			var output bytes.Buffer
			// os.Getwd, not testGetwd(t): corpus above is a relative path resolved
			// against the process's real working directory, and args is nil here
			// (no --session-id), so getwd is never actually invoked by replayAndRecord.
			if code, err := runWithIO(nil, input, &output, getenv, os.Getwd); err != nil || code != 0 {
				t.Fatalf("runWithIO = (%d, %v)", code, err)
			}
			rendered := bytes.TrimPrefix(output.Bytes(), []byte("Fake pi\nfake-pi argv: null\n"))

			want, err := os.ReadFile(filepath.Join(corpus, testCase.fixture))
			if err != nil {
				t.Fatalf("read captured fixture: %v", err)
			}
			if !bytes.Equal(rendered, want) {
				t.Fatalf("fake-pi rendered %q, want the captured fixture %q byte-for-byte", rendered, want)
			}

			status, reason := pi.Probe(string(rendered))
			if status != testCase.wantStatus || reason != testCase.wantReason {
				t.Fatalf("Probe(fake-pi rendering of %s) = (%q, %q), want (%q, %q) — a fake-pi pane must probe to the same verdict a real pi pane would", testCase.fixture, status, reason, testCase.wantStatus, testCase.wantReason)
			}
			if status == "starting" {
				t.Fatalf("fixture %s unexpectedly kept the default starting status; task 008 requires a matched, real-capture-backed verdict", testCase.fixture)
			}
		})
	}
}

func TestPaneFixtureCommandNeedsConfiguredCorpus(t *testing.T) {
	var output bytes.Buffer
	err := runCommands(strings.NewReader(`{"command":"fixture","name":"pi/running.txt"}`+"\n"), &output, "")
	if err == nil || !strings.Contains(err.Error(), "FAKE_AGENT_FIXTURE_DIR is not set") {
		t.Fatalf("runCommands error = %v, want missing corpus", err)
	}

}

// TestTranscriptWrittenAtPisRealPathConvention proves requirement 4: a
// per-conversation transcript lands at the same path/naming convention a
// real pi CLI uses ($HOME/.pi/agent/sessions/--<encoded-cwd>--/<timestamp>_
// <id>.jsonl, per docs/reports/phase3-fake-pi-transcript-provenance.md's recorded capture of a
// real pi 0.84.1 binary), mirroring cmd/fake-claude's own
// TestResumeReplaysOnlyItsOwnConversationsLastMessage in structure.
func TestTranscriptWrittenAtPisRealPathConvention(t *testing.T) {
	home := t.TempDir()
	getenv := testGetenv(home)
	getwd := testGetwd(t)

	const sessionID = "11111111-1111-1111-1111-111111111111"
	var out bytes.Buffer
	if _, err := run([]string{"--session-id", sessionID, "hello from fake pi"}, &out, getenv, getwd); err != nil {
		t.Fatalf("launch: %v", err)
	}

	cwd, err := getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	trimmed := strings.TrimPrefix(cwd, string(filepath.Separator))
	encoded := strings.NewReplacer(string(filepath.Separator), "-", ":", "-").Replace(trimmed)
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--"+encoded+"--")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read session dir %s: %v", dir, err)
	}
	var transcript string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_"+sessionID+".jsonl") {
			transcript = filepath.Join(dir, entry.Name())
		}
	}
	if transcript == "" {
		t.Fatalf("no transcript file matching *_%s.jsonl in %s: entries %v", sessionID, dir, entries)
	}

	contents, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if !strings.Contains(string(contents), sessionID) {
		t.Fatalf("transcript header %q missing session id", contents)
	}
	if !strings.Contains(string(contents), "hello from fake pi") {
		t.Fatalf("transcript %q missing the recorded message", contents)
	}

	// Resuming with the same --session-id must find and append to the same
	// file rather than create a second one.
	out.Reset()
	if _, err := run([]string{"--session-id", sessionID, "second turn"}, &out, getenv, getwd); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(out.String(), "fake-pi replay: hello from fake pi") {
		t.Fatalf("resume output %q does not replay the prior message", out.String())
	}
	entriesAfter, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read session dir after resume: %v", err)
	}
	count := 0
	for _, entry := range entriesAfter {
		if strings.HasSuffix(entry.Name(), "_"+sessionID+".jsonl") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d transcript files for the same conversation id, want 1: %v", count, entriesAfter)
	}
}
