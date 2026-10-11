package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// cwdShapeCases are the five spellings of one directory a session row's cwd
// can carry (R245): canonical, trailing slash, doubled trailing slash, "."
// element and ".." element.
var cwdShapeCases = []struct{ name, cwd string }{
	{"plain", "/a/b"},
	{"trailing slash", "/a/b/"},
	{"doubled trailing slash", "/a/b//"},
	{"dot element", "/a/./b"},
	{"dotdot element", "/a/b/../b"},
}

func TestPiTranscriptPathsCleansCWD(t *testing.T) {
	home := t.TempDir()
	const id = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	dir := filepath.Join(home, ".pi", "agent", "sessions", piEncodeCwd("/a/b"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "2026-01-02T03-04-05-678Z_"+id+".jsonl")
	if err := os.WriteFile(want, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cwdShapeCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: tc.cwd, ConversationID: id})
			if !ok || got != want {
				t.Fatalf("TranscriptPaths(%q) = %q, %v; want %q, true", tc.cwd, got, ok, want)
			}
		})
	}
}

func TestPiEncodeCwdCanonicalUnchanged(t *testing.T) {
	if got := piEncodeCwd("/a/b"); got != "--a-b--" {
		t.Fatalf("piEncodeCwd(/a/b) = %q, want --a-b--", got)
	}
}

// Codex and Copilot locate a transcript by conversation id alone, so the
// cwd's spelling cannot change the result. This holds without any code
// change; the test pins it.
func TestCodexAndCopilotTranscriptPathsIgnoreCWDShape(t *testing.T) {
	home := t.TempDir()
	const id = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	codexDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "02")
	copilotDir := filepath.Join(home, ".copilot", "session-state", id)
	for _, d := range []string{codexDir, copilotDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	codexWant := filepath.Join(codexDir, "rollout-2026-01-02T03-04-05-"+id+".jsonl")
	copilotWant := filepath.Join(copilotDir, "events.jsonl")
	for _, f := range []string{codexWant, copilotWant} {
		if err := os.WriteFile(f, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range cwdShapeCases {
		t.Run(tc.name, func(t *testing.T) {
			in := TranscriptInput{Home: home, CWD: tc.cwd, ConversationID: id}
			if got, ok := NewCodex().TranscriptPaths(in); !ok || got != codexWant {
				t.Fatalf("Codex TranscriptPaths(%q) = %q, %v; want %q, true", tc.cwd, got, ok, codexWant)
			}
			if got, ok := NewCopilot().TranscriptPaths(in); !ok || got != copilotWant {
				t.Fatalf("Copilot TranscriptPaths(%q) = %q, %v; want %q, true", tc.cwd, got, ok, copilotWant)
			}
		})
	}
}
