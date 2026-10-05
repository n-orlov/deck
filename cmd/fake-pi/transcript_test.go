package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeOrCreateTranscriptCreatesAHeaderOnlyTranscriptForANewConversation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	var stdout bytes.Buffer
	path, err := resumeOrCreateTranscript(dir, "conv-1", piGetwd(t), &stdout)
	if err != nil {
		t.Fatalf("resumeOrCreateTranscript error = %v", err)
	}
	if !strings.HasSuffix(path, "_conv-1.jsonl") || filepath.Dir(path) != dir {
		t.Fatalf("path = %q, want <timestamp>_conv-1.jsonl in %q", path, dir)
	}
	if stdout.Len() != 0 {
		t.Fatalf("a new conversation replayed %q", stdout.String())
	}
	if body, _ := os.ReadFile(path); !strings.Contains(string(body), `"type":"session"`) || strings.Count(string(body), "\n") != 1 {
		t.Fatalf("transcript = %q, want exactly the session header line", body)
	}
}

func TestResumeOrCreateTranscriptReplaysAnExistingConversationsLastMessage(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "2026-01-01T00-00-00-000Z_conv-2.jsonl")
	if err := os.WriteFile(existing, []byte(`{"message":"first"}`+"\n"+`{"message":"second"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	path, err := resumeOrCreateTranscript(dir, "conv-2", piGetwd(t), &stdout)
	if err != nil || path != existing {
		t.Fatalf("resumeOrCreateTranscript = (%q, %v), want the existing %q", path, err, existing)
	}
	if got, want := stdout.String(), "fake-pi replay: second\n"; got != want {
		t.Fatalf("replay = %q, want %q", got, want)
	}
}

func TestResumeOrCreateTranscriptSkipsLinesThatAreNotMessageEntries(t *testing.T) {
	dir := t.TempDir()
	body := `{"type":"session","id":"conv-3"}` + "\nnot json\n\n" + `{"message":"kept"}` + "\nalso not json\n"
	if err := os.WriteFile(filepath.Join(dir, "x_conv-3.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if _, err := resumeOrCreateTranscript(dir, "conv-3", piGetwd(t), &stdout); err != nil {
		t.Fatalf("error = %v", err)
	}
	if got, want := stdout.String(), "fake-pi replay: kept\n"; got != want {
		t.Fatalf("replay = %q, want %q", got, want)
	}
}

func TestResumeOrCreateTranscriptReportsAnUnreadableTranscript(t *testing.T) {
	dir := t.TempDir()
	// A single line past bufio.Scanner's token limit cannot be read back.
	if err := os.WriteFile(filepath.Join(dir, "x_conv-5.jsonl"), []byte(strings.Repeat("a", 200_000)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	_, err := resumeOrCreateTranscript(dir, "conv-5", piGetwd(t), &stdout)
	if err == nil || !strings.Contains(err.Error(), "read transcript") {
		t.Fatalf("error = %v, want the read failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("printed %q although the transcript could not be read", stdout.String())
	}
}

func TestResumeOrCreateTranscriptReportsAnUnresolvableWorkingDirectory(t *testing.T) {
	boom := errors.New("no cwd")
	_, err := resumeOrCreateTranscript(t.TempDir(), "conv-4", func() (string, error) { return "", boom }, &bytes.Buffer{})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the getwd failure", err)
	}
}
