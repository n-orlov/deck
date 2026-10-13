package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

// runPane drives the fake's pane loop with the given input lines against a
// fresh Codex home and returns the home, the working directory and the output.
func runPane(t *testing.T, lines ...string) (home, cwd, output string) {
	t.Helper()
	home = t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{commandsEnvironment: "1", "CODEX_HOME": home}
	var stdout bytes.Buffer
	input := strings.Join(append(lines, `{"command":"exit"}`), "\n") + "\n"
	if _, err := runWithIO(nil, strings.NewReader(input), &stdout, &bytes.Buffer{}, func(k string) string { return env[k] }); err != nil {
		t.Fatalf("runWithIO: %v", err)
	}
	return home, cwd, stdout.String()
}

func threadName(t *testing.T, home string) (id string, name sql.NullString) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(home, stateFile)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.QueryRow(`SELECT id, name FROM threads`).Scan(&id, &name); err != nil {
		t.Fatalf("read threads: %v", err)
	}
	return id, name
}

func indexLines(t *testing.T, home string) []map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, sessionIndexFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var rec map[string]string
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("index line %q: %v", line, err)
		}
		lines = append(lines, rec)
	}
	return lines
}

// TestThreadNamesRecordShapes pins the records Codex writes (issue for this
// harness): threads.name NULL until named, a session_index.jsonl line per
// naming with id, thread_name and updated_at, and an OSC 0 pane title
// "<name> | <cwd basename>".
func TestThreadNamesRecordShapes(t *testing.T) {
	home, cwd, output := runPane(t,
		`{"command":"prompt","text":"hello"}`,
		`{"command":"ai-title","name":"Write flaky test haiku"}`,
	)
	id, name := threadName(t, home)
	if !name.Valid || name.String != "Write flaky test haiku" {
		t.Fatalf("threads.name = %v, want the generated title", name)
	}
	lines := indexLines(t, home)
	if len(lines) != 1 || lines[0]["id"] != id || lines[0]["thread_name"] != name.String || lines[0]["updated_at"] == "" {
		t.Fatalf("session index = %v, want one line for %s", lines, id)
	}
	want := "\x1b]0;Write flaky test haiku | " + filepath.Base(cwd) + "\x07"
	if !strings.Contains(output, want) {
		t.Fatalf("output %q does not set the pane title %q", output, want)
	}
}

// TestThreadNamesUnnamedThreadHasNullName: a thread nobody named has a NULL
// name and no index line, which deck reads as "none yet".
func TestThreadNamesUnnamedThreadHasNullName(t *testing.T) {
	home, _, _ := runPane(t, `{"command":"prompt","text":"hello"}`)
	if _, name := threadName(t, home); name.Valid {
		t.Fatalf("threads.name = %q, want NULL", name.String)
	}
	if lines := indexLines(t, home); len(lines) != 0 {
		t.Fatalf("session index = %v, want none", lines)
	}
}

// TestThreadNamesTypedRenameBeforeFirstPromptIsHeldThenPersisted: a typed
// /rename before the thread exists shows in the pane title at once, is
// persisted when the first prompt creates the thread, and the generated title
// that follows does not replace it.
func TestThreadNamesTypedRenameBeforeFirstPromptIsHeldThenPersisted(t *testing.T) {
	home, _, output := runPane(t,
		"/rename billing audit",
		`{"command":"prompt","text":"hello"}`,
		`{"command":"ai-title","name":"Some generated guess"}`,
	)
	if !strings.Contains(output, "\x1b]0;billing audit | ") {
		t.Fatalf("output %q did not set the pane title before the first prompt", output)
	}
	if _, name := threadName(t, home); name.String != "billing audit" {
		t.Fatalf("threads.name = %q, want the typed name", name.String)
	}
	if lines := indexLines(t, home); len(lines) != 1 || lines[0]["thread_name"] != "billing audit" {
		t.Fatalf("session index = %v, want the typed name only", lines)
	}
}

// TestThreadNamesRenameAfterGeneratedTitleWins: a human /rename after the
// generated title replaces it, and the latest index line is the typed name.
func TestThreadNamesRenameAfterGeneratedTitleWins(t *testing.T) {
	home, _, _ := runPane(t,
		`{"command":"prompt","text":"hello"}`,
		`{"command":"ai-title","name":"Generated"}`,
		"/rename Mine",
		`{"command":"ai-title","name":"Generated again"}`,
	)
	if _, name := threadName(t, home); name.String != "Mine" {
		t.Fatalf("threads.name = %q, want Mine", name.String)
	}
	lines := indexLines(t, home)
	if len(lines) != 2 || lines[1]["thread_name"] != "Mine" {
		t.Fatalf("session index = %v, want Generated then Mine", lines)
	}
}

// TestThreadNamesAreReadBackByDeck proves the fake's database is the shape
// deck's production reader expects, so the features read what Codex writes.
func TestThreadNamesAreReadBackByDeck(t *testing.T) {
	home, cwd, _ := runPane(t, `{"command":"prompt","text":"hello"}`, "/rename Read me")
	id, _ := threadName(t, home)
	name, _, ok := agent.NewCodex().ReadName(agent.NameProbeInput{Env: map[string]string{"CODEX_HOME": home}, CWD: cwd, ConversationID: id})
	if !ok || name != "Read me" {
		t.Fatalf("ReadName = (%q, %v), want Read me", name, ok)
	}
}

func TestThreadNamesPaneCommandsNeedTheirInputs(t *testing.T) {
	env := map[string]string{commandsEnvironment: "1", "CODEX_HOME": t.TempDir()}
	for _, line := range []string{`{"command":"ai-title","name":"x"}`, `{"command":"rename","name":" "}`} {
		_, err := runWithIO(nil, strings.NewReader(line+"\n"), &bytes.Buffer{}, &bytes.Buffer{}, func(k string) string { return env[k] })
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", line)
		}
	}
	if name, ok := typedRename("/rename   "); ok {
		t.Fatalf("a blank /rename was recognised as %q", name)
	}
}

// TestStopRepaintsTheScreenWithoutTheWorkingIndicator: after a turn ends the
// visible screen no longer carries the finished turn's "esc to interrupt", so
// deck's probe reads the composer as ready for typed text.
func TestStopRepaintsTheScreenWithoutTheWorkingIndicator(t *testing.T) {
	_, _, output := runPane(t, `{"command":"prompt","text":"hello"}`, `{"command":"stop","message":"done"}`)
	index := strings.LastIndex(output, clearScreen)
	if index < 0 {
		t.Fatalf("output %q never repainted the screen", output)
	}
	if after := output[index:]; strings.Contains(after, "esc to interrupt") || !strings.Contains(after, "• done") {
		t.Fatalf("screen after the stop = %q", after)
	}
}

// TestThreadNamesWriteWaitsForAReaderInsteadOfFailing: deck and a scenario read
// state_5.sqlite while the fake writes it. A write that overlaps a reader must
// wait for it, not fail with "database is locked" and take the pane down.
func TestThreadNamesWriteWaitsForAReaderInsteadOfFailing(t *testing.T) {
	home := t.TempDir()
	names := newThreadNames(home, &bytes.Buffer{})
	if err := names.start("thread-1", "/tmp/rollout.jsonl"); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("sqlite", "file:"+filepath.Join(home, stateFile)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	tx, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = tx.Rollback()
		close(released)
	}()
	if err := names.rename("Inside Codex"); err != nil {
		t.Fatalf("rename while a reader held the database: %v", err)
	}
	<-released
	if _, name := threadName(t, home); name.String != "Inside Codex" {
		t.Fatalf("threads.name = %q, want the renamed thread", name.String)
	}
}

// TestRenameDraftSurvivesACommandTypedBehindItAndALateEnter reproduces the
// pane input of the stale-binding codex scenario: deck's "/rename bound"
// draft sits on the composer line when the scenario types its permission
// command behind it, the first Enter submits both, and deck's own Enter
// arrives afterwards as an empty line. The permission command must run, the
// name must be applied, and the empty line must not end the pane.
func TestRenameDraftSurvivesACommandTypedBehindItAndALateEnter(t *testing.T) {
	_, _, output := runPane(t,
		`{"command":"prompt","text":"hello"}`,
		`/rename bound{"command":"permission","tool_name":"apply_patch"}`,
		``,
		`   `,
		`{"command":"stop","message":"done"}`,
	)
	for _, want := range []string{"Press enter to confirm or esc to cancel", "• done"} {
		if !strings.Contains(output, want) {
			t.Fatalf("pane output lacks %q; the glued command or the line after the empty one did not run:\n%s", want, output)
		}
	}
}

func TestSplitRenameDraft(t *testing.T) {
	for _, tc := range []struct{ line, name, command string }{
		{`/rename bound`, "bound", ""},
		{`/rename bound{"command":"stop"}`, "bound", `{"command":"stop"}`},
		{`{"command":"stop"}`, "", `{"command":"stop"}`},
		{``, "", ``},
	} {
		name, command := splitRenameDraft(tc.line)
		if name != tc.name || command != tc.command {
			t.Errorf("splitRenameDraft(%q) = %q, %q; want %q, %q", tc.line, name, command, tc.name, tc.command)
		}
	}
}
