package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// R224 item 2: workspace.yaml at launch, events.jsonl not yet.
func TestWorkspaceYamlIsWrittenAtLaunchAndEventsAreNot(t *testing.T) {
	h := newHarness(t)
	got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), nil)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	data, err := os.ReadFile(filepath.Join(h.sessionDir(testSessionID), "workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: " + testSessionID + "\n", "cwd: " + h.cwd + "\n", "created_at: ", "updated_at: "} {
		if !strings.Contains(string(data), want) {
			t.Errorf("workspace.yaml %q lacks %q", data, want)
		}
	}
	info, err := os.Stat(h.sessionDir(testSessionID))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("session directory mode = %v (%v), want 0700", info, err)
	}
}

// Without commands mode the process exits at once, which is a clean shutdown.
func TestACleanExitWithNoPromptWritesTheShutdownTranscript(t *testing.T) {
	h := newHarness(t)
	h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), nil)
	got := eventTypes(t, filepath.Join(h.sessionDir(testSessionID), "events.jsonl"))
	if want := []string{"session.start", "session.model_change", "session.shutdown"}; !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// R224 item 2: events.jsonl appears at the first prompt, while the process lives.
func TestEventsAppearAtTheFirstPromptNotBefore(t *testing.T) {
	h := newHarness(t)
	cmd, stdin := h.startProcess(t, "--session-id", testSessionID)
	events := filepath.Join(h.sessionDir(testSessionID), "events.jsonl")
	waitFor(t, "workspace.yaml", func() bool { return h.exists(h.sessionDir(testSessionID), "workspace.yaml") })
	if h.exists(events) {
		t.Fatal("events.jsonl exists before any prompt")
	}
	if _, err := io.WriteString(stdin, `{"command":"prompt","text":"hello"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "events.jsonl", func() bool { return h.exists(events) })
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	got := eventTypes(t, events)
	if want := []string{"session.start", "session.model_change", "user.message", "assistant.turn_start", "assistant.message", "assistant.turn_end"}; !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// R224 item 2: an existing id resumes, and the first process's records stay.
func TestAnExistingIdResumes(t *testing.T) {
	h := newHarness(t)
	args := []string{"--session-id", testSessionID}
	first := h.run(args, strings.NewReader(`{"command":"prompt","text":"one"}`+"\n"), map[string]string{commandsEnvironment: "1"})
	if !strings.Contains(first.stdout, "fake-copilot session: new\n") {
		t.Fatalf("first launch banner %q", first.stdout)
	}
	workspace := filepath.Join(h.sessionDir(testSessionID), "workspace.yaml")
	before, err := os.ReadFile(workspace)
	if err != nil {
		t.Fatal(err)
	}
	second := h.run(args, strings.NewReader(`{"command":"prompt","text":"two"}`+"\n"), map[string]string{commandsEnvironment: "1"})
	if second.code != 0 || !strings.Contains(second.stdout, "fake-copilot session: resume\n") || !strings.Contains(second.stdout, "fake-copilot session-id: "+testSessionID+"\n") {
		t.Fatalf("second launch = (%d, %q), want a resume of the same id", second.code, second.stdout)
	}
	after, err := os.ReadFile(workspace)
	if err != nil || string(after) != string(before) {
		t.Fatalf("workspace.yaml changed on resume (err %v):\n%s\n--- was\n%s", err, after, before)
	}
	got := eventTypes(t, filepath.Join(h.sessionDir(testSessionID), "events.jsonl"))
	want := []string{
		"session.start", "session.model_change", "user.message", "assistant.turn_start", "assistant.message", "assistant.turn_end", "session.shutdown",
		"session.resume", "user.message", "assistant.turn_start", "assistant.message", "assistant.turn_end", "session.shutdown",
	}
	if !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// A launch killed before any message (workspace.yaml, no events.jsonl), relaunched
// on the same id, resumes it and writes a fresh transcript.
func TestAKilledBeforeFirstMessageSessionResumesOnTheSameId(t *testing.T) {
	h := newHarness(t)
	dir := h.sessionDir(testSessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "workspace.yaml"), "id: "+testSessionID+"\ncwd: /elsewhere\n")
	got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), nil)
	if got.code != 0 || !strings.Contains(got.stdout, "fake-copilot session: resume\n") {
		t.Fatalf("run = (%d, %q), want a resume", got.code, got.stdout)
	}
	types := eventTypes(t, filepath.Join(dir, "events.jsonl"))
	if want := []string{"session.start", "session.model_change", "session.shutdown"}; !equal(types, want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
}

// R224 item 2: a new UUID creates, a non-UUID exits 1.
func TestANewUUIDCreatesAndANonUUIDExitsOne(t *testing.T) {
	h := newHarness(t)
	if got := h.run([]string{"--session-id", "99999999-aaaa-4bbb-8ccc-dddddddddddd"}, strings.NewReader(""), nil); got.code != 0 || !strings.Contains(got.stdout, "session: new") {
		t.Fatalf("new uuid = (%d, %q)", got.code, got.stdout)
	}
	for _, id := range []string{"notauuid", "1111111-2222-4333-8444-555555555555", "11111111-2222-4333-8444-55555555555g", "../escape", ""} {
		h := newHarness(t)
		got := h.run([]string{"--session-id", id}, strings.NewReader(""), nil)
		wantHead := "Error: No session or task matched '" + id + "'.\nThe value is not a valid UUID, so a new session cannot be created.\n"
		if got.code != 1 || !strings.HasPrefix(got.stderr, wantHead) || got.stdout != "" {
			t.Errorf("--session-id %q = (%d, stderr %q, stdout %q), want exit 1 with %q", id, got.code, got.stderr, got.stdout, wantHead)
		}
		if h.exists(h.home) {
			t.Errorf("--session-id %q created a session directory", id)
		}
	}
}

func TestNoSessionIdMintsAValidOne(t *testing.T) {
	h := newHarness(t)
	got := h.run(nil, strings.NewReader(""), nil)
	match := regexp.MustCompile(`fake-copilot session-id: ([^\n]+)\n`).FindStringSubmatch(got.stdout)
	if got.code != 0 || match == nil || !uuidPattern.MatchString(match[1]) {
		t.Fatalf("run = (%d, %q), want a launch under a minted UUID", got.code, got.stdout)
	}
	if !h.exists(h.sessionDir(match[1]), "workspace.yaml") {
		t.Fatalf("no workspace.yaml for the minted id %s", match[1])
	}
}

func TestNewUUIDIsVersionFour(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		id := newUUID()
		if !uuidPattern.MatchString(id) || id[14] != '4' || strings.IndexByte("89ab", id[19]) < 0 || seen[id] {
			t.Fatalf("newUUID() = %q, want a fresh version 4 UUID", id)
		}
		seen[id] = true
	}
}

func TestEventsCarryTheRealRecordShape(t *testing.T) {
	h := newHarness(t)
	h.run([]string{"--session-id", testSessionID}, strings.NewReader(`{"command":"prompt","text":"x"}`+"\n"), map[string]string{commandsEnvironment: "1"})
	data, err := os.ReadFile(filepath.Join(h.sessionDir(testSessionID), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, key := range []string{`"type"`, `"data"`, `"id"`, `"timestamp"`, `"parentId"`} {
		if !strings.Contains(lines[0], key) {
			t.Errorf("first record %s lacks %s", lines[0], key)
		}
	}
	if !strings.Contains(lines[0], `"parentId":null`) || strings.Contains(lines[1], `"parentId":null`) {
		t.Errorf("parent chain wrong:\n%s\n%s", lines[0], lines[1])
	}
}

func TestEventsOnAnUnwritableDirectoryAreReported(t *testing.T) {
	h := newHarness(t)
	dir := h.sessionDir(testSessionID)
	if err := os.MkdirAll(filepath.Join(dir, "events.jsonl"), 0o700); err != nil { // a directory where the file must go
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "workspace.yaml"), "id: x\n")
	got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(`{"command":"prompt","text":"x"}`+"\n"), map[string]string{commandsEnvironment: "1"})
	if got.code != 2 || !strings.Contains(got.stderr, "open events.jsonl") {
		t.Fatalf("run = (%d, %q), want exit 2 naming events.jsonl", got.code, got.stderr)
	}
	clean := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), nil)
	if !strings.Contains(clean.stderr, "open events.jsonl") {
		t.Fatalf("a failing clean shutdown reported %q, want the events.jsonl error", clean.stderr)
	}
}
