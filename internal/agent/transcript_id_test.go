package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unsafeConversationIDs are the hook-supplied ids no adapter may turn into a
// transcript path: empty, a bare separator, "..", "a/b" and ".".
var unsafeConversationIDs = []struct{ name, id string }{
	{"empty", ""},
	{"slash", "/"},
	{"dotdot", ".."},
	{"a-slash-b", "a/b"},
	{"dot", "."},
	{"backslash", `a\b`},
	{"escape", "../../outside"},
}

func writeDecoy(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"m":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSafeConversationID(t *testing.T) {
	for _, tc := range unsafeConversationIDs {
		if safeConversationID(tc.id) {
			t.Errorf("safeConversationID(%q) [%s] = true, want false", tc.id, tc.name)
		}
	}
	for _, id := range []string{"20d34654-9462-4781-8b14-680862724dc7", "abc", "a.b", "..a", "a.."} {
		if !safeConversationID(id) {
			t.Errorf("safeConversationID(%q) = false, want true", id)
		}
	}
}

// TestClaudeTranscriptPathRejectsUnsafeIDs plants a transcript at exactly
// the path each unsafe id would build, so only the id validation can make
// the lookup decline.
func TestClaudeTranscriptPathRejectsUnsafeIDs(t *testing.T) {
	cwd := "/tmp/work"
	for _, tc := range unsafeConversationIDs {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			project := filepath.Join(home, ".claude", "projects", "-tmp-work")
			writeDecoy(t, filepath.Join(project, tc.id+".jsonl"))
			got, ok := NewClaude().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: tc.id})
			if ok || got != "" {
				t.Fatalf("Claude.TranscriptPaths(%q) = (%q, %v), want (\"\", false)", tc.id, got, ok)
			}
			if NewClaude().RelaunchFresh(TranscriptInput{Home: home, CWD: cwd, ConversationID: tc.id}) {
				t.Fatalf("Claude.RelaunchFresh(%q) = true, want false", tc.id)
			}
		})
	}
}

func TestPiTranscriptPathRejectsUnsafeIDs(t *testing.T) {
	cwd := "/tmp/work"
	for _, tc := range unsafeConversationIDs {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".pi", "agent", "sessions", piEncodeCwd(cwd))
			// An entry whose name ends in "_<id>.jsonl" (when the id has no
			// separator) is what the suffix match would pick up.
			writeDecoy(t, filepath.Join(dir, "2026-01-01T00-00-00-000Z_"+filepath.Base(tc.id)+".jsonl"))
			writeDecoy(t, filepath.Join(dir, "2026-01-01T00-00-00-000Z_"+tc.id+".jsonl"))
			reads := recordPiReadDir(t)
			got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: tc.id})
			if ok || got != "" {
				t.Fatalf("Pi.TranscriptPaths(%q) = (%q, %v), want (\"\", false)", tc.id, got, ok)
			}
			// A separator-holding id can never match an entry name, so the
			// result alone cannot tell a validated decline from a suffix miss:
			// the id must be refused before the directory is read at all.
			if len(*reads) != 0 {
				t.Fatalf("Pi.TranscriptPaths(%q) read %v, want no transcript-directory access", tc.id, *reads)
			}
		})
	}
}

func TestCodexTranscriptPathRejectsUnsafeIDs(t *testing.T) {
	for _, tc := range unsafeConversationIDs {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			day := filepath.Join(home, ".codex", "sessions", "2026", "01", "01")
			writeDecoy(t, filepath.Join(day, "rollout-2026-01-01T00-00-00-"+tc.id+".jsonl"))
			// Glob reads a backslash as an escape, so `a\b` would match "...-ab.jsonl".
			writeDecoy(t, filepath.Join(day, "rollout-2026-01-01T00-00-00-"+strings.ReplaceAll(tc.id, `\`, "")+".jsonl"))
			got, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, CWD: "/tmp/work", ConversationID: tc.id})
			if ok || got != "" {
				t.Fatalf("Codex.TranscriptPaths(%q) = (%q, %v), want (\"\", false)", tc.id, got, ok)
			}
		})
	}
}

// recordPiReadDir swaps piReadDir for a recorder that still reads the real
// directory, restoring it when the test ends, and returns the paths read.
func recordPiReadDir(t *testing.T) *[]string {
	t.Helper()
	var reads []string
	orig := piReadDir
	piReadDir = func(dir string) ([]os.DirEntry, error) {
		reads = append(reads, dir)
		return orig(dir)
	}
	t.Cleanup(func() { piReadDir = orig })
	return &reads
}

// TestPiTranscriptPathReadsDirForSafeID keeps the read recorder honest: a
// valid id does read the transcript directory and resolves the same file.
func TestPiTranscriptPathReadsDirForSafeID(t *testing.T) {
	cwd := "/tmp/work"
	home := t.TempDir()
	dir := filepath.Join(home, ".pi", "agent", "sessions", piEncodeCwd(cwd))
	want := filepath.Join(dir, "2026-01-01T00-00-00-000Z_abc-123.jsonl")
	writeDecoy(t, want)
	reads := recordPiReadDir(t)
	got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: "abc-123"})
	if !ok || got != want {
		t.Fatalf("Pi.TranscriptPaths(abc-123) = (%q, %v), want (%q, true)", got, ok, want)
	}
	if len(*reads) != 1 || (*reads)[0] != dir {
		t.Fatalf("Pi.TranscriptPaths(abc-123) read %v, want exactly [%s]", *reads, dir)
	}
}
