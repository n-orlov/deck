package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func claudeNameFixture(name string) string {
	return filepath.Join("testdata", "names", "claude", name)
}

// rootedFixture copies a fixture into a fresh Claude projects root and returns
// the probe input whose hook transcript path names the copy, with the root as
// the session's CLAUDE_CONFIG_DIR: the hook's path is read only inside it.
func rootedFixture(t *testing.T, name string) NameProbeInput {
	t.Helper()
	src, err := os.ReadFile(claudeNameFixture(name))
	if err != nil {
		t.Fatal(err)
	}
	return rootedTranscript(t, src)
}

func rootedTranscript(t *testing.T, body []byte) NameProbeInput {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-work-proj")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "t.jsonl")
	if err := os.WriteFile(path, body, 0o600); err != nil { //nolint:gosec // G703: a fixture written under t.TempDir
		t.Fatal(err)
	}
	return NameProbeInput{TranscriptPath: path, Env: map[string]string{claudeConfigDirKey: root}}
}

// TestClaudeReadNameTranscriptRecordShapes: each title record shape is
// understood, the last one wins, and a partial last line is tolerated.
func TestClaudeReadNameTranscriptRecordShapes(t *testing.T) {
	for _, tc := range []struct {
		fixture  string
		name     string
		explicit bool
		ok       bool
	}{
		{"ai-title.jsonl", "Fix the parser", false, true},
		{"custom-title.jsonl", "my review", true, true},
		{"agent-name.jsonl", "planner", true, true},
		{"last-wins.jsonl", "Chosen later", true, true},
		{"truncated-tail.jsonl", "Complete record", false, true},
		{"no-title.jsonl", "", false, false},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			name, explicit, ok := NewClaude().ReadName(rootedFixture(t, tc.fixture))
			if name != tc.name || explicit != tc.explicit || ok != tc.ok {
				t.Fatalf("ReadName = (%q, %v, %v), want (%q, %v, %v)", name, explicit, ok, tc.name, tc.explicit, tc.ok)
			}
		})
	}
}

// TestClaudeReadNameReadsOnlyTheBoundedTail: a transcript far larger than the
// tail bound is read from its end (the last record wins) and no more than the
// bound is read.
func TestClaudeReadNameReadsOnlyTheBoundedTail(t *testing.T) {
	filler := `{"type":"user","message":"` + strings.Repeat("x", 1000) + `"}` + "\n"
	body := `{"type":"ai-title","aiTitle":"Ancient"}` + "\n" +
		strings.Repeat(filler, 2*claudeTailBytes/len(filler)) +
		`{"type":"ai-title","aiTitle":"Recent"}` + "\n"
	in := rootedTranscript(t, []byte(body))
	path := in.TranscriptPath
	var read int64
	restore := claudeOpen
	t.Cleanup(func() { claudeOpen = restore })
	claudeOpen = func(p string) (io.ReadSeekCloser, int64, error) {
		f, size, err := restore(p)
		return &countingReader{ReadSeekCloser: f, n: &read}, size, err
	}
	name, _, ok := NewClaude().ReadName(in)
	if !ok || name != "Recent" {
		t.Fatalf("ReadName = (%q, %v), want Recent", name, ok)
	}
	if read > claudeTailBytes {
		t.Fatalf("read %d bytes of a %d byte file, want at most %d", read, len(body), claudeTailBytes)
	}
	// With the recent record gone, the ancient one is out of the window.
	if err := os.WriteFile(path, []byte(strings.TrimSuffix(body, `{"type":"ai-title","aiTitle":"Recent"}`+"\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if name, _, ok := NewClaude().ReadName(in); ok {
		t.Fatalf("ReadName found %q beyond the tail bound", name)
	}
}

type countingReader struct {
	io.ReadSeekCloser
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.ReadSeekCloser.Read(p)
	*c.n += int64(n)
	return n, err
}

// TestClaudeReadNamePaneTitleFallback: with no usable transcript the pane title
// is read, its glyph prefix stripped and the Claude Code placeholder ignored.
func TestClaudeReadNamePaneTitleFallback(t *testing.T) {
	for _, tc := range []struct {
		title string
		name  string
		ok    bool
	}{
		{"✳ Fix the parser", "Fix the parser", true},
		{"⠂ Fix the parser", "Fix the parser", true},
		{"⠋ Fix the lexer", "Fix the lexer", true},
		{"Fix the parser", "", false}, // no glyph: tmux's host-name default, not Claude's title
		{"myhost", "", false},
		{"✳ Claude Code", "", false},
		{"Claude Code", "", false},
		{"✳ ", "", false},
		{"", "", false},
	} {
		in := NameProbeInput{TranscriptPath: "/nonexistent/x.jsonl", PaneTitle: func() string { return tc.title }}
		name, explicit, ok := NewClaude().ReadName(in)
		if name != tc.name || ok != tc.ok || explicit {
			t.Errorf("pane title %q: ReadName = (%q, %v, %v), want (%q, false, %v)", tc.title, name, explicit, ok, tc.name, tc.ok)
		}
	}
	if _, _, ok := NewClaude().ReadName(NameProbeInput{}); ok {
		t.Fatal("ReadName answered with no transcript and no pane")
	}
}

// TestClaudeReadNameTranscriptBeatsPaneTitle: a usable transcript title is
// used and the pane title is never read.
func TestClaudeReadNameTranscriptBeatsPaneTitle(t *testing.T) {
	in := rootedFixture(t, "ai-title.jsonl")
	in.PaneTitle = func() string { t.Error("pane title read although the transcript answered"); return "x" }
	if name, _, ok := NewClaude().ReadName(in); !ok || name != "Fix the parser" {
		t.Fatalf("ReadName = (%q, %v)", name, ok)
	}
}

// TestClaudeReadNameDerivesTheTranscriptWhenNoHookPath: with no hook path the
// conversation's own transcript is read.
func TestClaudeReadNameDerivesTheTranscriptWhenNoHookPath(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-work-proj")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(claudeNameFixture("custom-title.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nameTestUUID+".jsonl"), src, 0o600); err != nil { //nolint:gosec // G703: a fixture written under t.TempDir
		t.Fatal(err)
	}
	name, explicit, ok := NewClaude().ReadName(NameProbeInput{Home: home, CWD: "/work/proj", ConversationID: nameTestUUID})
	if !ok || name != "my review" || !explicit {
		t.Fatalf("ReadName = (%q, %v, %v)", name, explicit, ok)
	}
}

// TestClaudeReadNameNeverOpensSessionsPidJSON: ~/.claude/sessions/<pid>.json
// (which holds an auto-title Claude rewrites) is never opened, even when it is
// present and would answer.
func TestClaudeReadNameNeverOpensSessionsPidJSON(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "4242.json"), []byte(`{"pid":4242,"name":"From sessions json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var opened []string
	restore := claudeOpen
	t.Cleanup(func() { claudeOpen = restore })
	claudeOpen = func(p string) (io.ReadSeekCloser, int64, error) {
		opened = append(opened, p)
		return restore(p)
	}
	in := NameProbeInput{Home: home, CWD: "/work", ConversationID: nameTestUUID, PaneTitle: func() string { return "✳ From the pane" }}
	name, _, ok := NewClaude().ReadName(in)
	if !ok || name != "From the pane" {
		t.Fatalf("ReadName = (%q, %v), want the pane title", name, ok)
	}
	for _, p := range opened {
		if strings.Contains(p, string(filepath.Separator)+"sessions"+string(filepath.Separator)) || strings.HasSuffix(p, ".json") {
			t.Fatalf("ReadName opened %s", p)
		}
	}
}

// TestClaudeReadNameIsDispatchedThroughTheCaps: Claude claims AutoTitle and the
// hook, transcript and pane_title observation channels.
func TestClaudeReadNameIsDispatchedThroughTheCaps(t *testing.T) {
	caps := NewClaude().Capabilities().Name
	if !caps.AutoTitle || !caps.ClaimsRead() {
		t.Fatalf("Claude NameCaps = %#v", caps)
	}
	name, _, ok := ReadName(NewClaude(), rootedFixture(t, "ai-title.jsonl"))
	if !ok || name != "Fix the parser" {
		t.Fatalf("agent.ReadName = (%q, %v)", name, ok)
	}
}
