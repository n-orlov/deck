package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationIDPrefersSessionIDOverResume(t *testing.T) {
	for name, tc := range map[string]struct {
		options options
		want    string
	}{
		"session id wins": {options{sessionID: firstUUID, resume: secondUUID}, firstUUID},
		"resume alone":    {options{resume: secondUUID}, secondUUID},
		"neither":         {options{}, ""},
	} {
		if got := tc.options.conversationID(); got != tc.want {
			t.Errorf("%s: conversationID() = %q, want %q", name, got, tc.want)
		}
	}
}

func TestReplayAndRecordWithoutAConversationDoesNothing(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	if err := replayAndRecord(options{message: "orphan"}, testGetenv(home), testGetwd(t), &stdout); err != nil {
		t.Fatalf("replayAndRecord error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("printed %q with no conversation id", stdout.String())
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("wrote %d entries under HOME with no conversation id", len(entries))
	}
}

func TestReplayAndRecordReportsAnUnresolvableWorkingDirectory(t *testing.T) {
	boom := errors.New("no cwd")
	err := replayAndRecord(options{sessionID: firstUUID}, testGetenv(t.TempDir()), func() (string, error) { return "", boom }, &bytes.Buffer{})
	if !errors.Is(err, boom) {
		t.Fatalf("replayAndRecord error = %v, want the getwd failure", err)
	}
}

func TestReplayAndRecordReportsACorruptTranscript(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	path, err := transcriptPath(testGetenv(home), getwd, firstUUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err = replayAndRecord(options{resume: firstUUID, message: "next"}, testGetenv(home), getwd, &stdout)
	if err == nil || !strings.Contains(err.Error(), "decode transcript entry") {
		t.Fatalf("replayAndRecord error = %v, want the decode failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("printed %q although the transcript could not be read", stdout.String())
	}
	if got, _ := os.ReadFile(path); string(got) != "not json\n" {
		t.Fatalf("transcript = %q, want it untouched after the failed replay", got)
	}
}

func TestReplayAndRecordReportsAnUnwritableTranscript(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	path, err := transcriptPath(testGetenv(home), getwd, firstUUID)
	if err != nil {
		t.Fatal(err)
	}
	// A regular file where the project directory must go: nothing can be appended.
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(path), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err = replayAndRecord(options{sessionID: firstUUID, message: "hello"}, testGetenv(home), getwd, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "create transcript directory") {
		t.Fatalf("replayAndRecord error = %v, want the create-directory failure", err)
	}
}
