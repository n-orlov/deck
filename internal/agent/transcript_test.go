package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellTranscriptPathAlwaysDeclines proves shell has no notion of a
// transcript at all (Capabilities().HasTranscript is false): TranscriptPaths
// declines regardless of input, never guessing a path.
func TestShellTranscriptPathAlwaysDeclines(t *testing.T) {
	if NewShell().Capabilities().HasTranscript {
		t.Fatalf("Shell.Capabilities().HasTranscript = true, want false")
	}
	home := t.TempDir()
	path, ok := NewShell().TranscriptPaths(TranscriptInput{Home: home, CWD: "/tmp/work", ConversationID: "any-id"})
	if ok || path != "" {
		t.Fatalf("Shell.TranscriptPaths = (%q, %v), want (\"\", false)", path, ok)
	}
}

// TestClaudeTranscriptPathFindsRealFile proves Claude's TranscriptPaths
// resolves exactly the convention recorded in
// docs/reports/phase3-findings.md: $HOME/.claude/projects/<cwd with every
// separator replaced by "-">/<conversation id>.jsonl.
func TestClaudeTranscriptPathFindsRealFile(t *testing.T) {
	if !NewClaude().Capabilities().HasTranscript {
		t.Fatalf("Claude.Capabilities().HasTranscript = false, want true")
	}
	home := t.TempDir()
	cwd := "/tmp/deck-scenario/agent-session-cwd"
	id := "20d34654-9462-4781-8b14-680862724dc7"
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	dir := filepath.Join(home, ".claude", "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(want, []byte(`{"message":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := NewClaude().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: id})
	if !ok {
		t.Fatalf("Claude.TranscriptPaths ok = false, want true")
	}
	if got != want {
		t.Fatalf("Claude.TranscriptPaths = %q, want %q", got, want)
	}
}

// TestClaudeTranscriptPathMissingHomeDegrades proves an empty Home degrades
// to an explicit "cannot locate" result rather than an error or a guess.
func TestClaudeTranscriptPathMissingHomeDegrades(t *testing.T) {
	got, ok := NewClaude().TranscriptPaths(TranscriptInput{Home: "", CWD: "/tmp/work", ConversationID: "some-id"})
	if ok || got != "" {
		t.Fatalf("Claude.TranscriptPaths with empty Home = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestClaudeTranscriptPathNoMatchingFileDegrades proves a well-formed,
// deterministic path that simply does not exist on disk (no project
// directory, or no file for this id) degrades the same way a missing HOME
// does -- never an error.
func TestClaudeTranscriptPathNoMatchingFileDegrades(t *testing.T) {
	home := t.TempDir() // no .claude/projects/... created at all

	got, ok := NewClaude().TranscriptPaths(TranscriptInput{Home: home, CWD: "/tmp/nowhere", ConversationID: "missing-id"})
	if ok || got != "" {
		t.Fatalf("Claude.TranscriptPaths with no project directory = (%q, %v), want (\"\", false)", got, ok)
	}

	// Project directory exists, but no file for this id.
	cwd := "/tmp/deck-scenario/agent-session-cwd"
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects", project), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok = NewClaude().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: "no-such-id"})
	if ok || got != "" {
		t.Fatalf("Claude.TranscriptPaths with no matching file = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestPiTranscriptPathFindsRealFile proves Pi's TranscriptPaths resolves
// exactly the convention recorded in docs/reports/phase3-findings.md:
// directory $HOME/.pi/agent/sessions/--<encoded cwd>--, filename
// "<timestamp>_<conversation id>.jsonl" located by globbing the id suffix
// (the filename's timestamp component is not recomputable from the id
// alone).
func TestPiTranscriptPathFindsRealFile(t *testing.T) {
	if !NewPi().Capabilities().HasTranscript {
		t.Fatalf("Pi.Capabilities().HasTranscript = false, want true")
	}
	home := t.TempDir()
	cwd := "/tmp/pi-provenance/work"
	id := "43ac9425-9b54-4c5d-8063-ac52768d0cdb"
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--tmp-pi-provenance-work--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filename := "2026-08-22T15-06-00-661Z_" + id + ".jsonl"
	want := filepath.Join(dir, filename)
	header, err := json.Marshal(map[string]any{"type": "session", "id": id, "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, append(header, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	// A decoy file for a different conversation id must not be matched.
	if err := os.WriteFile(filepath.Join(dir, "2026-08-22T15-05-00-000Z_other-id.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: id})
	if !ok {
		t.Fatalf("Pi.TranscriptPaths ok = false, want true")
	}
	if got != want {
		t.Fatalf("Pi.TranscriptPaths = %q, want %q", got, want)
	}
}

// TestPiTranscriptPathMissingHomeDegrades proves an empty Home degrades to
// an explicit "cannot locate" result rather than an error or a guess.
func TestPiTranscriptPathMissingHomeDegrades(t *testing.T) {
	got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: "", CWD: "/tmp/work", ConversationID: "some-id"})
	if ok || got != "" {
		t.Fatalf("Pi.TranscriptPaths with empty Home = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestPiTranscriptPathNoMatchingFileDegrades proves both a wholly absent
// session directory and an existing directory with no id-matching file
// degrade to "cannot locate" -- never an error, never a glob outside the
// one directory the convention names.
func TestPiTranscriptPathNoMatchingFileDegrades(t *testing.T) {
	home := t.TempDir() // no .pi/agent/sessions/... created at all

	got, ok := NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: "/tmp/nowhere", ConversationID: "missing-id"})
	if ok || got != "" {
		t.Fatalf("Pi.TranscriptPaths with no session directory = (%q, %v), want (\"\", false)", got, ok)
	}

	cwd := "/tmp/pi-provenance/work"
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--tmp-pi-provenance-work--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-08-22T15-05-00-000Z_other-id.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok = NewPi().TranscriptPaths(TranscriptInput{Home: home, CWD: cwd, ConversationID: "no-such-id"})
	if ok || got != "" {
		t.Fatalf("Pi.TranscriptPaths with no matching file = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestCodexTranscriptPathFindsRealFileUnderDefaultHome proves Codex's
// TranscriptPaths resolves the SPEC §8.2 / codex-cli 0.154.0 spike
// convention (docs/reports/codex-cli-0.154.0-spike.md) when the caller
// supplies no session-level CODEX_HOME override: <home>/.codex/sessions/
// <yyyy>/<mm>/<dd>/rollout-<ISO>-<conversation id>.jsonl, located by
// globbing the date directories and the timestamp-bearing filename.
func TestCodexTranscriptPathFindsRealFileUnderDefaultHome(t *testing.T) {
	if !NewCodex().Capabilities().HasTranscript {
		t.Fatalf("Codex.Capabilities().HasTranscript = false, want true")
	}
	home := t.TempDir()
	id := "01a09616-150d-7252-959c-d72a289dae41"
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filename := "rollout-2026-09-12T15-47-04-" + id + ".jsonl"
	want := filepath.Join(dir, filename)
	if err := os.WriteFile(want, []byte(`{"session_id":"`+id+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A decoy file for a different conversation id must not be matched.
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-09-12T15-40-00-other-id.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, ConversationID: id})
	if !ok {
		t.Fatalf("Codex.TranscriptPaths ok = false, want true")
	}
	if got != want {
		t.Fatalf("Codex.TranscriptPaths = %q, want %q", got, want)
	}
}

// TestCodexTranscriptPathHonoursSessionCodexHomeOverride proves that when
// the caller resolved a session-level CODEX_HOME override (SPEC §6.1's env
// layering) and filled TranscriptInput.Env["CODEX_HOME"] with it, Codex
// searches
// that tree instead of <home>/.codex -- and never consults its own
// process's ambient $CODEX_HOME to do so (the adapter takes no such
// reading at all; only the caller-supplied field is consulted).
func TestCodexTranscriptPathHonoursSessionCodexHomeOverride(t *testing.T) {
	home := t.TempDir()      // <home>/.codex deliberately left empty/absent
	codexHome := t.TempDir() // the session's own CODEX_HOME override
	id := "43ac9425-9b54-4c5d-8063-ac52768d0cdb"
	dir := filepath.Join(codexHome, "sessions", "2026", "01", "05")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "rollout-2026-01-05T09-00-00-"+id+".jsonl")
	if err := os.WriteFile(want, []byte(`{}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, ConversationID: id, Env: map[string]string{"CODEX_HOME": codexHome}})
	if !ok {
		t.Fatalf("Codex.TranscriptPaths ok = false, want true")
	}
	if got != want {
		t.Fatalf("Codex.TranscriptPaths = %q, want %q", got, want)
	}

	// The same id under the default <home>/.codex tree (which has nothing
	// in it) must not be found -- proving the override actually redirected
	// the search rather than merely widening it.
	if _, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, ConversationID: id}); ok {
		t.Fatalf("Codex.TranscriptPaths without the override unexpectedly found a file under <home>/.codex")
	}
}

// TestCodexTranscriptPathMissDegrades proves a miss -- no Home and no
// CODEX_HOME, a well-formed tree with nothing matching the id, and an empty
// ConversationID -- always degrades to "cannot locate", never an error.
func TestCodexTranscriptPathMissDegrades(t *testing.T) {
	if got, ok := NewCodex().TranscriptPaths(TranscriptInput{ConversationID: "some-id"}); ok || got != "" {
		t.Fatalf("Codex.TranscriptPaths with no Home and no CODEX_HOME = (%q, %v), want (\"\", false)", got, ok)
	}

	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-09-12T15-47-04-other-id.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, ConversationID: "missing-id"}); ok || got != "" {
		t.Fatalf("Codex.TranscriptPaths with no matching file = (%q, %v), want (\"\", false)", got, ok)
	}

	if got, ok := NewCodex().TranscriptPaths(TranscriptInput{Home: home, ConversationID: ""}); ok || got != "" {
		t.Fatalf("Codex.TranscriptPaths with empty ConversationID = (%q, %v), want (\"\", false)", got, ok)
	}
}
