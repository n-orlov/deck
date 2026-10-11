package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// transcriptRecords reads every JSON line of the conversation's transcript.
func transcriptRecords(t *testing.T, home, cwd, id string) []map[string]any {
	t.Helper()
	path := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, "/", "-"), id+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	defer file.Close()
	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("transcript line %q: %v", scanner.Text(), err)
		}
		records = append(records, rec)
	}
	return records
}

func cwdOf(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

// TestClaudeNameLaunchPersistsCustomTitleAndAgentName pins the record shapes a
// real `claude --session-id <id> --name <n>` writes: a custom-title and a
// paired agent-name record keyed by sessionId, and the OSC 0 pane title with
// Claude's glyph.
func TestClaudeNameLaunchPersistsCustomTitleAndAgentName(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	var out bytes.Buffer
	if code, err := run([]string{"--session-id", firstUUID, "--name", "Billing audit", "first prompt"}, &out, testGetenv(home), getwd); err != nil || code != 0 {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	records := transcriptRecords(t, home, cwdOf(t), firstUUID)
	want := []map[string]any{
		{"type": "custom-title", "customTitle": "Billing audit", "sessionId": firstUUID},
		{"type": "agent-name", "agentName": "Billing audit", "sessionId": firstUUID},
		{"message": "first prompt"},
	}
	if len(records) != len(want) {
		t.Fatalf("records = %v, want %v", records, want)
	}
	for i := range want {
		if got, w := toJSON(t, records[i]), toJSON(t, want[i]); got != w {
			t.Fatalf("record %d = %s, want %s", i, got, w)
		}
	}
	if !strings.Contains(out.String(), "\x1b]0;✳ Billing audit\x07") {
		t.Fatalf("output %q lacks the pane title OSC", out.String())
	}
}

// TestClaudeNameWaitsForTheFirstPromptBeforeCreatingTheTranscript pins that a
// --name alone never creates the conversation's transcript (Claude writes it
// with the first prompt, which deck's fresh-versus-resume choice relies on),
// while the pane title already shows the name; the first UserPromptSubmit
// then writes the records.
func TestClaudeNameWaitsForTheFirstPromptBeforeCreatingTheTranscript(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	var out bytes.Buffer
	env := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return testGetenv(home)(key)
	}
	record := filepath.Join(t.TempDir(), "hook.json")
	settings := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"cat > ` + record + `"}]}]}}`
	input := `{"command":"hook","event":"UserPromptSubmit","payload":{"prompt":"hi"}}` + "\n"
	path := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwdOf(t), "/", "-"), firstUUID+".jsonl")
	args := []string{"--session-id", firstUUID, "--name", "Held", "--settings", settings}
	// Commands run before the process exits; check the file at EOF of stdin is
	// too late, so stop the loop after the hook with a reader that checks first.
	checked := &checkingReader{reader: strings.NewReader(input), onEOF: func() {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("transcript missing after the first prompt: %v", err)
		}
	}}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("transcript exists before launch")
	}
	if code, err := runWithIO(args, checked, &out, &bytes.Buffer{}, env, getwd); err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v)", code, err)
	}
	if !strings.Contains(out.String(), "\x1b]0;✳ Held\x07") {
		t.Fatalf("output %q lacks the pane title set at launch", out.String())
	}
	hook, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(hook, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["session_title"] != "Held" || payload["transcript_path"] != path {
		t.Fatalf("hook payload = %v, want session_title Held and transcript_path %s", payload, path)
	}
}

func TestClaudeNameWithoutAPromptLeavesNoTranscript(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	if _, err := run([]string{"--session-id", firstUUID, "--name", "Held"}, &bytes.Buffer{}, testGetenv(home), getwd); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwdOf(t), "/", "-"), firstUUID+".jsonl")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("transcript stat err = %v, want not exist: a name alone must not create the transcript", err)
	}
}

// checkingReader runs onEOF when its underlying reader is exhausted.
type checkingReader struct {
	reader *strings.Reader
	onEOF  func()
}

func (c *checkingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if err != nil {
		c.onEOF()
	}
	return n, err
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestClaudeNameCommandsRenameAndAutoTitle drives the pane commands: a
// generated title is an ai-title record that follows each change, a human
// /rename (typed line or command) writes custom-title and then suppresses any
// further generated title.
func TestClaudeNameCommandsRenameAndAutoTitle(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	t.Setenv("HOME", home)
	input := strings.Join([]string{
		`{"command":"ai-title","name":"First guess"}`,
		`{"command":"ai-title","name":"Second guess"}`,
		`/rename Typed by deck`,
		`{"command":"ai-title","name":"Ignored once named"}`,
		`{"command":"rename","name":"Renamed inside"}`,
		`{"command":"exit"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	env := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return testGetenv(home)(key)
	}
	if code, err := runWithIO([]string{"--session-id", firstUUID}, strings.NewReader(input), &out, &bytes.Buffer{}, env, getwd); err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v)", code, err)
	}
	var got []string
	for _, rec := range transcriptRecords(t, home, cwdOf(t), firstUUID) {
		switch rec["type"] {
		case "ai-title":
			got = append(got, "ai:"+rec["aiTitle"].(string))
		case "custom-title":
			got = append(got, "custom:"+rec["customTitle"].(string))
		case "agent-name":
			got = append(got, "agent:"+rec["agentName"].(string))
		}
	}
	want := []string{"ai:First guess", "ai:Second guess", "custom:Typed by deck", "agent:Typed by deck", "custom:Renamed inside", "agent:Renamed inside"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v, want %v", got, want)
	}
	for _, title := range []string{"✳ First guess", "✳ Second guess", "✳ Typed by deck", "✳ Renamed inside"} {
		if !strings.Contains(out.String(), "\x1b]0;"+title+"\x07") {
			t.Fatalf("output lacks pane title %q", title)
		}
	}
	if strings.Contains(out.String(), "Ignored once named") {
		t.Fatal("an explicit name must suppress generated titles")
	}
}

// TestClaudeNameHooksCarrySessionTitleOnlyForExplicitNames pins the hook
// payload rule: session_title rides SessionStart/UserPromptSubmit for an
// explicit name only, never a generated one, and never on Stop.
func TestClaudeNameHooksCarrySessionTitleOnlyForExplicitNames(t *testing.T) {
	cwd := t.TempDir()
	n := &names{sessionID: firstUUID, path: filepath.Join(cwd, "t.jsonl"), out: &bytes.Buffer{}}
	if err := n.generate("Generated"); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{}
	n.annotate("UserPromptSubmit", payload)
	if _, ok := payload["session_title"]; ok {
		t.Fatalf("a generated title rode the payload: %v", payload)
	}
	if payload["transcript_path"] != n.path {
		t.Fatalf("transcript_path = %v, want %s", payload["transcript_path"], n.path)
	}
	if err := n.rename("Chosen"); err != nil {
		t.Fatal(err)
	}
	for event, want := range map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "Stop": false} {
		payload := map[string]any{}
		n.annotate(event, payload)
		got := payload["session_title"] == "Chosen"
		if got != want {
			t.Fatalf("%s session_title present = %v, want %v (%v)", event, got, want, payload)
		}
	}
}

// TestClaudeNameSurvivesResume pins that a resumed conversation reopens with
// its last title: an explicit name keeps suppressing generated titles.
func TestClaudeNameSurvivesResume(t *testing.T) {
	home := t.TempDir()
	getwd := testGetwd(t)
	if _, err := run([]string{"--session-id", firstUUID, "--name", "Kept", "hello"}, &bytes.Buffer{}, testGetenv(home), getwd); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := run([]string{"--resume", firstUUID}, &out, testGetenv(home), getwd); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b]0;✳ Kept\x07") {
		t.Fatalf("resume output %q lacks the restored pane title", out.String())
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(got, "/", "-"), firstUUID+".jsonl")
	n := &names{sessionID: firstUUID, path: path, out: &bytes.Buffer{}}
	n.load()
	if err := n.generate("Generated after resume"); err != nil {
		t.Fatal(err)
	}
	for _, rec := range transcriptRecords(t, home, got, firstUUID) {
		if rec["type"] == "ai-title" {
			t.Fatalf("an explicit name survived resume yet an ai-title was written: %v", rec)
		}
	}
}

func TestClaudeNameCommandWithoutConversationIsRefused(t *testing.T) {
	var n *names
	if handled, err := n.nameCommand(fixtureCommand{Command: "rename", Name: "x"}); !handled || err == nil {
		t.Fatalf("nameCommand = (%v, %v), want a refusal", handled, err)
	}
	if handled, _ := n.nameCommand(fixtureCommand{Command: "hook"}); handled {
		t.Fatal("hook is not a name command")
	}
	if name, ok := typedRename("/rename   "); ok {
		t.Fatalf("blank typed rename accepted as %q", name)
	}
}
